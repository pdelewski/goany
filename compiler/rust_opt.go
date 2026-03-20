package compiler

import (
	"fmt"
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// moveOptCallExt tracks a function call arg that needs to be extracted
// into a temp variable before the assignment to avoid borrow conflicts.
// Pattern: c = doADC(c, ReadIndirectX(c, zp))
// Becomes: let __mv0: u8 = ReadIndirectX(&c, zp); c = doADC(c, __mv0);
type moveOptCallExt struct {
	argIdx   int    // which argument index in the outer call
	tempName string // temp variable name (e.g., "__mv0")
	rustType string // Rust type of the result (e.g., "u8")
	code     string // captured Rust code for the function call (filled during PostVisitCallExprArg)
	node     IRNode // captured IR tree node (preserves OptCallArg metadata for ref-opt pass)
}

// RustOptState holds all optimization-related state, separated from the core emitter.
// Methods on this struct implement move optimization, reference optimization, and
// related analysis. The emitter embeds this as Opt and wires up the pkg and
// goTypeToRust dependencies during visitor traversal.
type RustOptState struct {
	// Configuration (set by CLI via struct literal)
	OptimizeMoves bool
	OptimizeRefs  bool
	MoveOptCount  int
	RefOptPass    *RefOptPass

	// Dependencies (set by emitter during traversal)
	pkg          *packages.Package
	goTypeToRust func(string) string

	// Ref optimization
	refOptReadOnly    *ReadOnlyAnalysis
	refOptCurrentFunc string
	refOptCurrentPkg  string
	// Move optimization
	currentAssignLhsNames     map[string]bool
	funcLitDepth              int
	currentCallArgIdentsStack []map[string]int
	currentCalleeName         string
	currentCalleeKey          string
	currentParamIndex         int
	currentCallIsLen          bool
	// Callee info save/restore stacks for nested calls
	calleeNameStack []string
	calleeKeyStack  []string
	callIsLenStack  []bool

	// Move extraction (temp binding) state
	moveOptActive          bool
	moveOptTempBindings    []string
	moveOptArgReplacements map[int]string
	moveOptModifiedCounts  map[string]int
	moveOptReplacingArg    bool
	moveOptArgStartIdx     int

	// Call expression extraction: capture function call args that reference a moved struct
	// Pattern: c = func(c, ReadIndirectX(c, zp)) → let __mv0 = ReadIndirectX(&c, zp); c = func(c, __mv0);
	moveOptCallExts []moveOptCallExt

	// Return temp extraction: extract later return results into temps so first result can be moved
	// Pattern: return c, c.Memory[addr] → let __mv0: u8 = c.Memory[addr]; return (c, __mv0);
	returnTempReplacements map[int]string
	returnTempPreamble     []IRNode // temp binding tokens to emit before return statement

	// argAlreadyCloned: when a vec element access already added .clone(), skip redundant clone
	argAlreadyCloned bool

	// std::mem::take state
	memTakeActive      bool
	memTakeLhsExpr     string
	memTakeArgIdx      int
	memTakeArgStartIdx int
}

// SetPkg sets the current package for type info lookups.
func (o *RustOptState) SetPkg(pkg *packages.Package) {
	o.pkg = pkg
}

// AccumulateReadOnlyAnalysis runs read-only parameter analysis for the given
// package and merges results into the accumulated analysis map.
func (o *RustOptState) AccumulateReadOnlyAnalysis(pkg *packages.Package) {
	pkgAnalysis := AnalyzeReadOnlyParams(pkg)
	if o.refOptReadOnly == nil {
		o.refOptReadOnly = pkgAnalysis
	} else {
		for k, v := range pkgAnalysis.ReadOnly {
			o.refOptReadOnly.ReadOnly[k] = v
		}
		for k, v := range pkgAnalysis.MutRef {
			o.refOptReadOnly.MutRef[k] = v
		}
		for k, v := range pkgAnalysis.FuncsAsValues {
			o.refOptReadOnly.FuncsAsValues[k] = v
		}
	}
	// Auto-register read-only info for runtime packages (hand-written Rust,
	// not transpiled Go code) by deriving flags from their Go type signatures.
	o.registerRuntimeReadOnly(pkg)
}

// registerRuntimeReadOnly registers read-only parameter flags for runtime/math
// functions. These are hand-written Rust functions that accept &[f64] slices,
// so slice parameters can safely use pass-by-reference optimization.
// Other runtime packages (graphics, net, fs) are excluded because their
// hand-written Rust functions accept owned types (Vec<u8>, Window, etc.).
func (o *RustOptState) registerRuntimeReadOnly(pkg *packages.Package) {
	if o.refOptReadOnly == nil {
		return
	}
	impPkg, ok := pkg.Imports["runtime/math"]
	if !ok || impPkg.Types == nil {
		return
	}
	scope := impPkg.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		fn, ok := obj.(*types.Func)
		if !ok || !fn.Exported() {
			continue
		}
		sig, ok := fn.Type().(*types.Signature)
		if !ok {
			continue
		}
		key := impPkg.Types.Name() + "." + name
		if _, exists := o.refOptReadOnly.ReadOnly[key]; exists {
			continue
		}
		params := sig.Params()
		flags := make([]bool, params.Len())
		for i := 0; i < params.Len(); i++ {
			_, isSlice := params.At(i).Type().Underlying().(*types.Slice)
			flags[i] = isSlice
		}
		o.refOptReadOnly.ReadOnly[key] = flags
	}
}

// canMoveArg checks if a call argument identifier can be moved instead of cloned.
// Returns true when the variable is being reassigned from this call's return value
// and is the only reference to itself across all args of the outermost call.
func (o *RustOptState) canMoveArg(varName string) bool {
	if !o.OptimizeMoves {
		return false
	}
	// Cannot move captured variables inside closures (FnMut)
	if o.funcLitDepth > 0 {
		return false
	}
	if o.currentAssignLhsNames == nil || !o.currentAssignLhsNames[varName] {
		return false
	}
	// When temp extraction is active, use modified counts that exclude extracted fields
	if o.moveOptActive && o.moveOptModifiedCounts != nil {
		if o.moveOptModifiedCounts[varName] <= 1 {
			o.MoveOptCount++
			return true
		}
		return false
	}
	// Check the outermost call's arg ident counts (bottom of stack)
	if len(o.currentCallArgIdentsStack) > 0 {
		outermostCounts := o.currentCallArgIdentsStack[0]
		if outermostCounts[varName] > 1 {
			return false
		}
	}
	o.MoveOptCount++
	return true
}

// refOptFuncKey converts a Rust-style function name to the analysis key format.
func (o *RustOptState) refOptFuncKey(rustFuncName string) string {
	key := strings.ReplaceAll(rustFuncName, "::", ".")
	if !strings.Contains(key, ".") {
		key = o.refOptCurrentPkg + "." + key
	}
	return key
}

// analyzeMoveOptExtraction scans an assignment's RHS call to find field accesses
// on a struct being moved, and creates temp variable bindings to extract them.
// Pattern: c = Func(c, c.Field) → let _v0 = c.Field; c = Func(c, _v0);
func (o *RustOptState) analyzeMoveOptExtraction(node *ast.AssignStmt) {
	o.moveOptTempBindings = nil
	o.moveOptArgReplacements = nil
	o.moveOptModifiedCounts = nil
	o.moveOptActive = false

	if !o.OptimizeMoves {
		return
	}
	if o.funcLitDepth > 0 {
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
		if o.currentAssignLhsNames == nil || !o.currentAssignLhsNames[ident.Name] {
			continue
		}
		if o.pkg == nil || o.pkg.TypesInfo == nil {
			continue
		}
		tv := o.pkg.TypesInfo.Types[arg]
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
	modifiedCounts := CollectCallArgIdentCounts(callExpr.Args, o.pkg)

	var callExts []moveOptCallExt

	for i, arg := range callExpr.Args {
		if i == structArgIdx {
			continue
		}
		if !ExprContainsIdent(arg, structArgName) {
			continue
		}
		tv := o.pkg.TypesInfo.Types[arg]
		if tv.Type == nil || !isCopyType(tv.Type) {
			continue
		}
		basic, isBasic := tv.Type.Underlying().(*types.Basic)
		if !isBasic {
			continue
		}
		tempName := fmt.Sprintf("__mv%d", tempIdx)
		tempIdx++
		rustType := o.goTypeToRust(basic.Name())
		// Try string-based extraction first (for simple expressions like c.A)
		exprStr := o.exprToRustCodeOpt(arg)
		if exprStr != "" {
			binding := fmt.Sprintf("let %s: %s = %s;\n", tempName, rustType, exprStr)
			bindings = append(bindings, binding)
			replacements[i] = tempName
			SubtractIdentsInExpr(arg, o.pkg, modifiedCounts)
			continue
		}
		// For function call expressions that can't be stringified, use deferred
		// code capture. The arg will be emitted normally, then its code is
		// captured in PostVisitCallExprArg and relocated before the assignment.
		if _, isCall := arg.(*ast.CallExpr); isCall {
			callExts = append(callExts, moveOptCallExt{
				argIdx:   i,
				tempName: tempName,
				rustType: rustType,
			})
			SubtractIdentsInExpr(arg, o.pkg, modifiedCounts)
			continue
		}
	}

	if len(replacements) > 0 || len(callExts) > 0 {
		o.moveOptTempBindings = bindings
		o.moveOptArgReplacements = replacements
		o.moveOptModifiedCounts = modifiedCounts
		o.moveOptActive = true
		o.moveOptCallExts = callExts
	}
}

// analyzeMemTakeOpt detects state.Field = func(state.Field, ...) and marks for std::mem::take.
func (o *RustOptState) analyzeMemTakeOpt(node *ast.AssignStmt) {
	o.memTakeActive = false
	o.memTakeLhsExpr = ""
	o.memTakeArgIdx = -1

	if !o.OptimizeMoves {
		return
	}
	if o.funcLitDepth > 0 {
		return
	}
	if len(node.Lhs) != 1 || len(node.Rhs) != 1 {
		return
	}

	lhsSel, ok := node.Lhs[0].(*ast.SelectorExpr)
	if !ok {
		return
	}
	callExpr, ok := node.Rhs[0].(*ast.CallExpr)
	if !ok {
		return
	}

	lhsStr := exprToString(lhsSel)
	if lhsStr == "" {
		return
	}

	if o.pkg == nil || o.pkg.TypesInfo == nil {
		return
	}
	lhsTV := o.pkg.TypesInfo.Types[lhsSel]
	if lhsTV.Type == nil || isCopyType(lhsTV.Type) {
		return
	}
	named, ok := lhsTV.Type.(*types.Named)
	if !ok {
		return
	}
	if _, isStruct := named.Underlying().(*types.Struct); !isStruct {
		return
	}

	targetArgIdx := -1
	for i, arg := range callExpr.Args {
		argStr := exprToString(arg)
		if argStr == lhsStr {
			targetArgIdx = i
			break
		}
	}
	if targetArgIdx < 0 {
		return
	}

	lhsPrefix := lhsStr + "."
	for i, arg := range callExpr.Args {
		if i == targetArgIdx {
			continue
		}
		if ExprContainsSelectorPath(arg, lhsStr, lhsPrefix) {
			return
		}
	}

	o.memTakeActive = true
	o.memTakeLhsExpr = lhsStr
	o.memTakeArgIdx = targetArgIdx
}

// exprToRustCodeOpt converts a simple AST expression to Rust code for optimization purposes.
func (o *RustOptState) exprToRustCodeOpt(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch ex := expr.(type) {
	case *ast.Ident:
		name := escapeRustKeyword(ex.Name)
		return name
	case *ast.SelectorExpr:
		base := o.exprToRustCodeOpt(ex.X)
		if base == "" {
			return ""
		}
		return base + "." + ex.Sel.Name
	case *ast.BasicLit:
		return ex.Value
	case *ast.IndexExpr:
		base := o.exprToRustCodeOpt(ex.X)
		idx := o.exprToRustCodeOpt(ex.Index)
		if base == "" || idx == "" {
			return ""
		}
		return base + "[" + idx + " as usize]"
	case *ast.BinaryExpr:
		left := o.exprToRustCodeOpt(ex.X)
		right := o.exprToRustCodeOpt(ex.Y)
		if left == "" || right == "" {
			return ""
		}
		return "(" + left + " " + ex.Op.String() + " " + right + ")"
	case *ast.ParenExpr:
		inner := o.exprToRustCodeOpt(ex.X)
		if inner == "" {
			return ""
		}
		return "(" + inner + ")"
	}
	return ""
}
