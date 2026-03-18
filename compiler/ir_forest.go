package compiler

import (
	"go/types"
	"strings"
)

// Aliases: map old semantic tag names to NodeKind for migration.
var (
	TagExpr    = KindExpr
	TagStmt    = KindStmt
	TagType    = KindType
	TagIdent   = KindIdent
	TagLiteral = KindLiteral
)

// PointerAndIndex tracks a named marker position within the forest.
type PointerAndIndex struct {
	Pointer string
	Index   int
}

// IRForestBuilder builds a forest of IR trees during emission.
type IRForestBuilder struct {
	forest  []IRNode
	markers []PointerAndIndex
}

// NewIRForestBuilder creates a new IRForestBuilder.
func NewIRForestBuilder() *IRForestBuilder {
	return &IRForestBuilder{}
}

// appendNode adds a token node with a string marker.
func (fb *IRForestBuilder) appendNode(token IRNode, pointer string) {
	fb.markers = append(fb.markers, PointerAndIndex{
		Pointer: pointer,
		Index:   len(fb.forest),
	})
	if token.Content != "" {
		fb.forest = append(fb.forest, token)
	}
}

// appendStringNode adds a string as an Identifier node with a string marker.
func (fb *IRForestBuilder) appendStringNode(s string, pointer string) {
	fb.markers = append(fb.markers, PointerAndIndex{
		Pointer: pointer,
		Index:   len(fb.forest),
	})
	if s != "" {
		token := CreateIRNode(Identifier, s)
		fb.forest = append(fb.forest, token)
	}
}

// AddVisitMarker adds a visit method marker to the forest.
func (fb *IRForestBuilder) AddVisitMarker(pointer VisitMethod) {
	fb.markers = append(fb.markers, PointerAndIndex{
		Pointer: string(pointer),
		Index:   len(fb.forest),
	})
}

// AddMarker adds a named marker to the forest (for CollectForest to find later).
func (fb *IRForestBuilder) AddMarker(name string) {
	fb.appendStringNode("", name)
}

// AddLeaf adds a leaf node to the forest with the given code, kind, and Go type.
func (fb *IRForestBuilder) AddLeaf(code string, kind NodeKind, goType types.Type) {
	token := IRNode{Content: code, Kind: kind, GoType: goType}
	fb.appendNode(token, "__PUSH")
}

// CollectForest finds the last marker matching the given visitMethod string, extracts all nodes
// from that position to the end, trims both forest and markers,
// and returns the extracted nodes.
func (fb *IRForestBuilder) CollectForest(visitMethod string) []IRNode {
	target := visitMethod
	// Find marker in markers (searching from end)
	pivIdx := -1
	for i := len(fb.markers) - 1; i >= 0; i-- {
		if fb.markers[i].Pointer == target {
			pivIdx = i
			break
		}
	}
	if pivIdx < 0 {
		return nil
	}
	tokenIdx := fb.markers[pivIdx].Index
	// Extract tokens from marker position to end
	var result []IRNode
	if tokenIdx < len(fb.forest) {
		result = make([]IRNode, len(fb.forest)-tokenIdx)
		copy(result, fb.forest[tokenIdx:])
	}
	// Trim forest
	fb.forest = fb.forest[:tokenIdx]
	// Remove all marker entries from marker position onwards
	fb.markers = fb.markers[:pivIdx]
	return result
}

// CollectText performs CollectForest and concatenates all node Serialize() outputs.
func (fb *IRForestBuilder) CollectText(visitMethod string) string {
	tokens := fb.CollectForest(visitMethod)
	var sb strings.Builder
	for _, t := range tokens {
		sb.WriteString(t.Serialize())
	}
	return sb.String()
}

// AddTree adds a tree node to the forest (auto-computes Content for backward compat).
func (fb *IRForestBuilder) AddTree(token IRNode) {
	// Ensure Content is populated for backward compatibility
	if len(token.Children) > 0 && token.Content == "" {
		token.Content = token.Serialize()
	}
	fb.appendNode(token, "__PUSH")
}
