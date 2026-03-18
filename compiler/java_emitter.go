package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	goanyrt "goany/runtime"

	"golang.org/x/tools/go/packages"
)

// JavaEmitter implements the Emitter interface using a shift/reduce FragmentStack
// architecture for Java code generation. This follows the same pattern as CSharpEmitter.
type JavaEmitter struct {
	// --- Core infrastructure ---
	fs              *FragmentStack
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string
	RuntimePackages map[string]string
	file            *os.File
	Emitter // interface embedding (provides GetGoFIR via BaseEmitter)
	pkg            *packages.Package
	currentPackage string
	indent         int
	numFuncResults int

	// --- Index/Map assignment tracking ---
	lastIndexXCode   string
	lastIndexKeyCode string
	lhsIndexXCode    string // saved from LHS before RHS overwrites lastIndexXCode
	lhsIndexKeyCode  string // saved from LHS before RHS overwrites lastIndexKeyCode
	mapAssignVar     string
	mapAssignKey     string
	structKeyTypes   map[string]string

	// --- Control flow stacks (for nesting support) ---
	forInitStack []string
	forCondStack []string
	forPostStack []string
	ifInitStack  []string
	ifCondStack  []string
	ifBodyStack  []string
	ifElseStack  []string

	// --- Java-specific state ---
	forwardDecl      bool
	nestedMapCounter int
	typeAliasMap     map[string]string
	aliases          map[string]Alias
	currentAliasName string
	rangeVarCounter  int
	funcReturnType   types.Type

	// --- Lambda/closure state ---
	funcParamNames         []string          // enclosing function param names for shadowing detection
	declaredVarNames       map[string]bool   // locally declared variables for shadowing detection
	closureCapturedMutVars map[string]bool   // mutable variables needing Object[] wrapping
	closureCapturedVarType map[string]string // captured variable types
	lambdaParamRenames     map[string]string // current FuncLit param renames (original → renamed)
}

// Java type mapping - note Java has no unsigned types
var javaTypesMap = map[string]string{
	"int8":    "byte",
	"int16":   "short",
	"int32":   "int",
	"int64":   "long",
	"uint8":   "byte",
	"uint16":  "int",
	"uint32":  "long",
	"uint64":  "long",
	"int":     "int",
	"byte":    "byte",
	"rune":    "int",
	"any":     "Object",
	"string":  "String",
	"float32": "float",
	"float64": "double",
	"bool":    "boolean",
}

// javaBoxedTypes maps primitive types to their boxed versions for generics.
var javaBoxedTypes = map[string]string{
	"byte":    "Byte",
	"short":   "Short",
	"int":     "Integer",
	"long":    "Long",
	"float":   "Float",
	"double":  "Double",
	"boolean": "Boolean",
	"char":    "Character",
}

// toBoxedType returns the boxed Java type for a primitive, or the type itself.
func toBoxedType(t string) string {
	if boxed, ok := javaBoxedTypes[t]; ok {
		return boxed
	}
	return t
}

// isJavaOuterClassPackage returns true if the package should be emitted as an outer class.
func isJavaOuterClassPackage(pkgName string) bool {
	if pkgName == "main" || pkgName == "hmap" {
		return false
	}
	_, exists := namespaces[pkgName]
	return exists
}

// sanitizeJavaIdentifier converts a string to a valid Java identifier
// by replacing invalid characters (like hyphens) with underscores
// and removing the .java extension if present.
func sanitizeJavaIdentifier(name string) string {
	if strings.HasSuffix(name, ".java") {
		name = strings.TrimSuffix(name, ".java")
	}
	return strings.ReplaceAll(name, "-", "_")
}

// getJavaKeyCast returns cast prefix/suffix for map key types that need explicit casting.
func getJavaKeyCast(keyType types.Type) (string, string) {
	switch keyType.Underlying().String() {
	case "int64":
		return "(long)(", ")"
	case "uint64":
		return "(long)(", ")"
	default:
		return "", ""
	}
}

// isJavaPrimitiveType returns true if the type is a Java primitive type.
func isJavaPrimitiveType(javaType string) bool {
	switch javaType {
	case "int", "long", "double", "float", "boolean", "char", "byte", "short", "String":
		return true
	default:
		return false
	}
}

// isJavaBuiltinReferenceType returns true if the type is a Java built-in reference type
// that should not be treated as a custom struct.
func isJavaBuiltinReferenceType(javaType string) bool {
	switch javaType {
	case "Object", "Integer", "Long", "Double", "Float", "Boolean", "Character", "Byte", "Short":
		return true
	default:
		return false
	}
}

// isJavaFunctionalInterface returns true if the type is a Java functional interface
// that should be copied by reference (not by calling a copy constructor).
func isJavaFunctionalInterface(javaType string) bool {
	if strings.HasPrefix(javaType, "BiFunction<") ||
		strings.HasPrefix(javaType, "Function<") ||
		strings.HasPrefix(javaType, "Consumer<") ||
		strings.HasPrefix(javaType, "BiConsumer<") ||
		strings.HasPrefix(javaType, "Supplier<") ||
		strings.HasPrefix(javaType, "Predicate<") ||
		strings.HasPrefix(javaType, "BiPredicate<") ||
		strings.HasPrefix(javaType, "Runnable") {
		return true
	}
	return false
}

// getJavaDefaultValueForStruct returns the default value for a field type,
// initializing struct types with new instances (to match Go value semantics).
func getJavaDefaultValueForStruct(javaType string) string {
	switch javaType {
	case "int":
		return "0"
	case "long":
		return "0L"
	case "double":
		return "0.0"
	case "float":
		return "0.0f"
	case "boolean":
		return "false"
	case "char":
		return "'\\0'"
	case "byte":
		return "(byte)0"
	case "short":
		return "(short)0"
	case "String":
		return "\"\""
	}
	if strings.HasPrefix(javaType, "ArrayList<") ||
		strings.HasPrefix(javaType, "HashMap<") ||
		isJavaFunctionalInterface(javaType) ||
		isJavaBuiltinReferenceType(javaType) {
		return "null"
	}
	return fmt.Sprintf("new %s()", javaType)
}

func (e *JavaEmitter) SetFile(file *os.File) { e.file = file }
func (e *JavaEmitter) GetFile() *os.File     { return e.file }

// isByteTypeJ returns true if the given Go type is byte (uint8) or int8.
func (e *JavaEmitter) isByteTypeJ(t types.Type) bool {
	if t == nil {
		return false
	}
	if basic, ok := t.Underlying().(*types.Basic); ok {
		return basic.Kind() == types.Uint8 || basic.Kind() == types.Int8
	}
	return false
}

// maskByteValueJ wraps a value with & 0xFF for unsigned byte semantics in Java.
func (e *JavaEmitter) maskByteValueJ(value string) string {
	return fmt.Sprintf("(%s & 0xFF)", value)
}

// escapeRawStringToJava converts a Go raw string (backtick) content to a Java string literal.
func escapeRawStringToJava(raw string) string {
	s := strings.ReplaceAll(raw, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return "\"" + s + "\""
}

// javaIndent returns indentation string for the given level.
func javaIndent(indent int) string {
	return strings.Repeat("  ", indent/2)
}

// javaDefaultForGoType returns Java default value for a Go type.
func javaDefaultForGoType(t types.Type) string {
	if t == nil {
		return "null"
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsString != 0:
			return `""`
		case u.Info()&types.IsBoolean != 0:
			return "false"
		case u.Info()&types.IsNumeric != 0:
			return "0"
		}
	case *types.Slice:
		elemType := getJavaPrimTypeName(u.Elem())
		return fmt.Sprintf("new ArrayList<%s>()", toBoxedType(elemType))
	case *types.Map:
		return "null"
	case *types.Struct:
		if named, ok := t.(*types.Named); ok {
			return fmt.Sprintf("new %s()", named.Obj().Name())
		}
		return "null"
	}
	return "null"
}

// javaDefaultForGoTypeQ is like javaDefaultForGoType but uses qualified names for structs.
func (e *JavaEmitter) javaDefaultForGoTypeQ(t types.Type) string {
	if t == nil {
		return "null"
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		switch {
		case u.Info()&types.IsString != 0:
			return `""`
		case u.Info()&types.IsBoolean != 0:
			return "false"
		case u.Info()&types.IsNumeric != 0:
			return "0"
		}
	case *types.Slice:
		// After pointer lowering, []*T fields become []int but types.Struct still has []*T
		if _, isPtr := u.Elem().(*types.Pointer); isPtr {
			return "new ArrayList<Integer>()"
		}
		elemType := e.qualifiedJavaTypeName(u.Elem())
		return fmt.Sprintf("new ArrayList<%s>()", toBoxedType(elemType))
	case *types.Pointer:
		// After pointer lowering, *T fields become int with -1 sentinel for nil
		return "-1"
	case *types.Map:
		return "null"
	case *types.Struct:
		typeName := e.qualifiedJavaTypeName(t)
		return fmt.Sprintf("new %s()", typeName)
	}
	return "null"
}

// getJavaPrimTypeName converts a Go type to its Java type name.
// This is a method-local version adapted from getJavaTypeName that handles
// Named, Basic, Slice, Map, and Signature types.
func getJavaPrimTypeName(t types.Type) string {
	if t == nil {
		return "Object"
	}
	// Handle named types
	if named, ok := t.(*types.Named); ok {
		// Type alias to basic type
		if basic, ok := named.Underlying().(*types.Basic); ok {
			return javaBasicTypeName(basic)
		}
		// Slice alias
		if sliceType, ok := named.Underlying().(*types.Slice); ok {
			elemType := getJavaPrimTypeName(sliceType.Elem())
			return fmt.Sprintf("ArrayList<%s>", toBoxedType(elemType))
		}
		// Map alias
		if _, ok := named.Underlying().(*types.Map); ok {
			return "hmap.HashMap"
		}
		// Struct or other named type
		typeName := named.Obj().Name()
		if pkg := named.Obj().Pkg(); pkg != nil {
			pkgName := pkg.Name()
			if isJavaOuterClassPackage(pkgName) {
				return pkgName + "." + typeName
			}
		}
		return typeName
	}

	switch ut := t.Underlying().(type) {
	case *types.Basic:
		return javaBasicTypeName(ut)
	case *types.Slice:
		elemType := getJavaPrimTypeName(ut.Elem())
		return fmt.Sprintf("ArrayList<%s>", toBoxedType(elemType))
	case *types.Map:
		return "hmap.HashMap"
	case *types.Pointer:
		return "int"
	case *types.Signature:
		return javaSignatureTypeName(ut)
	case *types.Interface:
		if ut.Empty() {
			return "Object"
		}
		return "Object"
	default:
		return "Object"
	}
}

// javaBasicTypeName maps a basic type to Java type name.
func javaBasicTypeName(basic *types.Basic) string {
	switch basic.Kind() {
	case types.Int, types.Int32, types.UntypedInt, types.UntypedRune:
		return "int"
	case types.Int8:
		return "byte"
	case types.Int16:
		return "short"
	case types.Int64:
		return "long"
	case types.Uint8:
		return "byte"
	case types.Uint, types.Uint16, types.Uint32:
		return "int"
	case types.Uint64:
		return "long"
	case types.Float32:
		return "float"
	case types.Float64, types.UntypedFloat:
		return "double"
	case types.Bool, types.UntypedBool:
		return "boolean"
	case types.String, types.UntypedString:
		return "String"
	default:
		return "Object"
	}
}

// javaSignatureTypeName maps a Go function signature to Java functional interface type.
func javaSignatureTypeName(sig *types.Signature) string {
	numParams := sig.Params().Len()
	hasReturn := sig.Results().Len() > 0

	if !hasReturn {
		switch numParams {
		case 0:
			return "Runnable"
		case 1:
			p1 := getJavaPrimTypeName(sig.Params().At(0).Type())
			return fmt.Sprintf("Consumer<%s>", toBoxedType(p1))
		case 2:
			p1 := getJavaPrimTypeName(sig.Params().At(0).Type())
			p2 := getJavaPrimTypeName(sig.Params().At(1).Type())
			return fmt.Sprintf("BiConsumer<%s, %s>", toBoxedType(p1), toBoxedType(p2))
		}
	} else {
		returnType := getJavaPrimTypeName(sig.Results().At(0).Type())
		switch numParams {
		case 0:
			return fmt.Sprintf("Supplier<%s>", toBoxedType(returnType))
		case 1:
			p1 := getJavaPrimTypeName(sig.Params().At(0).Type())
			return fmt.Sprintf("Function<%s, %s>", toBoxedType(p1), toBoxedType(returnType))
		case 2:
			p1 := getJavaPrimTypeName(sig.Params().At(0).Type())
			p2 := getJavaPrimTypeName(sig.Params().At(1).Type())
			return fmt.Sprintf("BiFunction<%s, %s, %s>", toBoxedType(p1), toBoxedType(p2), toBoxedType(returnType))
		}
	}
	return "Object"
}

// toBoxedJavaType converts a primitive Java type to its boxed version.
func toBoxedJavaType(t string) string {
	if boxed, ok := javaBoxedTypes[t]; ok {
		return boxed
	}
	return t
}

// qualifiedJavaTypeName returns the Java type name with package prefix for cross-package struct types.
func (e *JavaEmitter) qualifiedJavaTypeName(t types.Type) string {
	if t == nil {
		return "Object"
	}
	// Handle function types
	if sig, ok := t.Underlying().(*types.Signature); ok {
		return javaSignatureTypeName(sig)
	}
	// For named struct types from other packages, add the package prefix
	if named, ok := t.(*types.Named); ok {
		if _, isStruct := named.Underlying().(*types.Struct); isStruct {
			name := named.Obj().Name()
			if named.Obj().Pkg() != nil {
				pkgName := named.Obj().Pkg().Name()
				if pkgName != e.currentPackage && pkgName != "main" {
					return pkgName + "." + name
				}
			}
			return name
		}
	}
	// Handle slice of cross-package structs
	if slice, ok := t.(*types.Slice); ok {
		elemType := e.qualifiedJavaTypeName(slice.Elem())
		return "ArrayList<" + toBoxedType(elemType) + ">"
	}
	// Handle map type
	if _, ok := t.Underlying().(*types.Map); ok {
		return "hmap.HashMap"
	}
	return getJavaPrimTypeName(t)
}

// javaLowerBuiltin maps Go stdlib selectors to Java equivalents.
func javaLowerBuiltin(selector string) string {
	switch selector {
	case "fmt":
		return ""
	case "Sprintf":
		return "Formatter.Sprintf"
	case "Println":
		return "System.out.println"
	case "Printf":
		return "Formatter.Printf"
	case "Print":
		return "System.out.print"
	case "len":
		return "SliceBuiltins.Length"
	case "append":
		return "SliceBuiltins.Append"
	case "panic":
		return "GoanyPanic.goPanic"
	}
	return selector
}

// isMapTypeExprJ checks if an expression has map type via TypesInfo.
func (e *JavaEmitter) isMapTypeExprJ(expr ast.Expr) bool {
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
	// Fallback: for identifiers, try ObjectOf/Uses
	if ident, ok := expr.(*ast.Ident); ok {
		if obj := e.pkg.TypesInfo.ObjectOf(ident); obj != nil {
			_, isMap := obj.Type().Underlying().(*types.Map)
			return isMap
		}
	}
	return false
}

// getExprGoTypeJ returns the Go type for an expression, or nil.
func (e *JavaEmitter) getExprGoTypeJ(expr ast.Expr) types.Type {
	if e.pkg == nil || e.pkg.TypesInfo == nil {
		return nil
	}
	tv := e.pkg.TypesInfo.Types[expr]
	return tv.Type
}

// getMapKeyTypeConstJ returns the key type constant for a map's key type.
func (e *JavaEmitter) getMapKeyTypeConstJ(mapType *ast.MapType) int {
	if e.pkg != nil && e.pkg.TypesInfo != nil {
		if tv, ok := e.pkg.TypesInfo.Types[mapType.Key]; ok && tv.Type != nil {
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
				if named, ok := tv.Type.(*types.Named); ok {
					if e.structKeyTypes == nil {
						e.structKeyTypes = make(map[string]string)
					}
					structName := named.Obj().Name()
					e.structKeyTypes[structName] = structName
				}
				return 100
			}
		}
	}
	return 1
}

// getJavaKeyCastJ returns the map key cast prefix/suffix for Java.
func getJavaKeyCastJ(keyType types.Type) (string, string) {
	return getJavaKeyCast(keyType)
}

// exprToJavaString converts a simple AST expression to its Java string representation.
func exprToJavaString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Value
	case *ast.Ident:
		return e.Name
	case *ast.IndexExpr:
		xStr := exprToJavaString(e.X)
		indexStr := exprToJavaString(e.Index)
		if xStr != "" && indexStr != "" {
			return xStr + "[" + indexStr + "]"
		}
		return ""
	case *ast.SelectorExpr:
		xStr := exprToJavaString(e.X)
		if xStr != "" {
			return xStr + "." + e.Sel.Name
		}
		return ""
	default:
		return ""
	}
}

// javaMapKeyTypeConst returns the key type constant from types.Map.
func javaMapKeyTypeConst(t *types.Map) int {
	if t == nil {
		return 1
	}
	if basic, ok := t.Key().Underlying().(*types.Basic); ok {
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
	if named, ok := t.Key().(*types.Named); ok {
		if _, isStruct := named.Underlying().(*types.Struct); isStruct {
			return 100
		}
	}
	return 1
}

// getJavaFuncInterfaceMethod returns the method name to call on a Java functional interface
// based on the Go function signature (number of params, has return value).
func getJavaFuncInterfaceMethod(sig *types.Signature) string {
	numParams := sig.Params().Len()
	hasReturn := sig.Results().Len() > 0
	if !hasReturn {
		if numParams == 0 {
			return "run"
		}
		return "accept"
	}
	if numParams == 0 {
		return "get"
	}
	return "apply"
}

// getJavaFuncInterfaceType returns the Java functional interface type string for a Go function signature.
// e.g., func(int, string) bool -> BiFunction<Integer, String, Boolean>
func (e *JavaEmitter) getJavaFuncInterfaceType(sig *types.Signature) string {
	numParams := sig.Params().Len()
	hasReturn := sig.Results().Len() > 0

	if !hasReturn {
		if numParams == 0 {
			return "Runnable"
		}
		if numParams == 1 {
			p := toBoxedType(e.qualifiedJavaTypeName(sig.Params().At(0).Type()))
			return fmt.Sprintf("Consumer<%s>", p)
		}
		if numParams == 2 {
			p1 := toBoxedType(e.qualifiedJavaTypeName(sig.Params().At(0).Type()))
			p2 := toBoxedType(e.qualifiedJavaTypeName(sig.Params().At(1).Type()))
			return fmt.Sprintf("BiConsumer<%s, %s>", p1, p2)
		}
		return "Object"
	}
	retType := toBoxedType(e.qualifiedJavaTypeName(sig.Results().At(0).Type()))
	if numParams == 0 {
		return fmt.Sprintf("Supplier<%s>", retType)
	}
	if numParams == 1 {
		p := toBoxedType(e.qualifiedJavaTypeName(sig.Params().At(0).Type()))
		return fmt.Sprintf("Function<%s, %s>", p, retType)
	}
	if numParams == 2 {
		p1 := toBoxedType(e.qualifiedJavaTypeName(sig.Params().At(0).Type()))
		p2 := toBoxedType(e.qualifiedJavaTypeName(sig.Params().At(1).Type()))
		return fmt.Sprintf("BiFunction<%s, %s, %s>", p1, p2, retType)
	}
	return "Object"
}

// splitJavaArgs splits a Java argument string on ", " but respects parentheses, brackets, and angle brackets.
func splitJavaArgs(argsStr string) []string {
	if argsStr == "" {
		return nil
	}
	var args []string
	depth := 0
	start := 0
	for i := 0; i < len(argsStr); i++ {
		ch := argsStr[i]
		if ch == '(' || ch == '[' || ch == '<' || ch == '{' {
			depth++
		} else if ch == ')' || ch == ']' || ch == '>' || ch == '}' {
			depth--
		} else if ch == ',' && depth == 0 {
			args = append(args, strings.TrimSpace(argsStr[start:i]))
			start = i + 1
		}
	}
	args = append(args, strings.TrimSpace(argsStr[start:]))
	return args
}

// ============================================================
// Program / Package
// ============================================================

// writeJavaBoilerplate writes the standard Java imports, GoanyPanic, SliceBuiltins, and Formatter
// classes to the given file. Used for both the main file and when renaming due to naming conflicts.
func writeJavaBoilerplate(f *os.File) {
	f.WriteString("import java.util.*;\nimport java.util.function.*;\n\n")
	f.WriteString("// GoAny panic runtime\n")
	f.WriteString(goanyrt.PanicJavaSource)
	f.WriteString("\n")
	writeJavaHelperClasses(f)
}

func (e *JavaEmitter) PreVisitProgram(indent int) {
	var err error

	// Sanitize output name for Java
	e.OutputName = sanitizeJavaIdentifier(e.OutputName)
	// Rebuild the output path with sanitized name
	e.Output = filepath.Join(e.OutputDir, e.OutputName+".java")

	e.file, err = os.Create(e.Output)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}

	e.fs = NewFragmentStack(e.GetGoFIR())
	e.typeAliasMap = make(map[string]string)
	e.aliases = make(map[string]Alias)
	e.structKeyTypes = make(map[string]string)

	// Add runtime packages to namespaces for proper type prefixing
	for pkgName := range e.RuntimePackages {
		namespaces[pkgName] = struct{}{}
	}

	// Write Java header imports and runtime
	e.file.WriteString("import java.util.*;\nimport java.util.function.*;\n\n")
	e.file.WriteString("// GoAny panic runtime\n")
	e.file.WriteString(goanyrt.PanicJavaSource)
	e.file.WriteString("\n")

	// Write SliceBuiltins and Formatter helper classes
	writeJavaHelperClasses(e.file)
}

func writeJavaHelperClasses(f *os.File) {
	f.WriteString(`class SliceBuiltins {
    public static <T> ArrayList<T> Append(ArrayList<T> list, T element) {
        if (list == null) list = new ArrayList<T>();
        list.add(element);
        return list;
    }

    @SafeVarargs
    public static <T> ArrayList<T> Append(ArrayList<T> list, T... elements) {
        if (list == null) list = new ArrayList<T>();
        for (T e : elements) list.add(e);
        return list;
    }

    public static <T> ArrayList<T> Append(ArrayList<T> list, ArrayList<T> elements) {
        if (list == null) list = new ArrayList<T>();
        if (elements != null) list.addAll(elements);
        return list;
    }

    public static <T> int Length(List<T> list) {
        return list == null ? 0 : list.size();
    }

    public static int Length(String s) {
        return s == null ? 0 : s.length();
    }

    public static <T> int Length(T[] arr) {
        return arr == null ? 0 : arr.length;
    }

    // Create a slice with n elements initialized to null (like Go's make([]T, n))
    @SuppressWarnings("unchecked")
    public static <T> ArrayList<T> MakeSlice(int size) {
        ArrayList<T> list = new ArrayList<T>(size);
        for (int i = 0; i < size; i++) {
            list.add(null);
        }
        return list;
    }

    // Create a boolean slice with n elements initialized to false
    public static ArrayList<Boolean> MakeBoolSlice(int size) {
        ArrayList<Boolean> list = new ArrayList<Boolean>(size);
        for (int i = 0; i < size; i++) {
            list.add(false);
        }
        return list;
    }
}

class Formatter {
    public static void Printf(String format, Object... args) {
        int argIndex = 0;
        StringBuilder converted = new StringBuilder();
        List<Object> formattedArgs = new ArrayList<>();

        for (int i = 0; i < format.length(); i++) {
            if (format.charAt(i) == '%' && i + 1 < format.length()) {
                char next = format.charAt(i + 1);
                switch (next) {
                    case 'd':
                    case 's':
                    case 'f':
                        converted.append("%").append(next);
                        formattedArgs.add(args[argIndex]);
                        argIndex++;
                        i++;
                        continue;
                    case 'c':
                        converted.append("%c");
                        Object arg = args[argIndex];
                        if (arg instanceof Byte) {
                            formattedArgs.add((char)(byte)(Byte)arg);
                        } else if (arg instanceof Integer) {
                            formattedArgs.add((char)(int)(Integer)arg);
                        } else if (arg instanceof Character) {
                            formattedArgs.add(arg);
                        } else {
                            throw new IllegalArgumentException("Argument for %c must be char, int, or byte");
                        }
                        argIndex++;
                        i++;
                        continue;
                }
            }
            converted.append(format.charAt(i));
        }

        String result = converted.toString()
            .replace("\\n", "\n")
            .replace("\\t", "\t");

        System.out.printf(result, formattedArgs.toArray());
    }

    public static String Sprintf(String format, Object... args) {
        int argIndex = 0;
        StringBuilder converted = new StringBuilder();
        List<Object> formattedArgs = new ArrayList<>();

        for (int i = 0; i < format.length(); i++) {
            if (format.charAt(i) == '%' && i + 1 < format.length()) {
                char next = format.charAt(i + 1);
                switch (next) {
                    case 'd':
                    case 's':
                    case 'f':
                        converted.append("%").append(next);
                        formattedArgs.add(args[argIndex]);
                        argIndex++;
                        i++;
                        continue;
                    case 'c':
                        converted.append("%c");
                        Object arg = args[argIndex];
                        if (arg instanceof Byte) {
                            formattedArgs.add((char)(byte)(Byte)arg);
                        } else if (arg instanceof Integer) {
                            formattedArgs.add((char)(int)(Integer)arg);
                        } else if (arg instanceof Character) {
                            formattedArgs.add(arg);
                        } else {
                            throw new IllegalArgumentException("Argument for %c must be char, int, or byte");
                        }
                        argIndex++;
                        i++;
                        continue;
                }
            }
            converted.append(format.charAt(i));
        }

        String result = converted.toString()
            .replace("\\n", "\n")
            .replace("\\t", "\t");

        return String.format(result, formattedArgs.toArray());
    }
}

`)
}

func (e *JavaEmitter) PostVisitProgram(indent int) {
	// Reduce everything from program marker
	tokens := e.fs.Reduce(string(PreVisitProgram))
	// Write all accumulated code
	for _, t := range tokens {
		e.file.WriteString(t.Serialize())
	}
	e.file.Close()

	// Replace placeholder struct key functions with working implementations
	if len(e.structKeyTypes) > 0 {
		replaceStructKeyFunctionsJ(e.Output)
	}

	// Generate build files if link-runtime is enabled
	if e.LinkRuntime != "" {
		if err := GeneratePomXmlJ(e.OutputDir, e.OutputName, e.LinkRuntime); err != nil {
			log.Printf("Warning: %v", err)
		}
		if err := CopyRuntimePackagesJ(e.OutputDir, e.OutputName, e.LinkRuntime, e.RuntimePackages); err != nil {
			log.Printf("Warning: %v", err)
		}
	}
}

// replaceStructKeyFunctionsJ replaces placeholder hash/equality functions for struct keys.
func replaceStructKeyFunctionsJ(outputPath string) {
	content, err := os.ReadFile(outputPath)
	if err != nil {
		log.Printf("Warning: could not read file for struct key replacement: %v", err)
		return
	}

	newContent := string(content)

	hashPattern := regexp.MustCompile(`(?s)static\s+int\s+hashStructKey\s*\(\s*Object\s+key\s*\)\s*\{\s*return\s+0;\s*\}`)
	newHashBody := `static int hashStructKey(Object key) {
        int h = key.hashCode();
        if (h < 0) h = -h;
        return h;
    }`
	newContent = hashPattern.ReplaceAllString(newContent, newHashBody)

	equalPattern := regexp.MustCompile(`(?s)static\s+boolean\s+structKeysEqual\s*\(\s*Object\s+a\s*,\s*Object\s+b\s*\)\s*\{\s*return\s+false;\s*\}`)
	newEqualBody := `static boolean structKeysEqual(Object a, Object b) {
        return a.equals(b);
    }`
	newContent = equalPattern.ReplaceAllString(newContent, newEqualBody)

	if err := os.WriteFile(outputPath, []byte(newContent), 0644); err != nil {
		log.Printf("Warning: could not write struct key replacements: %v", err)
	}
}

// GeneratePomXmlJ generates a pom.xml for the Java project.
func GeneratePomXmlJ(outputDir, outputName, linkRuntime string) error {
	if linkRuntime == "" {
		return nil
	}

	pomPath := filepath.Join(outputDir, "pom.xml")
	file, err := os.Create(pomPath)
	if err != nil {
		return fmt.Errorf("failed to create pom.xml: %w", err)
	}
	defer file.Close()

	mainClass := strings.ReplaceAll(outputName, "-", "_")

	pom := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.goany</groupId>
    <artifactId>%s</artifactId>
    <version>1.0-SNAPSHOT</version>
    <packaging>jar</packaging>

    <properties>
        <maven.compiler.source>17</maven.compiler.source>
        <maven.compiler.target>17</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>

    <build>
        <sourceDirectory>${project.basedir}</sourceDirectory>
        <plugins>
            <plugin>
                <groupId>org.apache.maven.plugins</groupId>
                <artifactId>maven-compiler-plugin</artifactId>
                <version>3.11.0</version>
                <configuration>
                    <source>17</source>
                    <target>17</target>
                </configuration>
            </plugin>
            <plugin>
                <groupId>org.apache.maven.plugins</groupId>
                <artifactId>maven-jar-plugin</artifactId>
                <version>3.3.0</version>
                <configuration>
                    <archive>
                        <manifest>
                            <mainClass>%s</mainClass>
                        </manifest>
                    </archive>
                </configuration>
            </plugin>
            <plugin>
                <groupId>org.codehaus.mojo</groupId>
                <artifactId>exec-maven-plugin</artifactId>
                <version>3.1.0</version>
                <configuration>
                    <mainClass>%s</mainClass>
                </configuration>
            </plugin>
        </plugins>
    </build>
</project>
`, outputName, mainClass, mainClass)

	_, err = file.WriteString(pom)
	if err != nil {
		return fmt.Errorf("failed to write pom.xml: %w", err)
	}

	DebugLogPrintf("Generated pom.xml at %s", pomPath)
	return nil
}

// CopyRuntimePackagesJ copies runtime .java files for all detected runtime packages.
func CopyRuntimePackagesJ(outputDir, outputName, linkRuntime string, runtimePackages map[string]string) error {
	if linkRuntime == "" {
		return nil
	}

	for name, variant := range runtimePackages {
		if variant == "none" {
			continue
		}

		capName := strings.ToUpper(name[:1]) + name[1:]

		var srcFileName string
		if variant != "" {
			capVariant := strings.ToUpper(variant[:1]) + variant[1:]
			srcFileName = capName + "Runtime" + capVariant + ".java"
		} else {
			srcFileName = capName + "Runtime.java"
		}

		runtimeSrcPath := filepath.Join(linkRuntime, name, "java", srcFileName)
		content, err := os.ReadFile(runtimeSrcPath)
		if err != nil {
			DebugLogPrintf("Skipping Java runtime for %s: %v", name, err)
			continue
		}

		dstFileName := capName + "Runtime.java"
		dstPath := filepath.Join(outputDir, dstFileName)
		if err := os.WriteFile(dstPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", dstFileName, err)
		}
		DebugLogPrintf("Copied %s from %s to %s", dstFileName, runtimeSrcPath, dstPath)

		// For graphics with tigr variant, also copy native JNI files
		if name == "graphics" && variant == "tigr" {
			// Copy JNI C file
			jniSrc := filepath.Join(linkRuntime, "graphics", "java", "graphics_jni.c")
			if jniContent, err := os.ReadFile(jniSrc); err == nil {
				jniDst := filepath.Join(outputDir, "graphics_jni.c")
				if err := os.WriteFile(jniDst, jniContent, 0644); err != nil {
					return fmt.Errorf("failed to write graphics_jni.c: %w", err)
				}
				DebugLogPrintf("Copied graphics_jni.c")
			}

			// Copy Makefile as Makefile.jni (avoid overwriting C++ Makefile)
			makeSrc := filepath.Join(linkRuntime, "graphics", "java", "Makefile")
			if makeContent, err := os.ReadFile(makeSrc); err == nil {
				makeDst := filepath.Join(outputDir, "Makefile.jni")
				if err := os.WriteFile(makeDst, makeContent, 0644); err != nil {
					return fmt.Errorf("failed to write Makefile.jni: %w", err)
				}
				DebugLogPrintf("Copied Makefile.jni")
			}

			// Copy TIGR files from cpp directory
			tigrFiles := []string{"tigr.c", "tigr.h", "screen_helper.c"}
			for _, file := range tigrFiles {
				src := filepath.Join(linkRuntime, "graphics", "cpp", file)
				if fileContent, err := os.ReadFile(src); err == nil {
					dst := filepath.Join(outputDir, file)
					if err := os.WriteFile(dst, fileContent, 0644); err != nil {
						return fmt.Errorf("failed to write %s: %w", file, err)
					}
					DebugLogPrintf("Copied %s", file)
				}
			}

			// Generate run.sh script with correct platform-specific flags
			sanitizedName := sanitizeJavaIdentifier(outputName)
			runScript := `#!/bin/bash
# Run script for Java graphics application
# Automatically adds required JVM flags for each platform

MAIN_CLASS="${1:-` + sanitizedName + `}"

# Detect OS and set appropriate flags
case "$(uname -s)" in
    Darwin)
        # macOS requires -XstartOnFirstThread for GUI applications
        java -XstartOnFirstThread --enable-native-access=ALL-UNNAMED -Djava.library.path=. "$MAIN_CLASS"
        ;;
    Linux)
        java --enable-native-access=ALL-UNNAMED -Djava.library.path=. "$MAIN_CLASS"
        ;;
    MINGW*|MSYS*|CYGWIN*)
        java --enable-native-access=ALL-UNNAMED -Djava.library.path=. "$MAIN_CLASS"
        ;;
    *)
        echo "Unknown OS: $(uname -s)"
        java --enable-native-access=ALL-UNNAMED -Djava.library.path=. "$MAIN_CLASS"
        ;;
esac
`
			runScriptPath := filepath.Join(outputDir, "run.sh")
			if err := os.WriteFile(runScriptPath, []byte(runScript), 0755); err != nil {
				return fmt.Errorf("failed to write run.sh: %w", err)
			}
			DebugLogPrintf("Generated run.sh")
		}
	}
	return nil
}

// ============================================================
// Package visitors
// ============================================================

func (e *JavaEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	e.pkg = pkg
	name := pkg.Name
	if name == "main" {
		className := sanitizeJavaIdentifier(e.OutputName)
		if className == "" {
			className = "Main"
		}
		e.currentPackage = className
		e.fs.PushTree(IRTree(Keyword, TagExpr,
			LeafTag(Keyword, "public class ", TagJava),
			Leaf(Identifier, className),
			LeafTag(Keyword, " {\n\n", TagJava),
		))
	} else if name == "hmap" {
		// hmap is inlined in the main file
		e.currentPackage = name
		e.fs.PushTree(IRTree(Keyword, TagExpr,
			LeafTag(Keyword, "class ", TagJava),
			Leaf(Identifier, name),
			LeafTag(Keyword, " {\n\n", TagJava),
		))
	} else {
		// For non-main, non-hmap packages, create separate file with imports
		e.currentPackage = name

		// Check for naming conflict with main output file
		if name == e.OutputName {
			// Naming conflict - rename main output to avoid collision
			oldOutput := e.Output
			e.file.Close()
			os.Remove(oldOutput)

			e.OutputName = e.OutputName + "_main"
			e.Output = filepath.Join(e.OutputDir, e.OutputName+".java")
			var err error
			e.file, err = os.Create(e.Output)
			if err != nil {
				fmt.Println("Error creating renamed main file:", err)
				return
			}
			// Re-write imports and helpers to the new main file
			writeJavaBoilerplate(e.file)
		}

		// Flush current tokens to main file before switching
		tokens := e.fs.Reduce(string(PreVisitProgram))
		for _, t := range tokens {
			e.file.WriteString(t.Serialize())
		}
		// Re-emit program marker so subsequent packages accumulate correctly
		e.fs.EmitMarker(string(PreVisitProgram))

		// Create separate file for this package
		pkgFileName := filepath.Join(e.OutputDir, name+".java")
		pkgFile, err := os.Create(pkgFileName)
		if err != nil {
			fmt.Println("Error creating package file:", err)
			return
		}

		// Write imports and class header to package file
		pkgFile.WriteString("import java.util.*;\nimport java.util.function.*;\n\n")
		pkgFile.WriteString(fmt.Sprintf("public class %s {\n\n", name))
		pkgFile.Close()

		// The package content will be accumulated in the fragment stack
		// and appended to the package file in PostVisitPackage
		e.fs.PushCode("")
	}
}

func (e *JavaEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	name := pkg.Name

	if name != "main" && name != "hmap" {
		// Reduce tokens accumulated for this package
		// They were pushed after the PreVisitPackage marker
		// Close the class
		e.fs.PushTree(IRTree(PackageDeclaration, KindDecl, Leaf(Identifier, "}\n")))

		// Collect all tokens and append to the package file
		// The tokens from this package need to be written to the separate file
		tokens := e.fs.Reduce(string(PreVisitProgram))
		pkgFileName := filepath.Join(e.OutputDir, name+".java")
		pkgFile, err := os.OpenFile(pkgFileName, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			fmt.Println("Error opening package file:", err)
			return
		}
		for _, t := range tokens {
			pkgFile.WriteString(t.Serialize())
		}
		pkgFile.Close()
		// Re-push program marker for subsequent packages
		e.fs.PushMarker(string(PreVisitProgram))
		return
	}

	e.fs.PushTree(IRTree(PackageDeclaration, KindDecl, Leaf(Identifier, "}\n")))
}

// ============================================================
// Literals and Identifiers
// ============================================================

func (e *JavaEmitter) PreVisitBasicLit(node *ast.BasicLit, indent int) {
	val := node.Value
	if node.Kind == token.STRING && len(val) > 1 && val[0] == '`' {
		// Raw string literal -> Java regular string with escaped quotes
		val = escapeRawStringToJava(val[1 : len(val)-1])
	}
	// Add L suffix for large integer literals that exceed Java int range
	if node.Kind == token.INT {
		// Parse the integer value to check if it exceeds int range
		numVal, err := strconv.ParseInt(val, 0, 64)
		if err == nil && (numVal > 2147483647 || numVal < -2147483648) {
			if !strings.HasSuffix(val, "L") && !strings.HasSuffix(val, "l") {
				val = val + "L"
			}
		}
	}
	// Convert Go character literals to numeric values
	if node.Kind == token.CHAR && len(val) >= 3 && val[0] == '\'' {
		inner := val[1 : len(val)-1]
		if len(inner) == 1 {
			val = fmt.Sprintf("%d", inner[0])
		} else if inner == "\\n" {
			val = "10"
		} else if inner == "\\t" {
			val = "9"
		} else if inner == "\\r" {
			val = "13"
		} else if inner == "\\\\" {
			val = "92"
		} else if inner == "\\'" {
			val = "39"
		} else if inner == "\\\"" {
			val = "34"
		} else if inner == "\\0" {
			val = "0"
		} else {
			val = fmt.Sprintf("(int)'%s'", inner)
		}
	}
	e.fs.Push(val, TagLiteral, nil)
}

func (e *JavaEmitter) PreVisitIdent(node *ast.Ident, indent int) {
	name := node.Name
	// Map Go builtins
	switch name {
	case "true", "false":
		e.fs.Push(name, TagLiteral, nil)
		return
	case "nil":
		e.fs.Push("null", TagLiteral, nil)
		return
	case "string":
		e.fs.Push("String", TagType, nil)
		return
	case "bool":
		e.fs.Push("boolean", TagType, nil)
		return
	}
	// Check javaTypesMap for type mappings
	if javaType, ok := javaTypesMap[name]; ok {
		e.fs.Push(javaType, TagType, nil)
		return
	}
	// Check typeAliasMap
	if underlyingType, ok := e.typeAliasMap[name]; ok {
		e.fs.Push(underlyingType, TagType, nil)
		return
	}
	// Check if this is a reference to another package
	if e.pkg != nil && e.pkg.TypesInfo != nil {
		if obj := e.pkg.TypesInfo.Uses[node]; obj != nil {
			if obj.Pkg() != nil && obj.Pkg().Name() != e.currentPackage && obj.Pkg().Name() != "main" {
				name = obj.Pkg().Name() + "." + name
			}
		}
	}
	goType := e.getExprGoTypeJ(node)

	// Closure capture: wrap reads of captured mutable variables
	if e.closureCapturedMutVars != nil && e.closureCapturedMutVars[name] {
		capturedType := e.closureCapturedVarType[name]
		if capturedType != "" {
			e.fs.PushTree(IRTree(Identifier, TagIdent,
				Leaf(LeftParen, "(("),
				Leaf(TypeKeyword, capturedType),
				Leaf(RightParen, ")"),
				Leaf(Identifier, name),
				Leaf(LeftBracket, "["),
				Leaf(NumberLiteral, "0"),
				Leaf(RightBracket, "]"),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.PushTree(IRTree(Identifier, TagIdent,
				Leaf(Identifier, name),
				Leaf(LeftBracket, "["),
				Leaf(NumberLiteral, "0"),
				Leaf(RightBracket, "]"),
			))
		}
		return
	}

	e.fs.Push(name, TagIdent, goType)
}

// closureUnwrapLhs extracts "name[0]" from "((Type)name[0])" for assignment LHS.
func closureUnwrapLhs(expr string) string {
	// Pattern: ((Type)name[0]) -> name[0]
	if strings.HasPrefix(expr, "((") && strings.HasSuffix(expr, "[0])") {
		inner := expr[1 : len(expr)-1] // Remove outer parens: (Type)name[0]
		if idx := strings.Index(inner, ")"); idx >= 0 {
			return inner[idx+1:] // name[0]
		}
	}
	return expr
}

// closureUnwrapName extracts "name" from "((Type)name[0])" for declarations.
func closureUnwrapName(expr string) string {
	// Pattern: ((Type)name[0]) -> name
	if strings.HasPrefix(expr, "((") && strings.HasSuffix(expr, "[0])") {
		inner := expr[1 : len(expr)-1] // Remove outer parens: (Type)name[0]
		if idx := strings.Index(inner, ")"); idx >= 0 {
			rest := inner[idx+1:] // name[0]
			if bracketIdx := strings.Index(rest, "[0]"); bracketIdx >= 0 {
				return rest[:bracketIdx] // name
			}
		}
	}
	return expr
}

// ============================================================
// Binary Expressions
// ============================================================

func (e *JavaEmitter) PostVisitBinaryExprLeft(node ast.Expr, indent int) {
	left := e.fs.ReduceToCode(string(PreVisitBinaryExprLeft))
	e.fs.PushCode(left)
}

func (e *JavaEmitter) PostVisitBinaryExprRight(node ast.Expr, indent int) {
	right := e.fs.ReduceToCode(string(PreVisitBinaryExprRight))
	e.fs.PushCode(right)
}

func (e *JavaEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitBinaryExpr))
	left := ""
	right := ""
	if len(tokens) >= 1 {
		left = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		right = tokens[1].Serialize()
	}
	op := node.Op.String()

	// Check for string comparison: use .equals() for == and != on strings
	leftType := e.getExprGoTypeJ(node.X)
	rightType := e.getExprGoTypeJ(node.Y)
	if leftType != nil {
		if basic, ok := leftType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
			if op == "==" {
				e.fs.PushTree(IRTree(BinaryExpression, KindExpr,
					Leaf(Identifier, left),
					Leaf(Dot, "."),
					Leaf(Identifier, "equals"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, right),
					Leaf(RightParen, ")"),
				))
				return
			}
			if op == "!=" {
				e.fs.PushTree(IRTree(BinaryExpression, KindExpr,
					Leaf(UnaryOperator, "!"),
					Leaf(Identifier, left),
					Leaf(Dot, "."),
					Leaf(Identifier, "equals"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, right),
					Leaf(RightParen, ")"),
				))
				return
			}
		}
	}

	// Byte comparison masking: Java byte is signed (-128..127), Go byte/uint8 is unsigned (0..255).
	// Any comparison involving a byte value needs & 0xFF masking for correct unsigned semantics.
	// Skip masking for literal constants (they're already the correct value).
	if op == "==" || op == "!=" || op == "<" || op == ">" || op == "<=" || op == ">=" {
		_, lhsIsLit := node.X.(*ast.BasicLit)
		_, rhsIsLit := node.Y.(*ast.BasicLit)
		if e.isByteTypeJ(leftType) && !lhsIsLit {
			left = e.maskByteValueJ(left)
		}
		if e.isByteTypeJ(rightType) && !rhsIsLit {
			right = e.maskByteValueJ(right)
		}
	}

	// Right shift on byte: need & 0xFF for logical (unsigned) shift
	if op == ">>" && e.isByteTypeJ(leftType) {
		left = e.maskByteValueJ(left)
	}

	// Bitwise AND with byte operands compared against non-zero: mask result
	// e.g., (rowByte & mask) != 0 — the parent comparison will handle masking,
	// but (byte & byte) already promotes to int in Java, so the & 0xFF is needed
	// when the result is later compared.
	if op == "&" && e.isByteTypeJ(leftType) && e.isByteTypeJ(rightType) {
		e.fs.PushTree(IRTree(BinaryExpression, KindExpr,
			Leaf(LeftParen, "("),
			Leaf(Identifier, left),
			Leaf(WhiteSpace, " "),
			Leaf(BinaryOperator, op),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, right),
			Leaf(WhiteSpace, " "),
			Leaf(BinaryOperator, "&"),
			Leaf(WhiteSpace, " "),
			Leaf(NumberLiteral, "0xFF"),
			Leaf(RightParen, ")"),
		))
		return
	}

	expr := left + " " + op + " " + right

	// For arithmetic ops on narrow types, Java promotes to int - add narrowing cast
	goType := e.getExprGoTypeJ(node)
	if goType != nil {
		if basic, ok := goType.Underlying().(*types.Basic); ok {
			switch basic.Kind() {
			case types.Int8, types.Uint8:
				expr = "(byte)(" + expr + ")"
			case types.Int16:
				expr = "(short)(" + expr + ")"
			}
		}
	}

	e.fs.PushTree(IRTree(BinaryExpression, KindExpr,
		Leaf(Identifier, expr),
	))
}

// ============================================================
// Call Expressions
// ============================================================

func (e *JavaEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	funCode := e.fs.ReduceToCode(string(PreVisitCallExprFun))
	e.fs.PushCode(funCode)
}

func (e *JavaEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {
	argCode := e.fs.ReduceToCode(string(PreVisitCallExprArg))

	// Check if this arg is a function reference (identifier referencing a function)
	// In Java, function references need ClassName::methodName syntax
	if ident, ok := node.(*ast.Ident); ok && e.pkg != nil && e.pkg.TypesInfo != nil {
		if obj := e.pkg.TypesInfo.Uses[ident]; obj != nil {
			if _, isFunc := obj.(*types.Func); isFunc {
				className := strings.ReplaceAll(e.OutputName, "-", "_")
				// Check if it's from another package
				if obj.Pkg() != nil && obj.Pkg().Name() != e.currentPackage && obj.Pkg().Name() != "main" {
					className = obj.Pkg().Name()
				}
				argCode = className + "::" + ident.Name
			}
		}
	}

	e.fs.PushCode(argCode)
}

func (e *JavaEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	argTokens := e.fs.Reduce(string(PreVisitCallExprArgs))
	var args []string
	for _, t := range argTokens {
		if t.Serialize() != "" {
			args = append(args, t.Serialize())
		}
	}
	e.fs.PushCode(strings.Join(args, ", "))
}

func (e *JavaEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCallExpr))
	funName := ""
	argsStr := ""
	if len(tokens) >= 1 {
		funName = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		argsStr = tokens[1].Serialize()
	}

	// Handle special built-in functions
	switch funName {
	case "len", "SliceBuiltins.Length":
		// len(x) - for maps use hmap.hashMapLen(x), otherwise SliceBuiltins.Length(x)
		if len(node.Args) > 0 && e.isMapTypeExprJ(node.Args[0]) {
			e.fs.PushTree(IRTree(CallExpression, KindExpr,
				Leaf(Identifier, "hmap.hashMapLen"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.PushTree(IRTree(CallExpression, KindExpr,
				Leaf(Identifier, "SliceBuiltins.Length"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(RightParen, ")"),
			))
		}
		return
	case "append", "SliceBuiltins.Append":
		e.fs.PushTree(IRTree(CallExpression, KindExpr,
			Leaf(Identifier, "SliceBuiltins.Append"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
		return
	case "delete":
		// delete(m, k) -> m = hmap.hashMapDelete(m, k)
		if len(node.Args) >= 2 {
			mapName := exprToJavaString(node.Args[0])
			e.fs.PushTree(IRTree(CallExpression, KindExpr,
				Leaf(Identifier, mapName),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "hmap.hashMapDelete"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.PushTree(IRTree(CallExpression, KindExpr,
				Leaf(Identifier, "hmap.hashMapDelete"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(RightParen, ")"),
			))
		}
		return
	case "make":
		if len(node.Args) >= 1 {
			if mapType, ok := node.Args[0].(*ast.MapType); ok {
				keyTypeConst := e.getMapKeyTypeConstJ(mapType)
				e.fs.PushTree(IRTree(CallExpression, KindExpr,
					Leaf(Identifier, "hmap.newHashMap"),
					Leaf(LeftParen, "("),
					Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)),
					Leaf(RightParen, ")"),
				))
				return
			}
			if _, ok := node.Args[0].(*ast.ArrayType); ok {
				// make([]T, n) -> SliceBuiltins.MakeSlice(n) or MakeBoolSlice(n)
				elemType := "Object"
				isBool := false
				if e.pkg != nil && e.pkg.TypesInfo != nil {
					if tv, ok := e.pkg.TypesInfo.Types[node.Args[0]]; ok && tv.Type != nil {
						if slice, ok := tv.Type.(*types.Slice); ok {
							elemType = e.qualifiedJavaTypeName(slice.Elem())
							if basic, ok := slice.Elem().Underlying().(*types.Basic); ok && basic.Kind() == types.Bool {
								isBool = true
							}
						}
					}
				}
				parts := strings.SplitN(argsStr, ", ", 2)
				if len(parts) >= 2 {
					if isBool {
						e.fs.PushTree(IRTree(CallExpression, KindExpr,
							Leaf(Identifier, "SliceBuiltins.MakeBoolSlice"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, parts[1]),
							Leaf(RightParen, ")"),
						))
					} else {
						boxed := toBoxedType(elemType)
						e.fs.PushTree(IRTree(CallExpression, KindExpr,
							Leaf(Identifier, "SliceBuiltins."),
							Leaf(LeftAngle, "<"),
							Leaf(Identifier, boxed),
							Leaf(RightAngle, ">"),
							Leaf(Identifier, "MakeSlice"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, parts[1]),
							Leaf(RightParen, ")"),
						))
					}
				} else {
					e.fs.PushTree(IRTree(CallExpression, KindExpr,
						LeafTag(Keyword, "new ", TagJava),
						Leaf(Identifier, "ArrayList"),
						Leaf(LeftAngle, "<"),
						Leaf(Identifier, toBoxedType(elemType)),
						Leaf(RightAngle, ">"),
						Leaf(LeftParen, "("),
						Leaf(RightParen, ")"),
					))
				}
				return
			}
		}
		e.fs.PushTree(IRTree(CallExpression, KindExpr,
			Leaf(Identifier, "make"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
		return
	case "GoanyPanic.goPanic":
		e.fs.PushTree(IRTree(CallExpression, KindExpr,
			Leaf(Identifier, "GoanyPanic.goPanic"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
		return
	}

	// Check if this is a type conversion (e.g., int(x), string(x), byte(x))
	if ident, ok := node.Fun.(*ast.Ident); ok {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if obj := e.pkg.TypesInfo.ObjectOf(ident); obj != nil {
				if _, isTypeName := obj.(*types.TypeName); isTypeName {
					javaType := e.qualifiedJavaTypeName(obj.Type())
					if javaType == "String" {
						// string(x) -> String.valueOf((char)(x))
						// Check if the argument is a byte/int for char conversion
						if len(node.Args) > 0 {
							argType := e.getExprGoTypeJ(node.Args[0])
							if argType != nil {
								if basic, ok := argType.Underlying().(*types.Basic); ok {
									if basic.Kind() == types.Int || basic.Kind() == types.Int32 ||
										basic.Kind() == types.Uint8 || basic.Kind() == types.Int8 {
										e.fs.PushTree(IRTree(CallExpression, KindExpr,
										Leaf(Identifier, "String.valueOf"),
										Leaf(LeftParen, "((char)("),
										Leaf(Identifier, argsStr),
										Leaf(RightParen, "))"),
									))
										return
									}
								}
							}
						}
						e.fs.PushTree(IRTree(CallExpression, KindExpr,
							Leaf(Identifier, "String.valueOf"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, argsStr),
							Leaf(RightParen, ")"),
						))
						return
					}
					// byte-to-int/long: int(byteVar) -> (byteVar & 0xFF), int64(byteVar) -> (long)(byteVar & 0xFF)
					if (javaType == "int" || javaType == "long") && len(node.Args) > 0 {
						argType := e.getExprGoTypeJ(node.Args[0])
						if e.isByteTypeJ(argType) {
							if javaType == "long" {
								e.fs.PushTree(IRTree(CallExpression, KindExpr,
								Leaf(LeftParen, "(long)("),
								Leaf(Identifier, e.maskByteValueJ(argsStr)),
								Leaf(RightParen, ")"),
							))
							} else {
								e.fs.PushTree(IRTree(CallExpression, KindExpr, Leaf(Identifier, e.maskByteValueJ(argsStr))))
							}
							return
						}
					}
					e.fs.PushTree(IRTree(CallExpression, KindExpr,
						Leaf(LeftParen, "("),
						Leaf(TypeKeyword, javaType),
						Leaf(RightParen, ")("),
						Leaf(Identifier, argsStr),
						Leaf(RightParen, ")"),
					))
					return
				}
			}
		}
	}

	// Lower builtins (fmt.Println -> System.out.println, etc.)
	lowered := javaLowerBuiltin(funName)
	if lowered != funName {
		funName = lowered
	}

	// Check if calling a function from an indexed expression (e.g., funcs[0](args))
	// Java needs: funcs.get(0).apply(args) instead of funcs.get(0)(args)
	if indexExpr, ok := node.Fun.(*ast.IndexExpr); ok {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			tv := e.pkg.TypesInfo.Types[indexExpr]
			if tv.Type != nil {
				if sig, ok := tv.Type.Underlying().(*types.Signature); ok {
					method := getJavaFuncInterfaceMethod(sig)
					if method != "" {
						e.fs.PushTree(IRTree(CallExpression, KindExpr,
							Leaf(Identifier, funName),
							Leaf(Dot, "."),
							Leaf(Identifier, method),
							Leaf(LeftParen, "("),
							Leaf(Identifier, argsStr),
							Leaf(RightParen, ")"),
						))
						return
					}
				}
			}
		}
	}

	// Check if calling a struct field that is a function type
	// e.g., visitor.PreVisitFrom(args) -> visitor.PreVisitFrom.apply(args)
	if selExpr, ok := node.Fun.(*ast.SelectorExpr); ok {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if sel := e.pkg.TypesInfo.Selections[selExpr]; sel != nil {
				if sig, ok := sel.Type().Underlying().(*types.Signature); ok {
					method := getJavaFuncInterfaceMethod(sig)
					if method != "" {
						e.fs.PushTree(IRTree(CallExpression, KindExpr,
							Leaf(Identifier, funName),
							Leaf(Dot, "."),
							Leaf(Identifier, method),
							Leaf(LeftParen, "("),
							Leaf(Identifier, argsStr),
							Leaf(RightParen, ")"),
						))
						return
					}
				}
			}
		}
	}

	// Check if calling a function variable (e.g., fn(args) where fn is a var of function type)
	// Java needs: fn.apply(args) instead of fn(args)
	if ident, ok := node.Fun.(*ast.Ident); ok {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if obj := e.pkg.TypesInfo.ObjectOf(ident); obj != nil {
				if _, isVar := obj.(*types.Var); isVar {
					if sig, ok := obj.Type().Underlying().(*types.Signature); ok {
						method := getJavaFuncInterfaceMethod(sig)
						if method != "" {
							e.fs.PushTree(IRTree(CallExpression, KindExpr,
								Leaf(Identifier, funName),
								Leaf(Dot, "."),
								Leaf(Identifier, method),
								Leaf(LeftParen, "("),
								Leaf(Identifier, argsStr),
								Leaf(RightParen, ")"),
							))
							return
						}
					}
				}
			}
		}
	}

	// Add narrowing casts for byte/short function parameters
	if e.pkg != nil && e.pkg.TypesInfo != nil {
		var funType types.Type
		if identExpr, ok := node.Fun.(*ast.Ident); ok {
			if obj := e.pkg.TypesInfo.ObjectOf(identExpr); obj != nil {
				funType = obj.Type()
			}
		} else if selExpr, ok := node.Fun.(*ast.SelectorExpr); ok {
			if sel := e.pkg.TypesInfo.Selections[selExpr]; sel != nil {
				funType = sel.Type()
			} else if obj := e.pkg.TypesInfo.Uses[selExpr.Sel]; obj != nil {
				funType = obj.Type()
			}
		}
		if funType != nil {
			if sig, ok := funType.Underlying().(*types.Signature); ok {
				args := splitJavaArgs(argsStr)
				changed := false
				for i := 0; i < sig.Params().Len() && i < len(args); i++ {
					param := sig.Params().At(i)
					if basic, ok := param.Type().Underlying().(*types.Basic); ok {
						switch basic.Kind() {
						case types.Int8, types.Uint8:
							args[i] = "(byte)(" + args[i] + ")"
							changed = true
						case types.Int16, types.Uint16:
							args[i] = "(short)(" + args[i] + ")"
							changed = true
						}
					}
				}
				if changed {
					argsStr = strings.Join(args, ", ")
				}
			}
		}
	}

	e.fs.PushTree(IRTree(CallExpression, KindExpr,
		Leaf(Identifier, funName),
		Leaf(LeftParen, "("),
		Leaf(Identifier, argsStr),
		Leaf(RightParen, ")"),
	))
}

// ============================================================
// Selector Expressions (a.b)
// ============================================================

func (e *JavaEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitSelectorExprX))
	e.fs.PushCode(xCode)
}

func (e *JavaEmitter) PostVisitSelectorExprSel(node *ast.Ident, indent int) {
	e.fs.Reduce(string(PreVisitSelectorExprSel))
	e.fs.PushCode(node.Name)
}

func (e *JavaEmitter) PostVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSelectorExpr))
	xCode := ""
	selCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		selCode = tokens[1].Serialize()
	}

	if xCode == "os" && selCode == "Args" {
		e.fs.PushTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, "goany_os_args")))
		return
	}

	// Check if selector is a type alias
	if _, isAlias := e.typeAliasMap[selCode]; isAlias {
		e.fs.PushTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, e.typeAliasMap[selCode])))
		return
	}

	// Lower builtins: fmt.Println -> System.out.println
	loweredX := javaLowerBuiltin(xCode)
	loweredSel := javaLowerBuiltin(selCode)

	if loweredX == "" {
		e.fs.PushTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, loweredSel)))
	} else {
		e.fs.PushTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, loweredX+"."+loweredSel)))
	}
}

// ============================================================
// Index Expressions (a[i])
// ============================================================

func (e *JavaEmitter) PostVisitIndexExprX(node *ast.IndexExpr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitIndexExprX))
	e.fs.PushCode(xCode)
	e.lastIndexXCode = xCode
}

func (e *JavaEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	idxCode := e.fs.ReduceToCode(string(PreVisitIndexExprIndex))
	e.fs.PushCode(idxCode)
	e.lastIndexKeyCode = idxCode
}

func (e *JavaEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitIndexExpr))
	xCode := ""
	idxCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		idxCode = tokens[1].Serialize()
	}

	if e.isMapTypeExprJ(node.X) {
		mapGoType := e.getExprGoTypeJ(node.X)
		valType := "Object"
		pfx := ""
		sfx := ""
		if mapGoType != nil {
			if mapUnderlying, ok := mapGoType.Underlying().(*types.Map); ok {
				valType = e.qualifiedJavaTypeName(mapUnderlying.Elem())
				pfx, sfx = getJavaKeyCastJ(mapUnderlying.Key())
			}
		}
		indexNode := IRTree(IndexExpression, KindExpr,
			Leaf(Identifier, "((" + valType + ")hmap.hashMapGet(" + xCode + ", " + pfx + idxCode + sfx + "))"),
		)
		indexNode.GoType = e.getExprGoTypeJ(node)
		e.fs.PushTree(indexNode)
	} else {
		// Check for string indexing: s[i] -> (int)s.charAt(i)
		xType := e.getExprGoTypeJ(node.X)
		if xType != nil {
			if basic, ok := xType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
				e.fs.PushTree(IRTree(IndexExpression, KindExpr,
					Leaf(LeftParen, "(int)"),
					Leaf(Identifier, xCode),
					Leaf(Dot, "."),
					Leaf(Identifier, "charAt"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, idxCode),
					Leaf(RightParen, ")"),
				))
				return
			}
		}
		// Slice access: x[i] -> x.get(i)
		if xType != nil {
			if _, ok := xType.Underlying().(*types.Slice); ok {
				e.fs.PushTree(IRTree(IndexExpression, KindExpr,
					Leaf(Identifier, xCode),
					Leaf(Dot, "."),
					Leaf(Identifier, "get"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, idxCode),
					Leaf(RightParen, ")"),
				))
				return
			}
		}
		// Fallback to array-style access
		e.fs.PushTree(IRTree(IndexExpression, KindExpr,
			Leaf(Identifier, xCode),
			Leaf(Dot, "."),
			Leaf(Identifier, "get"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, idxCode),
			Leaf(RightParen, ")"),
		))
	}
}

// ============================================================
// Unary Expressions
// ============================================================

func (e *JavaEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitUnaryExpr))
	op := node.Op.String()
	// Go ^ is bitwise complement, Java uses ~
	if op == "^" {
		e.fs.PushTree(IRTree(UnaryExpression, KindExpr, Leaf(Identifier, "~"+xCode)))
	} else {
		e.fs.PushTree(IRTree(UnaryExpression, KindExpr, Leaf(Identifier, op+xCode)))
	}
}

// ============================================================
// Paren Expressions
// ============================================================

func (e *JavaEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {
	inner := e.fs.ReduceToCode(string(PreVisitParenExpr))
	e.fs.PushTree(IRTree(ParenExpression, KindExpr, Leaf(Identifier, "("+inner+")")))
}

// ============================================================
// javaMixedOp for nested map/slice index chain analysis
// ============================================================

type javaMixedOp struct {
	accessType    string // "map" or "slice"
	keyExpr       string
	valueJavaType string
	keyCastPfx    string
	keyCastSfx    string
	mapVarExpr    string
	tempVarName   string
}

// analyzeLhsIndexChainJ walks a chain of IndexExpr on the LHS of an assignment
// (e.g., m[k1][k2][i] = val) and returns a list of javaMixedOp describing each
// indexing step from outermost to innermost.
//
// For each step, javaMixedOp fields are populated as follows:
//   - accessType: "map" if the indexed expression is a map, "slice" otherwise
//   - keyExpr: the Java code for the index/key expression
//   - valueJavaType: (map only) the Java type name of the map's value type
//   - keyCastPfx/keyCastSfx: (map only) cast prefix/suffix for the map key
//   - mapVarExpr, tempVarName: populated later by the caller for intermediate map gets
//
// hasIntermediateMap is true if any non-final step in the chain is a map access,
// meaning temporary variables are needed for intermediate map.get() calls.
func (e *JavaEmitter) analyzeLhsIndexChainJ(expr ast.Expr) (ops []javaMixedOp, hasIntermediateMap bool) {
	var chain []ast.Expr
	current := expr
	for {
		indexExpr, ok := current.(*ast.IndexExpr)
		if !ok {
			break
		}
		chain = append(chain, indexExpr)
		current = indexExpr.X
	}
	// Reverse: chain[0] = outermost, chain[len-1] = innermost
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	hasIntermediateMap = false
	for i, node := range chain {
		ie := node.(*ast.IndexExpr)
		var exprType types.Type
		tv := e.pkg.TypesInfo.Types[ie.X]
		if tv.Type != nil {
			exprType = tv.Type
		} else if ident, ok2 := ie.X.(*ast.Ident); ok2 {
			if obj := e.pkg.TypesInfo.ObjectOf(ident); obj != nil {
				exprType = obj.Type()
			}
		}
		if exprType == nil {
			continue
		}
		isLast := (i == len(chain)-1)
		if mapType, ok := exprType.Underlying().(*types.Map); ok {
			op := javaMixedOp{
				accessType:    "map",
				keyExpr:       exprToJavaString(ie.Index),
				valueJavaType: e.qualifiedJavaTypeName(mapType.Elem()),
			}
			op.keyCastPfx, op.keyCastSfx = getJavaKeyCastJ(mapType.Key())
			if !isLast {
				hasIntermediateMap = true
			}
			ops = append(ops, op)
		} else {
			op := javaMixedOp{
				accessType: "slice",
				keyExpr:    exprToJavaString(ie.Index),
			}
			ops = append(ops, op)
		}
	}
	return ops, hasIntermediateMap
}

// convertGoTypeToJava converts a Go type string to Java syntax.
func convertGoTypeToJava(goType string) string {
	result := goType

	if strings.HasPrefix(result, "[]") {
		elementType := result[2:]
		elementType = convertGoTypeToJava(elementType)
		return "ArrayList<" + toBoxedType(elementType) + ">"
	}

	if strings.HasPrefix(result, "map[") {
		return "hmap.HashMap"
	}

	if strings.Contains(result, "/") {
		lastSlash := strings.LastIndex(result, "/")
		result = result[lastSlash+1:]
	}

	if javaType, exists := javaTypesMap[result]; exists {
		return javaType
	}

	return result
}

// ============================================================
// Composite Literals
// ============================================================

func (e *JavaEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitCompositeLitType))
}

func (e *JavaEmitter) PostVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	eltCode := e.fs.ReduceToCode(string(PreVisitCompositeLitElt))
	e.fs.PushCode(eltCode)
}

func (e *JavaEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	eltTokens := e.fs.Reduce(string(PreVisitCompositeLitElts))
	for _, t := range eltTokens {
		if t.Serialize() != "" {
			e.fs.Push(t.Serialize(), TagLiteral, nil)
		}
	}
}

func (e *JavaEmitter) PostVisitCompositeLit(node *ast.CompositeLit, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCompositeLit))
	var elts []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			elts = append(elts, t.Serialize())
		}
	}
	eltsStr := strings.Join(elts, ", ")

	litType := e.getExprGoTypeJ(node)
	if litType == nil {
		e.fs.PushTree(IRTree(CompositeLitExpression, KindExpr, Leaf(Identifier, "new ArrayList<>(java.util.Arrays.asList("+eltsStr+"))")))
		return
	}

	switch u := litType.Underlying().(type) {
	case *types.Struct:
		typeName := ""
		if node.Type != nil {
			typeName = exprToString(node.Type)
		}
		if typeName == "" {
			typeName = "Object"
		}
		// Helper to add narrowing casts for byte/short struct fields
		narrowArg := func(arg string, fieldType types.Type) string {
			if basic, ok := fieldType.Underlying().(*types.Basic); ok {
				switch basic.Kind() {
				case types.Int8, types.Uint8:
					return "(byte)(" + arg + ")"
				case types.Int16, types.Uint16:
					return "(short)(" + arg + ")"
				}
			}
			return arg
		}
		// Check if using named fields (KeyValueExpr)
		if len(node.Elts) > 0 {
			if _, isKV := node.Elts[0].(*ast.KeyValueExpr); isKV {
				kvMap := make(map[string]string)
				for _, elt := range elts {
					parts := strings.SplitN(elt, ": ", 2)
					if len(parts) == 2 {
						key := parts[0]
						if dotIdx := strings.LastIndex(key, "."); dotIdx >= 0 {
							key = key[dotIdx+1:]
						}
						kvMap[key] = parts[1]
					}
				}
				// Build ordered args by struct field order
				var orderedArgs []string
				for i := 0; i < u.NumFields(); i++ {
					fieldName := u.Field(i).Name()
					if val, ok := kvMap[fieldName]; ok {
						orderedArgs = append(orderedArgs, narrowArg(val, u.Field(i).Type()))
					} else {
						// Use default for missing fields (with narrowing)
						orderedArgs = append(orderedArgs, narrowArg(e.javaDefaultForGoTypeQ(u.Field(i).Type()), u.Field(i).Type()))
					}
				}
				e.fs.PushTree(IRTree(CompositeLitExpression, KindExpr,
					LeafTag(Keyword, "new ", TagJava),
					Leaf(Identifier, typeName),
					Leaf(LeftParen, "("),
					Leaf(Identifier, strings.Join(orderedArgs, ", ")),
					Leaf(RightParen, ")"),
				))
				return
			}
		}
		// Positional struct literal - add narrowing casts
		if u.NumFields() > 0 && len(elts) > 0 {
			var narrowedElts []string
			for i, elt := range elts {
				if i < u.NumFields() {
					narrowedElts = append(narrowedElts, narrowArg(elt, u.Field(i).Type()))
				} else {
					narrowedElts = append(narrowedElts, elt)
				}
			}
			eltsStr = strings.Join(narrowedElts, ", ")
		}
		e.fs.PushTree(IRTree(CompositeLitExpression, KindExpr,
			LeafTag(Keyword, "new ", TagJava),
			Leaf(Identifier, typeName),
			Leaf(LeftParen, "("),
			Leaf(Identifier, eltsStr),
			Leaf(RightParen, ")"),
		))
	case *types.Slice:
		elemType := e.qualifiedJavaTypeName(u.Elem())
		if eltsStr == "" {
			e.fs.PushTree(IRTree(CompositeLitExpression, KindExpr,
				LeafTag(Keyword, "new ", TagJava),
				Leaf(Identifier, "ArrayList"),
				Leaf(LeftAngle, "<"),
				Leaf(Identifier, toBoxedType(elemType)),
				Leaf(RightAngle, ">"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
			))
		} else {
			// Cast integer literals for byte/short slice elements
			needsCast := ""
			if basic, ok := u.Elem().Underlying().(*types.Basic); ok {
				if basic.Kind() == types.Int8 || basic.Kind() == types.Uint8 {
					needsCast = "(byte)"
				} else if basic.Kind() == types.Int16 || basic.Kind() == types.Uint16 {
					needsCast = "(short)"
				}
			}
			if needsCast != "" {
				var castElts []string
				for _, elt := range elts {
					castElts = append(castElts, needsCast+elt)
				}
				eltsStr = strings.Join(castElts, ", ")
			}
			// Check if elements contain lambdas (function type) — use double-brace init
			if _, isFuncElem := u.Elem().Underlying().(*types.Signature); isFuncElem {
				var children []IRNode
				children = append(children, LeafTag(Keyword, "new ", TagJava))
				children = append(children, Leaf(Identifier, "ArrayList"))
				children = append(children, Leaf(LeftAngle, "<"))
				children = append(children, Leaf(Identifier, toBoxedType(elemType)))
				children = append(children, Leaf(RightAngle, ">"))
				children = append(children, Leaf(LeftParen, "()"))
				children = append(children, Leaf(WhiteSpace, " "))
				children = append(children, Leaf(LeftBrace, "{{"))
				children = append(children, Leaf(WhiteSpace, " "))
				for _, elt := range elts {
					children = append(children, Leaf(Identifier, "add"))
					children = append(children, Leaf(LeftParen, "("))
					children = append(children, Leaf(Identifier, elt))
					children = append(children, Leaf(RightParen, ")"))
					children = append(children, Leaf(Semicolon, ";"))
					children = append(children, Leaf(WhiteSpace, " "))
				}
				children = append(children, Leaf(RightBrace, "}}"))
				e.fs.PushTree(IRTree(CompositeLitExpression, KindExpr, children...))
			} else {
				e.fs.PushTree(IRTree(CompositeLitExpression, KindExpr,
					LeafTag(Keyword, "new ", TagJava),
					Leaf(Identifier, "ArrayList"),
					Leaf(LeftAngle, "<"),
					Leaf(Identifier, toBoxedType(elemType)),
					Leaf(RightAngle, ">"),
					Leaf(LeftParen, "(java.util.Arrays.asList("),
					Leaf(Identifier, eltsStr),
					Leaf(RightParen, "))"),
				))
			}
		}
	case *types.Map:
		keyTypeConst := javaMapKeyTypeConst(u)
		if keyTypeConst == 100 {
			if e.structKeyTypes == nil {
				e.structKeyTypes = make(map[string]string)
			}
			if named, ok := u.Key().(*types.Named); ok {
				e.structKeyTypes[named.Obj().Name()] = named.Obj().Name()
			}
		}
		if len(elts) == 0 {
			e.fs.PushTree(IRTree(CompositeLitExpression, KindExpr,
				Leaf(Identifier, "hmap.newHashMap"),
				Leaf(LeftParen, "("),
				Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)),
				Leaf(RightParen, ")"),
			))
		} else {
			pfx, sfx := getJavaKeyCastJ(u.Key())
			e.nestedMapCounter++
			tmpVar := fmt.Sprintf("_m%d", e.nestedMapCounter)
			// Wrap in a lambda that creates and initializes
			initCode := fmt.Sprintf("((java.util.function.Supplier<Object>)(() -> { Object %s = hmap.newHashMap(%d); ", tmpVar, keyTypeConst)
			for _, elt := range elts {
				parts := strings.SplitN(elt, ": ", 2)
				if len(parts) == 2 {
					initCode += fmt.Sprintf("%s = hmap.hashMapSet(%s, %s%s%s, %s); ", tmpVar, tmpVar, pfx, parts[0], sfx, parts[1])
				}
			}
			initCode += fmt.Sprintf("return %s; })).get()", tmpVar)
			e.fs.PushTree(IRTree(CompositeLitExpression, KindExpr, Leaf(Identifier, initCode)))
		}
	default:
		elemType := "Object"
		if slice, ok := litType.(*types.Slice); ok {
			elemType = e.qualifiedJavaTypeName(slice.Elem())
		}
		if eltsStr == "" {
			e.fs.PushTree(IRTree(CompositeLitExpression, KindExpr,
				LeafTag(Keyword, "new ", TagJava),
				Leaf(Identifier, "ArrayList"),
				Leaf(LeftAngle, "<"),
				Leaf(Identifier, toBoxedType(elemType)),
				Leaf(RightAngle, ">"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.PushTree(IRTree(CompositeLitExpression, KindExpr,
				LeafTag(Keyword, "new ", TagJava),
				Leaf(Identifier, "ArrayList"),
				Leaf(LeftAngle, "<"),
				Leaf(Identifier, toBoxedType(elemType)),
				Leaf(RightAngle, ">"),
				Leaf(LeftParen, "(java.util.Arrays.asList("),
				Leaf(Identifier, eltsStr),
				Leaf(RightParen, "))"),
			))
		}
	}
}

// ============================================================
// KeyValue Expressions (for composite literals)
// ============================================================

func (e *JavaEmitter) PostVisitKeyValueExprKey(node ast.Expr, indent int) {
	keyCode := e.fs.ReduceToCode(string(PreVisitKeyValueExprKey))
	e.fs.PushCode(keyCode)
}

func (e *JavaEmitter) PostVisitKeyValueExprValue(node ast.Expr, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitKeyValueExprValue))
	e.fs.PushCode(valCode)
}

func (e *JavaEmitter) PostVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitKeyValueExpr))
	keyCode := ""
	valCode := ""
	if len(tokens) >= 1 {
		keyCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		valCode = tokens[1].Serialize()
	}
	e.fs.PushTree(IRTree(KeyValueExpression, KindExpr, Leaf(Identifier, keyCode+": "+valCode)))
}

// ============================================================
// Slice Expressions (a[lo:hi])
// ============================================================

func (e *JavaEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitSliceExprX))
	e.fs.PushCode(xCode)
}

func (e *JavaEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitSliceExprXBegin))
}

func (e *JavaEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	lowCode := e.fs.ReduceToCode(string(PreVisitSliceExprLow))
	e.fs.PushCode(lowCode)
}

func (e *JavaEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitSliceExprXEnd))
}

func (e *JavaEmitter) PostVisitSliceExprHigh(node ast.Expr, indent int) {
	highCode := e.fs.ReduceToCode(string(PreVisitSliceExprHigh))
	e.fs.PushCode(highCode)
}

func (e *JavaEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSliceExpr))
	xCode := ""
	lowCode := ""
	highCode := ""

	idx := 0
	if idx < len(tokens) {
		xCode = tokens[idx].Serialize()
		idx++
	}
	if node.Low != nil && idx < len(tokens) {
		lowCode = tokens[idx].Serialize()
		idx++
	}
	if node.High != nil && idx < len(tokens) {
		highCode = tokens[idx].Serialize()
	}

	if lowCode == "" {
		lowCode = "0"
	}

	// Check if slicing a string
	xType := e.getExprGoTypeJ(node.X)
	isString := false
	if xType != nil {
		if basic, ok := xType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
			isString = true
		}
	}

	if isString {
		if highCode == "" {
			e.fs.PushTree(IRTree(SliceExpression, KindExpr,
				Leaf(Identifier, xCode),
				Leaf(Dot, "."),
				Leaf(Identifier, "substring"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.PushTree(IRTree(SliceExpression, KindExpr,
				Leaf(Identifier, xCode),
				Leaf(Dot, "."),
				Leaf(Identifier, "substring"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(Comma, ", "),
				Leaf(Identifier, highCode),
				Leaf(RightParen, ")"),
			))
		}
	} else {
		if highCode == "" {
			e.fs.PushTree(IRTree(SliceExpression, KindExpr,
				LeafTag(Keyword, "new ", TagJava),
				Leaf(Identifier, "ArrayList<>"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, xCode),
				Leaf(Dot, "."),
				Leaf(Identifier, "subList"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(Comma, ", "),
				Leaf(Identifier, xCode),
				Leaf(Dot, "."),
				Leaf(Identifier, "size"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, "))"),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.PushTree(IRTree(SliceExpression, KindExpr,
				LeafTag(Keyword, "new ", TagJava),
				Leaf(Identifier, "ArrayList<>"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, xCode),
				Leaf(Dot, "."),
				Leaf(Identifier, "subList"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(Comma, ", "),
				Leaf(Identifier, highCode),
				Leaf(RightParen, ")"),
				Leaf(RightParen, ")"),
			))
		}
	}
}

// ============================================================
// Array Type
// ============================================================

func (e *JavaEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	typeTokens := e.fs.Reduce(string(PreVisitArrayType))
	elemType := ""
	for _, t := range typeTokens {
		elemType += t.Serialize()
	}
	if elemType == "" {
		elemType = "Object"
	}
	children := []IRNode{
		Leaf(Identifier, "ArrayList"),
		Leaf(LeftAngle, "<"),
		Leaf(Identifier, toBoxedType(elemType)),
		Leaf(RightAngle, ">"),
	}
	e.fs.PushTree(IRTree(TypeKeyword, TagType, children...))
}

// ============================================================
// Map Type
// ============================================================

func (e *JavaEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitMapKeyType))
}

func (e *JavaEmitter) PostVisitMapValueType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitMapValueType))
}

func (e *JavaEmitter) PostVisitMapType(node *ast.MapType, indent int) {
	e.fs.Reduce(string(PreVisitMapType))
	e.fs.PushTree(IRTree(MapTypeNode, KindType, Leaf(Identifier, "hmap.HashMap")))
}

// ============================================================
// Function Type (Java functional interfaces)
// ============================================================

func (e *JavaEmitter) PostVisitFuncTypeResult(node *ast.Field, index int, indent int) {
	resultCode := e.fs.ReduceToCode(string(PreVisitFuncTypeResult))
	e.fs.PushCode(resultCode)
}

func (e *JavaEmitter) PostVisitFuncTypeResults(node *ast.FieldList, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncTypeResults))
	var resultTypes []string
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			resultTypes = append(resultTypes, s)
		}
	}
	e.fs.PushCode(strings.Join(resultTypes, ", "))
}

func (e *JavaEmitter) PostVisitFuncTypeParam(node *ast.Field, index int, indent int) {
	paramCode := e.fs.ReduceToCode(string(PreVisitFuncTypeParam))
	e.fs.PushCode(paramCode)
}

func (e *JavaEmitter) PostVisitFuncTypeParams(node *ast.FieldList, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncTypeParams))
	var paramTypes []string
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			paramTypes = append(paramTypes, s)
		}
	}
	e.fs.PushCode(strings.Join(paramTypes, ", "))
}

func (e *JavaEmitter) PostVisitFuncType(node *ast.FuncType, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncType))
	resultTypes := ""
	paramTypes := ""
	if node.Results != nil && node.Results.NumFields() > 0 {
		if len(tokens) >= 1 {
			resultTypes = tokens[0].Serialize()
		}
		if len(tokens) >= 2 {
			paramTypes = tokens[1].Serialize()
		}
		resultBoxed := toBoxedJavaType(resultTypes)
		if paramTypes != "" {
			params := strings.Split(paramTypes, ", ")
			if len(params) == 1 {
				children := []IRNode{
					Leaf(Identifier, "Function"),
					Leaf(LeftAngle, "<"),
					Leaf(Identifier, toBoxedJavaType(params[0])),
					Leaf(Comma, ", "),
					Leaf(Identifier, resultBoxed),
					Leaf(RightAngle, ">"),
				}
				e.fs.PushTree(IRTree(TypeKeyword, TagExpr, children...))
			} else if len(params) == 2 {
				children := []IRNode{
					Leaf(Identifier, "BiFunction"),
					Leaf(LeftAngle, "<"),
					Leaf(Identifier, toBoxedJavaType(params[0])),
					Leaf(Comma, ", "),
					Leaf(Identifier, toBoxedJavaType(params[1])),
					Leaf(Comma, ", "),
					Leaf(Identifier, resultBoxed),
					Leaf(RightAngle, ">"),
				}
				e.fs.PushTree(IRTree(TypeKeyword, TagExpr, children...))
			} else {
				children := []IRNode{
					Leaf(Identifier, "Function"),
					Leaf(LeftAngle, "<"),
					Leaf(Identifier, toBoxedJavaType(params[0])),
					Leaf(Comma, ", "),
					Leaf(Identifier, resultBoxed),
					Leaf(RightAngle, ">"),
				}
				e.fs.PushTree(IRTree(TypeKeyword, TagExpr, children...))
			}
		} else {
			children := []IRNode{
				Leaf(Identifier, "Supplier"),
				Leaf(LeftAngle, "<"),
				Leaf(Identifier, resultBoxed),
				Leaf(RightAngle, ">"),
			}
			e.fs.PushTree(IRTree(TypeKeyword, TagExpr, children...))
		}
	} else {
		if len(tokens) >= 1 {
			paramTypes = tokens[0].Serialize()
		}
		if paramTypes != "" {
			params := strings.Split(paramTypes, ", ")
			if len(params) == 1 {
				children := []IRNode{
					Leaf(Identifier, "Consumer"),
					Leaf(LeftAngle, "<"),
					Leaf(Identifier, toBoxedJavaType(params[0])),
					Leaf(RightAngle, ">"),
				}
				e.fs.PushTree(IRTree(TypeKeyword, TagExpr, children...))
			} else if len(params) == 2 {
				children := []IRNode{
					Leaf(Identifier, "BiConsumer"),
					Leaf(LeftAngle, "<"),
					Leaf(Identifier, toBoxedJavaType(params[0])),
					Leaf(Comma, ", "),
					Leaf(Identifier, toBoxedJavaType(params[1])),
					Leaf(RightAngle, ">"),
				}
				e.fs.PushTree(IRTree(TypeKeyword, TagExpr, children...))
			} else {
				children := []IRNode{
					Leaf(Identifier, "Consumer"),
					Leaf(LeftAngle, "<"),
					Leaf(Identifier, toBoxedJavaType(params[0])),
					Leaf(RightAngle, ">"),
				}
				e.fs.PushTree(IRTree(TypeKeyword, TagExpr, children...))
			}
		} else {
			e.fs.PushCode("Runnable")
		}
	}
}

// ============================================================
// Function Literals (closures / lambdas)
// ============================================================

func (e *JavaEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	e.fs.Reduce(string(PreVisitFuncLitTypeParam))
	// For Java lambdas, omit parameter types and use inference
	// Rename params that shadow enclosing scope variables (Java doesn't allow this)
	for _, name := range node.Names {
		paramName := name.Name
		shadowed := false
		for _, fp := range e.funcParamNames {
			if fp == paramName {
				shadowed = true
				break
			}
		}
		if !shadowed && e.declaredVarNames != nil && e.declaredVarNames[paramName] {
			shadowed = true
		}
		if shadowed {
			newName := paramName + "_"
			if e.lambdaParamRenames == nil {
				e.lambdaParamRenames = make(map[string]string)
			}
			e.lambdaParamRenames[paramName] = newName
			paramName = newName
		}
		e.fs.Push(paramName, TagIdent, nil)
	}
}

func (e *JavaEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncLitTypeParams))
	var paramNames []string
	for _, t := range tokens {
		if t.Kind == TagIdent && t.Serialize() != "" {
			paramNames = append(paramNames, t.Serialize())
		}
	}
	paramsStr := strings.Join(paramNames, ", ")
	if paramsStr == "" {
		paramsStr = " "
	}
	e.fs.PushCode(paramsStr)
}

func (e *JavaEmitter) PostVisitFuncLitTypeResults(node *ast.FieldList, indent int) {
	e.fs.Reduce(string(PreVisitFuncLitTypeResults))
}

func (e *JavaEmitter) PostVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitFuncLitBody))
	e.fs.PushCode(bodyCode)
}

func (e *JavaEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncLit))
	paramsCode := ""
	bodyCode := ""
	if len(tokens) >= 1 {
		paramsCode = strings.TrimSpace(tokens[0].Serialize())
	}
	if len(tokens) >= 2 {
		bodyCode = tokens[1].Serialize()
	}

	// Apply lambda param renames to body code.
	// Per-rename regex compilation is acceptable here: word boundary matching (\b)
	// requires per-name patterns, and typically there are only 1-2 renames per lambda.
	if e.lambdaParamRenames != nil {
		for oldName, newName := range e.lambdaParamRenames {
			re := regexp.MustCompile(`\b` + regexp.QuoteMeta(oldName) + `\b`)
			bodyCode = re.ReplaceAllString(bodyCode, newName)
		}
		e.lambdaParamRenames = nil
	}

	e.fs.PushTree(IRTree(FuncLitExpression, KindExpr,
		Leaf(LeftParen, "("),
		Leaf(Identifier, paramsCode),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		LeafTag(Keyword, "->", TagJava),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, bodyCode),
	))
}

// ============================================================
// Type Assertions
// ============================================================

func (e *JavaEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	typeCode := e.fs.ReduceToCode(string(PreVisitTypeAssertExprType))
	e.fs.PushCode(typeCode)
}

func (e *JavaEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitTypeAssertExprX))
	e.fs.PushCode(xCode)
}

func (e *JavaEmitter) PostVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitTypeAssertExpr))
	typeCode := ""
	xCode := ""
	if len(tokens) >= 1 {
		typeCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		xCode = tokens[1].Serialize()
	}
	if typeCode != "" {
		e.fs.PushTree(IRTree(TypeAssertExpression, KindExpr,
			Leaf(LeftParen, "(("),
			Leaf(TypeKeyword, typeCode),
			Leaf(RightParen, ")"),
			Leaf(Identifier, xCode),
			Leaf(RightParen, ")"),
		))
	} else {
		e.fs.PushTree(IRTree(TypeAssertExpression, KindExpr, Leaf(Identifier, xCode)))
	}
}

// ============================================================
// Star Expressions (dereference - pass through in Java)
// ============================================================

func (e *JavaEmitter) PostVisitStarExpr(node *ast.StarExpr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitStarExpr))
	e.fs.PushTree(IRTree(StarExpression, KindExpr, Leaf(Identifier, xCode)))
}

// ============================================================
// Interface Type
// ============================================================

func (e *JavaEmitter) PostVisitInterfaceType(node *ast.InterfaceType, indent int) {
	e.fs.Reduce(string(PreVisitInterfaceType))
	e.fs.PushTree(IRTree(InterfaceTypeNode, KindType, Leaf(Identifier, "Object")))
}

// ============================================================
// Function Declarations
// ============================================================

func (e *JavaEmitter) PreVisitFuncDecl(node *ast.FuncDecl, indent int) {
	// Analyze closure captures for this function
	e.analyzeClosureCapturesJ(node.Body, node.Type.Params)
}

func (e *JavaEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
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

func (e *JavaEmitter) PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	typeCode := e.fs.ReduceToCode(string(PreVisitFuncDeclSignatureTypeResultsList))
	e.fs.PushCode(typeCode)
}

func (e *JavaEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeResults))
	var resultTypes []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			resultTypes = append(resultTypes, t.Serialize())
		}
	}
	if len(resultTypes) == 0 {
		e.fs.Push("void", TagType, nil)
	} else if len(resultTypes) == 1 {
		e.fs.Push(resultTypes[0], TagType, nil)
	} else {
		// Multi-return: use Object[] for now
		e.fs.Push("Object[]", TagType, nil)
	}
}

func (e *JavaEmitter) PostVisitFuncDeclName(node *ast.Ident, indent int) {
	e.fs.Reduce(string(PreVisitFuncDeclName))
	name := node.Name
	e.fs.Push(name, TagIdent, nil)
}

func (e *JavaEmitter) PostVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {
	typeCode := e.fs.ReduceToCode(string(PreVisitFuncDeclSignatureTypeParamsListType))
	e.fs.PushCode(typeCode)
}

func (e *JavaEmitter) PostVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeParamsArgName))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *JavaEmitter) PostVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeParamsList))
	typeStr := ""
	var names []string
	for _, t := range tokens {
		if t.Kind == TagExpr && typeStr == "" {
			typeStr = t.Serialize()
		} else if t.Kind == TagIdent {
			names = append(names, t.Serialize())
		}
	}
	for _, name := range names {
		e.fs.PushTree(IRTree(Identifier, TagIdent,
			Leaf(TypeKeyword, typeStr),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, name),
		))
	}
}

func (e *JavaEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeParams))
	var paramDecls []string
	for _, t := range tokens {
		if t.Kind == TagIdent {
			paramDecls = append(paramDecls, t.Serialize())
		}
	}
	// Track function parameter names for lambda shadowing detection
	e.funcParamNames = nil
	e.declaredVarNames = make(map[string]bool)
	if node.Type.Params != nil {
		for _, field := range node.Type.Params.List {
			for _, name := range field.Names {
				e.funcParamNames = append(e.funcParamNames, name.Name)
			}
		}
	}
	e.fs.PushCode(strings.Join(paramDecls, ", "))
}

func (e *JavaEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignature))
	returnType := "void"
	funcName := ""
	paramsStr := ""
	for _, t := range tokens {
		if t.Kind == TagType && returnType == "void" {
			returnType = t.Serialize()
		} else if t.Kind == TagIdent && funcName == "" {
			funcName = t.Serialize()
		} else if t.Kind == TagExpr {
			paramsStr = t.Serialize()
		}
	}

	if funcName == "main" {
		e.fs.PushTree(IRTree(Keyword, TagExpr,
			Leaf(NewLine, "\n"),
			LeafTag(Keyword, "public static void main", TagJava),
			Leaf(LeftParen, "("),
			Leaf(TypeKeyword, "String[]"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "args"),
			Leaf(RightParen, ")"),
		))
	} else {
		e.fs.PushTree(IRTree(Keyword, TagExpr,
			Leaf(NewLine, "\n"),
			LeafTag(Keyword, "public static ", TagJava),
			Leaf(TypeKeyword, returnType),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, funcName),
			Leaf(LeftParen, "("),
			Leaf(ParameterList, paramsStr),
			Leaf(RightParen, ")"),
		))
	}
}

// analyzeClosureCapturesJ identifies variables in a function body that are captured by closures
// and also mutated (assigned to), which in Java requires wrapping in Object[] arrays.
func (e *JavaEmitter) analyzeClosureCapturesJ(body *ast.BlockStmt, funcParams *ast.FieldList) {
	e.closureCapturedMutVars = nil
	e.closureCapturedVarType = nil

	if body == nil {
		return
	}

	// Collect outer (local) variable declarations and parameters
	outerVars := make(map[string]bool)
	outerVarTypes := make(map[string]types.Type)
	if funcParams != nil {
		for _, field := range funcParams.List {
			for _, name := range field.Names {
				outerVars[name.Name] = true
				if e.pkg != nil && e.pkg.TypesInfo != nil {
					if obj := e.pkg.TypesInfo.Defs[name]; obj != nil {
						outerVarTypes[name.Name] = obj.Type()
					}
				}
			}
		}
	}

	// Walk body to find local variable declarations (short var decls)
	var collectLocalVars func(stmts []ast.Stmt)
	collectLocalVars = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch s := stmt.(type) {
			case *ast.AssignStmt:
				if s.Tok == token.DEFINE {
					for _, lhs := range s.Lhs {
						if ident, ok := lhs.(*ast.Ident); ok {
							outerVars[ident.Name] = true
							if e.pkg != nil && e.pkg.TypesInfo != nil {
								if obj := e.pkg.TypesInfo.Defs[ident]; obj != nil {
									outerVarTypes[ident.Name] = obj.Type()
								}
							}
						}
					}
				}
			case *ast.DeclStmt:
				if gd, ok := s.Decl.(*ast.GenDecl); ok {
					for _, spec := range gd.Specs {
						if vs, ok := spec.(*ast.ValueSpec); ok {
							for _, name := range vs.Names {
								outerVars[name.Name] = true
								if e.pkg != nil && e.pkg.TypesInfo != nil {
									if obj := e.pkg.TypesInfo.Defs[name]; obj != nil {
										outerVarTypes[name.Name] = obj.Type()
									}
								}
							}
						}
					}
				}
			case *ast.BlockStmt:
				collectLocalVars(s.List)
			case *ast.IfStmt:
				if s.Body != nil {
					collectLocalVars(s.Body.List)
				}
				if s.Else != nil {
					if block, ok := s.Else.(*ast.BlockStmt); ok {
						collectLocalVars(block.List)
					}
				}
			case *ast.ForStmt:
				if s.Body != nil {
					collectLocalVars(s.Body.List)
				}
			case *ast.RangeStmt:
				if s.Body != nil {
					collectLocalVars(s.Body.List)
				}
			}
		}
	}
	collectLocalVars(body.List)

	// Find variables that are assigned to anywhere in the function (including inside closures)
	assignedVars := make(map[string]bool)
	ast.Inspect(body, func(n ast.Node) bool {
		switch s := n.(type) {
		case *ast.AssignStmt:
			for _, lhs := range s.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					assignedVars[ident.Name] = true
				}
			}
		case *ast.IncDecStmt:
			if ident, ok := s.X.(*ast.Ident); ok {
				assignedVars[ident.Name] = true
			}
		}
		return true
	})

	// Find variables that are captured by closures (used inside FuncLit bodies)
	capturedVars := make(map[string]bool)
	var findCaptured func(node ast.Node)
	findCaptured = func(node ast.Node) {
		if node == nil {
			return
		}
		ast.Inspect(node, func(n ast.Node) bool {
			if funcLit, ok := n.(*ast.FuncLit); ok {
				// Collect lambda parameter names to exclude
				lambdaParams := make(map[string]bool)
				if funcLit.Type != nil && funcLit.Type.Params != nil {
					for _, field := range funcLit.Type.Params.List {
						for _, name := range field.Names {
							lambdaParams[name.Name] = true
						}
					}
				}
				// Walk the lambda body looking for idents that match outer vars
				ast.Inspect(funcLit.Body, func(inner ast.Node) bool {
					if ident, ok := inner.(*ast.Ident); ok {
						if outerVars[ident.Name] && !lambdaParams[ident.Name] {
							capturedVars[ident.Name] = true
						}
					}
					// Don't recurse into nested func lits for this level
					if _, isFuncLit := inner.(*ast.FuncLit); isFuncLit && inner != funcLit.Body {
						return false
					}
					return true
				})
				return false // Don't recurse into FuncLit children again
			}
			return true
		})
	}
	findCaptured(body)

	// Variables that are captured AND assigned need wrapping
	for varName := range outerVars {
		if capturedVars[varName] && assignedVars[varName] {
			if e.closureCapturedMutVars == nil {
				e.closureCapturedMutVars = make(map[string]bool)
				e.closureCapturedVarType = make(map[string]string)
			}
			e.closureCapturedMutVars[varName] = true
			if varType, ok := outerVarTypes[varName]; ok && varType != nil {
				e.closureCapturedVarType[varName] = getJavaPrimTypeName(varType)
			}
		}
	}
}

func (e *JavaEmitter) PostVisitFuncDeclBody(node *ast.BlockStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitFuncDeclBody))
	e.fs.PushCode(bodyCode)
}

func (e *JavaEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDecl))
	sigCode := ""
	bodyCode := ""
	if len(tokens) >= 1 {
		sigCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		bodyCode = tokens[1].Serialize()
	}
	if node.Name.Name == "main" && strings.HasPrefix(bodyCode, "{\n") {
		bodyCode = "{\n" + javaIndent(2) + "ArrayList<String> goany_os_args = new ArrayList<>();\n" + javaIndent(2) + "goany_os_args.add(\"program\");\n" + javaIndent(2) + "goany_os_args.addAll(java.util.Arrays.asList(args));\n" + bodyCode[2:]
	}
	e.fs.PushTree(IRTree(FuncDeclaration, KindDecl, Leaf(Identifier, sigCode+" "+bodyCode+"\n")))
}

// ============================================================
// Forward Declaration Signatures (suppressed)
// ============================================================

func (e *JavaEmitter) PreVisitFuncDeclSignatures(indent int) {
	e.forwardDecl = true
}

func (e *JavaEmitter) PostVisitFuncDeclSignatures(indent int) {
	e.fs.Reduce(string(PreVisitFuncDeclSignatures))
	e.forwardDecl = false
}

// ============================================================
// Block Statements
// ============================================================

func (e *JavaEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	e.indent = indent
}

func (e *JavaEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	itemCode := e.fs.ReduceToCode(string(PreVisitBlockStmtList))
	e.fs.PushCode(itemCode)
}

func (e *JavaEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitBlockStmt))
	var children []IRNode
	children = append(children, Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
	for _, t := range tokens {
		if t.Serialize() != "" {
			children = append(children, Leaf(Identifier, t.Serialize()))
		}
	}
	children = append(children, Leaf(WhiteSpace, javaIndent(indent/2)), Leaf(RightBrace, "}"))
	e.fs.PushTree(IRTree(BlockStatement, KindStmt, children...))
}

// ============================================================
// Assignment Statements
// ============================================================

func (e *JavaEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	e.indent = indent
	e.mapAssignVar = ""
	e.mapAssignKey = ""
}

func (e *JavaEmitter) PostVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	lhsCode := e.fs.ReduceToCode(string(PreVisitAssignStmtLhsExpr))

	if indexExpr, ok := node.(*ast.IndexExpr); ok {
		// Save LHS index info before RHS is visited (RHS may overwrite lastIndexXCode/lastIndexKeyCode)
		e.lhsIndexXCode = e.lastIndexXCode
		e.lhsIndexKeyCode = e.lastIndexKeyCode
		if e.isMapTypeExprJ(indexExpr.X) {
			e.mapAssignVar = e.lhsIndexXCode
			e.mapAssignKey = e.lhsIndexKeyCode
			e.fs.PushCode(lhsCode)
			return
		}
	}
	e.fs.PushCode(lhsCode)
}

func (e *JavaEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmtLhs))
	var lhsExprs []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			lhsExprs = append(lhsExprs, t.Serialize())
		}
	}
	e.fs.PushCode(strings.Join(lhsExprs, ", "))
}

func (e *JavaEmitter) PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {
	rhsCode := e.fs.ReduceToCode(string(PreVisitAssignStmtRhsExpr))
	e.fs.PushCode(rhsCode)
}

func (e *JavaEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmtRhs))
	var rhsExprs []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			rhsExprs = append(rhsExprs, t.Serialize())
		}
	}
	e.fs.PushCode(strings.Join(rhsExprs, ", "))
}

func (e *JavaEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmt))
	lhsStr := ""
	rhsStr := ""
	if len(tokens) >= 1 {
		lhsStr = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		rhsStr = tokens[1].Serialize()
	}

	ind := javaIndent(indent / 2)

	// Pointer alias elimination: emit comment instead of assignment
	if len(node.Lhs) == 1 {
		if lhsIdent, ok := node.Lhs[0].(*ast.Ident); ok {
			if comment, ok := PtrLocalComments[lhsIdent.Pos()]; ok {
				e.fs.PushTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(LineComment, comment),
					Leaf(NewLine, "\n"),
				))
				return
			}
		}
	}

	tokStr := node.Tok.String()

	// Mixed index chain: nested map/slice assignments like m["outer"]["inner"] = v
	if len(node.Lhs) == 1 {
		if _, isIndex := node.Lhs[0].(*ast.IndexExpr); isIndex && e.pkg != nil && e.pkg.TypesInfo != nil {
			ops, hasIntermediateMap := e.analyzeLhsIndexChainJ(node.Lhs[0])
			if hasIntermediateMap && len(ops) >= 2 {
				// Find root variable
				rootExpr := node.Lhs[0]
				for {
					if ie, ok := rootExpr.(*ast.IndexExpr); ok {
						rootExpr = ie.X
					} else {
						break
					}
				}
				rootVar := exprToJavaString(rootExpr)

				// Assign temp var names
				currentVar := rootVar
				for i := range ops {
					if ops[i].accessType == "map" {
						ops[i].mapVarExpr = currentVar
						ops[i].tempVarName = fmt.Sprintf("__nested_inner_%d", e.nestedMapCounter)
						e.nestedMapCounter++
						currentVar = ops[i].tempVarName
					} else {
						ops[i].mapVarExpr = currentVar
						currentVar = currentVar + ".get(" + ops[i].keyExpr + ")"
					}
				}

				lastIdx := len(ops) - 1
				var children []IRNode

				// Prologue: extract temp variables for intermediate map accesses
				for i, op := range ops {
					if op.accessType == "map" && i < lastIdx {
						key := op.keyExpr
						if op.keyCastPfx != "" {
							key = op.keyCastPfx + key + op.keyCastSfx
						}
						children = append(children,
							Leaf(WhiteSpace, ind),
							LeafTag(Keyword, "var ", TagJava),
							Leaf(Identifier, op.tempVarName),
							Leaf(WhiteSpace, " "),
							Leaf(Assignment, "="),
							Leaf(WhiteSpace, " "),
							Leaf(LeftParen, "("),
							Leaf(TypeKeyword, op.valueJavaType),
							Leaf(RightParen, ")"),
							Leaf(Identifier, "hmap.hashMapGet"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, op.mapVarExpr),
							Leaf(Comma, ","),
							Leaf(WhiteSpace, " "),
							Leaf(Identifier, key),
							Leaf(RightParen, ")"),
							Leaf(Semicolon, ";"),
							Leaf(NewLine, "\n"),
						)
					}
				}

				// Assignment
				lastOp := ops[lastIdx]
				if lastOp.accessType == "map" {
					key := lastOp.keyExpr
					if lastOp.keyCastPfx != "" {
						key = lastOp.keyCastPfx + key + lastOp.keyCastSfx
					}
					// If mapVarExpr contains .get(), it's inside a slice — use .set() to write back
					if strings.Contains(lastOp.mapVarExpr, ".get(") {
						lastGetIdx := strings.LastIndex(lastOp.mapVarExpr, ".get(")
						parentExpr := lastOp.mapVarExpr[:lastGetIdx]
						closeParen := strings.LastIndex(lastOp.mapVarExpr, ")")
						idxExpr := lastOp.mapVarExpr[lastGetIdx+5 : closeParen]
						newMapVal := fmt.Sprintf("hmap.hashMapSet(%s, %s, %s)",
							lastOp.mapVarExpr, key, rhsStr)
						children = append(children,
							Leaf(WhiteSpace, ind),
							Leaf(Identifier, parentExpr),
							Leaf(Dot, "."),
							Leaf(Identifier, "set"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, idxExpr),
							Leaf(Comma, ","),
							Leaf(WhiteSpace, " "),
							Leaf(Identifier, newMapVal),
							Leaf(RightParen, ")"),
							Leaf(Semicolon, ";"),
							Leaf(NewLine, "\n"),
						)
					} else {
						children = append(children,
							Leaf(WhiteSpace, ind),
							Leaf(Identifier, lastOp.mapVarExpr),
							Leaf(WhiteSpace, " "),
							Leaf(Assignment, "="),
							Leaf(WhiteSpace, " "),
							Leaf(Identifier, "hmap.hashMapSet"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, lastOp.mapVarExpr),
							Leaf(Comma, ","),
							Leaf(WhiteSpace, " "),
							Leaf(Identifier, key),
							Leaf(Comma, ","),
							Leaf(WhiteSpace, " "),
							Leaf(Identifier, rhsStr),
							Leaf(RightParen, ")"),
							Leaf(Semicolon, ";"),
							Leaf(NewLine, "\n"),
						)
					}
				} else {
					children = append(children,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, lastOp.mapVarExpr),
						Leaf(Dot, "."),
						Leaf(Identifier, "set"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, lastOp.keyExpr),
						Leaf(Comma, ","),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, rhsStr),
						Leaf(RightParen, ")"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					)
				}

				// Epilogue: write back intermediate maps in reverse
				for i := lastIdx - 1; i >= 0; i-- {
					op := ops[i]
					if op.accessType == "map" {
						key := op.keyExpr
						if op.keyCastPfx != "" {
							key = op.keyCastPfx + key + op.keyCastSfx
						}
						children = append(children,
							Leaf(WhiteSpace, ind),
							Leaf(Identifier, op.mapVarExpr),
							Leaf(WhiteSpace, " "),
							Leaf(Assignment, "="),
							Leaf(WhiteSpace, " "),
							Leaf(Identifier, "hmap.hashMapSet"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, op.mapVarExpr),
							Leaf(Comma, ","),
							Leaf(WhiteSpace, " "),
							Leaf(Identifier, key),
							Leaf(Comma, ","),
							Leaf(WhiteSpace, " "),
							Leaf(Identifier, op.tempVarName),
							Leaf(RightParen, ")"),
							Leaf(Semicolon, ";"),
							Leaf(NewLine, "\n"),
						)
					}
				}
				e.fs.PushTree(IRTree(AssignStatement, KindStmt, children...))
				e.mapAssignVar = ""
				e.mapAssignKey = ""
				return
			}
		}
	}

	// Slice index assignment: x.set(idx, val)
	if len(node.Lhs) == 1 && e.mapAssignVar == "" {
		if indexExpr, ok := node.Lhs[0].(*ast.IndexExpr); ok {
			xType := e.getExprGoTypeJ(indexExpr.X)
			if xType != nil {
				if sliceType, isSlice := xType.Underlying().(*types.Slice); isSlice {
					xCode := e.lhsIndexXCode
					idxCode := e.lhsIndexKeyCode
					// Handle compound assignment operators (+=, -=, *=, /=, %=)
					valStr := rhsStr
					if node.Tok == token.ADD_ASSIGN {
						valStr = fmt.Sprintf("%s.get(%s) + %s", xCode, idxCode, rhsStr)
					} else if node.Tok == token.SUB_ASSIGN {
						valStr = fmt.Sprintf("%s.get(%s) - %s", xCode, idxCode, rhsStr)
					} else if node.Tok == token.MUL_ASSIGN {
						valStr = fmt.Sprintf("%s.get(%s) * %s", xCode, idxCode, rhsStr)
					} else if node.Tok == token.QUO_ASSIGN {
						valStr = fmt.Sprintf("%s.get(%s) / %s", xCode, idxCode, rhsStr)
					} else if node.Tok == token.REM_ASSIGN {
						valStr = fmt.Sprintf("%s.get(%s) %% %s", xCode, idxCode, rhsStr)
					}
					// Add byte/short narrowing for slice element type
					if basic, ok := sliceType.Elem().Underlying().(*types.Basic); ok {
						if basic.Kind() == types.Int8 || basic.Kind() == types.Uint8 {
							valStr = "(byte)(" + valStr + ")"
						} else if basic.Kind() == types.Int16 || basic.Kind() == types.Uint16 {
							valStr = "(short)(" + valStr + ")"
						}
					}
					e.fs.PushTree(IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, xCode),
						Leaf(Dot, "."),
						Leaf(Identifier, "set"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, idxCode),
						Leaf(Comma, ", "),
						Leaf(Identifier, valStr),
						Leaf(RightParen, ")"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
					return
				}
			}
		}
	}

	// Map assignment: m[k] = v -> hmap.hashMapSet(m, k, v)
	if e.mapAssignVar != "" && e.mapAssignKey != "" {
		mapGoType := e.getExprGoTypeJ(node.Lhs[0].(*ast.IndexExpr).X)
		pfx := ""
		sfx := ""
		if mapGoType != nil {
			if mapUnderlying, ok := mapGoType.Underlying().(*types.Map); ok {
				pfx, sfx = getJavaKeyCastJ(mapUnderlying.Key())
			}
		}
		// If mapAssignVar contains .get(), it's a slice-of-map pattern
		// e.g., sliceOfMaps.get(0) = hashMapSet(...) -> sliceOfMaps.set(0, hashMapSet(...))
		if strings.Contains(e.mapAssignVar, ".get(") {
			lastGetIdx := strings.LastIndex(e.mapAssignVar, ".get(")
			parentExpr := e.mapAssignVar[:lastGetIdx]
			closeParen := strings.LastIndex(e.mapAssignVar, ")")
			idxExpr := e.mapAssignVar[lastGetIdx+5 : closeParen]
			newMapVal := "hmap.hashMapSet(" + e.mapAssignVar + ", " + pfx + e.mapAssignKey + sfx + ", " + rhsStr + ")"
			e.fs.PushTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, parentExpr),
				Leaf(Dot, "."),
				Leaf(Identifier, "set"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, idxExpr),
				Leaf(Comma, ", "),
				Leaf(Identifier, newMapVal),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		} else {
			e.fs.PushTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, e.mapAssignVar),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "hmap.hashMapSet"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, e.mapAssignVar),
				Leaf(Comma, ","),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, pfx+e.mapAssignKey+sfx),
				Leaf(Comma, ","),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, rhsStr),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		}
		e.mapAssignVar = ""
		e.mapAssignKey = ""
		return
	}

	// Comma-ok map read: val, ok := m[key]
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if indexExpr, ok := node.Rhs[0].(*ast.IndexExpr); ok {
			if e.isMapTypeExprJ(indexExpr.X) {
				valName := exprToString(node.Lhs[0])
				okName := exprToString(node.Lhs[1])
				mapName := exprToString(indexExpr.X)
				keyStr := exprToString(indexExpr.Index)

				mapGoType := e.getExprGoTypeJ(indexExpr.X)
				valType := "Object"
				pfx := ""
				sfx := ""
				zeroVal := "null"
				if mapGoType != nil {
					if mapUnderlying, ok2 := mapGoType.Underlying().(*types.Map); ok2 {
						valType = e.qualifiedJavaTypeName(mapUnderlying.Elem())
						pfx, sfx = getJavaKeyCastJ(mapUnderlying.Key())
						zeroVal = e.javaDefaultForGoTypeQ(mapUnderlying.Elem())
					}
				}
				if tokStr == ":=" {
					e.fs.PushTree(IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						LeafTag(Keyword, "var ", TagJava),
						Leaf(Identifier, okName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "hmap.hashMapContains"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, mapName),
						Leaf(Comma, ","),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, pfx+keyStr+sfx),
						Leaf(RightParen, ")"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
					e.fs.PushTree(IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						LeafTag(Keyword, "var ", TagJava),
						Leaf(Identifier, valName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, okName),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "?"),
						Leaf(WhiteSpace, " "),
						Leaf(LeftParen, "("),
						Leaf(TypeKeyword, valType),
						Leaf(RightParen, ")"),
						Leaf(Identifier, "hmap.hashMapGet"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, mapName),
						Leaf(Comma, ","),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, pfx+keyStr+sfx),
						Leaf(RightParen, ")"),
						Leaf(WhiteSpace, " "),
						Leaf(Colon, ":"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, zeroVal),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
				} else {
					e.fs.PushTree(IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, okName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "hmap.hashMapContains"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, mapName),
						Leaf(Comma, ","),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, pfx+keyStr+sfx),
						Leaf(RightParen, ")"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
					e.fs.PushTree(IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, valName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, okName),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "?"),
						Leaf(WhiteSpace, " "),
						Leaf(LeftParen, "("),
						Leaf(TypeKeyword, valType),
						Leaf(RightParen, ")"),
						Leaf(Identifier, "hmap.hashMapGet"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, mapName),
						Leaf(Comma, ","),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, pfx+keyStr+sfx),
						Leaf(RightParen, ")"),
						Leaf(WhiteSpace, " "),
						Leaf(Colon, ":"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, zeroVal),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
				}
				return
			}
		}
	}

	// Comma-ok type assertion: val, ok := x.(Type)
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if typeAssert, ok := node.Rhs[0].(*ast.TypeAssertExpr); ok {
			valName := exprToString(node.Lhs[0])
			okName := exprToString(node.Lhs[1])
			assertType := ""
			if typeAssert.Type != nil {
				assertType = exprToString(typeAssert.Type)
				if javaType, ok := javaTypesMap[assertType]; ok {
					assertType = javaType
				}
			}
			boxedAssertType := toBoxedType(assertType)
			xExpr := exprToString(typeAssert.X)
			if tokStr == ":=" {
				e.fs.PushTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					LeafTag(Keyword, "var ", TagJava),
					Leaf(Identifier, okName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, xExpr),
					Leaf(WhiteSpace, " "),
					LeafTag(Keyword, "instanceof", TagJava),
					Leaf(WhiteSpace, " "),
					Leaf(TypeKeyword, boxedAssertType),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
				e.fs.PushTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					LeafTag(Keyword, "var ", TagJava),
					Leaf(Identifier, valName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, okName),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "?"),
					Leaf(WhiteSpace, " "),
					Leaf(LeftParen, "("),
					Leaf(TypeKeyword, assertType),
					Leaf(RightParen, ")"),
					Leaf(Identifier, xExpr),
					Leaf(WhiteSpace, " "),
					Leaf(Colon, ":"),
					Leaf(WhiteSpace, " "),
					LeafTag(Keyword, "null", TagJava),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
			} else {
				e.fs.PushTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, okName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, xExpr),
					Leaf(WhiteSpace, " "),
					LeafTag(Keyword, "instanceof", TagJava),
					Leaf(WhiteSpace, " "),
					Leaf(TypeKeyword, boxedAssertType),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
				e.fs.PushTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, valName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, okName),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "?"),
					Leaf(WhiteSpace, " "),
					Leaf(LeftParen, "("),
					Leaf(TypeKeyword, assertType),
					Leaf(RightParen, ")"),
					Leaf(Identifier, xExpr),
					Leaf(WhiteSpace, " "),
					Leaf(Colon, ":"),
					Leaf(WhiteSpace, " "),
					LeafTag(Keyword, "null", TagJava),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
			}
			return
		}
	}

	// Multi-value return unpacking: a, b := func() -> var result = func(); var a = result[0]; ...
	if len(node.Lhs) > 1 && len(node.Rhs) == 1 {
		tmpVar := fmt.Sprintf("_mr%d", e.nestedMapCounter)
		e.nestedMapCounter++

		// Get return types from the RHS function signature
		var returnTypes []types.Type
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if callExpr, ok := node.Rhs[0].(*ast.CallExpr); ok {
				funType := e.getExprGoTypeJ(callExpr.Fun)
				if funType != nil {
					if sig, ok := funType.Underlying().(*types.Signature); ok {
						results := sig.Results()
						for ri := 0; ri < results.Len(); ri++ {
							returnTypes = append(returnTypes, results.At(ri).Type())
						}
					}
				}
			}
		}

		// Helper to get Go type for LHS at index
		getLhsGoType := func(i int, lhs ast.Expr) types.Type {
			if i < len(returnTypes) && returnTypes[i] != nil {
				return returnTypes[i]
			}
			if ident, ok := lhs.(*ast.Ident); ok && e.pkg != nil && e.pkg.TypesInfo != nil {
				if obj := e.pkg.TypesInfo.Defs[ident]; obj != nil && obj.Type() != nil {
					return obj.Type()
				}
				if obj := e.pkg.TypesInfo.Uses[ident]; obj != nil && obj.Type() != nil {
					return obj.Type()
				}
			}
			return e.getExprGoTypeJ(lhs)
		}

		// Generate the cast expression for a multi-return element
		// For narrow types (byte/short), use Number methods to avoid ClassCastException
		genCast := func(goType types.Type, arrExpr string) string {
			javaType := "Object"
			if goType != nil {
				// Pointer types are transformed to int pool indices by pointer transform
				if _, isPtr := goType.(*types.Pointer); isPtr {
					return fmt.Sprintf("((Number)%s).intValue()", arrExpr)
				}
				// []*T types are transformed to []int (ArrayList<Integer>) by pointer transform
				if sliceType, isSlice := goType.(*types.Slice); isSlice {
					if _, isPtr := sliceType.Elem().(*types.Pointer); isPtr {
						return fmt.Sprintf("(ArrayList<Integer>)%s", arrExpr)
					}
				}
				javaType = e.qualifiedJavaTypeName(goType)
				if basic, ok := goType.Underlying().(*types.Basic); ok {
					switch basic.Kind() {
					case types.Int8, types.Uint8:
						return fmt.Sprintf("((Number)%s).byteValue()", arrExpr)
					case types.Int16, types.Uint16:
						return fmt.Sprintf("((Number)%s).shortValue()", arrExpr)
					case types.Int, types.Int32, types.Uint32:
						return fmt.Sprintf("((Number)%s).intValue()", arrExpr)
					case types.Int64, types.Uint64:
						return fmt.Sprintf("((Number)%s).longValue()", arrExpr)
					case types.Float32:
						return fmt.Sprintf("((Number)%s).floatValue()", arrExpr)
					case types.Float64:
						return fmt.Sprintf("((Number)%s).doubleValue()", arrExpr)
					case types.Bool:
						return fmt.Sprintf("(boolean)%s", arrExpr)
					case types.String:
						return fmt.Sprintf("(String)%s", arrExpr)
					}
				}
			}
			return fmt.Sprintf("(%s)%s", javaType, arrExpr)
		}

		var children []IRNode
		if tokStr == ":=" {
			children = append(children,
				Leaf(WhiteSpace, ind),
				LeafTag(Keyword, "var ", TagJava),
				Leaf(Identifier, tmpVar),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, rhsStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
			for i, lhs := range node.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && ident.Name == "_" {
					continue
				}
				lhsName := exprToString(lhs)
				goType := getLhsGoType(i, lhs)
				castExpr := genCast(goType, fmt.Sprintf("%s[%d]", tmpVar, i))
				// Check if this LHS var is closure-captured
				if ident, ok := lhs.(*ast.Ident); ok && e.closureCapturedMutVars != nil && e.closureCapturedMutVars[ident.Name] {
					children = append(children,
						Leaf(WhiteSpace, ind),
						Leaf(TypeKeyword, "Object[]"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, lhsName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(LeftBrace, "{"),
						Leaf(Identifier, castExpr),
						Leaf(RightBrace, "}"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					)
				} else {
					children = append(children,
						Leaf(WhiteSpace, ind),
						LeafTag(Keyword, "var ", TagJava),
						Leaf(Identifier, lhsName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, castExpr),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					)
				}
			}
		} else {
			children = append(children,
				Leaf(WhiteSpace, ind),
				LeafTag(Keyword, "var ", TagJava),
				Leaf(Identifier, tmpVar),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, rhsStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
			for i, lhs := range node.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok && ident.Name == "_" {
					continue
				}
				lhsName := exprToString(lhs)
				goType := getLhsGoType(i, lhs)
				castExpr := genCast(goType, fmt.Sprintf("%s[%d]", tmpVar, i))
				// Check if this LHS var is closure-captured
				if ident, ok := lhs.(*ast.Ident); ok && e.closureCapturedMutVars != nil && e.closureCapturedMutVars[ident.Name] {
					children = append(children,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, lhsName),
						Leaf(LeftBracket, "["),
						Leaf(NumberLiteral, "0"),
						Leaf(RightBracket, "]"),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, castExpr),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					)
				} else {
					children = append(children,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, lhsName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, castExpr),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					)
				}
			}
		}
		e.fs.PushTree(IRTree(AssignStatement, KindStmt, children...))
		return
	}

	// Check if LHS needs narrowing cast (byte, short)
	narrowCast := ""
	if len(node.Lhs) == 1 {
		if lhsType := e.getExprGoTypeJ(node.Lhs[0]); lhsType != nil {
			if basic, ok := lhsType.Underlying().(*types.Basic); ok {
				switch basic.Kind() {
				case types.Int8, types.Uint8:
					narrowCast = "(byte)"
				case types.Int16:
					narrowCast = "(short)"
				}
			}
		}
	}

	// Track declared variable names for lambda shadowing detection
	if tokStr == ":=" && e.declaredVarNames != nil {
		for _, lhs := range node.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok && ident.Name != "_" {
				e.declaredVarNames[ident.Name] = true
			}
		}
	}

	// Check if LHS is a closure-captured mutable variable
	isClosureCaptured := false
	closureCapturedName := ""
	if len(node.Lhs) == 1 {
		if ident, ok := node.Lhs[0].(*ast.Ident); ok && e.closureCapturedMutVars != nil && e.closureCapturedMutVars[ident.Name] {
			isClosureCaptured = true
			closureCapturedName = ident.Name
		}
	}

	switch tokStr {
	case ":=":
		if isClosureCaptured {
			// Declaration of closure-captured variable: Object[] name = {rhsStr};
			e.fs.PushTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(TypeKeyword, "Object[]"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, closureCapturedName),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(LeftBrace, "{"),
				Leaf(Identifier, rhsStr),
				Leaf(RightBrace, "}"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		} else if narrowCast != "" {
			// Use explicit type for narrow types
			lhsType := e.getExprGoTypeJ(node.Lhs[0])
			javaType := getJavaPrimTypeName(lhsType)
			e.fs.PushTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(TypeKeyword, javaType),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, lhsStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, narrowCast),
				Leaf(LeftParen, "("),
				Leaf(Identifier, rhsStr),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		} else {
			// Check if RHS is a func literal — Java can't infer lambda type with var
			usedExplicitType := false
			if len(node.Rhs) == 1 {
				if _, isFuncLit := node.Rhs[0].(*ast.FuncLit); isFuncLit {
					// For new definitions, get type from Defs
					var lhsType types.Type
					if ident, ok := node.Lhs[0].(*ast.Ident); ok && e.pkg != nil && e.pkg.TypesInfo != nil {
						if obj := e.pkg.TypesInfo.Defs[ident]; obj != nil {
							lhsType = obj.Type()
						}
					}
					if lhsType == nil {
						lhsType = e.getExprGoTypeJ(node.Lhs[0])
					}
					if lhsType != nil {
						if sig, ok := lhsType.Underlying().(*types.Signature); ok {
							funcType := e.getJavaFuncInterfaceType(sig)
							e.fs.PushTree(IRTree(AssignStatement, KindStmt,
								Leaf(WhiteSpace, ind),
								Leaf(TypeKeyword, funcType),
								Leaf(WhiteSpace, " "),
								Leaf(Identifier, lhsStr),
								Leaf(WhiteSpace, " "),
								Leaf(Assignment, "="),
								Leaf(WhiteSpace, " "),
								Leaf(Identifier, rhsStr),
								Leaf(Semicolon, ";"),
								Leaf(NewLine, "\n"),
							))
							usedExplicitType = true
						}
					}
				}
			}
			if !usedExplicitType {
				e.fs.PushTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					LeafTag(Keyword, "var ", TagJava),
					Leaf(Identifier, lhsStr),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, rhsStr),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
			}
		}
	case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
		if isClosureCaptured {
			// Compound assignment to captured var: name[0] = ((Type)name[0]) op rhsStr
			lhs := closureUnwrapLhs(lhsStr)
			e.fs.PushTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, lhs),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, lhsStr),
				Leaf(WhiteSpace, " "),
				Leaf(BinaryOperator, tokStr[:len(tokStr)-1]),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, rhsStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		} else if narrowCast != "" {
			e.fs.PushTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, lhsStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, narrowCast),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lhsStr),
				Leaf(WhiteSpace, " "),
				Leaf(BinaryOperator, tokStr[:len(tokStr)-1]),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, rhsStr),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		} else {
			e.fs.PushTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, lhsStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, tokStr),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, rhsStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		}
	default:
		if isClosureCaptured {
			// Simple assignment to captured var: name[0] = rhsStr
			lhs := closureUnwrapLhs(lhsStr)
			e.fs.PushTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, lhs),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, rhsStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		} else if narrowCast != "" {
			e.fs.PushTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, lhsStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, narrowCast),
				Leaf(LeftParen, "("),
				Leaf(Identifier, rhsStr),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		} else {
			e.fs.PushTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, lhsStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, rhsStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		}
	}
}

// ============================================================
// Declaration Statements (var x int, var y = 5)
// ============================================================

func (e *JavaEmitter) PreVisitDeclStmt(node *ast.DeclStmt, indent int) {
	e.indent = indent
}

func (e *JavaEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	tokens := e.fs.Reduce(string(PreVisitDeclStmtValueSpecType))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Serialize()
	}
	var goType types.Type
	if e.pkg != nil && e.pkg.TypesInfo != nil && index < len(node.Names) {
		if obj := e.pkg.TypesInfo.Defs[node.Names[index]]; obj != nil {
			goType = obj.Type()
		}
	}
	e.fs.Push(typeStr, TagType, goType)
}

func (e *JavaEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	e.fs.Reduce(string(PreVisitDeclStmtValueSpecNames))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *JavaEmitter) PostVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitDeclStmtValueSpecValue))
	e.fs.Push(valCode, TagExpr, nil)
}

func (e *JavaEmitter) PostVisitDeclStmt(node *ast.DeclStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitDeclStmt))
	ind := javaIndent(indent / 2)

	var children []IRNode
	i := 0
	for i < len(tokens) {
		typeStr := ""
		var goType types.Type
		nameStr := ""
		valueStr := ""

		if i < len(tokens) && tokens[i].Kind == TagType {
			typeStr = tokens[i].Serialize()
			goType = tokens[i].GoType
			i++
		}
		if i < len(tokens) && tokens[i].Kind == TagIdent {
			nameStr = tokens[i].Serialize()
			i++
		}
		if i < len(tokens) && tokens[i].Kind == TagExpr {
			valueStr = tokens[i].Serialize()
			i++
		}

		if nameStr == "" {
			continue
		}

		// Check if this is a closure-captured variable
		isCaptured := e.closureCapturedMutVars != nil && e.closureCapturedMutVars[nameStr]

		if valueStr != "" {
			if isCaptured {
				children = append(children,
					Leaf(WhiteSpace, ind),
					Leaf(TypeKeyword, "Object[]"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, nameStr),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(LeftBrace, "{"),
					Leaf(Identifier, valueStr),
					Leaf(RightBrace, "}"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				)
			} else {
				children = append(children,
					Leaf(WhiteSpace, ind),
					Leaf(TypeKeyword, typeStr),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, nameStr),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, valueStr),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				)
			}
		} else {
			// No initializer - generate default value
			defaultVal := "null"
			if goType != nil {
				if _, isSlice := goType.Underlying().(*types.Slice); isSlice {
					defaultVal = fmt.Sprintf("new %s()", typeStr)
				} else if _, isMap := goType.Underlying().(*types.Map); isMap {
					if e.pkg != nil && e.pkg.TypesInfo != nil {
						for _, spec := range node.Decl.(*ast.GenDecl).Specs {
							if vs, ok := spec.(*ast.ValueSpec); ok {
								if mapType, ok := vs.Type.(*ast.MapType); ok {
									keyConst := e.getMapKeyTypeConstJ(mapType)
									defaultVal = fmt.Sprintf("hmap.newHashMap(%d)", keyConst)
								}
							}
						}
					}
				} else if _, isStruct := goType.Underlying().(*types.Struct); isStruct {
					defaultVal = fmt.Sprintf("new %s()", typeStr)
				} else {
					defaultVal = e.javaDefaultForGoTypeQ(goType)
				}
			} else {
				// goType is nil (e.g., synthetic var decls from pointer transform)
				// Infer default from the Java type string
				switch typeStr {
				case "int", "long", "short", "byte", "float", "double":
					defaultVal = "0"
				case "boolean":
					defaultVal = "false"
				case "String":
					defaultVal = `""`
				}
			}
			if isCaptured {
				children = append(children,
					Leaf(WhiteSpace, ind),
					Leaf(TypeKeyword, "Object[]"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, nameStr),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(LeftBrace, "{"),
					Leaf(Identifier, defaultVal),
					Leaf(RightBrace, "}"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				)
			} else {
				children = append(children,
					Leaf(WhiteSpace, ind),
					Leaf(TypeKeyword, typeStr),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, nameStr),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, defaultVal),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				)
			}
		}
	}
	e.fs.PushTree(IRTree(DeclStatement, KindStmt, children...))
}

// ============================================================
// Return Statements
// ============================================================

func (e *JavaEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	e.indent = indent
}

func (e *JavaEmitter) PostVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	resultCode := e.fs.ReduceToCode(string(PreVisitReturnStmtResult))
	e.fs.PushCode(resultCode)
}

func (e *JavaEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitReturnStmt))
	ind := javaIndent(indent / 2)

	if len(tokens) == 0 {
		e.fs.PushTree(IRTree(ReturnStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	} else if len(tokens) == 1 {
		retExpr := tokens[0].Serialize()
		// Add narrowing cast if return type is narrower than int
		if e.funcReturnType != nil {
			if basic, ok := e.funcReturnType.Underlying().(*types.Basic); ok {
				switch basic.Kind() {
				case types.Int8, types.Uint8:
					retExpr = "(byte)(" + retExpr + ")"
				case types.Int16:
					retExpr = "(short)(" + retExpr + ")"
				}
			}
		}
		e.fs.PushTree(IRTree(ReturnStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, retExpr),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	} else {
		// Multi-value return: return new Object[]{a, b}
		var vals []string
		for _, t := range tokens {
			vals = append(vals, t.Serialize())
		}
		e.fs.PushTree(IRTree(ReturnStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return"),
			Leaf(WhiteSpace, " "),
			LeafTag(Keyword, "new Object[]", TagJava),
			Leaf(LeftBrace, "{"),
			Leaf(Identifier, strings.Join(vals, ", ")),
			Leaf(RightBrace, "}"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	}
}

// ============================================================
// Expression Statements
// ============================================================

func (e *JavaEmitter) PreVisitExprStmt(node *ast.ExprStmt, indent int) {
	e.indent = indent
}

func (e *JavaEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitExprStmtX))
	e.fs.PushCode(xCode)
}

func (e *JavaEmitter) PostVisitExprStmt(node *ast.ExprStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitExprStmt))
	code := ""
	if len(tokens) >= 1 {
		code = tokens[0].Serialize()
	}
	ind := javaIndent(indent / 2)
	e.fs.PushTree(IRTree(ExprStatement, KindStmt,
		Leaf(WhiteSpace, ind),
		Leaf(Identifier, code),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
	))
}

// ============================================================
// If Statements
// ============================================================

func (e *JavaEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	e.indent = indent
	e.ifInitStack = append(e.ifInitStack, "")
	e.ifCondStack = append(e.ifCondStack, "")
	e.ifBodyStack = append(e.ifBodyStack, "")
	e.ifElseStack = append(e.ifElseStack, "")
}

func (e *JavaEmitter) PostVisitIfStmtInit(node ast.Stmt, indent int) {
	e.ifInitStack[len(e.ifInitStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtInit))
}

func (e *JavaEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	e.ifCondStack[len(e.ifCondStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtCond))
}

func (e *JavaEmitter) PostVisitIfStmtBody(node *ast.IfStmt, indent int) {
	e.ifBodyStack[len(e.ifBodyStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtBody))
}

func (e *JavaEmitter) PostVisitIfStmtElse(node *ast.IfStmt, indent int) {
	e.ifElseStack[len(e.ifElseStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtElse))
}

func (e *JavaEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	e.fs.Reduce(string(PreVisitIfStmt))
	ind := javaIndent(indent / 2)

	n := len(e.ifInitStack)
	initCode := e.ifInitStack[n-1]
	condCode := e.ifCondStack[n-1]
	bodyCode := e.ifBodyStack[n-1]
	elseCode := e.ifElseStack[n-1]
	e.ifInitStack = e.ifInitStack[:n-1]
	e.ifCondStack = e.ifCondStack[:n-1]
	e.ifBodyStack = e.ifBodyStack[:n-1]
	e.ifElseStack = e.ifElseStack[:n-1]

	var children []IRNode
	if initCode != "" {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(LeftBrace, "{"),
			Leaf(NewLine, "\n"),
			Leaf(Identifier, initCode),
			Leaf(WhiteSpace, ind),
			Leaf(IfKeyword, "if"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			Leaf(Identifier, condCode),
			Leaf(RightParen, ")"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, bodyCode),
		)
	} else {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(IfKeyword, "if"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			Leaf(Identifier, condCode),
			Leaf(RightParen, ")"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, bodyCode),
		)
	}
	if elseCode != "" {
		trimmed := strings.TrimLeft(elseCode, " \t\n")
		if strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "if(") {
			children = append(children,
				Leaf(WhiteSpace, " "),
				Leaf(ElseKeyword, "else"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, trimmed),
			)
		} else {
			children = append(children,
				Leaf(WhiteSpace, " "),
				Leaf(ElseKeyword, "else"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, elseCode),
			)
		}
	}
	children = append(children, Leaf(NewLine, "\n"))
	if initCode != "" {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(RightBrace, "}"),
			Leaf(NewLine, "\n"),
		)
	}
	e.fs.PushTree(IRTree(IfStatement, KindStmt, children...))
}

// ============================================================
// For Statements
// ============================================================

func (e *JavaEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	e.indent = indent
	e.forInitStack = append(e.forInitStack, "")
	e.forCondStack = append(e.forCondStack, "")
	e.forPostStack = append(e.forPostStack, "")
}

func (e *JavaEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	initCode := e.fs.ReduceToCode(string(PreVisitForStmtInit))
	initCode = strings.TrimRight(initCode, ";\n \t")
	initCode = strings.TrimLeft(initCode, " \t")
	e.forInitStack[len(e.forInitStack)-1] = initCode
}

func (e *JavaEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	e.forCondStack[len(e.forCondStack)-1] = e.fs.ReduceToCode(string(PreVisitForStmtCond))
}

func (e *JavaEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	postCode := e.fs.ReduceToCode(string(PreVisitForStmtPost))
	postCode = strings.TrimRight(postCode, ";\n \t")
	postCode = strings.TrimLeft(postCode, " \t")
	e.forPostStack[len(e.forPostStack)-1] = postCode
}

func (e *JavaEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitForStmt))
	ind := javaIndent(indent / 2)

	n := len(e.forInitStack)
	initCode := e.forInitStack[n-1]
	condCode := e.forCondStack[n-1]
	postCode := e.forPostStack[n-1]
	e.forInitStack = e.forInitStack[:n-1]
	e.forCondStack = e.forCondStack[:n-1]
	e.forPostStack = e.forPostStack[:n-1]

	if node.Init == nil && node.Cond == nil && node.Post == nil {
		e.fs.PushTree(IRTree(ForStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(WhileKeyword, "while"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			Leaf(BooleanLiteral, "true"),
			Leaf(RightParen, ")"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, bodyCode),
			Leaf(NewLine, "\n"),
		))
		return
	}

	if node.Init == nil && node.Post == nil && node.Cond != nil {
		e.fs.PushTree(IRTree(ForStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(WhileKeyword, "while"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			Leaf(Identifier, condCode),
			Leaf(RightParen, ")"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, bodyCode),
			Leaf(NewLine, "\n"),
		))
		return
	}

	e.fs.PushTree(IRTree(ForStatement, KindStmt,
		Leaf(WhiteSpace, ind),
		Leaf(ForKeyword, "for"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftParen, "("),
		Leaf(Identifier, initCode),
		Leaf(Semicolon, "; "),
		Leaf(Identifier, condCode),
		Leaf(Semicolon, "; "),
		Leaf(Identifier, postCode),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, bodyCode),
		Leaf(NewLine, "\n"),
	))
}

// ============================================================
// Range Statements
// ============================================================

func (e *JavaEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	e.indent = indent
}

func (e *JavaEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	keyCode := e.fs.ReduceToCode(string(PreVisitRangeStmtKey))
	e.fs.Push(keyCode, TagIdent, nil)
}

func (e *JavaEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitRangeStmtValue))
	e.fs.Push(valCode, TagIdent, nil)
}

func (e *JavaEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitRangeStmtX))
	e.fs.PushCode(xCode)
}

func (e *JavaEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitRangeStmt))
	ind := javaIndent(indent / 2)

	keyCode := ""
	valCode := ""
	xCode := ""
	bodyCode := ""

	idx := 0
	if node.Key != nil {
		if idx < len(tokens) && tokens[idx].Kind == TagIdent {
			keyCode = tokens[idx].Serialize()
			idx++
		}
	}
	if node.Value != nil {
		if idx < len(tokens) && tokens[idx].Kind == TagIdent {
			valCode = tokens[idx].Serialize()
			idx++
		}
	}
	if idx < len(tokens) {
		xCode = tokens[idx].Serialize()
		idx++
	}
	if idx < len(tokens) {
		bodyCode = tokens[idx].Serialize()
	}
	if node.Key == nil && valCode != "" {
		keyCode = "_"
	}

	isMap := false
	if node.X != nil {
		isMap = e.isMapTypeExprJ(node.X)
	}

	if isMap {
		mapGoType := e.getExprGoTypeJ(node.X)
		valType := "Object"
		keyType := "Object"
		if mapGoType != nil {
			if mapUnderlying, ok := mapGoType.Underlying().(*types.Map); ok {
				valType = e.qualifiedJavaTypeName(mapUnderlying.Elem())
				keyType = e.qualifiedJavaTypeName(mapUnderlying.Key())
			}
		}
		keysVar := fmt.Sprintf("_keys%d", e.rangeVarCounter)
		loopIdx := fmt.Sprintf("_mi%d", e.rangeVarCounter)
		e.rangeVarCounter++
		if valCode != "" && valCode != "_" {
			var children []IRNode
			children = append(children,
				Leaf(WhiteSpace, ind),
				Leaf(LeftBrace, "{"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"  "),
				LeafTag(Keyword, "var ", TagJava),
				Leaf(Identifier, keysVar),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "hmap.hashMapKeys"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, xCode),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"  "),
				Leaf(ForKeyword, "for"),
				Leaf(WhiteSpace, " "),
				Leaf(LeftParen, "("),
				LeafTag(Keyword, "var ", TagJava),
				Leaf(Identifier, loopIdx),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(NumberLiteral, "0"),
				Leaf(Semicolon, ";"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, loopIdx),
				Leaf(WhiteSpace, " "),
				Leaf(ComparisonOperator, "<"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, keysVar),
				Leaf(Dot, "."),
				Leaf(Identifier, "size"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, loopIdx),
				Leaf(Identifier, "++"),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(LeftBrace, "{"),
				Leaf(NewLine, "\n"),
			)
			if keyCode != "_" {
				children = append(children,
					Leaf(WhiteSpace, ind+"    "),
					LeafTag(Keyword, "var ", TagJava),
					Leaf(Identifier, keyCode),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(LeftParen, "("),
					Leaf(TypeKeyword, keyType),
					Leaf(RightParen, ")"),
					Leaf(Identifier, keysVar),
					Leaf(Dot, "."),
					Leaf(Identifier, "get"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, loopIdx),
					Leaf(RightParen, ")"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				)
			}
			children = append(children,
				Leaf(WhiteSpace, ind+"    "),
				LeafTag(Keyword, "var ", TagJava),
				Leaf(Identifier, valCode),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(LeftParen, "("),
				Leaf(TypeKeyword, valType),
				Leaf(RightParen, ")"),
				Leaf(Identifier, "hmap.hashMapGet"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, xCode),
				Leaf(Comma, ","),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, keysVar),
				Leaf(Dot, "."),
				Leaf(Identifier, "get"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, loopIdx),
				Leaf(RightParen, ")"),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"    "),
				Leaf(Identifier, bodyCode),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"  "),
				Leaf(RightBrace, "}"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind),
				Leaf(RightBrace, "}"),
				Leaf(NewLine, "\n"),
			)
			e.fs.PushTree(IRTree(RangeStatement, KindStmt, children...))
		} else {
			var children []IRNode
			children = append(children,
				Leaf(WhiteSpace, ind),
				Leaf(LeftBrace, "{"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"  "),
				LeafTag(Keyword, "var ", TagJava),
				Leaf(Identifier, keysVar),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "hmap.hashMapKeys"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, xCode),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"  "),
				Leaf(ForKeyword, "for"),
				Leaf(WhiteSpace, " "),
				Leaf(LeftParen, "("),
				LeafTag(Keyword, "var ", TagJava),
				Leaf(Identifier, loopIdx),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(NumberLiteral, "0"),
				Leaf(Semicolon, ";"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, loopIdx),
				Leaf(WhiteSpace, " "),
				Leaf(ComparisonOperator, "<"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, keysVar),
				Leaf(Dot, "."),
				Leaf(Identifier, "size"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, loopIdx),
				Leaf(Identifier, "++"),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(LeftBrace, "{"),
				Leaf(NewLine, "\n"),
			)
			if keyCode != "_" {
				children = append(children,
					Leaf(WhiteSpace, ind+"    "),
					LeafTag(Keyword, "var ", TagJava),
					Leaf(Identifier, keyCode),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(LeftParen, "("),
					Leaf(TypeKeyword, keyType),
					Leaf(RightParen, ")"),
					Leaf(Identifier, keysVar),
					Leaf(Dot, "."),
					Leaf(Identifier, "get"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, loopIdx),
					Leaf(RightParen, ")"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				)
			}
			children = append(children,
				Leaf(WhiteSpace, ind+"    "),
				Leaf(Identifier, bodyCode),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"  "),
				Leaf(RightBrace, "}"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind),
				Leaf(RightBrace, "}"),
				Leaf(NewLine, "\n"),
			)
			e.fs.PushTree(IRTree(RangeStatement, KindStmt, children...))
		}
		return
	}

	// Check if ranging over string (affects .size() vs .length())
	xType := e.getExprGoTypeJ(node.X)
	lenExpr := xCode + ".size()"
	isString := false
	if xType != nil {
		if basic, ok := xType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
			lenExpr = xCode + ".length()"
			isString = true
		}
	}

	// If range expression is an inline composite literal, emit a temp variable
	if _, isCompLit := node.X.(*ast.CompositeLit); isCompLit {
		tmpVar := fmt.Sprintf("_range%d", e.rangeVarCounter)
		e.rangeVarCounter++
		var children []IRNode
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(LeftBrace, "{"),
			Leaf(NewLine, "\n"),
			Leaf(WhiteSpace, ind+"  "),
			LeafTag(Keyword, "var ", TagJava),
			Leaf(Identifier, tmpVar),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, "="),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, xCode),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		)
		xCode = tmpVar
		lenExpr = xCode + ".size()"
		if valCode != "" && valCode != "_" {
			loopVar := keyCode
			if loopVar == "_" || loopVar == "" {
				loopVar = fmt.Sprintf("_i%d", e.rangeVarCounter)
				e.rangeVarCounter++
			}
			var valDecl string
			if isString {
				valDecl = fmt.Sprintf("%s      var %s = (int)%s.charAt(%s);\n", ind, valCode, xCode, loopVar)
			} else {
				valDecl = fmt.Sprintf("%s      var %s = %s.get(%s);\n", ind, valCode, xCode, loopVar)
			}
			bodyWithDecl := strings.Replace(bodyCode, "{\n", "{\n"+valDecl, 1)
			children = append(children,
				Leaf(WhiteSpace, ind+"  "),
				Leaf(ForKeyword, "for"),
				Leaf(WhiteSpace, " "),
				Leaf(LeftParen, "("),
				LeafTag(Keyword, "var ", TagJava),
				Leaf(Identifier, loopVar),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(NumberLiteral, "0"),
				Leaf(Semicolon, ";"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, loopVar),
				Leaf(WhiteSpace, " "),
				Leaf(ComparisonOperator, "<"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, lenExpr),
				Leaf(Semicolon, ";"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, loopVar),
				Leaf(Identifier, "++"),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, bodyWithDecl),
				Leaf(NewLine, "\n"),
			)
		} else {
			loopVar := keyCode
			if loopVar == "_" || loopVar == "" {
				loopVar = fmt.Sprintf("_i%d", e.rangeVarCounter)
				e.rangeVarCounter++
			}
			children = append(children,
				Leaf(WhiteSpace, ind+"  "),
				Leaf(ForKeyword, "for"),
				Leaf(WhiteSpace, " "),
				Leaf(LeftParen, "("),
				LeafTag(Keyword, "var ", TagJava),
				Leaf(Identifier, loopVar),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(NumberLiteral, "0"),
				Leaf(Semicolon, ";"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, loopVar),
				Leaf(WhiteSpace, " "),
				Leaf(ComparisonOperator, "<"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, lenExpr),
				Leaf(Semicolon, ";"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, loopVar),
				Leaf(Identifier, "++"),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, bodyCode),
				Leaf(NewLine, "\n"),
			)
		}
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(RightBrace, "}"),
			Leaf(NewLine, "\n"),
		)
		e.fs.PushTree(IRTree(RangeStatement, KindStmt, children...))
		return
	}

	// Slice/string range
	if valCode != "" && valCode != "_" {
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = fmt.Sprintf("_i%d", e.rangeVarCounter)
			e.rangeVarCounter++
		}

		var valDecl string
		if isString {
			valDecl = fmt.Sprintf("%s    var %s = (int)%s.charAt(%s);\n", ind, valCode, xCode, loopVar)
		} else {
			valDecl = fmt.Sprintf("%s    var %s = %s.get(%s);\n", ind, valCode, xCode, loopVar)
		}
		bodyWithDecl := strings.Replace(bodyCode, "{\n", "{\n"+valDecl, 1)

		e.fs.PushTree(IRTree(RangeStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			LeafTag(Keyword, "var ", TagJava),
			Leaf(Identifier, loopVar),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, "="),
			Leaf(WhiteSpace, " "),
			Leaf(NumberLiteral, "0"),
			Leaf(Semicolon, "; "),
			Leaf(Identifier, loopVar),
			Leaf(WhiteSpace, " "),
			Leaf(ComparisonOperator, "<"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, lenExpr),
			Leaf(Semicolon, "; "),
			Leaf(Identifier, loopVar),
			Leaf(UnaryOperator, "++"),
			Leaf(RightParen, ")"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, bodyWithDecl),
			Leaf(NewLine, "\n"),
		))
	} else {
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = fmt.Sprintf("_i%d", e.rangeVarCounter)
			e.rangeVarCounter++
		}
		e.fs.PushTree(IRTree(RangeStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			LeafTag(Keyword, "var ", TagJava),
			Leaf(Identifier, loopVar),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, "="),
			Leaf(WhiteSpace, " "),
			Leaf(NumberLiteral, "0"),
			Leaf(Semicolon, "; "),
			Leaf(Identifier, loopVar),
			Leaf(WhiteSpace, " "),
			Leaf(ComparisonOperator, "<"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, lenExpr),
			Leaf(Semicolon, "; "),
			Leaf(Identifier, loopVar),
			Leaf(UnaryOperator, "++"),
			Leaf(RightParen, ")"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, bodyCode),
			Leaf(NewLine, "\n"),
		))
	}
}

// ============================================================
// Switch / Case Statements
// ============================================================

func (e *JavaEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	e.indent = indent
}

func (e *JavaEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	tagCode := e.fs.ReduceToCode(string(PreVisitSwitchStmtTag))
	e.fs.PushCode(tagCode)
}

func (e *JavaEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSwitchStmt))
	ind := javaIndent(indent / 2)

	tagCode := ""
	idx := 0
	if idx < len(tokens) {
		tagCode = tokens[idx].Serialize()
		idx++
	}

	var children []IRNode
	children = append(children,
		Leaf(WhiteSpace, ind),
		Leaf(SwitchKeyword, "switch"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftParen, "("),
		Leaf(Identifier, tagCode),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
	)
	for i := idx; i < len(tokens); i++ {
		children = append(children, Leaf(Identifier, tokens[i].Serialize()))
	}
	children = append(children,
		Leaf(WhiteSpace, ind),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
	)
	e.fs.PushTree(IRTree(SwitchStatement, KindStmt, children...))
}

func (e *JavaEmitter) PreVisitCaseClause(node *ast.CaseClause, indent int) {
	e.indent = indent
}

func (e *JavaEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	exprCode := e.fs.ReduceToCode(string(PreVisitCaseClauseListExpr))
	e.fs.PushCode(exprCode)
}

func (e *JavaEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCaseClauseList))
	var exprs []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			exprs = append(exprs, t.Serialize())
		}
	}
	e.fs.PushCode(strings.Join(exprs, ", "))
}

func (e *JavaEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCaseClause))
	ind := javaIndent(indent / 2)

	var children []IRNode
	idx := 0
	if len(node.List) == 0 {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(DefaultKeyword, "default"),
			Leaf(Colon, ":"),
			Leaf(NewLine, "\n"),
		)
	} else {
		caseExprs := ""
		if idx < len(tokens) {
			caseExprs = tokens[idx].Serialize()
			idx++
		}
		vals := strings.Split(caseExprs, ", ")
		for _, v := range vals {
			children = append(children,
				Leaf(WhiteSpace, ind),
				Leaf(CaseKeyword, "case"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, v),
				Leaf(Colon, ":"),
				Leaf(NewLine, "\n"),
			)
		}
	}
	for i := idx; i < len(tokens); i++ {
		children = append(children, Leaf(Identifier, tokens[i].Serialize()))
	}
	// Check if body already contains return or break
	bodyStr := IRTree(CaseClauseStatement, KindStmt, children...).Serialize()
	if !strings.Contains(bodyStr, "return ") && !strings.Contains(bodyStr, "break;") {
		children = append(children,
			Leaf(WhiteSpace, ind+"  "),
			Leaf(BreakKeyword, "break"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		)
	}
	e.fs.PushTree(IRTree(CaseClauseStatement, KindStmt, children...))
}

// ============================================================
// Inc/Dec Statements
// ============================================================

func (e *JavaEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitIncDecStmt))
	ind := javaIndent(indent / 2)

	// Handle closure-captured variables: ((Type)name[0])++ -> name[0] = ((Type)name[0]) + 1
	if ident, ok := node.X.(*ast.Ident); ok && e.closureCapturedMutVars != nil && e.closureCapturedMutVars[ident.Name] {
		opVal := "1"
		op := "+"
		if node.Tok == token.DEC {
			op = "-"
		}
		lhs := closureUnwrapLhs(xCode)
		e.fs.PushTree(IRTree(IncDecStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(Identifier, lhs),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, "="),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, xCode),
			Leaf(WhiteSpace, " "),
			Leaf(BinaryOperator, op),
			Leaf(WhiteSpace, " "),
			Leaf(NumberLiteral, opVal),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
		return
	}

	e.fs.PushTree(IRTree(IncDecStatement, KindStmt,
		Leaf(WhiteSpace, ind),
		Leaf(Identifier, xCode),
		Leaf(UnaryOperator, node.Tok.String()),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
	))
}

// ============================================================
// Branch Statements (break, continue)
// ============================================================

func (e *JavaEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	ind := javaIndent(indent / 2)
	switch node.Tok {
	case token.BREAK:
		e.fs.PushTree(IRTree(BranchStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(BreakKeyword, "break"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	case token.CONTINUE:
		e.fs.PushTree(IRTree(BranchStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ContinueKeyword, "continue"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	}
}

// ============================================================
// Struct Declarations (GenStructInfo)
// ============================================================

func (e *JavaEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	typeCode := e.fs.ReduceToCode(string(PreVisitGenStructFieldType))
	e.fs.PushCode(typeCode)
}

func (e *JavaEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	e.fs.Reduce(string(PreVisitGenStructFieldName))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *JavaEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {
	tokens := e.fs.Reduce(string(PreVisitGenStructInfo))

	if node.Struct == nil {
		return
	}

	// Collect field types and names
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
			fields = append(fields, fieldInfo{typeName: "Object", name: tokens[i].Serialize()})
			i++
		} else {
			i++
		}
	}

	var children []IRNode

	// Class header
	children = append(children,
		LeafTag(Keyword, "static class ", TagJava),
		Leaf(Identifier, node.Name),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
	)

	// Public fields
	for _, f := range fields {
		children = append(children,
			Leaf(WhiteSpace, "  "),
			LeafTag(Keyword, "public ", TagJava),
			Leaf(TypeKeyword, f.typeName),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, f.name),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		)
	}

	// No-arg constructor with defaults
	children = append(children,
		Leaf(WhiteSpace, "  "),
		LeafTag(Keyword, "public ", TagJava),
		Leaf(Identifier, node.Name),
		Leaf(LeftParen, "("),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
	)
	for _, f := range fields {
		children = append(children,
			Leaf(WhiteSpace, "    "),
			LeafTag(Keyword, "this", TagJava),
			Leaf(Dot, "."),
			Leaf(Identifier, f.name),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, "="),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, getJavaDefaultValueForStruct(f.typeName)),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		)
	}
	children = append(children,
		Leaf(WhiteSpace, "  "),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
	)

	// All-args constructor
	children = append(children,
		Leaf(WhiteSpace, "  "),
		LeafTag(Keyword, "public ", TagJava),
		Leaf(Identifier, node.Name),
		Leaf(LeftParen, "("),
	)
	for j, f := range fields {
		if j > 0 {
			children = append(children, Leaf(Comma, ","), Leaf(WhiteSpace, " "))
		}
		children = append(children,
			Leaf(TypeKeyword, f.typeName),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, f.name),
		)
	}
	children = append(children,
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
	)
	for _, f := range fields {
		if !isJavaPrimitiveType(f.typeName) &&
			!strings.HasPrefix(f.typeName, "ArrayList<") &&
			!strings.HasPrefix(f.typeName, "HashMap<") &&
			!isJavaFunctionalInterface(f.typeName) &&
			!isJavaBuiltinReferenceType(f.typeName) &&
			f.typeName != "hmap.HashMap" &&
			f.typeName != "Object" &&
			f.typeName != "String" {
			// this.field = field != null ? field : new Type();
			children = append(children,
				Leaf(WhiteSpace, "    "),
				LeafTag(Keyword, "this", TagJava),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, f.name),
				Leaf(WhiteSpace, " "),
				Leaf(ComparisonOperator, "!="),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "null", TagJava),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "?"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, f.name),
				Leaf(WhiteSpace, " "),
				Leaf(Colon, ":"),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "new ", TagJava),
				Leaf(Identifier, f.typeName),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		} else {
			children = append(children,
				Leaf(WhiteSpace, "    "),
				LeafTag(Keyword, "this", TagJava),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, f.name),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		}
	}
	children = append(children,
		Leaf(WhiteSpace, "  "),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
	)

	// Copy constructor
	children = append(children,
		Leaf(WhiteSpace, "  "),
		LeafTag(Keyword, "public ", TagJava),
		Leaf(Identifier, node.Name),
		Leaf(LeftParen, "("),
		Leaf(TypeKeyword, node.Name),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "other"),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
	)
	for _, f := range fields {
		if strings.HasPrefix(f.typeName, "ArrayList<") {
			children = append(children,
				Leaf(WhiteSpace, "    "),
				LeafTag(Keyword, "this", TagJava),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "other"),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
				Leaf(WhiteSpace, " "),
				Leaf(ComparisonOperator, "!="),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "null", TagJava),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "?"),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "new ", TagJava),
				Leaf(Identifier, "ArrayList<>"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, "other"),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Colon, ":"),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "null", TagJava),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		} else if f.typeName == "hmap.HashMap" || strings.HasPrefix(f.typeName, "HashMap<") ||
			isJavaPrimitiveType(f.typeName) ||
			isJavaFunctionalInterface(f.typeName) || isJavaBuiltinReferenceType(f.typeName) {
			children = append(children,
				Leaf(WhiteSpace, "    "),
				LeafTag(Keyword, "this", TagJava),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "other"),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		} else {
			children = append(children,
				Leaf(WhiteSpace, "    "),
				LeafTag(Keyword, "this", TagJava),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "other"),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
				Leaf(WhiteSpace, " "),
				Leaf(ComparisonOperator, "!="),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "null", TagJava),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "?"),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "new ", TagJava),
				Leaf(Identifier, f.typeName),
				Leaf(LeftParen, "("),
				Leaf(Identifier, "other"),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Colon, ":"),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "null", TagJava),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		}
	}
	children = append(children,
		Leaf(WhiteSpace, "  "),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
	)

	// hashCode()
	children = append(children,
		Leaf(WhiteSpace, "  "),
		Leaf(Identifier, "@Override"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "  "),
		LeafTag(Keyword, "public ", TagJava),
		Leaf(TypeKeyword, "int"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "hashCode"),
		Leaf(LeftParen, "("),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "    "),
		Leaf(ReturnKeyword, "return"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "java.util.Objects.hash"),
		Leaf(LeftParen, "("),
	)
	for j, f := range fields {
		if j > 0 {
			children = append(children, Leaf(Comma, ","), Leaf(WhiteSpace, " "))
		}
		children = append(children,
			LeafTag(Keyword, "this", TagJava),
			Leaf(Dot, "."),
			Leaf(Identifier, f.name),
		)
	}
	children = append(children,
		Leaf(RightParen, ")"),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "  "),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
	)

	// equals()
	children = append(children,
		Leaf(WhiteSpace, "  "),
		Leaf(Identifier, "@Override"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "  "),
		LeafTag(Keyword, "public ", TagJava),
		Leaf(TypeKeyword, "boolean"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "equals"),
		Leaf(LeftParen, "("),
		Leaf(TypeKeyword, "Object"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "obj"),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "    "),
		Leaf(IfKeyword, "if"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftParen, "("),
		LeafTag(Keyword, "this", TagJava),
		Leaf(WhiteSpace, " "),
		Leaf(ComparisonOperator, "=="),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "obj"),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(ReturnKeyword, "return"),
		Leaf(WhiteSpace, " "),
		Leaf(BooleanLiteral, "true"),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "    "),
		Leaf(IfKeyword, "if"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftParen, "("),
		Leaf(Identifier, "obj"),
		Leaf(WhiteSpace, " "),
		Leaf(ComparisonOperator, "=="),
		Leaf(WhiteSpace, " "),
		LeafTag(Keyword, "null", TagJava),
		Leaf(WhiteSpace, " "),
		Leaf(LogicalOperator, "||"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "getClass"),
		Leaf(LeftParen, "("),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(ComparisonOperator, "!="),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "obj"),
		Leaf(Dot, "."),
		Leaf(Identifier, "getClass"),
		Leaf(LeftParen, "("),
		Leaf(RightParen, ")"),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(ReturnKeyword, "return"),
		Leaf(WhiteSpace, " "),
		Leaf(BooleanLiteral, "false"),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "    "),
		Leaf(TypeKeyword, node.Name),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "other"),
		Leaf(WhiteSpace, " "),
		Leaf(Assignment, "="),
		Leaf(WhiteSpace, " "),
		Leaf(LeftParen, "("),
		Leaf(TypeKeyword, node.Name),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "obj"),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "    "),
		Leaf(ReturnKeyword, "return"),
		Leaf(WhiteSpace, " "),
	)
	for j, f := range fields {
		if j > 0 {
			children = append(children, Leaf(WhiteSpace, " "), Leaf(LogicalOperator, "&&"), Leaf(WhiteSpace, " "))
		}
		if isJavaPrimitiveType(f.typeName) && f.typeName != "String" {
			children = append(children,
				LeafTag(Keyword, "this", TagJava),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
				Leaf(WhiteSpace, " "),
				Leaf(ComparisonOperator, "=="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "other"),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
			)
		} else {
			children = append(children,
				Leaf(Identifier, "java.util.Objects.equals"),
				Leaf(LeftParen, "("),
				LeafTag(Keyword, "this", TagJava),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
				Leaf(Comma, ","),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "other"),
				Leaf(Dot, "."),
				Leaf(Identifier, f.name),
				Leaf(RightParen, ")"),
			)
		}
	}
	if len(fields) == 0 {
		children = append(children, Leaf(BooleanLiteral, "true"))
	}
	children = append(children,
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "  "),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
	)

	// Close class
	children = append(children,
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
		Leaf(NewLine, "\n"),
	)
	e.fs.PushTree(IRTree(ClassKeyword, TagExpr, children...))
}

func (e *JavaEmitter) PostVisitGenStructInfos(node []GenTypeInfo, indent int) {
	// Structs are already pushed to the stack
}

// ============================================================
// Constants (GenDeclConst)
// ============================================================

func (e *JavaEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	valTokens := e.fs.Reduce(string(PreVisitGenDeclConstName))
	valCode := ""
	for _, t := range valTokens {
		valCode += t.Serialize()
	}
	if valCode == "" {
		valCode = "0"
	}

	// Determine the type from type info
	constType := "int"
	if e.pkg != nil && e.pkg.TypesInfo != nil {
		if obj := e.pkg.TypesInfo.Defs[node]; obj != nil {
			ut := obj.Type().Underlying()
			resolved := getJavaPrimTypeName(ut)
			if resolved == "Object" {
				if basic, ok := ut.(*types.Basic); ok {
					if basic.Info()&types.IsInteger != 0 {
						resolved = "int"
					} else if basic.Info()&types.IsFloat != 0 {
						resolved = "double"
					} else if basic.Info()&types.IsString != 0 {
						resolved = "String"
					} else if basic.Info()&types.IsBoolean != 0 {
						resolved = "boolean"
					}
				}
			}
			constType = resolved
		}
	}

	name := node.Name
	e.fs.PushTree(IRTree(Keyword, TagExpr,
		LeafTag(Keyword, "public static final ", TagJava),
		Leaf(TypeKeyword, constType),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, name),
		Leaf(WhiteSpace, " "),
		Leaf(Assignment, "="),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, valCode),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
	))
}

func (e *JavaEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {
	// Let const tokens flow through
}

// ============================================================
// Type Aliases
// ============================================================

func (e *JavaEmitter) PreVisitTypeAliasName(node *ast.Ident, indent int) {
	e.currentAliasName = node.Name
}

func (e *JavaEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitTypeAliasName))

	if e.currentAliasName != "" {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if tv, ok := e.pkg.TypesInfo.Types[node]; ok && tv.Type != nil {
				underlyingType := tv.Type.String()
				underlyingType = convertGoTypeToJava(underlyingType)
				if e.typeAliasMap == nil {
					e.typeAliasMap = make(map[string]string)
				}
				e.typeAliasMap[e.currentAliasName] = underlyingType
			}
		}
	}
	e.currentAliasName = ""
}
