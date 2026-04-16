package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// MemoryLayoutPass transforms pool-based AoS (Array-of-Structs) layout into
// SoA (Structure-of-Arrays) layout for improved cache utilization.
// Runs after pointer lowering on the already-lowered AST.
// Auto-detects pools with structs having MinFields or more fields.
type MemoryLayoutPass struct {
	MinFields int // threshold for auto-detection (default 4)
}

func (p *MemoryLayoutPass) Name() string { return "MemoryLayout" }
func (p *MemoryLayoutPass) ProLog()      {}
func (p *MemoryLayoutPass) EpiLog()      {}

func (p *MemoryLayoutPass) Visitors(pkg *packages.Package) []ast.Visitor {
	minF := p.MinFields
	if minF == 0 {
		minF = 4
	}
	return []ast.Visitor{&memLayoutVisitor{pkg: pkg, minFields: minF}}
}

func (p *MemoryLayoutPass) PreVisit(visitor ast.Visitor) {}

func (p *MemoryLayoutPass) PostVisit(visitor ast.Visitor, visited map[string]struct{}) {
	v := visitor.(*memLayoutVisitor)
	v.transform()
}

// fieldLayoutInfo describes a single struct field for SoA transformation
type fieldLayoutInfo struct {
	name string
	typ  types.Type
}

// poolLayoutInfo describes a pool eligible for SoA transformation
type poolLayoutInfo struct {
	typeName string            // e.g. "Particle"
	fields   []fieldLayoutInfo // ordered list of struct fields
	soaName  string            // e.g. "_SoA_Particle"
}

// memLayoutVisitor collects AST info during walk, transforms in PostVisit
type memLayoutVisitor struct {
	pkg       *packages.Package
	minFields int
	files     []*ast.File
	funcs     []*ast.FuncDecl
	genDecls  []*ast.GenDecl
	poolInfo  map[string]*poolLayoutInfo // pool name → info (e.g. "_pool_Particle" → ...)
}

func (v *memLayoutVisitor) Visit(node ast.Node) ast.Visitor {
	if f, ok := node.(*ast.File); ok {
		v.files = append(v.files, f)
	}
	if fd, ok := node.(*ast.FuncDecl); ok {
		v.funcs = append(v.funcs, fd)
	}
	if gd, ok := node.(*ast.GenDecl); ok && gd.Tok == token.TYPE {
		v.genDecls = append(v.genDecls, gd)
	}
	return v
}

// registerType adds type info for a new AST node
func (v *memLayoutVisitor) registerType(expr ast.Expr, t types.Type) {
	if v.pkg != nil && v.pkg.TypesInfo != nil && t != nil {
		v.pkg.TypesInfo.Types[expr] = types.TypeAndValue{Type: t}
	}
}

// transform runs the SoA transformation after AST collection
func (v *memLayoutVisitor) transform() {
	v.discoverPools()
	if len(v.poolInfo) == 0 {
		return
	}

	// Filter pools by field-access-ratio: only apply SoA when at least one loop
	// accesses < 60% of the struct's fields, indicating a hot/cold field split.
	v.filterByFieldAccessRatio()

	// Generate SoA struct type declarations and insert into AST
	v.generateSoATypes()

	// Rewrite all function declarations
	for _, fd := range v.funcs {
		v.rewriteFuncDecl(fd)
	}

	// Optimize nested loops: hoist invariant SoA reads and use local accumulators
	for _, fd := range v.funcs {
		if fd.Body != nil {
			v.optimizeNestedLoops(fd.Body)
		}
	}

	// Eliminate pool index slice indirection: particles[i] → i
	// Since pointer lowering always produces sequential indices, particles[i] == i.
	v.eliminatePoolIndexSlices()
}

// discoverPools scans for _pool_T []T variable declarations where T has >= minFields fields
func (v *memLayoutVisitor) discoverPools() {
	v.poolInfo = make(map[string]*poolLayoutInfo)

	// Look up struct types from genDecls
	structTypes := make(map[string]*types.Struct) // type name → struct type
	for _, gd := range v.genDecls {
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			typeName := ts.Name.Name
			// Look up in TypesInfo
			if v.pkg != nil && v.pkg.TypesInfo != nil {
				if obj := v.pkg.TypesInfo.Defs[ts.Name]; obj != nil {
					if named, ok := obj.Type().(*types.Named); ok {
						if st, ok := named.Underlying().(*types.Struct); ok {
							structTypes[typeName] = st
						}
					}
				}
			}
		}
	}

	// Scan all functions for _pool_T declarations
	for _, fd := range v.funcs {
		if fd.Body == nil {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			switch stmt := n.(type) {
			case *ast.DeclStmt:
				if gd, ok := stmt.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
					for _, spec := range gd.Specs {
						vs, ok := spec.(*ast.ValueSpec)
						if !ok {
							continue
						}
						for _, name := range vs.Names {
							v.checkPoolCandidate(name.Name, vs.Type, structTypes)
						}
					}
				}
			}
			return true
		})
		// Also check function params for _pool_T []T
		if fd.Type.Params != nil {
			for _, field := range fd.Type.Params.List {
				for _, name := range field.Names {
					v.checkPoolCandidate(name.Name, field.Type, structTypes)
				}
			}
		}
	}

	for poolName, info := range v.poolInfo {
		fmt.Printf("  SoA: auto-detected %s (%d fields >= %d threshold)\n", poolName, len(info.fields), v.minFields)
	}
}

// checkPoolCandidate checks if a variable name/type matches _pool_T []T pattern
func (v *memLayoutVisitor) checkPoolCandidate(name string, typeExpr ast.Expr, structTypes map[string]*types.Struct) {
	if !strings.HasPrefix(name, "_pool_") {
		return
	}
	typeName := strings.TrimPrefix(name, "_pool_")

	// Check the type is []T (ArrayType with no Len)
	arrType, ok := typeExpr.(*ast.ArrayType)
	if !ok {
		return
	}
	if arrType.Len != nil {
		return // fixed-size array, not a slice
	}

	// Get the element type name
	var elemName string
	if ident, ok := arrType.Elt.(*ast.Ident); ok {
		elemName = ident.Name
	}
	if elemName == "" || elemName != typeName {
		return
	}

	// Already discovered?
	if _, exists := v.poolInfo[name]; exists {
		return
	}

	// Look up the struct type
	st, ok := structTypes[typeName]
	if !ok {
		return
	}

	numFields := st.NumFields()
	if numFields < v.minFields {
		return
	}

	// Skip structs that already have slice fields (already SoA layout).
	// This prevents double-transformation of manually written SoA structs.
	for i := 0; i < numFields; i++ {
		ft := st.Field(i).Type()
		if _, isSlice := ft.Underlying().(*types.Slice); isSlice {
			return
		}
	}

	// Collect field info. After pointer lowering, *T fields become int (pool index)
	// and []*T fields become []int. The types.Struct still has the original types,
	// so we must adjust pointer types to match the lowered AST.
	fields := make([]fieldLayoutInfo, numFields)
	for i := 0; i < numFields; i++ {
		f := st.Field(i)
		fieldType := f.Type()
		// *T → int (pointer lowering converts pointer fields to pool indices)
		if _, isPtr := fieldType.(*types.Pointer); isPtr {
			fieldType = types.Typ[types.Int]
		}
		// []*T → []int
		if sliceType, isSlice := fieldType.(*types.Slice); isSlice {
			if _, isPtr := sliceType.Elem().(*types.Pointer); isPtr {
				fieldType = types.NewSlice(types.Typ[types.Int])
			}
		}
		fields[i] = fieldLayoutInfo{
			name: f.Name(),
			typ:  fieldType,
		}
	}

	v.poolInfo[name] = &poolLayoutInfo{
		typeName: typeName,
		fields:   fields,
		soaName:  "_SoA_" + typeName,
	}
}

// filterByFieldAccessRatio removes pools from poolInfo where no loop accesses < 60% of fields.
// This prevents SoA transformation when all loops touch most fields anyway (no cache benefit).
func (v *memLayoutVisitor) filterByFieldAccessRatio() {
	for poolName, info := range v.poolInfo {
		hasLowRatioLoop := false
		totalFields := len(info.fields)
		if totalFields == 0 {
			continue
		}

		for _, fd := range v.funcs {
			if fd.Body == nil {
				continue
			}
			// Walk all for-loops in this function
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				forStmt, ok := n.(*ast.ForStmt)
				if !ok || forStmt.Body == nil {
					return true
				}
				// Count distinct fields of this pool accessed in this loop
				accessedFields := make(map[string]bool)
				v.collectAccessedFields(forStmt.Body, poolName, info, accessedFields)
				if len(accessedFields) == 0 {
					return true // loop doesn't touch this pool
				}
				ratio := float64(len(accessedFields)) / float64(totalFields)
				if ratio < 0.6 {
					hasLowRatioLoop = true
				}
				return !hasLowRatioLoop // stop early if found
			})

			if hasLowRatioLoop {
				break
			}
		}

		if !hasLowRatioLoop {
			fmt.Printf("  SoA: skipping %s (all loops access >= 60%% of %d fields)\n",
				poolName, totalFields)
			delete(v.poolInfo, poolName)
		}
	}

	if len(v.poolInfo) == 0 {
		return
	}
}

// collectAccessedFields finds all field names of a pool's struct accessed in a block.
// Looks for patterns: pool[idx].Field (pre-SoA access after pointer lowering)
func (v *memLayoutVisitor) collectAccessedFields(block *ast.BlockStmt, poolName string, info *poolLayoutInfo, result map[string]bool) {
	if block == nil {
		return
	}
	ast.Inspect(block, func(n ast.Node) bool {
		// Match pool[idx].Field → SelectorExpr with IndexExpr X
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		indexExpr, ok := sel.X.(*ast.IndexExpr)
		if !ok {
			return true
		}
		ident, ok := indexExpr.X.(*ast.Ident)
		if !ok || ident.Name != poolName {
			return true
		}
		// Verify this is a valid field of the struct
		fieldName := sel.Sel.Name
		for _, f := range info.fields {
			if f.name == fieldName {
				result[fieldName] = true
				break
			}
		}
		return true
	})
}

// generateSoATypes creates type _SoA_T struct { Field1 []Type1; ... } declarations
func (v *memLayoutVisitor) generateSoATypes() {
	for _, info := range v.poolInfo {
		// Build struct field list
		fieldList := make([]*ast.Field, len(info.fields))
		for i, f := range info.fields {
			elemTypeExpr := v.typeToExpr(f.typ)
			arrType := &ast.ArrayType{Elt: elemTypeExpr}
			v.registerType(arrType, types.NewSlice(f.typ))
			fieldList[i] = &ast.Field{
				Names: []*ast.Ident{{Name: f.name}},
				Type:  arrType,
			}
		}

		typeSpec := &ast.TypeSpec{
			Name: &ast.Ident{Name: info.soaName},
			Type: &ast.StructType{
				Fields: &ast.FieldList{List: fieldList},
			},
		}

		genDecl := &ast.GenDecl{
			Tok:   token.TYPE,
			Specs: []ast.Spec{typeSpec},
		}

		// Insert into the first file before the first FuncDecl
		if len(v.files) > 0 {
			file := v.files[0]
			var newDecls []ast.Decl
			inserted := false
			for _, decl := range file.Decls {
				if _, ok := decl.(*ast.FuncDecl); ok && !inserted {
					newDecls = append(newDecls, genDecl)
					inserted = true
				}
				newDecls = append(newDecls, decl)
			}
			if !inserted {
				newDecls = append(newDecls, genDecl)
			}
			file.Decls = newDecls
		}

		// Register the SoA type in TypesInfo
		v.registerSoAType(info)
	}
}

// registerSoAType creates and registers a types.Named for the SoA struct
func (v *memLayoutVisitor) registerSoAType(info *poolLayoutInfo) {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return
	}

	// Create struct fields for the types.Struct
	fields := make([]*types.Var, len(info.fields))
	for i, f := range info.fields {
		fields[i] = types.NewVar(token.NoPos, v.pkg.Types, f.name, types.NewSlice(f.typ))
	}
	structType := types.NewStruct(fields, nil)

	// Create named type
	obj := types.NewTypeName(token.NoPos, v.pkg.Types, info.soaName, nil)
	named := types.NewNamed(obj, structType, nil)
	_ = named // registered via obj

	// Register in package scope if available
	if v.pkg.Types != nil && v.pkg.Types.Scope() != nil {
		v.pkg.Types.Scope().Insert(obj)
	}
}

// getSoANamedType retrieves the registered SoA named type
func (v *memLayoutVisitor) getSoANamedType(info *poolLayoutInfo) types.Type {
	if v.pkg != nil && v.pkg.Types != nil && v.pkg.Types.Scope() != nil {
		if obj := v.pkg.Types.Scope().Lookup(info.soaName); obj != nil {
			return obj.Type()
		}
	}
	return nil
}

// rewriteFuncDecl rewrites a function declaration for SoA pools
func (v *memLayoutVisitor) rewriteFuncDecl(fd *ast.FuncDecl) {
	// Rewrite parameter types: []Particle → _SoA_Particle
	v.rewriteFieldList(fd.Type.Params)
	// Rewrite return types: []Particle → _SoA_Particle
	v.rewriteFieldList(fd.Type.Results)
	// Rewrite function body
	if fd.Body != nil {
		v.rewriteBlockStmt(fd.Body)
	}
}

// rewriteFieldList rewrites []T → _SoA_T in parameter/result lists
func (v *memLayoutVisitor) rewriteFieldList(fl *ast.FieldList) {
	if fl == nil {
		return
	}
	for _, field := range fl.List {
		for _, info := range v.poolInfo {
			if v.isSliceOfType(field.Type, info.typeName) {
				soaIdent := &ast.Ident{Name: info.soaName}
				soaType := v.getSoANamedType(info)
				if soaType != nil {
					v.registerType(soaIdent, soaType)
				}
				field.Type = soaIdent
			}
		}
	}
}

// isSliceOfType checks if an AST type is []typeName
func (v *memLayoutVisitor) isSliceOfType(typeExpr ast.Expr, typeName string) bool {
	arrType, ok := typeExpr.(*ast.ArrayType)
	if !ok || arrType.Len != nil {
		return false
	}
	if ident, ok := arrType.Elt.(*ast.Ident); ok {
		return ident.Name == typeName
	}
	return false
}

// rewriteBlockStmt rewrites all statements in a block
func (v *memLayoutVisitor) rewriteBlockStmt(block *ast.BlockStmt) {
	if block == nil {
		return
	}
	var newList []ast.Stmt
	for _, stmt := range block.List {
		expanded := v.rewriteStmt(stmt)
		newList = append(newList, expanded...)
	}
	block.List = newList
}

// rewriteStmt rewrites a single statement, possibly expanding to multiple
func (v *memLayoutVisitor) rewriteStmt(stmt ast.Stmt) []ast.Stmt {
	switch s := stmt.(type) {
	case *ast.DeclStmt:
		return v.rewriteDeclStmt(s)
	case *ast.AssignStmt:
		return v.rewriteAssignStmt(s)
	case *ast.ExprStmt:
		s.X = v.rewriteExpr(s.X)
		return []ast.Stmt{s}
	case *ast.ReturnStmt:
		for i, r := range s.Results {
			s.Results[i] = v.rewriteExpr(r)
		}
		return []ast.Stmt{s}
	case *ast.IfStmt:
		if s.Init != nil {
			expanded := v.rewriteStmt(s.Init)
			if len(expanded) == 1 {
				s.Init = expanded[0]
			}
		}
		s.Cond = v.rewriteExpr(s.Cond)
		v.rewriteBlockStmt(s.Body)
		if s.Else != nil {
			switch e := s.Else.(type) {
			case *ast.BlockStmt:
				v.rewriteBlockStmt(e)
			case *ast.IfStmt:
				v.rewriteStmt(e)
			}
		}
		return []ast.Stmt{s}
	case *ast.ForStmt:
		if s.Init != nil {
			expanded := v.rewriteStmt(s.Init)
			if len(expanded) == 1 {
				s.Init = expanded[0]
			}
		}
		if s.Cond != nil {
			s.Cond = v.rewriteExpr(s.Cond)
		}
		if s.Post != nil {
			expanded := v.rewriteStmt(s.Post)
			if len(expanded) == 1 {
				s.Post = expanded[0]
			}
		}
		v.rewriteBlockStmt(s.Body)
		return []ast.Stmt{s}
	case *ast.RangeStmt:
		s.X = v.rewriteExpr(s.X)
		v.rewriteBlockStmt(s.Body)
		return []ast.Stmt{s}
	case *ast.BlockStmt:
		v.rewriteBlockStmt(s)
		return []ast.Stmt{s}
	case *ast.SwitchStmt:
		if s.Init != nil {
			expanded := v.rewriteStmt(s.Init)
			if len(expanded) == 1 {
				s.Init = expanded[0]
			}
		}
		if s.Tag != nil {
			s.Tag = v.rewriteExpr(s.Tag)
		}
		v.rewriteBlockStmt(s.Body)
		return []ast.Stmt{s}
	case *ast.CaseClause:
		for i, e := range s.List {
			s.List[i] = v.rewriteExpr(e)
		}
		var newBody []ast.Stmt
		for _, bs := range s.Body {
			newBody = append(newBody, v.rewriteStmt(bs)...)
		}
		s.Body = newBody
		return []ast.Stmt{s}
	}
	return []ast.Stmt{stmt}
}

// rewriteDeclStmt rewrites var declarations: var _pool_T []T → _pool_T := _SoA_T{}
// Uses composite literal initialization so all backends create a proper struct (not null).
func (v *memLayoutVisitor) rewriteDeclStmt(s *ast.DeclStmt) []ast.Stmt {
	gd, ok := s.Decl.(*ast.GenDecl)
	if !ok || gd.Tok != token.VAR {
		return []ast.Stmt{s}
	}
	for _, spec := range gd.Specs {
		vs, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range vs.Names {
			if info, ok := v.poolInfo[name.Name]; ok {
				soaType := v.getSoANamedType(info)

				// Create: _pool_T := _SoA_T{}
				soaTypeIdent := &ast.Ident{Name: info.soaName}
				if soaType != nil {
					v.registerType(soaTypeIdent, soaType)
				}
				compLit := &ast.CompositeLit{Type: soaTypeIdent}
				if soaType != nil {
					v.registerType(compLit, soaType)
				}

				lhsIdent := &ast.Ident{Name: name.Name}
				if soaType != nil {
					v.registerType(lhsIdent, soaType)
				}

				assignStmt := &ast.AssignStmt{
					Lhs: []ast.Expr{lhsIdent},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{compLit},
				}
				return []ast.Stmt{assignStmt}
			}
		}
	}
	return []ast.Stmt{s}
}

// rewriteAssignStmt rewrites assignment statements, expanding appends
func (v *memLayoutVisitor) rewriteAssignStmt(s *ast.AssignStmt) []ast.Stmt {
	// Check for pool append pattern: _pool_T = append(_pool_T, T{...})
	if len(s.Lhs) == 1 && len(s.Rhs) == 1 {
		if lhsIdent, ok := s.Lhs[0].(*ast.Ident); ok {
			if info, ok := v.poolInfo[lhsIdent.Name]; ok {
				if call, ok := s.Rhs[0].(*ast.CallExpr); ok {
					if funIdent, ok := call.Fun.(*ast.Ident); ok && funIdent.Name == "append" {
						return v.expandAppend(lhsIdent.Name, info, call)
					}
				}
			}
		}
	}

	// Rewrite LHS and RHS expressions
	for i, lhs := range s.Lhs {
		s.Lhs[i] = v.rewriteExpr(lhs)
	}
	for i, rhs := range s.Rhs {
		s.Rhs[i] = v.rewriteExpr(rhs)
	}
	return []ast.Stmt{s}
}

// expandAppend expands _pool_T = append(_pool_T, T{...}) into per-field appends
func (v *memLayoutVisitor) expandAppend(poolName string, info *poolLayoutInfo, call *ast.CallExpr) []ast.Stmt {
	if len(call.Args) < 2 {
		return []ast.Stmt{&ast.AssignStmt{
			Lhs: []ast.Expr{&ast.Ident{Name: poolName}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{call},
		}}
	}

	// Extract field values from the composite literal (if any)
	fieldValues := make(map[string]ast.Expr)
	if compLit, ok := call.Args[1].(*ast.CompositeLit); ok {
		for _, elt := range compLit.Elts {
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				if keyIdent, ok := kv.Key.(*ast.Ident); ok {
					fieldValues[keyIdent.Name] = kv.Value
				}
			}
		}
	}

	var stmts []ast.Stmt
	soaType := v.getSoANamedType(info)

	for _, f := range info.fields {
		// Get value: from composite lit or zero value
		val, hasVal := fieldValues[f.name]
		if !hasVal {
			val = v.zeroValueExpr(f.typ)
		}
		v.registerType(val, f.typ)

		// _pool_T.Field = append(_pool_T.Field, val)
		poolFieldSel := &ast.SelectorExpr{
			X:   &ast.Ident{Name: poolName},
			Sel: &ast.Ident{Name: f.name},
		}
		sliceType := types.NewSlice(f.typ)
		if soaType != nil {
			poolIdent := &ast.Ident{Name: poolName}
			v.registerType(poolIdent, soaType)
		}
		v.registerType(poolFieldSel, sliceType)

		poolFieldSel2 := &ast.SelectorExpr{
			X:   &ast.Ident{Name: poolName},
			Sel: &ast.Ident{Name: f.name},
		}
		if soaType != nil {
			poolIdent2 := &ast.Ident{Name: poolName}
			v.registerType(poolIdent2, soaType)
		}
		v.registerType(poolFieldSel2, sliceType)

		appendCall := &ast.CallExpr{
			Fun:  &ast.Ident{Name: "append"},
			Args: []ast.Expr{poolFieldSel2, val},
		}
		v.registerType(appendCall, sliceType)

		lhsSel := &ast.SelectorExpr{
			X:   &ast.Ident{Name: poolName},
			Sel: &ast.Ident{Name: f.name},
		}
		if soaType != nil {
			lhsPoolIdent := &ast.Ident{Name: poolName}
			v.registerType(lhsPoolIdent, soaType)
		}
		v.registerType(lhsSel, sliceType)

		assignStmt := &ast.AssignStmt{
			Lhs: []ast.Expr{lhsSel},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{appendCall},
		}
		stmts = append(stmts, assignStmt)
	}

	return stmts
}

// rewriteExpr rewrites expressions, applying SoA transformations
func (v *memLayoutVisitor) rewriteExpr(expr ast.Expr) ast.Expr {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *ast.Ident:
		// Re-register pool idents with SoA type for downstream emitters
		if info, ok := v.poolInfo[e.Name]; ok {
			soaType := v.getSoANamedType(info)
			if soaType != nil {
				v.registerType(e, soaType)
			}
		}
		return e

	case *ast.SelectorExpr:
		// Check for pool[idx].Field pattern → pool.Field[idx]
		if indexExpr, ok := e.X.(*ast.IndexExpr); ok {
			if ident, ok := indexExpr.X.(*ast.Ident); ok {
				if info, ok := v.poolInfo[ident.Name]; ok {
					fieldName := e.Sel.Name
					// Find the field type
					var fieldType types.Type
					for _, f := range info.fields {
						if f.name == fieldName {
							fieldType = f.typ
							break
						}
					}
					if fieldType != nil {
						// Transform: pool[idx].Field → pool.Field[idx]
						soaType := v.getSoANamedType(info)
						poolIdent := &ast.Ident{Name: ident.Name}
						if soaType != nil {
							v.registerType(poolIdent, soaType)
						}

						selExpr := &ast.SelectorExpr{
							X:   poolIdent,
							Sel: &ast.Ident{Name: fieldName},
						}
						sliceType := types.NewSlice(fieldType)
						v.registerType(selExpr, sliceType)

						idx := v.rewriteExpr(indexExpr.Index)
						newIndexExpr := &ast.IndexExpr{
							X:     selExpr,
							Index: idx,
						}
						v.registerType(newIndexExpr, fieldType)
						return newIndexExpr
					}
				}
			}
		}
		// Not a pool pattern, recurse into X
		e.X = v.rewriteExpr(e.X)
		return e

	case *ast.IndexExpr:
		e.X = v.rewriteExpr(e.X)
		e.Index = v.rewriteExpr(e.Index)
		return e

	case *ast.CallExpr:
		return v.rewriteCallExpr(e)

	case *ast.BinaryExpr:
		e.X = v.rewriteExpr(e.X)
		e.Y = v.rewriteExpr(e.Y)
		return e

	case *ast.UnaryExpr:
		e.X = v.rewriteExpr(e.X)
		return e

	case *ast.ParenExpr:
		e.X = v.rewriteExpr(e.X)
		return e

	case *ast.StarExpr:
		e.X = v.rewriteExpr(e.X)
		return e

	case *ast.KeyValueExpr:
		e.Value = v.rewriteExpr(e.Value)
		return e

	case *ast.CompositeLit:
		for i, elt := range e.Elts {
			e.Elts[i] = v.rewriteExpr(elt)
		}
		return e

	case *ast.SliceExpr:
		e.X = v.rewriteExpr(e.X)
		if e.Low != nil {
			e.Low = v.rewriteExpr(e.Low)
		}
		if e.High != nil {
			e.High = v.rewriteExpr(e.High)
		}
		return e
	}

	return expr
}

// rewriteCallExpr rewrites function call expressions
func (v *memLayoutVisitor) rewriteCallExpr(call *ast.CallExpr) ast.Expr {
	// Check for len(_pool_T) → len(_pool_T.FirstField)
	if funIdent, ok := call.Fun.(*ast.Ident); ok && funIdent.Name == "len" {
		if len(call.Args) == 1 {
			if argIdent, ok := call.Args[0].(*ast.Ident); ok {
				if info, ok := v.poolInfo[argIdent.Name]; ok && len(info.fields) > 0 {
					soaType := v.getSoANamedType(info)
					poolIdent := &ast.Ident{Name: argIdent.Name}
					if soaType != nil {
						v.registerType(poolIdent, soaType)
					}
					firstField := info.fields[0]
					selExpr := &ast.SelectorExpr{
						X:   poolIdent,
						Sel: &ast.Ident{Name: firstField.name},
					}
					v.registerType(selExpr, types.NewSlice(firstField.typ))
					call.Args[0] = selExpr
					return call
				}
			}
		}
	}

	// Recurse into arguments
	for i, arg := range call.Args {
		call.Args[i] = v.rewriteExpr(arg)
	}
	call.Fun = v.rewriteExpr(call.Fun)
	return call
}

// typeToExpr converts a types.Type to an ast.Expr
func (v *memLayoutVisitor) typeToExpr(t types.Type) ast.Expr {
	switch typ := t.(type) {
	case *types.Basic:
		return &ast.Ident{Name: typ.Name()}
	case *types.Named:
		return &ast.Ident{Name: typ.Obj().Name()}
	case *types.Slice:
		return &ast.ArrayType{Elt: v.typeToExpr(typ.Elem())}
	}
	return &ast.Ident{Name: t.String()}
}

// zeroValueExpr creates a zero-value expression for a type
func (v *memLayoutVisitor) zeroValueExpr(t types.Type) ast.Expr {
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
		lit := &ast.CompositeLit{Type: v.typeToExpr(t)}
		v.registerType(lit, t)
		return lit
	}
	return &ast.BasicLit{Kind: token.INT, Value: "0"}
}

// ============================================================================
// Nested loop optimization: hoist invariant SoA reads + local accumulators
// ============================================================================

// matchSoAFieldAccess checks if expr is pool.Field[idx] and returns components.
func matchSoAFieldAccess(expr ast.Expr) (poolName, fieldName, idxName string, ok bool) {
	indexExpr, isIdx := expr.(*ast.IndexExpr)
	if !isIdx {
		return "", "", "", false
	}
	selExpr, isSel := indexExpr.X.(*ast.SelectorExpr)
	if !isSel {
		return "", "", "", false
	}
	poolIdent, isIdent := selExpr.X.(*ast.Ident)
	if !isIdent {
		return "", "", "", false
	}
	idxIdent, isIdent2 := indexExpr.Index.(*ast.Ident)
	if !isIdent2 {
		return "", "", "", false
	}
	return poolIdent.Name, selExpr.Sel.Name, idxIdent.Name, true
}

// optimizeNestedLoops walks a block looking for nested for loops with SoA accesses.
func (v *memLayoutVisitor) optimizeNestedLoops(block *ast.BlockStmt) {
	if block == nil {
		return
	}
	for i, stmt := range block.List {
		switch s := stmt.(type) {
		case *ast.ForStmt:
			block.List[i] = v.tryOptimizeOuterFor(s)
		case *ast.IfStmt:
			v.optimizeNestedLoops(s.Body)
			if elseBlock, ok := s.Else.(*ast.BlockStmt); ok {
				v.optimizeNestedLoops(elseBlock)
			}
		case *ast.BlockStmt:
			v.optimizeNestedLoops(s)
		}
	}
}

// accumInfo describes an accumulation pattern: pool.F[pi] = pool.F[pi] + expr
type accumInfo struct {
	poolName  string
	fieldName string
	stmtIdx   int
	addExpr   ast.Expr
	isAdd     bool // true = +, false = -
}

// tryOptimizeOuterFor attempts to optimize a doubly-nested for loop.
func (v *memLayoutVisitor) tryOptimizeOuterFor(outerFor *ast.ForStmt) ast.Stmt {
	if outerFor.Body == nil {
		return outerFor
	}

	// Recursively optimize any deeper nested loops first
	v.optimizeNestedLoops(outerFor.Body)

	// Find inner for loop and outer pool index variable
	outerPoolIdx := ""
	innerForIdx := -1

	for i, stmt := range outerFor.Body.List {
		// Look for: pi := particles[i] or pi := someSlice[i]
		if assign, ok := stmt.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
			if len(assign.Lhs) == 1 && len(assign.Rhs) == 1 {
				if lhsIdent, ok := assign.Lhs[0].(*ast.Ident); ok {
					if _, ok := assign.Rhs[0].(*ast.IndexExpr); ok {
						outerPoolIdx = lhsIdent.Name
					}
				}
			}
		}
		if _, ok := stmt.(*ast.ForStmt); ok {
			innerForIdx = i
		}
	}

	if innerForIdx < 0 || outerPoolIdx == "" {
		return outerFor
	}

	innerFor := outerFor.Body.List[innerForIdx].(*ast.ForStmt)
	if innerFor.Body == nil || len(innerFor.Body.List) == 0 {
		return outerFor
	}

	// Analyze inner loop: collect fields written and accumulated with outerPoolIdx
	writtenWithOuter := make(map[string]bool) // "poolName.fieldName" → true
	var accums []accumInfo

	for stmtIdx, stmt := range innerFor.Body.List {
		assign, ok := stmt.(*ast.AssignStmt)
		if !ok || assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
			continue
		}
		lhsPool, lhsField, lhsIdx, ok := matchSoAFieldAccess(assign.Lhs[0])
		if !ok || lhsIdx != outerPoolIdx {
			continue
		}
		if _, isPool := v.poolInfo[lhsPool]; !isPool {
			continue
		}
		writtenWithOuter[lhsPool+"."+lhsField] = true

		// Check for accumulation: pool.F[pi] = pool.F[pi] + expr
		binExpr, ok := assign.Rhs[0].(*ast.BinaryExpr)
		if !ok || (binExpr.Op != token.ADD && binExpr.Op != token.SUB) {
			continue
		}
		rhsPool, rhsField, rhsIdx, ok := matchSoAFieldAccess(binExpr.X)
		if !ok || rhsPool != lhsPool || rhsField != lhsField || rhsIdx != outerPoolIdx {
			continue
		}
		accums = append(accums, accumInfo{
			poolName:  lhsPool,
			fieldName: lhsField,
			stmtIdx:   stmtIdx,
			addExpr:   binExpr.Y,
			isAdd:     binExpr.Op == token.ADD,
		})
	}

	// Collect fields read with outerPoolIdx (for hoisting)
	readWithOuter := make(map[string]bool)
	v.collectSoAReads(innerFor.Body.List, outerPoolIdx, readWithOuter)

	// Hoistable fields: read with outerPoolIdx but NOT written with outerPoolIdx
	type hoistEntry struct {
		poolName  string
		fieldName string
		varName   string
	}
	var hoists []hoistEntry
	hoistMap := make(map[string]string) // "poolName.fieldName" → varName

	for key := range readWithOuter {
		if !writtenWithOuter[key] {
			// Parse poolName.fieldName from key
			parts := strings.SplitN(key, ".", 2)
			if len(parts) != 2 {
				continue
			}
			varName := "_hoist_" + parts[1]
			hoists = append(hoists, hoistEntry{
				poolName: parts[0], fieldName: parts[1], varName: varName,
			})
			hoistMap[key] = varName
		}
	}

	if len(accums) == 0 && len(hoists) == 0 {
		return outerFor
	}

	// === Apply transformations ===

	// 1. Generate hoist assignments before inner loop
	var preInnerStmts []ast.Stmt
	for _, h := range hoists {
		info := v.poolInfo[h.poolName]
		if info == nil {
			continue
		}
		var fieldType types.Type
		for _, f := range info.fields {
			if f.name == h.fieldName {
				fieldType = f.typ
				break
			}
		}
		if fieldType == nil {
			continue
		}
		// _hoist_Field := pool.Field[pi]
		soaType := v.getSoANamedType(info)
		poolIdent := &ast.Ident{Name: h.poolName}
		if soaType != nil {
			v.registerType(poolIdent, soaType)
		}
		selExpr := &ast.SelectorExpr{X: poolIdent, Sel: &ast.Ident{Name: h.fieldName}}
		v.registerType(selExpr, types.NewSlice(fieldType))
		idxIdent := &ast.Ident{Name: outerPoolIdx}
		v.registerType(idxIdent, types.Typ[types.Int])
		indexExpr := &ast.IndexExpr{X: selExpr, Index: idxIdent}
		v.registerType(indexExpr, fieldType)
		lhsIdent := &ast.Ident{Name: h.varName}
		v.registerType(lhsIdent, fieldType)
		preInnerStmts = append(preInnerStmts, &ast.AssignStmt{
			Lhs: []ast.Expr{lhsIdent},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{indexExpr},
		})
	}

	// 2. Generate accumulator initializations before inner loop
	accumVarMap := make(map[string]string) // "poolName.fieldName" → accVarName
	for _, a := range accums {
		key := a.poolName + "." + a.fieldName
		if _, exists := accumVarMap[key]; exists {
			continue // already created for this field
		}
		varName := "_acc_" + a.fieldName
		accumVarMap[key] = varName
		info := v.poolInfo[a.poolName]
		if info == nil {
			continue
		}
		var fieldType types.Type
		for _, f := range info.fields {
			if f.name == a.fieldName {
				fieldType = f.typ
				break
			}
		}
		if fieldType == nil {
			continue
		}
		zeroVal := v.zeroValueExpr(fieldType)
		v.registerType(zeroVal, fieldType)
		lhsIdent := &ast.Ident{Name: varName}
		v.registerType(lhsIdent, fieldType)
		preInnerStmts = append(preInnerStmts, &ast.AssignStmt{
			Lhs: []ast.Expr{lhsIdent},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{zeroVal},
		})
	}

	// 3. Replace reads and accumulations in inner loop body
	// Replace hoisted reads: pool.Field[pi] → _hoist_Field
	v.replaceHoistedReads(innerFor.Body, hoistMap, outerPoolIdx)

	// Replace accumulations: pool.F[pi] = pool.F[pi] + expr → _acc_F = _acc_F + expr
	for _, a := range accums {
		key := a.poolName + "." + a.fieldName
		accVar := accumVarMap[key]
		if accVar == "" {
			continue
		}
		stmt := innerFor.Body.List[a.stmtIdx]
		assign := stmt.(*ast.AssignStmt)

		info := v.poolInfo[a.poolName]
		var fieldType types.Type
		if info != nil {
			for _, f := range info.fields {
				if f.name == a.fieldName {
					fieldType = f.typ
					break
				}
			}
		}

		accIdent := &ast.Ident{Name: accVar}
		if fieldType != nil {
			v.registerType(accIdent, fieldType)
		}
		accIdent2 := &ast.Ident{Name: accVar}
		if fieldType != nil {
			v.registerType(accIdent2, fieldType)
		}

		op := token.ADD
		if !a.isAdd {
			op = token.SUB
		}
		assign.Lhs[0] = accIdent
		assign.Rhs[0] = &ast.BinaryExpr{
			X:  accIdent2,
			Op: op,
			Y:  a.addExpr,
		}
	}

	// 4. Generate write-back statements after inner loop
	var postInnerStmts []ast.Stmt
	for _, a := range accums {
		key := a.poolName + "." + a.fieldName
		accVar := accumVarMap[key]
		if accVar == "" {
			continue
		}
		// Check we haven't already generated write-back for this field
		alreadyDone := false
		for _, prev := range postInnerStmts {
			if pa, ok := prev.(*ast.AssignStmt); ok {
				if pn, pf, _, ok := matchSoAFieldAccess(pa.Lhs[0]); ok {
					if pn == a.poolName && pf == a.fieldName {
						alreadyDone = true
						break
					}
				}
			}
		}
		if alreadyDone {
			continue
		}

		info := v.poolInfo[a.poolName]
		var fieldType types.Type
		if info != nil {
			for _, f := range info.fields {
				if f.name == a.fieldName {
					fieldType = f.typ
					break
				}
			}
		}

		soaType := v.getSoANamedType(info)
		poolIdent := &ast.Ident{Name: a.poolName}
		if soaType != nil {
			v.registerType(poolIdent, soaType)
		}
		selExpr := &ast.SelectorExpr{X: poolIdent, Sel: &ast.Ident{Name: a.fieldName}}
		if fieldType != nil {
			v.registerType(selExpr, types.NewSlice(fieldType))
		}
		idxIdent := &ast.Ident{Name: outerPoolIdx}
		v.registerType(idxIdent, types.Typ[types.Int])
		lhsIdx := &ast.IndexExpr{X: selExpr, Index: idxIdent}
		if fieldType != nil {
			v.registerType(lhsIdx, fieldType)
		}

		// pool.F[pi] = pool.F[pi] + _acc_F
		poolIdent2 := &ast.Ident{Name: a.poolName}
		if soaType != nil {
			v.registerType(poolIdent2, soaType)
		}
		selExpr2 := &ast.SelectorExpr{X: poolIdent2, Sel: &ast.Ident{Name: a.fieldName}}
		if fieldType != nil {
			v.registerType(selExpr2, types.NewSlice(fieldType))
		}
		idxIdent2 := &ast.Ident{Name: outerPoolIdx}
		v.registerType(idxIdent2, types.Typ[types.Int])
		rhsIdx := &ast.IndexExpr{X: selExpr2, Index: idxIdent2}
		if fieldType != nil {
			v.registerType(rhsIdx, fieldType)
		}

		accIdent := &ast.Ident{Name: accVar}
		if fieldType != nil {
			v.registerType(accIdent, fieldType)
		}

		postInnerStmts = append(postInnerStmts, &ast.AssignStmt{
			Lhs: []ast.Expr{lhsIdx},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{&ast.BinaryExpr{
				X:  rhsIdx,
				Op: token.ADD,
				Y:  accIdent,
			}},
		})
	}

	// 5. Splice pre/post statements around the inner for loop
	var newBody []ast.Stmt
	for i, stmt := range outerFor.Body.List {
		if i == innerForIdx {
			newBody = append(newBody, preInnerStmts...)
			newBody = append(newBody, stmt)
			newBody = append(newBody, postInnerStmts...)
		} else {
			newBody = append(newBody, stmt)
		}
	}
	outerFor.Body.List = newBody

	fmt.Printf("  SoA loop opt: hoisted %d reads, %d accumulators\n", len(hoists), len(accumVarMap))
	return outerFor
}

// collectSoAReads walks inner loop statements collecting pool.Field[outerIdx] reads.
func (v *memLayoutVisitor) collectSoAReads(stmts []ast.Stmt, outerPoolIdx string, result map[string]bool) {
	for _, stmt := range stmts {
		v.walkExprForReads(stmt, outerPoolIdx, result)
	}
}

// walkExprForReads recursively collects SoA field reads indexed by outerPoolIdx.
func (v *memLayoutVisitor) walkExprForReads(node ast.Node, outerPoolIdx string, result map[string]bool) {
	if node == nil {
		return
	}
	ast.Inspect(node, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		if expr, ok := n.(ast.Expr); ok {
			poolName, fieldName, idxName, ok := matchSoAFieldAccess(expr)
			if ok && idxName == outerPoolIdx {
				if _, isPool := v.poolInfo[poolName]; isPool {
					result[poolName+"."+fieldName] = true
				}
			}
		}
		return true
	})
}

// replaceHoistedReads walks the inner loop body replacing pool.Field[pi] with _hoist_Field.
func (v *memLayoutVisitor) replaceHoistedReads(block *ast.BlockStmt, hoistMap map[string]string, outerPoolIdx string) {
	if block == nil || len(hoistMap) == 0 {
		return
	}
	for i, stmt := range block.List {
		block.List[i] = v.replaceReadsInStmt(stmt, hoistMap, outerPoolIdx)
	}
}

func (v *memLayoutVisitor) replaceReadsInStmt(stmt ast.Stmt, hoistMap map[string]string, outerPoolIdx string) ast.Stmt {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		// Don't replace LHS of assignments (those are writes, handled by accumulator)
		for i, rhs := range s.Rhs {
			s.Rhs[i] = v.replaceReadsInExpr(rhs, hoistMap, outerPoolIdx)
		}
		return s
	case *ast.ExprStmt:
		s.X = v.replaceReadsInExpr(s.X, hoistMap, outerPoolIdx)
		return s
	case *ast.IfStmt:
		if s.Cond != nil {
			s.Cond = v.replaceReadsInExpr(s.Cond, hoistMap, outerPoolIdx)
		}
		v.replaceHoistedReads(s.Body, hoistMap, outerPoolIdx)
		return s
	}
	return stmt
}

func (v *memLayoutVisitor) replaceReadsInExpr(expr ast.Expr, hoistMap map[string]string, outerPoolIdx string) ast.Expr {
	if expr == nil {
		return nil
	}
	// Check if this expression matches a hoisted SoA access
	poolName, fieldName, idxName, ok := matchSoAFieldAccess(expr)
	if ok && idxName == outerPoolIdx {
		key := poolName + "." + fieldName
		if varName, exists := hoistMap[key]; exists {
			ident := &ast.Ident{Name: varName}
			if info, exists := v.poolInfo[poolName]; exists {
				for _, f := range info.fields {
					if f.name == fieldName {
						v.registerType(ident, f.typ)
						break
					}
				}
			}
			return ident
		}
	}
	// Recurse into sub-expressions
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		e.X = v.replaceReadsInExpr(e.X, hoistMap, outerPoolIdx)
		e.Y = v.replaceReadsInExpr(e.Y, hoistMap, outerPoolIdx)
	case *ast.ParenExpr:
		e.X = v.replaceReadsInExpr(e.X, hoistMap, outerPoolIdx)
	case *ast.UnaryExpr:
		e.X = v.replaceReadsInExpr(e.X, hoistMap, outerPoolIdx)
	case *ast.CallExpr:
		for i, arg := range e.Args {
			e.Args[i] = v.replaceReadsInExpr(arg, hoistMap, outerPoolIdx)
		}
	case *ast.IndexExpr:
		e.X = v.replaceReadsInExpr(e.X, hoistMap, outerPoolIdx)
		e.Index = v.replaceReadsInExpr(e.Index, hoistMap, outerPoolIdx)
	case *ast.SelectorExpr:
		e.X = v.replaceReadsInExpr(e.X, hoistMap, outerPoolIdx)
	}
	return expr
}

// ============================================================================
// Pool index slice elimination: particles[i] → i
// After pointer lowering, []*T becomes []int (pool index slice) + _pool_T []T.
// Pool indices are always sequential [0,1,2,...,n-1], so sliceParam[i] == i.
// This eliminates the indirection, closing the gap with manual SoA code.
// ============================================================================

// poolIdxSliceInfo describes a pool index slice parameter to eliminate
type poolIdxSliceInfo struct {
	paramName string
	paramPos  int // 0-based flattened position in param list
}

// isSliceOfInt checks if a type expression is []int
func isSliceOfInt(typeExpr ast.Expr) bool {
	arrType, ok := typeExpr.(*ast.ArrayType)
	if !ok || arrType.Len != nil {
		return false
	}
	if ident, ok := arrType.Elt.(*ast.Ident); ok {
		return ident.Name == "int"
	}
	return false
}

// eliminatePoolIndexSlices removes pool index slice parameters and replaces
// sliceParam[idx] with just idx, since pool indices are always sequential.
func (v *memLayoutVisitor) eliminatePoolIndexSlices() {
	// Step 1: Identify pool index slice params in non-main functions.
	// A []int param coexisting with a _pool_* param is a lowered []*T.
	funcPIS := make(map[string][]poolIdxSliceInfo)

	for _, fd := range v.funcs {
		if fd.Name.Name == "main" || fd.Type.Params == nil {
			continue
		}
		hasPool := false
		for _, field := range fd.Type.Params.List {
			for _, name := range field.Names {
				if strings.HasPrefix(name.Name, "_pool_") {
					hasPool = true
				}
			}
		}
		if !hasPool {
			continue
		}
		pos := 0
		for _, field := range fd.Type.Params.List {
			for _, name := range field.Names {
				if !strings.HasPrefix(name.Name, "_pool_") && isSliceOfInt(field.Type) {
					funcPIS[fd.Name.Name] = append(funcPIS[fd.Name.Name],
						poolIdxSliceInfo{name.Name, pos})
				}
				pos++
			}
		}
	}

	if len(funcPIS) == 0 {
		return
	}

	// Step 2: Identify pool index slice local vars in main() by examining call sites.
	mainPoolVars := make(map[string]bool)
	for _, fd := range v.funcs {
		if fd.Name.Name != "main" || fd.Body == nil {
			continue
		}
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			funIdent, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			pis, exists := funcPIS[funIdent.Name]
			if !exists {
				return true
			}
			for _, p := range pis {
				if p.paramPos < len(call.Args) {
					if argIdent, ok := call.Args[p.paramPos].(*ast.Ident); ok {
						mainPoolVars[argIdent.Name] = true
					}
				}
			}
			return true
		})
	}

	// Step 3: Build per-function pool index slice name sets
	funcPoolIdxNames := make(map[string]map[string]bool)
	for funcName, pis := range funcPIS {
		names := make(map[string]bool)
		for _, p := range pis {
			names[p.paramName] = true
		}
		funcPoolIdxNames[funcName] = names
	}
	if len(mainPoolVars) > 0 {
		funcPoolIdxNames["main"] = mainPoolVars
	}

	// Step 4: Replace sliceVar[idx] → idx in all function bodies
	for _, fd := range v.funcs {
		names := funcPoolIdxNames[fd.Name.Name]
		if len(names) == 0 || fd.Body == nil {
			continue
		}
		v.elimReplaceInBlock(fd.Body, names)
	}

	// Step 5: Remove pool index slice args from ALL call sites
	for _, fd := range v.funcs {
		if fd.Body == nil {
			continue
		}
		v.elimRemoveCallArgs(fd.Body, funcPIS)
	}

	// Step 6: Remove pool index slice params from function signatures
	for _, fd := range v.funcs {
		pis := funcPIS[fd.Name.Name]
		if len(pis) == 0 {
			continue
		}
		removeNames := make(map[string]bool)
		for _, p := range pis {
			removeNames[p.paramName] = true
		}
		v.elimRemoveParams(fd, removeNames)
	}

	// Step 7: In main(), remove pool index slice variable declarations and appends
	for _, fd := range v.funcs {
		if fd.Name.Name != "main" || fd.Body == nil || len(mainPoolVars) == 0 {
			continue
		}
		v.elimCleanupMainVars(fd.Body, mainPoolVars)
	}

	fmt.Printf("  SoA: eliminated pool index slices from %d function(s)\n", len(funcPIS))
}

// elimReplaceInBlock walks a block replacing sliceVar[idx] → idx
func (v *memLayoutVisitor) elimReplaceInBlock(block *ast.BlockStmt, names map[string]bool) {
	if block == nil {
		return
	}
	for i, stmt := range block.List {
		block.List[i] = v.elimReplaceInStmt(stmt, names)
	}
}

func (v *memLayoutVisitor) elimReplaceInStmt(stmt ast.Stmt, names map[string]bool) ast.Stmt {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		for i, lhs := range s.Lhs {
			s.Lhs[i] = v.elimReplaceInExpr(lhs, names)
		}
		for i, rhs := range s.Rhs {
			s.Rhs[i] = v.elimReplaceInExpr(rhs, names)
		}
	case *ast.ExprStmt:
		s.X = v.elimReplaceInExpr(s.X, names)
	case *ast.ReturnStmt:
		for i, r := range s.Results {
			s.Results[i] = v.elimReplaceInExpr(r, names)
		}
	case *ast.IfStmt:
		if s.Cond != nil {
			s.Cond = v.elimReplaceInExpr(s.Cond, names)
		}
		v.elimReplaceInBlock(s.Body, names)
		if s.Else != nil {
			switch e := s.Else.(type) {
			case *ast.BlockStmt:
				v.elimReplaceInBlock(e, names)
			case *ast.IfStmt:
				v.elimReplaceInStmt(e, names)
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			v.elimReplaceInStmt(s.Init, names)
		}
		if s.Cond != nil {
			s.Cond = v.elimReplaceInExpr(s.Cond, names)
		}
		if s.Post != nil {
			v.elimReplaceInStmt(s.Post, names)
		}
		v.elimReplaceInBlock(s.Body, names)
	case *ast.RangeStmt:
		s.X = v.elimReplaceInExpr(s.X, names)
		v.elimReplaceInBlock(s.Body, names)
	case *ast.BlockStmt:
		v.elimReplaceInBlock(s, names)
	case *ast.SwitchStmt:
		if s.Tag != nil {
			s.Tag = v.elimReplaceInExpr(s.Tag, names)
		}
		v.elimReplaceInBlock(s.Body, names)
	case *ast.CaseClause:
		for i, e := range s.List {
			s.List[i] = v.elimReplaceInExpr(e, names)
		}
		for i, bs := range s.Body {
			s.Body[i] = v.elimReplaceInStmt(bs, names)
		}
	case *ast.DeclStmt:
		// handle var decls with init values
		if gd, ok := s.Decl.(*ast.GenDecl); ok {
			for _, spec := range gd.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for i, val := range vs.Values {
						vs.Values[i] = v.elimReplaceInExpr(val, names)
					}
				}
			}
		}
	}
	return stmt
}

func (v *memLayoutVisitor) elimReplaceInExpr(expr ast.Expr, names map[string]bool) ast.Expr {
	if expr == nil {
		return nil
	}

	// Match sliceVar[idx] → idx
	if indexExpr, ok := expr.(*ast.IndexExpr); ok {
		if ident, ok := indexExpr.X.(*ast.Ident); ok {
			if names[ident.Name] {
				idx := v.elimReplaceInExpr(indexExpr.Index, names)
				v.registerType(idx, types.Typ[types.Int])
				return idx
			}
		}
	}

	// Recurse into sub-expressions
	switch e := expr.(type) {
	case *ast.BinaryExpr:
		e.X = v.elimReplaceInExpr(e.X, names)
		e.Y = v.elimReplaceInExpr(e.Y, names)
	case *ast.UnaryExpr:
		e.X = v.elimReplaceInExpr(e.X, names)
	case *ast.ParenExpr:
		e.X = v.elimReplaceInExpr(e.X, names)
	case *ast.CallExpr:
		for i, arg := range e.Args {
			e.Args[i] = v.elimReplaceInExpr(arg, names)
		}
	case *ast.IndexExpr:
		e.X = v.elimReplaceInExpr(e.X, names)
		e.Index = v.elimReplaceInExpr(e.Index, names)
	case *ast.SelectorExpr:
		e.X = v.elimReplaceInExpr(e.X, names)
	case *ast.CompositeLit:
		for i, elt := range e.Elts {
			e.Elts[i] = v.elimReplaceInExpr(elt, names)
		}
	case *ast.SliceExpr:
		e.X = v.elimReplaceInExpr(e.X, names)
		if e.Low != nil {
			e.Low = v.elimReplaceInExpr(e.Low, names)
		}
		if e.High != nil {
			e.High = v.elimReplaceInExpr(e.High, names)
		}
	case *ast.KeyValueExpr:
		e.Value = v.elimReplaceInExpr(e.Value, names)
	case *ast.StarExpr:
		e.X = v.elimReplaceInExpr(e.X, names)
	}
	return expr
}

// elimRemoveCallArgs removes pool index slice arguments from call sites
func (v *memLayoutVisitor) elimRemoveCallArgs(block *ast.BlockStmt, funcPIS map[string][]poolIdxSliceInfo) {
	if block == nil {
		return
	}
	for _, stmt := range block.List {
		v.elimRemoveCallArgsInStmt(stmt, funcPIS)
	}
}

func (v *memLayoutVisitor) elimRemoveCallArgsInStmt(stmt ast.Stmt, funcPIS map[string][]poolIdxSliceInfo) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			v.elimRemoveCallArgsInExpr(rhs, funcPIS)
		}
	case *ast.ExprStmt:
		v.elimRemoveCallArgsInExpr(s.X, funcPIS)
	case *ast.ReturnStmt:
		for _, r := range s.Results {
			v.elimRemoveCallArgsInExpr(r, funcPIS)
		}
	case *ast.IfStmt:
		if s.Cond != nil {
			v.elimRemoveCallArgsInExpr(s.Cond, funcPIS)
		}
		v.elimRemoveCallArgs(s.Body, funcPIS)
		if s.Else != nil {
			switch e := s.Else.(type) {
			case *ast.BlockStmt:
				v.elimRemoveCallArgs(e, funcPIS)
			case *ast.IfStmt:
				v.elimRemoveCallArgsInStmt(e, funcPIS)
			}
		}
	case *ast.ForStmt:
		if s.Init != nil {
			v.elimRemoveCallArgsInStmt(s.Init, funcPIS)
		}
		v.elimRemoveCallArgs(s.Body, funcPIS)
	case *ast.BlockStmt:
		v.elimRemoveCallArgs(s, funcPIS)
	case *ast.SwitchStmt:
		v.elimRemoveCallArgs(s.Body, funcPIS)
	case *ast.CaseClause:
		for _, bs := range s.Body {
			v.elimRemoveCallArgsInStmt(bs, funcPIS)
		}
	}
}

func (v *memLayoutVisitor) elimRemoveCallArgsInExpr(expr ast.Expr, funcPIS map[string][]poolIdxSliceInfo) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.CallExpr:
		// Check if this is a call to a function with pool index slices
		if funIdent, ok := e.Fun.(*ast.Ident); ok {
			if pis, exists := funcPIS[funIdent.Name]; exists && len(pis) > 0 {
				removePos := make(map[int]bool)
				for _, p := range pis {
					removePos[p.paramPos] = true
				}
				var newArgs []ast.Expr
				for i, arg := range e.Args {
					if !removePos[i] {
						newArgs = append(newArgs, arg)
					}
				}
				e.Args = newArgs
			}
		}
		// Recurse into remaining args
		for _, arg := range e.Args {
			v.elimRemoveCallArgsInExpr(arg, funcPIS)
		}
	case *ast.BinaryExpr:
		v.elimRemoveCallArgsInExpr(e.X, funcPIS)
		v.elimRemoveCallArgsInExpr(e.Y, funcPIS)
	case *ast.UnaryExpr:
		v.elimRemoveCallArgsInExpr(e.X, funcPIS)
	case *ast.ParenExpr:
		v.elimRemoveCallArgsInExpr(e.X, funcPIS)
	}
}

// elimRemoveParams removes named params from a function signature
func (v *memLayoutVisitor) elimRemoveParams(fd *ast.FuncDecl, removeNames map[string]bool) {
	if fd.Type.Params == nil {
		return
	}
	var newFields []*ast.Field
	for _, field := range fd.Type.Params.List {
		var newNames []*ast.Ident
		for _, name := range field.Names {
			if !removeNames[name.Name] {
				newNames = append(newNames, name)
			}
		}
		if len(newNames) > 0 {
			field.Names = newNames
			newFields = append(newFields, field)
		}
	}
	fd.Type.Params.List = newFields
}

// elimCleanupMainVars removes pool index slice variable declarations and
// append statements in main()
func (v *memLayoutVisitor) elimCleanupMainVars(block *ast.BlockStmt, varNames map[string]bool) {
	// Remove top-level declarations: particles := make([]int, 0)
	var newList []ast.Stmt
	for _, stmt := range block.List {
		if v.elimIsPoolIdxVarDecl(stmt, varNames) {
			continue
		}
		// Recurse into for loops to remove append statements
		if forStmt, ok := stmt.(*ast.ForStmt); ok && forStmt.Body != nil {
			v.elimRemoveAppends(forStmt.Body, varNames)
		}
		newList = append(newList, stmt)
	}
	block.List = newList
}

// elimIsPoolIdxVarDecl checks if a statement declares a pool index slice variable
func (v *memLayoutVisitor) elimIsPoolIdxVarDecl(stmt ast.Stmt, varNames map[string]bool) bool {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok == token.DEFINE && len(s.Lhs) == 1 {
			if lhsIdent, ok := s.Lhs[0].(*ast.Ident); ok {
				if varNames[lhsIdent.Name] {
					return true
				}
			}
		}
	case *ast.DeclStmt:
		if gd, ok := s.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
			for _, spec := range gd.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range vs.Names {
						if varNames[name.Name] {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// elimRemoveAppends removes `poolSlice = append(poolSlice, expr)` from a block
// and replaces now-unused appended variables with _ in their defining assignments.
func (v *memLayoutVisitor) elimRemoveAppends(block *ast.BlockStmt, varNames map[string]bool) {
	// Collect variables being appended (second arg of append calls)
	appendedVars := make(map[string]bool)
	for _, stmt := range block.List {
		if v.elimIsAppendStmt(stmt, varNames) {
			assign := stmt.(*ast.AssignStmt)
			call := assign.Rhs[0].(*ast.CallExpr)
			if len(call.Args) >= 2 {
				if argIdent, ok := call.Args[1].(*ast.Ident); ok {
					appendedVars[argIdent.Name] = true
				}
			}
		}
	}

	// Remove append statements
	var newList []ast.Stmt
	for _, stmt := range block.List {
		if v.elimIsAppendStmt(stmt, varNames) {
			continue
		}
		newList = append(newList, stmt)
	}
	block.List = newList

	// For each appended variable, check if still used in remaining stmts.
	// If unused, replace with _ in assignment LHS and remove var declarations.
	for varName := range appendedVars {
		if !v.elimIsVarUsed(block.List, varName) {
			v.elimBlankifyVar(block, varName)
		}
	}
}

// elimIsVarUsed checks if a variable is used (read) in any statement.
// Excludes the variable appearing only on LHS of assignments (writes don't count as "used").
func (v *memLayoutVisitor) elimIsVarUsed(stmts []ast.Stmt, varName string) bool {
	for _, stmt := range stmts {
		if v.elimIsVarReadInStmt(stmt, varName) {
			return true
		}
	}
	return false
}

// elimIsVarReadInStmt checks if varName is read (not just written) in a statement.
func (v *memLayoutVisitor) elimIsVarReadInStmt(stmt ast.Stmt, varName string) bool {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		// Only check RHS (reads), not LHS (writes)
		for _, rhs := range s.Rhs {
			if v.elimExprContainsVar(rhs, varName) {
				return true
			}
		}
	case *ast.ExprStmt:
		return v.elimExprContainsVar(s.X, varName)
	case *ast.IfStmt:
		if s.Cond != nil && v.elimExprContainsVar(s.Cond, varName) {
			return true
		}
		if s.Body != nil && v.elimIsVarUsed(s.Body.List, varName) {
			return true
		}
	case *ast.ForStmt:
		if s.Cond != nil && v.elimExprContainsVar(s.Cond, varName) {
			return true
		}
		if s.Body != nil && v.elimIsVarUsed(s.Body.List, varName) {
			return true
		}
	case *ast.ReturnStmt:
		for _, r := range s.Results {
			if v.elimExprContainsVar(r, varName) {
				return true
			}
		}
	case *ast.DeclStmt:
		// var declarations: check init values
		if gd, ok := s.Decl.(*ast.GenDecl); ok {
			for _, spec := range gd.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, val := range vs.Values {
						if v.elimExprContainsVar(val, varName) {
							return true
						}
					}
				}
			}
		}
	}
	return false
}

// elimExprContainsVar checks if an expression contains a reference to varName
func (v *memLayoutVisitor) elimExprContainsVar(expr ast.Expr, varName string) bool {
	if expr == nil {
		return false
	}
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		if ident, ok := n.(*ast.Ident); ok && ident.Name == varName {
			found = true
		}
		return !found
	})
	return found
}

// elimBlankifyVar replaces an unused variable with _ (blank identifier) in assignment LHS
// and removes its var declaration.
func (v *memLayoutVisitor) elimBlankifyVar(block *ast.BlockStmt, varName string) {
	// Replace in assignment LHS: p, _pool_T = Func(...) → _, _pool_T = Func(...)
	for _, stmt := range block.List {
		if assign, ok := stmt.(*ast.AssignStmt); ok {
			for i, lhs := range assign.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && ident.Name == varName {
					assign.Lhs[i] = &ast.Ident{Name: "_"}
				}
			}
		}
	}

	// Remove var declarations: var p int (blank _ doesn't need a type declaration)
	var newList []ast.Stmt
	for _, stmt := range block.List {
		if declStmt, ok := stmt.(*ast.DeclStmt); ok {
			if gd, ok := declStmt.Decl.(*ast.GenDecl); ok && gd.Tok == token.VAR {
				isVarDecl := false
				for _, spec := range gd.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, name := range vs.Names {
							if name.Name == varName {
								isVarDecl = true
							}
						}
					}
				}
				if isVarDecl {
					continue // remove this declaration
				}
			}
		}
		newList = append(newList, stmt)
	}
	block.List = newList
}

// elimIsAppendStmt checks if a stmt is: poolSlice = append(poolSlice, expr)
func (v *memLayoutVisitor) elimIsAppendStmt(stmt ast.Stmt, varNames map[string]bool) bool {
	assign, ok := stmt.(*ast.AssignStmt)
	if !ok || assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false
	}
	lhsIdent, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || !varNames[lhsIdent.Name] {
		return false
	}
	callExpr, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	funIdent, ok := callExpr.Fun.(*ast.Ident)
	return ok && funIdent.Name == "append"
}
