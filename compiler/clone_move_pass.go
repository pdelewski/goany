package compiler

import (
	"strings"
)

// isBuiltinCallee returns true for Go builtin functions that handle their
// arguments by reference or method call, making .clone() unnecessary.
func isBuiltinCallee(name string) bool {
	switch name {
	case "len", "delete", "min", "max", "clear", "make":
		return true
	}
	return false
}

// CloneMovePass is an IR pass that handles ownership semantics:
// clone removal (Rust), move wrapping (C++), std::mem::take, temp extraction.
//
// The emitter generates conservative code with .clone() on all non-Copy args,
// composite field values, and multi-return values. This pass removes .clone()
// where ownership can be transferred (CanTransfer → move, IsReassignedSource
// → std::mem::take, IsExtractedArg → temp extraction).
//
// Runs BEFORE RefOptPass: removes unnecessary .clone(), then RefOptPass
// replaces remaining .clone() with & where read-only.
type CloneMovePass struct {
	Tag            int  // TagRust, TagCpp
	Enabled        bool
	TransformCount int
	extractedArgs  map[int]IRNode // ParamIndex → original IR tree (for temp extraction)
}

func (p *CloneMovePass) Name() string { return "CloneMove" }

func (p *CloneMovePass) Transform(root IRNode) IRNode {
	if !p.Enabled {
		return root
	}
	return p.transformTree(root)
}

// transformTree recursively transforms the IR tree using bottom-up traversal.
func (p *CloneMovePass) transformTree(node IRNode) IRNode {
	// Bottom-up: recurse into children first
	if len(node.Children) > 0 {
		newChildren := make([]IRNode, 0, len(node.Children))
		for _, child := range node.Children {
			result := p.transformTree(child)
			// Node expansion: if result has MultiNode marker, splice children
			if result.Type == MultiNode {
				newChildren = append(newChildren, result.Children...)
			} else {
				newChildren = append(newChildren, result)
			}
		}
		node.Children = newChildren
		node.Content = node.Serialize()
	}

	// Transform this node based on OptMeta annotations
	if node.OptMeta != nil {
		node = p.transformNode(node)
	}

	return node
}

// transformNode dispatches to the appropriate handler based on OptMeta.Kind.
func (p *CloneMovePass) transformNode(node IRNode) IRNode {
	m := node.OptMeta
	switch m.Kind {
	case OptCallArg:
		return p.transformCallArg(node)
	case OptAssignment:
		return p.transformAssignment(node)
	}
	return node
}

// transformCallArg handles ownership semantics for function call arguments.
// For Rust: removes .clone() where ownership can be transferred.
// For C++: wraps transferable args with std::move().
func (p *CloneMovePass) transformCallArg(node IRNode) IRNode {
	m := node.OptMeta
	if m == nil {
		return node
	}

	switch p.Tag {
	case TagRust:
		if m.IsReassignedSource {
			// Replace entirely with std::mem::take
			p.TransformCount++
			node.Content = "std::mem::take(&mut " + m.ReassignedExpr + ")"
			node.Children = nil
			return node
		}
		if m.IsExtractedArg {
			// Save the original arg tree for the parent AssignStatement
			if p.extractedArgs == nil {
				p.extractedArgs = make(map[int]IRNode)
			}
			savedNode := node
			savedNode.OptMeta = nil
			p.extractedArgs[m.ParamIndex] = savedNode

			p.TransformCount++
			node.Content = m.ExtractedName
			node.Children = nil
			return node
		}
		if m.CanTransfer {
			// Remove .clone() — variable can be moved
			p.TransformCount++
			node.Content = strings.TrimSuffix(node.Content, ".clone()")
			node.Children = nil
			return node
		}
		if isBuiltinCallee(m.CalleeName) {
			// Remove .clone() — builtin handles value by reference or method call
			p.TransformCount++
			node.Content = strings.TrimSuffix(node.Content, ".clone()")
			node.Children = nil
			return node
		}
		// IsElementCopy, IsOwnedValue — no clone was added, nothing to remove
		// Default (no optimization flag) — clone stays (conservative)
	case TagCpp:
		if m.CanTransfer {
			p.TransformCount++
			// Wrap with std::move()
			node = IRTree(CallExpression, KindExpr,
				Leaf(Identifier, "std::move("),
				IRNode{Type: Identifier, Content: node.Content, Kind: KindExpr},
				Leaf(RightParen, ")"),
			)
			node.Content = node.Serialize()
			return node
		}
	}
	return node
}

// transformAssignment handles move temp extraction for assignment statements.
// Creates temp variable bindings before the assignment and returns a MultiNode.
func (p *CloneMovePass) transformAssignment(node IRNode) IRNode {
	m := node.OptMeta
	if m == nil || len(m.TempExtractions) == 0 || p.Tag != TagRust {
		return node
	}
	if len(p.extractedArgs) == 0 {
		return node
	}

	// Extract indent from the assignment node's first WhiteSpace child
	indent := ""
	for _, child := range node.Children {
		if child.Type == WhiteSpace {
			indent = child.Content
			break
		}
	}

	// Build temp binding nodes from TempExtractions using captured arg trees
	var bindings []IRNode
	for _, ext := range m.TempExtractions {
		valueNode, ok := p.extractedArgs[ext.ArgIndex]
		if !ok {
			continue
		}
		binding := IRTree(AssignStatement, KindStmt,
			Leaf(WhiteSpace, indent),
			LeafTag(Keyword, "let", TagRust),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, ext.TempName),
			Leaf(Colon, ":"),
			Leaf(WhiteSpace, " "),
			Leaf(TypeKeyword, ext.TypeStr),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, "="),
			Leaf(WhiteSpace, " "),
			valueNode,
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		)
		bindings = append(bindings, binding)
	}

	// Clear extraction state
	p.extractedArgs = nil

	if len(bindings) == 0 {
		return node
	}

	// Return MultiNode: temp bindings followed by the modified assignment
	node.OptMeta = nil // clear to avoid re-processing
	result := make([]IRNode, 0, len(bindings)+1)
	result = append(result, bindings...)
	result = append(result, node)
	return IRNode{Type: MultiNode, Children: result}
}
