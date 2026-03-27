package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// InterfaceLoweringPass transforms non-empty interface declarations into structs
// with _data interface{} + function pointer fields, and rewrites all usage sites
// (assignments, method dispatch, type assertions, nil comparisons).
//
// This pass MUST run after MethodReceiverLoweringPass (so wrapper lambdas can
// reference the lowered Type_Method free functions) and before PointerLoweringPass
// (so any generated pointer ops get properly transformed).
//
// When only the Go backend is enabled, this pass is skipped (Go has native interfaces).
type InterfaceLoweringPass struct {
	Backends BackendSet
}

func (p *InterfaceLoweringPass) Name() string { return "InterfaceLowering" }
func (p *InterfaceLoweringPass) ProLog()      {}
func (p *InterfaceLoweringPass) EpiLog()      {}

func (p *InterfaceLoweringPass) Visitors(pkg *packages.Package) []ast.Visitor {
	return []ast.Visitor{&ifaceLoweringVisitor{pkg: pkg, backends: p.Backends}}
}

func (p *InterfaceLoweringPass) PreVisit(visitor ast.Visitor) {}

func (p *InterfaceLoweringPass) PostVisit(visitor ast.Visitor, visited map[string]struct{}) {
	v := visitor.(*ifaceLoweringVisitor)
	v.transform()
}

// ifaceLoweringVisitor collects AST files during the walk,
// then applies all transforms in PostVisit via transform().
type ifaceLoweringVisitor struct {
	pkg      *packages.Package
	backends BackendSet
	files    []*ast.File
}

func (v *ifaceLoweringVisitor) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return nil
	}
	if file, ok := node.(*ast.File); ok {
		v.files = append(v.files, file)
	}
	return nil
}

// ifaceInfo stores everything needed to generate the lowered struct and wrapper code
// for one interface.
type ifaceInfo struct {
	name       string
	methods    []ifaceMethod
	structType *types.Struct // the lowered struct type for TypesInfo registration
}

// ifaceMethod captures a single method signature from the original interface.
type ifaceMethod struct {
	name    string
	params  []*ast.Field
	results []*ast.Field
}

func (v *ifaceLoweringVisitor) transform() {
	// Skip when ONLY Go backend is enabled (Go has native interfaces)
	if v.backends & ^BackendGo == 0 {
		return
	}

	// Phase 1: Collect interface declarations
	ifaceDecls := make(map[string]*ifaceInfo)
	for _, file := range v.files {
		v.collectInterfaceDecls(file, ifaceDecls)
	}

	if len(ifaceDecls) == 0 {
		return
	}

	// Phase 2: Replace interface type declarations with structs
	for _, file := range v.files {
		v.replaceInterfaceDecls(file, ifaceDecls)
	}

	// Phase 3-6: Rewrite usage sites
	for _, file := range v.files {
		v.rewriteUsageSites(file, ifaceDecls)
	}
}

// collectInterfaceDecls walks type declarations and collects non-empty interface info.
func (v *ifaceLoweringVisitor) collectInterfaceDecls(file *ast.File, ifaceDecls map[string]*ifaceInfo) {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			ifaceType, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				continue
			}
			if ifaceType.Methods == nil || len(ifaceType.Methods.List) == 0 {
				continue
			}

			info := &ifaceInfo{name: ts.Name.Name}
			for _, method := range ifaceType.Methods.List {
				if len(method.Names) == 0 {
					continue // embedded interface — skip for now
				}
				ft, ok := method.Type.(*ast.FuncType)
				if !ok {
					continue
				}
				var params []*ast.Field
				if ft.Params != nil {
					params = ft.Params.List
				}
				var results []*ast.Field
				if ft.Results != nil {
					results = ft.Results.List
				}
				info.methods = append(info.methods, ifaceMethod{
					name:    method.Names[0].Name,
					params:  params,
					results: results,
				})
			}
			if len(info.methods) > 0 {
				ifaceDecls[ts.Name.Name] = info
			}
		}
	}
}

// replaceInterfaceDecls replaces interface type specs with struct type specs.
func (v *ifaceLoweringVisitor) replaceInterfaceDecls(file *ast.File, ifaceDecls map[string]*ifaceInfo) {
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			info, ok := ifaceDecls[ts.Name.Name]
			if !ok {
				continue
			}
			if _, isIface := ts.Type.(*ast.InterfaceType); !isIface {
				continue
			}

			// Build struct fields: _data interface{} + _hasValue bool + _Method func(interface{}, ...) ...
			fields := []*ast.Field{
				{
					Names: []*ast.Ident{ast.NewIdent("_data")},
					Type:  &ast.InterfaceType{Methods: &ast.FieldList{}},
				},
				{
					Names: []*ast.Ident{ast.NewIdent("_hasValue")},
					Type:  ast.NewIdent("bool"),
				},
			}

			// Build types.Struct for TypesInfo registration
			emptyIface := types.NewInterfaceType(nil, nil)
			structFields := []*types.Var{
				types.NewField(token.NoPos, nil, "_data", emptyIface, false),
				types.NewField(token.NoPos, nil, "_hasValue", types.Typ[types.Bool], false),
			}
			structTags := []string{"", ""}

			for _, m := range info.methods {
				fields = append(fields, &ast.Field{
					Names: []*ast.Ident{ast.NewIdent("_" + m.name)},
					Type:  v.buildMethodFuncType(m),
				})
				// Create a func signature type for the types.Struct
				funcSig := v.buildMethodSignatureType(m)
				structFields = append(structFields, types.NewField(token.NoPos, nil, "_"+m.name, funcSig, false))
				structTags = append(structTags, "")
			}

			info.structType = types.NewStruct(structFields, structTags)

			ts.Type = &ast.StructType{
				Fields: &ast.FieldList{List: fields},
			}

			// Update the named type's underlying from interface to struct so that
			// downstream emitters (C++, Rust, etc.) see Expr as a struct, not interface.
			// Without this, getCppTypeName falls through to std::any for []Expr elements.
			if v.pkg != nil && v.pkg.TypesInfo != nil {
				if obj := v.pkg.TypesInfo.Defs[ts.Name]; obj != nil {
					if typeName, ok := obj.(*types.TypeName); ok {
						if named, ok := typeName.Type().(*types.Named); ok {
							named.SetUnderlying(info.structType)
						}
					}
				}
			}
		}
	}
}

// buildMethodFuncType builds func(interface{}, params...) results for a method.
// Parameter names are stripped because Go doesn't allow mixing named and unnamed
// params in function types (the interface{} receiver has no name).
func (v *ifaceLoweringVisitor) buildMethodFuncType(m ifaceMethod) *ast.FuncType {
	// First param is interface{} for the receiver
	selfParam := &ast.Field{
		Type: &ast.InterfaceType{Methods: &ast.FieldList{}},
	}
	params := []*ast.Field{selfParam}
	// Copy params without names (for valid Go func type syntax in struct fields)
	for _, f := range m.params {
		params = append(params, &ast.Field{Type: f.Type})
	}

	ft := &ast.FuncType{
		Params: &ast.FieldList{List: params},
	}
	if len(m.results) > 0 {
		resultsCopy := make([]*ast.Field, len(m.results))
		for i, r := range m.results {
			resultsCopy[i] = &ast.Field{Type: r.Type}
		}
		ft.Results = &ast.FieldList{List: resultsCopy}
	}
	return ft
}

// buildMethodSignatureType builds a types.Signature for a method (for TypesInfo registration).
func (v *ifaceLoweringVisitor) buildMethodSignatureType(m ifaceMethod) *types.Signature {
	emptyIface := types.NewInterfaceType(nil, nil)
	params := []*types.Var{
		types.NewParam(token.NoPos, nil, "_self", emptyIface),
	}
	// Add remaining params — use interface{} as a placeholder type for all
	for _, p := range m.params {
		paramType := v.resolveFieldType(p)
		for _, name := range p.Names {
			params = append(params, types.NewParam(token.NoPos, nil, name.Name, paramType))
		}
		if len(p.Names) == 0 {
			params = append(params, types.NewParam(token.NoPos, nil, "", paramType))
		}
	}
	var results []*types.Var
	for _, r := range m.results {
		resultType := v.resolveFieldType(r)
		results = append(results, types.NewParam(token.NoPos, nil, "", resultType))
	}
	return types.NewSignatureType(nil, nil, nil, types.NewTuple(params...), types.NewTuple(results...), false)
}

// resolveFieldType resolves the types.Type for an AST field using TypesInfo.
func (v *ifaceLoweringVisitor) resolveFieldType(f *ast.Field) types.Type {
	if v.pkg != nil && v.pkg.TypesInfo != nil {
		if tv, ok := v.pkg.TypesInfo.Types[f.Type]; ok && tv.Type != nil {
			return tv.Type
		}
	}
	// Fallback to interface{} if we can't resolve
	return types.NewInterfaceType(nil, nil)
}

// registerCompositeLitType registers a composite literal in TypesInfo with its struct type.
func (v *ifaceLoweringVisitor) registerCompositeLitType(lit *ast.CompositeLit, info *ifaceInfo) {
	if v.pkg == nil || v.pkg.TypesInfo == nil || info.structType == nil {
		return
	}
	v.pkg.TypesInfo.Types[lit] = types.TypeAndValue{Type: info.structType}
}

// deepCopyFieldList creates a deep copy of a field list to avoid AST sharing issues.
func deepCopyFieldList(fields []*ast.Field) []*ast.Field {
	result := make([]*ast.Field, len(fields))
	for i, f := range fields {
		newField := &ast.Field{
			Type: f.Type,
		}
		if len(f.Names) > 0 {
			newField.Names = make([]*ast.Ident, len(f.Names))
			for j, n := range f.Names {
				newField.Names[j] = ast.NewIdent(n.Name)
			}
		}
		result[i] = newField
	}
	return result
}

// rewriteUsageSites rewrites all usage sites: assignments, method dispatch,
// type assertions, nil comparisons, and nil returns.
func (v *ifaceLoweringVisitor) rewriteUsageSites(file *ast.File, ifaceDecls map[string]*ifaceInfo) {
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Body == nil {
			continue
		}

		// Determine if this function returns an interface type (for nil return rewriting)
		var returnIfaceName string
		if fd.Type.Results != nil {
			for _, r := range fd.Type.Results.List {
				if typeName := v.exprToTypeName(r.Type); typeName != "" {
					if _, ok := ifaceDecls[typeName]; ok {
						returnIfaceName = typeName
						break
					}
				}
			}
		}

		v.rewriteBlock(fd.Body, ifaceDecls, returnIfaceName)
	}
}

// rewriteBlock walks a block statement and rewrites interface usage sites.
func (v *ifaceLoweringVisitor) rewriteBlock(block *ast.BlockStmt, ifaceDecls map[string]*ifaceInfo, returnIfaceName string) {
	for i, stmt := range block.List {
		block.List[i] = v.rewriteStmt(stmt, ifaceDecls, returnIfaceName)
	}
}

// rewriteStmt rewrites a single statement for interface lowering.
func (v *ifaceLoweringVisitor) rewriteStmt(stmt ast.Stmt, ifaceDecls map[string]*ifaceInfo, returnIfaceName string) ast.Stmt {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		v.rewriteAssignStmt(s, ifaceDecls)
	case *ast.DeclStmt:
		v.rewriteDeclStmt(s, ifaceDecls)
	case *ast.ReturnStmt:
		v.rewriteReturnStmt(s, ifaceDecls, returnIfaceName)
	case *ast.ExprStmt:
		s.X = v.rewriteExpr(s.X, ifaceDecls)
	case *ast.IfStmt:
		if s.Init != nil {
			s.Init = v.rewriteStmt(s.Init, ifaceDecls, returnIfaceName)
		}
		s.Cond = v.rewriteExpr(s.Cond, ifaceDecls)
		if s.Body != nil {
			v.rewriteBlock(s.Body, ifaceDecls, returnIfaceName)
		}
		if s.Else != nil {
			switch e := s.Else.(type) {
			case *ast.BlockStmt:
				v.rewriteBlock(e, ifaceDecls, returnIfaceName)
			case *ast.IfStmt:
				v.rewriteStmt(e, ifaceDecls, returnIfaceName)
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			s.Init = v.rewriteStmt(s.Init, ifaceDecls, returnIfaceName)
		}
		if s.Cond != nil {
			s.Cond = v.rewriteExpr(s.Cond, ifaceDecls)
		}
		if s.Post != nil {
			s.Post = v.rewriteStmt(s.Post, ifaceDecls, returnIfaceName)
		}
		if s.Body != nil {
			v.rewriteBlock(s.Body, ifaceDecls, returnIfaceName)
		}
	case *ast.RangeStmt:
		if s.Body != nil {
			v.rewriteBlock(s.Body, ifaceDecls, returnIfaceName)
		}
	case *ast.SwitchStmt:
		if s.Init != nil {
			s.Init = v.rewriteStmt(s.Init, ifaceDecls, returnIfaceName)
		}
		if s.Body != nil {
			for _, cc := range s.Body.List {
				clause := cc.(*ast.CaseClause)
				for j, bs := range clause.Body {
					clause.Body[j] = v.rewriteStmt(bs, ifaceDecls, returnIfaceName)
				}
			}
		}
	case *ast.BlockStmt:
		v.rewriteBlock(s, ifaceDecls, returnIfaceName)
	}
	return stmt
}

// rewriteExpr rewrites expressions for interface lowering (method dispatch,
// type assertions, nil comparisons).
func (v *ifaceLoweringVisitor) rewriteExpr(expr ast.Expr, ifaceDecls map[string]*ifaceInfo) ast.Expr {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.CallExpr:
		return v.rewriteCallExpr(e, ifaceDecls)
	case *ast.TypeAssertExpr:
		return v.rewriteTypeAssertExpr(e, ifaceDecls)
	case *ast.BinaryExpr:
		return v.rewriteBinaryExpr(e, ifaceDecls)
	case *ast.ParenExpr:
		e.X = v.rewriteExpr(e.X, ifaceDecls)
		return e
	case *ast.UnaryExpr:
		e.X = v.rewriteExpr(e.X, ifaceDecls)
		return e
	}
	return expr
}

// rewriteCallExpr rewrites method dispatch on interface types:
// w.Write(buf) → w._Write(w._data, buf)
func (v *ifaceLoweringVisitor) rewriteCallExpr(call *ast.CallExpr, ifaceDecls map[string]*ifaceInfo) ast.Expr {
	// Rewrite args recursively
	for i, arg := range call.Args {
		call.Args[i] = v.rewriteExpr(arg, ifaceDecls)
	}

	selExpr, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		// Not a selector call, just rewrite Fun
		call.Fun = v.rewriteExpr(call.Fun, ifaceDecls)
		return call
	}

	// Determine the receiver type name
	receiverTypeName := v.resolveIfaceTypeName(selExpr.X, ifaceDecls)
	if receiverTypeName == "" {
		return call
	}

	info, ok := ifaceDecls[receiverTypeName]
	if !ok {
		return call
	}

	// Check if the selector matches an interface method
	methodName := selExpr.Sel.Name
	isIfaceMethod := false
	for _, m := range info.methods {
		if m.name == methodName {
			isIfaceMethod = true
			break
		}
	}
	if !isIfaceMethod {
		return call
	}

	// Rewrite: w.Write(buf) → w._Write(w._data, buf)
	newFun := &ast.SelectorExpr{
		X:   selExpr.X,
		Sel: ast.NewIdent("_" + methodName),
	}

	// New args: w._data, original_args...
	dataArg := &ast.SelectorExpr{
		X:   deepCopyExpr(selExpr.X),
		Sel: ast.NewIdent("_data"),
	}
	v.registerAsEmptyInterface(dataArg)
	newArgs := make([]ast.Expr, 0, len(call.Args)+1)
	newArgs = append(newArgs, dataArg)
	newArgs = append(newArgs, call.Args...)

	call.Fun = newFun
	call.Args = newArgs
	return call
}

// rewriteTypeAssertExpr rewrites type assertions on interface types:
// w.(File) → w._data.(File)
func (v *ifaceLoweringVisitor) rewriteTypeAssertExpr(ta *ast.TypeAssertExpr, ifaceDecls map[string]*ifaceInfo) ast.Expr {
	typeName := v.resolveIfaceTypeName(ta.X, ifaceDecls)
	if typeName == "" {
		return ta
	}
	if _, ok := ifaceDecls[typeName]; !ok {
		return ta
	}

	// Rewrite: w.(File) → w._data.(File)
	dataSel := &ast.SelectorExpr{
		X:   ta.X,
		Sel: ast.NewIdent("_data"),
	}
	v.registerAsEmptyInterface(dataSel)
	ta.X = dataSel
	return ta
}

// rewriteBinaryExpr rewrites nil comparisons on interface types:
// w == nil → w._data == nil
func (v *ifaceLoweringVisitor) rewriteBinaryExpr(be *ast.BinaryExpr, ifaceDecls map[string]*ifaceInfo) ast.Expr {
	be.X = v.rewriteExpr(be.X, ifaceDecls)
	be.Y = v.rewriteExpr(be.Y, ifaceDecls)

	if be.Op != token.EQL && be.Op != token.NEQ {
		return be
	}

	// Rewrite nil comparisons:
	// w == nil → w._hasValue == false
	// w != nil → w._hasValue != false  (which is w._hasValue == true)
	if isNilIdent(be.Y) {
		typeName := v.resolveIfaceTypeName(be.X, ifaceDecls)
		if typeName != "" {
			if _, ok := ifaceDecls[typeName]; ok {
				be.X = &ast.SelectorExpr{
					X:   be.X,
					Sel: ast.NewIdent("_hasValue"),
				}
				be.Y = ast.NewIdent("false")
			}
		}
	} else if isNilIdent(be.X) {
		typeName := v.resolveIfaceTypeName(be.Y, ifaceDecls)
		if typeName != "" {
			if _, ok := ifaceDecls[typeName]; ok {
				be.Y = &ast.SelectorExpr{
					X:   be.Y,
					Sel: ast.NewIdent("_hasValue"),
				}
				be.X = ast.NewIdent("false")
			}
		}
	}
	return be
}

// rewriteAssignStmt rewrites assignment statements where the LHS is an interface type.
func (v *ifaceLoweringVisitor) rewriteAssignStmt(s *ast.AssignStmt, ifaceDecls map[string]*ifaceInfo) {
	// Rewrite RHS exprs first (handle nested calls, type assertions, etc.)
	for i, rhs := range s.Rhs {
		s.Rhs[i] = v.rewriteExpr(rhs, ifaceDecls)
	}

	if len(s.Lhs) != len(s.Rhs) {
		return
	}

	for i := 0; i < len(s.Lhs); i++ {
		lhsTypeName := v.resolveAssignTargetIfaceName(s.Lhs[i], ifaceDecls)
		if lhsTypeName == "" {
			continue
		}
		info, ok := ifaceDecls[lhsTypeName]
		if !ok {
			continue
		}

		rhs := s.Rhs[i]

		// Case 1: nil assignment → InterfaceName{_data: nil, _hasValue: false, ...}
		if isNilIdent(rhs) {
			s.Rhs[i] = v.buildNilIfaceLit(lhsTypeName, info)
			continue
		}

		// Case 2: Interface-to-interface assignment
		rhsTypeName := v.resolveIfaceTypeName(rhs, ifaceDecls)
		if rhsTypeName != "" {
			if rhsInfo, ok := ifaceDecls[rhsTypeName]; ok {
				s.Rhs[i] = v.buildIfaceToIfaceAssign(lhsTypeName, info, rhs, rhsInfo)
				continue
			}
		}

		// Case 3: Concrete-to-interface assignment
		s.Rhs[i] = v.buildConcreteToIfaceAssign(lhsTypeName, info, rhs)
	}
}

// rewriteDeclStmt handles var declarations with interface types:
// var w Writer = expr  or  var w Writer (zero value)
func (v *ifaceLoweringVisitor) rewriteDeclStmt(s *ast.DeclStmt, ifaceDecls map[string]*ifaceInfo) {
	gd, ok := s.Decl.(*ast.GenDecl)
	if !ok || gd.Tok != token.VAR {
		return
	}
	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		typeName := v.exprToTypeName(vs.Type)
		if typeName == "" {
			continue
		}
		info, ok := ifaceDecls[typeName]
		if !ok {
			continue
		}

		// If no initializer, add explicit zero-value struct literal.
		// This ensures C++ emits Writer{} instead of Writer w; (which
		// doesn't zero-initialize bool fields in C++), and Rust doesn't
		// use ..Default::default() (which fails for Rc<dyn Fn> fields).
		if len(vs.Values) == 0 {
			vs.Values = []ast.Expr{v.buildNilIfaceLit(typeName, info)}
			continue
		}

		// Handle initialization values
		for i, val := range vs.Values {
			val = v.rewriteExpr(val, ifaceDecls)

			if isNilIdent(val) {
				vs.Values[i] = v.buildNilIfaceLit(typeName, info)
				continue
			}

			// Check interface-to-interface
			rhsTypeName := v.resolveIfaceTypeName(val, ifaceDecls)
			if rhsTypeName != "" {
				if rhsInfo, ok := ifaceDecls[rhsTypeName]; ok {
					vs.Values[i] = v.buildIfaceToIfaceAssign(typeName, info, val, rhsInfo)
					continue
				}
			}

			// Concrete-to-interface
			vs.Values[i] = v.buildConcreteToIfaceAssign(typeName, info, val)
		}
	}
}

// rewriteReturnStmt rewrites return nil in functions returning an interface type.
func (v *ifaceLoweringVisitor) rewriteReturnStmt(s *ast.ReturnStmt, ifaceDecls map[string]*ifaceInfo, returnIfaceName string) {
	for i, result := range s.Results {
		s.Results[i] = v.rewriteExpr(result, ifaceDecls)
	}

	if returnIfaceName == "" {
		return
	}
	if info, ok := ifaceDecls[returnIfaceName]; ok {
		for i, result := range s.Results {
			if isNilIdent(result) {
				s.Results[i] = v.buildNilIfaceLit(returnIfaceName, info)
			}
		}
	}
}

// buildConcreteToIfaceAssign builds a composite literal for concrete-to-interface assignment:
// Writer{_data: concreteVal, _Write: func(_self interface{}, data []byte) int {
//
//	return ConcreteType_Write(_self.(ConcreteType), data)
//
// }}
func (v *ifaceLoweringVisitor) buildConcreteToIfaceAssign(ifaceName string, info *ifaceInfo, concreteExpr ast.Expr) ast.Expr {
	concreteTypeName := v.resolveConcreteTypeName(concreteExpr)
	if concreteTypeName == "" {
		// Can't determine concrete type — fall back to direct assignment.
		// This handles cases like function calls returning unknown types.
		return v.buildConcreteToIfaceAssignUnknown(ifaceName, info, concreteExpr)
	}

	elts := []ast.Expr{
		&ast.KeyValueExpr{
			Key:   ast.NewIdent("_data"),
			Value: concreteExpr,
		},
		&ast.KeyValueExpr{
			Key:   ast.NewIdent("_hasValue"),
			Value: ast.NewIdent("true"),
		},
	}

	for _, m := range info.methods {
		wrapper := v.buildWrapperLambda(m, concreteTypeName)
		elts = append(elts, &ast.KeyValueExpr{
			Key:   ast.NewIdent("_" + m.name),
			Value: wrapper,
		})
	}

	lit := &ast.CompositeLit{
		Type: ast.NewIdent(ifaceName),
		Elts: elts,
	}
	v.registerCompositeLitType(lit, info)
	return lit
}

// buildConcreteToIfaceAssignUnknown handles cases where we can't determine the
// concrete type name (e.g., function call results). We still store in _data but
// leave method fields nil. This is a partial lowering — method calls will panic
// at runtime like a nil interface would.
func (v *ifaceLoweringVisitor) buildConcreteToIfaceAssignUnknown(ifaceName string, info *ifaceInfo, expr ast.Expr) ast.Expr {
	elts := []ast.Expr{
		&ast.KeyValueExpr{
			Key:   ast.NewIdent("_data"),
			Value: expr,
		},
		&ast.KeyValueExpr{
			Key:   ast.NewIdent("_hasValue"),
			Value: ast.NewIdent("true"),
		},
	}

	lit := &ast.CompositeLit{
		Type: ast.NewIdent(ifaceName),
		Elts: elts,
	}
	v.registerCompositeLitType(lit, info)
	return lit
}

// buildWrapperLambda builds a wrapper lambda for a method:
// func(_self interface{}, params...) results {
//
//	return ConcreteType_Method(_self.(ConcreteType), params...)
//
// }
func (v *ifaceLoweringVisitor) buildWrapperLambda(m ifaceMethod, concreteTypeName string) ast.Expr {
	// Build param list: _self interface{}, then original params
	selfIdent := ast.NewIdent("_self")
	selfParam := &ast.Field{
		Names: []*ast.Ident{selfIdent},
		Type:  &ast.InterfaceType{Methods: &ast.FieldList{}},
	}
	params := []*ast.Field{selfParam}
	params = append(params, deepCopyFieldList(m.params)...)

	// Register _self in TypesInfo.Defs as interface{}
	if v.pkg != nil && v.pkg.TypesInfo != nil {
		emptyIface := types.NewInterfaceType(nil, nil)
		v.pkg.TypesInfo.Defs[selfIdent] = types.NewVar(token.NoPos, nil, "_self", emptyIface)
	}

	// Ensure all params have names (needed for referencing in the call)
	paramCounter := 0
	for _, p := range params[1:] { // skip _self
		if len(p.Names) == 0 {
			p.Names = []*ast.Ident{ast.NewIdent(fmt.Sprintf("_p%d", paramCounter))}
			paramCounter++
		}
	}

	// Register all non-_self params in TypesInfo.Defs with their resolved types
	if v.pkg != nil && v.pkg.TypesInfo != nil {
		for _, p := range params[1:] {
			paramType := v.resolveFieldType(p)
			for _, name := range p.Names {
				v.pkg.TypesInfo.Defs[name] = types.NewVar(token.NoPos, nil, name.Name, paramType)
			}
		}
	}

	// Build the inner call: ConcreteType_Method(_self.(ConcreteType), param1, param2, ...)
	callArgs := []ast.Expr{
		&ast.TypeAssertExpr{
			X:    ast.NewIdent("_self"),
			Type: ast.NewIdent(concreteTypeName),
		},
	}
	for _, p := range params[1:] { // skip _self
		for _, name := range p.Names {
			callArgs = append(callArgs, ast.NewIdent(name.Name))
		}
	}

	innerCall := &ast.CallExpr{
		Fun:  ast.NewIdent(concreteTypeName + "_" + m.name),
		Args: callArgs,
	}

	// Build body
	var bodyStmt ast.Stmt
	if len(m.results) > 0 {
		bodyStmt = &ast.ReturnStmt{
			Results: []ast.Expr{innerCall},
		}
	} else {
		bodyStmt = &ast.ExprStmt{X: innerCall}
	}

	funcType := &ast.FuncType{
		Params: &ast.FieldList{List: params},
	}
	if len(m.results) > 0 {
		funcType.Results = &ast.FieldList{List: deepCopyFieldList(m.results)}
	}

	return &ast.FuncLit{
		Type: funcType,
		Body: &ast.BlockStmt{
			List: []ast.Stmt{bodyStmt},
		},
	}
}

// buildIfaceToIfaceAssign builds a composite literal for interface-to-interface assignment:
// Writer{_data: rw._data, _Write: rw._Write}
func (v *ifaceLoweringVisitor) buildIfaceToIfaceAssign(lhsName string, lhsInfo *ifaceInfo, rhsExpr ast.Expr, rhsInfo *ifaceInfo) ast.Expr {
	elts := []ast.Expr{
		&ast.KeyValueExpr{
			Key: ast.NewIdent("_data"),
			Value: &ast.SelectorExpr{
				X:   deepCopyExpr(rhsExpr),
				Sel: ast.NewIdent("_data"),
			},
		},
		&ast.KeyValueExpr{
			Key: ast.NewIdent("_hasValue"),
			Value: &ast.SelectorExpr{
				X:   deepCopyExpr(rhsExpr),
				Sel: ast.NewIdent("_hasValue"),
			},
		},
	}

	for _, m := range lhsInfo.methods {
		// Find matching method in rhs interface
		found := false
		for _, rm := range rhsInfo.methods {
			if rm.name == m.name {
				found = true
				break
			}
		}
		if found {
			elts = append(elts, &ast.KeyValueExpr{
				Key: ast.NewIdent("_" + m.name),
				Value: &ast.SelectorExpr{
					X:   deepCopyExpr(rhsExpr),
					Sel: ast.NewIdent("_" + m.name),
				},
			})
		}
	}

	lit := &ast.CompositeLit{
		Type: ast.NewIdent(lhsName),
		Elts: elts,
	}
	v.registerCompositeLitType(lit, lhsInfo)
	return lit
}

// buildNilIfaceLit builds a zero-value composite literal with all fields explicitly set,
// so that Rust doesn't need ..Default::default() (which fails for Rc<dyn Fn(...)> fields).
// Method fields get dummy no-op lambdas (returns zero value) since nil interfaces
// should never have their methods called.
func (v *ifaceLoweringVisitor) buildNilIfaceLit(typeName string, info *ifaceInfo) *ast.CompositeLit {
	elts := []ast.Expr{
		&ast.KeyValueExpr{
			Key:   ast.NewIdent("_data"),
			Value: ast.NewIdent("nil"),
		},
		&ast.KeyValueExpr{
			Key:   ast.NewIdent("_hasValue"),
			Value: ast.NewIdent("false"),
		},
	}
	for _, m := range info.methods {
		elts = append(elts, &ast.KeyValueExpr{
			Key:   ast.NewIdent("_" + m.name),
			Value: v.buildNilMethodLambda(m),
		})
	}
	lit := &ast.CompositeLit{
		Type: ast.NewIdent(typeName),
		Elts: elts,
	}
	v.registerCompositeLitType(lit, info)
	return lit
}

// buildNilMethodLambda creates a dummy no-op function literal matching the method signature.
// Used for nil interface values — these lambdas should never be called.
func (v *ifaceLoweringVisitor) buildNilMethodLambda(m ifaceMethod) ast.Expr {
	selfIdent := ast.NewIdent("_self")
	selfParam := &ast.Field{
		Names: []*ast.Ident{selfIdent},
		Type:  &ast.InterfaceType{Methods: &ast.FieldList{}},
	}
	params := []*ast.Field{selfParam}
	params = append(params, deepCopyFieldList(m.params)...)

	// Register _self in TypesInfo
	if v.pkg != nil && v.pkg.TypesInfo != nil {
		emptyIface := types.NewInterfaceType(nil, nil)
		v.pkg.TypesInfo.Defs[selfIdent] = types.NewVar(token.NoPos, nil, "_self", emptyIface)
	}

	// Give names to unnamed params and register in TypesInfo
	paramCounter := 0
	for _, p := range params[1:] {
		if len(p.Names) == 0 {
			p.Names = []*ast.Ident{ast.NewIdent(fmt.Sprintf("_p%d", paramCounter))}
			paramCounter++
		}
		if v.pkg != nil && v.pkg.TypesInfo != nil {
			paramType := v.resolveFieldType(p)
			for _, name := range p.Names {
				v.pkg.TypesInfo.Defs[name] = types.NewVar(token.NoPos, nil, name.Name, paramType)
			}
		}
	}

	funcType := &ast.FuncType{
		Params: &ast.FieldList{List: params},
	}
	if len(m.results) > 0 {
		funcType.Results = &ast.FieldList{List: deepCopyFieldList(m.results)}
	}

	// Build body: return zero value(s) if has results, empty otherwise
	var bodyStmts []ast.Stmt
	if len(m.results) > 0 {
		var zeroVals []ast.Expr
		for _, r := range m.results {
			zeroVals = append(zeroVals, v.zeroValueExpr(r.Type))
		}
		bodyStmts = append(bodyStmts, &ast.ReturnStmt{Results: zeroVals})
	}

	return &ast.FuncLit{
		Type: funcType,
		Body: &ast.BlockStmt{List: bodyStmts},
	}
}

// zeroValueExpr returns an AST expression for the zero value of a given type AST node.
func (v *ifaceLoweringVisitor) zeroValueExpr(typeExpr ast.Expr) ast.Expr {
	switch t := typeExpr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64",
			"float32", "float64", "byte", "rune":
			return &ast.BasicLit{Kind: token.INT, Value: "0"}
		case "bool":
			return ast.NewIdent("false")
		case "string":
			return &ast.BasicLit{Kind: token.STRING, Value: `""`}
		}
	case *ast.InterfaceType:
		return ast.NewIdent("nil")
	case *ast.ArrayType:
		return ast.NewIdent("nil")
	}
	// Default: return zero-value of int
	return &ast.BasicLit{Kind: token.INT, Value: "0"}
}

// resolveIfaceTypeName resolves the interface type name of an expression using TypesInfo.
func (v *ifaceLoweringVisitor) resolveIfaceTypeName(expr ast.Expr, ifaceDecls map[string]*ifaceInfo) string {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return ""
	}
	if tv, ok := v.pkg.TypesInfo.Types[expr]; ok && tv.Type != nil {
		return v.typeToIfaceName(tv.Type, ifaceDecls)
	}
	// Try ident lookup via Uses
	if ident, ok := expr.(*ast.Ident); ok {
		if obj := v.pkg.TypesInfo.Uses[ident]; obj != nil {
			return v.typeToIfaceName(obj.Type(), ifaceDecls)
		}
		if obj := v.pkg.TypesInfo.Defs[ident]; obj != nil {
			return v.typeToIfaceName(obj.Type(), ifaceDecls)
		}
	}
	return ""
}

// typeToIfaceName extracts a named type's name if it's in our ifaceDecls.
func (v *ifaceLoweringVisitor) typeToIfaceName(t types.Type, ifaceDecls map[string]*ifaceInfo) string {
	if named, ok := t.(*types.Named); ok {
		name := named.Obj().Name()
		if _, ok := ifaceDecls[name]; ok {
			return name
		}
	}
	return ""
}

// resolveAssignTargetIfaceName resolves the interface type name for an assignment LHS.
func (v *ifaceLoweringVisitor) resolveAssignTargetIfaceName(expr ast.Expr, ifaceDecls map[string]*ifaceInfo) string {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return ""
	}
	// For idents, check Defs first (short var decl), then Uses
	if ident, ok := expr.(*ast.Ident); ok {
		if obj := v.pkg.TypesInfo.Defs[ident]; obj != nil {
			return v.typeToIfaceName(obj.Type(), ifaceDecls)
		}
		if obj := v.pkg.TypesInfo.Uses[ident]; obj != nil {
			return v.typeToIfaceName(obj.Type(), ifaceDecls)
		}
		if tv, ok := v.pkg.TypesInfo.Types[ident]; ok {
			return v.typeToIfaceName(tv.Type, ifaceDecls)
		}
	}
	// For other exprs, check Types
	if tv, ok := v.pkg.TypesInfo.Types[expr]; ok {
		return v.typeToIfaceName(tv.Type, ifaceDecls)
	}
	return ""
}

// resolveConcreteTypeName resolves the concrete type name of an expression.
func (v *ifaceLoweringVisitor) resolveConcreteTypeName(expr ast.Expr) string {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return ""
	}
	var t types.Type
	if tv, ok := v.pkg.TypesInfo.Types[expr]; ok {
		t = tv.Type
	} else if ident, ok := expr.(*ast.Ident); ok {
		if obj := v.pkg.TypesInfo.Uses[ident]; obj != nil {
			t = obj.Type()
		}
	}
	if t == nil {
		return ""
	}
	// Unwrap pointer
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	if named, ok := t.(*types.Named); ok {
		return named.Obj().Name()
	}
	return ""
}

// exprToTypeName extracts a simple type name from a type expression.
func (v *ifaceLoweringVisitor) exprToTypeName(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// registerAsEmptyInterface registers an expression in TypesInfo.Types as having
// the empty interface{} type. This is needed so downstream passes (sema) can
// determine the type of rewritten expressions like w._data.
func (v *ifaceLoweringVisitor) registerAsEmptyInterface(expr ast.Expr) {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return
	}
	emptyIface := types.NewInterfaceType(nil, nil)
	v.pkg.TypesInfo.Types[expr] = types.TypeAndValue{Type: emptyIface}
}

// deepCopyExpr creates a shallow copy of an expression suitable for reuse in AST.
func deepCopyExpr(expr ast.Expr) ast.Expr {
	switch e := expr.(type) {
	case *ast.Ident:
		return ast.NewIdent(e.Name)
	case *ast.SelectorExpr:
		return &ast.SelectorExpr{
			X:   deepCopyExpr(e.X),
			Sel: ast.NewIdent(e.Sel.Name),
		}
	case *ast.IndexExpr:
		return &ast.IndexExpr{
			X:     deepCopyExpr(e.X),
			Index: deepCopyExpr(e.Index),
		}
	}
	return expr
}

