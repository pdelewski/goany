package compiler

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

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

// ptrAnalysis holds analysis results for a single function
type ptrAnalysis struct {
	ptrParams map[string]types.Type // param name -> element type (for *T params)
	boxedVars map[string]types.Type // local var name -> original type (vars whose address is taken)
}

func (v *ptrTransformVisitor) transform() {
	for _, fd := range v.funcs {
		analysis := v.analyzeFuncDecl(fd)
		if len(analysis.ptrParams) == 0 && len(analysis.boxedVars) == 0 {
			continue
		}
		v.rewriteFuncDecl(fd, analysis)
	}
}

func (v *ptrTransformVisitor) analyzeFuncDecl(fd *ast.FuncDecl) *ptrAnalysis {
	result := &ptrAnalysis{
		ptrParams: make(map[string]types.Type),
		boxedVars: make(map[string]types.Type),
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
			// Get the type of the variable
			if v.pkg != nil && v.pkg.TypesInfo != nil {
				if obj := v.pkg.TypesInfo.Uses[ident]; obj != nil {
					result.boxedVars[ident.Name] = obj.Type()
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
}

func (v *ptrTransformVisitor) rewriteBlockStmt(block *ast.BlockStmt, analysis *ptrAnalysis) {
	for i, stmt := range block.List {
		block.List[i] = v.rewriteStmt(stmt, analysis)
	}
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
