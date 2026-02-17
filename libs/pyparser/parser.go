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

	// class statement
	if tokenType == TokenKeyword && tokenValue == "class" {
		return parseClass(tokens, pos)
	}

	// async function or async with or async for
	if tokenType == TokenKeyword && tokenValue == "async" {
		return parseAsync(tokens, pos)
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

	// try statement
	if tokenType == TokenKeyword && tokenValue == "try" {
		return parseTry(tokens, pos)
	}

	// with statement
	if tokenType == TokenKeyword && tokenValue == "with" {
		return parseWith(tokens, pos)
	}

	// match statement (Python 3.10+)
	if tokenType == TokenKeyword && tokenValue == "match" {
		return parseMatch(tokens, pos)
	}

	// yield statement
	if tokenType == TokenKeyword && tokenValue == "yield" {
		return parseYield(tokens, pos)
	}

	// raise statement
	if tokenType == TokenKeyword && tokenValue == "raise" {
		return parseRaise(tokens, pos)
	}

	// assert statement
	if tokenType == TokenKeyword && tokenValue == "assert" {
		return parseAssert(tokens, pos)
	}

	// global statement
	if tokenType == TokenKeyword && tokenValue == "global" {
		return parseGlobal(tokens, pos)
	}

	// nonlocal statement
	if tokenType == TokenKeyword && tokenValue == "nonlocal" {
		return parseNonlocal(tokens, pos)
	}

	// del statement
	if tokenType == TokenKeyword && tokenValue == "del" {
		return parseDelete(tokens, pos)
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
	// Look ahead to see if it's an assignment or augmented assignment
	if tokenType == TokenIdentifier {
		nextType := peekTokenType(tokens, pos+1)
		// Check for augmented assignment
		if nextType == TokenPlusAssign || nextType == TokenMinusAssign ||
			nextType == TokenStarAssign || nextType == TokenSlashAssign ||
			nextType == TokenPercentAssign {
			return parseAugmentedAssignment(tokens, pos)
		}
		// Check for tuple unpacking assignment (a, b = ...)
		if nextType == TokenComma {
			return parseTupleUnpackingAssignment(tokens, pos)
		}
		// Check for annotated assignment (x: int = 1)
		if nextType == TokenColon {
			// Look further to see if there's an = after the type
			return parseAnnotatedAssignment(tokens, pos)
		}
		// Check if next token is =
		if nextType == TokenAssign {
			return parseAssignment(tokens, pos)
		}
	}

	// Check for starred unpacking (*a, b = ...)
	if tokenType == TokenStar {
		return parseTupleUnpackingAssignment(tokens, pos)
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
				// Check for *args or keyword-only separator
				pos = pos + 1
				if peekTokenType(tokens, pos) == TokenIdentifier {
					param := NewNodeWithName(NodeStarArg, peekTokenValue(tokens, pos))
					paramList = AddChild(paramList, param)
					pos = pos + 1
				} else {
					// Just * without name - keyword-only separator
					marker := NewNode(NodeKwOnlyMarker)
					paramList = AddChild(paramList, marker)
				}
			} else if peekTokenType(tokens, pos) == TokenIdentifier {
				paramName := peekTokenValue(tokens, pos)
				pos = pos + 1
				// Check for type annotation
				var paramAnnotation Node
				hasAnnotation := false
				if peekTokenType(tokens, pos) == TokenColon {
					pos = pos + 1
					paramAnnotation, pos = parseExpression(tokens, pos)
					hasAnnotation = true
				}
				// Check for default value
				if peekTokenType(tokens, pos) == TokenAssign {
					pos = pos + 1
					var defaultValue Node
					defaultValue, pos = parseExpression(tokens, pos)
					// Create a Default node with name and default value
					param := NewNodeWithName(NodeDefault, paramName)
					if hasAnnotation {
						param = AddChild(param, paramAnnotation)
					}
					param = AddChild(param, defaultValue)
					paramList = AddChild(paramList, param)
				} else {
					// No default value
					param := NewNodeWithName(NodeName, paramName)
					if hasAnnotation {
						param = AddChild(param, paramAnnotation)
					}
					paramList = AddChild(paramList, param)
				}
			} else if peekTokenType(tokens, pos) == TokenOperator && peekTokenValue(tokens, pos) == "/" {
				// Positional-only separator
				marker := NewNode(NodePosOnlyMarker)
				paramList = AddChild(paramList, marker)
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

	// Return type annotation (-> type)
	if peekTokenType(tokens, pos) == TokenArrow {
		pos = pos + 1
		var returnType Node
		returnType, pos = parseExpression(tokens, pos)
		// Store return type as a child with special marker
		returnTypeNode := NewNode(NodeTypeHint)
		returnTypeNode = AddChild(returnTypeNode, returnType)
		node = AddChild(node, returnTypeNode)
	}

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

// parseFor parses a for statement (including optional else clause)
func parseFor(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeFor)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'for'
	pos = pos + 1

	// Loop variable(s) - could be tuple unpacking: for a, b in items
	if peekTokenType(tokens, pos) == TokenIdentifier {
		firstName := peekTokenValue(tokens, pos)
		pos = pos + 1

		// Check for tuple unpacking
		if peekTokenType(tokens, pos) == TokenComma {
			tupleNode := NewNode(NodeTuple)
			tupleNode = AddChild(tupleNode, NewNodeWithName(NodeName, firstName))
			for peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1
				if peekTokenType(tokens, pos) == TokenIdentifier {
					tupleNode = AddChild(tupleNode, NewNodeWithName(NodeName, peekTokenValue(tokens, pos)))
					pos = pos + 1
				}
			}
			node = AddChild(node, tupleNode)
		} else {
			varNode := NewNodeWithName(NodeName, firstName)
			node = AddChild(node, varNode)
		}
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

	// Optional else clause
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "else" {
		var elseNode Node
		elseNode, pos = parseElse(tokens, pos)
		node = AddChild(node, elseNode)
	}

	return node, pos
}

// parseWhile parses a while statement (including optional else clause)
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

	// Optional else clause
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "else" {
		var elseNode Node
		elseNode, pos = parseElse(tokens, pos)
		node = AddChild(node, elseNode)
	}

	return node, pos
}

// parseAssignment parses an assignment statement
func parseAssignment(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeAssign)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Collect all targets (for chained assignments like a = b = c = 1)
	for {
		// Target
		target := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
		node = AddChild(node, target)
		pos = pos + 1

		// Skip '='
		if peekTokenType(tokens, pos) == TokenAssign {
			pos = pos + 1
		}

		// Check if there's another target (identifier followed by =)
		if peekTokenType(tokens, pos) == TokenIdentifier && peekTokenType(tokens, pos+1) == TokenAssign {
			// Continue collecting targets
			continue
		}
		break
	}

	// Value (last child)
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

// parseExpression parses an expression (handles 'or' and walrus operator)
func parseExpression(tokens []Token, pos int) (Node, int) {
	// Check for walrus operator (named expression)
	// identifier := expression
	if peekTokenType(tokens, pos) == TokenIdentifier && peekTokenType(tokens, pos+1) == TokenWalrus {
		name := peekTokenValue(tokens, pos)
		startLine := peekToken(tokens, pos).Line
		pos = pos + 2 // Skip identifier and :=

		var value Node
		value, pos = parseExpression(tokens, pos)

		node := NewNode(NodeNamedExpr)
		node = SetLine(node, startLine)
		node.Name = name
		node = AddChild(node, value)
		return node, pos
	}

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

	// Check for ternary expression: value if condition else alternative
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "if" {
		pos = pos + 1 // Skip 'if'
		var condition Node
		condition, pos = parseExpression(tokens, pos)
		if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "else" {
			pos = pos + 1 // Skip 'else'
			var alternative Node
			alternative, pos = parseExpression(tokens, pos)
			node := NewNode(NodeTernary)
			node = SetLine(node, left.Line)
			node = AddChild(node, left)       // true value
			node = AddChild(node, condition)  // condition
			node = AddChild(node, alternative) // false value
			return node, pos
		}
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

// parseComparison parses comparison expressions including chained comparisons
// e.g., 1 < x < 10 becomes Compare with multiple ops
func parseComparison(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseBitwiseOr(tokens, pos)

	// Check for comparison operators
	tokenType := peekTokenType(tokens, pos)
	tokenValue := peekTokenValue(tokens, pos)

	if tokenType == TokenOperator {
		if tokenValue == "==" || tokenValue == "!=" || tokenValue == "<" || tokenValue == ">" || tokenValue == "<=" || tokenValue == ">=" {
			// First comparison
			op := tokenValue
			pos = pos + 1
			var right Node
			right, pos = parseBitwiseOr(tokens, pos)
			node := NewNodeWithOp(NodeCompare, op)
			node = AddChild(node, left)
			node = AddChild(node, right)

			// Check for chained comparisons
			for {
				tokenType = peekTokenType(tokens, pos)
				tokenValue = peekTokenValue(tokens, pos)
				if tokenType == TokenOperator {
					if tokenValue == "==" || tokenValue == "!=" || tokenValue == "<" || tokenValue == ">" || tokenValue == "<=" || tokenValue == ">=" {
						// Store the operator in the node (append to Op)
						node.Op = node.Op + "," + tokenValue
						pos = pos + 1
						var nextRight Node
						nextRight, pos = parseBitwiseOr(tokens, pos)
						node = AddChild(node, nextRight)
						continue
					}
				}
				break
			}

			return node, pos
		}
	}

	return left, pos
}

// parseBitwiseOr parses bitwise OR expressions
func parseBitwiseOr(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseBitwiseXor(tokens, pos)

	for peekTokenType(tokens, pos) == TokenPipe {
		pos = pos + 1
		var right Node
		right, pos = parseBitwiseXor(tokens, pos)
		node := NewNodeWithOp(NodeBinOp, "|")
		node = AddChild(node, left)
		node = AddChild(node, right)
		left = node
	}

	return left, pos
}

// parseBitwiseXor parses bitwise XOR expressions
func parseBitwiseXor(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseBitwiseAnd(tokens, pos)

	for peekTokenType(tokens, pos) == TokenCaret {
		pos = pos + 1
		var right Node
		right, pos = parseBitwiseAnd(tokens, pos)
		node := NewNodeWithOp(NodeBinOp, "^")
		node = AddChild(node, left)
		node = AddChild(node, right)
		left = node
	}

	return left, pos
}

// parseBitwiseAnd parses bitwise AND expressions
func parseBitwiseAnd(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseShiftExpr(tokens, pos)

	for peekTokenType(tokens, pos) == TokenAmpersand {
		pos = pos + 1
		var right Node
		right, pos = parseShiftExpr(tokens, pos)
		node := NewNodeWithOp(NodeBinOp, "&")
		node = AddChild(node, left)
		node = AddChild(node, right)
		left = node
	}

	return left, pos
}

// parseShiftExpr parses shift expressions
func parseShiftExpr(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseArithExpr(tokens, pos)

	for {
		tokenType := peekTokenType(tokens, pos)
		if tokenType == TokenLeftShift {
			pos = pos + 1
			var right Node
			right, pos = parseArithExpr(tokens, pos)
			node := NewNodeWithOp(NodeBinOp, "<<")
			node = AddChild(node, left)
			node = AddChild(node, right)
			left = node
		} else if tokenType == TokenRightShift {
			pos = pos + 1
			var right Node
			right, pos = parseArithExpr(tokens, pos)
			node := NewNodeWithOp(NodeBinOp, ">>")
			node = AddChild(node, left)
			node = AddChild(node, right)
			left = node
		} else {
			break
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

// parseUnary parses unary operators (-, +, ~, await)
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
	// Bitwise NOT
	if peekTokenType(tokens, pos) == TokenTilde {
		pos = pos + 1
		var operand Node
		operand, pos = parseUnary(tokens, pos)
		node := NewNodeWithOp(NodeUnaryOp, "~")
		node = AddChild(node, operand)
		return node, pos
	}
	// Await expression
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "await" {
		pos = pos + 1
		var operand Node
		operand, pos = parseUnary(tokens, pos)
		node := NewNode(NodeAwait)
		node = AddChild(node, operand)
		return node, pos
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

	// String (including f-strings)
	if tokenType == TokenString {
		// Check if it's an f-string
		if len(tokenValue) > 2 && tokenValue[0] == 'f' && tokenValue[1] == ':' {
			fstrContent := extractAfterIndex(tokenValue, 2)
			return parseFString(fstrContent, peekToken(tokens, pos).Line, tokens, pos)
		}
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

	// Ellipsis (...)
	if tokenType == TokenEllipsis {
		node := NewNode(NodeEllipsis)
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

	// Parenthesized expression, tuple, or generator expression
	if tokenType == TokenLParen {
		startLine := peekToken(tokens, pos).Line
		pos = pos + 1

		// Empty tuple ()
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1
			tupleNode := NewNode(NodeTuple)
			tupleNode = SetLine(tupleNode, startLine)
			return tupleNode, pos
		}

		// Parse first expression
		var firstExpr Node
		firstExpr, pos = parseExpression(tokens, pos)

		// Check for generator expression
		if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "for" {
			return parseGeneratorExpression(firstExpr, tokens, pos, startLine)
		}

		// Check for tuple (comma after first expression)
		if peekTokenType(tokens, pos) == TokenComma {
			tupleNode := NewNode(NodeTuple)
			tupleNode = SetLine(tupleNode, startLine)
			tupleNode = AddChild(tupleNode, firstExpr)

			for peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1 // Skip ','
				if peekTokenType(tokens, pos) == TokenRParen {
					break // Trailing comma
				}
				var elem Node
				elem, pos = parseExpression(tokens, pos)
				tupleNode = AddChild(tupleNode, elem)
			}

			if peekTokenType(tokens, pos) == TokenRParen {
				pos = pos + 1
			}
			return tupleNode, pos
		}

		// Simple parenthesized expression
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1
		}
		return firstExpr, pos
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
		} else if peekTokenType(tokens, pos) == TokenIdentifier && peekTokenType(tokens, pos+1) == TokenAssign {
			// Keyword argument: name=value
			kwName := peekTokenValue(tokens, pos)
			pos = pos + 2 // Skip name and =
			var kwValue Node
			kwValue, pos = parseExpression(tokens, pos)
			arg = NewNodeWithName(NodeKeywordArg, kwName)
			arg = AddChild(arg, kwValue)
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

// parseSubscript parses a subscript expression or slice
// x[0], x[1:3], x[::2], x[1:], x[:3], x[1:3:2]
func parseSubscript(valueNode Node, tokens []Token, pos int) (Node, int) {
	startLine := valueNode.Line

	// Skip '['
	pos = pos + 1

	// Check if this is a slice by looking for a colon
	// We need to determine if it's a simple index or a slice
	isSlice := false

	// Parse the first part (could be start of slice or simple index)
	var firstPart Node
	if peekTokenType(tokens, pos) == TokenColon {
		// Slice with no start: [:end] or [:end:step] or [:]
		isSlice = true
		firstPart = NewNode(NodeNone) // No start
	} else if peekTokenType(tokens, pos) != TokenRBracket {
		firstPart, pos = parseExpression(tokens, pos)
	}

	// Check for colon (indicates slice)
	if peekTokenType(tokens, pos) == TokenColon {
		isSlice = true
		pos = pos + 1 // Skip ':'

		// Create slice node
		node := NewNode(NodeSlice)
		node = SetLine(node, startLine)
		node = AddChild(node, valueNode)
		node = AddChild(node, firstPart) // Start

		// Parse stop
		var stopPart Node
		if peekTokenType(tokens, pos) == TokenColon || peekTokenType(tokens, pos) == TokenRBracket {
			stopPart = NewNode(NodeNone) // No stop
		} else {
			stopPart, pos = parseExpression(tokens, pos)
		}
		node = AddChild(node, stopPart)

		// Check for step
		if peekTokenType(tokens, pos) == TokenColon {
			pos = pos + 1 // Skip ':'
			var stepPart Node
			if peekTokenType(tokens, pos) == TokenRBracket {
				stepPart = NewNode(NodeNone) // No step
			} else {
				stepPart, pos = parseExpression(tokens, pos)
			}
			node = AddChild(node, stepPart)
		}

		// Skip ']'
		if peekTokenType(tokens, pos) == TokenRBracket {
			pos = pos + 1
		}

		return node, pos
	}

	// Simple subscript (not a slice)
	if !isSlice {
		node := NewNode(NodeSubscript)
		node = SetLine(node, startLine)
		node = AddChild(node, valueNode)
		node = AddChild(node, firstPart)

		// Skip ']'
		if peekTokenType(tokens, pos) == TokenRBracket {
			pos = pos + 1
		}

		return node, pos
	}

	// Fallback (shouldn't reach here)
	return valueNode, pos
}

// parseListLiteral parses a list literal [...] or list comprehension [expr for x in iter]
func parseListLiteral(tokens []Token, pos int) (Node, int) {
	startLine := peekToken(tokens, pos).Line

	// Skip '['
	pos = pos + 1

	// Check for empty list
	if peekTokenType(tokens, pos) == TokenRBracket {
		pos = pos + 1
		emptyList := NewNode(NodeList)
		emptyList = SetLine(emptyList, startLine)
		return emptyList, pos
	}

	// Parse first expression
	var firstExpr Node
	firstExpr, pos = parseExpression(tokens, pos)

	// Check if this is a list comprehension
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "for" {
		return parseListComprehension(firstExpr, tokens, pos, startLine)
	}

	// Regular list
	node := NewNode(NodeList)
	node = SetLine(node, startLine)
	node = AddChild(node, firstExpr)

	// Parse remaining elements
	for peekTokenType(tokens, pos) == TokenComma {
		pos = pos + 1 // Skip comma
		if peekTokenType(tokens, pos) == TokenRBracket {
			break
		}
		var elem Node
		elem, pos = parseExpression(tokens, pos)
		node = AddChild(node, elem)
	}

	// Skip ']'
	if peekTokenType(tokens, pos) == TokenRBracket {
		pos = pos + 1
	}

	return node, pos
}

// parseListComprehension parses a list comprehension [expr for x in iter if cond]
func parseListComprehension(expr Node, tokens []Token, pos int, startLine int) (Node, int) {
	node := NewNode(NodeListComp)
	node = SetLine(node, startLine)

	// Add the expression
	node = AddChild(node, expr)

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

	// Optional 'if' condition
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "if" {
		pos = pos + 1
		var condition Node
		condition, pos = parseExpression(tokens, pos)
		node = AddChild(node, condition)
	}

	// Skip ']'
	if peekTokenType(tokens, pos) == TokenRBracket {
		pos = pos + 1
	}

	return node, pos
}

// parseDictOrSetLiteral parses a dict or set literal {...}
// {} - empty dict
// {1, 2, 3} - set
// {k: v} - dict
// {x for x in iter} - set comprehension
// {k: v for k, v in iter} - dict comprehension
func parseDictLiteral(tokens []Token, pos int) (Node, int) {
	startLine := peekToken(tokens, pos).Line

	// Skip '{'
	pos = pos + 1

	// Empty dict
	if peekTokenType(tokens, pos) == TokenRBrace {
		pos = pos + 1
		emptyDict := NewNode(NodeDict)
		emptyDict = SetLine(emptyDict, startLine)
		return emptyDict, pos
	}

	// Parse first expression
	var firstExpr Node
	firstExpr, pos = parseExpression(tokens, pos)

	// Check what comes next to determine dict vs set vs comprehension
	if peekTokenType(tokens, pos) == TokenColon {
		// Dict or dict comprehension
		pos = pos + 1 // Skip ':'
		var valueExpr Node
		valueExpr, pos = parseExpression(tokens, pos)

		// Check for dict comprehension
		if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "for" {
			return parseDictComprehension(firstExpr, valueExpr, tokens, pos, startLine)
		}

		// Regular dict
		dictNode := NewNode(NodeDict)
		dictNode = SetLine(dictNode, startLine)

		entry := NewNode(NodeDictEntry)
		entry = AddChild(entry, firstExpr)
		entry = AddChild(entry, valueExpr)
		dictNode = AddChild(dictNode, entry)

		// Parse remaining entries
		for peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1 // Skip ','
			if peekTokenType(tokens, pos) == TokenRBrace {
				break
			}

			var key Node
			key, pos = parseExpression(tokens, pos)

			if peekTokenType(tokens, pos) == TokenColon {
				pos = pos + 1
			}

			var value Node
			value, pos = parseExpression(tokens, pos)

			entry = NewNode(NodeDictEntry)
			entry = AddChild(entry, key)
			entry = AddChild(entry, value)
			dictNode = AddChild(dictNode, entry)
		}

		// Skip '}'
		if peekTokenType(tokens, pos) == TokenRBrace {
			pos = pos + 1
		}

		return dictNode, pos
	}

	// Check for set comprehension
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "for" {
		return parseSetComprehension(firstExpr, tokens, pos, startLine)
	}

	// Set literal
	setNode := NewNode(NodeSet)
	setNode = SetLine(setNode, startLine)
	setNode = AddChild(setNode, firstExpr)

	// Parse remaining elements
	for peekTokenType(tokens, pos) == TokenComma {
		pos = pos + 1 // Skip ','
		if peekTokenType(tokens, pos) == TokenRBrace {
			break
		}
		var elem Node
		elem, pos = parseExpression(tokens, pos)
		setNode = AddChild(setNode, elem)
	}

	// Skip '}'
	if peekTokenType(tokens, pos) == TokenRBrace {
		pos = pos + 1
	}

	return setNode, pos
}

// parseGeneratorExpression parses a generator expression (x for x in iter)
func parseGeneratorExpression(expr Node, tokens []Token, pos int, startLine int) (Node, int) {
	node := NewNode(NodeGeneratorExp)
	node = SetLine(node, startLine)

	// Add the expression
	node = AddChild(node, expr)

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

	// Optional 'if' condition
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "if" {
		pos = pos + 1
		var condition Node
		condition, pos = parseExpression(tokens, pos)
		node = AddChild(node, condition)
	}

	// Skip ')'
	if peekTokenType(tokens, pos) == TokenRParen {
		pos = pos + 1
	}

	return node, pos
}

// parseSetComprehension parses a set comprehension {x for x in iter}
func parseSetComprehension(expr Node, tokens []Token, pos int, startLine int) (Node, int) {
	node := NewNode(NodeSetComp)
	node = SetLine(node, startLine)

	// Add the expression
	node = AddChild(node, expr)

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

	// Optional 'if' condition
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "if" {
		pos = pos + 1
		var condition Node
		condition, pos = parseExpression(tokens, pos)
		node = AddChild(node, condition)
	}

	// Skip '}'
	if peekTokenType(tokens, pos) == TokenRBrace {
		pos = pos + 1
	}

	return node, pos
}

// parseDictComprehension parses a dict comprehension {k: v for k, v in iter}
func parseDictComprehension(keyExpr Node, valueExpr Node, tokens []Token, pos int, startLine int) (Node, int) {
	node := NewNode(NodeDictComp)
	node = SetLine(node, startLine)

	// Add key and value expressions
	node = AddChild(node, keyExpr)
	node = AddChild(node, valueExpr)

	// Skip 'for'
	pos = pos + 1

	// Loop variable(s)
	if peekTokenType(tokens, pos) == TokenIdentifier {
		varNode := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
		node = AddChild(node, varNode)
		pos = pos + 1

		// Check for second variable (k, v)
		if peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
			if peekTokenType(tokens, pos) == TokenIdentifier {
				varNode2 := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
				node = AddChild(node, varNode2)
				pos = pos + 1
			}
		}
	}

	// Skip 'in'
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "in" {
		pos = pos + 1
	}

	// Iterable
	var iterable Node
	iterable, pos = parseExpression(tokens, pos)
	node = AddChild(node, iterable)

	// Optional 'if' condition
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "if" {
		pos = pos + 1
		var condition Node
		condition, pos = parseExpression(tokens, pos)
		node = AddChild(node, condition)
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

// parseClass parses a class definition
// class ClassName:
// class ClassName(BaseClass):
func parseClass(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeClass)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'class'
	pos = pos + 1

	// Class name
	if peekTokenType(tokens, pos) == TokenIdentifier {
		node.Name = peekTokenValue(tokens, pos)
		pos = pos + 1
	}

	// Base classes (optional)
	baseList := NewNode(NodeList)
	if peekTokenType(tokens, pos) == TokenLParen {
		pos = pos + 1
		for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
			if peekTokenType(tokens, pos) == TokenIdentifier {
				baseNode := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
				baseList = AddChild(baseList, baseNode)
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
	node = AddChild(node, baseList)

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

// parseTry parses a try/except/else/finally statement
// try:
//     ...
// except Exception:
//     ...
// else:
//     ...
// finally:
//     ...
func parseTry(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeTry)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'try'
	pos = pos + 1

	// Colon
	if peekTokenType(tokens, pos) == TokenColon {
		pos = pos + 1
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	// Try body
	var tryBody Node
	tryBody, pos = parseBlock(tokens, pos)
	node = AddChild(node, tryBody)

	// Handle except clauses
	for peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "except" {
		var exceptNode Node
		exceptNode, pos = parseExcept(tokens, pos)
		node = AddChild(node, exceptNode)
	}

	// Handle else clause (runs if no exception occurred)
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "else" {
		var elseNode Node
		elseNode, pos = parseElse(tokens, pos)
		node = AddChild(node, elseNode)
	}

	// Handle finally clause
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "finally" {
		var finallyNode Node
		finallyNode, pos = parseFinally(tokens, pos)
		node = AddChild(node, finallyNode)
	}

	return node, pos
}

// parseExcept parses an except clause
// except Exception as e:
// except (TypeError, ValueError) as e:
// except *ExceptionGroup as eg:  (Python 3.11+)
func parseExcept(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeExcept)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'except'
	pos = pos + 1

	// Check for except* (exception group handling)
	if peekTokenType(tokens, pos) == TokenStar {
		node.Op = "*"
		pos = pos + 1
	}

	// Exception type (optional) - could be single or tuple of types
	if peekTokenType(tokens, pos) == TokenLParen {
		// Multiple exception types: except (TypeError, ValueError)
		pos = pos + 1
		exceptTypes := NewNode(NodeTuple)
		for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
			if peekTokenType(tokens, pos) == TokenIdentifier {
				exceptTypes = AddChild(exceptTypes, NewNodeWithName(NodeName, peekTokenValue(tokens, pos)))
				pos = pos + 1
			}
			if peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1
			}
		}
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1
		}
		node = AddChild(node, exceptTypes)
	} else if peekTokenType(tokens, pos) == TokenIdentifier {
		// Single exception type
		node.Name = peekTokenValue(tokens, pos)
		pos = pos + 1
	}

	// Check for 'as' alias
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "as" {
		pos = pos + 1
		if peekTokenType(tokens, pos) == TokenIdentifier {
			node.Value = peekTokenValue(tokens, pos) // Store alias in Value
			pos = pos + 1
		}
	}

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

// parseFinally parses a finally clause
func parseFinally(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeFinally)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'finally'
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

// parseWith parses a with statement (supports multiple context managers)
// with open("file") as f:
// with open("a") as a, open("b") as b:
func parseWith(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeWith)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'with'
	pos = pos + 1

	// Parse context managers (can be multiple, separated by commas)
	for {
		// Context expression
		var contextExpr Node
		contextExpr, pos = parseExpression(tokens, pos)
		node = AddChild(node, contextExpr)

		// Check for 'as' alias
		if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "as" {
			pos = pos + 1
			if peekTokenType(tokens, pos) == TokenIdentifier {
				alias := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
				node = AddChild(node, alias)
				pos = pos + 1
			}
		}

		// Check for more context managers
		if peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
			continue
		}
		break
	}

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

// parseYield parses a yield statement/expression
// yield value
// yield from iterable
func parseYield(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeYield)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'yield'
	pos = pos + 1

	// Check for 'from' (yield from)
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "from" {
		node.Op = "from"
		pos = pos + 1
	}

	// Yield value (optional)
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

// parseRaise parses a raise statement
// raise Exception
// raise Exception("message")
// raise Exception from cause
// raise
func parseRaise(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeRaise)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'raise'
	pos = pos + 1

	// Exception (optional - bare raise re-raises current exception)
	if peekTokenType(tokens, pos) != TokenNewline && peekTokenType(tokens, pos) != TokenEOF &&
		!(peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "from") {
		var expr Node
		expr, pos = parseExpression(tokens, pos)
		node = AddChild(node, expr)
	}

	// Check for 'from' (exception chaining)
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "from" {
		pos = pos + 1
		node.Op = "from"
		var cause Node
		cause, pos = parseExpression(tokens, pos)
		node = AddChild(node, cause)
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	return node, pos
}

// parseAugmentedAssignment parses augmented assignment (+=, -=, etc.)
// x += 1
func parseAugmentedAssignment(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeAugAssign)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Target
	target := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
	node = AddChild(node, target)
	pos = pos + 1

	// Get the operator
	opType := peekTokenType(tokens, pos)
	if opType == TokenPlusAssign {
		node.Op = "+="
	} else if opType == TokenMinusAssign {
		node.Op = "-="
	} else if opType == TokenStarAssign {
		node.Op = "*="
	} else if opType == TokenSlashAssign {
		node.Op = "/="
	} else if opType == TokenPercentAssign {
		node.Op = "%="
	}
	pos = pos + 1

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

// parseAssert parses an assert statement
// assert condition
// assert condition, message
func parseAssert(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeAssert)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'assert'
	pos = pos + 1

	// Condition
	var condition Node
	condition, pos = parseExpression(tokens, pos)
	node = AddChild(node, condition)

	// Optional message
	if peekTokenType(tokens, pos) == TokenComma {
		pos = pos + 1
		var message Node
		message, pos = parseExpression(tokens, pos)
		node = AddChild(node, message)
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	return node, pos
}

// parseGlobal parses a global statement
// global x, y, z
func parseGlobal(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeGlobal)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'global'
	pos = pos + 1

	// Parse variable names
	for {
		if peekTokenType(tokens, pos) == TokenIdentifier {
			name := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
			node = AddChild(node, name)
			pos = pos + 1
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

// parseNonlocal parses a nonlocal statement
// nonlocal x, y, z
func parseNonlocal(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeNonlocal)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'nonlocal'
	pos = pos + 1

	// Parse variable names
	for {
		if peekTokenType(tokens, pos) == TokenIdentifier {
			name := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
			node = AddChild(node, name)
			pos = pos + 1
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

// parseDelete parses a del statement
// del x
// del x, y
func parseDelete(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeDelete)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'del'
	pos = pos + 1

	// Parse targets
	for {
		var target Node
		target, pos = parsePrimary(tokens, pos)
		node = AddChild(node, target)

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

// parseAsync parses async def, async with, async for
func parseAsync(tokens []Token, pos int) (Node, int) {
	startLine := peekToken(tokens, pos).Line

	// Skip 'async'
	pos = pos + 1

	// Check what follows
	if peekTokenType(tokens, pos) == TokenKeyword {
		keyword := peekTokenValue(tokens, pos)
		if keyword == "def" {
			// Async function definition
			var funcDef Node
			funcDef, pos = parseFunctionDef(tokens, pos)
			// Change the node type to AsyncFunctionDef
			funcDef.Type = NodeAsyncFunctionDef
			return funcDef, pos
		}
		if keyword == "with" {
			// Async with - parse as regular with and mark it
			var withNode Node
			withNode, pos = parseWith(tokens, pos)
			withNode.Op = "async"
			return withNode, pos
		}
		if keyword == "for" {
			// Async for - parse as regular for and mark it
			var forNode Node
			forNode, pos = parseFor(tokens, pos)
			forNode.Op = "async"
			return forNode, pos
		}
	}

	// Fallback - shouldn't happen
	node := NewNode(NodeExpr)
	node = SetLine(node, startLine)
	return node, pos
}

// parseTupleUnpackingAssignment parses tuple unpacking assignment
// a, b = 1, 2
// a, *rest = [1, 2, 3, 4]
func parseTupleUnpackingAssignment(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeAssign)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Parse left-hand side targets
	targets := NewNode(NodeTuple)
	for {
		// Check for starred target
		if peekTokenType(tokens, pos) == TokenStar {
			pos = pos + 1
			if peekTokenType(tokens, pos) == TokenIdentifier {
				starTarget := NewNodeWithName(NodeStarArg, peekTokenValue(tokens, pos))
				targets = AddChild(targets, starTarget)
				pos = pos + 1
			}
		} else if peekTokenType(tokens, pos) == TokenIdentifier {
			target := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
			targets = AddChild(targets, target)
			pos = pos + 1
		} else {
			break
		}

		if peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
		} else {
			break
		}
	}
	node = AddChild(node, targets)

	// Skip '='
	if peekTokenType(tokens, pos) == TokenAssign {
		pos = pos + 1
	}

	// Parse right-hand side value(s)
	// Could be multiple values like: a, b = 1, 2
	var firstValue Node
	firstValue, pos = parseExpression(tokens, pos)

	// Check if there are more values (comma-separated)
	if peekTokenType(tokens, pos) == TokenComma {
		// Multiple values - create a tuple
		values := NewNode(NodeTuple)
		values = AddChild(values, firstValue)
		for peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1 // Skip comma
			var nextValue Node
			nextValue, pos = parseExpression(tokens, pos)
			values = AddChild(values, nextValue)
		}
		node = AddChild(node, values)
	} else {
		// Single value
		node = AddChild(node, firstValue)
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	return node, pos
}

// parseFString parses an f-string value into an FString node
// Simplified version that stores f-string parts without recursively parsing expressions
// The fstrValue format is: STR:text|EXPR:code|STR:more text|...
func parseFString(fstrValue string, startLine int, tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeFString)
	node = SetLine(node, startLine)

	// Skip the f-string token
	pos = pos + 1

	// Parse the parts - simplified without recursive parsing
	currentPart := ""
	inExpr := false

	i := 0
	for i < len(fstrValue) {
		ch := fstrValue[i]
		if ch == '|' {
			// Process accumulated part
			if currentPart != "" {
				if !inExpr {
					// It's a STR: part
					if len(currentPart) > 4 {
						content := extractAfterIndex(currentPart, 4)
						strNode := NewNodeWithValue(NodeStr, content)
						node = AddChild(node, strNode)
					}
				} else {
					// It's an EXPR: part - store expression as string for now
					if len(currentPart) > 5 {
						exprStr := extractAfterIndex(currentPart, 5)
						formattedNode := NewNode(NodeFormattedValue)
						formattedNode.Value = exprStr
						node = AddChild(node, formattedNode)
					}
				}
				currentPart = ""
			}
			i = i + 1
			continue
		}

		// Check if this starts a new part type
		if currentPart == "" {
			if ch == 'S' {
				inExpr = false
			} else if ch == 'E' {
				inExpr = true
			}
		}

		currentPart = currentPart + charToString(int(ch))
		i = i + 1
	}

	// Process final part
	if currentPart != "" {
		if !inExpr {
			if len(currentPart) > 4 {
				content := extractAfterIndex(currentPart, 4)
				strNode := NewNodeWithValue(NodeStr, content)
				node = AddChild(node, strNode)
			}
		} else {
			if len(currentPart) > 5 {
				exprStr := extractAfterIndex(currentPart, 5)
				formattedNode := NewNode(NodeFormattedValue)
				formattedNode.Value = exprStr
				node = AddChild(node, formattedNode)
			}
		}
	}

	return node, pos
}

// extractAfterIndex extracts substring starting from given index
func extractAfterIndex(s string, startIndex int) string {
	result := ""
	for i := startIndex; i < len(s); i++ {
		result = result + charToString(int(s[i]))
	}
	return result
}

// parseAnnotatedAssignment parses annotated assignment
// x: int = 1
// x: int
func parseAnnotatedAssignment(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeAnnotatedAssign)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Target name
	target := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
	node = AddChild(node, target)
	pos = pos + 1

	// Skip ':'
	if peekTokenType(tokens, pos) == TokenColon {
		pos = pos + 1
	}

	// Type annotation
	var annotation Node
	annotation, pos = parseExpression(tokens, pos)
	node = AddChild(node, annotation)

	// Optional value
	if peekTokenType(tokens, pos) == TokenAssign {
		pos = pos + 1
		var value Node
		value, pos = parseExpression(tokens, pos)
		node = AddChild(node, value)
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	return node, pos
}

// parseMatch parses a match statement (Python 3.10+)
// match subject:
//
//	case pattern1:
//	    body1
//	case pattern2 if guard:
//	    body2
func parseMatch(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeMatch)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'match'
	pos = pos + 1

	// Parse subject expression
	var subject Node
	subject, pos = parseExpression(tokens, pos)
	node = AddChild(node, subject)

	// Colon
	if peekTokenType(tokens, pos) == TokenColon {
		pos = pos + 1
	}

	// Skip newline
	if peekTokenType(tokens, pos) == TokenNewline {
		pos = pos + 1
	}

	// Skip INDENT
	if peekTokenType(tokens, pos) == TokenIndent {
		pos = pos + 1
	}

	// Parse case clauses
	for peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "case" {
		var caseNode Node
		caseNode, pos = parseMatchCase(tokens, pos)
		node = AddChild(node, caseNode)
	}

	// Skip DEDENT
	if peekTokenType(tokens, pos) == TokenDedent {
		pos = pos + 1
	}

	return node, pos
}

// parseMatchCase parses a single case clause (with OR pattern support)
// case 1 | 2 | 3:
// case pattern if guard:
func parseMatchCase(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeMatchCase)
	node = SetLine(node, peekToken(tokens, pos).Line)

	// Skip 'case'
	pos = pos + 1

	// Parse pattern (with OR patterns)
	var pattern Node
	pattern, pos = parseMatchOrPattern(tokens, pos)
	node = AddChild(node, pattern)

	// Check for guard (if condition)
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "if" {
		pos = pos + 1
		var guard Node
		guard, pos = parseExpression(tokens, pos)
		node = AddChild(node, guard)
	}

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

// parseMatchOrPattern parses patterns with OR: pattern1 | pattern2 | pattern3
func parseMatchOrPattern(tokens []Token, pos int) (Node, int) {
	var firstPattern Node
	firstPattern, pos = parseMatchPattern(tokens, pos)

	// Check for OR patterns
	if peekTokenType(tokens, pos) == TokenPipe {
		orNode := NewNode(NodeMatchPattern)
		orNode.Op = "or"
		orNode = SetLine(orNode, firstPattern.Line)
		orNode = AddChild(orNode, firstPattern)

		for peekTokenType(tokens, pos) == TokenPipe {
			pos = pos + 1 // Skip |
			var nextPattern Node
			nextPattern, pos = parseMatchPattern(tokens, pos)
			orNode = AddChild(orNode, nextPattern)
		}
		return orNode, pos
	}

	return firstPattern, pos
}

// parseMatchPattern parses a pattern in a case clause
// Supports: literals, identifiers (capture), wildcards (_), sequences, mappings, class patterns
func parseMatchPattern(tokens []Token, pos int) (Node, int) {
	tokenType := peekTokenType(tokens, pos)
	tokenValue := peekTokenValue(tokens, pos)

	// Wildcard pattern (_)
	if tokenType == TokenIdentifier && tokenValue == "_" {
		wildcardNode := NewNode(NodeMatchPattern)
		wildcardNode.Op = "wildcard"
		wildcardNode = SetLine(wildcardNode, peekToken(tokens, pos).Line)
		pos = pos + 1
		return wildcardNode, pos
	}

	// Literal patterns: numbers, strings, True, False, None
	if tokenType == TokenNumber {
		numPatternNode := NewNode(NodeMatchPattern)
		numPatternNode.Op = "literal"
		numPatternNode = SetLine(numPatternNode, peekToken(tokens, pos).Line)
		numValue := NewNodeWithValue(NodeNum, tokenValue)
		numPatternNode = AddChild(numPatternNode, numValue)
		pos = pos + 1
		return numPatternNode, pos
	}

	if tokenType == TokenString {
		strPatternNode := NewNode(NodeMatchPattern)
		strPatternNode.Op = "literal"
		strPatternNode = SetLine(strPatternNode, peekToken(tokens, pos).Line)
		strValue := NewNodeWithValue(NodeStr, tokenValue)
		strPatternNode = AddChild(strPatternNode, strValue)
		pos = pos + 1
		return strPatternNode, pos
	}

	if tokenType == TokenKeyword && (tokenValue == "True" || tokenValue == "False") {
		boolPatternNode := NewNode(NodeMatchPattern)
		boolPatternNode.Op = "literal"
		boolPatternNode = SetLine(boolPatternNode, peekToken(tokens, pos).Line)
		boolValue := NewNodeWithValue(NodeBool, tokenValue)
		boolPatternNode = AddChild(boolPatternNode, boolValue)
		pos = pos + 1
		return boolPatternNode, pos
	}

	if tokenType == TokenKeyword && tokenValue == "None" {
		nonePatternNode := NewNode(NodeMatchPattern)
		nonePatternNode.Op = "literal"
		nonePatternNode = SetLine(nonePatternNode, peekToken(tokens, pos).Line)
		noneValue := NewNode(NodeNone)
		nonePatternNode = AddChild(nonePatternNode, noneValue)
		pos = pos + 1
		return nonePatternNode, pos
	}

	// Sequence pattern [a, b, c] or (a, b, c)
	if tokenType == TokenLBracket || tokenType == TokenLParen {
		seqPatternNode := NewNode(NodeMatchPattern)
		seqPatternNode.Op = "sequence"
		seqPatternNode = SetLine(seqPatternNode, peekToken(tokens, pos).Line)
		closingToken := TokenRBracket
		if tokenType == TokenLParen {
			closingToken = TokenRParen
		}
		pos = pos + 1
		for peekTokenType(tokens, pos) != closingToken && peekTokenType(tokens, pos) != TokenEOF {
			var elemPattern Node
			elemPattern, pos = parseMatchPattern(tokens, pos)
			seqPatternNode = AddChild(seqPatternNode, elemPattern)
			if peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1
			}
		}
		if peekTokenType(tokens, pos) == closingToken {
			pos = pos + 1
		}
		return seqPatternNode, pos
	}

	// Mapping pattern {key: pattern, ...}
	if tokenType == TokenLBrace {
		mapPatternNode := NewNode(NodeMatchPattern)
		mapPatternNode.Op = "mapping"
		mapPatternNode = SetLine(mapPatternNode, peekToken(tokens, pos).Line)
		pos = pos + 1
		for peekTokenType(tokens, pos) != TokenRBrace && peekTokenType(tokens, pos) != TokenEOF {
			// Key
			var keyPattern Node
			keyPattern, pos = parseMatchPattern(tokens, pos)
			// Colon
			if peekTokenType(tokens, pos) == TokenColon {
				pos = pos + 1
			}
			// Value pattern
			var valuePattern Node
			valuePattern, pos = parseMatchPattern(tokens, pos)
			// Create entry node
			mapEntryNode := NewNode(NodeDictEntry)
			mapEntryNode = AddChild(mapEntryNode, keyPattern)
			mapEntryNode = AddChild(mapEntryNode, valuePattern)
			mapPatternNode = AddChild(mapPatternNode, mapEntryNode)
			if peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1
			}
		}
		if peekTokenType(tokens, pos) == TokenRBrace {
			pos = pos + 1
		}
		return mapPatternNode, pos
	}

	// Identifier (capture pattern) or class pattern
	if tokenType == TokenIdentifier {
		patternName := tokenValue
		pos = pos + 1
		// Check for class pattern: ClassName(...)
		if peekTokenType(tokens, pos) == TokenLParen {
			classPatternNode := NewNode(NodeMatchPattern)
			classPatternNode.Op = "class"
			classPatternNode.Name = patternName
			classPatternNode = SetLine(classPatternNode, peekToken(tokens, pos-1).Line)
			pos = pos + 1 // Skip (
			for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
				var argPattern Node
				argPattern, pos = parseMatchPattern(tokens, pos)
				classPatternNode = AddChild(classPatternNode, argPattern)
				if peekTokenType(tokens, pos) == TokenComma {
					pos = pos + 1
				}
			}
			if peekTokenType(tokens, pos) == TokenRParen {
				pos = pos + 1
			}
			return classPatternNode, pos
		}
		// Simple capture pattern
		capturePatternNode := NewNode(NodeMatchPattern)
		capturePatternNode.Op = "capture"
		capturePatternNode.Name = patternName
		capturePatternNode = SetLine(capturePatternNode, peekToken(tokens, pos-1).Line)
		return capturePatternNode, pos
	}

	// Default: create empty pattern
	defaultPatternNode := NewNode(NodeMatchPattern)
	defaultPatternNode = SetLine(defaultPatternNode, peekToken(tokens, pos).Line)
	return defaultPatternNode, pos
}
