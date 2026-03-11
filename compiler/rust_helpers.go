package compiler

import (
	"go/ast"
	"go/token"
	"go/types"
)

var rustDestTypes = []string{"i8", "i16", "i32", "i64", "u8", "u16", "Rc<dyn Any>", "String", "i32"}

var rustTypesMap = map[string]string{
	"int8":    rustDestTypes[0],
	"int16":   rustDestTypes[1],
	"int32":   rustDestTypes[2],
	"int64":   rustDestTypes[3],
	"uint8":   rustDestTypes[4],
	"uint16":  rustDestTypes[5],
	"uint32":  "u32",
	"uint64":  "u64",
	"byte":    "u8", // Go byte is alias for uint8
	"any":     rustDestTypes[6],
	"string":  rustDestTypes[7],
	"int":     rustDestTypes[8],
	"bool":    "bool",
	"float32": "f32",
	"float64": "f64",
}

// escapeRustKeyword escapes Rust reserved keywords with r# prefix
func escapeRustKeyword(name string) string {
	// Rust reserved keywords that might conflict with Go identifiers
	rustKeywords := map[string]bool{
		"as": true, "break": true, "const": true, "continue": true,
		"crate": true, "else": true, "enum": true, "extern": true,
		// Note: "false" and "true" are NOT escaped - they're boolean literals
		"fn": true, "for": true, "if": true,
		"impl": true, "in": true, "let": true, "loop": true,
		"match": true, "mod": true, "move": true, "mut": true,
		"pub": true, "ref": true, "return": true, "self": true,
		"Self": true, "static": true, "struct": true, "super": true,
		"trait": true, "type": true, "unsafe": true,
		"use": true, "where": true, "while": true,
		// Reserved for future use
		"abstract": true, "async": true, "await": true, "become": true,
		"box": true, "do": true, "final": true, "macro": true,
		"override": true, "priv": true, "try": true, "typeof": true,
		"unsized": true, "virtual": true, "yield": true,
	}
	if rustKeywords[name] {
		return "r#" + name
	}
	return name
}

// getRustValueTypeCast returns the Rust downcast expression for a map value type
func getRustValueTypeCast(t types.Type) string {
	if basicType, ok := t.(*types.Basic); ok {
		switch basicType.Kind() {
		case types.Int, types.Int32:
			return "i32"
		case types.Int8:
			return "i8"
		case types.Int16:
			return "i16"
		case types.Int64:
			return "i64"
		case types.Uint8:
			return "u8"
		case types.Uint16:
			return "u16"
		case types.Uint32:
			return "u32"
		case types.Uint64:
			return "u64"
		case types.String:
			return "String"
		case types.Bool:
			return "bool"
		case types.Float32:
			return "f32"
		case types.Float64:
			return "f64"
		}
	}
	if iface, ok := t.(*types.Interface); ok && iface.Empty() {
		return "Rc<dyn Any>"
	}
	// Handle slice types - recursively get element type
	if slice, ok := t.(*types.Slice); ok {
		elemType := getRustValueTypeCast(slice.Elem())
		return "Vec<" + elemType + ">"
	}
	// Handle map types
	if _, ok := t.Underlying().(*types.Map); ok {
		return "hmap::HashMap"
	}
	// Handle pointer types (after pointer lowering, stored as i32 pool index)
	if _, ok := t.(*types.Pointer); ok {
		return "i32"
	}
	if named, ok := t.(*types.Named); ok {
		if _, isStruct := named.Underlying().(*types.Struct); isStruct {
			return named.Obj().Name()
		}
	}
	return "Rc<dyn Any>"
}

// getRustKeyCast returns a suffix to cast a map key to the correct Rust type.
// For types matching Rust defaults (i32, bool, f64), returns "".
func getRustKeyCast(keyType types.Type) string {
	if basic, isBasic := keyType.Underlying().(*types.Basic); isBasic {
		switch basic.Kind() {
		case types.Int8:
			return " as i8"
		case types.Int16:
			return " as i16"
		case types.Int64:
			return " as i64"
		case types.Uint8:
			return " as u8"
		case types.Uint16:
			return " as u16"
		case types.Uint32:
			return " as u32"
		case types.Uint64:
			return " as u64"
		case types.Float32:
			return " as f32"
		}
	}
	return ""
}

// isRustStringKey returns true if the map key type is string.
func isRustStringKey(keyType types.Type) bool {
	if basic, isBasic := keyType.Underlying().(*types.Basic); isBasic {
		return basic.Kind() == types.String
	}
	return false
}

// exprToRustString converts a simple expression (BasicLit, Ident, IndexExpr, SelectorExpr) to its Rust string representation
func exprToRustString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			// Go string literal to Rust: "foo" -> "foo".to_string() wrapped in Rc
			// But for map keys we just need the raw value for now
			return e.Value
		}
		return e.Value
	case *ast.Ident:
		return e.Name
	case *ast.IndexExpr:
		// Handle slice/array access like sliceOfMaps[0]
		xStr := exprToRustString(e.X)
		indexStr := exprToRustString(e.Index)
		if xStr != "" && indexStr != "" {
			return xStr + "[" + indexStr + "]"
		}
		return ""
	case *ast.SelectorExpr:
		xStr := exprToRustString(e.X)
		if xStr != "" {
			return xStr + "." + e.Sel.Name
		}
		return ""
	default:
		return ""
	}
}

// isCopyType checks if a Go type maps to a Rust Copy type (primitives)
func isCopyType(t types.Type) bool {
	if t == nil {
		return false
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch u.Kind() {
		case types.Bool, types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64,
			types.Float32, types.Float64:
			return true
		}
	}
	return false
}
