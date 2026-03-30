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

func (e *RustEmitter) PreVisitExprStmt(node *ast.ExprStmt, indent int) {
	e.indent = indent
}

func (e *RustEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitExprStmtX))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitExprStmt(node *ast.ExprStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitExprStmt))
	ind := rustIndent(indent / 2)
	var children []IRNode
	children = append(children, Leaf(WhiteSpace, ind))
	if len(tokens) >= 1 {
		children = append(children, tokens[0])
	}
	children = append(children, Leaf(Semicolon, ";"), Leaf(NewLine, "\n"))
	e.fs.AddTree(IRTree(ExprStatement, KindStmt, children...))
}

// ============================================================
// Assignment Statements
// ============================================================

func (e *RustEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	e.indent = indent
	e.mapAssignVar = ""
	e.mapAssignKey = ""

	// Track LHS names for move optimization
	if e.Opt.OptimizeMoves && len(node.Lhs) >= 1 && len(node.Rhs) == 1 {
		e.Opt.currentAssignLhsNames = make(map[string]bool)
		for _, lhs := range node.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				e.Opt.currentAssignLhsNames[ident.Name] = true
			}
		}
		// Build call arg ident counts for the RHS call
		if callExpr, ok := node.Rhs[0].(*ast.CallExpr); ok {
			counts := CollectCallArgIdentCounts(callExpr.Args, e.pkg)
			e.Opt.currentCallArgIdentsStack = append(e.Opt.currentCallArgIdentsStack, counts)
		}
		// Analyze for temp extraction and std::mem::take
		e.Opt.analyzeMoveOptExtraction(node)
		e.Opt.analyzeMemTakeOpt(node)
	}
}

func (e *RustEmitter) PreVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	e.inAssignLhs = true
}

func (e *RustEmitter) PostVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	e.inAssignLhs = false
	lhsCode := e.fs.CollectText(string(PreVisitAssignStmtLhsExpr))

	if indexExpr, ok := node.(*ast.IndexExpr); ok {
		if e.isMapTypeExpr(indexExpr.X) {
			e.mapAssignVar = e.lastIndexXCode
			e.mapAssignKey = e.lastIndexKeyCode
			e.fs.AddLeaf(lhsCode, KindExpr, nil)
			return
		}
	}
	e.fs.AddLeaf(lhsCode, KindExpr, nil)
}

func (e *RustEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitAssignStmtLhs))
	var lhsExprs []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			lhsExprs = append(lhsExprs, t.Serialize())
		}
	}
	e.fs.AddLeaf(strings.Join(lhsExprs, ", "), KindExpr, nil)
}

func (e *RustEmitter) PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitAssignStmtRhsExpr))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitAssignStmtRhs))
	var nonEmpty []IRNode
	for _, t := range tokens {
		if t.Serialize() != "" {
			nonEmpty = append(nonEmpty, t)
		}
	}
	if len(nonEmpty) == 1 {
		e.fs.AddTree(nonEmpty[0])
	} else if len(nonEmpty) > 1 {
		var children []IRNode
		for i, t := range nonEmpty {
			if i > 0 {
				children = append(children, IRNode{Type: Comma, Content: ", "})
			}
			children = append(children, t)
		}
		e.fs.AddTree(IRTree(Identifier, KindExpr, children...))
	}
}

func (e *RustEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	// Clean up move optimization tracking
	defer func() {
		e.Opt.currentAssignLhsNames = nil
		if len(e.Opt.currentCallArgIdentsStack) > 0 {
			e.Opt.currentCallArgIdentsStack = e.Opt.currentCallArgIdentsStack[:len(e.Opt.currentCallArgIdentsStack)-1]
		}
		e.Opt.moveOptActive = false
		e.Opt.moveOptModifiedCounts = nil
		e.Opt.moveOptTempExtractions = nil
		e.Opt.memTakeActive = false
		e.Opt.memTakeLhsExpr = ""
		e.Opt.memTakeArgIdx = -1
	}()

	tokens := e.fs.CollectForest(string(PreVisitAssignStmt))
	lhsStr := ""
	rhsStr := ""
	if len(tokens) >= 1 {
		lhsStr = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		rhsStr = tokens[1].Serialize()
	}

	ind := rustIndent(indent / 2)

	// Pointer alias elimination: emit comment instead of assignment
	if len(node.Lhs) == 1 {
		if lhsIdent, ok := node.Lhs[0].(*ast.Ident); ok {
			if comment, ok := PtrLocalComments[lhsIdent.Pos()]; ok {
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(LineComment, comment),
				Leaf(NewLine, "\n"),
			))
				return
			}
		}
	}

	tokStr := node.Tok.String()

	// Local closure inlining for parameterless closures that capture mutable variables.
	// Closures WITH parameters are emitted as normal Rust closures.
	// Parameterless closures (e.g., addToken := func() { ... }) are inlined at the call
	// site to avoid Rust borrow checker issues with captured mutable variables.
	if tokStr == ":=" && len(node.Lhs) == 1 && len(node.Rhs) == 1 {
		if funcLit, isFuncLit := node.Rhs[0].(*ast.FuncLit); isFuncLit {
			hasParams := funcLit.Type.Params != nil && funcLit.Type.Params.NumFields() > 0
			if !hasParams {
				varName := exprToString(node.Lhs[0])
				if e.localClosureBodies == nil {
					e.localClosureBodies = make(map[string]string)
				}
				e.localClosureBodies[varName] = rhsStr
				// Don't emit the assignment
				e.fs.AddTree(IRTree(AssignStatement, KindStmt, Leaf(Identifier, "")))
				return
			}
		}
	}

	// Map-of-slices assignment: map[key][idx] = value (or map[k1][k2][idx] = value)
	// Pattern: the LHS is a slice index, but the slice comes from a map access chain
	if len(node.Lhs) == 1 && len(node.Rhs) == 1 {
		if outerIdx, ok := node.Lhs[0].(*ast.IndexExpr); ok {
			code := e.emitMapSliceAssign(outerIdx, rhsStr, ind)
			if code != "" {
				mapSliceNode := IRTree(AssignStatement, KindStmt, Leaf(Identifier, code))
				mapSliceNode.OptMeta = &OptMeta{Kind: OptMapOp}
				e.fs.AddTree(mapSliceNode)
				e.mapAssignVar = ""
				e.mapAssignKey = ""
				return
			}
		}
	}

	// Map assignment: m[k] = v -> m = hmap::hashMapSet(m, Rc::new(k), Rc::new(v))
	if e.mapAssignVar != "" && e.mapAssignKey != "" {
		code := e.emitMapAssign(node, rhsStr, ind)
		mapAssignNode := IRTree(AssignStatement, KindStmt, Leaf(Identifier, code))
		mapAssignNode.OptMeta = &OptMeta{Kind: OptMapOp}
		e.fs.AddTree(mapAssignNode)
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

				mapRef := mapName + ".clone()"
				mapOptMeta := &OptMeta{Kind: OptMapOp}
				if tokStr == ":=" {
					containsNode := IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						LeafTag(Keyword, "let", TagRust),
						Leaf(WhiteSpace, " "),
						LeafTag(Keyword, "mut", TagRust),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, okName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "hmap::hashMapContains("+mapRef+", "+keyExpr+")"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					)
					containsNode.OptMeta = mapOptMeta
					e.fs.AddTree(containsNode)
					getNode := IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						LeafTag(Keyword, "let", TagRust),
						Leaf(WhiteSpace, " "),
						LeafTag(Keyword, "mut", TagRust),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, valName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(IfKeyword, "if"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, okName),
						Leaf(WhiteSpace, " "),
						Leaf(LeftBrace, "{"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "hmap::hashMapGet("+mapRef+", "+keyExpr+")"+castExpr),
						Leaf(WhiteSpace, " "),
						Leaf(RightBrace, "}"),
						Leaf(WhiteSpace, " "),
						Leaf(ElseKeyword, "else"),
						Leaf(WhiteSpace, " "),
						Leaf(LeftBrace, "{"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, zeroVal),
						Leaf(WhiteSpace, " "),
						Leaf(RightBrace, "}"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					)
					getNode.OptMeta = mapOptMeta
					e.fs.AddTree(getNode)
				} else {
					containsNode := IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, okName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "hmap::hashMapContains("+mapRef+", "+keyExpr+")"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					)
					containsNode.OptMeta = mapOptMeta
					e.fs.AddTree(containsNode)
					getNode := IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, valName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(IfKeyword, "if"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, okName),
						Leaf(WhiteSpace, " "),
						Leaf(LeftBrace, "{"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "hmap::hashMapGet("+mapRef+", "+keyExpr+")"+castExpr),
						Leaf(WhiteSpace, " "),
						Leaf(RightBrace, "}"),
						Leaf(WhiteSpace, " "),
						Leaf(ElseKeyword, "else"),
						Leaf(WhiteSpace, " "),
						Leaf(LeftBrace, "{"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, zeroVal),
						Leaf(WhiteSpace, " "),
						Leaf(RightBrace, "}"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					)
					getNode.OptMeta = mapOptMeta
					e.fs.AddTree(getNode)
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
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					LeafTag(Keyword, "let", TagRust),
					Leaf(WhiteSpace, " "),
					LeafTag(Keyword, "mut", TagRust),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, okName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, xExpr),
					Leaf(Dot, "."),
					Leaf(Identifier, "downcast_ref"),
					Leaf(Identifier, "::"),
					Leaf(LeftAngle, "<"),
					Leaf(TypeKeyword, assertType),
					Leaf(RightAngle, ">"),
					Leaf(Identifier, "().is_some()"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
				// Build let binding for the value; skip "mut" for blank identifier "_"
				valLetNodes := []IRNode{
					Leaf(WhiteSpace, ind),
					LeafTag(Keyword, "let", TagRust),
					Leaf(WhiteSpace, " "),
				}
				if valName != "_" {
					valLetNodes = append(valLetNodes, LeafTag(Keyword, "mut", TagRust), Leaf(WhiteSpace, " "))
				}
				valLetNodes = append(valLetNodes,
					Leaf(Identifier, valName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(IfKeyword, "if"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, okName),
					Leaf(WhiteSpace, " "),
					Leaf(LeftBrace, "{"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, xExpr),
					Leaf(Dot, "."),
					Leaf(Identifier, "downcast_ref"),
					Leaf(Identifier, "::"),
					Leaf(LeftAngle, "<"),
					Leaf(TypeKeyword, assertType),
					Leaf(RightAngle, ">"),
					Leaf(Identifier, "().unwrap().clone()"),
					Leaf(WhiteSpace, " "),
					Leaf(RightBrace, "}"),
					Leaf(WhiteSpace, " "),
					Leaf(ElseKeyword, "else"),
					Leaf(WhiteSpace, " "),
					Leaf(LeftBrace, "{"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, zeroVal),
					Leaf(WhiteSpace, " "),
					Leaf(RightBrace, "}"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				)
				e.fs.AddTree(IRTree(AssignStatement, KindStmt, valLetNodes...))
			} else {
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, okName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, xExpr),
					Leaf(Dot, "."),
					Leaf(Identifier, "downcast_ref"),
					Leaf(Identifier, "::"),
					Leaf(LeftAngle, "<"),
					Leaf(TypeKeyword, assertType),
					Leaf(RightAngle, ">"),
					Leaf(Identifier, "().is_some()"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, valName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(IfKeyword, "if"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, okName),
					Leaf(WhiteSpace, " "),
					Leaf(LeftBrace, "{"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, xExpr),
					Leaf(Dot, "."),
					Leaf(Identifier, "downcast_ref"),
					Leaf(Identifier, "::"),
					Leaf(LeftAngle, "<"),
					Leaf(TypeKeyword, assertType),
					Leaf(RightAngle, ">"),
					Leaf(Identifier, "().unwrap().clone()"),
					Leaf(WhiteSpace, " "),
					Leaf(RightBrace, "}"),
					Leaf(WhiteSpace, " "),
					Leaf(ElseKeyword, "else"),
					Leaf(WhiteSpace, " "),
					Leaf(LeftBrace, "{"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, zeroVal),
					Leaf(WhiteSpace, " "),
					Leaf(RightBrace, "}"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
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
			var rhsNode IRNode
			if len(tokens) >= 2 {
				rhsNode = tokens[1]
			} else {
				rhsNode = Leaf(Identifier, rhsStr)
			}
			assignNode := IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				LeafTag(Keyword, "let", TagRust),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, destructured),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				rhsNode,
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
			if e.Opt.moveOptActive && len(e.Opt.moveOptTempExtractions) > 0 {
				assignNode.OptMeta = &OptMeta{Kind: OptAssignment, TempExtractions: e.Opt.moveOptTempExtractions}
			}
			e.fs.AddTree(assignNode)
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
			var rhsNode IRNode
			if len(tokens) >= 2 {
				rhsNode = tokens[1]
			} else {
				rhsNode = Leaf(Identifier, rhsStr)
			}
			assignNode := IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, destructured),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				rhsNode,
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
			if e.Opt.moveOptActive && len(e.Opt.moveOptTempExtractions) > 0 {
				assignNode.OptMeta = &OptMeta{Kind: OptAssignment, TempExtractions: e.Opt.moveOptTempExtractions}
			}
			e.fs.AddTree(assignNode)
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

	// Start with tree-preserved RHS node (keeps OptCallArg annotations)
	rhsNode := Leaf(Identifier, rhsStr)
	if len(tokens) >= 2 {
		rhsNode = tokens[1]
	}

	// Cast RHS for small-int LHS fields (handles untyped Go constants emitted as i32)
	// Apply as tree wrapper to preserve inner OptCallArg annotations
	if len(node.Lhs) == 1 && len(node.Rhs) == 1 && !needsRcWrap {
		lhsType := e.getExprGoType(node.Lhs[0])
		if lhsType != nil {
			castStr := e.castSmallIntFieldValue(lhsType, rhsStr)
			if castStr != rhsStr {
				rhsStr = castStr
				rhsNode = e.wrapSmallIntCastTree(lhsType, rhsNode)
			}
		}
	}

	// Clone RHS for non-Copy identifiers in := and = assignments to prevent move
	if len(node.Lhs) == 1 && len(node.Rhs) == 1 && !needsRcWrap && (tokStr == ":=" || tokStr == "=") {
		if ident, ok := node.Rhs[0].(*ast.Ident); ok && ident.Name != "nil" && ident.Name != "true" && ident.Name != "false" {
			rhsType := e.getExprGoType(node.Rhs[0])
			if rhsType != nil && !isCopyType(rhsType) {
				if !strings.HasSuffix(rhsStr, ".clone()") {
					rhsStr = rhsStr + ".clone()"
					rhsNode = Leaf(Identifier, rhsStr)
				}
			}
		}
	}

	// Clone RHS for non-Copy selector expressions (struct field access) to prevent partial move
	if len(node.Lhs) == 1 && len(node.Rhs) == 1 && !needsRcWrap && (tokStr == ":=" || tokStr == "=") {
		if _, ok := node.Rhs[0].(*ast.SelectorExpr); ok {
			rhsType := e.getExprGoType(node.Rhs[0])
			if rhsType != nil && !isCopyType(rhsType) {
				if !strings.HasSuffix(rhsStr, ".clone()") {
					rhsStr = rhsStr + ".clone()"
					rhsNode = Leaf(Identifier, rhsStr)
				}
			}
		}
	}

	if needsRcWrap {
		wrappedRhs := "Rc::new(" + rhsStr + ")"
		switch tokStr {
		case ":=":
			e.fs.AddTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				LeafTag(Keyword, "let", TagRust),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "mut", TagRust),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, lhsStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, wrappedRhs),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		default:
			e.fs.AddTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, lhsStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, wrappedRhs),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		}
	} else {
		switch tokStr {
		case ":=":
			e.fs.AddTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				LeafTag(Keyword, "let", TagRust),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "mut", TagRust),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, lhsStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				rhsNode,
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
			// For string +=, RHS must be &str, so add & prefix
			if tokStr == "+=" && len(node.Lhs) == 1 {
				lhsType := e.getExprGoType(node.Lhs[0])
				if lhsType != nil {
					if basic, ok := lhsType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
						e.fs.AddTree(IRTree(AssignStatement, KindStmt,
							Leaf(WhiteSpace, ind),
							Leaf(Identifier, lhsStr),
							Leaf(WhiteSpace, " "),
							Leaf(ArithmeticOperator, "+="),
							Leaf(WhiteSpace, " "),
							Leaf(UnaryOperator, "&"),
							rhsNode,
							Leaf(Semicolon, ";"),
							Leaf(NewLine, "\n"),
						))
						return
					}
				}
			}
			e.fs.AddTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, lhsStr),
				Leaf(WhiteSpace, " "),
				Leaf(ArithmeticOperator, tokStr),
				Leaf(WhiteSpace, " "),
				rhsNode,
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		default:
			assignNode := IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, lhsStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				rhsNode,
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
			if e.Opt.moveOptActive && len(e.Opt.moveOptTempExtractions) > 0 {
				assignNode.OptMeta = &OptMeta{Kind: OptAssignment, TempExtractions: e.Opt.moveOptTempExtractions}
			}
			e.fs.AddTree(assignNode)
		}
	}
}

// ============================================================
// Declaration Statements (var x int, var y = 5)
// ============================================================

func (e *RustEmitter) PreVisitDeclStmt(node *ast.DeclStmt, indent int) {
	e.indent = indent
}

func (e *RustEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitDeclStmtValueSpecType))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Serialize()
	}
	var goType types.Type
	if e.pkg != nil && e.pkg.TypesInfo != nil && index < len(node.Names) {
		if obj := e.pkg.TypesInfo.Defs[node.Names[index]]; obj != nil {
			goType = obj.Type()
		}
	}
	e.fs.AddLeaf(typeStr, TagType, goType)
}

func (e *RustEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	e.fs.CollectForest(string(PreVisitDeclStmtValueSpecNames))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *RustEmitter) PostVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitDeclStmtValueSpecValue))
	if len(tokens) == 1 {
		t := tokens[0]
		t.Kind = TagExpr
		e.fs.AddTree(t)
	} else {
		valCode := ""
		for _, t := range tokens {
			valCode += t.Serialize()
		}
		e.fs.AddLeaf(valCode, TagExpr, nil)
	}
}

func (e *RustEmitter) PostVisitDeclStmt(node *ast.DeclStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitDeclStmt))
	ind := rustIndent(indent / 2)

	var children []IRNode
	i := 0
	for i < len(tokens) {
		typeStr := ""
		var goType types.Type
		nameStr := ""
		valueStr := ""
		var valueToken IRNode

		if i < len(tokens) && tokens[i].Kind == TagType {
			typeStr = tokens[i].Serialize()
			goType = tokens[i].GoType
			i++
		}
		if i < len(tokens) && tokens[i].Kind == TagIdent {
			nameStr = tokens[i].Serialize()
			i++
		}
		if i < len(tokens) && tokens[i].Kind == TagExpr {
			valueStr = tokens[i].Serialize()
			valueToken = tokens[i]
			i++
		}

		if nameStr == "" {
			continue
		}

		escapedName := escapeRustKeyword(nameStr)

		if valueStr != "" {
			children = append(children,
				Leaf(WhiteSpace, ind),
				LeafTag(Keyword, "let", TagRust),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "mut", TagRust),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, escapedName),
				Leaf(Colon, ":"),
				Leaf(WhiteSpace, " "),
				Leaf(TypeKeyword, typeStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				valueToken,
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
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
			children = append(children,
				Leaf(WhiteSpace, ind),
				LeafTag(Keyword, "let", TagRust),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "mut", TagRust),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, escapedName),
				Leaf(Colon, ":"),
				Leaf(WhiteSpace, " "),
				Leaf(TypeKeyword, typeStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, defaultVal),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		}
	}
	if len(children) > 0 {
		e.fs.AddTree(IRTree(DeclStatement, KindStmt, children...))
	} else {
		e.fs.AddTree(IRTree(DeclStatement, KindStmt, Leaf(Identifier, "")))
	}
}

// ============================================================
// Return Statements
// ============================================================

func (e *RustEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	e.indent = indent
	e.Opt.returnTempReplacements = nil
	e.Opt.returnTempPreamble = nil

	// Return temp extraction: when the first return result is an identifier and
	// later results reference it, extract those later results into temp variables
	// so the first result can be moved instead of cloned.
	if e.Opt.OptimizeMoves && len(node.Results) > 1 {
		if ident, ok := node.Results[0].(*ast.Ident); ok {
			ind := rustIndent(indent / 2)
			replacements := make(map[int]string)
			tempIdx := 0
			var preambleTokens []IRNode
			for i := 1; i < len(node.Results); i++ {
				if !ExprContainsIdent(node.Results[i], ident.Name) {
					continue
				}
				// Check if the result type is Copy
				tv := e.pkg.TypesInfo.Types[node.Results[i]]
				if tv.Type == nil || !isCopyType(tv.Type) {
					continue
				}
				basic, isBasic := tv.Type.Underlying().(*types.Basic)
				if !isBasic {
					continue
				}
				exprStr := e.Opt.exprToRustCodeOpt(node.Results[i])
				if exprStr == "" {
					continue
				}
				tempName := fmt.Sprintf("__mv%d", tempIdx)
				tempIdx++
				rustType := e.Opt.goTypeToRust(basic.Name())
				preambleTokens = append(preambleTokens,
					Leaf(WhiteSpace, ind),
					LeafTag(Keyword, "let", TagRust),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, tempName),
					Leaf(Colon, ":"),
					Leaf(WhiteSpace, " "),
					Leaf(TypeKeyword, rustType),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, exprStr),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				)
				replacements[i] = tempName
			}
			if len(replacements) > 0 {
				e.Opt.returnTempReplacements = replacements
				e.Opt.returnTempPreamble = preambleTokens
			}
		}
	}
}

func (e *RustEmitter) PostVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitReturnStmtResult))

	// If returning from a function that returns interface{}/any, wrap concrete types in Rc::new()
	if e.funcReturnType != nil && node != nil && e.pkg != nil && e.pkg.TypesInfo != nil {
		retTypeStr := e.funcReturnType.String()
		if retTypeStr == "interface{}" || retTypeStr == "any" {
			nodeTypeInfo := e.pkg.TypesInfo.Types[node]
			if nodeTypeInfo.Type != nil {
				nodeTypeStr := nodeTypeInfo.Type.String()
				if nodeTypeStr != "interface{}" && nodeTypeStr != "any" {
					var resultCode string
					for _, t := range tokens {
						resultCode += t.Serialize()
					}
					e.fs.AddLeaf(fmt.Sprintf("Rc::new(%s)", resultCode), KindExpr, nil)
					return
				}
			}
		}
	}

	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitReturnStmt))
	ind := rustIndent(indent / 2)

	if len(tokens) == 0 {
		e.fs.AddTree(IRTree(ReturnStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	} else if len(tokens) == 1 {
		e.fs.AddTree(IRTree(ReturnStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return"),
			Leaf(WhiteSpace, " "),
			tokens[0],
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	} else {
		// For multi-value returns, annotate non-Copy values for CloneMovePass
		var valNodes []IRNode

		// Return temp extraction: try to extract later results that reference the first
		// into temp variables so the first result can be moved instead of cloned.
		returnTempReplacements := e.Opt.returnTempReplacements
		returnTempPreamble := e.Opt.returnTempPreamble
		e.Opt.returnTempReplacements = nil
		e.Opt.returnTempPreamble = nil

		for i, t := range tokens {
			// If this result was extracted to a temp, use the temp name
			if returnTempReplacements != nil {
				if tempName, ok := returnTempReplacements[i]; ok {
					valNodes = append(valNodes, Leaf(Identifier, tempName))
					continue
				}
			}
			if i == 0 && len(node.Results) > 1 {
				// Annotate the first return value if it's a non-Copy type
				if len(node.Results) > 0 {
					resultType := e.getExprGoType(node.Results[i])
					if resultType != nil && !isCopyType(resultType) {
						needsClone := true
						if e.Opt.OptimizeMoves {
							firstName := ""
							if ident, ok := node.Results[0].(*ast.Ident); ok {
								firstName = ident.Name
							}
							if firstName != "" {
								referencesFirst := false
								for j := 1; j < len(node.Results); j++ {
									if ExprContainsIdent(node.Results[j], firstName) {
										referencesFirst = true
										break
									}
								}
								if !referencesFirst {
									needsClone = false
									e.Opt.MoveOptCount++
								}
							}
							if needsClone && returnTempReplacements != nil {
								allExtracted := true
								firstName := ""
								if ident, ok := node.Results[0].(*ast.Ident); ok {
									firstName = ident.Name
								}
								if firstName != "" {
									for j := 1; j < len(node.Results); j++ {
										if ExprContainsIdent(node.Results[j], firstName) {
											if _, replaced := returnTempReplacements[j]; !replaced {
												allExtracted = false
												break
											}
										}
									}
								}
								if allExtracted {
									needsClone = false
									e.Opt.MoveOptCount++
								}
							}
						}
						if needsClone {
							// Annotate for CloneMovePass instead of inline .clone()
							t.OptMeta = &OptMeta{Kind: OptReturnValue, NeedReturnCopy: true}
						}
					}
				}
			}
			valNodes = append(valNodes, t)
		}
		// Emit temp bindings before the return statement (e.g., let __mv0 = expr;)
		if len(returnTempPreamble) > 0 {
			e.fs.AddTree(IRTree(ReturnStatement, KindStmt, returnTempPreamble...))
		}
		children := []IRNode{
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
		}
		for i, vn := range valNodes {
			if i > 0 {
				children = append(children, Leaf(Comma, ","), Leaf(WhiteSpace, " "))
			}
			children = append(children, vn)
		}
		children = append(children,
			Leaf(RightParen, ")"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		)
		e.fs.AddTree(IRTree(ReturnStatement, KindStmt, children...))
	}
}

// ============================================================
// If Statements
// ============================================================

func (e *RustEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	e.indent = indent
	e.ifInitStack = append(e.ifInitStack, "")
	e.ifCondStack = append(e.ifCondStack, "")
	e.ifBodyStack = append(e.ifBodyStack, "")
	e.ifElseStack = append(e.ifElseStack, "")
	e.ifInitNodes = append(e.ifInitNodes, IRNode{})
	e.ifCondNodes = append(e.ifCondNodes, IRNode{})
	e.ifBodyNodes = append(e.ifBodyNodes, IRNode{})
	e.ifElseNodes = append(e.ifElseNodes, IRNode{})
}

func (e *RustEmitter) PostVisitIfStmtInit(node ast.Stmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIfStmtInit))
	e.ifInitNodes[len(e.ifInitNodes)-1] = collectToNode(tokens)
	e.ifInitStack[len(e.ifInitStack)-1] = e.ifInitNodes[len(e.ifInitNodes)-1].Serialize()
}

func (e *RustEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIfStmtCond))
	e.ifCondNodes[len(e.ifCondNodes)-1] = collectToNode(tokens)
	e.ifCondStack[len(e.ifCondStack)-1] = e.ifCondNodes[len(e.ifCondNodes)-1].Serialize()
}

func (e *RustEmitter) PostVisitIfStmtBody(node *ast.IfStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIfStmtBody))
	e.ifBodyNodes[len(e.ifBodyNodes)-1] = collectToNode(tokens)
	e.ifBodyStack[len(e.ifBodyStack)-1] = e.ifBodyNodes[len(e.ifBodyNodes)-1].Serialize()
}

func (e *RustEmitter) PostVisitIfStmtElse(node *ast.IfStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIfStmtElse))
	e.ifElseNodes[len(e.ifElseNodes)-1] = collectToNode(tokens)
	e.ifElseStack[len(e.ifElseStack)-1] = e.ifElseNodes[len(e.ifElseNodes)-1].Serialize()
}

func (e *RustEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	e.fs.CollectForest(string(PreVisitIfStmt))
	ind := rustIndent(indent / 2)

	n := len(e.ifInitStack)
	initCode := e.ifInitStack[n-1]
	_ = e.ifCondStack[n-1]
	_ = e.ifBodyStack[n-1]
	elseCode := e.ifElseStack[n-1]
	initNode := e.ifInitNodes[n-1]
	condNode := e.ifCondNodes[n-1]
	bodyNode := e.ifBodyNodes[n-1]
	elseNode := e.ifElseNodes[n-1]
	e.ifInitStack = e.ifInitStack[:n-1]
	e.ifCondStack = e.ifCondStack[:n-1]
	e.ifBodyStack = e.ifBodyStack[:n-1]
	e.ifElseStack = e.ifElseStack[:n-1]
	e.ifInitNodes = e.ifInitNodes[:n-1]
	e.ifCondNodes = e.ifCondNodes[:n-1]
	e.ifBodyNodes = e.ifBodyNodes[:n-1]
	e.ifElseNodes = e.ifElseNodes[:n-1]

	var children []IRNode
	if initCode != "" {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(LeftBrace, "{"),
			Leaf(NewLine, "\n"),
			initNode,
			Leaf(WhiteSpace, ind),
			Leaf(IfKeyword, "if"),
			Leaf(WhiteSpace, " "),
			condNode,
			Leaf(WhiteSpace, " "),
			bodyNode,
		)
	} else {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(IfKeyword, "if"),
			Leaf(WhiteSpace, " "),
			condNode,
			Leaf(WhiteSpace, " "),
			bodyNode,
		)
	}
	if elseCode != "" {
		trimmed := strings.TrimLeft(elseCode, " \t\n")
		if strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "if(") {
			// Else-if chain: strip leading whitespace from the inner IfStmt tree
			elseIfNode := stripLeadingWhitespace(elseNode)
			children = append(children,
				Leaf(WhiteSpace, " "),
				Leaf(ElseKeyword, "else"),
				Leaf(WhiteSpace, " "),
				elseIfNode,
			)
		} else {
			children = append(children,
				Leaf(WhiteSpace, " "),
				Leaf(ElseKeyword, "else"),
				Leaf(WhiteSpace, " "),
				elseNode,
			)
		}
	}
	children = append(children, Leaf(NewLine, "\n"))
	if initCode != "" {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(RightBrace, "}"),
			Leaf(NewLine, "\n"),
		)
	}
	e.fs.AddTree(IRTree(IfStatement, KindStmt, children...))
}

// ============================================================
// For Statements
// ============================================================

func (e *RustEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	e.indent = indent
	e.forInitStack = append(e.forInitStack, "")
	e.forCondStack = append(e.forCondStack, "")
	e.forPostStack = append(e.forPostStack, "")
	e.forCondNodes = append(e.forCondNodes, IRNode{})
	e.forBodyNodes = append(e.forBodyNodes, IRNode{})
}

func (e *RustEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitForStmtInit))
	initCode := collectToNode(tokens).Serialize()
	initCode = strings.TrimRight(initCode, ";\n \t")
	initCode = strings.TrimLeft(initCode, " \t")
	e.forInitStack[len(e.forInitStack)-1] = initCode
}

func (e *RustEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitForStmtCond))
	e.forCondNodes[len(e.forCondNodes)-1] = collectToNode(tokens)
	e.forCondStack[len(e.forCondStack)-1] = e.forCondNodes[len(e.forCondNodes)-1].Serialize()
}

func (e *RustEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitForStmtPost))
	postCode := collectToNode(tokens).Serialize()
	postCode = strings.TrimRight(postCode, ";\n \t")
	postCode = strings.TrimLeft(postCode, " \t")
	e.forPostStack[len(e.forPostStack)-1] = postCode
}

func (e *RustEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {
	bodyTokens := e.fs.CollectForest(string(PreVisitForStmt))
	bodyNode := collectToNode(bodyTokens)
	ind := rustIndent(indent / 2)

	n := len(e.forInitStack)
	initCode := e.forInitStack[n-1]
	condCode := e.forCondStack[n-1]
	condNode := e.forCondNodes[n-1]
	postCode := e.forPostStack[n-1]
	e.forInitStack = e.forInitStack[:n-1]
	e.forCondStack = e.forCondStack[:n-1]
	e.forPostStack = e.forPostStack[:n-1]
	e.forCondNodes = e.forCondNodes[:n-1]
	e.forBodyNodes = e.forBodyNodes[:n-1]

	// Infinite loop: for {}
	if node.Init == nil && node.Cond == nil && node.Post == nil {
		e.fs.AddTree(IRTree(ForStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "loop"),
			Leaf(WhiteSpace, " "),
			bodyNode,
			Leaf(NewLine, "\n"),
		))
		return
	}

	// Condition-only loop: for cond {}
	if node.Init == nil && node.Post == nil && node.Cond != nil {
		e.fs.AddTree(IRTree(ForStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(WhileKeyword, "while"),
			Leaf(WhiteSpace, " "),
			condNode,
			Leaf(WhiteSpace, " "),
			bodyNode,
			Leaf(NewLine, "\n"),
		))
		return
	}

	// Try to detect simple integer range pattern: for i := start; i < end; i++ (or variants)
	if rangeToken := e.tryEmitForRange(node, ind, bodyNode, initCode); rangeToken.Content != "" {
		e.fs.AddTree(rangeToken)
		return
	}

	// Full for loop that doesn't match a range pattern: emit as loop with manual control
	// Use __first_iter guard so that `continue` goes to top of loop which executes post
	// Pattern: { init; let mut __first = true; loop { if !__first { post; } __first = false; if !cond { break; } body; } }
	if condCode == "" {
		condCode = "true"
		condNode = Leaf(Identifier, "true")
	}
	ind4 := ind + "    "
	ind8 := ind + "        "
	ind12 := ind + "            "
	var children []IRNode
	children = append(children,
		Leaf(WhiteSpace, ind),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
	)
	if initCode != "" {
		children = append(children,
			Leaf(WhiteSpace, ind4),
			Leaf(Identifier, initCode),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		)
	}
	children = append(children,
		Leaf(WhiteSpace, ind4),
		LeafTag(Keyword, "let", TagRust),
		Leaf(WhiteSpace, " "),
		LeafTag(Keyword, "mut", TagRust),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "__first_iter"),
		Leaf(WhiteSpace, " "),
		Leaf(Assignment, "="),
		Leaf(WhiteSpace, " "),
		Leaf(BooleanLiteral, "true"),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, ind4),
		Leaf(ForKeyword, "loop"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
	)
	if postCode != "" {
		children = append(children,
			Leaf(WhiteSpace, ind8),
			Leaf(IfKeyword, "if"),
			Leaf(WhiteSpace, " "),
			Leaf(UnaryOperator, "!"),
			Leaf(Identifier, "__first_iter"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftBrace, "{"),
			Leaf(NewLine, "\n"),
			Leaf(WhiteSpace, ind12),
			Leaf(Identifier, postCode),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
			Leaf(WhiteSpace, ind8),
			Leaf(RightBrace, "}"),
			Leaf(NewLine, "\n"),
		)
	}
	children = append(children,
		Leaf(WhiteSpace, ind8),
		Leaf(Identifier, "__first_iter"),
		Leaf(WhiteSpace, " "),
		Leaf(Assignment, "="),
		Leaf(WhiteSpace, " "),
		Leaf(BooleanLiteral, "false"),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, ind8),
		Leaf(IfKeyword, "if"),
		Leaf(WhiteSpace, " "),
		Leaf(UnaryOperator, "!"),
		Leaf(LeftParen, "("),
		condNode,
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(WhiteSpace, " "),
		Leaf(BreakKeyword, "break"),
		Leaf(Semicolon, ";"),
		Leaf(WhiteSpace, " "),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
	)
	// Extract body children (preserve tree structure for pass-based optimization)
	bodyChildren := extractBlockChildren(bodyNode)
	children = append(children, bodyChildren...)
	children = append(children,
		Leaf(WhiteSpace, ind4),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, ind),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
	)
	e.fs.AddTree(IRTree(ForStatement, KindStmt, children...))
}

// tryEmitForRange detects simple integer range patterns and emits Rust for..in syntax.
// Returns a IRNode with children, or a zero IRNode if the pattern doesn't match.
func (e *RustEmitter) tryEmitForRange(node *ast.ForStmt, ind string, bodyNode IRNode, initCode string) IRNode {
	// Must have init, cond, and post
	if node.Init == nil || node.Cond == nil || node.Post == nil {
		return IRNode{}
	}

	// Init must be an assignment: i := start
	assignStmt, ok := node.Init.(*ast.AssignStmt)
	if !ok || len(assignStmt.Lhs) != 1 || len(assignStmt.Rhs) != 1 || assignStmt.Tok != token.DEFINE {
		return IRNode{}
	}
	loopIdent, ok := assignStmt.Lhs[0].(*ast.Ident)
	if !ok {
		return IRNode{}
	}
	loopVar := escapeRustKeyword(loopIdent.Name)

	// Get start value — accept literals, identifiers, or other simple expressions
	startVal := e.exprToRustCode(assignStmt.Rhs[0])
	if startVal == "" {
		return IRNode{}
	}

	// Cond must be binary expr: i < end, i <= end, i > end, i >= end
	condBin, ok := node.Cond.(*ast.BinaryExpr)
	if !ok {
		return IRNode{}
	}
	condIdent, ok := condBin.X.(*ast.Ident)
	if !ok || condIdent.Name != loopIdent.Name {
		return IRNode{}
	}

	// Get end expression as code
	// We need the end expression as a Rust string - reuse condCode's right side
	endCode := e.exprToRustCode(condBin.Y)
	if endCode == "" {
		return IRNode{}
	}

	// Post must be inc/dec statement or assign with +=/-=
	stepVal := ""
	isIncrement := false
	isDecrement := false

	if incDec, ok := node.Post.(*ast.IncDecStmt); ok {
		postIdent, ok := incDec.X.(*ast.Ident)
		if !ok || postIdent.Name != loopIdent.Name {
			return IRNode{}
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
			return IRNode{}
		}
		postIdent, ok := assignPost.Lhs[0].(*ast.Ident)
		if !ok || postIdent.Name != loopIdent.Name {
			return IRNode{}
		}
		rhsLit, ok := assignPost.Rhs[0].(*ast.BasicLit)
		if !ok || rhsLit.Kind != token.INT {
			return IRNode{}
		}
		if assignPost.Tok == token.ADD_ASSIGN {
			isIncrement = true
			stepVal = rhsLit.Value
		} else if assignPost.Tok == token.SUB_ASSIGN {
			isDecrement = true
			stepVal = rhsLit.Value
		} else {
			return IRNode{}
		}
	} else {
		return IRNode{}
	}

	// Helper to build a for..in IRTree
	buildForIn := func(rangeExpr string) IRNode {
		return IRTree(TypeKeyword, TagExpr,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, loopVar),
			Leaf(WhiteSpace, " "),
			LeafTag(Keyword, "in", TagRust),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, rangeExpr),
			Leaf(WhiteSpace, " "),
			bodyNode,
			Leaf(NewLine, "\n"),
		)
	}

	// Now compose the Rust range expression
	if isIncrement {
		if condBin.Op == token.LSS {
			// for i := start; i < end; i++ → for i in start..end
			rangeExpr := startVal + ".." + endCode
			if stepVal != "1" {
				rangeExpr = "(" + rangeExpr + ").step_by(" + stepVal + ")"
			}
			return buildForIn(rangeExpr)
		} else if condBin.Op == token.LEQ {
			// for i := start; i <= end; i++ → for i in start..=end
			rangeExpr := startVal + "..=" + endCode
			if stepVal != "1" {
				rangeExpr = "(" + rangeExpr + ").step_by(" + stepVal + ")"
			}
			return buildForIn(rangeExpr)
		}
	} else if isDecrement {
		if condBin.Op == token.GTR {
			// for i := start; i > end; i-- → for i in (end+1..=start).rev()
			rangeExpr := "(" + endCode + "+1..=" + startVal + ").rev()"
			if stepVal != "1" {
				rangeExpr = "(" + endCode + "+1..=" + startVal + ").rev().step_by(" + stepVal + ")"
			}
			return buildForIn(rangeExpr)
		} else if condBin.Op == token.GEQ {
			// for i := start; i >= end; i-- → for i in (end..=start).rev()
			rangeExpr := "(" + endCode + "..=" + startVal + ").rev()"
			if stepVal != "1" {
				rangeExpr = "(" + endCode + "..=" + startVal + ").rev().step_by(" + stepVal + ")"
			}
			return buildForIn(rangeExpr)
		}
	}

	return IRNode{}
}

// exprToRustCode converts a simple AST expression to a Rust code string.
func (e *RustEmitter) exprToRustCode(expr ast.Expr) string {
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

// ============================================================
// Range Statements
// ============================================================

func (e *RustEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	e.indent = indent
}

func (e *RustEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitRangeStmtKey))
	for _, t := range tokens {
		t.Kind = TagIdent
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitRangeStmtValue))
	for _, t := range tokens {
		t.Kind = TagIdent
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitRangeStmtX))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitRangeStmt))
	ind := rustIndent(indent / 2)

	keyCode := ""
	valCode := ""
	xCode := ""
	bodyNode := Leaf(Identifier, "")

	idx := 0
	if node.Key != nil {
		if idx < len(tokens) && tokens[idx].Kind == TagIdent {
			keyCode = tokens[idx].Serialize()
			idx++
		}
	}
	if node.Value != nil {
		if idx < len(tokens) && tokens[idx].Kind == TagIdent {
			valCode = tokens[idx].Serialize()
			idx++
		}
	}
	if idx < len(tokens) {
		xCode = tokens[idx].Serialize()
		idx++
	}
	if idx < len(tokens) {
		bodyNode = tokens[idx]
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
		keyType := "Rc<dyn Any>"
		if mapGoType != nil {
			if mapUnderlying, ok := mapGoType.Underlying().(*types.Map); ok {
				keyCast = getRustKeyCast(mapUnderlying.Key())
				keyIsStr = isRustStringKey(mapUnderlying.Key())
				valType = e.qualifiedRustTypeName(mapUnderlying.Elem())
				keyType = e.qualifiedRustTypeName(mapUnderlying.Key())
			}
		}
		_ = keyCast
		_ = keyIsStr
		keysVar := fmt.Sprintf("_keys%d", e.rangeVarCounter)
		loopIdx := fmt.Sprintf("_mi%d", e.rangeVarCounter)
		e.rangeVarCounter++

		castExpr := ""
		if valType != "Rc<dyn Any>" {
			castExpr = ".downcast_ref::<" + valType + ">().unwrap().clone()"
		}

		keyCastExpr := ""
		if keyType != "Rc<dyn Any>" {
			keyCastExpr = ".downcast_ref::<" + keyType + ">().unwrap().clone()"
		}

		mapKeysRef := xCode + ".clone()"
		ind4 := ind + "    "
		ind8 := ind + "        "
		var children []IRNode
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(LeftBrace, "{"),
			Leaf(NewLine, "\n"),
			// let keysVar = hmap::hashMapKeys(mapKeysRef);
			Leaf(WhiteSpace, ind4),
			LeafTag(Keyword, "let", TagRust),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, keysVar),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, "="),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "hmap::hashMapKeys("+mapKeysRef+")"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
			// let mut loopIdx: i32 = 0;
			Leaf(WhiteSpace, ind4),
			LeafTag(Keyword, "let", TagRust),
			Leaf(WhiteSpace, " "),
			LeafTag(Keyword, "mut", TagRust),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, loopIdx),
			Leaf(Colon, ":"),
			Leaf(WhiteSpace, " "),
			Leaf(TypeKeyword, "i32"),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, "="),
			Leaf(WhiteSpace, " "),
			Leaf(NumberLiteral, "0"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
			// while (loopIdx as usize) < keysVar.len() {
			Leaf(WhiteSpace, ind4),
			Leaf(WhileKeyword, "while"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			Leaf(Identifier, loopIdx),
			Leaf(WhiteSpace, " "),
			LeafTag(Keyword, "as", TagRust),
			Leaf(WhiteSpace, " "),
			Leaf(TypeKeyword, "usize"),
			Leaf(RightParen, ")"),
			Leaf(WhiteSpace, " "),
			Leaf(ComparisonOperator, "<"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, keysVar),
			Leaf(Dot, "."),
			Leaf(Identifier, "len()"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftBrace, "{"),
			Leaf(NewLine, "\n"),
		)
		if keyCode != "_" && keyCode != "" {
			children = append(children,
				Leaf(WhiteSpace, ind8),
				LeafTag(Keyword, "let", TagRust),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "mut", TagRust),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, keyCode),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, keysVar+"["+loopIdx+" as usize].clone()"+keyCastExpr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		}
		if valCode != "_" && valCode != "" {
			children = append(children,
				Leaf(WhiteSpace, ind8),
				LeafTag(Keyword, "let", TagRust),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "mut", TagRust),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, valCode),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "hmap::hashMapGet(&"+xCode+", "+keysVar+"["+loopIdx+" as usize].clone())"+castExpr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		}
		// Inject body
		children = append(children,
			Leaf(WhiteSpace, ind8),
			bodyNode,
			Leaf(NewLine, "\n"),
			// loopIdx += 1;
			Leaf(WhiteSpace, ind8),
			Leaf(Identifier, loopIdx),
			Leaf(WhiteSpace, " "),
			Leaf(ArithmeticOperator, "+="),
			Leaf(WhiteSpace, " "),
			Leaf(NumberLiteral, "1"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
			Leaf(WhiteSpace, ind4),
			Leaf(RightBrace, "}"),
			Leaf(NewLine, "\n"),
			Leaf(WhiteSpace, ind),
			Leaf(RightBrace, "}"),
			Leaf(NewLine, "\n"),
		)
		rangeNode := IRTree(RangeStatement, KindStmt, children...)
		rangeNode.OptMeta = &OptMeta{Kind: OptMapOp}
		e.fs.AddTree(rangeNode)
		return
	}

	// If range expression is an inline composite literal, emit a temp variable
	if _, isCompLit := node.X.(*ast.CompositeLit); isCompLit {
		tmpVar := fmt.Sprintf("_range%d", e.rangeVarCounter)
		e.rangeVarCounter++
		ind4 := ind + "    "
		var children []IRNode
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(LeftBrace, "{"),
			Leaf(NewLine, "\n"),
			// let tmpVar = xCode;
			Leaf(WhiteSpace, ind4),
			LeafTag(Keyword, "let", TagRust),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, tmpVar),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, "="),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, xCode),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		)
		xCode = tmpVar
		lenExpr := xCode + ".len() as i32"
		if valCode != "" && valCode != "_" {
			loopVar := keyCode
			if loopVar == "_" || loopVar == "" {
				loopVar = fmt.Sprintf("_i%d", e.rangeVarCounter)
				e.rangeVarCounter++
			}
			var valDecl string
			if isString {
				valDecl = fmt.Sprintf("%s        let mut %s = %s.as_bytes()[%s as usize] as i8;\n", ind, valCode, xCode, loopVar)
			} else {
				valDecl = fmt.Sprintf("%s        let mut %s = %s[%s as usize].clone();\n", ind, valCode, xCode, loopVar)
			}
			injectedBody := injectIntoBlock(bodyNode, Leaf(Identifier, valDecl))
			children = append(children,
				Leaf(WhiteSpace, ind4),
				Leaf(ForKeyword, "for"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, loopVar),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "in", TagRust),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "0.."+lenExpr),
				Leaf(WhiteSpace, " "),
				injectedBody,
				Leaf(NewLine, "\n"),
			)
		} else {
			loopVar := keyCode
			if loopVar == "_" || loopVar == "" {
				loopVar = fmt.Sprintf("_i%d", e.rangeVarCounter)
				e.rangeVarCounter++
			}
			children = append(children,
				Leaf(WhiteSpace, ind4),
				Leaf(ForKeyword, "for"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, loopVar),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "in", TagRust),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "0.."+lenExpr),
				Leaf(WhiteSpace, " "),
				bodyNode,
				Leaf(NewLine, "\n"),
			)
		}
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(RightBrace, "}"),
			Leaf(NewLine, "\n"),
		)
		e.fs.AddTree(IRTree(RangeStatement, KindStmt, children...))
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
		// Inject val decl into body, preserving tree structure
		injectedBody := injectIntoBlock(bodyNode, Leaf(Identifier, valDecl))

		lenExpr := xCode + ".len() as i32"
		e.fs.AddTree(IRTree(RangeStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, loopVar),
			Leaf(WhiteSpace, " "),
			LeafTag(Keyword, "in", TagRust),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "0.."+lenExpr),
			Leaf(WhiteSpace, " "),
			injectedBody,
			Leaf(NewLine, "\n"),
		))
	} else {
		// Key-only or no-key range — use for..in pattern for correct continue semantics
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = fmt.Sprintf("_i%d", e.rangeVarCounter)
			e.rangeVarCounter++
		}

		lenExpr := xCode + ".len() as i32"
		e.fs.AddTree(IRTree(RangeStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, loopVar),
			Leaf(WhiteSpace, " "),
			LeafTag(Keyword, "in", TagRust),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "0.."+lenExpr),
			Leaf(WhiteSpace, " "),
			bodyNode,
			Leaf(NewLine, "\n"),
		))
	}
}

// ============================================================
// Switch / Case Statements
// ============================================================

func (e *RustEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	e.indent = indent
}

func (e *RustEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSwitchStmtTag))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSwitchStmt))
	ind := rustIndent(indent / 2)

	tagCode := ""
	idx := 0
	if idx < len(tokens) {
		tagCode = tokens[idx].Serialize()
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

	var children []IRNode
	children = append(children,
		Leaf(WhiteSpace, ind),
		Leaf(SwitchKeyword, "match"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, tagCode),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
	)
	for i := idx; i < len(tokens); i++ {
		children = append(children, tokens[i])
	}
	if needsWildcard {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(DefaultKeyword, "_"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "=>"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftBrace, "{"),
			Leaf(RightBrace, "}"),
			Leaf(NewLine, "\n"),
		)
	}
	children = append(children,
		Leaf(WhiteSpace, ind),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
	)
	e.fs.AddTree(IRTree(SwitchStatement, KindStmt, children...))
}

func (e *RustEmitter) PreVisitCaseClause(node *ast.CaseClause, indent int) {
	e.indent = indent
}

func (e *RustEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCaseClauseListExpr))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCaseClauseList))
	var exprs []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			exprs = append(exprs, t.Serialize())
		}
	}
	e.fs.AddLeaf(strings.Join(exprs, " | "), KindExpr, nil)
}

func (e *RustEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCaseClause))
	ind := rustIndent(indent / 2)

	var children []IRNode
	idx := 0
	if len(node.List) == 0 {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(DefaultKeyword, "_"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "=>"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftBrace, "{"),
			Leaf(NewLine, "\n"),
		)
	} else {
		caseExprs := ""
		if idx < len(tokens) {
			caseExprs = tokens[idx].Serialize()
			idx++
		}
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(Identifier, caseExprs),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "=>"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftBrace, "{"),
			Leaf(NewLine, "\n"),
		)
	}
	for i := idx; i < len(tokens); i++ {
		children = append(children, tokens[i])
	}
	children = append(children,
		Leaf(WhiteSpace, ind),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
	)
	e.fs.AddTree(IRTree(CaseClauseStatement, KindStmt, children...))
}

// ============================================================
// Inc/Dec Statements
// ============================================================

func (e *RustEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIncDecStmt))
	xNode := collectToNode(tokens)
	ind := rustIndent(indent / 2)
	if node.Tok == token.INC {
		e.fs.AddTree(IRTree(IncDecStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			xNode,
			Leaf(WhiteSpace, " "),
			Leaf(ArithmeticOperator, "+="),
			Leaf(WhiteSpace, " "),
			Leaf(NumberLiteral, "1"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	} else {
		e.fs.AddTree(IRTree(IncDecStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			xNode,
			Leaf(WhiteSpace, " "),
			Leaf(ArithmeticOperator, "-="),
			Leaf(WhiteSpace, " "),
			Leaf(NumberLiteral, "1"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	}
}

// ============================================================
// Branch Statements (break, continue)
// ============================================================

func (e *RustEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	ind := rustIndent(indent / 2)
	switch node.Tok {
	case token.BREAK:
		e.fs.AddTree(IRTree(BranchStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(BreakKeyword, "break"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	case token.CONTINUE:
		e.fs.AddTree(IRTree(BranchStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ContinueKeyword, "continue"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	}
}
