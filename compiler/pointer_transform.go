package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// PtrLocalComments maps the token.Pos of ptrLocal LHS idents to comment strings.
// Populated by the transform, consumed by emitters to emit comments instead of code.
var PtrLocalComments = map[token.Pos]string{}

// PointerTransformPass transforms pointer operations into pool-based indexing
// for cross-backend compatibility. This runs between SemaChecker and emitter passes.
type PointerTransformPass struct{}

func (p *PointerTransformPass) Name() string { return "PointerTransform" }
func (p *PointerTransformPass) ProLog()      {}
func (p *PointerTransformPass) EpiLog()      {}

func (p *PointerTransformPass) Visitors(pkg *packages.Package) []ast.Visitor {
	return []ast.Visitor{&ptrTransformVisitor{pkg: pkg}}
}

func (p *PointerTransformPass) PreVisit(visitor ast.Visitor) {}

func (p *PointerTransformPass) PostVisit(visitor ast.Visitor, visited map[string]struct{}) {
	v := visitor.(*ptrTransformVisitor)
	v.transform()
}

// ptrTransformVisitor collects function declarations during AST walk
type ptrTransformVisitor struct {
	pkg         *packages.Package
	funcs       []*ast.FuncDecl
	genDecls    []*ast.GenDecl
	poolTypes   map[string]bool     // type names that are *T targets (e.g. "ListNode")
	ptrFieldMap map[string][]string // struct name → list of pointer field names
	tmpCounter  int                 // counter for generating unique temp variable names
}

func (v *ptrTransformVisitor) Visit(node ast.Node) ast.Visitor {
	if fd, ok := node.(*ast.FuncDecl); ok {
		v.funcs = append(v.funcs, fd)
	}
	if gd, ok := node.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
		v.genDecls = append(v.genDecls, gd)
	}
	return v
}

// ptrLocalInfo holds info about a local pointer alias (p := &x)
type ptrLocalInfo struct {
	targetName string     // the boxed variable this aliases (e.g., "x")
	elemType   types.Type // element type of the pointer
}

// ptrIndexLocalInfo holds info about a local pointer alias to a slice element (p := &arr[i])
type ptrIndexLocalInfo struct {
	sliceExpr ast.Expr   // e.g., ident "arr"
	indexExpr ast.Expr   // e.g., literal "1" or ident "i"
	elemType  types.Type // element type of the indexed expression
}

// ptrAnalysis holds analysis results for a single function
type ptrAnalysis struct {
	ptrParams       map[string]types.Type         // param name -> element type (for *T params)
	ptrVars         map[string]types.Type         // var p *T declarations -> element type
	poolVars        map[string]types.Type         // all vars whose addr is taken (pool index vars)
	ptrLocals       map[string]*ptrLocalInfo      // alias name -> target info (p := &x)
	ptrIndexLocals  map[string]*ptrIndexLocalInfo // alias name -> target info (p := &arr[i])
	ptrReturns      map[int]types.Type            // result index → element type (for *T returns)
	callReturnPools map[string]types.Type         // pool types needed for calling ptr-returning funcs
}

// poolNameForType returns the pool variable name for a given element type
func poolNameForType(t types.Type) string {
	if named, ok := t.(*types.Named); ok {
		return "_pool_" + named.Obj().Name()
	}
	if basic, ok := t.(*types.Basic); ok {
		return "_pool_" + basic.Name()
	}
	return "_pool_unknown"
}

// poolIndexExpr creates _pool_T[index] with proper type registration
func (v *ptrTransformVisitor) poolIndexExpr(index ast.Expr, elemType types.Type) *ast.IndexExpr {
	poolIdent := &ast.Ident{Name: poolNameForType(elemType)}
	v.registerType(poolIdent, types.NewSlice(elemType))
	indexExpr := &ast.IndexExpr{X: poolIdent, Index: index}
	v.registerType(indexExpr, elemType)
	return indexExpr
}

func (v *ptrTransformVisitor) transform() {
	// Rewrite struct types first: *T fields -> []T fields
	v.rewriteStructTypes()

	for _, fd := range v.funcs {
		analysis := v.analyzeFuncDecl(fd)
		if len(analysis.ptrParams) == 0 && len(analysis.ptrVars) == 0 && len(analysis.poolVars) == 0 &&
			len(analysis.ptrLocals) == 0 && len(analysis.ptrIndexLocals) == 0 &&
			len(analysis.ptrReturns) == 0 && len(analysis.callReturnPools) == 0 {
			continue
		}
		v.rewriteFuncDecl(fd, analysis)
	}
}

// rewriteStructTypes rewrites pointer fields (*T) in struct type declarations to int (pool index).
func (v *ptrTransformVisitor) rewriteStructTypes() {
	v.poolTypes = make(map[string]bool)
	v.ptrFieldMap = make(map[string][]string)
	for _, gd := range v.genDecls {
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range st.Fields.List {
				if starExpr, ok := field.Type.(*ast.StarExpr); ok {
					// Record pool type info
					if ident, ok := starExpr.X.(*ast.Ident); ok {
						v.poolTypes[ident.Name] = true
						for _, fname := range field.Names {
							v.ptrFieldMap[ts.Name.Name] = append(v.ptrFieldMap[ts.Name.Name], fname.Name)
						}
					}
					// Rewrite *T → int
					newType := &ast.Ident{Name: "int"}
					field.Type = newType
					v.registerType(newType, types.Typ[types.Int])
				}
			}
		}
	}
}

func (v *ptrTransformVisitor) analyzeFuncDecl(fd *ast.FuncDecl) *ptrAnalysis {
	result := &ptrAnalysis{
		ptrParams:       make(map[string]types.Type),
		ptrVars:         make(map[string]types.Type),
		poolVars:        make(map[string]types.Type),
		ptrLocals:       make(map[string]*ptrLocalInfo),
		ptrIndexLocals:  make(map[string]*ptrIndexLocalInfo),
		ptrReturns:      make(map[int]types.Type),
		callReturnPools: make(map[string]types.Type),
	}

	// Find pointer returns
	if fd.Type.Results != nil {
		for i, field := range fd.Type.Results.List {
			if starExpr, ok := field.Type.(*ast.StarExpr); ok {
				if v.pkg != nil && v.pkg.TypesInfo != nil {
					if tv, ok := v.pkg.TypesInfo.Types[starExpr.X]; ok {
						result.ptrReturns[i] = tv.Type
					}
				}
			}
		}
	}

	// Find pointer params
	if fd.Type.Params != nil {
		for _, field := range fd.Type.Params.List {
			if starExpr, ok := field.Type.(*ast.StarExpr); ok {
				if v.pkg != nil && v.pkg.TypesInfo != nil {
					if tv, ok := v.pkg.TypesInfo.Types[starExpr.X]; ok {
						for _, name := range field.Names {
							result.ptrParams[name.Name] = tv.Type
						}
					}
				}
			}
		}
	}

	// Find ptrVars: var p *T declarations
	if fd.Body != nil {
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			declStmt, ok := n.(*ast.DeclStmt)
			if !ok {
				return true
			}
			genDecl, ok := declStmt.Decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.VAR {
				return true
			}
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				starExpr, ok := valueSpec.Type.(*ast.StarExpr)
				if !ok {
					continue
				}
				if v.pkg != nil && v.pkg.TypesInfo != nil {
					if tv, ok := v.pkg.TypesInfo.Types[starExpr.X]; ok {
						for _, name := range valueSpec.Names {
							result.ptrVars[name.Name] = tv.Type
						}
					}
				}
			}
			return true
		})
	}

	// Find pool vars (locals whose address is taken via &) — all go to poolVars
	if fd.Body != nil {
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			unary, ok := n.(*ast.UnaryExpr)
			if !ok || unary.Op != token.AND {
				return true
			}
			ident, ok := unary.X.(*ast.Ident)
			if !ok {
				return true
			}
			// Skip pointer params (handled separately)
			if _, isParam := result.ptrParams[ident.Name]; isParam {
				return true
			}
			// Skip ptrVars (the target of &ptrVar is handled differently)
			if _, isPtrVar := result.ptrVars[ident.Name]; isPtrVar {
				return true
			}
			// Get the type of the variable
			if v.pkg != nil && v.pkg.TypesInfo != nil {
				if obj := v.pkg.TypesInfo.Uses[ident]; obj != nil {
					result.poolVars[ident.Name] = obj.Type()
				}
			}
			return true
		})

		// Find ptrLocals: p := &x where x is a boxed variable
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || assign.Tok != token.DEFINE {
				return true
			}
			for i, rhs := range assign.Rhs {
				unary, ok := rhs.(*ast.UnaryExpr)
				if !ok || unary.Op != token.AND {
					continue
				}
				rhsIdent, ok := unary.X.(*ast.Ident)
				if !ok {
					continue
				}
				if elemType, isBoxed := result.poolVars[rhsIdent.Name]; isBoxed {
					if i < len(assign.Lhs) {
						if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
							result.ptrLocals[lhsIdent.Name] = &ptrLocalInfo{
								targetName: rhsIdent.Name,
								elemType:   elemType,
							}
							PtrLocalComments[lhsIdent.Pos()] = fmt.Sprintf(
								"// %s := &%s  (pointer alias to %s, eliminated)",
								lhsIdent.Name, rhsIdent.Name, rhsIdent.Name)
						}
					}
				}
			}
			return true
		})

		// Find ptrVar assignments: p = &x where p is a ptrVar and x is a boxed variable
		// Convert these to ptrLocals (alias elimination) for cross-backend compatibility
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || assign.Tok != token.ASSIGN {
				return true
			}
			for i, rhs := range assign.Rhs {
				unary, ok := rhs.(*ast.UnaryExpr)
				if !ok || unary.Op != token.AND {
					continue
				}
				rhsIdent, ok := unary.X.(*ast.Ident)
				if !ok {
					continue
				}
				if i < len(assign.Lhs) {
					if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
						if elemType, isPtrVar := result.ptrVars[lhsIdent.Name]; isPtrVar {
							// x (the target) must be boxed
							if _, isBoxed := result.poolVars[rhsIdent.Name]; isBoxed {
								result.ptrLocals[lhsIdent.Name] = &ptrLocalInfo{
									targetName: rhsIdent.Name,
									elemType:   elemType,
								}
							}
						}
					}
				}
			}
			return true
		})

		// Find ptrIndexLocals: p := &arr[i] where arr is a slice
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || assign.Tok != token.DEFINE {
				return true
			}
			for i, rhs := range assign.Rhs {
				unary, ok := rhs.(*ast.UnaryExpr)
				if !ok || unary.Op != token.AND {
					continue
				}
				indexExpr, ok := unary.X.(*ast.IndexExpr)
				if !ok {
					continue
				}
				if i < len(assign.Lhs) {
					if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
						// Get element type from the IndexExpr
						if v.pkg != nil && v.pkg.TypesInfo != nil {
							if tv, ok := v.pkg.TypesInfo.Types[indexExpr]; ok {
								result.ptrIndexLocals[lhsIdent.Name] = &ptrIndexLocalInfo{
									sliceExpr: indexExpr.X,
									indexExpr: indexExpr.Index,
									elemType:  tv.Type,
								}
								PtrLocalComments[lhsIdent.Pos()] = fmt.Sprintf(
									"// %s := &%s[...]  (pointer alias to slice element, eliminated)",
									lhsIdent.Name, exprToString(indexExpr.X))
							}
						}
					}
				}
			}
			return true
		})

		// Find ptrVar assignments to index expressions: p = &arr[i] where p is a ptrVar
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || assign.Tok != token.ASSIGN {
				return true
			}
			for i, rhs := range assign.Rhs {
				unary, ok := rhs.(*ast.UnaryExpr)
				if !ok || unary.Op != token.AND {
					continue
				}
				indexExpr, ok := unary.X.(*ast.IndexExpr)
				if !ok {
					continue
				}
				if i < len(assign.Lhs) {
					if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
						if _, isPtrVar := result.ptrVars[lhsIdent.Name]; isPtrVar {
							if v.pkg != nil && v.pkg.TypesInfo != nil {
								if tv, ok := v.pkg.TypesInfo.Types[indexExpr]; ok {
									result.ptrIndexLocals[lhsIdent.Name] = &ptrIndexLocalInfo{
										sliceExpr: indexExpr.X,
										indexExpr: indexExpr.Index,
										elemType:  tv.Type,
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

	// Find calls to functions with *T returns (need pool declarations in caller)
	if fd.Body != nil && v.pkg != nil && v.pkg.TypesInfo != nil {
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			funIdent, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			obj := v.pkg.TypesInfo.Uses[funIdent]
			if obj == nil {
				return true
			}
			fnType, ok := obj.Type().(*types.Signature)
			if !ok {
				return true
			}
			results := fnType.Results()
			for i := 0; i < results.Len(); i++ {
				if ptrType, isPtr := results.At(i).Type().(*types.Pointer); isPtr {
					elemType := ptrType.Elem()
					pn := poolNameForType(elemType)
					result.callReturnPools[pn] = elemType
				}
			}
			return true
		})
	}

	return result
}


func (v *ptrTransformVisitor) rewriteFuncDecl(fd *ast.FuncDecl, analysis *ptrAnalysis) {
	// Track pool types from params (these already have pool passed in, no local decl needed)
	poolParamTypes := make(map[string]types.Type) // pool name → elem type

	// Rewrite param types: *T -> int, and collect pool types from params
	if fd.Type.Params != nil {
		for _, field := range fd.Type.Params.List {
			if starExpr, ok := field.Type.(*ast.StarExpr); ok {
				// Get elem type before rewriting
				var elemType types.Type
				if v.pkg != nil && v.pkg.TypesInfo != nil {
					if elemTV, ok := v.pkg.TypesInfo.Types[starExpr.X]; ok {
						elemType = elemTV.Type
					}
				}
				// *T → int
				newType := &ast.Ident{Name: "int"}
				field.Type = newType
				v.registerType(newType, types.Typ[types.Int])
				if elemType != nil {
					pn := poolNameForType(elemType)
					poolParamTypes[pn] = elemType
				}
			}
		}
		// Append _pool_T []T params at end for each unique pool type from pointer params
		for poolName, elemType := range poolParamTypes {
			typeExpr := v.typeToExpr(elemType)
			arrType := &ast.ArrayType{Elt: typeExpr}
			sliceType := types.NewSlice(elemType)
			v.registerType(arrType, sliceType)
			fd.Type.Params.List = append(fd.Type.Params.List, &ast.Field{
				Names: []*ast.Ident{{Name: poolName}},
				Type:  arrType,
			})
		}
	}

	// Rewrite return types: *T → int, and add pool returns
	if fd.Type.Results != nil && len(analysis.ptrReturns) > 0 {
		returnPoolTypes := make(map[string]types.Type)
		for _, field := range fd.Type.Results.List {
			if starExpr, ok := field.Type.(*ast.StarExpr); ok {
				var elemType types.Type
				if v.pkg != nil && v.pkg.TypesInfo != nil {
					if tv, ok := v.pkg.TypesInfo.Types[starExpr.X]; ok {
						elemType = tv.Type
					}
				}
				// *T → int
				newType := &ast.Ident{Name: "int"}
				field.Type = newType
				v.registerType(newType, types.Typ[types.Int])
				if elemType != nil {
					pn := poolNameForType(elemType)
					returnPoolTypes[pn] = elemType
				}
			}
		}
		// Append []T returns for each pool type from pointer returns
		for _, elemType := range returnPoolTypes {
			typeExpr := v.typeToExpr(elemType)
			arrType := &ast.ArrayType{Elt: typeExpr}
			sliceType := types.NewSlice(elemType)
			v.registerType(arrType, sliceType)
			fd.Type.Results.List = append(fd.Type.Results.List, &ast.Field{
				Type: arrType,
			})
		}
		// Add pool params for return types (dedup with param pools)
		for poolName, elemType := range returnPoolTypes {
			if _, fromParam := poolParamTypes[poolName]; !fromParam {
				typeExpr := v.typeToExpr(elemType)
				arrType := &ast.ArrayType{Elt: typeExpr}
				sliceType := types.NewSlice(elemType)
				v.registerType(arrType, sliceType)
				if fd.Type.Params == nil {
					fd.Type.Params = &ast.FieldList{}
				}
				fd.Type.Params.List = append(fd.Type.Params.List, &ast.Field{
					Names: []*ast.Ident{{Name: poolName}},
					Type:  arrType,
				})
				poolParamTypes[poolName] = elemType
			}
		}
	}

	// Rewrite body
	if fd.Body != nil {
		v.rewriteBlockStmt(fd.Body, analysis)
	}

	// Pool declarations + parameter boxing: combined so pool decls come first
	// Prepend order: [pool decls] [boxing stmts] [original body]
	if fd.Body != nil {
		var preamble []ast.Stmt

		// 1. Pool declarations for types not provided via params
		if len(analysis.poolVars) > 0 {
			poolTypesNeeded := make(map[string]types.Type)
			for _, elemType := range analysis.poolVars {
				pn := poolNameForType(elemType)
				if _, fromParam := poolParamTypes[pn]; !fromParam {
					poolTypesNeeded[pn] = elemType
				}
			}
			for poolName, elemType := range poolTypesNeeded {
				typeExpr := v.typeToExpr(elemType)
				arrType := &ast.ArrayType{Elt: typeExpr}
				sliceType := types.NewSlice(elemType)
				v.registerType(arrType, sliceType)
				decl := &ast.DeclStmt{Decl: &ast.GenDecl{
					Tok: token.VAR,
					Specs: []ast.Spec{&ast.ValueSpec{
						Names: []*ast.Ident{{Name: poolName}},
						Type:  arrType,
					}},
				}}
				preamble = append(preamble, decl)
			}
		}

		// 2. Pool declarations for types needed by pointer-returning function calls
		if len(analysis.callReturnPools) > 0 {
			// Collect already-declared pool names
			alreadyDeclared := make(map[string]bool)
			for pn := range poolParamTypes {
				alreadyDeclared[pn] = true
			}
			for _, elemType := range analysis.poolVars {
				pn := poolNameForType(elemType)
				if _, fromParam := poolParamTypes[pn]; !fromParam {
					alreadyDeclared[pn] = true
				}
			}
			for poolName, elemType := range analysis.callReturnPools {
				if alreadyDeclared[poolName] {
					continue
				}
				alreadyDeclared[poolName] = true
				typeExpr := v.typeToExpr(elemType)
				arrType := &ast.ArrayType{Elt: typeExpr}
				sliceType := types.NewSlice(elemType)
				v.registerType(arrType, sliceType)
				decl := &ast.DeclStmt{Decl: &ast.GenDecl{
					Tok: token.VAR,
					Specs: []ast.Spec{&ast.ValueSpec{
						Names: []*ast.Ident{{Name: poolName}},
						Type:  arrType,
					}},
				}}
				preamble = append(preamble, decl)
			}
		}

		// 3. Parameter boxing for non-pointer params whose address is taken
		if fd.Type.Params != nil {
			for _, field := range fd.Type.Params.List {
				for _, name := range field.Names {
					if elemType, isPool := analysis.poolVars[name.Name]; isPool {
						origName := name.Name
						prefixedName := "__" + origName
						name.Name = prefixedName

						pn := poolNameForType(elemType)
						sliceType := types.NewSlice(elemType)

						// _pool_T = append(_pool_T, __origName)
						poolIdent1 := &ast.Ident{Name: pn}
						poolIdent2 := &ast.Ident{Name: pn}
						paramRef := &ast.Ident{Name: prefixedName}
						appendCall := &ast.CallExpr{
							Fun:  &ast.Ident{Name: "append"},
							Args: []ast.Expr{poolIdent1, paramRef},
						}
						v.registerType(poolIdent1, sliceType)
						v.registerType(poolIdent2, sliceType)
						v.registerType(appendCall, sliceType)
						v.registerType(paramRef, elemType)
						preamble = append(preamble, &ast.AssignStmt{
							Lhs: []ast.Expr{poolIdent2},
							Tok: token.ASSIGN,
							Rhs: []ast.Expr{appendCall},
						})

						// origName := int(len(_pool_T) - 1)
						poolIdent3 := &ast.Ident{Name: pn}
						v.registerType(poolIdent3, sliceType)
						lenCall := &ast.CallExpr{
							Fun:  &ast.Ident{Name: "len"},
							Args: []ast.Expr{poolIdent3},
						}
						v.registerType(lenCall, types.Typ[types.Int])
						oneLit := &ast.BasicLit{Kind: token.INT, Value: "1"}
						v.registerType(oneLit, types.Typ[types.Int])
						lenMinus1 := &ast.BinaryExpr{X: lenCall, Op: token.SUB, Y: oneLit}
						v.registerType(lenMinus1, types.Typ[types.Int])
						intIdent := &ast.Ident{Name: "int"}
						intCast := &ast.CallExpr{Fun: intIdent, Args: []ast.Expr{lenMinus1}}
						v.registerType(intCast, types.Typ[types.Int])
						if v.pkg != nil && v.pkg.TypesInfo != nil {
							intObj := types.Universe.Lookup("int")
							if intObj != nil {
								v.pkg.TypesInfo.Uses[intIdent] = intObj
							}
						}
						preamble = append(preamble, &ast.AssignStmt{
							Lhs: []ast.Expr{&ast.Ident{Name: origName}},
							Tok: token.DEFINE,
							Rhs: []ast.Expr{intCast},
						})
					}
				}
			}
		}

		if len(preamble) > 0 {
			fd.Body.List = append(preamble, fd.Body.List...)
		}
	}
}

func (v *ptrTransformVisitor) rewriteBlockStmt(block *ast.BlockStmt, analysis *ptrAnalysis) {
	var newList []ast.Stmt
	for _, stmt := range block.List {
		if v.isPtrVarEliminatedStmt(stmt, analysis) {
			continue
		}
		expanded := v.rewriteStmtExpand(stmt, analysis)
		newList = append(newList, expanded...)
	}
	block.List = newList
}

// rewriteStmtExpand returns one or more statements. For pool var := assignments,
// it expands into: append to pool + index assignment.
func (v *ptrTransformVisitor) rewriteStmtExpand(stmt ast.Stmt, analysis *ptrAnalysis) []ast.Stmt {
	// Handle x := val where x is a pool var
	if assignStmt, ok := stmt.(*ast.AssignStmt); ok && assignStmt.Tok == token.DEFINE {
		if len(assignStmt.Lhs) == 1 && len(assignStmt.Rhs) == 1 {
			if lhsIdent, ok := assignStmt.Lhs[0].(*ast.Ident); ok {
				if elemType, isPool := analysis.poolVars[lhsIdent.Name]; isPool {
					return v.expandPoolVarAssign(lhsIdent.Name, assignStmt.Rhs[0], elemType, analysis)
				}
			}
		}
	}
	// Handle var x T where x is a pool var
	if declStmt, ok := stmt.(*ast.DeclStmt); ok {
		if genDecl, ok := declStmt.Decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			for _, spec := range genDecl.Specs {
				if valueSpec, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range valueSpec.Names {
						if elemType, isPool := analysis.poolVars[name.Name]; isPool {
							zeroVal := v.zeroValueExpr(elemType)
							return v.expandPoolVarAssign(name.Name, zeroVal, elemType, analysis)
						}
					}
				}
			}
		}
	}

	// Handle return statements in functions with pointer returns
	if retStmt, ok := stmt.(*ast.ReturnStmt); ok && len(analysis.ptrReturns) > 0 {
		return v.expandPtrReturn(retStmt, analysis)
	}

	// Handle call sites for pointer-returning functions
	if assignStmt, ok := stmt.(*ast.AssignStmt); ok {
		if len(assignStmt.Rhs) == 1 {
			if callExpr, ok := assignStmt.Rhs[0].(*ast.CallExpr); ok {
				if pools := v.getCallReturnPools(callExpr); len(pools) > 0 {
					return v.expandPtrReturnCallAssign(assignStmt, pools, analysis)
				}
			}
		}
	}

	return []ast.Stmt{v.rewriteStmt(stmt, analysis)}
}

// expandPoolVarAssign expands: varName := CompositeLit{...}
// into:
//   _pool_T = append(_pool_T, rewrittenRHS)
//   varName := len(_pool_T) - 1
func (v *ptrTransformVisitor) expandPoolVarAssign(varName string, rhs ast.Expr, elemType types.Type, analysis *ptrAnalysis) []ast.Stmt {
	poolName := poolNameForType(elemType)

	// Rewrite the RHS (handles nested &x → x, injects -1 defaults, etc.)
	rewrittenRHS := v.rewriteExpr(rhs, analysis)

	// Statement 1: _pool_T = append(_pool_T, rewrittenRHS)
	poolIdent1 := &ast.Ident{Name: poolName}
	poolIdent2 := &ast.Ident{Name: poolName}
	appendCall := &ast.CallExpr{
		Fun:  &ast.Ident{Name: "append"},
		Args: []ast.Expr{poolIdent1, rewrittenRHS},
	}
	sliceType := types.NewSlice(elemType)
	v.registerType(poolIdent1, sliceType)
	v.registerType(poolIdent2, sliceType)
	v.registerType(appendCall, sliceType)
	appendStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{poolIdent2},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{appendCall},
	}

	// Statement 2: varName := int(len(_pool_T) - 1)
	poolIdent3 := &ast.Ident{Name: poolName}
	v.registerType(poolIdent3, sliceType)
	lenCall := &ast.CallExpr{
		Fun:  &ast.Ident{Name: "len"},
		Args: []ast.Expr{poolIdent3},
	}
	v.registerType(lenCall, types.Typ[types.Int])
	oneLit := &ast.BasicLit{Kind: token.INT, Value: "1"}
	v.registerType(oneLit, types.Typ[types.Int])
	lenMinus1 := &ast.BinaryExpr{
		X:  lenCall,
		Op: token.SUB,
		Y:  oneLit,
	}
	v.registerType(lenMinus1, types.Typ[types.Int])
	// Wrap in int() to ensure C++ emits int type (len returns size_t in C++)
	intIdent := &ast.Ident{Name: "int"}
	intCast := &ast.CallExpr{
		Fun:  intIdent,
		Args: []ast.Expr{lenMinus1},
	}
	v.registerType(intCast, types.Typ[types.Int])
	// Register the int ident as a type name so emitters (especially C#) recognize it
	if v.pkg != nil && v.pkg.TypesInfo != nil {
		intObj := types.Universe.Lookup("int")
		if intObj != nil {
			v.pkg.TypesInfo.Uses[intIdent] = intObj
		}
	}
	indexStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.Ident{Name: varName}},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{intCast},
	}

	return []ast.Stmt{appendStmt, indexStmt}
}

// minusOneExpr creates the AST for -1 (sentinel for nil pointer)
func (v *ptrTransformVisitor) minusOneExpr() ast.Expr {
	oneLit := &ast.BasicLit{Kind: token.INT, Value: "1"}
	v.registerType(oneLit, types.Typ[types.Int])
	minusOne := &ast.UnaryExpr{Op: token.SUB, X: oneLit}
	v.registerType(minusOne, types.Typ[types.Int])
	return minusOne
}

// getCallReturnPools checks if a call expression targets a function with *T returns
// and returns the pool types needed (poolName → elemType)
func (v *ptrTransformVisitor) getCallReturnPools(call *ast.CallExpr) map[string]types.Type {
	pools := make(map[string]types.Type)
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return pools
	}
	funIdent, ok := call.Fun.(*ast.Ident)
	if !ok {
		return pools
	}
	obj := v.pkg.TypesInfo.Uses[funIdent]
	if obj == nil {
		return pools
	}
	fnType, ok := obj.Type().(*types.Signature)
	if !ok {
		return pools
	}
	results := fnType.Results()
	for i := 0; i < results.Len(); i++ {
		if ptrType, isPtr := results.At(i).Type().(*types.Pointer); isPtr {
			elemType := ptrType.Elem()
			pn := poolNameForType(elemType)
			pools[pn] = elemType
		}
	}
	return pools
}

// isPointerTypedExpr checks if an expression has a pointer type in the original TypesInfo
func (v *ptrTransformVisitor) isPointerTypedExpr(expr ast.Expr) bool {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return false
	}
	if tv, ok := v.pkg.TypesInfo.Types[expr]; ok && tv.Type != nil {
		if _, isPtr := tv.Type.(*types.Pointer); isPtr {
			return true
		}
	}
	if ident, ok := expr.(*ast.Ident); ok {
		if obj := v.pkg.TypesInfo.Uses[ident]; obj != nil {
			if _, isPtr := obj.Type().(*types.Pointer); isPtr {
				return true
			}
		}
	}
	return false
}

// expandPtrReturn handles return statements in functions with pointer returns.
// It rewrites return values and appends pool variables.
func (v *ptrTransformVisitor) expandPtrReturn(ret *ast.ReturnStmt, analysis *ptrAnalysis) []ast.Stmt {
	// First, rewrite all result expressions
	for i, r := range ret.Results {
		ret.Results[i] = v.rewriteExpr(r, analysis)
	}

	// Check if this is a single-result return with a CallExpr to a ptr-returning function
	// In Go, `return f()` where f returns (int, []T) is valid and covers all return values
	if len(ret.Results) == 1 {
		if callExpr, ok := ret.Results[0].(*ast.CallExpr); ok {
			pools := v.getCallReturnPools(callExpr)
			if len(pools) > 0 {
				// The call already returns the pool as part of its multi-value return
				return []ast.Stmt{ret}
			}
		}
	}

	// Handle nil results at pointer return positions → -1
	for i, r := range ret.Results {
		if _, hasPtrReturn := analysis.ptrReturns[i]; hasPtrReturn {
			if ident, ok := r.(*ast.Ident); ok && ident.Name == "nil" {
				ret.Results[i] = v.minusOneExpr()
			}
		}
	}

	// Handle &CompositeLit{...} at pointer return positions → expand to pool append
	for i, r := range ret.Results {
		elemType, hasPtrReturn := analysis.ptrReturns[i]
		if !hasPtrReturn {
			continue
		}
		unary, ok := r.(*ast.UnaryExpr)
		if !ok || unary.Op != token.AND {
			continue
		}
		if _, ok := unary.X.(*ast.CompositeLit); !ok {
			continue
		}
		// Expand: return &T{...} → _pool_T = append(_pool_T, T{...}); return int(len(_pool_T)-1), _pool_T
		poolName := poolNameForType(elemType)
		sliceType := types.NewSlice(elemType)

		poolIdent1 := &ast.Ident{Name: poolName}
		poolIdent2 := &ast.Ident{Name: poolName}
		appendCall := &ast.CallExpr{
			Fun:  &ast.Ident{Name: "append"},
			Args: []ast.Expr{poolIdent1, unary.X}, // unary.X is the composite lit
		}
		v.registerType(poolIdent1, sliceType)
		v.registerType(poolIdent2, sliceType)
		v.registerType(appendCall, sliceType)
		appendStmt := &ast.AssignStmt{
			Lhs: []ast.Expr{poolIdent2},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{appendCall},
		}

		// Replace result with int(len(_pool_T) - 1)
		poolIdent3 := &ast.Ident{Name: poolName}
		v.registerType(poolIdent3, sliceType)
		lenCall := &ast.CallExpr{
			Fun:  &ast.Ident{Name: "len"},
			Args: []ast.Expr{poolIdent3},
		}
		v.registerType(lenCall, types.Typ[types.Int])
		oneLit := &ast.BasicLit{Kind: token.INT, Value: "1"}
		v.registerType(oneLit, types.Typ[types.Int])
		lenMinus1 := &ast.BinaryExpr{X: lenCall, Op: token.SUB, Y: oneLit}
		v.registerType(lenMinus1, types.Typ[types.Int])
		intIdent := &ast.Ident{Name: "int"}
		intCast := &ast.CallExpr{Fun: intIdent, Args: []ast.Expr{lenMinus1}}
		v.registerType(intCast, types.Typ[types.Int])
		if v.pkg != nil && v.pkg.TypesInfo != nil {
			intObj := types.Universe.Lookup("int")
			if intObj != nil {
				v.pkg.TypesInfo.Uses[intIdent] = intObj
			}
		}
		ret.Results[i] = intCast

		// Append pool idents to return results
		for _, retElemType := range analysis.ptrReturns {
			pn := poolNameForType(retElemType)
			poolRetIdent := &ast.Ident{Name: pn}
			v.registerType(poolRetIdent, types.NewSlice(retElemType))
			ret.Results = append(ret.Results, poolRetIdent)
		}
		return []ast.Stmt{appendStmt, ret}
	}

	// For normal returns (with explicit values), append pool idents
	for _, elemType := range analysis.ptrReturns {
		pn := poolNameForType(elemType)
		poolIdent := &ast.Ident{Name: pn}
		v.registerType(poolIdent, types.NewSlice(elemType))
		ret.Results = append(ret.Results, poolIdent)
	}
	return []ast.Stmt{ret}
}

// containsPoolAccess checks if an expression contains an IndexExpr on a pool variable
func containsPoolAccess(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		if indexExpr, ok := n.(*ast.IndexExpr); ok {
			if ident, ok := indexExpr.X.(*ast.Ident); ok {
				if len(ident.Name) > 6 && ident.Name[:6] == "_pool_" {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

// expandPtrReturnCallAssign handles assignment statements where the RHS is a call
// to a function that returns *T. It expands the LHS to receive pool return values.
func (v *ptrTransformVisitor) expandPtrReturnCallAssign(assign *ast.AssignStmt, returnPools map[string]types.Type, analysis *ptrAnalysis) []ast.Stmt {
	// Rewrite all expressions first (including pool arg injection on the call)
	for i, lhs := range assign.Lhs {
		assign.Lhs[i] = v.rewriteExpr(lhs, analysis)
	}
	for i, rhs := range assign.Rhs {
		assign.Rhs[i] = v.rewriteExpr(rhs, analysis)
	}

	// Check if any rewritten LHS involves pool access (safety concern with pool reallocation)
	hasPoolAccessLHS := false
	for _, lhs := range assign.Lhs {
		if containsPoolAccess(lhs) {
			hasPoolAccessLHS = true
			break
		}
	}

	if hasPoolAccessLHS {
		// Use temp var approach for safety:
		// var __ptr_ret int; __ptr_ret, _pool_T = f(args, _pool_T); lhs = __ptr_ret
		var stmts []ast.Stmt
		tmpName := fmt.Sprintf("__ptr_ret_%d", v.tmpCounter)
		v.tmpCounter++

		// Declare temp var
		decl := &ast.DeclStmt{Decl: &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{&ast.ValueSpec{
				Names: []*ast.Ident{{Name: tmpName}},
				Type:  &ast.Ident{Name: "int"},
			}},
		}}
		stmts = append(stmts, decl)

		// Build multi-assign: __ptr_ret, _pool_T = f(args, _pool_T)
		newLhs := []ast.Expr{&ast.Ident{Name: tmpName}}
		for poolName, elemType := range returnPools {
			poolIdent := &ast.Ident{Name: poolName}
			v.registerType(poolIdent, types.NewSlice(elemType))
			newLhs = append(newLhs, poolIdent)
		}
		callAssign := &ast.AssignStmt{
			Lhs: newLhs,
			Tok: token.ASSIGN,
			Rhs: assign.Rhs,
		}
		stmts = append(stmts, callAssign)

		// Assign temp to original LHS: lhs = __ptr_ret
		origAssign := &ast.AssignStmt{
			Lhs: assign.Lhs,
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{&ast.Ident{Name: tmpName}},
		}
		stmts = append(stmts, origAssign)
		return stmts
	}

	// Simple case: add pool idents to LHS
	for poolName, elemType := range returnPools {
		poolIdent := &ast.Ident{Name: poolName}
		v.registerType(poolIdent, types.NewSlice(elemType))
		assign.Lhs = append(assign.Lhs, poolIdent)
	}

	// For := assignments, keep :=. For = assignments, keep =.
	// Go allows := with mix of new and existing vars (at least one must be new).
	// For = assignments, all vars must already exist.
	return []ast.Stmt{assign}
}

// isPtrVarEliminatedStmt checks if a statement should be eliminated because it's
// a ptrVar declaration or assignment that has been converted to a ptrLocal alias.
func (v *ptrTransformVisitor) isPtrVarEliminatedStmt(stmt ast.Stmt, analysis *ptrAnalysis) bool {
	switch s := stmt.(type) {
	case *ast.DeclStmt:
		// Eliminate: var p *int where p is in both ptrVars and ptrLocals (or ptrIndexLocals)
		if genDecl, ok := s.Decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			for _, spec := range genDecl.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range vs.Names {
						if _, isPtrVar := analysis.ptrVars[name.Name]; isPtrVar {
							if _, isLocal := analysis.ptrLocals[name.Name]; isLocal {
								return true
							}
							if _, isIndexLocal := analysis.ptrIndexLocals[name.Name]; isIndexLocal {
								return true
							}
						}
					}
				}
			}
		}
	case *ast.AssignStmt:
		// Eliminate: p := &arr[i] where p is a ptrIndexLocal
		if s.Tok == token.DEFINE && len(s.Lhs) == 1 && len(s.Rhs) == 1 {
			if lhsIdent, ok := s.Lhs[0].(*ast.Ident); ok {
				if _, isIndexLocal := analysis.ptrIndexLocals[lhsIdent.Name]; isIndexLocal {
					if unary, ok := s.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
						if _, ok := unary.X.(*ast.IndexExpr); ok {
							return true
						}
					}
				}
			}
		}
		// Eliminate: p = &x where p is a ptrVar converted to ptrLocal
		if s.Tok == token.ASSIGN && len(s.Lhs) == 1 && len(s.Rhs) == 1 {
			if lhsIdent, ok := s.Lhs[0].(*ast.Ident); ok {
				if _, isPtrVar := analysis.ptrVars[lhsIdent.Name]; isPtrVar {
					if _, isLocal := analysis.ptrLocals[lhsIdent.Name]; isLocal {
						if unary, ok := s.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
							return true
						}
					}
					if _, isIndexLocal := analysis.ptrIndexLocals[lhsIdent.Name]; isIndexLocal {
						if unary, ok := s.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

func (v *ptrTransformVisitor) rewriteStmt(stmt ast.Stmt, analysis *ptrAnalysis) ast.Stmt {
	if stmt == nil {
		return nil
	}
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		return v.rewriteAssignStmt(s, analysis)
	case *ast.ExprStmt:
		s.X = v.rewriteExpr(s.X, analysis)
		return s
	case *ast.ReturnStmt:
		for i, r := range s.Results {
			s.Results[i] = v.rewriteExpr(r, analysis)
		}
		return s
	case *ast.IfStmt:
		if s.Init != nil {
			s.Init = v.rewriteStmt(s.Init, analysis)
		}
		s.Cond = v.rewriteExpr(s.Cond, analysis)
		v.rewriteBlockStmt(s.Body, analysis)
		if s.Else != nil {
			s.Else = v.rewriteStmt(s.Else, analysis)
		}
		return s
	case *ast.ForStmt:
		if s.Init != nil {
			s.Init = v.rewriteStmt(s.Init, analysis)
		}
		if s.Cond != nil {
			s.Cond = v.rewriteExpr(s.Cond, analysis)
		}
		if s.Post != nil {
			s.Post = v.rewriteStmt(s.Post, analysis)
		}
		v.rewriteBlockStmt(s.Body, analysis)
		return s
	case *ast.RangeStmt:
		s.X = v.rewriteExpr(s.X, analysis)
		v.rewriteBlockStmt(s.Body, analysis)
		return s
	case *ast.BlockStmt:
		v.rewriteBlockStmt(s, analysis)
		return s
	case *ast.IncDecStmt:
		s.X = v.rewriteExpr(s.X, analysis)
		return s
	case *ast.DeclStmt:
		return v.rewriteDeclStmt(s, analysis)
	case *ast.BranchStmt:
		return s
	case *ast.SwitchStmt:
		if s.Init != nil {
			s.Init = v.rewriteStmt(s.Init, analysis)
		}
		if s.Tag != nil {
			s.Tag = v.rewriteExpr(s.Tag, analysis)
		}
		v.rewriteBlockStmt(s.Body, analysis)
		return s
	case *ast.CaseClause:
		for i, expr := range s.List {
			s.List[i] = v.rewriteExpr(expr, analysis)
		}
		for i, bodyStmt := range s.Body {
			s.Body[i] = v.rewriteStmt(bodyStmt, analysis)
		}
		return s
	}
	return stmt
}

func (v *ptrTransformVisitor) rewriteAssignStmt(s *ast.AssignStmt, analysis *ptrAnalysis) ast.Stmt {
	if s.Tok == token.DEFINE {
		// Pool var := assignments are handled by rewriteStmtExpand, not here.
		// Just rewrite all RHS normally.
		for i, rhs := range s.Rhs {
			s.Rhs[i] = v.rewriteExpr(rhs, analysis)
		}
		return s
	}

	// Regular assignment (=, +=, etc.)
	for i, lhs := range s.Lhs {
		s.Lhs[i] = v.rewriteExpr(lhs, analysis)
	}
	for i, rhs := range s.Rhs {
		s.Rhs[i] = v.rewriteExpr(rhs, analysis)
	}
	return s
}

func (v *ptrTransformVisitor) rewriteDeclStmt(s *ast.DeclStmt, analysis *ptrAnalysis) ast.Stmt {
	if genDecl, ok := s.Decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
		for _, spec := range genDecl.Specs {
			if valueSpec, ok := spec.(*ast.ValueSpec); ok {
				for _, name := range valueSpec.Names {
					// poolVars: var x T → handled by rewriteStmtExpand (pool append)
					// ptrVars that are ptrLocals/ptrIndexLocals: eliminated by isPtrVarEliminatedStmt
					if _, isPtrVar := analysis.ptrVars[name.Name]; isPtrVar {
						_, isLocal := analysis.ptrLocals[name.Name]
						_, isIndexLocal := analysis.ptrIndexLocals[name.Name]
						if !isLocal && !isIndexLocal {
							// Change type from *T (StarExpr) to int (pool index)
							newType := &ast.Ident{Name: "int"}
							valueSpec.Type = newType
							v.registerType(newType, types.Typ[types.Int])
						}
					}
				}
			}
		}
	}
	return s
}

func (v *ptrTransformVisitor) rewriteExpr(expr ast.Expr, analysis *ptrAnalysis) ast.Expr {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.StarExpr:
		// *p -> arr[i] for ptrIndexLocals (alias to slice element)
		if ident, ok := e.X.(*ast.Ident); ok {
			if info, isIndexLocal := analysis.ptrIndexLocals[ident.Name]; isIndexLocal {
				newIndexExpr := &ast.IndexExpr{X: info.sliceExpr, Index: info.indexExpr}
				v.registerType(newIndexExpr, info.elemType)
				return newIndexExpr
			}
		}

		// All pointer derefs use pool indexing: *p -> _pool_T[p]
		if ident, ok := e.X.(*ast.Ident); ok {
			// ptrParams: *p -> _pool_T[p]
			if elemType, isPtr := analysis.ptrParams[ident.Name]; isPtr {
				return v.poolIndexExpr(ident, elemType)
			}
			// ptrLocals: *p -> _pool_T[target]
			if info, isLocal := analysis.ptrLocals[ident.Name]; isLocal {
				targetIdent := &ast.Ident{Name: info.targetName}
				v.registerType(targetIdent, types.Typ[types.Int])
				return v.poolIndexExpr(targetIdent, info.elemType)
			}
			// ptrVars: *p -> _pool_T[p]
			if elemType, isPtrVar := analysis.ptrVars[ident.Name]; isPtrVar {
				return v.poolIndexExpr(ident, elemType)
			}
		}

		// Fallback: pool-based deref for non-Ident X (e.g., *n.next where X is SelectorExpr)
		if v.pkg != nil && v.pkg.TypesInfo != nil {
			if tv, ok := v.pkg.TypesInfo.Types[e.X]; ok {
				if ptrType, isPtr := tv.Type.(*types.Pointer); isPtr {
					inner := v.rewriteExpr(e.X, analysis)
					return v.poolIndexExpr(inner, ptrType.Elem())
				}
			}
		}

		// Last resort fallback (shouldn't normally be reached)
		inner := v.rewriteExpr(e.X, analysis)
		return inner

	case *ast.UnaryExpr:
		if e.Op == token.AND {
			// &x -> x (if x is a pool var, the value is the pool index)
			if ident, ok := e.X.(*ast.Ident); ok {
				if _, isPool := analysis.poolVars[ident.Name]; isPool {
					return ident
				}
			}
		}
		e.X = v.rewriteExpr(e.X, analysis)
		return e

	case *ast.SelectorExpr:
		// Handle p.field for ptrIndexLocals: p.field -> arr[i].field
		if ident, ok := e.X.(*ast.Ident); ok {
			if info, isIndexLocal := analysis.ptrIndexLocals[ident.Name]; isIndexLocal {
				newIndexExpr := &ast.IndexExpr{X: info.sliceExpr, Index: info.indexExpr}
				v.registerType(newIndexExpr, info.elemType)
				e.X = newIndexExpr
				return e
			}
		}
		// Handle p.field for pointer params/locals/vars: p.field -> _pool_T[p].field
		if ident, ok := e.X.(*ast.Ident); ok {
			if elemType, isPtr := analysis.ptrParams[ident.Name]; isPtr {
				e.X = v.poolIndexExpr(ident, elemType)
				return e
			} else if info, isLocal := analysis.ptrLocals[ident.Name]; isLocal {
				targetIdent := &ast.Ident{Name: info.targetName}
				v.registerType(targetIdent, types.Typ[types.Int])
				e.X = v.poolIndexExpr(targetIdent, info.elemType)
				return e
			} else if elemType, isPtrVar := analysis.ptrVars[ident.Name]; isPtrVar {
				e.X = v.poolIndexExpr(ident, elemType)
				return e
			}
		}
		// Save original type BEFORE rewriting (for auto-deref of pointer struct fields)
		var xOrigType types.Type
		if v.pkg != nil && v.pkg.TypesInfo != nil {
			if tv, ok := v.pkg.TypesInfo.Types[e.X]; ok {
				xOrigType = tv.Type
			}
		}

		// General case: recurse into X (handles pool vars via Ident case)
		e.X = v.rewriteExpr(e.X, analysis)

		// Auto-deref: if original X was *T (pointer), insert pool indexing
		if xOrigType != nil {
			if ptrType, isPtr := xOrigType.(*types.Pointer); isPtr {
				e.X = v.poolIndexExpr(e.X, ptrType.Elem())
			}
		}
		return e

	case *ast.Ident:
		// y -> _pool_T[y] for pool variables
		if elemType, isPool := analysis.poolVars[e.Name]; isPool {
			return v.poolIndexExpr(e, elemType)
		}
		// p -> target for ptrLocals (bare alias replacement — target is a pool index)
		if info, isLocal := analysis.ptrLocals[e.Name]; isLocal {
			targetIdent := &ast.Ident{Name: info.targetName}
			v.registerType(targetIdent, types.Typ[types.Int])
			return targetIdent
		}
		// p -> sliceExpr for ptrIndexLocals (bare alias, return the slice)
		if info, isIndexLocal := analysis.ptrIndexLocals[e.Name]; isIndexLocal {
			return info.sliceExpr
		}
		return e

	case *ast.BinaryExpr:
		// Handle nil comparison for pointer types: nil → -1
		// Must check BEFORE rewriting sub-expressions (original TypesInfo needed)
		if e.Op == token.EQL || e.Op == token.NEQ {
			if isNilIdent(e.Y) && v.isPointerTypedExpr(e.X) {
				e.X = v.rewriteExpr(e.X, analysis)
				e.Y = v.minusOneExpr()
				return e
			}
			if isNilIdent(e.X) && v.isPointerTypedExpr(e.Y) {
				e.X = v.minusOneExpr()
				e.Y = v.rewriteExpr(e.Y, analysis)
				return e
			}
		}
		e.X = v.rewriteExpr(e.X, analysis)
		e.Y = v.rewriteExpr(e.Y, analysis)
		return e

	case *ast.CallExpr:
		// Don't rewrite Fun (function/method name)
		for i, arg := range e.Args {
			e.Args[i] = v.rewriteExpr(arg, analysis)
		}
		// Inject pool args for calls to functions with *T params
		v.injectPoolArgs(e)
		return e

	case *ast.ParenExpr:
		e.X = v.rewriteExpr(e.X, analysis)
		return e

	case *ast.IndexExpr:
		e.X = v.rewriteExpr(e.X, analysis)
		e.Index = v.rewriteExpr(e.Index, analysis)
		return e

	case *ast.CompositeLit:
		// Rewrite elements first
		for i, elt := range e.Elts {
			e.Elts[i] = v.rewriteExpr(elt, analysis)
		}
		// Inject -1 defaults for missing pointer fields
		if typeIdent, ok := e.Type.(*ast.Ident); ok {
			if ptrFields, hasPtrFields := v.ptrFieldMap[typeIdent.Name]; hasPtrFields {
				// Find which pointer fields are already present
				present := make(map[string]bool)
				for _, elt := range e.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						if key, ok := kv.Key.(*ast.Ident); ok {
							present[key.Name] = true
						}
					}
				}
				// Inject -1 for missing pointer fields
				for _, fieldName := range ptrFields {
					if !present[fieldName] {
						minusOne := &ast.UnaryExpr{
							Op: token.SUB,
							X:  &ast.BasicLit{Kind: token.INT, Value: "1"},
						}
						v.registerType(minusOne, types.Typ[types.Int])
						v.registerType(minusOne.X.(*ast.BasicLit), types.Typ[types.Int])
						kv := &ast.KeyValueExpr{
							Key:   &ast.Ident{Name: fieldName},
							Value: minusOne,
						}
						e.Elts = append(e.Elts, kv)
					}
				}
			}
		}
		return e

	case *ast.BasicLit:
		return e

	case *ast.FuncLit:
		if e.Body != nil {
			v.rewriteBlockStmt(e.Body, analysis)
		}
		return e

	case *ast.KeyValueExpr:
		e.Value = v.rewriteExpr(e.Value, analysis)
		return e

	case *ast.TypeAssertExpr:
		e.X = v.rewriteExpr(e.X, analysis)
		return e

	case *ast.SliceExpr:
		e.X = v.rewriteExpr(e.X, analysis)
		if e.Low != nil {
			e.Low = v.rewriteExpr(e.Low, analysis)
		}
		if e.High != nil {
			e.High = v.rewriteExpr(e.High, analysis)
		}
		return e

	case *ast.ArrayType, *ast.MapType, *ast.InterfaceType, *ast.FuncType:
		// Don't rewrite type expressions
		return e
	}
	return expr
}

// injectPoolArgs appends _pool_T arguments for calls to functions with *T params
func (v *ptrTransformVisitor) injectPoolArgs(call *ast.CallExpr) {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return
	}
	// Get the function being called
	var funIdent *ast.Ident
	if id, ok := call.Fun.(*ast.Ident); ok {
		funIdent = id
	}
	if funIdent == nil {
		return
	}
	obj := v.pkg.TypesInfo.Uses[funIdent]
	if obj == nil {
		return
	}
	fnType, ok := obj.Type().(*types.Signature)
	if !ok {
		return
	}
	// Check params for *T types and collect unique pool names
	poolsNeeded := make(map[string]types.Type) // poolName → elemType (dedup)
	params := fnType.Params()
	for i := 0; i < params.Len(); i++ {
		paramType := params.At(i).Type()
		if ptrType, isPtr := paramType.(*types.Pointer); isPtr {
			elemType := ptrType.Elem()
			pn := poolNameForType(elemType)
			poolsNeeded[pn] = elemType
		}
	}
	// Also check return types for *T (functions returning pointers also need pool args)
	results := fnType.Results()
	for i := 0; i < results.Len(); i++ {
		resultType := results.At(i).Type()
		if ptrType, isPtr := resultType.(*types.Pointer); isPtr {
			elemType := ptrType.Elem()
			pn := poolNameForType(elemType)
			poolsNeeded[pn] = elemType
		}
	}
	// Append pool idents as extra args
	for poolName, elemType := range poolsNeeded {
		poolIdent := &ast.Ident{Name: poolName}
		v.registerType(poolIdent, types.NewSlice(elemType))
		call.Args = append(call.Args, poolIdent)
	}
}

// registerType adds a type info entry for an AST node
func (v *ptrTransformVisitor) registerType(expr ast.Expr, t types.Type) {
	if v.pkg != nil && v.pkg.TypesInfo != nil && t != nil {
		v.pkg.TypesInfo.Types[expr] = types.TypeAndValue{Type: t}
	}
}

// typeToExpr converts a types.Type to an ast.Expr for use in AST nodes
func (v *ptrTransformVisitor) typeToExpr(t types.Type) ast.Expr {
	switch typ := t.(type) {
	case *types.Basic:
		return &ast.Ident{Name: typ.Name()}
	case *types.Named:
		objPkg := typ.Obj().Pkg()
		if objPkg != nil && v.pkg != nil && objPkg.Path() != v.pkg.PkgPath {
			return &ast.SelectorExpr{
				X:   &ast.Ident{Name: objPkg.Name()},
				Sel: &ast.Ident{Name: typ.Obj().Name()},
			}
		}
		return &ast.Ident{Name: typ.Obj().Name()}
	case *types.Slice:
		return &ast.ArrayType{Elt: v.typeToExpr(typ.Elem())}
	case *types.Map:
		return &ast.MapType{
			Key:   v.typeToExpr(typ.Key()),
			Value: v.typeToExpr(typ.Elem()),
		}
	}
	return &ast.Ident{Name: t.String()}
}

// zeroValueExpr creates an AST expression for the zero value of a type
func (v *ptrTransformVisitor) zeroValueExpr(t types.Type) ast.Expr {
	switch typ := t.(type) {
	case *types.Basic:
		switch typ.Kind() {
		case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		case types.Float32, types.Float64:
			return &ast.BasicLit{Kind: token.FLOAT, Value: "0.0"}
		case types.Bool:
			return &ast.Ident{Name: "false"}
		case types.String:
			return &ast.BasicLit{Kind: token.STRING, Value: `""`}
		}
	case *types.Named:
		return &ast.CompositeLit{Type: v.typeToExpr(t)}
	}
	return &ast.BasicLit{Kind: token.INT, Value: "0"}
}
