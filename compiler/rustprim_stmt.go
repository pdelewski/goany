package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

// ============================================================
// Expression Statements
// ============================================================

func (e *RustPrimEmitter) PreVisitExprStmt(node *ast.ExprStmt, indent int) {
	e.indent = indent
}

func (e *RustPrimEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitExprStmtX))
	e.fs.PushCode(xCode)
}

func (e *RustPrimEmitter) PostVisitExprStmt(node *ast.ExprStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitExprStmt))
	code := ""
	if len(tokens) >= 1 {
		code = tokens[0].Content
	}
	ind := rustpIndent(indent / 2)
	e.fs.PushCode(ind + code + ";\n")
}

// ============================================================
// Assignment Statements
// ============================================================

func (e *RustPrimEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	e.indent = indent
	e.mapAssignVar = ""
	e.mapAssignKey = ""
}

func (e *RustPrimEmitter) PreVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	e.inAssignLhs = true
}

func (e *RustPrimEmitter) PostVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	e.inAssignLhs = false
	lhsCode := e.fs.ReduceToCode(string(PreVisitAssignStmtLhsExpr))

	if indexExpr, ok := node.(*ast.IndexExpr); ok {
		if e.isMapTypeExpr(indexExpr.X) {
			e.mapAssignVar = e.lastIndexXCode
			e.mapAssignKey = e.lastIndexKeyCode
			e.fs.PushCode(lhsCode)
			return
		}
	}
	e.fs.PushCode(lhsCode)
}

func (e *RustPrimEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmtLhs))
	var lhsExprs []string
	for _, t := range tokens {
		if t.Content != "" {
			lhsExprs = append(lhsExprs, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(lhsExprs, ", "))
}

func (e *RustPrimEmitter) PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {
	rhsCode := e.fs.ReduceToCode(string(PreVisitAssignStmtRhsExpr))
	e.fs.PushCode(rhsCode)
}

func (e *RustPrimEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmtRhs))
	var rhsExprs []string
	for _, t := range tokens {
		if t.Content != "" {
			rhsExprs = append(rhsExprs, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(rhsExprs, ", "))
}

func (e *RustPrimEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmt))
	lhsStr := ""
	rhsStr := ""
	if len(tokens) >= 1 {
		lhsStr = tokens[0].Content
	}
	if len(tokens) >= 2 {
		rhsStr = tokens[1].Content
	}

	ind := rustpIndent(indent / 2)
	tokStr := node.Tok.String()

	// Local closure inlining: save closure body for later call-site inlining
	// This avoids Rust borrow checker issues with closures that capture mutable variables
	if tokStr == ":=" && len(node.Lhs) == 1 && len(node.Rhs) == 1 {
		if _, isFuncLit := node.Rhs[0].(*ast.FuncLit); isFuncLit {
			varName := exprToString(node.Lhs[0])
			if e.localClosureBodies == nil {
				e.localClosureBodies = make(map[string]string)
			}
			// The RHS is a closure like "|| { body }" or "|params| -> ret { body }"
			// Save the entire closure code for inlining
			e.localClosureBodies[varName] = rhsStr
			// Don't emit the assignment
			e.fs.PushCode("")
			return
		}
	}

	// Map-of-slices assignment: map[key][idx] = value (or map[k1][k2][idx] = value)
	// Pattern: the LHS is a slice index, but the slice comes from a map access chain
	if len(node.Lhs) == 1 && len(node.Rhs) == 1 {
		if outerIdx, ok := node.Lhs[0].(*ast.IndexExpr); ok {
			code := e.emitMapSliceAssign(outerIdx, rhsStr, ind)
			if code != "" {
				e.fs.PushCode(code)
				e.mapAssignVar = ""
				e.mapAssignKey = ""
				return
			}
		}
	}

	// Map assignment: m[k] = v -> m = hmap::hashMapSet(m, Rc::new(k), Rc::new(v))
	if e.mapAssignVar != "" && e.mapAssignKey != "" {
		code := e.emitMapAssign(node, rhsStr, ind)
		e.fs.PushCode(code)
		e.mapAssignVar = ""
		e.mapAssignKey = ""
		return
	}

	// Comma-ok map read: val, ok := m[key]
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if indexExpr, ok := node.Rhs[0].(*ast.IndexExpr); ok {
			if e.isMapTypeExpr(indexExpr.X) {
				valName := exprToString(node.Lhs[0])
				okName := exprToString(node.Lhs[1])
				mapName := exprToString(indexExpr.X)
				keyStr := exprToString(indexExpr.Index)

				mapGoType := e.getExprGoType(indexExpr.X)
				keyCast := ""
				keyIsStr := false
				valType := "Rc<dyn Any>"
				zeroVal := "Default::default()"
				if mapGoType != nil {
					if mapUnderlying, ok2 := mapGoType.Underlying().(*types.Map); ok2 {
						keyCast = getRustKeyCast(mapUnderlying.Key())
						keyIsStr = isRustStringKey(mapUnderlying.Key())
						valType = e.qualifiedRustTypeName(mapUnderlying.Elem())
						zeroVal = rustDefaultForRustType(valType)
					}
				}
				var keyExpr string
				if keyIsStr {
					keyExpr = fmt.Sprintf("Rc::new(%s.to_string())", keyStr)
				} else {
					keyExpr = fmt.Sprintf("Rc::new(%s%s)", keyStr, keyCast)
				}

				castExpr := ""
				if valType != "Rc<dyn Any>" {
					castExpr = fmt.Sprintf(".downcast_ref::<%s>().unwrap().clone()", valType)
				}

				if tokStr == ":=" {
					e.fs.PushCode(fmt.Sprintf("%slet mut %s = hmap::hashMapContains(%s.clone(), %s);\n",
						ind, okName, mapName, keyExpr))
					e.fs.PushCode(fmt.Sprintf("%slet mut %s = if %s { hmap::hashMapGet(%s.clone(), %s)%s } else { %s };\n",
						ind, valName, okName, mapName, keyExpr, castExpr, zeroVal))
				} else {
					e.fs.PushCode(fmt.Sprintf("%s%s = hmap::hashMapContains(%s.clone(), %s);\n",
						ind, okName, mapName, keyExpr))
					e.fs.PushCode(fmt.Sprintf("%s%s = if %s { hmap::hashMapGet(%s.clone(), %s)%s } else { %s };\n",
						ind, valName, okName, mapName, keyExpr, castExpr, zeroVal))
				}
				return
			}
		}
	}

	// Comma-ok type assertion: val, ok := x.(Type)
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if typeAssert, ok := node.Rhs[0].(*ast.TypeAssertExpr); ok {
			valName := exprToString(node.Lhs[0])
			okName := exprToString(node.Lhs[1])
			assertType := ""
			if typeAssert.Type != nil {
				assertType = exprToString(typeAssert.Type)
				if rustType, ok := rustTypesMap[assertType]; ok {
					assertType = rustType
				}
			}
			xExpr := exprToString(typeAssert.X)
			zeroVal := rustDefaultForRustType(assertType)
			if tokStr == ":=" {
				e.fs.PushCode(fmt.Sprintf("%slet mut %s = %s.downcast_ref::<%s>().is_some();\n",
					ind, okName, xExpr, assertType))
				e.fs.PushCode(fmt.Sprintf("%slet mut %s = if %s { %s.downcast_ref::<%s>().unwrap().clone() } else { %s };\n",
					ind, valName, okName, xExpr, assertType, zeroVal))
			} else {
				e.fs.PushCode(fmt.Sprintf("%s%s = %s.downcast_ref::<%s>().is_some();\n",
					ind, okName, xExpr, assertType))
				e.fs.PushCode(fmt.Sprintf("%s%s = if %s { %s.downcast_ref::<%s>().unwrap().clone() } else { %s };\n",
					ind, valName, okName, xExpr, assertType, zeroVal))
			}
			return
		}
	}

	// Multi-value return: a, b := func() -> let (mut a, mut b) = func()
	if len(node.Lhs) > 1 && len(node.Rhs) == 1 {
		if tokStr == ":=" {
			lhsParts := make([]string, len(node.Lhs))
			for i, lhs := range node.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if ident.Name == "_" {
						lhsParts[i] = "_"
					} else {
						lhsParts[i] = "mut " + escapeRustKeyword(ident.Name)
					}
				} else {
					lhsParts[i] = exprToString(lhs)
				}
			}
			destructured := "(" + strings.Join(lhsParts, ", ") + ")"
			e.fs.PushCode(fmt.Sprintf("%slet %s = %s;\n", ind, destructured, rhsStr))
		} else {
			// For reassignment, don't use `mut`
			lhsParts := make([]string, len(node.Lhs))
			for i, lhs := range node.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if ident.Name == "_" {
						lhsParts[i] = "_"
					} else {
						lhsParts[i] = escapeRustKeyword(ident.Name)
					}
				} else {
					lhsParts[i] = exprToString(lhs)
				}
			}
			destructured := "(" + strings.Join(lhsParts, ", ") + ")"
			e.fs.PushCode(fmt.Sprintf("%s%s = %s;\n", ind, destructured, rhsStr))
		}
		return
	}

	// Check if LHS is interface{}/any type and RHS is a concrete type -> wrap in Rc::new()
	needsRcWrap := false
	if len(node.Lhs) == 1 && len(node.Rhs) == 1 && e.pkg != nil && e.pkg.TypesInfo != nil {
		lhsTypeInfo := e.pkg.TypesInfo.Types[node.Lhs[0]]
		rhsTypeInfo := e.pkg.TypesInfo.Types[node.Rhs[0]]
		if lhsTypeInfo.Type != nil && rhsTypeInfo.Type != nil {
			lhsTypeStr := lhsTypeInfo.Type.String()
			rhsTypeStr := rhsTypeInfo.Type.String()
			if (lhsTypeStr == "interface{}" || lhsTypeStr == "any") &&
				rhsTypeStr != "interface{}" && rhsTypeStr != "any" {
				needsRcWrap = true
			}
		}
	}

	// Cast RHS for small-int LHS fields (handles untyped Go constants emitted as i32)
	if len(node.Lhs) == 1 && len(node.Rhs) == 1 && !needsRcWrap {
		lhsType := e.getExprGoType(node.Lhs[0])
		if lhsType != nil {
			rhsStr = e.castSmallIntFieldValue(lhsType, rhsStr)
		}
	}

	// Clone RHS for non-Copy identifiers in := and = assignments to prevent move
	if len(node.Lhs) == 1 && len(node.Rhs) == 1 && !needsRcWrap && (tokStr == ":=" || tokStr == "=") {
		if ident, ok := node.Rhs[0].(*ast.Ident); ok && ident.Name != "nil" && ident.Name != "true" && ident.Name != "false" {
			rhsType := e.getExprGoType(node.Rhs[0])
			if rhsType != nil && !isCopyType(rhsType) {
				if !strings.HasSuffix(rhsStr, ".clone()") {
					rhsStr = rhsStr + ".clone()"
				}
			}
		}
	}

	if needsRcWrap {
		wrappedRhs := fmt.Sprintf("Rc::new(%s)", rhsStr)
		switch tokStr {
		case ":=":
			e.fs.PushCode(fmt.Sprintf("%slet mut %s = %s;\n", ind, lhsStr, wrappedRhs))
		default:
			e.fs.PushCode(fmt.Sprintf("%s%s = %s;\n", ind, lhsStr, wrappedRhs))
		}
	} else {
		switch tokStr {
		case ":=":
			e.fs.PushCode(fmt.Sprintf("%slet mut %s = %s;\n", ind, lhsStr, rhsStr))
		case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
			// For string +=, RHS must be &str, so add & prefix
			if tokStr == "+=" && len(node.Lhs) == 1 {
				lhsType := e.getExprGoType(node.Lhs[0])
				if lhsType != nil {
					if basic, ok := lhsType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
						e.fs.PushCode(fmt.Sprintf("%s%s += &%s;\n", ind, lhsStr, rhsStr))
						return
					}
				}
			}
			e.fs.PushCode(fmt.Sprintf("%s%s %s %s;\n", ind, lhsStr, tokStr, rhsStr))
		default:
			e.fs.PushCode(fmt.Sprintf("%s%s = %s;\n", ind, lhsStr, rhsStr))
		}
	}
}

// ============================================================
// Declaration Statements (var x int, var y = 5)
// ============================================================

func (e *RustPrimEmitter) PreVisitDeclStmt(node *ast.DeclStmt, indent int) {
	e.indent = indent
}

func (e *RustPrimEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	tokens := e.fs.Reduce(string(PreVisitDeclStmtValueSpecType))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Content
	}
	var goType types.Type
	if e.pkg != nil && e.pkg.TypesInfo != nil && index < len(node.Names) {
		if obj := e.pkg.TypesInfo.Defs[node.Names[index]]; obj != nil {
			goType = obj.Type()
		}
	}
	e.fs.Push(typeStr, TagType, goType)
}

func (e *RustPrimEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	e.fs.Reduce(string(PreVisitDeclStmtValueSpecNames))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *RustPrimEmitter) PostVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitDeclStmtValueSpecValue))
	e.fs.Push(valCode, TagExpr, nil)
}

func (e *RustPrimEmitter) PostVisitDeclStmt(node *ast.DeclStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitDeclStmt))
	ind := rustpIndent(indent / 2)

	var sb strings.Builder
	i := 0
	for i < len(tokens) {
		typeStr := ""
		var goType types.Type
		nameStr := ""
		valueStr := ""

		if i < len(tokens) && tokens[i].Tag == TagType {
			typeStr = tokens[i].Content
			goType = tokens[i].GoType
			i++
		}
		if i < len(tokens) && tokens[i].Tag == TagIdent {
			nameStr = tokens[i].Content
			i++
		}
		if i < len(tokens) && tokens[i].Tag == TagExpr {
			valueStr = tokens[i].Content
			i++
		}

		if nameStr == "" {
			continue
		}

		escapedName := escapeRustKeyword(nameStr)

		if valueStr != "" {
			sb.WriteString(fmt.Sprintf("%slet mut %s: %s = %s;\n", ind, escapedName, typeStr, valueStr))
		} else {
			// Generate default value
			defaultVal := "Default::default()"
			if goType != nil {
				if _, isSlice := goType.Underlying().(*types.Slice); isSlice {
					defaultVal = "Vec::new()"
				} else if _, isMap := goType.Underlying().(*types.Map); isMap {
					// Find map type for key type
					for _, spec := range node.Decl.(*ast.GenDecl).Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							if mapType, ok := vs.Type.(*ast.MapType); ok {
								keyConst := e.getMapKeyTypeConst(mapType)
								defaultVal = fmt.Sprintf("hmap::newHashMap(%d)", keyConst)
							}
						}
					}
				} else if _, isStruct := goType.Underlying().(*types.Struct); isStruct {
					defaultVal = fmt.Sprintf("%s::default()", typeStr)
				} else {
					defaultVal = e.rustDefaultForGoType(goType)
				}
			} else if typeStr != "" {
				defaultVal = rustDefaultForRustType(typeStr)
			}
			sb.WriteString(fmt.Sprintf("%slet mut %s: %s = %s;\n", ind, escapedName, typeStr, defaultVal))
		}
	}
	e.fs.PushCode(sb.String())
}

// ============================================================
// Return Statements
// ============================================================

func (e *RustPrimEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	e.indent = indent
}

func (e *RustPrimEmitter) PostVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	resultCode := e.fs.ReduceToCode(string(PreVisitReturnStmtResult))

	// If returning from a function that returns interface{}/any, wrap concrete types in Rc::new()
	if e.funcReturnType != nil && node != nil && e.pkg != nil && e.pkg.TypesInfo != nil {
		retTypeStr := e.funcReturnType.String()
		if retTypeStr == "interface{}" || retTypeStr == "any" {
			nodeTypeInfo := e.pkg.TypesInfo.Types[node]
			if nodeTypeInfo.Type != nil {
				nodeTypeStr := nodeTypeInfo.Type.String()
				if nodeTypeStr != "interface{}" && nodeTypeStr != "any" {
					resultCode = fmt.Sprintf("Rc::new(%s)", resultCode)
				}
			}
		}
	}

	e.fs.PushCode(resultCode)
}

func (e *RustPrimEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitReturnStmt))
	ind := rustpIndent(indent / 2)

	if len(tokens) == 0 {
		e.fs.PushCode(ind + "return;\n")
	} else if len(tokens) == 1 {
		e.fs.PushCode(fmt.Sprintf("%sreturn %s;\n", ind, tokens[0].Content))
	} else {
		// For multi-value returns, clone non-Copy values that might be referenced
		// by other return values (avoids borrow-after-move)
		var vals []string
		for i, t := range tokens {
			val := t.Content
			if i == 0 && len(node.Results) > 1 {
				// Clone the first return value if it's a non-Copy type
				// to avoid move-before-borrow issues
				if len(node.Results) > 0 {
					resultType := e.getExprGoType(node.Results[i])
					if resultType != nil && !isCopyType(resultType) {
						val = val + ".clone()"
					}
				}
			}
			vals = append(vals, val)
		}
		e.fs.PushCode(fmt.Sprintf("%sreturn (%s);\n", ind, strings.Join(vals, ", ")))
	}
}

// ============================================================
// If Statements
// ============================================================

func (e *RustPrimEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	e.indent = indent
	e.ifInitStack = append(e.ifInitStack, "")
	e.ifCondStack = append(e.ifCondStack, "")
	e.ifBodyStack = append(e.ifBodyStack, "")
	e.ifElseStack = append(e.ifElseStack, "")
}

func (e *RustPrimEmitter) PostVisitIfStmtInit(node ast.Stmt, indent int) {
	e.ifInitStack[len(e.ifInitStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtInit))
}

func (e *RustPrimEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	e.ifCondStack[len(e.ifCondStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtCond))
}

func (e *RustPrimEmitter) PostVisitIfStmtBody(node *ast.IfStmt, indent int) {
	e.ifBodyStack[len(e.ifBodyStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtBody))
}

func (e *RustPrimEmitter) PostVisitIfStmtElse(node *ast.IfStmt, indent int) {
	e.ifElseStack[len(e.ifElseStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtElse))
}

func (e *RustPrimEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	e.fs.Reduce(string(PreVisitIfStmt))
	ind := rustpIndent(indent / 2)

	n := len(e.ifInitStack)
	initCode := e.ifInitStack[n-1]
	condCode := e.ifCondStack[n-1]
	bodyCode := e.ifBodyStack[n-1]
	elseCode := e.ifElseStack[n-1]
	e.ifInitStack = e.ifInitStack[:n-1]
	e.ifCondStack = e.ifCondStack[:n-1]
	e.ifBodyStack = e.ifBodyStack[:n-1]
	e.ifElseStack = e.ifElseStack[:n-1]

	var sb strings.Builder
	if initCode != "" {
		sb.WriteString(fmt.Sprintf("%s{\n", ind))
		sb.WriteString(initCode)
		sb.WriteString(fmt.Sprintf("%sif %s %s", ind, condCode, bodyCode))
	} else {
		sb.WriteString(fmt.Sprintf("%sif %s %s", ind, condCode, bodyCode))
	}
	if elseCode != "" {
		trimmed := strings.TrimLeft(elseCode, " \t\n")
		if strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "if(") {
			sb.WriteString(" else " + trimmed)
		} else {
			sb.WriteString(" else " + elseCode)
		}
	}
	sb.WriteString("\n")
	if initCode != "" {
		sb.WriteString(fmt.Sprintf("%s}\n", ind))
	}
	e.fs.PushCode(sb.String())
}

// ============================================================
// For Statements
// ============================================================

func (e *RustPrimEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	e.indent = indent
	e.forInitStack = append(e.forInitStack, "")
	e.forCondStack = append(e.forCondStack, "")
	e.forPostStack = append(e.forPostStack, "")
}

func (e *RustPrimEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	initCode := e.fs.ReduceToCode(string(PreVisitForStmtInit))
	initCode = strings.TrimRight(initCode, ";\n \t")
	initCode = strings.TrimLeft(initCode, " \t")
	e.forInitStack[len(e.forInitStack)-1] = initCode
}

func (e *RustPrimEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	e.forCondStack[len(e.forCondStack)-1] = e.fs.ReduceToCode(string(PreVisitForStmtCond))
}

func (e *RustPrimEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	postCode := e.fs.ReduceToCode(string(PreVisitForStmtPost))
	postCode = strings.TrimRight(postCode, ";\n \t")
	postCode = strings.TrimLeft(postCode, " \t")
	e.forPostStack[len(e.forPostStack)-1] = postCode
}

func (e *RustPrimEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitForStmt))
	ind := rustpIndent(indent / 2)

	n := len(e.forInitStack)
	initCode := e.forInitStack[n-1]
	condCode := e.forCondStack[n-1]
	postCode := e.forPostStack[n-1]
	e.forInitStack = e.forInitStack[:n-1]
	e.forCondStack = e.forCondStack[:n-1]
	e.forPostStack = e.forPostStack[:n-1]

	// Infinite loop: for {}
	if node.Init == nil && node.Cond == nil && node.Post == nil {
		e.fs.PushCode(fmt.Sprintf("%sloop %s\n", ind, bodyCode))
		return
	}

	// Condition-only loop: for cond {}
	if node.Init == nil && node.Post == nil && node.Cond != nil {
		e.fs.PushCode(fmt.Sprintf("%swhile %s %s\n", ind, condCode, bodyCode))
		return
	}

	// Try to detect simple integer range pattern: for i := start; i < end; i++ (or variants)
	if rangeCode := e.tryEmitForRange(node, ind, bodyCode, initCode); rangeCode != "" {
		e.fs.PushCode(rangeCode)
		return
	}

	// Full for loop that doesn't match a range pattern: emit as loop with manual control
	// Use __first_iter guard so that `continue` goes to top of loop which executes post
	// Pattern: { init; let mut __first = true; loop { if !__first { post; } __first = false; if !cond { break; } body; } }
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s{\n", ind))
	if initCode != "" {
		sb.WriteString(fmt.Sprintf("%s    %s;\n", ind, initCode))
	}
	if condCode == "" {
		condCode = "true"
	}
	sb.WriteString(fmt.Sprintf("%s    let mut __first_iter = true;\n", ind))
	sb.WriteString(fmt.Sprintf("%s    loop {\n", ind))
	if postCode != "" {
		sb.WriteString(fmt.Sprintf("%s        if !__first_iter {\n", ind))
		sb.WriteString(fmt.Sprintf("%s            %s;\n", ind, postCode))
		sb.WriteString(fmt.Sprintf("%s        }\n", ind))
	}
	sb.WriteString(fmt.Sprintf("%s        __first_iter = false;\n", ind))
	sb.WriteString(fmt.Sprintf("%s        if !(%s) { break; }\n", ind, condCode))
	// Extract body content (strip outer braces)
	innerBody := extractBlockBody(bodyCode)
	if innerBody != "" {
		sb.WriteString(innerBody)
	}
	sb.WriteString(fmt.Sprintf("%s    }\n", ind))
	sb.WriteString(fmt.Sprintf("%s}\n", ind))
	e.fs.PushCode(sb.String())
}

// tryEmitForRange detects simple integer range patterns and emits Rust for..in syntax.
// Returns the generated code string, or "" if the pattern doesn't match.
func (e *RustPrimEmitter) tryEmitForRange(node *ast.ForStmt, ind string, bodyCode string, initCode string) string {
	// Must have init, cond, and post
	if node.Init == nil || node.Cond == nil || node.Post == nil {
		return ""
	}

	// Init must be an assignment: i := start
	assignStmt, ok := node.Init.(*ast.AssignStmt)
	if !ok || len(assignStmt.Lhs) != 1 || len(assignStmt.Rhs) != 1 || assignStmt.Tok != token.DEFINE {
		return ""
	}
	loopIdent, ok := assignStmt.Lhs[0].(*ast.Ident)
	if !ok {
		return ""
	}
	loopVar := escapeRustKeyword(loopIdent.Name)

	// Get start value — accept literals, identifiers, or other simple expressions
	startVal := e.exprToRustCode(assignStmt.Rhs[0])
	if startVal == "" {
		return ""
	}

	// Cond must be binary expr: i < end, i <= end, i > end, i >= end
	condBin, ok := node.Cond.(*ast.BinaryExpr)
	if !ok {
		return ""
	}
	condIdent, ok := condBin.X.(*ast.Ident)
	if !ok || condIdent.Name != loopIdent.Name {
		return ""
	}

	// Get end expression as code
	// We need the end expression as a Rust string - reuse condCode's right side
	endCode := e.exprToRustCode(condBin.Y)
	if endCode == "" {
		return ""
	}

	// Post must be inc/dec statement or assign with +=/-=
	stepVal := ""
	isIncrement := false
	isDecrement := false

	if incDec, ok := node.Post.(*ast.IncDecStmt); ok {
		postIdent, ok := incDec.X.(*ast.Ident)
		if !ok || postIdent.Name != loopIdent.Name {
			return ""
		}
		if incDec.Tok == token.INC {
			isIncrement = true
			stepVal = "1"
		} else if incDec.Tok == token.DEC {
			isDecrement = true
			stepVal = "1"
		}
	} else if assignPost, ok := node.Post.(*ast.AssignStmt); ok {
		if len(assignPost.Lhs) != 1 || len(assignPost.Rhs) != 1 {
			return ""
		}
		postIdent, ok := assignPost.Lhs[0].(*ast.Ident)
		if !ok || postIdent.Name != loopIdent.Name {
			return ""
		}
		rhsLit, ok := assignPost.Rhs[0].(*ast.BasicLit)
		if !ok || rhsLit.Kind != token.INT {
			return ""
		}
		if assignPost.Tok == token.ADD_ASSIGN {
			isIncrement = true
			stepVal = rhsLit.Value
		} else if assignPost.Tok == token.SUB_ASSIGN {
			isDecrement = true
			stepVal = rhsLit.Value
		} else {
			return ""
		}
	} else {
		return ""
	}

	// Now compose the Rust range expression
	if isIncrement {
		if condBin.Op == token.LSS {
			// for i := start; i < end; i++ → for i in start..end
			rangeExpr := fmt.Sprintf("%s..%s", startVal, endCode)
			if stepVal != "1" {
				rangeExpr = fmt.Sprintf("(%s).step_by(%s)", rangeExpr, stepVal)
			}
			return fmt.Sprintf("%sfor %s in %s %s\n", ind, loopVar, rangeExpr, bodyCode)
		} else if condBin.Op == token.LEQ {
			// for i := start; i <= end; i++ → for i in start..=end
			rangeExpr := fmt.Sprintf("%s..=%s", startVal, endCode)
			if stepVal != "1" {
				rangeExpr = fmt.Sprintf("(%s).step_by(%s)", rangeExpr, stepVal)
			}
			return fmt.Sprintf("%sfor %s in %s %s\n", ind, loopVar, rangeExpr, bodyCode)
		}
	} else if isDecrement {
		if condBin.Op == token.GTR {
			// for i := start; i > end; i-- → for i in (end+1..=start).rev()
			rangeExpr := fmt.Sprintf("(%s+1..=%s).rev()", endCode, startVal)
			if stepVal != "1" {
				rangeExpr = fmt.Sprintf("(%s+1..=%s).rev().step_by(%s)", endCode, startVal, stepVal)
			}
			return fmt.Sprintf("%sfor %s in %s %s\n", ind, loopVar, rangeExpr, bodyCode)
		} else if condBin.Op == token.GEQ {
			// for i := start; i >= end; i-- → for i in (end..=start).rev()
			rangeExpr := fmt.Sprintf("(%s..=%s).rev()", endCode, startVal)
			if stepVal != "1" {
				rangeExpr = fmt.Sprintf("(%s..=%s).rev().step_by(%s)", endCode, startVal, stepVal)
			}
			return fmt.Sprintf("%sfor %s in %s %s\n", ind, loopVar, rangeExpr, bodyCode)
		}
	}

	return ""
}

// exprToRustCode converts a simple AST expression to a Rust code string.
func (e *RustPrimEmitter) exprToRustCode(expr ast.Expr) string {
	if lit, ok := expr.(*ast.BasicLit); ok {
		return lit.Value
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return escapeRustKeyword(ident.Name)
	}
	// SelectorExpr: a.b
	if sel, ok := expr.(*ast.SelectorExpr); ok {
		xCode := e.exprToRustCode(sel.X)
		if xCode != "" {
			return fmt.Sprintf("%s.%s", xCode, sel.Sel.Name)
		}
	}
	if call, ok := expr.(*ast.CallExpr); ok {
		// len() call
		if fun, ok := call.Fun.(*ast.Ident); ok && fun.Name == "len" {
			if len(call.Args) == 1 {
				argCode := e.exprToRustCode(call.Args[0])
				if argCode != "" {
					// Check if argument is a string
					argType := e.getExprGoType(call.Args[0])
					if argType != nil {
						if basic, ok := argType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
							return fmt.Sprintf("%s.len() as i32", argCode)
						}
					}
					return fmt.Sprintf("len(&%s.clone())", argCode)
				}
			}
		}
		// int() type cast
		if fun, ok := call.Fun.(*ast.Ident); ok && fun.Name == "int" {
			if len(call.Args) == 1 {
				argCode := e.exprToRustCode(call.Args[0])
				if argCode != "" {
					return fmt.Sprintf("%s as i32", argCode)
				}
			}
		}
	}
	// Binary expression: a + b, a - b, etc.
	if bin, ok := expr.(*ast.BinaryExpr); ok {
		xCode := e.exprToRustCode(bin.X)
		yCode := e.exprToRustCode(bin.Y)
		if xCode != "" && yCode != "" {
			return fmt.Sprintf("%s %s %s", xCode, bin.Op.String(), yCode)
		}
	}
	return ""
}

// extractBlockBody extracts the content between the first { and last } of a block string.
func extractBlockBody(bodyCode string) string {
	first := strings.Index(bodyCode, "{")
	last := strings.LastIndex(bodyCode, "}")
	if first < 0 || last < 0 || first >= last {
		return ""
	}
	inner := bodyCode[first+1 : last]
	return strings.TrimRight(inner, " \t\n")+ "\n"
}

// ============================================================
// Range Statements
// ============================================================

func (e *RustPrimEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	e.indent = indent
}

func (e *RustPrimEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	keyCode := e.fs.ReduceToCode(string(PreVisitRangeStmtKey))
	e.fs.Push(keyCode, TagIdent, nil)
}

func (e *RustPrimEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitRangeStmtValue))
	e.fs.Push(valCode, TagIdent, nil)
}

func (e *RustPrimEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitRangeStmtX))
	e.fs.PushCode(xCode)
}

func (e *RustPrimEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitRangeStmt))
	ind := rustpIndent(indent / 2)

	keyCode := ""
	valCode := ""
	xCode := ""
	bodyCode := ""

	idx := 0
	if node.Key != nil {
		if idx < len(tokens) && tokens[idx].Tag == TagIdent {
			keyCode = tokens[idx].Content
			idx++
		}
	}
	if node.Value != nil {
		if idx < len(tokens) && tokens[idx].Tag == TagIdent {
			valCode = tokens[idx].Content
			idx++
		}
	}
	if idx < len(tokens) {
		xCode = tokens[idx].Content
		idx++
	}
	if idx < len(tokens) {
		bodyCode = tokens[idx].Content
	}

	isMap := false
	if node.X != nil {
		isMap = e.isMapTypeExpr(node.X)
	}

	// Check for string range
	xType := e.getExprGoType(node.X)
	isString := false
	if xType != nil {
		if basic, ok := xType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
			isString = true
		}
	}

	if isMap {
		// Map range: iterate using hashMapKeys
		mapGoType := e.getExprGoType(node.X)
		keyCast := ""
		keyIsStr := false
		valType := "Rc<dyn Any>"
		if mapGoType != nil {
			if mapUnderlying, ok := mapGoType.Underlying().(*types.Map); ok {
				keyCast = getRustKeyCast(mapUnderlying.Key())
				keyIsStr = isRustStringKey(mapUnderlying.Key())
				valType = e.qualifiedRustTypeName(mapUnderlying.Elem())
			}
		}
		_ = keyCast
		_ = keyIsStr
		keysVar := fmt.Sprintf("_keys%d", e.rangeVarCounter)
		loopIdx := fmt.Sprintf("_mi%d", e.rangeVarCounter)
		e.rangeVarCounter++

		castExpr := ""
		if valType != "Rc<dyn Any>" {
			castExpr = fmt.Sprintf(".downcast_ref::<%s>().unwrap().clone()", valType)
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%s{\n", ind))
		sb.WriteString(fmt.Sprintf("%s    let %s = hmap::hashMapKeys(%s.clone());\n", ind, keysVar, xCode))
		sb.WriteString(fmt.Sprintf("%s    let mut %s: i32 = 0;\n", ind, loopIdx))
		sb.WriteString(fmt.Sprintf("%s    while (%s as usize) < %s.len() {\n", ind, loopIdx, keysVar))
		if keyCode != "_" && keyCode != "" {
			sb.WriteString(fmt.Sprintf("%s        let mut %s = %s[%s as usize].clone();\n", ind, keyCode, keysVar, loopIdx))
		}
		if valCode != "_" && valCode != "" {
			sb.WriteString(fmt.Sprintf("%s        let mut %s = hmap::hashMapGet(&%s, %s[%s as usize].clone())%s;\n",
				ind, valCode, xCode, keysVar, loopIdx, castExpr))
		}
		// Inject body
		sb.WriteString(fmt.Sprintf("%s        %s\n", ind, bodyCode))
		sb.WriteString(fmt.Sprintf("%s        %s += 1;\n", ind, loopIdx))
		sb.WriteString(fmt.Sprintf("%s    }\n", ind))
		sb.WriteString(fmt.Sprintf("%s}\n", ind))
		e.fs.PushCode(sb.String())
		return
	}

	// Slice/string range with key and value — use for..in pattern for correct continue semantics
	if valCode != "" && valCode != "_" {
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = fmt.Sprintf("_i%d", e.rangeVarCounter)
			e.rangeVarCounter++
		}

		var valDecl string
		if isString {
			valDecl = fmt.Sprintf("%s    let mut %s = %s.as_bytes()[%s as usize] as i8;\n", ind, valCode, xCode, loopVar)
		} else {
			valDecl = fmt.Sprintf("%s    let mut %s = %s[%s as usize].clone();\n", ind, valCode, xCode, loopVar)
		}
		// Inject val decl into body
		bodyWithDecl := strings.Replace(bodyCode, "{\n", "{\n"+valDecl, 1)

		lenExpr := fmt.Sprintf("%s.len() as i32", xCode)
		e.fs.PushCode(fmt.Sprintf("%sfor %s in 0..%s %s\n",
			ind, loopVar, lenExpr, bodyWithDecl))
	} else {
		// Key-only or no-key range — use for..in pattern for correct continue semantics
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = fmt.Sprintf("_i%d", e.rangeVarCounter)
			e.rangeVarCounter++
		}

		lenExpr := fmt.Sprintf("%s.len() as i32", xCode)
		e.fs.PushCode(fmt.Sprintf("%sfor %s in 0..%s %s\n",
			ind, loopVar, lenExpr, bodyCode))
	}
}

// ============================================================
// Switch / Case Statements
// ============================================================

func (e *RustPrimEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	e.indent = indent
}

func (e *RustPrimEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	tagCode := e.fs.ReduceToCode(string(PreVisitSwitchStmtTag))
	e.fs.PushCode(tagCode)
}

func (e *RustPrimEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSwitchStmt))
	ind := rustpIndent(indent / 2)

	tagCode := ""
	idx := 0
	if idx < len(tokens) {
		tagCode = tokens[idx].Content
		idx++
	}

	// Cast small-int match tags to i32 so they match i32 constants
	needsWildcard := false
	if node.Tag != nil {
		tagType := e.getExprGoType(node.Tag)
		if tagType != nil {
			if basic, ok := tagType.Underlying().(*types.Basic); ok {
				switch basic.Kind() {
				case types.Int8, types.Uint8, types.Int16, types.Uint16:
					tagCode = fmt.Sprintf("%s as i32", tagCode)
					// Check if there's already a default case
					hasDefault := false
					if node.Body != nil {
						for _, stmt := range node.Body.List {
							if cc, ok := stmt.(*ast.CaseClause); ok && len(cc.List) == 0 {
								hasDefault = true
								break
							}
						}
					}
					if !hasDefault {
						needsWildcard = true
					}
				}
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%smatch %s {\n", ind, tagCode))
	for i := idx; i < len(tokens); i++ {
		sb.WriteString(tokens[i].Content)
	}
	if needsWildcard {
		sb.WriteString(ind + "_ => {}\n")
	}
	sb.WriteString(ind + "}\n")
	e.fs.PushCode(sb.String())
}

func (e *RustPrimEmitter) PreVisitCaseClause(node *ast.CaseClause, indent int) {
	e.indent = indent
}

func (e *RustPrimEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	exprCode := e.fs.ReduceToCode(string(PreVisitCaseClauseListExpr))
	e.fs.PushCode(exprCode)
}

func (e *RustPrimEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCaseClauseList))
	var exprs []string
	for _, t := range tokens {
		if t.Content != "" {
			exprs = append(exprs, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(exprs, " | "))
}

func (e *RustPrimEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCaseClause))
	ind := rustpIndent(indent / 2)

	var sb strings.Builder
	idx := 0
	if len(node.List) == 0 {
		sb.WriteString(ind + "_ => {\n")
	} else {
		caseExprs := ""
		if idx < len(tokens) {
			caseExprs = tokens[idx].Content
			idx++
		}
		sb.WriteString(fmt.Sprintf("%s%s => {\n", ind, caseExprs))
	}
	for i := idx; i < len(tokens); i++ {
		sb.WriteString(tokens[i].Content)
	}
	sb.WriteString(ind + "}\n")
	e.fs.PushCode(sb.String())
}

// ============================================================
// Inc/Dec Statements
// ============================================================

func (e *RustPrimEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitIncDecStmt))
	ind := rustpIndent(indent / 2)
	if node.Tok == token.INC {
		e.fs.PushCode(fmt.Sprintf("%s%s += 1;\n", ind, xCode))
	} else {
		e.fs.PushCode(fmt.Sprintf("%s%s -= 1;\n", ind, xCode))
	}
}

// ============================================================
// Branch Statements (break, continue)
// ============================================================

func (e *RustPrimEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	ind := rustpIndent(indent / 2)
	switch node.Tok {
	case token.BREAK:
		e.fs.PushCode(ind + "break;\n")
	case token.CONTINUE:
		e.fs.PushCode(ind + "continue;\n")
	}
}
