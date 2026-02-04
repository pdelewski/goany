package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"golang.org/x/tools/go/packages"
	"os"
)

// SemaChecker performs semantic analysis to detect unsupported Go constructs.
//
// ============================================
// SECTION 1: Unsupported Go Features (Errors)
// ============================================
// - Pointers (*T, &x)
// - Maps (map[K]V)
// - Defer statements
// - Goroutines (go keyword)
// - Channels (chan T)
// - Select statements
// - Goto and labels
// - Method receivers
// - Variadic functions (...T)
// - Non-empty interfaces
// - Struct embedding (anonymous fields)
// - Init functions
// - Named return values
// - iota constant enumeration
// - Type switch statements
// - Package-level variable declarations
//
// ============================================
// SECTION 2: Backend-Specific Constraints
// ============================================
// - Range over inline composite literals
// - Nil comparisons (== nil, != nil)
// - String variable reuse after concatenation (Rust move semantics)
// - Same variable multiple times in expression (Rust ownership)
// - Slice self-assignment (Rust borrow checker)
// - Multiple closures capturing same variable (Rust borrow checker)
// - Struct field initialization order (C++ designated initializers)
// - Variable shadowing (C# does not allow shadowing within same function)
//
// ============================================
// SECTION 3: Supported with Limitations
// ============================================
//   - interface{} / any - maps to std::any (C++), Box<dyn Any> (Rust), object (C#)
//     Note: type assertions x.(T) supported in C++ only for now
type SemaChecker struct {
	Emitter
	pkg      *packages.Package
	constCtx bool
	// Track string variables consumed by concatenation (for Rust compatibility)
	consumedStringVars map[string]token.Pos
	// Track variables used in closures for multiple closure detection
	closureVars map[string]token.Pos
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
}

func (sema *SemaChecker) PreVisitPackage(pkg *packages.Package, indent int) {
	sema.pkg = pkg
	// Reset consumed variables map for each package
	sema.consumedStringVars = make(map[string]token.Pos)
	// Reset closure variables map for each package
	sema.closureVars = make(map[string]token.Pos)
	// Reset range targets for each package
	sema.rangeTargets = make(map[string]token.Pos)
	// Reset closure captures for each package
	sema.closureCaptures = make(map[string]token.Pos)

	// Check for package-level variable declarations (not supported by transpiler)
	sema.checkPackageLevelVars(pkg)
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
					fmt.Println("\033[31m\033[1mCompilation error: package-level variables are not supported\033[0m")
					fmt.Printf("  Variable '%s' is declared at package level.\n", name.Name)
					fmt.Println("  Package-level variable declarations are not transpiled correctly.")
					fmt.Println()
					fmt.Println("  \033[32mMove the variable inside a function, or use a function that returns the value:\033[0m")
					fmt.Println("    func getMyVar() T {")
					fmt.Println("        return T{...}")
					fmt.Println("    }")
					os.Exit(-1)
				}
			}
		}
	}
}

// ============================================
// SECTION 1: Unsupported Go Features
// ============================================

// PreVisitStarExpr checks for pointer types (*T) which are not supported
func (sema *SemaChecker) PreVisitStarExpr(node *ast.StarExpr, indent int) {
	fmt.Println("\033[31m\033[1mCompilation error: pointer types are not supported\033[0m")
	fmt.Println("  Pointer types (*T) and pointer dereferencing are not allowed.")
	fmt.Println("  goany targets languages with different memory models (Rust, C#, JS).")
	fmt.Println()
	fmt.Println("  \033[32mUse value types or slices instead.\033[0m")
	os.Exit(-1)
}

// PreVisitUnaryExpr checks for address-of operator (&x) which is not supported
func (sema *SemaChecker) PreVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	if node.Op == token.AND {
		fmt.Println("\033[31m\033[1mCompilation error: address-of operator is not supported\033[0m")
		fmt.Println("  The address-of operator (&x) is not allowed.")
		fmt.Println("  goany targets languages with different memory models.")
		fmt.Println()
		fmt.Println("  \033[32mUse value types or redesign without pointers.\033[0m")
		os.Exit(-1)
	}
}

// PreVisitMapType validates map types - only supported key types allowed, no nested maps
func (sema *SemaChecker) PreVisitMapType(node *ast.MapType, indent int) {
	if sema.pkg == nil || sema.pkg.TypesInfo == nil {
		return
	}

	// Check for nested maps (map value is another map)
	if valueTv, ok := sema.pkg.TypesInfo.Types[node.Value]; ok && valueTv.Type != nil {
		if _, isMap := valueTv.Type.Underlying().(*types.Map); isMap {
			fmt.Println("\033[31m\033[1mCompilation error: nested maps are not supported\033[0m")
			fmt.Println("  Map values cannot be maps (e.g., map[K]map[K2]V2).")
			fmt.Println("  Nested maps have complex semantics across target languages.")
			fmt.Println()
			fmt.Println("  \033[32mUse a struct with a map field, or flatten the structure:\033[0m")
			fmt.Println("    type Entry struct { innerMap map[K2]V2 }")
			fmt.Println("    outerMap := make(map[K]Entry)")
			os.Exit(-1)
		}
	}

	// Check for unsupported key types
	if tv, ok := sema.pkg.TypesInfo.Types[node.Key]; ok && tv.Type != nil {
		if basic, isBasic := tv.Type.Underlying().(*types.Basic); isBasic {
			switch basic.Kind() {
			case types.String, types.Int, types.Bool,
				types.Int8, types.Int16, types.Int32, types.Int64,
				types.Uint8, types.Uint16, types.Uint32, types.Uint64,
				types.Float32, types.Float64:
				return // supported key types
			}
		}
		// If not a basic type or unsupported basic type, error
		fmt.Println("\033[31m\033[1mCompilation error: unsupported map key type\033[0m")
		fmt.Printf("  Map key type '%s' is not supported.\n", tv.Type.String())
		fmt.Println("  Supported key types: string, int, bool, int8-int64, uint8-uint64, float32, float64.")
		fmt.Println()
		fmt.Println("  \033[32mUse a supported primitive type as the key.\033[0m")
		os.Exit(-1)
	}
}

// Note: Structural checks for DeferStmt, GoStmt, ChanType, SelectStmt, LabeledStmt
// are now handled by the PatternChecker (whitelist approach).
// These constructs are rejected because no patterns exist for them in tests/examples.

// PreVisitBranchStmt checks for goto statements which are not supported
func (sema *SemaChecker) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	if node.Tok == token.GOTO {
		fmt.Println("\033[31m\033[1mCompilation error: goto statements are not supported\033[0m")
		fmt.Printf("  goto %s is not allowed.\n", node.Label.Name)
		fmt.Println("  Goto has limited support in target languages like Rust and JavaScript.")
		fmt.Println()
		fmt.Println("  \033[32mUse structured control flow (functions, loops with break).\033[0m")
		os.Exit(-1)
	}
}

// PreVisitFuncDecl checks for method receivers, init functions, variadic params, and named returns
func (sema *SemaChecker) PreVisitFuncDecl(node *ast.FuncDecl, indent int) {
	// Reset closure variables for each function to avoid false positives
	// between closures in different functions
	sema.closureVars = make(map[string]token.Pos)
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

	// Check for method receivers
	if node.Recv != nil && len(node.Recv.List) > 0 {
		fmt.Println("\033[31m\033[1mCompilation error: method receivers are not supported\033[0m")
		fmt.Printf("  Function '%s' has a receiver, making it a method.\n", node.Name.Name)
		fmt.Println("  Methods are not allowed; use standalone functions instead.")
		fmt.Println()
		fmt.Println("  \033[33mInstead of:\033[0m")
		fmt.Println("    func (t *Type) Method() { ... }")
		fmt.Println()
		fmt.Println("  \033[32mUse:\033[0m")
		fmt.Println("    func TypeMethod(t Type) Type { ... }")
		os.Exit(-1)
	}

	// Check for init functions
	if node.Name.Name == "init" {
		fmt.Println("\033[31m\033[1mCompilation error: init functions are not supported\033[0m")
		fmt.Println("  Package init() functions are not allowed.")
		fmt.Println("  Init functions have no direct equivalent in target languages.")
		fmt.Println()
		fmt.Println("  \033[32mCall initialization explicitly from main() or use constructors.\033[0m")
		os.Exit(-1)
	}

	// Check for variadic parameters
	if node.Type != nil && node.Type.Params != nil {
		for _, field := range node.Type.Params.List {
			if _, ok := field.Type.(*ast.Ellipsis); ok {
				fmt.Println("\033[31m\033[1mCompilation error: variadic functions are not supported\033[0m")
				fmt.Printf("  Function '%s' has variadic parameter (...T).\n", node.Name.Name)
				fmt.Println("  Variadic functions have different semantics across target languages.")
				fmt.Println()
				fmt.Println("  \033[32mUse a slice parameter instead:\033[0m")
				fmt.Println("    func foo(args []T) { ... }")
				os.Exit(-1)
			}
		}
	}

	// Check for named return values
	if node.Type != nil && node.Type.Results != nil {
		for _, field := range node.Type.Results.List {
			if len(field.Names) > 0 {
				fmt.Println("\033[31m\033[1mCompilation error: named return values are not supported\033[0m")
				fmt.Printf("  Function '%s' has named return values.\n", node.Name.Name)
				fmt.Println("  Named returns have no equivalent in C++, Rust, or JavaScript.")
				fmt.Println()
				fmt.Println("  \033[33mInstead of:\033[0m")
				fmt.Println("    func foo() (result int) { ... }")
				fmt.Println()
				fmt.Println("  \033[32mUse:\033[0m")
				fmt.Println("    func foo() int { ... }")
				os.Exit(-1)
			}
		}
	}
}

func (sema *SemaChecker) PreVisitGenDeclConstName(node *ast.Ident, indent int) {
	sema.constCtx = true

	// Check if the constant is declared without an explicit type
	if sema.pkg != nil && sema.pkg.TypesInfo != nil {
		if obj := sema.pkg.TypesInfo.Defs[node]; obj != nil {
			if constObj, ok := obj.(*types.Const); ok {
				if basic, ok := constObj.Type().(*types.Basic); ok {
					if basic.Info()&types.IsUntyped != 0 {
						fmt.Printf("\033[33m\033[1mWarning: constant '%s' declared without explicit type\033[0m\n", node.Name)
						fmt.Println("  For cross-platform compatibility, constants should have explicit types.")
						fmt.Println()
						fmt.Println("  \033[33mInstead of:\033[0m")
						fmt.Printf("    const %s = value\n", node.Name)
						fmt.Println()
						fmt.Println("  \033[32mUse explicit type:\033[0m")
						fmt.Printf("    const %s int = value\n", node.Name)
						fmt.Println()
					}
				}
			}
		}
	}
}

func (sema *SemaChecker) PreVisitIdent(node *ast.Ident, indent int) {
	if sema.constCtx {
		if node.String() == "iota" {
			fmt.Println("\033[31m\033[1mCompilation error : iota is not allowed for now\033[0m")
			os.Exit(-1)
		}
	}

	// Check if this identifier was consumed by string concatenation
	if sema.consumedStringVars != nil {
		if consumedPos, wasConsumed := sema.consumedStringVars[node.Name]; wasConsumed {
			// Only error if this use is after the consumption point
			if node.Pos() > consumedPos {
				fmt.Println("\033[31m\033[1mCompilation error: string variable reuse after concatenation\033[0m")
				fmt.Printf("  Variable '%s' was consumed by '+' and cannot be reused.\n", node.Name)
				fmt.Println("  This pattern fails in Rust due to move semantics.")
				fmt.Println()
				fmt.Println("  \033[33mInstead of:\033[0m")
				fmt.Printf("    y = %s + a\n", node.Name)
				fmt.Printf("    z = %s + b  // error: %s was moved\n", node.Name, node.Name)
				fmt.Println()
				fmt.Println("  \033[32mUse separate += statements:\033[0m")
				fmt.Println("    y += a")
				fmt.Println("    y += b")
				os.Exit(-1)
			}
		}
	}

	// Note: "whole struct use after field access" check removed
	// This pattern is valid in Go, and Rust handles it via .clone()
}

func (sema *SemaChecker) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	sema.constCtx = false
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
					fmt.Println("\033[31m\033[1mCompilation error: collection mutation during iteration\033[0m")
					fmt.Printf("  Variable '%s' is modified while being iterated over.\n", targetName)
					fmt.Println("  This pattern fails in Rust due to borrow checker rules.")
					fmt.Println()
					fmt.Println("  \033[33mInstead of:\033[0m")
					fmt.Printf("    for i := 0; i < len(%s); i++ {\n", targetName)
					fmt.Printf("        %s = append(%s, ...)  // mutation during iteration\n", targetName, targetName)
					fmt.Println("    }")
					fmt.Println()
					fmt.Println("  \033[32mCollect changes and apply after loop:\033[0m")
					fmt.Println("    var toAdd []T")
					fmt.Printf("    for i := 0; i < len(%s); i++ {\n", targetName)
					fmt.Println("        toAdd = append(toAdd, ...)")
					fmt.Println("    }")
					fmt.Printf("    %s = append(%s, toAdd...)\n", targetName, targetName)
					os.Exit(-1)
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

	// Check for range over maps - not supported
	if sema.pkg != nil && sema.pkg.TypesInfo != nil {
		if tv, ok := sema.pkg.TypesInfo.Types[node.X]; ok && tv.Type != nil {
			if _, isMap := tv.Type.Underlying().(*types.Map); isMap {
				fmt.Println("\033[31m\033[1mCompilation error: range over maps is not supported\033[0m")
				fmt.Println("  Iterating over maps with 'for k, v := range m' is not allowed.")
				fmt.Println("  Map iteration order is undefined and implementation varies across languages.")
				fmt.Println()
				fmt.Println("  \033[32mMaintain a separate slice of keys if iteration order matters:\033[0m")
				fmt.Println("    keys := []KeyType{...}")
				fmt.Println("    for _, k := range keys {")
				fmt.Println("        v := m[k]")
				fmt.Println("        // use k, v")
				fmt.Println("    }")
				os.Exit(-1)
			}
		}
	}

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

	// Check for range over inline composite literal (e.g., for _, x := range []int{1,2,3})
	if _, ok := node.X.(*ast.CompositeLit); ok {
		fmt.Println("\033[31m\033[1mCompilation error : range over inline slice literal (e.g., for _, x := range []int{1,2,3}) is not allowed for now\033[0m")
		os.Exit(-1)
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
						fmt.Println("\033[31m\033[1mCompilation error: collection mutation during iteration\033[0m")
						fmt.Printf("  Variable '%s' is modified while being iterated over.\n", targetName)
						fmt.Println("  This pattern fails in Rust due to borrow checker rules.")
						fmt.Println()
						fmt.Println("  \033[33mInstead of:\033[0m")
						fmt.Printf("    for _, v := range %s {\n", targetName)
						fmt.Printf("        %s = append(%s, v)  // mutation during iteration\n", targetName, targetName)
						fmt.Println("    }")
						fmt.Println()
						fmt.Println("  \033[32mCollect changes and apply after loop:\033[0m")
						fmt.Println("    var toAdd []T")
						fmt.Printf("    for _, v := range %s {\n", targetName)
						fmt.Println("        toAdd = append(toAdd, v)")
						fmt.Println("    }")
						fmt.Printf("    %s = append(%s, toAdd...)\n", targetName, targetName)
						os.Exit(-1)
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
			fmt.Println("\033[31m\033[1mCompilation error : nil comparison (== nil or != nil) is not allowed for now\033[0m")
			os.Exit(-1)
		}

		// Check for map comparisons (maps can only be compared to nil in Go, not to each other)
		if sema.pkg != nil && sema.pkg.TypesInfo != nil {
			// Check left operand
			if tvX, ok := sema.pkg.TypesInfo.Types[node.X]; ok && tvX.Type != nil {
				if _, isMap := tvX.Type.Underlying().(*types.Map); isMap {
					fmt.Println("\033[31m\033[1mCompilation error: map comparison is not supported\033[0m")
					fmt.Println("  Maps cannot be compared with == or !=.")
					fmt.Println("  In Go, maps can only be compared to nil, not to each other.")
					fmt.Println()
					fmt.Println("  \033[32mCompare map contents manually if needed:\033[0m")
					fmt.Println("    func mapsEqual(a, b map[K]V) bool {")
					fmt.Println("        if len(a) != len(b) { return false }")
					fmt.Println("        // compare each key-value pair")
					fmt.Println("    }")
					os.Exit(-1)
				}
			}
			// Check right operand
			if tvY, ok := sema.pkg.TypesInfo.Types[node.Y]; ok && tvY.Type != nil {
				if _, isMap := tvY.Type.Underlying().(*types.Map); isMap {
					fmt.Println("\033[31m\033[1mCompilation error: map comparison is not supported\033[0m")
					fmt.Println("  Maps cannot be compared with == or !=.")
					fmt.Println("  In Go, maps can only be compared to nil, not to each other.")
					fmt.Println()
					fmt.Println("  \033[32mCompare map contents manually if needed:\033[0m")
					fmt.Println("    func mapsEqual(a, b map[K]V) bool {")
					fmt.Println("        if len(a) != len(b) { return false }")
					fmt.Println("        // compare each key-value pair")
					fmt.Println("    }")
					os.Exit(-1)
				}
			}
		}
	}

	// Check for string concatenation that consumes variables (Rust move semantics)
	// Pattern: stringVar + "literal" or stringVar + otherVar
	// This pattern causes issues in Rust because the left operand is moved
	if node.Op == token.ADD {
		// Check if left operand is a string type identifier
		if ident, ok := node.X.(*ast.Ident); ok {
			if sema.pkg != nil && sema.pkg.TypesInfo != nil {
				if tv, exists := sema.pkg.TypesInfo.Types[node.X]; exists {
					if tv.Type != nil && tv.Type.String() == "string" {
						// Initialize map if needed
						if sema.consumedStringVars == nil {
							sema.consumedStringVars = make(map[string]token.Pos)
						}
						// Check if this variable was already consumed
						if consumedPos, wasConsumed := sema.consumedStringVars[ident.Name]; wasConsumed {
							if ident.Pos() > consumedPos {
								fmt.Println("\033[31m\033[1mCompilation error: string variable reuse after concatenation\033[0m")
								fmt.Printf("  Variable '%s' was consumed by '+' and cannot be reused.\n", ident.Name)
								fmt.Println("  This pattern fails in Rust due to move semantics.")
								fmt.Println()
								fmt.Println("  \033[33mInstead of:\033[0m")
								fmt.Printf("    y = %s + a\n", ident.Name)
								fmt.Printf("    z = %s + b  // error: %s was moved\n", ident.Name, ident.Name)
								fmt.Println()
								fmt.Println("  \033[32mUse separate += statements:\033[0m")
								fmt.Println("    y += a")
								fmt.Println("    y += b")
								os.Exit(-1)
							}
						}
						// Mark this variable as consumed at this position
						sema.consumedStringVars[ident.Name] = ident.Pos()
					}
				}
			}
		}
	}
}

// PreVisitAssignStmt checks for problematic patterns like: x += x + a
// where x is both borrowed (for +=) and moved (in x + a) in the same statement
// Also checks for slice self-assignment: slice[i] = slice[j]
// Also checks for variable shadowing (C# compatibility)
func (sema *SemaChecker) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	// Check for variable shadowing (C# does not allow shadowing within the same function)
	sema.checkVariableShadowing(node)

	// Check for slice self-assignment pattern
	sema.checkSliceSelfAssignment(node)

	// Check for += with string concatenation on RHS that uses the same variable
	if node.Tok == token.ADD_ASSIGN {
		for _, lhs := range node.Lhs {
			if lhsIdent, ok := lhs.(*ast.Ident); ok {
				// Check if this is a string type
				if sema.pkg != nil && sema.pkg.TypesInfo != nil {
					if tv, exists := sema.pkg.TypesInfo.Types[lhs]; exists {
						if tv.Type != nil && tv.Type.String() == "string" {
							// Check if RHS contains a binary + with this variable on the left
							if sema.rhsContainsStringConcatWithVar(node.Rhs[0], lhsIdent.Name) {
								fmt.Println("\033[31m\033[1mCompilation error: self-referencing string concatenation\033[0m")
								fmt.Printf("  Pattern '%s += %s + ...' is not allowed.\n", lhsIdent.Name, lhsIdent.Name)
								fmt.Printf("  Variable '%s' is both borrowed (+=) and moved (+) in the same statement.\n", lhsIdent.Name)
								fmt.Println()
								fmt.Println("  \033[33mInstead of:\033[0m")
								fmt.Printf("    %s += %s + other\n", lhsIdent.Name, lhsIdent.Name)
								fmt.Println()
								fmt.Println("  \033[32mUse separate statements:\033[0m")
								fmt.Printf("    %s += other\n", lhsIdent.Name)
								os.Exit(-1)
							}
						}
					}
				}
			}
		}
	}

	// Check for same non-Copy variable in binary expression: f(x) + g(x)
	for _, rhs := range node.Rhs {
		sema.checkBinaryExprWithSameVar(rhs)
	}

	// Check for mutation of variable after closure capture
	sema.checkClosureCaptureMutation(node)
}

// rhsContainsStringConcatWithVar checks if an expression contains a binary + with varName on the left
func (sema *SemaChecker) rhsContainsStringConcatWithVar(expr ast.Expr, varName string) bool {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			if ident, ok := e.X.(*ast.Ident); ok {
				if ident.Name == varName {
					return true
				}
			}
		}
		// Recursively check both sides
		return sema.rhsContainsStringConcatWithVar(e.X, varName) || sema.rhsContainsStringConcatWithVar(e.Y, varName)
	case *ast.ParenExpr:
		return sema.rhsContainsStringConcatWithVar(e.X, varName)
	}
	return false
}

// PostVisitAssignStmt clears consumed state for variables that are reassigned
// This handles patterns like: x = x + a; y = x + b (which should be valid)
func (sema *SemaChecker) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	if sema.consumedStringVars == nil {
		return
	}
	// Clear consumed state for any string variables on the LHS
	for _, lhs := range node.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok {
			delete(sema.consumedStringVars, ident.Name)
		}
	}
}

// checkVariableShadowing detects when a new variable declaration shadows an outer scope variable
// C# does not allow variable shadowing within the same function/method
func (sema *SemaChecker) checkVariableShadowing(node *ast.AssignStmt) {
	// Only check short variable declarations (:=)
	if node.Tok != token.DEFINE {
		return
	}

	if sema.scopeStack == nil || len(sema.scopeStack) == 0 {
		return
	}

	// Get the current scope (last element in the stack)
	currentScopeIdx := len(sema.scopeStack) - 1

	// Check each variable being declared
	for _, lhs := range node.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}

		// Skip blank identifier
		if ident.Name == "_" {
			continue
		}

		// Check if this variable exists in any outer scope (but not current scope)
		for i := 0; i < currentScopeIdx; i++ {
			if prevPos, exists := sema.scopeStack[i][ident.Name]; exists {
				fmt.Println("\033[31m\033[1mCompilation error: variable shadowing is not supported\033[0m")
				fmt.Printf("  Variable '%s' shadows a variable from an outer scope.\n", ident.Name)
				fmt.Println("  C# does not allow variable shadowing within the same function.")
				fmt.Println()
				fmt.Println("  \033[33mInstead of:\033[0m")
				fmt.Printf("    %s := ...  // outer scope\n", ident.Name)
				fmt.Println("    {")
				fmt.Printf("        %s := ...  // shadows outer %s\n", ident.Name, ident.Name)
				fmt.Println("    }")
				fmt.Println()
				fmt.Println("  \033[32mUse a different variable name:\033[0m")
				fmt.Printf("    %s := ...  // outer scope\n", ident.Name)
				fmt.Println("    {")
				fmt.Printf("        %s2 := ...  // or another descriptive name\n", ident.Name)
				fmt.Println("    }")
				_ = prevPos // suppress unused variable warning
				os.Exit(-1)
			}
		}

		// Add this variable to the current scope
		sema.scopeStack[currentScopeIdx][ident.Name] = ident.Pos()
	}
}

// PreVisitInterfaceType checks for interface types - only empty interface{} is supported
func (sema *SemaChecker) PreVisitInterfaceType(node *ast.InterfaceType, indent int) {
	// Empty interface (interface{} / any) is supported
	// Maps to: C++ std::any, Rust Box<dyn Any>, C# object
	// Non-empty interfaces are NOT supported
	if node.Methods != nil && len(node.Methods.List) > 0 {
		fmt.Println("\033[31m\033[1mCompilation error: non-empty interfaces are not supported\033[0m")
		fmt.Println("  Only empty interface (interface{} / any) is allowed.")
		fmt.Println("  Interfaces with methods have no direct equivalent in all target languages.")
		fmt.Println()
		fmt.Println("  \033[32mUse concrete types or interface{} with type assertions.\033[0m")
		os.Exit(-1)
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
			fmt.Println("\033[31m\033[1mCompilation error: struct embedding is not supported\033[0m")
			fmt.Printf("  Embedded field '%s' (anonymous field) is not allowed.\n", typeName)
			fmt.Println("  Struct embedding has different semantics in target languages.")
			fmt.Println()
			fmt.Println("  \033[33mInstead of:\033[0m")
			fmt.Printf("    type MyStruct struct { %s }\n", typeName)
			fmt.Println()
			fmt.Println("  \033[32mUse named field:\033[0m")
			fmt.Printf("    type MyStruct struct { field %s }\n", typeName)
			os.Exit(-1)
		}
	}
}

// PreVisitTypeSwitchStmt checks for type switch statements (not supported)
func (sema *SemaChecker) PreVisitTypeSwitchStmt(node *ast.TypeSwitchStmt, indent int) {
	fmt.Println("\033[31m\033[1mCompilation error : type switch statement is not allowed for now\033[0m")
	os.Exit(-1)
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

	// Check for map literals - not supported
	if _, isMap := tv.Type.Underlying().(*types.Map); isMap {
		fmt.Println("\033[31m\033[1mCompilation error: map literals are not supported\033[0m")
		fmt.Println("  Inline map initialization (e.g., map[K]V{k1: v1, k2: v2}) is not allowed.")
		fmt.Println()
		fmt.Println("  \033[33mInstead of:\033[0m")
		fmt.Println("    m := map[string]int{\"a\": 1, \"b\": 2}")
		fmt.Println()
		fmt.Println("  \033[32mUse make and assignment:\033[0m")
		fmt.Println("    m := make(map[string]int)")
		fmt.Println("    m[\"a\"] = 1")
		fmt.Println("    m[\"b\"] = 2")
		os.Exit(-1)
	}

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

// collectIdentifiers collects identifiers that are actually "consumed" (moved) in an expression
// It excludes identifiers used as the base of selector expressions (field access doesn't move)
// and identifiers that are the Sel part of selector expressions (field names)
func (sema *SemaChecker) collectIdentifiers(node ast.Node) []*ast.Ident {
	var idents []*ast.Ident
	// Track identifiers that are used as selector bases (field access doesn't move)
	selectorBases := make(map[*ast.Ident]bool)
	// Track identifiers that are field names (the Sel part of SelectorExpr)
	selectorFields := make(map[*ast.Ident]bool)

	// First pass: find all selector bases and field names
	ast.Inspect(node, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok := sel.X.(*ast.Ident); ok {
				selectorBases[ident] = true
			}
			// The Sel part is always the field name, not a variable
			selectorFields[sel.Sel] = true
		}
		return true
	})

	// Second pass: collect identifiers that are not selector bases or field names
	ast.Inspect(node, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			if !selectorBases[ident] && !selectorFields[ident] {
				idents = append(idents, ident)
			}
		}
		return true
	})
	return idents
}

// getDirectFunctionArgs returns variable names passed directly to a function call
// Returns empty if the expression is not a direct function call
func (sema *SemaChecker) getDirectFunctionArgs(expr ast.Expr) map[string]bool {
	args := make(map[string]bool)
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return args
	}

	// Skip built-in functions
	if ident, ok := call.Fun.(*ast.Ident); ok {
		builtins := map[string]bool{
			"len": true, "cap": true, "append": true, "copy": true,
			"make": true, "new": true, "delete": true, "close": true,
			"panic": true, "recover": true, "print": true, "println": true,
			"complex": true, "real": true, "imag": true,
		}
		if builtins[ident.Name] {
			return args
		}
	}

	// Collect direct identifier arguments
	for _, arg := range call.Args {
		if ident, ok := arg.(*ast.Ident); ok {
			args[ident.Name] = true
		}
	}
	return args
}

// checkBinaryExprWithSameVar checks for binary expressions like f(x) + g(x)
// where the same non-Copy variable is passed to function calls on both sides
func (sema *SemaChecker) checkBinaryExprWithSameVar(node ast.Node) {
	if sema.pkg == nil || sema.pkg.TypesInfo == nil {
		return
	}

	ast.Inspect(node, func(n ast.Node) bool {
		// Skip closure bodies
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}

		binExpr, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}

		// Get args from left and right sides (only if they are direct function calls)
		leftArgs := sema.getDirectFunctionArgs(binExpr.X)
		rightArgs := sema.getDirectFunctionArgs(binExpr.Y)

		// Check for same variable in both sides
		for varName := range leftArgs {
			if rightArgs[varName] {
				// Check if it's a non-Copy type
				// We need to find the identifier to get type info
				var foundIdent *ast.Ident
				ast.Inspect(binExpr.X, func(inner ast.Node) bool {
					if ident, ok := inner.(*ast.Ident); ok && ident.Name == varName {
						foundIdent = ident
						return false
					}
					return true
				})

				if foundIdent != nil {
					if obj := sema.pkg.TypesInfo.Uses[foundIdent]; obj != nil {
						if _, isConst := obj.(*types.Const); !isConst {
							if _, isFunc := obj.(*types.Func); !isFunc {
								if sema.isNonCopyType(obj.Type()) {
									fmt.Println("\033[31m\033[1mCompilation error: same variable used multiple times in expression\033[0m")
									fmt.Printf("  Variable '%s' (non-Copy type) appears in both sides of a binary expression.\n", varName)
									fmt.Println("  This pattern fails in Rust due to move semantics.")
									fmt.Println()
									fmt.Println("  \033[33mInstead of:\033[0m")
									fmt.Printf("    foo(%s) + bar(%s)\n", varName, varName)
									fmt.Println()
									fmt.Println("  \033[32mUse separate statements:\033[0m")
									fmt.Printf("    a := foo(%s)\n", varName)
									fmt.Printf("    b := bar(%s.clone())  // or redesign to avoid multiple uses\n", varName)
									os.Exit(-1)
								}
							}
						}
					}
				}
			}
		}
		return true
	})
}

// checkNestedCallArgSharing checks for nested function calls sharing non-Copy variable: f(x, g(x))
func (sema *SemaChecker) checkNestedCallArgSharing(node ast.Node) {
	if sema.pkg == nil || sema.pkg.TypesInfo == nil {
		return
	}

	ast.Inspect(node, func(n ast.Node) bool {
		// Skip closure bodies
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}

		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Skip built-in functions
		if ident, ok := call.Fun.(*ast.Ident); ok {
			builtins := map[string]bool{
				"len": true, "cap": true, "append": true, "copy": true,
				"make": true, "new": true, "delete": true, "close": true,
				"panic": true, "recover": true, "print": true, "println": true,
				"complex": true, "real": true, "imag": true, "int": true,
				"int8": true, "int16": true, "int32": true, "int64": true,
				"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
				"string": true, "byte": true, "rune": true,
			}
			if builtins[ident.Name] {
				return true
			}
		}

		// Collect direct identifier arguments at the top level
		topLevelArgs := make(map[string]*ast.Ident)
		for _, arg := range call.Args {
			if ident, ok := arg.(*ast.Ident); ok {
				topLevelArgs[ident.Name] = ident
			}
		}

		// Check if any nested call uses the same variable (direct argument, not field access)
		for _, arg := range call.Args {
			if nestedCall, ok := arg.(*ast.CallExpr); ok {
				nestedArgs := sema.collectDirectCallArgIdents(nestedCall)
				for _, nestedIdent := range nestedArgs {
					if topIdent, exists := topLevelArgs[nestedIdent.Name]; exists {
						// Check if it's a non-Copy type
						if obj := sema.pkg.TypesInfo.Uses[topIdent]; obj != nil {
							if _, isConst := obj.(*types.Const); !isConst {
								if _, isFunc := obj.(*types.Func); !isFunc {
									if sema.isNonCopyType(obj.Type()) {
										fmt.Println("\033[31m\033[1mCompilation error: nested function calls share non-Copy variable\033[0m")
										fmt.Printf("  Variable '%s' is passed to outer function and used in nested call.\n", nestedIdent.Name)
										fmt.Println("  This pattern fails in Rust due to move semantics.")
										fmt.Println()
										fmt.Println("  \033[33mInstead of:\033[0m")
										fmt.Printf("    f(%s, g(%s))\n", nestedIdent.Name, nestedIdent.Name)
										fmt.Println()
										fmt.Println("  \033[32mUse separate statements:\033[0m")
										fmt.Printf("    tmp := g(%s)\n", nestedIdent.Name)
										fmt.Printf("    f(%s, tmp)  // or clone if needed\n", nestedIdent.Name)
										os.Exit(-1)
									}
								}
							}
						}
					}
				}
			}
		}
		return true
	})
}

// collectDirectCallArgIdents collects identifiers that are passed directly as arguments
// to function calls within an expression (not field accesses like ctx.Field)
func (sema *SemaChecker) collectDirectCallArgIdents(expr ast.Expr) []*ast.Ident {
	var idents []*ast.Ident
	ast.Inspect(expr, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			for _, arg := range call.Args {
				// Only collect direct identifiers, not selector expressions
				if ident, ok := arg.(*ast.Ident); ok {
					idents = append(idents, ident)
				}
			}
		}
		return true
	})
	return idents
}

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
							fmt.Println("\033[31m\033[1mCompilation error: mutation of variable after closure capture\033[0m")
							fmt.Printf("  Variable '%s' is captured by a closure and then mutated.\n", ident.Name)
							fmt.Println("  This pattern fails in Rust due to ownership rules.")
							fmt.Println()
							fmt.Println("  \033[33mInstead of:\033[0m")
							fmt.Printf("    fn := func() { use(%s) }\n", ident.Name)
							fmt.Printf("    %s = modify(%s)  // mutation after capture\n", ident.Name, ident.Name)
							fmt.Println()
							fmt.Println("  \033[32mMutate before capture or use separate variables:\033[0m")
							fmt.Printf("    %s = modify(%s)\n", ident.Name, ident.Name)
							fmt.Printf("    fn := func() { use(%s) }  // capture after mutation\n", ident.Name)
							os.Exit(-1)
						}
					}
				}
			}
		}
	}
}

// checkSliceSelfAssignment checks for slice self-reference patterns that cause Rust borrow issues:
// - slice[i] = slice[j] (direct self-assignment)
// - slice[i] = slice[i] + slice[j] (self-reference in expression)
// - slice[i] = f(slice[i]) (self-reference through function call)
// - slice = append(slice, slice[i]) (append with self-reference)
// Note: This only applies to slices of non-Copy types (strings, structs, slices).
// For Copy types (int, bool, etc.), self-reference is safe.
func (sema *SemaChecker) checkSliceSelfAssignment(node *ast.AssignStmt) {
	if sema.pkg == nil || sema.pkg.TypesInfo == nil {
		return
	}

	// Check for slice = append(slice, slice[i]) pattern
	// LHS is a plain identifier, RHS is append call with self-reference
	// Note: This applies to ALL slice element types, not just non-Copy types,
	// because in Rust the borrow checker prevents simultaneous mutable (&mut for append)
	// and immutable (&slice[i]) borrows regardless of element type.
	for i, lhs := range node.Lhs {
		lhsIdent, ok := lhs.(*ast.Ident)
		if !ok {
			continue
		}

		// Check if LHS is a slice type
		tv, exists := sema.pkg.TypesInfo.Types[lhs]
		if !exists {
			continue
		}
		_, isSlice := tv.Type.Underlying().(*types.Slice)
		if !isSlice {
			continue
		}

		// Get the corresponding RHS
		if i >= len(node.Rhs) {
			continue
		}
		rhs := node.Rhs[i]

		// Check if RHS is an append call
		callExpr, ok := rhs.(*ast.CallExpr)
		if !ok {
			continue
		}
		funIdent, ok := callExpr.Fun.(*ast.Ident)
		if !ok || funIdent.Name != "append" {
			continue
		}

		// Check if first arg is the same slice as LHS
		if len(callExpr.Args) < 2 {
			continue
		}
		firstArg, ok := callExpr.Args[0].(*ast.Ident)
		if !ok || firstArg.Name != lhsIdent.Name {
			continue
		}

		// Check if any other argument contains an access to the same slice
		for j := 1; j < len(callExpr.Args); j++ {
			if sema.exprContainsSliceAccess(callExpr.Args[j], lhsIdent.Name) {
				fmt.Println("\033[31m\033[1mCompilation error: slice self-reference in append\033[0m")
				fmt.Printf("  Slice '%s' is both mutated (via append) and read from in the same statement.\n", lhsIdent.Name)
				fmt.Println("  This causes Rust borrow checker issues (simultaneous mutable and immutable borrow).")
				fmt.Println()
				fmt.Println("  \033[33mProblematic pattern:\033[0m")
				fmt.Printf("    %s = append(%s, %s[i])\n", lhsIdent.Name, lhsIdent.Name, lhsIdent.Name)
				fmt.Println()
				fmt.Println("  \033[32mUse a temporary variable:\033[0m")
				fmt.Printf("    tmp := %s[i]\n", lhsIdent.Name)
				fmt.Printf("    %s = append(%s, tmp)\n", lhsIdent.Name, lhsIdent.Name)
				os.Exit(-1)
			}
		}
	}

	// Check if LHS is an index expression on a slice
	for i, lhs := range node.Lhs {
		lhsIndex, ok := lhs.(*ast.IndexExpr)
		if !ok {
			continue
		}

		// Get the slice name from LHS
		lhsSlice, ok := lhsIndex.X.(*ast.Ident)
		if !ok {
			continue
		}

		// Check if it's a slice type and get element type
		tv, exists := sema.pkg.TypesInfo.Types[lhsIndex.X]
		if !exists {
			continue
		}
		sliceType, isSlice := tv.Type.Underlying().(*types.Slice)
		if !isSlice {
			continue
		}

		// Skip check for Copy types (primitives) - these are safe for self-reference in Rust
		if sema.isCopyType(sliceType.Elem()) {
			continue
		}

		// Get the corresponding RHS
		if i >= len(node.Rhs) {
			continue
		}
		rhs := node.Rhs[i]

		// Check if RHS contains any index expression on the same slice
		if sema.exprContainsSliceAccess(rhs, lhsSlice.Name) {
			fmt.Println("\033[31m\033[1mCompilation error: slice self-reference pattern\033[0m")
			fmt.Printf("  Slice '%s' is both written to and read from in the same statement.\n", lhsSlice.Name)
			fmt.Println("  This causes Rust borrow checker issues (simultaneous mutable and immutable borrow).")
			fmt.Println()
			fmt.Println("  \033[33mProblematic patterns:\033[0m")
			fmt.Printf("    %s[i] = %s[j]\n", lhsSlice.Name, lhsSlice.Name)
			fmt.Printf("    %s[i] = %s[i] + %s[j]\n", lhsSlice.Name, lhsSlice.Name, lhsSlice.Name)
			fmt.Printf("    %s[i] = f(%s[i])\n", lhsSlice.Name, lhsSlice.Name)
			fmt.Println()
			fmt.Println("  \033[32mUse a temporary variable:\033[0m")
			fmt.Printf("    tmp := %s[j]  // or the expression using %s\n", lhsSlice.Name, lhsSlice.Name)
			fmt.Printf("    %s[i] = tmp\n", lhsSlice.Name)
			os.Exit(-1)
		}
	}
}

// exprContainsSliceAccess checks if an expression contains an index access to the given slice
func (sema *SemaChecker) exprContainsSliceAccess(expr ast.Expr, sliceName string) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		if indexExpr, ok := n.(*ast.IndexExpr); ok {
			if ident, ok := indexExpr.X.(*ast.Ident); ok {
				if ident.Name == sliceName {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

// isCopyType checks if a Go type maps to a Rust Copy type (primitives that are copied, not moved)
func (sema *SemaChecker) isCopyType(t types.Type) bool {
	switch underlying := t.Underlying().(type) {
	case *types.Basic:
		// All basic types (int, bool, float, byte, etc.) are Copy in Rust
		switch underlying.Kind() {
		case types.Bool,
			types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Uintptr,
			types.Float32, types.Float64,
			types.Complex64, types.Complex128:
			return true
		}
	}
	// Strings, slices, structs, etc. are NOT Copy (they're moved)
	return false
}

// PreVisitFuncLit checks for multiple closures capturing the same non-Copy variable
func (sema *SemaChecker) PreVisitFuncLit(node *ast.FuncLit, indent int) {
	// Track that we're inside a closure (for mutation-after-capture check)
	sema.insideClosureDepth++

	if sema.pkg == nil || sema.pkg.TypesInfo == nil {
		return
	}

	// Collect all identifiers used in the closure body
	idents := sema.collectIdentifiers(node.Body)

	// Dedupe: track which variables we've already processed for this closure
	processedInThisClosure := make(map[string]bool)

	for _, ident := range idents {
		// Skip if we've already processed this variable name in this closure
		if processedInThisClosure[ident.Name] {
			continue
		}

		// Get the object this identifier refers to
		if obj := sema.pkg.TypesInfo.Uses[ident]; obj != nil {
			// Skip constants, functions, and type names
			if _, isConst := obj.(*types.Const); isConst {
				continue
			}
			if _, isFunc := obj.(*types.Func); isFunc {
				continue
			}
			if _, isTypeName := obj.(*types.TypeName); isTypeName {
				continue
			}

			// Check if it's a non-Copy type
			if !sema.isNonCopyType(obj.Type()) {
				continue
			}

			// Check if this is a variable from an outer scope (closure capture)
			// by verifying it was declared before this function literal
			if obj.Pos() < node.Pos() {
				// Mark as processed for this closure
				processedInThisClosure[ident.Name] = true

				// Check if another closure already captured this variable
				if prevPos, exists := sema.closureVars[ident.Name]; exists {
					fmt.Println("\033[31m\033[1mCompilation error: multiple closures capture same variable\033[0m")
					fmt.Printf("  Variable '%s' (non-Copy type) is captured by multiple closures.\n", ident.Name)
					fmt.Println("  This causes Rust borrow checker issues.")
					fmt.Println()
					fmt.Println("  \033[33mInstead of:\033[0m")
					fmt.Printf("    fn1 := func() { use(%s) }\n", ident.Name)
					fmt.Printf("    fn2 := func() { use(%s) }  // error: already captured\n", ident.Name)
					fmt.Println()
					fmt.Println("  \033[32mUse Arc/Rc for shared ownership or redesign:\033[0m")
					fmt.Printf("    %s1 := %s.clone()\n", ident.Name, ident.Name)
					fmt.Printf("    fn1 := func() { use(%s) }\n", ident.Name)
					fmt.Printf("    fn2 := func() { use(%s1) }\n", ident.Name)
					_ = prevPos // suppress unused variable warning
					os.Exit(-1)
				}
				// Mark this variable as captured by a closure
				sema.closureVars[ident.Name] = node.Pos()
				// Also track for mutation-after-capture detection
				if sema.closureCaptures == nil {
					sema.closureCaptures = make(map[string]token.Pos)
				}
				sema.closureCaptures[ident.Name] = node.Pos()
			}
		}
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

	// Skip built-in functions - they handle arguments specially
	if ident, ok := node.Fun.(*ast.Ident); ok {
		builtins := map[string]bool{
			"len": true, "cap": true, "append": true, "copy": true,
			"make": true, "new": true, "delete": true, "close": true,
			"panic": true, "recover": true, "print": true, "println": true,
			"complex": true, "real": true, "imag": true,
		}
		if builtins[ident.Name] {
			return
		}
	}

	// Collect identifiers that are direct arguments (not nested in expressions)
	argIdents := make(map[string]*ast.Ident)
	for _, arg := range node.Args {
		if ident, ok := arg.(*ast.Ident); ok {
			// Check if we've already seen this identifier
			if prevIdent, exists := argIdents[ident.Name]; exists {
				// Check if it's a non-Copy type
				if obj := sema.pkg.TypesInfo.Uses[ident]; obj != nil {
					if _, isConst := obj.(*types.Const); !isConst {
						if _, isFunc := obj.(*types.Func); !isFunc {
							if sema.isNonCopyType(obj.Type()) {
								fmt.Println("\033[31m\033[1mCompilation error: same variable used multiple times as function argument\033[0m")
								fmt.Printf("  Variable '%s' (non-Copy type) is passed multiple times to the same function call.\n", ident.Name)
								fmt.Println("  This pattern fails in Rust due to move semantics - the variable is moved on first use.")
								fmt.Println()
								fmt.Println("  \033[33mInstead of:\033[0m")
								fmt.Printf("    f(%s, %s)\n", ident.Name, ident.Name)
								fmt.Println()
								fmt.Println("  \033[32mClone the variable for the second use:\033[0m")
								fmt.Printf("    f(%s, %s.clone())\n", ident.Name, ident.Name)
								fmt.Println("  Or use separate variables:")
								fmt.Printf("    %s2 := %s  // explicit copy\n", ident.Name, ident.Name)
								fmt.Printf("    f(%s, %s2)\n", ident.Name, ident.Name)
								_ = prevIdent // suppress unused variable warning
								os.Exit(-1)
							}
						}
					}
				}
			}
			argIdents[ident.Name] = ident
		}
	}

	// Check for nested function calls sharing non-Copy variable: outer(inner(data))
	sema.checkNestedCallArgSharing(node)
}
