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
//   - RustOwnership         (Rust move/borrow semantics — 7 sub-patterns)
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
			call.Args = append(call.Args[:idx], sliceLit)
		} else if len(call.Args) == idx {
			// Zero variadic args: insert []T{}
			sliceLit := &ast.CompositeLit{
				Type: &ast.ArrayType{Elt: info.elemType},
			}
			call.Args = append(call.Args, sliceLit)
		}

		return true
	})
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
//   - Blank identifier _
//   - Redeclarations in the same scope (x, err := f(); y, err := g())
//   - Variables that shadow package-level names (only function-local shadowing)
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
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return
	}
	// Single-statement transforms (extract to temp, applied per-block)
	v.transformSelfRefConcat(file)
	v.transformSliceSelfRef(file)
	v.transformSameVarMultipleArgs(file)
	v.transformBinaryExprSameVar(file)
	v.transformNestedCallSharing(file)
	// Multi-statement transforms (track state across a block)
	v.transformStringReuseAfterConcat(file)
	v.transformMultiClosureSameVar(file)
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

// isStringType returns true if expr has type string.
func (v *canonicalizeVisitor) isStringType(expr ast.Expr) bool {
	t := v.exprType(expr)
	return t != nil && t.String() == "string"
}

// isNonCopyType returns true if t is a type that is moved (not copied) in Rust.
// Non-Copy types: string, slices, maps. Primitive types are Copy.
func (v *canonicalizeVisitor) isNonCopyType(t types.Type) bool {
	if t == nil {
		return false
	}
	if t.String() == "string" {
		return true
	}
	if _, ok := t.Underlying().(*types.Slice); ok {
		return true
	}
	if _, ok := t.Underlying().(*types.Map); ok {
		return true
	}
	return false
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

// identIsNonCopy checks if an identifier refers to a non-Copy variable
// (not a constant, function, or type name).
func (v *canonicalizeVisitor) identIsNonCopy(ident *ast.Ident) bool {
	obj := v.pkg.TypesInfo.Uses[ident]
	if obj == nil {
		return false
	}
	if _, isConst := obj.(*types.Const); isConst {
		return false
	}
	if _, isFunc := obj.(*types.Func); isFunc {
		return false
	}
	if _, isTypeName := obj.(*types.TypeName); isTypeName {
		return false
	}
	return v.isNonCopyType(obj.Type())
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

// transformSelfRefConcat rewrites `x += x + a` into `tmp := x + a; x = tmp`.
//
// Pattern matched:
//
//	x += x + a
//
// Why: In Rust, `x += x + a` borrows x mutably (for +=) and also moves x
// (in x + a) in the same expression, which the borrow checker rejects.
//
// Transform:
//
//	x += x + a   →   _t0 := x + a
//	                  x = _t0
//
// Skipped:
//   - += where RHS does not contain LHS variable in a + sub-expression
//   - += on non-string types (only string concat causes moves)
func (v *canonicalizeVisitor) transformSelfRefConcat(file *ast.File) {
	v.applyBlockTransform(file, func(list *[]ast.Stmt) {
		var newList []ast.Stmt
		changed := false
		for _, stmt := range *list {
			assign, ok := stmt.(*ast.AssignStmt)
			if !ok || assign.Tok != token.ADD_ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
				newList = append(newList, stmt)
				continue
			}
			lhsIdent, ok := assign.Lhs[0].(*ast.Ident)
			if !ok || !v.isStringType(assign.Lhs[0]) {
				newList = append(newList, stmt)
				continue
			}
			if !rhsContainsStringConcatWithVar(assign.Rhs[0], lhsIdent.Name) {
				newList = append(newList, stmt)
				continue
			}
			// Transform: x += x + a  →  tmp := x + a; x = tmp
			tmpAssign, tmpObj := v.createTempAssign(assign.Rhs[0], assign.TokPos)
			newList = append(newList, tmpAssign)
			assign.Tok = token.ASSIGN
			assign.Rhs[0] = v.createTempRef(tmpObj, assign.TokPos)
			newList = append(newList, assign)
			changed = true
		}
		if changed {
			*list = newList
		}
	})
}

// rhsContainsStringConcatWithVar checks if an expression contains a binary +
// with varName on the left side.
func rhsContainsStringConcatWithVar(expr ast.Expr, varName string) bool {
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			if ident, ok := e.X.(*ast.Ident); ok && ident.Name == varName {
				return true
			}
		}
		return rhsContainsStringConcatWithVar(e.X, varName) ||
			rhsContainsStringConcatWithVar(e.Y, varName)
	case *ast.ParenExpr:
		return rhsContainsStringConcatWithVar(e.X, varName)
	}
	return false
}

// ---------------------------------------------------------------------------
// 2. Slice self-reference
// ---------------------------------------------------------------------------

// transformSliceSelfRef rewrites slice self-reference patterns that cause
// simultaneous mutable + immutable borrow errors in Rust.
//
// Pattern A — index self-assignment:
//
//	slice[i] = slice[j]
//
// Pattern B — append with self-reference:
//
//	slice = append(slice, slice[i])
//
// Why: In Rust, `slice[i] = slice[j]` requires both &mut slice (for LHS)
// and &slice (for RHS) simultaneously, which the borrow checker rejects.
// Similarly, append mutates the slice while reading an element.
//
// Transform A:
//
//	slice[i] = slice[j]   →   _t0 := slice[j]
//	                           slice[i] = _t0
//
// Transform B:
//
//	slice = append(slice, slice[i])  →  _t0 := slice[i]
//	                                    slice = append(slice, _t0)
//
// Skipped:
//   - Slices of Copy types (int, bool, etc.) — these are safe in Rust
//     (only for pattern A; pattern B always applies due to &mut for append)
func (v *canonicalizeVisitor) transformSliceSelfRef(file *ast.File) {
	v.applyBlockTransform(file, func(list *[]ast.Stmt) {
		var newList []ast.Stmt
		changed := false
		for _, stmt := range *list {
			assign, ok := stmt.(*ast.AssignStmt)
			if !ok {
				newList = append(newList, stmt)
				continue
			}
			prefixes := v.extractSliceSelfRefPrefixes(assign)
			if len(prefixes) > 0 {
				newList = append(newList, prefixes...)
				changed = true
			}
			newList = append(newList, stmt)
		}
		if changed {
			*list = newList
		}
	})
}

// extractSliceSelfRefPrefixes checks an assignment for slice self-reference
// patterns and returns temp variable assignments to insert before it.
// It also modifies the original assignment to use the temp variables.
func (v *canonicalizeVisitor) extractSliceSelfRefPrefixes(assign *ast.AssignStmt) []ast.Stmt {
	var prefixes []ast.Stmt

	for i, lhs := range assign.Lhs {
		if i >= len(assign.Rhs) {
			continue
		}
		rhs := assign.Rhs[i]

		// Pattern B: slice = append(slice, slice[i])
		if lhsIdent, ok := lhs.(*ast.Ident); ok {
			if t := v.exprType(lhs); t != nil {
				if _, isSlice := t.Underlying().(*types.Slice); isSlice {
					if call, ok := rhs.(*ast.CallExpr); ok {
						if funId, ok := call.Fun.(*ast.Ident); ok && funId.Name == "append" && len(call.Args) >= 2 {
							if firstArg, ok := call.Args[0].(*ast.Ident); ok && firstArg.Name == lhsIdent.Name {
								for j := 1; j < len(call.Args); j++ {
									if exprContainsSliceAccess(call.Args[j], lhsIdent.Name) {
										tmpAssign, tmpObj := v.createTempAssign(call.Args[j], assign.TokPos)
										prefixes = append(prefixes, tmpAssign)
										call.Args[j] = v.createTempRef(tmpObj, assign.TokPos)
									}
								}
							}
						}
					}
				}
			}
		}

		// Pattern A: slice[i] = slice[j]
		if lhsIndex, ok := lhs.(*ast.IndexExpr); ok {
			if lhsSlice, ok := lhsIndex.X.(*ast.Ident); ok {
				if t := v.exprType(lhsIndex.X); t != nil {
					if sliceType, isSlice := t.Underlying().(*types.Slice); isSlice {
						// Skip Copy types for pattern A
						if v.isCopyType(sliceType.Elem()) {
							continue
						}
						if exprContainsSliceAccess(rhs, lhsSlice.Name) {
							tmpAssign, tmpObj := v.createTempAssign(rhs, assign.TokPos)
							prefixes = append(prefixes, tmpAssign)
							assign.Rhs[i] = v.createTempRef(tmpObj, assign.TokPos)
						}
					}
				}
			}
		}
	}
	return prefixes
}

// exprContainsSliceAccess checks if an expression contains an index access
// to the named slice variable.
func exprContainsSliceAccess(expr ast.Expr, sliceName string) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		if indexExpr, ok := n.(*ast.IndexExpr); ok {
			if ident, ok := indexExpr.X.(*ast.Ident); ok && ident.Name == sliceName {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// ---------------------------------------------------------------------------
// 3. Same variable as multiple function arguments
// ---------------------------------------------------------------------------

// transformSameVarMultipleArgs rewrites `f(x, x)` where x is non-Copy.
//
// Pattern matched:
//
//	f(x, x)   // same non-Copy variable passed twice
//
// Why: In Rust, passing a non-Copy variable to a function moves it.
// Passing the same variable twice would require two moves, which is invalid.
//
// Transform:
//
//	f(x, x)   →   _t0 := x
//	               f(x, _t0)
//
// Skipped:
//   - Built-in functions (len, append, make, etc.)
//   - Copy types (int, bool, etc.)
//   - Constants and function names
func (v *canonicalizeVisitor) transformSameVarMultipleArgs(file *ast.File) {
	v.applyBlockTransform(file, func(list *[]ast.Stmt) {
		var newList []ast.Stmt
		changed := false
		for _, stmt := range *list {
			prefixes := v.extractDuplicateArgPrefixes(stmt)
			if len(prefixes) > 0 {
				newList = append(newList, prefixes...)
				changed = true
			}
			newList = append(newList, stmt)
		}
		if changed {
			*list = newList
		}
	})
}

// extractDuplicateArgPrefixes finds function calls with duplicate non-Copy
// arguments and extracts duplicates to temp variables.
func (v *canonicalizeVisitor) extractDuplicateArgPrefixes(stmt ast.Stmt) []ast.Stmt {
	var prefixes []ast.Stmt
	ast.Inspect(stmt, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false // skip closures
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if isBuiltinCall(call) {
			return true
		}
		// Find duplicate idents among direct arguments
		seen := make(map[string]int) // name → first index
		for i, arg := range call.Args {
			ident, ok := arg.(*ast.Ident)
			if !ok {
				continue
			}
			if !v.identIsNonCopy(ident) {
				continue
			}
			if _, exists := seen[ident.Name]; exists {
				// Duplicate — extract to temp
				tmpAssign, tmpObj := v.createTempAssign(arg, ident.NamePos)
				prefixes = append(prefixes, tmpAssign)
				call.Args[i] = v.createTempRef(tmpObj, ident.NamePos)
			} else {
				seen[ident.Name] = i
			}
		}
		return true
	})
	return prefixes
}

// ---------------------------------------------------------------------------
// 4. Same variable in binary expression with function calls
// ---------------------------------------------------------------------------

// transformBinaryExprSameVar rewrites `f(x) + g(x)` where x is non-Copy.
//
// Pattern matched:
//
//	f(x) + g(x)   // same non-Copy variable in function calls on both sides
//
// Why: In Rust, x is moved into f(x) and then cannot be used in g(x).
//
// Transform:
//
//	result := f(x) + g(x)   →   _t0 := g(x)
//	                             result := f(x) + _t0
//
// Skipped:
//   - Built-in function calls
//   - Copy types
func (v *canonicalizeVisitor) transformBinaryExprSameVar(file *ast.File) {
	v.applyBlockTransform(file, func(list *[]ast.Stmt) {
		var newList []ast.Stmt
		changed := false
		for _, stmt := range *list {
			prefixes := v.extractBinaryExprSameVarPrefixes(stmt)
			if len(prefixes) > 0 {
				newList = append(newList, prefixes...)
				changed = true
			}
			newList = append(newList, stmt)
		}
		if changed {
			*list = newList
		}
	})
}

func (v *canonicalizeVisitor) extractBinaryExprSameVarPrefixes(stmt ast.Stmt) []ast.Stmt {
	var prefixes []ast.Stmt
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok {
		return nil
	}
	for idx, rhs := range assign.Rhs {
		ast.Inspect(rhs, func(n ast.Node) bool {
			if _, ok := n.(*ast.FuncLit); ok {
				return false
			}
			binExpr, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			leftArgs := getDirectFuncCallArgs(binExpr.X)
			rightArgs := getDirectFuncCallArgs(binExpr.Y)
			for varName := range leftArgs {
				if rightArgs[varName] {
					// Find the ident on the left to check its type
					var foundIdent *ast.Ident
					ast.Inspect(binExpr.X, func(inner ast.Node) bool {
						if id, ok := inner.(*ast.Ident); ok && id.Name == varName {
							foundIdent = id
							return false
						}
						return true
					})
					if foundIdent != nil && v.identIsNonCopy(foundIdent) {
						// Extract right side to temp
						tmpAssign, tmpObj := v.createTempAssign(binExpr.Y, binExpr.OpPos)
						prefixes = append(prefixes, tmpAssign)
						binExpr.Y = v.createTempRef(tmpObj, binExpr.OpPos)
						// Update the RHS in the parent assign
						assign.Rhs[idx] = rhs
						return false // done with this binary expr
					}
				}
			}
			return true
		})
	}
	return prefixes
}

// getDirectFuncCallArgs returns variable names passed as direct arguments to
// a function call. Returns empty if expr is not a function call or is built-in.
func getDirectFuncCallArgs(expr ast.Expr) map[string]bool {
	args := make(map[string]bool)
	call, ok := expr.(*ast.CallExpr)
	if !ok || isBuiltinCall(call) {
		return args
	}
	for _, arg := range call.Args {
		if ident, ok := arg.(*ast.Ident); ok {
			args[ident.Name] = true
		}
	}
	return args
}

// ---------------------------------------------------------------------------
// 5. Nested function calls sharing non-Copy variable
// ---------------------------------------------------------------------------

// transformNestedCallSharing rewrites `f(x, g(x))` where x is non-Copy.
//
// Pattern matched:
//
//	f(x, g(x))   // x passed to outer call and also used in nested call
//
// Why: In Rust, x is moved into the outer function. Using x again in the
// nested call argument requires a second move, which is invalid.
//
// Transform:
//
//	f(x, g(x))   →   _t0 := g(x)
//	                  f(x, _t0)
//
// Skipped:
//   - Built-in functions (len, append, etc.)
//   - Copy types
func (v *canonicalizeVisitor) transformNestedCallSharing(file *ast.File) {
	v.applyBlockTransform(file, func(list *[]ast.Stmt) {
		var newList []ast.Stmt
		changed := false
		for _, stmt := range *list {
			prefixes := v.extractNestedCallPrefixes(stmt)
			if len(prefixes) > 0 {
				newList = append(newList, prefixes...)
				changed = true
			}
			newList = append(newList, stmt)
		}
		if changed {
			*list = newList
		}
	})
}

func (v *canonicalizeVisitor) extractNestedCallPrefixes(stmt ast.Stmt) []ast.Stmt {
	var prefixes []ast.Stmt
	ast.Inspect(stmt, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok || isBuiltinCall(call) {
			return true
		}
		// Collect direct identifier arguments at top level
		topLevelArgs := make(map[string]*ast.Ident)
		for _, arg := range call.Args {
			if ident, ok := arg.(*ast.Ident); ok {
				topLevelArgs[ident.Name] = ident
			}
		}
		// Check if any nested call uses the same variable
		for i, arg := range call.Args {
			nestedCall, ok := arg.(*ast.CallExpr)
			if !ok || isBuiltinCall(nestedCall) {
				continue
			}
			nestedArgs := collectDirectCallArgIdents(nestedCall)
			for _, nestedIdent := range nestedArgs {
				if topIdent, exists := topLevelArgs[nestedIdent.Name]; exists {
					if v.identIsNonCopy(topIdent) {
						// Extract the nested call to a temp
						tmpAssign, tmpObj := v.createTempAssign(arg, nestedIdent.NamePos)
						prefixes = append(prefixes, tmpAssign)
						call.Args[i] = v.createTempRef(tmpObj, nestedIdent.NamePos)
						return false
					}
				}
			}
		}
		return true
	})
	return prefixes
}

// collectDirectCallArgIdents collects identifiers that are passed directly
// as arguments to function calls within an expression.
func collectDirectCallArgIdents(expr ast.Expr) []*ast.Ident {
	var idents []*ast.Ident
	ast.Inspect(expr, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			for _, arg := range call.Args {
				if ident, ok := arg.(*ast.Ident); ok {
					idents = append(idents, ident)
				}
			}
		}
		return true
	})
	return idents
}

// isBuiltinCall checks if a call expression is a built-in function call.
func isBuiltinCall(call *ast.CallExpr) bool {
	ident, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	builtins := map[string]bool{
		"len": true, "cap": true, "append": true, "copy": true,
		"make": true, "new": true, "delete": true, "close": true,
		"panic": true, "recover": true, "print": true, "println": true,
		"complex": true, "real": true, "imag": true,
		"min": true, "max": true, "clear": true,
		"int": true, "int8": true, "int16": true, "int32": true, "int64": true,
		"uint": true, "uint8": true, "uint16": true, "uint32": true, "uint64": true,
		"string": true, "byte": true, "rune": true,
	}
	return builtins[ident.Name]
}

// ---------------------------------------------------------------------------
// 6. String reuse after concatenation
// ---------------------------------------------------------------------------

// transformStringReuseAfterConcat inserts a copy of a string variable
// before its first use as a concatenation operand, so that the copy is
// available for subsequent uses.
//
// Pattern matched:
//
//	y := x + a   // x consumed (moved) by +
//	z := x + b   // x used again — invalid in Rust
//
// Why: In Rust, `x + a` moves x. A second use of x after the move is a
// compile error.
//
// Transform:
//
//	y := x + a      →   x_copy := x
//	z := x + b          y := x + a
//	                     z := x_copy + b
//
// Skipped:
//   - Variables reassigned between uses (the reassignment clears "consumed" state)
//   - Non-string types
func (v *canonicalizeVisitor) transformStringReuseAfterConcat(file *ast.File) {
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			continue
		}
		v.stringReuseInBlock(funcDecl.Body)
	}
}

func (v *canonicalizeVisitor) stringReuseInBlock(block *ast.BlockStmt) {
	// Recurse into nested blocks first
	for _, stmt := range block.List {
		v.stringReuseRecurse(stmt)
	}

	// Pass 1: Find string variables consumed by + in multiple statements.
	// Track consumption point (statement index) per variable.
	type useRecord struct {
		stmtIdx int
		ident   *ast.Ident // the ident node in the expression
	}
	varUses := make(map[string][]useRecord)

	for i, stmt := range block.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok {
			continue
		}
		// Track string vars on the left of + in RHS expressions
		for _, rhs := range assign.Rhs {
			v.findStringConcatLeftIdents(rhs, func(ident *ast.Ident) {
				varUses[ident.Name] = append(varUses[ident.Name], useRecord{i, ident})
			})
		}
		// Clear tracking for variables reassigned on the LHS
		for _, lhs := range assign.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				delete(varUses, ident.Name)
			}
		}
	}

	// Pass 2: For variables used more than once, insert copy and rename later uses.
	type copyInsert struct {
		beforeIdx int    // insert copy before this statement index
		varName   string // variable to copy
		copyName  string // name of the copy variable
		copyObj   types.Object
	}
	var inserts []copyInsert

	for varName, uses := range varUses {
		if len(uses) <= 1 {
			continue
		}
		// Get the type of the variable
		var varType types.Type
		if obj := v.pkg.TypesInfo.Uses[uses[0].ident]; obj != nil {
			varType = obj.Type()
		}
		if varType == nil {
			continue
		}

		copyName := varName + "_copy"
		copyDefIdent := &ast.Ident{Name: copyName, NamePos: uses[0].ident.NamePos}
		copyObj := types.NewVar(uses[0].ident.NamePos, v.pkg.Types, copyName, varType)
		v.pkg.TypesInfo.Defs[copyDefIdent] = copyObj

		// Insert copy before the first use
		inserts = append(inserts, copyInsert{
			beforeIdx: uses[0].stmtIdx,
			varName:   varName,
			copyName:  copyName,
			copyObj:   copyObj,
		})

		// Rename all subsequent uses (uses[1:]) to use the copy
		for _, use := range uses[1:] {
			use.ident.Name = copyName
			v.pkg.TypesInfo.Uses[use.ident] = copyObj
		}
	}

	if len(inserts) == 0 {
		return
	}

	// Build new list with copy statements inserted
	insertMap := make(map[int][]ast.Stmt) // stmtIdx → stmts to insert before
	for _, ins := range inserts {
		// Create: varName_copy := varName
		origRef := &ast.Ident{Name: ins.varName, NamePos: block.List[ins.beforeIdx].Pos()}
		if obj := v.pkg.TypesInfo.Uses[origRef]; obj == nil {
			// Try to find the original variable's object
			for _, use := range varUses[ins.varName] {
				if origObj := v.pkg.TypesInfo.Uses[use.ident]; origObj != nil {
					v.pkg.TypesInfo.Uses[origRef] = origObj
					break
				}
			}
		}
		copyAssign := &ast.AssignStmt{
			Lhs:    []ast.Expr{&ast.Ident{Name: ins.copyName, NamePos: origRef.NamePos}},
			Tok:    token.DEFINE,
			TokPos: origRef.NamePos,
			Rhs:    []ast.Expr{origRef},
		}
		// Register the def ident
		v.pkg.TypesInfo.Defs[copyAssign.Lhs[0].(*ast.Ident)] = ins.copyObj

		insertMap[ins.beforeIdx] = append(insertMap[ins.beforeIdx], copyAssign)
	}

	var newList []ast.Stmt
	for i, stmt := range block.List {
		if stmts, ok := insertMap[i]; ok {
			newList = append(newList, stmts...)
		}
		newList = append(newList, stmt)
	}
	block.List = newList
}

// stringReuseRecurse walks into nested blocks for string reuse processing.
func (v *canonicalizeVisitor) stringReuseRecurse(stmt ast.Stmt) {
	switch s := stmt.(type) {
	case *ast.BlockStmt:
		v.stringReuseInBlock(s)
	case *ast.IfStmt:
		v.stringReuseInBlock(s.Body)
		if s.Else != nil {
			v.stringReuseRecurse(s.Else)
		}
	case *ast.ForStmt:
		v.stringReuseInBlock(s.Body)
	case *ast.RangeStmt:
		v.stringReuseInBlock(s.Body)
	case *ast.SwitchStmt:
		v.stringReuseInBlock(s.Body)
	}
}

// findStringConcatLeftIdents finds all identifiers that appear as the LEFT
// operand of a binary + expression where the operand has string type.
func (v *canonicalizeVisitor) findStringConcatLeftIdents(expr ast.Expr, fn func(*ast.Ident)) {
	ast.Inspect(expr, func(n ast.Node) bool {
		binExpr, ok := n.(*ast.BinaryExpr)
		if !ok || binExpr.Op != token.ADD {
			return true
		}
		if ident, ok := binExpr.X.(*ast.Ident); ok {
			if v.isStringType(binExpr.X) {
				fn(ident)
			}
		}
		return true
	})
}

// ---------------------------------------------------------------------------
// 7. Multiple closures capturing the same non-Copy variable
// ---------------------------------------------------------------------------

// transformMultiClosureSameVar inserts a copy of a variable before the
// second closure that captures it.
//
// Pattern matched:
//
//	fn1 := func() { use(x) }  // first closure captures x
//	fn2 := func() { use(x) }  // second closure also captures x
//
// Why: In Rust, closures capture variables by move (for non-Copy types).
// The first closure moves x, leaving it unavailable for the second.
//
// Transform:
//
//	fn1 := func() { use(x) }   →   fn1 := func() { use(x) }
//	fn2 := func() { use(x) }       x_copy := x
//	                                fn2 := func() { use(x_copy) }
//
// Skipped:
//   - Copy types (int, bool, etc.)
//   - Constants and function names
func (v *canonicalizeVisitor) transformMultiClosureSameVar(file *ast.File) {
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Body == nil {
			continue
		}
		v.multiClosureInBlock(funcDecl.Body, funcDecl.Pos())
	}
}

func (v *canonicalizeVisitor) multiClosureInBlock(block *ast.BlockStmt, funcStart token.Pos) {
	// Track which non-Copy variables have been captured by closures.
	// varName → first capture position
	captured := make(map[string]token.Pos)

	// We need to process each statement and check for FuncLit nodes.
	// If a FuncLit captures a variable that was already captured by a
	// previous FuncLit, we insert a copy before this statement.
	type copyInfo struct {
		stmtIdx  int
		varName  string
		copyName string
		copyObj  types.Object
		idents   []*ast.Ident // idents inside the closure to rename
	}
	var copies []copyInfo

	for i, stmt := range block.List {
		// Find all FuncLit nodes in this statement
		ast.Inspect(stmt, func(n ast.Node) bool {
			funcLit, ok := n.(*ast.FuncLit)
			if !ok {
				return true
			}
			// Collect identifiers used in the closure body
			closureIdents := v.collectCapturedIdents(funcLit, funcStart)
			processedInClosure := make(map[string]bool)
			for _, ident := range closureIdents {
				if processedInClosure[ident.Name] {
					continue
				}
				processedInClosure[ident.Name] = true

				if _, wasCaptured := captured[ident.Name]; wasCaptured {
					// This variable is captured by multiple closures
					// Get its type for the copy
					obj := v.pkg.TypesInfo.Uses[ident]
					if obj == nil {
						continue
					}
					copyName := ident.Name + "_copy"
					copyObj := types.NewVar(ident.NamePos, v.pkg.Types, copyName, obj.Type())

					// Collect all idents of this variable in the closure body to rename
					var toRename []*ast.Ident
					ast.Inspect(funcLit.Body, func(inner ast.Node) bool {
						if id, ok := inner.(*ast.Ident); ok && id.Name == ident.Name {
							if innerObj := v.pkg.TypesInfo.Uses[id]; innerObj != nil {
								if innerObj == obj {
									toRename = append(toRename, id)
								}
							}
						}
						return true
					})

					copies = append(copies, copyInfo{
						stmtIdx:  i,
						varName:  ident.Name,
						copyName: copyName,
						copyObj:  copyObj,
						idents:   toRename,
					})
				} else {
					captured[ident.Name] = funcLit.Pos()
				}
			}
			return false // don't recurse into nested closures
		})
	}

	if len(copies) == 0 {
		return
	}

	// Apply renames inside closures
	for _, cp := range copies {
		for _, ident := range cp.idents {
			ident.Name = cp.copyName
			v.pkg.TypesInfo.Uses[ident] = cp.copyObj
		}
	}

	// Build new list with copy statements inserted before affected closures
	insertMap := make(map[int][]ast.Stmt)
	for _, cp := range copies {
		origRef := &ast.Ident{Name: cp.varName, NamePos: block.List[cp.stmtIdx].Pos()}
		// Try to find the original variable's object
		if obj := v.pkg.TypesInfo.ObjectOf(origRef); obj != nil {
			v.pkg.TypesInfo.Uses[origRef] = obj
		}
		copyDefIdent := &ast.Ident{Name: cp.copyName, NamePos: origRef.NamePos}
		v.pkg.TypesInfo.Defs[copyDefIdent] = cp.copyObj

		copyAssign := &ast.AssignStmt{
			Lhs:    []ast.Expr{copyDefIdent},
			Tok:    token.DEFINE,
			TokPos: origRef.NamePos,
			Rhs:    []ast.Expr{origRef},
		}
		insertMap[cp.stmtIdx] = append(insertMap[cp.stmtIdx], copyAssign)
	}

	var newList []ast.Stmt
	for i, stmt := range block.List {
		if stmts, ok := insertMap[i]; ok {
			newList = append(newList, stmts...)
		}
		newList = append(newList, stmt)
	}
	block.List = newList
}

// collectCapturedIdents returns non-Copy identifiers used in a closure body
// that were declared before the closure (i.e., are captures from an outer scope).
func (v *canonicalizeVisitor) collectCapturedIdents(funcLit *ast.FuncLit, funcStart token.Pos) []*ast.Ident {
	var result []*ast.Ident
	// Track identifiers that are selector bases or field names (not captures)
	selectorBases := make(map[*ast.Ident]bool)
	selectorFields := make(map[*ast.Ident]bool)
	ast.Inspect(funcLit.Body, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				selectorBases[id] = true
			}
			selectorFields[sel.Sel] = true
		}
		return true
	})

	ast.Inspect(funcLit.Body, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || selectorBases[ident] || selectorFields[ident] {
			return true
		}
		obj := v.pkg.TypesInfo.Uses[ident]
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
		if !v.isNonCopyType(obj.Type()) {
			return true
		}
		// Check if declared before this closure (outer scope capture)
		if obj.Pos() < funcLit.Pos() && obj.Pos() >= funcStart {
			result = append(result, ident)
		}
		return true
	})
	return result
}
