package compiler

import (
	"fmt"
	"go/ast"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	goanyrt "goany/runtime"

	"golang.org/x/tools/go/packages"
)

// RustEmitter implements the Emitter interface using a shift/reduce IRForestBuilder
// architecture for Rust code generation.
type RustEmitter struct {
	fs              *IRForestBuilder
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string
	RuntimePackages map[string]string
	Opt             RustOptState
	file            *os.File
	Emitter
	pkg            *packages.Package
	currentPackage string
	indent         int
	numFuncResults int
	// Map assignment detection
	lastIndexXCode   string
	lastIndexKeyCode string
	mapAssignVar     string
	mapAssignKey     string
	structKeyTypes   map[string]string
	// For loop components (stacks for nesting)
	forInitStack []string
	forCondStack []string
	forPostStack []string
	forCondNodes []IRNode
	forBodyNodes []IRNode
	// If statement components (stacks for nesting)
	ifInitStack []string
	ifCondStack []string
	ifBodyStack []string
	ifElseStack []string
	// Parallel node stacks for tree preservation
	ifInitNodes []IRNode
	ifCondNodes []IRNode
	ifBodyNodes []IRNode
	ifElseNodes []IRNode
	// Rust-specific
	forwardDecl      bool
	nestedMapCounter int
	typeAliasMap     map[string]string
	aliases          map[string]Alias
	currentAliasName string
	funcReturnType      types.Type
	savedFuncRetTypes   []types.Type // stack for nested closures
	rangeVarCounter     int
	inAssignLhs         bool // true when visiting LHS of assignment
	callExprArgDepth    int  // >0 when inside a call expression argument
	compositeLitDepth   int  // >0 when inside a composite literal (struct/slice init)
	localClosureBodies  map[string]string // maps local closure variable names to their body code
	outputPath          string
	outputs             []OutputEntry
}

func (e *RustEmitter) SetFile(file *os.File) { e.file = file }
func (e *RustEmitter) GetFile() *os.File     { return e.file }

// rustIndent returns indentation string for the given level.
func rustIndent(indent int) string {
	return strings.Repeat("    ", indent/2)
}

// injectBeforeLastBrace inserts text before the last closing brace in a block string.
func injectBeforeLastBrace(body string, text string) string {
	lastBrace := strings.LastIndex(body, "}")
	if lastBrace < 0 {
		return body + text
	}
	return body[:lastBrace] + text + body[lastBrace:]
}

// ============================================================
// Helper methods
// ============================================================

func (e *RustEmitter) getExprGoType(expr ast.Expr) types.Type {
	if e.pkg == nil || e.pkg.TypesInfo == nil {
		return nil
	}
	tv := e.pkg.TypesInfo.Types[expr]
	return tv.Type
}

func (e *RustEmitter) isMapTypeExpr(expr ast.Expr) bool {
	if e.pkg == nil || e.pkg.TypesInfo == nil {
		return false
	}
	tv := e.pkg.TypesInfo.Types[expr]
	if tv.Type != nil {
		_, ok := tv.Type.Underlying().(*types.Map)
		if ok {
			return true
		}
	}
	if ident, ok := expr.(*ast.Ident); ok {
		if obj := e.pkg.TypesInfo.ObjectOf(ident); obj != nil {
			_, isMap := obj.Type().Underlying().(*types.Map)
			return isMap
		}
	}
	return false
}

// collectMapIndexChain walks an IndexExpr chain from outermost to innermost,
// collecting all IndexExpr levels. Returns the chain from innermost to outermost.
// For `a[x][y][z]`, returns [{X:a, Index:x}, {X:a[x], Index:y}, {X:a[x][y], Index:z}]
func collectMapIndexChain(expr ast.Expr) []*ast.IndexExpr {
	var chain []*ast.IndexExpr
	for {
		idx, ok := expr.(*ast.IndexExpr)
		if !ok {
			break
		}
		chain = append([]*ast.IndexExpr{idx}, chain...)
		expr = idx.X
	}
	return chain
}

// emitMapSliceAssign handles map-chain-then-slice assignments like:
//   map[key][sliceIdx] = value
//   map[k1][k2][sliceIdx] = value
// It generates extract-modify-put-back code.
// Returns the generated code or "" if the pattern doesn't match.
func (e *RustEmitter) emitMapSliceAssign(outerIdx *ast.IndexExpr, rhsStr string, ind string) string {
	// Collect the full index chain from LHS
	chain := collectMapIndexChain(outerIdx)
	if len(chain) < 2 {
		return ""
	}

	// The last index is the slice index (non-map)
	// The chain before that should all be map accesses
	// Find how many leading indices are map accesses
	mapCount := 0
	for i, idx := range chain {
		if i == len(chain)-1 {
			break // last one is the slice index
		}
		if e.isMapTypeExpr(idx.X) {
			mapCount++
		} else {
			break
		}
	}

	if mapCount == 0 {
		return ""
	}

	// Verify the last chain element is a non-map access (slice index)
	lastChainX := chain[len(chain)-1].X
	if e.isMapTypeExpr(lastChainX) {
		return "" // all indices are map accesses, not a map-then-slice pattern
	}

	// Build hashMapGet chain for reading the slice
	// Start from the root map variable
	rootMapExpr := chain[0].X
	rootMapName := exprToString(rootMapExpr)

	type mapLevel struct {
		mapExpr  ast.Expr // the map expression
		keyStr   string   // the key as string
		keyExpr  string   // Rc::new(...) key expression
		valType  string   // Rust type of the value
	}

	var levels []mapLevel
	currentExpr := rootMapExpr
	for i := 0; i < mapCount; i++ {
		idx := chain[i]
		mapGoType := e.getExprGoType(currentExpr)
		if mapGoType == nil {
			return ""
		}
		mapUnderlying, ok := mapGoType.Underlying().(*types.Map)
		if !ok {
			return ""
		}
		keyCast := getRustKeyCast(mapUnderlying.Key())
		keyIsStr := isRustStringKey(mapUnderlying.Key())
		keyStr := exprToString(idx.Index)
		var keyExpr string
		if keyIsStr {
			keyExpr = fmt.Sprintf("Rc::new(%s.to_string())", keyStr)
		} else {
			keyExpr = fmt.Sprintf("Rc::new(%s%s)", keyStr, keyCast)
		}
		valType := e.qualifiedRustTypeName(mapUnderlying.Elem())
		levels = append(levels, mapLevel{
			mapExpr: currentExpr,
			keyStr:  keyStr,
			keyExpr: keyExpr,
			valType: valType,
		})
		currentExpr = idx // advance to the result type
	}

	// Get the final slice type (the type of the innermost map's value)
	sliceType := levels[len(levels)-1].valType

	// Collect all trailing slice indices (there may be more than one, e.g. map[k][0][1])
	var sliceIndices []string
	for i := mapCount; i < len(chain); i++ {
		sliceIndices = append(sliceIndices, exprToString(chain[i].Index))
	}

	e.nestedMapCounter++
	sliceTempVar := fmt.Sprintf("__temp_%d", e.nestedMapCounter)

	var sb strings.Builder

	// Build the hashMapGet chain to read the slice
	getExpr := fmt.Sprintf("%s.clone()", rootMapName)
	if e.Opt.OptimizeRefs {
		getExpr = "&" + rootMapName
		e.Opt.RefOptPass.TransformCount++
	}
	for i, lvl := range levels {
		castType := "hmap::HashMap"
		if i == len(levels)-1 {
			castType = sliceType
		}
		innerGetExpr := fmt.Sprintf("hmap::hashMapGet(%s, %s).downcast_ref::<%s>().unwrap().clone()",
			getExpr, lvl.keyExpr, castType)
		if e.Opt.OptimizeRefs && i < len(levels)-1 {
			getExpr = "&" + innerGetExpr
			e.Opt.RefOptPass.TransformCount++
		} else {
			getExpr = innerGetExpr
		}
	}
	sb.WriteString(fmt.Sprintf("%slet mut %s = %s;\n", ind, sliceTempVar, getExpr))

	// Build the slice indexing chain (e.g. __temp[0 as usize][1 as usize] = value)
	indexChain := sliceTempVar
	for _, idx := range sliceIndices {
		indexChain = fmt.Sprintf("%s[%s as usize]", indexChain, idx)
	}
	sb.WriteString(fmt.Sprintf("%s%s = %s;\n", ind, indexChain, rhsStr))

	// Write back: propagate changes from innermost map to outermost
	// For 1 map level: mapName = hashMapSet(mapName, key, Rc::new(sliceTemp))
	// For 2 map levels: extract inner map, set slice, set inner back into outer
	if len(levels) == 1 {
		sb.WriteString(fmt.Sprintf("%s%s = hmap::hashMapSet(%s.clone(), %s, Rc::new(%s));\n",
			ind, rootMapName, rootMapName, levels[0].keyExpr, sliceTempVar))
	} else {
		// Multi-level: extract each intermediate map, modify, write back
		tempVars := make([]string, len(levels))
		// Extract maps from outermost to one-before-innermost
		extractExpr := fmt.Sprintf("%s.clone()", rootMapName)
		if e.Opt.OptimizeRefs {
			extractExpr = "&" + rootMapName
			e.Opt.RefOptPass.TransformCount++
		}
		for i := 0; i < len(levels)-1; i++ {
			e.nestedMapCounter++
			tempVars[i] = fmt.Sprintf("__temp_%d", e.nestedMapCounter)
			sb.WriteString(fmt.Sprintf("%slet mut %s = hmap::hashMapGet(%s, %s).downcast_ref::<hmap::HashMap>().unwrap().clone();\n",
				ind, tempVars[i], extractExpr, levels[i].keyExpr))
			extractExpr = fmt.Sprintf("%s.clone()", tempVars[i])
			if e.Opt.OptimizeRefs {
				extractExpr = "&" + tempVars[i]
				e.Opt.RefOptPass.TransformCount++
			}
		}
		// Set slice into innermost map
		innermostMap := tempVars[len(levels)-2]
		sb.WriteString(fmt.Sprintf("%s%s = hmap::hashMapSet(%s.clone(), %s, Rc::new(%s));\n",
			ind, innermostMap, innermostMap, levels[len(levels)-1].keyExpr, sliceTempVar))
		// Propagate back up
		for i := len(levels) - 2; i > 0; i-- {
			parentMap := tempVars[i-1]
			sb.WriteString(fmt.Sprintf("%s%s = hmap::hashMapSet(%s.clone(), %s, Rc::new(%s));\n",
				ind, parentMap, parentMap, levels[i].keyExpr, tempVars[i]))
		}
		// Set outermost
		sb.WriteString(fmt.Sprintf("%s%s = hmap::hashMapSet(%s.clone(), %s, Rc::new(%s));\n",
			ind, rootMapName, rootMapName, levels[0].keyExpr, tempVars[0]))
	}

	return sb.String()
}

// emitMapAssign generates code for all map assignment patterns.
// Handles: simple m[k]=v, nested map-of-maps m[k1][k2]=v, and mixed chains like map[k][sliceIdx][k2]=v.
func (e *RustEmitter) emitMapAssign(node *ast.AssignStmt, rhsStr string, ind string) string {
	outerIdxExpr := node.Lhs[0].(*ast.IndexExpr)

	// Get the key info for the outermost (final) map access
	mapGoType := e.getExprGoType(outerIdxExpr.X)
	keyCast := ""
	keyIsStr := false
	if mapGoType != nil {
		if mapUnderlying, ok := mapGoType.Underlying().(*types.Map); ok {
			keyCast = getRustKeyCast(mapUnderlying.Key())
			keyIsStr = isRustStringKey(mapUnderlying.Key())
		}
	}
	var keyExpr string
	if keyIsStr {
		keyExpr = fmt.Sprintf("Rc::new(%s)", e.mapAssignKey)
	} else {
		keyExpr = fmt.Sprintf("Rc::new(%s%s)", e.mapAssignKey, keyCast)
	}

	// Collect the full chain of IndexExprs from the LHS
	chain := collectMapIndexChain(outerIdxExpr)
	if len(chain) <= 1 {
		// Simple case: m[k] = v
		return fmt.Sprintf("%s%s = hmap::hashMapSet(%s.clone(), %s, Rc::new(%s));\n",
			ind, e.mapAssignVar, e.mapAssignVar, keyExpr, rhsStr)
	}

	// Multi-level: find the root map (the first map in the chain)
	// Walk from the beginning to find the first map access
	rootMapIdx := -1
	for i := 0; i < len(chain); i++ {
		if e.isMapTypeExpr(chain[i].X) {
			rootMapIdx = i
			break
		}
	}
	if rootMapIdx < 0 || rootMapIdx > 0 {
		// No root map found, or root map is accessed through slice indices only.
		// Simple case: the simple hashMapSet assignment works because Rust
		// can assign to slice elements (vec[idx] = ...) directly.
		return fmt.Sprintf("%s%s = hmap::hashMapSet(%s.clone(), %s, Rc::new(%s));\n",
			ind, e.mapAssignVar, e.mapAssignVar, keyExpr, rhsStr)
	}

	rootMapExpr := chain[rootMapIdx].X
	rootMapName := exprToString(rootMapExpr)
	rootMapGoType := e.getExprGoType(rootMapExpr)
	if rootMapGoType == nil {
		return fmt.Sprintf("%s%s = hmap::hashMapSet(%s.clone(), %s, Rc::new(%s));\n",
			ind, e.mapAssignVar, e.mapAssignVar, keyExpr, rhsStr)
	}

	rootMapUnderlying, ok := rootMapGoType.Underlying().(*types.Map)
	if !ok {
		return fmt.Sprintf("%s%s = hmap::hashMapSet(%s.clone(), %s, Rc::new(%s));\n",
			ind, e.mapAssignVar, e.mapAssignVar, keyExpr, rhsStr)
	}

	// Get the root map's key info
	rootKeyCast := getRustKeyCast(rootMapUnderlying.Key())
	rootKeyIsStr := isRustStringKey(rootMapUnderlying.Key())
	rootKey := exprToString(chain[rootMapIdx].Index)
	var rootKeyExpr string
	if rootKeyIsStr {
		rootKeyExpr = fmt.Sprintf("Rc::new(%s.to_string())", rootKey)
	} else {
		rootKeyExpr = fmt.Sprintf("Rc::new(%s%s)", rootKey, rootKeyCast)
	}

	// Extract the value type from root map
	rootValType := e.qualifiedRustTypeName(rootMapUnderlying.Elem())

	e.nestedMapCounter++
	tempVar := fmt.Sprintf("__temp_%d", e.nestedMapCounter)

	var sb strings.Builder

	// Extract from root map
	rootMapRef := rootMapName + ".clone()"
	if e.Opt.OptimizeRefs {
		rootMapRef = "&" + rootMapName
		e.Opt.RefOptPass.TransformCount++
	}
	sb.WriteString(fmt.Sprintf("%slet mut %s = hmap::hashMapGet(%s, %s).downcast_ref::<%s>().unwrap().clone();\n",
		ind, tempVar, rootMapRef, rootKeyExpr, rootValType))

	// Build the intermediate index chain between root and the final map assign
	// For chain[rootMapIdx+1 .. len(chain)-1], apply indices to tempVar
	indexPrefix := tempVar
	for i := rootMapIdx + 1; i < len(chain)-1; i++ {
		idx := chain[i]
		if e.isMapTypeExpr(idx.X) {
			// This intermediate level is a map access on the temp
			intermMapGoType := e.getExprGoType(idx.X)
			if intermMapGoType != nil {
				if intermMapUnderlying, ok := intermMapGoType.Underlying().(*types.Map); ok {
					intermKeyCast := getRustKeyCast(intermMapUnderlying.Key())
					intermKeyIsStr := isRustStringKey(intermMapUnderlying.Key())
					intermKey := exprToString(idx.Index)
					var intermKeyExpr string
					if intermKeyIsStr {
						intermKeyExpr = fmt.Sprintf("Rc::new(%s.to_string())", intermKey)
					} else {
						intermKeyExpr = fmt.Sprintf("Rc::new(%s%s)", intermKey, intermKeyCast)
					}
					intermValType := e.qualifiedRustTypeName(intermMapUnderlying.Elem())
					e.nestedMapCounter++
					innerTemp := fmt.Sprintf("__temp_%d", e.nestedMapCounter)
					intermMapRef := indexPrefix + ".clone()"
					if e.Opt.OptimizeRefs {
						intermMapRef = "&" + indexPrefix
						e.Opt.RefOptPass.TransformCount++
					}
					sb.WriteString(fmt.Sprintf("%slet mut %s = hmap::hashMapGet(%s, %s).downcast_ref::<%s>().unwrap().clone();\n",
						ind, innerTemp, intermMapRef, intermKeyExpr, intermValType))
					indexPrefix = innerTemp
				}
			}
		} else {
			// Slice index on the temp
			sliceIdx := exprToString(idx.Index)
			indexPrefix = fmt.Sprintf("%s[%s as usize]", indexPrefix, sliceIdx)
		}
	}

	// Apply the final map assign
	sb.WriteString(fmt.Sprintf("%s%s = hmap::hashMapSet(%s.clone(), %s, Rc::new(%s));\n",
		ind, indexPrefix, indexPrefix, keyExpr, rhsStr))

	// Write back: propagate temp vars to root
	// We need to write back all intermediate temps and finally the root
	// For now, just write back the root
	sb.WriteString(fmt.Sprintf("%s%s = hmap::hashMapSet(%s.clone(), %s, Rc::new(%s));\n",
		ind, rootMapName, rootMapName, rootKeyExpr, tempVar))

	return sb.String()
}

// mapGoTypeToRust converts a Go type string to its Rust equivalent.
func (e *RustEmitter) mapGoTypeToRust(goType string) string {
	if rustType, ok := rustTypesMap[goType]; ok {
		return rustType
	}
	return goType
}

// qualifiedRustTypeName converts a Go type to its fully-qualified Rust type name.
// For cross-package named types, it adds the Rust module prefix (e.g., ast::Statement).
func (e *RustEmitter) qualifiedRustTypeName(t types.Type) string {
	// Handle named types from other packages
	if named, ok := t.(*types.Named); ok {
		if _, isStruct := named.Underlying().(*types.Struct); isStruct {
			typeName := named.Obj().Name()
			if named.Obj().Pkg() != nil && e.pkg != nil && named.Obj().Pkg() != e.pkg.Types {
				return named.Obj().Pkg().Name() + "::" + typeName
			}
			return typeName
		}
	}
	// Handle slice of cross-package types
	if slice, ok := t.(*types.Slice); ok {
		elemType := e.qualifiedRustTypeName(slice.Elem())
		return "Vec<" + elemType + ">"
	}
	return getRustValueTypeCast(t)
}

// castSmallIntFieldValue wraps a value expression with an `as` cast when the
// target field type is a small integer (i8, u8, i16, u16). This handles Go
// untyped constants that are emitted as i32 but assigned to small-int fields.
func (e *RustEmitter) castSmallIntFieldValue(fieldType types.Type, val string) string {
	if basic, ok := fieldType.Underlying().(*types.Basic); ok {
		switch basic.Kind() {
		case types.Int8:
			return fmt.Sprintf("(%s as i8)", val)
		case types.Uint8:
			return fmt.Sprintf("(%s as u8)", val)
		case types.Int16:
			return fmt.Sprintf("(%s as i16)", val)
		case types.Uint16:
			return fmt.Sprintf("(%s as u16)", val)
		}
	}
	return val
}

// wrapSmallIntCastTree wraps an IRNode in a `(node as castType)` tree when the
// target field type is a small integer (i8, u8, i16, u16). Unlike castSmallIntFieldValue
// which returns a flat string, this preserves the inner tree structure (e.g., OptCallArg metadata).
func (e *RustEmitter) wrapSmallIntCastTree(fieldType types.Type, node IRNode) IRNode {
	if basic, ok := fieldType.Underlying().(*types.Basic); ok {
		var castType string
		switch basic.Kind() {
		case types.Int8:
			castType = "i8"
		case types.Uint8:
			castType = "u8"
		case types.Int16:
			castType = "i16"
		case types.Uint16:
			castType = "u16"
		default:
			return node
		}
		return IRTree(CallExpression, KindExpr,
			Leaf(LeftParen, "("),
			node,
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "as"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, castType),
			Leaf(RightParen, ")"),
		)
	}
	return node
}

// cloneStructFieldValue adds .clone() to non-Copy struct field values that are
// simple identifiers (variables). This prevents move-in-loop errors in Rust.
func (e *RustEmitter) cloneStructFieldValue(fieldType types.Type, val string) string {
	if !isCopyType(fieldType) {
		// Don't clone values that are already calls (end with ")"), literals, or already cloned
		trimmed := strings.TrimSpace(val)
		if !strings.HasSuffix(trimmed, ")") &&
			!strings.HasSuffix(trimmed, ".clone()") &&
			!strings.HasPrefix(trimmed, "\"") &&
			!strings.HasPrefix(trimmed, "vec![") &&
			!strings.HasPrefix(trimmed, "Vec::") &&
			trimmed != "Default::default()" &&
			trimmed != "true" && trimmed != "false" {
			return val + ".clone()"
		}
	}
	return val
}

// getMapKeyTypeConst returns the integer constant for a map's key type.
func (e *RustEmitter) getMapKeyTypeConst(mapType *ast.MapType) int {
	if ident, ok := mapType.Key.(*ast.Ident); ok {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if tv, ok2 := e.pkg.TypesInfo.Types[ident]; ok2 && tv.Type != nil {
				if basic, isBasic := tv.Type.Underlying().(*types.Basic); isBasic {
					switch basic.Kind() {
					case types.String:
						return 1
					case types.Int:
						return 2
					case types.Bool:
						return 3
					case types.Int8:
						return 4
					case types.Int16:
						return 5
					case types.Int32:
						return 6
					case types.Int64:
						return 7
					case types.Uint8:
						return 8
					case types.Uint16:
						return 9
					case types.Uint32:
						return 10
					case types.Uint64:
						return 11
					case types.Float32:
						return 12
					case types.Float64:
						return 13
					}
				}
				if _, isStruct := tv.Type.Underlying().(*types.Struct); isStruct {
					if e.structKeyTypes == nil {
						e.structKeyTypes = make(map[string]string)
					}
					rustPath := "super::" + ident.Name
					if named, ok3 := tv.Type.(*types.Named); ok3 {
						if structPkg := named.Obj().Pkg(); structPkg != nil {
							pkgName := structPkg.Name()
							if pkgName != "" && pkgName != "main" {
								rustPath = "super::" + pkgName + "::" + ident.Name
							}
						}
					}
					e.structKeyTypes[ident.Name] = rustPath
					return 100
				}
			}
		}
	}
	return 1
}

// rustDefaultForGoType returns the Rust default value for a Go type.
func (e *RustEmitter) rustDefaultForGoType(t types.Type) string {
	if t == nil {
		return "Default::default()"
	}
	typeStr := t.String()
	if strings.HasPrefix(typeStr, "[]") {
		return "Vec::new()"
	}
	if strings.Contains(typeStr, "interface{}") || strings.Contains(typeStr, "interface {") || typeStr == "any" {
		return "Rc::new(0_i32)"
	}
	if strings.HasPrefix(typeStr, "func(") {
		return "Rc::new(|_| {})"
	}
	switch typeStr {
	case "int", "int8", "int16", "int32", "int64":
		return "0"
	case "uint8", "uint16", "uint32", "uint64":
		return "0"
	case "float32":
		return "0.0_f32"
	case "float64":
		return "0.0"
	case "bool":
		return "false"
	case "string":
		return "String::new()"
	}
	if named, ok := t.(*types.Named); ok {
		if _, isStruct := named.Underlying().(*types.Struct); isStruct {
			return named.Obj().Name() + "::default()"
		}
		if _, isSlice := named.Underlying().(*types.Slice); isSlice {
			return "Vec::new()"
		}
	}
	return "Default::default()"
}

// rustDefaultForRustType returns the default value for a Rust type string.
func rustDefaultForRustType(rustType string) string {
	switch rustType {
	case "i8", "i16", "i32", "i64", "u8", "u16", "u32", "u64":
		return "0"
	case "f32":
		return "0.0_f32"
	case "f64":
		return "0.0"
	case "bool":
		return "false"
	case "String":
		return "String::new()"
	}
	if strings.HasPrefix(rustType, "Vec<") {
		// Extract element type for typed Vec::new() (needed when used as default in vec![...])
		elemType := rustType[4 : len(rustType)-1]
		return fmt.Sprintf("Vec::<%s>::new()", elemType)
	}
	if rustType == "hmap::HashMap" {
		return "hmap::newHashMap(1)"
	}
	return "Default::default()"
}

// isTypeConversion checks if a function name represents a type conversion.
func (e *RustEmitter) isTypeConversion(funName string) (bool, string) {
	typeConversions := map[string]string{
		"int8": "i8", "int16": "i16", "int32": "i32", "int64": "i64",
		"int": "i32", "uint8": "u8", "uint16": "u16", "uint32": "u32",
		"uint64": "u64", "uint": "u32", "float32": "f32", "float64": "f64",
		"byte": "u8", "rune": "i32",
		"i8": "i8", "i16": "i16", "i32": "i32", "i64": "i64",
		"u8": "u8", "u16": "u16", "u32": "u32", "u64": "u64",
		"f32": "f32", "f64": "f64",
	}
	if rustType, ok := typeConversions[funName]; ok {
		return true, rustType
	}
	return false, ""
}

// lowerToBuiltins maps Go builtins to Rust equivalents.
func (e *RustEmitter) lowerToBuiltins(selector string) string {
	switch selector {
	case "fmt":
		return ""
	case "Sprintf":
		return "string_format"
	case "Println":
		return "println"
	case "Printf":
		return "printf"
	case "Print":
		return "printf"
	case "len":
		return "len"
	case "panic":
		return "goany_panic"
	}
	return selector
}

// structHasInterfaceFields checks if a struct has interface{} fields
func (e *RustEmitter) structHasInterfaceFields(structName string) bool {
	return e.structHasInterfaceFieldsRecursive(structName, make(map[string]bool))
}

func (e *RustEmitter) structHasInterfaceFieldsRecursive(structName string, visited map[string]bool) bool {
	if visited[structName] {
		return false
	}
	visited[structName] = true
	for _, file := range e.pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if typeSpec.Name.Name == structName {
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								if structType.Fields != nil {
									for _, field := range structType.Fields.List {
										fieldType := e.pkg.TypesInfo.Types[field.Type].Type
										typeStr := fieldType.String()
										if strings.Contains(typeStr, "interface{}") || strings.Contains(typeStr, "interface {") {
											return true
										}
										if strings.Contains(typeStr, "func(") {
											return true
										}
										if named, ok2 := fieldType.(*types.Named); ok2 {
											if _, isStruct := named.Underlying().(*types.Struct); isStruct {
												if e.structHasInterfaceFieldsRecursive(named.Obj().Name(), visited) {
													return true
												}
											}
										}
										if slice, ok2 := fieldType.(*types.Slice); ok2 {
											if named, ok3 := slice.Elem().(*types.Named); ok3 {
												if _, isStruct := named.Underlying().(*types.Struct); isStruct {
													if e.structHasInterfaceFieldsRecursive(named.Obj().Name(), visited) {
														return true
													}
												}
											}
										}
									}
								}
								return false
							}
						}
					}
				}
			}
		}
	}
	return false
}

// structHasFunctionFields checks if a struct has function/closure fields
func (e *RustEmitter) structHasFunctionFields(structName string) bool {
	for _, file := range e.pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if typeSpec.Name.Name == structName {
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								if structType.Fields != nil {
									for _, field := range structType.Fields.List {
										fieldType := e.pkg.TypesInfo.Types[field.Type].Type
										typeStr := fieldType.String()
										if strings.HasPrefix(typeStr, "func(") {
											return true
										}
										if _, isSig := fieldType.Underlying().(*types.Signature); isSig {
											return true
										}
									}
								}
								return false
							}
						}
					}
				}
			}
		}
	}
	return false
}

// structCanDeriveCopy checks if a struct only contains Copy fields
func (e *RustEmitter) structCanDeriveCopy(structName string) bool {
	for _, file := range e.pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if typeSpec.Name.Name == structName {
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								if structType.Fields != nil {
									for _, field := range structType.Fields.List {
										fieldType := e.pkg.TypesInfo.Types[field.Type].Type
										if !e.isCopyableType(fieldType, make(map[string]bool)) {
											return false
										}
									}
								}
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

func (e *RustEmitter) isCopyableType(t types.Type, visited map[string]bool) bool {
	if named, ok := t.(*types.Named); ok {
		name := named.Obj().Name()
		if named.Obj().Pkg() != nil {
			name = named.Obj().Pkg().Path() + "." + name
		}
		if visited[name] {
			return false
		}
		visited[name] = true
	}
	underlying := t.Underlying()
	if basic, isBasic := underlying.(*types.Basic); isBasic {
		if basic.Kind() == types.String {
			return false
		}
		return true
	}
	if structType, isStruct := underlying.(*types.Struct); isStruct {
		for i := 0; i < structType.NumFields(); i++ {
			if !e.isCopyableType(structType.Field(i).Type(), visited) {
				return false
			}
		}
		return true
	}
	return false
}

// structCanDeriveHash checks if a struct can derive Hash/PartialEq/Eq
func (e *RustEmitter) structCanDeriveHash(structName string) bool {
	for _, file := range e.pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if typeSpec.Name.Name == structName {
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								if structType.Fields != nil {
									for _, field := range structType.Fields.List {
										fieldType := e.pkg.TypesInfo.Types[field.Type].Type
										if !e.isHashableType(fieldType, make(map[string]bool)) {
											return false
										}
									}
								}
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

func (e *RustEmitter) isHashableType(t types.Type, visited map[string]bool) bool {
	if named, ok := t.(*types.Named); ok {
		name := named.Obj().Name()
		if named.Obj().Pkg() != nil {
			name = named.Obj().Pkg().Path() + "." + name
		}
		if visited[name] {
			return false
		}
		visited[name] = true
	}
	underlying := t.Underlying()
	if basic, isBasic := underlying.(*types.Basic); isBasic {
		if basic.Kind() == types.Float32 || basic.Kind() == types.Float64 {
			return false
		}
		return true
	}
	if structType, isStruct := underlying.(*types.Struct); isStruct {
		for i := 0; i < structType.NumFields(); i++ {
			if !e.isHashableType(structType.Field(i).Type(), visited) {
				return false
			}
		}
		return true
	}
	return false
}

// ============================================================
// Program / Package
// ============================================================

func (e *RustEmitter) PreVisitProgram(indent int) {
	e.aliases = make(map[string]Alias)
	e.typeAliasMap = make(map[string]string)
	e.outputPath = e.Output

	if e.LinkRuntime != "" {
		srcDir := filepath.Join(e.OutputDir, "src")
		if err := os.MkdirAll(srcDir, 0755); err != nil {
			fmt.Println("Error creating src directory:", err)
			return
		}
		e.outputPath = filepath.Join(srcDir, "main.rs")
	}

	e.fs = e.GetForestBuilder()

	// Write Rust preamble
	builtin := `use std::fmt;
use std::any::Any;
use std::rc::Rc;

// Type aliases (Go-style)
type Int8 = i8;
type Int16 = i16;
type Int32 = i32;
type Int64 = i64;
type Uint8 = u8;
type Uint16 = u16;
type Uint32 = u32;
type Uint64 = u64;

// println equivalents
pub fn println<T: fmt::Display>(val: T) {
    std::println!("{}", val);
}

pub fn println0() {
    std::println!();
}

// printf variants
pub fn printf<T: fmt::Display>(val: T) {
    print!("{}", val);
}

pub fn printf2<T: fmt::Display>(fmt_str: String, val: T) {
    let rust_fmt = fmt_str.replace("%d", "{}").replace("%s", "{}").replace("%v", "{}");
    let result = rust_fmt.replace("{}", &format!("{}", val));
    print!("{}", result);
}

pub fn printf3<T1: fmt::Display, T2: fmt::Display>(fmt_str: String, v1: T1, v2: T2) {
    let rust_fmt = fmt_str.replace("%d", "{}").replace("%s", "{}").replace("%v", "{}");
    let result = rust_fmt.replacen("{}", &format!("{}", v1), 1).replacen("{}", &format!("{}", v2), 1);
    print!("{}", result);
}

pub fn printf4<T1: fmt::Display, T2: fmt::Display, T3: fmt::Display>(fmt_str: String, v1: T1, v2: T2, v3: T3) {
    let rust_fmt = fmt_str.replace("%d", "{}").replace("%s", "{}").replace("%v", "{}");
    let result = rust_fmt.replacen("{}", &format!("{}", v1), 1).replacen("{}", &format!("{}", v2), 1).replacen("{}", &format!("{}", v3), 1);
    print!("{}", result);
}

pub fn printf5<T1: fmt::Display, T2: fmt::Display, T3: fmt::Display, T4: fmt::Display>(fmt_str: String, v1: T1, v2: T2, v3: T3, v4: T4) {
    let rust_fmt = fmt_str.replace("%d", "{}").replace("%s", "{}").replace("%v", "{}");
    let result = rust_fmt.replacen("{}", &format!("{}", v1), 1).replacen("{}", &format!("{}", v2), 1).replacen("{}", &format!("{}", v3), 1).replacen("{}", &format!("{}", v4), 1);
    print!("{}", result);
}

pub fn printc(b: i8) {
    print!("{}", b as u8 as char);
}

pub fn byte_to_char(b: i8) -> String {
    (b as u8 as char).to_string()
}

pub fn append<T>(mut vec: Vec<T>, value: T) -> Vec<T> {
    vec.push(value);
    vec
}

pub fn append_many<T: Clone>(mut vec: Vec<T>, values: &[T]) -> Vec<T> {
    vec.extend_from_slice(values);
    vec
}

pub fn string_format(fmt_str: String, args: &[&dyn fmt::Display]) -> String {
    let mut result = String::new();
    let mut split = fmt_str.split("{}");
    for (i, segment) in split.enumerate() {
        result.push_str(segment);
        if i < args.len() {
            result.push_str(&format!("{}", args[i]));
        }
    }
    result
}

pub fn string_format2<T: fmt::Display>(fmt_str: String, val: T) -> String {
    let rust_fmt = fmt_str.replace("%d", "{}").replace("%s", "{}").replace("%v", "{}");
    rust_fmt.replace("{}", &format!("{}", val))
}

pub fn len<T>(slice: &[T]) -> i32 {
    slice.len() as i32
}
`
	e.fs.AddTree(IRNode{Type: Preamble, Kind: KindDecl, Content: builtin})

	// Panic runtime
	e.fs.AddTree(IRNode{Type: Preamble, Kind: KindDecl, Content: "\n// GoAny panic runtime\n"})
	e.fs.AddTree(IRNode{Type: Preamble, Kind: KindDecl, Content: goanyrt.PanicRustSource})
	e.fs.AddTree(IRNode{Type: Preamble, Kind: KindDecl, Content: "\n"})

	// Runtime module imports
	if e.LinkRuntime != "" {
		for name, variant := range e.RuntimePackages {
			if variant == "none" {
				continue
			}
			e.fs.AddTree(IRNode{Type: Preamble, Kind: KindDecl, Content: fmt.Sprintf("\n// %s runtime\nmod %s;\nuse %s::*;\n", name, name, name)})
		}
	}
}

func (e *RustEmitter) PostVisitProgram(indent int) {
	tokens := e.fs.CollectForest(string(PreVisitProgram))
	root := IRNode{Type: ScopeNode, Kind: KindDecl, Children: tokens}
	root.Content = root.Serialize()
	e.outputs = []OutputEntry{{Path: e.outputPath, Root: root}}
}

func (e *RustEmitter) GetOutputEntries() []OutputEntry { return e.outputs }

func (e *RustEmitter) PostFileEmit() {
	if len(e.structKeyTypes) > 0 {
		e.replaceStructKeyFunctions()
	}
	if e.LinkRuntime != "" {
		if err := e.GenerateCargoToml(); err != nil {
			log.Printf("Warning: %v", err)
		}
		if err := e.GenerateBuildRs(); err != nil {
			log.Printf("Warning: %v", err)
		}
		if err := e.CopyRuntimeMods(); err != nil {
			log.Printf("Warning: %v", err)
		}
	}
	if e.Opt.OptimizeMoves && e.Opt.MoveOptCount > 0 {
		fmt.Printf("  Rust: %d clone(s) removed by move optimization\n", e.Opt.MoveOptCount)
	}
}

func (e *RustEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	e.pkg = pkg
	e.currentPackage = pkg.Name
	e.Opt.refOptCurrentPkg = pkg.Name
	e.Opt.SetPkg(pkg)
	e.Opt.goTypeToRust = e.mapGoTypeToRust

	if e.Opt.OptimizeRefs {
		e.Opt.AccumulateReadOnlyAnalysis(pkg)
		if e.Opt.RefOptPass != nil {
			e.Opt.RefOptPass.Analysis = e.Opt.refOptReadOnly
		}
	}

	if pkg.Name != "main" {
		e.fs.AddTree(IRTree(PackageDeclaration, KindDecl,
			LeafTag(Keyword, "pub", TagRust),
			Leaf(WhiteSpace, " "),
			LeafTag(Keyword, "mod", TagRust),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, pkg.Name),
			Leaf(WhiteSpace, " "),
			Leaf(LeftBrace, "{"),
			Leaf(NewLine, "\n"),
			LeafTag(Keyword, "use", TagRust),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "crate::*"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
			Leaf(NewLine, "\n"),
		))
	}
}

func (e *RustEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	if pkg.Name != "main" {
		e.fs.AddTree(IRTree(PackageDeclaration, KindDecl,
			Leaf(RightBrace, "}"),
			Leaf(WhiteSpace, " "),
			Leaf(LineComment, "// pub mod "+pkg.Name),
			Leaf(NewLine, "\n"),
			Leaf(NewLine, "\n"),
		))
	}
}

// ============================================================
// Forward Declaration Signatures (suppressed)
// ============================================================

func (e *RustEmitter) PreVisitFuncDeclSignatures(indent int) {
	e.forwardDecl = true
}

func (e *RustEmitter) PostVisitFuncDeclSignatures(indent int) {
	e.fs.CollectForest(string(PreVisitFuncDeclSignatures))
	e.forwardDecl = false
}

// ============================================================
// Function Declarations
// ============================================================

func (e *RustEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	e.numFuncResults = 0
	e.funcReturnType = nil
	if node.Type.Results != nil {
		e.numFuncResults = node.Type.Results.NumFields()
		if e.numFuncResults == 1 && e.pkg != nil && e.pkg.TypesInfo != nil {
			field := node.Type.Results.List[0]
			if tv, ok := e.pkg.TypesInfo.Types[field.Type]; ok && tv.Type != nil {
				e.funcReturnType = tv.Type
			}
		}
	}
}

func (e *RustEmitter) PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	typeCode := e.fs.CollectText(string(PreVisitFuncDeclSignatureTypeResultsList))
	e.fs.AddLeaf(typeCode, KindExpr, nil)
}

func (e *RustEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeResults))
	var resultTypes []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			resultTypes = append(resultTypes, t.Serialize())
		}
	}
	if len(resultTypes) == 0 {
		e.fs.AddLeaf("", TagType, nil)
	} else if len(resultTypes) == 1 {
		e.fs.AddLeaf(resultTypes[0], TagType, nil)
	} else {
		e.fs.AddLeaf("("+strings.Join(resultTypes, ", ")+")", TagType, nil)
	}
}

func (e *RustEmitter) PostVisitFuncDeclName(node *ast.Ident, indent int) {
	e.fs.CollectForest(string(PreVisitFuncDeclName))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
	// Track current function key for optimization
	e.Opt.refOptCurrentFunc = e.Opt.refOptCurrentPkg + "." + node.Name
	e.Opt.currentParamIndex = 0
}

func (e *RustEmitter) PostVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {
	typeCode := e.fs.CollectText(string(PreVisitFuncDeclSignatureTypeParamsListType))
	e.fs.AddLeaf(typeCode, KindExpr, nil)
}

func (e *RustEmitter) PostVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeParamsArgName))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *RustEmitter) PostVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeParamsList))
	// tokens: type (TagExpr), then names (TagIdent)
	typeStr := ""
	var names []string
	for _, t := range tokens {
		if t.Kind == TagExpr && typeStr == "" {
			typeStr = t.Serialize()
		} else if t.Kind == TagIdent {
			names = append(names, t.Serialize())
		}
	}
	// Rust params: name: mut Type, name: &Type (ref-opt), or name: &mut Type (mut-ref-opt)
	goType := e.getExprGoType(node.Type)
	paramIdx := e.Opt.currentParamIndex
	for _, name := range names {
		escapedName := escapeRustKeyword(name)
		optMeta := &OptMeta{
			Kind:          OptFuncParam,
			FuncKey:       e.Opt.refOptCurrentFunc,
			ParamIndex:    paramIdx,
			ParamName:     escapedName,
			TypeStr:       typeStr,
			IsRefEligible: isRefOptEligibleType(goType),
		}
		isRefOpt := false
		isMutRefOpt := false
		if e.Opt.OptimizeRefs && e.Opt.refOptReadOnly != nil {
			if readOnlyFlags, ok := e.Opt.refOptReadOnly.ReadOnly[e.Opt.refOptCurrentFunc]; ok {
				if paramIdx >= 0 && paramIdx < len(readOnlyFlags) && readOnlyFlags[paramIdx] {
					isRefOpt = true
				}
			}
			if !isRefOpt {
				if mutRefFlags, ok := e.Opt.refOptReadOnly.MutRef[e.Opt.refOptCurrentFunc]; ok {
					if paramIdx >= 0 && paramIdx < len(mutRefFlags) && mutRefFlags[paramIdx] {
						isMutRefOpt = true
					}
				}
			}
		}
		optMeta.IsReadOnly = isRefOpt
		optMeta.IsMutRef = isMutRefOpt
		// Always emit base form; RefOptPass transforms to &T / &mut T
		paramNode := IRTree(Identifier, TagIdent,
			LeafTag(Keyword, "mut", TagRust),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, escapedName),
			Leaf(Colon, ":"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, typeStr),
		)
		paramNode.OptMeta = optMeta
		e.fs.AddTree(paramNode)
		paramIdx++
	}
	e.Opt.currentParamIndex = paramIdx
}

func (e *RustEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeParams))
	// Build wrapper tree preserving individual param nodes (with OptMeta for RefOptPass)
	var children []IRNode
	first := true
	for _, t := range tokens {
		if t.Kind == TagIdent {
			if !first {
				children = append(children, IRNode{Type: Comma, Content: ", "})
			}
			children = append(children, t)
			first = false
		}
	}
	e.fs.AddTree(IRTree(Identifier, KindExpr, children...))
}

func (e *RustEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignature))
	var returnTypeToken IRNode
	var funcNameToken IRNode
	var paramsToken IRNode
	hasReturnType := false
	hasFuncName := false
	for _, t := range tokens {
		if t.Kind == TagType && !hasReturnType {
			returnTypeToken = t
			hasReturnType = true
		} else if t.Kind == TagIdent && !hasFuncName {
			funcNameToken = t
			hasFuncName = true
		} else if t.Kind == TagExpr {
			paramsToken = t
		}
	}

	var children []IRNode
	children = append(children, Leaf(NewLine, "\n"))
	children = append(children, LeafTag(Keyword, "pub", TagRust))
	children = append(children, Leaf(WhiteSpace, " "))
	children = append(children, LeafTag(Keyword, "fn", TagRust))
	children = append(children, Leaf(WhiteSpace, " "))
	children = append(children, funcNameToken)
	children = append(children, Leaf(LeftParen, "("))
	children = append(children, paramsToken)
	children = append(children, Leaf(RightParen, ")"))
	if hasReturnType && returnTypeToken.Serialize() != "" {
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(Identifier, "->"))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, returnTypeToken)
	}
	e.fs.AddTree(IRTree(FuncDeclaration, KindDecl, children...))
}

func (e *RustEmitter) PostVisitFuncDeclBody(node *ast.BlockStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclBody))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDecl))
	var children []IRNode
	if len(tokens) >= 1 {
		children = append(children, tokens[0])
	}
	children = append(children, Leaf(WhiteSpace, " "))
	if len(tokens) >= 2 {
		children = append(children, tokens[1])
	}
	children = append(children, Leaf(NewLine, "\n"))
	children = append(children, Leaf(NewLine, "\n"))
	e.fs.AddTree(IRTree(FuncDeclaration, KindDecl, children...))
}

// ============================================================
// Block Statements
// ============================================================

func (e *RustEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	e.indent = indent
}

func (e *RustEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBlockStmtList))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *RustEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBlockStmt))
	var children []IRNode
	children = append(children, Leaf(LeftBrace, "{"))
	children = append(children, Leaf(NewLine, "\n"))
	for _, t := range tokens {
		if t.Serialize() != "" {
			children = append(children, t)
		}
	}
	children = append(children, Leaf(WhiteSpace, rustIndent(indent/2)))
	children = append(children, Leaf(RightBrace, "}"))
	e.fs.AddTree(IRTree(BlockStatement, KindStmt, children...))
}

// ============================================================
// Struct Declarations (GenStructInfo)
// ============================================================

func (e *RustEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	typeCode := e.fs.CollectText(string(PreVisitGenStructFieldType))
	e.fs.AddLeaf(typeCode, KindExpr, nil)
}

func (e *RustEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	e.fs.CollectForest(string(PreVisitGenStructFieldName))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *RustEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitGenStructInfo))

	if node.Struct == nil {
		return
	}

	type fieldInfo struct {
		typeName string
		name     string
	}
	var fields []fieldInfo
	i := 0
	for i < len(tokens) {
		if tokens[i].Kind == TagExpr {
			fi := fieldInfo{typeName: tokens[i].Serialize()}
			i++
			if i < len(tokens) && tokens[i].Kind == TagIdent {
				fi.name = tokens[i].Serialize()
				i++
			}
			fields = append(fields, fi)
		} else if tokens[i].Kind == TagIdent {
			fields = append(fields, fieldInfo{typeName: "Rc<dyn Any>", name: tokens[i].Serialize()})
			i++
		} else {
			i++
		}
	}

	// Determine derives
	hasInterfaceFields := e.structHasInterfaceFields(node.Name)
	hasFunctionFields := e.structHasFunctionFields(node.Name)

	var children []IRNode

	// Derive attribute
	if hasFunctionFields {
		children = append(children, Leaf(Identifier, "#[derive(Clone)]"))
		children = append(children, Leaf(NewLine, "\n"))
	} else if hasInterfaceFields {
		children = append(children, Leaf(Identifier, "#[derive(Clone, Debug)]"))
		children = append(children, Leaf(NewLine, "\n"))
	} else {
		canCopy := e.structCanDeriveCopy(node.Name)
		canHash := e.structCanDeriveHash(node.Name)
		if canCopy && canHash {
			children = append(children, Leaf(Identifier, "#[derive(Default, Clone, Copy, Debug, Hash, PartialEq, Eq)]"))
			children = append(children, Leaf(NewLine, "\n"))
		} else if canCopy {
			children = append(children, Leaf(Identifier, "#[derive(Default, Clone, Copy, Debug)]"))
			children = append(children, Leaf(NewLine, "\n"))
		} else if canHash {
			children = append(children, Leaf(Identifier, "#[derive(Default, Clone, Debug, Hash, PartialEq, Eq)]"))
			children = append(children, Leaf(NewLine, "\n"))
		} else {
			children = append(children, Leaf(Identifier, "#[derive(Default, Clone, Debug)]"))
			children = append(children, Leaf(NewLine, "\n"))
		}
	}

	// pub struct Name {
	children = append(children, LeafTag(Keyword, "pub", TagRust))
	children = append(children, Leaf(WhiteSpace, " "))
	children = append(children, LeafTag(Keyword, "struct", TagRust))
	children = append(children, Leaf(WhiteSpace, " "))
	children = append(children, Leaf(Identifier, node.Name))
	children = append(children, Leaf(WhiteSpace, " "))
	children = append(children, Leaf(LeftBrace, "{"))
	children = append(children, Leaf(NewLine, "\n"))

	// Fields
	for _, f := range fields {
		children = append(children, Leaf(WhiteSpace, "    "))
		children = append(children, LeafTag(Keyword, "pub", TagRust))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(Identifier, f.name))
		children = append(children, Leaf(Colon, ":"))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(Identifier, f.typeName))
		children = append(children, Leaf(Comma, ","))
		children = append(children, Leaf(NewLine, "\n"))
	}

	children = append(children, Leaf(RightBrace, "}"))
	children = append(children, Leaf(NewLine, "\n"))

	// Manual impl Default for structs with interface{} fields
	if hasInterfaceFields && !hasFunctionFields && node.Struct != nil && node.Struct.Fields != nil {
		children = append(children, Leaf(NewLine, "\n"))
		children = append(children, LeafTag(Keyword, "impl", TagRust))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(Identifier, "Default"))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, LeafTag(Keyword, "for", TagRust))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(Identifier, node.Name))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(LeftBrace, "{"))
		children = append(children, Leaf(NewLine, "\n"))
		// fn default() -> Self {
		children = append(children, Leaf(WhiteSpace, "    "))
		children = append(children, LeafTag(Keyword, "fn", TagRust))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(Identifier, "default"))
		children = append(children, Leaf(LeftParen, "("))
		children = append(children, Leaf(RightParen, ")"))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(Identifier, "->"))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, LeafTag(Keyword, "Self", TagRust))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(LeftBrace, "{"))
		children = append(children, Leaf(NewLine, "\n"))
		// StructName {
		children = append(children, Leaf(WhiteSpace, "        "))
		children = append(children, Leaf(Identifier, node.Name))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(LeftBrace, "{"))
		children = append(children, Leaf(NewLine, "\n"))
		for _, field := range node.Struct.Fields.List {
			if len(field.Names) == 0 {
				continue
			}
			fieldName := field.Names[0].Name
			fieldType := e.pkg.TypesInfo.Types[field.Type].Type
			defaultVal := e.rustDefaultForGoType(fieldType)
			children = append(children, Leaf(WhiteSpace, "            "))
			children = append(children, Leaf(Identifier, fieldName))
			children = append(children, Leaf(Colon, ":"))
			children = append(children, Leaf(WhiteSpace, " "))
			children = append(children, Leaf(Identifier, defaultVal))
			children = append(children, Leaf(Comma, ","))
			children = append(children, Leaf(NewLine, "\n"))
		}
		children = append(children, Leaf(WhiteSpace, "        "))
		children = append(children, Leaf(RightBrace, "}"))
		children = append(children, Leaf(NewLine, "\n"))
		children = append(children, Leaf(WhiteSpace, "    "))
		children = append(children, Leaf(RightBrace, "}"))
		children = append(children, Leaf(NewLine, "\n"))
		children = append(children, Leaf(RightBrace, "}"))
		children = append(children, Leaf(NewLine, "\n"))
	}

	children = append(children, Leaf(NewLine, "\n"))
	e.fs.AddTree(IRTree(StructTypeNode, KindType, children...))
}

func (e *RustEmitter) PostVisitGenStructInfos(node []GenTypeInfo, indent int) {
	// Structs already pushed to stack
}

// ============================================================
// Constants (GenDeclConst)
// ============================================================

func (e *RustEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	valTokens := e.fs.CollectForest(string(PreVisitGenDeclConstName))
	valCode := ""
	for _, t := range valTokens {
		valCode += t.Serialize()
	}
	if valCode == "" {
		valCode = "0"
	}

	constType := "i32"
	if e.pkg != nil && e.pkg.TypesInfo != nil {
		if obj := e.pkg.TypesInfo.Defs[node]; obj != nil {
			ut := obj.Type().Underlying()
			resolved := getRustValueTypeCast(ut)
			if resolved == "Rc<dyn Any>" {
				if basic, ok := ut.(*types.Basic); ok {
					if basic.Info()&types.IsInteger != 0 {
						resolved = "i32"
					} else if basic.Info()&types.IsFloat != 0 {
						resolved = "f64"
					} else if basic.Info()&types.IsString != 0 {
						resolved = "&str"
					} else if basic.Info()&types.IsBoolean != 0 {
						resolved = "bool"
					}
				}
			}
			constType = resolved
		}
	}

	name := node.Name
	// String constants use &str
	if constType == "String" {
		constType = "&str"
	}
	e.fs.AddTree(IRTree(DeclStatement, KindStmt,
		LeafTag(Keyword, "pub", TagRust),
		Leaf(WhiteSpace, " "),
		LeafTag(Keyword, "const", TagRust),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, name),
		Leaf(Colon, ":"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, constType),
		Leaf(WhiteSpace, " "),
		Leaf(Assignment, "="),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, valCode),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
	))
}

func (e *RustEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {
	// Constants flow through
}

// ============================================================
// Type Aliases
// ============================================================

func (e *RustEmitter) PreVisitTypeAliasName(node *ast.Ident, indent int) {
	e.currentAliasName = node.Name
}

func (e *RustEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitTypeAliasName))

	if e.currentAliasName != "" {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if tv, ok := e.pkg.TypesInfo.Types[node]; ok && tv.Type != nil {
				rustType := e.qualifiedRustTypeName(tv.Type)
				if e.typeAliasMap == nil {
					e.typeAliasMap = make(map[string]string)
				}
				e.typeAliasMap[e.currentAliasName] = rustType
				// Emit Rust type alias
				e.fs.AddTree(IRTree(DeclStatement, KindStmt,
					LeafTag(Keyword, "pub", TagRust),
					Leaf(WhiteSpace, " "),
					LeafTag(Keyword, "type", TagRust),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, e.currentAliasName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, rustType),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
			}
		}
	}
	e.currentAliasName = ""
}

// ============================================================
// replaceStructKeyFunctions replaces placeholder hash/equality functions
// ============================================================

func (e *RustEmitter) replaceStructKeyFunctions() {
	outputPath := e.Output
	if e.LinkRuntime != "" {
		outputPath = filepath.Join(e.OutputDir, "src", "main.rs")
	}
	content, err := os.ReadFile(outputPath)
	if err != nil {
		log.Printf("Warning: could not read file for struct key replacement: %v", err)
		return
	}

	newContent := string(content)

	var hashCases strings.Builder
	for _, rustPath := range e.structKeyTypes {
		hashCases.WriteString(fmt.Sprintf(`
        if let Some(s) = key.downcast_ref::<%s>() {
            use std::hash::{Hash, Hasher};
            use std::collections::hash_map::DefaultHasher;
            let mut hasher = DefaultHasher::new();
            s.hash(&mut hasher);
            let h = (hasher.finish() %% (i32::MAX as u64)) as i32;
            return h;
        }`, rustPath))
	}

	newHashBody := fmt.Sprintf(`pub fn hashStructKey(key: Rc<dyn Any>) -> i32 {%s
        return 0;
    }`, hashCases.String())

	hashPattern := regexp.MustCompile(`(?s)pub fn hashStructKey\s*\([^)]*\)\s*->\s*i32\s*\{\s*return 0;\s*\}`)
	newContent = hashPattern.ReplaceAllString(newContent, newHashBody)

	var equalCases strings.Builder
	for _, rustPath := range e.structKeyTypes {
		equalCases.WriteString(fmt.Sprintf(`
        if let (Some(sa), Some(sb)) = (a.downcast_ref::<%s>(), b.downcast_ref::<%s>()) {
            return sa == sb;
        }`, rustPath, rustPath))
	}

	newEqualBody := fmt.Sprintf(`pub fn structKeysEqual(a: Rc<dyn Any>, b: Rc<dyn Any>) -> bool {%s
        return false;
    }`, equalCases.String())

	equalPattern := regexp.MustCompile(`(?s)pub fn structKeysEqual\s*\([^)]*\)\s*->\s*bool\s*\{\s*return false;\s*\}`)
	newContent = equalPattern.ReplaceAllString(newContent, newEqualBody)

	if err := os.WriteFile(outputPath, []byte(newContent), 0644); err != nil {
		log.Printf("Warning: could not write struct key replacements: %v", err)
	}
}

// ============================================================
// Cargo project generation
// ============================================================

func (e *RustEmitter) GenerateCargoToml() error {
	if e.LinkRuntime == "" {
		return nil
	}
	cargoPath := filepath.Join(e.OutputDir, "Cargo.toml")
	file, err := os.Create(cargoPath)
	if err != nil {
		return fmt.Errorf("failed to create Cargo.toml: %w", err)
	}
	defer file.Close()

	graphicsBackend := e.RuntimePackages["graphics"]
	if graphicsBackend == "" {
		graphicsBackend = "none"
	}

	var cargoToml string
	if graphicsBackend == "tigr" {
		cargoToml = fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[dependencies]

[build-dependencies]
cc = "1.0"
`, e.OutputName)
	} else if graphicsBackend == "sdl2" {
		cargoToml = fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[dependencies]
sdl2 = "0.36"
`, e.OutputName)
	} else {
		cargoToml = fmt.Sprintf(`[package]
name = "%s"
version = "0.1.0"
edition = "2021"

[dependencies]
`, e.OutputName)
	}

	// Add HTTP runtime dependencies
	if _, hasHTTP := e.RuntimePackages["http"]; hasHTTP {
		cargoToml = strings.Replace(cargoToml, "[dependencies]", "[dependencies]\nureq = \"2\"\ntiny_http = \"0.12\"\nlazy_static = \"1.4\"", 1)
	}

	// Add FS runtime dependencies (lazy_static for file handle storage)
	if _, hasFS := e.RuntimePackages["fs"]; hasFS {
		if _, hasHTTP := e.RuntimePackages["http"]; !hasHTTP {
			cargoToml = strings.Replace(cargoToml, "[dependencies]", "[dependencies]\nlazy_static = \"1.4\"", 1)
		}
	}

	// Add NET runtime dependencies
	if _, hasNet := e.RuntimePackages["net"]; hasNet {
		_, hasHTTP := e.RuntimePackages["http"]
		_, hasFS := e.RuntimePackages["fs"]
		if !hasHTTP && !hasFS {
			cargoToml = strings.Replace(cargoToml, "[dependencies]", "[dependencies]\nlazy_static = \"1.4\"", 1)
		}
	}

	// Add math SIMD runtime dependencies (needs cc to compile math_simd.c)
	if _, hasMath := e.RuntimePackages["math"]; hasMath {
		if !strings.Contains(cargoToml, "[build-dependencies]") {
			cargoToml += "\n[build-dependencies]\ncc = \"1.0\"\n"
		} else if !strings.Contains(cargoToml, "cc = ") {
			cargoToml = strings.Replace(cargoToml, "[build-dependencies]", "[build-dependencies]\ncc = \"1.0\"", 1)
		}
	}

	_, err = file.WriteString(cargoToml)
	return err
}

func (e *RustEmitter) GenerateBuildRs() error {
	if e.LinkRuntime == "" {
		return nil
	}
	graphicsBackend := e.RuntimePackages["graphics"]
	_, hasMath := e.RuntimePackages["math"]

	if graphicsBackend != "tigr" && graphicsBackend != "sdl2" && !hasMath {
		return nil
	}

	buildRsPath := filepath.Join(e.OutputDir, "build.rs")
	file, err := os.Create(buildRsPath)
	if err != nil {
		return fmt.Errorf("failed to create build.rs: %w", err)
	}
	defer file.Close()

	content := "fn main() {\n"

	if graphicsBackend == "tigr" {
		content += `    cc::Build::new()
        .file("src/tigr.c")
        .file("src/screen_helper.c")
        .compile("tigr");

    #[cfg(target_os = "macos")]
    {
        println!("cargo:rustc-link-lib=framework=OpenGL");
        println!("cargo:rustc-link-lib=framework=Cocoa");
        println!("cargo:rustc-link-lib=framework=CoreGraphics");
    }
    #[cfg(target_os = "linux")]
    {
        println!("cargo:rustc-link-lib=GL");
        println!("cargo:rustc-link-lib=X11");
    }
    #[cfg(target_os = "windows")]
    {
        println!("cargo:rustc-link-lib=opengl32");
        println!("cargo:rustc-link-lib=gdi32");
        println!("cargo:rustc-link-lib=user32");
        println!("cargo:rustc-link-lib=shell32");
        println!("cargo:rustc-link-lib=advapi32");
        println!("cargo:rustc-link-lib=legacy_stdio_definitions");
    }
`
	}

	if hasMath {
		content += `    cc::Build::new()
        .file("src/math_simd.c")
        .opt_level(3)
        .compile("math_simd");
`
	}

	content += "}\n"

	_, err = file.WriteString(content)
	return err
}

func (e *RustEmitter) CopyRuntimeMods() error {
	if e.LinkRuntime == "" {
		return nil
	}
	srcDir := filepath.Join(e.OutputDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		return fmt.Errorf("failed to create src directory: %w", err)
	}
	for name, variant := range e.RuntimePackages {
		if variant == "none" {
			continue
		}
		srcFileName := name + "_runtime"
		if variant != "" {
			srcFileName += "_" + variant
		}
		srcFileName += ".rs"

		runtimeSrcPath := filepath.Join(e.LinkRuntime, name, "rust", srcFileName)
		content, err := os.ReadFile(runtimeSrcPath)
		if err != nil {
			DebugLogPrintf("Skipping Rust runtime for %s: %v", name, err)
			continue
		}

		dstFileName := name + ".rs"
		dstPath := filepath.Join(srcDir, dstFileName)
		if err := os.WriteFile(dstPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", dstFileName, err)
		}

		// Copy extra files for tigr graphics
		if name == "graphics" && variant == "tigr" {
			for _, extraFile := range []string{"tigr.c", "tigr.h", "screen_helper.c"} {
				src := filepath.Join(e.LinkRuntime, "graphics", "cpp", extraFile)
				dst := filepath.Join(srcDir, extraFile)
				data, err := os.ReadFile(src)
				if err != nil {
					return fmt.Errorf("failed to read %s: %w", extraFile, err)
				}
				if err := os.WriteFile(dst, data, 0644); err != nil {
					return fmt.Errorf("failed to write %s: %w", extraFile, err)
				}
			}
		}

		// Copy extra C files for math SIMD runtime
		if name == "math" {
			for _, extraFile := range []string{"math_simd.c", "math_simd.h"} {
				src := filepath.Join(e.LinkRuntime, "math", "c", extraFile)
				dst := filepath.Join(srcDir, extraFile)
				data, err := os.ReadFile(src)
				if err != nil {
					return fmt.Errorf("failed to read %s: %w", extraFile, err)
				}
				if err := os.WriteFile(dst, data, 0644); err != nil {
					return fmt.Errorf("failed to write %s: %w", extraFile, err)
				}
			}
		}
	}
	return nil
}
