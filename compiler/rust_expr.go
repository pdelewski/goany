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
	e.fs.Push(val, TagLiteral, nil)
}

func (e *RustEmitter) PreVisitIdent(node *ast.Ident, indent int) {
	name := node.Name
	switch name {
	case "true", "false":
		e.fs.Push(name, TagLiteral, nil)
		return
	case "nil":
		// Context-dependent nil: for interface{}/any returns use Rc, otherwise Vec::new()
		if e.funcReturnType != nil {
			typeStr := e.funcReturnType.String()
			if strings.Contains(typeStr, "interface{}") || strings.Contains(typeStr, "interface {") || typeStr == "any" {
				e.fs.Push("Rc::new(0_i32) as Rc<dyn Any>", TagLiteral, nil)
				return
			}
		}
		e.fs.Push("Vec::new()", TagLiteral, nil)
		return
	case "string":
		e.fs.Push("String", TagType, nil)
		return
	}
	// Check type mappings
	if rustType, ok := rustTypesMap[name]; ok {
		e.fs.Push(rustType, TagType, nil)
		return
	}
	// Check type alias map
	if underlyingType, ok := e.typeAliasMap[name]; ok {
		e.fs.Push(underlyingType, TagType, nil)
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
	e.fs.Push(name, TagIdent, goType)
}

// ============================================================
// Binary Expressions
// ============================================================

func (e *RustEmitter) PostVisitBinaryExprLeft(node ast.Expr, indent int) {
	left := e.fs.ReduceToCode(string(PreVisitBinaryExprLeft))
	e.fs.PushCode(left)
}

func (e *RustEmitter) PostVisitBinaryExprRight(node ast.Expr, indent int) {
	right := e.fs.ReduceToCode(string(PreVisitBinaryExprRight))
	e.fs.PushCode(right)
}

func (e *RustEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitBinaryExpr))
	left := ""
	right := ""
	if len(tokens) >= 1 {
		left = tokens[0].Content
	}
	if len(tokens) >= 2 {
		right = tokens[1].Content
	}
	op := node.Op.String()

	// String concatenation: left + &right
	leftType := e.getExprGoType(node.X)
	if leftType != nil {
		if basic, ok := leftType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
			if op == "+" {
				// String concat: format!("{}{}", left, right) or left + &right
				e.fs.PushCode(fmt.Sprintf("%s + &%s", left, right))
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
		left = fmt.Sprintf("(%s as %s)", left, castStr)
		right = fmt.Sprintf("(%s as %s)", right, castStr)
		expr := fmt.Sprintf("%s %s %s", left, op, right)
		if !isComparison {
			expr = fmt.Sprintf("((%s) as %s)", expr, castStr)
		}
		e.fs.PushCode(expr)
		return
	}

	e.fs.PushCode(fmt.Sprintf("%s %s %s", left, op, right))
}

// ============================================================
// Call Expressions
// ============================================================

func (e *RustEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	funCode := e.fs.ReduceToCode(string(PreVisitCallExprFun))

	// Track callee name and push read-only flags for ref optimization
	if ident, ok := node.(*ast.Ident); ok {
		e.Opt.currentCalleeName = ident.Name
		e.Opt.currentCallIsLen = (ident.Name == "len")
	} else if sel, ok := node.(*ast.SelectorExpr); ok {
		e.Opt.currentCalleeName = sel.Sel.Name
	} else {
		e.Opt.currentCalleeName = ""
	}

	// Push callee's read-only and mut-ref parameter flags for ref optimization
	if e.Opt.OptimizeRefs && e.Opt.refOptReadOnly != nil {
		funcKey := e.Opt.refOptFuncKey(funCode)
		if flags, ok := e.Opt.refOptReadOnly.ReadOnly[funcKey]; ok {
			e.Opt.refOptCalleeReadOnly = append(e.Opt.refOptCalleeReadOnly, flags)
		} else {
			e.Opt.refOptCalleeReadOnly = append(e.Opt.refOptCalleeReadOnly, nil)
		}
		if flags, ok := e.Opt.refOptReadOnly.MutRef[funcKey]; ok {
			e.Opt.refOptCalleeMutRef = append(e.Opt.refOptCalleeMutRef, flags)
		} else {
			e.Opt.refOptCalleeMutRef = append(e.Opt.refOptCalleeMutRef, nil)
		}
	}

	e.fs.PushCode(funCode)
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
			e.fs.Reduce(string(PreVisitCallExprArg))
			e.fs.PushCode(replacement)
			e.Opt.MoveOptCount++
			return
		}
	}

	// Call expression extraction: capture emitted code and replace with temp name
	// Only at the outermost call arg level (callExprArgDepth == 0 means just decremented from 1)
	if e.Opt.moveOptActive && len(e.Opt.moveOptCallExts) > 0 && e.callExprArgDepth == 0 {
		for i := range e.Opt.moveOptCallExts {
			if e.Opt.moveOptCallExts[i].argIdx == index {
				argCode := e.fs.ReduceToCode(string(PreVisitCallExprArg))
				e.Opt.moveOptCallExts[i].code = argCode
				e.fs.PushCode(e.Opt.moveOptCallExts[i].tempName)
				e.Opt.MoveOptCount++
				return
			}
		}
	}

	argCode := e.fs.ReduceToCode(string(PreVisitCallExprArg))

	// Reference optimization: if callee param is read-only, use & instead of .clone()
	if e.Opt.isRefOptArg(index) {
		// If the argument is already a reference param in the current function, pass as-is
		isAlreadyRef := false
		if ident, ok := node.(*ast.Ident); ok {
			isAlreadyRef = e.Opt.refOptCurrentRefParams[ident.Name]
		}
		if !isAlreadyRef {
			argCode = "&" + argCode
		}
		e.Opt.RefOptCount++
		e.fs.PushCode(argCode)
		return
	}

	// Mutable reference optimization: if callee param is mut-ref, use &mut instead of .clone()
	if e.Opt.isMutRefOptArg(index) {
		isAlreadyMutRef := false
		if ident, ok := node.(*ast.Ident); ok {
			isAlreadyMutRef = e.Opt.refOptCurrentMutRefParams[ident.Name]
		}
		if !isAlreadyMutRef {
			argCode = "&mut " + argCode
		}
		e.Opt.RefOptCount++
		e.fs.PushCode(argCode)
		return
	}

	// std::mem::take optimization: replace state.Field with std::mem::take(&mut state.Field)
	if e.Opt.memTakeActive && index == e.Opt.memTakeArgIdx && len(e.Opt.currentCallArgIdentsStack) <= 1 {
		e.fs.PushCode("std::mem::take(&mut " + e.Opt.memTakeLhsExpr + ")")
		e.Opt.MoveOptCount++
		return
	}

	// len() is read-only — skip all cloning for its arguments
	if e.Opt.currentCallIsLen && e.Opt.OptimizeMoves {
		e.fs.PushCode(argCode)
		return
	}

	// Skip redundant .clone() when vec element access already produced an owned value
	if e.Opt.argAlreadyCloned && e.Opt.OptimizeMoves {
		e.Opt.argAlreadyCloned = false
		e.Opt.MoveOptCount++
		e.fs.PushCode(argCode)
		return
	}

	// Add .clone() for non-Copy types passed as arguments
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
							e.fs.PushCode(argCode)
							return
						}
						argCode = argCode + ".clone()"
					}
				}
			}
		}
	}

	e.fs.PushCode(argCode)
}

func (e *RustEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	argTokens := e.fs.Reduce(string(PreVisitCallExprArgs))
	var args []string
	for _, t := range argTokens {
		if t.Content != "" {
			args = append(args, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(args, ", "))
}

func (e *RustEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCallExpr))
	funName := ""
	argsStr := ""
	if len(tokens) >= 1 {
		funName = tokens[0].Content
	}
	if len(tokens) >= 2 {
		argsStr = tokens[1].Content
	}

	// Clear callee state after processing call
	defer func() {
		e.Opt.currentCalleeName = ""
		e.Opt.currentCallIsLen = false
		if e.Opt.OptimizeRefs && len(e.Opt.refOptCalleeReadOnly) > 0 {
			e.Opt.refOptCalleeReadOnly = e.Opt.refOptCalleeReadOnly[:len(e.Opt.refOptCalleeReadOnly)-1]
		}
		if e.Opt.OptimizeRefs && len(e.Opt.refOptCalleeMutRef) > 0 {
			e.Opt.refOptCalleeMutRef = e.Opt.refOptCalleeMutRef[:len(e.Opt.refOptCalleeMutRef)-1]
		}
	}()

	// Local closure inlining: replace call with inline body block
	if ident, ok := node.Fun.(*ast.Ident); ok {
		if body, found := e.localClosureBodies[ident.Name]; found {
			// Extract the body from the closure code: "|| { body }" -> "{ body }"
			// Find the first '{' and wrap the body in a block
			if braceIdx := strings.Index(body, "{"); braceIdx >= 0 {
				e.fs.PushCode(body[braceIdx:])
			} else {
				e.fs.PushCode("{ " + body + " }")
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
					e.fs.PushCode(fmt.Sprintf("hmap::hashMapLen(&%s)", argsStr))
					e.Opt.RefOptCount++
				} else {
					e.fs.PushCode(fmt.Sprintf("hmap::hashMapLen(%s.clone())", argsStr))
				}
				return
			}
			// Check for string len
			argType := e.getExprGoType(node.Args[0])
			if argType != nil {
				if basic, ok := argType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
					e.fs.PushCode(fmt.Sprintf("%s.len() as i32", argsStr))
					return
				}
			}
		}
		e.fs.PushCode(fmt.Sprintf("len(&%s)", argsStr))
		return
	case "append":
		e.fs.PushCode(fmt.Sprintf("append(%s)", argsStr))
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
				e.fs.PushCode(fmt.Sprintf("%s = hmap::hashMapDelete(%s, Rc::new(%s))", mapName, mapName, keyStr))
			} else {
				e.fs.PushCode(fmt.Sprintf("%s = hmap::hashMapDelete(%s, Rc::new(%s%s))", mapName, mapName, keyStr, keyCast))
			}
		} else {
			e.fs.PushCode(fmt.Sprintf("hmap::hashMapDelete(%s)", argsStr))
		}
		return
	case "make":
		if len(node.Args) >= 1 {
			if mapType, ok := node.Args[0].(*ast.MapType); ok {
				keyTypeConst := e.getMapKeyTypeConst(mapType)
				e.fs.PushCode(fmt.Sprintf("hmap::newHashMap(%d)", keyTypeConst))
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
					e.fs.PushCode(fmt.Sprintf("vec![%s; %s as usize]", defaultVal, sizeArg))
				} else {
					e.fs.PushCode(fmt.Sprintf("Vec::<%s>::new()", elemType))
				}
				return
			}
		}
		e.fs.PushCode(fmt.Sprintf("make(%s)", argsStr))
		return
	}

	// Check if type conversion (e.g., int(x), string(x))
	if ident, ok := node.Fun.(*ast.Ident); ok {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if obj := e.pkg.TypesInfo.ObjectOf(ident); obj != nil {
				if _, isTypeName := obj.(*types.TypeName); isTypeName {
					rustType := e.qualifiedRustTypeName(obj.Type())
					if rustType == "String" {
						// string(x) -> format!("{}", x) or (x as u8 as char).to_string()
						e.fs.PushCode(fmt.Sprintf("format!(\"{{}}\", %s)", argsStr))
						return
					}
					// Numeric cast: ((x) as type) - inner parens for operator precedence
					e.fs.PushCode(fmt.Sprintf("((%s) as %s)", argsStr, rustType))
					return
				}
			}
		}
		// Fallback for type conversions
		if isConv, rustType := e.isTypeConversion(funName); isConv {
			e.fs.PushCode(fmt.Sprintf("((%s) as %s)", argsStr, rustType))
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
			e.fs.PushCode("println0()")
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
						e.fs.PushCode(fmt.Sprintf("printc(%s)", strings.TrimSpace(parts[1])))
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
						e.fs.PushCode(fmt.Sprintf("byte_to_char(%s)", strings.TrimSpace(parts[1])))
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
					e.fs.PushCode(fmt.Sprintf("(%s)(%s)", funName, argsStr))
					return
				}
			}
		}
	}

	e.fs.PushCode(fmt.Sprintf("%s(%s)", funName, argsStr))
}

// ============================================================
// Selector Expressions (a.b)
// ============================================================

func (e *RustEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitSelectorExprX))
	e.fs.PushCode(xCode)
}

func (e *RustEmitter) PostVisitSelectorExprSel(node *ast.Ident, indent int) {
	e.fs.Reduce(string(PreVisitSelectorExprSel))
	e.fs.PushCode(node.Name)
}

func (e *RustEmitter) PostVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSelectorExpr))
	xCode := ""
	selCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Content
	}
	if len(tokens) >= 2 {
		selCode = tokens[1].Content
	}

	if xCode == "os" && selCode == "Args" {
		e.fs.PushCode("std::env::args().collect::<Vec<String>>()")
		return
	}

	// Lower builtins: fmt.Println -> println, etc.
	loweredX := e.lowerToBuiltins(xCode)
	loweredSel := e.lowerToBuiltins(selCode)

	if loweredX == "" {
		// Check if selector is a type alias (only for non-package-qualified access)
		if _, isAlias := e.typeAliasMap[selCode]; isAlias {
			e.fs.PushCode(e.typeAliasMap[selCode])
			return
		}
		e.fs.PushCode(loweredSel)
	} else {
		// Use :: for package access, . for field access
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if ident, ok := node.X.(*ast.Ident); ok {
				if obj := e.pkg.TypesInfo.Uses[ident]; obj != nil {
					if _, isPkg := obj.(*types.PkgName); isPkg {
						e.fs.PushCode(loweredX + "::" + loweredSel)
						return
					}
				}
			}
		}
		e.fs.PushCode(loweredX + "." + loweredSel)
	}
}

// ============================================================
// Index Expressions (a[i])
// ============================================================

func (e *RustEmitter) PostVisitIndexExprX(node *ast.IndexExpr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitIndexExprX))
	e.fs.PushCode(xCode)
	e.lastIndexXCode = xCode
}

func (e *RustEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	idxCode := e.fs.ReduceToCode(string(PreVisitIndexExprIndex))
	e.fs.PushCode(idxCode)
	e.lastIndexKeyCode = idxCode
}

func (e *RustEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitIndexExpr))
	xCode := ""
	idxCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Content
	}
	if len(tokens) >= 2 {
		idxCode = tokens[1].Content
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
			e.Opt.RefOptCount++
		}
		e.fs.PushCodeWithType(
			fmt.Sprintf("hmap::hashMapGet(%s, %s)%s", mapRef, keyExpr, castExpr),
			e.getExprGoType(node),
		)
	} else {
		// Check for string indexing
		xType := e.getExprGoType(node.X)
		if xType != nil {
			if basic, ok := xType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
				e.fs.PushCode(fmt.Sprintf("%s.as_bytes()[(%s) as usize] as i8", xCode, idxCode))
				return
			}
		}
		// Add .clone() for non-Copy element types (but NOT on LHS of assignment)
		elemType := e.getExprGoType(node)
		if elemType != nil && !isCopyType(elemType) && !e.inAssignLhs {
			e.fs.PushCode(fmt.Sprintf("%s[(%s) as usize].clone()", xCode, idxCode))
			if e.callExprArgDepth > 0 {
				e.Opt.argAlreadyCloned = true
			}
		} else {
			e.fs.PushCode(fmt.Sprintf("%s[(%s) as usize]", xCode, idxCode))
		}
	}
}

// ============================================================
// Unary Expressions
// ============================================================

func (e *RustEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitUnaryExpr))
	op := node.Op.String()
	if op == "^" {
		e.fs.PushCode("!" + xCode)
	} else {
		e.fs.PushCode(op + xCode)
	}
}

// ============================================================
// Paren Expressions
// ============================================================

func (e *RustEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {
	inner := e.fs.ReduceToCode(string(PreVisitParenExpr))
	e.fs.PushCode("(" + inner + ")")
}

// ============================================================
// Slice Expressions (a[lo:hi])
// ============================================================

func (e *RustEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitSliceExprX))
	e.fs.PushCode(xCode)
}

func (e *RustEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitSliceExprXBegin))
}

func (e *RustEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	lowCode := e.fs.ReduceToCode(string(PreVisitSliceExprLow))
	e.fs.PushCode(lowCode)
}

func (e *RustEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitSliceExprXEnd))
}

func (e *RustEmitter) PostVisitSliceExprHigh(node ast.Expr, indent int) {
	highCode := e.fs.ReduceToCode(string(PreVisitSliceExprHigh))
	e.fs.PushCode(highCode)
}

func (e *RustEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSliceExpr))
	xCode := ""
	lowCode := ""
	highCode := ""

	idx := 0
	if idx < len(tokens) {
		xCode = tokens[idx].Content
		idx++
	}
	if node.Low != nil && idx < len(tokens) {
		lowCode = tokens[idx].Content
		idx++
	}
	if node.High != nil && idx < len(tokens) {
		highCode = tokens[idx].Content
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
			e.fs.PushCode(fmt.Sprintf("%s[(%s) as usize..].to_string()", xCode, lowCode))
		} else {
			e.fs.PushCode(fmt.Sprintf("%s[(%s) as usize..(%s) as usize].to_string()", xCode, lowCode, highCode))
		}
	} else {
		if highCode == "" {
			e.fs.PushCode(fmt.Sprintf("%s[(%s) as usize..].to_vec()", xCode, lowCode))
		} else {
			e.fs.PushCode(fmt.Sprintf("%s[(%s) as usize..(%s) as usize].to_vec()", xCode, lowCode, highCode))
		}
	}
}

// ============================================================
// Type Assertions
// ============================================================

func (e *RustEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	typeCode := e.fs.ReduceToCode(string(PreVisitTypeAssertExprType))
	e.fs.PushCode(typeCode)
}

func (e *RustEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitTypeAssertExprX))
	e.fs.PushCode(xCode)
}

func (e *RustEmitter) PostVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitTypeAssertExpr))
	typeCode := ""
	xCode := ""
	if len(tokens) >= 1 {
		typeCode = tokens[0].Content
	}
	if len(tokens) >= 2 {
		xCode = tokens[1].Content
	}
	// x.(Type) -> x.downcast_ref::<Type>().unwrap().clone()
	if typeCode != "" {
		e.fs.PushCode(fmt.Sprintf("%s.downcast_ref::<%s>().unwrap().clone()", xCode, typeCode))
	} else {
		e.fs.PushCode(xCode)
	}
}

// ============================================================
// Star Expressions (dereference — pass through)
// ============================================================

func (e *RustEmitter) PostVisitStarExpr(node *ast.StarExpr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitStarExpr))
	e.fs.PushCode(xCode)
}

// ============================================================
// Interface Type
// ============================================================

func (e *RustEmitter) PostVisitInterfaceType(node *ast.InterfaceType, indent int) {
	e.fs.Reduce(string(PreVisitInterfaceType))
	e.fs.Push("Rc<dyn Any>", TagType, nil)
}

// ============================================================
// Composite Literals
// ============================================================

func (e *RustEmitter) PreVisitCompositeLit(node *ast.CompositeLit, indent int) {
	e.compositeLitDepth++
}

func (e *RustEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitCompositeLitType))
}

func (e *RustEmitter) PostVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	eltCode := e.fs.ReduceToCode(string(PreVisitCompositeLitElt))
	e.fs.PushCode(eltCode)
}

func (e *RustEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	eltTokens := e.fs.Reduce(string(PreVisitCompositeLitElts))
	for _, t := range eltTokens {
		if t.Content != "" {
			e.fs.Push(t.Content, TagLiteral, nil)
		}
	}
}

func (e *RustEmitter) PostVisitCompositeLit(node *ast.CompositeLit, indent int) {
	defer func() { e.compositeLitDepth-- }()
	tokens := e.fs.Reduce(string(PreVisitCompositeLit))
	var elts []string
	for _, t := range tokens {
		if t.Content != "" {
			elts = append(elts, t.Content)
		}
	}
	eltsStr := strings.Join(elts, ", ")

	litType := e.getExprGoType(node)
	if litType == nil {
		e.fs.PushCode("vec![" + eltsStr + "]")
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
				for _, elt := range elts {
					parts := strings.SplitN(elt, ": ", 2)
					if len(parts) == 2 {
						key := parts[0]
						if dotIdx := strings.LastIndex(key, "."); dotIdx >= 0 {
							key = key[dotIdx+1:]
						}
						// Strip Rust package prefix (types::FieldName -> FieldName)
						if colonIdx := strings.LastIndex(key, "::"); colonIdx >= 0 {
							key = key[colonIdx+2:]
						}
						kvMap[key] = parts[1]
					}
				}
				var sb strings.Builder
				sb.WriteString(fmt.Sprintf("%s { ", typeName))
				first := true
				allFieldsProvided := len(kvMap) >= u.NumFields()
				for i := 0; i < u.NumFields(); i++ {
					fieldName := u.Field(i).Name()
					if val, ok := kvMap[fieldName]; ok {
						if !first {
							sb.WriteString(", ")
						}
						val = e.castSmallIntFieldValue(u.Field(i).Type(), val)
						val = e.cloneStructFieldValue(u.Field(i).Type(), val)
						sb.WriteString(fmt.Sprintf("%s: %s", fieldName, val))
						first = false
					}
				}
				if !allFieldsProvided {
					if !first {
						sb.WriteString(", ")
					}
					sb.WriteString("..Default::default() ")
				} else {
					sb.WriteString(" ")
				}
				sb.WriteString("}")
				e.fs.PushCode(sb.String())
				return
			}
		}
		// Positional struct literal or empty struct
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("%s { ", typeName))
		for i, elt := range elts {
			if i > 0 {
				sb.WriteString(", ")
			}
			if i < u.NumFields() {
				elt = e.castSmallIntFieldValue(u.Field(i).Type(), elt)
				elt = e.cloneStructFieldValue(u.Field(i).Type(), elt)
				sb.WriteString(fmt.Sprintf("%s: %s", u.Field(i).Name(), elt))
			}
		}
		if len(elts) < u.NumFields() {
			if len(elts) > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString("..Default::default()")
		}
		sb.WriteString(" }")
		e.fs.PushCode(sb.String())
	case *types.Slice:
		if len(elts) == 0 {
			elemRustType := e.qualifiedRustTypeName(u.Elem())
			e.fs.PushCode(fmt.Sprintf("Vec::<%s>::new()", elemRustType))
		} else {
			e.fs.PushCode(fmt.Sprintf("vec![%s]", eltsStr))
		}
	case *types.Map:
		keyTypeConst := 1
		if node.Type != nil {
			if mapType, ok := node.Type.(*ast.MapType); ok {
				keyTypeConst = e.getMapKeyTypeConst(mapType)
			}
		}
		if len(elts) == 0 {
			e.fs.PushCode(fmt.Sprintf("hmap::newHashMap(%d)", keyTypeConst))
		} else {
			// Build map with initial values
			e.nestedMapCounter++
			tmpVar := fmt.Sprintf("_m%d", e.nestedMapCounter)
			var sb strings.Builder
			sb.WriteString("{\n")
			sb.WriteString(fmt.Sprintf("let mut %s = hmap::newHashMap(%d);\n", tmpVar, keyTypeConst))

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
						keyExpr = fmt.Sprintf("Rc::new(%s)", keyStr)
					} else {
						keyExpr = fmt.Sprintf("Rc::new(%s%s)", keyStr, keyCast)
					}
					sb.WriteString(fmt.Sprintf("%s = hmap::hashMapSet(%s, %s, Rc::new(%s));\n", tmpVar, tmpVar, keyExpr, valStr))
				}
			}
			sb.WriteString(tmpVar)
			sb.WriteString("\n}")
			e.fs.PushCode(sb.String())
		}
	default:
		if len(elts) == 0 {
			e.fs.PushCode("Vec::new()")
		} else {
			e.fs.PushCode(fmt.Sprintf("vec![%s]", eltsStr))
		}
	}
}

// ============================================================
// KeyValue Expressions
// ============================================================

func (e *RustEmitter) PostVisitKeyValueExprKey(node ast.Expr, indent int) {
	keyCode := e.fs.ReduceToCode(string(PreVisitKeyValueExprKey))
	e.fs.PushCode(keyCode)
}

func (e *RustEmitter) PostVisitKeyValueExprValue(node ast.Expr, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitKeyValueExprValue))
	e.fs.PushCode(valCode)
}

func (e *RustEmitter) PostVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitKeyValueExpr))
	keyCode := ""
	valCode := ""
	if len(tokens) >= 1 {
		keyCode = tokens[0].Content
	}
	if len(tokens) >= 2 {
		valCode = tokens[1].Content
	}
	e.fs.PushCode(keyCode + ": " + valCode)
}

// ============================================================
// Array Type
// ============================================================

func (e *RustEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	typeTokens := e.fs.Reduce(string(PreVisitArrayType))
	elemType := ""
	for _, t := range typeTokens {
		elemType += t.Content
	}
	if elemType == "" {
		elemType = "i32"
	}
	e.fs.Push(fmt.Sprintf("Vec<%s>", elemType), TagType, nil)
}

// ============================================================
// Map Type
// ============================================================

func (e *RustEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitMapKeyType))
}

func (e *RustEmitter) PostVisitMapValueType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitMapValueType))
}

func (e *RustEmitter) PostVisitMapType(node *ast.MapType, indent int) {
	e.fs.Reduce(string(PreVisitMapType))
	e.fs.Push("hmap::HashMap", TagType, nil)
}

// ============================================================
// Function Type (Rc<dyn Fn(...)>)
// ============================================================

func (e *RustEmitter) PostVisitFuncTypeResult(node *ast.Field, index int, indent int) {
	resultCode := e.fs.ReduceToCode(string(PreVisitFuncTypeResult))
	e.fs.PushCode(resultCode)
}

func (e *RustEmitter) PostVisitFuncTypeResults(node *ast.FieldList, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncTypeResults))
	var resultTypes []string
	for _, t := range tokens {
		if t.Content != "" {
			resultTypes = append(resultTypes, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(resultTypes, ", "))
}

func (e *RustEmitter) PostVisitFuncTypeParam(node *ast.Field, index int, indent int) {
	paramCode := e.fs.ReduceToCode(string(PreVisitFuncTypeParam))
	e.fs.PushCode(paramCode)
}

func (e *RustEmitter) PostVisitFuncTypeParams(node *ast.FieldList, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncTypeParams))
	var paramTypes []string
	for _, t := range tokens {
		if t.Content != "" {
			paramTypes = append(paramTypes, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(paramTypes, ", "))
}

func (e *RustEmitter) PostVisitFuncType(node *ast.FuncType, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncType))
	resultTypes := ""
	paramTypes := ""
	if node.Results != nil && node.Results.NumFields() > 0 {
		if len(tokens) >= 1 {
			resultTypes = tokens[0].Content
		}
		if len(tokens) >= 2 {
			paramTypes = tokens[1].Content
		}
		e.fs.PushCode(fmt.Sprintf("Rc<dyn Fn(%s) -> %s>", paramTypes, resultTypes))
	} else {
		if len(tokens) >= 1 {
			paramTypes = tokens[0].Content
		}
		e.fs.PushCode(fmt.Sprintf("Rc<dyn Fn(%s)>", paramTypes))
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
	e.fs.Reduce(string(PreVisitFuncLitTypeParam))
	typeStr := "Rc<dyn Any>"
	if e.pkg != nil && e.pkg.TypesInfo != nil && len(node.Names) > 0 {
		if obj := e.pkg.TypesInfo.Defs[node.Names[0]]; obj != nil {
			typeStr = e.qualifiedRustTypeName(obj.Type())
		}
	}
	for _, name := range node.Names {
		escapedName := escapeRustKeyword(name.Name)
		e.fs.Push(fmt.Sprintf("mut %s: %s", escapedName, typeStr), TagIdent, nil)
	}
}

func (e *RustEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncLitTypeParams))
	var paramNames []string
	for _, t := range tokens {
		if t.Tag == TagIdent && t.Content != "" {
			paramNames = append(paramNames, t.Content)
		}
	}
	paramsStr := strings.Join(paramNames, ", ")
	e.fs.PushCode(paramsStr)
}

func (e *RustEmitter) PostVisitFuncLitTypeResults(node *ast.FieldList, indent int) {
	e.fs.Reduce(string(PreVisitFuncLitTypeResults))
}

func (e *RustEmitter) PostVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitFuncLitBody))
	e.fs.PushCode(bodyCode)
}

func (e *RustEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncLit))
	paramsCode := ""
	bodyCode := ""
	if len(tokens) == 1 {
		// No params were pushed (empty string tokens are dropped),
		// so the only token is the body
		bodyCode = tokens[0].Content
	} else if len(tokens) >= 2 {
		paramsCode = strings.TrimSpace(tokens[0].Content)
		bodyCode = tokens[1].Content
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

	// Wrap in Rc::new() only when used as a struct field value (inside composite literal)
	// Local variable closures and call arguments should be bare closures
	closureCode := fmt.Sprintf("|%s|%s %s", paramsCode, retType, bodyCode)
	if e.compositeLitDepth > 0 {
		e.fs.PushCode(fmt.Sprintf("Rc::new(%s)", closureCode))
	} else {
		e.fs.PushCode(closureCode)
	}
}
