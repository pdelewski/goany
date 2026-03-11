package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"strings"

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

// ptrFieldLocalInfo holds info about a local pointer alias to a struct field (p := &s.field)
type ptrFieldLocalInfo struct {
	baseExpr  ast.Expr   // e.g., ident "s"
	fieldName string     // e.g., "age"
	elemType  types.Type // type of the field
}

// ptrAnalysis holds analysis results for a single function
type ptrAnalysis struct {
	ptrParams          map[string]types.Type         // param name -> element type (for *T params)
	ptrVars            map[string]types.Type         // var p *T declarations -> element type
	poolVars           map[string]types.Type         // all vars whose addr is taken (pool index vars)
	ptrLocals          map[string]*ptrLocalInfo      // alias name -> target info (p := &x)
	ptrIndexLocals     map[string]*ptrIndexLocalInfo // alias name -> target info (p := &arr[i])
	ptrFieldLocals     map[string]*ptrFieldLocalInfo // alias name -> target info (p := &s.field)
	ptrCompositeLits   map[string]types.Type         // var name -> elem type (p := &Type{...})
	ptrReturns         map[int]types.Type            // result index → element type (for *T returns)
	ptrParamPools      map[string]types.Type         // pool name → elem type (from *T params)
	callReturnPools    map[string]types.Type         // pool types needed for calling ptr-returning funcs
	sliceOfPtrVars     map[string]types.Type         // var name -> elem type (for []*T vars)
	mapOfPtrVars       map[string]types.Type         // var name -> elem type (for map[K]*T vars)
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

// reportPtrError reports a pointer transform error with source location and exits.
func (v *ptrTransformVisitor) reportPtrError(pos token.Pos, title, description string, suggestions []string) {
	if v.pkg != nil && v.pkg.Fset != nil && pos.IsValid() {
		position := v.pkg.Fset.Position(pos)
		fmt.Printf("\033[31m\033[1mSemantic error: %s\033[0m\n", title)
		fmt.Printf("  \033[36m-->\033[0m %s:%d:%d\n", position.Filename, position.Line, position.Column)
	} else {
		fmt.Printf("\033[31m\033[1mSemantic error: %s\033[0m\n", title)
	}
	fmt.Printf("  %s\n", description)
	if len(suggestions) > 0 {
		fmt.Println()
		fmt.Println("  \033[32mSuggestion:\033[0m")
		for _, s := range suggestions {
			fmt.Printf("    %s\n", s)
		}
	}
	fmt.Println()
	os.Exit(-1)
}

// poolIndexExpr creates _pool_T[index] with proper type registration
func (v *ptrTransformVisitor) poolIndexExpr(index ast.Expr, elemType types.Type) *ast.IndexExpr {
	poolIdent := &ast.Ident{Name: poolNameForType(elemType)}
	v.registerType(poolIdent, types.NewSlice(elemType))
	indexExpr := &ast.IndexExpr{X: poolIdent, Index: index}
	v.registerType(indexExpr, elemType)
	return indexExpr
}

// hoistNestedPoolIndicesInAssign checks if an assignment LHS contains nested pool
// indexing (e.g., _pool_T[_pool_T[x].F1].F2 = val) and hoists the inner pool
// index to a temp variable. This is required for Rust borrow checker safety:
// nested pool indexing creates simultaneous mutable+immutable borrows on the same vector.
// Returns pre-statements (temp var declarations) and modifies LHS in place.
func (v *ptrTransformVisitor) hoistNestedPoolIndicesInAssign(s *ast.AssignStmt) []ast.Stmt {
	var preStmts []ast.Stmt
	for i, lhs := range s.Lhs {
		s.Lhs[i] = v.hoistNestedPoolIndex(lhs, &preStmts)
	}
	return preStmts
}

// hoistNestedPoolIndex walks an LHS expression. If it finds _pool_T[innerExpr]
// where innerExpr contains another _pool_* access, it hoists innerExpr to a temp
// variable and replaces the index with the temp variable reference.
func (v *ptrTransformVisitor) hoistNestedPoolIndex(expr ast.Expr, preStmts *[]ast.Stmt) ast.Expr {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.SelectorExpr:
		// For _pool_T[innerExpr].Field, process the IndexExpr inside
		e.X = v.hoistNestedPoolIndex(e.X, preStmts)
		return e
	case *ast.IndexExpr:
		if ident, ok := e.X.(*ast.Ident); ok && strings.HasPrefix(ident.Name, "_pool_") {
			// This is _pool_T[innerExpr]. Check if innerExpr contains pool access.
			if containsPoolAccess(e.Index) {
				// Hoist innerExpr to a temp variable
				tmpName := fmt.Sprintf("__nested_idx_%d", v.tmpCounter)
				v.tmpCounter++
				tmpIdent := &ast.Ident{Name: tmpName}
				v.registerType(tmpIdent, types.Typ[types.Int])

				// Generate: __nested_idx_N := innerExpr
				declStmt := &ast.AssignStmt{
					Lhs: []ast.Expr{&ast.Ident{Name: tmpName}},
					Tok: token.DEFINE,
					Rhs: []ast.Expr{e.Index},
				}
				*preStmts = append(*preStmts, declStmt)

				// Replace the index with the temp variable
				e.Index = tmpIdent
			}
		}
		return e
	}
	return expr
}

func (v *ptrTransformVisitor) transform() {
	// Rewrite struct types first: *T fields -> []T fields
	v.rewriteStructTypes()

	for _, fd := range v.funcs {
		analysis := v.analyzeFuncDecl(fd)
		if len(analysis.ptrParams) == 0 && len(analysis.ptrVars) == 0 && len(analysis.poolVars) == 0 &&
			len(analysis.ptrLocals) == 0 && len(analysis.ptrIndexLocals) == 0 && len(analysis.ptrFieldLocals) == 0 &&
			len(analysis.ptrCompositeLits) == 0 && len(analysis.ptrReturns) == 0 && len(analysis.callReturnPools) == 0 &&
			len(analysis.sliceOfPtrVars) == 0 && len(analysis.mapOfPtrVars) == 0 && len(analysis.ptrParamPools) == 0 {
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
				// Rewrite []*T → []int in struct fields
				if arrType, ok := field.Type.(*ast.ArrayType); ok {
					if starExpr, ok := arrType.Elt.(*ast.StarExpr); ok {
						if ident, ok := starExpr.X.(*ast.Ident); ok {
							v.poolTypes[ident.Name] = true
						}
						newElt := &ast.Ident{Name: "int"}
						arrType.Elt = newElt
						sliceOfInt := types.NewSlice(types.Typ[types.Int])
						v.registerType(newElt, types.Typ[types.Int])
						v.registerType(arrType, sliceOfInt)
					}
				}
			}
		}
	}
}

func (v *ptrTransformVisitor) analyzeFuncDecl(fd *ast.FuncDecl) *ptrAnalysis {
	result := &ptrAnalysis{
		ptrParams:        make(map[string]types.Type),
		ptrVars:          make(map[string]types.Type),
		poolVars:         make(map[string]types.Type),
		ptrLocals:        make(map[string]*ptrLocalInfo),
		ptrIndexLocals:   make(map[string]*ptrIndexLocalInfo),
		ptrFieldLocals:   make(map[string]*ptrFieldLocalInfo),
		ptrCompositeLits: make(map[string]types.Type),
		ptrReturns:       make(map[int]types.Type),
		ptrParamPools:    make(map[string]types.Type),
		callReturnPools:  make(map[string]types.Type),
		sliceOfPtrVars:   make(map[string]types.Type),
		mapOfPtrVars:     make(map[string]types.Type),
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
			// []*T returns need pool types registered for return handling
			if arrType, ok := field.Type.(*ast.ArrayType); ok {
				if _, ok := arrType.Elt.(*ast.StarExpr); ok {
					if v.pkg != nil && v.pkg.TypesInfo != nil {
						if tv, ok := v.pkg.TypesInfo.Types[field.Type]; ok {
							if sliceType, ok := tv.Type.(*types.Slice); ok {
								if ptrType, ok := sliceType.Elem().(*types.Pointer); ok {
									elemType := ptrType.Elem()
									pn := poolNameForType(elemType)
									result.callReturnPools[pn] = elemType
									// Also add to ptrParamPools so expandPtrReturn appends pool to returns
									result.ptrParamPools[pn] = elemType
								}
							}
						}
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

	// Find []*T params — slice of pointers as function parameters
	if fd.Type.Params != nil {
		for _, field := range fd.Type.Params.List {
			if arrType, ok := field.Type.(*ast.ArrayType); ok {
				if _, ok := arrType.Elt.(*ast.StarExpr); ok {
					if v.pkg != nil && v.pkg.TypesInfo != nil {
						if tv, ok := v.pkg.TypesInfo.Types[field.Type]; ok {
							if sliceType, ok := tv.Type.(*types.Slice); ok {
								if ptrType, ok := sliceType.Elem().(*types.Pointer); ok {
									elemType := ptrType.Elem()
									pn := poolNameForType(elemType)
									result.ptrParamPools[pn] = elemType
								}
							}
						}
					}
				}
			}
		}
	}

	// Build ptrParamPools from ptrParams
	for _, elemType := range result.ptrParams {
		pn := poolNameForType(elemType)
		result.ptrParamPools[pn] = elemType
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

		// Find address-of composite literals: p := &Type{...}
		// These are direct pool allocations without an intermediate variable
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
				if _, ok := unary.X.(*ast.CompositeLit); !ok {
					continue
				}
				if i < len(assign.Lhs) {
					if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
						// Get the element type from the type info
						if v.pkg != nil && v.pkg.TypesInfo != nil {
							if tv, ok := v.pkg.TypesInfo.Types[unary]; ok {
								// tv.Type is *T, get T
								if ptr, ok := tv.Type.(*types.Pointer); ok {
									elemType := ptr.Elem()
									result.ptrCompositeLits[lhsIdent.Name] = elemType
									// Also register the pool for this type
									pn := poolNameForType(elemType)
									if _, exists := result.callReturnPools[pn]; !exists {
										result.callReturnPools[pn] = elemType
									}
								}
							}
						}
					}
				}
			}
			return true
		})

		// Scan for nested &CompositeLit inside composite literal fields
		// e.g., &TreeNode{left: &TreeNode{value: 2}} — register inner types for pool allocation
		v.findNestedAddrOfCompositeLits(fd.Body, result)

		// Find new(T) calls: p := new(T) — treat as &T{} for pool allocation
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok || assign.Tok != token.DEFINE {
				return true
			}
			for i, rhs := range assign.Rhs {
				callExpr, ok := rhs.(*ast.CallExpr)
				if !ok {
					continue
				}
				funIdent, ok := callExpr.Fun.(*ast.Ident)
				if !ok || funIdent.Name != "new" {
					continue
				}
				if i < len(assign.Lhs) {
					if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
						if v.pkg != nil && v.pkg.TypesInfo != nil {
							if tv, ok := v.pkg.TypesInfo.Types[callExpr]; ok {
								// tv.Type is *T, get T
								if ptr, ok := tv.Type.(*types.Pointer); ok {
									elemType := ptr.Elem()
									result.ptrCompositeLits[lhsIdent.Name] = elemType
									pn := poolNameForType(elemType)
									if _, exists := result.callReturnPools[pn]; !exists {
										result.callReturnPools[pn] = elemType
									}
								}
							}
						}
					}
				}
			}
			return true
		})

		// Find []*T variables: items := []*T{...} or var items []*T
		if v.pkg != nil && v.pkg.TypesInfo != nil {
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				// Handle short variable declarations: items := []*T{...}
				if assign, ok := n.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
					for i, lhs := range assign.Lhs {
						if lhsIdent, ok := lhs.(*ast.Ident); ok {
							if i < len(assign.Rhs) {
								if tv, ok := v.pkg.TypesInfo.Types[assign.Rhs[i]]; ok {
									if sliceType, ok := tv.Type.(*types.Slice); ok {
										if ptrType, ok := sliceType.Elem().(*types.Pointer); ok {
											elemType := ptrType.Elem()
											result.sliceOfPtrVars[lhsIdent.Name] = elemType
											pn := poolNameForType(elemType)
											if _, exists := result.callReturnPools[pn]; !exists {
												result.callReturnPools[pn] = elemType
											}
										}
									}
								}
							}
						}
					}
				}
				// Handle var declarations: var items []*T
				if declStmt, ok := n.(*ast.DeclStmt); ok {
					if genDecl, ok := declStmt.Decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
						for _, spec := range genDecl.Specs {
							if valueSpec, ok := spec.(*ast.ValueSpec); ok {
								if arrType, ok := valueSpec.Type.(*ast.ArrayType); ok {
									if _, ok := arrType.Elt.(*ast.StarExpr); ok {
										for _, name := range valueSpec.Names {
											if obj := v.pkg.TypesInfo.Defs[name]; obj != nil {
												if sliceType, ok := obj.Type().(*types.Slice); ok {
													if ptrType, ok := sliceType.Elem().(*types.Pointer); ok {
														elemType := ptrType.Elem()
														result.sliceOfPtrVars[name.Name] = elemType
														pn := poolNameForType(elemType)
														if _, exists := result.callReturnPools[pn]; !exists {
															result.callReturnPools[pn] = elemType
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
				}
				return true
			})
		}

		// Find map[K]*T variables: m := make(map[K]*T) or var m map[K]*T
		ast.Inspect(fd.Body, func(n ast.Node) bool {
			// Handle short variable declarations: m := make(map[K]*T)
			if assign, ok := n.(*ast.AssignStmt); ok && assign.Tok == token.DEFINE {
				for i, lhs := range assign.Lhs {
					if lhsIdent, ok := lhs.(*ast.Ident); ok {
						if i < len(assign.Rhs) {
							if tv, ok := v.pkg.TypesInfo.Types[assign.Rhs[i]]; ok {
								if mapType, ok := tv.Type.(*types.Map); ok {
									if ptrType, ok := mapType.Elem().(*types.Pointer); ok {
										elemType := ptrType.Elem()
										result.mapOfPtrVars[lhsIdent.Name] = elemType
										pn := poolNameForType(elemType)
										if _, exists := result.callReturnPools[pn]; !exists {
											result.callReturnPools[pn] = elemType
										}
									}
								}
							}
						}
					}
				}
			}
			// Handle var declarations: var m map[K]*T
			if declStmt, ok := n.(*ast.DeclStmt); ok {
				if genDecl, ok := declStmt.Decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
					for _, spec := range genDecl.Specs {
						if valueSpec, ok := spec.(*ast.ValueSpec); ok {
							if mapType, ok := valueSpec.Type.(*ast.MapType); ok {
								if _, ok := mapType.Value.(*ast.StarExpr); ok {
									for _, name := range valueSpec.Names {
										if obj := v.pkg.TypesInfo.Defs[name]; obj != nil {
											if mt, ok := obj.Type().(*types.Map); ok {
												if ptrType, ok := mt.Elem().(*types.Pointer); ok {
													elemType := ptrType.Elem()
													result.mapOfPtrVars[name.Name] = elemType
													pn := poolNameForType(elemType)
													if _, exists := result.callReturnPools[pn]; !exists {
														result.callReturnPools[pn] = elemType
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

		// Find ptrFieldLocals: p := &s.field where s is a struct
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
				selExpr, ok := unary.X.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				if i < len(assign.Lhs) {
					if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
						if v.pkg != nil && v.pkg.TypesInfo != nil {
							if tv, ok := v.pkg.TypesInfo.Types[selExpr]; ok {
								result.ptrFieldLocals[lhsIdent.Name] = &ptrFieldLocalInfo{
									baseExpr:  selExpr.X,
									fieldName: selExpr.Sel.Name,
									elemType:  tv.Type,
								}
							}
						}
					}
				}
			}
			return true
		})

		// Find ptrVar assignments to field expressions: p = &s.field where p is a ptrVar
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
				selExpr, ok := unary.X.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				if i < len(assign.Lhs) {
					if lhsIdent, ok := assign.Lhs[i].(*ast.Ident); ok {
						if _, isPtrVar := result.ptrVars[lhsIdent.Name]; isPtrVar {
							if v.pkg != nil && v.pkg.TypesInfo != nil {
								if tv, ok := v.pkg.TypesInfo.Types[selExpr]; ok {
									result.ptrFieldLocals[lhsIdent.Name] = &ptrFieldLocalInfo{
										baseExpr:  selExpr.X,
										fieldName: selExpr.Sel.Name,
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

		// ptrFieldLocals and ptrIndexLocals now use pool-based indexing
		// (copy-in at creation, copy-out after mutation) so escape is not a problem.
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
				resultType := results.At(i).Type()
				if ptrType, isPtr := resultType.(*types.Pointer); isPtr {
					elemType := ptrType.Elem()
					pn := poolNameForType(elemType)
					result.callReturnPools[pn] = elemType
				}
				if sliceType, isSlice := resultType.(*types.Slice); isSlice {
					if ptrType, isPtr := sliceType.Elem().(*types.Pointer); isPtr {
						elemType := ptrType.Elem()
						pn := poolNameForType(elemType)
						result.callReturnPools[pn] = elemType
					}
				}
			}
			params := fnType.Params()
			for i := 0; i < params.Len(); i++ {
				paramType := params.At(i).Type()
				if ptrType, isPtr := paramType.(*types.Pointer); isPtr {
					elemType := ptrType.Elem()
					pn := poolNameForType(elemType)
					result.callReturnPools[pn] = elemType
				}
				if sliceType, isSlice := paramType.(*types.Slice); isSlice {
					if ptrType, isPtr := sliceType.Elem().(*types.Pointer); isPtr {
						elemType := ptrType.Elem()
						pn := poolNameForType(elemType)
						result.callReturnPools[pn] = elemType
					}
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
			// Rewrite []*T → []int for slice-of-pointer params
			if arrType, ok := field.Type.(*ast.ArrayType); ok {
				if _, ok := arrType.Elt.(*ast.StarExpr); ok {
					var elemType types.Type
					if v.pkg != nil && v.pkg.TypesInfo != nil {
						if tv, ok := v.pkg.TypesInfo.Types[field.Type]; ok {
							if sliceType, ok := tv.Type.(*types.Slice); ok {
								if ptrType, ok := sliceType.Elem().(*types.Pointer); ok {
									elemType = ptrType.Elem()
								}
							}
						}
					}
					// []*T → []int
					newElt := &ast.Ident{Name: "int"}
					arrType.Elt = newElt
					sliceOfInt := types.NewSlice(types.Typ[types.Int])
					v.registerType(newElt, types.Typ[types.Int])
					v.registerType(arrType, sliceOfInt)
					if elemType != nil {
						pn := poolNameForType(elemType)
						poolParamTypes[pn] = elemType
					}
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

	// Rewrite []*T return types → []int and add pool params/returns
	if fd.Type.Results != nil {
		for _, field := range fd.Type.Results.List {
			if arrType, ok := field.Type.(*ast.ArrayType); ok {
				if _, ok := arrType.Elt.(*ast.StarExpr); ok {
					var elemType types.Type
					if v.pkg != nil && v.pkg.TypesInfo != nil {
						if tv, ok := v.pkg.TypesInfo.Types[field.Type]; ok {
							if sliceType, ok := tv.Type.(*types.Slice); ok {
								if ptrType, ok := sliceType.Elem().(*types.Pointer); ok {
									elemType = ptrType.Elem()
								}
							}
						}
					}
					newElt := &ast.Ident{Name: "int"}
					arrType.Elt = newElt
					sliceOfInt := types.NewSlice(types.Typ[types.Int])
					v.registerType(newElt, types.Typ[types.Int])
					v.registerType(arrType, sliceOfInt)
					if elemType != nil {
						pn := poolNameForType(elemType)
						if _, fromParam := poolParamTypes[pn]; !fromParam {
							// Add pool param for []*T return type
							typeExpr := v.typeToExpr(elemType)
							pArrType := &ast.ArrayType{Elt: typeExpr}
							pSliceType := types.NewSlice(elemType)
							v.registerType(pArrType, pSliceType)
							if fd.Type.Params == nil {
								fd.Type.Params = &ast.FieldList{}
							}
							fd.Type.Params.List = append(fd.Type.Params.List, &ast.Field{
								Names: []*ast.Ident{{Name: pn}},
								Type:  pArrType,
							})
							poolParamTypes[pn] = elemType
						}
					}
				}
			}
		}
	}

	// Rewrite return types: *T → int, and add pool returns
	returnPoolTypes := make(map[string]types.Type)
	if fd.Type.Results != nil && len(analysis.ptrReturns) > 0 {
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

	// Add pool returns for *T params (dedup with return pool types)
	for poolName, elemType := range poolParamTypes {
		if _, alreadyReturned := returnPoolTypes[poolName]; alreadyReturned {
			continue
		}
		if fd.Type.Results == nil {
			fd.Type.Results = &ast.FieldList{}
		}
		typeExpr := v.typeToExpr(elemType)
		arrType := &ast.ArrayType{Elt: typeExpr}
		v.registerType(arrType, types.NewSlice(elemType))
		fd.Type.Results.List = append(fd.Type.Results.List, &ast.Field{Type: arrType})
	}

	// Rewrite body
	if fd.Body != nil {
		v.rewriteBlockStmt(fd.Body, analysis)
	}

	// Add implicit return at end of function body for param pool returns
	if len(poolParamTypes) > 0 && fd.Body != nil && len(fd.Body.List) > 0 {
		lastStmt := fd.Body.List[len(fd.Body.List)-1]
		if _, isReturn := lastStmt.(*ast.ReturnStmt); !isReturn {
			retStmt := &ast.ReturnStmt{}
			for pn, elemType := range analysis.ptrParamPools {
				alreadyFromReturn := false
				for _, retType := range analysis.ptrReturns {
					if poolNameForType(retType) == pn {
						alreadyFromReturn = true
						break
					}
				}
				if !alreadyFromReturn {
					poolIdent := &ast.Ident{Name: pn}
					v.registerType(poolIdent, types.NewSlice(elemType))
					retStmt.Results = append(retStmt.Results, poolIdent)
				}
			}
			fd.Body.List = append(fd.Body.List, retStmt)
		}
	}

	// Pool declarations + parameter boxing: combined so pool decls come first
	// Prepend order: [pool decls] [boxing stmts] [original body]
	if fd.Body != nil {
		var preamble []ast.Stmt

		// 1. Pool declarations for types not provided via params
		// Includes poolVars, ptrFieldLocals, and ptrIndexLocals element types
		{
			poolTypesNeeded := make(map[string]types.Type)
			for _, elemType := range analysis.poolVars {
				pn := poolNameForType(elemType)
				if _, fromParam := poolParamTypes[pn]; !fromParam {
					poolTypesNeeded[pn] = elemType
				}
			}
			for _, info := range analysis.ptrFieldLocals {
				pn := poolNameForType(info.elemType)
				if _, fromParam := poolParamTypes[pn]; !fromParam {
					poolTypesNeeded[pn] = info.elemType
				}
			}
			for _, info := range analysis.ptrIndexLocals {
				pn := poolNameForType(info.elemType)
				if _, fromParam := poolParamTypes[pn]; !fromParam {
					poolTypesNeeded[pn] = info.elemType
				}
			}
			for _, elemType := range analysis.ptrCompositeLits {
				pn := poolNameForType(elemType)
				if _, fromParam := poolParamTypes[pn]; !fromParam {
					poolTypesNeeded[pn] = elemType
				}
			}
			for _, elemType := range analysis.sliceOfPtrVars {
				pn := poolNameForType(elemType)
				if _, fromParam := poolParamTypes[pn]; !fromParam {
					poolTypesNeeded[pn] = elemType
				}
			}
			for _, elemType := range analysis.mapOfPtrVars {
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
			for _, info := range analysis.ptrFieldLocals {
				pn := poolNameForType(info.elemType)
				alreadyDeclared[pn] = true
			}
			for _, info := range analysis.ptrIndexLocals {
				pn := poolNameForType(info.elemType)
				alreadyDeclared[pn] = true
			}
			for _, elemType := range analysis.ptrCompositeLits {
				pn := poolNameForType(elemType)
				alreadyDeclared[pn] = true
			}
			for _, elemType := range analysis.sliceOfPtrVars {
				pn := poolNameForType(elemType)
				alreadyDeclared[pn] = true
			}
			for _, elemType := range analysis.mapOfPtrVars {
				pn := poolNameForType(elemType)
				alreadyDeclared[pn] = true
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
		// Detect copy-out needs BEFORE rewriting (uses original AST expressions)
		copyOutPtrs := v.detectCopyOutNeeds(stmt, analysis)
		expanded := v.rewriteStmtExpand(stmt, analysis)
		newList = append(newList, expanded...)
		// Insert copy-out statements after the rewritten statements
		for ptrName := range copyOutPtrs {
			if coStmt := v.generateCopyOut(ptrName, analysis); coStmt != nil {
				newList = append(newList, coStmt)
			}
		}
	}
	block.List = newList
}

// detectCopyOutNeeds checks a statement (before rewriting) for mutations through
// ptrFieldLocals or ptrIndexLocals, returning the set of pointer names needing copy-out.
func (v *ptrTransformVisitor) detectCopyOutNeeds(stmt ast.Stmt, analysis *ptrAnalysis) map[string]bool {
	needed := make(map[string]bool)
	if len(analysis.ptrFieldLocals) == 0 && len(analysis.ptrIndexLocals) == 0 {
		return needed
	}
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		// Check LHS for *p = val or *p += val
		for _, lhs := range s.Lhs {
			if star, ok := lhs.(*ast.StarExpr); ok {
				if ident, ok := star.X.(*ast.Ident); ok {
					if _, isField := analysis.ptrFieldLocals[ident.Name]; isField {
						needed[ident.Name] = true
					}
					if _, isIndex := analysis.ptrIndexLocals[ident.Name]; isIndex {
						needed[ident.Name] = true
					}
				}
			}
			// Check for p.field = val (SelectorExpr on ptrIndexLocal with struct elem)
			if sel, ok := lhs.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok {
					if _, isIndex := analysis.ptrIndexLocals[ident.Name]; isIndex {
						needed[ident.Name] = true
					}
				}
			}
		}
		// Check RHS for function calls with ptr field/index args
		for _, rhs := range s.Rhs {
			v.collectCopyOutFromExpr(rhs, analysis, needed)
		}
	case *ast.ExprStmt:
		// Standalone function call: modifyInt(p)
		v.collectCopyOutFromExpr(s.X, analysis, needed)
	case *ast.IncDecStmt:
		// (*p)++ or similar
		if star, ok := s.X.(*ast.StarExpr); ok {
			if ident, ok := star.X.(*ast.Ident); ok {
				if _, isField := analysis.ptrFieldLocals[ident.Name]; isField {
					needed[ident.Name] = true
				}
				if _, isIndex := analysis.ptrIndexLocals[ident.Name]; isIndex {
					needed[ident.Name] = true
				}
			}
		}
	}
	return needed
}

// collectCopyOutFromExpr scans an expression for function calls that pass
// ptrFieldLocal or ptrIndexLocal pointers as arguments.
func (v *ptrTransformVisitor) collectCopyOutFromExpr(expr ast.Expr, analysis *ptrAnalysis, needed map[string]bool) {
	ast.Inspect(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, arg := range call.Args {
			if ident, ok := arg.(*ast.Ident); ok {
				if _, isField := analysis.ptrFieldLocals[ident.Name]; isField {
					needed[ident.Name] = true
				}
				if _, isIndex := analysis.ptrIndexLocals[ident.Name]; isIndex {
					needed[ident.Name] = true
				}
			}
		}
		return true
	})
}

// generateCopyOut creates a copy-out statement: target = _pool_T[p]
// For ptrFieldLocals: s.field = _pool_T[p]
// For ptrIndexLocals: arr[i] = _pool_T[p]
func (v *ptrTransformVisitor) generateCopyOut(ptrName string, analysis *ptrAnalysis) ast.Stmt {
	if info, isField := analysis.ptrFieldLocals[ptrName]; isField {
		lhs := &ast.SelectorExpr{X: info.baseExpr, Sel: &ast.Ident{Name: info.fieldName}}
		v.registerType(lhs, info.elemType)
		ptrIdent := &ast.Ident{Name: ptrName}
		v.registerType(ptrIdent, types.Typ[types.Int])
		rhs := v.poolIndexExpr(ptrIdent, info.elemType)
		return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{rhs}}
	}
	if info, isIndex := analysis.ptrIndexLocals[ptrName]; isIndex {
		lhs := &ast.IndexExpr{X: info.sliceExpr, Index: info.indexExpr}
		v.registerType(lhs, info.elemType)
		ptrIdent := &ast.Ident{Name: ptrName}
		v.registerType(ptrIdent, types.Typ[types.Int])
		rhs := v.poolIndexExpr(ptrIdent, info.elemType)
		return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{rhs}}
	}
	return nil
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
				// Handle p := &s.field → pool-based copy-in
				if info, isFieldLocal := analysis.ptrFieldLocals[lhsIdent.Name]; isFieldLocal {
					if unary, ok := assignStmt.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
						return v.expandPoolCopyIn(lhsIdent.Name, unary.X, info.elemType, token.DEFINE, analysis)
					}
				}
				// Handle p := &arr[i] → pool-based copy-in
				if info, isIndexLocal := analysis.ptrIndexLocals[lhsIdent.Name]; isIndexLocal {
					if unary, ok := assignStmt.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
						return v.expandPoolCopyIn(lhsIdent.Name, unary.X, info.elemType, token.DEFINE, analysis)
					}
				}
				// Handle p := new(T) → direct pool allocation with zero-value struct
				if elemType, isCompLit := analysis.ptrCompositeLits[lhsIdent.Name]; isCompLit {
					if callExpr, ok := assignStmt.Rhs[0].(*ast.CallExpr); ok {
						if funIdent, ok := callExpr.Fun.(*ast.Ident); ok && funIdent.Name == "new" {
							zeroVal := v.zeroValueExpr(elemType)
							return v.expandPoolVarAssign(lhsIdent.Name, zeroVal, elemType, analysis)
						}
					}
				}
				// Handle p := &Type{...} → direct pool allocation
				if elemType, isCompLit := analysis.ptrCompositeLits[lhsIdent.Name]; isCompLit {
					if unary, ok := assignStmt.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
						return v.expandPoolVarAssign(lhsIdent.Name, unary.X, elemType, analysis)
					}
				}
			}
		}
	}
	// Handle p = &s.field or p = &arr[i] where p is a ptrVar (ASSIGN form)
	if assignStmt, ok := stmt.(*ast.AssignStmt); ok && assignStmt.Tok == token.ASSIGN {
		if len(assignStmt.Lhs) == 1 && len(assignStmt.Rhs) == 1 {
			if lhsIdent, ok := assignStmt.Lhs[0].(*ast.Ident); ok {
				if _, isPtrVar := analysis.ptrVars[lhsIdent.Name]; isPtrVar {
					if info, isFieldLocal := analysis.ptrFieldLocals[lhsIdent.Name]; isFieldLocal {
						if unary, ok := assignStmt.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
							return v.expandPoolCopyIn(lhsIdent.Name, unary.X, info.elemType, token.ASSIGN, analysis)
						}
					}
					if info, isIndexLocal := analysis.ptrIndexLocals[lhsIdent.Name]; isIndexLocal {
						if unary, ok := assignStmt.Rhs[0].(*ast.UnaryExpr); ok && unary.Op == token.AND {
							return v.expandPoolCopyIn(lhsIdent.Name, unary.X, info.elemType, token.ASSIGN, analysis)
						}
					}
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

	// Handle append to []*T slices: items = append(items, &CompositeLit{...})
	// Hoist &CompositeLit args to pool allocations before the append
	if assignStmt, ok := stmt.(*ast.AssignStmt); ok && (assignStmt.Tok == token.ASSIGN || assignStmt.Tok == token.DEFINE) {
		if len(assignStmt.Rhs) == 1 {
			if callExpr, ok := assignStmt.Rhs[0].(*ast.CallExpr); ok {
				if funIdent, ok := callExpr.Fun.(*ast.Ident); ok && funIdent.Name == "append" {
					if len(callExpr.Args) >= 2 {
						// Check if target is a sliceOfPtrVars
						isSliceOfPtr := false
						if len(assignStmt.Lhs) == 1 {
							if lhsIdent, ok := assignStmt.Lhs[0].(*ast.Ident); ok {
								if _, ok := analysis.sliceOfPtrVars[lhsIdent.Name]; ok {
									isSliceOfPtr = true
								}
							}
						}
						if isSliceOfPtr {
							var preStmts []ast.Stmt
							for i := 1; i < len(callExpr.Args); i++ {
								callExpr.Args[i] = v.hoistAddrOfCompositeLit(callExpr.Args[i], analysis, &preStmts)
							}
							// Rewrite the statement normally
							rewritten := v.rewriteStmt(stmt, analysis)
							return append(preStmts, rewritten)
						}
					}
				}
			}
		}
	}

	// Handle map[K]*T assignment: m[key] = &Struct{...}
	// Hoist &CompositeLit to pool allocation before the assignment
	if assignStmt, ok := stmt.(*ast.AssignStmt); ok && assignStmt.Tok == token.ASSIGN {
		if len(assignStmt.Lhs) == 1 && len(assignStmt.Rhs) == 1 {
			if indexExpr, ok := assignStmt.Lhs[0].(*ast.IndexExpr); ok {
				if ident, ok := indexExpr.X.(*ast.Ident); ok {
					if _, ok := analysis.mapOfPtrVars[ident.Name]; ok {
						var preStmts []ast.Stmt
						assignStmt.Rhs[0] = v.hoistAddrOfCompositeLit(assignStmt.Rhs[0], analysis, &preStmts)
						rewritten := v.rewriteStmt(stmt, analysis)
						return append(preStmts, rewritten)
					}
				}
			}
		}
	}

	// Handle &CompositeLit{} as function arguments in any call expression
	// Hoist &CompositeLit args to pool allocations before the call
	// The hoisted pre-statements are collected and the rest of the function continues
	var addrOfCompLitPreStmts []ast.Stmt
	v.hoistAddrOfCompositeLitInCallArgs(stmt, analysis, &addrOfCompLitPreStmts)

	// Handle return statements in functions with pointer returns or param pools
	if retStmt, ok := stmt.(*ast.ReturnStmt); ok && (len(analysis.ptrReturns) > 0 || len(analysis.ptrParamPools) > 0) {
		result := v.expandPtrReturn(retStmt, analysis)
		return append(addrOfCompLitPreStmts, result...)
	}

	// Handle standalone call expressions that return pools (e.g., increment(&x))
	if exprStmt, ok := stmt.(*ast.ExprStmt); ok {
		if callExpr, ok := exprStmt.X.(*ast.CallExpr); ok {
			pools := v.getCallReturnPools(callExpr)
			if len(pools) > 0 {
				rewrittenCall := v.rewriteExpr(callExpr, analysis)
				numOrigReturns := v.getOriginalReturnCount(callExpr)
				var lhs []ast.Expr
				for i := 0; i < numOrigReturns; i++ {
					lhs = append(lhs, &ast.Ident{Name: "_"})
				}
				for poolName, elemType := range pools {
					poolIdent := &ast.Ident{Name: poolName}
					v.registerType(poolIdent, types.NewSlice(elemType))
					lhs = append(lhs, poolIdent)
				}
				result := []ast.Stmt{&ast.AssignStmt{Lhs: lhs, Tok: token.ASSIGN, Rhs: []ast.Expr{rewrittenCall}}}
				return append(addrOfCompLitPreStmts, result...)
			}
		}
	}

	// Handle call sites for pointer-returning functions
	if assignStmt, ok := stmt.(*ast.AssignStmt); ok {
		if len(assignStmt.Rhs) == 1 {
			if callExpr, ok := assignStmt.Rhs[0].(*ast.CallExpr); ok {
				if pools := v.getCallReturnPools(callExpr); len(pools) > 0 {
					result := v.expandPtrReturnCallAssign(assignStmt, pools, analysis)
					return append(addrOfCompLitPreStmts, result...)
				}
			}
		}
	}

	// Handle if statements with pool-returning calls in condition
	if ifStmt, ok := stmt.(*ast.IfStmt); ok {
		if ifStmt.Init != nil {
			ifStmt.Init = v.rewriteStmt(ifStmt.Init, analysis)
		}
		ifStmt.Cond = v.rewriteExpr(ifStmt.Cond, analysis)
		var hoistStmts []ast.Stmt
		ifStmt.Cond = v.hoistPoolCalls(ifStmt.Cond, analysis, &hoistStmts)
		v.rewriteBlockStmt(ifStmt.Body, analysis)
		if ifStmt.Else != nil {
			ifStmt.Else = v.rewriteStmt(ifStmt.Else, analysis)
		}
		allHoist := append(addrOfCompLitPreStmts, hoistStmts...)
		if len(allHoist) > 0 {
			return append(allHoist, ifStmt)
		}
		return []ast.Stmt{ifStmt}
	}

	// Handle for statements with pool-returning calls in condition
	if forStmt, ok := stmt.(*ast.ForStmt); ok {
		if forStmt.Init != nil {
			forStmt.Init = v.rewriteStmt(forStmt.Init, analysis)
		}
		if forStmt.Cond != nil {
			forStmt.Cond = v.rewriteExpr(forStmt.Cond, analysis)
			var hoistStmts []ast.Stmt
			forStmt.Cond = v.hoistPoolCalls(forStmt.Cond, analysis, &hoistStmts)
			if len(hoistStmts) > 0 {
				// For loop conditions with hoisted calls: move hoisted stmts before the loop
				if forStmt.Post != nil {
					forStmt.Post = v.rewriteStmt(forStmt.Post, analysis)
				}
				v.rewriteBlockStmt(forStmt.Body, analysis)
				return append(hoistStmts, forStmt)
			}
		}
		if forStmt.Post != nil {
			forStmt.Post = v.rewriteStmt(forStmt.Post, analysis)
		}
		v.rewriteBlockStmt(forStmt.Body, analysis)
		return []ast.Stmt{forStmt}
	}

	// For assignment statements, rewrite first, then:
	// 1. Hoist pool-returning calls from RHS binary expressions to temp variables
	//    (C++ can't use tuple results directly in arithmetic: int + tuple<int,vec> fails)
	// 2. Hoist nested pool indices from LHS to temp variables
	//    (Rust borrow checker: nested pool indexing = simultaneous mut+immut borrow)
	if assignStmt, ok := stmt.(*ast.AssignStmt); ok && assignStmt.Tok == token.ASSIGN {
		rewritten := v.rewriteAssignStmt(assignStmt, analysis)
		if rewrittenAssign, isAssign := rewritten.(*ast.AssignStmt); isAssign {
			var preStmts []ast.Stmt
			// Hoist pool-returning calls from RHS expressions
			for i, rhs := range rewrittenAssign.Rhs {
				rewrittenAssign.Rhs[i] = v.hoistPoolCalls(rhs, analysis, &preStmts)
			}
			// Hoist nested pool indices from LHS
			nestedPreStmts := v.hoistNestedPoolIndicesInAssign(rewrittenAssign)
			preStmts = append(preStmts, nestedPreStmts...)
			allPreStmts := append(addrOfCompLitPreStmts, preStmts...)
			if len(allPreStmts) > 0 {
				return append(allPreStmts, rewrittenAssign)
			}
		}
		result := []ast.Stmt{rewritten}
		return append(addrOfCompLitPreStmts, result...)
	}

	result := []ast.Stmt{v.rewriteStmt(stmt, analysis)}
	return append(addrOfCompLitPreStmts, result...)
}

// expandPoolVarAssign expands: varName := CompositeLit{...}
// into:
//   _pool_T = append(_pool_T, rewrittenRHS)
//   varName := len(_pool_T) - 1
func (v *ptrTransformVisitor) expandPoolVarAssign(varName string, rhs ast.Expr, elemType types.Type, analysis *ptrAnalysis) []ast.Stmt {
	poolName := poolNameForType(elemType)

	// Hoist nested &CompositeLit{...} in composite literal fields before rewriting
	var nestedPreStmts []ast.Stmt
	rhs = v.hoistAddrOfCompositeLit(rhs, analysis, &nestedPreStmts)

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

	result := append(nestedPreStmts, appendStmt, indexStmt)
	return result
}

// expandPoolCopyIn expands p := &s.field or p := &arr[i] into pool-based copy-in:
//   _pool_T = append(_pool_T, valueExpr)
//   varName :=/= int(len(_pool_T) - 1)
func (v *ptrTransformVisitor) expandPoolCopyIn(varName string, valueExpr ast.Expr, elemType types.Type, tok token.Token, analysis *ptrAnalysis) []ast.Stmt {
	poolName := poolNameForType(elemType)

	// Rewrite the value expression (handles nested pool vars, etc.)
	rewrittenRHS := v.rewriteExpr(valueExpr, analysis)

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

	// Statement 2: varName :=/= int(len(_pool_T) - 1)
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
	intIdent := &ast.Ident{Name: "int"}
	intCast := &ast.CallExpr{
		Fun:  intIdent,
		Args: []ast.Expr{lenMinus1},
	}
	v.registerType(intCast, types.Typ[types.Int])
	if v.pkg != nil && v.pkg.TypesInfo != nil {
		intObj := types.Universe.Lookup("int")
		if intObj != nil {
			v.pkg.TypesInfo.Uses[intIdent] = intObj
		}
	}
	indexStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.Ident{Name: varName}},
		Tok: tok,
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
		resultType := results.At(i).Type()
		if ptrType, isPtr := resultType.(*types.Pointer); isPtr {
			elemType := ptrType.Elem()
			pn := poolNameForType(elemType)
			pools[pn] = elemType
		}
		if sliceType, isSlice := resultType.(*types.Slice); isSlice {
			if ptrType, isPtr := sliceType.Elem().(*types.Pointer); isPtr {
				elemType := ptrType.Elem()
				pn := poolNameForType(elemType)
				pools[pn] = elemType
			}
		}
	}
	params := fnType.Params()
	for i := 0; i < params.Len(); i++ {
		paramType := params.At(i).Type()
		if ptrType, isPtr := paramType.(*types.Pointer); isPtr {
			elemType := ptrType.Elem()
			pn := poolNameForType(elemType)
			pools[pn] = elemType
		}
		if sliceType, isSlice := paramType.(*types.Slice); isSlice {
			if ptrType, isPtr := sliceType.Elem().(*types.Pointer); isPtr {
				elemType := ptrType.Elem()
				pn := poolNameForType(elemType)
				pools[pn] = elemType
			}
		}
	}
	return pools
}

// getOriginalReturnCount returns the number of return values in the original function signature
func (v *ptrTransformVisitor) getOriginalReturnCount(call *ast.CallExpr) int {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return 0
	}
	funIdent, ok := call.Fun.(*ast.Ident)
	if !ok {
		return 0
	}
	obj := v.pkg.TypesInfo.Uses[funIdent]
	if obj == nil {
		return 0
	}
	fnType, ok := obj.Type().(*types.Signature)
	if !ok {
		return 0
	}
	return fnType.Results().Len()
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
	// Hoist nested &CompositeLit before rewriting
	var hoistStmts []ast.Stmt
	for i, r := range ret.Results {
		if unary, ok := r.(*ast.UnaryExpr); ok && unary.Op == token.AND {
			if _, ok := unary.X.(*ast.CompositeLit); ok {
				var nestedPreStmts []ast.Stmt
				unary.X = v.hoistAddrOfCompositeLit(unary.X, analysis, &nestedPreStmts)
				hoistStmts = append(hoistStmts, nestedPreStmts...)
				ret.Results[i] = unary
			}
		}
	}

	// Rewrite all result expressions
	for i, r := range ret.Results {
		ret.Results[i] = v.rewriteExpr(r, analysis)
	}
	for i, r := range ret.Results {
		ret.Results[i] = v.hoistPoolCalls(r, analysis, &hoistStmts)
	}

	// Check if this is a single-result return with a CallExpr to a ptr-returning function
	// In Go, `return f()` where f returns (int, []T) is valid and covers all return values
	if len(ret.Results) == 1 {
		if callExpr, ok := ret.Results[0].(*ast.CallExpr); ok {
			pools := v.getCallReturnPools(callExpr)
			if len(pools) > 0 {
				// Check if call's pools cover all param pools too
				allCovered := true
				for pn := range analysis.ptrParamPools {
					if _, covered := pools[pn]; !covered {
						allCovered = false
						break
					}
				}
				if allCovered {
					// The call already returns the pool as part of its multi-value return
					return append(hoistStmts, ret)
				}
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

	// Handle &X at pointer return positions → expand to pool append
	// Covers &CompositeLit{...}, &s.field, &arr[i], and &localVar
	for i, r := range ret.Results {
		elemType, hasPtrReturn := analysis.ptrReturns[i]
		if !hasPtrReturn {
			continue
		}
		unary, ok := r.(*ast.UnaryExpr)
		if !ok || unary.Op != token.AND {
			continue
		}
		// Expand: return &X → _pool_T = append(_pool_T, X); return int(len(_pool_T)-1), _pool_T
		poolName := poolNameForType(elemType)
		sliceType := types.NewSlice(elemType)

		poolIdent1 := &ast.Ident{Name: poolName}
		poolIdent2 := &ast.Ident{Name: poolName}
		appendCall := &ast.CallExpr{
			Fun:  &ast.Ident{Name: "append"},
			Args: []ast.Expr{poolIdent1, unary.X}, // unary.X already rewritten by rewriteExpr above
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
		// Also append param pool idents (dedup with ptrReturns)
		for pn, elemType := range analysis.ptrParamPools {
			alreadyFromReturn := false
			for _, retType := range analysis.ptrReturns {
				if poolNameForType(retType) == pn {
					alreadyFromReturn = true
					break
				}
			}
			if !alreadyFromReturn {
				poolIdent := &ast.Ident{Name: pn}
				v.registerType(poolIdent, types.NewSlice(elemType))
				ret.Results = append(ret.Results, poolIdent)
			}
		}
		return append(hoistStmts, appendStmt, ret)
	}

	// For normal returns (with explicit values), append pool idents
	for _, elemType := range analysis.ptrReturns {
		pn := poolNameForType(elemType)
		poolIdent := &ast.Ident{Name: pn}
		v.registerType(poolIdent, types.NewSlice(elemType))
		ret.Results = append(ret.Results, poolIdent)
	}
	// Also append param pool idents (dedup with ptrReturns)
	for pn, elemType := range analysis.ptrParamPools {
		alreadyFromReturn := false
		for _, retType := range analysis.ptrReturns {
			if poolNameForType(retType) == pn {
				alreadyFromReturn = true
				break
			}
		}
		if !alreadyFromReturn {
			poolIdent := &ast.Ident{Name: pn}
			v.registerType(poolIdent, types.NewSlice(elemType))
			ret.Results = append(ret.Results, poolIdent)
		}
	}
	return append(hoistStmts, ret)
}

// hoistPoolCalls walks an expression tree and replaces calls that need pool capture
// with temp variables, generating pre-statements for the hoisted calls.
// This handles expression-position calls like: root.value + treeSum(root.left) + treeSum(root.right)
func (v *ptrTransformVisitor) hoistPoolCalls(expr ast.Expr, analysis *ptrAnalysis, preStmts *[]ast.Stmt) ast.Expr {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.CallExpr:
		pools := v.getCallReturnPools(e)
		if len(pools) > 0 {
			tmpName := fmt.Sprintf("__call_ret_%d", v.tmpCounter)
			v.tmpCounter++

			// Determine the correct type for the temp var
			var tmpType types.Type = types.Typ[types.Int]
			var tmpTypeExpr ast.Expr = &ast.Ident{Name: "int"}
			if funIdent, ok := e.Fun.(*ast.Ident); ok {
				if v.pkg != nil && v.pkg.TypesInfo != nil {
					if obj := v.pkg.TypesInfo.Uses[funIdent]; obj != nil {
						if fnType, ok := obj.Type().(*types.Signature); ok {
							if fnType.Results().Len() > 0 {
								firstResult := fnType.Results().At(0).Type()
								if sliceType, isSlice := firstResult.(*types.Slice); isSlice {
									if _, isPtr := sliceType.Elem().(*types.Pointer); isPtr {
										// []*T return → []int
										intIdent := &ast.Ident{Name: "int"}
										arrType := &ast.ArrayType{Elt: intIdent}
										sliceOfInt := types.NewSlice(types.Typ[types.Int])
										v.registerType(intIdent, types.Typ[types.Int])
										v.registerType(arrType, sliceOfInt)
										tmpTypeExpr = arrType
										tmpType = sliceOfInt
									} else {
										// []T return (non-pointer slice) → keep original []T type
										elemExpr := v.typeToExpr(sliceType.Elem())
										arrType := &ast.ArrayType{Elt: elemExpr}
										v.registerType(elemExpr, sliceType.Elem())
										v.registerType(arrType, firstResult)
										tmpTypeExpr = arrType
										tmpType = firstResult
									}
								} else if _, isPtr := firstResult.(*types.Pointer); !isPtr {
									// Non-pointer, non-slice return (bool, string, etc.) → keep original type
									tmpTypeExpr = v.typeToExpr(firstResult)
									tmpType = firstResult
									v.registerType(tmpTypeExpr, firstResult)
								}
								// else: *T return → int (the default)
							}
						}
					}
				}
			}

			// Declare temp var
			decl := &ast.DeclStmt{Decl: &ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{&ast.ValueSpec{
					Names: []*ast.Ident{{Name: tmpName}},
					Type:  tmpTypeExpr,
				}},
			}}
			*preStmts = append(*preStmts, decl)

			// Build assignment: __call_ret_N, _pool_T = f(args, _pool_T)
			numOrigReturns := v.getOriginalReturnCount(e)
			var lhs []ast.Expr
			tmpIdent := &ast.Ident{Name: tmpName}
			v.registerType(tmpIdent, tmpType)
			lhs = append(lhs, tmpIdent)
			for i := 1; i < numOrigReturns; i++ {
				lhs = append(lhs, &ast.Ident{Name: "_"})
			}
			for poolName, elemType := range pools {
				poolIdent := &ast.Ident{Name: poolName}
				v.registerType(poolIdent, types.NewSlice(elemType))
				lhs = append(lhs, poolIdent)
			}
			*preStmts = append(*preStmts, &ast.AssignStmt{
				Lhs: lhs,
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{e},
			})

			retIdent := &ast.Ident{Name: tmpName}
			v.registerType(retIdent, tmpType)
			return retIdent
		}
		return e
	case *ast.BinaryExpr:
		e.X = v.hoistPoolCalls(e.X, analysis, preStmts)
		e.Y = v.hoistPoolCalls(e.Y, analysis, preStmts)
		return e
	case *ast.ParenExpr:
		e.X = v.hoistPoolCalls(e.X, analysis, preStmts)
		return e
	case *ast.UnaryExpr:
		e.X = v.hoistPoolCalls(e.X, analysis, preStmts)
		return e
	}
	return expr
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

	if hasPoolAccessLHS || assign.Tok == token.DEFINE {
		var stmts []ast.Stmt

		// Determine the correct type for the var declaration
		// For *T returns → int, for []*T returns → []int, otherwise use original type
		var tmpTypeExpr ast.Expr
		tmpTypeExpr = &ast.Ident{Name: "int"}
		v.registerType(tmpTypeExpr, types.Typ[types.Int])
		if callExpr, ok := assign.Rhs[0].(*ast.CallExpr); ok {
			if funIdent, ok := callExpr.Fun.(*ast.Ident); ok {
				if v.pkg != nil && v.pkg.TypesInfo != nil {
					if obj := v.pkg.TypesInfo.Uses[funIdent]; obj != nil {
						if fnType, ok := obj.Type().(*types.Signature); ok {
							if fnType.Results().Len() > 0 {
								firstResult := fnType.Results().At(0).Type()
								if sliceType, isSlice := firstResult.(*types.Slice); isSlice {
									if _, isPtr := sliceType.Elem().(*types.Pointer); isPtr {
										// []*T return → []int
										intIdent := &ast.Ident{Name: "int"}
										arrType := &ast.ArrayType{Elt: intIdent}
										sliceOfInt := types.NewSlice(types.Typ[types.Int])
										v.registerType(intIdent, types.Typ[types.Int])
										v.registerType(arrType, sliceOfInt)
										tmpTypeExpr = arrType
									} else {
										// []T return (non-pointer slice) → keep original []T type
										elemExpr := v.typeToExpr(sliceType.Elem())
										arrType := &ast.ArrayType{Elt: elemExpr}
										v.registerType(elemExpr, sliceType.Elem())
										v.registerType(arrType, firstResult)
										tmpTypeExpr = arrType
									}
								} else if _, isPtr := firstResult.(*types.Pointer); !isPtr {
									// Non-pointer, non-slice return (bool, string, etc.) → keep original type
									tmpTypeExpr = v.typeToExpr(firstResult)
									v.registerType(tmpTypeExpr, firstResult)
								}
								// else: *T return → int (the default)
							}
						}
					}
				}
			}
		}

		if hasPoolAccessLHS {
			// Use temp var to avoid pool access on LHS (safety concern with pool reallocation)
			// var __ptr_ret TYPE; __ptr_ret, _pool_T = f(args, _pool_T); originalLHS = __ptr_ret
			tmpName := fmt.Sprintf("__ptr_ret_%d", v.tmpCounter)
			v.tmpCounter++

			decl := &ast.DeclStmt{Decl: &ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{&ast.ValueSpec{
					Names: []*ast.Ident{{Name: tmpName}},
					Type:  tmpTypeExpr,
				}},
			}}
			stmts = append(stmts, decl)

			newLhs := []ast.Expr{&ast.Ident{Name: tmpName}}
			for poolName, elemType := range returnPools {
				poolIdent := &ast.Ident{Name: poolName}
				v.registerType(poolIdent, types.NewSlice(elemType))
				newLhs = append(newLhs, poolIdent)
			}
			stmts = append(stmts, &ast.AssignStmt{
				Lhs: newLhs,
				Tok: token.ASSIGN,
				Rhs: assign.Rhs,
			})

			stmts = append(stmts, &ast.AssignStmt{
				Lhs: assign.Lhs,
				Tok: assign.Tok,
				Rhs: []ast.Expr{&ast.Ident{Name: tmpName}},
			})
		} else {
			// := case: declare variable directly, no temp needed
			// var n1 TYPE; n1, _pool_T = f(args, _pool_T)
			lhsIdent := assign.Lhs[0].(*ast.Ident)

			decl := &ast.DeclStmt{Decl: &ast.GenDecl{
				Tok: token.VAR,
				Specs: []ast.Spec{&ast.ValueSpec{
					Names: []*ast.Ident{{Name: lhsIdent.Name}},
					Type:  tmpTypeExpr,
				}},
			}}
			stmts = append(stmts, decl)

			newLhs := []ast.Expr{lhsIdent}
			for poolName, elemType := range returnPools {
				poolIdent := &ast.Ident{Name: poolName}
				v.registerType(poolIdent, types.NewSlice(elemType))
				newLhs = append(newLhs, poolIdent)
			}
			stmts = append(stmts, &ast.AssignStmt{
				Lhs: newLhs,
				Tok: token.ASSIGN,
				Rhs: assign.Rhs,
			})
		}
		return stmts
	}

	// Simple case (= assignments only): add pool idents to LHS
	for poolName, elemType := range returnPools {
		poolIdent := &ast.Ident{Name: poolName}
		v.registerType(poolIdent, types.NewSlice(elemType))
		assign.Lhs = append(assign.Lhs, poolIdent)
	}
	return []ast.Stmt{assign}
}

// isPtrVarEliminatedStmt checks if a statement should be eliminated because it's
// a ptrVar declaration or assignment that has been converted to a ptrLocal alias.
func (v *ptrTransformVisitor) isPtrVarEliminatedStmt(stmt ast.Stmt, analysis *ptrAnalysis) bool {
	switch s := stmt.(type) {
	case *ast.DeclStmt:
		// Eliminate: var p *int where p is in both ptrVars and ptrLocals
		// (ptrLocals use alias elimination, so the var decl is not needed)
		// NOTE: ptrFieldLocals and ptrIndexLocals use pool-based indexing, so
		// their var decls are KEPT (type rewritten from *T to int in rewriteDeclStmt)
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
		// Eliminate: p = &x where p is a ptrVar converted to ptrLocal (alias elimination)
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
		// Pool var := assignments are handled by rewriteStmtExpand, not here.
		// Just rewrite all RHS normally.
		for i, rhs := range s.Rhs {
			s.Rhs[i] = v.rewriteExpr(rhs, analysis)
		}
		return s
	}

	// Regular assignment (=, +=, etc.)
	// Replace nil RHS with -1 when LHS is a pointer-typed field
	for i, rhs := range s.Rhs {
		if isNilIdent(rhs) && i < len(s.Lhs) {
			if v.isPointerTypedExpr(s.Lhs[i]) {
				s.Rhs[i] = v.minusOneExpr()
			}
		}
	}
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
					// ptrVars that are ptrLocals: eliminated by isPtrVarEliminatedStmt
					// ptrVars that are ptrFieldLocals/ptrIndexLocals: keep decl, rewrite type
					if _, isPtrVar := analysis.ptrVars[name.Name]; isPtrVar {
						_, isLocal := analysis.ptrLocals[name.Name]
						if !isLocal {
							// Change type from *T (StarExpr) to int (pool index)
							newType := &ast.Ident{Name: "int"}
							valueSpec.Type = newType
							v.registerType(newType, types.Typ[types.Int])
						}
					}
				}
				// Rewrite var items []*T → var items []int
				if arrType, ok := valueSpec.Type.(*ast.ArrayType); ok {
					if _, ok := arrType.Elt.(*ast.StarExpr); ok {
						newElt := &ast.Ident{Name: "int"}
						arrType.Elt = newElt
						sliceOfInt := types.NewSlice(types.Typ[types.Int])
						v.registerType(newElt, types.Typ[types.Int])
						v.registerType(arrType, sliceOfInt)
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
		// All pointer derefs use pool indexing: *p -> _pool_T[p]
		if ident, ok := e.X.(*ast.Ident); ok {
			// ptrFieldLocals: *p -> _pool_T[p] (pool-based, not alias)
			if info, isFieldLocal := analysis.ptrFieldLocals[ident.Name]; isFieldLocal {
				return v.poolIndexExpr(ident, info.elemType)
			}
			// ptrIndexLocals: *p -> _pool_T[p] (pool-based, not alias)
			if info, isIndexLocal := analysis.ptrIndexLocals[ident.Name]; isIndexLocal {
				return v.poolIndexExpr(ident, info.elemType)
			}
			// ptrCompositeLits: *p -> _pool_T[p] (direct pool allocation)
			if elemType, isCompLit := analysis.ptrCompositeLits[ident.Name]; isCompLit {
				return v.poolIndexExpr(ident, elemType)
			}
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
		// Handle p.field for ptrIndexLocals: p.field -> _pool_T[p].field (pool-based)
		if ident, ok := e.X.(*ast.Ident); ok {
			if info, isIndexLocal := analysis.ptrIndexLocals[ident.Name]; isIndexLocal {
				e.X = v.poolIndexExpr(ident, info.elemType)
				return e
			}
		}
		// Handle p.field for ptrCompositeLits: p.field -> _pool_T[p].field
		if ident, ok := e.X.(*ast.Ident); ok {
			if elemType, isCompLit := analysis.ptrCompositeLits[ident.Name]; isCompLit {
				e.X = v.poolIndexExpr(ident, elemType)
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
		// ptrFieldLocals and ptrIndexLocals: bare p stays as p (it's an int pool index)
		// No alias replacement needed — pool-based indexing handles this.
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
		// Handle new(T) → convert to &T{} (UnaryExpr wrapping zero-value CompositeLit)
		if funIdent, ok := e.Fun.(*ast.Ident); ok && funIdent.Name == "new" && len(e.Args) == 1 {
			if v.pkg != nil && v.pkg.TypesInfo != nil {
				if tv, ok := v.pkg.TypesInfo.Types[e]; ok {
					if ptr, ok := tv.Type.(*types.Pointer); ok {
						elemType := ptr.Elem()
						zeroLit := &ast.CompositeLit{Type: v.typeToExpr(elemType)}
						addrOf := &ast.UnaryExpr{Op: token.AND, X: zeroLit}
						v.registerType(addrOf, tv.Type)
						v.registerType(zeroLit, elemType)
						return v.rewriteExpr(addrOf, analysis)
					}
				}
			}
		}
		// Handle make([]*T, n) → make([]int, n)
		if funIdent, ok := e.Fun.(*ast.Ident); ok && funIdent.Name == "make" && len(e.Args) >= 1 {
			if arrType, ok := e.Args[0].(*ast.ArrayType); ok {
				if _, ok := arrType.Elt.(*ast.StarExpr); ok {
					newElt := &ast.Ident{Name: "int"}
					arrType.Elt = newElt
					sliceOfInt := types.NewSlice(types.Typ[types.Int])
					v.registerType(newElt, types.Typ[types.Int])
					v.registerType(arrType, sliceOfInt)
					v.registerType(e, sliceOfInt)
				}
			}
		}
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
		// Handle []*T composite literals: rewrite type to []int
		if arrType, ok := e.Type.(*ast.ArrayType); ok {
			if _, ok := arrType.Elt.(*ast.StarExpr); ok {
				newElt := &ast.Ident{Name: "int"}
				arrType.Elt = newElt
				sliceOfInt := types.NewSlice(types.Typ[types.Int])
				v.registerType(newElt, types.Typ[types.Int])
				v.registerType(arrType, sliceOfInt)
				v.registerType(e, sliceOfInt)
			}
		}
		// Rewrite elements first
		for i, elt := range e.Elts {
			e.Elts[i] = v.rewriteExpr(elt, analysis)
		}
		// Handle pointer fields in struct literals
		if typeIdent, ok := e.Type.(*ast.Ident); ok {
			if ptrFields, hasPtrFields := v.ptrFieldMap[typeIdent.Name]; hasPtrFields {
				// Build set of pointer field names for quick lookup
				ptrFieldSet := make(map[string]bool)
				for _, fn := range ptrFields {
					ptrFieldSet[fn] = true
				}
				// Find which pointer fields are already present and replace nil values with -1
				present := make(map[string]bool)
				for _, elt := range e.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						if key, ok := kv.Key.(*ast.Ident); ok {
							present[key.Name] = true
							// Replace nil values with -1 for pointer fields
							if ptrFieldSet[key.Name] && isNilIdent(kv.Value) {
								minusOne := &ast.UnaryExpr{
									Op: token.SUB,
									X:  &ast.BasicLit{Kind: token.INT, Value: "1"},
								}
								v.registerType(minusOne, types.Typ[types.Int])
								v.registerType(minusOne.X.(*ast.BasicLit), types.Typ[types.Int])
								kv.Value = minusOne
							}
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
	// Check params for *T and []*T types and collect unique pool names
	poolsNeeded := make(map[string]types.Type) // poolName → elemType (dedup)
	params := fnType.Params()
	for i := 0; i < params.Len(); i++ {
		paramType := params.At(i).Type()
		if ptrType, isPtr := paramType.(*types.Pointer); isPtr {
			elemType := ptrType.Elem()
			pn := poolNameForType(elemType)
			poolsNeeded[pn] = elemType
		}
		// []*T params also need pool args
		if sliceType, isSlice := paramType.(*types.Slice); isSlice {
			if ptrType, isPtr := sliceType.Elem().(*types.Pointer); isPtr {
				elemType := ptrType.Elem()
				pn := poolNameForType(elemType)
				poolsNeeded[pn] = elemType
			}
		}
	}
	// Also check return types for *T and []*T (functions returning pointers also need pool args)
	results := fnType.Results()
	for i := 0; i < results.Len(); i++ {
		resultType := results.At(i).Type()
		if ptrType, isPtr := resultType.(*types.Pointer); isPtr {
			elemType := ptrType.Elem()
			pn := poolNameForType(elemType)
			poolsNeeded[pn] = elemType
		}
		// []*T returns also need pool args
		if sliceType, isSlice := resultType.(*types.Slice); isSlice {
			if ptrType, isPtr := sliceType.Elem().(*types.Pointer); isPtr {
				elemType := ptrType.Elem()
				pn := poolNameForType(elemType)
				poolsNeeded[pn] = elemType
			}
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
		lit := &ast.CompositeLit{Type: v.typeToExpr(t)}
		v.registerType(lit, t)
		return lit
	}
	return &ast.BasicLit{Kind: token.INT, Value: "0"}
}

// findNestedAddrOfCompositeLits scans the function body for nested &CompositeLit
// inside composite literal fields (e.g., &TreeNode{left: &TreeNode{value: 2}})
// and registers the inner types in callReturnPools so pool declarations are created.
func (v *ptrTransformVisitor) findNestedAddrOfCompositeLits(body *ast.BlockStmt, analysis *ptrAnalysis) {
	if body == nil || v.pkg == nil || v.pkg.TypesInfo == nil {
		return
	}
	ast.Inspect(body, func(n ast.Node) bool {
		compLit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range compLit.Elts {
			v.scanForNestedAddrOf(elt, analysis)
		}
		return true
	})
}

// scanForNestedAddrOf recursively scans an expression for &CompositeLit{...}
// and registers the element type in callReturnPools.
func (v *ptrTransformVisitor) scanForNestedAddrOf(expr ast.Expr, analysis *ptrAnalysis) {
	if expr == nil {
		return
	}
	if kv, ok := expr.(*ast.KeyValueExpr); ok {
		v.scanForNestedAddrOf(kv.Value, analysis)
		return
	}
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		if innerLit, ok := unary.X.(*ast.CompositeLit); ok {
			// Register this type for pool allocation
			if tv, ok := v.pkg.TypesInfo.Types[unary]; ok {
				if ptr, ok := tv.Type.(*types.Pointer); ok {
					elemType := ptr.Elem()
					pn := poolNameForType(elemType)
					if _, exists := analysis.callReturnPools[pn]; !exists {
						analysis.callReturnPools[pn] = elemType
					}
				}
			}
			// Recurse into the inner composite literal's elements
			for _, elt := range innerLit.Elts {
				v.scanForNestedAddrOf(elt, analysis)
			}
		}
	}
}

// hoistAddrOfCompositeLit walks an expression tree and replaces nested &CompositeLit{...}
// with pool-allocated temp variables. For each &CompositeLit found:
// 1. Recursively hoist deeper nesting first
// 2. Rewrite the CompositeLit
// 3. Generate: _pool_T = append(_pool_T, rewrittenLit)
// 4. Generate: __nested_lit_N := int(len(_pool_T) - 1)
// 5. Return the __nested_lit_N ident
func (v *ptrTransformVisitor) hoistAddrOfCompositeLit(expr ast.Expr, analysis *ptrAnalysis, preStmts *[]ast.Stmt) ast.Expr {
	if expr == nil {
		return nil
	}
	if kv, ok := expr.(*ast.KeyValueExpr); ok {
		kv.Value = v.hoistAddrOfCompositeLit(kv.Value, analysis, preStmts)
		return kv
	}
	if unary, ok := expr.(*ast.UnaryExpr); ok && unary.Op == token.AND {
		if compLit, ok := unary.X.(*ast.CompositeLit); ok {
			// First, recursively hoist any nested &CompositeLit in this literal's fields
			for i, elt := range compLit.Elts {
				compLit.Elts[i] = v.hoistAddrOfCompositeLit(elt, analysis, preStmts)
			}

			// Get the element type
			var elemType types.Type
			if v.pkg != nil && v.pkg.TypesInfo != nil {
				if tv, ok := v.pkg.TypesInfo.Types[unary]; ok {
					if ptr, ok := tv.Type.(*types.Pointer); ok {
						elemType = ptr.Elem()
					}
				}
			}
			if elemType == nil {
				return expr
			}

			// Rewrite the composite literal (handles -1 defaults for pointer fields, etc.)
			rewrittenLit := v.rewriteExpr(compLit, analysis)

			// Generate pool append and index assignment
			poolName := poolNameForType(elemType)
			sliceType := types.NewSlice(elemType)

			// _pool_T = append(_pool_T, rewrittenLit)
			poolIdent1 := &ast.Ident{Name: poolName}
			poolIdent2 := &ast.Ident{Name: poolName}
			appendCall := &ast.CallExpr{
				Fun:  &ast.Ident{Name: "append"},
				Args: []ast.Expr{poolIdent1, rewrittenLit},
			}
			v.registerType(poolIdent1, sliceType)
			v.registerType(poolIdent2, sliceType)
			v.registerType(appendCall, sliceType)
			*preStmts = append(*preStmts, &ast.AssignStmt{
				Lhs: []ast.Expr{poolIdent2},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{appendCall},
			})

			// __nested_lit_N := int(len(_pool_T) - 1)
			tmpName := fmt.Sprintf("__nested_lit_%d", v.tmpCounter)
			v.tmpCounter++
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
			tmpIdent := &ast.Ident{Name: tmpName}
			v.registerType(tmpIdent, types.Typ[types.Int])
			*preStmts = append(*preStmts, &ast.AssignStmt{
				Lhs: []ast.Expr{tmpIdent},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{intCast},
			})

			// Return the temp variable ident
			retIdent := &ast.Ident{Name: tmpName}
			v.registerType(retIdent, types.Typ[types.Int])
			return retIdent
		}
	}
	if compLit, ok := expr.(*ast.CompositeLit); ok {
		for i, elt := range compLit.Elts {
			compLit.Elts[i] = v.hoistAddrOfCompositeLit(elt, analysis, preStmts)
		}
		return compLit
	}
	return expr
}

// hoistAddrOfCompositeLitInCallArgs scans a statement for call expressions whose arguments
// contain &CompositeLit{...} and hoists them to pool allocations. Pre-statements are
// appended to the provided slice.
func (v *ptrTransformVisitor) hoistAddrOfCompositeLitInCallArgs(stmt ast.Stmt, analysis *ptrAnalysis, preStmts *[]ast.Stmt) {
	var callExpr *ast.CallExpr

	// Extract CallExpr from different statement types
	if exprStmt, ok := stmt.(*ast.ExprStmt); ok {
		if ce, ok := exprStmt.X.(*ast.CallExpr); ok {
			callExpr = ce
		}
	}
	if assignStmt, ok := stmt.(*ast.AssignStmt); ok && len(assignStmt.Rhs) == 1 {
		if ce, ok := assignStmt.Rhs[0].(*ast.CallExpr); ok {
			// Don't handle append to []*T — that's handled separately above
			if funIdent, ok := ce.Fun.(*ast.Ident); ok && funIdent.Name == "append" {
				return
			}
			callExpr = ce
		}
	}

	if callExpr == nil {
		return
	}

	// Check if any arg is &CompositeLit{}
	hasAddrOfCompLit := false
	for _, arg := range callExpr.Args {
		if unary, ok := arg.(*ast.UnaryExpr); ok && unary.Op == token.AND {
			if _, ok := unary.X.(*ast.CompositeLit); ok {
				hasAddrOfCompLit = true
				break
			}
		}
	}
	if !hasAddrOfCompLit {
		return
	}

	// Hoist &CompositeLit args
	for i, arg := range callExpr.Args {
		callExpr.Args[i] = v.hoistAddrOfCompositeLit(arg, analysis, preStmts)
	}
}
