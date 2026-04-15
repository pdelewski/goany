package compiler

import (
	"strings"
	"testing"
)

func TestCloneMovePass_RustRemoveClone_CanTransfer(t *testing.T) {
	// Emitter outputs "x.clone()" with CanTransfer — pass removes .clone()
	node := Leaf(Identifier, "x.clone()")
	node.OptMeta = &OptMeta{Kind: OptCallArg, CanTransfer: true}

	pass := &CloneMovePass{Tag: TagRust, Enabled: true}
	result := pass.Transform(node)

	if result.Content != "x" {
		t.Errorf("expected 'x', got %q", result.Content)
	}
	if pass.TransformCount != 1 {
		t.Errorf("expected TransformCount=1, got %d", pass.TransformCount)
	}
}

func TestCloneMovePass_RustKeepClone_NoFlag(t *testing.T) {
	// Emitter outputs "x.clone()" with OptCallArg but no optimization flag — clone stays
	node := Leaf(Identifier, "x.clone()")
	node.OptMeta = &OptMeta{Kind: OptCallArg}

	pass := &CloneMovePass{Tag: TagRust, Enabled: true}
	result := pass.Transform(node)

	if result.Content != "x.clone()" {
		t.Errorf("expected 'x.clone()', got %q", result.Content)
	}
	if pass.TransformCount != 0 {
		t.Errorf("expected TransformCount=0, got %d", pass.TransformCount)
	}
}

func TestCloneMovePass_RustReassignedSource(t *testing.T) {
	// IsReassignedSource replaces content with std::mem::take
	node := Leaf(Identifier, "state.cpu")
	node.OptMeta = &OptMeta{Kind: OptCallArg, IsReassignedSource: true, ReassignedExpr: "state.cpu"}

	pass := &CloneMovePass{Tag: TagRust, Enabled: true}
	result := pass.Transform(node)

	if result.Content != "std::mem::take(&mut state.cpu)" {
		t.Errorf("expected 'std::mem::take(&mut state.cpu)', got %q", result.Content)
	}
	if pass.TransformCount != 1 {
		t.Errorf("expected TransformCount=1, got %d", pass.TransformCount)
	}
}

func TestCloneMovePass_RustExtractedArg(t *testing.T) {
	// IsExtractedArg replaces content with temp variable name
	node := Leaf(Identifier, "expr")
	node.OptMeta = &OptMeta{Kind: OptCallArg, IsExtractedArg: true, ExtractedName: "__mv0", ParamIndex: 1}

	pass := &CloneMovePass{Tag: TagRust, Enabled: true}
	result := pass.Transform(node)

	if result.Content != "__mv0" {
		t.Errorf("expected '__mv0', got %q", result.Content)
	}
	if pass.TransformCount != 1 {
		t.Errorf("expected TransformCount=1, got %d", pass.TransformCount)
	}
}

func TestCloneMovePass_RustElementCopy(t *testing.T) {
	// IsElementCopy — no clone was added, nothing to remove
	node := Leaf(Identifier, "vec[0].clone()")
	node.OptMeta = &OptMeta{Kind: OptCallArg, IsElementCopy: true}

	pass := &CloneMovePass{Tag: TagRust, Enabled: true}
	result := pass.Transform(node)

	if result.Content != "vec[0].clone()" {
		t.Errorf("expected 'vec[0].clone()', got %q", result.Content)
	}
	if pass.TransformCount != 0 {
		t.Errorf("expected TransformCount=0, got %d", pass.TransformCount)
	}
}

func TestCloneMovePass_CppCanTransfer(t *testing.T) {
	// C++ CanTransfer wraps with std::move()
	node := Leaf(Identifier, "x")
	node.OptMeta = &OptMeta{Kind: OptCallArg, CanTransfer: true}

	pass := &CloneMovePass{Tag: TagCpp, Enabled: true}
	result := pass.Transform(node)

	if result.Content != "std::move(x)" {
		t.Errorf("expected 'std::move(x)', got %q", result.Content)
	}
	if pass.TransformCount != 1 {
		t.Errorf("expected TransformCount=1, got %d", pass.TransformCount)
	}
}

func TestCloneMovePass_Disabled(t *testing.T) {
	// When disabled, pass is a no-op
	node := Leaf(Identifier, "x.clone()")
	node.OptMeta = &OptMeta{Kind: OptCallArg, CanTransfer: true}

	pass := &CloneMovePass{Tag: TagRust, Enabled: false}
	result := pass.Transform(node)

	if result.Content != "x.clone()" {
		t.Errorf("expected 'x.clone()' (unchanged), got %q", result.Content)
	}
	if pass.TransformCount != 0 {
		t.Errorf("expected TransformCount=0, got %d", pass.TransformCount)
	}
}

func TestCloneMovePass_NestedTree(t *testing.T) {
	// Test that bottom-up traversal processes children first
	inner := Leaf(Identifier, "y.clone()")
	inner.OptMeta = &OptMeta{Kind: OptCallArg, CanTransfer: true}

	outer := IRTree(CallExpression, KindExpr,
		Leaf(Identifier, "f("),
		inner,
		Leaf(RightParen, ")"),
	)

	pass := &CloneMovePass{Tag: TagRust, Enabled: true}
	result := pass.Transform(outer)

	if result.Content != "f(y)" {
		t.Errorf("expected 'f(y)', got %q", result.Content)
	}
	if pass.TransformCount != 1 {
		t.Errorf("expected TransformCount=1, got %d", pass.TransformCount)
	}
}

func TestCloneMovePass_RustBuiltinCallee(t *testing.T) {
	// Builtin callees (len, delete, etc.) remove .clone() since they handle by reference
	for _, name := range []string{"len", "delete", "min", "max", "clear", "make"} {
		node := Leaf(Identifier, "x.clone()")
		node.OptMeta = &OptMeta{Kind: OptCallArg, CalleeName: name}

		pass := &CloneMovePass{Tag: TagRust, Enabled: true}
		result := pass.Transform(node)

		if result.Content != "x" {
			t.Errorf("builtin %q: expected 'x', got %q", name, result.Content)
		}
		if pass.TransformCount != 1 {
			t.Errorf("builtin %q: expected TransformCount=1, got %d", name, pass.TransformCount)
		}
	}
}

func TestCloneMovePass_RustOwnedValue(t *testing.T) {
	// IsOwnedValue — no clone was added by emitter, nothing to remove
	node := Leaf(Identifier, "String::from(\"hi\")")
	node.OptMeta = &OptMeta{Kind: OptCallArg, IsOwnedValue: true}

	pass := &CloneMovePass{Tag: TagRust, Enabled: true}
	result := pass.Transform(node)

	if result.Content != "String::from(\"hi\")" {
		t.Errorf("expected unchanged, got %q", result.Content)
	}
	if pass.TransformCount != 0 {
		t.Errorf("expected TransformCount=0, got %d", pass.TransformCount)
	}
}

func TestCloneMovePass_CppNoFlag(t *testing.T) {
	// C++ with no CanTransfer — no-op
	node := Leaf(Identifier, "x")
	node.OptMeta = &OptMeta{Kind: OptCallArg}

	pass := &CloneMovePass{Tag: TagCpp, Enabled: true}
	result := pass.Transform(node)

	if result.Content != "x" {
		t.Errorf("expected 'x', got %q", result.Content)
	}
	if pass.TransformCount != 0 {
		t.Errorf("expected TransformCount=0, got %d", pass.TransformCount)
	}
}

func TestCloneMovePass_RustAssignment_TempExtraction(t *testing.T) {
	// OptAssignment with TempExtractions produces MultiNode with temp bindings
	// Simulate: f(a.clone(), b.clone()) where b is extracted to __mv0
	argB := Leaf(Identifier, "b.clone()")
	argB.OptMeta = &OptMeta{Kind: OptCallArg, IsExtractedArg: true, ExtractedName: "__mv0", ParamIndex: 1}

	argA := Leaf(Identifier, "a.clone()")
	argA.OptMeta = &OptMeta{Kind: OptCallArg}

	assignNode := IRTree(AssignStatement, KindStmt,
		Leaf(WhiteSpace, "    "),
		Leaf(Identifier, "result"),
		Leaf(Assignment, "="),
		Leaf(Identifier, "f("),
		argA,
		Leaf(Comma, ","),
		argB,
		Leaf(RightParen, ")"),
		Leaf(Semicolon, ";"),
	)
	assignNode.OptMeta = &OptMeta{
		Kind: OptAssignment,
		TempExtractions: []TempExtraction{
			{ArgIndex: 1, TempName: "__mv0", TypeStr: "String"},
		},
	}

	pass := &CloneMovePass{Tag: TagRust, Enabled: true}
	result := pass.Transform(assignNode)

	// Should produce MultiNode with temp binding + assignment
	if result.Type != MultiNode {
		t.Fatalf("expected MultiNode, got %v", result.Type)
	}
	if len(result.Children) != 2 {
		t.Fatalf("expected 2 children (binding + assignment), got %d", len(result.Children))
	}
	// First child: temp binding "let __mv0: String = b.clone();"
	bindingStr := result.Children[0].Serialize()
	if !strings.Contains(bindingStr, "__mv0") || !strings.Contains(bindingStr, "b.clone()") {
		t.Errorf("expected binding with __mv0 and b.clone(), got %q", bindingStr)
	}
	// Second child: modified assignment using __mv0
	assignStr := result.Children[1].Serialize()
	if !strings.Contains(assignStr, "__mv0") {
		t.Errorf("expected assignment with __mv0, got %q", assignStr)
	}
}

func TestCloneMovePass_MultiNodeSplicing(t *testing.T) {
	// When a child transforms into MultiNode, its children should be spliced into parent
	argB := Leaf(Identifier, "b.clone()")
	argB.OptMeta = &OptMeta{Kind: OptCallArg, IsExtractedArg: true, ExtractedName: "__mv0", ParamIndex: 0}

	inner := IRTree(AssignStatement, KindStmt,
		Leaf(WhiteSpace, "    "),
		Leaf(Identifier, "f("),
		argB,
		Leaf(RightParen, ")"),
		Leaf(Semicolon, ";"),
	)
	inner.OptMeta = &OptMeta{
		Kind: OptAssignment,
		TempExtractions: []TempExtraction{
			{ArgIndex: 0, TempName: "__mv0", TypeStr: "String"},
		},
	}

	// Wrap in a parent block
	outer := IRTree(BlockStatement, KindStmt,
		Leaf(LeftBrace, "{"),
		inner,
		Leaf(RightBrace, "}"),
	)

	pass := &CloneMovePass{Tag: TagRust, Enabled: true}
	result := pass.Transform(outer)

	// The inner MultiNode should have been spliced, so parent has more children
	// Original: { inner } → { binding, assignment, }
	if len(result.Children) < 4 {
		t.Errorf("expected at least 4 children after splicing, got %d", len(result.Children))
	}
}
