//go:generate go run ../cmd/generate_base_emitter.go

// Package main provides the core functionality for the goany compiler.
package compiler

import (
	"go/ast"
	"go/token"
	"golang.org/x/tools/go/packages"
	"os"
)

// Emitter defines the interface for code generation in the goany compiler.
// It provides a comprehensive set of methods for traversing and emitting code
// for different types of AST nodes. The interface is designed to support
// multiple output languages (e.g., C++, C#) while maintaining a consistent
// traversal pattern.
//
// The interface methods are organized into pre-visit and post-visit pairs
// for each AST node type, allowing for precise control over code generation
// before and after processing each node.
//
// Key features:
// - File handling: SetFile and GetFile methods for managing output files
// - Program structure: Methods for handling program and package boundaries
// - Expression handling: Comprehensive support for all Go expression types
// - Statement handling: Support for various statement types including control flow
// - Type handling: Methods for processing type declarations and assertions
type Emitter interface {
	// SetFile sets the output file for code generation.
	SetFile(file *os.File)
	// GetFile returns the current output file.
	GetFile() *os.File
	// GetGoFIR returns the GoFIR instance for emitting to file buffer.
	GetGoFIR() *GoFIR

	// PreVisitProgram is called before visiting the program node.
	PreVisitProgram(indent int)
	// PostVisitProgram is called after visiting the program node.
	PostVisitProgram(indent int)
	// PreVisitPackage is called before visiting a package node.
	PreVisitPackage(pkg *packages.Package, indent int)
	// PostVisitPackage is called after visiting a package node.
	PostVisitPackage(pkg *packages.Package, indent int)

	// PreVisitBasicLit is called before visiting a basic literal (e.g., string, number).
	PreVisitBasicLit(node *ast.BasicLit, indent int)
	// PostVisitBasicLit is called after visiting a basic literal.
	PostVisitBasicLit(node *ast.BasicLit, indent int)
	// PreVisitIdent is called before visiting an identifier.
	PreVisitIdent(node *ast.Ident, indent int)
	// PostVisitIdent is called after visiting an identifier.
	PostVisitIdent(node *ast.Ident, indent int)

	// PreVisitBinaryExpr is called before visiting a binary expression.
	PreVisitBinaryExpr(node *ast.BinaryExpr, indent int)
	// PostVisitBinaryExpr is called after visiting a binary expression.
	PostVisitBinaryExpr(node *ast.BinaryExpr, indent int)
	// PreVisitBinaryExprLeft is called before visiting the left operand of a binary expression.
	PreVisitBinaryExprLeft(node ast.Expr, indent int)
	// PostVisitBinaryExprLeft is called after visiting the left operand of a binary expression.
	PostVisitBinaryExprLeft(node ast.Expr, indent int)
	// PreVisitBinaryExprRight is called before visiting the right operand of a binary expression.
	PreVisitBinaryExprRight(node ast.Expr, indent int)
	// PostVisitBinaryExprRight is called after visiting the right operand of a binary expression.
	PostVisitBinaryExprRight(node ast.Expr, indent int)
	// PreVisitBinaryExprOperator is called before visiting the operator of a binary expression.
	PreVisitBinaryExprOperator(op token.Token, indent int)
	// PostVisitBinaryExprOperator is called after visiting the operator of a binary expression.
	PostVisitBinaryExprOperator(op token.Token, indent int)

	// PreVisitCallExpr is called before visiting a function call expression.
	PreVisitCallExpr(node *ast.CallExpr, indent int)
	// PostVisitCallExpr is called after visiting a function call expression.
	PostVisitCallExpr(node *ast.CallExpr, indent int)
	// PreVisitCallExprFun is called before visiting the function part of a call expression.
	PreVisitCallExprFun(node ast.Expr, indent int)
	// PostVisitCallExprFun is called after visiting the function part of a call expression.
	PostVisitCallExprFun(node ast.Expr, indent int)
	// PreVisitCallExprArgs is called before visiting the arguments of a function call.
	PreVisitCallExprArgs(node []ast.Expr, indent int)
	// PostVisitCallExprArgs is called after visiting the arguments of a function call.
	PostVisitCallExprArgs(node []ast.Expr, indent int)
	// PreVisitCallExprArg is called before visiting a specific argument of a function call.
	PreVisitCallExprArg(node ast.Expr, index int, indent int)
	// PostVisitCallExprArg is called after visiting a specific argument of a function call.
	PostVisitCallExprArg(node ast.Expr, index int, indent int)

	// PreVisitParenExpr is called before visiting a parenthesized expression.
	PreVisitParenExpr(node *ast.ParenExpr, indent int)
	// PostVisitParenExpr is called after visiting a parenthesized expression.
	PostVisitParenExpr(node *ast.ParenExpr, indent int)

	// PreVisitCompositeLit is called before visiting a composite literal.
	PreVisitCompositeLit(node *ast.CompositeLit, indent int)
	// PostVisitCompositeLit is called after visiting a composite literal.
	PostVisitCompositeLit(node *ast.CompositeLit, indent int)
	// PreVisitCompositeLitType is called before visiting the type of a composite literal.
	PreVisitCompositeLitType(node ast.Expr, indent int)
	// PostVisitCompositeLitType is called after visiting the type of a composite literal.
	PostVisitCompositeLitType(node ast.Expr, indent int)
	// PreVisitCompositeLitElts is called before visiting the elements of a composite literal.
	PreVisitCompositeLitElts(node []ast.Expr, indent int)
	// PostVisitCompositeLitElts is called after visiting the elements of a composite literal.
	PostVisitCompositeLitElts(node []ast.Expr, indent int)
	// PreVisitCompositeLitElt is called before visiting a specific element of a composite literal.
	PreVisitCompositeLitElt(node ast.Expr, index int, indent int)
	// PostVisitCompositeLitElt is called after visiting a specific element of a composite literal.
	PostVisitCompositeLitElt(node ast.Expr, index int, indent int)

	// PreVisitArrayType is called before visiting an array type.
	PreVisitArrayType(node ast.ArrayType, indent int)
	// PostVisitArrayType is called after visiting an array type.
	PostVisitArrayType(node ast.ArrayType, indent int)

	// PreVisitMapType is called before visiting a map type.
	PreVisitMapType(node *ast.MapType, indent int)
	// PostVisitMapType is called after visiting a map type.
	PostVisitMapType(node *ast.MapType, indent int)

	// PreVisitMapKeyType is called before visiting a map's key type.
	PreVisitMapKeyType(node ast.Expr, indent int)
	// PostVisitMapKeyType is called after visiting a map's key type.
	PostVisitMapKeyType(node ast.Expr, indent int)
	// PreVisitMapValueType is called before visiting a map's value type.
	PreVisitMapValueType(node ast.Expr, indent int)
	// PostVisitMapValueType is called after visiting a map's value type.
	PostVisitMapValueType(node ast.Expr, indent int)

	// PreVisitChanType is called before visiting a channel type.
	PreVisitChanType(node *ast.ChanType, indent int)
	// PostVisitChanType is called after visiting a channel type.
	PostVisitChanType(node *ast.ChanType, indent int)

	// PreVisitEllipsis is called before visiting an ellipsis (variadic).
	PreVisitEllipsis(node *ast.Ellipsis, indent int)
	// PostVisitEllipsis is called after visiting an ellipsis (variadic).
	PostVisitEllipsis(node *ast.Ellipsis, indent int)

	// PreVisitSelectorExpr is called before visiting a selector expression.
	PreVisitSelectorExpr(node *ast.SelectorExpr, indent int)
	// PostVisitSelectorExpr is called after visiting a selector expression.
	PostVisitSelectorExpr(node *ast.SelectorExpr, indent int)
	// PreVisitSelectorExprX is called before visiting the X part of a selector expression.
	PreVisitSelectorExprX(node ast.Expr, indent int)
	// PostVisitSelectorExprX is called after visiting the X part of a selector expression.
	PostVisitSelectorExprX(node ast.Expr, indent int)
	// PreVisitSelectorExprSel is called before visiting the selector part of a selector expression.
	PreVisitSelectorExprSel(node *ast.Ident, indent int)
	// PostVisitSelectorExprSel is called after visiting the selector part of a selector expression.
	PostVisitSelectorExprSel(node *ast.Ident, indent int)

	// PreVisitIndexExpr is called before visiting an index expression.
	PreVisitIndexExpr(node *ast.IndexExpr, indent int)
	// PostVisitIndexExpr is called after visiting an index expression.
	PostVisitIndexExpr(node *ast.IndexExpr, indent int)
	// PreVisitIndexExprX is called before visiting the X part of an index expression.
	PreVisitIndexExprX(node *ast.IndexExpr, indent int)
	// PostVisitIndexExprX is called after visiting the X part of an index expression.
	PostVisitIndexExprX(node *ast.IndexExpr, indent int)
	// PreVisitIndexExprIndex is called before visiting the index part of an index expression.
	PreVisitIndexExprIndex(node *ast.IndexExpr, indent int)
	// PostVisitIndexExprIndex is called after visiting the index part of an index expression.
	PostVisitIndexExprIndex(node *ast.IndexExpr, indent int)

	// PreVisitUnaryExpr is called before visiting a unary expression.
	PreVisitUnaryExpr(node *ast.UnaryExpr, indent int)
	// PostVisitUnaryExpr is called after visiting a unary expression.
	PostVisitUnaryExpr(node *ast.UnaryExpr, indent int)

	// PreVisitSliceExpr is called before visiting a slice expression.
	PreVisitSliceExpr(node *ast.SliceExpr, indent int)
	// PostVisitSliceExpr is called after visiting a slice expression.
	PostVisitSliceExpr(node *ast.SliceExpr, indent int)
	// PreVisitSliceExprX is called before visiting the X part of a slice expression.
	PreVisitSliceExprX(node ast.Expr, indent int)
	// PostVisitSliceExprX is called after visiting the X part of a slice expression.
	PostVisitSliceExprX(node ast.Expr, indent int)
	// PreVisitSliceExprXBegin is called before visiting the beginning of the X part of a slice expression.
	PreVisitSliceExprXBegin(node ast.Expr, indent int)
	// PostVisitSliceExprXBegin is called after visiting the beginning of the X part of a slice expression.
	PostVisitSliceExprXBegin(node ast.Expr, indent int)
	// PreVisitSliceExprXEnd is called before visiting the end of the X part of a slice expression.
	PreVisitSliceExprXEnd(node ast.Expr, indent int)
	// PostVisitSliceExprXEnd is called after visiting the end of the X part of a slice expression.
	PostVisitSliceExprXEnd(node ast.Expr, indent int)
	// PreVisitSliceExprLow is called before visiting the low bound of a slice expression.
	PreVisitSliceExprLow(node ast.Expr, indent int)
	// PostVisitSliceExprLow is called after visiting the low bound of a slice expression.
	PostVisitSliceExprLow(node ast.Expr, indent int)
	// PreVisitSliceExprHigh is called before visiting the high bound of a slice expression.
	PreVisitSliceExprHigh(node ast.Expr, indent int)
	// PostVisitSliceExprHigh is called after visiting the high bound of a slice expression.
	PostVisitSliceExprHigh(node ast.Expr, indent int)

	// PreVisitFuncType is called before visiting a function type.
	PreVisitFuncType(node *ast.FuncType, indent int)
	// PostVisitFuncType is called after visiting a function type.
	PostVisitFuncType(node *ast.FuncType, indent int)
	// PreVisitFuncTypeResults is called before visiting the results of a function type.
	PreVisitFuncTypeResults(node *ast.FieldList, indent int)
	// PostVisitFuncTypeResults is called after visiting the results of a function type.
	PostVisitFuncTypeResults(node *ast.FieldList, indent int)
	// PreVisitFuncTypeResult is called before visiting a specific result of a function type.
	PreVisitFuncTypeResult(node *ast.Field, index int, indent int)
	// PostVisitFuncTypeResult is called after visiting a specific result of a function type.
	PostVisitFuncTypeResult(node *ast.Field, index int, indent int)
	// PreVisitFuncTypeParams is called before visiting the parameters of a function type.
	PreVisitFuncTypeParams(node *ast.FieldList, indent int)
	// PostVisitFuncTypeParams is called after visiting the parameters of a function type.
	PostVisitFuncTypeParams(node *ast.FieldList, indent int)
	// PreVisitFuncTypeParam is called before visiting a specific parameter of a function type.
	PreVisitFuncTypeParam(node *ast.Field, index int, indent int)
	// PostVisitFuncTypeParam is called after visiting a specific parameter of a function type.
	PostVisitFuncTypeParam(node *ast.Field, index int, indent int)

	// PreVisitKeyValueExpr is called before visiting a key-value expression.
	PreVisitKeyValueExpr(node *ast.KeyValueExpr, indent int)
	// PostVisitKeyValueExpr is called after visiting a key-value expression.
	PostVisitKeyValueExpr(node *ast.KeyValueExpr, indent int)
	// PreVisitKeyValueExprKey is called before visiting the key part of a key-value expression.
	PreVisitKeyValueExprKey(node ast.Expr, indent int)
	// PostVisitKeyValueExprKey is called after visiting the key part of a key-value expression.
	PostVisitKeyValueExprKey(node ast.Expr, indent int)
	// PreVisitKeyValueExprValue is called before visiting the value part of a key-value expression.
	PreVisitKeyValueExprValue(node ast.Expr, indent int)
	// PostVisitKeyValueExprValue is called after visiting the value part of a key-value expression.
	PostVisitKeyValueExprValue(node ast.Expr, indent int)

	// PreVisitFuncLit is called before visiting a function literal.
	PreVisitFuncLit(node *ast.FuncLit, indent int)
	// PostVisitFuncLit is called after visiting a function literal.
	PostVisitFuncLit(node *ast.FuncLit, indent int)
	// PreVisitFuncLitTypeParams is called before visiting the parameters of a function literal.
	PreVisitFuncLitTypeParams(node *ast.FieldList, indent int)
	// PostVisitFuncLitTypeParams is called after visiting the parameters of a function literal.
	PostVisitFuncLitTypeParams(node *ast.FieldList, indent int)
	// PreVisitFuncLitTypeParam is called before visiting a specific parameter of a function literal.
	PreVisitFuncLitTypeParam(node *ast.Field, index int, indent int)
	// PostVisitFuncLitTypeParam is called after visiting a specific parameter of a function literal.
	PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int)
	// PreVisitFuncLitBody is called before visiting the body of a function literal.
	PreVisitFuncLitBody(node *ast.BlockStmt, indent int)
	// PostVisitFuncLitBody is called after visiting the body of a function literal.
	PostVisitFuncLitBody(node *ast.BlockStmt, indent int)
	// PreVisitFuncLitTypeResults is called before visiting the results of a function literal.
	PreVisitFuncLitTypeResults(node *ast.FieldList, indent int)
	// PostVisitFuncLitTypeResults is called after visiting the results of a function literal.
	PostVisitFuncLitTypeResults(node *ast.FieldList, indent int)
	// PreVisitFuncLitTypeResult is called before visiting a specific result of a function literal.
	PreVisitFuncLitTypeResult(node *ast.Field, index int, indent int)
	// PostVisitFuncLitTypeResult is called after visiting a specific result of a function literal.
	PostVisitFuncLitTypeResult(node *ast.Field, index int, indent int)

	// PreVisitTypeAssertExpr is called before visiting a type assertion expression.
	PreVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int)
	// PostVisitTypeAssertExpr is called after visiting a type assertion expression.
	PostVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int)
	// PreVisitTypeAssertExprX is called before visiting the X part of a type assertion.
	PreVisitTypeAssertExprX(node ast.Expr, indent int)
	// PostVisitTypeAssertExprX is called after visiting the X part of a type assertion.
	PostVisitTypeAssertExprX(node ast.Expr, indent int)
	// PreVisitTypeAssertExprType is called before visiting the type part of a type assertion.
	PreVisitTypeAssertExprType(node ast.Expr, indent int)
	// PostVisitTypeAssertExprType is called after visiting the type part of a type assertion.
	PostVisitTypeAssertExprType(node ast.Expr, indent int)

	// PreVisitStarExpr is called before visiting a star expression (pointer type).
	PreVisitStarExpr(node *ast.StarExpr, indent int)
	// PostVisitStarExpr is called after visiting a star expression (pointer type).
	PostVisitStarExpr(node *ast.StarExpr, indent int)
	// PreVisitStarExprX is called before visiting the X part of a star expression.
	PreVisitStarExprX(node ast.Expr, indent int)
	// PostVisitStarExprX is called after visiting the X part of a star expression.
	PostVisitStarExprX(node ast.Expr, indent int)

	// PreVisitInterfaceType is called before visiting an interface type.
	PreVisitInterfaceType(node *ast.InterfaceType, indent int)
	// PostVisitInterfaceType is called after visiting an interface type.
	PostVisitInterfaceType(node *ast.InterfaceType, indent int)

	// PreVisitStructType is called before visiting a struct type.
	PreVisitStructType(node *ast.StructType, indent int)
	// PostVisitStructType is called after visiting a struct type.
	PostVisitStructType(node *ast.StructType, indent int)

	// PreVisitTypeSwitchStmt is called before visiting a type switch statement.
	PreVisitTypeSwitchStmt(node *ast.TypeSwitchStmt, indent int)
	// PostVisitTypeSwitchStmt is called after visiting a type switch statement.
	PostVisitTypeSwitchStmt(node *ast.TypeSwitchStmt, indent int)

	// PreVisitExprStmt is called before visiting an expression statement.
	PreVisitExprStmt(node *ast.ExprStmt, indent int)
	// PostVisitExprStmt is called after visiting an expression statement.
	PostVisitExprStmt(node *ast.ExprStmt, indent int)
	// PreVisitExprStmtX is called before visiting the expression part of an expression statement.
	PreVisitExprStmtX(node ast.Expr, indent int)
	// PostVisitExprStmtX is called after visiting the expression part of an expression statement.
	PostVisitExprStmtX(node ast.Expr, indent int)
	// PreVisitDeclStmt is called before visiting a declaration statement.
	PreVisitDeclStmt(node *ast.DeclStmt, indent int)
	// PostVisitDeclStmt is called after visiting a declaration statement.
	PostVisitDeclStmt(node *ast.DeclStmt, indent int)
	// PreVisitDeclStmtValueSpecType is called before visiting the type of a value specification in a declaration.
	PreVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int)
	// PostVisitDeclStmtValueSpecType is called after visiting the type of a value specification in a declaration.
	PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int)
	// PreVisitDeclStmtValueSpecNames is called before visiting the names of a value specification in a declaration.
	PreVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int)
	// PostVisitDeclStmtValueSpecNames is called after visiting the names of a value specification in a declaration.
	PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int)
	// PreVisitDeclStmtValueSpecValue is called before visiting the value of a value specification in a declaration.
	PreVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int)
	// PostVisitDeclStmtValueSpecValue is called after visiting the value of a value specification in a declaration.
	PostVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int)
	// PreVisitBranchStmt is called before visiting a branch statement (break, continue, goto).
	PreVisitBranchStmt(node *ast.BranchStmt, indent int)
	// PostVisitBranchStmt is called after visiting a branch statement (break, continue, goto).
	PostVisitBranchStmt(node *ast.BranchStmt, indent int)
	// PreVisitDeferStmt is called before visiting a defer statement.
	PreVisitDeferStmt(node *ast.DeferStmt, indent int)
	// PostVisitDeferStmt is called after visiting a defer statement.
	PostVisitDeferStmt(node *ast.DeferStmt, indent int)
	// PreVisitGoStmt is called before visiting a go statement.
	PreVisitGoStmt(node *ast.GoStmt, indent int)
	// PostVisitGoStmt is called after visiting a go statement.
	PostVisitGoStmt(node *ast.GoStmt, indent int)
	// PreVisitSendStmt is called before visiting a send statement.
	PreVisitSendStmt(node *ast.SendStmt, indent int)
	// PostVisitSendStmt is called after visiting a send statement.
	PostVisitSendStmt(node *ast.SendStmt, indent int)
	// PreVisitSelectStmt is called before visiting a select statement.
	PreVisitSelectStmt(node *ast.SelectStmt, indent int)
	// PostVisitSelectStmt is called after visiting a select statement.
	PostVisitSelectStmt(node *ast.SelectStmt, indent int)
	// PreVisitIncDecStmt is called before visiting an increment/decrement statement.
	PreVisitIncDecStmt(node *ast.IncDecStmt, indent int)
	// PostVisitIncDecStmt is called after visiting an increment/decrement statement.
	PostVisitIncDecStmt(node *ast.IncDecStmt, indent int)
	// PreVisitAssignStmt is called before visiting an assignment statement.
	PreVisitAssignStmt(node *ast.AssignStmt, indent int)
	// PostVisitAssignStmt is called after visiting an assignment statement.
	PostVisitAssignStmt(node *ast.AssignStmt, indent int)
	// PostVisitForStmt is called after visiting a for statement.
	PostVisitForStmt(node *ast.ForStmt, indent int)
	// PreVisitForStmt is called before visiting a for statement.
	PreVisitForStmt(node *ast.ForStmt, indent int)
	// PreVisitForStmtInit is called before visiting the initialization part of a for statement.
	PreVisitForStmtInit(node ast.Stmt, indent int)
	// PostVisitForStmtInit is called after visiting the initialization part of a for statement.
	PostVisitForStmtInit(node ast.Stmt, indent int)
	// PreVisitForStmtCond is called before visiting the condition part of a for statement.
	PreVisitForStmtCond(node ast.Expr, indent int)
	// PostVisitForStmtCond is called after visiting the condition part of a for statement.
	PostVisitForStmtCond(node ast.Expr, indent int)
	// PreVisitForStmtPost is called before visiting the post statement of a for statement.
	PreVisitForStmtPost(node ast.Stmt, indent int)
	// PostVisitForStmtPost is called after visiting the post statement of a for statement.
	PostVisitForStmtPost(node ast.Stmt, indent int)
	// PreVisitAssignStmtLhs is called before visiting the left-hand side of an assignment statement.
	PreVisitAssignStmtLhs(node *ast.AssignStmt, indent int)
	// PostVisitAssignStmtLhs is called after visiting the left-hand side of an assignment statement.
	PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int)
	// PreVisitAssignStmtRhs is called before visiting the right-hand side of an assignment statement.
	PreVisitAssignStmtRhs(node *ast.AssignStmt, indent int)
	// PostVisitAssignStmtRhs is called after visiting the right-hand side of an assignment statement.
	PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int)
	// PreVisitAssignStmtLhsExpr is called before visiting a specific left-hand side expression of an assignment statement.
	PreVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int)
	// PostVisitAssignStmtLhsExpr is called after visiting a specific left-hand side expression of an assignment statement.
	PostVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int)
	// PreVisitAssignStmtRhsExpr is called before visiting a specific right-hand side expression of an assignment statement.
	PreVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int)
	// PostVisitAssignStmtRhsExpr is called after visiting a specific right-hand side expression of an assignment statement.
	PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int)
	// PreVisitReturnStmt is called before visiting a return statement.
	PreVisitReturnStmt(node *ast.ReturnStmt, indent int)
	// PostVisitReturnStmt is called after visiting a return statement.
	PostVisitReturnStmt(node *ast.ReturnStmt, indent int)
	// PreVisitReturnStmtResult is called before visiting the results of a return statement.
	PreVisitReturnStmtResult(node ast.Expr, index int, indent int)
	// PostVisitReturnStmtResult is called after visiting the results of a return statement.
	PostVisitReturnStmtResult(node ast.Expr, index int, indent int)
	// PreVisitIfStmt is called before visiting an if statement.
	PreVisitIfStmt(node *ast.IfStmt, indent int)
	// PostVisitIfStmt is called after visiting an if statement.
	PostVisitIfStmt(node *ast.IfStmt, indent int)
	// PreVisitIfStmtInit is called before visiting the Init part of an if statement.
	PreVisitIfStmtInit(node ast.Stmt, indent int)
	// PostVisitIfStmtInit is called after visiting the Init part of an if statement.
	PostVisitIfStmtInit(node ast.Stmt, indent int)
	// PreVisitIfStmtCond is called before visiting the Cond part of an if statement.
	PreVisitIfStmtCond(node *ast.IfStmt, indent int)
	// PostVisitIfStmtCond is called after visiting the Cond part of an if statement.
	PostVisitIfStmtCond(node *ast.IfStmt, indent int)
	// PreVisitIfStmtBody is called before visiting the Body part of an if statement.
	PreVisitIfStmtBody(node *ast.IfStmt, indent int)
	// PostVisitIfStmtBody is called after visiting the Body part of an if statement.
	PostVisitIfStmtBody(node *ast.IfStmt, indent int)
	// PreVisitIfStmtElse is called before visiting the Else part of an if statement.
	PreVisitIfStmtElse(node *ast.IfStmt, indent int)
	// PostVisitIfStmtElse is called after visiting the Else part of an if statement.
	PostVisitIfStmtElse(node *ast.IfStmt, indent int)
	PreVisitRangeStmt(node *ast.RangeStmt, indent int)
	PostVisitRangeStmt(node *ast.RangeStmt, indent int)
	PreVisitRangeStmtKey(node ast.Expr, indent int)
	PostVisitRangeStmtKey(node ast.Expr, indent int)
	PreVisitRangeStmtValue(node ast.Expr, indent int)
	PostVisitRangeStmtValue(node ast.Expr, indent int)
	PreVisitRangeStmtX(node ast.Expr, indent int)
	PostVisitRangeStmtX(node ast.Expr, indent int)
	PreVisitSwitchStmt(node *ast.SwitchStmt, indent int)
	PostVisitSwitchStmt(node *ast.SwitchStmt, indent int)
	PreVisitSwitchStmtTag(node ast.Expr, indent int)
	PostVisitSwitchStmtTag(node ast.Expr, indent int)
	PreVisitCaseClause(node *ast.CaseClause, indent int)
	PostVisitCaseClause(node *ast.CaseClause, indent int)
	PreVisitCaseClauseList(node []ast.Expr, indent int)
	PostVisitCaseClauseList(node []ast.Expr, indent int)
	PreVisitCaseClauseListExpr(node ast.Expr, index int, indent int)
	PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int)
	PreVisitBlockStmt(node *ast.BlockStmt, indent int)
	PostVisitBlockStmt(node *ast.BlockStmt, indent int)
	PreVisitBlockStmtList(node ast.Stmt, index int, indent int)
	PostVisitBlockStmtList(node ast.Stmt, index int, indent int)
	PreVisitFuncDecl(node *ast.FuncDecl, indent int)
	PostVisitFuncDecl(node *ast.FuncDecl, indent int)
	PreVisitFuncDeclBody(node *ast.BlockStmt, indent int)
	PostVisitFuncDeclBody(node *ast.BlockStmt, indent int)
	PreVisitFuncDeclSignature(node *ast.FuncDecl, indent int)
	PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int)
	PreVisitFuncDeclName(node *ast.Ident, indent int)
	PostVisitFuncDeclName(node *ast.Ident, indent int)
	PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int)
	PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int)
	PreVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int)
	PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int)
	PreVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int)
	PostVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int)
	PreVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int)
	PostVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int)
	PreVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int)
	PostVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int)
	PreVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int)
	PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int)
	PreVisitGenStructInfos(node []GenTypeInfo, indent int)
	PostVisitGenStructInfos(node []GenTypeInfo, indent int)
	PreVisitGenStructInfo(node GenTypeInfo, indent int)
	PostVisitGenStructInfo(node GenTypeInfo, indent int)
	PreVisitFuncDeclSignatures(indent int)
	PostVisitFuncDeclSignatures(indent int)
	PreVisitGenDeclConst(node *ast.GenDecl, indent int)
	PostVisitGenDeclConst(node *ast.GenDecl, indent int)
	PreVisitGenStructFieldType(node ast.Expr, indent int)
	PostVisitGenStructFieldType(node ast.Expr, indent int)
	PreVisitGenStructFieldName(node *ast.Ident, indent int)
	PostVisitGenStructFieldName(node *ast.Ident, indent int)
	PreVisitGenDeclConstName(node *ast.Ident, indent int)
	PostVisitGenDeclConstName(node *ast.Ident, indent int)
	PreVisitTypeAliasName(node *ast.Ident, indent int)
	PostVisitTypeAliasName(node *ast.Ident, indent int)
	PreVisitTypeAliasType(node ast.Expr, indent int)
	PostVisitTypeAliasType(node ast.Expr, indent int)
}
