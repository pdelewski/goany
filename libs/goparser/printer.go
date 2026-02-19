package goparser

// PrintAST prints the AST as a string
func PrintAST(node Node) string {
	return PrintASTIndented(node, 0)
}

// PrintASTIndented prints the AST with indentation
func PrintASTIndented(node Node, indent int) string {
	result := ""

	// Add indentation
	for i := 0; i < indent; i++ {
		result = result + "  "
	}

	// Print node type
	result = result + NodeTypeName(node.Type)

	// Print name if present
	if node.Name != "" {
		result = result + " name=" + node.Name
	}

	// Print value if present
	if node.Value != "" {
		result = result + " value=" + node.Value
	}

	// Print operator if present
	if node.Op != "" {
		result = result + " op=" + node.Op
	}

	result = result + "\n"

	// Print children
	for i := 0; i < len(node.Children); i++ {
		result = result + PrintASTIndented(node.Children[i], indent+1)
	}

	return result
}

// PrintTokens prints tokens for debugging
func PrintTokens(tokens []Token) string {
	result := ""
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		result = result + TokenTypeName(token.Type)
		if token.Value != "" {
			result = result + "(" + token.Value + ")"
		}
		result = result + " "
	}
	result = result + "\n"
	return result
}
