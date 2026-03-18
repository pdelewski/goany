package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

var primTypes = map[string]struct{}{
	"int8":    {},
	"int16":   {},
	"int32":   {},
	"int64":   {},
	"uint8":   {},
	"uint16":  {},
	"uint32":  {},
	"uint64":  {},
	"float32": {},
	"float64": {},
}

var namespaces = map[string]struct{}{}

type GenTypeInfo struct {
	Name       string
	Struct     *ast.StructType
	Other      ast.Node
	IsExternal bool // Whether this struct is external or local
	Pkg        string
	BaseType   string
}

type BasePass struct {
	PassName   string
	outputFile string
	file       *os.File
	visitor    *BasePassVisitor
	Emitter    Emitter
}

type BasePassVisitor struct {
	pkg     *packages.Package
	pass    *BasePass
	nodes   []ast.Node
	emitter Emitter
}

func (v *BasePass) Name() string {
	return v.PassName
}

func (v *BasePass) Visitors(pkg *packages.Package) []ast.Visitor {
	v.visitor = &BasePassVisitor{pkg: pkg, emitter: v.Emitter}
	v.visitor.pass = v
	return []ast.Visitor{v.visitor}
}

func (v *BasePassVisitor) emitAsString(s string, indent int) string {
	return strings.Repeat(" ", indent) + s
}

func (v *BasePassVisitor) emitArgs(node *ast.CallExpr, indent int) {
	v.emitter.GetForestBuilder().AddVisitMarker(PreVisitCallExprArgs)
	v.emitter.PreVisitCallExprArgs(node.Args, indent)
	for i, arg := range node.Args {
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitCallExprArg)
		v.emitter.PreVisitCallExprArg(arg, i, indent)
		v.traverseExpression(arg, 0) // Function arguments
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitCallExprArg)
		v.emitter.PostVisitCallExprArg(arg, i, indent)
	}
	v.emitter.GetForestBuilder().AddVisitMarker(PostVisitCallExprArgs)
	v.emitter.PostVisitCallExprArgs(node.Args, indent)
}

func (v *BasePassVisitor) traverseExpression(expr ast.Expr, indent int) string {
	var str string
	switch e := expr.(type) {
	case *ast.BasicLit:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitBasicLit)
		v.emitter.PreVisitBasicLit(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitBasicLit)
		v.emitter.PostVisitBasicLit(e, indent)
	case *ast.Ident:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitIdent)
		v.emitter.PreVisitIdent(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitIdent)
		v.emitter.PostVisitIdent(e, indent)
	case *ast.BinaryExpr:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitBinaryExpr)
		v.emitter.PreVisitBinaryExpr(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitBinaryExprLeft)
		v.emitter.PreVisitBinaryExprLeft(e.X, indent)
		v.traverseExpression(e.X, indent) // Left operand
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitBinaryExprLeft)
		v.emitter.PostVisitBinaryExprLeft(e.X, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitBinaryExprOperator)
		v.emitter.PreVisitBinaryExprOperator(e.Op, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitBinaryExprOperator)
		v.emitter.PostVisitBinaryExprOperator(e.Op, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitBinaryExprRight)
		v.emitter.PreVisitBinaryExprRight(e.Y, indent)
		v.traverseExpression(e.Y, indent) // Right operand
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitBinaryExprRight)
		v.emitter.PostVisitBinaryExprRight(e.Y, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitBinaryExpr)
		v.emitter.PostVisitBinaryExpr(e, indent)
	case *ast.CallExpr:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitCallExpr)
		v.emitter.PreVisitCallExpr(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitCallExprFun)
		v.emitter.PreVisitCallExprFun(e.Fun, indent)
		v.traverseExpression(e.Fun, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitCallExprFun)
		v.emitter.PostVisitCallExprFun(e.Fun, indent)
		v.emitArgs(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitCallExpr)
		v.emitter.PostVisitCallExpr(e, indent)
	case *ast.ParenExpr:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitParenExpr)
		v.emitter.PreVisitParenExpr(e, indent)
		v.traverseExpression(e.X, indent) // Dump inner expression
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitParenExpr)
		v.emitter.PostVisitParenExpr(e, indent)
	case *ast.CompositeLit:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitCompositeLit)
		v.emitter.PreVisitCompositeLit(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitCompositeLitType)
		v.emitter.PreVisitCompositeLitType(e.Type, indent)
		v.traverseExpression(e.Type, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitCompositeLitType)
		v.emitter.PostVisitCompositeLitType(e.Type, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitCompositeLitElts)
		v.emitter.PreVisitCompositeLitElts(e.Elts, indent)
		for i, elt := range e.Elts {
			v.emitter.GetForestBuilder().AddVisitMarker(PreVisitCompositeLitElt)
			v.emitter.PreVisitCompositeLitElt(elt, i, indent)
			v.traverseExpression(elt, 0) // Function arguments
			v.emitter.GetForestBuilder().AddVisitMarker(PostVisitCompositeLitElt)
			v.emitter.PostVisitCompositeLitElt(elt, i, indent)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitCompositeLitElts)
		v.emitter.PostVisitCompositeLitElts(e.Elts, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitCompositeLit)
		v.emitter.PostVisitCompositeLit(e, indent)
	case *ast.ArrayType:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitArrayType)
		v.emitter.PreVisitArrayType(*e, indent)
		v.traverseExpression(e.Elt, 0)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitArrayType)
		v.emitter.PostVisitArrayType(*e, indent)
	case *ast.SelectorExpr:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitSelectorExpr)
		v.emitter.PreVisitSelectorExpr(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitSelectorExprX)
		v.emitter.PreVisitSelectorExprX(e.X, indent)
		v.traverseExpression(e.X, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitSelectorExprX)
		v.emitter.PostVisitSelectorExprX(e.X, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitSelectorExprSel)
		v.emitter.PreVisitSelectorExprSel(e.Sel, indent)
		oldIndent := indent
		v.traverseExpression(e.Sel, 0)
		indent = oldIndent
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitSelectorExprSel)
		v.emitter.PostVisitSelectorExprSel(e.Sel, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitSelectorExpr)
		v.emitter.PostVisitSelectorExpr(e, indent)
	case *ast.IndexExpr:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitIndexExpr)
		v.emitter.PreVisitIndexExpr(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitIndexExprX)
		v.emitter.PreVisitIndexExprX(e, indent)
		v.traverseExpression(e.X, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitIndexExprX)
		v.emitter.PostVisitIndexExprX(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitIndexExprIndex)
		v.emitter.PreVisitIndexExprIndex(e, indent)
		v.traverseExpression(e.Index, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitIndexExprIndex)
		v.emitter.PostVisitIndexExprIndex(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitIndexExpr)
		v.emitter.PostVisitIndexExpr(e, indent)
	case *ast.UnaryExpr:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitUnaryExpr)
		v.emitter.PreVisitUnaryExpr(e, indent)
		v.traverseExpression(e.X, 0)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitUnaryExpr)
		v.emitter.PostVisitUnaryExpr(e, indent)
	case *ast.SliceExpr:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitSliceExpr)
		v.emitter.PreVisitSliceExpr(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitSliceExprX)
		v.emitter.PreVisitSliceExprX(e.X, indent)
		v.traverseExpression(e.X, 0)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitSliceExprX)
		v.emitter.PostVisitSliceExprX(e.X, indent)
		// Check and print Low, High, and Max
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitSliceExprXBegin)
		v.emitter.PreVisitSliceExprXBegin(e.X, indent)
		v.traverseExpression(e.X, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitSliceExprXBegin)
		v.emitter.PostVisitSliceExprXBegin(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitSliceExprLow)
		v.emitter.PreVisitSliceExprLow(e.Low, indent)
		if e.Low != nil {
			v.traverseExpression(e.Low, indent)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitSliceExprLow)
		v.emitter.PostVisitSliceExprLow(e.Low, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitSliceExprXEnd)
		v.emitter.PreVisitSliceExprXEnd(e, indent)
		v.traverseExpression(e.X, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitSliceExprXEnd)
		v.emitter.PostVisitSliceExprXEnd(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitSliceExprHigh)
		v.emitter.PreVisitSliceExprHigh(e.High, indent)
		if e.High != nil {
			v.traverseExpression(e.High, indent)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitSliceExprHigh)
		v.emitter.PostVisitSliceExprHigh(e.High, indent)
		if e.Slice3 && e.Max != nil {
			v.traverseExpression(e.Max, indent)
		} else if e.Slice3 {
			DebugLogPrintf("Max index: <nil>\n")
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitSliceExpr)
		v.emitter.PostVisitSliceExpr(e, indent)
	case *ast.FuncType:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncType)
		v.emitter.PreVisitFuncType(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncTypeResults)
		v.emitter.PreVisitFuncTypeResults(e.Results, indent)
		if e.Results != nil {
			for i, result := range e.Results.List {
				v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncTypeResult)
				v.emitter.PreVisitFuncTypeResult(result, i, indent)
				v.traverseExpression(result.Type, indent)
				v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncTypeResult)
				v.emitter.PostVisitFuncTypeResult(result, i, indent)
			}
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncTypeResults)
		v.emitter.PostVisitFuncTypeResults(e.Results, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncTypeParams)
		v.emitter.PreVisitFuncTypeParams(e.Params, indent)
		for i, param := range e.Params.List {
			v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncTypeParam)
			v.emitter.PreVisitFuncTypeParam(param, i, indent)
			v.traverseExpression(param.Type, 0)
			v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncTypeParam)
			v.emitter.PostVisitFuncTypeParam(param, i, indent)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncTypeParams)
		v.emitter.PostVisitFuncTypeParams(e.Params, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncType)
		v.emitter.PostVisitFuncType(e, indent)
	case *ast.KeyValueExpr:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitKeyValueExpr)
		v.emitter.PreVisitKeyValueExpr(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitKeyValueExprKey)
		v.emitter.PreVisitKeyValueExprKey(e.Key, indent)
		v.traverseExpression(e.Key, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitKeyValueExprKey)
		v.emitter.PostVisitKeyValueExprKey(e.Key, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitKeyValueExprValue)
		v.emitter.PreVisitKeyValueExprValue(e.Value, indent)
		v.traverseExpression(e.Value, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitKeyValueExprValue)
		v.emitter.PostVisitKeyValueExprValue(e.Value, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitKeyValueExpr)
		v.emitter.PostVisitKeyValueExpr(e, indent)
	case *ast.FuncLit:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncLit)
		v.emitter.PreVisitFuncLit(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncLitTypeParams)
		v.emitter.PreVisitFuncLitTypeParams(e.Type.Params, indent)
		for i, param := range e.Type.Params.List {
			v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncLitTypeParam)
			v.emitter.PreVisitFuncLitTypeParam(param, i, indent)
			v.traverseExpression(param.Type, indent)
			v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncLitTypeParam)
			v.emitter.PostVisitFuncLitTypeParam(param, i, indent)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncLitTypeParams)
		v.emitter.PostVisitFuncLitTypeParams(e.Type.Params, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncLitTypeResults)
		v.emitter.PreVisitFuncLitTypeResults(e.Type.Results, indent)

		if e.Type.Results != nil {
			for i, result := range e.Type.Results.List {
				v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncLitTypeResult)
				v.emitter.PreVisitFuncLitTypeResult(result, i, indent)
				v.traverseExpression(result.Type, indent)
				v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncLitTypeResult)
				v.emitter.PostVisitFuncLitTypeResult(result, i, indent)
			}
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncLitTypeResults)
		v.emitter.PostVisitFuncLitTypeResults(e.Type.Results, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncLitBody)
		v.emitter.PreVisitFuncLitBody(e.Body, indent)
		v.traverseStmt(e.Body, indent+4)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncLitBody)
		v.emitter.PostVisitFuncLitBody(e.Body, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncLit)
		v.emitter.PostVisitFuncLit(e, indent)
	case *ast.TypeAssertExpr:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitTypeAssertExpr)
		v.emitter.PreVisitTypeAssertExpr(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitTypeAssertExprType)
		v.emitter.PreVisitTypeAssertExprType(e.Type, indent)
		v.traverseExpression(e.Type, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitTypeAssertExprType)
		v.emitter.PostVisitTypeAssertExprType(e.Type, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitTypeAssertExprX)
		v.emitter.PreVisitTypeAssertExprX(e.X, indent)
		v.traverseExpression(e.X, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitTypeAssertExprX)
		v.emitter.PostVisitTypeAssertExprX(e.X, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitTypeAssertExpr)
		v.emitter.PostVisitTypeAssertExpr(e, indent)
	case *ast.StarExpr:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitStarExpr)
		v.emitter.PreVisitStarExpr(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitStarExprX)
		v.emitter.PreVisitStarExprX(e.X, indent)
		v.traverseExpression(e.X, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitStarExprX)
		v.emitter.PostVisitStarExprX(e.X, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitStarExpr)
		v.emitter.PostVisitStarExpr(e, indent)
	case *ast.InterfaceType:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitInterfaceType)
		v.emitter.PreVisitInterfaceType(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitInterfaceType)
		v.emitter.PostVisitInterfaceType(e, indent)
	case *ast.StructType:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitStructType)
		v.emitter.PreVisitStructType(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitStructType)
		v.emitter.PostVisitStructType(e, indent)
	case *ast.MapType:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitMapType)
		v.emitter.PreVisitMapType(e, indent)
		// Traverse key and value types to check nested type patterns
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitMapKeyType)
		v.emitter.PreVisitMapKeyType(e.Key, indent)
		v.traverseExpression(e.Key, 0)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitMapKeyType)
		v.emitter.PostVisitMapKeyType(e.Key, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitMapValueType)
		v.emitter.PreVisitMapValueType(e.Value, indent)
		v.traverseExpression(e.Value, 0)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitMapValueType)
		v.emitter.PostVisitMapValueType(e.Value, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitMapType)
		v.emitter.PostVisitMapType(e, indent)
	case *ast.ChanType:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitChanType)
		v.emitter.PreVisitChanType(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitChanType)
		v.emitter.PostVisitChanType(e, indent)
	case *ast.Ellipsis:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitEllipsis)
		v.emitter.PreVisitEllipsis(e, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitEllipsis)
		v.emitter.PostVisitEllipsis(e, indent)
	default:
		panic(fmt.Sprintf("unsupported expression type: %T", e))
	}
	return str
}

func (v *BasePassVisitor) traverseAssignment(assignStmt *ast.AssignStmt, indent int) {
	v.emitter.GetForestBuilder().AddVisitMarker(PreVisitAssignStmtLhs)
	v.emitter.PreVisitAssignStmtLhs(assignStmt, indent)
	for i := 0; i < len(assignStmt.Lhs); i++ {
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitAssignStmtLhsExpr)
		v.emitter.PreVisitAssignStmtLhsExpr(assignStmt.Lhs[i], i, indent)
		v.traverseExpression(assignStmt.Lhs[i], indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitAssignStmtLhsExpr)
		v.emitter.PostVisitAssignStmtLhsExpr(assignStmt.Lhs[i], i, indent)
	}
	v.emitter.GetForestBuilder().AddVisitMarker(PostVisitAssignStmtLhs)
	v.emitter.PostVisitAssignStmtLhs(assignStmt, indent)

	v.emitter.GetForestBuilder().AddVisitMarker(PreVisitAssignStmtRhs)
	v.emitter.PreVisitAssignStmtRhs(assignStmt, indent)
	for i := 0; i < len(assignStmt.Rhs); i++ {
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitAssignStmtRhsExpr)
		v.emitter.PreVisitAssignStmtRhsExpr(assignStmt.Rhs[i], i, indent)
		v.traverseExpression(assignStmt.Rhs[i], indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitAssignStmtRhsExpr)
		v.emitter.PostVisitAssignStmtRhsExpr(assignStmt.Rhs[i], i, indent)
	}
	v.emitter.GetForestBuilder().AddVisitMarker(PostVisitAssignStmtRhs)
	v.emitter.PostVisitAssignStmtRhs(assignStmt, indent)
}

func (v *BasePassVisitor) traverseStmt(stmt ast.Stmt, indent int) {
	switch stmt := stmt.(type) {
	case *ast.ExprStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitExprStmt)
		v.emitter.PreVisitExprStmt(stmt, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitExprStmtX)
		v.emitter.PreVisitExprStmtX(stmt.X, indent)
		v.traverseExpression(stmt.X, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitExprStmtX)
		v.emitter.PostVisitExprStmtX(stmt.X, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitExprStmt)
		v.emitter.PostVisitExprStmt(stmt, indent)
	case *ast.DeclStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitDeclStmt)
		v.emitter.PreVisitDeclStmt(stmt, indent)
		if genDecl, ok := stmt.Decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			for _, spec := range genDecl.Specs {
				if valueSpec, ok := spec.(*ast.ValueSpec); ok {
					// Iterate through all variables declared
					for i := 0; i < len(valueSpec.Names); i++ {
						v.emitter.GetForestBuilder().AddVisitMarker(PreVisitDeclStmtValueSpecType)
						v.emitter.PreVisitDeclStmtValueSpecType(valueSpec, i, indent)
						v.traverseExpression(valueSpec.Type, indent)
						v.emitter.GetForestBuilder().AddVisitMarker(PostVisitDeclStmtValueSpecType)
						v.emitter.PostVisitDeclStmtValueSpecType(valueSpec, i, indent)
						v.emitter.GetForestBuilder().AddVisitMarker(PreVisitDeclStmtValueSpecNames)
						v.emitter.PreVisitDeclStmtValueSpecNames(valueSpec.Names[i], i, indent)
						v.traverseExpression(valueSpec.Names[i], 0)
						v.emitter.GetForestBuilder().AddVisitMarker(PostVisitDeclStmtValueSpecNames)
						v.emitter.PostVisitDeclStmtValueSpecNames(valueSpec.Names[i], i, indent)
						// Traverse initialization value if present
						if i < len(valueSpec.Values) {
							v.emitter.GetForestBuilder().AddVisitMarker(PreVisitDeclStmtValueSpecValue)
							v.emitter.PreVisitDeclStmtValueSpecValue(valueSpec.Values[i], i, indent)
							v.traverseExpression(valueSpec.Values[i], indent)
							v.emitter.GetForestBuilder().AddVisitMarker(PostVisitDeclStmtValueSpecValue)
							v.emitter.PostVisitDeclStmtValueSpecValue(valueSpec.Values[i], i, indent)
						}
					}
				}
			}
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitDeclStmt)
		v.emitter.PostVisitDeclStmt(stmt, indent)
	case *ast.AssignStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitAssignStmt)
		v.emitter.PreVisitAssignStmt(stmt, indent)
		v.traverseAssignment(stmt, 0)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitAssignStmt)
		v.emitter.PostVisitAssignStmt(stmt, indent)
	case *ast.ReturnStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitReturnStmt)
		v.emitter.PreVisitReturnStmt(stmt, indent)
		for i := 0; i < len(stmt.Results); i++ {
			v.emitter.GetForestBuilder().AddVisitMarker(PreVisitReturnStmtResult)
			v.emitter.PreVisitReturnStmtResult(stmt.Results[i], i, indent)
			v.traverseExpression(stmt.Results[i], 0)
			v.emitter.GetForestBuilder().AddVisitMarker(PostVisitReturnStmtResult)
			v.emitter.PostVisitReturnStmtResult(stmt.Results[i], i, indent)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitReturnStmt)
		v.emitter.PostVisitReturnStmt(stmt, indent)
	case *ast.IfStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitIfStmt)
		v.emitter.PreVisitIfStmt(stmt, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitIfStmtInit)
		v.emitter.PreVisitIfStmtInit(stmt.Init, indent)
		if stmt.Init != nil {
			v.traverseStmt(stmt.Init, indent)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitIfStmtInit)
		v.emitter.PostVisitIfStmtInit(stmt.Init, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitIfStmtCond)
		v.emitter.PreVisitIfStmtCond(stmt, indent)
		v.traverseExpression(stmt.Cond, 0)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitIfStmtCond)
		v.emitter.PostVisitIfStmtCond(stmt, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitIfStmtBody)
		v.emitter.PreVisitIfStmtBody(stmt, indent)
		v.traverseStmt(stmt.Body, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitIfStmtBody)
		v.emitter.PostVisitIfStmtBody(stmt, indent)
		if stmt.Else != nil {
			v.emitter.GetForestBuilder().AddVisitMarker(PreVisitIfStmtElse)
			v.emitter.PreVisitIfStmtElse(stmt, indent)
			v.traverseStmt(stmt.Else, indent)
			v.emitter.GetForestBuilder().AddVisitMarker(PostVisitIfStmtElse)
			v.emitter.PostVisitIfStmtElse(stmt, indent)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitIfStmt)
		v.emitter.PostVisitIfStmt(stmt, indent)
	case *ast.ForStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitForStmt)
		v.emitter.PreVisitForStmt(stmt, indent)

		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitForStmtInit)
		v.emitter.PreVisitForStmtInit(stmt.Init, indent)
		if stmt.Init != nil {
			v.traverseStmt(stmt.Init, 0)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitForStmtInit)
		v.emitter.PostVisitForStmtInit(stmt.Init, indent)

		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitForStmtCond)
		v.emitter.PreVisitForStmtCond(stmt.Cond, indent)
		if stmt.Cond != nil {
			v.traverseExpression(stmt.Cond, 0)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitForStmtCond)
		v.emitter.PostVisitForStmtCond(stmt.Cond, indent)

		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitForStmtPost)
		v.emitter.PreVisitForStmtPost(stmt.Post, indent)
		if stmt.Post != nil {
			v.traverseStmt(stmt.Post, 0)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitForStmtPost)
		v.emitter.PostVisitForStmtPost(stmt.Post, indent)

		v.traverseStmt(stmt.Body, indent)

		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitForStmt)
		v.emitter.PostVisitForStmt(stmt, indent)
	case *ast.RangeStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitRangeStmt)
		v.emitter.PreVisitRangeStmt(stmt, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitRangeStmtKey)
		v.emitter.PreVisitRangeStmtKey(stmt.Key, indent)
		if stmt.Key != nil {
			v.traverseExpression(stmt.Key, 0)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitRangeStmtKey)
		v.emitter.PostVisitRangeStmtKey(stmt.Key, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitRangeStmtValue)
		v.emitter.PreVisitRangeStmtValue(stmt.Value, indent)
		if stmt.Value != nil {
			v.traverseExpression(stmt.Value, 0)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitRangeStmtValue)
		v.emitter.PostVisitRangeStmtValue(stmt.Value, indent)

		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitRangeStmtX)
		v.emitter.PreVisitRangeStmtX(stmt.X, indent)
		v.traverseExpression(stmt.X, 0)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitRangeStmtX)
		v.emitter.PostVisitRangeStmtX(stmt.X, indent)

		v.traverseStmt(stmt.Body, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitRangeStmt)
		v.emitter.PostVisitRangeStmt(stmt, indent)
	case *ast.SwitchStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitSwitchStmt)
		v.emitter.PreVisitSwitchStmt(stmt, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitSwitchStmtTag)
		v.emitter.PreVisitSwitchStmtTag(stmt.Tag, indent)
		v.traverseExpression(stmt.Tag, 0)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitSwitchStmtTag)
		v.emitter.PostVisitSwitchStmtTag(stmt.Tag, indent)

		for _, stmt := range stmt.Body.List {
			v.traverseStmt(stmt, indent+2)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitSwitchStmt)
		v.emitter.PostVisitSwitchStmt(stmt, indent)
	case *ast.TypeSwitchStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitTypeSwitchStmt)
		v.emitter.PreVisitTypeSwitchStmt(stmt, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitTypeSwitchStmt)
		v.emitter.PostVisitTypeSwitchStmt(stmt, indent)
	case *ast.BranchStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitBranchStmt)
		v.emitter.PreVisitBranchStmt(stmt, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitBranchStmt)
		v.emitter.PostVisitBranchStmt(stmt, indent)
	case *ast.IncDecStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitIncDecStmt)
		v.emitter.PreVisitIncDecStmt(stmt, indent)
		v.traverseExpression(stmt.X, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitIncDecStmt)
		v.emitter.PostVisitIncDecStmt(stmt, indent)
	case *ast.CaseClause:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitCaseClause)
		v.emitter.PreVisitCaseClause(stmt, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitCaseClauseList)
		v.emitter.PreVisitCaseClauseList(stmt.List, indent)
		for i := 0; i < len(stmt.List); i++ {
			v.emitter.GetForestBuilder().AddVisitMarker(PreVisitCaseClauseListExpr)
			v.emitter.PreVisitCaseClauseListExpr(stmt.List[i], i, indent)
			v.traverseExpression(stmt.List[i], 0)
			v.emitter.GetForestBuilder().AddVisitMarker(PostVisitCaseClauseListExpr)
			v.emitter.PostVisitCaseClauseListExpr(stmt.List[i], i, indent)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitCaseClauseList)
		v.emitter.PostVisitCaseClauseList(stmt.List, indent)
		for i := 0; i < len(stmt.Body); i++ {
			v.traverseStmt(stmt.Body[i], indent+4)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitCaseClause)
		v.emitter.PostVisitCaseClause(stmt, indent)
	case *ast.BlockStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitBlockStmt)
		v.emitter.PreVisitBlockStmt(stmt, indent)
		for i := 0; i < len(stmt.List); i++ {
			v.emitter.GetForestBuilder().AddVisitMarker(PreVisitBlockStmtList)
			v.emitter.PreVisitBlockStmtList(stmt.List[i], i, indent+2)
			v.traverseStmt(stmt.List[i], indent+2)
			v.emitter.GetForestBuilder().AddVisitMarker(PostVisitBlockStmtList)
			v.emitter.PostVisitBlockStmtList(stmt.List[i], i, indent+2)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitBlockStmt)
		v.emitter.PostVisitBlockStmt(stmt, indent)
	case *ast.DeferStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitDeferStmt)
		v.emitter.PreVisitDeferStmt(stmt, indent)
		v.traverseExpression(stmt.Call, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitDeferStmt)
		v.emitter.PostVisitDeferStmt(stmt, indent)
	case *ast.GoStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitGoStmt)
		v.emitter.PreVisitGoStmt(stmt, indent)
		v.traverseExpression(stmt.Call, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitGoStmt)
		v.emitter.PostVisitGoStmt(stmt, indent)
	case *ast.SendStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitSendStmt)
		v.emitter.PreVisitSendStmt(stmt, indent)
		v.traverseExpression(stmt.Chan, indent)
		v.traverseExpression(stmt.Value, indent)
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitSendStmt)
		v.emitter.PostVisitSendStmt(stmt, indent)
	case *ast.SelectStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitSelectStmt)
		v.emitter.PreVisitSelectStmt(stmt, indent)
		if stmt.Body != nil {
			v.traverseStmt(stmt.Body, indent)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitSelectStmt)
		v.emitter.PostVisitSelectStmt(stmt, indent)
	case *ast.LabeledStmt:
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitLabeledStmt)
		v.emitter.PreVisitLabeledStmt(stmt, indent)
		if stmt.Stmt != nil {
			v.traverseStmt(stmt.Stmt, indent)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitLabeledStmt)
		v.emitter.PostVisitLabeledStmt(stmt, indent)
	default:
		DebugPrintf("<Other statement type>\n")
	}
}

func (v *BasePassVisitor) generateFuncDeclSignature(node *ast.FuncDecl) ast.Visitor {
	v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncDeclSignature)
	v.emitter.PreVisitFuncDeclSignature(node, 0)
	v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncDeclSignatureTypeResults)
	v.emitter.PreVisitFuncDeclSignatureTypeResults(node, 0)

	if node.Type.Results != nil {
		for i := 0; i < len(node.Type.Results.List); i++ {
			v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncDeclSignatureTypeResultsList)
			v.emitter.PreVisitFuncDeclSignatureTypeResultsList(node.Type.Results.List[i], i, 0)
			v.traverseExpression(node.Type.Results.List[i].Type, 0)
			v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncDeclSignatureTypeResultsList)
			v.emitter.PostVisitFuncDeclSignatureTypeResultsList(node.Type.Results.List[i], i, 0)
		}
	}

	v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncDeclSignatureTypeResults)
	v.emitter.PostVisitFuncDeclSignatureTypeResults(node, 0)

	v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncDeclName)
	v.emitter.PreVisitFuncDeclName(node.Name, 0)
	v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncDeclName)
	v.emitter.PostVisitFuncDeclName(node.Name, 0)

	v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncDeclSignatureTypeParams)
	v.emitter.PreVisitFuncDeclSignatureTypeParams(node, 0)
	for i := 0; i < len(node.Type.Params.List); i++ {
		v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncDeclSignatureTypeParamsList)
		v.emitter.PreVisitFuncDeclSignatureTypeParamsList(node.Type.Params.List[i], i, 0)
		for j := 0; j < len(node.Type.Params.List[i].Names); j++ {
			argName := node.Type.Params.List[i].Names[j]
			v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncDeclSignatureTypeParamsListType)
			v.emitter.PreVisitFuncDeclSignatureTypeParamsListType(node.Type.Params.List[i].Type, argName, j, 0)
			v.traverseExpression(node.Type.Params.List[i].Type, 0)
			v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncDeclSignatureTypeParamsListType)
			v.emitter.PostVisitFuncDeclSignatureTypeParamsListType(node.Type.Params.List[i].Type, argName, j, 0)
			v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncDeclSignatureTypeParamsArgName)
			v.emitter.PreVisitFuncDeclSignatureTypeParamsArgName(argName, j, 0)
			v.traverseExpression(argName, 0)
			v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncDeclSignatureTypeParamsArgName)
			v.emitter.PostVisitFuncDeclSignatureTypeParamsArgName(argName, j, 0)
		}
		v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncDeclSignatureTypeParamsList)
		v.emitter.PostVisitFuncDeclSignatureTypeParamsList(node.Type.Params.List[i], i, 0)
	}
	v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncDeclSignatureTypeParams)
	v.emitter.PostVisitFuncDeclSignatureTypeParams(node, 0)
	v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncDeclSignature)
	v.emitter.PostVisitFuncDeclSignature(node, 0)
	return v
}

func (v *BasePassVisitor) generateFuncDecl(node *ast.FuncDecl) ast.Visitor {
	v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncDecl)
	v.emitter.PreVisitFuncDecl(node, 0)
	v.generateFuncDeclSignature(node)
	v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncDeclBody)
	v.emitter.PreVisitFuncDeclBody(node.Body, 0)
	v.traverseStmt(node.Body, 0)
	v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncDeclBody)
	v.emitter.PostVisitFuncDeclBody(node.Body, 0)
	v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncDecl)
	v.emitter.PostVisitFuncDecl(node, 0)
	return v
}

func (v *BasePassVisitor) buildTypesGraph() map[string][]string {
	typesGraph := make(map[string][]string)
	for _, node := range v.nodes {
		switch node := node.(type) {
		case *ast.TypeSpec:
			if st, ok := node.Type.(*ast.StructType); ok {
				structType := v.pkg.Name + "::" + node.Name.Name
				for _, field := range st.Fields.List {
					switch typ := field.Type.(type) {
					case *ast.Ident:
						if _, ok := primTypes[typ.Name]; !ok {
							fieldType := v.pkg.Name + "::" + typ.Name
							if fieldType != structType {
								typesGraph[fieldType] = append(typesGraph[fieldType], structType)
							}
						}
					case *ast.SelectorExpr: // External struct from another package
						if obj := v.pkg.TypesInfo.Uses[typ.Sel]; obj != nil {
							if named, ok := obj.Type().(*types.Named); ok {
								if _, ok := named.Underlying().(*types.Struct); ok {
									fieldType := named.Obj().Pkg().Name() + "::" + named.Obj().Name()
									if fieldType != structType {
										typesGraph[fieldType] = append(typesGraph[fieldType], structType)
									}
								}
							}
						}
					case *ast.ArrayType:
						switch elt := typ.Elt.(type) {
						case *ast.Ident:
							fieldType := v.pkg.Name + "::" + elt.Name
							if _, ok := primTypes[fieldType]; !ok {
								if fieldType != structType {
									typesGraph[fieldType] = append(typesGraph[fieldType], structType)
								}
							}
						case *ast.SelectorExpr: // Imported types
							if pkgIdent, ok := elt.X.(*ast.Ident); ok {
								fieldType := pkgIdent.Name + "::" + elt.Sel.Name
								if fieldType != structType {
									typesGraph[fieldType] = append(typesGraph[fieldType], structType)
								}
							}
						}
					}
				}
			} else if arrType, ok := node.Type.(*ast.ArrayType); ok {
				// Non-struct type alias (e.g., type AST []Statement)
				aliasType := v.pkg.Name + "::" + node.Name.Name
				switch elt := arrType.Elt.(type) {
				case *ast.Ident:
					if _, ok := primTypes[elt.Name]; !ok {
						elemType := v.pkg.Name + "::" + elt.Name
						if elemType != aliasType {
							typesGraph[elemType] = append(typesGraph[elemType], aliasType)
						}
					}
				case *ast.SelectorExpr:
					if pkgIdent, ok := elt.X.(*ast.Ident); ok {
						elemType := pkgIdent.Name + "::" + elt.Sel.Name
						if elemType != aliasType {
							typesGraph[elemType] = append(typesGraph[elemType], aliasType)
						}
					}
				}
			}
		}
	}
	return typesGraph
}

// GetBaseTypeNameFromSlices unwraps nested slices and returns the base type name.
func (v *BasePassVisitor) GetBaseTypeNameFromSlices(node *ast.TypeSpec) string {
	typ := v.pkg.TypesInfo.Types[node.Type].Type
	if typ == nil {
		return ""
	}

	// Unwrap nested slices
	for {
		slice, ok := typ.Underlying().(*types.Slice)
		if !ok {
			break
		}
		typ = slice.Elem()
	}

	// Return base type name
	switch t := typ.Underlying().(type) {
	case *types.Basic:
		return t.Name()
	case *types.Named:
		return t.Obj().Name()
	default:
		return typ.String()
	}
}

func (v *BasePassVisitor) gen(precedence map[string]int) {
	typeInfos := make([]GenTypeInfo, 0)
	for i := 0; i < len(v.nodes); i++ {
		switch node := v.nodes[i].(type) {
		case *ast.TypeSpec:
			if st, ok := node.Type.(*ast.StructType); ok {
				typeInfos = append(typeInfos, GenTypeInfo{
					Name:       node.Name.Name,
					Struct:     st, // We don't have type info for local structs yet
					IsExternal: false,
					Pkg:        v.pkg.Name,
					BaseType:   v.pkg.Name + "::" + node.Name.Name,
				})
			} else {
				typeInfos = append(typeInfos, GenTypeInfo{
					Name:       node.Name.Name,
					Struct:     nil, // We don't have type info for local structs yet
					Other:      node,
					IsExternal: false,
					Pkg:        v.pkg.Name,
					BaseType:   v.pkg.Name + "::" + node.Name.Name,
				})
			}

		}
	}
	// Sort structs based on the precedence map
	sort.Slice(typeInfos, func(i, j int) bool {
		// If the struct name is in the map, use its precedence value
		// Otherwise, treat it with the highest precedence (e.g., 0 or max int)
		precI := precedence[typeInfos[i].BaseType]
		precJ := precedence[typeInfos[j].BaseType]
		return precI < precJ
	})

	v.emitter.GetForestBuilder().AddVisitMarker(PreVisitGenStructInfos)
	v.emitter.PreVisitGenStructInfos(typeInfos, 0)
	for i := 0; i < len(typeInfos); i++ {
		if typeInfos[i].Struct != nil {
			v.emitter.GetForestBuilder().AddVisitMarker(PreVisitGenStructInfo)
			v.emitter.PreVisitGenStructInfo(typeInfos[i], 0)
			for _, field := range typeInfos[i].Struct.Fields.List {
				for _, fieldName := range field.Names {
					v.emitter.GetForestBuilder().AddVisitMarker(PreVisitGenStructFieldType)
					v.emitter.PreVisitGenStructFieldType(field.Type, 2)
					v.traverseExpression(field.Type, 2)
					v.emitter.GetForestBuilder().AddVisitMarker(PostVisitGenStructFieldType)
					v.emitter.PostVisitGenStructFieldType(field.Type, 2)
					v.emitter.GetForestBuilder().AddVisitMarker(PreVisitGenStructFieldName)
					v.emitter.PreVisitGenStructFieldName(fieldName, 0)
					v.traverseExpression(fieldName, 0)
					v.emitter.GetForestBuilder().AddVisitMarker(PostVisitGenStructFieldName)
					v.emitter.PostVisitGenStructFieldName(fieldName, 0)
				}
			}
			v.emitter.GetForestBuilder().AddVisitMarker(PostVisitGenStructInfo)
			v.emitter.PostVisitGenStructInfo(typeInfos[i], 0)
		} else if node, ok := typeInfos[i].Other.(*ast.TypeSpec); ok {
			if _, ok2 := typeInfos[i].Other.(*ast.StructType); !ok2 {
				v.emitter.GetForestBuilder().AddVisitMarker(PreVisitTypeAliasName)
				v.emitter.PreVisitTypeAliasName(node.Name, 0)
				v.traverseExpression(node.Name, 0)
				v.emitter.GetForestBuilder().AddVisitMarker(PostVisitTypeAliasName)
				v.emitter.PostVisitTypeAliasName(node.Name, 0)
				v.emitter.GetForestBuilder().AddVisitMarker(PreVisitTypeAliasType)
				v.emitter.PreVisitTypeAliasType(node.Type, 0)
				v.traverseExpression(node.Type, 0)
				v.emitter.GetForestBuilder().AddVisitMarker(PostVisitTypeAliasType)
				v.emitter.PostVisitTypeAliasType(node.Type, 0)
			}
		}
	}
	v.emitter.GetForestBuilder().AddVisitMarker(PostVisitGenStructInfos)
	v.emitter.PostVisitGenStructInfos(typeInfos, 0)
	for _, node := range v.nodes {
		if genDecl, ok := node.(*ast.GenDecl); ok && genDecl.Tok == token.CONST {
			v.emitter.GetForestBuilder().AddVisitMarker(PreVisitGenDeclConst)
			v.emitter.PreVisitGenDeclConst(genDecl, 0)
			for _, spec := range genDecl.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				for i, name := range valueSpec.Names {
					v.emitter.GetForestBuilder().AddVisitMarker(PreVisitGenDeclConstName)
					v.emitter.PreVisitGenDeclConstName(name, 0)
					if i < len(valueSpec.Values) {
						v.traverseExpression(valueSpec.Values[i], 0)
					}
					v.emitter.GetForestBuilder().AddVisitMarker(PostVisitGenDeclConstName)
					v.emitter.PostVisitGenDeclConstName(name, 0)
				}
			}
			v.emitter.GetForestBuilder().AddVisitMarker(PostVisitGenDeclConst)
			v.emitter.PostVisitGenDeclConst(genDecl, 0)
		}
	}

	v.emitter.GetForestBuilder().AddVisitMarker(PreVisitFuncDeclSignatures)
	v.emitter.PreVisitFuncDeclSignatures(0)
	for _, node := range v.nodes {
		switch node := node.(type) {
		case *ast.FuncDecl:
			v.generateFuncDeclSignature(node)
		}
	}
	v.emitter.GetForestBuilder().AddVisitMarker(PostVisitFuncDeclSignatures)
	v.emitter.PostVisitFuncDeclSignatures(0)
	for _, node := range v.nodes {
		switch node := node.(type) {
		case *ast.FuncDecl:
			v.generateFuncDecl(node)
		}
	}
}

func (v *BasePassVisitor) Visit(node ast.Node) ast.Visitor {
	v.nodes = append(v.nodes, node)
	return v
}

func (v *BasePass) ProLog() {
	namespaces = make(map[string]struct{})
	v.Emitter.GetForestBuilder().AddVisitMarker(PreVisitProgram)
	v.Emitter.PreVisitProgram(0)
	v.file = v.Emitter.GetFile()
}

func (v *BasePass) EpiLog() {
	v.Emitter.GetForestBuilder().AddVisitMarker(PostVisitProgram)
	v.Emitter.PostVisitProgram(0)
}

func (v *BasePass) PreVisit(visitor ast.Visitor) {
	cppVisitor := visitor.(*BasePassVisitor)
	namespaces[cppVisitor.pkg.Name] = struct{}{}
	// Add imported package names to namespaces
	for _, imp := range cppVisitor.pkg.Imports {
		namespaces[imp.Name] = struct{}{}
	}
	// Also scan AST imports to catch pseudo-packages like "runtime/graphics"
	for _, file := range cppVisitor.pkg.Syntax {
		for _, imp := range file.Imports {
			if imp.Path != nil {
				// Extract package name from import path (last component)
				path := imp.Path.Value
				// Remove quotes
				if len(path) > 2 {
					path = path[1 : len(path)-1]
				}
				// Get last component of path
				lastSlash := -1
				for i := len(path) - 1; i >= 0; i-- {
					if path[i] == '/' {
						lastSlash = i
						break
					}
				}
				var pkgName string
				if lastSlash >= 0 {
					pkgName = path[lastSlash+1:]
				} else {
					pkgName = path
				}
				namespaces[pkgName] = struct{}{}
			}
		}
	}
	v.Emitter.GetForestBuilder().AddVisitMarker(PreVisitPackage)
	v.Emitter.PreVisitPackage(cppVisitor.pkg, 0)
}

func (v *BasePassVisitor) complementPrecedenceMap(sortedTypes map[string]int) {
	for _, node := range v.nodes {
		switch node := node.(type) {
		case *ast.TypeSpec:
			if _, ok := node.Type.(*ast.StructType); ok {
				if _, exists := sortedTypes[v.pkg.Name+"::"+node.Name.Name]; !exists {
					sortedTypes[v.pkg.Name+"::"+node.Name.Name] = len(sortedTypes)
				}
			}
		}
	}
}

func (v *BasePass) PostVisit(visitor ast.Visitor, visited map[string]struct{}) {
	cppVisitor := visitor.(*BasePassVisitor)
	typesGraph := cppVisitor.buildTypesGraph()
	for name, val := range typesGraph {
		DebugPrintf("Type: %s Parent: %v\n", name, val)
	}
	typesTopoSorted, err := TopologicalSort(typesGraph)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	DebugPrintf("Types Topological Sort: %v\n", typesTopoSorted)
	typesPrecedence := SliceToMap(typesTopoSorted)
	cppVisitor.complementPrecedenceMap(typesPrecedence)
	for name, _ := range typesPrecedence {
		if !strings.HasPrefix(name, cppVisitor.pkg.Name) {
			delete(typesPrecedence, name)
		}
	}
	DebugPrintf("Types precedence: %v\n", typesPrecedence)
	cppVisitor.gen(typesPrecedence)
	v.Emitter.GetForestBuilder().AddVisitMarker(PostVisitPackage)
	v.Emitter.PostVisitPackage(cppVisitor.pkg, 0)
}
