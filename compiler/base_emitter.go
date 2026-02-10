package compiler

import (
	"go/ast"
	"go/token"
	"golang.org/x/tools/go/packages"
	"os"
)

// VisitMethod represents a visit method identifier
type VisitMethod string

// Visit method name constants
const (
	EmptyVisitMethod VisitMethod = ""
	PreVisitProgram VisitMethod = "PreVisitProgram"
	PostVisitProgram VisitMethod = "PostVisitProgram"
	PreVisitPackage VisitMethod = "PreVisitPackage"
	PostVisitPackage VisitMethod = "PostVisitPackage"
	PreVisitBasicLit VisitMethod = "PreVisitBasicLit"
	PostVisitBasicLit VisitMethod = "PostVisitBasicLit"
	PreVisitIdent VisitMethod = "PreVisitIdent"
	PostVisitIdent VisitMethod = "PostVisitIdent"
	PreVisitBinaryExpr VisitMethod = "PreVisitBinaryExpr"
	PostVisitBinaryExpr VisitMethod = "PostVisitBinaryExpr"
	PreVisitBinaryExprLeft VisitMethod = "PreVisitBinaryExprLeft"
	PostVisitBinaryExprLeft VisitMethod = "PostVisitBinaryExprLeft"
	PreVisitBinaryExprRight VisitMethod = "PreVisitBinaryExprRight"
	PostVisitBinaryExprRight VisitMethod = "PostVisitBinaryExprRight"
	PreVisitBinaryExprOperator VisitMethod = "PreVisitBinaryExprOperator"
	PostVisitBinaryExprOperator VisitMethod = "PostVisitBinaryExprOperator"
	PreVisitCallExpr VisitMethod = "PreVisitCallExpr"
	PostVisitCallExpr VisitMethod = "PostVisitCallExpr"
	PreVisitCallExprFun VisitMethod = "PreVisitCallExprFun"
	PostVisitCallExprFun VisitMethod = "PostVisitCallExprFun"
	PreVisitCallExprArgs VisitMethod = "PreVisitCallExprArgs"
	PostVisitCallExprArgs VisitMethod = "PostVisitCallExprArgs"
	PreVisitCallExprArg VisitMethod = "PreVisitCallExprArg"
	PostVisitCallExprArg VisitMethod = "PostVisitCallExprArg"
	PreVisitParenExpr VisitMethod = "PreVisitParenExpr"
	PostVisitParenExpr VisitMethod = "PostVisitParenExpr"
	PreVisitCompositeLit VisitMethod = "PreVisitCompositeLit"
	PostVisitCompositeLit VisitMethod = "PostVisitCompositeLit"
	PreVisitCompositeLitType VisitMethod = "PreVisitCompositeLitType"
	PostVisitCompositeLitType VisitMethod = "PostVisitCompositeLitType"
	PreVisitCompositeLitElts VisitMethod = "PreVisitCompositeLitElts"
	PostVisitCompositeLitElts VisitMethod = "PostVisitCompositeLitElts"
	PreVisitCompositeLitElt VisitMethod = "PreVisitCompositeLitElt"
	PostVisitCompositeLitElt VisitMethod = "PostVisitCompositeLitElt"
	PreVisitArrayType VisitMethod = "PreVisitArrayType"
	PostVisitArrayType VisitMethod = "PostVisitArrayType"
	PreVisitMapType VisitMethod = "PreVisitMapType"
	PostVisitMapType VisitMethod = "PostVisitMapType"
	PreVisitMapKeyType VisitMethod = "PreVisitMapKeyType"
	PostVisitMapKeyType VisitMethod = "PostVisitMapKeyType"
	PreVisitMapValueType VisitMethod = "PreVisitMapValueType"
	PostVisitMapValueType VisitMethod = "PostVisitMapValueType"
	PreVisitChanType VisitMethod = "PreVisitChanType"
	PostVisitChanType VisitMethod = "PostVisitChanType"
	PreVisitEllipsis VisitMethod = "PreVisitEllipsis"
	PostVisitEllipsis VisitMethod = "PostVisitEllipsis"
	PreVisitSelectorExpr VisitMethod = "PreVisitSelectorExpr"
	PostVisitSelectorExpr VisitMethod = "PostVisitSelectorExpr"
	PreVisitSelectorExprX VisitMethod = "PreVisitSelectorExprX"
	PostVisitSelectorExprX VisitMethod = "PostVisitSelectorExprX"
	PreVisitSelectorExprSel VisitMethod = "PreVisitSelectorExprSel"
	PostVisitSelectorExprSel VisitMethod = "PostVisitSelectorExprSel"
	PreVisitIndexExpr VisitMethod = "PreVisitIndexExpr"
	PostVisitIndexExpr VisitMethod = "PostVisitIndexExpr"
	PreVisitIndexExprX VisitMethod = "PreVisitIndexExprX"
	PostVisitIndexExprX VisitMethod = "PostVisitIndexExprX"
	PreVisitIndexExprIndex VisitMethod = "PreVisitIndexExprIndex"
	PostVisitIndexExprIndex VisitMethod = "PostVisitIndexExprIndex"
	PreVisitUnaryExpr VisitMethod = "PreVisitUnaryExpr"
	PostVisitUnaryExpr VisitMethod = "PostVisitUnaryExpr"
	PreVisitSliceExpr VisitMethod = "PreVisitSliceExpr"
	PostVisitSliceExpr VisitMethod = "PostVisitSliceExpr"
	PreVisitSliceExprX VisitMethod = "PreVisitSliceExprX"
	PostVisitSliceExprX VisitMethod = "PostVisitSliceExprX"
	PreVisitSliceExprXBegin VisitMethod = "PreVisitSliceExprXBegin"
	PostVisitSliceExprXBegin VisitMethod = "PostVisitSliceExprXBegin"
	PreVisitSliceExprXEnd VisitMethod = "PreVisitSliceExprXEnd"
	PostVisitSliceExprXEnd VisitMethod = "PostVisitSliceExprXEnd"
	PreVisitSliceExprLow VisitMethod = "PreVisitSliceExprLow"
	PostVisitSliceExprLow VisitMethod = "PostVisitSliceExprLow"
	PreVisitSliceExprHigh VisitMethod = "PreVisitSliceExprHigh"
	PostVisitSliceExprHigh VisitMethod = "PostVisitSliceExprHigh"
	PreVisitFuncType VisitMethod = "PreVisitFuncType"
	PostVisitFuncType VisitMethod = "PostVisitFuncType"
	PreVisitFuncTypeResults VisitMethod = "PreVisitFuncTypeResults"
	PostVisitFuncTypeResults VisitMethod = "PostVisitFuncTypeResults"
	PreVisitFuncTypeResult VisitMethod = "PreVisitFuncTypeResult"
	PostVisitFuncTypeResult VisitMethod = "PostVisitFuncTypeResult"
	PreVisitFuncTypeParams VisitMethod = "PreVisitFuncTypeParams"
	PostVisitFuncTypeParams VisitMethod = "PostVisitFuncTypeParams"
	PreVisitFuncTypeParam VisitMethod = "PreVisitFuncTypeParam"
	PostVisitFuncTypeParam VisitMethod = "PostVisitFuncTypeParam"
	PreVisitKeyValueExpr VisitMethod = "PreVisitKeyValueExpr"
	PostVisitKeyValueExpr VisitMethod = "PostVisitKeyValueExpr"
	PreVisitKeyValueExprKey VisitMethod = "PreVisitKeyValueExprKey"
	PostVisitKeyValueExprKey VisitMethod = "PostVisitKeyValueExprKey"
	PreVisitKeyValueExprValue VisitMethod = "PreVisitKeyValueExprValue"
	PostVisitKeyValueExprValue VisitMethod = "PostVisitKeyValueExprValue"
	PreVisitFuncLit VisitMethod = "PreVisitFuncLit"
	PostVisitFuncLit VisitMethod = "PostVisitFuncLit"
	PreVisitFuncLitTypeParams VisitMethod = "PreVisitFuncLitTypeParams"
	PostVisitFuncLitTypeParams VisitMethod = "PostVisitFuncLitTypeParams"
	PreVisitFuncLitTypeParam VisitMethod = "PreVisitFuncLitTypeParam"
	PostVisitFuncLitTypeParam VisitMethod = "PostVisitFuncLitTypeParam"
	PreVisitFuncLitBody VisitMethod = "PreVisitFuncLitBody"
	PostVisitFuncLitBody VisitMethod = "PostVisitFuncLitBody"
	PreVisitFuncLitTypeResults VisitMethod = "PreVisitFuncLitTypeResults"
	PostVisitFuncLitTypeResults VisitMethod = "PostVisitFuncLitTypeResults"
	PreVisitFuncLitTypeResult VisitMethod = "PreVisitFuncLitTypeResult"
	PostVisitFuncLitTypeResult VisitMethod = "PostVisitFuncLitTypeResult"
	PreVisitTypeAssertExpr VisitMethod = "PreVisitTypeAssertExpr"
	PostVisitTypeAssertExpr VisitMethod = "PostVisitTypeAssertExpr"
	PreVisitTypeAssertExprX VisitMethod = "PreVisitTypeAssertExprX"
	PostVisitTypeAssertExprX VisitMethod = "PostVisitTypeAssertExprX"
	PreVisitTypeAssertExprType VisitMethod = "PreVisitTypeAssertExprType"
	PostVisitTypeAssertExprType VisitMethod = "PostVisitTypeAssertExprType"
	PreVisitStarExpr VisitMethod = "PreVisitStarExpr"
	PostVisitStarExpr VisitMethod = "PostVisitStarExpr"
	PreVisitStarExprX VisitMethod = "PreVisitStarExprX"
	PostVisitStarExprX VisitMethod = "PostVisitStarExprX"
	PreVisitInterfaceType VisitMethod = "PreVisitInterfaceType"
	PostVisitInterfaceType VisitMethod = "PostVisitInterfaceType"
	PreVisitStructType VisitMethod = "PreVisitStructType"
	PostVisitStructType VisitMethod = "PostVisitStructType"
	PreVisitTypeSwitchStmt VisitMethod = "PreVisitTypeSwitchStmt"
	PostVisitTypeSwitchStmt VisitMethod = "PostVisitTypeSwitchStmt"
	PreVisitExprStmt VisitMethod = "PreVisitExprStmt"
	PostVisitExprStmt VisitMethod = "PostVisitExprStmt"
	PreVisitExprStmtX VisitMethod = "PreVisitExprStmtX"
	PostVisitExprStmtX VisitMethod = "PostVisitExprStmtX"
	PreVisitDeclStmt VisitMethod = "PreVisitDeclStmt"
	PostVisitDeclStmt VisitMethod = "PostVisitDeclStmt"
	PreVisitDeclStmtValueSpecType VisitMethod = "PreVisitDeclStmtValueSpecType"
	PostVisitDeclStmtValueSpecType VisitMethod = "PostVisitDeclStmtValueSpecType"
	PreVisitDeclStmtValueSpecNames VisitMethod = "PreVisitDeclStmtValueSpecNames"
	PostVisitDeclStmtValueSpecNames VisitMethod = "PostVisitDeclStmtValueSpecNames"
	PreVisitDeclStmtValueSpecValue VisitMethod = "PreVisitDeclStmtValueSpecValue"
	PostVisitDeclStmtValueSpecValue VisitMethod = "PostVisitDeclStmtValueSpecValue"
	PreVisitBranchStmt VisitMethod = "PreVisitBranchStmt"
	PostVisitBranchStmt VisitMethod = "PostVisitBranchStmt"
	PreVisitDeferStmt VisitMethod = "PreVisitDeferStmt"
	PostVisitDeferStmt VisitMethod = "PostVisitDeferStmt"
	PreVisitGoStmt VisitMethod = "PreVisitGoStmt"
	PostVisitGoStmt VisitMethod = "PostVisitGoStmt"
	PreVisitSendStmt VisitMethod = "PreVisitSendStmt"
	PostVisitSendStmt VisitMethod = "PostVisitSendStmt"
	PreVisitSelectStmt VisitMethod = "PreVisitSelectStmt"
	PostVisitSelectStmt VisitMethod = "PostVisitSelectStmt"
	PreVisitIncDecStmt VisitMethod = "PreVisitIncDecStmt"
	PostVisitIncDecStmt VisitMethod = "PostVisitIncDecStmt"
	PreVisitAssignStmt VisitMethod = "PreVisitAssignStmt"
	PostVisitAssignStmt VisitMethod = "PostVisitAssignStmt"
	PostVisitForStmt VisitMethod = "PostVisitForStmt"
	PreVisitForStmt VisitMethod = "PreVisitForStmt"
	PreVisitForStmtInit VisitMethod = "PreVisitForStmtInit"
	PostVisitForStmtInit VisitMethod = "PostVisitForStmtInit"
	PreVisitForStmtCond VisitMethod = "PreVisitForStmtCond"
	PostVisitForStmtCond VisitMethod = "PostVisitForStmtCond"
	PreVisitForStmtPost VisitMethod = "PreVisitForStmtPost"
	PostVisitForStmtPost VisitMethod = "PostVisitForStmtPost"
	PreVisitAssignStmtLhs VisitMethod = "PreVisitAssignStmtLhs"
	PostVisitAssignStmtLhs VisitMethod = "PostVisitAssignStmtLhs"
	PreVisitAssignStmtRhs VisitMethod = "PreVisitAssignStmtRhs"
	PostVisitAssignStmtRhs VisitMethod = "PostVisitAssignStmtRhs"
	PreVisitAssignStmtLhsExpr VisitMethod = "PreVisitAssignStmtLhsExpr"
	PostVisitAssignStmtLhsExpr VisitMethod = "PostVisitAssignStmtLhsExpr"
	PreVisitAssignStmtRhsExpr VisitMethod = "PreVisitAssignStmtRhsExpr"
	PostVisitAssignStmtRhsExpr VisitMethod = "PostVisitAssignStmtRhsExpr"
	PreVisitReturnStmt VisitMethod = "PreVisitReturnStmt"
	PostVisitReturnStmt VisitMethod = "PostVisitReturnStmt"
	PreVisitReturnStmtResult VisitMethod = "PreVisitReturnStmtResult"
	PostVisitReturnStmtResult VisitMethod = "PostVisitReturnStmtResult"
	PreVisitIfStmt VisitMethod = "PreVisitIfStmt"
	PostVisitIfStmt VisitMethod = "PostVisitIfStmt"
	PreVisitIfStmtInit VisitMethod = "PreVisitIfStmtInit"
	PostVisitIfStmtInit VisitMethod = "PostVisitIfStmtInit"
	PreVisitIfStmtCond VisitMethod = "PreVisitIfStmtCond"
	PostVisitIfStmtCond VisitMethod = "PostVisitIfStmtCond"
	PreVisitIfStmtBody VisitMethod = "PreVisitIfStmtBody"
	PostVisitIfStmtBody VisitMethod = "PostVisitIfStmtBody"
	PreVisitIfStmtElse VisitMethod = "PreVisitIfStmtElse"
	PostVisitIfStmtElse VisitMethod = "PostVisitIfStmtElse"
	PreVisitRangeStmt VisitMethod = "PreVisitRangeStmt"
	PostVisitRangeStmt VisitMethod = "PostVisitRangeStmt"
	PreVisitRangeStmtKey VisitMethod = "PreVisitRangeStmtKey"
	PostVisitRangeStmtKey VisitMethod = "PostVisitRangeStmtKey"
	PreVisitRangeStmtValue VisitMethod = "PreVisitRangeStmtValue"
	PostVisitRangeStmtValue VisitMethod = "PostVisitRangeStmtValue"
	PreVisitRangeStmtX VisitMethod = "PreVisitRangeStmtX"
	PostVisitRangeStmtX VisitMethod = "PostVisitRangeStmtX"
	PreVisitSwitchStmt VisitMethod = "PreVisitSwitchStmt"
	PostVisitSwitchStmt VisitMethod = "PostVisitSwitchStmt"
	PreVisitSwitchStmtTag VisitMethod = "PreVisitSwitchStmtTag"
	PostVisitSwitchStmtTag VisitMethod = "PostVisitSwitchStmtTag"
	PreVisitCaseClause VisitMethod = "PreVisitCaseClause"
	PostVisitCaseClause VisitMethod = "PostVisitCaseClause"
	PreVisitCaseClauseList VisitMethod = "PreVisitCaseClauseList"
	PostVisitCaseClauseList VisitMethod = "PostVisitCaseClauseList"
	PreVisitCaseClauseListExpr VisitMethod = "PreVisitCaseClauseListExpr"
	PostVisitCaseClauseListExpr VisitMethod = "PostVisitCaseClauseListExpr"
	PreVisitBlockStmt VisitMethod = "PreVisitBlockStmt"
	PostVisitBlockStmt VisitMethod = "PostVisitBlockStmt"
	PreVisitBlockStmtList VisitMethod = "PreVisitBlockStmtList"
	PostVisitBlockStmtList VisitMethod = "PostVisitBlockStmtList"
	PreVisitFuncDecl VisitMethod = "PreVisitFuncDecl"
	PostVisitFuncDecl VisitMethod = "PostVisitFuncDecl"
	PreVisitFuncDeclBody VisitMethod = "PreVisitFuncDeclBody"
	PostVisitFuncDeclBody VisitMethod = "PostVisitFuncDeclBody"
	PreVisitFuncDeclSignature VisitMethod = "PreVisitFuncDeclSignature"
	PostVisitFuncDeclSignature VisitMethod = "PostVisitFuncDeclSignature"
	PreVisitFuncDeclName VisitMethod = "PreVisitFuncDeclName"
	PostVisitFuncDeclName VisitMethod = "PostVisitFuncDeclName"
	PreVisitFuncDeclSignatureTypeResults VisitMethod = "PreVisitFuncDeclSignatureTypeResults"
	PostVisitFuncDeclSignatureTypeResults VisitMethod = "PostVisitFuncDeclSignatureTypeResults"
	PreVisitFuncDeclSignatureTypeResultsList VisitMethod = "PreVisitFuncDeclSignatureTypeResultsList"
	PostVisitFuncDeclSignatureTypeResultsList VisitMethod = "PostVisitFuncDeclSignatureTypeResultsList"
	PreVisitFuncDeclSignatureTypeParamsList VisitMethod = "PreVisitFuncDeclSignatureTypeParamsList"
	PostVisitFuncDeclSignatureTypeParamsList VisitMethod = "PostVisitFuncDeclSignatureTypeParamsList"
	PreVisitFuncDeclSignatureTypeParamsListType VisitMethod = "PreVisitFuncDeclSignatureTypeParamsListType"
	PostVisitFuncDeclSignatureTypeParamsListType VisitMethod = "PostVisitFuncDeclSignatureTypeParamsListType"
	PreVisitFuncDeclSignatureTypeParamsArgName VisitMethod = "PreVisitFuncDeclSignatureTypeParamsArgName"
	PostVisitFuncDeclSignatureTypeParamsArgName VisitMethod = "PostVisitFuncDeclSignatureTypeParamsArgName"
	PreVisitFuncDeclSignatureTypeParams VisitMethod = "PreVisitFuncDeclSignatureTypeParams"
	PostVisitFuncDeclSignatureTypeParams VisitMethod = "PostVisitFuncDeclSignatureTypeParams"
	PreVisitGenStructInfos VisitMethod = "PreVisitGenStructInfos"
	PostVisitGenStructInfos VisitMethod = "PostVisitGenStructInfos"
	PreVisitGenStructInfo VisitMethod = "PreVisitGenStructInfo"
	PostVisitGenStructInfo VisitMethod = "PostVisitGenStructInfo"
	PreVisitFuncDeclSignatures VisitMethod = "PreVisitFuncDeclSignatures"
	PostVisitFuncDeclSignatures VisitMethod = "PostVisitFuncDeclSignatures"
	PreVisitGenDeclConst VisitMethod = "PreVisitGenDeclConst"
	PostVisitGenDeclConst VisitMethod = "PostVisitGenDeclConst"
	PreVisitGenStructFieldType VisitMethod = "PreVisitGenStructFieldType"
	PostVisitGenStructFieldType VisitMethod = "PostVisitGenStructFieldType"
	PreVisitGenStructFieldName VisitMethod = "PreVisitGenStructFieldName"
	PostVisitGenStructFieldName VisitMethod = "PostVisitGenStructFieldName"
	PreVisitGenDeclConstName VisitMethod = "PreVisitGenDeclConstName"
	PostVisitGenDeclConstName VisitMethod = "PostVisitGenDeclConstName"
	PreVisitTypeAliasName VisitMethod = "PreVisitTypeAliasName"
	PostVisitTypeAliasName VisitMethod = "PostVisitTypeAliasName"
	PreVisitTypeAliasType VisitMethod = "PreVisitTypeAliasType"
	PostVisitTypeAliasType VisitMethod = "PostVisitTypeAliasType"
)

type BaseEmitter struct{
	gir GoFIR
}

func (v *BaseEmitter) SetFile(file *os.File) {}
func (v *BaseEmitter) GetFile() *os.File { return nil }
func (v *BaseEmitter) GetGoFIR() *GoFIR { return &v.gir }
func (v *BaseEmitter) PreVisitProgram(indent int) {}
func (v *BaseEmitter) PostVisitProgram(indent int) {}
func (v *BaseEmitter) PreVisitPackage(pkg *packages.Package, indent int) {}
func (v *BaseEmitter) PostVisitPackage(pkg *packages.Package, indent int) {}
func (v *BaseEmitter) PreVisitBasicLit(node *ast.BasicLit, indent int) {}
func (v *BaseEmitter) PostVisitBasicLit(node *ast.BasicLit, indent int) {}
func (v *BaseEmitter) PreVisitIdent(node *ast.Ident, indent int) {}
func (v *BaseEmitter) PostVisitIdent(node *ast.Ident, indent int) {}
func (v *BaseEmitter) PreVisitBinaryExpr(node *ast.BinaryExpr, indent int) {}
func (v *BaseEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {}
func (v *BaseEmitter) PreVisitBinaryExprLeft(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitBinaryExprLeft(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitBinaryExprRight(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitBinaryExprRight(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitBinaryExprOperator(op token.Token, indent int) {}
func (v *BaseEmitter) PostVisitBinaryExprOperator(op token.Token, indent int) {}
func (v *BaseEmitter) PreVisitCallExpr(node *ast.CallExpr, indent int) {}
func (v *BaseEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {}
func (v *BaseEmitter) PreVisitCallExprFun(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitCallExprArgs(node []ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitCallExprArg(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PreVisitParenExpr(node *ast.ParenExpr, indent int) {}
func (v *BaseEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {}
func (v *BaseEmitter) PreVisitCompositeLit(node *ast.CompositeLit, indent int) {}
func (v *BaseEmitter) PostVisitCompositeLit(node *ast.CompositeLit, indent int) {}
func (v *BaseEmitter) PreVisitCompositeLitType(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitCompositeLitElts(node []ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitCompositeLitElt(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PostVisitCompositeLitElt(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PreVisitArrayType(node ast.ArrayType, indent int) {}
func (v *BaseEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {}
func (v *BaseEmitter) PreVisitMapType(node *ast.MapType, indent int) {}
func (v *BaseEmitter) PostVisitMapType(node *ast.MapType, indent int) {}
func (v *BaseEmitter) PreVisitMapKeyType(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitMapValueType(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitMapValueType(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitChanType(node *ast.ChanType, indent int) {}
func (v *BaseEmitter) PostVisitChanType(node *ast.ChanType, indent int) {}
func (v *BaseEmitter) PreVisitEllipsis(node *ast.Ellipsis, indent int) {}
func (v *BaseEmitter) PostVisitEllipsis(node *ast.Ellipsis, indent int) {}
func (v *BaseEmitter) PreVisitSelectorExpr(node *ast.SelectorExpr, indent int) {}
func (v *BaseEmitter) PostVisitSelectorExpr(node *ast.SelectorExpr, indent int) {}
func (v *BaseEmitter) PreVisitSelectorExprX(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitSelectorExprSel(node *ast.Ident, indent int) {}
func (v *BaseEmitter) PostVisitSelectorExprSel(node *ast.Ident, indent int) {}
func (v *BaseEmitter) PreVisitIndexExpr(node *ast.IndexExpr, indent int) {}
func (v *BaseEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {}
func (v *BaseEmitter) PreVisitIndexExprX(node *ast.IndexExpr, indent int) {}
func (v *BaseEmitter) PostVisitIndexExprX(node *ast.IndexExpr, indent int) {}
func (v *BaseEmitter) PreVisitIndexExprIndex(node *ast.IndexExpr, indent int) {}
func (v *BaseEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {}
func (v *BaseEmitter) PreVisitUnaryExpr(node *ast.UnaryExpr, indent int) {}
func (v *BaseEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {}
func (v *BaseEmitter) PreVisitSliceExpr(node *ast.SliceExpr, indent int) {}
func (v *BaseEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {}
func (v *BaseEmitter) PreVisitSliceExprX(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitSliceExprXBegin(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitSliceExprXEnd(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitSliceExprLow(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitSliceExprHigh(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitSliceExprHigh(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitFuncType(node *ast.FuncType, indent int) {}
func (v *BaseEmitter) PostVisitFuncType(node *ast.FuncType, indent int) {}
func (v *BaseEmitter) PreVisitFuncTypeResults(node *ast.FieldList, indent int) {}
func (v *BaseEmitter) PostVisitFuncTypeResults(node *ast.FieldList, indent int) {}
func (v *BaseEmitter) PreVisitFuncTypeResult(node *ast.Field, index int, indent int) {}
func (v *BaseEmitter) PostVisitFuncTypeResult(node *ast.Field, index int, indent int) {}
func (v *BaseEmitter) PreVisitFuncTypeParams(node *ast.FieldList, indent int) {}
func (v *BaseEmitter) PostVisitFuncTypeParams(node *ast.FieldList, indent int) {}
func (v *BaseEmitter) PreVisitFuncTypeParam(node *ast.Field, index int, indent int) {}
func (v *BaseEmitter) PostVisitFuncTypeParam(node *ast.Field, index int, indent int) {}
func (v *BaseEmitter) PreVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {}
func (v *BaseEmitter) PostVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {}
func (v *BaseEmitter) PreVisitKeyValueExprKey(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitKeyValueExprKey(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitKeyValueExprValue(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitKeyValueExprValue(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitFuncLit(node *ast.FuncLit, indent int) {}
func (v *BaseEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {}
func (v *BaseEmitter) PreVisitFuncLitTypeParams(node *ast.FieldList, indent int) {}
func (v *BaseEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {}
func (v *BaseEmitter) PreVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {}
func (v *BaseEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {}
func (v *BaseEmitter) PreVisitFuncLitBody(node *ast.BlockStmt, indent int) {}
func (v *BaseEmitter) PostVisitFuncLitBody(node *ast.BlockStmt, indent int) {}
func (v *BaseEmitter) PreVisitFuncLitTypeResults(node *ast.FieldList, indent int) {}
func (v *BaseEmitter) PostVisitFuncLitTypeResults(node *ast.FieldList, indent int) {}
func (v *BaseEmitter) PreVisitFuncLitTypeResult(node *ast.Field, index int, indent int) {}
func (v *BaseEmitter) PostVisitFuncLitTypeResult(node *ast.Field, index int, indent int) {}
func (v *BaseEmitter) PreVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {}
func (v *BaseEmitter) PostVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {}
func (v *BaseEmitter) PreVisitTypeAssertExprX(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitTypeAssertExprType(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitStarExpr(node *ast.StarExpr, indent int) {}
func (v *BaseEmitter) PostVisitStarExpr(node *ast.StarExpr, indent int) {}
func (v *BaseEmitter) PreVisitStarExprX(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitStarExprX(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitInterfaceType(node *ast.InterfaceType, indent int) {}
func (v *BaseEmitter) PostVisitInterfaceType(node *ast.InterfaceType, indent int) {}
func (v *BaseEmitter) PreVisitStructType(node *ast.StructType, indent int) {}
func (v *BaseEmitter) PostVisitStructType(node *ast.StructType, indent int) {}
func (v *BaseEmitter) PreVisitTypeSwitchStmt(node *ast.TypeSwitchStmt, indent int) {}
func (v *BaseEmitter) PostVisitTypeSwitchStmt(node *ast.TypeSwitchStmt, indent int) {}
func (v *BaseEmitter) PreVisitExprStmt(node *ast.ExprStmt, indent int) {}
func (v *BaseEmitter) PostVisitExprStmt(node *ast.ExprStmt, indent int) {}
func (v *BaseEmitter) PreVisitExprStmtX(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitDeclStmt(node *ast.DeclStmt, indent int) {}
func (v *BaseEmitter) PostVisitDeclStmt(node *ast.DeclStmt, indent int) {}
func (v *BaseEmitter) PreVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {}
func (v *BaseEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {}
func (v *BaseEmitter) PreVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {}
func (v *BaseEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {}
func (v *BaseEmitter) PreVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PostVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {}
func (v *BaseEmitter) PostVisitBranchStmt(node *ast.BranchStmt, indent int) {}
func (v *BaseEmitter) PreVisitDeferStmt(node *ast.DeferStmt, indent int) {}
func (v *BaseEmitter) PostVisitDeferStmt(node *ast.DeferStmt, indent int) {}
func (v *BaseEmitter) PreVisitGoStmt(node *ast.GoStmt, indent int) {}
func (v *BaseEmitter) PostVisitGoStmt(node *ast.GoStmt, indent int) {}
func (v *BaseEmitter) PreVisitSendStmt(node *ast.SendStmt, indent int) {}
func (v *BaseEmitter) PostVisitSendStmt(node *ast.SendStmt, indent int) {}
func (v *BaseEmitter) PreVisitSelectStmt(node *ast.SelectStmt, indent int) {}
func (v *BaseEmitter) PostVisitSelectStmt(node *ast.SelectStmt, indent int) {}
func (v *BaseEmitter) PreVisitIncDecStmt(node *ast.IncDecStmt, indent int) {}
func (v *BaseEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {}
func (v *BaseEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {}
func (v *BaseEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {}
func (v *BaseEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {}
func (v *BaseEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {}
func (v *BaseEmitter) PreVisitForStmtInit(node ast.Stmt, indent int) {}
func (v *BaseEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {}
func (v *BaseEmitter) PreVisitForStmtCond(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitForStmtPost(node ast.Stmt, indent int) {}
func (v *BaseEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {}
func (v *BaseEmitter) PreVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {}
func (v *BaseEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {}
func (v *BaseEmitter) PreVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {}
func (v *BaseEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {}
func (v *BaseEmitter) PreVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PostVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PreVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {}
func (v *BaseEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {}
func (v *BaseEmitter) PreVisitReturnStmtResult(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PostVisitReturnStmtResult(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {}
func (v *BaseEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {}
func (v *BaseEmitter) PreVisitIfStmtInit(node ast.Stmt, indent int) {}
func (v *BaseEmitter) PostVisitIfStmtInit(node ast.Stmt, indent int) {}
func (v *BaseEmitter) PreVisitIfStmtCond(node *ast.IfStmt, indent int) {}
func (v *BaseEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {}
func (v *BaseEmitter) PreVisitIfStmtBody(node *ast.IfStmt, indent int) {}
func (v *BaseEmitter) PostVisitIfStmtBody(node *ast.IfStmt, indent int) {}
func (v *BaseEmitter) PreVisitIfStmtElse(node *ast.IfStmt, indent int) {}
func (v *BaseEmitter) PostVisitIfStmtElse(node *ast.IfStmt, indent int) {}
func (v *BaseEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {}
func (v *BaseEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {}
func (v *BaseEmitter) PreVisitRangeStmtKey(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitRangeStmtValue(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitRangeStmtX(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {}
func (v *BaseEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {}
func (v *BaseEmitter) PreVisitSwitchStmtTag(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitCaseClause(node *ast.CaseClause, indent int) {}
func (v *BaseEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {}
func (v *BaseEmitter) PreVisitCaseClauseList(node []ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {}
func (v *BaseEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {}
func (v *BaseEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {}
func (v *BaseEmitter) PreVisitBlockStmtList(node ast.Stmt, index int, indent int) {}
func (v *BaseEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {}
func (v *BaseEmitter) PreVisitFuncDecl(node *ast.FuncDecl, indent int) {}
func (v *BaseEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {}
func (v *BaseEmitter) PreVisitFuncDeclBody(node *ast.BlockStmt, indent int) {}
func (v *BaseEmitter) PostVisitFuncDeclBody(node *ast.BlockStmt, indent int) {}
func (v *BaseEmitter) PreVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {}
func (v *BaseEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {}
func (v *BaseEmitter) PreVisitFuncDeclName(node *ast.Ident, indent int) {}
func (v *BaseEmitter) PostVisitFuncDeclName(node *ast.Ident, indent int) {}
func (v *BaseEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {}
func (v *BaseEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {}
func (v *BaseEmitter) PreVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {}
func (v *BaseEmitter) PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {}
func (v *BaseEmitter) PreVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {}
func (v *BaseEmitter) PostVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {}
func (v *BaseEmitter) PreVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {}
func (v *BaseEmitter) PostVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {}
func (v *BaseEmitter) PreVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {}
func (v *BaseEmitter) PostVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {}
func (v *BaseEmitter) PreVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {}
func (v *BaseEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {}
func (v *BaseEmitter) PreVisitGenStructInfos(node []GenTypeInfo, indent int) {}
func (v *BaseEmitter) PostVisitGenStructInfos(node []GenTypeInfo, indent int) {}
func (v *BaseEmitter) PreVisitGenStructInfo(node GenTypeInfo, indent int) {}
func (v *BaseEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {}
func (v *BaseEmitter) PreVisitFuncDeclSignatures(indent int) {}
func (v *BaseEmitter) PostVisitFuncDeclSignatures(indent int) {}
func (v *BaseEmitter) PreVisitGenDeclConst(node *ast.GenDecl, indent int) {}
func (v *BaseEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {}
func (v *BaseEmitter) PreVisitGenStructFieldType(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {}
func (v *BaseEmitter) PreVisitGenStructFieldName(node *ast.Ident, indent int) {}
func (v *BaseEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {}
func (v *BaseEmitter) PreVisitGenDeclConstName(node *ast.Ident, indent int) {}
func (v *BaseEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {}
func (v *BaseEmitter) PreVisitTypeAliasName(node *ast.Ident, indent int) {}
func (v *BaseEmitter) PostVisitTypeAliasName(node *ast.Ident, indent int) {}
func (v *BaseEmitter) PreVisitTypeAliasType(node ast.Expr, indent int) {}
func (v *BaseEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {}
