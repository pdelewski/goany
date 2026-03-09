package rustparser

// PrintAST prints the AST as a string
func PrintAST(node Node) string {
	return PrintASTIndented(node, 0)
}

// PrintASTIndented prints the AST with indentation
func PrintASTIndented(node Node, indent int) string {
	result := ""

	// Add indentation
	i := 0
	for i < indent {
		result += "  "
		i = i + 1
	}

	// Print node type
	result += NodeTypeName(node.Type)

	// Print name if present
	if node.Name != "" {
		result += " name="
		result += node.Name
	}

	// Print value if present
	if node.Value != "" {
		result += " value="
		result += node.Value
	}

	// Print operator if present
	if node.Op != "" {
		result += " op="
		result += node.Op
	}

	result += "\n"

	// Print children
	i = 0
	for i < len(node.Children) {
		result += PrintASTIndented(node.Children[i], indent+1)
		i = i + 1
	}

	return result
}
