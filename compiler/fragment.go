package compiler

import (
	"fmt"
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

// Marker constants — which construct this marker belongs to
const (
	MarkerFunc              int = 1
	MarkerFuncSig           int = 2
	MarkerFuncSigParams     int = 3
	MarkerFuncSigResults    int = 4
	MarkerBlock             int = 5
	MarkerAssign            int = 6
	MarkerAssignLhs         int = 7
	MarkerAssignRhs         int = 8
	MarkerReturn            int = 9
	MarkerIf                int = 10
	MarkerIfCond            int = 11
	MarkerIfBody            int = 12
	MarkerIfElse            int = 13
	MarkerFor               int = 14
	MarkerForInit           int = 15
	MarkerForCond           int = 16
	MarkerForPost           int = 17
	MarkerRange             int = 18
	MarkerSwitch            int = 19
	MarkerCase              int = 20
	MarkerCall              int = 21
	MarkerCallArgs          int = 22
	MarkerBinOp             int = 23
	MarkerUnaryOp           int = 24
	MarkerIndex             int = 25
	MarkerSelector          int = 26
	MarkerParen             int = 27
	MarkerExprStmt          int = 28
	MarkerDecl              int = 29
	MarkerCompositeLit      int = 30
	MarkerSliceExpr         int = 31
	MarkerIncDec            int = 32
	MarkerProgram           int = 33
	MarkerPackage           int = 34
	MarkerFuncBody          int = 35
	MarkerCallFun           int = 36
	MarkerCallArg           int = 37
	MarkerBinOpLeft         int = 38
	MarkerBinOpRight        int = 39
	MarkerAssignLhsExpr     int = 40
	MarkerAssignRhsExpr     int = 41
	MarkerSelectorX         int = 42
	MarkerSelectorSel       int = 43
	MarkerIndexX            int = 44
	MarkerIndexIdx          int = 45
	MarkerBlockItem         int = 46
	MarkerReturnResult      int = 47
	MarkerDeclType          int = 48
	MarkerDeclName          int = 49
	MarkerDeclValue         int = 50
	MarkerIfInit            int = 51
	MarkerForBody           int = 52
	MarkerRangeKey          int = 53
	MarkerRangeValue        int = 54
	MarkerRangeX            int = 55
	MarkerCaseList          int = 56
	MarkerCaseListExpr      int = 57
	MarkerSwitchTag         int = 58
	MarkerCompositeLitType  int = 59
	MarkerCompositeLitElts  int = 60
	MarkerCompositeLitElt   int = 61
	MarkerSliceExprX        int = 62
	MarkerSliceExprLow      int = 63
	MarkerSliceExprHigh     int = 64
	MarkerSliceExprXBegin   int = 65
	MarkerSliceExprXEnd     int = 66
	MarkerExprStmtX         int = 67
	MarkerGenStructInfos    int = 68
	MarkerGenStructInfo     int = 69
	MarkerGenStructField    int = 70
	MarkerGenStructFieldName int = 71
	MarkerGenDeclConst      int = 72
	MarkerGenDeclConstName  int = 73
	MarkerFuncSigParamsList int = 74
	MarkerFuncSigParamType  int = 75
	MarkerFuncSigParamName  int = 76
	MarkerFuncDeclName      int = 77
	MarkerFuncSigResultsList int = 78
	MarkerFuncLit           int = 79
	MarkerFuncLitParams     int = 80
	MarkerFuncLitResults    int = 81
	MarkerFuncLitBody       int = 82
	MarkerFuncLitParam      int = 83
	MarkerKeyValue          int = 84
	MarkerKeyValueKey       int = 85
	MarkerKeyValueValue     int = 86
	MarkerTypeAssert        int = 87
	MarkerTypeAssertType    int = 88
	MarkerTypeAssertX       int = 89
	MarkerTypeAlias         int = 90
	MarkerTypeAliasType     int = 91
	MarkerFuncSignatures    int = 92
	MarkerArrayType         int = 93
	MarkerMapType           int = 94
	MarkerMapKeyType        int = 95
	MarkerMapValueType      int = 96
)

// FragmentStack provides a clean Push/Pop/Reduce API on top of GoFIR's storage.
type FragmentStack struct {
	gir *GoFIR
}

// NewFragmentStack creates a new FragmentStack wrapping the given GoFIR instance.
func NewFragmentStack(gir *GoFIR) *FragmentStack {
	return &FragmentStack{gir: gir}
}

// markerString converts a marker int to a unique pointer string for PIV lookup.
func markerString(marker int) string {
	return fmt.Sprintf("__JSPRIM_M_%d", marker)
}

// PushMarker records a marker in the GoFIR's pointerAndIndexVec without adding a token.
func (fs *FragmentStack) PushMarker(marker int) {
	fs.gir.emitToFileBufferString("", markerString(marker))
}

// Push adds a token to the GoFIR's tokenSlice with the given code, tag, and Go type.
func (fs *FragmentStack) Push(code string, tag int, goType types.Type) {
	token := Token{Content: code, Tag: tag, GoType: goType}
	fs.gir.emitTokenToFileBufferString(token, "__JSPRIM_PUSH")
}

// PushCode is a convenience method that pushes code with TagExpr and nil GoType.
func (fs *FragmentStack) PushCode(code string) {
	fs.Push(code, TagExpr, nil)
}

// PushCodeWithType is a convenience method that pushes code with TagExpr and a Go type.
func (fs *FragmentStack) PushCodeWithType(code string, t types.Type) {
	fs.Push(code, TagExpr, t)
}

// Reduce finds the last marker matching the given marker int, extracts all tokens
// from that position to the end, trims both tokenSlice and pointerAndIndexVec,
// and returns the extracted tokens.
func (fs *FragmentStack) Reduce(marker int) []Token {
	target := markerString(marker)
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
func (fs *FragmentStack) ReduceToCode(marker int) string {
	tokens := fs.Reduce(marker)
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
