// pattern_extract.go provides functions to extract patterns from AST nodes.
// These functions are used by the sema checker to validate constructs against
// the pattern database.
package compiler

import (
	"fmt"
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// ExtractForStmtPattern extracts a pattern from a for statement
func ExtractForStmtPattern(n *ast.ForStmt, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"has_init": fmt.Sprintf("%t", n.Init != nil),
		"has_cond": fmt.Sprintf("%t", n.Cond != nil),
		"has_post": fmt.Sprintf("%t", n.Post != nil),
	}

	// Capture init statement type
	if n.Init != nil {
		switch init := n.Init.(type) {
		case *ast.AssignStmt:
			attrs["init_type"] = init.Tok.String()
		case *ast.DeclStmt:
			attrs["init_type"] = "decl"
		default:
			attrs["init_type"] = fmt.Sprintf("%T", n.Init)
		}
	}

	// Capture post statement type
	if n.Post != nil {
		switch post := n.Post.(type) {
		case *ast.IncDecStmt:
			attrs["post_type"] = post.Tok.String()
		case *ast.AssignStmt:
			attrs["post_type"] = post.Tok.String()
		default:
			attrs["post_type"] = fmt.Sprintf("%T", n.Post)
		}
	}

	return Pattern{Kind: "ForStmt", Attrs: attrs}
}

// ExtractRangeStmtPattern extracts a pattern from a range statement
func ExtractRangeStmtPattern(n *ast.RangeStmt, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"tok": n.Tok.String(),
	}

	// Key variable status
	if n.Key == nil {
		attrs["key"] = "none"
	} else if ident, ok := n.Key.(*ast.Ident); ok && ident.Name == "_" {
		attrs["key"] = "blank"
	} else {
		attrs["key"] = "used"
	}

	// Value variable status
	if n.Value == nil {
		attrs["value"] = "none"
	} else if ident, ok := n.Value.(*ast.Ident); ok && ident.Name == "_" {
		attrs["value"] = "blank"
	} else {
		attrs["value"] = "used"
	}

	// Collection type being ranged over
	if pkg != nil && pkg.TypesInfo != nil {
		if tv, ok := pkg.TypesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["collection_type"] = typeCategory(tv.Type)
		}
	}

	return Pattern{Kind: "RangeStmt", Attrs: attrs}
}

// ExtractIfStmtPattern extracts a pattern from an if statement
func ExtractIfStmtPattern(n *ast.IfStmt, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"has_init": fmt.Sprintf("%t", n.Init != nil),
		"has_else": fmt.Sprintf("%t", n.Else != nil),
	}

	if n.Else != nil {
		switch n.Else.(type) {
		case *ast.IfStmt:
			attrs["else_type"] = "else_if"
		case *ast.BlockStmt:
			attrs["else_type"] = "else_block"
		}
	}

	return Pattern{Kind: "IfStmt", Attrs: attrs}
}

// ExtractSwitchStmtPattern extracts a pattern from a switch statement
func ExtractSwitchStmtPattern(n *ast.SwitchStmt, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"has_init": fmt.Sprintf("%t", n.Init != nil),
		"has_tag":  fmt.Sprintf("%t", n.Tag != nil),
	}

	// Count cases and check for default
	hasDefault := false
	caseCount := 0
	for _, stmt := range n.Body.List {
		if cc, ok := stmt.(*ast.CaseClause); ok {
			caseCount++
			if cc.List == nil {
				hasDefault = true
			}
		}
	}
	attrs["case_count"] = fmt.Sprintf("%d", caseCount)
	attrs["has_default"] = fmt.Sprintf("%t", hasDefault)

	return Pattern{Kind: "SwitchStmt", Attrs: attrs}
}

// ExtractTypeSwitchStmtPattern extracts a pattern from a type switch statement
func ExtractTypeSwitchStmtPattern(n *ast.TypeSwitchStmt, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"has_init": fmt.Sprintf("%t", n.Init != nil),
	}
	return Pattern{Kind: "TypeSwitchStmt", Attrs: attrs}
}

// ExtractAssignStmtPattern extracts a pattern from an assignment statement
func ExtractAssignStmtPattern(n *ast.AssignStmt, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"tok":       n.Tok.String(),
		"lhs_count": fmt.Sprintf("%d", len(n.Lhs)),
		"rhs_count": fmt.Sprintf("%d", len(n.Rhs)),
	}

	// Check for comma-ok idiom (v, ok := m[key] or v, ok := x.(T))
	if len(n.Lhs) == 2 && len(n.Rhs) == 1 {
		switch rhs := n.Rhs[0].(type) {
		case *ast.IndexExpr:
			// Check if it's map access
			if pkg != nil && pkg.TypesInfo != nil {
				if tv, ok := pkg.TypesInfo.Types[rhs.X]; ok && tv.Type != nil {
					if _, isMap := tv.Type.Underlying().(*types.Map); isMap {
						attrs["comma_ok"] = "map"
					}
				}
			}
		case *ast.TypeAssertExpr:
			attrs["comma_ok"] = "type_assert"
		}
	}

	// Check RHS types
	if len(n.Rhs) == 1 && pkg != nil && pkg.TypesInfo != nil {
		if tv, ok := pkg.TypesInfo.Types[n.Rhs[0]]; ok && tv.Type != nil {
			attrs["rhs_type"] = typeCategory(tv.Type)
		}
	}

	return Pattern{Kind: "AssignStmt", Attrs: attrs}
}

// ExtractCallExprPattern extracts a pattern from a call expression
func ExtractCallExprPattern(n *ast.CallExpr, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"arg_count": fmt.Sprintf("%d", len(n.Args)),
	}

	switch fn := n.Fun.(type) {
	case *ast.Ident:
		attrs["func_type"] = "ident"
		attrs["func_name"] = fn.Name

		// Check for builtins
		switch fn.Name {
		case "make":
			attrs["builtin"] = "make"
			if len(n.Args) > 0 {
				if pkg != nil && pkg.TypesInfo != nil {
					if tv, ok := pkg.TypesInfo.Types[n.Args[0]]; ok && tv.Type != nil {
						attrs["make_type"] = typeCategory(tv.Type)
					}
				}
				// Also check AST for type
				switch n.Args[0].(type) {
				case *ast.ArrayType:
					attrs["make_ast"] = "slice"
				case *ast.MapType:
					attrs["make_ast"] = "map"
				case *ast.ChanType:
					attrs["make_ast"] = "chan"
				}
			}
		case "len", "cap", "append", "copy", "delete", "close", "panic", "recover", "print", "println", "new", "complex", "real", "imag":
			attrs["builtin"] = fn.Name
		}

	case *ast.SelectorExpr:
		attrs["func_type"] = "selector"
		attrs["method"] = fn.Sel.Name
		if ident, ok := fn.X.(*ast.Ident); ok {
			attrs["receiver"] = ident.Name
		}
		// Check if it's a method call on a type
		if pkg != nil && pkg.TypesInfo != nil {
			if tv, ok := pkg.TypesInfo.Types[fn.X]; ok && tv.Type != nil {
				attrs["receiver_type"] = typeCategory(tv.Type)
			}
		}

	case *ast.FuncLit:
		attrs["func_type"] = "literal"

	case *ast.IndexExpr:
		attrs["func_type"] = "generic"
	}

	// Check if variadic call
	if n.Ellipsis.IsValid() {
		attrs["variadic"] = "true"
	}

	return Pattern{Kind: "CallExpr", Attrs: attrs}
}

// ExtractCompositeLitPattern extracts a pattern from a composite literal
func ExtractCompositeLitPattern(n *ast.CompositeLit, pkg *packages.Package) Pattern {
	attrs := map[string]string{}

	// Check if elements are keyed
	hasKeys := false
	for _, elt := range n.Elts {
		if _, ok := elt.(*ast.KeyValueExpr); ok {
			hasKeys = true
			break
		}
	}
	attrs["keyed"] = fmt.Sprintf("%t", hasKeys)
	attrs["elt_count"] = fmt.Sprintf("%d", len(n.Elts))

	// Type of composite literal
	if pkg != nil && pkg.TypesInfo != nil {
		if tv, ok := pkg.TypesInfo.Types[n]; ok && tv.Type != nil {
			attrs["type"] = typeCategory(tv.Type)
		}
	}

	// Also check AST type
	switch n.Type.(type) {
	case *ast.ArrayType:
		attrs["ast_type"] = "array_or_slice"
	case *ast.MapType:
		attrs["ast_type"] = "map"
	case *ast.StructType:
		attrs["ast_type"] = "anon_struct"
	case *ast.Ident:
		attrs["ast_type"] = "named"
	case *ast.SelectorExpr:
		attrs["ast_type"] = "qualified"
	}

	return Pattern{Kind: "CompositeLit", Attrs: attrs}
}

// ExtractMapTypePattern extracts a pattern from a map type
func ExtractMapTypePattern(n *ast.MapType, pkg *packages.Package) Pattern {
	attrs := map[string]string{}

	// Key type
	if pkg != nil && pkg.TypesInfo != nil {
		if tv, ok := pkg.TypesInfo.Types[n.Key]; ok && tv.Type != nil {
			attrs["key_type"] = typeCategory(tv.Type)
		}
		if tv, ok := pkg.TypesInfo.Types[n.Value]; ok && tv.Type != nil {
			attrs["value_type"] = typeCategory(tv.Type)

			// Check for nested map
			if _, isMap := tv.Type.Underlying().(*types.Map); isMap {
				attrs["nested"] = "true"
			}
		}
	}

	// Also check AST for nested map
	if _, isMap := n.Value.(*ast.MapType); isMap {
		attrs["nested_ast"] = "true"
	}

	return Pattern{Kind: "MapType", Attrs: attrs}
}

// ExtractIndexExprPattern extracts a pattern from an index expression
func ExtractIndexExprPattern(n *ast.IndexExpr, pkg *packages.Package) Pattern {
	attrs := map[string]string{}

	if pkg != nil && pkg.TypesInfo != nil {
		if tv, ok := pkg.TypesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["collection_type"] = typeCategory(tv.Type)
		}
		if tv, ok := pkg.TypesInfo.Types[n.Index]; ok && tv.Type != nil {
			attrs["index_type"] = typeCategory(tv.Type)
		}
	}

	return Pattern{Kind: "IndexExpr", Attrs: attrs}
}

// ExtractSliceExprPattern extracts a pattern from a slice expression
func ExtractSliceExprPattern(n *ast.SliceExpr, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"has_low":  fmt.Sprintf("%t", n.Low != nil),
		"has_high": fmt.Sprintf("%t", n.High != nil),
		"has_max":  fmt.Sprintf("%t", n.Max != nil),
		"slice3":   fmt.Sprintf("%t", n.Slice3),
	}

	if pkg != nil && pkg.TypesInfo != nil {
		if tv, ok := pkg.TypesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["collection_type"] = typeCategory(tv.Type)
		}
	}

	return Pattern{Kind: "SliceExpr", Attrs: attrs}
}

// ExtractUnaryExprPattern extracts a pattern from a unary expression
func ExtractUnaryExprPattern(n *ast.UnaryExpr, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"op": n.Op.String(),
	}

	if pkg != nil && pkg.TypesInfo != nil {
		if tv, ok := pkg.TypesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["operand_type"] = typeCategory(tv.Type)
		}
	}

	return Pattern{Kind: "UnaryExpr", Attrs: attrs}
}

// ExtractBinaryExprPattern extracts a pattern from a binary expression
func ExtractBinaryExprPattern(n *ast.BinaryExpr, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"op": n.Op.String(),
	}

	if pkg != nil && pkg.TypesInfo != nil {
		if tv, ok := pkg.TypesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["left_type"] = typeCategory(tv.Type)
		}
		if tv, ok := pkg.TypesInfo.Types[n.Y]; ok && tv.Type != nil {
			attrs["right_type"] = typeCategory(tv.Type)
		}
	}

	return Pattern{Kind: "BinaryExpr", Attrs: attrs}
}

// ExtractFuncDeclPattern extracts a pattern from a function declaration
func ExtractFuncDeclPattern(n *ast.FuncDecl, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"has_recv": fmt.Sprintf("%t", n.Recv != nil),
	}

	if n.Recv != nil && len(n.Recv.List) > 0 {
		// Check receiver type
		recv := n.Recv.List[0]
		switch recv.Type.(type) {
		case *ast.StarExpr:
			attrs["recv_ptr"] = "true"
		case *ast.Ident:
			attrs["recv_ptr"] = "false"
		}
	}

	if n.Type != nil {
		if n.Type.Params != nil {
			attrs["param_count"] = fmt.Sprintf("%d", len(n.Type.Params.List))
		}
		if n.Type.Results != nil {
			attrs["result_count"] = fmt.Sprintf("%d", len(n.Type.Results.List))
		} else {
			attrs["result_count"] = "0"
		}
	}

	return Pattern{Kind: "FuncDecl", Attrs: attrs}
}

// ExtractFuncLitPattern extracts a pattern from a function literal
func ExtractFuncLitPattern(n *ast.FuncLit, pkg *packages.Package) Pattern {
	attrs := map[string]string{}

	if n.Type != nil {
		if n.Type.Params != nil {
			attrs["param_count"] = fmt.Sprintf("%d", len(n.Type.Params.List))
		}
		if n.Type.Results != nil {
			attrs["result_count"] = fmt.Sprintf("%d", len(n.Type.Results.List))
		} else {
			attrs["result_count"] = "0"
		}
	}

	return Pattern{Kind: "FuncLit", Attrs: attrs}
}

// ExtractReturnStmtPattern extracts a pattern from a return statement
func ExtractReturnStmtPattern(n *ast.ReturnStmt, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"result_count": fmt.Sprintf("%d", len(n.Results)),
	}
	return Pattern{Kind: "ReturnStmt", Attrs: attrs}
}

// ExtractBranchStmtPattern extracts a pattern from a branch statement
func ExtractBranchStmtPattern(n *ast.BranchStmt, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"tok":       n.Tok.String(),
		"has_label": fmt.Sprintf("%t", n.Label != nil),
	}
	return Pattern{Kind: "BranchStmt", Attrs: attrs}
}

// ExtractIncDecStmtPattern extracts a pattern from an increment/decrement statement
func ExtractIncDecStmtPattern(n *ast.IncDecStmt, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"tok": n.Tok.String(),
	}

	if pkg != nil && pkg.TypesInfo != nil {
		if tv, ok := pkg.TypesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["operand_type"] = typeCategory(tv.Type)
		}
	}

	return Pattern{Kind: "IncDecStmt", Attrs: attrs}
}

// ExtractTypeAssertExprPattern extracts a pattern from a type assertion expression
func ExtractTypeAssertExprPattern(n *ast.TypeAssertExpr, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"type_switch": fmt.Sprintf("%t", n.Type == nil),
	}

	if pkg != nil && pkg.TypesInfo != nil {
		if n.Type != nil {
			if tv, ok := pkg.TypesInfo.Types[n.Type]; ok && tv.Type != nil {
				attrs["assert_type"] = typeCategory(tv.Type)
			}
		}
	}

	return Pattern{Kind: "TypeAssertExpr", Attrs: attrs}
}

// ExtractStarExprPattern extracts a pattern from a star expression (pointer)
func ExtractStarExprPattern(n *ast.StarExpr, pkg *packages.Package) Pattern {
	attrs := map[string]string{}

	if pkg != nil && pkg.TypesInfo != nil {
		if tv, ok := pkg.TypesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["operand_type"] = typeCategory(tv.Type)
		}
	}

	return Pattern{Kind: "StarExpr", Attrs: attrs}
}

// ExtractArrayTypePattern extracts a pattern from an array type
func ExtractArrayTypePattern(n *ast.ArrayType, pkg *packages.Package) Pattern {
	attrs := map[string]string{
		"is_slice": fmt.Sprintf("%t", n.Len == nil),
	}

	if pkg != nil && pkg.TypesInfo != nil {
		if tv, ok := pkg.TypesInfo.Types[n.Elt]; ok && tv.Type != nil {
			attrs["elt_type"] = typeCategory(tv.Type)
		}
	}

	return Pattern{Kind: "ArrayType", Attrs: attrs}
}

// ExtractStructTypePattern extracts a pattern from a struct type
func ExtractStructTypePattern(n *ast.StructType, pkg *packages.Package) Pattern {
	attrs := map[string]string{}
	if n.Fields != nil {
		attrs["field_count"] = fmt.Sprintf("%d", len(n.Fields.List))
	}
	return Pattern{Kind: "StructType", Attrs: attrs}
}

// ExtractInterfaceTypePattern extracts a pattern from an interface type
func ExtractInterfaceTypePattern(n *ast.InterfaceType, pkg *packages.Package) Pattern {
	attrs := map[string]string{}
	if n.Methods != nil {
		attrs["method_count"] = fmt.Sprintf("%d", len(n.Methods.List))
	}
	return Pattern{Kind: "InterfaceType", Attrs: attrs}
}

// ExtractChanTypePattern extracts a pattern from a channel type
func ExtractChanTypePattern(n *ast.ChanType, pkg *packages.Package) Pattern {
	attrs := map[string]string{}
	switch n.Dir {
	case ast.SEND:
		attrs["dir"] = "send"
	case ast.RECV:
		attrs["dir"] = "recv"
	default:
		attrs["dir"] = "both"
	}
	return Pattern{Kind: "ChanType", Attrs: attrs}
}

// ExtractFuncTypePattern extracts a pattern from a function type
func ExtractFuncTypePattern(n *ast.FuncType, pkg *packages.Package) Pattern {
	attrs := map[string]string{}
	if n.Params != nil {
		attrs["param_count"] = fmt.Sprintf("%d", len(n.Params.List))
	}
	if n.Results != nil {
		attrs["result_count"] = fmt.Sprintf("%d", len(n.Results.List))
	}
	return Pattern{Kind: "FuncType", Attrs: attrs}
}

// ExtractDeferStmtPattern extracts a pattern from a defer statement
func ExtractDeferStmtPattern(n *ast.DeferStmt, pkg *packages.Package) Pattern {
	return Pattern{Kind: "DeferStmt", Attrs: map[string]string{}}
}

// ExtractGoStmtPattern extracts a pattern from a go statement
func ExtractGoStmtPattern(n *ast.GoStmt, pkg *packages.Package) Pattern {
	return Pattern{Kind: "GoStmt", Attrs: map[string]string{}}
}

// ExtractSendStmtPattern extracts a pattern from a send statement
func ExtractSendStmtPattern(n *ast.SendStmt, pkg *packages.Package) Pattern {
	return Pattern{Kind: "SendStmt", Attrs: map[string]string{}}
}

// ExtractSelectStmtPattern extracts a pattern from a select statement
func ExtractSelectStmtPattern(n *ast.SelectStmt, pkg *packages.Package) Pattern {
	return Pattern{Kind: "SelectStmt", Attrs: map[string]string{}}
}

// ExtractEllipsisPattern extracts a pattern from an ellipsis (variadic)
func ExtractEllipsisPattern(n *ast.Ellipsis, pkg *packages.Package) Pattern {
	return Pattern{Kind: "Ellipsis", Attrs: map[string]string{}}
}
