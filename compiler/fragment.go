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


// Push adds a token to the GoFIR's tokenSlice with the given code, tag, and Go type.
func (fs *FragmentStack) Push(code string, tag int, goType types.Type) {
	token := Token{Content: code, Tag: tag, GoType: goType}
	fs.gir.emitTokenToFileBufferString(token, "__JS_PUSH")
}

// PushCode is a convenience method that pushes code with TagExpr and nil GoType.
func (fs *FragmentStack) PushCode(code string) {
	fs.Push(code, TagExpr, nil)
}

// PushCodeWithType is a convenience method that pushes code with TagExpr and a Go type.
func (fs *FragmentStack) PushCodeWithType(code string, t types.Type) {
	fs.Push(code, TagExpr, t)
}

// Reduce finds the last marker matching the given visitMethod string, extracts all tokens
// from that position to the end, trims both tokenSlice and pointerAndIndexVec,
// and returns the extracted tokens.
func (fs *FragmentStack) Reduce(visitMethod string) []Token {
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
	var result []Token
	if tokenIdx < len(fs.gir.tokenSlice) {
		result = make([]Token, len(fs.gir.tokenSlice)-tokenIdx)
		copy(result, fs.gir.tokenSlice[tokenIdx:])
	}
	// Trim tokenSlice
	fs.gir.tokenSlice = fs.gir.tokenSlice[:tokenIdx]
	// Remove all PIV entries from marker position onwards
	fs.gir.pointerAndIndexVec = fs.gir.pointerAndIndexVec[:pivIdx]
	return result
}

// ReduceToCode performs Reduce and concatenates all token Content fields.
func (fs *FragmentStack) ReduceToCode(visitMethod string) string {
	tokens := fs.Reduce(visitMethod)
	var sb strings.Builder
	for _, t := range tokens {
		sb.WriteString(t.Content)
	}
	return sb.String()
}

// Pop removes and returns the last token from the tokenSlice.
func (fs *FragmentStack) Pop() Token {
	if len(fs.gir.tokenSlice) == 0 {
		return Token{}
	}
	last := fs.gir.tokenSlice[len(fs.gir.tokenSlice)-1]
	fs.gir.tokenSlice = fs.gir.tokenSlice[:len(fs.gir.tokenSlice)-1]
	return last
}

// Peek returns the last token without removing it.
func (fs *FragmentStack) Peek() Token {
	if len(fs.gir.tokenSlice) == 0 {
		return Token{}
	}
	return fs.gir.tokenSlice[len(fs.gir.tokenSlice)-1]
}

// Len returns the current number of tokens in the stack.
func (fs *FragmentStack) Len() int {
	return len(fs.gir.tokenSlice)
}
