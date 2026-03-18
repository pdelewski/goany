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

// stackFrame holds a marker name and the children collected at that scope level.
type stackFrame struct {
	marker   string
	children []IRNode
}

// IRForestBuilder builds a forest of IR trees during emission.
type IRForestBuilder struct {
	stack []stackFrame
}

// NewIRForestBuilder creates a new IRForestBuilder.
func NewIRForestBuilder() *IRForestBuilder {
	return &IRForestBuilder{
		stack: []stackFrame{{marker: "__ROOT"}},
	}
}

// AddVisitMarker pushes a new scope frame for a visit method marker.
func (fb *IRForestBuilder) AddVisitMarker(pointer VisitMethod) {
	fb.stack = append(fb.stack, stackFrame{marker: string(pointer)})
}

// AddMarker pushes a new scope frame with a named marker.
func (fb *IRForestBuilder) AddMarker(name string) {
	fb.stack = append(fb.stack, stackFrame{marker: name})
}

// AddLeaf adds a leaf node to the current scope's children.
func (fb *IRForestBuilder) AddLeaf(code string, kind NodeKind, goType types.Type) {
	if code == "" {
		return
	}
	token := IRNode{Content: code, Kind: kind, GoType: goType}
	top := &fb.stack[len(fb.stack)-1]
	top.children = append(top.children, token)
}

// AddTree adds a tree node to the current scope's children (auto-computes Content if needed).
func (fb *IRForestBuilder) AddTree(token IRNode) {
	if len(token.Children) > 0 && token.Content == "" {
		token.Content = token.Serialize()
	}
	if token.Content == "" {
		return
	}
	top := &fb.stack[len(fb.stack)-1]
	top.children = append(top.children, token)
}

// CollectForest finds the last marker matching visitMethod, pops all frames from
// that position onward, concatenates their children, and returns them.
func (fb *IRForestBuilder) CollectForest(visitMethod string) []IRNode {
	// Find marker in stack (searching from end)
	pivIdx := -1
	for i := len(fb.stack) - 1; i >= 0; i-- {
		if fb.stack[i].marker == visitMethod {
			pivIdx = i
			break
		}
	}
	if pivIdx < 0 {
		return nil
	}
	// Concatenate children from all popped frames
	var result []IRNode
	for i := pivIdx; i < len(fb.stack); i++ {
		result = append(result, fb.stack[i].children...)
	}
	// Pop all frames from marker position onwards
	fb.stack = fb.stack[:pivIdx]
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

// AutoCollect cleans up the stack after a PostVisitXxx call when the emitter
// didn't explicitly collect via CollectForest/CollectText.
//
// When the emitter's PostVisitXxx already called CollectForest/CollectText,
// both Pre and Post frames were consumed, so AutoCollect is a no-op.
//
// When the emitter was a no-op, AutoCollect pops the Post frame and merges
// any children it may hold into the frame below. The Pre frame and its
// children are left intact — a parent or sibling emitter may collect from
// the Pre marker later (some emitters span multiple Pre/Post pairs).
func (fb *IRForestBuilder) AutoCollect(postMarker VisitMethod) {
	name := string(postMarker)
	if !strings.HasPrefix(name, "Post") {
		return
	}
	// Check if the Post marker frame is still on the stack.
	postName := name
	postIdx := -1
	for i := len(fb.stack) - 1; i >= 0; i-- {
		if fb.stack[i].marker == postName {
			postIdx = i
			break
		}
	}
	if postIdx < 0 {
		return // emitter already collected (consumed both Pre and Post markers)
	}
	// Merge children from the Post frame (and any frames above) into
	// the frame below, then pop.  This does NOT consume the Pre marker.
	if postIdx > 0 {
		for i := postIdx; i < len(fb.stack); i++ {
			fb.stack[postIdx-1].children = append(fb.stack[postIdx-1].children, fb.stack[i].children...)
		}
	}
	fb.stack = fb.stack[:postIdx]
}

// GetTree collects everything remaining on the stack into a single root node.
func (fb *IRForestBuilder) GetTree() IRNode {
	var allChildren []IRNode
	for i := 0; i < len(fb.stack); i++ {
		allChildren = append(allChildren, fb.stack[i].children...)
	}
	root := IRNode{Type: ScopeNode, Kind: KindDecl, Children: allChildren}
	root.Content = root.Serialize()
	return root
}
