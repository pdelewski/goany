package pyparser

// Parse parses Python source code and returns the AST
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

	// Skip leading newlines
	for pos < len(tokens) && peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	for pos < len(tokens) && peekTokenType(tokens, pos) != TokenEOF {
		// Skip newlines between statements
		for pos < len(tokens) && peekTokenType(tokens, pos) == TokenNewline {
			pos = pos + 1
		}

		if peekTokenType(tokens, pos) == TokenEOF {
			break
		}

		var stmt Node
		stmt, pos = parseStatement(tokens, pos)
		module = AddChild(module, stmt)
	}

	return module, pos
}

// parseStatement parses a single statement
func parseStatement(tokens []Token, pos int) (Node, int) {
	tokenType := peekTokenType(tokens, pos)
	tokenValue := peekTokenValue(tokens, pos)

	// def statement
	if tokenType == TokenKeyword && tokenValue == "def" {
		return parseFunctionDef(tokens, pos)
	}

	// return statement
	if tokenType == TokenKeyword && tokenValue == "return" {
		return parseReturn(tokens, pos)
	}

	// if statement
	if tokenType == TokenKeyword && tokenValue == "if" {
		return parseIf(tokens, pos)
	}

	// for statement
	if tokenType == TokenKeyword && tokenValue == "for" {
		return parseFor(tokens, pos)
	}

	// while statement
	if tokenType == TokenKeyword && tokenValue == "while" {
		return parseWhile(tokens, pos)
	}

	// pass statement
	if tokenType == TokenKeyword && tokenValue == "pass" {
		node := NewNode(NodePass)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		// Skip newline
		if peekTokenType(tokens, pos) == TokenNewline {
			pos = pos + 1
		}
		return node, pos
	}

	// break statement
	if tokenType == TokenKeyword && tokenValue == "break" {
		node := NewNode(NodeBreak)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		if peekTokenType(tokens, pos) == TokenNewline {
			pos = pos + 1
		}
		return node, pos
	}

	// continue statement
	if tokenType == TokenKeyword && tokenValue == "continue" {
		node := NewNode(NodeContinue)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		if peekTokenType(tokens, pos) == TokenNewline {
			pos = pos + 1
		}
		return node, pos
	}

	// import statement
	if tokenType == TokenKeyword && tokenValue == "import" {
		return parseImport(tokens, pos)
	}

	// from...import statement
	if tokenType == TokenKeyword && tokenValue == "from" {
		return parseImportFrom(tokens, pos)
	}

	// decorator
	if tokenType == TokenAt {
		return parseDecorated(tokens, pos)
	}

	// print statement (treat as function call)
	if tokenType == TokenKeyword && tokenValue == "print" {
		return parseExpressionStatement(tokens, pos)
	}

	// Assignment or expression statement
	// Look ahead to see if it's an assignment
	if tokenType == TokenIdentifier {
		// Check if next token is =
		if peekTokenType(tokens, pos+1) == TokenAssign {
			return parseAssignment(tokens, pos)
		}
	}

	// Expression statement
	return parseExpressionStatement(tokens, pos)
}

// parseFunctionDef parses a function definition
func parseFunctionDef(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeFunctionDef)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'def'
	pos = pos + 1

	// Function name
	if peekTokenType(tokens, pos) == TokenIdentifier {
		node.Name = peekTokenValue(tokens, pos)
		pos = pos + 1
	}

	// Parameters
	paramList := NewNode(NodeList) // Use List node for parameters
	if peekTokenType(tokens, pos) == TokenLParen {
		pos = pos + 1
		for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
			// Check for **kwargs
			if peekTokenType(tokens, pos) == TokenDoubleStar {
				pos = pos + 1
				if peekTokenType(tokens, pos) == TokenIdentifier {
					param := NewNodeWithName(NodeKwArg, peekTokenValue(tokens, pos))
					paramList = AddChild(paramList, param)
					pos = pos + 1
				}
			} else if peekTokenType(tokens, pos) == TokenStar {
				// Check for *args
				pos = pos + 1
				if peekTokenType(tokens, pos) == TokenIdentifier {
					param := NewNodeWithName(NodeStarArg, peekTokenValue(tokens, pos))
					paramList = AddChild(paramList, param)
					pos = pos + 1
				}
			} else if peekTokenType(tokens, pos) == TokenIdentifier {
				param := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
				paramList = AddChild(paramList, param)
				pos = pos + 1
			}
			if peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1
			}
		}
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1
		}
	}
	node = AddChild(node, paramList)

	// Colon
	if peekTokenType(tokens, pos) == TokenColon {
		pos = pos + 1
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	// Body (indented block)
	var body Node
	body, pos = parseBlock(tokens, pos)
	node = AddChild(node, body)

	return node, pos
}

// parseReturn parses a return statement
func parseReturn(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeReturn)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'return'
	pos = pos + 1

	// Return value (optional)
	if peekTokenType(tokens, pos) != TokenNewline && peekTokenType(tokens, pos) != TokenEOF {
		var expr Node
		expr, pos = parseExpression(tokens, pos)
		node = AddChild(node, expr)
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	return node, pos
}

// parseIf parses an if statement
func parseIf(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeIf)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'if'
	pos = pos + 1

	// Condition
	var condition Node
	condition, pos = parseExpression(tokens, pos)
	node = AddChild(node, condition)

	// Colon
	if peekTokenType(tokens, pos) == TokenColon {
		pos = pos + 1
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	// Body
	var body Node
	body, pos = parseBlock(tokens, pos)
	node = AddChild(node, body)

	// Handle elif/else
	for peekTokenType(tokens, pos) == TokenKeyword {
		value := peekTokenValue(tokens, pos)
		if value == "elif" {
			var elifNode Node
			elifNode, pos = parseElif(tokens, pos)
			node = AddChild(node, elifNode)
		} else if value == "else" {
			var elseNode Node
			elseNode, pos = parseElse(tokens, pos)
			node = AddChild(node, elseNode)
			break // else is always last
		} else {
			break
		}
	}

	return node, pos
}

// parseElif parses an elif clause
func parseElif(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeElif)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'elif'
	pos = pos + 1

	// Condition
	var condition Node
	condition, pos = parseExpression(tokens, pos)
	node = AddChild(node, condition)

	// Colon
	if peekTokenType(tokens, pos) == TokenColon {
		pos = pos + 1
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	// Body
	var body Node
	body, pos = parseBlock(tokens, pos)
	node = AddChild(node, body)

	return node, pos
}

// parseElse parses an else clause
func parseElse(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeElse)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'else'
	pos = pos + 1

	// Colon
	if peekTokenType(tokens, pos) == TokenColon {
		pos = pos + 1
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	// Body
	var body Node
	body, pos = parseBlock(tokens, pos)
	node = AddChild(node, body)

	return node, pos
}

// parseFor parses a for statement
func parseFor(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeFor)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'for'
	pos = pos + 1

	// Loop variable
	if peekTokenType(tokens, pos) == TokenIdentifier {
		varNode := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
		node = AddChild(node, varNode)
		pos = pos + 1
	}

	// Skip 'in'
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "in" {
		pos = pos + 1
	}

	// Iterable
	var iterable Node
	iterable, pos = parseExpression(tokens, pos)
	node = AddChild(node, iterable)

	// Colon
	if peekTokenType(tokens, pos) == TokenColon {
		pos = pos + 1
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	// Body
	var body Node
	body, pos = parseBlock(tokens, pos)
	node = AddChild(node, body)

	return node, pos
}

// parseWhile parses a while statement
func parseWhile(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeWhile)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'while'
	pos = pos + 1

	// Condition
	var condition Node
	condition, pos = parseExpression(tokens, pos)
	node = AddChild(node, condition)

	// Colon
	if peekTokenType(tokens, pos) == TokenColon {
		pos = pos + 1
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	// Body
	var body Node
	body, pos = parseBlock(tokens, pos)
	node = AddChild(node, body)

	return node, pos
}

// parseAssignment parses an assignment statement
func parseAssignment(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeAssign)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Target
	target := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
	node = AddChild(node, target)
	pos = pos + 1

	// Skip '='
	if peekTokenType(tokens, pos) == TokenAssign {
		pos = pos + 1
	}

	// Value
	var value Node
	value, pos = parseExpression(tokens, pos)
	node = AddChild(node, value)

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	return node, pos
}

// parseExpressionStatement parses an expression statement
func parseExpressionStatement(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeExpr)
	node = SetLine(node, peekToken(tokens, pos).Line)

	var expr Node
	expr, pos = parseExpression(tokens, pos)
	node = AddChild(node, expr)

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	return node, pos
}

// parseBlock parses an indented block of statements
func parseBlock(tokens []Token, pos int) (Node, int) {
	block := NewNode(NodeList) // Use List to hold block statements

	// Expect INDENT
	if peekTokenType(tokens, pos) == TokenIndent {
		pos = pos + 1
	}

	// Parse statements until DEDENT
	for peekTokenType(tokens, pos) != TokenDedent && peekTokenType(tokens, pos) != TokenEOF {
		// Skip newlines
		for peekTokenType(tokens, pos) == TokenNewline {
			pos = pos + 1
		}

		if peekTokenType(tokens, pos) == TokenDedent || peekTokenType(tokens, pos) == TokenEOF {
			break
		}

		var stmt Node
		stmt, pos = parseStatement(tokens, pos)
		block = AddChild(block, stmt)
	}

	// Consume DEDENT
	if peekTokenType(tokens, pos) == TokenDedent {
		pos = pos + 1
	}

	return block, pos
}

// parseExpression parses an expression (handles 'or')
func parseExpression(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseAndExpr(tokens, pos)

	for peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "or" {
		op := peekTokenValue(tokens, pos)
		pos = pos + 1
		var right Node
		right, pos = parseAndExpr(tokens, pos)
		node := NewNodeWithOp(NodeBinOp, op)
		node = AddChild(node, left)
		node = AddChild(node, right)
		left = node
	}

	return left, pos
}

// parseAndExpr parses 'and' expressions
func parseAndExpr(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseNotExpr(tokens, pos)

	for peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "and" {
		op := peekTokenValue(tokens, pos)
		pos = pos + 1
		var right Node
		right, pos = parseNotExpr(tokens, pos)
		node := NewNodeWithOp(NodeBinOp, op)
		node = AddChild(node, left)
		node = AddChild(node, right)
		left = node
	}

	return left, pos
}

// parseNotExpr parses 'not' expressions
func parseNotExpr(tokens []Token, pos int) (Node, int) {
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "not" {
		pos = pos + 1
		var operand Node
		operand, pos = parseNotExpr(tokens, pos)
		node := NewNodeWithOp(NodeUnaryOp, "not")
		node = AddChild(node, operand)
		return node, pos
	}
	return parseComparison(tokens, pos)
}

// parseComparison parses comparison expressions
func parseComparison(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseArithExpr(tokens, pos)

	// Check for comparison operators
	tokenType := peekTokenType(tokens, pos)
	tokenValue := peekTokenValue(tokens, pos)

	if tokenType == TokenOperator {
		if tokenValue == "==" || tokenValue == "!=" || tokenValue == "<" || tokenValue == ">" || tokenValue == "<=" || tokenValue == ">=" {
			op := tokenValue
			pos = pos + 1
			var right Node
			right, pos = parseArithExpr(tokens, pos)
			node := NewNodeWithOp(NodeCompare, op)
			node = AddChild(node, left)
			node = AddChild(node, right)
			return node, pos
		}
	}

	return left, pos
}

// parseArithExpr parses addition and subtraction
func parseArithExpr(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseTerm(tokens, pos)

	for peekTokenType(tokens, pos) == TokenOperator {
		op := peekTokenValue(tokens, pos)
		if op != "+" && op != "-" {
			break
		}
		pos = pos + 1
		var right Node
		right, pos = parseTerm(tokens, pos)
		node := NewNodeWithOp(NodeBinOp, op)
		node = AddChild(node, left)
		node = AddChild(node, right)
		left = node
	}

	return left, pos
}

// parseTerm parses multiplication, division, and modulo
func parseTerm(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseUnary(tokens, pos)

	for {
		tokenType := peekTokenType(tokens, pos)
		op := peekTokenValue(tokens, pos)

		// Handle * (TokenStar) for multiplication
		if tokenType == TokenStar {
			pos = pos + 1
			var right Node
			right, pos = parseUnary(tokens, pos)
			node := NewNodeWithOp(NodeBinOp, "*")
			node = AddChild(node, left)
			node = AddChild(node, right)
			left = node
			continue
		}

		// Handle ** (TokenDoubleStar) for power
		if tokenType == TokenDoubleStar {
			pos = pos + 1
			var right Node
			right, pos = parseUnary(tokens, pos)
			node := NewNodeWithOp(NodeBinOp, "**")
			node = AddChild(node, left)
			node = AddChild(node, right)
			left = node
			continue
		}

		// Handle /, %, // as operators
		if tokenType == TokenOperator {
			if op != "/" && op != "%" && op != "//" {
				break
			}
			pos = pos + 1
			var right Node
			right, pos = parseUnary(tokens, pos)
			node := NewNodeWithOp(NodeBinOp, op)
			node = AddChild(node, left)
			node = AddChild(node, right)
			left = node
			continue
		}

		break
	}

	return left, pos
}

// parseUnary parses unary operators (-, +)
func parseUnary(tokens []Token, pos int) (Node, int) {
	if peekTokenType(tokens, pos) == TokenOperator {
		op := peekTokenValue(tokens, pos)
		if op == "-" || op == "+" {
			pos = pos + 1
			var operand Node
			operand, pos = parseUnary(tokens, pos)
			node := NewNodeWithOp(NodeUnaryOp, op)
			node = AddChild(node, operand)
			return node, pos
		}
	}
	return parsePrimary(tokens, pos)
}

// parsePrimary parses primary expressions (atoms, calls, subscripts)
func parsePrimary(tokens []Token, pos int) (Node, int) {
	var node Node
	node, pos = parseAtom(tokens, pos)

	// Handle calls and subscripts
	for {
		if peekTokenType(tokens, pos) == TokenLParen {
			// Function call
			node, pos = parseCall(node, tokens, pos)
		} else if peekTokenType(tokens, pos) == TokenLBracket {
			// Subscript
			node, pos = parseSubscript(node, tokens, pos)
		} else if peekTokenType(tokens, pos) == TokenDot {
			// Attribute access (simplified - just parse as name)
			pos = pos + 1
			if peekTokenType(tokens, pos) == TokenIdentifier {
				attrName := peekTokenValue(tokens, pos)
				pos = pos + 1
				// Create a simple attribute node using BinOp with "." operator
				attrNode := NewNodeWithOp(NodeBinOp, ".")
				attrNode = AddChild(attrNode, node)
				attrNode = AddChild(attrNode, NewNodeWithName(NodeName, attrName))
				node = attrNode
			}
		} else {
			break
		}
	}

	return node, pos
}

// parseAtom parses atomic expressions
func parseAtom(tokens []Token, pos int) (Node, int) {
	tokenType := peekTokenType(tokens, pos)
	tokenValue := peekTokenValue(tokens, pos)

	// Number
	if tokenType == TokenNumber {
		node := NewNodeWithValue(NodeNum, tokenValue)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		return node, pos
	}

	// String
	if tokenType == TokenString {
		node := NewNodeWithValue(NodeStr, tokenValue)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		return node, pos
	}

	// True/False
	if tokenType == TokenKeyword && (tokenValue == "True" || tokenValue == "False") {
		node := NewNodeWithValue(NodeBool, tokenValue)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		return node, pos
	}

	// None
	if tokenType == TokenKeyword && tokenValue == "None" {
		node := NewNode(NodeNone)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		return node, pos
	}

	// Lambda expression
	if tokenType == TokenKeyword && tokenValue == "lambda" {
		return parseLambda(tokens, pos)
	}

	// Identifier (including keywords like print, range that are used as functions)
	if tokenType == TokenIdentifier || (tokenType == TokenKeyword && (tokenValue == "print" || tokenValue == "range")) {
		node := NewNodeWithName(NodeName, tokenValue)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		return node, pos
	}

	// List literal
	if tokenType == TokenLBracket {
		return parseListLiteral(tokens, pos)
	}

	// Dict literal
	if tokenType == TokenLBrace {
		return parseDictLiteral(tokens, pos)
	}

	// Parenthesized expression
	if tokenType == TokenLParen {
		pos = pos + 1
		var expr Node
		expr, pos = parseExpression(tokens, pos)
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1
		}
		return expr, pos
	}

	// Default: return empty name node
	return NewNode(NodeName), pos
}

// parseCall parses a function call
func parseCall(funcNode Node, tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeCall)
	node = SetLine(node, funcNode.Line)
	node = AddChild(node, funcNode) // Function being called

	// Skip '('
	pos = pos + 1

	// Arguments
	args := NewNode(NodeList)
	for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
		var arg Node

		// Check for **kwargs spread
		if peekTokenType(tokens, pos) == TokenDoubleStar {
			pos = pos + 1
			var expr Node
			expr, pos = parseExpression(tokens, pos)
			arg = NewNode(NodeKwArg)
			arg = AddChild(arg, expr)
		} else if peekTokenType(tokens, pos) == TokenStar {
			// Check for *args spread
			pos = pos + 1
			var expr Node
			expr, pos = parseExpression(tokens, pos)
			arg = NewNode(NodeStarArg)
			arg = AddChild(arg, expr)
		} else {
			arg, pos = parseExpression(tokens, pos)
		}

		args = AddChild(args, arg)

		if peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
		}
	}
	node = AddChild(node, args)

	// Skip ')'
	if peekTokenType(tokens, pos) == TokenRParen {
		pos = pos + 1
	}

	return node, pos
}

// parseSubscript parses a subscript expression
func parseSubscript(valueNode Node, tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeSubscript)
	node = SetLine(node, valueNode.Line)
	node = AddChild(node, valueNode)

	// Skip '['
	pos = pos + 1

	// Index
	var index Node
	index, pos = parseExpression(tokens, pos)
	node = AddChild(node, index)

	// Skip ']'
	if peekTokenType(tokens, pos) == TokenRBracket {
		pos = pos + 1
	}

	return node, pos
}

// parseListLiteral parses a list literal [...]
func parseListLiteral(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeList)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip '['
	pos = pos + 1

	for peekTokenType(tokens, pos) != TokenRBracket && peekTokenType(tokens, pos) != TokenEOF {
		var elem Node
		elem, pos = parseExpression(tokens, pos)
		node = AddChild(node, elem)

		if peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
		}
	}

	// Skip ']'
	if peekTokenType(tokens, pos) == TokenRBracket {
		pos = pos + 1
	}

	return node, pos
}

// parseDictLiteral parses a dict literal {...}
func parseDictLiteral(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeDict)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip '{'
	pos = pos + 1

	for peekTokenType(tokens, pos) != TokenRBrace && peekTokenType(tokens, pos) != TokenEOF {
		// Key
		var key Node
		key, pos = parseExpression(tokens, pos)

		// Skip ':'
		if peekTokenType(tokens, pos) == TokenColon {
			pos = pos + 1
		}

		// Value
		var value Node
		value, pos = parseExpression(tokens, pos)

		// Create a DictEntry node
		entry := NewNode(NodeDictEntry)
		entry = AddChild(entry, key)
		entry = AddChild(entry, value)
		node = AddChild(node, entry)

		if peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
		}
	}

	// Skip '}'
	if peekTokenType(tokens, pos) == TokenRBrace {
		pos = pos + 1
	}

	return node, pos
}

// parseImport parses an import statement
// import x, y, z
// import x as alias
func parseImport(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeImport)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'import'
	pos = pos + 1

	// Parse module names
	for {
		if peekTokenType(tokens, pos) == TokenIdentifier {
			name := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
			pos = pos + 1

			// Check for 'as' alias
			if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "as" {
				pos = pos + 1
				if peekTokenType(tokens, pos) == TokenIdentifier {
					name.Value = peekTokenValue(tokens, pos) // Store alias in Value
					pos = pos + 1
				}
			}

			node = AddChild(node, name)
		}

		if peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
		} else {
			break
		}
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	return node, pos
}

// parseImportFrom parses a from...import statement
// from module import x, y, z
// from module import x as alias
func parseImportFrom(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeImportFrom)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'from'
	pos = pos + 1

	// Module name
	if peekTokenType(tokens, pos) == TokenIdentifier {
		node.Name = peekTokenValue(tokens, pos)
		pos = pos + 1
	}

	// Skip 'import'
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "import" {
		pos = pos + 1
	}

	// Parse imported names
	for {
		if peekTokenType(tokens, pos) == TokenIdentifier {
			name := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
			pos = pos + 1

			// Check for 'as' alias
			if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "as" {
				pos = pos + 1
				if peekTokenType(tokens, pos) == TokenIdentifier {
					name.Value = peekTokenValue(tokens, pos) // Store alias in Value
					pos = pos + 1
				}
			}

			node = AddChild(node, name)
		}

		if peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
		} else {
			break
		}
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	return node, pos
}

// parseLambda parses a lambda expression
// lambda x, y: x + y
func parseLambda(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeLambda)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'lambda'
	pos = pos + 1

	// Parameters
	paramList := NewNode(NodeList)
	for peekTokenType(tokens, pos) != TokenColon && peekTokenType(tokens, pos) != TokenEOF {
		if peekTokenType(tokens, pos) == TokenIdentifier {
			param := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
			paramList = AddChild(paramList, param)
			pos = pos + 1
		}
		if peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
		}
	}
	node = AddChild(node, paramList)

	// Skip ':'
	if peekTokenType(tokens, pos) == TokenColon {
		pos = pos + 1
	}

	// Body expression
	var body Node
	body, pos = parseExpression(tokens, pos)
	node = AddChild(node, body)

	return node, pos
}

// parseDecorated parses a decorated function definition
// @decorator
// def func(): ...
func parseDecorated(tokens []Token, pos int) (Node, int) {
	// Collect decorators
	var decorators []Node
	for peekTokenType(tokens, pos) == TokenAt {
		pos = pos + 1 // Skip '@'

		decorator := NewNode(NodeDecorator)
		decorator = SetLine(decorator, peekToken(tokens, pos).Line)

		// Decorator name (could be a dotted name or call)
		var decoratorExpr Node
		decoratorExpr, pos = parsePrimary(tokens, pos)
		decorator = AddChild(decorator, decoratorExpr)

		decorators = append(decorators, decorator)

		// Skip newline after decorator
		if peekTokenType(tokens, pos) == TokenNewline {
			pos = pos + 1
		}
	}

	// Parse the function definition
	var funcDef Node
	funcDef, pos = parseFunctionDef(tokens, pos)

	// Add decorators as first children of the function
	// We'll store them by prepending to Children
	newChildren := make([]Node, 0)
	for i := 0; i < len(decorators); i++ {
		newChildren = append(newChildren, decorators[i])
	}
	for i := 0; i < len(funcDef.Children); i++ {
		newChildren = append(newChildren, funcDef.Children[i])
	}
	funcDef.Children = newChildren

	return funcDef, pos
}
