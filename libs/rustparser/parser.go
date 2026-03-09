package rustparser

// Peek helpers

func peekTokenType(tokens []Token, pos int) int {
	if pos >= len(tokens) {
		return TokenEOF
	}
	return tokens[pos].Type
}

func peekTokenValue(tokens []Token, pos int) string {
	if pos >= len(tokens) {
		return ""
	}
	return tokens[pos].Value
}

func peekToken(tokens []Token, pos int) Token {
	if pos >= len(tokens) {
		return NewToken(TokenEOF, "", 0, 0)
	}
	return tokens[pos]
}

// Parse parses Rust source code and returns the AST
func Parse(input string) Node {
	tokens := Tokenize(input)
	module, finalPos := parseModule(tokens, 0)
	if finalPos > 0 {
		// suppress unused warning
	}
	return module
}

// parseModule parses the top-level module
func parseModule(tokens []Token, pos int) (Node, int) {
	module := NewNode(NodeModule)

	for pos < len(tokens) && peekTokenType(tokens, pos) != TokenEOF {
		tokenType := peekTokenType(tokens, pos)
		tokenValue := peekTokenValue(tokens, pos)

		// Skip semicolons at top level
		if tokenType == TokenSemicolon {
			pos = pos + 1
			continue
		}

		// fn definition
		if tokenType == TokenKeyword && tokenValue == "fn" {
			var item Node
			item, pos = parseFnDef(tokens, pos)
			module = AddChild(module, item)
			continue
		}

		// struct definition
		if tokenType == TokenKeyword && tokenValue == "struct" {
			var item Node
			item, pos = parseStructDef(tokens, pos)
			module = AddChild(module, item)
			continue
		}

		// impl block
		if tokenType == TokenKeyword && tokenValue == "impl" {
			var item Node
			item, pos = parseImplBlock(tokens, pos)
			module = AddChild(module, item)
			continue
		}

		// Skip pub keyword (treat pub fn, pub struct as fn, struct)
		if tokenType == TokenKeyword && tokenValue == "pub" {
			pos = pos + 1
			continue
		}

		// Otherwise parse as statement
		var stmt Node
		stmt, pos = parseStatement(tokens, pos)
		module = AddChild(module, stmt)
	}

	return module, pos
}

// parseFnDef parses: fn name(params) -> type { block }
func parseFnDef(tokens []Token, pos int) (Node, int) {
	fnLine := peekToken(tokens, pos).Line
	pos = pos + 1 // skip "fn"

	fnName := peekTokenValue(tokens, pos)
	pos = pos + 1 // skip name

	fn := NewNodeWithName(NodeFnDef, fnName)
	fn = SetLine(fn, fnLine)

	// Parse parameters
	if peekTokenType(tokens, pos) == TokenLParen {
		pos = pos + 1 // skip "("
		for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
			// Skip self parameter
			if peekTokenType(tokens, pos) == TokenAmpersand {
				pos = pos + 1
				if peekTokenValue(tokens, pos) == "self" {
					pos = pos + 1
					if peekTokenType(tokens, pos) == TokenComma {
						pos = pos + 1
					}
					continue
				}
			}
			if peekTokenValue(tokens, pos) == "self" {
				pos = pos + 1
				if peekTokenType(tokens, pos) == TokenComma {
					pos = pos + 1
				}
				continue
			}
			// Skip "mut" keyword in parameters
			if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "mut" {
				pos = pos + 1
			}

			paramName := peekTokenValue(tokens, pos)
			pos = pos + 1 // skip param name

			param := NewNodeWithName(NodeParam, paramName)

			if peekTokenType(tokens, pos) == TokenColon {
				pos = pos + 1 // skip ":"
				var typeNode Node
				typeNode, pos = parseTypeRef(tokens, pos)
				param = AddChild(param, typeNode)
			}

			fn = AddChild(fn, param)

			if peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1
			}
		}
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1 // skip ")"
		}
	}

	// Parse return type
	if peekTokenType(tokens, pos) == TokenArrow {
		pos = pos + 1 // skip "->"
		var retType Node
		retType, pos = parseTypeRef(tokens, pos)
		fn = AddChild(fn, retType)
	}

	// Parse body
	if peekTokenType(tokens, pos) == TokenLBrace {
		var body Node
		body, pos = parseBlock(tokens, pos)
		fn = AddChild(fn, body)
	}

	return fn, pos
}

// parseTypeRef parses a type reference like i32, String, &str
func parseTypeRef(tokens []Token, pos int) (Node, int) {
	typeName := ""
	if peekTokenType(tokens, pos) == TokenAmpersand {
		typeName = "&"
		pos = pos + 1
	}
	typeName += peekTokenValue(tokens, pos)
	pos = pos + 1
	typeNode := NewNodeWithName(NodeTypeRef, typeName)
	return typeNode, pos
}

// parseStructDef parses: struct Name { field: Type, ... }
func parseStructDef(tokens []Token, pos int) (Node, int) {
	structLine := peekToken(tokens, pos).Line
	pos = pos + 1 // skip "struct"

	structName := peekTokenValue(tokens, pos)
	pos = pos + 1 // skip name

	st := NewNodeWithName(NodeStructDef, structName)
	st = SetLine(st, structLine)

	if peekTokenType(tokens, pos) == TokenLBrace {
		pos = pos + 1 // skip "{"
		for peekTokenType(tokens, pos) != TokenRBrace && peekTokenType(tokens, pos) != TokenEOF {
			// Skip pub keyword on fields
			if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "pub" {
				pos = pos + 1
			}

			fieldName := peekTokenValue(tokens, pos)
			pos = pos + 1 // skip field name

			field := NewNodeWithName(NodeField, fieldName)

			if peekTokenType(tokens, pos) == TokenColon {
				pos = pos + 1 // skip ":"
				var typeNode Node
				typeNode, pos = parseTypeRef(tokens, pos)
				field = AddChild(field, typeNode)
			}

			st = AddChild(st, field)

			if peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1
			}
		}
		if peekTokenType(tokens, pos) == TokenRBrace {
			pos = pos + 1 // skip "}"
		}
	}

	return st, pos
}

// parseImplBlock parses: impl Name { fn ... fn ... }
func parseImplBlock(tokens []Token, pos int) (Node, int) {
	implLine := peekToken(tokens, pos).Line
	pos = pos + 1 // skip "impl"

	implName := peekTokenValue(tokens, pos)
	pos = pos + 1 // skip name

	impl := NewNodeWithName(NodeImplBlock, implName)
	impl = SetLine(impl, implLine)

	if peekTokenType(tokens, pos) == TokenLBrace {
		pos = pos + 1 // skip "{"
		for peekTokenType(tokens, pos) != TokenRBrace && peekTokenType(tokens, pos) != TokenEOF {
			// Skip pub keyword
			if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "pub" {
				pos = pos + 1
			}
			if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "fn" {
				var fn Node
				fn, pos = parseFnDef(tokens, pos)
				impl = AddChild(impl, fn)
			} else {
				pos = pos + 1 // skip unknown
			}
		}
		if peekTokenType(tokens, pos) == TokenRBrace {
			pos = pos + 1 // skip "}"
		}
	}

	return impl, pos
}

// parseBlock parses: { stmt* }
func parseBlock(tokens []Token, pos int) (Node, int) {
	block := NewNode(NodeBlock)
	block = SetLine(block, peekToken(tokens, pos).Line)

	pos = pos + 1 // skip "{"

	for peekTokenType(tokens, pos) != TokenRBrace && peekTokenType(tokens, pos) != TokenEOF {
		// Skip semicolons
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
			continue
		}

		var stmt Node
		stmt, pos = parseStatement(tokens, pos)
		block = AddChild(block, stmt)
	}

	if peekTokenType(tokens, pos) == TokenRBrace {
		pos = pos + 1 // skip "}"
	}

	return block, pos
}

// parseStatement parses a single statement
func parseStatement(tokens []Token, pos int) (Node, int) {
	tokenType := peekTokenType(tokens, pos)
	tokenValue := peekTokenValue(tokens, pos)

	// let binding
	if tokenType == TokenKeyword && tokenValue == "let" {
		return parseLetBinding(tokens, pos)
	}

	// if statement
	if tokenType == TokenKeyword && tokenValue == "if" {
		return parseIfStmt(tokens, pos)
	}

	// while loop
	if tokenType == TokenKeyword && tokenValue == "while" {
		return parseWhileStmt(tokens, pos)
	}

	// loop
	if tokenType == TokenKeyword && tokenValue == "loop" {
		return parseLoopStmt(tokens, pos)
	}

	// for loop
	if tokenType == TokenKeyword && tokenValue == "for" {
		return parseForStmt(tokens, pos)
	}

	// return statement
	if tokenType == TokenKeyword && tokenValue == "return" {
		return parseReturnStmt(tokens, pos)
	}

	// break
	if tokenType == TokenKeyword && tokenValue == "break" {
		node := NewNode(NodeBreak)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return node, pos
	}

	// continue
	if tokenType == TokenKeyword && tokenValue == "continue" {
		node := NewNode(NodeContinue)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return node, pos
	}

	// Expression statement (may be assignment)
	return parseExprStmtOrAssign(tokens, pos)
}

// parseLetBinding parses: let [mut] name [: type] = expr;
func parseLetBinding(tokens []Token, pos int) (Node, int) {
	letLine := peekToken(tokens, pos).Line
	pos = pos + 1 // skip "let"

	isMut := false
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "mut" {
		isMut = true
		pos = pos + 1
	}

	varName := peekTokenValue(tokens, pos)
	pos = pos + 1

	node := NewNodeWithName(NodeLetBinding, varName)
	node = SetLine(node, letLine)
	if isMut {
		node.Op = "mut"
	}

	// Optional type annotation
	if peekTokenType(tokens, pos) == TokenColon {
		pos = pos + 1 // skip ":"
		var typeNode Node
		typeNode, pos = parseTypeRef(tokens, pos)
		node = AddChild(node, typeNode)
	}

	// = expr
	if peekTokenType(tokens, pos) == TokenAssign {
		pos = pos + 1 // skip "="
		var expr Node
		expr, pos = parseExpr(tokens, pos)
		node = AddChild(node, expr)
	}

	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return node, pos
}

// parseIfStmt parses: if expr { block } [else { block }]
func parseIfStmt(tokens []Token, pos int) (Node, int) {
	ifLine := peekToken(tokens, pos).Line
	pos = pos + 1 // skip "if"

	node := NewNode(NodeIf)
	node = SetLine(node, ifLine)

	// Condition
	var cond Node
	cond, pos = parseExpr(tokens, pos)
	node = AddChild(node, cond)

	// Then block
	var thenBlock Node
	thenBlock, pos = parseBlock(tokens, pos)
	node = AddChild(node, thenBlock)

	// Optional else
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "else" {
		pos = pos + 1 // skip "else"
		if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "if" {
			var elseIf Node
			elseIf, pos = parseIfStmt(tokens, pos)
			node = AddChild(node, elseIf)
		} else {
			var elseBlock Node
			elseBlock, pos = parseBlock(tokens, pos)
			node = AddChild(node, elseBlock)
		}
	}

	return node, pos
}

// parseWhileStmt parses: while expr { block }
func parseWhileStmt(tokens []Token, pos int) (Node, int) {
	whileLine := peekToken(tokens, pos).Line
	pos = pos + 1 // skip "while"

	node := NewNode(NodeWhile)
	node = SetLine(node, whileLine)

	var cond Node
	cond, pos = parseExpr(tokens, pos)
	node = AddChild(node, cond)

	var body Node
	body, pos = parseBlock(tokens, pos)
	node = AddChild(node, body)

	return node, pos
}

// parseLoopStmt parses: loop { block }
func parseLoopStmt(tokens []Token, pos int) (Node, int) {
	loopLine := peekToken(tokens, pos).Line
	pos = pos + 1 // skip "loop"

	node := NewNode(NodeLoop)
	node = SetLine(node, loopLine)

	var body Node
	body, pos = parseBlock(tokens, pos)
	node = AddChild(node, body)

	return node, pos
}

// parseForStmt parses: for ident in expr { block }
func parseForStmt(tokens []Token, pos int) (Node, int) {
	forLine := peekToken(tokens, pos).Line
	pos = pos + 1 // skip "for"

	iterName := peekTokenValue(tokens, pos)
	pos = pos + 1

	node := NewNodeWithName(NodeForLoop, iterName)
	node = SetLine(node, forLine)

	// skip "in"
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "in" {
		pos = pos + 1
	}

	// Iterator expression
	var iterExpr Node
	iterExpr, pos = parseExpr(tokens, pos)
	node = AddChild(node, iterExpr)

	// Body
	var body Node
	body, pos = parseBlock(tokens, pos)
	node = AddChild(node, body)

	return node, pos
}

// parseReturnStmt parses: return [expr];
func parseReturnStmt(tokens []Token, pos int) (Node, int) {
	retLine := peekToken(tokens, pos).Line
	pos = pos + 1 // skip "return"

	node := NewNode(NodeReturn)
	node = SetLine(node, retLine)

	// Optional expression
	if peekTokenType(tokens, pos) != TokenSemicolon && peekTokenType(tokens, pos) != TokenRBrace {
		var expr Node
		expr, pos = parseExpr(tokens, pos)
		node = AddChild(node, expr)
	}

	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return node, pos
}

// parseExprStmtOrAssign parses expression statement or assignment
func parseExprStmtOrAssign(tokens []Token, pos int) (Node, int) {
	stmtLine := peekToken(tokens, pos).Line

	var expr Node
	expr, pos = parseExpr(tokens, pos)

	// Check for assignment
	if peekTokenType(tokens, pos) == TokenAssign {
		pos = pos + 1 // skip "="
		assign := NewNode(NodeAssign)
		assign = SetLine(assign, stmtLine)
		assign = AddChild(assign, expr)
		var rhs Node
		rhs, pos = parseExpr(tokens, pos)
		assign = AddChild(assign, rhs)
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return assign, pos
	}

	// Expression statement
	exprStmt := NewNode(NodeExprStmt)
	exprStmt = SetLine(exprStmt, stmtLine)
	exprStmt = AddChild(exprStmt, expr)
	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}
	return exprStmt, pos
}

// Expression parsing with operator precedence

// parseExpr parses: logical_or
func parseExpr(tokens []Token, pos int) (Node, int) {
	return parseLogicalOr(tokens, pos)
}

// parseLogicalOr parses: logical_and (|| logical_and)*
func parseLogicalOr(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseLogicalAnd(tokens, pos)

	for peekTokenType(tokens, pos) == TokenOrOr {
		op := peekTokenValue(tokens, pos)
		pos = pos + 1
		var right Node
		right, pos = parseLogicalAnd(tokens, pos)
		binOp := NewNodeWithOp(NodeBinOp, op)
		binOp = AddChild(binOp, left)
		binOp = AddChild(binOp, right)
		left = binOp
	}

	return left, pos
}

// parseLogicalAnd parses: comparison (&& comparison)*
func parseLogicalAnd(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseComparison(tokens, pos)

	for peekTokenType(tokens, pos) == TokenAndAnd {
		op := peekTokenValue(tokens, pos)
		pos = pos + 1
		var right Node
		right, pos = parseComparison(tokens, pos)
		binOp := NewNodeWithOp(NodeBinOp, op)
		binOp = AddChild(binOp, left)
		binOp = AddChild(binOp, right)
		left = binOp
	}

	return left, pos
}

// parseComparison parses: addition ((==|!=|<|>|<=|>=) addition)*
func parseComparison(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseAddition(tokens, pos)

	for isComparisonOp(peekTokenType(tokens, pos)) {
		op := peekTokenValue(tokens, pos)
		pos = pos + 1
		var right Node
		right, pos = parseAddition(tokens, pos)
		binOp := NewNodeWithOp(NodeBinOp, op)
		binOp = AddChild(binOp, left)
		binOp = AddChild(binOp, right)
		left = binOp
	}

	return left, pos
}

func isComparisonOp(t int) bool {
	if t == TokenEq {
		return true
	}
	if t == TokenNeq {
		return true
	}
	if t == TokenLt {
		return true
	}
	if t == TokenGt {
		return true
	}
	if t == TokenLe {
		return true
	}
	if t == TokenGe {
		return true
	}
	return false
}

// parseAddition parses: multiplication ((+|-) multiplication)*
func parseAddition(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseMultiplication(tokens, pos)

	for peekTokenType(tokens, pos) == TokenPlus || peekTokenType(tokens, pos) == TokenMinus {
		op := peekTokenValue(tokens, pos)
		pos = pos + 1
		var right Node
		right, pos = parseMultiplication(tokens, pos)
		binOp := NewNodeWithOp(NodeBinOp, op)
		binOp = AddChild(binOp, left)
		binOp = AddChild(binOp, right)
		left = binOp
	}

	return left, pos
}

// parseMultiplication parses: unary ((*|/|%) unary)*
func parseMultiplication(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseUnary(tokens, pos)

	for peekTokenType(tokens, pos) == TokenStar || peekTokenType(tokens, pos) == TokenSlash || peekTokenType(tokens, pos) == TokenPercent {
		op := peekTokenValue(tokens, pos)
		pos = pos + 1
		var right Node
		right, pos = parseUnary(tokens, pos)
		binOp := NewNodeWithOp(NodeBinOp, op)
		binOp = AddChild(binOp, left)
		binOp = AddChild(binOp, right)
		left = binOp
	}

	return left, pos
}

// parseUnary parses: (!|-)? primary
func parseUnary(tokens []Token, pos int) (Node, int) {
	if peekTokenType(tokens, pos) == TokenExcl {
		pos = pos + 1
		var operand Node
		operand, pos = parseUnary(tokens, pos)
		unary := NewNodeWithOp(NodeUnaryOp, "!")
		unary = AddChild(unary, operand)
		return unary, pos
	}
	if peekTokenType(tokens, pos) == TokenMinus {
		pos = pos + 1
		var operand Node
		operand, pos = parseUnary(tokens, pos)
		unary := NewNodeWithOp(NodeUnaryOp, "-")
		unary = AddChild(unary, operand)
		return unary, pos
	}
	return parsePrimary(tokens, pos)
}

// parsePrimary parses: number | string | bool | ident | ident(args) | (expr)
func parsePrimary(tokens []Token, pos int) (Node, int) {
	tokenType := peekTokenType(tokens, pos)
	tokenValue := peekTokenValue(tokens, pos)
	tokenLine := peekToken(tokens, pos).Line

	// Number
	if tokenType == TokenNumber {
		node := NewNodeWithValue(NodeNum, tokenValue)
		node = SetLine(node, tokenLine)
		pos = pos + 1
		return node, pos
	}

	// String
	if tokenType == TokenString {
		node := NewNodeWithValue(NodeStr, tokenValue)
		node = SetLine(node, tokenLine)
		pos = pos + 1
		return node, pos
	}

	// Bool
	if tokenType == TokenKeyword && (tokenValue == "true" || tokenValue == "false") {
		node := NewNodeWithValue(NodeBool, tokenValue)
		node = SetLine(node, tokenLine)
		pos = pos + 1
		return node, pos
	}

	// Parenthesized expression
	if tokenType == TokenLParen {
		pos = pos + 1 // skip "("
		var expr Node
		expr, pos = parseExpr(tokens, pos)
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1 // skip ")"
		}
		return expr, pos
	}

	// Identifier (possibly followed by function call or field access)
	if tokenType == TokenIdent || tokenType == TokenKeyword {
		node := NewNodeWithName(NodeName, tokenValue)
		node = SetLine(node, tokenLine)
		pos = pos + 1

		// Check for function call: ident(args)
		for peekTokenType(tokens, pos) == TokenLParen || peekTokenType(tokens, pos) == TokenDot {
			if peekTokenType(tokens, pos) == TokenLParen {
				pos = pos + 1 // skip "("
				call := NewNodeWithName(NodeCall, node.Name)
				call = SetLine(call, tokenLine)
				// Parse arguments
				for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
					var arg Node
					arg, pos = parseExpr(tokens, pos)
					call = AddChild(call, arg)
					if peekTokenType(tokens, pos) == TokenComma {
						pos = pos + 1
					}
				}
				if peekTokenType(tokens, pos) == TokenRParen {
					pos = pos + 1 // skip ")"
				}
				node = call
			} else if peekTokenType(tokens, pos) == TokenDot {
				pos = pos + 1 // skip "."
				fieldName := peekTokenValue(tokens, pos)
				pos = pos + 1
				access := NewNodeWithName(NodeFieldAccess, fieldName)
				access = SetLine(access, tokenLine)
				access = AddChild(access, node)
				node = access
			}
		}

		return node, pos
	}

	// Fallback: skip unknown token
	unknown := NewNodeWithValue(NodeName, tokenValue)
	unknown = SetLine(unknown, tokenLine)
	pos = pos + 1
	return unknown, pos
}
