package compiler

import (
	"go/types"
	"strings"
)

// Fragment tags — what produced this fragment
const (
	TagMarker  int = 0
	TagExpr    int = 1
	TagStmt    int = 2
	TagType    int = 3
	TagIdent   int = 4
	TagLiteral int = 5
)

// FragmentStack provides a clean Push/Pop/Reduce API on top of GoFIR's storage.
type FragmentStack struct {
	gir *GoFIR
}

// NewFragmentStack creates a new FragmentStack wrapping the given GoFIR instance.
func NewFragmentStack(gir *GoFIR) *FragmentStack {
	return &FragmentStack{gir: gir}
}

// PushMarker pushes a named marker onto the stack (for Reduce to find later).
func (fs *FragmentStack) PushMarker(name string) {
	fs.gir.emitToFileBufferString("", name)
}

// Push adds a token to the GoFIR's tokenSlice with the given code, tag, and Go type.
func (fs *FragmentStack) Push(code string, tag int, goType types.Type) {
	token := IRNode{Content: code, Tag: tag, GoType: goType}
	fs.gir.emitTokenToFileBufferString(token, "__PUSH")
}

// PushCode is a convenience method that pushes code with TagExpr and nil GoType.
func (fs *FragmentStack) PushCode(code string) {
	fs.Push(code, TagExpr, nil)
}

// PushCodeWithType is a convenience method that pushes code with TagExpr and a Go type.
func (fs *FragmentStack) PushCodeWithType(code string, t types.Type) {
	fs.Push(code, TagExpr, t)
}

// PushWithMeta adds a token with optimization metadata to the stack.
func (fs *FragmentStack) PushWithMeta(code string, tag int, meta *OptMeta) {
	token := IRNode{Content: code, Tag: tag, OptMeta: meta}
	fs.gir.emitTokenToFileBufferString(token, "__PUSH")
}

// Reduce finds the last marker matching the given visitMethod string, extracts all tokens
// from that position to the end, trims both tokenSlice and pointerAndIndexVec,
// and returns the extracted tokens.
func (fs *FragmentStack) Reduce(visitMethod string) []IRNode {
	target := visitMethod
	// Find marker in pointerAndIndexVec (searching from end)
	pivIdx := -1
	for i := len(fs.gir.pointerAndIndexVec) - 1; i >= 0; i-- {
		if fs.gir.pointerAndIndexVec[i].Pointer == target {
			pivIdx = i
			break
		}
	}
	if pivIdx < 0 {
		return nil
	}
	tokenIdx := fs.gir.pointerAndIndexVec[pivIdx].Index
	// Extract tokens from marker position to end
	var result []IRNode
	if tokenIdx < len(fs.gir.tokenSlice) {
		result = make([]IRNode, len(fs.gir.tokenSlice)-tokenIdx)
		copy(result, fs.gir.tokenSlice[tokenIdx:])
	}
	// Trim tokenSlice
	fs.gir.tokenSlice = fs.gir.tokenSlice[:tokenIdx]
	// Remove all PIV entries from marker position onwards
	fs.gir.pointerAndIndexVec = fs.gir.pointerAndIndexVec[:pivIdx]
	return result
}

// ReduceToCode performs Reduce and concatenates all token Serialize() outputs.
func (fs *FragmentStack) ReduceToCode(visitMethod string) string {
	tokens := fs.Reduce(visitMethod)
	var sb strings.Builder
	for _, t := range tokens {
		sb.WriteString(t.Serialize())
	}
	return sb.String()
}

// PushTree pushes a tree token onto the stack (auto-computes Content for backward compat).
func (fs *FragmentStack) PushTree(token IRNode) {
	// Ensure Content is populated for backward compatibility
	if len(token.Children) > 0 && token.Content == "" {
		token.Content = token.Serialize()
	}
	fs.gir.emitTokenToFileBufferString(token, "__PUSH")
}

// PushLeaf pushes an atomic leaf token onto the stack.
func (fs *FragmentStack) PushLeaf(tokenType IRNodeType, content string, tag int) {
	token := LeafTag(tokenType, content, tag)
	fs.gir.emitTokenToFileBufferString(token, "__PUSH")
}

// ReduceToTree reduces tokens from the marker and wraps them as children of a new parent token.
func (fs *FragmentStack) ReduceToTree(visitMethod string, tokenType IRNodeType, tag int) IRNode {
	children := fs.Reduce(visitMethod)
	return IRTree(tokenType, tag, children...)
}

// Pop removes and returns the last token from the tokenSlice.
func (fs *FragmentStack) Pop() IRNode {
	if len(fs.gir.tokenSlice) == 0 {
		return IRNode{}
	}
	last := fs.gir.tokenSlice[len(fs.gir.tokenSlice)-1]
	fs.gir.tokenSlice = fs.gir.tokenSlice[:len(fs.gir.tokenSlice)-1]
	return last
}

// Peek returns the last token without removing it.
func (fs *FragmentStack) Peek() IRNode {
	if len(fs.gir.tokenSlice) == 0 {
		return IRNode{}
	}
	return fs.gir.tokenSlice[len(fs.gir.tokenSlice)-1]
}

// Len returns the current number of tokens in the stack.
func (fs *FragmentStack) Len() int {
	return len(fs.gir.tokenSlice)
}
