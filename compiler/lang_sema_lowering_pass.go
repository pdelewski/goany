package compiler

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// LangSemaLoweringPass rewrites Go AST patterns that lack direct equivalents
// in one or more target backends. It runs between the first SemaChecker
// and MethodReceiverLoweringPass.
//
// Transforms are divided into two categories:
//   - Always-run: the pattern has no 1:1 equivalent in ANY target language.
//   - Backend-conditional: the pattern has a 1:1 equivalent in some targets,
//     so it only fires when a backend that lacks the equivalent is enabled.
//
// Backend-conditional transforms:
//   - MultiAssignSplit      (C++ lacks tuple destructuring in decls)
//   - ShadowingRename       (C# forbids variable shadowing in same function)
//   - FieldNameConflict     (C++ -Wchanges-meaning when field name == type name)
//   - RustOwnership         (no-op — emitter handles clone/move directly)
type LangSemaLoweringPass struct {
	Backends BackendSet
}

func (p *LangSemaLoweringPass) Name() string { return "LangSemaLowering" }
func (p *LangSemaLoweringPass) ProLog()      {}
func (p *LangSemaLoweringPass) EpiLog()      {}

func (p *LangSemaLoweringPass) Visitors(pkg *packages.Package) []ast.Visitor {
	return []ast.Visitor{&canonicalizeVisitor{pkg: pkg, backends: p.Backends}}
}

func (p *LangSemaLoweringPass) PreVisit(visitor ast.Visitor) {}

func (p *LangSemaLoweringPass) PostVisit(visitor ast.Visitor, visited map[string]struct{}) {
	v := visitor.(*canonicalizeVisitor)
	v.transform()
}

// canonicalizeVisitor collects AST files during the walk,
// then applies all transforms in PostVisit via transform().
type canonicalizeVisitor struct {
	pkg        *packages.Package
	backends   BackendSet
	files      []*ast.File
	tmpCounter int // counter for generating unique temp variable names
}

func (v *canonicalizeVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}
	if file, ok := node.(*ast.File); ok {
		v.files = append(v.files, file)
	}
	// Don't recurse — transform() processes files directly via ast.Inspect.
	return nil
}

func (v *canonicalizeVisitor) transform() {
	for _, file := range v.files {
		// Always-run transforms (no 1:1 equivalent in any target language)
		v.transformTypeSwitch(file)
		v.transformTaglessSwitch(file)
		v.transformStringSwitch(file)
		v.transformNamedReturns(file)
		v.transformIotaExpansion(file)
		v.transformVariadics(file)
		v.transformMakeCapacity(file)
		v.transformIntegerRange(file)

		// Defer lowering — skip when ONLY Go backend is enabled (Go has native defer).
		// Must run after transformNamedReturns so bare returns are already explicit.
		if v.backends & ^BackendGo != 0 {
			v.transformDefer(file)
		}

		// Backend-conditional transforms (preserve 1:1 where target supports it)
		if v.backends.Has(BackendCpp) {
			v.transformMultiAssigns(file)
			v.transformFieldConflicts(file)
		}
		if v.backends.Has(BackendCSharp) {
			v.transformShadowedVars(file)
		}
		if v.backends.Has(BackendRust) {
			v.transformRustOwnership(file)
		}
	}
}

// ===========================================================================
// Always-Run Transforms (no target language has these concepts)
// ===========================================================================

// ---------------------------------------------------------------------------
// Named Return Lowering
// ---------------------------------------------------------------------------

// transformNamedReturns lowers named return values to explicit variable
// declarations and explicit return statements.
//
// Pattern matched:
//
//	func f() (x int, err error) {
//	    x = 42
//	    return          // bare return
//	}
//
// Transform:
//
//	func f() (int, error) {
//	    var x int
//	    var err error
//	    x = 42
//	    return x, err   // explicit return
//	}
func (v *canonicalizeVisitor) transformNamedReturns(file *ast.File) {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return
	}
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil || funcDecl.Type.Results == nil {
			continue
		}

		// Collect named return (name, type) pairs.
		type namedReturn struct {
			name    string
			typeExpr ast.Expr
		}
		var namedReturns []namedReturn
		for _, field := range funcDecl.Type.Results.List {
			if len(field.Names) == 0 {
				continue
			}
			for _, name := range field.Names {
				namedReturns = append(namedReturns, namedReturn{
					name:    name.Name,
					typeExpr: field.Type,
				})
			}
		}
		if len(namedReturns) == 0 {
			continue
		}

		// Strip names from the result fields.
		for _, field := range funcDecl.Type.Results.List {
			field.Names = nil
		}

		// Insert var declarations at the start of the function body.
		var varDecls []ast.Stmt
		for _, nr := range namedReturns {
			nameIdent := &ast.Ident{Name: nr.name, NamePos: funcDecl.Body.Lbrace}
			varDecls = append(varDecls, &ast.DeclStmt{
				Decl: &ast.GenDecl{
					Tok: token.VAR,
					Specs: []ast.Spec{
						&ast.ValueSpec{
							Names: []*ast.Ident{nameIdent},
							Type:  nr.typeExpr,
						},
					},
				},
			})
			// Register in TypesInfo so downstream passes can resolve the new ident.
			if v.pkg.TypesInfo != nil {
				if existingObj := v.pkg.TypesInfo.Defs[nameIdent]; existingObj == nil {
					// Try to find the type from the type expression
					if tv, ok := v.pkg.TypesInfo.Types[nr.typeExpr]; ok {
						obj := types.NewVar(funcDecl.Body.Lbrace, v.pkg.Types, nr.name, tv.Type)
						v.pkg.TypesInfo.Defs[nameIdent] = obj
					}
				}
			}
		}
		funcDecl.Body.List = append(varDecls, funcDecl.Body.List...)

		// Walk body and fill bare returns with named return identifiers.
		ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
			// Don't recurse into nested function literals
			if _, ok := n.(*ast.FuncLit); ok {
				return false
			}
			retStmt, ok := n.(*ast.ReturnStmt)
			if !ok || len(retStmt.Results) > 0 {
				return true
			}
			// Bare return — populate with named return idents
			for _, nr := range namedReturns {
				retStmt.Results = append(retStmt.Results, &ast.Ident{
					Name:    nr.name,
					NamePos: retStmt.Return,
				})
			}
			return true
		})
	}
}

// ---------------------------------------------------------------------------
// Iota Expansion
// ---------------------------------------------------------------------------

// transformIotaExpansion replaces iota-based const expressions with their
// computed integer values.
//
// Pattern matched:
//
//	const (
//	    A = iota      // 0
//	    B             // 1 (continuation)
//	    C             // 2
//	)
//
// Transform:
//
//	const (
//	    A int = 0
//	    B int = 1
//	    C int = 2
//	)
func (v *canonicalizeVisitor) transformIotaExpansion(file *ast.File) {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return
	}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}

		// Check if this const block uses iota.
		usesIota := false
		for _, spec := range genDecl.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			// Empty Values means continuation (implicitly repeats previous expression)
			if len(vs.Values) == 0 {
				usesIota = true
				break
			}
			for _, val := range vs.Values {
				if v.exprContainsIota(val) {
					usesIota = true
					break
				}
			}
			if usesIota {
				break
			}
		}
		if !usesIota {
			continue
		}

		// Replace each ValueSpec with computed values from the type checker.
		for _, spec := range genDecl.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				constObj, ok := v.pkg.TypesInfo.Defs[name].(*types.Const)
				if !ok || constObj == nil {
					continue
				}
				val := constObj.Val()
				valStr := val.String()
				// For integer values, use exact string
				if val.Kind() == constant.Int {
					valStr = val.ExactString()
				}

				// Build the replacement literal
				litKind := token.INT
				if val.Kind() == constant.String {
					litKind = token.STRING
				} else if val.Kind() == constant.Float {
					litKind = token.FLOAT
				}
				lit := &ast.BasicLit{Kind: litKind, Value: valStr}

				// Ensure Values slice is long enough
				for len(vs.Values) <= i {
					vs.Values = append(vs.Values, nil)
				}
				vs.Values[i] = lit

				// Set explicit type if not already present.
				if vs.Type == nil {
					constType := constObj.Type()
					if named, ok := constType.(*types.Named); ok {
						// Named type (e.g., type Color int) — preserve type name
						vs.Type = &ast.Ident{Name: named.Obj().Name()}
					} else if basic, ok := constType.(*types.Basic); ok {
						if basic.Info()&types.IsUntyped != 0 {
							// Untyped → use default typed version
							vs.Type = &ast.Ident{Name: "int"}
						} else {
							vs.Type = &ast.Ident{Name: basic.Name()}
						}
					}
				}
			}
		}
	}
}

// exprContainsIota checks if an expression contains an iota identifier.
func (v *canonicalizeVisitor) exprContainsIota(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		if ident, ok := n.(*ast.Ident); ok && ident.Name == "iota" {
			found = true
			return false
		}
		return true
	})
	return found
}

// ---------------------------------------------------------------------------
// Variadic Lowering
// ---------------------------------------------------------------------------

// transformVariadics lowers variadic functions to explicit slice parameters
// and wraps call-site arguments in slice literals.
//
// Pattern matched:
//
//	func f(prefix string, args ...int) { use(args) }
//	f("hi", 1, 2, 3)        // individual args
//	f("hi", slice...)        // spread
//	f("hi")                  // zero variadic args
//
// Transform:
//
//	func f(prefix string, args []int) { use(args) }
//	f("hi", []int{1, 2, 3}) // individual args wrapped
//	f("hi", slice)           // spread removed
//	f("hi", []int{})         // zero args → empty slice
func (v *canonicalizeVisitor) transformVariadics(file *ast.File) {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return
	}

	// Phase 1: Collect variadic functions and rewrite declarations.
	type variadicInfo struct {
		paramIndex int      // index of the variadic param
		elemType   ast.Expr // the element type T from ...T
	}
	variadics := make(map[types.Object]*variadicInfo)

	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Type.Params == nil {
			continue
		}
		params := funcDecl.Type.Params.List

		// Compute param index (accounting for multi-name fields)
		paramIndex := 0
		for i, field := range params {
			if ellipsis, ok := field.Type.(*ast.Ellipsis); ok {
				// Record the variadic function
				var funcObj types.Object
				if funcDecl.Name != nil {
					funcObj = v.pkg.TypesInfo.Defs[funcDecl.Name]
				}
				if funcObj != nil {
					variadics[funcObj] = &variadicInfo{
						paramIndex: paramIndex,
						elemType:   ellipsis.Elt,
					}
				}
				// Rewrite: ...T → []T
				params[i].Type = &ast.ArrayType{Elt: ellipsis.Elt}
				break
			}
			// Count names for this field (unnamed fields count as 1)
			if len(field.Names) > 0 {
				paramIndex += len(field.Names)
			} else {
				paramIndex++
			}
		}
	}

	if len(variadics) == 0 {
		return
	}

	// Phase 2: Rewrite call sites.
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		// Resolve the called function
		var funcObj types.Object
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			funcObj = v.pkg.TypesInfo.Uses[fn]
		case *ast.SelectorExpr:
			if sel, ok := v.pkg.TypesInfo.Selections[fn]; ok {
				funcObj = sel.Obj()
			} else {
				funcObj = v.pkg.TypesInfo.Uses[fn.Sel]
			}
		}

		info, ok := variadics[funcObj]
		if !ok {
			return true
		}

		idx := info.paramIndex

		if call.Ellipsis.IsValid() {
			// Spread call: f(args...) → f(args)
			call.Ellipsis = token.NoPos
		} else if len(call.Args) > idx {
			// Individual args: wrap trailing args in []T{...}
			// Copy to avoid aliasing: call.Args[:idx] and call.Args[idx:]
			// share the same backing array, so append would corrupt varArgs.
			varArgs := make([]ast.Expr, len(call.Args)-idx)
			copy(varArgs, call.Args[idx:])
			sliceLit := &ast.CompositeLit{
				Type: &ast.ArrayType{Elt: info.elemType},
				Elts: varArgs,
			}
			v.registerVariadicSliceLit(sliceLit, info.elemType)
			call.Args = append(call.Args[:idx], sliceLit)
		} else if len(call.Args) == idx {
			// Zero variadic args: insert []T{}
			sliceLit := &ast.CompositeLit{
				Type: &ast.ArrayType{Elt: info.elemType},
			}
			v.registerVariadicSliceLit(sliceLit, info.elemType)
			call.Args = append(call.Args, sliceLit)
		}

		return true
	})
}

// registerVariadicSliceLit registers a variadic slice literal in TypesInfo so
// downstream emitters can resolve the slice element type (e.g., List<Expr>
// instead of List<object> in C#).
func (v *canonicalizeVisitor) registerVariadicSliceLit(lit *ast.CompositeLit, elemTypeExpr ast.Expr) {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return
	}
	// Resolve the element type from TypesInfo
	if tv, ok := v.pkg.TypesInfo.Types[elemTypeExpr]; ok && tv.Type != nil {
		sliceType := types.NewSlice(tv.Type)
		v.pkg.TypesInfo.Types[lit] = types.TypeAndValue{Type: sliceType}
		// Also register the ArrayType node
		if arrType, ok := lit.Type.(*ast.ArrayType); ok {
			v.pkg.TypesInfo.Types[arrType] = types.TypeAndValue{Type: sliceType}
		}
	}
}

// ---------------------------------------------------------------------------
// Make Capacity Stripping
// ---------------------------------------------------------------------------

// transformMakeCapacity strips the capacity argument from make([]T, len, cap).
//
// Pattern matched:
//
//	make([]int, 5, 10)
//
// Why: No target language has a direct equivalent to Go's slice capacity hint.
// The capacity is a performance optimization that doesn't affect semantics.
//
// Transform:
//
//	make([]int, 5, 10)  →  make([]int, 5)
//
// Skipped:
//   - make(map[K]V) — maps don't have a 3rd arg
//   - make([]T, n) — already 2-arg, nothing to strip
func (v *canonicalizeVisitor) transformMakeCapacity(file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) != 3 {
			return true
		}
		ident, ok := call.Fun.(*ast.Ident)
		if !ok || ident.Name != "make" {
			return true
		}
		// Only strip for slice types (not maps)
		if _, ok := call.Args[0].(*ast.ArrayType); ok {
			call.Args = call.Args[:2] // drop capacity arg
		}
		return true
	})
}

// ---------------------------------------------------------------------------
// Integer Range Lowering
// ---------------------------------------------------------------------------

// transformIntegerRange lowers `for i := range N` (Go 1.22) to a classic
// three-clause for loop.
//
// Pattern matched:
//
//	for i := range 10 { ... }
//
// Why: All emitters expect range over slices/maps/strings. An integer range
// expression has no equivalent in target languages.
//
// Transform:
//
//	for i := range 10 { ... }  →  for i := 0; i < 10; i++ { ... }
//	for range 5 { ... }       →  for _0 := 0; _0 < 5; _0++ { ... }
func (v *canonicalizeVisitor) transformIntegerRange(file *ast.File) {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return
	}
	v.applyBlockTransform(file, func(list *[]ast.Stmt) {
		var newList []ast.Stmt
		changed := false
		for _, stmt := range *list {
			rangeStmt, ok := stmt.(*ast.RangeStmt)
			if !ok {
				newList = append(newList, stmt)
				continue
			}
			tv, ok := v.pkg.TypesInfo.Types[rangeStmt.X]
			if !ok {
				newList = append(newList, stmt)
				continue
			}
			basic, ok := tv.Type.Underlying().(*types.Basic)
			if !ok || basic.Info()&types.IsInteger == 0 {
				newList = append(newList, stmt)
				continue
			}

			// This is `for key := range N` — build a classic for loop
			// Determine the loop variable
			var keyIdent *ast.Ident
			tok := rangeStmt.Tok
			if rangeStmt.Key != nil {
				if ident, ok := rangeStmt.Key.(*ast.Ident); ok && ident.Name != "_" {
					keyIdent = ident
				}
			}
			if keyIdent == nil {
				// No key or blank key — generate a temp variable
				tmpName := v.nextTempName()
				keyIdent = &ast.Ident{Name: tmpName, NamePos: rangeStmt.Pos()}
				tok = token.DEFINE
			}

			// Build: for key := 0; key < N; key++ { body }
			forStmt := &ast.ForStmt{
				For: rangeStmt.Pos(),
				Init: &ast.AssignStmt{
					Lhs:    []ast.Expr{keyIdent},
					Tok:    tok,
					TokPos: rangeStmt.TokPos,
					Rhs:    []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: "0"}},
				},
				Cond: &ast.BinaryExpr{
					X:  &ast.Ident{Name: keyIdent.Name, NamePos: rangeStmt.Pos()},
					Op: token.LSS,
					Y:  rangeStmt.X,
				},
				Post: &ast.IncDecStmt{
					X:   &ast.Ident{Name: keyIdent.Name, NamePos: rangeStmt.Pos()},
					Tok: token.INC,
				},
				Body: rangeStmt.Body,
			}
			newList = append(newList, forStmt)
			changed = true
		}
		if changed {
			*list = newList
		}
	})
}

// ---------------------------------------------------------------------------
// Defer Lowering
// ---------------------------------------------------------------------------

// transformDefer lowers `defer f(args...)` to explicit cleanup calls inserted
// before every return point (LIFO order). Arguments are captured at the defer
// site into temp variables when they are not simple literals or constants.
//
// Pattern matched:
//
//	func foo() int {
//	    defer cleanup()
//	    if cond { return 1 }
//	    return 2
//	}
//
// Transform:
//
//	func foo() int {
//	    if cond { cleanup(); return 1 }
//	    cleanup()
//	    return 2
//	}
//
// Multiple defers are inserted in LIFO order (last defer first).
//
// Argument capture:
//
//	x := compute()
//	defer f(x)       →   _t0 := x        // capture at defer site
//	x = other()          x = other()
//	return               f(_t0); return   // use captured value
func (v *canonicalizeVisitor) transformDefer(file *ast.File) {
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			continue
		}
		v.lowerDeferInFunc(funcDecl)
	}
}

// deferInfo holds a collected defer statement's captured call and any
// temp variable assignments needed to capture arguments.
type deferInfo struct {
	captures []*ast.AssignStmt // temp := arg assignments (inserted at defer site)
	call     *ast.CallExpr     // the call with args replaced by temps where needed
}

// lowerDeferInFunc processes a single function declaration:
// 1. Collect defer statements and build argument captures.
// 2. Remove defer stmts, insert captures at original sites.
// 3. Insert deferred calls (LIFO) before every return statement.
func (v *canonicalizeVisitor) lowerDeferInFunc(funcDecl *ast.FuncDecl) {
	body := funcDecl.Body
	if body == nil {
		return
	}

	// Phase 1: Collect defers from the top-level body.
	var defers []deferInfo
	var newBody []ast.Stmt
	for _, stmt := range body.List {
		deferStmt, ok := stmt.(*ast.DeferStmt)
		if !ok {
			newBody = append(newBody, stmt)
			continue
		}
		// Build captures for non-trivial arguments
		di := v.buildDeferInfo(deferStmt)
		defers = append(defers, di)
		// Insert capture assignments at the defer site (if any)
		for _, cap := range di.captures {
			newBody = append(newBody, cap)
		}
	}

	if len(defers) == 0 {
		return // no defers to lower
	}

	body.List = newBody

	// Phase 2: Build the LIFO call list (reverse order).
	var lifoCalls []*ast.CallExpr
	for i := len(defers) - 1; i >= 0; i-- {
		lifoCalls = append(lifoCalls, defers[i].call)
	}

	// Phase 3: Insert deferred calls before every return statement.
	v.insertDeferredBeforeReturns(&body.List, lifoCalls)

	// Phase 4: If the function doesn't end with a return, append cleanup at end.
	if len(body.List) == 0 || !v.endsWithReturn(body.List) {
		for _, call := range lifoCalls {
			body.List = append(body.List, &ast.ExprStmt{X: v.cloneCall(call)})
		}
	}
}

// buildDeferInfo creates capture assignments and a rewritten call for a defer stmt.
func (v *canonicalizeVisitor) buildDeferInfo(deferStmt *ast.DeferStmt) deferInfo {
	call := deferStmt.Call
	di := deferInfo{}

	// Clone the call's args, replacing non-trivial ones with temp captures.
	newArgs := make([]ast.Expr, len(call.Args))
	for i, arg := range call.Args {
		if v.needsCapture(arg) {
			tmpAssign, tmpObj := v.createTempAssign(arg, deferStmt.Pos())
			di.captures = append(di.captures, tmpAssign)
			newArgs[i] = v.createTempRef(tmpObj, deferStmt.Pos())
		} else {
			newArgs[i] = arg
		}
	}

	di.call = &ast.CallExpr{
		Fun:    call.Fun,
		Args:   newArgs,
		Lparen: call.Lparen,
		Rparen: call.Rparen,
	}
	return di
}

// needsCapture returns true if an argument expression needs to be captured
// into a temp variable at the defer site. Literals, function names, and
// composite literals don't need capture.
func (v *canonicalizeVisitor) needsCapture(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return false // literal: 1, "hi", etc.
	case *ast.FuncLit:
		return false // closure literal
	case *ast.CompositeLit:
		return false // []int{1,2,3}, Struct{}, etc.
	case *ast.Ident:
		// Constants and function names don't need capture.
		if v.pkg != nil && v.pkg.TypesInfo != nil {
			if obj := v.pkg.TypesInfo.Uses[e]; obj != nil {
				if _, isConst := obj.(*types.Const); isConst {
					return false
				}
				if _, isFunc := obj.(*types.Func); isFunc {
					return false
				}
			}
		}
		return true // variable — needs capture
	default:
		return true // selector, call, index, etc. — needs capture
	}
}

// cloneCall creates a shallow copy of a CallExpr so each insertion site
// has its own AST node (required since a single node cannot appear at
// multiple positions in the tree).
func (v *canonicalizeVisitor) cloneCall(call *ast.CallExpr) *ast.CallExpr {
	argsCopy := make([]ast.Expr, len(call.Args))
	copy(argsCopy, call.Args)
	return &ast.CallExpr{
		Fun:    call.Fun,
		Args:   argsCopy,
		Lparen: call.Lparen,
		Rparen: call.Rparen,
	}
}

// insertDeferredBeforeReturns walks a statement list recursively and splices
// deferred calls (as ExprStmts) before every *ast.ReturnStmt. It does NOT
// recurse into *ast.FuncLit (nested functions have their own defer scope).
func (v *canonicalizeVisitor) insertDeferredBeforeReturns(list *[]ast.Stmt, calls []*ast.CallExpr) {
	var newList []ast.Stmt
	changed := false
	for _, stmt := range *list {
		// Recurse into sub-blocks (if/else/switch/for) but NOT FuncLit.
		switch s := stmt.(type) {
		case *ast.IfStmt:
			v.insertDeferredBeforeReturns(&s.Body.List, calls)
			if s.Else != nil {
				v.insertDeferredInElse(s, calls)
			}
		case *ast.SwitchStmt:
			if s.Body != nil {
				for _, cc := range s.Body.List {
					if clause, ok := cc.(*ast.CaseClause); ok {
						v.insertDeferredBeforeReturns(&clause.Body, calls)
					}
				}
			}
		case *ast.ForStmt:
			if s.Body != nil {
				v.insertDeferredBeforeReturns(&s.Body.List, calls)
			}
		case *ast.RangeStmt:
			if s.Body != nil {
				v.insertDeferredBeforeReturns(&s.Body.List, calls)
			}
		case *ast.BlockStmt:
			v.insertDeferredBeforeReturns(&s.List, calls)
		}

		if ret, ok := stmt.(*ast.ReturnStmt); ok {
			// Insert deferred calls before this return.
			for _, call := range calls {
				newList = append(newList, &ast.ExprStmt{X: v.cloneCall(call)})
			}
			newList = append(newList, ret)
			changed = true
		} else {
			newList = append(newList, stmt)
		}
	}
	if changed {
		*list = newList
	}
}

// insertDeferredInElse handles the else branch of an if statement, which can be
// either a *ast.BlockStmt or another *ast.IfStmt (else-if chain).
func (v *canonicalizeVisitor) insertDeferredInElse(ifStmt *ast.IfStmt, calls []*ast.CallExpr) {
	switch e := ifStmt.Else.(type) {
	case *ast.BlockStmt:
		v.insertDeferredBeforeReturns(&e.List, calls)
	case *ast.IfStmt:
		v.insertDeferredBeforeReturns(&e.Body.List, calls)
		if e.Else != nil {
			v.insertDeferredInElse(e, calls)
		}
	}
}

// endsWithReturn checks if the last statement in a list is a return statement.
func (v *canonicalizeVisitor) endsWithReturn(list []ast.Stmt) bool {
	if len(list) == 0 {
		return false
	}
	_, ok := list[len(list)-1].(*ast.ReturnStmt)
	return ok
}

// ===========================================================================
// Backend-Conditional Transforms
// ===========================================================================

// ---------------------------------------------------------------------------
// MultiAssignSplit
// ---------------------------------------------------------------------------

// transformMultiAssigns splits multi-variable assignments into separate statements.
//
// Pattern matched:
//
//	a, b := 1, 2
//	x, y = 3, 4
//
// Why: The C++ backend cannot generate correct code for tuple unpacking
// in variable declarations. Rust has tuple destructuring (let (a,b) = (1,2))
// so this transform is skipped when only Rust is enabled.
//
// Transform:
//
//	a, b := 1, 2  →  a := 1
//	                  b := 2
//
// Skipped (not transformed):
//   - v, ok := m[key]      (comma-ok map access — single RHS)
//   - v, ok := x.(T)       (comma-ok type assertion — single RHS)
//   - a, b := f()           (multi-return function call — single RHS)
//   - Any case where len(Lhs) != len(Rhs)
func (v *canonicalizeVisitor) transformMultiAssigns(file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BlockStmt:
			v.splitMultiAssignsInList(&node.List)
		case *ast.CaseClause:
			v.splitMultiAssignsInList(&node.Body)
		}
		return true
	})
}

// splitMultiAssignsInList scans a statement list for multi-variable assignments
// and replaces each with individual assignment statements.
func (v *canonicalizeVisitor) splitMultiAssignsInList(list *[]ast.Stmt) {
	var newList []ast.Stmt
	changed := false
	for _, stmt := range *list {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || !v.shouldSplitAssign(assign) {
			newList = append(newList, stmt)
			continue
		}
		for i := range assign.Lhs {
			newList = append(newList, &ast.AssignStmt{
				Lhs:    []ast.Expr{assign.Lhs[i]},
				Tok:    assign.Tok,
				TokPos: assign.TokPos,
				Rhs:    []ast.Expr{assign.Rhs[i]},
			})
		}
		changed = true
	}
	if changed {
		*list = newList
	}
}

// shouldSplitAssign returns true if the assignment has multiple LHS and RHS
// expressions that can be split into individual assignments.
// It returns false for comma-ok patterns and multi-return calls where
// a single RHS expression produces multiple values.
func (v *canonicalizeVisitor) shouldSplitAssign(assign *ast.AssignStmt) bool {
	// Only split when there are multiple LHS expressions with matching RHS.
	// Comma-ok patterns (map access, type assertion, multi-return call) have
	// len(Rhs)==1, so they are automatically excluded by requiring len(Lhs)==len(Rhs).
	return len(assign.Lhs) > 1 && len(assign.Rhs) > 1 && len(assign.Lhs) == len(assign.Rhs)
}

// ---------------------------------------------------------------------------
// ShadowingRename
// ---------------------------------------------------------------------------

// transformShadowedVars renames variables that shadow outer-scope variables
// within the same function.
//
// Pattern matched:
//
//	func f() {
//	    x := 1
//	    if true {
//	        x := 2   // shadows outer x
//	        _ = x
//	    }
//	}
//
// Why: C# does not allow variable shadowing within the same function/method.
// Rust, Go, and JavaScript all permit shadowing, so this transform is
// skipped when C# is not an enabled backend.
//
// Transform:
//
//	x := 1            x := 1
//	if true {     →   if true {
//	    x := 2            x_2 := 2
//	    _ = x             _ = x_2
//	}                 }
//
// Skipped (not transformed):
//   - Redeclarations in the same scope (x, err := f(); y, err := g())
//   - Variables that shadow package-level names (only function-local shadowing)
//
// Note: blank identifier `_` is skipped (not renamed). C# handles discards
// through special-casing in the emitter (not declaring `_` as a variable).
func (v *canonicalizeVisitor) transformShadowedVars(file *ast.File) {
	if v.pkg == nil || v.pkg.TypesInfo == nil || v.pkg.Types == nil {
		return
	}

	// Map from types.Object (the shadowing declaration) → new name.
	renames := make(map[types.Object]string)
	usedNames := v.collectAllNames(file)

	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			continue
		}

		// Find shadowing declarations within this function.
		ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || assign.Tok != token.DEFINE {
				return true
			}

			for _, lhs := range assign.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || ident.Name == "_" {
					continue
				}

				obj := v.pkg.TypesInfo.Defs[ident]
				if obj == nil {
					continue
				}

				scope := obj.Parent()
				if scope == nil {
					continue
				}

				// Walk outward through parent scopes looking for a variable
				// with the same name. Stop at the package scope — shadowing
				// of package-level names is allowed in C#.
				shadows := false
				for outer := scope.Parent(); outer != nil; outer = outer.Parent() {
					if outer == v.pkg.Types.Scope() {
						break
					}
					if outerObj := outer.Lookup(ident.Name); outerObj != nil {
						if _, isVar := outerObj.(*types.Var); isVar {
							shadows = true
							break
						}
					}
				}

				if shadows {
					newName := generateUniqueName(ident.Name, usedNames)
					usedNames[newName] = true
					renames[obj] = newName
				}
			}
			return true
		})
	}

	if len(renames) == 0 {
		return
	}

	// Rename all definitions and uses that refer to the shadowing objects.
	// Because we modify ident.Name in place (same pointer), TypesInfo
	// maps keyed by *ast.Ident remain valid.
	ast.Inspect(file, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if obj := v.pkg.TypesInfo.Defs[ident]; obj != nil {
			if newName, ok := renames[obj]; ok {
				ident.Name = newName
			}
		}
		if obj := v.pkg.TypesInfo.Uses[ident]; obj != nil {
			if newName, ok := renames[obj]; ok {
				ident.Name = newName
			}
		}
		return true
	})
}

// collectAllNames returns a set of all identifier names used in the file.
// This is used to avoid generating a rename that collides with an existing name.
func (v *canonicalizeVisitor) collectAllNames(file *ast.File) map[string]bool {
	names := make(map[string]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		if ident, ok := n.(*ast.Ident); ok {
			names[ident.Name] = true
		}
		return true
	})
	return names
}

// generateUniqueName produces a name like "x_2", "x_3", etc. that is not
// already present in the used set.
func generateUniqueName(base string, used map[string]bool) string {
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s_%d", base, i)
		if !used[candidate] {
			return candidate
		}
	}
}

// ---------------------------------------------------------------------------
// FieldNameConflict
// ---------------------------------------------------------------------------

// transformFieldConflicts renames struct fields whose name matches their type name.
//
// Pattern matched:
//
//	type Baz int
//	type Foo struct {
//	    Baz Baz   // field name "Baz" == type name "Baz"
//	}
//
// Why: In C++, a member whose name shadows a type name causes
// -Wchanges-meaning: "declaration of 'Baz Foo::Baz' changes meaning of 'Baz'".
// Other backends handle this fine, so the transform only fires when C++ is enabled.
//
// Transform:
//
//	type Foo struct {       type Foo struct {
//	    Baz Baz         →       BazVal Baz
//	}                       }
//	f.Baz = 1           →  f.BazVal = 1
//
// Skipped (not transformed):
//   - Qualified types (pkg.Type) — these become pkg::Type in C++, no conflict.
//   - Fields where the name does not match the type name.
func (v *canonicalizeVisitor) transformFieldConflicts(file *ast.File) {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return
	}

	// Map from field's types.Object → new name.
	fieldRenames := make(map[types.Object]string)

	// Find struct fields where name matches type name.
	ast.Inspect(file, func(n ast.Node) bool {
		structType, ok := n.(*ast.StructType)
		if !ok || structType.Fields == nil {
			return true
		}

		for _, field := range structType.Fields.List {
			// Only check simple identifiers (same-package types).
			typeIdent, ok := field.Type.(*ast.Ident)
			if !ok {
				continue
			}
			typeName := typeIdent.Name
			for _, name := range field.Names {
				if name.Name == typeName {
					newName := name.Name + "Val"
					if obj := v.pkg.TypesInfo.Defs[name]; obj != nil {
						fieldRenames[obj] = newName
					}
				}
			}
		}
		return true
	})

	if len(fieldRenames) == 0 {
		return
	}

	// Rename field declarations and all uses (selectors, composite-lit keys).
	ast.Inspect(file, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok {
			return true
		}
		if obj := v.pkg.TypesInfo.Defs[ident]; obj != nil {
			if newName, ok := fieldRenames[obj]; ok {
				ident.Name = newName
			}
		}
		if obj := v.pkg.TypesInfo.Uses[ident]; obj != nil {
			if newName, ok := fieldRenames[obj]; ok {
				ident.Name = newName
			}
		}
		return true
	})
}

// ===========================================================================
// Rust Ownership Transforms
// ===========================================================================
//
// These transforms rewrite Go patterns that violate Rust's move semantics
// or borrow checker rules. They only fire when the Rust backend is enabled.
// Each sub-pattern is documented with what it matches, why it needs
// transformation, and what the output looks like.

// transformRustOwnership applies all Rust ownership sub-pattern transforms.
func (v *canonicalizeVisitor) transformRustOwnership(file *ast.File) {
	// All Rust ownership transforms (slice self-reference, clone insertion)
	// are now handled directly by the Rust emitter's .clone() additions.
	// No AST-level temp extraction needed.
}

// ---------------------------------------------------------------------------
// Temp variable helpers
// ---------------------------------------------------------------------------

// nextTempName returns a unique temp variable name like "_t0", "_t1", etc.
func (v *canonicalizeVisitor) nextTempName() string {
	name := fmt.Sprintf("_t%d", v.tmpCounter)
	v.tmpCounter++
	return name
}

// exprType returns the Go type of an expression, or nil if unknown.
func (v *canonicalizeVisitor) exprType(expr ast.Expr) types.Type {
	if tv, ok := v.pkg.TypesInfo.Types[expr]; ok {
		return tv.Type
	}
	return nil
}

// isCopyType returns true if t is a Rust Copy type (primitives).
func (v *canonicalizeVisitor) isCopyType(t types.Type) bool {
	if basic, ok := t.Underlying().(*types.Basic); ok {
		switch basic.Kind() {
		case types.Bool,
			types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Uintptr, types.Float32, types.Float64,
			types.Complex64, types.Complex128:
			return true
		}
	}
	return false
}

// createTempAssign creates `tmpName := expr` and registers it in TypesInfo.
// Returns the assignment statement and the types.Object for the temp variable.
func (v *canonicalizeVisitor) createTempAssign(expr ast.Expr, pos token.Pos) (*ast.AssignStmt, types.Object) {
	tmpName := v.nextTempName()
	defIdent := &ast.Ident{Name: tmpName, NamePos: pos}

	var obj types.Object
	if t := v.exprType(expr); t != nil {
		obj = types.NewVar(pos, v.pkg.Types, tmpName, t)
		v.pkg.TypesInfo.Defs[defIdent] = obj
	}

	assign := &ast.AssignStmt{
		Lhs:    []ast.Expr{defIdent},
		Tok:    token.DEFINE,
		TokPos: pos,
		Rhs:    []ast.Expr{expr},
	}
	return assign, obj
}

// createTempRef creates an *ast.Ident referencing a previously created temp var.
func (v *canonicalizeVisitor) createTempRef(obj types.Object, pos token.Pos) *ast.Ident {
	ref := &ast.Ident{Name: obj.Name(), NamePos: pos}
	v.pkg.TypesInfo.Uses[ref] = obj
	// Also register in Types so downstream passes can query the type
	if obj.Type() != nil {
		v.pkg.TypesInfo.Types[ref] = types.TypeAndValue{Type: obj.Type()}
	}
	return ref
}

// applyBlockTransform walks all block-like nodes and calls fn on each stmt list.
func (v *canonicalizeVisitor) applyBlockTransform(file *ast.File, fn func(*[]ast.Stmt)) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BlockStmt:
			fn(&node.List)
		case *ast.CaseClause:
			fn(&node.Body)
		}
		return true
	})
}

// ---------------------------------------------------------------------------
// 1. Self-referencing += concatenation
// ---------------------------------------------------------------------------

// Slice self-reference transforms (transformSliceSelfRef, extractSliceSelfRefPrefixes,
// exprContainsSliceAccess) were removed — the Rust emitter now handles these cases
// directly: exprNeedsClone adds .clone() for IndexExpr RHS in assignments, and
// PostVisitCallExprArg adds .clone() for non-Copy call arguments (covering the
// append(slice, slice[i]) pattern).

// Transforms 3-5 (transformSameVarMultipleArgs, transformBinaryExprSameVar,
// transformNestedCallSharing) were removed — the emitter + CloneMovePass
// pipeline handles these cases correctly: emitter adds .clone() on all
// non-Copy args, CloneMovePass removes the last-use clone (move optimization).
// The AST temp extraction approach was incorrect for 3+ uses of the same
// variable (e.g., f(x,x,x) → _t0:=x; _t1:=x; f(x,_t0,_t1) produces
// multiple moves of x, which is invalid in Rust).


// Transforms 6-7 (transformStringReuseAfterConcat, transformMultiClosureSameVar)
// were also removed — same issue as transforms 3-5: `x_copy := x` moves x in
// Rust, making the original variable unusable. The emitter + CloneMovePass
// pipeline handles these patterns correctly via conservative .clone().

// ===========================================================================
// Type Switch Lowering
// ===========================================================================

// ===========================================================================
// Type Switch Lowering
// ===========================================================================

// transformTypeSwitch rewrites type switch statements into chained if/else
// with comma-ok type assertions, which all backends already support.
//
// Pattern matched:
//
//	switch v := x.(type) {
//	case Number:
//	    // use v as Number
//	case BinOp:
//	    // use v as BinOp
//	default:
//	    // default body
//	}
//
// Transform:
//
//	if v, _ok := x.(Number); _ok {
//	    // use v as Number
//	} else if v, _ok := x.(BinOp); _ok {
//	    // use v as BinOp
//	} else {
//	    // default body
//	}
//
// Also handles the no-variable form:
//
//	switch x.(type) {
//	case int:
//	    // body
//	}
//
// Transform (no-variable form):
//
//	if _, _ok := x.(int); _ok {
//	    // body
//	}
func (v *canonicalizeVisitor) transformTypeSwitch(file *ast.File) {
	v.applyBlockTransform(file, func(list *[]ast.Stmt) {
		var newList []ast.Stmt
		changed := false
		for _, stmt := range *list {
			ts, ok := stmt.(*ast.TypeSwitchStmt)
			if !ok {
				newList = append(newList, stmt)
				continue
			}

			result := v.lowerTypeSwitch(ts)
			if result != nil {
				newList = append(newList, result)
				changed = true
			} else {
				newList = append(newList, stmt)
			}
		}
		if changed {
			*list = newList
		}
	})
}

// transformTaglessSwitch rewrites tagless switch statements (switch { case cond: ... })
// into chained if/else statements. This is needed because C++/Java switch statements
// require a tag expression and cannot have boolean condition cases.
//
// Input:
//
//	switch {
//	case x > 0:
//	    // body1
//	case x < 0:
//	    // body2
//	default:
//	    // body3
//	}
//
// Output:
//
//	if x > 0 {
//	    // body1
//	} else if x < 0 {
//	    // body2
//	} else {
//	    // body3
//	}
func (v *canonicalizeVisitor) transformTaglessSwitch(file *ast.File) {
	v.applyBlockTransform(file, func(list *[]ast.Stmt) {
		var newList []ast.Stmt
		changed := false
		for _, stmt := range *list {
			sw, ok := stmt.(*ast.SwitchStmt)
			if !ok || sw.Tag != nil {
				newList = append(newList, stmt)
				continue
			}

			result := v.lowerTaglessSwitch(sw)
			if result != nil {
				newList = append(newList, result)
				changed = true
			} else {
				newList = append(newList, stmt)
			}
		}
		if changed {
			*list = newList
		}
	})
}

// lowerTaglessSwitch converts a tagless SwitchStmt into an if/else chain.
func (v *canonicalizeVisitor) lowerTaglessSwitch(sw *ast.SwitchStmt) ast.Stmt {
	if sw.Body == nil || len(sw.Body.List) == 0 {
		return nil
	}

	var condCases []*ast.CaseClause
	var defaultCase *ast.CaseClause

	for _, c := range sw.Body.List {
		cc, ok := c.(*ast.CaseClause)
		if !ok {
			continue
		}
		if cc.List == nil || len(cc.List) == 0 {
			defaultCase = cc
		} else {
			condCases = append(condCases, cc)
		}
	}

	if len(condCases) == 0 && defaultCase == nil {
		return nil
	}

	// Build the if/else chain from the bottom up.
	var elseBlock ast.Stmt
	if defaultCase != nil && len(defaultCase.Body) > 0 {
		elseBlock = &ast.BlockStmt{List: defaultCase.Body}
	}

	for i := len(condCases) - 1; i >= 0; i-- {
		cc := condCases[i]
		// Build condition: for multi-case (case a, b:), use a || b
		var cond ast.Expr
		for _, expr := range cc.List {
			if cond == nil {
				cond = expr
			} else {
				cond = &ast.BinaryExpr{
					X:  cond,
					Op: token.LOR,
					Y:  expr,
				}
			}
		}

		ifStmt := &ast.IfStmt{
			Cond: cond,
			Body: &ast.BlockStmt{List: cc.Body},
			Else: elseBlock,
		}
		elseBlock = ifStmt
	}

	if elseBlock == nil {
		return nil
	}
	return elseBlock
}

// transformStringSwitch rewrites switch statements with string tags into
// if/else chains. C++ cannot switch on std::string, so we lower these
// to chained if/else with == comparisons.
func (v *canonicalizeVisitor) transformStringSwitch(file *ast.File) {
	v.applyBlockTransform(file, func(list *[]ast.Stmt) {
		var newList []ast.Stmt
		changed := false
		for _, stmt := range *list {
			sw, ok := stmt.(*ast.SwitchStmt)
			if !ok || sw.Tag == nil {
				newList = append(newList, stmt)
				continue
			}

			// Check if the tag is a string type
			tv := v.pkg.TypesInfo.Types[sw.Tag]
			if tv.Type == nil {
				newList = append(newList, stmt)
				continue
			}
			basic, ok := tv.Type.Underlying().(*types.Basic)
			if !ok || basic.Kind() != types.String {
				newList = append(newList, stmt)
				continue
			}

			result := v.lowerStringSwitch(sw)
			if result != nil {
				newList = append(newList, result)
				changed = true
			} else {
				newList = append(newList, stmt)
			}
		}
		if changed {
			*list = newList
		}
	})
}

// lowerStringSwitch converts a string-tagged SwitchStmt into an if/else chain.
func (v *canonicalizeVisitor) lowerStringSwitch(sw *ast.SwitchStmt) ast.Stmt {
	if sw.Body == nil || len(sw.Body.List) == 0 {
		return nil
	}

	var condCases []*ast.CaseClause
	var defaultCase *ast.CaseClause

	for _, c := range sw.Body.List {
		cc, ok := c.(*ast.CaseClause)
		if !ok {
			continue
		}
		if cc.List == nil || len(cc.List) == 0 {
			defaultCase = cc
		} else {
			condCases = append(condCases, cc)
		}
	}

	if len(condCases) == 0 && defaultCase == nil {
		return nil
	}

	// Build the if/else chain from the bottom up.
	var elseBlock ast.Stmt
	if defaultCase != nil && len(defaultCase.Body) > 0 {
		elseBlock = &ast.BlockStmt{List: defaultCase.Body}
	}

	for i := len(condCases) - 1; i >= 0; i-- {
		cc := condCases[i]
		// Build condition: tag == val1 || tag == val2 || ...
		var cond ast.Expr
		for _, expr := range cc.List {
			eq := &ast.BinaryExpr{
				X:  sw.Tag,
				Op: token.EQL,
				Y:  expr,
			}
			// Register type info for the comparison
			v.pkg.TypesInfo.Types[eq] = types.TypeAndValue{
				Type: types.Typ[types.Bool],
			}
			if cond == nil {
				cond = eq
			} else {
				or := &ast.BinaryExpr{
					X:  cond,
					Op: token.LOR,
					Y:  eq,
				}
				v.pkg.TypesInfo.Types[or] = types.TypeAndValue{
					Type: types.Typ[types.Bool],
				}
				cond = or
			}
		}

		ifStmt := &ast.IfStmt{
			Cond: cond,
			Body: &ast.BlockStmt{List: cc.Body},
			Else: elseBlock,
		}
		elseBlock = ifStmt
	}

	if elseBlock == nil {
		return nil
	}
	return elseBlock
}

// lowerTypeSwitch converts a single TypeSwitchStmt into an if/else chain.
func (v *canonicalizeVisitor) lowerTypeSwitch(ts *ast.TypeSwitchStmt) ast.Stmt {
	// Extract the switched expression and optional variable name.
	// Two forms:
	//   switch v := x.(type) { ... }   →  AssignStmt with x.(type) on RHS
	//   switch x.(type) { ... }         →  ExprStmt with x.(type)
	var switchExpr ast.Expr // the expression being type-switched (x)
	var varName string      // the variable name (v), empty if no variable

	switch a := ts.Assign.(type) {
	case *ast.AssignStmt:
		// v := x.(type)
		if len(a.Lhs) >= 1 {
			if ident, ok := a.Lhs[0].(*ast.Ident); ok {
				varName = ident.Name
			}
		}
		if len(a.Rhs) >= 1 {
			if ta, ok := a.Rhs[0].(*ast.TypeAssertExpr); ok {
				switchExpr = ta.X
			}
		}
	case *ast.ExprStmt:
		if ta, ok := a.X.(*ast.TypeAssertExpr); ok {
			switchExpr = ta.X
		}
	}

	if switchExpr == nil {
		return nil
	}

	if ts.Body == nil || len(ts.Body.List) == 0 {
		return nil
	}

	// Collect case clauses
	var typeCases []*ast.CaseClause
	var defaultCase *ast.CaseClause

	for _, c := range ts.Body.List {
		cc, ok := c.(*ast.CaseClause)
		if !ok {
			continue
		}
		if cc.List == nil || len(cc.List) == 0 {
			defaultCase = cc
		} else {
			typeCases = append(typeCases, cc)
		}
	}

	if len(typeCases) == 0 && defaultCase == nil {
		return nil
	}

	// Build the if/else chain from the bottom up.
	// Start with the default (else) block, then prepend each type case.
	var elseBlock ast.Stmt
	if defaultCase != nil && len(defaultCase.Body) > 0 {
		elseBlock = &ast.BlockStmt{List: defaultCase.Body}
	}

	// Build from last case to first, chaining else clauses
	for i := len(typeCases) - 1; i >= 0; i-- {
		cc := typeCases[i]
		ifStmt := v.buildTypeCaseIf(switchExpr, varName, cc)
		if ifStmt == nil {
			continue
		}

		// For multi-type cases, buildTypeCaseIf returns a chain of if/else ifs.
		// We need to attach the elseBlock to the LAST if in that chain, not the first.
		lastIf := ifStmt
		for {
			next, ok := lastIf.Else.(*ast.IfStmt)
			if !ok {
				break
			}
			lastIf = next
		}
		lastIf.Else = elseBlock

		elseBlock = ifStmt
	}

	if elseBlock == nil {
		return nil
	}

	// If there was an Init statement, wrap in a block:
	// { init; if ... }
	if ts.Init != nil {
		return &ast.BlockStmt{
			List: []ast.Stmt{ts.Init, elseBlock},
		}
	}

	return elseBlock
}

// buildTypeCaseIf builds a single if-statement for one type case:
//
//	if v, _ok := x.(T); _ok { body }
//
// For multi-type cases like `case int, string:`, it generates OR conditions:
//
//	if _, _ok1 := x.(int); _ok1 { body } (picks first match)
func (v *canonicalizeVisitor) buildTypeCaseIf(switchExpr ast.Expr, varName string, cc *ast.CaseClause) *ast.IfStmt {
	if len(cc.List) == 0 {
		return nil
	}

	// Single type case (most common): if v, _ok := x.(T); _ok { body }
	if len(cc.List) == 1 {
		return v.buildSingleTypeCaseIf(switchExpr, varName, cc.List[0], cc.Body)
	}

	// Multi-type case: case T1, T2: body
	// In Go, multi-type case gives the variable the interface type (not concrete).
	// We generate: if _, _ok := x.(T1); _ok { body } else if _, _ok := x.(T2); _ok { body }
	// With no typed variable — if the original code used `v`, we rename it to the
	// switch expression ident so it keeps its interface type.
	var result *ast.IfStmt
	for i := len(cc.List) - 1; i >= 0; i-- {
		// Pass "" for varName: multi-type case uses blank identifier for assertion
		ifStmt := v.buildSingleTypeCaseIf(switchExpr, "", cc.List[i], cc.Body)
		if ifStmt == nil {
			continue
		}
		// If there was a variable name, rename it to the switch expression in the body.
		// In Go multi-type case, the variable has interface type = same as switch expr.
		if varName != "" {
			if switchIdent, ok := switchExpr.(*ast.Ident); ok {
				v.renameIdentInStmts(ifStmt.Body.List, varName, switchIdent.Name)
			}
		}
		if result != nil {
			ifStmt.Else = result
		}
		result = ifStmt
	}
	return result
}

// buildSingleTypeCaseIf builds: if _tsv_N, _tsok_N := x.(T); _tsok_N { body }
// Each case gets a unique variable name to avoid C# CS0136 (variable shadowing
// across if/else branches). References to the original varName in the body are
// renamed to the unique name.
func (v *canonicalizeVisitor) buildSingleTypeCaseIf(switchExpr ast.Expr, varName string, caseType ast.Expr, body []ast.Stmt) *ast.IfStmt {
	idx := v.tmpCounter
	v.tmpCounter++
	okName := fmt.Sprintf("_tsok_%d", idx)

	// Generate unique variable name per case to avoid C# scoping issues.
	// If the original code uses `v`, generate `_tsv_0`, `_tsv_1`, etc.
	// If no variable (switch x.(type)), use Go's blank identifier `_`.
	var uniqueVarName string
	if varName != "" {
		uniqueVarName = fmt.Sprintf("_tsv_%d", idx)
	} else {
		uniqueVarName = "_"
	}

	lhsVar := ast.NewIdent(uniqueVarName)
	okIdent := ast.NewIdent(okName)

	// Build the type assertion: x.(T)
	typeAssert := &ast.TypeAssertExpr{
		X:    switchExpr,
		Type: caseType,
	}

	// Deep-copy the body and rename references from varName to uniqueVarName
	bodyCopy := make([]ast.Stmt, len(body))
	copy(bodyCopy, body)
	if varName != "" {
		v.renameIdentInStmts(bodyCopy, varName, uniqueVarName)
	}

	// Register the type assertion result type in TypesInfo if possible
	if v.pkg != nil && v.pkg.TypesInfo != nil {
		if tv, ok := v.pkg.TypesInfo.Types[caseType]; ok && tv.Type != nil {
			v.pkg.TypesInfo.Types[typeAssert] = types.TypeAndValue{
				Type: tv.Type,
			}
			// Register the unique variable in Defs for downstream emitters
			obj := types.NewVar(lhsVar.Pos(), v.pkg.Types, uniqueVarName, tv.Type)
			v.pkg.TypesInfo.Defs[lhsVar] = obj
			// Also register Uses for any renamed references in the body
			v.registerRenamedUses(bodyCopy, uniqueVarName, obj)
		}
		// Register ok var
		okObj := types.NewVar(okIdent.Pos(), v.pkg.Types, okName, types.Typ[types.Bool])
		v.pkg.TypesInfo.Defs[okIdent] = okObj

		// Register the ok condition reference
		okRef := ast.NewIdent(okName)
		v.pkg.TypesInfo.Uses[okRef] = okObj

		// Build the init: _tsv_N, _tsok_N := x.(T)
		initAssign := &ast.AssignStmt{
			Lhs: []ast.Expr{lhsVar, okIdent},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{typeAssert},
		}

		return &ast.IfStmt{
			Init: initAssign,
			Cond: okRef,
			Body: &ast.BlockStmt{List: bodyCopy},
		}
	}

	// Fallback without TypesInfo (shouldn't happen in practice)
	initAssign := &ast.AssignStmt{
		Lhs: []ast.Expr{lhsVar, okIdent},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{typeAssert},
	}

	okRef := ast.NewIdent(okName)
	return &ast.IfStmt{
		Init: initAssign,
		Cond: okRef,
		Body: &ast.BlockStmt{List: bodyCopy},
	}
}

// renameIdentInStmts renames all occurrences of oldName to newName in the given statements.
func (v *canonicalizeVisitor) renameIdentInStmts(stmts []ast.Stmt, oldName, newName string) {
	for _, stmt := range stmts {
		ast.Inspect(stmt, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok && ident.Name == oldName {
				ident.Name = newName
			}
			return true
		})
	}
}

// registerRenamedUses registers TypesInfo.Uses for all identifiers matching name in the statements.
func (v *canonicalizeVisitor) registerRenamedUses(stmts []ast.Stmt, name string, obj types.Object) {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return
	}
	for _, stmt := range stmts {
		ast.Inspect(stmt, func(n ast.Node) bool {
			if ident, ok := n.(*ast.Ident); ok && ident.Name == name {
				v.pkg.TypesInfo.Uses[ident] = obj
			}
			return true
		})
	}
}
