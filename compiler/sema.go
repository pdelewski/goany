package compiler

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
)

// SemaChecker performs semantic analysis to detect unsupported Go constructs.
//
// ============================================
// SECTION 1: Unsupported Go Features (Errors)
// ============================================
// - Pointers (*T, &x)
// - Defer statements
// - Goroutines (go keyword)
// - Channels (chan T)
// - Select statements
// - Goto and labels
// - Method receivers
// - Non-empty interfaces
// - Struct embedding (anonymous fields)
// - Init functions
// - Type switch statements
// - Package-level variable declarations
//
// Moved to LangSemaLoweringPass transforms:
// - Named return values → transformNamedReturns
// - iota constant enumeration → transformIotaExpansion
// - Variadic functions (...T) → transformVariadics
// - Map literals → direct emitter support
//
// ============================================
// SECTION 2: Backend-Specific Constraints
// ============================================
// - Range over inline composite literals
// - Nil comparisons (== nil, != nil)
// - Struct field initialization order (C++ designated initializers)
// - Collection mutation during iteration (Rust borrow checker)
// - Mutation of variable after closure capture (Rust ownership)
//
// Note: Many Rust ownership checks (string reuse, slice self-reference,
// same variable in multiple args/expressions, multiple closures) have been
// moved to LangSemaLoweringPass which rewrites the AST instead of rejecting.
// Variable shadowing is also handled by LangSemaLoweringPass.
//
// ============================================
// SECTION 3: Supported with Limitations
// ============================================
//   - interface{} / any - maps to std::any (C++), Box<dyn Any> (Rust), object (C#)
//     Note: type assertions x.(T) supported in C++ only for now
type SemaChecker struct {
	Emitter
	pkg *packages.Package
	// Track range loop targets to detect mutation during iteration
	rangeTargets map[string]token.Pos
	// Track closure captures for detecting mutation after capture
	closureCaptures map[string]token.Pos
	// Track current function's parameters for mutation+return detection
	currentFuncParams map[string]bool
	mutatedParams     map[string]bool
	// Track variable declarations per scope for shadowing detection
	// Each map in the slice represents a scope level (index 0 = outermost)
	scopeStack []map[string]token.Pos
	// Track closure depth to allow mutations inside closures
	insideClosureDepth int
	// Track loop depth to allow mutations in loop patterns
	insideLoopDepth int
	// Track variables assigned &s.field (pointer to struct field)
	ptrFieldVars map[string]token.Pos
}

// reportSemaError reports a semantic error with source code context, similar to syntax errors
func (sema *SemaChecker) reportSemaError(pos token.Pos, title, description string, suggestions []string) {
	var filename string
	var line, col int

	if sema.pkg != nil && sema.pkg.Fset != nil && pos.IsValid() {
		position := sema.pkg.Fset.Position(pos)
		filename = position.Filename
		line = position.Line
		col = position.Column
	}

	fmt.Printf("\033[31m\033[1mSemantic error: %s\033[0m\n", title)

	// Show the source code if we have location info
	if filename != "" && line > 0 {
		fmt.Printf("  \033[36m-->\033[0m %s:%d:%d\n", filename, line, col)

		// Read and display the source line
		if sourceLine := sema.readSourceLine(filename, line); sourceLine != "" {
			fmt.Printf("   \033[90m%4d |\033[0m %s\n", line, sourceLine)

			// Add caret pointing to the column
			if col > 0 {
				padding := strings.Repeat(" ", col-1)
				fmt.Printf("   \033[90m     |\033[0m \033[31m%s^\033[0m\n", padding)
			}
		}
	}

	fmt.Printf("  %s\n", description)

	if len(suggestions) > 0 {
		fmt.Println()
		fmt.Println("  \033[32mSuggestion:\033[0m")
		for _, s := range suggestions {
			fmt.Printf("    %s\n", s)
		}
	}
	fmt.Println()
	os.Exit(-1)
}

// reportSemaWarning reports a semantic warning with source code context
func (sema *SemaChecker) reportSemaWarning(pos token.Pos, title, description string, suggestions []string) {
	var filename string
	var line, col int

	if sema.pkg != nil && sema.pkg.Fset != nil && pos.IsValid() {
		position := sema.pkg.Fset.Position(pos)
		filename = position.Filename
		line = position.Line
		col = position.Column
	}

	fmt.Printf("\033[33m\033[1mWarning: %s\033[0m\n", title)

	// Show the source code if we have location info
	if filename != "" && line > 0 {
		fmt.Printf("  \033[36m-->\033[0m %s:%d:%d\n", filename, line, col)

		// Read and display the source line
		if sourceLine := sema.readSourceLine(filename, line); sourceLine != "" {
			fmt.Printf("   \033[90m%4d |\033[0m %s\n", line, sourceLine)

			// Add caret pointing to the column
			if col > 0 {
				padding := strings.Repeat(" ", col-1)
				fmt.Printf("   \033[90m     |\033[0m \033[33m%s^\033[0m\n", padding)
			}
		}
	}

	fmt.Printf("  %s\n", description)

	if len(suggestions) > 0 {
		fmt.Println()
		fmt.Println("  \033[32mSuggestion:\033[0m")
		for _, s := range suggestions {
			fmt.Printf("    %s\n", s)
		}
	}
	fmt.Println()
}

// readSourceLine reads a specific line from a file
func (sema *SemaChecker) readSourceLine(filename string, lineNum int) string {
	file, err := os.Open(filename)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	currentLine := 0
	for scanner.Scan() {
		currentLine++
		if currentLine == lineNum {
			return scanner.Text()
		}
	}
	return ""
}

func (sema *SemaChecker) PreVisitPackage(pkg *packages.Package, indent int) {
	sema.pkg = pkg
	// Reset range targets for each package
	sema.rangeTargets = make(map[string]token.Pos)
	// Reset closure captures for each package
	sema.closureCaptures = make(map[string]token.Pos)

	// Check for package-level variable declarations (not supported by transpiler)
	sema.checkPackageLevelVars(pkg)
	// Check for unsupported imports
	sema.checkBlockedImports(pkg)
}

// checkPackageLevelVars detects package-level variable declarations which are not supported
// The transpiler does not handle var declarations at package scope
func (sema *SemaChecker) checkPackageLevelVars(pkg *packages.Package) {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.VAR {
				continue
			}
			// Found a package-level var declaration
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range valueSpec.Names {
					sema.reportSemaError(name.Pos(),
						"package-level variables are not supported",
						fmt.Sprintf("Variable '%s' is declared at package level.\n  Package-level variable declarations are not transpiled correctly.", name.Name),
						[]string{
							"Move the variable inside a function, or use a function that returns the value:",
							"  func getMyVar() T {",
							"      return T{...}",
							"  }",
						})
				}
			}
		}
	}
}

// checkBlockedImports detects imports of unsupported packages
func (sema *SemaChecker) checkBlockedImports(pkg *packages.Package) {
	blockedImports := map[string]string{
		"unsafe": "The unsafe package is not supported. Use safe alternatives.",
	}
	for _, file := range pkg.Syntax {
		for _, imp := range file.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			if msg, blocked := blockedImports[path]; blocked {
				sema.reportSemaError(imp.Pos(),
					fmt.Sprintf("unsupported import '%s'", path),
					msg, nil)
			}
		}
	}
}

// ============================================
// SECTION 1: Unsupported Go Features
// ============================================

// PreVisitStarExpr allows pointer types (*T) in supported contexts (params, dereference).
// Context-specific checks below block unsupported pointer patterns.
func (sema *SemaChecker) PreVisitStarExpr(node *ast.StarExpr, indent int) {
	// Reject multi-level pointers (**T, ***T, etc.) — only single-level *T is supported
	if _, ok := node.X.(*ast.StarExpr); ok {
		sema.reportSemaError(node.Pos(),
			"multi-level pointer types are not supported",
			"The transpiler only supports single-level pointers (*T). Multi-level pointers (**T, ***T, etc.) are not supported.",
			[]string{"Use a single-level pointer *T instead, or restructure your data to avoid pointer-to-pointer."})
		return
	}
}

// PreVisitDeclStmtValueSpecType allows pointer types in local variable declarations (var p *int)
// These are transformed by the pointer transform pass (*T → []T).
func (sema *SemaChecker) PreVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
}

// PreVisitFuncTypeResult allows pointer return types (func f() *T)
// These are transformed by the pointer transform pass via pool-based indexing.
func (sema *SemaChecker) PreVisitFuncTypeResult(node *ast.Field, index int, indent int) {
}

// PreVisitGenStructFieldType checks pointer types in struct fields (type S struct { p *int })
// Pointer fields are now supported via []T boxing (same approach as pointer params).
func (sema *SemaChecker) PreVisitGenStructFieldType(node ast.Expr, indent int) {
}

// PreVisitReturnStmt checks return statements
// Pointers to struct fields can now escape via pool-based indexing.
func (sema *SemaChecker) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
}

// PreVisitUnaryExpr checks for unsupported unary operators
func (sema *SemaChecker) PreVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	// Address-of (&x) is supported for simple identifiers and index expressions
	if node.Op == token.AND {
		// Reject &x where x has a pointer type (would create **T)
		if sema.pkg != nil && sema.pkg.TypesInfo != nil {
			if tv, ok := sema.pkg.TypesInfo.Types[node.X]; ok {
				if _, isPtr := tv.Type.(*types.Pointer); isPtr {
					sema.reportSemaError(node.Pos(),
						"multi-level pointer types are not supported",
						"Taking the address of a pointer variable creates **T, which is not supported by the transpiler.",
						[]string{"Use a single-level pointer *T instead, or restructure your data to avoid pointer-to-pointer."})
					return
				}
			}
			if ident, ok := node.X.(*ast.Ident); ok {
				if obj := sema.pkg.TypesInfo.Uses[ident]; obj != nil {
					if _, isPtr := obj.Type().(*types.Pointer); isPtr {
						sema.reportSemaError(node.Pos(),
							"multi-level pointer types are not supported",
							"Taking the address of a pointer variable creates **T, which is not supported by the transpiler.",
							[]string{"Use a single-level pointer *T instead, or restructure your data to avoid pointer-to-pointer."})
						return
					}
				}
			}
		}
		switch node.X.(type) {
		case *ast.Ident:
			// &x — allowed
		case *ast.IndexExpr:
			// &arr[i] — allowed
		case *ast.SelectorExpr:
			// &s.field — allowed
		case *ast.CompositeLit:
			// &Type{...} — allowed
		default:
			sema.reportSemaError(node.Pos(),
				"address-of complex expression is not supported",
				"Only address-of simple identifiers (&x), index expressions (&arr[i]), and field selectors (&s.field) are allowed.",
				[]string{
					"Assign the expression to a variable first, then take its address.",
				})
		}
	}
	if node.Op == token.ARROW {
		sema.reportSemaError(node.Pos(),
			"channel receive is not supported",
			"Channel receive operator (<-ch) is not allowed.\n  Channels are not supported in goany.",
			[]string{"Redesign without channels, or use a different synchronization mechanism."})
	}
}

// PreVisitSliceExpr checks for three-index slice expressions (s[low:high:max])
func (sema *SemaChecker) PreVisitSliceExpr(node *ast.SliceExpr, indent int) {
	if node.Slice3 {
		sema.reportSemaError(node.Pos(),
			"three-index slice expression is not supported",
			"Full slice expression with capacity (s[low:high:max]) is not allowed.\n  This controls capacity which has no equivalent in target languages.",
			[]string{"Use standard two-index slicing: s[low:high]"})
	}
}

// PreVisitMapType validates map types - only supported key types allowed, no nested maps
func (sema *SemaChecker) PreVisitMapType(node *ast.MapType, indent int) {
	if sema.pkg == nil || sema.pkg.TypesInfo == nil {
		return
	}

	// Check for unsupported key types
	if tv, ok := sema.pkg.TypesInfo.Types[node.Key]; ok && tv.Type != nil {
		if sema.isComparableKeyType(tv.Type, make(map[string]bool)) {
			// supported key type — continue to value check
		} else {
			// If not a supported key type, error
			sema.reportSemaError(node.Pos(),
				"unsupported map key type",
				fmt.Sprintf("Map key type '%s' is not supported.\n  Supported key types: primitives (string, int, bool, floats) or structs with only primitive fields.", tv.Type.String()),
				[]string{"Use a supported primitive type or a simple struct as the key."})
		}
	}

}

// isComparableKeyType checks if a type can be used as a map key
// Supports: primitives (string, int, bool, floats) and structs with comparable fields
func (sema *SemaChecker) isComparableKeyType(t types.Type, visited map[string]bool) bool {
	// Handle named types
	if named, ok := t.(*types.Named); ok {
		typeName := named.Obj().Pkg().Path() + "." + named.Obj().Name()
		if visited[typeName] {
			return false // Recursive type, not allowed
		}
		visited[typeName] = true
		return sema.isComparableKeyType(named.Underlying(), visited)
	}

	// Check basic types (primitives)
	if basic, isBasic := t.(*types.Basic); isBasic {
		switch basic.Kind() {
		case types.String, types.Int, types.Bool,
			types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64:
			return true
		}
		return false
	}

	// Check struct types - all fields must be comparable (recursively)
	if structType, isStruct := t.(*types.Struct); isStruct {
		for i := 0; i < structType.NumFields(); i++ {
			field := structType.Field(i)
			if !sema.isComparableKeyType(field.Type(), visited) {
				return false
			}
		}
		return true
	}

	// Other types (slices, maps, channels, functions, interfaces) are not comparable
	return false
}

// containsMutableReference checks if a type contains a slice or map (directly or nested in structs).
// Such types have different pass-by-value semantics across backends.
func (sema *SemaChecker) containsMutableReference(t types.Type, visited map[string]bool) bool {
	if named, ok := t.(*types.Named); ok {
		typeName := named.Obj().Pkg().Path() + "." + named.Obj().Name()
		if visited[typeName] {
			return false
		}
		visited[typeName] = true
		return sema.containsMutableReference(named.Underlying(), visited)
	}
	if _, ok := t.(*types.Slice); ok {
		return true
	}
	if _, ok := t.(*types.Map); ok {
		return true
	}
	if st, ok := t.(*types.Struct); ok {
		for i := 0; i < st.NumFields(); i++ {
			if sema.containsMutableReference(st.Field(i).Type(), visited) {
				return true
			}
		}
	}
	return false
}

// functionBodyMutatesParam checks if the function body contains any direct mutations
// of a parameter (index assignments like param[i] = x, or param.field[i] = x).
// This is used to avoid flagging read-only functions that take slice/map parameters.
func functionBodyMutatesParam(body *ast.BlockStmt, paramName string) bool {
	if body == nil {
		return false
	}
	mutates := false
	ast.Inspect(body, func(n ast.Node) bool {
		if mutates {
			return false
		}
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range stmt.Lhs {
				if indexExprReferencesIdent(lhs, paramName) {
					mutates = true
					return false
				}
			}
		case *ast.IncDecStmt:
			if indexExprReferencesIdent(stmt.X, paramName) {
				mutates = true
				return false
			}
		}
		return true
	})
	return mutates
}

// indexExprReferencesIdent checks if an expression is an index expression
// whose base references the given identifier name.
// Handles patterns like: param[i], param.field[i], param[i].field[j]
func indexExprReferencesIdent(expr ast.Expr, name string) bool {
	switch e := expr.(type) {
	case *ast.IndexExpr:
		return baseExprReferencesIdent(e.X, name)
	}
	return false
}

// baseExprReferencesIdent checks if an expression ultimately references
// the given identifier name through selector or index chains.
func baseExprReferencesIdent(expr ast.Expr, name string) bool {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == name
	case *ast.SelectorExpr:
		return baseExprReferencesIdent(e.X, name)
	case *ast.IndexExpr:
		return baseExprReferencesIdent(e.X, name)
	}
	return false
}

// Note: Structural checks for DeferStmt, GoStmt, ChanType, SelectStmt, LabeledStmt
// are handled by the SyntaxChecker (handwritten rules in syntax_checker.go).

// PreVisitBranchStmt checks for goto statements which are not supported
func (sema *SemaChecker) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	if node.Tok == token.GOTO {
		sema.reportSemaError(node.Pos(),
			"goto statements are not supported",
			fmt.Sprintf("goto %s is not allowed.\n  Goto has limited support in target languages like Rust and JavaScript.", node.Label.Name),
			[]string{"Use structured control flow (functions, loops with break)."})
	}
}

// PreVisitFuncDecl checks for method receivers, init functions, variadic params, and named returns
func (sema *SemaChecker) PreVisitFuncDecl(node *ast.FuncDecl, indent int) {
	// Reset ptr field vars for each function
	sema.ptrFieldVars = make(map[string]token.Pos)
	// Reset range targets for each function
	sema.rangeTargets = make(map[string]token.Pos)
	// Reset closure captures for each function
	sema.closureCaptures = make(map[string]token.Pos)
	// Track current function's parameters for mutation+return detection
	sema.currentFuncParams = make(map[string]bool)
	sema.mutatedParams = make(map[string]bool)
	// Initialize scope stack for variable shadowing detection
	sema.scopeStack = []map[string]token.Pos{}
	// Push initial function scope and add parameters
	funcScope := make(map[string]token.Pos)
	if node.Type != nil && node.Type.Params != nil {
		for _, field := range node.Type.Params.List {
			for _, name := range field.Names {
				funcScope[name.Name] = name.Pos()
			}
		}
	}
	sema.scopeStack = append(sema.scopeStack, funcScope)

	// Collect parameter names for mutation tracking
	if node.Type != nil && node.Type.Params != nil {
		for _, field := range node.Type.Params.List {
			for _, name := range field.Names {
				sema.currentFuncParams[name.Name] = true
			}
		}
	}

	// Check for init functions
	if node.Name.Name == "init" {
		sema.reportSemaError(node.Pos(),
			"init functions are not supported",
			"Package init() functions are not allowed.\n  Init functions have no direct equivalent in target languages.",
			[]string{"Call initialization explicitly from main() or use constructors."})
	}

	// Named return values and variadic parameters are handled by
	// LangSemaLoweringPass transforms (transformNamedReturns, transformVariadics).

	// Check for mutable reference parameters (slice/map) not returned
	// Skip runtime files — they're internal transpiler infrastructure with hardcoded calling conventions
	isRuntimeFile := false
	if sema.pkg != nil && sema.pkg.Fset != nil && node.Pos().IsValid() {
		fname := sema.pkg.Fset.Position(node.Pos()).Filename
		if strings.Contains(fname, "goany-runtime") {
			isRuntimeFile = true
		}
	}
	if !isRuntimeFile && node.Type != nil && node.Type.Params != nil && sema.pkg != nil && sema.pkg.TypesInfo != nil {
		var returnTypes []types.Type
		if node.Type.Results != nil {
			for _, field := range node.Type.Results.List {
				if tv, ok := sema.pkg.TypesInfo.Types[field.Type]; ok {
					returnTypes = append(returnTypes, tv.Type)
				}
			}
		}
		for _, field := range node.Type.Params.List {
			if tv, ok := sema.pkg.TypesInfo.Types[field.Type]; ok {
				paramType := tv.Type
				// Skip pointer params — handled by pointer lowering
				if _, isPtr := paramType.(*types.Pointer); isPtr {
					continue
				}
				if sema.containsMutableReference(paramType, make(map[string]bool)) {
					found := false
					for _, rt := range returnTypes {
						if types.Identical(paramType, rt) {
							found = true
							break
						}
					}
					if !found {
						for _, name := range field.Names {
							// Only flag if the function body actually mutates the parameter
							// (index assignments like param[i] = x). Read-only access is safe
							// across all backends since reading a copy gives the same result.
							if functionBodyMutatesParam(node.Body, name.Name) {
								sema.reportSemaError(name.Pos(),
									"mutable reference parameter not returned",
									fmt.Sprintf("Parameter '%s' of function '%s' contains a slice or map.\n  Mutations to slice/map elements inside the function behave differently across backends\n  (C++/Rust copy the container, C#/Java/JS pass by reference).", name.Name, node.Name.Name),
									[]string{
										"Return the parameter so the caller reassigns it: items = foo(items)",
										fmt.Sprintf("Change signature to: func %s(%s %s) %s", node.Name.Name, name.Name, tv.Type.String(), tv.Type.String()),
									})
							}
						}
					}
				}
			}
		}
	}
}

func (sema *SemaChecker) PreVisitGenDeclConstName(node *ast.Ident, indent int) {
	// iota and untyped const warnings are no longer needed here.
	// LangSemaLoweringPass.transformIotaExpansion handles iota expansion
	// and sets explicit types for all iota-based constants.
}

func (sema *SemaChecker) PreVisitIdent(node *ast.Ident, indent int) {
	// iota is handled by LangSemaLoweringPass.transformIotaExpansion.
	// String variable reuse is handled by LangSemaLoweringPass.transformStringReuseAfterConcat.

	// Block unsupported types
	unsupportedTypes := map[string]string{
		"complex64":  "Complex number types are not supported by the transpiler backends.",
		"complex128": "Complex number types are not supported by the transpiler backends.",
	}
	if msg, blocked := unsupportedTypes[node.Name]; blocked {
		// Only report if this is actually a type reference (not a variable named complex64)
		if sema.pkg != nil && sema.pkg.TypesInfo != nil {
			if obj := sema.pkg.TypesInfo.Uses[node]; obj != nil {
				if _, isTypeName := obj.(*types.TypeName); isTypeName {
					sema.reportSemaError(node.Pos(),
						fmt.Sprintf("unsupported type '%s'", node.Name),
						msg,
						[]string{"Use float64 for numeric computations instead."})
				}
			}
		}
	}
}

func (sema *SemaChecker) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
}

// PostVisitFuncDecl clears the scope stack after function processing
func (sema *SemaChecker) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	sema.scopeStack = nil
}

// PreVisitBlockStmt pushes a new scope for variable shadowing detection
func (sema *SemaChecker) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	if sema.scopeStack != nil {
		sema.scopeStack = append(sema.scopeStack, make(map[string]token.Pos))
	}
}

// PostVisitBlockStmt pops the current scope
func (sema *SemaChecker) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	if sema.scopeStack != nil && len(sema.scopeStack) > 0 {
		sema.scopeStack = sema.scopeStack[:len(sema.scopeStack)-1]
	}
}

// PreVisitForStmt pushes a scope for the for loop (to contain the init variable)
// In Go, `for i := 0; ...` creates an implicit scope containing both Init and Body
// Also checks for mutation during iteration (e.g., for i := 0; i < len(items); i++ { items = append(...) })
func (sema *SemaChecker) PreVisitForStmt(node *ast.ForStmt, indent int) {
	if sema.scopeStack != nil {
		sema.scopeStack = append(sema.scopeStack, make(map[string]token.Pos))
	}

	// Track loop depth for closure capture mutation check
	sema.insideLoopDepth++

	// Check for mutation during iteration pattern:
	// for i := 0; i < len(slice); i++ { slice = ... }
	if node.Cond != nil && node.Body != nil {
		// Extract slice name from condition if it contains len(slice)
		sliceName := sema.extractLenTarget(node.Cond)
		if sliceName != "" {
			// Check if the slice is mutated in the loop body
			sema.checkForBodyMutation(node.Body, sliceName)
		}
	}
}

// extractLenTarget extracts the variable name from an index-based loop condition
// e.g., from "i < len(items)" returns "items"
// Only triggers for index-based patterns like "i < len(slice)" or "len(slice) > i"
// Does NOT trigger for stack-based patterns like "len(stk) > 0"
func (sema *SemaChecker) extractLenTarget(cond ast.Expr) string {
	// Only check binary expressions (comparisons)
	binExpr, ok := cond.(*ast.BinaryExpr)
	if !ok {
		return ""
	}

	// Only for comparison operators
	if binExpr.Op != token.LSS && binExpr.Op != token.LEQ &&
		binExpr.Op != token.GTR && binExpr.Op != token.GEQ {
		return ""
	}

	// Check if this is an index-based pattern:
	// Either: i < len(slice) OR len(slice) > i
	// We need both a len() call and a variable (the index) on opposite sides
	var lenCall *ast.CallExpr
	var hasIndexVar bool

	// Check left side
	if call, ok := binExpr.X.(*ast.CallExpr); ok {
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "len" {
			lenCall = call
		}
	} else if _, ok := binExpr.X.(*ast.Ident); ok {
		hasIndexVar = true
	}

	// Check right side
	if call, ok := binExpr.Y.(*ast.CallExpr); ok {
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "len" {
			lenCall = call
		}
	} else if _, ok := binExpr.Y.(*ast.Ident); ok {
		hasIndexVar = true
	}

	// Only trigger if we have both len() and an index variable (not just len() > 0)
	if lenCall == nil || !hasIndexVar {
		return ""
	}

	// Extract the slice name from len(slice)
	if len(lenCall.Args) == 1 {
		if arg, ok := lenCall.Args[0].(*ast.Ident); ok {
			return arg.Name
		}
	}
	return ""
}

// checkForBodyMutation checks if a variable is mutated inside a for loop body
func (sema *SemaChecker) checkForBodyMutation(body *ast.BlockStmt, targetName string) {
	if body == nil {
		return
	}

	ast.Inspect(body, func(n ast.Node) bool {
		if stmt, ok := n.(*ast.AssignStmt); ok {
			for _, lhs := range stmt.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && ident.Name == targetName {
					sema.reportSemaError(stmt.Pos(),
						"collection mutation during iteration",
						fmt.Sprintf("Variable '%s' is modified while being iterated over.\n  This pattern fails in Rust due to borrow checker rules.", targetName),
						[]string{
							"Collect changes and apply after loop:",
							"  var toAdd []T",
							fmt.Sprintf("  for i := 0; i < len(%s); i++ { toAdd = append(toAdd, ...) }", targetName),
							fmt.Sprintf("  %s = append(%s, toAdd...)", targetName, targetName),
						})
				}
			}
		}
		return true
	})
}

// PostVisitForStmt pops the for loop scope
func (sema *SemaChecker) PostVisitForStmt(node *ast.ForStmt, indent int) {
	if sema.scopeStack != nil && len(sema.scopeStack) > 0 {
		sema.scopeStack = sema.scopeStack[:len(sema.scopeStack)-1]
	}
	// Decrement loop depth
	sema.insideLoopDepth--
}

func (sema *SemaChecker) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	// Push scope for range loop variables (like for i, v := range ...)
	if sema.scopeStack != nil {
		sema.scopeStack = append(sema.scopeStack, make(map[string]token.Pos))
	}

	// Track loop depth for closure capture mutation check
	sema.insideLoopDepth++

	// Handle for _, v := range (value-only): set Key to nil so emitters work correctly
	// for i, v := range (key-value) is now allowed and handled by emitters
	// for i := range (index-only) is allowed (Value is nil)
	if node.Key != nil && node.Value != nil {
		if node.Key.(*ast.Ident).Name == "_" {
			// For value-only range (for _, v := range), set Key to nil so emitters work correctly
			node.Key = nil
		}
		// Otherwise, keep both Key and Value for key-value range loops
	}


	// Track range target for mutation detection
	if ident, ok := node.X.(*ast.Ident); ok {
		if sema.rangeTargets == nil {
			sema.rangeTargets = make(map[string]token.Pos)
		}
		sema.rangeTargets[ident.Name] = node.Pos()

		// Check for mutations to range target inside the loop body
		sema.checkRangeBodyMutation(node.Body, ident.Name)
	}
}

// PostVisitRangeStmt clears the range target after the loop and pops scope
func (sema *SemaChecker) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	if ident, ok := node.X.(*ast.Ident); ok {
		if sema.rangeTargets != nil {
			delete(sema.rangeTargets, ident.Name)
		}
	}
	// Pop scope for range loop variables
	if sema.scopeStack != nil && len(sema.scopeStack) > 0 {
		sema.scopeStack = sema.scopeStack[:len(sema.scopeStack)-1]
	}
	// Decrement loop depth
	sema.insideLoopDepth--
}

// checkRangeBodyMutation checks if the range target is mutated inside the loop body
func (sema *SemaChecker) checkRangeBodyMutation(body *ast.BlockStmt, targetName string) {
	if body == nil {
		return
	}

	ast.Inspect(body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range stmt.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if ident.Name == targetName {
						sema.reportSemaError(stmt.Pos(),
							"collection mutation during iteration",
							fmt.Sprintf("Variable '%s' is modified while being iterated over.\n  This pattern fails in Rust due to borrow checker rules.", targetName),
							[]string{
								"Collect changes and apply after loop:",
								"  var toAdd []T",
								fmt.Sprintf("  for _, v := range %s { toAdd = append(toAdd, v) }", targetName),
								fmt.Sprintf("  %s = append(%s, toAdd...)", targetName, targetName),
							})
					}
				}
			}
		}
		return true
	})
}

// PreVisitBinaryExpr checks for nil comparisons which are not supported for slices
// and tracks string variable consumption for Rust compatibility
func (sema *SemaChecker) PreVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	// Check for == nil or != nil comparisons
	if node.Op == token.EQL || node.Op == token.NEQ {
		if isNilIdent(node.Y) || isNilIdent(node.X) {
			// Allow nil comparisons for pointer types (*T) — transformed to == -1 / != -1
			isPointerNil := false
			if sema.pkg != nil && sema.pkg.TypesInfo != nil {
				var nonNilExpr ast.Expr
				if isNilIdent(node.Y) {
					nonNilExpr = node.X
				} else {
					nonNilExpr = node.Y
				}
				if tv, ok := sema.pkg.TypesInfo.Types[nonNilExpr]; ok && tv.Type != nil {
					if _, isPtr := tv.Type.(*types.Pointer); isPtr {
						isPointerNil = true
					}
				}
				if !isPointerNil {
					if ident, ok := nonNilExpr.(*ast.Ident); ok {
						if obj := sema.pkg.TypesInfo.Uses[ident]; obj != nil {
							if _, isPtr := obj.Type().(*types.Pointer); isPtr {
								isPointerNil = true
							}
						}
					}
				}
			}
			if !isPointerNil {
				sema.reportSemaError(node.Pos(),
					"nil comparison is not allowed",
					"Nil comparison (== nil or != nil) is not supported.",
					[]string{"Use len() == 0 for slices, or redesign to avoid nil checks."})
			}
		}

		// Check for map comparisons (maps can only be compared to nil in Go, not to each other)
		if sema.pkg != nil && sema.pkg.TypesInfo != nil {
			// Check left operand
			if tvX, ok := sema.pkg.TypesInfo.Types[node.X]; ok && tvX.Type != nil {
				if _, isMap := tvX.Type.Underlying().(*types.Map); isMap {
					sema.reportSemaError(node.Pos(),
						"map comparison is not supported",
						"Maps cannot be compared with == or !=.\n  In Go, maps can only be compared to nil, not to each other.",
						[]string{"Compare map contents manually if needed."})
				}
			}
			// Check right operand
			if tvY, ok := sema.pkg.TypesInfo.Types[node.Y]; ok && tvY.Type != nil {
				if _, isMap := tvY.Type.Underlying().(*types.Map); isMap {
					sema.reportSemaError(node.Pos(),
						"map comparison is not supported",
						"Maps cannot be compared with == or !=.\n  In Go, maps can only be compared to nil, not to each other.",
						[]string{"Compare map contents manually if needed."})
				}
			}
		}
	}

	// Note: String concatenation consumption tracking has been moved to
	// LangSemaLoweringPass.transformStringReuseAfterConcat which rewrites the AST.
}

// PreVisitAssignStmt checks for problematic patterns like: x += x + a
// where x is both borrowed (for +=) and moved (in x + a) in the same statement
// Also checks for slice self-assignment: slice[i] = slice[j]
//
// Note: Multi-assign split and variable shadowing checks have been moved
// to the LangSemaLoweringPass which rewrites the AST instead of rejecting.
func (sema *SemaChecker) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	// Track variables assigned &s.field (pointer to struct field)
	if (node.Tok == token.DEFINE || node.Tok == token.ASSIGN) && len(node.Lhs) == 1 && len(node.Rhs) == 1 {
		if lhsIdent, ok := node.Lhs[0].(*ast.Ident); ok {
			if unary, ok := node.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
				if _, ok := unary.X.(*ast.SelectorExpr); ok {
					sema.ptrFieldVars[lhsIdent.Name] = node.Pos()
				}
			}
		}
	}

	// Note: checkSliceSelfAssignment, self-ref concat, and checkBinaryExprWithSameVar
	// have been moved to LangSemaLoweringPass which rewrites the AST instead of rejecting.

	// Check for mutation of variable after closure capture
	sema.checkClosureCaptureMutation(node)
}

// Note: rhsContainsStringConcatWithVar and PostVisitAssignStmt have been removed.
// Self-referencing string concatenation is now handled by LangSemaLoweringPass.transformSelfRefConcat
// and string reuse after concatenation by LangSemaLoweringPass.transformStringReuseAfterConcat.

// Note: checkVariableShadowing has been removed.
// Variable shadowing is now handled by LangSemaLoweringPass.transformShadowedVars
// which renames the inner variable instead of rejecting the code.

// PreVisitInterfaceType checks for interface types - only empty interface{} is supported
func (sema *SemaChecker) PreVisitInterfaceType(node *ast.InterfaceType, indent int) {
	// Empty interface (interface{} / any) is supported
	// Maps to: C++ std::any, Rust Box<dyn Any>, C# object
	// Non-empty interfaces are NOT supported
	if node.Methods != nil && len(node.Methods.List) > 0 {
		sema.reportSemaError(node.Pos(),
			"non-empty interfaces are not supported",
			"Only empty interface (interface{} / any) is allowed.\n  Interfaces with methods have no direct equivalent in all target languages.",
			[]string{"Use concrete types or interface{} with type assertions."})
	}
}

// PreVisitGenStructInfo checks struct type declarations for embedding (anonymous fields)
// This is called during struct generation to validate type declarations
func (sema *SemaChecker) PreVisitGenStructInfo(info GenTypeInfo, indent int) {
	if info.Struct == nil {
		return
	}
	sema.checkStructEmbedding(info.Struct)
}

// PreVisitStructType checks for struct embedding (anonymous fields) which is not supported
// This handles struct types that appear in expressions (e.g., composite literals)
func (sema *SemaChecker) PreVisitStructType(node *ast.StructType, indent int) {
	sema.checkStructEmbedding(node)
}

// checkStructEmbedding validates that a struct has no embedded (anonymous) fields
func (sema *SemaChecker) checkStructEmbedding(node *ast.StructType) {
	if node.Fields == nil {
		return
	}
	for _, field := range node.Fields.List {
		// Anonymous field (embedding) has no names
		if len(field.Names) == 0 {
			// Get the embedded type name for the error message
			typeName := "unknown"
			switch t := field.Type.(type) {
			case *ast.Ident:
				typeName = t.Name
			case *ast.SelectorExpr:
				typeName = t.Sel.Name
			}
			sema.reportSemaError(field.Pos(),
				"struct embedding is not supported",
				fmt.Sprintf("Embedded field '%s' (anonymous field) is not allowed.\n  Struct embedding has different semantics in target languages.", typeName),
				[]string{
					fmt.Sprintf("Instead of: type MyStruct struct { %s }", typeName),
					fmt.Sprintf("Use named field: type MyStruct struct { field %s }", typeName),
				})
		}
	}
}

// Note: checkFieldNameMatchesType has been removed.
// Field name conflicts are now handled by LangSemaLoweringPass.transformFieldConflicts
// which renames the field instead of rejecting the code.

// PreVisitTypeSwitchStmt checks for type switch statements (not supported)
func (sema *SemaChecker) PreVisitTypeSwitchStmt(node *ast.TypeSwitchStmt, indent int) {
	sema.reportSemaError(node.Pos(),
		"type switch statement is not allowed",
		"Type switch statements are not supported in goany.",
		[]string{"Use explicit type assertions or redesign without type switches."})
}

// isNilIdent checks if an expression is the nil identifier
func isNilIdent(expr ast.Expr) bool {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name == "nil"
	}
	return false
}

// PreVisitCompositeLit checks for struct field initialization order and map literals
// C++ designated initializers require fields to be in declaration order
// Map literals are not supported
func (sema *SemaChecker) PreVisitCompositeLit(node *ast.CompositeLit, indent int) {
	if sema.pkg == nil || sema.pkg.TypesInfo == nil {
		return
	}

	// Get the type of the composite literal
	tv, ok := sema.pkg.TypesInfo.Types[node]
	if !ok || tv.Type == nil {
		return
	}

	// Map literals are handled directly by each backend's emitter.

	// Check if it's a struct type
	structType, ok := tv.Type.Underlying().(*types.Struct)
	if !ok {
		return
	}

	// Get field names from the struct declaration (in order)
	declaredFields := make([]string, structType.NumFields())
	fieldIndex := make(map[string]int)
	for i := 0; i < structType.NumFields(); i++ {
		field := structType.Field(i)
		declaredFields[i] = field.Name()
		fieldIndex[field.Name()] = i
	}

	// Get field names from the initialization (in order)
	var initFields []string
	for _, elt := range node.Elts {
		if kv, ok := elt.(*ast.KeyValueExpr); ok {
			if ident, ok := kv.Key.(*ast.Ident); ok {
				initFields = append(initFields, ident.Name)
			}
		}
	}

	// If no keyed fields, skip the order check (positional initialization)
	if len(initFields) == 0 {
		return
	}

	// Check if initialization order matches declaration order
	lastIndex := -1
	for _, fieldName := range initFields {
		idx, exists := fieldIndex[fieldName]
		if !exists {
			continue // Unknown field, skip
		}
		if idx < lastIndex {
			// Fields are out of order
			fmt.Println("\033[33m\033[1mWarning: struct field initialization order does not match declaration order\033[0m")
			fmt.Printf("  Field '%s' appears before a previously initialized field.\n", fieldName)
			fmt.Println("  C++ designated initializers require fields in declaration order.")
			fmt.Println()
			fmt.Println("  \033[36mDeclared order:\033[0m")
			for _, f := range declaredFields {
				fmt.Printf("    - %s\n", f)
			}
			fmt.Println()
			fmt.Println("  \033[36mInitialization order:\033[0m")
			for _, f := range initFields {
				fmt.Printf("    - %s\n", f)
			}
			fmt.Println()
			fmt.Println("  \033[32mPlease reorder the initializers to match the struct declaration.\033[0m")
			os.Exit(-1)
		}
		lastIndex = idx
	}
}

// isNonCopyType checks if a type requires cloning in Rust (non-Copy types)
func (sema *SemaChecker) isNonCopyType(t types.Type) bool {
	if t == nil {
		return false
	}

	typeStr := t.String()
	// Strings are non-Copy
	if typeStr == "string" {
		return true
	}
	// Slices are non-Copy (direct slice types only)
	if _, ok := t.Underlying().(*types.Slice); ok {
		return true
	}
	// Maps are non-Copy
	if _, ok := t.Underlying().(*types.Map); ok {
		return true
	}
	// Note: Structs are NOT flagged here, even if they contain non-Copy fields.
	// The transpiler handles structs by reference passing, so nested calls with
	// struct arguments are safe. Only direct slice/string/map usage is problematic.
	return false
}

// collectCapturedIdentsForMutation collects non-Copy identifiers captured by a closure
// for mutation-after-capture detection. Returns identifiers from outer scope.
func (sema *SemaChecker) collectCapturedIdentsForMutation(funcLit *ast.FuncLit) []*ast.Ident {
	var result []*ast.Ident
	seen := make(map[string]bool)
	ast.Inspect(funcLit.Body, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || seen[ident.Name] {
			return true
		}
		obj := sema.pkg.TypesInfo.Uses[ident]
		if obj == nil {
			return true
		}
		if _, isConst := obj.(*types.Const); isConst {
			return true
		}
		if _, isFunc := obj.(*types.Func); isFunc {
			return true
		}
		if _, isTypeName := obj.(*types.TypeName); isTypeName {
			return true
		}
		if !sema.isNonCopyType(obj.Type()) {
			return true
		}
		if obj.Pos() < funcLit.Pos() {
			seen[ident.Name] = true
			result = append(result, ident)
		}
		return true
	})
	return result
}

// Note: getDirectFunctionArgs has been removed from SemaChecker.
// The equivalent is now in LangSemaLoweringPass (lang_sema_lowering_pass.go).

// Note: checkBinaryExprWithSameVar has been removed.
// Same-variable binary expressions are now handled by LangSemaLoweringPass.transformBinaryExprSameVar
// which rewrites the AST instead of rejecting.

// Note: checkNestedCallArgSharing has been removed.
// Nested call sharing is now handled by LangSemaLoweringPass.transformNestedCallSharing
// which rewrites the AST instead of rejecting.

// Note: collectDirectCallArgIdents has been removed from SemaChecker.
// The equivalent is now in LangSemaLoweringPass (lang_sema_lowering_pass.go).

// checkClosureCaptureMutation checks if a variable captured by a closure is mutated after capture
func (sema *SemaChecker) checkClosureCaptureMutation(node *ast.AssignStmt) {
	// Skip check if we're inside a closure - mutations inside closures are allowed
	if sema.insideClosureDepth > 0 {
		return
	}
	// Skip check if we're inside a loop - loop patterns typically call closure before mutation
	// within each iteration, which is safe
	if sema.insideLoopDepth > 0 {
		return
	}
	if sema.closureCaptures == nil || len(sema.closureCaptures) == 0 {
		return
	}
	if sema.pkg == nil || sema.pkg.TypesInfo == nil {
		return
	}

	for _, lhs := range node.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok {
			if capturePos, captured := sema.closureCaptures[ident.Name]; captured {
				// Check if this assignment is after the closure capture (in outer scope)
				if node.Pos() > capturePos {
					// Check if it's a non-Copy type
					if obj := sema.pkg.TypesInfo.Uses[ident]; obj != nil {
						if sema.isNonCopyType(obj.Type()) {
							sema.reportSemaError(ident.Pos(),
								"mutation of variable after closure capture",
								fmt.Sprintf("Variable '%s' is captured by a closure and then mutated.\n  This pattern fails in Rust due to ownership rules.", ident.Name),
								[]string{
									"Mutate before capture or use separate variables:",
									fmt.Sprintf("  %s = modify(%s); fn := func() { use(%s) }", ident.Name, ident.Name, ident.Name),
								})
						}
					}
				}
			}
		}
	}
}

// Note: checkSliceSelfAssignment has been removed.
// Slice self-reference is now handled by LangSemaLoweringPass.transformSliceSelfRef
// which rewrites the AST instead of rejecting.

// Note: exprContainsSliceAccess has been removed from SemaChecker.
// The equivalent is now in LangSemaLoweringPass (lang_sema_lowering_pass.go).

// Note: isCopyType has been removed from SemaChecker.
// The equivalent is now in LangSemaLoweringPass (lang_sema_lowering_pass.go).

// PreVisitFuncLit tracks closure depth and captures for mutation-after-capture detection.
//
// Note: Multiple-closures-same-variable check has been moved to
// LangSemaLoweringPass.transformMultiClosureSameVar which rewrites the AST instead of rejecting.
func (sema *SemaChecker) PreVisitFuncLit(node *ast.FuncLit, indent int) {
	// Track that we're inside a closure (for mutation-after-capture check)
	sema.insideClosureDepth++

	if sema.pkg == nil || sema.pkg.TypesInfo == nil {
		return
	}

	// Track closure captures for mutation-after-capture detection
	idents := sema.collectCapturedIdentsForMutation(node)
	for _, ident := range idents {
		if sema.closureCaptures == nil {
			sema.closureCaptures = make(map[string]token.Pos)
		}
		sema.closureCaptures[ident.Name] = node.Pos()
	}
}

// PostVisitFuncLit decrements the closure depth counter
func (sema *SemaChecker) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	sema.insideClosureDepth--
}

// PreVisitCallExpr checks for same non-Copy variable passed multiple times to a function
// Pattern: f(x, x) where x is a non-Copy type (slice, string, struct)
func (sema *SemaChecker) PreVisitCallExpr(node *ast.CallExpr, indent int) {
	if sema.pkg == nil || sema.pkg.TypesInfo == nil {
		return
	}

	// Check for built-in functions
	if ident, ok := node.Fun.(*ast.Ident); ok {
		// Unsupported builtins - emit an error
		unsupportedBuiltins := map[string]string{
			"cap":     "Use len() instead for slice length, or track capacity separately if needed.",
			"copy":    "Use manual element-by-element copy or slice assignment instead.",
			"close":   "Channel operations are not supported in transpiled code.",
			"recover": "Panic recovery is not supported in transpiled code. Use explicit error handling.",
			"complex": "Complex numbers are not supported in transpiled code.",
			"real":    "Complex numbers are not supported in transpiled code.",
			"imag":    "Complex numbers are not supported in transpiled code.",
			"new":     "Use &T{} (address of zero-value literal) instead of new(T).",
		}
		if suggestion, unsupported := unsupportedBuiltins[ident.Name]; unsupported {
			sema.reportSemaError(node.Pos(),
				fmt.Sprintf("unsupported builtin function '%s'", ident.Name),
				fmt.Sprintf("The builtin function '%s' is not supported by the transpiler backends.", ident.Name),
				[]string{suggestion})
			return
		}

		// Supported builtins - skip further argument checks
		supportedBuiltins := map[string]bool{
			"len": true, "append": true, "make": true, "delete": true,
			"panic": true, "print": true, "println": true,
			"min": true, "max": true, "clear": true,
		}
		if supportedBuiltins[ident.Name] {
			// Validate min/max: only 2 args supported
			if (ident.Name == "min" || ident.Name == "max") && len(node.Args) > 2 {
				sema.reportSemaError(node.Pos(),
					fmt.Sprintf("%s() with more than 2 arguments is not supported", ident.Name),
					fmt.Sprintf("The transpiler only supports %s(a, b) with exactly 2 arguments.", ident.Name),
					[]string{fmt.Sprintf("Use nested calls: %s(%s(a, b), c)", ident.Name, ident.Name)})
				return
			}
			// Validate clear: only maps supported, not slices
			if ident.Name == "clear" {
				if len(node.Args) == 1 && sema.pkg != nil && sema.pkg.TypesInfo != nil {
					if tv, ok := sema.pkg.TypesInfo.Types[node.Args[0]]; ok && tv.Type != nil {
						if _, isSlice := tv.Type.Underlying().(*types.Slice); isSlice {
							sema.reportSemaError(node.Pos(),
								"clear() on slices is not supported",
								"The transpiler only supports clear() on maps, not slices.",
								[]string{"Use a loop to zero slice elements, or reassign with make()."})
							return
						}
					}
				}
			}
			return
		}
	}

	// Note: Same-variable-multiple-args and nested-call-sharing checks
	// have been moved to LangSemaLoweringPass which rewrites the AST instead of rejecting.
}
