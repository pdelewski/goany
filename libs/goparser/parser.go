package goparser

// Parse parses Go source code and returns the AST
func Parse(input string) Node {
	tokens := Tokenize(input)
	file, finalPos := parseFile(tokens, 0)
	if finalPos > 0 {
		// suppress unused warning
	}
	return file
}

// parseFile parses a Go source file
func parseFile(tokens []Token, pos int) (Node, int) {
	file := NewNode(NodeFile)

	// Skip leading semicolons
	for pos < len(tokens) && peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	// Parse package clause
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "package" {
		pos = pos + 1
		pkgNode := NewNode(NodePackage)
		if peekTokenType(tokens, pos) == TokenIdentifier {
			pkgNode.Name = peekTokenValue(tokens, pos)
			pkgNode = SetLine(pkgNode, peekToken(tokens, pos).Line)
			pos = pos + 1
		}
		file = AddChild(file, pkgNode)
		// Skip semicolon
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
	}

	// Parse import declarations
	for peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "import" {
		var importNode Node
		importNode, pos = parseImport(tokens, pos)
		file = AddChild(file, importNode)
		// Skip semicolons
		for peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
	}

	// Parse top-level declarations
	for pos < len(tokens) && peekTokenType(tokens, pos) != TokenEOF {
		// Skip semicolons
		for peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		if peekTokenType(tokens, pos) == TokenEOF {
			break
		}

		var decl Node
		decl, pos = parseTopLevelDecl(tokens, pos)
		file = AddChild(file, decl)
	}

	return file, pos
}

// parseImport parses an import declaration
func parseImport(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeImport)
	node = SetLine(node, peekToken(tokens, pos).Line)
	pos = pos + 1 // skip 'import'

	var spec Node
	if peekTokenType(tokens, pos) == TokenLParen {
		// Grouped import
		pos = pos + 1 // skip '('
		for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
			// Skip semicolons
			for peekTokenType(tokens, pos) == TokenSemicolon {
				pos = pos + 1
			}
			if peekTokenType(tokens, pos) == TokenRParen {
				break
			}
			spec, pos = parseImportSpec(tokens, pos)
			node = AddChild(node, spec)
		}
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1
		}
	} else {
		// Single import
		spec, pos = parseImportSpec(tokens, pos)
		node = AddChild(node, spec)
	}

	// Skip semicolon
	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return node, pos
}

// parseImportSpec parses a single import spec
func parseImportSpec(tokens []Token, pos int) (Node, int) {
	spec := NewNode(NodeImportSpec)
	spec = SetLine(spec, peekToken(tokens, pos).Line)

	// Check for alias (. or name before string)
	if peekTokenType(tokens, pos) == TokenDot {
		spec.Name = "."
		pos = pos + 1
	} else if peekTokenType(tokens, pos) == TokenIdentifier && peekTokenType(tokens, pos+1) == TokenString {
		spec.Name = peekTokenValue(tokens, pos)
		pos = pos + 1
	}

	// Import path (string)
	if peekTokenType(tokens, pos) == TokenString {
		spec.Value = peekTokenValue(tokens, pos)
		pos = pos + 1
	}

	// Skip semicolon
	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return spec, pos
}

// parseTopLevelDecl parses a top-level declaration
func parseTopLevelDecl(tokens []Token, pos int) (Node, int) {
	tokenType := peekTokenType(tokens, pos)
	tokenValue := peekTokenValue(tokens, pos)

	if tokenType == TokenKeyword && tokenValue == "func" {
		return parseFuncDecl(tokens, pos)
	}
	if tokenType == TokenKeyword && tokenValue == "var" {
		return parseVarDecl(tokens, pos)
	}
	if tokenType == TokenKeyword && tokenValue == "const" {
		return parseConstDecl(tokens, pos)
	}
	if tokenType == TokenKeyword && tokenValue == "type" {
		return parseTypeDecl(tokens, pos)
	}

	// If we can't parse it, skip the token to avoid infinite loop
	var expr Node
	expr, pos = parseStatement(tokens, pos)
	return expr, pos
}

// parseFuncDecl parses a function or method declaration
func parseFuncDecl(tokens []Token, pos int) (Node, int) {
	startLine := peekToken(tokens, pos).Line
	pos = pos + 1 // skip 'func'

	// Check for receiver (method declaration)
	isMethod := false
	var receiverNode Node
	if peekTokenType(tokens, pos) == TokenLParen {
		// This is a method with a receiver
		isMethod = true
		receiverNode = NewNode(NodeReceiver)
		pos = pos + 1 // skip '('
		// Parse receiver: (name Type) or (*Type)
		if peekTokenType(tokens, pos) == TokenIdentifier {
			recvName := peekTokenValue(tokens, pos)
			pos = pos + 1
			// Check if next is * (pointer receiver) or identifier (type name)
			if peekTokenType(tokens, pos) == TokenStar {
				pos = pos + 1
				if peekTokenType(tokens, pos) == TokenIdentifier {
					receiverNode.Name = recvName
					ptrType := NewNodeWithName(NodePointerType, peekTokenValue(tokens, pos))
					receiverNode = AddChild(receiverNode, ptrType)
					pos = pos + 1
				}
			} else if peekTokenType(tokens, pos) == TokenIdentifier {
				receiverNode.Name = recvName
				typeNode := NewNodeWithName(NodeName, peekTokenValue(tokens, pos))
				receiverNode = AddChild(receiverNode, typeNode)
				pos = pos + 1
			} else if peekTokenType(tokens, pos) == TokenRParen {
				// Single identifier in parens: it's actually the type, not a name
				typeNode := NewNodeWithName(NodeName, recvName)
				receiverNode = AddChild(receiverNode, typeNode)
			}
		} else if peekTokenType(tokens, pos) == TokenStar {
			pos = pos + 1
			if peekTokenType(tokens, pos) == TokenIdentifier {
				ptrType := NewNodeWithName(NodePointerType, peekTokenValue(tokens, pos))
				receiverNode = AddChild(receiverNode, ptrType)
				pos = pos + 1
			}
		}
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1
		}
	}

	// Function name
	funcName := ""
	if peekTokenType(tokens, pos) == TokenIdentifier {
		funcName = peekTokenValue(tokens, pos)
		pos = pos + 1
	}

	var node Node
	if isMethod {
		node = NewNodeWithName(NodeMethodDecl, funcName)
		node = AddChild(node, receiverNode)
	} else {
		node = NewNodeWithName(NodeFuncDecl, funcName)
	}
	node = SetLine(node, startLine)

	// Parameters
	var paramList Node
	paramList, pos = parseParamList(tokens, pos)
	node = AddChild(node, paramList)

	// Return types
	var resultList Node
	resultList, pos = parseResultTypes(tokens, pos)
	if len(resultList.Children) > 0 {
		node = AddChild(node, resultList)
	}

	// Function body
	if peekTokenType(tokens, pos) == TokenLBrace {
		var body Node
		body, pos = parseBlock(tokens, pos)
		node = AddChild(node, body)
	}

	// Skip semicolon
	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return node, pos
}

// parseParamList parses a parameter list in parentheses
func parseParamList(tokens []Token, pos int) (Node, int) {
	paramList := NewNode(NodeParamList)

	if peekTokenType(tokens, pos) != TokenLParen {
		return paramList, pos
	}
	pos = pos + 1 // skip '('

	for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
		param := NewNode(NodeParam)
		param = SetLine(param, peekToken(tokens, pos).Line)

		// Check for variadic (...)
		if peekTokenType(tokens, pos) == TokenEllipsis {
			pos = pos + 1
			// Type after ...
			if peekTokenType(tokens, pos) == TokenIdentifier {
				param.Name = ""
				varType := NewNodeWithName(NodeVariadic, peekTokenValue(tokens, pos))
				param = AddChild(param, varType)
				pos = pos + 1
			}
			paramList = AddChild(paramList, param)
			if peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1
			}
			continue
		}

		// Parse parameter: could be "name Type", "name ...Type", "Type", or just name
		if peekTokenType(tokens, pos) == TokenIdentifier {
			firstNameForName := peekTokenValue(tokens, pos)
			firstNameForConcat := peekTokenValue(tokens, pos)
			firstNameForNode := peekTokenValue(tokens, pos)
			pos = pos + 1

			// Check what follows
			if peekTokenType(tokens, pos) == TokenEllipsis {
				// name ...Type
				param.Name = firstNameForName
				pos = pos + 1
				if peekTokenType(tokens, pos) == TokenIdentifier || peekTokenType(tokens, pos) == TokenLBracket || peekTokenType(tokens, pos) == TokenStar || (peekTokenType(tokens, pos) == TokenKeyword && (peekTokenValue(tokens, pos) == "func" || peekTokenValue(tokens, pos) == "map" || peekTokenValue(tokens, pos) == "struct" || peekTokenValue(tokens, pos) == "interface" || peekTokenValue(tokens, pos) == "chan")) {
					var varType Node
					varType, pos = parseTypeExpr(tokens, pos)
					varNode := NewNode(NodeVariadic)
					varNode = AddChild(varNode, varType)
					param = AddChild(param, varNode)
				}
			} else if peekTokenType(tokens, pos) == TokenIdentifier || peekTokenType(tokens, pos) == TokenLBracket || peekTokenType(tokens, pos) == TokenStar || (peekTokenType(tokens, pos) == TokenKeyword && (peekTokenValue(tokens, pos) == "func" || peekTokenValue(tokens, pos) == "map" || peekTokenValue(tokens, pos) == "struct" || peekTokenValue(tokens, pos) == "interface" || peekTokenValue(tokens, pos) == "chan")) {
				// name Type
				param.Name = firstNameForName
				var typeNode Node
				typeNode, pos = parseTypeExpr(tokens, pos)
				param = AddChild(param, typeNode)
			} else if peekTokenType(tokens, pos) == TokenDot {
				// qualified type: pkg.Type
				pos = pos + 1
				if peekTokenType(tokens, pos) == TokenIdentifier {
					qualName := firstNameForConcat
					qualName += "."
					qualName += peekTokenValue(tokens, pos)
					pos = pos + 1
					typeNode := NewNodeWithName(NodeName, qualName)
					param = AddChild(param, typeNode)
				}
			} else if peekTokenType(tokens, pos) == TokenComma || peekTokenType(tokens, pos) == TokenRParen {
				// Just a name (type will come later or this is unnamed params)
				param.Name = firstNameForName
			} else {
				// Just a type name (no param name)
				param.Name = ""
				typeNode := NewNodeWithName(NodeName, firstNameForNode)
				param = AddChild(param, typeNode)
			}
		} else if peekTokenType(tokens, pos) == TokenStar {
			// *Type parameter (unnamed)
			var typeNode Node
			typeNode, pos = parseTypeExpr(tokens, pos)
			param = AddChild(param, typeNode)
		} else if peekTokenType(tokens, pos) == TokenLBracket {
			// []Type parameter (unnamed)
			var typeNode Node
			typeNode, pos = parseTypeExpr(tokens, pos)
			param = AddChild(param, typeNode)
		} else if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "func" {
			var typeNode Node
			typeNode, pos = parseTypeExpr(tokens, pos)
			param = AddChild(param, typeNode)
		}

		paramList = AddChild(paramList, param)

		if peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
		}
	}

	if peekTokenType(tokens, pos) == TokenRParen {
		pos = pos + 1
	}

	return paramList, pos
}

// parseResultTypes parses return types
func parseResultTypes(tokens []Token, pos int) (Node, int) {
	resultList := NewNode(NodeResultList)

	// No return type
	if peekTokenType(tokens, pos) == TokenLBrace || peekTokenType(tokens, pos) == TokenSemicolon || peekTokenType(tokens, pos) == TokenEOF {
		return resultList, pos
	}

	if peekTokenType(tokens, pos) == TokenLParen {
		// Multiple return types: (Type1, Type2)
		pos = pos + 1 // skip '('
		for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
			// Check for named return
			if peekTokenType(tokens, pos) == TokenIdentifier && peekTokenType(tokens, pos+1) == TokenIdentifier {
				// Named return: name Type
				param := NewNode(NodeParam)
				param.Name = peekTokenValue(tokens, pos)
				pos = pos + 1
				var typeNode Node
				typeNode, pos = parseTypeExpr(tokens, pos)
				param = AddChild(param, typeNode)
				resultList = AddChild(resultList, param)
			} else {
				var typeNode Node
				typeNode, pos = parseTypeExpr(tokens, pos)
				resultList = AddChild(resultList, typeNode)
			}
			if peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1
			}
		}
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1
		}
	} else {
		// Single return type
		var typeNode Node
		typeNode, pos = parseTypeExpr(tokens, pos)
		resultList = AddChild(resultList, typeNode)
	}

	return resultList, pos
}

// parseTypeExpr parses a type expression
func parseTypeExpr(tokens []Token, pos int) (Node, int) {
	// Pointer type
	if peekTokenType(tokens, pos) == TokenStar {
		pos = pos + 1
		var innerType Node
		innerType, pos = parseTypeExpr(tokens, pos)
		node := NewNode(NodePointerType)
		node = AddChild(node, innerType)
		return node, pos
	}

	// Slice or array type
	if peekTokenType(tokens, pos) == TokenLBracket {
		pos = pos + 1
		if peekTokenType(tokens, pos) == TokenRBracket {
			// Slice type: []Type
			pos = pos + 1
			var sliceElem Node
			sliceElem, pos = parseTypeExpr(tokens, pos)
			sliceNode := NewNode(NodeSliceType)
			sliceNode = AddChild(sliceNode, sliceElem)
			return sliceNode, pos
		}
		// Array type: [N]Type
		var sizeExpr Node
		sizeExpr, pos = parseExpression(tokens, pos)
		if peekTokenType(tokens, pos) == TokenRBracket {
			pos = pos + 1
		}
		var arrElem Node
		arrElem, pos = parseTypeExpr(tokens, pos)
		arrNode := NewNode(NodeArrayType)
		arrNode = AddChild(arrNode, sizeExpr)
		arrNode = AddChild(arrNode, arrElem)
		return arrNode, pos
	}

	// Map type
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "map" {
		pos = pos + 1
		node := NewNode(NodeMapType)
		if peekTokenType(tokens, pos) == TokenLBracket {
			pos = pos + 1
			var keyType Node
			keyType, pos = parseTypeExpr(tokens, pos)
			node = AddChild(node, keyType)
			if peekTokenType(tokens, pos) == TokenRBracket {
				pos = pos + 1
			}
		}
		var valType Node
		valType, pos = parseTypeExpr(tokens, pos)
		node = AddChild(node, valType)
		return node, pos
	}

	// Channel type
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "chan" {
		pos = pos + 1
		node := NewNode(NodeChanType)
		// Check for chan<- (send-only)
		if peekTokenType(tokens, pos) == TokenArrow {
			node.Op = "send"
			pos = pos + 1
		}
		var elemType Node
		elemType, pos = parseTypeExpr(tokens, pos)
		node = AddChild(node, elemType)
		return node, pos
	}

	// <-chan (receive-only channel)
	if peekTokenType(tokens, pos) == TokenArrow {
		pos = pos + 1
		if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "chan" {
			pos = pos + 1
			node := NewNode(NodeChanType)
			node.Op = "recv"
			var elemType Node
			elemType, pos = parseTypeExpr(tokens, pos)
			node = AddChild(node, elemType)
			return node, pos
		}
	}

	// func type
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "func" {
		pos = pos + 1
		node := NewNode(NodeFuncType)
		var plist Node
		plist, pos = parseParamList(tokens, pos)
		node = AddChild(node, plist)
		var results Node
		results, pos = parseResultTypes(tokens, pos)
		if len(results.Children) > 0 {
			node = AddChild(node, results)
		}
		return node, pos
	}

	// struct type
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "struct" {
		return parseStructType(tokens, pos)
	}

	// interface type
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "interface" {
		return parseInterfaceType(tokens, pos)
	}

	// Named type (identifier, possibly qualified)
	if peekTokenType(tokens, pos) == TokenIdentifier {
		typeNameForConcat := peekTokenValue(tokens, pos)
		typeNameForReturn := peekTokenValue(tokens, pos)
		pos = pos + 1
		// Check for qualified name (pkg.Type)
		if peekTokenType(tokens, pos) == TokenDot && peekTokenType(tokens, pos+1) == TokenIdentifier {
			pos = pos + 1 // skip '.'
			qualName := typeNameForConcat
			qualName += "."
			qualName += peekTokenValue(tokens, pos)
			pos = pos + 1
			return NewNodeWithName(NodeName, qualName), pos
		}
		return NewNodeWithName(NodeName, typeNameForReturn), pos
	}

	// Fallback: return empty name node
	return NewNode(NodeName), pos
}

// parseStructType parses a struct type definition
func parseStructType(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeStructType)
	node = SetLine(node, peekToken(tokens, pos).Line)
	pos = pos + 1 // skip 'struct'

	if peekTokenType(tokens, pos) == TokenLBrace {
		pos = pos + 1 // skip '{'
		for peekTokenType(tokens, pos) != TokenRBrace && peekTokenType(tokens, pos) != TokenEOF {
			// Skip semicolons
			for peekTokenType(tokens, pos) == TokenSemicolon {
				pos = pos + 1
			}
			if peekTokenType(tokens, pos) == TokenRBrace {
				break
			}

			field := NewNode(NodeField)
			field = SetLine(field, peekToken(tokens, pos).Line)

			if peekTokenType(tokens, pos) == TokenIdentifier {
				// Could be "Name Type" or just embedded type
				firstNameForField := peekTokenValue(tokens, pos)
				firstNameForConcat := peekTokenValue(tokens, pos)
				savedPos := pos
				pos = pos + 1

				if peekTokenType(tokens, pos) == TokenIdentifier || peekTokenType(tokens, pos) == TokenStar || peekTokenType(tokens, pos) == TokenLBracket || (peekTokenType(tokens, pos) == TokenKeyword && (peekTokenValue(tokens, pos) == "func" || peekTokenValue(tokens, pos) == "map" || peekTokenValue(tokens, pos) == "struct" || peekTokenValue(tokens, pos) == "interface" || peekTokenValue(tokens, pos) == "chan")) {
					// Name Type
					field.Name = firstNameForField
					var fieldType Node
					fieldType, pos = parseTypeExpr(tokens, pos)
					field = AddChild(field, fieldType)
				} else if peekTokenType(tokens, pos) == TokenDot {
					// Could be embedded qualified type or name with qualified type
					pos = pos + 1
					if peekTokenType(tokens, pos) == TokenIdentifier {
						qualName := firstNameForConcat
						qualName += "."
						qualName += peekTokenValue(tokens, pos)
						pos = pos + 1
						// Embedded qualified type
						field.Name = ""
						typeNode := NewNodeWithName(NodeName, qualName)
						field = AddChild(field, typeNode)
					}
				} else {
					// Embedded type (just a name)
					pos = savedPos
					var fieldType Node
					fieldType, pos = parseTypeExpr(tokens, pos)
					field = AddChild(field, fieldType)
				}
			} else if peekTokenType(tokens, pos) == TokenStar {
				// Embedded pointer type
				var fieldType Node
				fieldType, pos = parseTypeExpr(tokens, pos)
				field = AddChild(field, fieldType)
			}

			// Skip optional struct tag (string literal)
			if peekTokenType(tokens, pos) == TokenString {
				field.Value = peekTokenValue(tokens, pos)
				pos = pos + 1
			}

			node = AddChild(node, field)

			// Skip semicolons
			for peekTokenType(tokens, pos) == TokenSemicolon {
				pos = pos + 1
			}
		}
		if peekTokenType(tokens, pos) == TokenRBrace {
			pos = pos + 1
		}
	}

	return node, pos
}

// parseInterfaceType parses an interface type definition
func parseInterfaceType(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeInterfaceType)
	node = SetLine(node, peekToken(tokens, pos).Line)
	pos = pos + 1 // skip 'interface'

	if peekTokenType(tokens, pos) == TokenLBrace {
		pos = pos + 1 // skip '{'
		for peekTokenType(tokens, pos) != TokenRBrace && peekTokenType(tokens, pos) != TokenEOF {
			// Skip semicolons
			for peekTokenType(tokens, pos) == TokenSemicolon {
				pos = pos + 1
			}
			if peekTokenType(tokens, pos) == TokenRBrace {
				break
			}

			if peekTokenType(tokens, pos) == TokenIdentifier {
				methodNameForMethod := peekTokenValue(tokens, pos)
				methodNameForEmbed := peekTokenValue(tokens, pos)
				methodNameForConcat := peekTokenValue(tokens, pos)
				pos = pos + 1

				if peekTokenType(tokens, pos) == TokenLParen {
					// Method spec: Name(plist) returnTypes
					method := NewNodeWithName(NodeMethodSpec, methodNameForMethod)
					method = SetLine(method, peekToken(tokens, pos-1).Line)
					var plist Node
					plist, pos = parseParamList(tokens, pos)
					method = AddChild(method, plist)
					var results Node
					results, pos = parseResultTypes(tokens, pos)
					if len(results.Children) > 0 {
						method = AddChild(method, results)
					}
					node = AddChild(node, method)
				} else {
					// Embedded interface type
					embeddedType := NewNodeWithName(NodeName, methodNameForEmbed)
					if peekTokenType(tokens, pos) == TokenDot {
						pos = pos + 1
						if peekTokenType(tokens, pos) == TokenIdentifier {
							qualConcat := methodNameForConcat
							qualConcat += "."
							qualConcat += peekTokenValue(tokens, pos)
							embeddedType.Name = qualConcat
							pos = pos + 1
						}
					}
					node = AddChild(node, embeddedType)
				}
			}

			// Skip semicolons
			for peekTokenType(tokens, pos) == TokenSemicolon {
				pos = pos + 1
			}
		}
		if peekTokenType(tokens, pos) == TokenRBrace {
			pos = pos + 1
		}
	}

	return node, pos
}

// parseVarDecl parses a var declaration
func parseVarDecl(tokens []Token, pos int) (Node, int) {
	startLine := peekToken(tokens, pos).Line
	pos = pos + 1 // skip 'var'

	var spec Node
	if peekTokenType(tokens, pos) == TokenLParen {
		// Grouped var declaration
		pos = pos + 1 // skip '('
		groupNode := NewNode(NodeVarDecl)
		groupNode = SetLine(groupNode, startLine)
		groupNode.Name = "(group)"
		for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
			for peekTokenType(tokens, pos) == TokenSemicolon {
				pos = pos + 1
			}
			if peekTokenType(tokens, pos) == TokenRParen {
				break
			}
			spec, pos = parseVarSpec(tokens, pos, startLine)
			groupNode = AddChild(groupNode, spec)
		}
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1
		}
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return groupNode, pos
	}

	// Single var declaration
	spec, pos = parseVarSpec(tokens, pos, startLine)
	return spec, pos
}

// parseVarSpec parses a single var spec: name Type = expr or name = expr
func parseVarSpec(tokens []Token, pos int, startLine int) (Node, int) {
	node := NewNode(NodeVarDecl)
	node = SetLine(node, startLine)

	if peekTokenType(tokens, pos) == TokenIdentifier {
		node.Name = peekTokenValue(tokens, pos)
		pos = pos + 1
	}

	// Check for type
	if peekTokenType(tokens, pos) != TokenAssign && peekTokenType(tokens, pos) != TokenSemicolon && peekTokenType(tokens, pos) != TokenEOF && peekTokenType(tokens, pos) != TokenRParen {
		var typeNode Node
		typeNode, pos = parseTypeExpr(tokens, pos)
		node = AddChild(node, typeNode)
	}

	// Check for initializer
	if peekTokenType(tokens, pos) == TokenAssign {
		pos = pos + 1
		var value Node
		value, pos = parseExpressionWithCompositeLit(tokens, pos)
		node = AddChild(node, value)
	}

	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return node, pos
}

// parseConstDecl parses a const declaration
func parseConstDecl(tokens []Token, pos int) (Node, int) {
	startLine := peekToken(tokens, pos).Line
	pos = pos + 1 // skip 'const'

	var spec Node
	if peekTokenType(tokens, pos) == TokenLParen {
		// Grouped const declaration
		pos = pos + 1 // skip '('
		groupNode := NewNode(NodeConstDecl)
		groupNode = SetLine(groupNode, startLine)
		groupNode.Name = "(group)"
		for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
			for peekTokenType(tokens, pos) == TokenSemicolon {
				pos = pos + 1
			}
			if peekTokenType(tokens, pos) == TokenRParen {
				break
			}
			spec, pos = parseConstSpec(tokens, pos, startLine)
			groupNode = AddChild(groupNode, spec)
		}
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1
		}
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return groupNode, pos
	}

	// Single const declaration
	spec, pos = parseConstSpec(tokens, pos, startLine)
	return spec, pos
}

// parseConstSpec parses a single const spec
func parseConstSpec(tokens []Token, pos int, startLine int) (Node, int) {
	node := NewNode(NodeConstDecl)
	node = SetLine(node, startLine)

	if peekTokenType(tokens, pos) == TokenIdentifier {
		node.Name = peekTokenValue(tokens, pos)
		pos = pos + 1
	}

	// Check for type
	if peekTokenType(tokens, pos) != TokenAssign && peekTokenType(tokens, pos) != TokenSemicolon && peekTokenType(tokens, pos) != TokenEOF && peekTokenType(tokens, pos) != TokenRParen {
		var typeNode Node
		typeNode, pos = parseTypeExpr(tokens, pos)
		node = AddChild(node, typeNode)
	}

	// Check for initializer
	if peekTokenType(tokens, pos) == TokenAssign {
		pos = pos + 1
		var value Node
		value, pos = parseExpressionWithCompositeLit(tokens, pos)
		node = AddChild(node, value)
	}

	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return node, pos
}

// parseTypeDecl parses a type declaration
func parseTypeDecl(tokens []Token, pos int) (Node, int) {
	startLine := peekToken(tokens, pos).Line
	pos = pos + 1 // skip 'type'

	var spec Node
	if peekTokenType(tokens, pos) == TokenLParen {
		// Grouped type declaration
		pos = pos + 1 // skip '('
		groupNode := NewNode(NodeTypeDecl)
		groupNode = SetLine(groupNode, startLine)
		groupNode.Name = "(group)"
		for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
			for peekTokenType(tokens, pos) == TokenSemicolon {
				pos = pos + 1
			}
			if peekTokenType(tokens, pos) == TokenRParen {
				break
			}
			spec, pos = parseTypeSpec(tokens, pos, startLine)
			groupNode = AddChild(groupNode, spec)
		}
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1
		}
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return groupNode, pos
	}

	// Single type declaration
	spec, pos = parseTypeSpec(tokens, pos, startLine)
	return spec, pos
}

// parseTypeSpec parses a single type spec: Name Type
func parseTypeSpec(tokens []Token, pos int, startLine int) (Node, int) {
	node := NewNode(NodeTypeDecl)
	node = SetLine(node, startLine)

	if peekTokenType(tokens, pos) == TokenIdentifier {
		node.Name = peekTokenValue(tokens, pos)
		pos = pos + 1
	}

	// Parse the underlying type
	var typeNode Node
	typeNode, pos = parseTypeExpr(tokens, pos)
	node = AddChild(node, typeNode)

	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return node, pos
}

// parseBlock parses a braced block
func parseBlock(tokens []Token, pos int) (Node, int) {
	block := NewNode(NodeBlock)
	block = SetLine(block, peekToken(tokens, pos).Line)

	if peekTokenType(tokens, pos) != TokenLBrace {
		return block, pos
	}
	pos = pos + 1 // skip '{'

	for peekTokenType(tokens, pos) != TokenRBrace && peekTokenType(tokens, pos) != TokenEOF {
		// Skip semicolons
		for peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		if peekTokenType(tokens, pos) == TokenRBrace || peekTokenType(tokens, pos) == TokenEOF {
			break
		}

		var stmt Node
		stmt, pos = parseStatement(tokens, pos)
		block = AddChild(block, stmt)
	}

	if peekTokenType(tokens, pos) == TokenRBrace {
		pos = pos + 1
	}

	// Skip semicolon after block
	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return block, pos
}

// parseStatement parses a single statement
func parseStatement(tokens []Token, pos int) (Node, int) {
	tokenType := peekTokenType(tokens, pos)
	tokenValue := peekTokenValue(tokens, pos)

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

	// switch statement
	if tokenType == TokenKeyword && tokenValue == "switch" {
		return parseSwitch(tokens, pos)
	}

	// select statement
	if tokenType == TokenKeyword && tokenValue == "select" {
		return parseSelect(tokens, pos)
	}

	// var declaration
	if tokenType == TokenKeyword && tokenValue == "var" {
		return parseVarDecl(tokens, pos)
	}

	// const declaration
	if tokenType == TokenKeyword && tokenValue == "const" {
		return parseConstDecl(tokens, pos)
	}

	// type declaration
	if tokenType == TokenKeyword && tokenValue == "type" {
		return parseTypeDecl(tokens, pos)
	}

	// defer statement
	if tokenType == TokenKeyword && tokenValue == "defer" {
		return parseDefer(tokens, pos)
	}

	// go statement
	if tokenType == TokenKeyword && tokenValue == "go" {
		return parseGo(tokens, pos)
	}

	// break statement
	if tokenType == TokenKeyword && tokenValue == "break" {
		node := NewNode(NodeBreak)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		// Optional label
		if peekTokenType(tokens, pos) == TokenIdentifier {
			node.Name = peekTokenValue(tokens, pos)
			pos = pos + 1
		}
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return node, pos
	}

	// continue statement
	if tokenType == TokenKeyword && tokenValue == "continue" {
		node := NewNode(NodeContinue)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		// Optional label
		if peekTokenType(tokens, pos) == TokenIdentifier {
			node.Name = peekTokenValue(tokens, pos)
			pos = pos + 1
		}
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return node, pos
	}

	// goto statement
	if tokenType == TokenKeyword && tokenValue == "goto" {
		node := NewNode(NodeGoto)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		if peekTokenType(tokens, pos) == TokenIdentifier {
			node.Name = peekTokenValue(tokens, pos)
			pos = pos + 1
		}
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return node, pos
	}

	// fallthrough statement
	if tokenType == TokenKeyword && tokenValue == "fallthrough" {
		node := NewNode(NodeFallthrough)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return node, pos
	}

	// func literal or nested func declaration
	if tokenType == TokenKeyword && tokenValue == "func" {
		return parseFuncDecl(tokens, pos)
	}

	// Label statement (identifier followed by colon)
	if tokenType == TokenIdentifier && peekTokenType(tokens, pos+1) == TokenColon {
		node := NewNodeWithName(NodeLabel, peekTokenValue(tokens, pos))
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 2 // skip name and ':'
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		// Parse the labeled statement if there is one
		if peekTokenType(tokens, pos) != TokenRBrace && peekTokenType(tokens, pos) != TokenEOF {
			var stmt Node
			stmt, pos = parseStatement(tokens, pos)
			node = AddChild(node, stmt)
		}
		return node, pos
	}

	// Expression statement, assignment, short decl, send, inc/dec
	return parseSimpleStmt(tokens, pos)
}

// parseSimpleStmt parses an expression-based statement
func parseSimpleStmt(tokens []Token, pos int) (Node, int) {
	startLine := peekToken(tokens, pos).Line

	// Parse left-hand side expression(s)
	var firstExpr Node
	firstExpr, pos = parseExpression(tokens, pos)

	// Check for multiple left-hand expressions (a, b, c = ... or a, b := ...)
	if peekTokenType(tokens, pos) == TokenComma {
		exprList := NewNode(NodeExprList)
		exprList = AddChild(exprList, firstExpr)
		for peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
			var nextExpr Node
			nextExpr, pos = parseExpression(tokens, pos)
			exprList = AddChild(exprList, nextExpr)
		}

		// Assignment or short declaration
		if peekTokenType(tokens, pos) == TokenAssign {
			pos = pos + 1
			node := NewNode(NodeAssign)
			node = SetLine(node, startLine)
			node = AddChild(node, exprList)
			// Parse right-hand side
			rhsList := NewNode(NodeExprList)
			var rhs Node
			rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
			rhsList = AddChild(rhsList, rhs)
			for peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1
				rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
				rhsList = AddChild(rhsList, rhs)
			}
			node = AddChild(node, rhsList)
			if peekTokenType(tokens, pos) == TokenSemicolon {
				pos = pos + 1
			}
			return node, pos
		}

		if peekTokenType(tokens, pos) == TokenColonAssign {
			pos = pos + 1
			node := NewNode(NodeShortDecl)
			node = SetLine(node, startLine)
			node = AddChild(node, exprList)
			rhsList := NewNode(NodeExprList)
			var rhs Node
			rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
			rhsList = AddChild(rhsList, rhs)
			for peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1
				rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
				rhsList = AddChild(rhsList, rhs)
			}
			node = AddChild(node, rhsList)
			if peekTokenType(tokens, pos) == TokenSemicolon {
				pos = pos + 1
			}
			return node, pos
		}

		// Just an expression list statement (shouldn't normally happen)
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return exprList, pos
	}

	// Single assignment: expr = expr
	if peekTokenType(tokens, pos) == TokenAssign {
		pos = pos + 1
		node := NewNode(NodeAssign)
		node = SetLine(node, startLine)
		node = AddChild(node, firstExpr)
		var rhs Node
		rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
		node = AddChild(node, rhs)
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return node, pos
	}

	// Short variable declaration: expr := expr
	if peekTokenType(tokens, pos) == TokenColonAssign {
		pos = pos + 1
		node := NewNode(NodeShortDecl)
		node = SetLine(node, startLine)
		node = AddChild(node, firstExpr)
		var rhs Node
		rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
		node = AddChild(node, rhs)
		// Check for multiple values on rhs
		for peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
			rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
			node = AddChild(node, rhs)
		}
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return node, pos
	}

	// Augmented assignment: +=, -=, etc.
	augType := peekTokenType(tokens, pos)
	if augType == TokenPlusAssign || augType == TokenMinusAssign || augType == TokenStarAssign || augType == TokenSlashAssign || augType == TokenPercentAssign || augType == TokenAmpAssign || augType == TokenPipeAssign || augType == TokenCaretAssign || augType == TokenLeftShiftAssign || augType == TokenRightShiftAssign || augType == TokenAmpCaretAssign {
		op := peekTokenValue(tokens, pos)
		pos = pos + 1
		node := NewNodeWithOp(NodeAugAssign, op)
		node = SetLine(node, startLine)
		node = AddChild(node, firstExpr)
		var rhs Node
		rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
		node = AddChild(node, rhs)
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return node, pos
	}

	// Increment/decrement: expr++ or expr--
	if peekTokenType(tokens, pos) == TokenIncrement || peekTokenType(tokens, pos) == TokenDecrement {
		op := peekTokenValue(tokens, pos)
		pos = pos + 1
		node := NewNodeWithOp(NodeIncDec, op)
		node = SetLine(node, startLine)
		node = AddChild(node, firstExpr)
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return node, pos
	}

	// Channel send: expr <- expr
	if peekTokenType(tokens, pos) == TokenArrow {
		pos = pos + 1
		node := NewNode(NodeSend)
		node = SetLine(node, startLine)
		node = AddChild(node, firstExpr)
		var value Node
		value, pos = parseExpression(tokens, pos)
		node = AddChild(node, value)
		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		return node, pos
	}

	// Just an expression statement
	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return firstExpr, pos
}

// parseReturn parses a return statement
func parseReturn(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeReturn)
	node = SetLine(node, peekToken(tokens, pos).Line)
	pos = pos + 1 // skip 'return'

	// Return values (optional)
	if peekTokenType(tokens, pos) != TokenSemicolon && peekTokenType(tokens, pos) != TokenRBrace && peekTokenType(tokens, pos) != TokenEOF {
		var expr Node
		expr, pos = parseExpressionWithCompositeLit(tokens, pos)
		node = AddChild(node, expr)
		for peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
			expr, pos = parseExpressionWithCompositeLit(tokens, pos)
			node = AddChild(node, expr)
		}
	}

	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return node, pos
}

// parseIf parses an if statement
func parseIf(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeIf)
	node = SetLine(node, peekToken(tokens, pos).Line)
	pos = pos + 1 // skip 'if'

	// Check for init statement (simple stmt; condition)
	// We need to look ahead to see if there's a semicolon before '{'
	savedPos := pos
	var firstExpr Node
	firstExpr, pos = parseSimpleStmtNoSemiSkip(tokens, pos)

	if peekTokenType(tokens, pos) == TokenSemicolon {
		// There's an init statement
		pos = pos + 1 // skip ';'
		node = AddChild(node, firstExpr) // init statement
		// Parse condition
		var condition Node
		condition, pos = parseExpression(tokens, pos)
		node = AddChild(node, condition)
	} else if peekTokenType(tokens, pos) == TokenLBrace {
		// No init statement, firstExpr is the condition
		node = AddChild(node, firstExpr)
	} else {
		// Fallback: restore and just parse expression
		pos = savedPos
		var condition Node
		condition, pos = parseExpression(tokens, pos)
		node = AddChild(node, condition)
	}

	// Body block
	if peekTokenType(tokens, pos) == TokenLBrace {
		var body Node
		body, pos = parseBlock(tokens, pos)
		node = AddChild(node, body)
	}

	// Else clause
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "else" {
		pos = pos + 1 // skip 'else'
		if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "if" {
			// else if
			var elseIf Node
			elseIf, pos = parseIf(tokens, pos)
			elseNode := NewNode(NodeElse)
			elseNode = AddChild(elseNode, elseIf)
			node = AddChild(node, elseNode)
		} else if peekTokenType(tokens, pos) == TokenLBrace {
			var elseBody Node
			elseBody, pos = parseBlock(tokens, pos)
			elseNode := NewNode(NodeElse)
			elseNode = AddChild(elseNode, elseBody)
			node = AddChild(node, elseNode)
		}
	}

	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return node, pos
}

// parseSimpleStmtNoSemiSkip parses a simple statement but doesn't consume the trailing semicolon
func parseSimpleStmtNoSemiSkip(tokens []Token, pos int) (Node, int) {
	startLine := peekToken(tokens, pos).Line

	var firstExpr Node
	firstExpr, pos = parseExpression(tokens, pos)

	// Check for multiple left-hand expressions
	if peekTokenType(tokens, pos) == TokenComma {
		exprList := NewNode(NodeExprList)
		exprList = AddChild(exprList, firstExpr)
		for peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
			var nextExpr Node
			nextExpr, pos = parseExpression(tokens, pos)
			exprList = AddChild(exprList, nextExpr)
		}

		if peekTokenType(tokens, pos) == TokenAssign {
			pos = pos + 1
			node := NewNode(NodeAssign)
			node = SetLine(node, startLine)
			node = AddChild(node, exprList)
			rhsList := NewNode(NodeExprList)
			var rhs Node
			rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
			rhsList = AddChild(rhsList, rhs)
			for peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1
				rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
				rhsList = AddChild(rhsList, rhs)
			}
			node = AddChild(node, rhsList)
			return node, pos
		}

		if peekTokenType(tokens, pos) == TokenColonAssign {
			pos = pos + 1
			node := NewNode(NodeShortDecl)
			node = SetLine(node, startLine)
			node = AddChild(node, exprList)
			rhsList := NewNode(NodeExprList)
			var rhs Node
			rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
			rhsList = AddChild(rhsList, rhs)
			for peekTokenType(tokens, pos) == TokenComma {
				pos = pos + 1
				rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
				rhsList = AddChild(rhsList, rhs)
			}
			node = AddChild(node, rhsList)
			return node, pos
		}

		return exprList, pos
	}

	// Single assignment
	if peekTokenType(tokens, pos) == TokenAssign {
		pos = pos + 1
		node := NewNode(NodeAssign)
		node = SetLine(node, startLine)
		node = AddChild(node, firstExpr)
		var rhs Node
		rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
		node = AddChild(node, rhs)
		return node, pos
	}

	// Short variable declaration
	if peekTokenType(tokens, pos) == TokenColonAssign {
		pos = pos + 1
		node := NewNode(NodeShortDecl)
		node = SetLine(node, startLine)
		node = AddChild(node, firstExpr)
		var rhs Node
		rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
		node = AddChild(node, rhs)
		for peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
			rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
			node = AddChild(node, rhs)
		}
		return node, pos
	}

	// Augmented assignment
	augType := peekTokenType(tokens, pos)
	if augType == TokenPlusAssign || augType == TokenMinusAssign || augType == TokenStarAssign || augType == TokenSlashAssign || augType == TokenPercentAssign || augType == TokenAmpAssign || augType == TokenPipeAssign || augType == TokenCaretAssign || augType == TokenLeftShiftAssign || augType == TokenRightShiftAssign || augType == TokenAmpCaretAssign {
		op := peekTokenValue(tokens, pos)
		pos = pos + 1
		node := NewNodeWithOp(NodeAugAssign, op)
		node = SetLine(node, startLine)
		node = AddChild(node, firstExpr)
		var rhs Node
		rhs, pos = parseExpressionWithCompositeLit(tokens, pos)
		node = AddChild(node, rhs)
		return node, pos
	}

	// Inc/Dec
	if peekTokenType(tokens, pos) == TokenIncrement || peekTokenType(tokens, pos) == TokenDecrement {
		op := peekTokenValue(tokens, pos)
		pos = pos + 1
		node := NewNodeWithOp(NodeIncDec, op)
		node = SetLine(node, startLine)
		node = AddChild(node, firstExpr)
		return node, pos
	}

	// Channel send
	if peekTokenType(tokens, pos) == TokenArrow {
		pos = pos + 1
		node := NewNode(NodeSend)
		node = SetLine(node, startLine)
		node = AddChild(node, firstExpr)
		var value Node
		value, pos = parseExpression(tokens, pos)
		node = AddChild(node, value)
		return node, pos
	}

	return firstExpr, pos
}

// parseFor parses a for statement
func parseFor(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeFor)
	node = SetLine(node, peekToken(tokens, pos).Line)
	pos = pos + 1 // skip 'for'

	// Infinite loop: for { ... }
	if peekTokenType(tokens, pos) == TokenLBrace {
		var body Node
		body, pos = parseBlock(tokens, pos)
		node = AddChild(node, body)
		return node, pos
	}

	// Try to parse: for range expr { ... }
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "range" {
		pos = pos + 1
		rangeClause := NewNode(NodeRangeClause)
		var rangeExpr Node
		rangeExpr, pos = parseExpression(tokens, pos)
		rangeClause = AddChild(rangeClause, rangeExpr)
		node = AddChild(node, rangeClause)
		if peekTokenType(tokens, pos) == TokenLBrace {
			var body Node
			body, pos = parseBlock(tokens, pos)
			node = AddChild(node, body)
		}
		return node, pos
	}

	// Check if this is a range loop: for vars := range expr or for vars = range expr
	if hasRangeKeyword(tokens, pos) {
		return parseForRange(tokens, pos, node)
	}

	// Parse first part (could be init or condition)
	savedPos := pos
	var firstStmt Node
	firstStmt, pos = parseSimpleStmtNoSemiSkip(tokens, pos)

	// Check if semicolon follows (3-clause for)
	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
		// 3-clause for: for init; condition; post { body }
		forClause := NewNode(NodeForClause)
		forClause = AddChild(forClause, firstStmt) // init

		// Parse condition
		if peekTokenType(tokens, pos) != TokenSemicolon {
			var condition Node
			condition, pos = parseExpression(tokens, pos)
			forClause = AddChild(forClause, condition)
		} else {
			// Empty condition
			forClause = AddChild(forClause, NewNode(NodeName))
		}

		if peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}

		// Parse post statement
		if peekTokenType(tokens, pos) != TokenLBrace {
			var post Node
			post, pos = parseSimpleStmtNoSemiSkip(tokens, pos)
			forClause = AddChild(forClause, post)
		} else {
			forClause = AddChild(forClause, NewNode(NodeName))
		}

		node = AddChild(node, forClause)
	} else if peekTokenType(tokens, pos) == TokenLBrace {
		// condition-only for: for condition { body }
		node = AddChild(node, firstStmt) // condition
	} else {
		// Shouldn't normally happen, restore
		pos = savedPos
		var condition Node
		condition, pos = parseExpression(tokens, pos)
		node = AddChild(node, condition)
	}

	if peekTokenType(tokens, pos) == TokenLBrace {
		var body Node
		body, pos = parseBlock(tokens, pos)
		node = AddChild(node, body)
	}

	return node, pos
}

// parseForRange parses a for-range statement: for [vars :=/=] range expr { body }
func parseForRange(tokens []Token, pos int, node Node) (Node, int) {
	rangeClause := NewNode(NodeRangeClause)

	// Parse variables before := or = range
	var vars []Node
	var firstVar Node
	firstVar, pos = parseExpression(tokens, pos)
	vars = append(vars, firstVar)

	for peekTokenType(tokens, pos) == TokenComma {
		pos = pos + 1
		var nextVar Node
		nextVar, pos = parseExpression(tokens, pos)
		vars = append(vars, nextVar)
	}

	// Detect := or =
	assignOp := ""
	if peekTokenType(tokens, pos) == TokenColonAssign {
		assignOp = ":="
		pos = pos + 1
	} else if peekTokenType(tokens, pos) == TokenAssign {
		assignOp = "="
		pos = pos + 1
	}

	// Skip 'range' keyword
	if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "range" {
		pos = pos + 1
	}

	// Build the variable list
	if len(vars) == 1 {
		rangeClause = AddChild(rangeClause, vars[0])
	} else {
		exprList := NewNode(NodeExprList)
		for i := 0; i < len(vars); i++ {
			exprList = AddChild(exprList, vars[i])
		}
		rangeClause = AddChild(rangeClause, exprList)
	}
	rangeClause.Op = assignOp

	// Parse range expression
	var rangeExpr Node
	rangeExpr, pos = parseExpression(tokens, pos)
	rangeClause = AddChild(rangeClause, rangeExpr)

	node = AddChild(node, rangeClause)

	if peekTokenType(tokens, pos) == TokenLBrace {
		var body Node
		body, pos = parseBlock(tokens, pos)
		node = AddChild(node, body)
	}

	return node, pos
}

// parseSwitch parses a switch statement
func parseSwitch(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeSwitch)
	node = SetLine(node, peekToken(tokens, pos).Line)
	pos = pos + 1 // skip 'switch'

	// Optional init statement and/or tag expression
	if peekTokenType(tokens, pos) != TokenLBrace {
		savedPos := pos
		var firstExpr Node
		firstExpr, pos = parseSimpleStmtNoSemiSkip(tokens, pos)

		if peekTokenType(tokens, pos) == TokenSemicolon {
			// init statement; tag
			pos = pos + 1
			node = AddChild(node, firstExpr) // init
			if peekTokenType(tokens, pos) != TokenLBrace {
				var tag Node
				tag, pos = parseExpression(tokens, pos)
				node = AddChild(node, tag) // tag
			}
		} else if peekTokenType(tokens, pos) == TokenLBrace {
			// Just a tag expression
			node = AddChild(node, firstExpr)
		} else {
			pos = savedPos
			var tag Node
			tag, pos = parseExpression(tokens, pos)
			node = AddChild(node, tag)
		}
	}

	// Switch body
	if peekTokenType(tokens, pos) == TokenLBrace {
		pos = pos + 1 // skip '{'
		for peekTokenType(tokens, pos) != TokenRBrace && peekTokenType(tokens, pos) != TokenEOF {
			for peekTokenType(tokens, pos) == TokenSemicolon {
				pos = pos + 1
			}
			if peekTokenType(tokens, pos) == TokenRBrace {
				break
			}

			if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "case" {
				var caseNode Node
				caseNode, pos = parseCaseClause(tokens, pos)
				node = AddChild(node, caseNode)
			} else if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "default" {
				var defaultNode Node
				defaultNode, pos = parseDefaultClause(tokens, pos)
				node = AddChild(node, defaultNode)
			} else {
				// Skip unexpected token
				pos = pos + 1
			}
		}
		if peekTokenType(tokens, pos) == TokenRBrace {
			pos = pos + 1
		}
	}

	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return node, pos
}

// parseCaseClause parses a case clause in switch
func parseCaseClause(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeCase)
	node = SetLine(node, peekToken(tokens, pos).Line)
	pos = pos + 1 // skip 'case'

	// Parse case expressions (comma-separated)
	var expr Node
	expr, pos = parseExpression(tokens, pos)
	node = AddChild(node, expr)
	for peekTokenType(tokens, pos) == TokenComma {
		pos = pos + 1
		expr, pos = parseExpression(tokens, pos)
		node = AddChild(node, expr)
	}

	// Colon
	if peekTokenType(tokens, pos) == TokenColon {
		pos = pos + 1
	}

	// Skip semicolons
	for peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	// Case body (statements until next case/default/})
	for peekTokenType(tokens, pos) != TokenEOF {
		if peekTokenType(tokens, pos) == TokenKeyword && (peekTokenValue(tokens, pos) == "case" || peekTokenValue(tokens, pos) == "default") {
			break
		}
		if peekTokenType(tokens, pos) == TokenRBrace {
			break
		}
		for peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		if peekTokenType(tokens, pos) == TokenKeyword && (peekTokenValue(tokens, pos) == "case" || peekTokenValue(tokens, pos) == "default") {
			break
		}
		if peekTokenType(tokens, pos) == TokenRBrace || peekTokenType(tokens, pos) == TokenEOF {
			break
		}
		var stmt Node
		stmt, pos = parseStatement(tokens, pos)
		node = AddChild(node, stmt)
	}

	return node, pos
}

// parseDefaultClause parses a default clause in switch
func parseDefaultClause(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeDefault)
	node = SetLine(node, peekToken(tokens, pos).Line)
	pos = pos + 1 // skip 'default'

	// Colon
	if peekTokenType(tokens, pos) == TokenColon {
		pos = pos + 1
	}

	// Skip semicolons
	for peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	// Default body
	for peekTokenType(tokens, pos) != TokenEOF {
		if peekTokenType(tokens, pos) == TokenKeyword && (peekTokenValue(tokens, pos) == "case" || peekTokenValue(tokens, pos) == "default") {
			break
		}
		if peekTokenType(tokens, pos) == TokenRBrace {
			break
		}
		for peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		if peekTokenType(tokens, pos) == TokenKeyword && (peekTokenValue(tokens, pos) == "case" || peekTokenValue(tokens, pos) == "default") {
			break
		}
		if peekTokenType(tokens, pos) == TokenRBrace || peekTokenType(tokens, pos) == TokenEOF {
			break
		}
		var stmt Node
		stmt, pos = parseStatement(tokens, pos)
		node = AddChild(node, stmt)
	}

	return node, pos
}

// parseSelect parses a select statement
func parseSelect(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeSelect)
	node = SetLine(node, peekToken(tokens, pos).Line)
	pos = pos + 1 // skip 'select'

	if peekTokenType(tokens, pos) == TokenLBrace {
		pos = pos + 1 // skip '{'
		for peekTokenType(tokens, pos) != TokenRBrace && peekTokenType(tokens, pos) != TokenEOF {
			for peekTokenType(tokens, pos) == TokenSemicolon {
				pos = pos + 1
			}
			if peekTokenType(tokens, pos) == TokenRBrace {
				break
			}

			if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "case" {
				caseNode := NewNode(NodeSelectCase)
				caseNode = SetLine(caseNode, peekToken(tokens, pos).Line)
				pos = pos + 1 // skip 'case'

				// Parse comm clause (send or receive)
				var commStmt Node
				commStmt, pos = parseSimpleStmtNoSemiSkip(tokens, pos)
				caseNode = AddChild(caseNode, commStmt)

				if peekTokenType(tokens, pos) == TokenColon {
					pos = pos + 1
				}
				for peekTokenType(tokens, pos) == TokenSemicolon {
					pos = pos + 1
				}

				// Body statements
				for peekTokenType(tokens, pos) != TokenEOF {
					if peekTokenType(tokens, pos) == TokenKeyword && (peekTokenValue(tokens, pos) == "case" || peekTokenValue(tokens, pos) == "default") {
						break
					}
					if peekTokenType(tokens, pos) == TokenRBrace {
						break
					}
					for peekTokenType(tokens, pos) == TokenSemicolon {
						pos = pos + 1
					}
					if peekTokenType(tokens, pos) == TokenKeyword && (peekTokenValue(tokens, pos) == "case" || peekTokenValue(tokens, pos) == "default") {
						break
					}
					if peekTokenType(tokens, pos) == TokenRBrace || peekTokenType(tokens, pos) == TokenEOF {
						break
					}
					var stmt Node
					stmt, pos = parseStatement(tokens, pos)
					caseNode = AddChild(caseNode, stmt)
				}
				node = AddChild(node, caseNode)
			} else if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "default" {
				var defaultNode Node
				defaultNode, pos = parseDefaultClause(tokens, pos)
				node = AddChild(node, defaultNode)
			} else {
				pos = pos + 1
			}
		}
		if peekTokenType(tokens, pos) == TokenRBrace {
			pos = pos + 1
		}
	}

	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return node, pos
}

// parseDefer parses a defer statement
func parseDefer(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeDefer)
	node = SetLine(node, peekToken(tokens, pos).Line)
	pos = pos + 1 // skip 'defer'

	var expr Node
	expr, pos = parseExpression(tokens, pos)
	node = AddChild(node, expr)

	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return node, pos
}

// parseGo parses a go statement
func parseGo(tokens []Token, pos int) (Node, int) {
	node := NewNode(NodeGo)
	node = SetLine(node, peekToken(tokens, pos).Line)
	pos = pos + 1 // skip 'go'

	var expr Node
	expr, pos = parseExpression(tokens, pos)
	node = AddChild(node, expr)

	if peekTokenType(tokens, pos) == TokenSemicolon {
		pos = pos + 1
	}

	return node, pos
}

// Expression parsing - precedence climbing

// parseExpression is the entry point for expression parsing
func parseExpression(tokens []Token, pos int) (Node, int) {
	return parseOrExpr(tokens, pos)
}

// parseOrExpr handles || operator (lowest precedence)
func parseOrExpr(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseAndExpr(tokens, pos)

	for peekTokenType(tokens, pos) == TokenLogicalOr {
		pos = pos + 1
		node := NewNodeWithOp(NodeBinOp, "||")
		node = SetLine(node, left.Line)
		var right Node
		right, pos = parseAndExpr(tokens, pos)
		node = AddChild(node, left)
		node = AddChild(node, right)
		left = node
	}

	return left, pos
}

// parseAndExpr handles && operator
func parseAndExpr(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseComparison(tokens, pos)

	for peekTokenType(tokens, pos) == TokenLogicalAnd {
		pos = pos + 1
		node := NewNodeWithOp(NodeBinOp, "&&")
		node = SetLine(node, left.Line)
		var right Node
		right, pos = parseComparison(tokens, pos)
		node = AddChild(node, left)
		node = AddChild(node, right)
		left = node
	}

	return left, pos
}

// parseComparison handles ==, !=, <, <=, >, >=
func parseComparison(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseAddExpr(tokens, pos)

	for {
		tt := peekTokenType(tokens, pos)
		if tt == TokenEqual || tt == TokenNotEqual || tt == TokenLess || tt == TokenLessEqual || tt == TokenGreater || tt == TokenGreaterEqual {
			op := peekTokenValue(tokens, pos)
			pos = pos + 1
			node := NewNodeWithOp(NodeBinOp, op)
			node = SetLine(node, left.Line)
			var right Node
			right, pos = parseAddExpr(tokens, pos)
			node = AddChild(node, left)
			node = AddChild(node, right)
			left = node
		} else {
			break
		}
	}

	return left, pos
}

// parseAddExpr handles +, -, |, ^
func parseAddExpr(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseMulExpr(tokens, pos)

	for {
		tt := peekTokenType(tokens, pos)
		if tt == TokenPlus || tt == TokenMinus || tt == TokenPipe || tt == TokenCaret {
			op := peekTokenValue(tokens, pos)
			pos = pos + 1
			node := NewNodeWithOp(NodeBinOp, op)
			node = SetLine(node, left.Line)
			var right Node
			right, pos = parseMulExpr(tokens, pos)
			node = AddChild(node, left)
			node = AddChild(node, right)
			left = node
		} else {
			break
		}
	}

	return left, pos
}

// parseMulExpr handles *, /, %, <<, >>, &, &^
func parseMulExpr(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseUnary(tokens, pos)

	for {
		tt := peekTokenType(tokens, pos)
		if tt == TokenStar || tt == TokenSlash || tt == TokenPercent || tt == TokenLeftShift || tt == TokenRightShift || tt == TokenAmpersand || tt == TokenAmpCaret {
			op := peekTokenValue(tokens, pos)
			pos = pos + 1
			node := NewNodeWithOp(NodeBinOp, op)
			node = SetLine(node, left.Line)
			var right Node
			right, pos = parseUnary(tokens, pos)
			node = AddChild(node, left)
			node = AddChild(node, right)
			left = node
		} else {
			break
		}
	}

	return left, pos
}

// parseUnary handles unary operators: -, !, ^, *, &, <-
func parseUnary(tokens []Token, pos int) (Node, int) {
	tt := peekTokenType(tokens, pos)

	if tt == TokenMinus || tt == TokenNot || tt == TokenCaret {
		op := peekTokenValue(tokens, pos)
		pos = pos + 1
		node := NewNodeWithOp(NodeUnaryOp, op)
		node = SetLine(node, peekToken(tokens, pos-1).Line)
		var operand Node
		operand, pos = parseUnary(tokens, pos)
		node = AddChild(node, operand)
		return node, pos
	}

	if tt == TokenStar {
		// Dereference (pointer)
		op := "*"
		pos = pos + 1
		node := NewNodeWithOp(NodeUnaryOp, op)
		node = SetLine(node, peekToken(tokens, pos-1).Line)
		var operand Node
		operand, pos = parseUnary(tokens, pos)
		node = AddChild(node, operand)
		return node, pos
	}

	if tt == TokenAmpersand {
		// Address-of
		op := "&"
		pos = pos + 1
		node := NewNodeWithOp(NodeUnaryOp, op)
		node = SetLine(node, peekToken(tokens, pos-1).Line)
		var operand Node
		operand, pos = parseUnary(tokens, pos)
		node = AddChild(node, operand)
		return node, pos
	}

	if tt == TokenArrow {
		// Receive operator <-
		pos = pos + 1
		node := NewNode(NodeRecv)
		node = SetLine(node, peekToken(tokens, pos-1).Line)
		var operand Node
		operand, pos = parseUnary(tokens, pos)
		node = AddChild(node, operand)
		return node, pos
	}

	return parsePrimary(tokens, pos)
}

// parsePrimary handles postfix operations: calls, selectors, index, slice, type assert
func parsePrimary(tokens []Token, pos int) (Node, int) {
	var left Node
	left, pos = parseAtom(tokens, pos)

	for {
		tt := peekTokenType(tokens, pos)

		// Function call: expr(args)
		if tt == TokenLParen {
			pos = pos + 1 // skip '('
			callNode := NewNode(NodeCall)
			callNode = SetLine(callNode, left.Line)
			callNode = AddChild(callNode, left)
			// Parse arguments
			for peekTokenType(tokens, pos) != TokenRParen && peekTokenType(tokens, pos) != TokenEOF {
				var arg Node
				arg, pos = parseExpressionWithCompositeLit(tokens, pos)
				// Check for variadic argument (arg...)
				if peekTokenType(tokens, pos) == TokenEllipsis {
					pos = pos + 1
					varNode := NewNode(NodeVariadic)
					varNode = AddChild(varNode, arg)
					callNode = AddChild(callNode, varNode)
				} else {
					callNode = AddChild(callNode, arg)
				}
				if peekTokenType(tokens, pos) == TokenComma {
					pos = pos + 1
				}
			}
			if peekTokenType(tokens, pos) == TokenRParen {
				pos = pos + 1
			}
			left = callNode
			continue
		}

		// Selector: expr.field
		if tt == TokenDot {
			if peekTokenType(tokens, pos+1) == TokenIdentifier {
				pos = pos + 1 // skip '.'
				selNode := NewNode(NodeSelector)
				selNode = SetLine(selNode, left.Line)
				selNode.Name = peekTokenValue(tokens, pos)
				selNode = AddChild(selNode, left)
				pos = pos + 1
				left = selNode
				continue
			} else if peekTokenType(tokens, pos+1) == TokenLParen {
				// Type assertion: expr.(Type)
				pos = pos + 1 // skip '.'
				pos = pos + 1 // skip '('
				assertNode := NewNode(NodeTypeAssert)
				assertNode = SetLine(assertNode, left.Line)
				assertNode = AddChild(assertNode, left)
				if peekTokenType(tokens, pos) == TokenKeyword && peekTokenValue(tokens, pos) == "type" {
					// Type switch: x.(type)
					assertNode.Value = "type"
					pos = pos + 1
				} else {
					var typeExpr Node
					typeExpr, pos = parseTypeExpr(tokens, pos)
					assertNode = AddChild(assertNode, typeExpr)
				}
				if peekTokenType(tokens, pos) == TokenRParen {
					pos = pos + 1
				}
				left = assertNode
				continue
			}
			break
		}

		// Index or slice: expr[index] or expr[lo:hi] or expr[lo:hi:max]
		if tt == TokenLBracket {
			pos = pos + 1 // skip '['

			// Check for empty slice start: [:hi]
			if peekTokenType(tokens, pos) == TokenColon {
				pos = pos + 1
				sliceNode := NewNode(NodeSliceExpr)
				sliceNode = SetLine(sliceNode, left.Line)
				sliceNode = AddChild(sliceNode, left)
				sliceNode = AddChild(sliceNode, NewNode(NodeName)) // empty low
				if peekTokenType(tokens, pos) != TokenRBracket {
					var hi Node
					hi, pos = parseExpression(tokens, pos)
					sliceNode = AddChild(sliceNode, hi)
				}
				if peekTokenType(tokens, pos) == TokenRBracket {
					pos = pos + 1
				}
				left = sliceNode
				continue
			}

			var indexExpr Node
			indexExpr, pos = parseExpression(tokens, pos)

			if peekTokenType(tokens, pos) == TokenColon {
				// Slice expression
				pos = pos + 1
				sliceNode := NewNode(NodeSliceExpr)
				sliceNode = SetLine(sliceNode, left.Line)
				sliceNode = AddChild(sliceNode, left)
				sliceNode = AddChild(sliceNode, indexExpr) // low

				if peekTokenType(tokens, pos) != TokenRBracket && peekTokenType(tokens, pos) != TokenColon {
					var hi Node
					hi, pos = parseExpression(tokens, pos)
					sliceNode = AddChild(sliceNode, hi)
				}

				// 3-index slice: [lo:hi:max]
				if peekTokenType(tokens, pos) == TokenColon {
					pos = pos + 1
					var maxExpr Node
					maxExpr, pos = parseExpression(tokens, pos)
					sliceNode = AddChild(sliceNode, maxExpr)
				}

				if peekTokenType(tokens, pos) == TokenRBracket {
					pos = pos + 1
				}
				left = sliceNode
				continue
			}

			// Simple index
			idxNode := NewNode(NodeIndex)
			idxNode = SetLine(idxNode, left.Line)
			idxNode = AddChild(idxNode, left)
			idxNode = AddChild(idxNode, indexExpr)
			if peekTokenType(tokens, pos) == TokenRBracket {
				pos = pos + 1
			}
			left = idxNode
			continue
		}

		// Composite literal: Type{...}
		if tt == TokenLBrace {
			// Only parse as composite literal if left is a type-like expression
			if isTypeLikeNode(left) {
				var litNode Node
				litNode, pos = parseCompositeLitBody(tokens, pos, left)
				left = litNode
				continue
			}
			break
		}

		break
	}

	return left, pos
}

// isTypeLikeNode checks if a node looks like a compound type (for composite literal detection)
// Note: bare Name and Selector are NOT included here to avoid ambiguity with
// switch/if/for bodies. Use parseExpressionWithCompositeLit for contexts
// where Name{...} composite literals are valid.
func isTypeLikeNode(node Node) bool {
	if node.Type == NodeSliceType {
		return true
	}
	if node.Type == NodeArrayType {
		return true
	}
	if node.Type == NodeMapType {
		return true
	}
	return false
}

// parseExpressionWithCompositeLit parses an expression and then checks if
// it should be followed by a composite literal body (Name{...} or Selector{...}).
// Use this in contexts where composite literals are valid (RHS of assignments,
// function arguments, return values, etc.) but NOT in control flow conditions.
func parseExpressionWithCompositeLit(tokens []Token, pos int) (Node, int) {
	var expr Node
	expr, pos = parseExpression(tokens, pos)
	if (expr.Type == NodeName || expr.Type == NodeSelector) && peekTokenType(tokens, pos) == TokenLBrace {
		expr, pos = parseCompositeLitBody(tokens, pos, expr)
	}
	return expr, pos
}

// hasRangeKeyword scans ahead from pos to check if there's a 'range' keyword
// before the body brace '{' (at nesting depth 0)
func hasRangeKeyword(tokens []Token, pos int) bool {
	depth := 0
	for i := pos; i < len(tokens); i++ {
		tt := tokens[i].Type
		if tt == TokenLParen || tt == TokenLBracket {
			depth = depth + 1
		} else if tt == TokenRParen || tt == TokenRBracket {
			depth = depth - 1
		} else if tt == TokenLBrace && depth == 0 {
			return false
		} else if tt == TokenKeyword && tokens[i].Value == "range" && depth == 0 {
			return true
		}
	}
	return false
}

// parseCompositeLitBody parses the body of a composite literal: { key: value, ... }
func parseCompositeLitBody(tokens []Token, pos int, typeNode Node) (Node, int) {
	litNode := NewNode(NodeCompositeLit)
	litNode = SetLine(litNode, typeNode.Line)
	litNode = AddChild(litNode, typeNode)

	pos = pos + 1 // skip '{'

	for peekTokenType(tokens, pos) != TokenRBrace && peekTokenType(tokens, pos) != TokenEOF {
		// Skip semicolons (from newlines)
		for peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
		if peekTokenType(tokens, pos) == TokenRBrace {
			break
		}

		// Handle anonymous nested composite literal: {val, val}
		var elem Node
		if peekTokenType(tokens, pos) == TokenLBrace {
			elem, pos = parseCompositeLitBody(tokens, pos, NewNode(NodeName))
		} else {
			elem, pos = parseExpressionWithCompositeLit(tokens, pos)
		}

		if peekTokenType(tokens, pos) == TokenColon {
			// Key-value pair
			pos = pos + 1
			kv := NewNode(NodeKeyValue)
			kv = SetLine(kv, elem.Line)
			kv = AddChild(kv, elem)
			var val Node
			if peekTokenType(tokens, pos) == TokenLBrace {
				val, pos = parseCompositeLitBody(tokens, pos, NewNode(NodeName))
			} else {
				val, pos = parseExpressionWithCompositeLit(tokens, pos)
			}
			kv = AddChild(kv, val)
			litNode = AddChild(litNode, kv)
		} else {
			litNode = AddChild(litNode, elem)
		}

		if peekTokenType(tokens, pos) == TokenComma {
			pos = pos + 1
		}
		// Skip semicolons
		for peekTokenType(tokens, pos) == TokenSemicolon {
			pos = pos + 1
		}
	}

	if peekTokenType(tokens, pos) == TokenRBrace {
		pos = pos + 1
	}

	return litNode, pos
}

// parseAtom parses atomic expressions: literals, identifiers, parenthesized, composite types
func parseAtom(tokens []Token, pos int) (Node, int) {
	tt := peekTokenType(tokens, pos)
	tv := peekTokenValue(tokens, pos)
	var node Node

	// Number literal
	if tt == TokenNumber {
		node = NewNodeWithValue(NodeNum, tv)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		return node, pos
	}

	// String literal
	if tt == TokenString {
		node = NewNodeWithValue(NodeStr, tv)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		return node, pos
	}

	// Rune literal
	if tt == TokenRune {
		node = NewNodeWithValue(NodeRuneLit, tv)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		return node, pos
	}

	// Identifier (including true, false, nil, iota)
	if tt == TokenIdentifier {
		if tv == "true" || tv == "false" {
			node = NewNodeWithValue(NodeBool, tv)
			node = SetLine(node, peekToken(tokens, pos).Line)
			pos = pos + 1
			return node, pos
		}
		if tv == "nil" {
			node = NewNode(NodeNil)
			node = SetLine(node, peekToken(tokens, pos).Line)
			pos = pos + 1
			return node, pos
		}
		if tv == "iota" {
			node = NewNode(NodeIota)
			node = SetLine(node, peekToken(tokens, pos).Line)
			pos = pos + 1
			return node, pos
		}
		if tv == "_" {
			node = NewNode(NodeBlankIdent)
			node = SetLine(node, peekToken(tokens, pos).Line)
			pos = pos + 1
			return node, pos
		}
		node = NewNodeWithName(NodeName, tv)
		node = SetLine(node, peekToken(tokens, pos).Line)
		pos = pos + 1
		return node, pos
	}

	// Parenthesized expression
	if tt == TokenLParen {
		pos = pos + 1 // skip '('
		var expr Node
		expr, pos = parseExpression(tokens, pos)
		if peekTokenType(tokens, pos) == TokenRParen {
			pos = pos + 1
		}
		return expr, pos
	}

	// Slice type followed by composite literal: []Type{...}
	if tt == TokenLBracket {
		savedPos := pos
		pos = pos + 1 // skip '['
		if peekTokenType(tokens, pos) == TokenRBracket {
			// []Type
			pos = pos + 1
			var elemType Node
			elemType, pos = parseTypeExpr(tokens, pos)
			sliceType := NewNode(NodeSliceType)
			sliceType = AddChild(sliceType, elemType)
			// Check for composite literal
			if peekTokenType(tokens, pos) == TokenLBrace {
				var litNode Node
				litNode, pos = parseCompositeLitBody(tokens, pos, sliceType)
				return litNode, pos
			}
			return sliceType, pos
		}
		// Array type: [N]Type or just subscript - restore
		pos = savedPos
		// If we get here, it's not a type expr atom, fall through
	}

	// Map type: map[K]V{...}
	if tt == TokenKeyword && tv == "map" {
		var mapType Node
		mapType, pos = parseTypeExpr(tokens, pos)
		if peekTokenType(tokens, pos) == TokenLBrace {
			var litNode Node
			litNode, pos = parseCompositeLitBody(tokens, pos, mapType)
			return litNode, pos
		}
		return mapType, pos
	}

	// func literal: func(plist) retTypes { body }
	if tt == TokenKeyword && tv == "func" {
		pos = pos + 1
		funcNode := NewNode(NodeFuncType)
		funcNode = SetLine(funcNode, peekToken(tokens, pos-1).Line)
		var plist Node
		plist, pos = parseParamList(tokens, pos)
		funcNode = AddChild(funcNode, plist)
		var results Node
		results, pos = parseResultTypes(tokens, pos)
		if len(results.Children) > 0 {
			funcNode = AddChild(funcNode, results)
		}
		if peekTokenType(tokens, pos) == TokenLBrace {
			var body Node
			body, pos = parseBlock(tokens, pos)
			funcNode = AddChild(funcNode, body)
		}
		return funcNode, pos
	}

	// struct type (for inline struct{} or struct{...}{...})
	if tt == TokenKeyword && tv == "struct" {
		var structType Node
		structType, pos = parseStructType(tokens, pos)
		if peekTokenType(tokens, pos) == TokenLBrace {
			var litNode Node
			litNode, pos = parseCompositeLitBody(tokens, pos, structType)
			return litNode, pos
		}
		return structType, pos
	}

	// interface type
	if tt == TokenKeyword && tv == "interface" {
		return parseInterfaceType(tokens, pos)
	}

	// Channel type used as expression (e.g., make(chan int))
	if tt == TokenKeyword && tv == "chan" {
		return parseTypeExpr(tokens, pos)
	}

	// Fallback: skip unknown token
	node = NewNode(NodeName)
	node = SetLine(node, peekToken(tokens, pos).Line)
	if pos < len(tokens) {
		pos = pos + 1
	}
	return node, pos
}
