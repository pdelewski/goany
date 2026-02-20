package compiler

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"
)

var cppTypesMap = map[string]string{
	"int8":    "std::int8_t",
	"int16":   "std::int16_t",
	"int32":   "std::int32_t",
	"int64":   "std::int64_t",
	"uint8":   "std::uint8_t",
	"uint16":  "std::uint16_t",
	"uint32":  "std::uint32_t",
	"uint64":  "std::uint64_t",
	"byte":    "std::uint8_t", // Go byte is alias for uint8
	"float32": "float",
	"float64": "double",
	"any":     "std::any",
	"string":  "std::string",
}

// mixedIndexOp represents a single index operation in a chain
type mixedIndexOp struct {
	accessType   string // "map" or "slice"
	keyExpr      string // Key/index expression
	keyCastPfx   string // Cast prefix for map key
	keyCastSfx   string // Cast suffix for map key
	valueCppType string // C++ type of the value at this level
	tempVarName  string // Temp variable name (only for map access)
	mapVarExpr   string // The expression to call hashMapGet on
}

// pendingHashSpec stores info for deferred hash specialization generation
type pendingHashSpec struct {
	structName string   // Struct name (unqualified)
	pkgName    string   // Package/namespace name (empty for main)
	fieldNames []string // Field names of the struct
}

// getCppTypeName returns the C++ type name for a Go type
func getCppTypeName(t types.Type) string {
	if basic, isBasic := t.Underlying().(*types.Basic); isBasic {
		switch basic.Kind() {
		case types.Int:
			return "int"
		case types.String:
			return "std::string"
		case types.Bool:
			return "bool"
		case types.Float64:
			return "double"
		case types.Float32:
			return "float"
		case types.Int8:
			return "std::int8_t"
		case types.Int16:
			return "std::int16_t"
		case types.Int32:
			return "std::int32_t"
		case types.Int64:
			return "std::int64_t"
		case types.Uint8: // also types.Byte
			return "std::uint8_t"
		case types.Uint16:
			return "std::uint16_t"
		case types.Uint32:
			return "std::uint32_t"
		case types.Uint64:
			return "std::uint64_t"
		}
	}
	// Handle slice types - recursively get element type
	if slice, ok := t.(*types.Slice); ok {
		elemType := getCppTypeName(slice.Elem())
		return "std::vector<" + elemType + ">"
	}
	// Handle map types
	if _, ok := t.Underlying().(*types.Map); ok {
		return "hmap::HashMap"
	}
	if named, ok := t.(*types.Named); ok {
		if _, isStruct := named.Underlying().(*types.Struct); isStruct {
			name := named.Obj().Name()
			// Qualify with namespace if from another package (not main)
			if pkg := named.Obj().Pkg(); pkg != nil {
				pkgName := pkg.Name()
				if pkgName != "" && pkgName != "main" {
					if _, isNs := namespaces[pkgName]; isNs {
						name = pkgName + "::" + name
					}
				}
			}
			return name
		}
	}
	// Handle function types → std::function<ret(params)>
	if sig, ok := t.Underlying().(*types.Signature); ok {
		var params []string
		for i := 0; i < sig.Params().Len(); i++ {
			params = append(params, getCppTypeName(sig.Params().At(i).Type()))
		}
		retType := "void"
		if sig.Results().Len() == 1 {
			retType = getCppTypeName(sig.Results().At(0).Type())
		} else if sig.Results().Len() > 1 {
			var rets []string
			for i := 0; i < sig.Results().Len(); i++ {
				rets = append(rets, getCppTypeName(sig.Results().At(i).Type()))
			}
			retType = "std::tuple<" + strings.Join(rets, ", ") + ">"
		}
		return "std::function<" + retType + "(" + strings.Join(params, ", ") + ")>"
	}
	return "std::any"
}

// getCppKeyCast returns prefix/suffix to cast a map key expression to the correct C++ type.
func getCppKeyCast(keyType types.Type) (string, string) {
	if basic, isBasic := keyType.Underlying().(*types.Basic); isBasic {
		switch basic.Kind() {
		case types.String:
			return "std::string(", ")"
		case types.Int8:
			return "(std::int8_t)(", ")"
		case types.Int16:
			return "(std::int16_t)(", ")"
		case types.Int32:
			return "(std::int32_t)(", ")"
		case types.Int64:
			return "(std::int64_t)(", ")"
		case types.Uint8:
			return "(uint8_t)(", ")"
		case types.Uint16:
			return "(uint16_t)(", ")"
		case types.Uint32:
			return "(uint32_t)(", ")"
		case types.Uint64:
			return "(uint64_t)(", ")"
		case types.Float32:
			return "(float)(", ")"
		}
	}
	return "", ""
}

// exprToCppString converts a simple expression (BasicLit, Ident, IndexExpr) to its C++ string representation
// Used for extracting map key expressions and slice access expressions from AST nodes
func exprToCppString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind == token.STRING {
			// Go string literal to C++ - keep the quotes
			return e.Value
		}
		return e.Value
	case *ast.Ident:
		return e.Name
	case *ast.IndexExpr:
		// Handle slice/array access like sliceOfMaps[0]
		xStr := exprToCppString(e.X)
		indexStr := exprToCppString(e.Index)
		if xStr != "" && indexStr != "" {
			return xStr + "[" + indexStr + "]"
		}
		return ""
	case *ast.SelectorExpr:
		xStr := exprToCppString(e.X)
		if xStr != "" {
			return xStr + "." + e.Sel.Name
		}
		return ""
	default:
		return ""
	}
}
