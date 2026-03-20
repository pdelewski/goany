package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

// ============================================================
// Literals and Identifiers
// ============================================================

func (e *RustEmitter) PreVisitBasicLit(node *ast.BasicLit, indent int) {
	val := node.Value
	if node.Kind == token.STRING {
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			// Regular string -> "...".to_string()
			inner := val[1 : len(val)-1]
			val = fmt.Sprintf("\"%s\".to_string()", inner)
		} else if len(val) >= 2 && val[0] == '`' && val[len(val)-1] == '`' {
			// Raw string -> r#"..."#.to_string()
			inner := val[1 : len(val)-1]
			val = fmt.Sprintf("r#\"%s\"#.to_string()", inner)
		}
	}
	// If an INT literal has a float Go type (untyped constant in float context), add .0 suffix
	if node.Kind == token.INT {
		goType := e.getExprGoType(node)
		if goType != nil {
			if basic, ok := goType.Underlying().(*types.Basic); ok {
				if basic.Kind() == types.Float64 {
					val = val + ".0"
				} else if basic.Kind() == types.Float32 {
					val = val + ".0_f32"
				} else if basic.Kind() == types.UntypedFloat {
					val = val + ".0"
				}
			}
		}
	}

	// Convert character literals to numeric values
	if node.Kind == token.CHAR && len(val) >= 3 && val[0] == '\'' {
		inner := val[1 : len(val)-1]
		if len(inner) == 1 {
			val = fmt.Sprintf("%d", inner[0])
		} else if inner == "\\n" {
			val = "10"
		} else if inner == "\\t" {
			val = "9"
		} else if inner == "\\r" {
			val = "13"
		} else if inner == "\\\\" {
			val = "92"
		} else if inner == "\\'" {
			val = "39"
		} else if inner == "\\\"" {
			val = "34"
		} else if inner == "\\0" {
			val = "0"
		} else {
			val = fmt.Sprintf("'%s' as i32", inner)
		}
	}
	e.fs.AddLeaf(val, TagLiteral, nil)
}

func (e *RustEmitter) PreVisitIdent(node *ast.Ident, indent int) {
	name := node.Name
	switch name {
	case "true", "false":
		e.fs.AddLeaf(name, TagLiteral, nil)
		return
	case "nil":
		// Context-dependent nil: for interface{}/any returns use Rc, otherwise Vec::new()
		if e.funcReturnType != nil {
			typeStr := e.funcReturnType.String()
			if strings.Contains(typeStr, "interface{}") || strings.Contains(typeStr, "interface {") || typeStr == "any" {
				e.fs.AddLeaf("Rc::new(0_i32) as Rc<dyn Any>", TagLiteral, nil)
				return
			}
		}
		e.fs.AddLeaf("Vec::new()", TagLiteral, nil)
		return
	case "string":
		e.fs.AddLeaf("String", TagType, nil)
		return
	}
	// Check type mappings
	if rustType, ok := rustTypesMap[name]; ok {
		e.fs.AddLeaf(rustType, TagType, nil)
		return
	}
	// Check type alias map
	if underlyingType, ok := e.typeAliasMap[name]; ok {
		e.fs.AddLeaf(underlyingType, TagType, nil)
		return
	}
	// Check if reference to another package
	if e.pkg != nil && e.pkg.TypesInfo != nil {
		if obj := e.pkg.TypesInfo.Uses[node]; obj != nil {
			if obj.Pkg() != nil && obj.Pkg().Name() != e.currentPackage && obj.Pkg().Name() != "main" {
				name = obj.Pkg().Name() + "::" + name
			}
		}
	}
	// Escape Rust keywords
	name = escapeRustKeyword(name)
	goType := e.getExprGoType(node)
	e.fs.AddLeaf(name, TagIdent, goType)
}

// ============================================================
// Binary Expressions
// ============================================================

func (e *RustEmitter) PostVisitBinaryExprLeft(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBinaryExprLeft))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitBinaryExprRight(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBinaryExprRight))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBinaryExpr))
	left := ""
	right := ""
	var leftNode, rightNode IRNode
	if len(tokens) >= 1 {
		leftNode = tokens[0]
		left = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		rightNode = tokens[1]
		right = tokens[1].Serialize()
	}
	op := node.Op.String()

	// String concatenation: left + &right
	leftType := e.getExprGoType(node.X)
	if leftType != nil {
		if basic, ok := leftType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
			if op == "+" {
				// String concat: left + &right — preserve tree nodes for OptCallArg
				e.fs.AddTree(IRTree(BinaryExpression, KindExpr,
					leftNode,
					Leaf(WhiteSpace, " "),
					Leaf(BinaryOperator, "+"),
					Leaf(WhiteSpace, " "),
					IRTree(UnaryExpression, KindExpr, Leaf(UnaryOperator, "&"), rightNode),
				))
				return
			}
			// String comparisons work as-is
		}
	}

	// Detect mixed-type float/int operations: cast int operand to float
	rightType := e.getExprGoType(node.Y)
	if leftType != nil && rightType != nil {
		leftBasic, leftIsBasic := leftType.Underlying().(*types.Basic)
		rightBasic, rightIsBasic := rightType.Underlying().(*types.Basic)
		if leftIsBasic && rightIsBasic {
			leftIsFloat := leftBasic.Kind() == types.Float64 || leftBasic.Kind() == types.Float32
			rightIsFloat := rightBasic.Kind() == types.Float64 || rightBasic.Kind() == types.Float32
			leftIsInt := leftBasic.Info()&types.IsInteger != 0
			rightIsInt := rightBasic.Info()&types.IsInteger != 0
			if leftIsFloat && rightIsInt {
				if leftBasic.Kind() == types.Float32 {
					right = fmt.Sprintf("(%s as f32)", right)
				} else {
					right = fmt.Sprintf("(%s as f64)", right)
				}
			} else if rightIsFloat && leftIsInt {
				if rightBasic.Kind() == types.Float32 {
					left = fmt.Sprintf("(%s as f32)", left)
				} else {
					left = fmt.Sprintf("(%s as f64)", left)
				}
			}
		}
	}

	// Determine small-int cast: use result type for arithmetic/bitwise, or left operand type for comparisons
	var castStr string
	isComparison := op == "==" || op == "!=" || op == "<" || op == ">" || op == "<=" || op == ">="

	exprType := e.getExprGoType(node)
	if isComparison && leftType != nil {
		// For comparisons, check the left operand type
		if basic, ok := leftType.Underlying().(*types.Basic); ok {
			switch basic.Kind() {
			case types.Int8:
				castStr = "i8"
			case types.Uint8:
				castStr = "u8"
			case types.Int16:
				castStr = "i16"
			case types.Uint16:
				castStr = "u16"
			}
		}
	} else if exprType != nil {
		if basic, ok := exprType.Underlying().(*types.Basic); ok {
			switch basic.Kind() {
			case types.Int8:
				castStr = "i8"
			case types.Uint8:
				castStr = "u8"
			case types.Int16:
				castStr = "i16"
			case types.Uint16:
				castStr = "u16"
			}
		}
	}

	if castStr != "" {
		// Always cast both operands to the target type
		// This handles untyped Go constants that are emitted as i32 in Rust
		// Preserve tree structure (leftNode/rightNode) for pass-based ref-opt
		leftCast := IRTree(CallExpression, KindExpr,
			Leaf(LeftParen, "("),
			leftNode,
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "as"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, castStr),
			Leaf(RightParen, ")"),
		)
		rightCast := IRTree(CallExpression, KindExpr,
			Leaf(LeftParen, "("),
			rightNode,
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "as"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, castStr),
			Leaf(RightParen, ")"),
		)
		if !isComparison {
			e.fs.AddTree(IRTree(BinaryExpression, KindExpr,
				Leaf(LeftParen, "("),
				Leaf(LeftParen, "("),
				leftCast,
				Leaf(WhiteSpace, " "),
				Leaf(BinaryOperator, op),
				Leaf(WhiteSpace, " "),
				rightCast,
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "as"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, castStr),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.AddTree(IRTree(BinaryExpression, KindExpr,
				leftCast,
				Leaf(WhiteSpace, " "),
				Leaf(BinaryOperator, op),
				Leaf(WhiteSpace, " "),
				rightCast,
			))
		}
		return
	}

	e.fs.AddTree(IRTree(BinaryExpression, KindExpr,
		leftNode,
		Leaf(WhiteSpace, " "),
		Leaf(BinaryOperator, op),
		Leaf(WhiteSpace, " "),
		rightNode,
	))
}

// ============================================================
// Call Expressions
// ============================================================

func (e *RustEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	funCode := e.fs.CollectText(string(PreVisitCallExprFun))

	// Save current callee state before overwriting (for nested calls)
	e.Opt.calleeNameStack = append(e.Opt.calleeNameStack, e.Opt.currentCalleeName)
	e.Opt.calleeKeyStack = append(e.Opt.calleeKeyStack, e.Opt.currentCalleeKey)
	e.Opt.callIsLenStack = append(e.Opt.callIsLenStack, e.Opt.currentCallIsLen)

	// Track callee name and push read-only flags for ref optimization
	if ident, ok := node.(*ast.Ident); ok {
		e.Opt.currentCalleeName = ident.Name
		e.Opt.currentCallIsLen = (ident.Name == "len")
	} else if sel, ok := node.(*ast.SelectorExpr); ok {
		e.Opt.currentCalleeName = sel.Sel.Name
		e.Opt.currentCallIsLen = false
	} else {
		e.Opt.currentCalleeName = ""
		e.Opt.currentCallIsLen = false
	}

	// Track callee key for OptMeta annotations on call args
	e.Opt.currentCalleeKey = e.Opt.refOptFuncKey(funCode)

	e.fs.AddLeaf(funCode, KindExpr, nil)
}

func (e *RustEmitter) PreVisitCallExprArg(node ast.Expr, index int, indent int) {
	e.callExprArgDepth++
	e.Opt.argAlreadyCloned = false
}

func (e *RustEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {
	e.callExprArgDepth--

	// Move extraction: replace arg with temp variable name (simple expression case)
	if e.Opt.moveOptActive && e.Opt.moveOptArgReplacements != nil {
		if replacement, ok := e.Opt.moveOptArgReplacements[index]; ok {
			e.fs.CollectForest(string(PreVisitCallExprArg))
			e.fs.AddLeaf(replacement, KindExpr, nil)
			e.Opt.MoveOptCount++
			return
		}
	}

	// Call expression extraction: capture emitted code and replace with temp name
	// Only at the outermost call arg level (callExprArgDepth == 0 means just decremented from 1)
	if e.Opt.moveOptActive && len(e.Opt.moveOptCallExts) > 0 && e.callExprArgDepth == 0 {
		for i := range e.Opt.moveOptCallExts {
			if e.Opt.moveOptCallExts[i].argIdx == index {
				tokens := e.fs.CollectForest(string(PreVisitCallExprArg))
				argNode := collectToNode(tokens)
				e.Opt.moveOptCallExts[i].code = argNode.Serialize()
				e.Opt.moveOptCallExts[i].node = argNode
				e.fs.AddLeaf(e.Opt.moveOptCallExts[i].tempName, KindExpr, nil)
				e.Opt.MoveOptCount++
				return
			}
		}
	}

	tokens := e.fs.CollectForest(string(PreVisitCallExprArg))
	argNode := collectToNode(tokens)
	argCode := argNode.Serialize()

	// std::mem::take optimization: replace state.Field with std::mem::take(&mut state.Field)
	if e.Opt.memTakeActive && index == e.Opt.memTakeArgIdx && len(e.Opt.currentCallArgIdentsStack) <= 1 {
		e.fs.AddLeaf("std::mem::take(&mut "+e.Opt.memTakeLhsExpr+")", KindExpr, nil)
		e.Opt.MoveOptCount++
		return
	}

	// len() is read-only — skip all cloning for its arguments
	if e.Opt.currentCallIsLen && e.Opt.OptimizeMoves {
		e.fs.AddTree(argNode)
		return
	}

	// Skip redundant .clone() when vec element access already produced an owned value
	if e.Opt.argAlreadyCloned && e.Opt.OptimizeMoves {
		e.Opt.argAlreadyCloned = false
		e.Opt.MoveOptCount++
		e.fs.AddTree(argNode)
		return
	}

	// Add .clone() for non-Copy types passed as arguments
	needsClone := false
	argType := e.getExprGoType(node)
	if argType != nil && !isCopyType(argType) {
		// Don't clone string literals (already .to_string())
		if _, isBasicLit := node.(*ast.BasicLit); !isBasicLit {
			// Don't clone composite literals (already owned)
			if _, isCompLit := node.(*ast.CompositeLit); !isCompLit {
				// Don't clone function calls (already return owned)
				if _, isCallExpr := node.(*ast.CallExpr); !isCallExpr {
					// Don't clone function literals (closures) — they are passed by value
					if _, isFuncLit := node.(*ast.FuncLit); !isFuncLit {
						// Move optimization: skip .clone() if the variable can be moved
						if identNode, isIdent := node.(*ast.Ident); isIdent && e.Opt.canMoveArg(identNode.Name) {
							e.fs.AddTree(argNode)
							return
						}
						argCode = argCode + ".clone()"
						needsClone = true
					}
				}
			}
		}
	}

	if needsClone {
		e.fs.AddLeaf(argCode, KindExpr, nil)
	} else {
		e.fs.AddTree(argNode)
	}
}

func (e *RustEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	argTokens := e.fs.CollectForest(string(PreVisitCallExprArgs))
	first := true
	argIdx := 0
	for _, t := range argTokens {
		s := t.Serialize()
		if s == "" {
			continue
		}
		if !first {
			e.fs.AddTree(IRNode{Type: Comma, Content: ", "})
		}
		t.Type = CallExpression
		isIdent := false
		if argIdx < len(node) {
			_, isIdent = node[argIdx].(*ast.Ident)
		}
		t.OptMeta = &OptMeta{
			Kind:       OptCallArg,
			CalleeName: e.Opt.currentCalleeName,
			FuncKey:    e.Opt.currentCalleeKey,
			ParamIndex: argIdx,
			IsIdentArg: isIdent,
		}
		e.fs.AddTree(t)
		first = false
		argIdx++
	}
}

func (e *RustEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCallExpr))
	funName := ""
	argsStr := ""
	if len(tokens) >= 1 {
		funName = tokens[0].Serialize()
	}
	if len(tokens) > 1 {
		var sb strings.Builder
		for _, t := range tokens[1:] {
			sb.WriteString(t.Serialize())
		}
		argsStr = sb.String()
	}

	// Restore saved callee state (for nested calls)
	defer func() {
		if n := len(e.Opt.calleeNameStack); n > 0 {
			e.Opt.currentCalleeName = e.Opt.calleeNameStack[n-1]
			e.Opt.calleeNameStack = e.Opt.calleeNameStack[:n-1]
			e.Opt.currentCalleeKey = e.Opt.calleeKeyStack[n-1]
			e.Opt.calleeKeyStack = e.Opt.calleeKeyStack[:n-1]
			e.Opt.currentCallIsLen = e.Opt.callIsLenStack[n-1]
			e.Opt.callIsLenStack = e.Opt.callIsLenStack[:n-1]
		} else {
			e.Opt.currentCalleeName = ""
			e.Opt.currentCallIsLen = false
			e.Opt.currentCalleeKey = ""
		}
	}()

	// Local closure inlining: replace call with inline body block
	if ident, ok := node.Fun.(*ast.Ident); ok {
		if body, found := e.localClosureBodies[ident.Name]; found {
			// Extract the body from the closure code: "|| { body }" -> "{ body }"
			// Find the first '{' and wrap the body in a block
			if braceIdx := strings.Index(body, "{"); braceIdx >= 0 {
				e.fs.AddTree(IRTree(CallExpression, KindExpr, Leaf(Identifier, body[braceIdx:])))
			} else {
				e.fs.AddTree(IRTree(CallExpression, KindExpr, Leaf(Identifier, "{ "+body+" }")))
			}
			return
		}
	}

	// Handle builtins
	switch funName {
	case "len":
		if len(node.Args) > 0 {
			if e.isMapTypeExpr(node.Args[0]) {
				if e.Opt.OptimizeRefs {
					// Ref opt: use &map instead of map.clone()
					e.fs.AddTree(IRTree(CallExpression, KindExpr,
						Leaf(Identifier, "hmap::hashMapLen"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, "&"+argsStr),
						Leaf(RightParen, ")"),
					))
					e.Opt.RefOptPass.TransformCount++
				} else {
					e.fs.AddTree(IRTree(CallExpression, KindExpr,
						Leaf(Identifier, "hmap::hashMapLen"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, argsStr),
						Leaf(Dot, "."),
						Leaf(Identifier, "clone"),
						Leaf(LeftParen, "("),
						Leaf(RightParen, ")"),
						Leaf(RightParen, ")"),
					))
				}
				return
			}
			// Check for string len
			argType := e.getExprGoType(node.Args[0])
			if argType != nil {
				if basic, ok := argType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
					e.fs.AddTree(IRTree(CallExpression, KindExpr,
						Leaf(Identifier, argsStr),
						Leaf(Dot, "."),
						Leaf(Identifier, "len"),
						Leaf(LeftParen, "("),
						Leaf(RightParen, ")"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "as"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "i32"),
					))
					return
				}
			}
		}
		e.fs.AddTree(IRTree(CallExpression, KindExpr,
			Leaf(Identifier, "len"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, "&"+argsStr),
			Leaf(RightParen, ")"),
		))
		return
	case "append":
		e.fs.AddTree(IRTree(CallExpression, KindExpr,
			Leaf(Identifier, "append"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
		return
	case "delete":
		if len(node.Args) >= 2 {
			mapName := exprToRustString(node.Args[0])
			// Parse args to get key
			parts := strings.SplitN(argsStr, ", ", 2)
			keyStr := ""
			if len(parts) >= 2 {
				keyStr = parts[1]
			}
			mapGoType := e.getExprGoType(node.Args[0])
			keyCast := ""
			keyIsStr := false
			if mapGoType != nil {
				if mapUnderlying, ok := mapGoType.Underlying().(*types.Map); ok {
					keyCast = getRustKeyCast(mapUnderlying.Key())
					keyIsStr = isRustStringKey(mapUnderlying.Key())
				}
			}
			if keyIsStr {
				e.fs.AddTree(IRTree(CallExpression, KindExpr,
					Leaf(Identifier, mapName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "hmap::hashMapDelete"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, mapName),
					Leaf(Comma, ","),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "Rc::new"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, keyStr),
					Leaf(RightParen, ")"),
					Leaf(RightParen, ")"),
				))
			} else {
				e.fs.AddTree(IRTree(CallExpression, KindExpr,
					Leaf(Identifier, mapName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "hmap::hashMapDelete"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, mapName),
					Leaf(Comma, ","),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "Rc::new"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, keyStr+keyCast),
					Leaf(RightParen, ")"),
					Leaf(RightParen, ")"),
				))
			}
		} else {
			e.fs.AddTree(IRTree(CallExpression, KindExpr,
				Leaf(Identifier, "hmap::hashMapDelete"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(RightParen, ")"),
			))
		}
		return
	case "make":
		if len(node.Args) >= 1 {
			if mapType, ok := node.Args[0].(*ast.MapType); ok {
				keyTypeConst := e.getMapKeyTypeConst(mapType)
				e.fs.AddTree(IRTree(CallExpression, KindExpr,
					Leaf(Identifier, "hmap::newHashMap"),
					Leaf(LeftParen, "("),
					Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)),
					Leaf(RightParen, ")"),
				))
				return
			}
			if _, ok := node.Args[0].(*ast.ArrayType); ok {
				elemType := "i32"
				if e.pkg != nil && e.pkg.TypesInfo != nil {
					if tv, ok := e.pkg.TypesInfo.Types[node.Args[0]]; ok && tv.Type != nil {
						if slice, ok := tv.Type.(*types.Slice); ok {
							elemType = e.qualifiedRustTypeName(slice.Elem())
						}
					}
				}
				parts := strings.SplitN(argsStr, ", ", 2)
				if len(parts) >= 2 {
					sizeArg := strings.TrimSpace(parts[1])
					defaultVal := rustDefaultForRustType(elemType)
					if elemType == "Rc<dyn Any>" {
						defaultVal = "Rc::new(0_i32) as Rc<dyn Any>"
					}
					e.fs.AddTree(IRTree(CallExpression, KindExpr,
						Leaf(Identifier, "vec!"),
						Leaf(LeftBracket, "["),
						Leaf(Identifier, defaultVal),
						Leaf(Semicolon, ";"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, sizeArg),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "as"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "usize"),
						Leaf(RightBracket, "]"),
					))
				} else {
					children := []IRNode{Leaf(Identifier, "Vec::"), Leaf(LeftAngle, "<"), Leaf(Identifier, elemType), Leaf(RightAngle, ">"), Leaf(Identifier, "::new"), Leaf(LeftParen, "("), Leaf(RightParen, ")")}
					e.fs.AddTree(IRTree(CallExpression, KindExpr, children...))
				}
				return
			}
		}
		e.fs.AddTree(IRTree(CallExpression, KindExpr,
			Leaf(Identifier, "make"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
		return
	}

	// Check if type conversion (e.g., int(x), string(x))
	if ident, ok := node.Fun.(*ast.Ident); ok {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if obj := e.pkg.TypesInfo.ObjectOf(ident); obj != nil {
				if _, isTypeName := obj.(*types.TypeName); isTypeName {
					rustType := e.qualifiedRustTypeName(obj.Type())
					if rustType == "String" {
						// string(x) -> format!("{}", x) - preserve arg tree
						fmtChildren := []IRNode{
							Leaf(Identifier, "format!"),
							Leaf(LeftParen, "("),
							Leaf(StringLiteral, "\"{}\""),
							Leaf(Comma, ","),
							Leaf(WhiteSpace, " "),
						}
						fmtChildren = append(fmtChildren, tokens[1:]...)
						fmtChildren = append(fmtChildren, Leaf(RightParen, ")"))
						e.fs.AddTree(IRTree(CallExpression, KindExpr, fmtChildren...))
						return
					}
					// Numeric cast: ((x) as type) - preserve arg tree for pass-based ref-opt
					castChildren := []IRNode{
						Leaf(LeftParen, "("),
						Leaf(LeftParen, "("),
					}
					castChildren = append(castChildren, tokens[1:]...)
					castChildren = append(castChildren,
						Leaf(RightParen, ")"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "as"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, rustType),
						Leaf(RightParen, ")"),
					)
					e.fs.AddTree(IRTree(CallExpression, KindExpr, castChildren...))
					return
				}
			}
		}
		// Fallback for type conversions - preserve arg tree for pass-based ref-opt
		if isConv, rustType := e.isTypeConversion(funName); isConv {
			castChildren := []IRNode{
				Leaf(LeftParen, "("),
				Leaf(LeftParen, "("),
			}
			castChildren = append(castChildren, tokens[1:]...)
			castChildren = append(castChildren,
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "as"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, rustType),
				Leaf(RightParen, ")"),
			)
			e.fs.AddTree(IRTree(CallExpression, KindExpr, castChildren...))
			return
		}
	}

	// Lower builtins
	lowered := e.lowerToBuiltins(funName)
	if lowered != funName {
		funName = lowered
	}

	// Handle println/printf variants
	if funName == "println" {
		if len(node.Args) == 0 {
			e.fs.AddTree(IRTree(CallExpression, KindExpr, Leaf(Identifier, "println0()")))
			return
		}
	}
	if funName == "printf" {
		switch len(node.Args) {
		case 1:
			// printf(val) -> printf(val)
		case 2:
			// Check for %c format
			if basicLit, ok := node.Args[0].(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
				fmtStr := strings.Trim(basicLit.Value, "\"")
				if fmtStr == "%c" {
					parts := strings.SplitN(argsStr, ", ", 2)
					if len(parts) >= 2 {
						e.fs.AddTree(IRTree(CallExpression, KindExpr,
							Leaf(Identifier, "printc"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, strings.TrimSpace(parts[1])),
							Leaf(RightParen, ")"),
						))
						return
					}
				}
			}
			funName = "printf2"
		case 3:
			funName = "printf3"
		case 4:
			funName = "printf4"
		case 5:
			funName = "printf5"
		}
	}
	if funName == "string_format" {
		switch len(node.Args) {
		case 2:
			// Check for %c format
			if basicLit, ok := node.Args[0].(*ast.BasicLit); ok && basicLit.Kind == token.STRING {
				fmtStr := strings.Trim(basicLit.Value, "\"")
				if fmtStr == "%c" {
					parts := strings.SplitN(argsStr, ", ", 2)
					if len(parts) >= 2 {
						e.fs.AddTree(IRTree(CallExpression, KindExpr,
							Leaf(Identifier, "byte_to_char"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, strings.TrimSpace(parts[1])),
							Leaf(RightParen, ")"),
						))
						return
					}
				}
			}
			funName = "string_format2"
		}
	}

	// Detect struct field closure calls: obj.field(args) -> (obj.field)(args)
	// Rust requires parentheses to call a closure stored in a struct field
	if selExpr, ok := node.Fun.(*ast.SelectorExpr); ok {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			sel := e.pkg.TypesInfo.Selections[selExpr]
			if sel != nil && sel.Kind() == types.FieldVal {
				if _, isSig := sel.Type().(*types.Signature); isSig {
					closureChildren := []IRNode{
						Leaf(LeftParen, "("),
						Leaf(Identifier, funName),
						Leaf(RightParen, ")"),
						Leaf(LeftParen, "("),
					}
					closureChildren = append(closureChildren, tokens[1:]...)
					closureChildren = append(closureChildren, Leaf(RightParen, ")"))
					e.fs.AddTree(IRTree(CallExpression, KindExpr, closureChildren...))
					return
				}
			}
		}
	}

	var callChildren []IRNode
	callChildren = append(callChildren, Leaf(Identifier, funName))
	callChildren = append(callChildren, Leaf(LeftParen, "("))
	for _, t := range tokens[1:] {
		callChildren = append(callChildren, t)
	}
	callChildren = append(callChildren, Leaf(RightParen, ")"))
	e.fs.AddTree(IRTree(CallExpression, KindExpr, callChildren...))
}

// ============================================================
// Selector Expressions (a.b)
// ============================================================

func (e *RustEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSelectorExprX))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitSelectorExprSel(node *ast.Ident, indent int) {
	e.fs.CollectForest(string(PreVisitSelectorExprSel))
	e.fs.AddLeaf(node.Name, KindExpr, nil)
}

func (e *RustEmitter) PostVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSelectorExpr))
	xCode := ""
	selCode := ""
	var xNode IRNode
	if len(tokens) >= 1 {
		xNode = tokens[0]
		xCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		selCode = tokens[1].Serialize()
	}

	if xCode == "os" && selCode == "Args" {
		e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, "std::env::args().collect::<Vec<String>>()")))
		return
	}

	// Lower builtins: fmt.Println -> println, etc.
	loweredX := e.lowerToBuiltins(xCode)
	loweredSel := e.lowerToBuiltins(selCode)

	if loweredX == "" {
		// Check if selector is a type alias (only for non-package-qualified access)
		if _, isAlias := e.typeAliasMap[selCode]; isAlias {
			e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, e.typeAliasMap[selCode])))
			return
		}
		e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, loweredSel)))
	} else {
		// Use :: for package access, . for field access
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if ident, ok := node.X.(*ast.Ident); ok {
				if obj := e.pkg.TypesInfo.Uses[ident]; obj != nil {
					if _, isPkg := obj.(*types.PkgName); isPkg {
						e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, loweredX+"::"+loweredSel)))
						return
					}
				}
			}
		}
		// Preserve tree structure for field access to keep OptCallArg metadata
		if loweredX == xCode {
			// X was not lowered — keep original tree node
			e.fs.AddTree(IRTree(SelectorExpression, KindExpr,
				xNode,
				Leaf(Dot, "."),
				Leaf(Identifier, loweredSel),
			))
		} else {
			e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, loweredX+"."+loweredSel)))
		}
	}
}

// ============================================================
// Index Expressions (a[i])
// ============================================================

func (e *RustEmitter) PostVisitIndexExprX(node *ast.IndexExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIndexExprX))
	xNode := collectToNode(tokens)
	xNode.Kind = KindExpr
	e.fs.AddTree(xNode)
	e.lastIndexXCode = xNode.Serialize()
}

func (e *RustEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIndexExprIndex))
	idxNode := collectToNode(tokens)
	idxNode.Kind = KindExpr
	e.fs.AddTree(idxNode)
	e.lastIndexKeyCode = idxNode.Serialize()
}

func (e *RustEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIndexExpr))
	xCode := ""
	idxCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		idxCode = tokens[1].Serialize()
	}

	if e.isMapTypeExpr(node.X) {
		mapGoType := e.getExprGoType(node.X)
		valType := "Rc<dyn Any>"
		keyCast := ""
		keyIsStr := false
		if mapGoType != nil {
			if mapUnderlying, ok := mapGoType.Underlying().(*types.Map); ok {
				valType = e.qualifiedRustTypeName(mapUnderlying.Elem())
				keyCast = getRustKeyCast(mapUnderlying.Key())
				keyIsStr = isRustStringKey(mapUnderlying.Key())
			}
		}
		// Map read: hashMapGet(m, Rc::new(key))
		var keyExpr string
		if keyIsStr {
			keyExpr = fmt.Sprintf("Rc::new(%s)", idxCode)
		} else {
			keyExpr = fmt.Sprintf("Rc::new(%s%s)", idxCode, keyCast)
		}
		castExpr := ""
		if valType != "Rc<dyn Any>" {
			castExpr = fmt.Sprintf(".downcast_ref::<%s>().unwrap().clone()", valType)
		}
		mapRef := xCode + ".clone()"
		if e.Opt.OptimizeRefs {
			mapRef = "&" + xCode
			e.Opt.RefOptPass.TransformCount++
		}
		token := IRTree(IndexExpression, KindExpr,
			Leaf(Identifier, "hmap::hashMapGet"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, mapRef),
			Leaf(Comma, ","),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, keyExpr),
			Leaf(RightParen, ")"),
			Leaf(Identifier, castExpr),
		)
		token.GoType = e.getExprGoType(node)
		e.fs.AddTree(token)
	} else {
		// Check for string indexing
		xType := e.getExprGoType(node.X)
		if xType != nil {
			if basic, ok := xType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
				e.fs.AddTree(IRTree(IndexExpression, KindExpr,
					Leaf(Identifier, xCode),
					Leaf(Dot, "."),
					Leaf(Identifier, "as_bytes"),
					Leaf(LeftParen, "("),
					Leaf(RightParen, ")"),
					Leaf(LeftBracket, "["),
					Leaf(LeftParen, "("),
					Leaf(Identifier, idxCode),
					Leaf(RightParen, ")"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "as"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "usize"),
					Leaf(RightBracket, "]"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "as"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "i8"),
				))
				return
			}
		}
		// Add .clone() for non-Copy element types (but NOT on LHS of assignment)
		elemType := e.getExprGoType(node)
		if elemType != nil && !isCopyType(elemType) && !e.inAssignLhs {
			e.fs.AddTree(IRTree(IndexExpression, KindExpr,
				Leaf(Identifier, xCode),
				Leaf(LeftBracket, "["),
				Leaf(LeftParen, "("),
				Leaf(Identifier, idxCode),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "as"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "usize"),
				Leaf(RightBracket, "]"),
				Leaf(Dot, "."),
				Leaf(Identifier, "clone"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
			))
			if e.callExprArgDepth > 0 {
				e.Opt.argAlreadyCloned = true
			}
		} else {
			e.fs.AddTree(IRTree(IndexExpression, KindExpr,
				Leaf(Identifier, xCode),
				Leaf(LeftBracket, "["),
				Leaf(LeftParen, "("),
				Leaf(Identifier, idxCode),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "as"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "usize"),
				Leaf(RightBracket, "]"),
			))
		}
	}
}

// ============================================================
// Unary Expressions
// ============================================================

func (e *RustEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitUnaryExpr))
	xNode := collectToNode(tokens)
	op := node.Op.String()
	if op == "^" {
		op = "!"
	}
	e.fs.AddTree(IRTree(UnaryExpression, KindExpr,
		Leaf(UnaryOperator, op),
		xNode,
	))
}

// ============================================================
// Paren Expressions
// ============================================================

func (e *RustEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitParenExpr))
	innerNode := collectToNode(tokens)
	e.fs.AddTree(IRTree(ParenExpression, KindExpr,
		Leaf(LeftParen, "("),
		innerNode,
		Leaf(RightParen, ")"),
	))
}

// ============================================================
// Slice Expressions (a[lo:hi])
// ============================================================

func (e *RustEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSliceExprX))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitSliceExprXBegin))
}

func (e *RustEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSliceExprLow))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitSliceExprXEnd))
}

func (e *RustEmitter) PostVisitSliceExprHigh(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSliceExprHigh))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSliceExpr))
	xCode := ""
	lowCode := ""
	highCode := ""

	idx := 0
	if idx < len(tokens) {
		xCode = tokens[idx].Serialize()
		idx++
	}
	if node.Low != nil && idx < len(tokens) {
		lowCode = tokens[idx].Serialize()
		idx++
	}
	if node.High != nil && idx < len(tokens) {
		highCode = tokens[idx].Serialize()
	}

	if lowCode == "" {
		lowCode = "0"
	}

	xType := e.getExprGoType(node.X)
	isString := false
	if xType != nil {
		if basic, ok := xType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
			isString = true
		}
	}

	if isString {
		if highCode == "" {
			e.fs.AddTree(IRTree(SliceExpression, KindExpr,
				Leaf(Identifier, xCode),
				Leaf(LeftBracket, "["),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "as"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "usize.."),
				Leaf(RightBracket, "]"),
				Leaf(Dot, "."),
				Leaf(Identifier, "to_string"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.AddTree(IRTree(SliceExpression, KindExpr,
				Leaf(Identifier, xCode),
				Leaf(LeftBracket, "["),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "as"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "usize.."),
				Leaf(LeftParen, "("),
				Leaf(Identifier, highCode),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "as"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "usize"),
				Leaf(RightBracket, "]"),
				Leaf(Dot, "."),
				Leaf(Identifier, "to_string"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
			))
		}
	} else {
		if highCode == "" {
			e.fs.AddTree(IRTree(SliceExpression, KindExpr,
				Leaf(Identifier, xCode),
				Leaf(LeftBracket, "["),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "as"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "usize.."),
				Leaf(RightBracket, "]"),
				Leaf(Dot, "."),
				Leaf(Identifier, "to_vec"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.AddTree(IRTree(SliceExpression, KindExpr,
				Leaf(Identifier, xCode),
				Leaf(LeftBracket, "["),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "as"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "usize.."),
				Leaf(LeftParen, "("),
				Leaf(Identifier, highCode),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "as"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "usize"),
				Leaf(RightBracket, "]"),
				Leaf(Dot, "."),
				Leaf(Identifier, "to_vec"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
			))
		}
	}
}

// ============================================================
// Type Assertions
// ============================================================

func (e *RustEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitTypeAssertExprType))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitTypeAssertExprX))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitTypeAssertExpr))
	typeCode := ""
	xCode := ""
	if len(tokens) >= 1 {
		typeCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		xCode = tokens[1].Serialize()
	}
	// x.(Type) -> x.downcast_ref::<Type>().unwrap().clone()
	if typeCode != "" {
		e.fs.AddTree(IRTree(TypeAssertExpression, KindExpr,
			Leaf(Identifier, xCode),
			Leaf(Dot, "."),
			Leaf(Identifier, "downcast_ref::"),
			Leaf(LeftAngle, "<"),
			Leaf(Identifier, typeCode),
			Leaf(RightAngle, ">"),
			Leaf(LeftParen, "("),
			Leaf(RightParen, ")"),
			Leaf(Dot, "."),
			Leaf(Identifier, "unwrap"),
			Leaf(LeftParen, "("),
			Leaf(RightParen, ")"),
			Leaf(Dot, "."),
			Leaf(Identifier, "clone"),
			Leaf(LeftParen, "("),
			Leaf(RightParen, ")"),
		))
	} else {
		e.fs.AddTree(IRTree(TypeAssertExpression, KindExpr, Leaf(Identifier, xCode)))
	}
}

// ============================================================
// Star Expressions (dereference — pass through)
// ============================================================

func (e *RustEmitter) PostVisitStarExpr(node *ast.StarExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitStarExpr))
	xNode := collectToNode(tokens)
	e.fs.AddTree(IRTree(StarExpression, KindExpr, xNode))
}

// ============================================================
// Interface Type
// ============================================================

func (e *RustEmitter) PostVisitInterfaceType(node *ast.InterfaceType, indent int) {
	e.fs.CollectForest(string(PreVisitInterfaceType))
	e.fs.AddTree(IRTree(InterfaceTypeNode, KindType, Leaf(Identifier, "Rc<dyn Any>")))
}

// ============================================================
// Composite Literals
// ============================================================

func (e *RustEmitter) PreVisitCompositeLit(node *ast.CompositeLit, indent int) {
	e.compositeLitDepth++
}

func (e *RustEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitCompositeLitType))
}

func (e *RustEmitter) PostVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCompositeLitElt))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	eltTokens := e.fs.CollectForest(string(PreVisitCompositeLitElts))
	for _, t := range eltTokens {
		s := t.Serialize()
		if s != "" {
			e.fs.AddTree(t)
		}
	}
}

func (e *RustEmitter) PostVisitCompositeLit(node *ast.CompositeLit, indent int) {
	defer func() { e.compositeLitDepth-- }()
	tokens := e.fs.CollectForest(string(PreVisitCompositeLit))
	var elts []string
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			elts = append(elts, s)
		}
	}
	eltsStr := strings.Join(elts, ", ")

	litType := e.getExprGoType(node)
	if litType == nil {
		e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr, Leaf(Identifier, "vec!["+eltsStr+"]")))
		return
	}

	switch u := litType.Underlying().(type) {
	case *types.Struct:
		typeName := ""
		if node.Type != nil {
			typeName = exprToString(node.Type)
		}
		if typeName == "" {
			typeName = "UnknownStruct"
		}
		// Strip any package prefix from AST and use Rust-style
		if dotIdx := strings.LastIndex(typeName, "."); dotIdx >= 0 {
			pkgPart := typeName[:dotIdx]
			typePart := typeName[dotIdx+1:]
			typeName = pkgPart + "::" + typePart
		}

		if len(node.Elts) > 0 {
			if _, isKV := node.Elts[0].(*ast.KeyValueExpr); isKV {
				// Named fields: Name { field: value, ..Default::default() }
				kvMap := make(map[string]string)
				kvNodeMap := make(map[string]IRNode) // preserves tree structure (OptCallArg metadata)
				for _, t := range tokens {
					// KV tree: Children[0]=key, Children[1]=": ", Children[2]=value
					if t.Type == KeyValueExpression && len(t.Children) >= 3 {
						key := t.Children[0].Serialize()
						if dotIdx := strings.LastIndex(key, "."); dotIdx >= 0 {
							key = key[dotIdx+1:]
						}
						if colonIdx := strings.LastIndex(key, "::"); colonIdx >= 0 {
							key = key[colonIdx+2:]
						}
						kvMap[key] = t.Children[2].Serialize()
						kvNodeMap[key] = t.Children[2]
					} else {
						// Fallback: parse from serialized string
						s := t.Serialize()
						parts := strings.SplitN(s, ": ", 2)
						if len(parts) == 2 {
							key := parts[0]
							if dotIdx := strings.LastIndex(key, "."); dotIdx >= 0 {
								key = key[dotIdx+1:]
							}
							if colonIdx := strings.LastIndex(key, "::"); colonIdx >= 0 {
								key = key[colonIdx+2:]
							}
							kvMap[key] = parts[1]
						}
					}
				}
				children := []IRNode{
					Leaf(Identifier, typeName),
					Leaf(WhiteSpace, " "),
					Leaf(LeftBrace, "{"),
					Leaf(WhiteSpace, " "),
				}
				first := true
				allFieldsProvided := len(kvMap) >= u.NumFields()
				for i := 0; i < u.NumFields(); i++ {
					fieldName := u.Field(i).Name()
					if val, ok := kvMap[fieldName]; ok {
						if !first {
							children = append(children, Leaf(Comma, ","), Leaf(WhiteSpace, " "))
						}
						castVal := e.castSmallIntFieldValue(u.Field(i).Type(), val)
						cloneVal := e.cloneStructFieldValue(u.Field(i).Type(), val)
						children = append(children,
							Leaf(Identifier, fieldName),
							Leaf(Colon, ":"),
							Leaf(WhiteSpace, " "),
						)
						// Use tree node when value wasn't modified (preserves OptCallArg metadata)
						if castVal == val && cloneVal == val {
							if valNode, ok := kvNodeMap[fieldName]; ok {
								children = append(children, valNode)
							} else {
								children = append(children, Leaf(Identifier, val))
							}
						} else {
							finalVal := val
							if castVal != val {
								finalVal = castVal
							}
							if cloneVal != val {
								finalVal = cloneVal
							}
							children = append(children, Leaf(Identifier, finalVal))
						}
						first = false
					}
				}
				if !allFieldsProvided {
					if !first {
						children = append(children, Leaf(Comma, ","), Leaf(WhiteSpace, " "))
					}
					children = append(children, Leaf(Identifier, "..Default::default()"), Leaf(WhiteSpace, " "))
				} else {
					children = append(children, Leaf(WhiteSpace, " "))
				}
				children = append(children, Leaf(RightBrace, "}"))
				e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr, children...))
				return
			}
		}
		// Positional struct literal or empty struct
		children := []IRNode{
			Leaf(Identifier, typeName),
			Leaf(WhiteSpace, " "),
			Leaf(LeftBrace, "{"),
			Leaf(WhiteSpace, " "),
		}
		for i, elt := range elts {
			if i > 0 {
				children = append(children, Leaf(Comma, ","), Leaf(WhiteSpace, " "))
			}
			if i < u.NumFields() {
				castVal := e.castSmallIntFieldValue(u.Field(i).Type(), elt)
				cloneVal := e.cloneStructFieldValue(u.Field(i).Type(), elt)
				children = append(children,
					Leaf(Identifier, u.Field(i).Name()),
					Leaf(Colon, ":"),
					Leaf(WhiteSpace, " "),
				)
				// Use tree node when value wasn't modified (preserves OptCallArg metadata)
				if castVal == elt && cloneVal == elt && i < len(tokens) {
					children = append(children, tokens[i])
				} else {
					finalVal := elt
					if castVal != elt {
						finalVal = castVal
					}
					if cloneVal != elt {
						finalVal = cloneVal
					}
					children = append(children, Leaf(Identifier, finalVal))
				}
			}
		}
		if len(elts) < u.NumFields() {
			if len(elts) > 0 {
				children = append(children, Leaf(Comma, ","), Leaf(WhiteSpace, " "))
			}
			children = append(children, Leaf(Identifier, "..Default::default()"))
		}
		children = append(children, Leaf(WhiteSpace, " "), Leaf(RightBrace, "}"))
		e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr, children...))
	case *types.Slice:
		if len(elts) == 0 {
			elemRustType := e.qualifiedRustTypeName(u.Elem())
			children := []IRNode{Leaf(Identifier, "Vec::"), Leaf(LeftAngle, "<"), Leaf(Identifier, elemRustType), Leaf(RightAngle, ">"), Leaf(Identifier, "::new"), Leaf(LeftParen, "("), Leaf(RightParen, ")")}
			e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr, children...))
		} else {
			e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr,
				Leaf(Identifier, "vec!"),
				Leaf(LeftBracket, "["),
				Leaf(Identifier, eltsStr),
				Leaf(RightBracket, "]"),
			))
		}
	case *types.Map:
		keyTypeConst := 1
		if node.Type != nil {
			if mapType, ok := node.Type.(*ast.MapType); ok {
				keyTypeConst = e.getMapKeyTypeConst(mapType)
			}
		}
		if len(elts) == 0 {
			e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr,
				Leaf(Identifier, "hmap::newHashMap"),
				Leaf(LeftParen, "("),
				Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)),
				Leaf(RightParen, ")"),
			))
		} else {
			// Build map with initial values
			e.nestedMapCounter++
			tmpVar := fmt.Sprintf("_m%d", e.nestedMapCounter)
			children := []IRNode{
				Leaf(LeftBrace, "{"),
				Leaf(Identifier, "\n"),
				Leaf(Identifier, "let mut "),
				Leaf(Identifier, tmpVar),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "hmap::newHashMap"),
				Leaf(LeftParen, "("),
				Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(Identifier, "\n"),
			}

			mapUnderlying := litType.Underlying().(*types.Map)
			keyCast := getRustKeyCast(mapUnderlying.Key())
			keyIsStr := isRustStringKey(mapUnderlying.Key())

			for _, elt := range elts {
				parts := strings.SplitN(elt, ": ", 2)
				if len(parts) == 2 {
					keyStr := parts[0]
					valStr := parts[1]
					var keyExpr string
					if keyIsStr {
						keyExpr = "Rc::new(" + keyStr + ")"
					} else {
						keyExpr = "Rc::new(" + keyStr + keyCast + ")"
					}
					children = append(children,
						Leaf(Identifier, tmpVar),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "hmap::hashMapSet"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, tmpVar),
						Leaf(Comma, ","),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, keyExpr),
						Leaf(Comma, ","),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "Rc::new("),
						Leaf(Identifier, valStr),
						Leaf(RightParen, ")"),
						Leaf(RightParen, ")"),
						Leaf(Semicolon, ";"),
						Leaf(Identifier, "\n"),
					)
				}
			}
			children = append(children, Leaf(Identifier, tmpVar), Leaf(Identifier, "\n"), Leaf(RightBrace, "}"))
			e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr, children...))
		}
	default:
		if len(elts) == 0 {
			e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr, Leaf(Identifier, "Vec::new()")))
		} else {
			e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr,
				Leaf(Identifier, "vec!"),
				Leaf(LeftBracket, "["),
				Leaf(Identifier, eltsStr),
				Leaf(RightBracket, "]"),
			))
		}
	}
}

// ============================================================
// KeyValue Expressions
// ============================================================

func (e *RustEmitter) PostVisitKeyValueExprKey(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitKeyValueExprKey))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitKeyValueExprValue(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitKeyValueExprValue))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitKeyValueExpr))
	keyNode := Leaf(Identifier, "")
	valNode := Leaf(Identifier, "")
	if len(tokens) >= 1 {
		keyNode = tokens[0]
	}
	if len(tokens) >= 2 {
		valNode = tokens[1]
	}
	// Preserve key and value as separate children so PostVisitCompositeLit
	// can extract the value tree node (preserving OptCallArg metadata).
	// Children: [0]=key, [1]=": ", [2]=value
	e.fs.AddTree(IRTree(KeyValueExpression, KindExpr, keyNode, Leaf(Colon, ": "), valNode))
}

// ============================================================
// Array Type
// ============================================================

func (e *RustEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	typeTokens := e.fs.CollectForest(string(PreVisitArrayType))
	if len(typeTokens) == 0 {
		typeTokens = []IRNode{Leaf(Identifier, "i32")}
	}
	children := []IRNode{Leaf(Identifier, "Vec"), Leaf(LeftAngle, "<")}
	children = append(children, typeTokens...)
	children = append(children, Leaf(RightAngle, ">"))
	e.fs.AddTree(IRTree(ArrayTypeNode, KindType, children...))
}

// ============================================================
// Map Type
// ============================================================

func (e *RustEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitMapKeyType))
}

func (e *RustEmitter) PostVisitMapValueType(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitMapValueType))
}

func (e *RustEmitter) PostVisitMapType(node *ast.MapType, indent int) {
	e.fs.CollectForest(string(PreVisitMapType))
	e.fs.AddTree(IRTree(MapTypeNode, KindType, Leaf(Identifier, "hmap::HashMap")))
}

// ============================================================
// Function Type (Rc<dyn Fn(...)>)
// ============================================================

func (e *RustEmitter) PostVisitFuncTypeResult(node *ast.Field, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncTypeResult))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitFuncTypeResults(node *ast.FieldList, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncTypeResults))
	var resultTypes []string
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			resultTypes = append(resultTypes, s)
		}
	}
	e.fs.AddLeaf(strings.Join(resultTypes, ", "), KindExpr, nil)
}

func (e *RustEmitter) PostVisitFuncTypeParam(node *ast.Field, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncTypeParam))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitFuncTypeParams(node *ast.FieldList, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncTypeParams))
	var paramTypes []string
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			paramTypes = append(paramTypes, s)
		}
	}
	e.fs.AddLeaf(strings.Join(paramTypes, ", "), KindExpr, nil)
}

func (e *RustEmitter) PostVisitFuncType(node *ast.FuncType, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncType))
	resultTypes := ""
	paramTypes := ""
	if node.Results != nil && node.Results.NumFields() > 0 {
		if len(tokens) >= 1 {
			resultTypes = tokens[0].Serialize()
		}
		if len(tokens) >= 2 {
			paramTypes = tokens[1].Serialize()
		}
		children := []IRNode{
			Leaf(Identifier, "Rc"),
			Leaf(LeftAngle, "<"),
			Leaf(Identifier, "dyn Fn"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, paramTypes),
			Leaf(RightParen, ")"),
			Leaf(Identifier, " -> "),
			Leaf(Identifier, resultTypes),
			Leaf(RightAngle, ">"),
		}
		e.fs.AddTree(IRTree(FuncTypeExpression, KindType, children...))
	} else {
		if len(tokens) >= 1 {
			paramTypes = tokens[0].Serialize()
		}
		children := []IRNode{
			Leaf(Identifier, "Rc"),
			Leaf(LeftAngle, "<"),
			Leaf(Identifier, "dyn Fn"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, paramTypes),
			Leaf(RightParen, ")"),
			Leaf(RightAngle, ">"),
		}
		e.fs.AddTree(IRTree(FuncTypeExpression, KindType, children...))
	}
}

// ============================================================
// Function Literals (closures)
// ============================================================

func (e *RustEmitter) PreVisitFuncLit(node *ast.FuncLit, indent int) {
	e.Opt.funcLitDepth++
	// Save the current funcReturnType and set the closure's return type
	e.savedFuncRetTypes = append(e.savedFuncRetTypes, e.funcReturnType)
	e.funcReturnType = nil
	if node.Type.Results != nil && node.Type.Results.NumFields() == 1 {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			field := node.Type.Results.List[0]
			if tv, ok := e.pkg.TypesInfo.Types[field.Type]; ok && tv.Type != nil {
				e.funcReturnType = tv.Type
			}
		}
	}
}

func (e *RustEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	e.fs.CollectForest(string(PreVisitFuncLitTypeParam))
	typeStr := "Rc<dyn Any>"
	if e.pkg != nil && e.pkg.TypesInfo != nil && len(node.Names) > 0 {
		if obj := e.pkg.TypesInfo.Defs[node.Names[0]]; obj != nil {
			typeStr = e.qualifiedRustTypeName(obj.Type())
		}
	}
	for _, name := range node.Names {
		escapedName := escapeRustKeyword(name.Name)
		token := IRTree(Identifier, TagIdent,
			LeafTag(Keyword, "mut", TagRust),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, escapedName),
			Leaf(Colon, ":"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, typeStr),
		)
		e.fs.AddTree(token)
	}
}

func (e *RustEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncLitTypeParams))
	var paramNames []string
	for _, t := range tokens {
		s := t.Serialize()
		if t.Kind == TagIdent && s != "" {
			paramNames = append(paramNames, s)
		}
	}
	paramsStr := strings.Join(paramNames, ", ")
	e.fs.AddLeaf(paramsStr, KindExpr, nil)
}

func (e *RustEmitter) PostVisitFuncLitTypeResults(node *ast.FieldList, indent int) {
	e.fs.CollectForest(string(PreVisitFuncLitTypeResults))
}

func (e *RustEmitter) PostVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncLitBody))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncLit))
	paramsCode := ""
	var bodyNode IRNode
	if len(tokens) == 1 {
		// No params were pushed (empty string tokens are dropped),
		// so the only token is the body
		bodyNode = tokens[0]
	} else if len(tokens) >= 2 {
		paramsCode = strings.TrimSpace(tokens[0].Serialize())
		bodyNode = tokens[1]
	}

	// Determine return type
	retType := ""
	if node.Type.Results != nil && node.Type.Results.NumFields() > 0 {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			var resultTypes []string
			for _, field := range node.Type.Results.List {
				if tv, ok := e.pkg.TypesInfo.Types[field.Type]; ok && tv.Type != nil {
					resultTypes = append(resultTypes, e.qualifiedRustTypeName(tv.Type))
				}
			}
			if len(resultTypes) == 1 {
				retType = " -> " + resultTypes[0]
			} else if len(resultTypes) > 1 {
				retType = " -> (" + strings.Join(resultTypes, ", ") + ")"
			}
		}
	}

	// Restore the previous funcReturnType
	if len(e.savedFuncRetTypes) > 0 {
		e.funcReturnType = e.savedFuncRetTypes[len(e.savedFuncRetTypes)-1]
		e.savedFuncRetTypes = e.savedFuncRetTypes[:len(e.savedFuncRetTypes)-1]
	}

	e.Opt.funcLitDepth--

	// Build closure header: |params| retType
	header := fmt.Sprintf("|%s|%s ", paramsCode, retType)

	// Wrap in Rc::new() only when used as a struct field value (inside composite literal)
	// Local variable closures and call arguments should be bare closures
	if e.compositeLitDepth > 0 {
		e.fs.AddTree(IRTree(FuncLitExpression, KindExpr,
			Leaf(Identifier, "Rc::new"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, header),
			bodyNode,
			Leaf(RightParen, ")"),
		))
	} else {
		e.fs.AddTree(IRTree(FuncLitExpression, KindExpr,
			Leaf(Identifier, header),
			bodyNode,
		))
	}
}
