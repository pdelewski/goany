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

// PointerTransformPass transforms pointer parameters (*T) to single-element
// slices ([]T) and rewrites pointer operations accordingly.
// This runs between SemaChecker and emitter passes.
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
	pkg   *packages.Package
	funcs []*ast.FuncDecl
}

func (v *ptrTransformVisitor) Visit(node ast.Node) ast.Visitor {
	if fd, ok := node.(*ast.FuncDecl); ok {
		v.funcs = append(v.funcs, fd)
	}
	return v
}

// ptrLocalInfo holds info about a local pointer alias (p := &x)
type ptrLocalInfo struct {
	targetName string     // the boxed variable this aliases (e.g., "x")
	elemType   types.Type // element type of the pointer
}

// ptrAnalysis holds analysis results for a single function
type ptrAnalysis struct {
	ptrParams map[string]types.Type    // param name -> element type (for *T params)
	ptrVars   map[string]types.Type    // var p *T declarations -> element type
	boxedVars map[string]types.Type    // local var name -> original type (vars whose address is taken)
	ptrLocals map[string]*ptrLocalInfo // alias name -> target info (p := &x)
}

func (v *ptrTransformVisitor) transform() {
	for _, fd := range v.funcs {
		analysis := v.analyzeFuncDecl(fd)
		if len(analysis.ptrParams) == 0 && len(analysis.ptrVars) == 0 && len(analysis.boxedVars) == 0 && len(analysis.ptrLocals) == 0 {
			continue
		}
		v.rewriteFuncDecl(fd, analysis)
	}
}

func (v *ptrTransformVisitor) analyzeFuncDecl(fd *ast.FuncDecl) *ptrAnalysis {
	result := &ptrAnalysis{
		ptrParams: make(map[string]types.Type),
		ptrVars:   make(map[string]types.Type),
		boxedVars: make(map[string]types.Type),
		ptrLocals: make(map[string]*ptrLocalInfo),
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

	// Find boxed vars (locals whose address is taken via &)
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
					result.boxedVars[ident.Name] = obj.Type()
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
				if elemType, isBoxed := result.boxedVars[rhsIdent.Name]; isBoxed {
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
							if _, isBoxed := result.boxedVars[rhsIdent.Name]; isBoxed {
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
	}

	return result
}

func (v *ptrTransformVisitor) rewriteFuncDecl(fd *ast.FuncDecl, analysis *ptrAnalysis) {
	// Rewrite param types: *T -> []T
	if fd.Type.Params != nil {
		for _, field := range fd.Type.Params.List {
			if starExpr, ok := field.Type.(*ast.StarExpr); ok {
				newType := &ast.ArrayType{Elt: starExpr.X}
				field.Type = newType
				// Register type info for the new ArrayType
				if v.pkg != nil && v.pkg.TypesInfo != nil {
					if elemTV, ok := v.pkg.TypesInfo.Types[starExpr.X]; ok {
						v.registerType(newType, types.NewSlice(elemTV.Type))
					}
				}
			}
		}
	}

	// Rewrite body
	if fd.Body != nil {
		v.rewriteBlockStmt(fd.Body, analysis)
	}

	// Parameter boxing: for non-pointer params whose address is taken (in boxedVars),
	// rename param x -> __x and prepend boxing statement: x := []T{__x}
	if fd.Type.Params != nil && fd.Body != nil {
		var boxingStmts []ast.Stmt
		for _, field := range fd.Type.Params.List {
			for _, name := range field.Names {
				if elemType, isBoxed := analysis.boxedVars[name.Name]; isBoxed {
					origName := name.Name
					prefixedName := "__" + origName

					// Rename the parameter ident
					name.Name = prefixedName

					// Build boxing assignment: origName := []T{__origName}
					typeExpr := v.typeToExpr(elemType)
					arrType := &ast.ArrayType{Elt: typeExpr}
					paramRef := &ast.Ident{Name: prefixedName}
					compLit := &ast.CompositeLit{
						Type: arrType,
						Elts: []ast.Expr{paramRef},
					}
					sliceType := types.NewSlice(elemType)
					v.registerType(arrType, sliceType)
					v.registerType(compLit, sliceType)
					v.registerType(paramRef, elemType)

					boxingStmt := &ast.AssignStmt{
						Lhs: []ast.Expr{&ast.Ident{Name: origName}},
						Tok: token.DEFINE,
						Rhs: []ast.Expr{compLit},
					}
					boxingStmts = append(boxingStmts, boxingStmt)
				}
			}
		}
		if len(boxingStmts) > 0 {
			fd.Body.List = append(boxingStmts, fd.Body.List...)
		}
	}
}

func (v *ptrTransformVisitor) rewriteBlockStmt(block *ast.BlockStmt, analysis *ptrAnalysis) {
	var newList []ast.Stmt
	for _, stmt := range block.List {
		if v.isPtrVarEliminatedStmt(stmt, analysis) {
			continue
		}
		newList = append(newList, v.rewriteStmt(stmt, analysis))
	}
	block.List = newList
}

// isPtrVarEliminatedStmt checks if a statement should be eliminated because it's
// a ptrVar declaration or assignment that has been converted to a ptrLocal alias.
func (v *ptrTransformVisitor) isPtrVarEliminatedStmt(stmt ast.Stmt, analysis *ptrAnalysis) bool {
	switch s := stmt.(type) {
	case *ast.DeclStmt:
		// Eliminate: var p *int where p is in both ptrVars and ptrLocals
		if genDecl, ok := s.Decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			for _, spec := range genDecl.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range vs.Names {
						if _, isPtrVar := analysis.ptrVars[name.Name]; isPtrVar {
							if _, isLocal := analysis.ptrLocals[name.Name]; isLocal {
								return true
							}
						}
					}
				}
			}
		}
	case *ast.AssignStmt:
		// Eliminate: p = &x where p is a ptrVar converted to ptrLocal
		if s.Tok == token.ASSIGN && len(s.Lhs) == 1 && len(s.Rhs) == 1 {
			if lhsIdent, ok := s.Lhs[0].(*ast.Ident); ok {
				if _, isPtrVar := analysis.ptrVars[lhsIdent.Name]; isPtrVar {
					if _, isLocal := analysis.ptrLocals[lhsIdent.Name]; isLocal {
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
		// Short variable declaration (:=)
		// Track which RHS indices were handled by boxing
		boxedIndices := make(map[int]bool)

		for i, lhs := range s.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				if elemType, isBoxed := analysis.boxedVars[ident.Name]; isBoxed {
					if i < len(s.Rhs) {
						// Rewrite RHS first (may contain pointer ops)
						rewrittenRhs := v.rewriteExpr(s.Rhs[i], analysis)
						// Wrap in []T{val}
						typeExpr := v.typeToExpr(elemType)
						arrType := &ast.ArrayType{Elt: typeExpr}
						compLit := &ast.CompositeLit{
							Type: arrType,
							Elts: []ast.Expr{rewrittenRhs},
						}
						s.Rhs[i] = compLit
						// Register type info
						sliceType := types.NewSlice(elemType)
						v.registerType(arrType, sliceType)
						v.registerType(compLit, sliceType)
						boxedIndices[i] = true
					}
				}
			}
		}

		// Rewrite non-boxed RHS
		for i, rhs := range s.Rhs {
			if !boxedIndices[i] {
				s.Rhs[i] = v.rewriteExpr(rhs, analysis)
			}
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
					if elemType, isBoxed := analysis.boxedVars[name.Name]; isBoxed {
						// Change type from T to []T
						typeExpr := v.typeToExpr(elemType)
						arrType := &ast.ArrayType{Elt: typeExpr}
						valueSpec.Type = arrType
						// Add initialization: = []T{zero}
						zeroVal := v.zeroValueExpr(elemType)
						initArrType := &ast.ArrayType{Elt: v.typeToExpr(elemType)}
						compLit := &ast.CompositeLit{
							Type: initArrType,
							Elts: []ast.Expr{zeroVal},
						}
						valueSpec.Values = []ast.Expr{compLit}
						// Register type info
						sliceType := types.NewSlice(elemType)
						v.registerType(arrType, sliceType)
						v.registerType(initArrType, sliceType)
						v.registerType(compLit, sliceType)
					} else if elemType, isPtrVar := analysis.ptrVars[name.Name]; isPtrVar {
						// Skip ptrVars that were converted to ptrLocals (will be eliminated)
						if _, isLocal := analysis.ptrLocals[name.Name]; !isLocal {
							// Change type from *T (StarExpr) to []T (ArrayType)
							typeExpr := v.typeToExpr(elemType)
							arrType := &ast.ArrayType{Elt: typeExpr}
							valueSpec.Type = arrType
							// No zero-value init (nil slice = nil pointer)
							// Register type info
							sliceType := types.NewSlice(elemType)
							v.registerType(arrType, sliceType)
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
		// *p -> p[0]
		inner := v.rewriteExpr(e.X, analysis)
		zeroLit := &ast.BasicLit{Kind: token.INT, Value: "0"}
		indexExpr := &ast.IndexExpr{X: inner, Index: zeroLit}
		v.registerType(zeroLit, types.Typ[types.Int])
		// Register type info for the IndexExpr result and X slice type
		if ident, ok := e.X.(*ast.Ident); ok {
			if elemType, isPtr := analysis.ptrParams[ident.Name]; isPtr {
				v.registerType(indexExpr, elemType)
				v.registerType(inner, types.NewSlice(elemType))
			} else if info, isLocal := analysis.ptrLocals[ident.Name]; isLocal {
				v.registerType(indexExpr, info.elemType)
				v.registerType(inner, types.NewSlice(info.elemType))
			} else if elemType, isPtrVar := analysis.ptrVars[ident.Name]; isPtrVar {
				v.registerType(indexExpr, elemType)
				v.registerType(inner, types.NewSlice(elemType))
			}
		}
		return indexExpr

	case *ast.UnaryExpr:
		if e.Op == token.AND {
			// &y -> y (if y is boxed, the var is already a slice)
			if ident, ok := e.X.(*ast.Ident); ok {
				if _, isBoxed := analysis.boxedVars[ident.Name]; isBoxed {
					return ident
				}
			}
		}
		e.X = v.rewriteExpr(e.X, analysis)
		return e

	case *ast.SelectorExpr:
		// Handle p.field for pointer params: p.field -> p[0].field
		if ident, ok := e.X.(*ast.Ident); ok {
			if elemType, isPtr := analysis.ptrParams[ident.Name]; isPtr {
				zeroLit := &ast.BasicLit{Kind: token.INT, Value: "0"}
				indexExpr := &ast.IndexExpr{X: ident, Index: zeroLit}
				v.registerType(zeroLit, types.Typ[types.Int])
				v.registerType(indexExpr, elemType)
				v.registerType(ident, types.NewSlice(elemType))
				e.X = indexExpr
				return e
			} else if info, isLocal := analysis.ptrLocals[ident.Name]; isLocal {
				// p.field -> target[0].field
				targetIdent := &ast.Ident{Name: info.targetName}
				zeroLit := &ast.BasicLit{Kind: token.INT, Value: "0"}
				indexExpr := &ast.IndexExpr{X: targetIdent, Index: zeroLit}
				v.registerType(zeroLit, types.Typ[types.Int])
				v.registerType(indexExpr, info.elemType)
				v.registerType(targetIdent, types.NewSlice(info.elemType))
				e.X = indexExpr
				return e
			} else if elemType, isPtrVar := analysis.ptrVars[ident.Name]; isPtrVar {
				// p.field -> p[0].field for pure ptrVars (not converted to ptrLocals)
				zeroLit := &ast.BasicLit{Kind: token.INT, Value: "0"}
				indexExpr := &ast.IndexExpr{X: ident, Index: zeroLit}
				v.registerType(zeroLit, types.Typ[types.Int])
				v.registerType(indexExpr, elemType)
				v.registerType(ident, types.NewSlice(elemType))
				e.X = indexExpr
				return e
			}
		}
		// General case: recurse into X (handles boxed vars via Ident case)
		e.X = v.rewriteExpr(e.X, analysis)
		return e

	case *ast.Ident:
		// y -> y[0] for boxed variables
		if elemType, isBoxed := analysis.boxedVars[e.Name]; isBoxed {
			zeroLit := &ast.BasicLit{Kind: token.INT, Value: "0"}
			indexExpr := &ast.IndexExpr{X: e, Index: zeroLit}
			v.registerType(zeroLit, types.Typ[types.Int])
			v.registerType(indexExpr, elemType)
			v.registerType(e, types.NewSlice(elemType))
			return indexExpr
		}
		// p -> target for ptrLocals (bare alias replacement, no [0])
		if info, isLocal := analysis.ptrLocals[e.Name]; isLocal {
			targetIdent := &ast.Ident{Name: info.targetName}
			v.registerType(targetIdent, types.NewSlice(info.elemType))
			return targetIdent
		}
		return e

	case *ast.BinaryExpr:
		e.X = v.rewriteExpr(e.X, analysis)
		e.Y = v.rewriteExpr(e.Y, analysis)
		return e

	case *ast.CallExpr:
		// Don't rewrite Fun (function/method name)
		for i, arg := range e.Args {
			e.Args[i] = v.rewriteExpr(arg, analysis)
		}
		return e

	case *ast.ParenExpr:
		e.X = v.rewriteExpr(e.X, analysis)
		return e

	case *ast.IndexExpr:
		e.X = v.rewriteExpr(e.X, analysis)
		e.Index = v.rewriteExpr(e.Index, analysis)
		return e

	case *ast.CompositeLit:
		for i, elt := range e.Elts {
			e.Elts[i] = v.rewriteExpr(elt, analysis)
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
