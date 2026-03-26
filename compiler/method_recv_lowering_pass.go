package compiler

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// MethodReceiverLoweringPass transforms method declarations and method calls
// into standalone functions. This runs between SemaChecker and PointerLoweringPass.
//
// Example:
//
//	func (c *Counter) Inc() { c.count++ }  →  func Counter_Inc(c *Counter) { c.count++ }
//	c.Inc()                                →  Counter_Inc(&c)
type MethodReceiverLoweringPass struct{}

func (p *MethodReceiverLoweringPass) Name() string { return "MethodReceiverLowering" }
func (p *MethodReceiverLoweringPass) ProLog()      {}
func (p *MethodReceiverLoweringPass) EpiLog()      {}

func (p *MethodReceiverLoweringPass) Visitors(pkg *packages.Package) []ast.Visitor {
	return []ast.Visitor{&methodLoweringVisitor{pkg: pkg}}
}

func (p *MethodReceiverLoweringPass) PreVisit(visitor ast.Visitor) {}

func (p *MethodReceiverLoweringPass) PostVisit(visitor ast.Visitor, visited map[string]struct{}) {
	v := visitor.(*methodLoweringVisitor)
	v.transform()
}

// methodLoweringVisitor collects function declarations during AST walk.
type methodLoweringVisitor struct {
	pkg     *packages.Package
	funcs   []*ast.FuncDecl
	methods map[string]*methodInfo // "Type.Method" → info
}

// methodInfo stores information about a method being lowered.
type methodInfo struct {
	typeName      string
	methodName    string
	loweredName   string
	isPointerRecv bool
	funcObj       types.Object // lowered function object for TypesInfo registration
}

func (v *methodLoweringVisitor) Visit(node ast.Node) ast.Visitor {
	if fd, ok := node.(*ast.FuncDecl); ok {
		v.funcs = append(v.funcs, fd)
	}
	return v
}

// extractReceiverTypeName extracts the type name from a receiver type expression.
// Returns the type name and whether it's a pointer receiver.
func extractReceiverTypeName(expr ast.Expr) (string, bool) {
	if starExpr, ok := expr.(*ast.StarExpr); ok {
		if ident, ok := starExpr.X.(*ast.Ident); ok {
			return ident.Name, true
		}
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name, false
	}
	return "", false
}

func (v *methodLoweringVisitor) transform() {
	v.methods = make(map[string]*methodInfo)

	// Phase 1: Collect method info from declarations (before any AST mutation)
	for _, fd := range v.funcs {
		if fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		recv := fd.Recv.List[0]
		typeName, isPointer := extractReceiverTypeName(recv.Type)
		if typeName == "" {
			continue
		}
		methodName := fd.Name.Name
		loweredName := typeName + "_" + methodName

		// Create lowered function type object (receiver becomes first param)
		var funcObj types.Object
		if v.pkg != nil && v.pkg.TypesInfo != nil {
			if obj := v.pkg.TypesInfo.Defs[fd.Name]; obj != nil {
				if fn, ok := obj.(*types.Func); ok {
					origSig := fn.Type().(*types.Signature)
					recvVar := origSig.Recv()
					origParams := origSig.Params()
					var newParamVars []*types.Var
					newParamVars = append(newParamVars, recvVar)
					for i := 0; i < origParams.Len(); i++ {
						newParamVars = append(newParamVars, origParams.At(i))
					}
					newParams := types.NewTuple(newParamVars...)
					loweredSig := types.NewSignatureType(nil, nil, nil, newParams, origSig.Results(), origSig.Variadic())
					funcObj = types.NewFunc(fd.Name.Pos(), v.pkg.Types, loweredName, loweredSig)
				}
			}
		}

		key := typeName + "." + methodName
		v.methods[key] = &methodInfo{
			typeName:      typeName,
			methodName:    methodName,
			loweredName:   loweredName,
			isPointerRecv: isPointer,
			funcObj:       funcObj,
		}
	}

	if len(v.methods) == 0 {
		return
	}

	// Phase 2: Rewrite call sites in ALL function bodies
	for _, fd := range v.funcs {
		if fd.Body == nil {
			continue
		}
		v.rewriteCallSites(fd.Body)
	}

	// Phase 3: Lower method declarations (rename, move receiver to first param)
	for _, fd := range v.funcs {
		if fd.Recv == nil || len(fd.Recv.List) == 0 {
			continue
		}
		recv := fd.Recv.List[0]
		typeName, _ := extractReceiverTypeName(recv.Type)
		if typeName == "" {
			continue
		}
		key := typeName + "." + fd.Name.Name
		info, ok := v.methods[key]
		if !ok {
			continue
		}

		// Rename function
		fd.Name.Name = info.loweredName

		// Move receiver to first param
		if fd.Type.Params == nil {
			fd.Type.Params = &ast.FieldList{}
		}
		fd.Type.Params.List = append([]*ast.Field{recv}, fd.Type.Params.List...)

		// Remove receiver
		fd.Recv = nil

		// Update TypesInfo.Defs with lowered function object
		if v.pkg != nil && v.pkg.TypesInfo != nil && info.funcObj != nil {
			v.pkg.TypesInfo.Defs[fd.Name] = info.funcObj
		}
	}
}

// rewriteCallSites walks a block statement and rewrites method calls to standalone function calls.
func (v *methodLoweringVisitor) rewriteCallSites(block *ast.BlockStmt) {
	if v.pkg == nil || v.pkg.TypesInfo == nil {
		return
	}
	ast.Inspect(block, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		selExpr, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		// Check if this is a method call via TypesInfo.Selections
		sel, ok := v.pkg.TypesInfo.Selections[selExpr]
		if !ok || sel.Kind() != types.MethodVal {
			return true
		}

		// Get the method object and its signature
		methodObj := sel.Obj()
		sig, ok := methodObj.Type().(*types.Signature)
		if !ok {
			return true
		}

		// Extract type name from the method's receiver declaration
		recvType := sig.Recv().Type()
		typeName := ""
		isPointerRecv := false
		if ptr, ok := recvType.(*types.Pointer); ok {
			isPointerRecv = true
			if named, ok := ptr.Elem().(*types.Named); ok {
				typeName = named.Obj().Name()
			}
		} else if named, ok := recvType.(*types.Named); ok {
			typeName = named.Obj().Name()
		}
		if typeName == "" {
			return true
		}

		key := typeName + "." + methodObj.Name()
		info, ok := v.methods[key]
		if !ok {
			return true
		}

		// Replace Fun with lowered function name
		newFunIdent := &ast.Ident{Name: info.loweredName}
		call.Fun = newFunIdent

		// Register in TypesInfo.Uses so PointerLoweringPass can find the function
		if info.funcObj != nil {
			v.pkg.TypesInfo.Uses[newFunIdent] = info.funcObj
		}

		// Prepare receiver argument
		receiverExpr := selExpr.X
		actualRecvType := sel.Recv() // type of the receiver expression (x in x.Method())

		if isPointerRecv {
			// Method wants *T
			if _, isPtr := actualRecvType.(*types.Pointer); !isPtr {
				// Variable is T, need &
				receiverExpr = &ast.UnaryExpr{Op: token.AND, X: receiverExpr}
			}
		} else {
			// Method wants T (value receiver)
			if _, isPtr := actualRecvType.(*types.Pointer); isPtr {
				// Variable is *T, need *
				receiverExpr = &ast.StarExpr{X: receiverExpr}
			}
		}

		// Prepend receiver to args
		newArgs := make([]ast.Expr, 0, len(call.Args)+1)
		newArgs = append(newArgs, receiverExpr)
		newArgs = append(newArgs, call.Args...)
		call.Args = newArgs

		return true
	})
}
