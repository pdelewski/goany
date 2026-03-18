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

// IRForestBuilder builds a forest of IR trees during emission.
type IRForestBuilder struct {
	gir *GoFIR
}

// NewIRForestBuilder creates a new IRForestBuilder wrapping the given GoFIR instance.
func NewIRForestBuilder(gir *GoFIR) *IRForestBuilder {
	return &IRForestBuilder{gir: gir}
}

// AddMarker adds a named marker to the forest (for CollectForest to find later).
func (fb *IRForestBuilder) AddMarker(name string) {
	fb.gir.emitToFileBufferString("", name)
}

// AddLeaf adds a leaf node to the forest with the given code, kind, and Go type.
func (fb *IRForestBuilder) AddLeaf(code string, kind NodeKind, goType types.Type) {
	token := IRNode{Content: code, Kind: kind, GoType: goType}
	fb.gir.emitTokenToFileBufferString(token, "__PUSH")
}

// CollectForest finds the last marker matching the given visitMethod string, extracts all nodes
// from that position to the end, trims both tokenSlice and pointerAndIndexVec,
// and returns the extracted nodes.
func (fb *IRForestBuilder) CollectForest(visitMethod string) []IRNode {
	target := visitMethod
	// Find marker in pointerAndIndexVec (searching from end)
	pivIdx := -1
	for i := len(fb.gir.pointerAndIndexVec) - 1; i >= 0; i-- {
		if fb.gir.pointerAndIndexVec[i].Pointer == target {
			pivIdx = i
			break
		}
	}
	if pivIdx < 0 {
		return nil
	}
	tokenIdx := fb.gir.pointerAndIndexVec[pivIdx].Index
	// Extract tokens from marker position to end
	var result []IRNode
	if tokenIdx < len(fb.gir.tokenSlice) {
		result = make([]IRNode, len(fb.gir.tokenSlice)-tokenIdx)
		copy(result, fb.gir.tokenSlice[tokenIdx:])
	}
	// Trim tokenSlice
	fb.gir.tokenSlice = fb.gir.tokenSlice[:tokenIdx]
	// Remove all PIV entries from marker position onwards
	fb.gir.pointerAndIndexVec = fb.gir.pointerAndIndexVec[:pivIdx]
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
	fb.gir.emitTokenToFileBufferString(token, "__PUSH")
}
