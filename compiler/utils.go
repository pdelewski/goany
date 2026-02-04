package compiler

import (
	"errors"
	"fmt"
	"go/ast"
	"os"
	"strings"
	"unicode"
)

// exprToString converts an ast.Expr to its string representation.
// Handles *ast.Ident (e.g., "m") and *ast.SelectorExpr (e.g., "s.Settings").
func exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return exprToString(e.X) + "." + e.Sel.Name
	default:
		return ""
	}
}

// TopologicalSort performs a topological sort on the given graph.
// The input graph is a map where keys are nodes and values are slices of their dependencies.
func TopologicalSort(graph map[string][]string) ([]string, error) {
	// Track the state of each node: 0 = unvisited, 1 = visiting, 2 = visited
	visited := make(map[string]int)
	result := []string{}

	// Helper function for depth-first search (DFS)
	var visit func(string) error
	visit = func(node string) error {
		state := visited[node]

		// If the node is already visited, return
		if state == 2 {
			return nil
		}
		// If we find a node in "visiting" state, there is a cycle
		if state == 1 {
			return errors.New("cycle detected in the graph")
		}

		// Mark the node as visiting
		visited[node] = 1

		// Visit all the dependencies of the current node, if any
		if deps, exists := graph[node]; exists {
			for _, dep := range deps {
				if err := visit(dep); err != nil {
					return err // propagate the cycle detection error
				}
			}
		}

		// Mark the node as visited and add it to the result
		visited[node] = 2
		result = append(result, node)

		return nil
	}

	// Visit all nodes in the graph (including those without dependencies)
	for node := range graph {
		if visited[node] == 0 {
			if err := visit(node); err != nil {
				return nil, err
			}
		}
	}

	// Ensure we include nodes without outgoing edges
	// For example, in a graph {A -> B}, if C has no dependencies, it should also be in the result.
	for node := range visited {
		if visited[node] == 0 {
			if err := visit(node); err != nil {
				return nil, err
			}
		}
	}

	// Reverse the result because nodes are added in post-order
	reverse(result)

	return result, nil
}

// reverse reverses a slice of strings in place
func reverse(arr []string) {
	for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
		arr[i], arr[j] = arr[j], arr[i]
	}
}

func SliceToMap(slice []string) map[string]int {
	// Create a map to store the string and its index
	result := make(map[string]int)

	// Loop over the slice and fill the map
	for index, value := range slice {
		result[value] = index
	}

	return result
}

func mergeStackElements(marker string, stack []string) []string {
	var merged strings.Builder

	// Process the stack in reverse until we find a marker
	for len(stack) > 0 {
		top := stack[len(stack)-1]
		stack = stack[:len(stack)-1] // Pop element

		// Stop merging when we find a marker
		if strings.HasPrefix(top, marker) {
			stack = append(stack, merged.String()) // Push merged string
			return stack
		}

		// Prepend the element to the merged string (reverse order)
		mergedString := top + merged.String() // Prepend instead of append
		merged.Reset()
		merged.WriteString(mergedString)
	}
	return stack
}

func SearchPointerIndexReverse(target VisitMethod, pointerAndIndexVec []PointerAndIndex) *PointerAndIndex {
	for i := len(pointerAndIndexVec) - 1; i >= 0; i-- {
		if pointerAndIndexVec[i].Pointer == string(target) {
			return &pointerAndIndexVec[i]
		}
	}
	return nil // Return nil if the pointer is not found
}

// SearchPointerIndexReverseString provides string-based search for custom markers and dynamic values
func SearchPointerIndexReverseString(target string, pointerAndIndexVec []PointerAndIndex) *PointerAndIndex {
	for i := len(pointerAndIndexVec) - 1; i >= 0; i-- {
		if pointerAndIndexVec[i].Pointer == target {
			return &pointerAndIndexVec[i]
		}
	}
	return nil // Return nil if the pointer is not found
}

// RemovePointerEntryReverse removes the last entry matching the target and returns the new slice
func RemovePointerEntryReverse(pointerAndIndexVec []PointerAndIndex, target VisitMethod) []PointerAndIndex {
	for i := len(pointerAndIndexVec) - 1; i >= 0; i-- {
		if pointerAndIndexVec[i].Pointer == string(target) {
			return append(pointerAndIndexVec[:i], pointerAndIndexVec[i+1:]...)
		}
	}
	return pointerAndIndexVec
}

// RemovePointerEntryReverseString removes the last entry matching the target string and returns the new slice
func RemovePointerEntryReverseString(pointerAndIndexVec []PointerAndIndex, target string) []PointerAndIndex {
	for i := len(pointerAndIndexVec) - 1; i >= 0; i-- {
		if pointerAndIndexVec[i].Pointer == target {
			return append(pointerAndIndexVec[:i], pointerAndIndexVec[i+1:]...)
		}
	}
	return pointerAndIndexVec
}

// New token-based functions
func ExtractTokensNew(position int, tokenSlice []Token) ([]Token, error) {
	if position < 0 || position >= len(tokenSlice) {
		return nil, fmt.Errorf("position %d is out of bounds", position)
	}
	return tokenSlice[position:], nil
}

// Backward compatibility for string-based ExtractTokens
func ExtractTokens(position int, tokenSlice []Token) ([]string, error) {
	if position < 0 || position >= len(tokenSlice) {
		return nil, fmt.Errorf("position %d is out of bounds", position)
	}
	result := make([]string, len(tokenSlice)-position)
	for i, token := range tokenSlice[position:] {
		result[i] = token.Content
	}
	return result, nil
}

func ExtractTokensBetween(begin int, end int, tokenSlice []Token) ([]Token, error) {
	if begin < 0 || end > len(tokenSlice) || begin > end {
		return nil, fmt.Errorf("invalid range: begin %d, end %d", begin, end)
	}
	return tokenSlice[begin:end], nil
}

// New token-based functions
func RewriteTokensBetweenNew(tokenSlice []Token, begin int, end int, content []Token) ([]Token, error) {
	if begin < 0 || end > len(tokenSlice) || begin > end {
		return tokenSlice, fmt.Errorf("invalid range: begin %d, end %d", begin, end)
	}
	result := make([]Token, 0, begin+len(content)+(len(tokenSlice)-end))
	result = append(result, tokenSlice[:begin]...)
	result = append(result, content...)
	result = append(result, tokenSlice[end:]...)
	return result, nil
}

// Backward compatibility for string-based RewriteTokensBetween
func RewriteTokensBetween(tokenSlice []Token, begin int, end int, content []string) ([]Token, error) {
	if begin < 0 || end > len(tokenSlice) || begin > end {
		return tokenSlice, fmt.Errorf("invalid range: begin %d, end %d", begin, end)
	}
	// Convert strings to tokens
	tokenContent := make([]Token, len(content))
	for i, s := range content {
		tokenContent[i] = CreateToken(Identifier, s)
	}
	result := make([]Token, 0, begin+len(tokenContent)+(len(tokenSlice)-end))
	result = append(result, tokenSlice[:begin]...)
	result = append(result, tokenContent...)
	result = append(result, tokenSlice[end:]...)
	return result, nil
}

// Backward compatibility for string-based RewriteTokens
func RewriteTokens(tokenSlice []Token, position int, oldContent, newContent []string) ([]Token, error) {
	if position < 0 || position+len(oldContent) > len(tokenSlice) {
		return tokenSlice, fmt.Errorf("position %d is out of bounds or oldContent does not match", position)
	}
	for i, content := range oldContent {
		if position+i >= len(tokenSlice) || tokenSlice[position+i].Content != content {
			return tokenSlice, fmt.Errorf("oldContent does not match the existing content at position %d", position)
		}
	}
	// Convert strings to tokens
	tokenNewContent := make([]Token, len(newContent))
	for i, s := range newContent {
		tokenNewContent[i] = CreateToken(Identifier, s)
	}
	result := make([]Token, 0, len(tokenSlice)-len(oldContent)+len(tokenNewContent))
	result = append(result, tokenSlice[:position]...)
	result = append(result, tokenNewContent...)
	result = append(result, tokenSlice[position+len(oldContent):]...)
	return result, nil
}

type PointerAndIndex struct {
	Pointer string // Pointer to the visit method type (keeping as string for flexibility)
	Index   int
}

// New Token-based methods
func (gir *GoFIR) emitTokenToFileBuffer(
	token Token, pointer VisitMethod) error {
	gir.pointerAndIndexVec = append(gir.pointerAndIndexVec, PointerAndIndex{
		Pointer: string(pointer),
		Index:   len(gir.tokenSlice),
	})
	if token.Content != "" {
		gir.tokenSlice = append(gir.tokenSlice, token)
	}
	return nil
}

func (gir *GoFIR) emitTokenToFileBufferString(
	token Token, pointer string) error {
	gir.pointerAndIndexVec = append(gir.pointerAndIndexVec, PointerAndIndex{
		Pointer: pointer,
		Index:   len(gir.tokenSlice),
	})
	if token.Content != "" {
		gir.tokenSlice = append(gir.tokenSlice, token)
	}
	return nil
}

// Backward compatibility - keep old string-based methods
func (gir *GoFIR) emitToFileBuffer(
	s string, pointer VisitMethod) error {
	gir.pointerAndIndexVec = append(gir.pointerAndIndexVec, PointerAndIndex{
		Pointer: string(pointer),
		Index:   len(gir.tokenSlice),
	})
	if s != "" {
		// Convert string to token for storage
		token := CreateToken(Identifier, s) // Default token type
		gir.tokenSlice = append(gir.tokenSlice, token)
	}
	return nil
}

// emitToFileBufferString provides backward compatibility for string pointers
func (gir *GoFIR) emitToFileBufferString(
	s string, pointer string) error {
	gir.pointerAndIndexVec = append(gir.pointerAndIndexVec, PointerAndIndex{
		Pointer: pointer,
		Index:   len(gir.tokenSlice),
	})
	if s != "" {
		// Convert string to token for storage
		token := CreateToken(Identifier, s) // Default token type
		gir.tokenSlice = append(gir.tokenSlice, token)
	}
	return nil
}

func emitToFile(file *os.File, fileBuffer string) error {
	_, err := file.WriteString(fileBuffer)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return err
	}
	return nil
}

func emitTokensToFile(file *os.File, tokenSlice []Token) error {
	for _, token := range tokenSlice {
		_, err := file.WriteString(token.Content)
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return err
		}
	}
	return nil
}

func RebuildNestedType(reprs []AliasRepr) string {
	if len(reprs) == 0 {
		return ""
	}

	// Start from the innermost type
	result := formatAlias(reprs[len(reprs)-1])
	for i := len(reprs) - 2; i >= 0; i-- {
		result = fmt.Sprintf("%s<%s>", formatAlias(reprs[i]), result)
	}
	return result
}

func formatAlias(r AliasRepr) string {
	if r.PackageName != "" {
		return r.PackageName + "." + r.TypeName
	}
	return r.TypeName
}

func containsWhitespace(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) {
			return true
		}
	}
	return false
}

type GoFIR struct {
	tokenSlice         []Token
	pointerAndIndexVec []PointerAndIndex
}

// RemoveTokenAt removes the token at the specified index from the tokenSlice
func RemoveTokenAt(tokenSlice []Token, index int) ([]Token, error) {
	if index < 0 || index >= len(tokenSlice) {
		return tokenSlice, fmt.Errorf("index %d is out of bounds for tokenSlice of length %d", index, len(tokenSlice))
	}

	result := make([]Token, 0, len(tokenSlice)-1)
	result = append(result, tokenSlice[:index]...)
	result = append(result, tokenSlice[index+1:]...)

	return result, nil
}

// Helper function to emit string as Token (backward compatibility)
func (gir *GoFIR) emitStringAsToken(s string, tokenType TokenType, pointer VisitMethod) error {
	token := CreateToken(tokenType, s)
	return gir.emitTokenToFileBuffer(token, pointer)
}
