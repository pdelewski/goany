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
	v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitCallExprArgs)
	v.emitter.PreVisitCallExprArgs(node.Args, indent)
	for i, arg := range node.Args {
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitCallExprArg)
		v.emitter.PreVisitCallExprArg(arg, i, indent)
		v.traverseExpression(arg, 0) // Function arguments
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitCallExprArg)
		v.emitter.PostVisitCallExprArg(arg, i, indent)
	}
	v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitCallExprArgs)
	v.emitter.PostVisitCallExprArgs(node.Args, indent)
}

func (v *BasePassVisitor) traverseExpression(expr ast.Expr, indent int) string {
	var str string
	switch e := expr.(type) {
	case *ast.BasicLit:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitBasicLit)
		v.emitter.PreVisitBasicLit(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitBasicLit)
		v.emitter.PostVisitBasicLit(e, indent)
	case *ast.Ident:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitIdent)
		v.emitter.PreVisitIdent(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitIdent)
		v.emitter.PostVisitIdent(e, indent)
	case *ast.BinaryExpr:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitBinaryExpr)
		v.emitter.PreVisitBinaryExpr(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitBinaryExprLeft)
		v.emitter.PreVisitBinaryExprLeft(e.X, indent)
		v.traverseExpression(e.X, indent) // Left operand
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitBinaryExprLeft)
		v.emitter.PostVisitBinaryExprLeft(e.X, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitBinaryExprOperator)
		v.emitter.PreVisitBinaryExprOperator(e.Op, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitBinaryExprOperator)
		v.emitter.PostVisitBinaryExprOperator(e.Op, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitBinaryExprRight)
		v.emitter.PreVisitBinaryExprRight(e.Y, indent)
		v.traverseExpression(e.Y, indent) // Right operand
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitBinaryExprRight)
		v.emitter.PostVisitBinaryExprRight(e.Y, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitBinaryExpr)
		v.emitter.PostVisitBinaryExpr(e, indent)
	case *ast.CallExpr:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitCallExpr)
		v.emitter.PreVisitCallExpr(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitCallExprFun)
		v.emitter.PreVisitCallExprFun(e.Fun, indent)
		v.traverseExpression(e.Fun, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitCallExprFun)
		v.emitter.PostVisitCallExprFun(e.Fun, indent)
		v.emitArgs(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitCallExpr)
		v.emitter.PostVisitCallExpr(e, indent)
	case *ast.ParenExpr:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitParenExpr)
		v.emitter.PreVisitParenExpr(e, indent)
		v.traverseExpression(e.X, indent) // Dump inner expression
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitParenExpr)
		v.emitter.PostVisitParenExpr(e, indent)
	case *ast.CompositeLit:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitCompositeLit)
		v.emitter.PreVisitCompositeLit(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitCompositeLitType)
		v.emitter.PreVisitCompositeLitType(e.Type, indent)
		v.traverseExpression(e.Type, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitCompositeLitType)
		v.emitter.PostVisitCompositeLitType(e.Type, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitCompositeLitElts)
		v.emitter.PreVisitCompositeLitElts(e.Elts, indent)
		for i, elt := range e.Elts {
			v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitCompositeLitElt)
			v.emitter.PreVisitCompositeLitElt(elt, i, indent)
			v.traverseExpression(elt, 0) // Function arguments
			v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitCompositeLitElt)
			v.emitter.PostVisitCompositeLitElt(elt, i, indent)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitCompositeLitElts)
		v.emitter.PostVisitCompositeLitElts(e.Elts, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitCompositeLit)
		v.emitter.PostVisitCompositeLit(e, indent)
	case *ast.ArrayType:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitArrayType)
		v.emitter.PreVisitArrayType(*e, indent)
		v.traverseExpression(e.Elt, 0)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitArrayType)
		v.emitter.PostVisitArrayType(*e, indent)
	case *ast.SelectorExpr:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitSelectorExpr)
		v.emitter.PreVisitSelectorExpr(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitSelectorExprX)
		v.emitter.PreVisitSelectorExprX(e.X, indent)
		v.traverseExpression(e.X, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitSelectorExprX)
		v.emitter.PostVisitSelectorExprX(e.X, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitSelectorExprSel)
		v.emitter.PreVisitSelectorExprSel(e.Sel, indent)
		oldIndent := indent
		v.traverseExpression(e.Sel, 0)
		indent = oldIndent
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitSelectorExprSel)
		v.emitter.PostVisitSelectorExprSel(e.Sel, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitSelectorExpr)
		v.emitter.PostVisitSelectorExpr(e, indent)
	case *ast.IndexExpr:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitIndexExpr)
		v.emitter.PreVisitIndexExpr(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitIndexExprX)
		v.emitter.PreVisitIndexExprX(e, indent)
		v.traverseExpression(e.X, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitIndexExprX)
		v.emitter.PostVisitIndexExprX(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitIndexExprIndex)
		v.emitter.PreVisitIndexExprIndex(e, indent)
		v.traverseExpression(e.Index, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitIndexExprIndex)
		v.emitter.PostVisitIndexExprIndex(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitIndexExpr)
		v.emitter.PostVisitIndexExpr(e, indent)
	case *ast.UnaryExpr:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitUnaryExpr)
		v.emitter.PreVisitUnaryExpr(e, indent)
		v.traverseExpression(e.X, 0)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitUnaryExpr)
		v.emitter.PostVisitUnaryExpr(e, indent)
	case *ast.SliceExpr:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitSliceExpr)
		v.emitter.PreVisitSliceExpr(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitSliceExprX)
		v.emitter.PreVisitSliceExprX(e.X, indent)
		v.traverseExpression(e.X, 0)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitSliceExprX)
		v.emitter.PostVisitSliceExprX(e.X, indent)
		// Check and print Low, High, and Max
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitSliceExprXBegin)
		v.emitter.PreVisitSliceExprXBegin(e.X, indent)
		v.traverseExpression(e.X, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitSliceExprXBegin)
		v.emitter.PostVisitSliceExprXBegin(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitSliceExprLow)
		v.emitter.PreVisitSliceExprLow(e.Low, indent)
		if e.Low != nil {
			v.traverseExpression(e.Low, indent)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitSliceExprLow)
		v.emitter.PostVisitSliceExprLow(e.Low, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitSliceExprXEnd)
		v.emitter.PreVisitSliceExprXEnd(e, indent)
		v.traverseExpression(e.X, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitSliceExprXEnd)
		v.emitter.PostVisitSliceExprXEnd(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitSliceExprHigh)
		v.emitter.PreVisitSliceExprHigh(e.High, indent)
		if e.High != nil {
			v.traverseExpression(e.High, indent)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitSliceExprHigh)
		v.emitter.PostVisitSliceExprHigh(e.High, indent)
		if e.Slice3 && e.Max != nil {
			v.traverseExpression(e.Max, indent)
		} else if e.Slice3 {
			DebugLogPrintf("Max index: <nil>\n")
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitSliceExpr)
		v.emitter.PostVisitSliceExpr(e, indent)
	case *ast.FuncType:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncType)
		v.emitter.PreVisitFuncType(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncTypeResults)
		v.emitter.PreVisitFuncTypeResults(e.Results, indent)
		if e.Results != nil {
			for i, result := range e.Results.List {
				v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncTypeResult)
				v.emitter.PreVisitFuncTypeResult(result, i, indent)
				v.traverseExpression(result.Type, indent)
				v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncTypeResult)
				v.emitter.PostVisitFuncTypeResult(result, i, indent)
			}
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncTypeResults)
		v.emitter.PostVisitFuncTypeResults(e.Results, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncTypeParams)
		v.emitter.PreVisitFuncTypeParams(e.Params, indent)
		for i, param := range e.Params.List {
			v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncTypeParam)
			v.emitter.PreVisitFuncTypeParam(param, i, indent)
			v.traverseExpression(param.Type, 0)
			v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncTypeParam)
			v.emitter.PostVisitFuncTypeParam(param, i, indent)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncTypeParams)
		v.emitter.PostVisitFuncTypeParams(e.Params, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncType)
		v.emitter.PostVisitFuncType(e, indent)
	case *ast.KeyValueExpr:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitKeyValueExpr)
		v.emitter.PreVisitKeyValueExpr(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitKeyValueExprKey)
		v.emitter.PreVisitKeyValueExprKey(e.Key, indent)
		v.traverseExpression(e.Key, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitKeyValueExprKey)
		v.emitter.PostVisitKeyValueExprKey(e.Key, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitKeyValueExprValue)
		v.emitter.PreVisitKeyValueExprValue(e.Value, indent)
		v.traverseExpression(e.Value, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitKeyValueExprValue)
		v.emitter.PostVisitKeyValueExprValue(e.Value, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitKeyValueExpr)
		v.emitter.PostVisitKeyValueExpr(e, indent)
	case *ast.FuncLit:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncLit)
		v.emitter.PreVisitFuncLit(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncLitTypeParams)
		v.emitter.PreVisitFuncLitTypeParams(e.Type.Params, indent)
		for i, param := range e.Type.Params.List {
			v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncLitTypeParam)
			v.emitter.PreVisitFuncLitTypeParam(param, i, indent)
			v.traverseExpression(param.Type, indent)
			v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncLitTypeParam)
			v.emitter.PostVisitFuncLitTypeParam(param, i, indent)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncLitTypeParams)
		v.emitter.PostVisitFuncLitTypeParams(e.Type.Params, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncLitTypeResults)
		v.emitter.PreVisitFuncLitTypeResults(e.Type.Results, indent)

		if e.Type.Results != nil {
			for i, result := range e.Type.Results.List {
				v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncLitTypeResult)
				v.emitter.PreVisitFuncLitTypeResult(result, i, indent)
				v.traverseExpression(result.Type, indent)
				v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncLitTypeResult)
				v.emitter.PostVisitFuncLitTypeResult(result, i, indent)
			}
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncLitTypeResults)
		v.emitter.PostVisitFuncLitTypeResults(e.Type.Results, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncLitBody)
		v.emitter.PreVisitFuncLitBody(e.Body, indent)
		v.traverseStmt(e.Body, indent+4)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncLitBody)
		v.emitter.PostVisitFuncLitBody(e.Body, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncLit)
		v.emitter.PostVisitFuncLit(e, indent)
	case *ast.TypeAssertExpr:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitTypeAssertExpr)
		v.emitter.PreVisitTypeAssertExpr(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitTypeAssertExprType)
		v.emitter.PreVisitTypeAssertExprType(e.Type, indent)
		v.traverseExpression(e.Type, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitTypeAssertExprType)
		v.emitter.PostVisitTypeAssertExprType(e.Type, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitTypeAssertExprX)
		v.emitter.PreVisitTypeAssertExprX(e.X, indent)
		v.traverseExpression(e.X, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitTypeAssertExprX)
		v.emitter.PostVisitTypeAssertExprX(e.X, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitTypeAssertExpr)
		v.emitter.PostVisitTypeAssertExpr(e, indent)
	case *ast.StarExpr:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitStarExpr)
		v.emitter.PreVisitStarExpr(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitStarExprX)
		v.emitter.PreVisitStarExprX(e.X, indent)
		v.traverseExpression(e.X, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitStarExprX)
		v.emitter.PostVisitStarExprX(e.X, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitStarExpr)
		v.emitter.PostVisitStarExpr(e, indent)
	case *ast.InterfaceType:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitInterfaceType)
		v.emitter.PreVisitInterfaceType(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitInterfaceType)
		v.emitter.PostVisitInterfaceType(e, indent)
	case *ast.StructType:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitStructType)
		v.emitter.PreVisitStructType(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitStructType)
		v.emitter.PostVisitStructType(e, indent)
	case *ast.MapType:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitMapType)
		v.emitter.PreVisitMapType(e, indent)
		// Traverse key and value types to check nested type patterns
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitMapKeyType)
		v.emitter.PreVisitMapKeyType(e.Key, indent)
		v.traverseExpression(e.Key, 0)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitMapKeyType)
		v.emitter.PostVisitMapKeyType(e.Key, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitMapValueType)
		v.emitter.PreVisitMapValueType(e.Value, indent)
		v.traverseExpression(e.Value, 0)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitMapValueType)
		v.emitter.PostVisitMapValueType(e.Value, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitMapType)
		v.emitter.PostVisitMapType(e, indent)
	case *ast.ChanType:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitChanType)
		v.emitter.PreVisitChanType(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitChanType)
		v.emitter.PostVisitChanType(e, indent)
	case *ast.Ellipsis:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitEllipsis)
		v.emitter.PreVisitEllipsis(e, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitEllipsis)
		v.emitter.PostVisitEllipsis(e, indent)
	default:
		panic(fmt.Sprintf("unsupported expression type: %T", e))
	}
	return str
}

func (v *BasePassVisitor) traverseAssignment(assignStmt *ast.AssignStmt, indent int) {
	v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitAssignStmtLhs)
	v.emitter.PreVisitAssignStmtLhs(assignStmt, indent)
	for i := 0; i < len(assignStmt.Lhs); i++ {
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitAssignStmtLhsExpr)
		v.emitter.PreVisitAssignStmtLhsExpr(assignStmt.Lhs[i], i, indent)
		v.traverseExpression(assignStmt.Lhs[i], indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitAssignStmtLhsExpr)
		v.emitter.PostVisitAssignStmtLhsExpr(assignStmt.Lhs[i], i, indent)
	}
	v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitAssignStmtLhs)
	v.emitter.PostVisitAssignStmtLhs(assignStmt, indent)

	v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitAssignStmtRhs)
	v.emitter.PreVisitAssignStmtRhs(assignStmt, indent)
	for i := 0; i < len(assignStmt.Rhs); i++ {
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitAssignStmtRhsExpr)
		v.emitter.PreVisitAssignStmtRhsExpr(assignStmt.Rhs[i], i, indent)
		v.traverseExpression(assignStmt.Rhs[i], indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitAssignStmtRhsExpr)
		v.emitter.PostVisitAssignStmtRhsExpr(assignStmt.Rhs[i], i, indent)
	}
	v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitAssignStmtRhs)
	v.emitter.PostVisitAssignStmtRhs(assignStmt, indent)
}

func (v *BasePassVisitor) traverseStmt(stmt ast.Stmt, indent int) {
	switch stmt := stmt.(type) {
	case *ast.ExprStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitExprStmt)
		v.emitter.PreVisitExprStmt(stmt, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitExprStmtX)
		v.emitter.PreVisitExprStmtX(stmt.X, indent)
		v.traverseExpression(stmt.X, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitExprStmtX)
		v.emitter.PostVisitExprStmtX(stmt.X, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitExprStmt)
		v.emitter.PostVisitExprStmt(stmt, indent)
	case *ast.DeclStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitDeclStmt)
		v.emitter.PreVisitDeclStmt(stmt, indent)
		if genDecl, ok := stmt.Decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			for _, spec := range genDecl.Specs {
				if valueSpec, ok := spec.(*ast.ValueSpec); ok {
					// Iterate through all variables declared
					for i := 0; i < len(valueSpec.Names); i++ {
						v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitDeclStmtValueSpecType)
						v.emitter.PreVisitDeclStmtValueSpecType(valueSpec, i, indent)
						v.traverseExpression(valueSpec.Type, indent)
						v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitDeclStmtValueSpecType)
						v.emitter.PostVisitDeclStmtValueSpecType(valueSpec, i, indent)
						v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitDeclStmtValueSpecNames)
						v.emitter.PreVisitDeclStmtValueSpecNames(valueSpec.Names[i], i, indent)
						v.traverseExpression(valueSpec.Names[i], 0)
						v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitDeclStmtValueSpecNames)
						v.emitter.PostVisitDeclStmtValueSpecNames(valueSpec.Names[i], i, indent)
						// Traverse initialization value if present
						if i < len(valueSpec.Values) {
							v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitDeclStmtValueSpecValue)
							v.emitter.PreVisitDeclStmtValueSpecValue(valueSpec.Values[i], i, indent)
							v.traverseExpression(valueSpec.Values[i], indent)
							v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitDeclStmtValueSpecValue)
							v.emitter.PostVisitDeclStmtValueSpecValue(valueSpec.Values[i], i, indent)
						}
					}
				}
			}
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitDeclStmt)
		v.emitter.PostVisitDeclStmt(stmt, indent)
	case *ast.AssignStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitAssignStmt)
		v.emitter.PreVisitAssignStmt(stmt, indent)
		v.traverseAssignment(stmt, 0)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitAssignStmt)
		v.emitter.PostVisitAssignStmt(stmt, indent)
	case *ast.ReturnStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitReturnStmt)
		v.emitter.PreVisitReturnStmt(stmt, indent)
		for i := 0; i < len(stmt.Results); i++ {
			v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitReturnStmtResult)
			v.emitter.PreVisitReturnStmtResult(stmt.Results[i], i, indent)
			v.traverseExpression(stmt.Results[i], 0)
			v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitReturnStmtResult)
			v.emitter.PostVisitReturnStmtResult(stmt.Results[i], i, indent)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitReturnStmt)
		v.emitter.PostVisitReturnStmt(stmt, indent)
	case *ast.IfStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitIfStmt)
		v.emitter.PreVisitIfStmt(stmt, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitIfStmtInit)
		v.emitter.PreVisitIfStmtInit(stmt.Init, indent)
		if stmt.Init != nil {
			v.traverseStmt(stmt.Init, indent)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitIfStmtInit)
		v.emitter.PostVisitIfStmtInit(stmt.Init, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitIfStmtCond)
		v.emitter.PreVisitIfStmtCond(stmt, indent)
		v.traverseExpression(stmt.Cond, 0)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitIfStmtCond)
		v.emitter.PostVisitIfStmtCond(stmt, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitIfStmtBody)
		v.emitter.PreVisitIfStmtBody(stmt, indent)
		v.traverseStmt(stmt.Body, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitIfStmtBody)
		v.emitter.PostVisitIfStmtBody(stmt, indent)
		if stmt.Else != nil {
			v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitIfStmtElse)
			v.emitter.PreVisitIfStmtElse(stmt, indent)
			v.traverseStmt(stmt.Else, indent)
			v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitIfStmtElse)
			v.emitter.PostVisitIfStmtElse(stmt, indent)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitIfStmt)
		v.emitter.PostVisitIfStmt(stmt, indent)
	case *ast.ForStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitForStmt)
		v.emitter.PreVisitForStmt(stmt, indent)

		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitForStmtInit)
		v.emitter.PreVisitForStmtInit(stmt.Init, indent)
		if stmt.Init != nil {
			v.traverseStmt(stmt.Init, 0)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitForStmtInit)
		v.emitter.PostVisitForStmtInit(stmt.Init, indent)

		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitForStmtCond)
		v.emitter.PreVisitForStmtCond(stmt.Cond, indent)
		if stmt.Cond != nil {
			v.traverseExpression(stmt.Cond, 0)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitForStmtCond)
		v.emitter.PostVisitForStmtCond(stmt.Cond, indent)

		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitForStmtPost)
		v.emitter.PreVisitForStmtPost(stmt.Post, indent)
		if stmt.Post != nil {
			v.traverseStmt(stmt.Post, 0)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitForStmtPost)
		v.emitter.PostVisitForStmtPost(stmt.Post, indent)

		v.traverseStmt(stmt.Body, indent)

		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitForStmt)
		v.emitter.PostVisitForStmt(stmt, indent)
	case *ast.RangeStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitRangeStmt)
		v.emitter.PreVisitRangeStmt(stmt, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitRangeStmtKey)
		v.emitter.PreVisitRangeStmtKey(stmt.Key, indent)
		if stmt.Key != nil {
			v.traverseExpression(stmt.Key, 0)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitRangeStmtKey)
		v.emitter.PostVisitRangeStmtKey(stmt.Key, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitRangeStmtValue)
		v.emitter.PreVisitRangeStmtValue(stmt.Value, indent)
		if stmt.Value != nil {
			v.traverseExpression(stmt.Value, 0)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitRangeStmtValue)
		v.emitter.PostVisitRangeStmtValue(stmt.Value, indent)

		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitRangeStmtX)
		v.emitter.PreVisitRangeStmtX(stmt.X, indent)
		v.traverseExpression(stmt.X, 0)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitRangeStmtX)
		v.emitter.PostVisitRangeStmtX(stmt.X, indent)

		v.traverseStmt(stmt.Body, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitRangeStmt)
		v.emitter.PostVisitRangeStmt(stmt, indent)
	case *ast.SwitchStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitSwitchStmt)
		v.emitter.PreVisitSwitchStmt(stmt, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitSwitchStmtTag)
		v.emitter.PreVisitSwitchStmtTag(stmt.Tag, indent)
		v.traverseExpression(stmt.Tag, 0)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitSwitchStmtTag)
		v.emitter.PostVisitSwitchStmtTag(stmt.Tag, indent)

		for _, stmt := range stmt.Body.List {
			v.traverseStmt(stmt, indent+2)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitSwitchStmt)
		v.emitter.PostVisitSwitchStmt(stmt, indent)
	case *ast.TypeSwitchStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitTypeSwitchStmt)
		v.emitter.PreVisitTypeSwitchStmt(stmt, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitTypeSwitchStmt)
		v.emitter.PostVisitTypeSwitchStmt(stmt, indent)
	case *ast.BranchStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitBranchStmt)
		v.emitter.PreVisitBranchStmt(stmt, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitBranchStmt)
		v.emitter.PostVisitBranchStmt(stmt, indent)
	case *ast.IncDecStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitIncDecStmt)
		v.emitter.PreVisitIncDecStmt(stmt, indent)
		v.traverseExpression(stmt.X, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitIncDecStmt)
		v.emitter.PostVisitIncDecStmt(stmt, indent)
	case *ast.CaseClause:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitCaseClause)
		v.emitter.PreVisitCaseClause(stmt, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitCaseClauseList)
		v.emitter.PreVisitCaseClauseList(stmt.List, indent)
		for i := 0; i < len(stmt.List); i++ {
			v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitCaseClauseListExpr)
			v.emitter.PreVisitCaseClauseListExpr(stmt.List[i], i, indent)
			v.traverseExpression(stmt.List[i], 0)
			v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitCaseClauseListExpr)
			v.emitter.PostVisitCaseClauseListExpr(stmt.List[i], i, indent)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitCaseClauseList)
		v.emitter.PostVisitCaseClauseList(stmt.List, indent)
		for i := 0; i < len(stmt.Body); i++ {
			v.traverseStmt(stmt.Body[i], indent+4)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitCaseClause)
		v.emitter.PostVisitCaseClause(stmt, indent)
	case *ast.BlockStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitBlockStmt)
		v.emitter.PreVisitBlockStmt(stmt, indent)
		for i := 0; i < len(stmt.List); i++ {
			v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitBlockStmtList)
			v.emitter.PreVisitBlockStmtList(stmt.List[i], i, indent+2)
			v.traverseStmt(stmt.List[i], indent+2)
			v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitBlockStmtList)
			v.emitter.PostVisitBlockStmtList(stmt.List[i], i, indent+2)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitBlockStmt)
		v.emitter.PostVisitBlockStmt(stmt, indent)
	case *ast.DeferStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitDeferStmt)
		v.emitter.PreVisitDeferStmt(stmt, indent)
		v.traverseExpression(stmt.Call, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitDeferStmt)
		v.emitter.PostVisitDeferStmt(stmt, indent)
	case *ast.GoStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitGoStmt)
		v.emitter.PreVisitGoStmt(stmt, indent)
		v.traverseExpression(stmt.Call, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitGoStmt)
		v.emitter.PostVisitGoStmt(stmt, indent)
	case *ast.SendStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitSendStmt)
		v.emitter.PreVisitSendStmt(stmt, indent)
		v.traverseExpression(stmt.Chan, indent)
		v.traverseExpression(stmt.Value, indent)
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitSendStmt)
		v.emitter.PostVisitSendStmt(stmt, indent)
	case *ast.SelectStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitSelectStmt)
		v.emitter.PreVisitSelectStmt(stmt, indent)
		if stmt.Body != nil {
			v.traverseStmt(stmt.Body, indent)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitSelectStmt)
		v.emitter.PostVisitSelectStmt(stmt, indent)
	case *ast.LabeledStmt:
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitLabeledStmt)
		v.emitter.PreVisitLabeledStmt(stmt, indent)
		if stmt.Stmt != nil {
			v.traverseStmt(stmt.Stmt, indent)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitLabeledStmt)
		v.emitter.PostVisitLabeledStmt(stmt, indent)
	default:
		DebugPrintf("<Other statement type>\n")
	}
}

func (v *BasePassVisitor) generateFuncDeclSignature(node *ast.FuncDecl) ast.Visitor {
	v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncDeclSignature)
	v.emitter.PreVisitFuncDeclSignature(node, 0)
	v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncDeclSignatureTypeResults)
	v.emitter.PreVisitFuncDeclSignatureTypeResults(node, 0)

	if node.Type.Results != nil {
		for i := 0; i < len(node.Type.Results.List); i++ {
			v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncDeclSignatureTypeResultsList)
			v.emitter.PreVisitFuncDeclSignatureTypeResultsList(node.Type.Results.List[i], i, 0)
			v.traverseExpression(node.Type.Results.List[i].Type, 0)
			v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncDeclSignatureTypeResultsList)
			v.emitter.PostVisitFuncDeclSignatureTypeResultsList(node.Type.Results.List[i], i, 0)
		}
	}

	v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncDeclSignatureTypeResults)
	v.emitter.PostVisitFuncDeclSignatureTypeResults(node, 0)

	v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncDeclName)
	v.emitter.PreVisitFuncDeclName(node.Name, 0)
	v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncDeclName)
	v.emitter.PostVisitFuncDeclName(node.Name, 0)

	v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncDeclSignatureTypeParams)
	v.emitter.PreVisitFuncDeclSignatureTypeParams(node, 0)
	for i := 0; i < len(node.Type.Params.List); i++ {
		v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncDeclSignatureTypeParamsList)
		v.emitter.PreVisitFuncDeclSignatureTypeParamsList(node.Type.Params.List[i], i, 0)
		for j := 0; j < len(node.Type.Params.List[i].Names); j++ {
			argName := node.Type.Params.List[i].Names[j]
			v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncDeclSignatureTypeParamsListType)
			v.emitter.PreVisitFuncDeclSignatureTypeParamsListType(node.Type.Params.List[i].Type, argName, j, 0)
			v.traverseExpression(node.Type.Params.List[i].Type, 0)
			v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncDeclSignatureTypeParamsListType)
			v.emitter.PostVisitFuncDeclSignatureTypeParamsListType(node.Type.Params.List[i].Type, argName, j, 0)
			v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncDeclSignatureTypeParamsArgName)
			v.emitter.PreVisitFuncDeclSignatureTypeParamsArgName(argName, j, 0)
			v.traverseExpression(argName, 0)
			v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncDeclSignatureTypeParamsArgName)
			v.emitter.PostVisitFuncDeclSignatureTypeParamsArgName(argName, j, 0)
		}
		v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncDeclSignatureTypeParamsList)
		v.emitter.PostVisitFuncDeclSignatureTypeParamsList(node.Type.Params.List[i], i, 0)
	}
	v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncDeclSignatureTypeParams)
	v.emitter.PostVisitFuncDeclSignatureTypeParams(node, 0)
	v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncDeclSignature)
	v.emitter.PostVisitFuncDeclSignature(node, 0)
	return v
}

func (v *BasePassVisitor) generateFuncDecl(node *ast.FuncDecl) ast.Visitor {
	v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncDecl)
	v.emitter.PreVisitFuncDecl(node, 0)
	v.generateFuncDeclSignature(node)
	v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncDeclBody)
	v.emitter.PreVisitFuncDeclBody(node.Body, 0)
	v.traverseStmt(node.Body, 0)
	v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncDeclBody)
	v.emitter.PostVisitFuncDeclBody(node.Body, 0)
	v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncDecl)
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
					BaseType:   v.pkg.Name + "::" + trimBeforeChar(v.GetBaseTypeNameFromSlices(node), '.'),
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

	v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitGenStructInfos)
	v.emitter.PreVisitGenStructInfos(typeInfos, 0)
	for i := 0; i < len(typeInfos); i++ {
		if typeInfos[i].Struct != nil {
			v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitGenStructInfo)
			v.emitter.PreVisitGenStructInfo(typeInfos[i], 0)
			for _, field := range typeInfos[i].Struct.Fields.List {
				for _, fieldName := range field.Names {
					v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitGenStructFieldType)
					v.emitter.PreVisitGenStructFieldType(field.Type, 2)
					v.traverseExpression(field.Type, 2)
					v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitGenStructFieldType)
					v.emitter.PostVisitGenStructFieldType(field.Type, 2)
					v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitGenStructFieldName)
					v.emitter.PreVisitGenStructFieldName(fieldName, 0)
					v.traverseExpression(fieldName, 0)
					v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitGenStructFieldName)
					v.emitter.PostVisitGenStructFieldName(fieldName, 0)
				}
			}
			v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitGenStructInfo)
			v.emitter.PostVisitGenStructInfo(typeInfos[i], 0)
		} else if node, ok := typeInfos[i].Other.(*ast.TypeSpec); ok {
			if _, ok2 := typeInfos[i].Other.(*ast.StructType); !ok2 {
				v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitTypeAliasName)
				v.emitter.PreVisitTypeAliasName(node.Name, 0)
				v.traverseExpression(node.Name, 0)
				v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitTypeAliasName)
				v.emitter.PostVisitTypeAliasName(node.Name, 0)
				v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitTypeAliasType)
				v.emitter.PreVisitTypeAliasType(node.Type, 0)
				v.traverseExpression(node.Type, 0)
				v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitTypeAliasType)
				v.emitter.PostVisitTypeAliasType(node.Type, 0)
			}
		}
	}
	v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitGenStructInfos)
	v.emitter.PostVisitGenStructInfos(typeInfos, 0)
	for _, node := range v.nodes {
		if genDecl, ok := node.(*ast.GenDecl); ok && genDecl.Tok == token.CONST {
			v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitGenDeclConst)
			v.emitter.PreVisitGenDeclConst(genDecl, 0)
			for _, spec := range genDecl.Specs {
				valueSpec := spec.(*ast.ValueSpec)
				for i, name := range valueSpec.Names {
					v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitGenDeclConstName)
					v.emitter.PreVisitGenDeclConstName(name, 0)
					if i < len(valueSpec.Values) {
						v.traverseExpression(valueSpec.Values[i], 0)
					}
					v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitGenDeclConstName)
					v.emitter.PostVisitGenDeclConstName(name, 0)
				}
			}
			v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitGenDeclConst)
			v.emitter.PostVisitGenDeclConst(genDecl, 0)
		}
	}

	v.emitter.GetGoFIR().emitToFileBuffer("", PreVisitFuncDeclSignatures)
	v.emitter.PreVisitFuncDeclSignatures(0)
	for _, node := range v.nodes {
		switch node := node.(type) {
		case *ast.FuncDecl:
			v.generateFuncDeclSignature(node)
		}
	}
	v.emitter.GetGoFIR().emitToFileBuffer("", PostVisitFuncDeclSignatures)
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
	v.Emitter.GetGoFIR().emitToFileBuffer("", PreVisitProgram)
	v.Emitter.PreVisitProgram(0)
	v.file = v.Emitter.GetFile()
}

func (v *BasePass) EpiLog() {
	v.Emitter.GetGoFIR().emitToFileBuffer("", PostVisitProgram)
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
	v.Emitter.GetGoFIR().emitToFileBuffer("", PreVisitPackage)
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
	v.Emitter.GetGoFIR().emitToFileBuffer("", PostVisitPackage)
	v.Emitter.PostVisitPackage(cppVisitor.pkg, 0)
}
