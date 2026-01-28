package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

var rustDestTypes = []string{"i8", "i16", "i32", "i64", "u8", "u16", "Box<dyn Any>", "String", "i32"}

var rustTypesMap = map[string]string{
	"int8":    rustDestTypes[0],
	"int16":   rustDestTypes[1],
	"int32":   rustDestTypes[2],
	"int64":   rustDestTypes[3],
	"uint8":   rustDestTypes[4],
	"uint16":  rustDestTypes[5],
	"uint32":  "u32",
	"uint64":  "u64",
	"any":     rustDestTypes[6],
	"string":  rustDestTypes[7],
	"int":     rustDestTypes[8],
	"bool":    "bool",
	"float32": "f32",
	"float64": "f64",
}

// mapGoTypeToRust converts a Go type string to its Rust equivalent
func (re *RustEmitter) mapGoTypeToRust(goType string) string {
	if rustType, ok := rustTypesMap[goType]; ok {
		return rustType
	}
	// Return the original if not found in map
	return goType
}

type RustEmitter struct {
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string // Path to runtime directory (empty = disabled)
	GraphicsRuntime string // Graphics backend: tigr (default), sdl2, none
	OptimizeMoves   bool   // Enable move optimizations to reduce struct cloning
	MoveOptCount    int    // Count of clones removed by move optimizations
	file            *os.File
	BaseEmitter
	pkg                          *packages.Package
	insideForPostCond            bool
	assignmentToken              string
	forwardDecls                 bool
	shouldGenerate               bool
	numFuncResults               int
	aliases                      map[string]Alias
	currentPackage               string
	isArray                      bool
	arrayType                    string
	isTuple                      bool
	sawIncrement                 bool                    // Track if we saw ++ in for loop post statement
	sawDecrement                 bool                    // Track if we saw -- in for loop post statement
	forLoopStep                  int                     // Step value for += n or -= n (0 means not set)
	forLoopInclusive             bool                    // Track if condition uses <= or >= (inclusive range)
	forLoopReverse               bool                    // Track if loop should be reversed (decrement or >)
	isInfiniteLoop               bool                    // Track if current for loop is infinite (no init, cond, post)
	declType                     string                  // Store the type for multi-name declarations
	declNameCount                int                     // Count of names in current declaration
	declNameIndex                int                     // Current name index
	inAssignRhs                  bool                    // Track if we're in assignment RHS
	inAssignLhs                  bool                    // Track if we're in assignment LHS
	inFieldAssign                bool                    // Track if we're assigning to a struct field
	isArrayStack                 []bool                  // Stack to save/restore isArray for nested composite literals
	pkgHasInterfaceTypes         bool                    // Track if current package has any interface{} types
	currentCompLitTypeNoDefault  bool                    // Track if current composite literal's type doesn't derive Default
	compLitTypeNoDefaultStack    []bool                  // Stack to save/restore currentCompLitTypeNoDefault for nested composite literals
	inFuncParam                  bool                    // Track if we're in function parameter type (for slice -> &[T])
	currentCallIsAppend          bool                    // Track if current function call is to append (takes ownership)
	inCallExprArg                bool                    // Track if we're inside a call expression argument (for closure wrapping)
	closureWrappedInRc           bool                    // Track if current closure was wrapped in Rc::new()
	currentCompLitType           types.Type              // Track the current composite literal's type for checking at post-visit
	compLitTypeStack             []types.Type            // Stack of composite literal types
	processedPkgsInterfaceTypes  map[string]bool         // Cache for package interface{} type checks
	inKeyValueExpr               bool                    // Track if we're inside a KeyValueExpr (struct field init)
	inMultiValueReturn           bool                    // Track if we're in a multi-value return statement
	multiValueReturnResultIndex  int                     // Current result index in multi-value return
	inReturnStmt                 bool                    // Track if we're inside a return statement
	inMultiValueDecl             bool                    // Track if we're in a multi-value := declaration
	currentFuncReturnsAny        bool                    // Track if current function returns any/interface{}
	callExprFunMarkerStack       []int                   // Stack of indices for nested call markers
	callExprFunEndMarkerStack    []int                   // Stack of end indices for nested call markers
	callExprArgsMarkerStack      []int                   // Stack of indices for nested call arg markers
	localClosureAssign           bool                    // Track if current assignment has a function literal RHS
	localClosures                map[string]*ast.FuncLit // Map of local closure names to their AST
	localClosureBodyTokens       map[string][]Token      // Map of local closure names to their body tokens
	currentClosureName           string                  // Name of the variable being assigned a closure
	inLocalClosureInline         bool                    // Track if we're inlining a local closure
	inLocalClosureBody           bool                    // Track if we're inside a local closure body being processed
	localClosureBodyStartIndex   int                     // Token index where closure body starts (after opening brace)
	localClosureAssignStartIndex int                     // Token index where the assignment statement starts
	currentCompLitIsSlice        bool                    // Track if current composite literal is a slice type alias
	binaryNeedsLeftCast          bool                    // Track if left operand of binary expr needs cast to i32
	binaryNeedsLeftCastStack     []bool                  // Stack for nested binary expressions
	binaryNeedsRightCast         string                  // Type to cast right operand of binary expr (e.g., "u8")
	binaryNeedsRightCastStack    []string                // Stack for nested binary expressions
	inFloatBinaryExpr            bool                    // Track if we're in a binary expr where operands should be float
	inFloatBinaryExprStack       []bool                  // Stack for nested binary expressions
	// Key-value range loop support
	isKeyValueRange              bool
	rangeKeyName                 string
	rangeValueName               string
	rangeCollectionExpr          string
	captureRangeExpr             bool
	suppressRangeEmit            bool
	rangeStmtIndent              int
	// Liveness analysis for cross-statement clone detection
	varFutureUses                map[string]bool   // Variables that will be used in later statements from current position
	currentFuncBody              *ast.BlockStmt    // Current function body being processed
	currentStmtIndex             int               // Index of current statement in function body
	stmtVarUsages                []map[string]bool // Variable usages per statement
	// Parameter mutation tracking
	mutatedParams                map[string]bool   // Parameters that are assigned to in the function body
	// Compound condition for loop support (generates while loop with manual increment)
	pendingLoopIncrement         bool              // Track if we need to add increment at end of next block
	loopIncrementVar             string            // Variable to increment
	loopIncrementOp              string            // Operator: += or -=
	loopIncrementVal             string            // Value to increment by
	inForLoopBody                bool              // Track if current block is the for loop body
	forLoopBodyDepth             int               // Depth counter to track nested blocks within loop body
	// Move optimization: avoid unnecessary .clone() for structs
	currentAssignLhsNames        map[string]bool   // LHS variable names of current assignment
	currentCallArgIdentsStack    []map[string]int  // Stack of identifier counts per nested call
	currentReturnNode            *ast.ReturnStmt   // Current return statement being processed
	funcLitDepth                 int               // Nesting depth of function literals (closures)
	// Temp extraction: pre-extract field accesses so struct can be moved
	moveOptTempBindings          []string          // temp var declarations to emit before assignment
	moveOptArgReplacements       map[int]string    // arg index → replacement variable name
	moveOptModifiedCounts        map[string]int    // modified ident counts excluding extracted fields
	moveOptActive                bool              // whether extraction is active for current assignment
	moveOptReplacingArg          bool              // whether current arg is being replaced
	moveOptArgStartMarker        int               // token buffer position at start of replaced arg
}

func (*RustEmitter) lowerToBuiltins(selector string) string {
	switch selector {
	case "fmt":
		return ""
	case "Sprintf":
		return "string_format"
	case "Println":
		return "println"
	case "Printf":
		return "printf"
	case "Print":
		return "printf"
	case "len":
		return "len"
	}
	return selector
}

// escapeRustKeyword escapes Rust reserved keywords with r# prefix
func escapeRustKeyword(name string) string {
	// Rust reserved keywords that might conflict with Go identifiers
	rustKeywords := map[string]bool{
		"as": true, "break": true, "const": true, "continue": true,
		"crate": true, "else": true, "enum": true, "extern": true,
		// Note: "false" and "true" are NOT escaped - they're boolean literals
		"fn": true, "for": true, "if": true,
		"impl": true, "in": true, "let": true, "loop": true,
		"match": true, "mod": true, "move": true, "mut": true,
		"pub": true, "ref": true, "return": true, "self": true,
		"Self": true, "static": true, "struct": true, "super": true,
		"trait": true, "type": true, "unsafe": true,
		"use": true, "where": true, "while": true,
		// Reserved for future use
		"abstract": true, "async": true, "await": true, "become": true,
		"box": true, "do": true, "final": true, "macro": true,
		"override": true, "priv": true, "try": true, "typeof": true,
		"unsized": true, "virtual": true, "yield": true,
	}
	if rustKeywords[name] {
		return "r#" + name
	}
	return name
}

func (re *RustEmitter) emitAsString(s string, indent int) string {
	return strings.Repeat(" ", indent) + s
}

// Helper function to determine token type for Rust specific content
func (re *RustEmitter) getTokenType(content string) TokenType {
	// Check for Rust keywords
	switch content {
	case "fn", "let", "mut", "impl", "trait", "mod", "use", "pub", "struct", "enum", "match", "if", "else", "loop", "while", "for", "in", "return", "break", "continue":
		return RustKeyword
	case "(":
		return LeftParen
	case ")":
		return RightParen
	case "{":
		return LeftBrace
	case "}":
		return RightBrace
	case "[":
		return LeftBracket
	case "]":
		return RightBracket
	case ";":
		return Semicolon
	case ",":
		return Comma
	case ".":
		return Dot
	case "=", "+=", "-=", "*=", "/=":
		return Assignment
	case "+", "-", "*", "/", "%":
		return ArithmeticOperator
	case "==", "!=", "<", ">", "<=", ">=":
		return ComparisonOperator
	case "&&", "||", "!":
		return LogicalOperator
	case "++":
		return UnaryOperator
	case " ", "\t":
		return WhiteSpace
	case "\n":
		return NewLine
	}

	// Check if it's a number
	if len(content) > 0 && (content[0] >= '0' && content[0] <= '9') {
		return NumberLiteral
	}

	// Check if it's a string literal
	if len(content) >= 2 && content[0] == '"' && content[len(content)-1] == '"' {
		return StringLiteral
	}

	// Default to identifier
	return Identifier
}

// Helper function to emit token
func (re *RustEmitter) emitToken(content string, tokenType TokenType, indent int) {
	token := CreateToken(tokenType, re.emitAsString(content, indent))
	_ = re.gir.emitTokenToFileBuffer(token, EmptyVisitMethod)
}

// Helper function to convert []Token to []string for backward compatibility
func tokensToStrings(tokens []Token) []string {
	result := make([]string, len(tokens))
	for i, token := range tokens {
		result[i] = token.Content
	}
	return result
}

// collectIdentifiersInCallArgs collects all identifiers used as function call arguments in an expression
func (re *RustEmitter) collectIdentifiersInCallArgs(expr ast.Expr, result map[string]bool) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Ident:
		// Don't collect type names or package names
		if obj := re.pkg.TypesInfo.ObjectOf(e); obj != nil {
			if _, isTypeName := obj.(*types.TypeName); isTypeName {
				return
			}
			if _, isPkgName := obj.(*types.PkgName); isPkgName {
				return
			}
		}
		result[e.Name] = true
	case *ast.CallExpr:
		// Collect identifiers in the arguments
		for _, arg := range e.Args {
			re.collectIdentifiersInCallArgs(arg, result)
		}
		// Also check the function being called (for method receivers)
		re.collectIdentifiersInCallArgs(e.Fun, result)
	case *ast.SelectorExpr:
		// For selectors like x.Method(), collect the base (x)
		re.collectIdentifiersInCallArgs(e.X, result)
	case *ast.IndexExpr:
		re.collectIdentifiersInCallArgs(e.X, result)
		re.collectIdentifiersInCallArgs(e.Index, result)
	case *ast.BinaryExpr:
		re.collectIdentifiersInCallArgs(e.X, result)
		re.collectIdentifiersInCallArgs(e.Y, result)
	case *ast.UnaryExpr:
		re.collectIdentifiersInCallArgs(e.X, result)
	case *ast.ParenExpr:
		re.collectIdentifiersInCallArgs(e.X, result)
	case *ast.TypeAssertExpr:
		re.collectIdentifiersInCallArgs(e.X, result)
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			re.collectIdentifiersInCallArgs(elt, result)
		}
	case *ast.KeyValueExpr:
		re.collectIdentifiersInCallArgs(e.Value, result)
	}
}

// collectCallArgIdentCounts counts how many times each base identifier
// appears across all arguments of a function call
func (re *RustEmitter) collectCallArgIdentCounts(args []ast.Expr) map[string]int {
	counts := make(map[string]int)
	for _, arg := range args {
		re.countIdentsInExpr(arg, counts)
	}
	return counts
}

// countIdentsInExpr recursively counts identifier occurrences in an expression
func (re *RustEmitter) countIdentsInExpr(expr ast.Expr, counts map[string]int) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Ident:
		if obj := re.pkg.TypesInfo.ObjectOf(e); obj != nil {
			if _, isTypeName := obj.(*types.TypeName); isTypeName {
				return
			}
			if _, isPkgName := obj.(*types.PkgName); isPkgName {
				return
			}
		}
		counts[e.Name]++
	case *ast.SelectorExpr:
		re.countIdentsInExpr(e.X, counts)
	case *ast.CallExpr:
		for _, arg := range e.Args {
			re.countIdentsInExpr(arg, counts)
		}
		re.countIdentsInExpr(e.Fun, counts)
	case *ast.IndexExpr:
		re.countIdentsInExpr(e.X, counts)
		re.countIdentsInExpr(e.Index, counts)
	case *ast.BinaryExpr:
		re.countIdentsInExpr(e.X, counts)
		re.countIdentsInExpr(e.Y, counts)
	case *ast.UnaryExpr:
		re.countIdentsInExpr(e.X, counts)
	case *ast.ParenExpr:
		re.countIdentsInExpr(e.X, counts)
	case *ast.TypeAssertExpr:
		re.countIdentsInExpr(e.X, counts)
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			re.countIdentsInExpr(elt, counts)
		}
	case *ast.KeyValueExpr:
		re.countIdentsInExpr(e.Value, counts)
	}
}

// exprContainsIdent checks if an expression references a given identifier
func (re *RustEmitter) exprContainsIdent(expr ast.Expr, name string) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == name
	case *ast.SelectorExpr:
		return re.exprContainsIdent(e.X, name)
	case *ast.CallExpr:
		for _, arg := range e.Args {
			if re.exprContainsIdent(arg, name) {
				return true
			}
		}
		return re.exprContainsIdent(e.Fun, name)
	case *ast.IndexExpr:
		return re.exprContainsIdent(e.X, name) || re.exprContainsIdent(e.Index, name)
	case *ast.BinaryExpr:
		return re.exprContainsIdent(e.X, name) || re.exprContainsIdent(e.Y, name)
	case *ast.UnaryExpr:
		return re.exprContainsIdent(e.X, name)
	case *ast.ParenExpr:
		return re.exprContainsIdent(e.X, name)
	case *ast.TypeAssertExpr:
		return re.exprContainsIdent(e.X, name)
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			if re.exprContainsIdent(elt, name) {
				return true
			}
		}
	case *ast.KeyValueExpr:
		return re.exprContainsIdent(e.Value, name)
	}
	return false
}

// canMoveArg checks if a call argument identifier can be moved instead of cloned.
// Returns true when the variable is being reassigned from this call's return value
// and is the only reference to itself across all args of the outermost call.
func (re *RustEmitter) canMoveArg(varName string) bool {
	if !re.OptimizeMoves {
		return false
	}
	// Cannot move captured variables inside closures (FnMut)
	if re.funcLitDepth > 0 {
		return false
	}
	if re.currentAssignLhsNames == nil || !re.currentAssignLhsNames[varName] {
		return false
	}
	// When temp extraction is active, use modified counts that exclude extracted fields
	if re.moveOptActive && re.moveOptModifiedCounts != nil {
		if re.moveOptModifiedCounts[varName] <= 1 {
			re.MoveOptCount++
			return true
		}
		return false
	}
	// Check the outermost call's arg ident counts (bottom of stack)
	if len(re.currentCallArgIdentsStack) > 0 {
		outermostCounts := re.currentCallArgIdentsStack[0]
		if outermostCounts[varName] > 1 {
			return false
		}
	}
	re.MoveOptCount++
	return true
}

// isCopyType checks if a Go type maps to a Rust Copy type (primitives)
func isCopyType(t types.Type) bool {
	if t == nil {
		return false
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.Bool, types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64:
			return true
		}
	}
	return false
}

// analyzeMoveOptExtraction scans an assignment's RHS call to find field accesses
// on a struct being moved, and creates temp variable bindings to extract them.
// Pattern: c = Func(c, c.Field) → let _v0 = c.Field; c = Func(c, _v0);
func (re *RustEmitter) analyzeMoveOptExtraction(node *ast.AssignStmt, indent int) {
	re.moveOptTempBindings = nil
	re.moveOptArgReplacements = nil
	re.moveOptModifiedCounts = nil
	re.moveOptActive = false

	if !re.OptimizeMoves {
		return
	}
	if re.funcLitDepth > 0 {
		return
	}
	if len(node.Rhs) != 1 {
		return
	}
	callExpr, ok := node.Rhs[0].(*ast.CallExpr)
	if !ok {
		return
	}
	if len(callExpr.Args) < 2 {
		return
	}

	// Find which arg is a struct ident matching an LHS name
	structArgName := ""
	structArgIdx := -1
	for i, arg := range callExpr.Args {
		ident, ok := arg.(*ast.Ident)
		if !ok {
			continue
		}
		if re.currentAssignLhsNames == nil || !re.currentAssignLhsNames[ident.Name] {
			continue
		}
		// Check if it's a struct type
		tv := re.pkg.TypesInfo.Types[arg]
		if tv.Type == nil {
			continue
		}
		if named, ok := tv.Type.(*types.Named); ok {
			if _, isStruct := named.Underlying().(*types.Struct); isStruct {
				structArgName = ident.Name
				structArgIdx = i
				break
			}
		}
	}
	if structArgIdx < 0 {
		return
	}

	// Check other args for field accesses on the same struct with Copy-type results
	tempIdx := 0
	replacements := make(map[int]string)
	var bindings []string
	modifiedCounts := re.collectCallArgIdentCounts(callExpr.Args)

	for i, arg := range callExpr.Args {
		if i == structArgIdx {
			continue
		}
		// Check if this arg (or sub-expressions) references the struct
		if !re.exprContainsIdent(arg, structArgName) {
			continue
		}
		// Get the result type of this arg - must be Copy
		tv := re.pkg.TypesInfo.Types[arg]
		if tv.Type == nil || !isCopyType(tv.Type) {
			continue
		}
		// This arg references the struct and produces a Copy value - extract it
		tempName := fmt.Sprintf("__mv%d", tempIdx)
		tempIdx++
		rustType := re.mapGoTypeToRust(tv.Type.Underlying().(*types.Basic).Name())
		binding := fmt.Sprintf("let %s: %s = ", tempName, rustType)
		// We'll emit the binding text; the actual expression will be emitted by
		// re-visiting the arg AST. Instead, we'll emit the expression manually.
		// For SelectorExpr like c.A, emit "c.A"
		exprStr := re.exprToString(arg)
		if exprStr == "" {
			continue
		}
		binding += exprStr + ";\n"
		bindings = append(bindings, binding)
		replacements[i] = tempName
		// Remove this arg's struct references from the modified counts
		re.subtractIdentsInExpr(arg, modifiedCounts)
	}

	if len(replacements) > 0 {
		re.moveOptTempBindings = bindings
		re.moveOptArgReplacements = replacements
		re.moveOptModifiedCounts = modifiedCounts
		re.moveOptActive = true
	}
}

// exprToString converts a simple expression to its Rust string representation
func (re *RustEmitter) exprToString(expr ast.Expr) string {
	return re.exprToStringImpl(expr, "")
}

// exprToStringImpl converts an expression to Rust string with optional cast hint.
// castHint is the Rust type name to cast untyped constants to (e.g. "u8" inside uint8(...)).
func (re *RustEmitter) exprToStringImpl(expr ast.Expr, castHint string) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		name := escapeRustKeyword(e.Name)
		// If we have a cast hint and this ident is a constant, cast it
		if castHint != "" && re.pkg != nil && re.pkg.TypesInfo != nil {
			if obj := re.pkg.TypesInfo.ObjectOf(e); obj != nil {
				if _, isConst := obj.(*types.Const); isConst {
					// Check if the constant's Rust type differs from the cast hint
					if named, ok := obj.Type().(*types.Basic); ok {
						constRustType := re.mapGoTypeToRust(named.Name())
						if constRustType != castHint {
							return "(" + name + " as " + castHint + ")"
						}
					}
				}
			}
		}
		return name
	case *ast.SelectorExpr:
		base := re.exprToStringImpl(e.X, "")
		if base == "" {
			return ""
		}
		return base + "." + e.Sel.Name
	case *ast.IndexExpr:
		base := re.exprToStringImpl(e.X, "")
		idx := re.exprToStringImpl(e.Index, "")
		if base == "" || idx == "" {
			return ""
		}
		return base + "[" + idx + " as usize]"
	case *ast.BasicLit:
		return e.Value
	case *ast.BinaryExpr:
		left := re.exprToStringImpl(e.X, castHint)
		right := re.exprToStringImpl(e.Y, castHint)
		if left == "" || right == "" {
			return ""
		}
		return "(" + left + " " + e.Op.String() + " " + right + ")"
	case *ast.ParenExpr:
		inner := re.exprToStringImpl(e.X, castHint)
		if inner == "" {
			return ""
		}
		return "(" + inner + ")"
	case *ast.CallExpr:
		// Handle type conversions: int(x), uint8(x), etc.
		if len(e.Args) == 1 {
			if funIdent, ok := e.Fun.(*ast.Ident); ok {
				if rustType, ok := rustTypesMap[funIdent.Name]; ok {
					// Pass the target type as cast hint so inner constants get cast
					inner := re.exprToStringImpl(e.Args[0], rustType)
					if inner != "" {
						return "(" + inner + " as " + rustType + ")"
					}
				}
			}
		}
		// Don't handle actual function calls - bail out
		return ""
	}
	return ""
}

// subtractIdentsInExpr removes identifier occurrences found in expr from counts
func (re *RustEmitter) subtractIdentsInExpr(expr ast.Expr, counts map[string]int) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Ident:
		if re.pkg != nil && re.pkg.TypesInfo != nil {
			if obj := re.pkg.TypesInfo.ObjectOf(e); obj != nil {
				if _, isTypeName := obj.(*types.TypeName); isTypeName {
					return
				}
				if _, isPkgName := obj.(*types.PkgName); isPkgName {
					return
				}
			}
		}
		counts[e.Name]--
	case *ast.SelectorExpr:
		re.subtractIdentsInExpr(e.X, counts)
	case *ast.IndexExpr:
		re.subtractIdentsInExpr(e.X, counts)
		re.subtractIdentsInExpr(e.Index, counts)
	case *ast.BinaryExpr:
		re.subtractIdentsInExpr(e.X, counts)
		re.subtractIdentsInExpr(e.Y, counts)
	case *ast.UnaryExpr:
		re.subtractIdentsInExpr(e.X, counts)
	case *ast.ParenExpr:
		re.subtractIdentsInExpr(e.X, counts)
	case *ast.CallExpr:
		for _, arg := range e.Args {
			re.subtractIdentsInExpr(arg, counts)
		}
		re.subtractIdentsInExpr(e.Fun, counts)
	}
}

// collectIdentifiersInStmt collects all identifiers used as function call arguments in a statement
func (re *RustEmitter) collectIdentifiersInStmt(stmt ast.Stmt) map[string]bool {
	result := make(map[string]bool)
	if stmt == nil {
		return result
	}
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		re.collectIdentifiersInCallArgs(s.X, result)
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			re.collectIdentifiersInCallArgs(rhs, result)
		}
	case *ast.DeclStmt:
		if genDecl, ok := s.Decl.(*ast.GenDecl); ok {
			for _, spec := range genDecl.Specs {
				if valueSpec, ok := spec.(*ast.ValueSpec); ok {
					for _, value := range valueSpec.Values {
						re.collectIdentifiersInCallArgs(value, result)
					}
				}
			}
		}
	case *ast.ReturnStmt:
		for _, r := range s.Results {
			re.collectIdentifiersInCallArgs(r, result)
		}
	case *ast.IfStmt:
		re.collectIdentifiersInCallArgs(s.Cond, result)
		// Recursively process body
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				for k, v := range re.collectIdentifiersInStmt(bodyStmt) {
					result[k] = v
				}
			}
		}
		if s.Else != nil {
			for k, v := range re.collectIdentifiersInStmt(s.Else) {
				result[k] = v
			}
		}
	case *ast.ForStmt:
		re.collectIdentifiersInCallArgs(s.Cond, result)
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				for k, v := range re.collectIdentifiersInStmt(bodyStmt) {
					result[k] = v
				}
			}
		}
	case *ast.RangeStmt:
		re.collectIdentifiersInCallArgs(s.X, result)
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				for k, v := range re.collectIdentifiersInStmt(bodyStmt) {
					result[k] = v
				}
			}
		}
	case *ast.BlockStmt:
		for _, bodyStmt := range s.List {
			for k, v := range re.collectIdentifiersInStmt(bodyStmt) {
				result[k] = v
			}
		}
	case *ast.SwitchStmt:
		re.collectIdentifiersInCallArgs(s.Tag, result)
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				for k, v := range re.collectIdentifiersInStmt(bodyStmt) {
					result[k] = v
				}
			}
		}
	}
	return result
}

// collectMutatedVarsInExpr recursively collects variables that are assigned to in an expression
func (re *RustEmitter) collectMutatedVarsInExpr(expr ast.Expr, result map[string]bool) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Ident:
		result[e.Name] = true
	case *ast.IndexExpr:
		// For index expressions like arr[i], check the base
		re.collectMutatedVarsInExpr(e.X, result)
	case *ast.SelectorExpr:
		// For selector expressions like obj.field, check the base
		re.collectMutatedVarsInExpr(e.X, result)
	}
}

// collectMutatedVarsInStmt recursively collects variables that are assigned to (mutated) in a statement
func (re *RustEmitter) collectMutatedVarsInStmt(stmt ast.Stmt, result map[string]bool) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		// Only count assignments (=), not declarations (:=) which create new variables
		if s.Tok == token.ASSIGN || s.Tok == token.ADD_ASSIGN || s.Tok == token.SUB_ASSIGN ||
			s.Tok == token.MUL_ASSIGN || s.Tok == token.QUO_ASSIGN || s.Tok == token.REM_ASSIGN {
			for _, lhs := range s.Lhs {
				re.collectMutatedVarsInExpr(lhs, result)
			}
		}
	case *ast.IncDecStmt:
		re.collectMutatedVarsInExpr(s.X, result)
	case *ast.IfStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				re.collectMutatedVarsInStmt(bodyStmt, result)
			}
		}
		if s.Else != nil {
			re.collectMutatedVarsInStmt(s.Else, result)
		}
	case *ast.ForStmt:
		if s.Post != nil {
			re.collectMutatedVarsInStmt(s.Post, result)
		}
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				re.collectMutatedVarsInStmt(bodyStmt, result)
			}
		}
	case *ast.RangeStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				re.collectMutatedVarsInStmt(bodyStmt, result)
			}
		}
	case *ast.BlockStmt:
		for _, bodyStmt := range s.List {
			re.collectMutatedVarsInStmt(bodyStmt, result)
		}
	case *ast.SwitchStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				re.collectMutatedVarsInStmt(bodyStmt, result)
			}
		}
	case *ast.CaseClause:
		for _, bodyStmt := range s.Body {
			re.collectMutatedVarsInStmt(bodyStmt, result)
		}
	}
}

// analyzeParamMutations analyzes a function body to find which parameters are mutated (assigned to)
func (re *RustEmitter) analyzeParamMutations(params []*ast.Field, body *ast.BlockStmt) {
	re.mutatedParams = make(map[string]bool)
	if body == nil || params == nil {
		return
	}

	// Collect all mutated variables in the function body
	mutatedVars := make(map[string]bool)
	for _, stmt := range body.List {
		re.collectMutatedVarsInStmt(stmt, mutatedVars)
	}

	// Check which parameters are in the mutated set
	for _, field := range params {
		for _, name := range field.Names {
			if mutatedVars[name.Name] {
				re.mutatedParams[name.Name] = true
			}
		}
	}
}

// analyzeVariableLiveness performs liveness analysis for a function body
// It builds stmtVarUsages which maps each statement index to the set of variables used in that statement
func (re *RustEmitter) analyzeVariableLiveness(body *ast.BlockStmt) {
	if body == nil {
		re.stmtVarUsages = nil
		re.currentFuncBody = nil
		return
	}
	re.currentFuncBody = body
	re.stmtVarUsages = make([]map[string]bool, len(body.List))
	for i, stmt := range body.List {
		re.stmtVarUsages[i] = re.collectIdentifiersInStmt(stmt)
	}
	re.currentStmtIndex = 0
}

// isVariableUsedInLaterStatements checks if a variable will be used in any statement after the current one
func (re *RustEmitter) isVariableUsedInLaterStatements(varName string) bool {
	if re.stmtVarUsages == nil || re.currentFuncBody == nil {
		return false
	}
	// Check all statements after the current one
	for i := re.currentStmtIndex + 1; i < len(re.stmtVarUsages); i++ {
		if re.stmtVarUsages[i][varName] {
			return true
		}
	}
	return false
}

func (re *RustEmitter) SetFile(file *os.File) {
	re.file = file
}

func (re *RustEmitter) GetFile() *os.File {
	return re.file
}

func (re *RustEmitter) PreVisitProgram(indent int) {
	re.aliases = make(map[string]Alias)
	outputFile := re.Output

	// For Cargo projects, write to src/main.rs instead
	if re.LinkRuntime != "" {
		srcDir := filepath.Join(re.OutputDir, "src")
		if err := os.MkdirAll(srcDir, 0755); err != nil {
			fmt.Println("Error creating src directory:", err)
			return
		}
		outputFile = filepath.Join(srcDir, "main.rs")
	}

	var err error
	re.file, err = os.Create(outputFile)
	re.SetFile(re.file)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	builtin := `use std::fmt;
use std::any::Any;
use std::rc::Rc;

// Type aliases (Go-style)
type Int8 = i8;
type Int16 = i16;
type Int32 = i32;
type Int64 = i64;
type Uint8 = u8;
type Uint16 = u16;
type Uint32 = u32;
type Uint64 = u64;

// println equivalents - multiple versions for different arg counts
pub fn println<T: fmt::Display>(val: T) {
    std::println!("{}", val);
}

pub fn println0() {
    std::println!();
}

// printf - multiple versions for different arg counts
pub fn printf<T: fmt::Display>(val: T) {
    print!("{}", val);
}

pub fn printf2<T: fmt::Display>(fmt_str: String, val: T) {
    // Convert C-style format to Rust format
    let rust_fmt = fmt_str.replace("%d", "{}").replace("%s", "{}").replace("%v", "{}");
    let result = rust_fmt.replace("{}", &format!("{}", val));
    print!("{}", result);
}

pub fn printf3<T1: fmt::Display, T2: fmt::Display>(fmt_str: String, v1: T1, v2: T2) {
    let rust_fmt = fmt_str.replace("%d", "{}").replace("%s", "{}").replace("%v", "{}");
    let result = rust_fmt.replacen("{}", &format!("{}", v1), 1).replacen("{}", &format!("{}", v2), 1);
    print!("{}", result);
}

pub fn printf4<T1: fmt::Display, T2: fmt::Display, T3: fmt::Display>(fmt_str: String, v1: T1, v2: T2, v3: T3) {
    let rust_fmt = fmt_str.replace("%d", "{}").replace("%s", "{}").replace("%v", "{}");
    let result = rust_fmt.replacen("{}", &format!("{}", v1), 1).replacen("{}", &format!("{}", v2), 1).replacen("{}", &format!("{}", v3), 1);
    print!("{}", result);
}

pub fn printf5<T1: fmt::Display, T2: fmt::Display, T3: fmt::Display, T4: fmt::Display>(fmt_str: String, v1: T1, v2: T2, v3: T3, v4: T4) {
    let rust_fmt = fmt_str.replace("%d", "{}").replace("%s", "{}").replace("%v", "{}");
    let result = rust_fmt.replacen("{}", &format!("{}", v1), 1).replacen("{}", &format!("{}", v2), 1).replacen("{}", &format!("{}", v3), 1).replacen("{}", &format!("{}", v4), 1);
    print!("{}", result);
}

// Print byte as character (for %c format)
pub fn printc(b: i8) {
    print!("{}", b as u8 as char);
}

// Convert byte to character string (for Sprintf %c format)
pub fn byte_to_char(b: i8) -> String {
    (b as u8 as char).to_string()
}

// Go-style append - takes ownership to avoid cloning
pub fn append<T>(mut vec: Vec<T>, value: T) -> Vec<T> {
    vec.push(value);
    vec
}

pub fn append_many<T: Clone>(mut vec: Vec<T>, values: &[T]) -> Vec<T> {
    vec.extend_from_slice(values);
    vec
}

// Simple string_format using format!
pub fn string_format(fmt_str: &str, args: &[&dyn fmt::Display]) -> String {
    let mut result = String::new();
    let mut split = fmt_str.split("{}");
    for (i, segment) in split.enumerate() {
        result.push_str(segment);
        if i < args.len() {
            result.push_str(&format!("{}", args[i]));
        }
    }
    result
}

// string_format for 2 args (format string + 1 value)
pub fn string_format2<T: fmt::Display>(fmt_str: &str, val: T) -> String {
    let rust_fmt = fmt_str.replace("%d", "{}").replace("%s", "{}").replace("%v", "{}");
    rust_fmt.replace("{}", &format!("{}", val))
}

pub fn len<T>(slice: &[T]) -> i32 {
    slice.len() as i32
}
`
	str := re.emitAsString(builtin, indent)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)

	// Include runtime module if link-runtime is enabled
	if re.LinkRuntime != "" {
		runtimeInclude := `
// Graphics runtime
mod graphics;
use graphics::*;
`
		re.gir.emitToFileBuffer(runtimeInclude, EmptyVisitMethod)
	}

	re.insideForPostCond = false
}

func (re *RustEmitter) PostVisitProgram(indent int) {
	emitTokensToFile(re.file, re.gir.tokenSlice)
	re.file.Close()

	// Generate Cargo project files if link-runtime is enabled
	if re.LinkRuntime != "" {
		if err := re.GenerateCargoToml(); err != nil {
			log.Printf("Warning: %v", err)
		}
		if err := re.GenerateGraphicsMod(); err != nil {
			log.Printf("Warning: %v", err)
		}
		if err := re.GenerateBuildRs(); err != nil {
			log.Printf("Warning: %v", err)
		}
	}

	if re.OptimizeMoves && re.MoveOptCount > 0 {
		fmt.Printf("  Rust: %d clone(s) removed by move optimization\n", re.MoveOptCount)
	}
}

func (re *RustEmitter) PreVisitFuncDeclSignatures(indent int) {
	re.forwardDecls = true
}

func (re *RustEmitter) PostVisitFuncDeclSignatures(indent int) {
	re.forwardDecls = false
}

func (re *RustEmitter) PreVisitFuncDeclName(node *ast.Ident, indent int) {
	if re.forwardDecls {
		return
	}
	var str string
	str = re.emitAsString(fmt.Sprintf("pub fn %s", node.Name), 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitFuncDeclBody(node *ast.BlockStmt, indent int) {
	// Perform liveness analysis before emitting the function body
	re.analyzeVariableLiveness(node)
}

func (re *RustEmitter) PreVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	// Update current statement index if this statement is in the function body
	if re.currentFuncBody != nil {
		for i, stmt := range re.currentFuncBody.List {
			if stmt == node {
				re.currentStmtIndex = i
				break
			}
		}
	}
}

func (re *RustEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	if re.forwardDecls {
		return
	}
	// Track block depth when inside a for loop body that needs increment
	if re.inForLoopBody {
		re.forLoopBodyDepth++
	} else if re.pendingLoopIncrement {
		// This is the for loop body block
		re.inForLoopBody = true
		re.forLoopBodyDepth = 1
	}
	re.emitToken("{", LeftBrace, 1)
	str := re.emitAsString("\n", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	if re.forwardDecls {
		return
	}
	// Track block depth and add loop increment only when exiting the loop body block itself
	if re.inForLoopBody {
		re.forLoopBodyDepth--
		if re.forLoopBodyDepth == 0 && re.pendingLoopIncrement && re.loopIncrementVar != "" {
			// Emit the increment statement at end of loop body
			incStr := re.emitAsString(re.loopIncrementVar+" "+re.loopIncrementOp+" "+re.loopIncrementVal+";\n", indent)
			re.gir.emitToFileBuffer(incStr, EmptyVisitMethod)
			// Clear the flags
			re.inForLoopBody = false
			re.pendingLoopIncrement = false
		}
	}
	re.emitToken("}", RightBrace, 1)
	// Note: removed isArray = false as it interfered with composite literal stack management
}

func (re *RustEmitter) PreVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	if re.forwardDecls {
		return
	}
	// Analyze which parameters are mutated in the function body
	if node.Type != nil && node.Type.Params != nil {
		re.analyzeParamMutations(node.Type.Params.List, node.Body)
	} else {
		re.mutatedParams = make(map[string]bool)
	}
	re.shouldGenerate = true
	re.inFuncParam = true // Track that we're in function parameters
	re.emitToken("(", LeftParen, 0)
}

func (re *RustEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	if re.forwardDecls {
		return
	}
	re.shouldGenerate = false
	re.inFuncParam = false // Done with function parameters
	re.emitToken(")", RightParen, 0)

	p1 := SearchPointerIndexReverse("@PreVisitFuncDeclSignatureTypeResults", re.gir.pointerAndIndexVec)
	p2 := SearchPointerIndexReverse("@PostVisitFuncDeclSignatureTypeResults", re.gir.pointerAndIndexVec)
	if p1 != nil && p2 != nil {
		results, err := ExtractTokensBetween(p1.Index, p2.Index, re.gir.tokenSlice)
		if err != nil {
			fmt.Println("Error extracting results:", err)
			return
		}

		re.gir.tokenSlice, err = RewriteTokensBetween(re.gir.tokenSlice, p1.Index, p2.Index, []string{""})
		if err != nil {
			fmt.Println("Error rewriting file buffer:", err)
			return
		}
		if strings.TrimSpace(strings.Join(tokensToStrings(results), "")) != "" {
			re.gir.tokenSlice = append(re.gir.tokenSlice, CreateToken(RustKeyword, " -> "))
			re.gir.tokenSlice = append(re.gir.tokenSlice, CreateToken(Identifier, strings.Join(tokensToStrings(results), "")))
		}
	}
}

func (re *RustEmitter) PreVisitIdent(e *ast.Ident, indent int) {
	if re.forwardDecls {
		return
	}
	if !re.shouldGenerate {
		return
	}
	// Skip emission during key-value range key/value visits
	if re.suppressRangeEmit {
		return
	}
	// Capture to buffer during range collection expression visit
	if re.captureRangeExpr {
		re.rangeCollectionExpr += e.Name
		return
	}
	re.gir.emitToFileBuffer("", "@PreVisitIdent")

	var str string
	name := e.Name
	name = re.lowerToBuiltins(name)
	if name == "nil" {
		// In Go, nil for slices means empty slice - use Vec::new() in Rust
		// For pointers/interfaces, None would be correct, but Vec::new() is safer
		// for the common case of slice assignment
		str = re.emitAsString("Vec::new()", indent)
	} else {
		if n, ok := rustTypesMap[name]; ok {
			str = re.emitAsString(n, indent)
		} else {
			// Escape Rust keywords
			name = escapeRustKeyword(name)
			str = re.emitAsString(name, indent)
		}
	}

	re.emitToken(str, Identifier, 0)

}
func (re *RustEmitter) PreVisitCallExprArgs(node []ast.Expr, indent int) {
	if re.forwardDecls {
		return
	}
	// Push the args start position to the stack for nested call handling
	re.callExprArgsMarkerStack = append(re.callExprArgsMarkerStack, len(re.gir.tokenSlice))
	re.gir.emitToFileBuffer("", "@PreVisitCallExprArgs")
	re.emitToken("(", LeftParen, 0)
	// Push call arg identifier counts for move optimization
	re.currentCallArgIdentsStack = append(re.currentCallArgIdentsStack, re.collectCallArgIdentCounts(node))
	// Use stack indices for function name extraction (top of stacks = current call)
	re.currentCallIsAppend = false // Reset for each call
	if len(re.callExprFunMarkerStack) > 0 && len(re.callExprFunEndMarkerStack) > 0 {
		p1Index := re.callExprFunMarkerStack[len(re.callExprFunMarkerStack)-1]
		p2Index := re.callExprFunEndMarkerStack[len(re.callExprFunEndMarkerStack)-1]
		// Extract the substring between the positions of the pointers
		funName, err := ExtractTokensBetween(p1Index, p2Index, re.gir.tokenSlice)
		if err != nil {
			fmt.Println("Error extracting function name:", err)
			return
		}
		funNameStr := strings.Join(tokensToStrings(funName), "")
		// Track if this is an append call (takes ownership, not reference)
		if strings.Contains(funNameStr, "append") {
			re.currentCallIsAppend = true
		}
		// Skip adding & for type conversions
		if isConversion, _ := re.isTypeConversion(funNameStr); !isConversion {
			if strings.Contains(funNameStr, "len") {
				// add & before the first argument for len (but not append - it takes ownership)
				str := re.emitAsString("&", 0)
				re.gir.emitToFileBuffer(str, EmptyVisitMethod)
			}
		}
	}
}

// isTypeConversion checks if a function name represents a type conversion
func (re *RustEmitter) isTypeConversion(funName string) (bool, string) {
	// Map Go type names and Rust type names to Rust cast targets
	typeConversions := map[string]string{
		// Go type names
		"int8":    "i8",
		"int16":   "i16",
		"int32":   "i32",
		"int64":   "i64",
		"int":     "i32",
		"uint8":   "u8",
		"uint16":  "u16",
		"uint32":  "u32",
		"uint64":  "u64",
		"uint":    "u32",
		"float32": "f32",
		"float64": "f64",
		"byte":    "u8",
		"rune":    "i32",
		// Rust type names (in case they're already converted)
		"i8":  "i8",
		"i16": "i16",
		"i32": "i32",
		"i64": "i64",
		"u8":  "u8",
		"u16": "u16",
		"u32": "u32",
		"u64": "u64",
		"f32": "f32",
		"f64": "f64",
	}
	if rustType, ok := typeConversions[funName]; ok {
		return true, rustType
	}
	return false, ""
}

func (re *RustEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	if re.forwardDecls {
		return
	}

	// Pop from stacks at the end (defer to ensure it happens even on early returns)
	defer func() {
		if len(re.callExprFunMarkerStack) > 0 {
			re.callExprFunMarkerStack = re.callExprFunMarkerStack[:len(re.callExprFunMarkerStack)-1]
		}
		if len(re.callExprFunEndMarkerStack) > 0 {
			re.callExprFunEndMarkerStack = re.callExprFunEndMarkerStack[:len(re.callExprFunEndMarkerStack)-1]
		}
		if len(re.callExprArgsMarkerStack) > 0 {
			re.callExprArgsMarkerStack = re.callExprArgsMarkerStack[:len(re.callExprArgsMarkerStack)-1]
		}
		if len(re.currentCallArgIdentsStack) > 0 {
			re.currentCallArgIdentsStack = re.currentCallArgIdentsStack[:len(re.currentCallArgIdentsStack)-1]
		}
	}()

	// Use stack indices for the current call (top of stacks)
	if len(re.callExprFunMarkerStack) == 0 || len(re.callExprFunEndMarkerStack) == 0 || len(re.callExprArgsMarkerStack) == 0 {
		re.emitToken(")", RightParen, 0)
		return
	}

	p1Index := re.callExprFunMarkerStack[len(re.callExprFunMarkerStack)-1]
	p2Index := re.callExprFunEndMarkerStack[len(re.callExprFunEndMarkerStack)-1]
	pArgsIndex := re.callExprArgsMarkerStack[len(re.callExprArgsMarkerStack)-1]

	funName, err := ExtractTokensBetween(p1Index, p2Index, re.gir.tokenSlice)
	if err == nil {
		funNameStr := strings.Join(tokensToStrings(funName), "")

		// Handle local closure inlining: addToken() -> { body }
		funNameTrimmedForClosure := strings.TrimSpace(funNameStr)
		if bodyTokens, ok := re.localClosureBodyTokens[funNameTrimmedForClosure]; ok && len(node) == 0 {
			// Replace the entire call with the inlined body wrapped in a block
			newTokens := []string{"{"}
			for _, tok := range bodyTokens {
				newTokens = append(newTokens, tok.Content)
			}
			newTokens = append(newTokens, "}")
			re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, p1Index, len(re.gir.tokenSlice), newTokens)
			return // Skip emitting closing paren since we replaced everything
		}

		// Handle type conversions: i8(x) -> (x as i8)
		funNameTrimmed := strings.TrimSpace(funNameStr)
		if isConv, rustType := re.isTypeConversion(funNameTrimmed); isConv && len(node) == 1 {
			// Extract the argument tokens (between @PreVisitCallExprArgs and current position)
			argTokens, err := ExtractTokensBetween(pArgsIndex, len(re.gir.tokenSlice), re.gir.tokenSlice)
			if err == nil && len(argTokens) > 0 {
				// Remove the opening paren from call args (added by PreVisitCallExprArgs)
				// but keep any inner parens (e.g., from binary expressions)
				argStr := strings.TrimSpace(strings.Join(tokensToStrings(argTokens), ""))
				if len(argStr) > 0 && argStr[0] == '(' {
					argStr = argStr[1:]
				}
				argStr = strings.TrimSpace(argStr)
				// Generate: (arg as type)
				newTokens := []string{"(", argStr, " as ", rustType, ")"}
				re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, p1Index, len(re.gir.tokenSlice), newTokens)
				return // Skip emitting closing paren since we replaced everything
			}
		}

		// Handle println with 0 args
		if funNameStr == "println" && len(node) == 0 {
			re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, p1Index, p2Index, []string{"println0"})
		}
		// Handle printf with different arg counts (count includes format string)
		if funNameStr == "printf" {
			switch len(node) {
			case 2:
				// Special case: printf("%c", byte) -> printc(byte)
				if basicLit, ok := node[0].(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
					fmtStr := strings.Trim(basicLit.Value, "\"")
					if fmtStr == "%c" {
						// Rewrite to printc and remove the format string argument
						// Find the argument tokens
						argTokens, err := ExtractTokensBetween(pArgsIndex, len(re.gir.tokenSlice), re.gir.tokenSlice)
						if err == nil && len(argTokens) > 0 {
							// Find the comma that separates the format string from the actual argument
							argStr := strings.Join(tokensToStrings(argTokens), "")
							// Skip the opening paren
							if len(argStr) > 0 && argStr[0] == '(' {
								argStr = argStr[1:]
							}
							// Find comma and extract just the second argument
							commaIdx := strings.Index(argStr, ",")
							if commaIdx >= 0 {
								secondArg := strings.TrimSpace(argStr[commaIdx+1:])
								// Rewrite: printf("%c", b) -> printc(b)
								newTokens := []string{"printc", "(", secondArg}
								re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, p1Index, len(re.gir.tokenSlice), newTokens)
								// Don't return here - let the closing paren be added normally
								break
							}
						}
					}
				}
				re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, p1Index, p2Index, []string{"printf2"})
			case 3:
				re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, p1Index, p2Index, []string{"printf3"})
			case 4:
				re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, p1Index, p2Index, []string{"printf4"})
			case 5:
				re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, p1Index, p2Index, []string{"printf5"})
			}
		}
		// Handle string_format with different arg counts
		if funNameStr == "string_format" {
			switch len(node) {
			case 2:
				// Special case: Sprintf("%c", byte) -> byte_to_char(byte)
				if basicLit, ok := node[0].(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
					fmtStr := strings.Trim(basicLit.Value, "\"")
					if fmtStr == "%c" {
						// Rewrite to byte_to_char and remove the format string argument
						argTokens, err := ExtractTokensBetween(pArgsIndex, len(re.gir.tokenSlice), re.gir.tokenSlice)
						if err == nil && len(argTokens) > 0 {
							argStr := strings.Join(tokensToStrings(argTokens), "")
							if len(argStr) > 0 && argStr[0] == '(' {
								argStr = argStr[1:]
							}
							commaIdx := strings.Index(argStr, ",")
							if commaIdx >= 0 {
								secondArg := strings.TrimSpace(argStr[commaIdx+1:])
								newTokens := []string{"byte_to_char", "(", secondArg}
								re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, p1Index, len(re.gir.tokenSlice), newTokens)
								break
							}
						}
					}
				}
				re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, p1Index, p2Index, []string{"string_format2"})
			}
		}

		// Handle len() on String - convert to method syntax: len(str) -> str.len() as i32
		if funNameStr == "len" && len(node) == 1 {
			argType := re.pkg.TypesInfo.Types[node[0]]
			if argType.Type != nil && argType.Type.String() == "string" {
				// Extract the argument tokens
				argTokens, err := ExtractTokensBetween(pArgsIndex, len(re.gir.tokenSlice), re.gir.tokenSlice)
				if err == nil && len(argTokens) > 0 {
					argStr := strings.TrimSpace(strings.Join(tokensToStrings(argTokens), ""))
					// Remove ( from start and & if present
					if len(argStr) > 0 && argStr[0] == '(' {
						argStr = argStr[1:]
					}
					argStr = strings.TrimPrefix(argStr, "&")
					argStr = strings.TrimSpace(argStr)
					// Generate: str.len() as i32
					newTokens := []string{argStr, ".len() as i32"}
					re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, p1Index, len(re.gir.tokenSlice), newTokens)
					return // Skip emitting closing paren
				}
			}
		}
	}
	re.emitToken(")", RightParen, 0)
}

func (re *RustEmitter) PreVisitBasicLit(e *ast.BasicLit, indent int) {
	if re.forwardDecls {
		return
	}
	var str string
	if e.Kind == token.STRING {
		// Use a local copy to avoid mutating the AST (which affects other emitters)
		value := e.Value
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			// Remove only the outer quotes, keep escaped content intact
			value = value[1 : len(value)-1]
			str = (re.emitAsString(fmt.Sprintf("\"%s\"", value), 0))
		} else if len(value) >= 2 && value[0] == '`' && value[len(value)-1] == '`' {
			// Raw string literal - use Rust raw string
			value = value[1 : len(value)-1]
			str = (re.emitAsString(fmt.Sprintf("r#\"%s\"#", value), 0))
		} else {
			str = (re.emitAsString(fmt.Sprintf("\"%s\"", value), 0))
		}
		re.emitToken(str, StringLiteral, 0)
	} else if e.Kind == token.CHAR {
		// Character literals in Go are runes - convert to numeric i8 for Rust
		// This allows use in match patterns (which don't allow `as` casts)
		charVal := e.Value
		if len(charVal) >= 3 && charVal[0] == '\'' && charVal[len(charVal)-1] == '\'' {
			inner := charVal[1 : len(charVal)-1]
			var numVal int
			// Handle escape sequences
			if len(inner) >= 2 && inner[0] == '\\' {
				switch inner[1] {
				case 'n':
					numVal = 10 // newline
				case 't':
					numVal = 9 // tab
				case 'r':
					numVal = 13 // carriage return
				case '\\':
					numVal = 92 // backslash
				case '\'':
					numVal = 39 // single quote
				case '0':
					numVal = 0 // null
				default:
					numVal = int(inner[1])
				}
			} else if len(inner) == 1 {
				// Single character - use ASCII value
				numVal = int(inner[0])
			} else {
				// Fallback - just emit as is
				str = re.emitAsString(charVal, 0)
				re.emitToken(str, CharLiteral, 0)
				return
			}
			// Don't add i8 suffix - let Rust infer the type from context
			// This allows character literals to work in match expressions cast to i32
			str = re.emitAsString(fmt.Sprintf("%d", numVal), 0)
		} else {
			str = re.emitAsString(charVal, 0)
		}
		re.emitToken(str, CharLiteral, 0)
	} else if e.Kind == token.INT {
		// Check if the integer literal should be emitted as a float
		// This happens when:
		// 1. The expected type from context is float64 or float32
		// 2. We're in a binary expression where one operand is float
		tv := re.pkg.TypesInfo.Types[e]
		value := e.Value
		needsFloatSuffix := false
		if tv.Type != nil {
			typeStr := tv.Type.String()
			if typeStr == "float64" || typeStr == "float32" || typeStr == "untyped float" {
				needsFloatSuffix = true
			}
		}
		// Also check if we're in a float binary expression
		if re.inFloatBinaryExpr {
			needsFloatSuffix = true
		}
		if needsFloatSuffix && !strings.Contains(value, ".") {
			value = value + ".0"
		}
		str = (re.emitAsString(value, 0))
		re.emitToken(str, NumberLiteral, 0)
	} else {
		str = (re.emitAsString(e.Value, 0))
		re.emitToken(str, NumberLiteral, 0)
	}
}

func (re *RustEmitter) PostVisitBasicLit(e *ast.BasicLit, indent int) {
	if re.forwardDecls {
		return
	}
	// For string literals, add .to_string() to convert &str to String
	// But skip if we're in a += context (Rust's += for String expects &str)
	if e.Kind == token.STRING {
		if re.inAssignRhs && re.assignmentToken == "+=" {
			// Don't add .to_string() for += operations
			return
		}
		re.gir.emitToFileBuffer(".to_string()", EmptyVisitMethod)
	}
}

func (re *RustEmitter) PreVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	if re.forwardDecls {
		return
	}
	// For second and subsequent names, start a new let statement
	if index > 0 {
		re.emitToken(";", Semicolon, 0)
		re.emitToken("\n", NewLine, 0)
		re.emitToken("let", RustKeyword, indent)
		re.emitToken(" ", WhiteSpace, 0)
		re.emitToken("mut", RustKeyword, 0)
		re.emitToken(" ", WhiteSpace, 0)
	}
	re.gir.emitToFileBuffer("", "@PreVisitDeclStmtValueSpecType")
}

func (re *RustEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	if re.forwardDecls {
		return
	}
	pointerAndPosition := SearchPointerIndexReverse("@PreVisitDeclStmtValueSpecType", re.gir.pointerAndIndexVec)
	if pointerAndPosition != nil {
		typeInfo := re.pkg.TypesInfo.Types[node.Type]
		// Only do alias replacement if the type is NOT already a named type (alias)
		// If it's a named type like types.ExprKind, don't replace it with another alias
		if typeInfo.Type != nil {
			if _, isNamed := typeInfo.Type.(*types.Named); !isNamed {
				// Type is a basic/primitive type - check for alias replacement
				for aliasName, alias := range re.aliases {
					if alias.UnderlyingType == typeInfo.Type.Underlying().String() {
						re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, pointerAndPosition.Index, len(re.gir.tokenSlice), []string{aliasName})
						break
					}
				}
			}
		}
	}
	str := re.emitAsString(" ", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	re.gir.emitToFileBuffer("", "@PostVisitDeclStmtValueSpecType")
}

func (re *RustEmitter) PreVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	if re.forwardDecls {
		return
	}
	re.declNameIndex = index
	re.gir.emitToFileBuffer("", "@PreVisitDeclStmtValueSpecNames")
}

func (re *RustEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	if re.forwardDecls {
		return
	}
	// Reorder tokens: swap type and name to get "name: type" format
	// This needs to be done for each name-type pair
	p1 := SearchPointerIndexReverse("@PreVisitDeclStmtValueSpecType", re.gir.pointerAndIndexVec)
	p2 := SearchPointerIndexReverse("@PostVisitDeclStmtValueSpecType", re.gir.pointerAndIndexVec)
	p3 := SearchPointerIndexReverse("@PreVisitDeclStmtValueSpecNames", re.gir.pointerAndIndexVec)

	// Save the type name BEFORE reordering for default initialization
	var typeName string
	if p1 != nil && p2 != nil {
		fieldType, err := ExtractTokensBetween(p1.Index, p2.Index, re.gir.tokenSlice)
		if err == nil && len(fieldType) > 0 {
			typeName = strings.TrimSpace(strings.Join(tokensToStrings(fieldType), ""))
		}
	}

	if p1 != nil && p2 != nil && p3 != nil {
		// Extract the type tokens
		fieldType, err := ExtractTokensBetween(p1.Index, p2.Index, re.gir.tokenSlice)
		if err == nil && len(fieldType) > 0 {
			// Extract the name tokens (from p3 to end)
			fieldName, err := ExtractTokensBetween(p3.Index, len(re.gir.tokenSlice), re.gir.tokenSlice)
			if err == nil && len(fieldName) > 0 {
				// Build new tokens: name: type
				newTokens := []string{}
				newTokens = append(newTokens, tokensToStrings(fieldName)...)
				newTokens = append(newTokens, ":")
				newTokens = append(newTokens, tokensToStrings(fieldType)...)
				re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, p1.Index, len(re.gir.tokenSlice), newTokens)
			}
		}
	}
	re.gir.emitToFileBuffer("", "@PostVisitDeclStmtValueSpecNames")
	var str string
	if re.isArray {
		str += " = Vec::new()"
		re.isArray = false
	} else {
		// Add default initialization based on type
		// Primitive numeric types get zero initialization
		primitiveDefaults := map[string]string{
			"i8": "0", "i16": "0", "i32": "0", "i64": "0",
			"u8": "0", "u16": "0", "u32": "0", "u64": "0",
			"f32": "0.0", "f64": "0.0",
			"bool": "false",
		}
		if defaultVal, isPrimitive := primitiveDefaults[typeName]; isPrimitive {
			str += " = " + defaultVal
		} else if typeName == "String" {
			str += " = String::new()"
		} else if len(typeName) > 0 && !strings.Contains(typeName, "Box<dyn") {
			// For struct types declared without value (var x StructType), initialize with default
			// Skip Box<dyn Any> - can't call default() on trait objects
			// Handle module-qualified types like types::Plan by checking the type name part
			typeNamePart := typeName
			if idx := strings.LastIndex(typeName, "::"); idx >= 0 {
				typeNamePart = typeName[idx+2:]
			}
			// Check if type name starts with uppercase (struct type)
			if len(typeNamePart) > 0 && typeNamePart[0] >= 'A' && typeNamePart[0] <= 'Z' {
				str += " = " + typeName + "::default()"
			}
		}
	}
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitGenStructFieldType(node ast.Expr, indent int) {
	if re.forwardDecls {
		return
	}
	str := re.emitAsString("pub ", indent+2)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	re.gir.emitToFileBuffer("", "@PreVisitGenStructFieldType")
}

func (re *RustEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	if re.forwardDecls {
		return
	}
	re.gir.emitToFileBuffer("", "@PostVisitGenStructFieldType")
	re.gir.emitToFileBuffer(" ", EmptyVisitMethod)
	// clean array marker as we should generate
	// initializer only for expression statements
	// not for struct fields
	re.isArray = false

}

func (re *RustEmitter) PreVisitGenStructFieldName(node *ast.Ident, indent int) {
	if re.forwardDecls {
		return
	}
	re.gir.emitToFileBuffer("", "@PreVisitGenStructFieldName")

}
func (re *RustEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	if re.forwardDecls {
		return
	}
	re.gir.emitToFileBuffer("", "@PostVisitGenStructFieldName")
	p1 := SearchPointerIndexReverse("@PreVisitGenStructFieldType", re.gir.pointerAndIndexVec)
	p2 := SearchPointerIndexReverse("@PostVisitGenStructFieldType", re.gir.pointerAndIndexVec)
	p3 := SearchPointerIndexReverse("@PreVisitGenStructFieldName", re.gir.pointerAndIndexVec)
	p4 := SearchPointerIndexReverse("@PostVisitGenStructFieldName", re.gir.pointerAndIndexVec)

	if p1 != nil && p2 != nil && p3 != nil && p4 != nil {
		fieldType, err := ExtractTokensBetween(p1.Index, p2.Index, re.gir.tokenSlice)
		if err != nil {
			fmt.Println("Error extracting field type:", err)
			return
		}
		fieldName, err := ExtractTokensBetween(p3.Index, p4.Index, re.gir.tokenSlice)
		if err != nil {
			fmt.Println("Error extracting field name:", err)
			return
		}
		newTokens := []string{}
		newTokens = append(newTokens, tokensToStrings(fieldName)...)
		newTokens = append(newTokens, ":")
		newTokens = append(newTokens, tokensToStrings(fieldType)...)
		re.gir.tokenSlice, err = RewriteTokensBetween(re.gir.tokenSlice, p1.Index, p4.Index, newTokens)
		if err != nil {
			fmt.Println("Error rewriting file buffer:", err)
			return
		}
	}

	re.gir.emitToFileBuffer(",\n", EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	if re.forwardDecls {
		return
	}
	re.pkg = pkg
	re.currentPackage = pkg.Name
	// Initialize the caches if not already done
	if re.processedPkgsInterfaceTypes == nil {
		re.processedPkgsInterfaceTypes = make(map[string]bool)
	}
	if re.localClosures == nil {
		re.localClosures = make(map[string]*ast.FuncLit)
	}
	if re.localClosureBodyTokens == nil {
		re.localClosureBodyTokens = make(map[string][]Token)
	}
	// Check if package has any interface{} types
	re.pkgHasInterfaceTypes = re.packageHasInterfaceTypes(pkg)
	// Cache this package's result
	re.processedPkgsInterfaceTypes[pkg.PkgPath] = re.pkgHasInterfaceTypes

	// Generate module declaration for non-main packages
	if pkg.Name != "main" {
		str := re.emitAsString(fmt.Sprintf("pub mod %s {\n", pkg.Name), indent)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
		// Import crate-root items (helper functions like append, len, println, etc.)
		str = re.emitAsString("use crate::*;\n\n", indent)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
}

// packageHasInterfaceTypes scans all structs in the package for interface{} fields
func (re *RustEmitter) packageHasInterfaceTypes(pkg *packages.Package) bool {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if structType, ok := typeSpec.Type.(*ast.StructType); ok {
							if structType.Fields != nil {
								for _, field := range structType.Fields.List {
									if field.Type != nil {
										typeStr := pkg.TypesInfo.Types[field.Type].Type.String()
										if strings.Contains(typeStr, "interface{}") || strings.Contains(typeStr, "interface {") {
											return true
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}
	return false
}

// typeHasInterfaceFields checks if a type contains interface{} fields (directly or transitively)
func (re *RustEmitter) typeHasInterfaceFields(t types.Type) bool {
	// Get the underlying type
	underlying := t.Underlying()
	if structType, ok := underlying.(*types.Struct); ok {
		for i := 0; i < structType.NumFields(); i++ {
			field := structType.Field(i)
			fieldTypeStr := field.Type().String()
			// Check for interface{} fields
			if strings.Contains(fieldTypeStr, "interface{}") || strings.Contains(fieldTypeStr, "interface {") {
				return true
			}
			// Check for function fields (Box<dyn Fn> in Rust doesn't implement Default)
			if strings.Contains(fieldTypeStr, "func(") {
				return true
			}
			// Check nested structs recursively
			if re.typeHasInterfaceFields(field.Type()) {
				return true
			}
		}
	}
	return false
}

func (re *RustEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	if re.forwardDecls {
		return
	}
	// Close the module declaration for non-main packages
	if pkg.Name != "main" {
		str := re.emitAsString(fmt.Sprintf("} // pub mod %s\n\n", pkg.Name), indent)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
}

func (re *RustEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	if re.forwardDecls {
		return
	}
	re.isArray = false
}

func (re *RustEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	if re.forwardDecls {
		return
	}
	str := re.emitAsString("\n", indent)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	if re.forwardDecls {
		return
	}
	str := re.emitAsString("\n\n", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitGenStructInfo(node GenTypeInfo, indent int) {
	if re.forwardDecls {
		return
	}
	// Check if this specific struct has interface{} fields or function fields
	// (can't derive Clone/Default/Debug for these)
	var str string
	hasInterfaceFields := re.structHasInterfaceFields(node.Name)
	hasFunctionFields := re.structHasFunctionFields(node.Name)
	if hasFunctionFields {
		// Structs with function fields can derive Clone (Rc implements Clone)
		// but not Default or Debug (dyn Fn doesn't implement these)
		str = re.emitAsString("#[derive(Clone)]\n", indent+2)
	} else if hasInterfaceFields {
		// Only derive Debug for structs with Any/interface{} fields
		str = re.emitAsString("#[derive(Debug)]\n", indent+2)
	} else {
		// Check if struct only has primitive/Copy types (can derive Copy)
		canCopy := re.structCanDeriveCopy(node.Name)
		if canCopy {
			str = re.emitAsString("#[derive(Default, Clone, Copy, Debug)]\n", indent+2)
		} else {
			// Add derive macros for Default (needed for ..Default::default() in struct init)
			str = re.emitAsString("#[derive(Default, Clone, Debug)]\n", indent+2)
		}
	}
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	str = re.emitAsString(fmt.Sprintf("pub struct %s\n", node.Name), indent+2)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	re.emitToken("{", LeftBrace, indent+2)
	str = re.emitAsString("\n", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	re.shouldGenerate = true
}

// structHasInterfaceFields checks if a struct has interface{} fields (directly or in nested structs)
func (re *RustEmitter) structHasInterfaceFields(structName string) bool {
	return re.structHasInterfaceFieldsRecursive(structName, make(map[string]bool))
}

// structHasInterfaceFieldsRecursive checks recursively if a struct has interface{} fields
func (re *RustEmitter) structHasInterfaceFieldsRecursive(structName string, visited map[string]bool) bool {
	// Prevent infinite recursion
	if visited[structName] {
		return false
	}
	visited[structName] = true

	for _, file := range re.pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if typeSpec.Name.Name == structName {
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								if structType.Fields != nil {
									for _, field := range structType.Fields.List {
										fieldType := re.pkg.TypesInfo.Types[field.Type].Type
										typeStr := fieldType.String()
										// Direct interface{} check
										if strings.Contains(typeStr, "interface{}") || strings.Contains(typeStr, "interface {") {
											return true
										}
										// Check for function fields (Box<dyn Fn> in Rust doesn't implement Clone)
										if strings.Contains(typeStr, "func(") {
											return true
										}
										// Check nested struct fields recursively
										if named, ok := fieldType.(*types.Named); ok {
											if _, isStruct := named.Underlying().(*types.Struct); isStruct {
												nestedName := named.Obj().Name()
												if re.structHasInterfaceFieldsRecursive(nestedName, visited) {
													return true
												}
											}
										}
										// Check slice element type
										if slice, ok := fieldType.(*types.Slice); ok {
											elemType := slice.Elem()
											if named, ok := elemType.(*types.Named); ok {
												if _, isStruct := named.Underlying().(*types.Struct); isStruct {
													nestedName := named.Obj().Name()
													if re.structHasInterfaceFieldsRecursive(nestedName, visited) {
														return true
													}
												}
											}
										}
									}
								}
								return false
							}
						}
					}
				}
			}
		}
	}
	return false
}

// structCanDeriveCopy checks if a struct only contains primitive/Copy fields
func (re *RustEmitter) structCanDeriveCopy(structName string) bool {
	for _, file := range re.pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if typeSpec.Name.Name == structName {
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								if structType.Fields != nil {
									for _, field := range structType.Fields.List {
										fieldType := re.pkg.TypesInfo.Types[field.Type].Type
										typeStr := fieldType.String()
										// If field is a slice/array, String, or interface{}, can't derive Copy
										if strings.HasPrefix(typeStr, "[]") ||
											typeStr == "string" ||
											strings.Contains(typeStr, "interface") {
											return false
										}
										// If field is a function type (will become Box<dyn Fn...>), can't derive Copy
										if strings.HasPrefix(typeStr, "func(") {
											return false
										}
										// If field is a struct type, can't safely derive Copy
										// (the nested struct might have non-Copy fields)
										if named, ok := fieldType.(*types.Named); ok {
											if _, isStruct := named.Underlying().(*types.Struct); isStruct {
												return false
											}
										}
										// Check underlying type for function signatures
										if _, isSig := fieldType.Underlying().(*types.Signature); isSig {
											return false
										}
									}
								}
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

// structHasFunctionFields checks if a struct has function/closure fields
func (re *RustEmitter) structHasFunctionFields(structName string) bool {
	for _, file := range re.pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if typeSpec.Name.Name == structName {
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								if structType.Fields != nil {
									for _, field := range structType.Fields.List {
										fieldType := re.pkg.TypesInfo.Types[field.Type].Type
										typeStr := fieldType.String()
										// Check for function types (will become Box<dyn Fn...>)
										if strings.HasPrefix(typeStr, "func(") {
											return true
										}
										// Check underlying type for function signatures
										if _, isSig := fieldType.Underlying().(*types.Signature); isSig {
											return true
										}
									}
								}
								return false
							}
						}
					}
				}
			}
		}
	}
	return false
}

func (re *RustEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {
	if re.forwardDecls {
		return
	}
	re.emitToken("}", RightBrace, indent+2)
	str := re.emitAsString("\n\n", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	re.shouldGenerate = false
}

func (re *RustEmitter) PreVisitArrayType(node ast.ArrayType, indent int) {
	if re.forwardDecls {
		return
	}
	re.gir.emitToFileBuffer("", "@@PreVisitArrayType")
	re.emitToken("<", LeftAngle, 0)
}
func (re *RustEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	if re.forwardDecls {
		return
	}

	re.emitToken(">", RightAngle, 0)

	pointerAndPosition := SearchPointerIndexReverse("@@PreVisitArrayType", re.gir.pointerAndIndexVec)
	if pointerAndPosition != nil {
		tokens, _ := ExtractTokens(pointerAndPosition.Index, re.gir.tokenSlice)
		re.isArray = true
		re.arrayType = strings.Join(tokens, "")
		// Prepend "Vec" before the array type tokens
		re.gir.tokenSlice, _ = RewriteTokens(re.gir.tokenSlice, pointerAndPosition.Index, []string{}, []string{"Vec"})
	}
}

func (re *RustEmitter) PreVisitFuncType(node *ast.FuncType, indent int) {
	if re.forwardDecls {
		return
	}
	re.gir.emitToFileBuffer("", "@@PreVisitFuncType")
	// Use Rc<dyn Fn> for function types - Rc implements Clone so structs with function fields can be cloned
	str := re.emitAsString("Rc<dyn Fn(", indent)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}
func (re *RustEmitter) PostVisitFuncType(node *ast.FuncType, indent int) {
	if re.forwardDecls {
		return
	}

	pointerAndPosition := SearchPointerIndexReverse("@@PreVisitFuncType", re.gir.pointerAndIndexVec)
	if pointerAndPosition != nil && re.numFuncResults > 0 {
		// For function types with return values, we need to reorder tokens
		// to move return type to the end (Rust syntax requirement)
		tokens, _ := ExtractTokens(pointerAndPosition.Index, re.gir.tokenSlice)
		if len(tokens) > 2 {
			// Find and move return type to end with arrow separator
			var reorderedTokens []string
			reorderedTokens = append(reorderedTokens, tokens[0]) // "Rc<dyn Fn("
			if len(tokens) > 3 {
				// Skip return type (index 1) and add parameters first
				reorderedTokens = append(reorderedTokens, tokens[2:]...)
				reorderedTokens = append(reorderedTokens, ") -> ")
				reorderedTokens = append(reorderedTokens, tokens[1]) // Add return type at end
				reorderedTokens = append(reorderedTokens, ">")
			} else {
				reorderedTokens = append(reorderedTokens, tokens[1:]...)
				reorderedTokens = append(reorderedTokens, ")>")
			}
			re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, pointerAndPosition.Index, len(re.gir.tokenSlice), reorderedTokens)
			return
		}
	}

	re.emitToken(")", RightParen, 0)
	re.emitToken(">", RightAngle, 0)
}

func (re *RustEmitter) PreVisitFuncTypeParam(node *ast.Field, index int, indent int) {
	if re.forwardDecls {
		return
	}
	if index > 0 {
		str := re.emitAsString(", ", 0)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
}

func (re *RustEmitter) PreVisitSelectorExprX(node ast.Expr, indent int) {
	// For builtin package names (like fmt), suppress generation
	// For user-defined module names (like types, ast), generate them
	if ident, ok := node.(*ast.Ident); ok {
		obj := re.pkg.TypesInfo.Uses[ident]
		if obj != nil {
			if _, ok := obj.(*types.PkgName); ok {
				// Check if this is a builtin package that gets lowered
				if re.lowerToBuiltins(ident.Name) == "" {
					// Builtin package (fmt) - suppress generation
					re.shouldGenerate = false
					return
				}
				// User-defined module - let it be generated
			}
		}
	}
}

func (re *RustEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	if re.forwardDecls {
		return
	}
	var str string
	scopeOperator := "." // Default to dot for field access
	isBuiltinPackage := false
	if ident, ok := node.(*ast.Ident); ok {
		// Check if this is a builtin package (like fmt) that we lower to crate-level functions
		if re.lowerToBuiltins(ident.Name) == "" {
			// This is a builtin package like "fmt"
			isBuiltinPackage = true
		}

		// Check if this is a package name - use :: for module-qualified access
		obj := re.pkg.TypesInfo.Uses[ident]
		if obj != nil {
			if _, ok := obj.(*types.PkgName); ok {
				// For builtin packages (fmt), don't emit any operator
				// The selector will be lowered to a crate-level function
				if isBuiltinPackage {
					re.shouldGenerate = true
					return
				}
				// Use :: for module-qualified access in Rust
				scopeOperator = "::"
			}
		}
		// Also check if the identifier is a known namespace/module
		if _, found := namespaces[ident.Name]; found {
			scopeOperator = "::"
		}
	}

	str = re.emitAsString(scopeOperator, 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitFuncTypeResults(node *ast.FieldList, indent int) {
	if re.forwardDecls {
		return
	}
	if node != nil {
		re.numFuncResults = len(node.List)
	}
}

func (re *RustEmitter) PreVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	if re.forwardDecls {
		return
	}
	if index > 0 {
		str := re.emitAsString(", ", 0)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
}

func (re *RustEmitter) PreVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	if re.forwardDecls {
		return
	}
	re.gir.emitToFileBuffer(" ", EmptyVisitMethod)
	re.gir.emitToFileBuffer("", "@PreVisitFuncDeclSignatureTypeParamsArgName")
}

func (re *RustEmitter) PreVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	if re.forwardDecls {
		return
	}
	if index > 0 {
		str := re.emitAsString(",", 0)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
	re.gir.emitToFileBuffer("", "@PreVisitFuncDeclSignatureTypeResultsList")
}

func (re *RustEmitter) PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	if re.forwardDecls {
		return
	}
	pointerAndPosition := SearchPointerIndexReverse("@PreVisitFuncDeclSignatureTypeResultsList", re.gir.pointerAndIndexVec)
	if pointerAndPosition != nil {
		typeInfo := re.pkg.TypesInfo.Types[node.Type]
		// Only do alias replacement if the type is NOT already a named type (alias)
		// If it's a named type like ast.AST, don't replace it with another alias
		if typeInfo.Type != nil {
			if _, isNamed := typeInfo.Type.(*types.Named); !isNamed {
				// Type is a basic/primitive type - check for alias replacement
				for aliasName, alias := range re.aliases {
					if alias.UnderlyingType == typeInfo.Type.Underlying().String() {
						re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, pointerAndPosition.Index, len(re.gir.tokenSlice), []string{aliasName})
						break
					}
				}
			}
		}
	}
}

func (re *RustEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	if re.forwardDecls {
		return
	}
	re.gir.emitToFileBuffer("", "@PreVisitFuncDeclSignatureTypeResults")
	re.shouldGenerate = true // Enable generating result types

	if node.Type.Results != nil {
		if len(node.Type.Results.List) > 1 {
			re.emitToken("(", LeftParen, 0)
		}
	}
}

func (re *RustEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	if re.forwardDecls {
		return
	}

	if node.Type.Results != nil {
		if len(node.Type.Results.List) > 1 {
			re.emitToken(")", RightParen, 0)
		}
	}

	str := re.emitAsString("", 1)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	re.gir.emitToFileBuffer("", "@PostVisitFuncDeclSignatureTypeResults")
}

func (re *RustEmitter) PreVisitTypeAliasName(node *ast.Ident, indent int) {
	if re.forwardDecls {
		return
	}
	re.gir.emitToFileBuffer("", "@@PreVisitTypeAliasName")
	str := re.emitAsString("pub type ", indent+2)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	re.shouldGenerate = true
}

func (re *RustEmitter) PostVisitTypeAliasName(node *ast.Ident, indent int) {
	if re.forwardDecls {
		return
	}
	re.gir.emitToFileBuffer(" = ", EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitTypeAliasType(node ast.Expr, indent int) {
	if re.forwardDecls {
		return
	}
}

func (re *RustEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	if re.forwardDecls {
		return
	}
	str := re.emitAsString(";\n\n", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)

	// Extract tokens for alias processing
	pointerAndPosition := SearchPointerIndexReverse("@@PreVisitTypeAliasName", re.gir.pointerAndIndexVec)
	if pointerAndPosition != nil {
		tokens, _ := ExtractTokens(pointerAndPosition.Index, re.gir.tokenSlice)
		if len(tokens) >= 3 {
			// tokens[0] = "type ", tokens[1] = alias name, tokens[2] = " = ", tokens[3+] = type
			aliasName := tokens[1]
			typeTokens := tokens[3 : len(tokens)-1] // exclude the ";\n\n" at the end
			typeStr := strings.Join(typeTokens, "")
			re.aliases[aliasName] = Alias{
				PackageName:    re.pkg.Name + ".Api",
				representation: ConvertToAliasRepr(ParseNestedTypes(typeStr), []string{"", re.pkg.Name + ".Api"}),
				UnderlyingType: re.pkg.TypesInfo.Types[node].Type.String(),
			}
		}
	}
	re.shouldGenerate = false
}

func (re *RustEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	if re.forwardDecls {
		return
	}
	re.shouldGenerate = true
	re.inReturnStmt = true
	re.currentReturnNode = node
	str := re.emitAsString("return ", indent)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)

	if len(node.Results) > 1 {
		re.inMultiValueReturn = true
		re.multiValueReturnResultIndex = 0
		re.emitToken("(", LeftParen, 0)
	}
}

func (re *RustEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	if re.forwardDecls {
		return
	}
	if len(node.Results) > 1 {
		re.emitToken(")", RightParen, 0)
	}
	re.inMultiValueReturn = false
	re.inReturnStmt = false
	re.currentReturnNode = nil
	str := re.emitAsString(";", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	if index > 0 {
		str := re.emitAsString(", ", 0)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
	re.multiValueReturnResultIndex = index

	// If returning from a function that returns any (Box<dyn Any>),
	// and the return value is a concrete type, wrap in Box::new()
	if re.currentFuncReturnsAny && node != nil {
		nodeType := re.pkg.TypesInfo.Types[node]
		if nodeType.Type != nil {
			typeStr := nodeType.Type.String()
			// Don't wrap if already Box<dyn Any> or interface{}
			if typeStr != "interface{}" && typeStr != "any" && !strings.Contains(typeStr, "Box<dyn Any>") {
				re.gir.emitToFileBuffer("Box::new(", EmptyVisitMethod)
			}
		}
	}
}

func (re *RustEmitter) PostVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	if re.forwardDecls {
		return
	}
	// Add .clone() to the first result in a multi-value return if it's an identifier.
	// With OptimizeMoves: only clone when a later result references the same identifier.
	// Without OptimizeMoves: always clone (baseline behavior).
	if re.inMultiValueReturn && index == 0 {
		if _, ok := node.(*ast.Ident); ok {
			if !re.OptimizeMoves {
				// Baseline: unconditional clone
				re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
			} else {
				// Optimized: only clone when a later result references the same identifier
				needsClone := false
				if ident, ok2 := node.(*ast.Ident); ok2 && re.currentReturnNode != nil {
					for i := 1; i < len(re.currentReturnNode.Results); i++ {
						if re.exprContainsIdent(re.currentReturnNode.Results[i], ident.Name) {
							needsClone = true
							break
						}
					}
				}
				if needsClone {
					re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
				} else {
					re.MoveOptCount++
				}
			}
		}
	}

	// Close Box::new() if we opened it in Pre
	if re.currentFuncReturnsAny && node != nil {
		nodeType := re.pkg.TypesInfo.Types[node]
		if nodeType.Type != nil {
			typeStr := nodeType.Type.String()
			if typeStr != "interface{}" && typeStr != "any" && !strings.Contains(typeStr, "Box<dyn Any>") {
				re.gir.emitToFileBuffer(")", EmptyVisitMethod)
			}
		}
	}
}

func (re *RustEmitter) PreVisitCallExpr(node *ast.CallExpr, indent int) {
	re.shouldGenerate = true
	// In += context, string functions return String but += expects &str
	// Add & before calls to string_format (Sprintf)
	if re.inAssignRhs && re.assignmentToken == "+=" {
		if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
			if sel.Sel.Name == "Sprintf" {
				re.gir.emitToFileBuffer("&", EmptyVisitMethod)
			}
		}
	}
}

func (re *RustEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {
	// Note: Do NOT set shouldGenerate = false here!
	// This would prevent subsequent operands in expressions from being generated.
	// For example, in (a + b) + c where b is a call, setting false would suppress 'c'.
}

func (re *RustEmitter) PreVisitDeclStmt(node *ast.DeclStmt, indent int) {
	re.shouldGenerate = true
	// Get info about this declaration for multi-name handling
	if genDecl, ok := node.Decl.(*ast.GenDecl); ok {
		for _, spec := range genDecl.Specs {
			if valueSpec, ok := spec.(*ast.ValueSpec); ok {
				re.declNameCount = len(valueSpec.Names)
				// Store type info for multi-name declarations
				if valueSpec.Type != nil {
					re.declType = re.pkg.TypesInfo.Types[valueSpec.Type].Type.String()
				}
			}
		}
	}
	re.declNameIndex = 0
	// Use "let mut" for var declarations since they may be reassigned
	re.emitToken("let", RustKeyword, indent)
	re.emitToken(" ", WhiteSpace, 0)
	re.emitToken("mut", RustKeyword, 0)
	re.emitToken(" ", WhiteSpace, 0)
}

func (re *RustEmitter) PostVisitDeclStmt(node *ast.DeclStmt, indent int) {
	// Reordering is now done per-name in PostVisitDeclStmtValueSpecNames
	re.emitToken(";", Semicolon, 0)
	re.shouldGenerate = false
}

func (re *RustEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	// Check if all LHS are blank identifiers - if so, suppress the statement
	allBlank := true
	for _, lhs := range node.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok {
			if ident.Name != "_" {
				allBlank = false
				break
			}
		} else {
			allBlank = false
			break
		}
	}
	if allBlank {
		re.suppressRangeEmit = true
		return
	}
	re.shouldGenerate = true
	// Capture LHS variable names for move optimization
	re.currentAssignLhsNames = make(map[string]bool)
	for _, lhs := range node.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok {
			re.currentAssignLhsNames[ident.Name] = true
		}
	}
	// Analyze for temp extraction (pre-extract field accesses so struct can be moved)
	re.analyzeMoveOptExtraction(node, indent)
	// Emit temp variable bindings before the assignment
	if len(re.moveOptTempBindings) > 0 {
		for _, binding := range re.moveOptTempBindings {
			bindStr := re.emitAsString(binding, indent)
			re.gir.emitToFileBuffer(bindStr, EmptyVisitMethod)
		}
	}
	str := re.emitAsString("", indent)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}
func (re *RustEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	re.currentAssignLhsNames = nil
	re.moveOptActive = false
	re.moveOptTempBindings = nil
	re.moveOptArgReplacements = nil
	re.moveOptModifiedCounts = nil
	// Reset blank identifier suppression if it was set
	if re.suppressRangeEmit {
		re.suppressRangeEmit = false
		return
	}
	// Don't emit semicolon inside for loop post statement
	if !re.insideForPostCond {
		re.emitToken(";", Semicolon, 0)
	}
	re.shouldGenerate = false
}

func (re *RustEmitter) PreVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	re.shouldGenerate = true
	re.inAssignRhs = true
	opTokenType := re.getTokenType(re.assignmentToken)
	re.emitToken(re.assignmentToken, opTokenType, indent+1)
	re.emitToken(" ", WhiteSpace, 0)

	// For += with String, Rust expects &str on RHS
	// Add & before RHS if it's a String variable (not a string literal)
	if re.assignmentToken == "+=" && len(node.Rhs) == 1 {
		rhsType := re.pkg.TypesInfo.Types[node.Rhs[0]]
		if rhsType.Type != nil && rhsType.Type.String() == "string" {
			// Check if RHS is not a string literal (literals are handled separately)
			if _, isBasicLit := node.Rhs[0].(*ast.BasicLit); !isBasicLit {
				re.gir.emitToFileBuffer("&", EmptyVisitMethod)
			}
		}
	}

	// If assigning to a variable of type any (interface{}), wrap RHS in Box::new()
	if len(node.Lhs) == 1 && len(node.Rhs) == 1 && re.assignmentToken == "=" {
		lhsType := re.pkg.TypesInfo.Types[node.Lhs[0]]
		rhsType := re.pkg.TypesInfo.Types[node.Rhs[0]]
		if lhsType.Type != nil && rhsType.Type != nil {
			lhsTypeStr := lhsType.Type.String()
			rhsTypeStr := rhsType.Type.String()
			// If LHS is any/interface{} and RHS is a concrete type, wrap in Box::new()
			if (lhsTypeStr == "interface{}" || lhsTypeStr == "any") &&
				rhsTypeStr != "interface{}" && rhsTypeStr != "any" {
				re.gir.emitToFileBuffer("Box::new(", EmptyVisitMethod)
			}
		}
	}
}

func (re *RustEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	// Check if we need to add a type cast for constant assignments
	// This handles untyped int constants assigned to i8 variables
	if len(node.Lhs) == 1 && len(node.Rhs) == 1 {
		// Get the LHS type
		lhsType := re.pkg.TypesInfo.Types[node.Lhs[0]]
		rhsType := re.pkg.TypesInfo.Types[node.Rhs[0]]
		if lhsType.Type != nil {
			lhsTypeStr := lhsType.Type.String()

			// Close Box::new() if we opened it for any/interface{} assignment
			if rhsType.Type != nil {
				rhsTypeStr := rhsType.Type.String()
				if (lhsTypeStr == "interface{}" || lhsTypeStr == "any") &&
					rhsTypeStr != "interface{}" && rhsTypeStr != "any" &&
					re.assignmentToken == "=" {
					re.gir.emitToFileBuffer(")", EmptyVisitMethod)
				}
			}

			// Check if RHS is a constant identifier
			if rhsIdent, ok := node.Rhs[0].(*ast.Ident); ok {
				if obj := re.pkg.TypesInfo.Uses[rhsIdent]; obj != nil {
					if _, isConst := obj.(*types.Const); isConst {
						// Get the constant type
						constType := obj.Type().String()
						// If assigning int constant to int8 field/variable, add cast
						if (constType == "int" || constType == "untyped int") && lhsTypeStr == "int8" {
							re.gir.emitToFileBuffer(" as i8", EmptyVisitMethod)
						}
					}
				}
			}
		}

		// Add .clone() for simple identifier RHS of non-Copy types
		// This handles cases like: x = y where y is a struct/string/slice variable
		// Skip if RHS is an index expression (already clones) or call expression
		if rhsIdent, ok := node.Rhs[0].(*ast.Ident); ok {
			// Skip constants - they don't need cloning
			if obj := re.pkg.TypesInfo.Uses[rhsIdent]; obj != nil {
				if _, isConst := obj.(*types.Const); isConst {
					// Constants don't need clone
				} else if rhsType.Type != nil {
					// Check if it's a non-Copy type that needs cloning
					needsClone := false
					typeStr := rhsType.Type.String()

					// String type
					if typeStr == "string" {
						needsClone = true
					}

					// Slice type
					if strings.HasPrefix(typeStr, "[]") {
						needsClone = true
					}

					// Struct type (named or anonymous)
					if named, ok := rhsType.Type.(*types.Named); ok {
						if _, isStruct := named.Underlying().(*types.Struct); isStruct {
							needsClone = true
						}
					}
					if _, isStruct := rhsType.Type.(*types.Struct); isStruct {
						needsClone = true
					}

					if needsClone {
						re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
					}
				}
			}
		}
	}

	// For local closure assignments, remove the entire statement from token stream
	// The body tokens have already been stored in PostVisitFuncLit
	// Only truncate for the outer closure assignment, not inner assignments
	// inLocalClosureBody is false after PostVisitFuncLit, true while inside closure body
	if re.localClosureAssign && re.currentClosureName != "" && !re.inLocalClosureBody {
		// Remove all tokens from the assignment start to current position
		if re.localClosureAssignStartIndex < len(re.gir.tokenSlice) {
			re.gir.tokenSlice = re.gir.tokenSlice[:re.localClosureAssignStartIndex]
		}
		// Reset flags
		re.localClosureAssign = false
		re.currentClosureName = ""
	}

	re.shouldGenerate = false
	re.isTuple = false
	re.inAssignRhs = false
}

func (re *RustEmitter) PreVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	if index > 0 {
		str := re.emitAsString(", ", indent)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
		// For multi-value declarations, add mut before each subsequent variable
		if re.inMultiValueDecl {
			re.emitToken("mut", RustKeyword, 0)
			re.emitToken(" ", WhiteSpace, 0)
		}
	}
}

func (re *RustEmitter) PreVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	re.shouldGenerate = true
	re.inAssignLhs = true // Track that we're in LHS
	assignmentToken := node.Tok.String()

	// Track += and -= in for loop post statement for step detection
	if re.insideForPostCond {
		if assignmentToken == "+=" {
			// Check if RHS is a numeric literal for step value
			if len(node.Rhs) == 1 {
				if lit, ok := node.Rhs[0].(*ast.BasicLit); ok && lit.Kind == token.INT {
					step := 1 // default
					fmt.Sscanf(lit.Value, "%d", &step)
					re.forLoopStep = step
					re.sawIncrement = true
				}
			}
		} else if assignmentToken == "-=" {
			// Decrement step
			if len(node.Rhs) == 1 {
				if lit, ok := node.Rhs[0].(*ast.BasicLit); ok && lit.Kind == token.INT {
					step := 1 // default
					fmt.Sscanf(lit.Value, "%d", &step)
					re.forLoopStep = step
					re.sawDecrement = true
					re.forLoopReverse = true
				}
			}
		}
	}
	// Check if LHS is a field access (SelectorExpr)
	re.inFieldAssign = false
	if len(node.Lhs) == 1 {
		if _, ok := node.Lhs[0].(*ast.SelectorExpr); ok {
			re.inFieldAssign = true
		}
	}
	// Check if RHS is a function literal (for local closure inlining)
	// Don't reset if we're inside a local closure body being processed
	if !re.inLocalClosureBody {
		re.localClosureAssign = false
		re.currentClosureName = ""
	}
	if assignmentToken == ":=" && len(node.Rhs) == 1 {
		if funcLit, ok := node.Rhs[0].(*ast.FuncLit); ok {
			if ident, ok := node.Lhs[0].(*ast.Ident); ok {
				re.localClosureAssign = true
				re.currentClosureName = ident.Name
				re.localClosures[ident.Name] = funcLit
				// Record assignment start index for later removal
				re.localClosureAssignStartIndex = len(re.gir.tokenSlice)
				// Skip emitting the assignment - we'll inline the closure body at call sites
				re.shouldGenerate = false
				return
			}
		}
	}
	re.inMultiValueDecl = false
	if assignmentToken == ":=" && len(node.Lhs) == 1 {
		re.emitToken("let", RustKeyword, indent)
		re.emitToken(" ", WhiteSpace, 0)
		re.emitToken("mut", RustKeyword, 0)
		re.emitToken(" ", WhiteSpace, 0)
	} else if assignmentToken == ":=" && len(node.Lhs) > 1 {
		// Multi-value declaration: let (mut a, mut b) = ...
		re.inMultiValueDecl = true
		re.emitToken("let", RustKeyword, indent)
		re.emitToken(" ", WhiteSpace, 0)
		re.emitToken("(", LeftParen, 0)
		re.emitToken("mut", RustKeyword, 0)
		re.emitToken(" ", WhiteSpace, 0)
	} else if assignmentToken == "=" && len(node.Lhs) > 1 {
		re.emitToken("(", LeftParen, indent)
		re.isTuple = true
	}
	// Preserve compound assignment operators, convert := to =
	if assignmentToken != "+=" && assignmentToken != "-=" && assignmentToken != "*=" && assignmentToken != "/=" {
		assignmentToken = "="
	}
	re.assignmentToken = assignmentToken
}

func (re *RustEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	// For local closure inlining, skip all emission - the assignment will be removed
	if re.localClosureAssign && re.currentClosureName != "" {
		re.shouldGenerate = false
		return
	}
	if node.Tok.String() == ":=" && len(node.Lhs) > 1 {
		re.emitToken(")", RightParen, indent)
	} else if node.Tok.String() == "=" && len(node.Lhs) > 1 {
		re.emitToken(")", RightParen, indent)
	}
	re.inAssignLhs = false // Done with LHS
	re.shouldGenerate = false
}

func (re *RustEmitter) PreVisitIndexExpr(node *ast.IndexExpr, indent int) {
	// For assignment RHS, check if the element type is a function (needs borrowing in Rust)
	if re.inAssignRhs {
		tv := re.pkg.TypesInfo.Types[node.X]
		if tv.Type != nil {
			// Check if it's a slice/array of functions
			typeStr := tv.Type.String()
			if strings.Contains(typeStr, "func(") {
				// Add borrow operator for function types
				re.gir.emitToFileBuffer("&", EmptyVisitMethod)
			}
		}
	}
}

func (re *RustEmitter) PreVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	re.shouldGenerate = true
	// If the base expression is a string, we need .as_bytes() for indexing
	if node.X != nil {
		tv := re.pkg.TypesInfo.Types[node.X]
		if tv.Type != nil && tv.Type.String() == "string" {
			re.gir.emitToFileBuffer(".as_bytes()", EmptyVisitMethod)
		}
	}
	re.emitToken("[", LeftBracket, 0)

}
func (re *RustEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	// Check if the index type is an integer (not usize) - need to add "as usize"
	if node.Index != nil {
		tv := re.pkg.TypesInfo.Types[node.Index]
		if tv.Type != nil {
			typeStr := tv.Type.String()
			// Go int types need to be cast to usize for Rust indexing
			if typeStr == "int" || typeStr == "int32" || typeStr == "int64" ||
				typeStr == "int8" || typeStr == "int16" {
				re.gir.emitToFileBuffer(" as usize", EmptyVisitMethod)
			}
		}
	}
	re.emitToken("]", RightBracket, 0)

	// Add .clone() for Vec element access when the element type doesn't implement Copy
	// This is needed because Rust doesn't allow moving out of indexed collections
	// BUT: Don't add .clone() when we're in the LHS of an assignment (we're assigning TO it)
	if node.X != nil && !re.inAssignLhs {
		tv := re.pkg.TypesInfo.Types[node.X]
		if tv.Type != nil {
			// Check if it's a slice/array type
			underlying := tv.Type.Underlying()
			if sliceType, ok := underlying.(*types.Slice); ok {
				elemType := sliceType.Elem()
				// Check if element type is a struct (non-Copy type)
				if _, isStruct := elemType.Underlying().(*types.Struct); isStruct {
					re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
				}
				// Check if element type is a string (also non-Copy in Rust)
				if basic, isBasic := elemType.Underlying().(*types.Basic); isBasic {
					if basic.Kind() == types.String {
						re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
					}
				}
			}
		}
	}
}

func (re *RustEmitter) PreVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	re.shouldGenerate = true
	re.emitToken("(", LeftParen, 1)

	// Save current state for nested expressions
	re.binaryNeedsLeftCastStack = append(re.binaryNeedsLeftCastStack, re.binaryNeedsLeftCast)
	re.binaryNeedsRightCastStack = append(re.binaryNeedsRightCastStack, re.binaryNeedsRightCast)
	re.inFloatBinaryExprStack = append(re.inFloatBinaryExprStack, re.inFloatBinaryExpr)
	re.binaryNeedsLeftCast = false
	re.binaryNeedsRightCast = ""
	re.inFloatBinaryExpr = false

	// Check if either operand is float64 - integer literals need .0 suffix
	leftType := re.pkg.TypesInfo.Types[node.X]
	rightType := re.pkg.TypesInfo.Types[node.Y]
	if leftType.Type != nil && (leftType.Type.String() == "float64" || leftType.Type.String() == "float32") {
		re.inFloatBinaryExpr = true
	}
	if rightType.Type != nil && (rightType.Type.String() == "float64" || rightType.Type.String() == "float32") {
		re.inFloatBinaryExpr = true
	}

	// Check for string concatenation - in Rust, String + &str is needed
	if node.Op == token.ADD {
		if leftType.Type != nil && leftType.Type.String() == "string" {
			// Check if right side is a function call or identifier returning string
			if rightType.Type != nil && rightType.Type.String() == "string" {
				// Mark that we need to add & before right operand
				re.binaryNeedsRightCast = "&"
			}
		}
	}

	// Check if left operand is u8/i8/u16/i16 and right is a constant
	// Go's type checker sees both as same type after implicit conversion, but
	// we generate constants as i32, so we need to handle type mismatches
	isComparisonOp := node.Op == token.EQL || node.Op == token.NEQ ||
		node.Op == token.LSS || node.Op == token.GTR ||
		node.Op == token.LEQ || node.Op == token.GEQ
	isBitwiseOp := node.Op == token.AND || node.Op == token.OR || node.Op == token.XOR

	if leftType.Type != nil {
		leftStr := leftType.Type.String()
		// Check if left is a small integer type
		if leftStr == "uint8" || leftStr == "int8" || leftStr == "uint16" || leftStr == "int16" {
			// Helper function to check if right side is a constant
			rightIsConst := false
			if ident, ok := node.Y.(*ast.Ident); ok {
				if obj := re.pkg.TypesInfo.Uses[ident]; obj != nil {
					if _, isConst := obj.(*types.Const); isConst {
						rightIsConst = true
					}
				}
			}
			if sel, ok := node.Y.(*ast.SelectorExpr); ok {
				if obj := re.pkg.TypesInfo.Uses[sel.Sel]; obj != nil {
					if _, isConst := obj.(*types.Const); isConst {
						rightIsConst = true
					}
				}
			}

			if rightIsConst {
				if isComparisonOp {
					// For comparisons, cast left to i32 (result is bool)
					re.binaryNeedsLeftCast = true
				} else if isBitwiseOp {
					// For bitwise ops, cast right constant to match left type
					// so the result has the correct type
					rustType := re.mapGoTypeToRust(leftStr)
					re.binaryNeedsRightCast = rustType
				}
			}

			// Also check if right side is a binary expression or paren expr containing one
			// (e.g., 0xFF - FlagZ or (0xFF - FlagZ)) that will evaluate to i32 in Rust
			if isBitwiseOp {
				// Get the actual expression, unwrapping ParenExpr if needed
				rightExpr := node.Y
				if parenExpr, ok := rightExpr.(*ast.ParenExpr); ok {
					rightExpr = parenExpr.X
				}

				if _, ok := rightExpr.(*ast.BinaryExpr); ok {
					// Check if the expression contains constants or literals
					// that will result in i32
					hasIntLiteral := false
					ast.Inspect(rightExpr, func(n ast.Node) bool {
						if lit, ok := n.(*ast.BasicLit); ok {
							if lit.Kind == token.INT {
								hasIntLiteral = true
							}
						}
						if ident, ok := n.(*ast.Ident); ok {
							if obj := re.pkg.TypesInfo.Uses[ident]; obj != nil {
								if _, isConst := obj.(*types.Const); isConst {
									hasIntLiteral = true
								}
							}
						}
						return true
					})
					if hasIntLiteral {
						rustType := re.mapGoTypeToRust(leftStr)
						re.binaryNeedsRightCast = rustType
					}
				}
			}
		}
	}
}

func (re *RustEmitter) PostVisitBinaryExprLeft(node ast.Expr, indent int) {
	// Add cast to i32 if needed for type compatibility with constants
	if re.binaryNeedsLeftCast {
		re.gir.emitToFileBuffer(" as i32", EmptyVisitMethod)
	}
}

func (re *RustEmitter) PreVisitBinaryExprRight(node ast.Expr, indent int) {
	// Add & before right operand for string concatenation (Rust requires String + &str)
	if re.binaryNeedsRightCast == "&" && !re.forwardDecls {
		re.gir.emitToFileBuffer("&", EmptyVisitMethod)
	}
}

func (re *RustEmitter) PostVisitBinaryExprRight(node ast.Expr, indent int) {
	// Add cast for right operand (constant) if needed for bitwise operations
	// Skip if it was the & for string concatenation (that's a prefix, not a suffix)
	if re.binaryNeedsRightCast != "" && re.binaryNeedsRightCast != "&" && !re.forwardDecls {
		re.gir.emitToFileBuffer(fmt.Sprintf(" as %s", re.binaryNeedsRightCast), EmptyVisitMethod)
	}
}

func (re *RustEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	re.emitToken(")", RightParen, 1)
	// Restore previous state for nested expressions
	if len(re.binaryNeedsLeftCastStack) > 0 {
		re.binaryNeedsLeftCast = re.binaryNeedsLeftCastStack[len(re.binaryNeedsLeftCastStack)-1]
		re.binaryNeedsLeftCastStack = re.binaryNeedsLeftCastStack[:len(re.binaryNeedsLeftCastStack)-1]
	}
	if len(re.binaryNeedsRightCastStack) > 0 {
		re.binaryNeedsRightCast = re.binaryNeedsRightCastStack[len(re.binaryNeedsRightCastStack)-1]
		re.binaryNeedsRightCastStack = re.binaryNeedsRightCastStack[:len(re.binaryNeedsRightCastStack)-1]
	}
	if len(re.inFloatBinaryExprStack) > 0 {
		re.inFloatBinaryExpr = re.inFloatBinaryExprStack[len(re.inFloatBinaryExprStack)-1]
		re.inFloatBinaryExprStack = re.inFloatBinaryExprStack[:len(re.inFloatBinaryExprStack)-1]
	}
	// Note: Do NOT set shouldGenerate = false here!
	// This would prevent the right operand of nested binary expressions from being generated.
	// For example, in (a + b) + c, setting false after (a + b) would suppress 'c'.
}

func (re *RustEmitter) PreVisitBinaryExprOperator(op token.Token, indent int) {
	content := op.String()
	opTokenType := re.getTokenType(content)
	re.emitToken(content, opTokenType, 0)
	re.emitToken(" ", WhiteSpace, 0)
}

func (re *RustEmitter) PreVisitCallExprArg(node ast.Expr, index int, indent int) {
	if index > 0 {
		str := re.emitAsString(", ", 0)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
	// Track that we're inside a call argument (for closure wrapping decisions)
	re.inCallExprArg = true
	// Record marker if this arg will be replaced by a temp variable.
	// Only apply at outermost call depth (stack depth == 1) to avoid
	// nested CallExpr (like type conversions int(x)) resetting the flag.
	if re.moveOptActive && re.moveOptArgReplacements != nil && len(re.currentCallArgIdentsStack) == 1 {
		if _, ok := re.moveOptArgReplacements[index]; ok {
			re.moveOptArgStartMarker = len(re.gir.tokenSlice)
			re.moveOptReplacingArg = true
		}
	}
}

func (re *RustEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {
	// Replace arg tokens with temp var name if this arg was pre-extracted.
	// Only at outermost call depth (stack depth == 1).
	defer func() {
		if re.moveOptReplacingArg && len(re.currentCallArgIdentsStack) == 1 {
			re.gir.tokenSlice = re.gir.tokenSlice[:re.moveOptArgStartMarker]
			re.emitToken(re.moveOptArgReplacements[index], Identifier, 0)
			re.moveOptReplacingArg = false
		}
	}()
	if re.forwardDecls {
		re.inCallExprArg = false
		return
	}
	// Check if the argument type needs .clone()
	tv := re.pkg.TypesInfo.Types[node]
	if tv.Type != nil {
		typeStr := tv.Type.String()

		// Clone Vec/slice types (but not for append which takes ownership)
		if strings.HasPrefix(typeStr, "[]") {
			if !re.currentCallIsAppend {
				re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
			}
			re.inCallExprArg = false
			return
		}

		// Clone String types (but not string literals - those get .to_string() anyway)
		if typeStr == "string" {
			if _, isBasicLit := node.(*ast.BasicLit); !isBasicLit {
				re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
				re.inCallExprArg = false
				return
			}
		}

		// Check if it's a named type (potential struct or slice alias)
		if named, ok := tv.Type.(*types.Named); ok {
			// Check if underlying type is a slice (e.g., type AST []Statement)
			if _, isSlice := named.Underlying().(*types.Slice); isSlice {
				if !re.currentCallIsAppend {
					re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
				}
				re.inCallExprArg = false
				return
			}
			// Check if underlying type is a struct
			if underlyingStruct, isStruct := named.Underlying().(*types.Struct); isStruct {
				// Check if any field has interface{} type (Box<dyn Any> doesn't implement Clone)
				// Note: function fields now use Rc<dyn Fn> which implements Clone
				hasNonClonableField := false
				for i := 0; i < underlyingStruct.NumFields(); i++ {
					field := underlyingStruct.Field(i)
					fieldTypeStr := field.Type().String()
					if strings.Contains(fieldTypeStr, "interface{}") || strings.Contains(fieldTypeStr, "interface {") {
						hasNonClonableField = true
						break
					}
				}
				if hasNonClonableField {
					// Don't clone structs with interface fields (Box<dyn Any> doesn't implement Clone)
					re.inCallExprArg = false
					return
				}
				// Clone structs unless the argument can be moved (consumed and reassigned)
				if identNode, isIdent := node.(*ast.Ident); !isIdent || !re.canMoveArg(identNode.Name) {
					re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
				}
				re.inCallExprArg = false
				return
			}
		}
		// Also handle non-named struct types (rare but possible)
		if _, isStruct := tv.Type.(*types.Struct); isStruct {
			if identNode, isIdent := node.(*ast.Ident); !isIdent || !re.canMoveArg(identNode.Name) {
				re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
			}
			re.inCallExprArg = false
			return
		}
	}

	// Liveness-based clone: if this identifier will be used in a later statement,
	// we need to clone it now to avoid Rust move errors
	if ident, isIdent := node.(*ast.Ident); isIdent {
		if re.isVariableUsedInLaterStatements(ident.Name) {
			// Skip clone if variable is being reassigned from this call's return value
			// Later uses will see the new value, so no clone needed
			if re.canMoveArg(ident.Name) {
				re.inCallExprArg = false
				return
			}
			// Check if the type requires cloning (non-Copy types)
			if tv.Type != nil {
				typeStr := tv.Type.String()
				// Clone for slice/Vec types
				if strings.Contains(typeStr, "[]") {
					if !re.currentCallIsAppend {
						re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
					}
					re.inCallExprArg = false
					return
				}
				// Clone for string types
				if typeStr == "string" {
					re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
					re.inCallExprArg = false
					return
				}
				// Clone for named types (structs or slices)
				if named, ok := tv.Type.(*types.Named); ok {
					// Check if underlying type is a slice (e.g., type AST []Statement)
					if _, isSlice := named.Underlying().(*types.Slice); isSlice {
						if !re.currentCallIsAppend {
							re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
						}
						re.inCallExprArg = false
						return
					}
					if _, isStruct := named.Underlying().(*types.Struct); isStruct {
						re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
						re.inCallExprArg = false
						return
					}
				}
			}
		}
	}
	// Clear the call argument flag
	re.inCallExprArg = false
}

func (re *RustEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	str := re.emitAsString(";", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	re.shouldGenerate = true
}
func (re *RustEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	re.shouldGenerate = false
}

func (re *RustEmitter) PreVisitIfStmtCond(node *ast.IfStmt, indent int) {
	str := re.emitAsString("if ", 1)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	re.emitToken("(", LeftParen, 0)
}

func (re *RustEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	re.emitToken(")", RightParen, 0)
	str := re.emitAsString("\n", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	re.insideForPostCond = true
	// Reset all for loop tracking flags
	re.sawIncrement = false
	re.sawDecrement = false
	re.forLoopStep = 0
	re.forLoopInclusive = false
	re.forLoopReverse = false
	re.pendingLoopIncrement = false
	re.inForLoopBody = false

	// Check for compound condition (contains && or ||)
	hasCompoundCond := re.hasCompoundCondition(node.Cond)
	if hasCompoundCond && node.Init != nil && node.Post != nil {
		// This is a for loop with compound condition - will be converted to while loop
		// Extract the loop variable and increment info from Init and Post
		if assign, ok := node.Init.(*ast.AssignStmt); ok && len(assign.Lhs) > 0 {
			if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
				re.loopIncrementVar = ident.Name
			}
		}
		// Check Post for increment/decrement
		if inc, ok := node.Post.(*ast.IncDecStmt); ok {
			if inc.Tok.String() == "++" {
				re.loopIncrementOp = "+="
			} else {
				re.loopIncrementOp = "-="
			}
			re.loopIncrementVal = "1"
		} else if assign, ok := node.Post.(*ast.AssignStmt); ok {
			// Handle i += n or i -= n
			if assign.Tok.String() == "+=" {
				re.loopIncrementOp = "+="
			} else if assign.Tok.String() == "-=" {
				re.loopIncrementOp = "-="
			}
			if len(assign.Rhs) > 0 {
				if lit, ok := assign.Rhs[0].(*ast.BasicLit); ok {
					re.loopIncrementVal = lit.Value
				}
			}
		}
		re.pendingLoopIncrement = true
	}

	// Detect loop type upfront to emit correct Rust keyword
	var str string
	if node.Init == nil && node.Cond == nil && node.Post == nil {
		// Infinite loop: for { } -> loop { }
		str = re.emitAsString("loop", indent)
		re.isInfiniteLoop = true
	} else {
		str = re.emitAsString("for ", indent)
		re.isInfiniteLoop = false
	}
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	re.shouldGenerate = true
}

// hasCompoundCondition checks if the expression contains && or ||
func (re *RustEmitter) hasCompoundCondition(expr ast.Expr) bool {
	if expr == nil {
		return false
	}
	if binExpr, ok := expr.(*ast.BinaryExpr); ok {
		if binExpr.Op.String() == "&&" || binExpr.Op.String() == "||" {
			return true
		}
		// Check nested expressions
		return re.hasCompoundCondition(binExpr.X) || re.hasCompoundCondition(binExpr.Y)
	}
	return false
}

func (re *RustEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	// Don't emit semicolon for infinite loops (they use `loop` keyword)
	if re.isInfiniteLoop {
		return
	}
	if node == nil {
		str := re.emitAsString(";", 0)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
}

func (re *RustEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	re.insideForPostCond = false
	str := re.emitAsString("\n", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitIfStmtElse(node *ast.IfStmt, indent int) {
	str := re.emitAsString("else", 1)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	// Don't emit semicolon for infinite loops (they use `loop` keyword)
	if !re.isInfiniteLoop {
		str := re.emitAsString(";", 0)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
	re.shouldGenerate = false
}

func (re *RustEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {
	re.shouldGenerate = false
	re.insideForPostCond = false

	p1 := SearchPointerIndexReverse(PreVisitForStmtInit, re.gir.pointerAndIndexVec)
	p2 := SearchPointerIndexReverse(PostVisitForStmtInit, re.gir.pointerAndIndexVec)
	var forVars []Token
	var rangeTokens []Token
	hasInit := false
	if p1 != nil && p2 != nil {
		// Extract the substring between the positions of the pointers
		initTokens, err := ExtractTokensBetween(p1.Index, p2.Index, re.gir.tokenSlice)
		if err != nil {
			fmt.Println("Error extracting init statement:", err)
			return
		}
		for i := 0; i < len(initTokens); i++ {
			tok := initTokens[i]
			if tok.Type == WhiteSpace {
				initTokens, _ = RemoveTokenAt(initTokens, i)
				i = i - 1
			}
		}
		// Check if there's actual init content (not just empty)
		hasInit = len(initTokens) > 0 && !(len(initTokens) == 1 && initTokens[0].Content == ";")
		for i, tok := range initTokens {
			if tok.Type == Assignment {
				forVars = append(forVars, initTokens[i-1])
				// Extract ALL tokens after the assignment as the start value
				startTokens := initTokens[i+1:]
				// Combine all start tokens into a single string
				startStr := ""
				for _, t := range startTokens {
					if t.Content != ";" {
						startStr += t.Content
					}
				}
				startStr = strings.TrimSpace(startStr)
				if startStr != "" {
					rangeTokens = append(rangeTokens, CreateToken(Identifier, startStr))
				}
				break // Only process first assignment
			}
		}
	}

	p3 := SearchPointerIndexReverse(PreVisitForStmtCond, re.gir.pointerAndIndexVec)
	p4 := SearchPointerIndexReverse(PostVisitForStmtCond, re.gir.pointerAndIndexVec)
	var condTokens []Token
	hasCond := false
	if p3 != nil && p4 != nil {
		// Extract the substring between the positions of the pointers
		var err error
		condTokens, err = ExtractTokensBetween(p3.Index, p4.Index, re.gir.tokenSlice)
		if err != nil {
			fmt.Println("Error extracting condition statement:", err)
			return
		}
		for i := 0; i < len(condTokens); i++ {
			tok := condTokens[i]
			if tok.Type == WhiteSpace {
				condTokens, _ = RemoveTokenAt(condTokens, i)
				i = i - 1
			}
		}
		// Check if there's actual condition content
		hasCond = len(condTokens) > 0 && !(len(condTokens) == 1 && condTokens[0].Content == ";")

		// Look for comparison operators: <, <=, >, >=
		for i, tok := range condTokens {
			if tok.Type == ComparisonOperator {
				var boundTokens []Token
				isLessThan := tok.Content == "<" || tok.Content == "<="

				if isLessThan {
					// i < n or i <= n: extract tokens after operator
					boundTokens = condTokens[i+1:]
					if tok.Content == "<=" {
						re.forLoopInclusive = true
					}
				} else if tok.Content == ">" || tok.Content == ">=" {
					// i > n or i >= n: reversed loop, extract tokens after operator
					boundTokens = condTokens[i+1:]
					re.forLoopReverse = true
					if tok.Content == ">=" {
						re.forLoopInclusive = true
					}
				} else {
					continue // Not a comparison we handle
				}

				// Remove trailing semicolons and whitespace
				for len(boundTokens) > 0 {
					lastTok := boundTokens[len(boundTokens)-1]
					trimmed := strings.TrimSpace(lastTok.Content)
					if trimmed == ";" || trimmed == "" {
						boundTokens = boundTokens[:len(boundTokens)-1]
					} else {
						break
					}
				}
				// Combine all bound tokens into a single token for simplicity
				boundStr := ""
				for _, t := range boundTokens {
					boundStr += t.Content
				}
				boundStr = strings.TrimSpace(boundStr)
				// Count parens and strip only UNMATCHED trailing )
				for strings.HasSuffix(boundStr, ")") {
					openCount := strings.Count(boundStr, "(")
					closeCount := strings.Count(boundStr, ")")
					if closeCount > openCount {
						boundStr = strings.TrimSuffix(boundStr, ")")
						boundStr = strings.TrimSpace(boundStr)
					} else {
						break
					}
				}
				if boundStr != "" {
					rangeTokens = append(rangeTokens, CreateToken(Identifier, boundStr))
				}
				break // Only process first comparison operator
			}
		}
	}

	p6 := SearchPointerIndexReverse(PostVisitForStmtPost, re.gir.pointerAndIndexVec)
	pFor := SearchPointerIndexReverse(PreVisitForStmt, re.gir.pointerAndIndexVec)

	// Case 0: Infinite loop (no init, no cond, no post) → loop
	// Go: for { } → Rust: loop { }
	if pFor != nil && p6 != nil && !hasInit && !hasCond && node.Post == nil {
		// Build new tokens for Rust loop: "loop\n"
		newTokens := []string{"loop\n"}
		re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, pFor.Index, p6.Index, newTokens)
		return
	}

	// Case 1: Condition-only for loop (no init, no post) → while loop
	// Go: for cond { } → Rust: while cond { }
	if pFor != nil && p6 != nil && !hasInit && hasCond && node.Post == nil {
		// Build new tokens for Rust while loop: "while cond\n"
		newTokens := []string{}
		newTokens = append(newTokens, "while ")
		// Remove trailing semicolon from condition tokens
		for _, tok := range condTokens {
			if tok.Content != ";" {
				newTokens = append(newTokens, tok.Content)
			}
		}
		newTokens = append(newTokens, "\n")

		// Rewrite the tokens from PreVisitForStmt to PostVisitForStmtPost
		re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, pFor.Index, p6.Index, newTokens)
		return
	}

	// Case 1.5: For loop with compound condition (contains && or ||) → while loop
	// Go: for i := 0; i < n && someOtherCond; i++ { } → Rust: let mut i = 0; while i < n && someOtherCond { ... i += 1; }
	hasCompoundCond := false
	for _, tok := range condTokens {
		// Check if token contains && or || (may be combined with other chars in some token formats)
		if tok.Content == "&&" || tok.Content == "||" ||
			strings.Contains(tok.Content, "&&") || strings.Contains(tok.Content, "||") {
			hasCompoundCond = true
			break
		}
	}
	if pFor != nil && p6 != nil && hasInit && hasCond && hasCompoundCond && len(forVars) > 0 {
		// Build new tokens: init statement + while loop
		newTokens := []string{}

		// Add init statement: let mut var = value;
		newTokens = append(newTokens, "let mut ")
		newTokens = append(newTokens, forVars[0].Content)
		newTokens = append(newTokens, " = ")
		if len(rangeTokens) > 0 {
			newTokens = append(newTokens, rangeTokens[0].Content)
		} else {
			newTokens = append(newTokens, "0")
		}
		newTokens = append(newTokens, ";\n")

		// Add while loop with condition
		newTokens = append(newTokens, "while ")
		for _, tok := range condTokens {
			if tok.Content != ";" {
				newTokens = append(newTokens, tok.Content)
			}
		}
		newTokens = append(newTokens, " ")

		// Rewrite the tokens from PreVisitForStmt to PostVisitForStmtPost
		re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, pFor.Index, p6.Index, newTokens)
		// Note: The increment is already added by PostVisitBlockStmt using flags set in PreVisitForStmt
		// Fall through to cleanup at the end, but skip Case 2 by clearing rangeTokens
		rangeTokens = nil
	}

	// Case 2: Traditional for loop with init, cond, post and increment/decrement → for in range
	if pFor != nil && p6 != nil && len(forVars) > 0 && len(rangeTokens) >= 2 && (re.sawIncrement || re.sawDecrement) {
		// Build new tokens for Rust for loop
		newTokens := []string{}
		newTokens = append(newTokens, "for ")
		newTokens = append(newTokens, forVars[0].Content)
		newTokens = append(newTokens, " in ")

		// Determine range operator: .. or ..=
		rangeOp := ".."
		if re.forLoopInclusive {
			rangeOp = "..="
		}

		// Build the range expression
		startVal := rangeTokens[0].Content
		endVal := rangeTokens[1].Content

		// Check if we need modifiers (.rev(), .step_by())
		needsRev := re.forLoopReverse
		needsStep := re.forLoopStep > 1

		if needsRev || needsStep {
			// Complex range: need parentheses
			newTokens = append(newTokens, "(")
			if needsRev {
				// For reverse: swap start and end, and adjust for inclusive
				// i > 0 means: (0..start+1).rev() or for i >= 0: (0..=start).rev()
				// Actually for i := n; i > 0; i-- we want (1..=n).rev() to get n, n-1, ..., 1
				// For i := n; i >= 0; i-- we want (0..=n).rev() to get n, n-1, ..., 0
				if re.forLoopInclusive {
					// i >= end: range is (end..=start).rev()
					newTokens = append(newTokens, endVal)
					newTokens = append(newTokens, rangeOp)
					newTokens = append(newTokens, startVal)
				} else {
					// i > end: range is (end+1..=start).rev()
					// We need to add 1 to end for exclusive lower bound
					newTokens = append(newTokens, "(")
					newTokens = append(newTokens, endVal)
					newTokens = append(newTokens, "+1)")
					newTokens = append(newTokens, "..=")
					newTokens = append(newTokens, startVal)
				}
			} else {
				// Forward range
				newTokens = append(newTokens, startVal)
				newTokens = append(newTokens, rangeOp)
				newTokens = append(newTokens, endVal)
			}
			newTokens = append(newTokens, ")")

			// Add modifiers
			if needsRev {
				newTokens = append(newTokens, ".rev()")
			}
			if needsStep {
				newTokens = append(newTokens, fmt.Sprintf(".step_by(%d)", re.forLoopStep))
			}
		} else {
			// Simple range: start..end or start..=end
			newTokens = append(newTokens, startVal)
			newTokens = append(newTokens, rangeOp)
			newTokens = append(newTokens, endVal)
		}

		newTokens = append(newTokens, "\n")

		// Rewrite the tokens from PreVisitForStmt to PostVisitForStmtPost
		re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, pFor.Index, p6.Index, newTokens)
	}

	// Remove used pointer entries to handle nested loops correctly
	// After processing this for loop, its markers should not be found by parent loops
	re.gir.pointerAndIndexVec = RemovePointerEntryReverse(re.gir.pointerAndIndexVec, PreVisitForStmt)
	re.gir.pointerAndIndexVec = RemovePointerEntryReverse(re.gir.pointerAndIndexVec, PreVisitForStmtInit)
	re.gir.pointerAndIndexVec = RemovePointerEntryReverse(re.gir.pointerAndIndexVec, PostVisitForStmtInit)
	re.gir.pointerAndIndexVec = RemovePointerEntryReverse(re.gir.pointerAndIndexVec, PreVisitForStmtCond)
	re.gir.pointerAndIndexVec = RemovePointerEntryReverse(re.gir.pointerAndIndexVec, PostVisitForStmtCond)
	re.gir.pointerAndIndexVec = RemovePointerEntryReverse(re.gir.pointerAndIndexVec, PreVisitForStmtPost)
	re.gir.pointerAndIndexVec = RemovePointerEntryReverse(re.gir.pointerAndIndexVec, PostVisitForStmtPost)
}

func (re *RustEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	re.shouldGenerate = true
	// Check if this is a key-value range (both Key and Value present)
	if node.Key != nil && node.Value != nil {
		re.isKeyValueRange = true
		re.rangeKeyName = node.Key.(*ast.Ident).Name
		re.rangeValueName = node.Value.(*ast.Ident).Name
		re.rangeCollectionExpr = ""
		re.suppressRangeEmit = true
		re.rangeStmtIndent = indent
		// Don't emit anything yet - we'll emit in PostVisitRangeStmtX
	} else {
		re.isKeyValueRange = false
		str := re.emitAsString("for ", indent)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
}

func (re *RustEmitter) PreVisitRangeStmtKey(node ast.Expr, indent int) {
	// For key-value range, we've already captured the key name
}

func (re *RustEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	// Nothing special needed here
}

func (re *RustEmitter) PreVisitRangeStmtValue(node ast.Expr, indent int) {
	// For key-value range, we've already captured the value name
}

func (re *RustEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	if re.isKeyValueRange {
		// Stop suppressing, start capturing collection expression
		re.suppressRangeEmit = false
		re.captureRangeExpr = true
	} else {
		str := re.emitAsString(" in ", 0)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
}

func (re *RustEmitter) PreVisitRangeStmtX(node ast.Expr, indent int) {
	// For key-value range, we're already in capture mode
}

func (re *RustEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	if re.isKeyValueRange {
		// Stop capturing and emit the complete for loop
		re.captureRangeExpr = false
		collection := re.rangeCollectionExpr
		key := re.rangeKeyName
		value := re.rangeValueName
		indent := re.rangeStmtIndent

		// Check if collection is a string
		tv := re.pkg.TypesInfo.Types[node]
		iterMethod := ".clone().iter().enumerate()"
		if tv.Type != nil && tv.Type.String() == "string" {
			iterMethod = ".bytes().enumerate()"
		}

		// Emit: for (key, value) in collection.clone().iter().enumerate()
		str := re.emitAsString(fmt.Sprintf("for (%s, %s) in %s%s\n", key, value, collection, iterMethod), indent)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)

		// Reset range state
		re.isKeyValueRange = false
		re.rangeKeyName = ""
		re.rangeValueName = ""
		re.rangeCollectionExpr = ""
		re.shouldGenerate = false
	} else {
		// Check the type of the expression being ranged over
		tv := re.pkg.TypesInfo.Types[node]
		if tv.Type != nil {
			typeStr := tv.Type.String()
			if typeStr == "string" {
				// String needs .bytes() to iterate and get i8 values
				re.gir.emitToFileBuffer(".bytes()", EmptyVisitMethod)
			} else {
				// Add .clone() to the collection to avoid ownership transfer
				re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
			}
		} else {
			re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
		}
		str := re.emitAsString("\n", 0)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
		re.shouldGenerate = false
	}
}

func (re *RustEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	// Reset any range-related state
}

func (re *RustEmitter) PreVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	re.shouldGenerate = true
	// Track if we see ++ or -- for for loop rewriting
	if node.Tok.String() == "++" {
		re.sawIncrement = true
		re.forLoopStep = 1
	} else if node.Tok.String() == "--" {
		re.sawDecrement = true
		re.forLoopReverse = true
		re.forLoopStep = 1
	}
}

func (re *RustEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	content := node.Tok.String()
	// Rust doesn't support ++ or --, convert to += 1 or -= 1
	if content == "++" {
		re.gir.emitToFileBuffer(" += 1", EmptyVisitMethod)
	} else if content == "--" {
		re.gir.emitToFileBuffer(" -= 1", EmptyVisitMethod)
	}
	if !re.insideForPostCond {
		re.emitToken(";", Semicolon, 0)
	}
	re.shouldGenerate = false
}

func (re *RustEmitter) PreVisitCompositeLit(node *ast.CompositeLit, indent int) {
	// Save current isArray state for nested composite literals
	re.isArrayStack = append(re.isArrayStack, re.isArray)
	// Reset for this composite literal
	re.isArray = false
	re.currentCompLitIsSlice = false

	// Push the type to the stack so we can check it in PostVisitCompositeLitElts
	var compLitType types.Type
	if node.Type != nil {
		typeInfo := re.pkg.TypesInfo.Types[node.Type]
		if typeInfo.Type != nil {
			compLitType = typeInfo.Type
			// Check if the underlying type is a slice (for type aliases like AST = []Statement)
			if underlying := compLitType.Underlying(); underlying != nil {
				if _, ok := underlying.(*types.Slice); ok {
					// Only use Vec::new() for empty slice literals
					// For non-empty, set isArray so vec![] syntax is used
					if len(node.Elts) == 0 {
						re.currentCompLitIsSlice = true
					} else {
						re.isArray = true
					}
				}
			}
		}
	}
	re.compLitTypeStack = append(re.compLitTypeStack, compLitType)
}

// packageScopeHasInterfaceTypes checks if any struct in the package has interface{} fields
func (re *RustEmitter) packageScopeHasInterfaceTypes(pkg *types.Package) bool {
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if typeName, ok := obj.(*types.TypeName); ok {
			if re.typeHasInterfaceFields(typeName.Type()) {
				return true
			}
		}
	}
	return false
}

func (re *RustEmitter) PreVisitCompositeLitType(node ast.Expr, indent int) {
	re.gir.emitToFileBuffer("", "@PreVisitCompositeLitType")
}

func (re *RustEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	pointerAndPosition := SearchPointerIndexReverse("@PreVisitCompositeLitType", re.gir.pointerAndIndexVec)
	if pointerAndPosition != nil {
		// For slice type aliases (like AST = []Statement), replace with Vec::new()
		// The braces will be suppressed in PreVisitCompositeLitElts/PostVisitCompositeLitElts
		if re.currentCompLitIsSlice {
			if re.inKeyValueExpr || re.inFieldAssign || re.inReturnStmt {
				// Inside struct field initialization, field assignment, or return statement
				re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, pointerAndPosition.Index, len(re.gir.tokenSlice), []string{"Vec::new()"})
			} else {
				// Variable declaration: let x = []Type{} -> let x: Vec<type> = Vec::new()
				// Extract the type tokens for the type annotation
				vecTypeStrRepr, _ := ExtractTokensBetween(pointerAndPosition.Index, len(re.gir.tokenSlice), re.gir.tokenSlice)
				newTokens := []string{}
				newTokens = append(newTokens, ":")
				newTokens = append(newTokens, tokensToStrings(vecTypeStrRepr)...)
				newTokens = append(newTokens, " = Vec::new()")
				re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, pointerAndPosition.Index-len("=")-len(" "), len(re.gir.tokenSlice), newTokens)
			}
			return
		}
		// TODO not very effective
		// go through all aliases and check if the underlying type matches
		// Only do alias replacement if the type is NOT already a named type (alias)
		typeInfo := re.pkg.TypesInfo.Types[node]
		if typeInfo.Type != nil {
			if _, isNamed := typeInfo.Type.(*types.Named); !isNamed {
				// Type is a basic/primitive type - check for alias replacement
				for aliasName, alias := range re.aliases {
					if alias.UnderlyingType == typeInfo.Type.Underlying().String() {
						re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, pointerAndPosition.Index, len(re.gir.tokenSlice), []string{aliasName})
						break
					}
				}
			}
		}
		if re.isArray {
			// TODO that's still hack
			// we operate on string representation of the type
			// has to be rewritten to use some kind of IR
			if re.inKeyValueExpr || re.inFieldAssign || re.inReturnStmt {
				// Inside struct field initialization, field assignment, or return statement: []Type{} -> vec![]
				// Just replace the type with vec!, keeping context intact
				newTokens := []string{"vec!"}
				re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, pointerAndPosition.Index, len(re.gir.tokenSlice), newTokens)
			} else {
				// Variable declaration: let x = []Type{} -> let x: Vec<type> = vec![]
				vecTypeStrRepr, _ := ExtractTokensBetween(pointerAndPosition.Index, len(re.gir.tokenSlice), re.gir.tokenSlice)
				newTokens := []string{}
				newTokens = append(newTokens, ":")
				newTokens = append(newTokens, tokensToStrings(vecTypeStrRepr)...)
				newTokens = append(newTokens, " = vec!")
				re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, pointerAndPosition.Index-len("=")-len(" "), len(re.gir.tokenSlice), newTokens)
			}
		}
	}
}

func (re *RustEmitter) PreVisitCompositeLitElts(node []ast.Expr, indent int) {
	// Skip braces for slice type aliases - Vec::new() is already emitted
	if re.currentCompLitIsSlice {
		return
	}
	re.emitToken("{", LeftBrace, 0)
}

func (re *RustEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	// For struct initialization in Rust, add ..Default::default() to handle missing fields
	// But NOT for arrays/vectors - those just use {}
	// Don't use Default::default() if the type doesn't derive Default (due to interface{}/func fields)

	// Get the current composite literal's type from the stack
	var currentType types.Type
	if len(re.compLitTypeStack) > 0 {
		currentType = re.compLitTypeStack[len(re.compLitTypeStack)-1]
	}

	// Check if this specific type has interface/function fields that prevent Default
	typeHasNoDefault := currentType != nil && re.typeHasInterfaceFields(currentType)

	// Check if the underlying type is a slice (for type aliases like AST = []Statement)
	isSliceType := false
	if currentType != nil {
		underlying := currentType.Underlying()
		if _, ok := underlying.(*types.Slice); ok {
			isSliceType = true
		}
	}

	// Skip braces and default for slice type aliases - Vec::new() is already emitted
	if re.currentCompLitIsSlice {
		re.currentCompLitIsSlice = false
	} else {
		if !re.isArray && !isSliceType && !typeHasNoDefault {
			if len(node) > 0 {
				// Partial struct init - add comma before Default
				re.gir.emitToFileBuffer(", ..Default::default()", EmptyVisitMethod)
			} else {
				// Empty struct init
				re.gir.emitToFileBuffer("..Default::default()", EmptyVisitMethod)
			}
		}
		re.emitToken("}", RightBrace, 0)
	}

	// Restore isArray from stack for nested composite literals
	if len(re.isArrayStack) > 0 {
		re.isArray = re.isArrayStack[len(re.isArrayStack)-1]
		re.isArrayStack = re.isArrayStack[:len(re.isArrayStack)-1]
	}
	// Pop from compLitTypeStack
	if len(re.compLitTypeStack) > 0 {
		re.compLitTypeStack = re.compLitTypeStack[:len(re.compLitTypeStack)-1]
	}
}

func (re *RustEmitter) PreVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	if index > 0 {
		str := re.emitAsString(", ", 0)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
}

func (re *RustEmitter) PreVisitSliceExpr(node *ast.SliceExpr, indent int) {
	// Don't add & - we'll add .to_vec() at the end to get Vec back
}

func (re *RustEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	re.emitToken("[", LeftBracket, 0)
	re.shouldGenerate = false
}

func (re *RustEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	re.emitToken("]", RightBracket, 0)
	// Convert slice to Vec to match Go semantics
	re.gir.emitToFileBuffer(".to_vec()", EmptyVisitMethod)
	re.shouldGenerate = true
}

func (re *RustEmitter) PreVisitSliceExprLow(node ast.Expr, indent int) {
	// Re-enable generation for the low index expression
	re.shouldGenerate = true
}

func (re *RustEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	// Cast to usize for slice indexing (Rust requires usize for slice indices)
	if node != nil {
		re.gir.emitToFileBuffer(" as usize", EmptyVisitMethod)
	}
	re.gir.emitToFileBuffer("..", EmptyVisitMethod)
	re.shouldGenerate = false
}

func (re *RustEmitter) PreVisitSliceExprHigh(node ast.Expr, indent int) {
	// Re-enable generation for the high index expression
	re.shouldGenerate = true
}

func (re *RustEmitter) PostVisitSliceExprHigh(node ast.Expr, indent int) {
	// Cast to usize for slice indexing (Rust requires usize for slice indices)
	if node != nil {
		re.gir.emitToFileBuffer(" as usize", EmptyVisitMethod)
	}
	re.shouldGenerate = false
}

func (re *RustEmitter) PreVisitFuncLit(node *ast.FuncLit, indent int) {
	re.funcLitDepth++
	// For local closure inlining, skip wrapper emission
	if re.localClosureAssign && re.currentClosureName != "" {
		return
	}
	// Check if this closure returns any (interface{})
	re.currentFuncReturnsAny = false
	if node.Type != nil && node.Type.Results != nil {
		for _, result := range node.Type.Results.List {
			if result.Type != nil {
				resultType := re.pkg.TypesInfo.Types[result.Type]
				if resultType.Type != nil {
					typeStr := resultType.Type.String()
					if typeStr == "interface{}" || typeStr == "any" {
						re.currentFuncReturnsAny = true
						break
					}
				}
			}
		}
	}
	// Only wrap closure with Rc::new() when NOT passed as a direct call argument
	// Functions that take closure arguments expect FnMut, not Rc<dyn Fn>
	if !re.inCallExprArg {
		wrapperStr := "Rc::new("
		str := re.emitAsString(wrapperStr, 0)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
		re.closureWrappedInRc = true
	} else {
		re.closureWrappedInRc = false
	}
	re.emitToken("|", Identifier, indent)
}
func (re *RustEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	re.funcLitDepth--
	// For local closures being inlined, extract and store body tokens, skip wrapper
	if re.inLocalClosureBody && re.currentClosureName != "" {
		// Extract body tokens (from after { to current position)
		bodyEndIndex := len(re.gir.tokenSlice)
		if re.localClosureBodyStartIndex < bodyEndIndex {
			bodyTokens := make([]Token, bodyEndIndex-re.localClosureBodyStartIndex)
			copy(bodyTokens, re.gir.tokenSlice[re.localClosureBodyStartIndex:bodyEndIndex])
			re.localClosureBodyTokens[re.currentClosureName] = bodyTokens
		}
		// Clear the flag
		re.inLocalClosureBody = false
		// Don't emit the closing braces - the entire assignment will be truncated
		return
	}
	re.emitToken("}", RightBrace, 0)
	// Close the Rc::new() wrapper only if it was opened
	if re.closureWrappedInRc {
		re.emitToken(")", RightParen, 0)
	}
	re.currentFuncReturnsAny = false
	re.closureWrappedInRc = false
}

func (re *RustEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	// For local closure inlining, skip wrapper emission
	if re.localClosureAssign && re.currentClosureName != "" {
		return
	}
	re.emitToken("|", Identifier, 0)
}

func (re *RustEmitter) PreVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	str := ""
	if index > 0 {
		str += re.emitAsString(", ", 0)
	}
	// Emit name first, then colon, then type will follow
	if len(node.Names) > 0 {
		// Escape Rust keywords in parameter names
		paramName := escapeRustKeyword(node.Names[0].Name)
		str += re.emitAsString(paramName+": ", 0)
	}
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	// Type has already been emitted, nothing to do
}

func (re *RustEmitter) PreVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	// For local closures being inlined, skip wrapper emission but record body start
	if re.localClosureAssign && re.currentClosureName != "" {
		re.localClosureBodyStartIndex = len(re.gir.tokenSlice)
		re.inLocalClosureBody = true // Track that we're inside the closure body
		return
	}
	re.emitToken("{", LeftBrace, 0)
	str := re.emitAsString("\n", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitFuncLitTypeResults(node *ast.FieldList, indent int) {
	re.shouldGenerate = false
}

func (re *RustEmitter) PreVisitInterfaceType(node *ast.InterfaceType, indent int) {
	str := re.emitAsString("Box<dyn Any>", indent)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PostVisitInterfaceType(node *ast.InterfaceType, indent int) {
}

func (re *RustEmitter) PreVisitKeyValueExprValue(node ast.Expr, indent int) {
	// In Rust struct initialization, use `:` not `=`
	str := re.emitAsString(": ", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PostVisitKeyValueExprValue(node ast.Expr, indent int) {
	// Add .clone() for non-Copy types in struct field assignments
	// This is needed because Rust closures that move values become FnOnce, not Fn
	// For slices (Vec in Rust) and strings, we need to clone to avoid moving the captured variable
	if node != nil {
		tv := re.pkg.TypesInfo.Types[node]
		if tv.Type != nil {
			typeStr := tv.Type.String()
			// Check if it's a slice type (will become Vec in Rust) or string type
			if strings.HasPrefix(typeStr, "[]") || typeStr == "string" {
				re.gir.emitToFileBuffer(".clone()", EmptyVisitMethod)
			}
		}
	}
}

func (re *RustEmitter) PreVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	re.emitToken("(", LeftParen, 0)
	str := re.emitAsString(node.Op.String(), 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}
func (re *RustEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	re.emitToken(")", RightParen, 0)
}

func (re *RustEmitter) PreVisitGenDeclConstName(node *ast.Ident, indent int) {
	// TODO dummy implementation
	// not very well performed
	for constIdent, obj := range re.pkg.TypesInfo.Defs {
		if obj == nil {
			continue
		}
		if con, ok := obj.(*types.Const); ok {
			if constIdent.Name != node.Name {
				continue
			}
			constType := con.Type().String()
			constType = strings.TrimPrefix(constType, "untyped ")
			if constType == re.pkg.TypesInfo.Defs[node].Type().String() {
				constType = trimBeforeChar(constType, '.')
			}

			// Map Go types to Rust types for constants
			// Keep untyped int as i32 since Go's implicit type conversion at usage
			// sites will be handled by explicit casts in binary expressions
			rustType := re.mapGoTypeToRust(constType)
			str := re.emitAsString(fmt.Sprintf("pub const %s: %s = ", node.Name, rustType), 0)

			re.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	}
}
func (re *RustEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	str := re.emitAsString(";\n", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}
func (re *RustEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {
	str := re.emitAsString("\n", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	re.shouldGenerate = true
	str := re.emitAsString("match ", indent)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}
func (re *RustEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	// Check if the switch has a default case
	hasDefault := false
	if node.Body != nil {
		for _, stmt := range node.Body.List {
			if caseClause, ok := stmt.(*ast.CaseClause); ok {
				if len(caseClause.List) == 0 {
					hasDefault = true
					break
				}
			}
		}
	}
	// If no default case, add one for Rust match exhaustiveness
	if !hasDefault {
		str := re.emitAsString("_ => {}\n", indent+2)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
	re.emitToken("}", RightBrace, indent)
}

func (re *RustEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	// Check if we need to cast to i32 to match constants (which are i32 by default)
	if node != nil {
		tv := re.pkg.TypesInfo.Types[node]
		if tv.Type != nil {
			typeStr := tv.Type.String()
			// Cast smaller integer types to i32 so they match constant types
			if typeStr == "int8" || typeStr == "uint8" ||
				typeStr == "int16" || typeStr == "uint16" {
				str := re.emitAsString(" as i32", 0)
				re.gir.emitToFileBuffer(str, EmptyVisitMethod)
			}
		}
	}
	// Rust match doesn't use parentheses around the tag
	str := re.emitAsString(" ", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	re.emitToken("{", LeftBrace, 0)
	str = re.emitAsString("\n", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	// In Rust match, close the block for this arm
	str := re.emitAsString("}\n", indent+2)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	if len(node) == 0 {
		// Rust match uses _ for default case
		str := re.emitAsString("_ => {\n", indent+2)
		re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	}
}

func (re *RustEmitter) PreVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	// Rust match arms don't need "case" keyword - just the pattern
	str := re.emitAsString("", indent+2)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
	re.shouldGenerate = true
}

func (re *RustEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	// Rust match uses => and block
	str := re.emitAsString(" => {\n", 0)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	re.gir.emitToFileBuffer("", "@PreVisitTypeAssertExpr")
}

func (re *RustEmitter) PreVisitTypeAssertExprType(node ast.Expr, indent int) {
	re.gir.emitToFileBuffer("", "@PreVisitTypeAssertExprType")
}

func (re *RustEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	re.gir.emitToFileBuffer("", "@PostVisitTypeAssertExprType")
}

func (re *RustEmitter) PreVisitTypeAssertExprX(node ast.Expr, indent int) {
	re.gir.emitToFileBuffer("", "@PreVisitTypeAssertExprX")
}

func (re *RustEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	re.gir.emitToFileBuffer("", "@PostVisitTypeAssertExprX")
}

func (re *RustEmitter) PostVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	// Reorder type assertion from (Type)X to X.downcast_ref::<Type>().unwrap().clone()
	p1 := SearchPointerIndexReverseString("@PreVisitTypeAssertExprType", re.gir.pointerAndIndexVec)
	p2 := SearchPointerIndexReverseString("@PostVisitTypeAssertExprType", re.gir.pointerAndIndexVec)
	p3 := SearchPointerIndexReverseString("@PreVisitTypeAssertExprX", re.gir.pointerAndIndexVec)
	p4 := SearchPointerIndexReverseString("@PostVisitTypeAssertExprX", re.gir.pointerAndIndexVec)
	p0 := SearchPointerIndexReverseString("@PreVisitTypeAssertExpr", re.gir.pointerAndIndexVec)

	if p0 != nil && p1 != nil && p2 != nil && p3 != nil && p4 != nil {
		typeTokens, err := ExtractTokensBetween(p1.Index, p2.Index, re.gir.tokenSlice)
		if err != nil {
			return
		}
		exprTokens, err := ExtractTokensBetween(p3.Index, p4.Index, re.gir.tokenSlice)
		if err != nil {
			return
		}
		typeStr := strings.TrimSpace(strings.Join(tokensToStrings(typeTokens), ""))
		exprStr := strings.TrimSpace(strings.Join(tokensToStrings(exprTokens), ""))

		// Generate Rust downcast syntax: X.downcast_ref::<Type>().unwrap().clone()
		newTokens := []string{exprStr, ".downcast_ref::<", typeStr, ">().unwrap().clone()"}
		re.gir.tokenSlice, _ = RewriteTokensBetween(re.gir.tokenSlice, p0.Index, p4.Index, newTokens)
	}
}

func (re *RustEmitter) PreVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	re.shouldGenerate = true
	re.inKeyValueExpr = true
}

func (re *RustEmitter) PostVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	re.inKeyValueExpr = false
	// Add type cast if needed for struct field initialization
	// This handles untyped int constants assigned to int8 fields
	if node.Value == nil {
		return
	}

	// Get the key (field name)
	keyIdent, ok := node.Key.(*ast.Ident)
	if !ok {
		return
	}
	fieldName := keyIdent.Name

	// Get the expected field type from the struct
	// We need to find the struct type and look up the field
	fieldType := re.getFieldTypeForKeyValue(node, fieldName)
	if fieldType == "" {
		return
	}

	// Determine if we need to cast based on field type and value type
	var valueType string
	if valueIdent, ok := node.Value.(*ast.Ident); ok {
		obj := re.pkg.TypesInfo.Uses[valueIdent]
		if obj != nil {
			valueType = obj.Type().String()
		} else if len(valueIdent.Name) > 0 && valueIdent.Name[0] >= 'A' && valueIdent.Name[0] <= 'Z' {
			// Assume uppercase identifiers are constants (untyped int)
			valueType = "untyped int"
		}
	} else if selExpr, ok := node.Value.(*ast.SelectorExpr); ok {
		obj := re.pkg.TypesInfo.Uses[selExpr.Sel]
		if obj != nil {
			if _, isConst := obj.(*types.Const); isConst {
				valueType = obj.Type().String()
			}
		}
	}

	// Add cast if assigning int to a smaller integer type
	if valueType == "int" || valueType == "untyped int" || strings.HasSuffix(valueType, ".int") {
		switch fieldType {
		case "int8":
			re.gir.emitToFileBuffer(" as i8", EmptyVisitMethod)
		case "int16":
			re.gir.emitToFileBuffer(" as i16", EmptyVisitMethod)
		case "uint8":
			re.gir.emitToFileBuffer(" as u8", EmptyVisitMethod)
		case "uint16":
			re.gir.emitToFileBuffer(" as u16", EmptyVisitMethod)
		}
	}
}

// getFieldTypeForKeyValue looks up the struct field type for a KeyValueExpr
func (re *RustEmitter) getFieldTypeForKeyValue(node *ast.KeyValueExpr, fieldName string) string {
	// Try to get the type from TypesInfo
	if re.pkg.TypesInfo == nil {
		return ""
	}

	// Find the parent composite literal to get the struct type
	// We use the current composite literal type from the stack if available
	if len(re.compLitTypeStack) > 0 {
		compLitType := re.compLitTypeStack[len(re.compLitTypeStack)-1]
		if compLitType != nil {
			// Get the underlying type (in case it's a named type)
			underlying := compLitType.Underlying()
			if structType, ok := underlying.(*types.Struct); ok {
				// Look up the field by name
				for i := 0; i < structType.NumFields(); i++ {
					field := structType.Field(i)
					if field.Name() == fieldName {
						return field.Type().String()
					}
				}
			}
		}
	}
	return ""
}

func (re *RustEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	str := re.emitAsString(node.Tok.String()+";", indent)
	re.gir.emitToFileBuffer(str, EmptyVisitMethod)
}

func (re *RustEmitter) PreVisitCallExprFun(node ast.Expr, indent int) {
	// Check if this is a selector expression (obj.field) where the field is a function type
	// In Rust, calling a function stored in a struct field requires: (obj.field)(args)
	if sel, ok := node.(*ast.SelectorExpr); ok {
		// Get the type of the selector (the field)
		if tv := re.pkg.TypesInfo.Selections[sel]; tv != nil {
			// Check if the field type is a function type (Signature)
			if _, isSig := tv.Type().Underlying().(*types.Signature); isSig {
				re.gir.emitToFileBuffer("(", EmptyVisitMethod)
			}
		}
	}
	// Push the current position to the stack for nested call handling
	re.callExprFunMarkerStack = append(re.callExprFunMarkerStack, len(re.gir.tokenSlice))
	re.gir.emitToFileBuffer("", "@PreVisitCallExprFun")
}

func (re *RustEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	// Push the current position to the stack for nested call handling (end of function name)
	re.callExprFunEndMarkerStack = append(re.callExprFunEndMarkerStack, len(re.gir.tokenSlice))
	re.gir.emitToFileBuffer("", "@PostVisitCallExprFun")
	// Close the paren if we opened one for function field call
	if sel, ok := node.(*ast.SelectorExpr); ok {
		if tv := re.pkg.TypesInfo.Selections[sel]; tv != nil {
			if _, isSig := tv.Type().Underlying().(*types.Signature); isSig {
				re.gir.emitToFileBuffer(")", EmptyVisitMethod)
			}
		}
	}
}

func (re *RustEmitter) PreVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {
	re.gir.emitToFileBuffer("", "@PreVisitFuncDeclSignatureTypeParamsListType")
}

func (re *RustEmitter) PostVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {
	re.gir.emitToFileBuffer("", "@PostVisitFuncDeclSignatureTypeParamsListType")
}

func (re *RustEmitter) PostVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	re.gir.emitToFileBuffer("", "@PostVisitFuncDeclSignatureTypeParamsArgName")
}

func (re *RustEmitter) PostVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	p1 := SearchPointerIndexReverse("@PreVisitFuncDeclSignatureTypeParamsListType", re.gir.pointerAndIndexVec)
	p2 := SearchPointerIndexReverse("@PostVisitFuncDeclSignatureTypeParamsListType", re.gir.pointerAndIndexVec)
	p3 := SearchPointerIndexReverse("@PreVisitFuncDeclSignatureTypeParamsArgName", re.gir.pointerAndIndexVec)
	p4 := SearchPointerIndexReverse("@PostVisitFuncDeclSignatureTypeParamsArgName", re.gir.pointerAndIndexVec)

	if p1 != nil && p2 != nil && p3 != nil && p4 != nil {
		typeStrRepr, err := ExtractTokensBetween(p1.Index, p2.Index, re.gir.tokenSlice)
		if err != nil {
			fmt.Println("Error extracting type representation:", err)
			return
		}
		nameStrRepr, err := ExtractTokensBetween(p3.Index, p4.Index, re.gir.tokenSlice)
		if err != nil {
			fmt.Println("Error extracting name representation:", err)
			return
		}
		// Only check name for whitespace - types can have spaces (e.g., Box<dyn Any>)
		nameStr := strings.TrimSpace(strings.Join(tokensToStrings(nameStrRepr), ""))
		if nameStr == "" || containsWhitespace(nameStr) {
			// If name is empty or has whitespace, skip reordering
			return
		}
		newTokens := []string{}
		// Add mut for non-primitive types (struct parameters are often modified in Go)
		// OR if the parameter is reassigned in the function body
		typeStr := strings.TrimSpace(strings.Join(tokensToStrings(typeStrRepr), ""))
		isPrimitive := typeStr == "i8" || typeStr == "i16" || typeStr == "i32" || typeStr == "i64" ||
			typeStr == "u8" || typeStr == "u16" || typeStr == "u32" || typeStr == "u64" ||
			typeStr == "bool" || typeStr == "f32" || typeStr == "f64" || typeStr == "String" ||
			typeStr == "&str" || typeStr == "usize" || typeStr == "isize"
		isMutated := re.mutatedParams != nil && re.mutatedParams[nameStr]
		if !isPrimitive || isMutated {
			newTokens = append(newTokens, "mut ")
		}
		newTokens = append(newTokens, nameStr)
		newTokens = append(newTokens, ": ")
		newTokens = append(newTokens, typeStr)
		re.gir.tokenSlice, err = RewriteTokensBetween(re.gir.tokenSlice, p1.Index, p4.Index, newTokens)
		if err != nil {
			fmt.Println("Error rewriting file buffer:", err)
			return
		}
	}
}

// GenerateCargoToml creates a Cargo.toml for building the Rust project
func (re *RustEmitter) GenerateCargoToml() error {
	if re.LinkRuntime == "" {
		return nil
	}

	cargoPath := filepath.Join(re.OutputDir, "Cargo.toml")
	file, err := os.Create(cargoPath)
	if err != nil {
		return fmt.Errorf("failed to create Cargo.toml: %w", err)
	}
	defer file.Close()

	// Determine graphics backend
	graphicsBackend := re.GraphicsRuntime
	if graphicsBackend == "" {
		graphicsBackend = "tigr" // Default to tigr for Rust
	}

	var cargoToml string
	switch graphicsBackend {
	case "none":
		// No graphics dependencies
		cargoToml = fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[dependencies]

[profile.release]
opt-level = 3
lto = true
codegen-units = 1
`, re.OutputName)
	case "tigr":
		// tigr graphics - uses cc build dependency to compile tigr.c
		cargoToml = fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[dependencies]

[build-dependencies]
cc = "1.0"

[profile.release]
opt-level = 3
lto = true
codegen-units = 1
`, re.OutputName)
	default:
		// SDL2 graphics
		cargoToml = fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[dependencies]
sdl2 = "0.36"

[profile.release]
opt-level = 3
lto = true
codegen-units = 1
`, re.OutputName)
	}

	_, err = file.WriteString(cargoToml)
	if err != nil {
		return fmt.Errorf("failed to write Cargo.toml: %w", err)
	}

	DebugLogPrintf("Generated Cargo.toml at %s (graphics: %s)", cargoPath, graphicsBackend)
	return nil
}

// GenerateGraphicsMod creates the graphics.rs module file by copying from runtime
func (re *RustEmitter) GenerateGraphicsMod() error {
	if re.LinkRuntime == "" {
		return nil
	}

	// Create src directory if needed (Cargo convention)
	srcDir := filepath.Join(re.OutputDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return fmt.Errorf("failed to create src directory: %w", err)
	}

	// Determine graphics backend
	graphicsBackend := re.GraphicsRuntime
	if graphicsBackend == "" {
		graphicsBackend = "tigr"
	}

	// Select the appropriate runtime file based on backend
	var runtimeFileName string
	switch graphicsBackend {
	case "tigr":
		runtimeFileName = "graphics_runtime_tigr.rs"
	case "sdl2":
		runtimeFileName = "graphics_runtime_sdl2.rs"
	default:
		// For "none" or unknown, use SDL2 runtime as fallback
		runtimeFileName = "graphics_runtime_sdl2.rs"
	}

	// Source path: LinkRuntime points to runtime directory, graphics runtime is in graphics/rust/
	runtimeSrcPath := filepath.Join(re.LinkRuntime, "graphics", "rust", runtimeFileName)
	graphicsRs, err := os.ReadFile(runtimeSrcPath)
	if err != nil {
		return fmt.Errorf("failed to read graphics runtime from %s: %w", runtimeSrcPath, err)
	}

	// Destination path
	graphicsPath := filepath.Join(srcDir, "graphics.rs")
	if err := os.WriteFile(graphicsPath, graphicsRs, 0644); err != nil {
		return fmt.Errorf("failed to write graphics.rs: %w", err)
	}

	DebugLogPrintf("Copied graphics.rs from %s to %s", runtimeSrcPath, graphicsPath)

	// For tigr backend, copy tigr.c and tigr.h (tigr.c includes tigr.h)
	if graphicsBackend == "tigr" {
		// Copy tigr.c
		tigrCSrc := filepath.Join(re.LinkRuntime, "graphics", "cpp", "tigr.c")
		tigrCDst := filepath.Join(srcDir, "tigr.c")
		tigrCContent, err := os.ReadFile(tigrCSrc)
		if err != nil {
			return fmt.Errorf("failed to read tigr.c from %s: %w", tigrCSrc, err)
		}
		if err := os.WriteFile(tigrCDst, tigrCContent, 0644); err != nil {
			return fmt.Errorf("failed to write tigr.c: %w", err)
		}
		DebugLogPrintf("Copied tigr.c to %s", tigrCDst)

		// Copy tigr.h (required by tigr.c)
		tigrHSrc := filepath.Join(re.LinkRuntime, "graphics", "cpp", "tigr.h")
		tigrHDst := filepath.Join(srcDir, "tigr.h")
		tigrHContent, err := os.ReadFile(tigrHSrc)
		if err != nil {
			return fmt.Errorf("failed to read tigr.h from %s: %w", tigrHSrc, err)
		}
		if err := os.WriteFile(tigrHDst, tigrHContent, 0644); err != nil {
			return fmt.Errorf("failed to write tigr.h: %w", err)
		}
		DebugLogPrintf("Copied tigr.h to %s", tigrHDst)
	}

	return nil
}

// GenerateBuildRs creates a build.rs file for compiling native code
func (re *RustEmitter) GenerateBuildRs() error {
	if re.LinkRuntime == "" {
		return nil
	}

	buildRsPath := filepath.Join(re.OutputDir, "build.rs")
	file, err := os.Create(buildRsPath)
	if err != nil {
		return fmt.Errorf("failed to create build.rs: %w", err)
	}
	defer file.Close()

	// Determine graphics backend
	graphicsBackend := re.GraphicsRuntime
	if graphicsBackend == "" {
		graphicsBackend = "tigr"
	}

	var buildRs string
	if graphicsBackend == "tigr" {
		// tigr: compile tigr.c and link platform libraries
		buildRs = `fn main() {
    // Compile tigr.c
    cc::Build::new()
        .file("src/tigr.c")
        .compile("tigr");

    // Link platform-specific libraries
    #[cfg(target_os = "macos")]
    {
        println!("cargo:rustc-link-lib=framework=OpenGL");
        println!("cargo:rustc-link-lib=framework=Cocoa");
    }

    #[cfg(target_os = "linux")]
    {
        println!("cargo:rustc-link-lib=GL");
        println!("cargo:rustc-link-lib=X11");
    }

    #[cfg(target_os = "windows")]
    {
        println!("cargo:rustc-link-lib=opengl32");
        println!("cargo:rustc-link-lib=gdi32");
    }
}
`
	} else {
		// SDL2: just add library search paths
		buildRs = `fn main() {
    // Add Homebrew library path for macOS
    #[cfg(target_os = "macos")]
    {
        // Apple Silicon Macs
        println!("cargo:rustc-link-search=/opt/homebrew/lib");
        // Intel Macs
        println!("cargo:rustc-link-search=/usr/local/lib");
    }

    // Add common Linux library paths
    #[cfg(target_os = "linux")]
    {
        println!("cargo:rustc-link-search=/usr/lib");
        println!("cargo:rustc-link-search=/usr/local/lib");
    }
}
`
	}

	_, err = file.WriteString(buildRs)
	if err != nil {
		return fmt.Errorf("failed to write build.rs: %w", err)
	}

	DebugLogPrintf("Generated build.rs at %s (graphics: %s)", buildRsPath, graphicsBackend)
	return nil
}
