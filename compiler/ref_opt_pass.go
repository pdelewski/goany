package compiler

import (
	"strings"
)

// RefOptPass is an IR pass that transforms function parameter types
// and call-site arguments to use reference semantics when the analysis
// indicates the parameter is read-only or mut-ref eligible.
//
// Active for Rust, C++, and C# — all three emitters preserve tree structure
// for both params (OptFuncParam) and call args (OptCallArg), allowing the
// pass to handle all ref-opt transformations.
//
// Uses bottom-up traversal so inner OptCallArg nodes (nested calls) are
// transformed before outer nodes flatten their children.
//
// Param transformations:
//   - Rust:  mut name: T → name: &T (read-only) or name: &mut T (mut-ref)
//   - C++:   Type name → const Type& name (read-only) or Type& name (mut-ref)
//   - C#:    Type name → in Type name (read-only) or ref Type name (mut-ref)
type RefOptPass struct {
	Tag            int              // TagRust, TagCpp, TagCSharp
	Enabled        bool             // when false, Transform is a no-op (coexists with inline ref-opt)
	TransformCount int              // number of nodes transformed by the pass
	Analysis       *ReadOnlyAnalysis // cross-package read-only/mut-ref data (set by emitter)
}

func (p *RefOptPass) Name() string { return "RefOpt" }

// Transform walks the IR tree and applies reference optimizations based on OptMeta annotations.
func (p *RefOptPass) Transform(root IRNode) IRNode {
	if !p.Enabled {
		return root
	}

	// Phase 1: Collect function param info from OptFuncParam annotations
	funcParams := make(map[string][]paramInfo) // funcKey → params
	p.collectFuncParams(root, funcParams)

	// Phase 1b: Seed funcParams from cross-package ReadOnlyAnalysis
	// OptFuncParam annotations only exist for functions defined in the current
	// file's IR tree. For cross-package calls, the callee's params are in a
	// different tree. Use the accumulated ReadOnlyAnalysis to fill in the gaps.
	if p.Analysis != nil {
		for key, flags := range p.Analysis.ReadOnly {
			if _, exists := funcParams[key]; exists {
				continue // already have from tree annotations
			}
			mutFlags := p.Analysis.MutRef[key]
			for i, ro := range flags {
				isMut := false
				if mutFlags != nil && i < len(mutFlags) {
					isMut = mutFlags[i]
				}
				if ro || isMut {
					funcParams[key] = append(funcParams[key], paramInfo{
						ParamIndex: i,
						IsReadOnly: ro,
						IsMutRef:   isMut,
					})
				}
			}
		}
	}

	// Phase 2: Transform param nodes and call arg nodes
	return p.transformTree(root, funcParams)
}

// paramInfo holds ref-opt flags for a single function parameter.
type paramInfo struct {
	ParamIndex int
	ParamName  string
	TypeStr    string
	IsReadOnly bool
	IsMutRef   bool
}

// collectFuncParams walks the tree to find all OptFuncParam annotations.
func (p *RefOptPass) collectFuncParams(node IRNode, result map[string][]paramInfo) {
	if node.OptMeta != nil && node.OptMeta.Kind == OptFuncParam {
		key := node.OptMeta.FuncKey
		result[key] = append(result[key], paramInfo{
			ParamIndex: node.OptMeta.ParamIndex,
			ParamName:  node.OptMeta.ParamName,
			TypeStr:    node.OptMeta.TypeStr,
			IsReadOnly: node.OptMeta.IsReadOnly,
			IsMutRef:   node.OptMeta.IsMutRef,
		})
	}
	for _, child := range node.Children {
		p.collectFuncParams(child, result)
	}
}

// transformTree recursively transforms the IR tree using bottom-up traversal.
// Children are transformed first so that inner OptCallArg nodes (e.g., nested calls)
// get their & prefix before an outer OptCallArg flattens children with Children=nil.
func (p *RefOptPass) transformTree(node IRNode, funcParams map[string][]paramInfo) IRNode {
	// Bottom-up: recurse into children first
	for i := range node.Children {
		node.Children[i] = p.transformTree(node.Children[i], funcParams)
	}

	// Recompute Content after children changed
	if len(node.Children) > 0 {
		node.Content = node.Serialize()
	}

	// Transform this node
	if node.OptMeta != nil {
		switch node.OptMeta.Kind {
		case OptFuncParam:
			node = p.transformParam(node)
		case OptCallArg:
			node = p.transformCallArg(node, funcParams)
		}
	}

	return node
}

// transformParam transforms a function parameter node based on IsReadOnly/IsMutRef flags.
func (p *RefOptPass) transformParam(node IRNode) IRNode {
	m := node.OptMeta
	if !m.IsReadOnly && !m.IsMutRef {
		return node
	}

	p.TransformCount++
	switch p.Tag {
	case TagRust:
		return p.transformRustParam(node)
	case TagCpp:
		return p.transformCppParam(node)
	case TagCSharp:
		return p.transformCSharpParam(node)
	}
	return node
}

// transformRustParam transforms Rust function parameter nodes.
// Base: "mut name: Type" → ReadOnly: "name: &Type" or MutRef: "name: &mut Type"
func (p *RefOptPass) transformRustParam(node IRNode) IRNode {
	m := node.OptMeta
	content := node.Content

	// Skip if already transformed (contains reference markers)
	if strings.Contains(content, ": &") {
		return node
	}

	if m.IsReadOnly {
		node.Children = []IRNode{
			Leaf(Identifier, m.ParamName),
			Leaf(Colon, ":"),
			Leaf(WhiteSpace, " "),
			LeafTag(Keyword, "&", TagRust),
			Leaf(Identifier, m.TypeStr),
		}
		node.Content = node.Serialize()
	} else if m.IsMutRef {
		node.Children = []IRNode{
			Leaf(Identifier, m.ParamName),
			Leaf(Colon, ":"),
			Leaf(WhiteSpace, " "),
			LeafTag(Keyword, "&mut", TagRust),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, m.TypeStr),
		}
		node.Content = node.Serialize()
	}
	return node
}

// transformCppParam transforms C++ function parameter nodes.
// Base: "Type name" → ReadOnly: "const Type& name" or MutRef: "Type& name"
func (p *RefOptPass) transformCppParam(node IRNode) IRNode {
	m := node.OptMeta

	// Skip if already transformed
	if strings.HasPrefix(node.Content, "const ") || strings.Contains(node.Content, "& ") {
		return node
	}

	if m.IsReadOnly {
		node.Content = "const " + m.TypeStr + "& " + m.ParamName
		node.Children = nil
	} else if m.IsMutRef {
		node.Content = m.TypeStr + "& " + m.ParamName
		node.Children = nil
	}
	return node
}

// transformCSharpParam transforms C# function parameter nodes.
// Base: "Type name" → ReadOnly: "in Type name" or MutRef: "ref Type name"
func (p *RefOptPass) transformCSharpParam(node IRNode) IRNode {
	m := node.OptMeta
	content := node.Content

	// Skip if already transformed
	if strings.HasPrefix(content, "in ") || strings.HasPrefix(content, "ref ") {
		return node
	}

	if m.IsReadOnly {
		node.Children = []IRNode{
			LeafTag(Keyword, "in ", TagCSharp),
			Leaf(Identifier, m.TypeStr),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, m.ParamName),
		}
		node.Content = node.Serialize()
	} else if m.IsMutRef {
		node.Children = []IRNode{
			LeafTag(Keyword, "ref ", TagCSharp),
			Leaf(Identifier, m.TypeStr),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, m.ParamName),
		}
		node.Content = node.Serialize()
	}
	return node
}

// transformCallArg transforms a call argument node to add reference prefix.
func (p *RefOptPass) transformCallArg(node IRNode, funcParams map[string][]paramInfo) IRNode {
	m := node.OptMeta
	if m.FuncKey == "" {
		return node
	}

	params, ok := funcParams[m.FuncKey]
	if !ok {
		return node
	}

	// Find the param info for this argument's position
	var param *paramInfo
	for i := range params {
		if params[i].ParamIndex == m.ParamIndex {
			param = &params[i]
			break
		}
	}
	if param == nil {
		return node
	}

	if !param.IsReadOnly && !param.IsMutRef {
		return node
	}

	switch p.Tag {
	case TagRust:
		return p.transformRustCallArg(node, param)
	case TagCSharp:
		return p.transformCSharpCallArg(node, param)
	}
	// C++ doesn't need call-site changes
	return node
}

// transformRustCallArg adds & or &mut prefix to Rust call arguments.
// Also strips .clone() calls since borrowing replaces cloning.
func (p *RefOptPass) transformRustCallArg(node IRNode, param *paramInfo) IRNode {
	content := node.Content

	// Skip if already has reference prefix
	if strings.HasPrefix(content, "&") {
		return node
	}

	// Remove trailing .clone() — borrowing replaces cloning.
	// Only removes the suffix to preserve inner .clone() calls needed for
	// other reasons (e.g., moving out of shared references).
	content = strings.TrimSuffix(content, ".clone()")

	if param.IsReadOnly {
		node.Content = "&" + content
		node.Children = nil
		p.TransformCount++
	} else if param.IsMutRef {
		node.Content = "&mut " + content
		node.Children = nil
		p.TransformCount++
	}
	return node
}

// transformCSharpCallArg adds in/ref prefix to C# call arguments.
func (p *RefOptPass) transformCSharpCallArg(node IRNode, param *paramInfo) IRNode {
	// C# requires lvalues for in/ref — skip non-identifier args
	if node.OptMeta != nil && !node.OptMeta.IsIdentArg {
		return node
	}

	content := node.Content

	// Skip if already has in/ref prefix
	if strings.HasPrefix(content, "in ") || strings.HasPrefix(content, "ref ") {
		return node
	}

	if param.IsReadOnly {
		node.Content = "in " + content
		node.Children = nil
		p.TransformCount++
	} else if param.IsMutRef {
		node.Content = "ref " + content
		node.Children = nil
		p.TransformCount++
	}
	return node
}
