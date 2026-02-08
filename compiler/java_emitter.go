package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	goanyrt "goany/runtime"

	"golang.org/x/tools/go/packages"
)

// Java type mapping - note Java has no unsigned types
var javaTypesMap = map[string]string{
	"int8":    "byte",
	"int16":   "short",
	"int32":   "int",
	"int64":   "long",
	"uint8":   "int",    // No unsigned in Java, use wider signed
	"uint16":  "int",    // No unsigned in Java
	"uint32":  "long",   // No unsigned in Java
	"uint64":  "long",   // No unsigned in Java (would need BigInteger for full range)
	"int":     "int",
	"byte":    "byte",
	"rune":    "int",
	"any":     "Object",
	"string":  "String",
	"float32": "float",
	"float64": "double",
	"bool":    "boolean",
}

// javaBoxedTypes maps primitive types to their boxed versions for generics
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

// javaKeyCastState holds key cast values for nested map reads
type javaKeyCastState struct {
	prefix    string
	suffix    string
	valueType string
}

// mapIndexStackEntry holds state for nested map index expressions
type mapIndexStackEntry struct {
	isMapIndex        bool
	mapIndexValueType string
	mapReadVarName    string
	mapKeyCastPrefix  string
	mapKeyCastSuffix  string
}

type javaMixedIndexOp struct {
	accessType   string
	keyExpr      string
	keyCastPfx   string
	keyCastSfx   string
	valueJavaType string
	tempVarName  string
	mapVarExpr   string
}

type JavaEmitter struct {
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string
	RuntimePackages map[string]string
	file            *os.File
	BaseEmitter
	pkg                        *packages.Package
	insideForPostCond          bool
	assignmentToken            string
	forwardDecls               bool
	shouldGenerate             bool
	numFuncResults             int
	currentPackage             string
	isArray                    bool
	arrayType                  string
	isTuple                    bool
	isInfiniteLoop             bool
	isKeyValueRange            bool
	rangeKeyName               string
	rangeValueName             string
	rangeCollectionExpr        string
	captureRangeExpr           bool
	suppressRangeEmit          bool
	rangeStmtIndent            int
	pendingRangeValueDecl      bool
	pendingValueName           string
	pendingCollectionExpr      string
	pendingKeyName             string
	inTypeContext              bool
	inArrayTypeContext         bool  // Track when we're inside ArrayList<...> to use boxed types
	apiClassOpened             bool
	suppressTypeAliasEmit      bool
	currentAliasName           string
	typeAliasMap               map[string]string
	suppressTypeAliasSelectorX bool
	// Map support
	isMapMakeCall     bool
	mapMakeKeyType    int
	isMapIndex        bool
	mapIndexValueType string
	mapIndexStack     []mapIndexStackEntry // Stack for nested map index expressions
	isMapAssign       bool
	mapAssignVarName  string
	mapAssignIndent   int
	isNestedMapAssign      bool
	nestedMapRootVar       string             // Root variable for nested map (e.g., "m" for m["a"]["b"])
	nestedMapLevels        []nestedMapLevel   // All levels from root to the assignment level
	nestedMapAssignKey     string             // The final key being assigned to
	nestedMapAssignIndent  int
	// Slice-of-map assignment: sliceOfMaps[i][j]...["key"] = v (supports any depth)
	isSliceOfMapAssign    bool
	sliceOfMapSliceVar    string   // The root slice variable name
	sliceOfMapSliceIndices []string // All slice indices from root to map
	sliceOfMapMapKey      string   // The map key expression
	sliceOfMapIndent      int
	captureMapKey         bool
	capturedMapKey        string
	suppressMapEmit       bool
	isDeleteCall          bool
	deleteMapVarName      string
	mapKeyCastPrefix      string
	mapKeyCastSuffix      string
	mapKeyCastStack       []javaKeyCastState
	isMapLenCall          bool
	pendingMapInit        bool
	pendingMapKeyType     int
	pendingStructInit     bool   // Whether to add = new Type() for struct variable
	pendingStructTypeName string // Name of the struct type to initialize
	pendingSliceInit      bool   // Whether to add = new ArrayList<>() for slice variable
	pendingSliceElemType  string // Element type for the slice initialization
	pendingPrimitiveInit    bool   // Whether to add = default for primitive variable
	pendingPrimitiveDefault string // Default value for primitive ("0", "false", etc.)
	isMapCommaOk          bool
	mapCommaOkValName     string
	mapCommaOkOkName      string
	mapCommaOkMapName     string
	mapCommaOkValType     string
	mapCommaOkIsDecl      bool
	mapCommaOkIndent      int
	isTypeAssertCommaOk        bool
	typeAssertCommaOkValName   string
	typeAssertCommaOkOkName    string
	typeAssertCommaOkXName     string
	typeAssertCommaOkType      string
	typeAssertCommaOkBoxedType string
	typeAssertCommaOkIsDecl    bool
	typeAssertCommaOkIndent    int
	isSliceMakeCall   bool
	sliceMakeElemType string
	indexAccessTypeStack    []string
	suppressMapEmitStack    []bool
	isMixedIndexAssign      bool
	mixedIndexOps           []javaMixedIndexOp
	mixedBaseVarName        string
	mixedAssignIndent       int
	// fmt package suppression
	suppressFmtPackage bool
	// struct field key suppression in composite literals
	suppressKeyValueKey bool
	// multiple return type suppression
	suppressMultiReturnTypes bool
	// flag to track if we're inside for loop init (to suppress semicolon)
	insideForInit bool
	// flag to suppress lambda parameter types (Java uses type inference)
	inLambdaParams bool
	// flag to suppress LHS emission during map assignment
	suppressMapAssignLhs bool
	// flag to suppress map index X emission (for map reads)
	suppressMapIndexX bool
	// stored map variable name for read
	mapReadVarName string
	// Multi-return function support
	currentFuncName       string              // Name of current function being processed
	multiReturnFuncs      map[string][]string // Map of func name to return type names
	isMultiReturnAssign   bool                // Flag for multi-return assignment
	multiReturnLhsNames   []string            // LHS variable names for unpacking
	multiReturnIsDecl     bool                // Whether it's a := declaration
	multiReturnIndent     int                 // Indent for the assignment
	resultClassDefs       []string            // Collected result class definitions
	multiReturnCounter    int                 // Counter for unique result variable names
	// If-init handling (Java doesn't support if init; cond)
	hasIfInit     bool // Flag for current if statement having an init
	ifInitIndent  int  // Stored indent for if statement with init
	// Unsupported indexed function call (x[0](args))
	isUnsupportedIndexedCall bool
	unsupportedCallIndent    int
	// Slice index assignment (x[i] = value -> x.set(i, value))
	isSliceIndexAssign  bool
	sliceIndexVarName   string
	sliceIndexIndent    int
	sliceIndexLhsExpr   *ast.IndexExpr // Track the actual LHS index expression
	// Multi-return type reuse - map signature to class name
	resultClassRegistry map[string]string
	// HashMap value type tracking - map variable name to value type
	mapVarValueTypes map[string]string
	// Variable shadowing - track declared variables per scope
	declaredVarsStack []map[string]bool
	// Track map value type for current map get operation
	currentMapGetValueType string
	// Slice expression handling
	sliceExprXName string // Store the X expression for a[low:high] to use in size() call
	// Byte/short casting for slice literals
	sliceLitElemCast string // Cast prefix for slice literal elements (e.g., "(byte)" or "(short)")
	// Byte/short arithmetic casting
	needsSmallIntCast bool   // Whether assignment RHS needs byte/short cast
	smallIntCastType  string // The type to cast to ("byte" or "short")
	// Functional interface call tracking
	isFuncVarCall bool // Whether current call is a function variable (needs .accept() etc.)
}

// Helper functions

func (je *JavaEmitter) convertGoTypeToJava(goType string) string {
	if javaType, ok := javaTypesMap[goType]; ok {
		return javaType
	}
	return goType
}

func (je *JavaEmitter) getBoxedType(primitiveType string) string {
	if boxed, ok := javaBoxedTypes[primitiveType]; ok {
		return boxed
	}
	return primitiveType
}

// isVarDeclared checks if a variable is already declared in any active scope
func (je *JavaEmitter) isVarDeclared(name string) bool {
	for _, scope := range je.declaredVarsStack {
		if scope[name] {
			return true
		}
	}
	return false
}

// declareVar marks a variable as declared in the current scope
func (je *JavaEmitter) declareVar(name string) {
	if len(je.declaredVarsStack) > 0 {
		je.declaredVarsStack[len(je.declaredVarsStack)-1][name] = true
	}
}

// nestedMapLevel represents one level of a nested map access
type nestedMapLevel struct {
	keyExpr   string
	valueType string // Java type name of the map's value
}

// collectNestedMapLevels walks a nested IndexExpr and returns:
// - rootVar: the root variable name (e.g., "m" for m["a"]["b"])
// - levels: list of key expressions from outermost to innermost
// This function is used for handling nested map assignments like m["a"]["b"] = v
func (je *JavaEmitter) collectNestedMapLevels(expr ast.Expr) (rootVar string, levels []nestedMapLevel) {
	for {
		indexExpr, ok := expr.(*ast.IndexExpr)
		if !ok {
			rootVar = exprToString(expr)
			return
		}
		// Get the value type of this map access
		var valueType string
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			tv := je.pkg.TypesInfo.Types[indexExpr.X]
			if tv.Type != nil {
				if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
					valueType = getJavaTypeName(mapType.Elem())
				}
			}
		}
		levels = append([]nestedMapLevel{{
			keyExpr:   exprToString(indexExpr.Index),
			valueType: valueType,
		}}, levels...)
		expr = indexExpr.X
	}
}

// generateNestedMapGetExpr generates a Java expression for reading a nested map
// e.g., m["a"]["b"] -> ((Integer)hashMapGet(((HashMap)hashMapGet(m, "a")), "b"))
func (je *JavaEmitter) generateNestedMapGetExpr(rootVar string, levels []nestedMapLevel) string {
	result := rootVar
	for _, level := range levels {
		boxedType := toBoxedType(level.valueType)
		result = fmt.Sprintf("((%s)hashMapGet(%s, %s))", boxedType, result, level.keyExpr)
	}
	return result
}

// sliceIndexLevel represents one level of slice indexing
type sliceIndexLevel struct {
	indexExpr string // The index expression (e.g., "0", "i")
}

// collectSliceIndices walks a chain of IndexExprs and collects all slice indices
// For expr = slice[0][1], returns rootExpr="slice", indices=["0", "1"]
// It stops when it finds a non-IndexExpr or when the type is not a slice
// If the root is a map access, rootExpr will be the Java expression for the map get
func (je *JavaEmitter) collectSliceIndices(expr ast.Expr) (rootExpr string, indices []string) {
	for {
		indexExpr, ok := expr.(*ast.IndexExpr)
		if !ok {
			rootExpr = exprToString(expr)
			return
		}
		// Check if X is a slice type
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			tv := je.pkg.TypesInfo.Types[indexExpr.X]
			if tv.Type != nil {
				if _, isSlice := tv.Type.Underlying().(*types.Slice); !isSlice {
					// Not a slice - check if it's a map access
					if mapType, isMap := tv.Type.Underlying().(*types.Map); isMap {
						// The root is a map access, generate proper Java expression
						// e.g., m[1] -> ((ArrayList<HashMap>)hashMapGet(m, 1))
						mapVar := exprToString(indexExpr.X)
						keyExpr := exprToString(indexExpr.Index)
						elemType := getJavaTypeName(mapType.Elem())
						boxedElemType := toBoxedType(elemType)
						rootExpr = fmt.Sprintf("((%s)hashMapGet(%s, %s))", boxedElemType, mapVar, keyExpr)
						return
					}
					// Not a slice or map, stop here
					rootExpr = exprToString(expr)
					return
				}
			}
		}
		// Prepend the index (we're walking from outer to inner, but we want inner to outer)
		indices = append([]string{exprToString(indexExpr.Index)}, indices...)
		expr = indexExpr.X
	}
}

// generateSliceGetChain generates the Java expression for getting through multiple slice levels
// e.g., slice, ["0", "1"] -> slice.get(0).get(1)
func (je *JavaEmitter) generateSliceGetChain(rootVar string, indices []string) string {
	result := rootVar
	for _, idx := range indices {
		result = fmt.Sprintf("%s.get(%s)", result, idx)
	}
	return result
}

func (je *JavaEmitter) emitAsString(s string, indent int) string {
	if indent > 0 {
		return strings.Repeat("    ", indent) + s
	}
	return s
}

func (je *JavaEmitter) getTokenType(content string) TokenType {
	switch content {
	case "{":
		return LeftBrace
	case "}":
		return RightBrace
	case "(":
		return LeftParen
	case ")":
		return RightParen
	case "[":
		return LeftBracket
	case "]":
		return RightBracket
	case ";":
		return Semicolon
	case ",":
		return Comma
	case "+", "-", "*", "/", "%", "=", "==", "!=", "<", ">", "<=", ">=", "&&", "||", "!":
		return BinaryOperator
	default:
		return Identifier
	}
}

func (je *JavaEmitter) emitToken(content string, tokenType TokenType, indent int) {
	je.gir.emitToFileBuffer(content, EmptyVisitMethod)
}

func (je *JavaEmitter) getMapKeyTypeConst(mapType *ast.MapType) int {
	if je.pkg == nil || je.pkg.TypesInfo == nil {
		return 1 // Default to string
	}
	tv := je.pkg.TypesInfo.Types[mapType.Key]
	if tv.Type == nil {
		return 1 // Default to string
	}
	switch tv.Type.Underlying().String() {
	case "string":
		return 1 // KeyTypeString
	case "int":
		return 2 // KeyTypeInt
	case "bool":
		return 3 // KeyTypeBool
	case "int8":
		return 4 // KeyTypeInt8
	case "int16":
		return 5 // KeyTypeInt16
	case "int32":
		return 6 // KeyTypeInt32
	case "int64":
		return 7 // KeyTypeInt64
	case "uint8":
		return 8 // KeyTypeUint8
	case "uint16":
		return 9 // KeyTypeUint16
	case "uint32":
		return 10 // KeyTypeUint32
	case "uint64":
		return 11 // KeyTypeUint64
	case "float32":
		return 12 // KeyTypeFloat32
	case "float64":
		return 13 // KeyTypeFloat64
	default:
		return 1 // Default to string
	}
}

func getJavaTypeName(t types.Type) string {
	// First, check if this is a named type
	if named, ok := t.(*types.Named); ok {
		// Check if the underlying type is a basic type (type alias like `type ExprKind int`)
		// In that case, use the underlying type since Java doesn't have type aliases
		if basic, ok := named.Underlying().(*types.Basic); ok {
			// Use the underlying basic type
			switch basic.Kind() {
			case types.Int, types.Int32:
				return "int"
			case types.Int8:
				return "byte"
			case types.Int16:
				return "short"
			case types.Int64:
				return "long"
			case types.Uint, types.Uint8, types.Uint16, types.Uint32:
				return "int"
			case types.Uint64:
				return "long"
			case types.Float32:
				return "float"
			case types.Float64:
				return "double"
			case types.Bool:
				return "boolean"
			case types.String:
				return "String"
			}
		}
		// Otherwise, it's a struct or other named type - use the name
		return named.Obj().Name()
	}

	switch ut := t.Underlying().(type) {
	case *types.Basic:
		switch ut.Kind() {
		case types.Int, types.Int32:
			return "int"
		case types.Int8:
			return "byte"
		case types.Int16:
			return "short"
		case types.Int64:
			return "long"
		case types.Uint, types.Uint8, types.Uint16, types.Uint32:
			return "int"
		case types.Uint64:
			return "long"
		case types.Float32:
			return "float"
		case types.Float64:
			return "double"
		case types.Bool:
			return "boolean"
		case types.String:
			return "String"
		default:
			return "Object"
		}
	case *types.Slice:
		elemType := getJavaTypeName(ut.Elem())
		return fmt.Sprintf("ArrayList<%s>", toBoxedType(elemType))
	case *types.Map:
		return "HashMap"
	case *types.Pointer:
		return getJavaTypeName(ut.Elem())
	default:
		return "Object"
	}
}

func toBoxedType(t string) string {
	if boxed, ok := javaBoxedTypes[t]; ok {
		return boxed
	}
	return t
}

func getJavaDefaultValue(javaType string) string {
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
	default:
		return "null"
	}
}

func getJavaKeyCast(keyType types.Type) (string, string) {
	switch keyType.Underlying().String() {
	case "int64":
		return "(long)(", ")"
	case "uint64":
		return "(long)(", ")"
	default:
		// No cast needed for other types - they can be boxed directly
		return "", ""
	}
}

// getJavaTypeFromExpr converts an AST expression to a Java type string (fallback when TypesInfo unavailable)
func (je *JavaEmitter) getJavaTypeFromExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		// Check type alias map first
		if javaType, ok := je.typeAliasMap[e.Name]; ok {
			return javaType
		}
		// Check standard Go to Java type mapping
		if javaType, ok := javaTypesMap[e.Name]; ok {
			return javaType
		}
		// Return as-is (for struct names, etc.)
		return e.Name
	case *ast.ArrayType:
		elemType := je.getJavaTypeFromExpr(e.Elt)
		return fmt.Sprintf("ArrayList<%s>", toBoxedType(elemType))
	case *ast.MapType:
		return "HashMap"
	case *ast.StarExpr:
		return je.getJavaTypeFromExpr(e.X)
	case *ast.SelectorExpr:
		// For package.Type, just use the type name
		return e.Sel.Name
	case *ast.FuncType:
		return je.getFunctionalInterfaceType(e)
	default:
		return "Object"
	}
}

// getFunctionalInterfaceType maps a Go function type to a Java functional interface
func (je *JavaEmitter) getFunctionalInterfaceType(funcType *ast.FuncType) string {
	numParams := 0
	if funcType.Params != nil {
		for _, field := range funcType.Params.List {
			if len(field.Names) == 0 {
				numParams++
			} else {
				numParams += len(field.Names)
			}
		}
	}

	hasReturn := funcType.Results != nil && len(funcType.Results.List) > 0

	// Map to Java functional interfaces
	if !hasReturn {
		switch numParams {
		case 0:
			return "Runnable"
		case 1:
			paramType := je.getFuncParamType(funcType, 0)
			return fmt.Sprintf("Consumer<%s>", toBoxedType(paramType))
		case 2:
			p1 := je.getFuncParamType(funcType, 0)
			p2 := je.getFuncParamType(funcType, 1)
			return fmt.Sprintf("BiConsumer<%s, %s>", toBoxedType(p1), toBoxedType(p2))
		default:
			return "Object" // No standard interface for 3+ params
		}
	} else {
		returnType := je.getFuncReturnType(funcType)
		switch numParams {
		case 0:
			return fmt.Sprintf("Supplier<%s>", toBoxedType(returnType))
		case 1:
			paramType := je.getFuncParamType(funcType, 0)
			return fmt.Sprintf("Function<%s, %s>", toBoxedType(paramType), toBoxedType(returnType))
		case 2:
			p1 := je.getFuncParamType(funcType, 0)
			p2 := je.getFuncParamType(funcType, 1)
			return fmt.Sprintf("BiFunction<%s, %s, %s>", toBoxedType(p1), toBoxedType(p2), toBoxedType(returnType))
		default:
			return "Object"
		}
	}
}

// getFuncParamType gets the Java type of a function parameter at the given index
func (je *JavaEmitter) getFuncParamType(funcType *ast.FuncType, index int) string {
	if funcType.Params == nil {
		return "Object"
	}
	idx := 0
	for _, field := range funcType.Params.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		if index < idx+count {
			return je.getJavaTypeFromExpr(field.Type)
		}
		idx += count
	}
	return "Object"
}

// getFuncReturnType gets the Java type of a function's return value
func (je *JavaEmitter) getFuncReturnType(funcType *ast.FuncType) string {
	if funcType.Results == nil || len(funcType.Results.List) == 0 {
		return "Void"
	}
	// For simplicity, just get the first return type
	return je.getJavaTypeFromExpr(funcType.Results.List[0].Type)
}

// getFunctionalInterfaceMethod returns the method name to call on a functional interface
// based on the function signature
func (je *JavaEmitter) getFunctionalInterfaceMethod(sig *types.Signature) string {
	numParams := sig.Params().Len()
	hasReturn := sig.Results().Len() > 0

	if !hasReturn {
		switch numParams {
		case 0:
			return "run" // Runnable
		case 1, 2:
			return "accept" // Consumer, BiConsumer
		}
	} else {
		switch numParams {
		case 0:
			return "get" // Supplier
		case 1, 2:
			return "apply" // Function, BiFunction
		}
	}
	return ""
}

func (je *JavaEmitter) SetFile(file *os.File) {
	je.file = file
}

func (je *JavaEmitter) GetFile() *os.File {
	return je.file
}

func (je *JavaEmitter) executeIfNotForwardDecls(fn func()) {
	if je.forwardDecls {
		return
	}
	fn()
}

func (je *JavaEmitter) lowerToBuiltins(name string) string {
	switch name {
	case "println":
		return "System.out.println"
	case "print":
		return "System.out.print"
	case "len":
		return "SliceBuiltins.Length"
	case "append":
		return "SliceBuiltins.Append"
	case "panic":
		return "GoanyPanic.goPanic"
	case "true":
		return "true"
	case "false":
		return "false"
	// Handle fmt package functions
	case "Printf":
		return "Formatter.Printf"
	case "Sprintf":
		return "Formatter.Sprintf"
	case "Println":
		return "System.out.println"
	case "Print":
		return "System.out.print"
	default:
		return name
	}
}

// Main visitor methods

func (je *JavaEmitter) PreVisitProgram(indent int) {
	je.typeAliasMap = make(map[string]string)
	je.multiReturnFuncs = make(map[string][]string)
	je.resultClassDefs = nil
	je.resultClassRegistry = make(map[string]string)
	je.mapVarValueTypes = make(map[string]string)
	je.declaredVarsStack = []map[string]bool{make(map[string]bool)}
	outputFile := je.Output
	je.shouldGenerate = true
	var err error
	je.file, err = os.Create(outputFile)
	je.SetFile(je.file)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}

	// Java imports
	imports := `import java.util.*;
import java.util.function.*;

`
	_, err = je.file.WriteString(imports)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

	// Include panic runtime
	je.file.WriteString("// GoAny panic runtime\n")
	je.file.WriteString(goanyrt.PanicJavaSource)
	je.file.WriteString("\n")

	// Builtin helper classes
	builtin := `class SliceBuiltins {
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

`
	str := je.emitAsString(builtin, indent)
	_ = je.gir.emitToFileBuffer(str, EmptyVisitMethod)

	je.insideForPostCond = false
}

func (je *JavaEmitter) PostVisitProgram(indent int) {
	// Close the main class
	if je.apiClassOpened {
		je.gir.emitToFileBuffer("}\n", EmptyVisitMethod)
	}

	emitTokensToFile(je.file, je.gir.tokenSlice)
	je.file.Close()

	// Generate build files if link-runtime is enabled
	if je.LinkRuntime != "" {
		if err := je.GeneratePomXml(); err != nil {
			log.Printf("Warning: %v", err)
		}
		if err := je.CopyRuntimePackages(); err != nil {
			log.Printf("Warning: %v", err)
		}
	}
}

func (je *JavaEmitter) PreVisitFuncDeclSignatures(indent int) {
	je.forwardDecls = true
}

func (je *JavaEmitter) PostVisitFuncDeclSignatures(indent int) {
	je.forwardDecls = false
}

func (je *JavaEmitter) PreVisitFuncDeclName(node *ast.Ident, indent int) {
	je.executeIfNotForwardDecls(func() {
		var str string
		if node.Name == "main" {
			str = je.emitAsString("main", 0)
		} else {
			str = je.emitAsString(node.Name, 0)
		}
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Push new scope for variable tracking
		je.declaredVarsStack = append(je.declaredVarsStack, make(map[string]bool))

		je.emitToken("{", LeftBrace, 1)
		str := je.emitAsString("\n", 1)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)

		if je.pendingRangeValueDecl {
			valueDecl := je.emitAsString(fmt.Sprintf("var %s = %s.get(%s);\n",
				je.pendingValueName, je.pendingCollectionExpr, je.pendingKeyName), indent+2)
			je.gir.emitToFileBuffer(valueDecl, EmptyVisitMethod)
			// Track this variable in current scope
			if len(je.declaredVarsStack) > 0 {
				je.declaredVarsStack[len(je.declaredVarsStack)-1][je.pendingValueName] = true
			}
			je.pendingRangeValueDecl = false
			je.pendingValueName = ""
			je.pendingCollectionExpr = ""
			je.pendingKeyName = ""
		}
	})
}

func (je *JavaEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("}", indent)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		// Pop scope for variable tracking
		if len(je.declaredVarsStack) > 0 {
			je.declaredVarsStack = je.declaredVarsStack[:len(je.declaredVarsStack)-1]
		}
		// Note: Don't reset isArray here - it's reset in PostVisitCompositeLitElts
	})
}

func (je *JavaEmitter) PreVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.emitToken("(", LeftParen, 0)
		// Add String[] args for main function
		if node.Name.Name == "main" {
			je.gir.emitToFileBuffer("String[] args", EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.emitToken(")", RightParen, 0)
	})
}

// isTypeConversion tracks if current call expression is a type conversion
var javaIsTypeConversion bool
var javaSuppressTypeCastIdent bool

func (je *JavaEmitter) PreVisitIdent(e *ast.Ident, indent int) {
	je.executeIfNotForwardDecls(func() {
		if !je.shouldGenerate {
			return
		}
		if je.suppressTypeAliasEmit {
			return
		}
		if je.suppressRangeEmit {
			return
		}
		if javaSuppressTypeCastIdent {
			return
		}
		if je.suppressTypeAliasSelectorX {
			// When suppressTypeAliasSelectorX is set, we already emitted the resolved type
			// in PreVisitDeclStmtValueSpecType, so suppress all identifiers in this context
			return
		}
		if je.captureRangeExpr {
			je.rangeCollectionExpr += e.Name
			return
		}
		// Suppress package identifier (e.g., fmt, types) - we emit just the type/function name
		if je.suppressFmtPackage {
			return
		}
		// Suppress struct field key in composite literal (Java uses positional args)
		if je.suppressKeyValueKey {
			return
		}
		// Suppress identifier when we're in a function variable call (already emitted with .accept etc.)
		if je.isFuncVarCall {
			return
		}
		// Suppress individual return types when function has multiple return values
		if je.suppressMultiReturnTypes {
			return
		}
		// Suppress type identifiers in make([]Type, size) - handled separately
		if je.isSliceMakeCall && je.inTypeContext {
			return
		}
		// Suppress type identifiers in lambda parameters (Java uses type inference)
		if je.inLambdaParams && je.inTypeContext {
			return
		}
		// Suppress map key/value type identifiers during make(map[K]V) call
		// But don't suppress "make" itself as it needs to be replaced
		if je.isMapMakeCall && je.suppressMapEmit && e.Name != "make" {
			return
		}
		// Also suppress when inside map type declarations (inTypeContext and suppressMapEmit)
		if je.inTypeContext && je.suppressMapEmit {
			return
		}
		// Suppress LHS identifiers during map assignment (handled in PreVisitAssignStmtRhs)
		if je.suppressMapAssignLhs {
			return
		}
		// Suppress map variable during map index read (handled in PostVisitIndexExprX)
		if je.suppressMapIndexX {
			return
		}
		// Suppress all identifiers during map comma-ok (handled in PostVisitAssignStmt)
		if je.isMapCommaOk {
			return
		}
		// Suppress all identifiers during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		// Suppress all identifiers during type assert comma-ok (handled in PostVisitAssignStmt)
		if je.isTypeAssertCommaOk {
			return
		}
		// Suppress for unsupported indexed function call (handled in PostVisitCallExpr)
		if je.isUnsupportedIndexedCall {
			return
		}

		var str string
		name := e.Name

		// Map operation identifier replacements
		if je.isMapMakeCall && name == "make" {
			name = "newHashMap"
		} else if je.isSliceMakeCall && name == "make" {
			name = "" // Suppress "make" - we emit SliceBuiltins.MakeSlice in PreVisitCallExprArg
		} else if je.isDeleteCall && name == "delete" {
			name = je.deleteMapVarName + " = hashMapDelete"
		} else if je.isMapLenCall && name == "len" {
			name = "hashMapLen"
		} else {
			name = je.lowerToBuiltins(name)
		}

		if name == "nil" {
			str = je.emitAsString("null", indent)
		} else {
			if n, ok := javaTypesMap[name]; ok {
				// Use boxed types when inside ArrayList<...>
				if je.inArrayTypeContext {
					n = toBoxedType(n)
				}
				str = je.emitAsString(n, indent)
			} else if underlyingType, ok := je.typeAliasMap[name]; ok {
				// Use boxed types when inside ArrayList<...>
				if je.inArrayTypeContext {
					underlyingType = toBoxedType(underlyingType)
				}
				str = je.emitAsString(underlyingType, indent)
			} else {
				str = je.emitAsString(name, indent)
			}
		}

		je.emitToken(str, Identifier, 0)
	})
}

func (je *JavaEmitter) PreVisitCallExpr(node *ast.CallExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Map operations
		if ident, ok := node.Fun.(*ast.Ident); ok {
			if ident.Name == "make" && len(node.Args) >= 1 {
				if mapType, ok := node.Args[0].(*ast.MapType); ok {
					je.isMapMakeCall = true
					je.mapMakeKeyType = je.getMapKeyTypeConst(mapType)
					je.suppressMapEmit = true // Suppress map type key/value emission
					javaIsTypeConversion = false
					return
				}
				if _, ok := node.Args[0].(*ast.ArrayType); ok && len(node.Args) >= 2 {
					je.isSliceMakeCall = true
					je.sliceMakeElemType = "Object"
					if je.pkg != nil && je.pkg.TypesInfo != nil {
						tv := je.pkg.TypesInfo.Types[node.Args[0]]
						if tv.Type != nil {
							if sliceType, ok := tv.Type.Underlying().(*types.Slice); ok {
								je.sliceMakeElemType = getJavaTypeName(sliceType.Elem())
							}
						}
					}
					javaIsTypeConversion = false
					return
				}
			}

			if ident.Name == "delete" && len(node.Args) >= 2 {
				je.isDeleteCall = true
				je.deleteMapVarName = exprToString(node.Args[0])
				je.mapKeyCastPrefix = ""
				je.mapKeyCastSuffix = ""
				if je.pkg != nil && je.pkg.TypesInfo != nil {
					tv := je.pkg.TypesInfo.Types[node.Args[0]]
					if tv.Type != nil {
						if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
							je.mapKeyCastPrefix, je.mapKeyCastSuffix = getJavaKeyCast(mapType.Key())
						}
					}
				}
				javaIsTypeConversion = false
				return
			}

			if ident.Name == "len" && len(node.Args) >= 1 {
				if je.pkg != nil && je.pkg.TypesInfo != nil {
					tv := je.pkg.TypesInfo.Types[node.Args[0]]
					if tv.Type != nil {
						if _, ok := tv.Type.Underlying().(*types.Map); ok {
							je.isMapLenCall = true
							javaIsTypeConversion = false
							return
						}
					}
				}
			}
		}

		// Check if calling a function variable (functional interface)
		// e.g., f(10, 20) where f is a BiConsumer -> f.accept(10, 20)
		// Only applies to variables, not function declarations
		if ident, ok := node.Fun.(*ast.Ident); ok {
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				if obj := je.pkg.TypesInfo.ObjectOf(ident); obj != nil {
					// Check if this is a variable (not a function declaration)
					if _, isVar := obj.(*types.Var); isVar {
						if sig, isFunc := obj.Type().Underlying().(*types.Signature); isFunc {
							// This is a function variable call - transform to functional interface method
							methodName := je.getFunctionalInterfaceMethod(sig)
							if methodName != "" {
								je.gir.emitToFileBuffer(fmt.Sprintf("%s.%s(", ident.Name, methodName), EmptyVisitMethod)
								je.isFuncVarCall = true
								return
							}
						}
					}
				}
			}
		}

		// Check for unsupported indexed function call: x[0](args)
		// Java doesn't support calling functions from slice index directly
		if indexExpr, ok := node.Fun.(*ast.IndexExpr); ok {
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				tv := je.pkg.TypesInfo.Types[indexExpr.X]
				if tv.Type != nil {
					if sliceType, ok := tv.Type.Underlying().(*types.Slice); ok {
						if _, isFunc := sliceType.Elem().Underlying().(*types.Signature); isFunc {
							je.isUnsupportedIndexedCall = true
							je.unsupportedCallIndent = indent
							return
						}
					}
				}
			}
		}

		// Check for type conversion
		if ident, ok := node.Fun.(*ast.Ident); ok {
			if _, isType := javaTypesMap[ident.Name]; isType {
				javaIsTypeConversion = true
				return
			}
		}
		javaIsTypeConversion = false
	})
}

func (je *JavaEmitter) PreVisitCallExprFun(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress for unsupported indexed function call
		if je.isUnsupportedIndexedCall {
			return
		}
		if javaIsTypeConversion {
			if ident, ok := node.(*ast.Ident); ok {
				typeName := ident.Name
				if mapped, ok := javaTypesMap[typeName]; ok {
					typeName = mapped
				}
				je.emitToken("(", LeftParen, 0)
				je.emitToken(typeName, Identifier, 0)
				je.emitToken(")", RightParen, 0)
				javaSuppressTypeCastIdent = true
			}
		}
	})
}

func (je *JavaEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		javaSuppressTypeCastIdent = false
	})
}

func (je *JavaEmitter) PreVisitCallExprArgs(node []ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress for unsupported indexed function call
		if je.isUnsupportedIndexedCall {
			return
		}
		if je.isMultiReturnAssign {
			return
		}
		if je.isMapMakeCall {
			je.emitToken(fmt.Sprintf("(%d", je.mapMakeKeyType), LeftParen, 0)
			return
		}
		if je.isSliceMakeCall {
			return
		}
		// Suppress opening paren for func var call (already emitted with f.accept()
		if je.isFuncVarCall {
			return
		}
		je.emitToken("(", LeftParen, 0)
	})
}

func (je *JavaEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress for unsupported indexed function call
		if je.isUnsupportedIndexedCall {
			return
		}
		if je.isMultiReturnAssign {
			return
		}
		if je.isSliceMakeCall {
			je.emitToken(")", RightParen, 0)
			je.isSliceMakeCall = false
			je.sliceMakeElemType = ""
			je.isArray = false
			return
		}
		je.emitToken(")", RightParen, 0)
		// Reset functional interface call flag
		je.isFuncVarCall = false
		if je.isMapMakeCall {
			je.suppressMapEmit = false
		}
		je.isMapMakeCall = false
		if je.isDeleteCall {
			je.mapKeyCastPrefix = ""
			je.mapKeyCastSuffix = ""
		}
		je.isDeleteCall = false
		je.deleteMapVarName = ""
		je.isMapLenCall = false
	})
}

func (je *JavaEmitter) PreVisitBasicLit(e *ast.BasicLit, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress literals during map assignment LHS (handled in PreVisitAssignStmtRhs)
		if je.suppressMapAssignLhs {
			return
		}
		// Suppress literals during map comma-ok (handled in PostVisitAssignStmt)
		if je.isMapCommaOk {
			return
		}
		// Suppress literals during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		// Suppress literals during type assert comma-ok (handled in PostVisitAssignStmt)
		if je.isTypeAssertCommaOk {
			return
		}
		// Suppress for unsupported indexed function call (handled in PostVisitCallExpr)
		if je.isUnsupportedIndexedCall {
			return
		}
		var str string
		if e.Kind == token.STRING {
			value := e.Value
			if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
				value = value[1 : len(value)-1]
				str = je.emitAsString(fmt.Sprintf("\"%s\"", value), 0)
			} else if len(value) >= 2 && value[0] == '`' && value[len(value)-1] == '`' {
				// Raw string literal - Java doesn't have verbatim strings, escape as needed
				value = value[1 : len(value)-1]
				// Escape backslashes and quotes for Java
				value = strings.ReplaceAll(value, "\\", "\\\\")
				value = strings.ReplaceAll(value, "\"", "\\\"")
				str = je.emitAsString(fmt.Sprintf("\"%s\"", value), 0)
			} else {
				str = je.emitAsString(fmt.Sprintf("\"%s\"", value), 0)
			}
			je.emitToken(str, StringLiteral, 0)
		} else if e.Kind == token.CHAR {
			str = je.emitAsString(e.Value, 0)
			je.emitToken(str, NumberLiteral, 0)
		} else if e.Kind == token.INT {
			// Check if integer literal exceeds Java int max (2147483647)
			value := e.Value
			// Parse the integer value to check if it needs L suffix
			if len(value) > 0 {
				// Remove any underscores (Go allows _ in numeric literals)
				cleanValue := strings.ReplaceAll(value, "_", "")
				// Check if it's a large number (simple heuristic: > 10 digits or compare)
				if len(cleanValue) >= 10 {
					// Parse as int64 to check
					if n, err := strconv.ParseInt(cleanValue, 0, 64); err == nil {
						if n > 2147483647 || n < -2147483648 {
							value = value + "L"
						}
					}
				}
			}
			str = je.emitAsString(value, 0)
			je.emitToken(str, NumberLiteral, 0)
		} else {
			str = je.emitAsString(e.Value, 0)
			je.emitToken(str, NumberLiteral, 0)
		}
	})
}

func (je *JavaEmitter) PreVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.isArray = false
		je.inTypeContext = true
		if len(node.Values) == 0 && node.Type != nil {
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				if typeAndValue, ok := je.pkg.TypesInfo.Types[node.Type]; ok {
					if _, isMap := typeAndValue.Type.Underlying().(*types.Map); isMap {
						if mapType, ok := node.Type.(*ast.MapType); ok {
							je.pendingMapInit = true
							je.pendingMapKeyType = je.getMapKeyTypeConst(mapType)
						}
					}
					// Check for struct types that need initialization
					if _, isStruct := typeAndValue.Type.Underlying().(*types.Struct); isStruct {
						// Get the type name (either Ident or SelectorExpr)
						if ident, ok := node.Type.(*ast.Ident); ok {
							je.pendingStructInit = true
							je.pendingStructTypeName = ident.Name
						} else if sel, ok := node.Type.(*ast.SelectorExpr); ok {
							je.pendingStructInit = true
							je.pendingStructTypeName = sel.Sel.Name
						}
					}
					// Check for slice types - initialize to empty ArrayList
					if sliceType, isSlice := typeAndValue.Type.Underlying().(*types.Slice); isSlice {
						je.pendingSliceInit = true
						je.pendingSliceElemType = toBoxedType(getJavaTypeName(sliceType.Elem()))
					}
					// Check for primitive types - initialize to default value
					if basic, isBasic := typeAndValue.Type.Underlying().(*types.Basic); isBasic {
						switch basic.Kind() {
						case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
							types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
							je.pendingPrimitiveInit = true
							je.pendingPrimitiveDefault = "0"
						case types.Float32, types.Float64:
							je.pendingPrimitiveInit = true
							je.pendingPrimitiveDefault = "0.0"
						case types.Bool:
							je.pendingPrimitiveInit = true
							je.pendingPrimitiveDefault = "false"
						case types.String:
							je.pendingPrimitiveInit = true
							je.pendingPrimitiveDefault = "\"\""
						}
					}
					// Check if it's a type alias from another package - resolve to underlying type
					if _, ok := node.Type.(*ast.SelectorExpr); ok {
						underlyingType := typeAndValue.Type.Underlying()
						if basic, ok := underlyingType.(*types.Basic); ok {
							// Emit the underlying basic type directly
							javaType := getJavaTypeName(basic)
							je.gir.emitToFileBuffer(javaType, EmptyVisitMethod)
							je.suppressTypeAliasSelectorX = true
							je.inTypeContext = false // Skip normal type emission
						}
					}
				}
			}
		}
	})
}

func (je *JavaEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inTypeContext = false
		je.suppressTypeAliasSelectorX = false
		je.isArray = false // Reset array flag after declaration type
	})
}

func (je *JavaEmitter) PreVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Just emit space before name - the name itself is emitted by PreVisitIdent
		str := je.emitAsString(" ", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.pendingMapInit {
			je.gir.emitToFileBuffer(fmt.Sprintf(" = newHashMap(%d)", je.pendingMapKeyType), EmptyVisitMethod)
			je.pendingMapInit = false
			je.pendingMapKeyType = 0
		}
		if je.pendingStructInit {
			je.gir.emitToFileBuffer(fmt.Sprintf(" = new %s()", je.pendingStructTypeName), EmptyVisitMethod)
			je.pendingStructInit = false
			je.pendingStructTypeName = ""
		}
		if je.pendingSliceInit {
			je.gir.emitToFileBuffer(fmt.Sprintf(" = new ArrayList<%s>()", je.pendingSliceElemType), EmptyVisitMethod)
			je.pendingSliceInit = false
			je.pendingSliceElemType = ""
		}
		if je.pendingPrimitiveInit {
			je.gir.emitToFileBuffer(fmt.Sprintf(" = %s", je.pendingPrimitiveDefault), EmptyVisitMethod)
			je.pendingPrimitiveInit = false
			je.pendingPrimitiveDefault = ""
		}
		je.gir.emitToFileBuffer(";\n", EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitGenStructFieldType(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("public ", indent+4)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(" ", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.gir.emitToFileBuffer(";\n", EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.pkg = pkg
		// Only create class if not already opened
		if je.apiClassOpened {
			return
		}
		var className string
		if je.OutputName != "" {
			// Use the output file name as the class name
			className = je.OutputName
		} else if pkg.Name == "main" {
			className = "Main"
		} else {
			className = pkg.Name
		}
		str := je.emitAsString(fmt.Sprintf("public class %s {\n\n", className), indent)
		err := je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		err = je.gir.emitToFileBufferString("", pkg.Name)
		je.currentPackage = className
		je.apiClassOpened = true
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
		}
	})
}

func (je *JavaEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Don't close the class here - it will be closed in PostVisitProgram
		// This prevents multiple class closings when processing multiple packages
	})
}

func (je *JavaEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.isArray = false
	})
}

func (je *JavaEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("\n", indent)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("\n\n", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitGenStructInfo(node GenTypeInfo, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(fmt.Sprintf("public static class %s {\n", node.Name), indent+2)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Generate constructor for the struct
		if node.Struct != nil && node.Struct.Fields != nil {
			var fields []struct {
				name     string
				javaType string
			}

			// Collect field names and types
			for _, field := range node.Struct.Fields.List {
				// Get Java type for the field
				javaType := "Object"
				if je.pkg != nil && je.pkg.TypesInfo != nil {
					tv := je.pkg.TypesInfo.Types[field.Type]
					if tv.Type != nil {
						javaType = getJavaTypeName(tv.Type)
					}
				} else {
					// Fallback: try to get type from AST
					javaType = je.getJavaTypeFromExpr(field.Type)
				}

				// Handle multiple field names (e.g., `a, b int`)
				for _, name := range field.Names {
					fields = append(fields, struct {
						name     string
						javaType string
					}{name: name.Name, javaType: javaType})
				}
			}

			// Generate default (no-arg) constructor that initializes fields to defaults
			defaultConstructor := fmt.Sprintf("        public %s() {\n", node.Name)
			for _, f := range fields {
				defaultConstructor += fmt.Sprintf("            this.%s = %s;\n", f.name, getJavaDefaultValue(f.javaType))
			}
			defaultConstructor += "        }\n"
			je.gir.emitToFileBuffer(defaultConstructor, EmptyVisitMethod)

			// Generate all-args constructor
			constructorCode := fmt.Sprintf("        public %s(", node.Name)
			for i, f := range fields {
				if i > 0 {
					constructorCode += ", "
				}
				constructorCode += fmt.Sprintf("%s %s", f.javaType, f.name)
			}
			constructorCode += ") {\n"
			for _, f := range fields {
				constructorCode += fmt.Sprintf("            this.%s = %s;\n", f.name, f.name)
			}
			constructorCode += "        }\n"
			je.gir.emitToFileBuffer(constructorCode, EmptyVisitMethod)
		}

		str := je.emitAsString("}\n\n", indent+2)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitArrayType(node ast.ArrayType, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.suppressTypeAliasEmit || je.suppressMapEmit || je.suppressMultiReturnTypes || je.isSliceMakeCall {
			return
		}
		je.inArrayTypeContext = true // Track for boxed type conversion
		// Detect byte/short element types for literal casting
		if ident, ok := node.Elt.(*ast.Ident); ok {
			switch ident.Name {
			case "int8", "byte":
				je.sliceLitElemCast = "(byte)"
			case "int16":
				je.sliceLitElemCast = "(short)"
			default:
				je.sliceLitElemCast = ""
			}
		} else {
			je.sliceLitElemCast = ""
		}
		str := je.emitAsString("ArrayList", indent)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		str = je.emitAsString("<", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.suppressTypeAliasEmit || je.suppressMapEmit || je.suppressMultiReturnTypes || je.isSliceMakeCall {
			return
		}
		je.inArrayTypeContext = false // Clear boxed type context
		str := je.emitAsString(">", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)

		if je.isSliceMakeCall {
			return
		}

		pointerAndPosition := SearchPointerIndexReverse(PreVisitArrayType, je.gir.pointerAndIndexVec)
		if pointerAndPosition != nil {
			tokens, _ := ExtractTokens(pointerAndPosition.Index, je.gir.tokenSlice)
			je.isArray = true
			je.arrayType = strings.Join(tokens, "")
			je.gir.pointerAndIndexVec = RemovePointerEntryReverse(je.gir.pointerAndIndexVec, PreVisitArrayType)
		}
	})
}

func (je *JavaEmitter) PreVisitMapType(node *ast.MapType, indent int) {
	if je.isMapMakeCall {
		return
	}
	// Also suppress HashMap emission when inside slice make call type argument
	if je.isSliceMakeCall && je.inTypeContext {
		return
	}
	je.executeIfNotForwardDecls(func() {
		if je.suppressMapEmit {
			return
		}
		// Use HashMap without prefix since it's defined as an inner class
		je.gir.emitToFileBuffer("HashMap", EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitMapType(node *ast.MapType, indent int) {
}

func (je *JavaEmitter) PreVisitMapKeyType(node ast.Expr, indent int) {
	je.suppressMapEmitStack = append(je.suppressMapEmitStack, je.suppressMapEmit)
	je.suppressMapEmit = true
	je.inTypeContext = true // Ensure type context is set
}

func (je *JavaEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {
	je.inTypeContext = false
	if len(je.suppressMapEmitStack) > 0 {
		je.suppressMapEmit = je.suppressMapEmitStack[len(je.suppressMapEmitStack)-1]
		je.suppressMapEmitStack = je.suppressMapEmitStack[:len(je.suppressMapEmitStack)-1]
	} else {
		je.suppressMapEmit = false
	}
}

func (je *JavaEmitter) PreVisitMapValueType(node ast.Expr, indent int) {
	je.suppressMapEmitStack = append(je.suppressMapEmitStack, je.suppressMapEmit)
	je.suppressMapEmit = true
	je.inTypeContext = true // Ensure type context is set
}

func (je *JavaEmitter) PostVisitMapValueType(node ast.Expr, indent int) {
	je.inTypeContext = false
	if len(je.suppressMapEmitStack) > 0 {
		je.suppressMapEmit = je.suppressMapEmitStack[len(je.suppressMapEmitStack)-1]
		je.suppressMapEmitStack = je.suppressMapEmitStack[:len(je.suppressMapEmitStack)-1]
	} else {
		je.suppressMapEmit = false
	}
}

func (je *JavaEmitter) PreVisitFuncType(node *ast.FuncType, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inTypeContext = true
		// Map Go function types to Java functional interfaces
		funcInterfaceType := je.getFunctionalInterfaceType(node)
		str := je.emitAsString(funcInterfaceType, indent)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		// Suppress all type parameter emissions (we already emitted the full type)
		je.suppressMultiReturnTypes = true
	})
}

func (je *JavaEmitter) PostVisitFuncType(node *ast.FuncType, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inTypeContext = false
		je.numFuncResults = 0
		je.suppressMultiReturnTypes = false
	})
}

func (je *JavaEmitter) PreVisitFuncTypeParam(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress parameter emissions for function types
	})
}

func (je *JavaEmitter) PreVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Check if X is a package - suppress it since all code is in one Java file
		// Do this first before any early returns
		if ident, ok := node.X.(*ast.Ident); ok {
			if ident.Name == "fmt" {
				je.suppressFmtPackage = true
			}
			// Check if this is a package-qualified reference (not a struct field access)
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				if uses, ok := je.pkg.TypesInfo.Uses[ident]; ok {
					if _, isPkg := uses.(*types.PkgName); isPkg {
						// This is a package reference - suppress the package name
						je.suppressFmtPackage = true // reuse this flag for package suppression
					}
				}
			}
		}

		if je.isMultiReturnAssign {
			return
		}
		// Suppress during map operations (handled elsewhere)
		if je.suppressMapAssignLhs || je.isMapCommaOk || je.suppressMapIndexX {
			return
		}
		// Suppress during type assert comma-ok (handled in PostVisitAssignStmt)
		if je.isTypeAssertCommaOk {
			return
		}
		// Skip if we've already resolved a type alias
		if je.suppressTypeAliasSelectorX && je.inTypeContext {
			return
		}
		if je.captureRangeExpr {
			je.rangeCollectionExpr += exprToString(node.X) + "."
			return
		}
	})
}

func (je *JavaEmitter) PostVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressFmtPackage = false
	})
}

func (je *JavaEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isMultiReturnAssign {
			return
		}
		if je.suppressTypeAliasSelectorX {
			return
		}
		// Don't emit dot when capturing range expression
		if je.captureRangeExpr {
			return
		}
		// Skip package prefix (use types/functions directly since all in one file)
		// Reset the flag so the type/function name is emitted
		if je.suppressFmtPackage {
			je.suppressFmtPackage = false // Reset so Sel identifier is emitted
			return                        // Don't emit dot
		}
		// Don't emit dot for package-qualified type aliases that were resolved
		if je.inTypeContext && je.suppressTypeAliasSelectorX {
			return
		}
		// Suppress dot during map operations (handled in PreVisitAssignStmtRhs or PostVisitIndexExprX)
		if je.suppressMapAssignLhs || je.isMapCommaOk || je.suppressMapIndexX {
			return
		}
		// Suppress dot during type assert comma-ok (handled in PostVisitAssignStmt)
		if je.isTypeAssertCommaOk {
			return
		}
		str := je.emitAsString(".", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitFuncTypeResults(node *ast.FieldList, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.suppressMultiReturnTypes {
			return
		}
		if node != nil && len(node.List) > 0 {
			je.numFuncResults = len(node.List)
			je.emitToken(", ", Comma, 0)
		}
	})
}

func (je *JavaEmitter) PreVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if index > 0 {
			str := je.emitAsString(", ", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PreVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inTypeContext = false
		je.gir.emitToFileBuffer(" ", EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.suppressMultiReturnTypes {
			return
		}
		if index > 0 {
			str := je.emitAsString(", ", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.isTuple = true
	})
}

func (je *JavaEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.isTuple = false
		je.numFuncResults = 0
		je.suppressMultiReturnTypes = false
		je.currentFuncName = node.Name.Name
		if node.Type.Results != nil {
			je.numFuncResults = len(node.Type.Results.List)
		}

		var resultClassName string
		if je.numFuncResults > 1 {
			// Collect return types
			var returnTypes []string
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				for _, field := range node.Type.Results.List {
					tv := je.pkg.TypesInfo.Types[field.Type]
					if tv.Type != nil {
						returnTypes = append(returnTypes, getJavaTypeName(tv.Type))
					} else {
						returnTypes = append(returnTypes, "Object")
					}
				}
			} else {
				for range node.Type.Results.List {
					returnTypes = append(returnTypes, "Object")
				}
			}

			// Create signature key for registry
			signature := strings.Join(returnTypes, ",")

			// Check if we already have a class for this signature
			if existingClass, ok := je.resultClassRegistry[signature]; ok {
				// Reuse existing class
				resultClassName = existingClass
			} else {
				// Create new class with first function's name
				resultClassName = node.Name.Name + "Result"
				je.resultClassRegistry[signature] = resultClassName

				// Generate and emit the result class definition
				classCode := fmt.Sprintf("\n    public static class %s {\n", resultClassName)
				for i, typeName := range returnTypes {
					classCode += fmt.Sprintf("        public %s _%d;\n", typeName, i)
				}
				// Constructor
				classCode += fmt.Sprintf("        public %s(", resultClassName)
				for i, typeName := range returnTypes {
					if i > 0 {
						classCode += ", "
					}
					classCode += fmt.Sprintf("%s _%d", typeName, i)
				}
				classCode += ") {\n"
				for i := range returnTypes {
					classCode += fmt.Sprintf("            this._%d = _%d;\n", i, i)
				}
				classCode += "        }\n    }\n\n"
				je.gir.emitToFileBuffer(classCode, EmptyVisitMethod)
			}

			// Store for later use at call sites (use the shared class name)
			je.multiReturnFuncs[node.Name.Name] = returnTypes
		}

		// Now emit the method signature
		str := je.emitAsString("public static ", indent+2)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)

		if je.numFuncResults == 0 {
			str = je.emitAsString("void ", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		} else if je.numFuncResults > 1 {
			// Get the result class name from registry based on signature
			var returnTypes []string
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				for _, field := range node.Type.Results.List {
					tv := je.pkg.TypesInfo.Types[field.Type]
					if tv.Type != nil {
						returnTypes = append(returnTypes, getJavaTypeName(tv.Type))
					} else {
						returnTypes = append(returnTypes, "Object")
					}
				}
			}
			signature := strings.Join(returnTypes, ",")
			resultClassName = je.resultClassRegistry[signature]

			// Emit the result class name as return type
			str = je.emitAsString(resultClassName+" ", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
			// Suppress individual return type emissions
			je.suppressMultiReturnTypes = true
		}
	})
}

func (je *JavaEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressMultiReturnTypes = false
		if je.numFuncResults == 1 {
			str := je.emitAsString(" ", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PreVisitTypeAliasName(node *ast.Ident, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressTypeAliasEmit = true
		je.currentAliasName = node.Name
	})
}

func (je *JavaEmitter) PostVisitTypeAliasName(node *ast.Ident, indent int) {
}

func (je *JavaEmitter) PreVisitTypeAliasType(node ast.Expr, indent int) {
	// Type aliases are recorded but not emitted in Java
}

func (je *JavaEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	if je.currentAliasName != "" {
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			tv := je.pkg.TypesInfo.Types[node]
			if tv.Type != nil {
				underlyingType := getJavaTypeName(tv.Type)
				je.typeAliasMap[je.currentAliasName] = underlyingType
			}
		}
	}
	je.suppressTypeAliasEmit = false
	je.currentAliasName = ""
}

func (je *JavaEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("return ", indent)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)

		if len(node.Results) > 1 {
			// Look up the shared result class name from the registry
			var resultClassName string
			if returnTypes, ok := je.multiReturnFuncs[je.currentFuncName]; ok {
				signature := strings.Join(returnTypes, ",")
				if className, ok := je.resultClassRegistry[signature]; ok {
					resultClassName = className
				} else {
					resultClassName = je.currentFuncName + "Result"
				}
			} else {
				resultClassName = je.currentFuncName + "Result"
			}
			je.emitToken(fmt.Sprintf("new %s(", resultClassName), LeftParen, 0)
		}
	})
}

func (je *JavaEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		if len(node.Results) > 1 {
			je.emitToken(")", RightParen, 0)
		}
		str := je.emitAsString(";", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if index > 0 {
			str := je.emitAsString(", ", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
		// Add cast for byte/short return values when returning literals
		if returnTypes, ok := je.multiReturnFuncs[je.currentFuncName]; ok {
			if index < len(returnTypes) {
				// Only cast BasicLit (numeric literals)
				if _, isLit := node.(*ast.BasicLit); isLit {
					switch returnTypes[index] {
					case "byte":
						je.gir.emitToFileBuffer("(byte)", EmptyVisitMethod)
					case "short":
						je.gir.emitToFileBuffer("(short)", EmptyVisitMethod)
					}
				}
			}
		}
	})
}

func (je *JavaEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isUnsupportedIndexedCall {
			// Emit a comment for unsupported indexed function call
			indentStr := strings.Repeat("    ", je.unsupportedCallIndent)
			je.gir.emitToFileBuffer(fmt.Sprintf("%s/* TODO: Unsupported indexed function call: %s */",
				indentStr, exprToString(node)), EmptyVisitMethod)
			je.isUnsupportedIndexedCall = false
		}
	})
}

func (je *JavaEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	// Check if all LHS are blank identifiers
	allBlank := true
	for _, lhs := range node.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok {
			if ident.Name != "_" {
				allBlank = false
				break
			}
		} else {
			allBlank = false
			break
		}
	}
	if allBlank {
		je.suppressRangeEmit = true
		return
	}

	// Detect comma-ok map access
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if indexExpr, ok := node.Rhs[0].(*ast.IndexExpr); ok {
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				tv := je.pkg.TypesInfo.Types[indexExpr.X]
				if tv.Type != nil {
					if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
						je.isMapCommaOk = true
						je.mapCommaOkValName = node.Lhs[0].(*ast.Ident).Name
						je.mapCommaOkOkName = node.Lhs[1].(*ast.Ident).Name
						je.mapCommaOkMapName = exprToString(indexExpr.X)
						je.mapCommaOkValType = getJavaTypeName(mapType.Elem())
						je.mapCommaOkIsDecl = (node.Tok == token.DEFINE)
						je.mapCommaOkIndent = indent
						je.mapKeyCastPrefix, je.mapKeyCastSuffix = getJavaKeyCast(mapType.Key())
						je.suppressMapEmit = true
						return
					}
				}
			}
		}
		// Detect type assert comma-ok: val, ok := x.(Type)
		if typeAssert, ok := node.Rhs[0].(*ast.TypeAssertExpr); ok {
			je.isTypeAssertCommaOk = true
			je.typeAssertCommaOkValName = node.Lhs[0].(*ast.Ident).Name
			je.typeAssertCommaOkOkName = node.Lhs[1].(*ast.Ident).Name
			je.typeAssertCommaOkXName = exprToString(typeAssert.X)
			je.typeAssertCommaOkIsDecl = (node.Tok == token.DEFINE)
			je.typeAssertCommaOkIndent = indent
			// Get the type being asserted
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				tv := je.pkg.TypesInfo.Types[typeAssert.Type]
				if tv.Type != nil {
					je.typeAssertCommaOkType = getJavaTypeName(tv.Type)
					je.typeAssertCommaOkBoxedType = toBoxedType(je.typeAssertCommaOkType)
				}
			}
			return
		}
	}

	// Detect multi-return function call assignment
	if len(node.Lhs) > 1 && len(node.Rhs) == 1 {
		if callExpr, ok := node.Rhs[0].(*ast.CallExpr); ok {
			funcName := ""
			if ident, ok := callExpr.Fun.(*ast.Ident); ok {
				funcName = ident.Name
			} else if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
				funcName = sel.Sel.Name
			}
			if _, isMultiReturn := je.multiReturnFuncs[funcName]; isMultiReturn || funcName != "" {
				// Collect LHS variable names
				je.multiReturnLhsNames = nil
				for _, lhs := range node.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						je.multiReturnLhsNames = append(je.multiReturnLhsNames, ident.Name)
					}
				}
				je.isMultiReturnAssign = true
				je.multiReturnIsDecl = (node.Tok == token.DEFINE)
				je.multiReturnIndent = indent
				return
			}
		}
	}

	// Detect map assignment (including nested maps)
	if len(node.Lhs) == 1 {
		if indexExpr, ok := node.Lhs[0].(*ast.IndexExpr); ok {
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				tv := je.pkg.TypesInfo.Types[indexExpr.X]
				if tv.Type != nil {
					if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
						je.suppressMapEmit = true
						je.suppressMapAssignLhs = true // Suppress LHS emission
						je.assignmentToken = "="
						je.mapKeyCastPrefix, je.mapKeyCastSuffix = getJavaKeyCast(mapType.Key())

						// Check if this is a nested map assignment (e.g., m["a"]["b"] = v)
						// Only handle as nested map if the inner expression is also a map access
						if innerIndex, isNestedIndex := indexExpr.X.(*ast.IndexExpr); isNestedIndex {
							// Check if the inner IndexExpr is accessing a map (not a slice)
							innerTv := je.pkg.TypesInfo.Types[innerIndex.X]
							if innerTv.Type != nil {
								if _, isInnerMap := innerTv.Type.Underlying().(*types.Map); isInnerMap {
									// Nested map assignment (map[K1]map[K2]V)
									je.isNestedMapAssign = true
									je.nestedMapAssignIndent = indent
									rootVar, levels := je.collectNestedMapLevels(indexExpr.X)
									je.nestedMapRootVar = rootVar
									je.nestedMapLevels = levels
									je.nestedMapAssignKey = exprToString(indexExpr.Index)
									return
								}
								// Inner is a slice access with map element (e.g., sliceOfMaps[0][1]["a"] = v)
								// This needs special handling for any depth of slice nesting
								if _, isInnerSlice := innerTv.Type.Underlying().(*types.Slice); isInnerSlice {
									je.isSliceOfMapAssign = true
									// Collect all slice indices from the chain
									rootVar, indices := je.collectSliceIndices(indexExpr.X)
									je.sliceOfMapSliceVar = rootVar
									je.sliceOfMapSliceIndices = indices
									je.sliceOfMapMapKey = exprToString(indexExpr.Index)
									je.sliceOfMapIndent = indent
									je.suppressMapEmit = true
									je.suppressMapAssignLhs = true
									return
								}
							}
						}

						// Simple map assignment
						je.isMapAssign = true
						je.mapAssignIndent = indent
						je.mapAssignVarName = exprToString(indexExpr.X)
						return
					}
					// Detect slice index assignment: x[i] = value -> x.set(i, value)
					if _, ok := tv.Type.Underlying().(*types.Slice); ok {
						je.isSliceIndexAssign = true
						je.sliceIndexVarName = exprToString(indexExpr.X)
						je.sliceIndexIndent = indent
						je.sliceIndexLhsExpr = indexExpr // Track the specific LHS index expr
						return
					}
				}
			}
		}
	}

	// Detect byte/short arithmetic: a = a + 5 where a is byte/short
	// In Java, byte/short arithmetic produces int, so we need to cast back
	if len(node.Lhs) == 1 && len(node.Rhs) == 1 {
		if ident, ok := node.Lhs[0].(*ast.Ident); ok {
			// Check if RHS contains a binary expression (arithmetic)
			if _, isBinary := node.Rhs[0].(*ast.BinaryExpr); isBinary {
				if je.pkg != nil && je.pkg.TypesInfo != nil {
					if obj := je.pkg.TypesInfo.ObjectOf(ident); obj != nil {
						typeName := obj.Type().String()
						switch typeName {
						case "int8", "byte":
							je.needsSmallIntCast = true
							je.smallIntCastType = "byte"
						case "int16":
							je.needsSmallIntCast = true
							je.smallIntCastType = "short"
						}
					}
				}
			}
		}
	}

	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("", indent)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	// Handle map comma-ok
	if je.isMapCommaOk {
		// Check if val and ok are already declared
		valVarDecl := ""
		okVarDecl := ""
		if je.mapCommaOkIsDecl {
			if !je.isVarDeclared(je.mapCommaOkOkName) {
				okVarDecl = "var "
				je.declareVar(je.mapCommaOkOkName)
			}
			if !je.isVarDeclared(je.mapCommaOkValName) {
				valVarDecl = "var "
				je.declareVar(je.mapCommaOkValName)
			}
		}
		// Add cast for the value type
		boxedValType := toBoxedType(je.mapCommaOkValType)
		keyExpr := exprToString(node.Rhs[0].(*ast.IndexExpr).Index)
		// First emit ok variable
		code := fmt.Sprintf("%s%s = hashMapContains(%s, %s%s%s);\n",
			okVarDecl, je.mapCommaOkOkName,
			je.mapCommaOkMapName,
			je.mapKeyCastPrefix, keyExpr, je.mapKeyCastSuffix)
		// Then emit val with ternary to provide default value when key not found
		// This avoids NPE when auto-unboxing null for comparisons
		defaultVal := getJavaDefaultValue(je.mapCommaOkValType)
		code += fmt.Sprintf("%s%s = %s ? (%s)hashMapGet(%s, %s%s%s) : %s;\n",
			valVarDecl, je.mapCommaOkValName,
			je.mapCommaOkOkName,
			boxedValType,
			je.mapCommaOkMapName,
			je.mapKeyCastPrefix, keyExpr, je.mapKeyCastSuffix,
			defaultVal)
		je.gir.emitToFileBuffer(code, EmptyVisitMethod)
		je.isMapCommaOk = false
		je.suppressMapEmit = false
		return
	}

	// Handle type assert comma-ok: val, ok := x.(Type)
	if je.isTypeAssertCommaOk {
		// Check if val and ok are already declared
		valVarDecl := ""
		okVarDecl := ""
		if je.typeAssertCommaOkIsDecl {
			if !je.isVarDeclared(je.typeAssertCommaOkOkName) {
				okVarDecl = "var "
				je.declareVar(je.typeAssertCommaOkOkName)
			}
			if !je.isVarDeclared(je.typeAssertCommaOkValName) {
				valVarDecl = "var "
				je.declareVar(je.typeAssertCommaOkValName)
			}
		}
		indentStr := strings.Repeat("    ", je.typeAssertCommaOkIndent)
		// Generate: var ok = x instanceof Type;
		// Generate: var val = ok ? (Type)x : defaultValue;
		code := fmt.Sprintf("%s%s%s = %s instanceof %s;\n",
			indentStr, okVarDecl, je.typeAssertCommaOkOkName,
			je.typeAssertCommaOkXName, je.typeAssertCommaOkBoxedType)

		// Get default value for the type
		defaultVal := getJavaDefaultValue(je.typeAssertCommaOkType)
		code += fmt.Sprintf("%s%s%s = %s ? (%s)%s : %s;\n",
			indentStr, valVarDecl, je.typeAssertCommaOkValName,
			je.typeAssertCommaOkOkName,
			je.typeAssertCommaOkType, je.typeAssertCommaOkXName,
			defaultVal)

		je.gir.emitToFileBuffer(code, EmptyVisitMethod)
		je.isTypeAssertCommaOk = false
		return
	}

	// Handle multi-return function call assignment
	if je.isMultiReturnAssign {
		callExpr := node.Rhs[0].(*ast.CallExpr)
		funcName := ""
		if ident, ok := callExpr.Fun.(*ast.Ident); ok {
			funcName = ident.Name
		} else if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
			// Check if X is a package - if so, just use the function name
			if xIdent, ok := sel.X.(*ast.Ident); ok {
				if je.pkg != nil && je.pkg.TypesInfo != nil {
					if uses, ok := je.pkg.TypesInfo.Uses[xIdent]; ok {
						if _, isPkg := uses.(*types.PkgName); isPkg {
							// It's a package reference - just use the function name
							funcName = sel.Sel.Name
						} else {
							funcName = exprToString(sel.X) + "." + sel.Sel.Name
						}
					} else {
						funcName = exprToString(sel.X) + "." + sel.Sel.Name
					}
				} else {
					funcName = exprToString(sel.X) + "." + sel.Sel.Name
				}
			} else {
				funcName = exprToString(sel.X) + "." + sel.Sel.Name
			}
		}

		// Generate function call arguments
		var args []string
		for _, arg := range callExpr.Args {
			args = append(args, exprToString(arg))
		}
		argsStr := strings.Join(args, ", ")

		// Use unique result variable name to avoid redeclaration
		je.multiReturnCounter++
		resultVarName := fmt.Sprintf("_result%d", je.multiReturnCounter)

		indentStr := strings.Repeat("    ", je.multiReturnIndent)
		code := fmt.Sprintf("%svar %s = %s(%s);\n", indentStr, resultVarName, funcName, argsStr)

		// Unpack fields with variable shadowing check
		for i, name := range je.multiReturnLhsNames {
			if name == "_" {
				continue // Skip blank identifiers
			}
			if je.multiReturnIsDecl && !je.isVarDeclared(name) {
				code += fmt.Sprintf("%svar %s = %s._%d;\n", indentStr, name, resultVarName, i)
				je.declareVar(name)
			} else {
				code += fmt.Sprintf("%s%s = %s._%d;\n", indentStr, name, resultVarName, i)
			}
		}

		je.gir.emitToFileBuffer(code, EmptyVisitMethod)
		je.isMultiReturnAssign = false
		je.multiReturnLhsNames = nil
		return
	}

	// Handle map assignment
	if je.isMapAssign {
		je.isMapAssign = false
		je.suppressMapEmit = false
		return
	}

	// Handle nested map assignment
	if je.isNestedMapAssign {
		// Close all the nested hashMapSet calls
		// For n levels, we have n+1 hashMapSet calls, so we need n+1 closing parens
		// Plus 1 for the semicolon
		closingParens := strings.Repeat(")", len(je.nestedMapLevels)+1) + ";"
		je.gir.emitToFileBuffer(closingParens, EmptyVisitMethod)
		je.isNestedMapAssign = false
		je.nestedMapRootVar = ""
		je.nestedMapLevels = nil
		je.nestedMapAssignKey = ""
		je.suppressMapEmit = false
		return
	}

	// Handle slice-of-map assignment
	if je.isSliceOfMapAssign {
		// Close the hashMapSet and set calls: ));
		je.gir.emitToFileBuffer("));", EmptyVisitMethod)
		je.isSliceOfMapAssign = false
		je.sliceOfMapSliceVar = ""
		je.sliceOfMapSliceIndices = nil
		je.sliceOfMapMapKey = ""
		je.suppressMapEmit = false
		return
	}

	// Handle slice index assignment
	if je.isSliceIndexAssign {
		je.isSliceIndexAssign = false
		je.sliceIndexLhsExpr = nil
		return
	}

	if je.suppressRangeEmit {
		je.suppressRangeEmit = false
		return
	}

	// Don't emit semicolon if we're inside for loop init or post (handled elsewhere)
	if je.insideForInit || je.insideForPostCond {
		return
	}

	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(";", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	if je.isMapCommaOk || je.isTypeAssertCommaOk || je.isMultiReturnAssign {
		return
	}
	je.executeIfNotForwardDecls(func() {
		if je.isMapAssign {
			je.suppressMapAssignLhs = false // Reset LHS suppression
			indexExpr := node.Lhs[0].(*ast.IndexExpr)
			keyStr := exprToString(indexExpr.Index)
			je.gir.emitToFileBuffer(fmt.Sprintf("%s = hashMapSet(%s, %s%s%s, ",
				je.mapAssignVarName, je.mapAssignVarName,
				je.mapKeyCastPrefix, keyStr, je.mapKeyCastSuffix), EmptyVisitMethod)
			return
		}
		// Handle nested map assignment: m["a"]["b"] = v
		// Generates: m = hashMapSet(m, "a", hashMapSet(((HashMap)hashMapGet(m, "a")), "b", v));
		if je.isNestedMapAssign {
			je.suppressMapAssignLhs = false // Reset LHS suppression

			// Build the nested hashMapSet/hashMapGet expression
			// For m["a"]["b"] = v with levels=[{"a", "HashMap"}] and finalKey="b":
			// Output: m = hashMapSet(m, "a", hashMapSet(((HashMap)hashMapGet(m, "a")), "b", v));

			var code string
			rootVar := je.nestedMapRootVar
			levels := je.nestedMapLevels
			finalKey := je.nestedMapAssignKey

			// Start with outermost: rootVar = hashMapSet(rootVar, levels[0].key,
			code = fmt.Sprintf("%s = hashMapSet(%s, %s, ", rootVar, rootVar, levels[0].keyExpr)

			// For each intermediate level (1 to n-1), wrap with hashMapSet(hashMapGet(...), key,
			for i := 1; i < len(levels); i++ {
				// Get expression for levels[0..i-1]
				getExpr := je.generateNestedMapGetExpr(rootVar, levels[:i])
				code += fmt.Sprintf("hashMapSet(%s, %s, ", getExpr, levels[i].keyExpr)
			}

			// Innermost: hashMapSet(getExpr(all levels), finalKey, [value will be emitted next]
			getExpr := je.generateNestedMapGetExpr(rootVar, levels)
			code += fmt.Sprintf("hashMapSet(%s, %s, ", getExpr, finalKey)

			je.gir.emitToFileBuffer(code, EmptyVisitMethod)
			return
		}
		// Handle slice-of-map assignment: sliceOfMaps[i][j]...["key"] = v
		// For slice[0][1]["x"] = v, generates:
		// slice.get(0).set(1, hashMapSet(slice.get(0).get(1), "x", v));
		if je.isSliceOfMapAssign {
			je.suppressMapAssignLhs = false // Reset LHS suppression
			indices := je.sliceOfMapSliceIndices
			rootVar := je.sliceOfMapSliceVar

			var code string
			if len(indices) == 1 {
				// Simple case: slice[0]["key"] = v
				code = fmt.Sprintf("%s.set(%s, hashMapSet(%s.get(%s), %s, ",
					rootVar, indices[0],
					rootVar, indices[0],
					je.sliceOfMapMapKey)
			} else {
				// Multi-level case: slice[0][1]...["key"] = v
				// First, generate the get chain for all but the last index
				getChainBeforeLast := je.generateSliceGetChain(rootVar, indices[:len(indices)-1])
				// Then, generate the full get chain for the map access
				fullGetChain := je.generateSliceGetChain(rootVar, indices)
				// Generate: getChainBeforeLast.set(lastIdx, hashMapSet(fullGetChain, key,
				lastIdx := indices[len(indices)-1]
				code = fmt.Sprintf("%s.set(%s, hashMapSet(%s, %s, ",
					getChainBeforeLast, lastIdx,
					fullGetChain,
					je.sliceOfMapMapKey)
			}
			je.gir.emitToFileBuffer(code, EmptyVisitMethod)
			return
		}
		// For slice index assignment: emit ", " between index and value
		if je.isSliceIndexAssign {
			je.gir.emitToFileBuffer(", ", EmptyVisitMethod)
			return
		}
		str := je.emitAsString(" ", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		if node.Tok == token.DEFINE {
			str = je.emitAsString("= ", 0)
		} else {
			str = je.emitAsString(node.Tok.String()+" ", 0)
		}
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		// Emit cast prefix for byte/short arithmetic
		if je.needsSmallIntCast {
			je.gir.emitToFileBuffer(fmt.Sprintf("(%s)(", je.smallIntCastType), EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	// Close byte/short arithmetic cast
	if je.needsSmallIntCast {
		je.gir.emitToFileBuffer(")", EmptyVisitMethod)
		je.needsSmallIntCast = false
		je.smallIntCastType = ""
	}
	if je.isMapAssign {
		je.gir.emitToFileBuffer(");\n", EmptyVisitMethod)
	}
	// For slice index assignment: emit closing );\n
	if je.isSliceIndexAssign {
		je.gir.emitToFileBuffer(");\n", EmptyVisitMethod)
	}
}

func (je *JavaEmitter) PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	if index > 0 && !je.isMapCommaOk && !je.isTypeAssertCommaOk && !je.isMultiReturnAssign {
		je.executeIfNotForwardDecls(func() {
			str := je.emitAsString(", ", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		})
	}
}

func (je *JavaEmitter) PreVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	if je.isMapCommaOk || je.isTypeAssertCommaOk || je.isMapAssign {
		return
	}
	je.executeIfNotForwardDecls(func() {
		if node.Tok == token.DEFINE {
			// Check if any LHS variable is already declared
			allNew := true
			for _, lhs := range node.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if ident.Name != "_" && je.isVarDeclared(ident.Name) {
						allNew = false
						break
					}
				}
			}
			if allNew {
				str := je.emitAsString("var ", 0)
				je.gir.emitToFileBuffer(str, EmptyVisitMethod)
				// Mark variables as declared
				for _, lhs := range node.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						if ident.Name != "_" {
							je.declareVar(ident.Name)
						}
					}
				}
			}
		}
	})
}

func (je *JavaEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitIndexExpr(node *ast.IndexExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isMapAssign || je.isMapCommaOk || je.isNestedMapAssign || je.isSliceOfMapAssign {
			return
		}
		// Push current map index state to stack before processing this expression
		// This allows nested index expressions to restore the outer state when done
		je.mapIndexStack = append(je.mapIndexStack, mapIndexStackEntry{
			isMapIndex:        je.isMapIndex,
			mapIndexValueType: je.mapIndexValueType,
			mapReadVarName:    je.mapReadVarName,
			mapKeyCastPrefix:  je.mapKeyCastPrefix,
			mapKeyCastSuffix:  je.mapKeyCastSuffix,
		})
		// Reset state for this expression
		je.isMapIndex = false
		je.mapIndexValueType = ""
		je.mapReadVarName = ""
		je.mapKeyCastPrefix = ""
		je.mapKeyCastSuffix = ""

		// Check if this is a map index
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			tv := je.pkg.TypesInfo.Types[node.X]
			if tv.Type != nil {
				if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
					je.isMapIndex = true
					je.mapIndexValueType = getJavaTypeName(mapType.Elem())
					je.mapKeyCastPrefix, je.mapKeyCastSuffix = getJavaKeyCast(mapType.Key())
					// For nested map access, emit ((Type)hashMapGet( BEFORE X is traversed
					// This way the X expression (which could be another IndexExpr) is emitted inside
					boxedType := toBoxedType(je.mapIndexValueType)
					je.gir.emitToFileBuffer(fmt.Sprintf("((%s)hashMapGet(", boxedType), EmptyVisitMethod)
					// Store the map variable name - but don't suppress X emission for nested cases
					je.mapReadVarName = exprToString(node.X)
					// Only suppress X if it's a simple identifier (not another IndexExpr)
					if _, isIndexExpr := node.X.(*ast.IndexExpr); !isIndexExpr {
						je.suppressMapIndexX = true
					}
				}
			}
		}
	})
}

func (je *JavaEmitter) PostVisitIndexExprX(node *ast.IndexExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isMapAssign || je.isMapCommaOk || je.isNestedMapAssign || je.isSliceOfMapAssign {
			return
		}
		if je.isUnsupportedIndexedCall {
			return
		}
		if je.isMapIndex {
			je.suppressMapIndexX = false // Reset
			je.currentMapGetValueType = je.mapIndexValueType
			// The ((Type)hashMapGet( was already emitted in PreVisitIndexExpr
			// Check if X was a nested IndexExpr (already emitted) or a simple ident (needs emitting)
			if _, isIndexExpr := node.X.(*ast.IndexExpr); isIndexExpr {
				// X was a nested IndexExpr, it has been emitted, just add comma
				je.gir.emitToFileBuffer(", ", EmptyVisitMethod)
			} else {
				// X was a simple expression, emit it then comma
				je.gir.emitToFileBuffer(je.mapReadVarName+", ", EmptyVisitMethod)
			}
			return
		}
		// Check if base is a string - use charAt instead of get
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			tv := je.pkg.TypesInfo.Types[node.X]
			if tv.Type != nil {
				if basic, ok := tv.Type.Underlying().(*types.Basic); ok {
					if basic.Kind() == types.String {
						je.emitToken(".charAt(", LeftBracket, 0)
						return
					}
				}
			}
		}
		// For slice index assignment: x[i] = v -> x.set(i, v)
		// Only the actual LHS index expression should use .set(), nested indices use .get()
		if je.isSliceIndexAssign && node == je.sliceIndexLhsExpr {
			je.emitToken(".set(", LeftBracket, 0)
			return
		}
		je.emitToken(".get(", LeftBracket, 0)
	})
}

func (je *JavaEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isMapAssign || je.isMapCommaOk || je.isNestedMapAssign || je.isSliceOfMapAssign {
			return
		}
		// Reset map read state
		je.suppressMapIndexX = false
		je.mapReadVarName = ""
	})
}

func (je *JavaEmitter) PreVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isMapAssign || je.isMapCommaOk || je.isNestedMapAssign || je.isSliceOfMapAssign {
			return
		}
		if je.isUnsupportedIndexedCall {
			return
		}
		if je.isMapIndex {
			// Map variable already emitted in PostVisitIndexExprX, just emit key cast if needed
			if je.mapKeyCastPrefix != "" {
				je.gir.emitToFileBuffer(je.mapKeyCastPrefix, EmptyVisitMethod)
			}
		}
	})
}

func (je *JavaEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isMapAssign || je.isMapCommaOk || je.isNestedMapAssign || je.isSliceOfMapAssign {
			return
		}
		if je.isUnsupportedIndexedCall {
			return
		}
		if je.isMapIndex {
			if je.mapKeyCastSuffix != "" {
				je.gir.emitToFileBuffer(je.mapKeyCastSuffix, EmptyVisitMethod)
			}
			// Close both hashMapGet() and the cast wrapper
			je.gir.emitToFileBuffer("))", EmptyVisitMethod)
		} else {
			// For slice index assignment, don't emit ) here - it will be emitted after the value
			// Only suppress for the actual LHS index expression
			if je.isSliceIndexAssign && node == je.sliceIndexLhsExpr {
				// Don't pop stack here - let the outer handler deal with it
			} else {
				je.emitToken(")", RightBracket, 0)
			}
		}

		// Pop and restore previous map index state from stack
		if len(je.mapIndexStack) > 0 {
			prevState := je.mapIndexStack[len(je.mapIndexStack)-1]
			je.mapIndexStack = je.mapIndexStack[:len(je.mapIndexStack)-1]
			je.isMapIndex = prevState.isMapIndex
			je.mapIndexValueType = prevState.mapIndexValueType
			je.mapReadVarName = prevState.mapReadVarName
			je.mapKeyCastPrefix = prevState.mapKeyCastPrefix
			je.mapKeyCastSuffix = prevState.mapKeyCastSuffix
		}
		je.currentMapGetValueType = ""
	})
}

func (je *JavaEmitter) PreVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitBinaryExprOperator(op token.Token, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(" "+op.String()+" ", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitCallExprArg(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isUnsupportedIndexedCall {
			return
		}
		if je.isMultiReturnAssign {
			return
		}
		if je.isSliceMakeCall {
			if index == 0 {
				// Skip the type argument - mark that we're in the type arg
				je.inTypeContext = true
				return
			}
			if index == 1 {
				// Use MakeSlice helper to create ArrayList with actual elements
				if je.sliceMakeElemType == "boolean" || je.sliceMakeElemType == "Boolean" {
					je.gir.emitToFileBuffer("SliceBuiltins.MakeBoolSlice(", EmptyVisitMethod)
				} else {
					je.gir.emitToFileBuffer("SliceBuiltins.<"+toBoxedType(je.sliceMakeElemType)+">MakeSlice(", EmptyVisitMethod)
				}
				return
			}
		}
		if index > 0 {
			str := je.emitAsString(", ", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isSliceMakeCall && index == 0 {
			// Clear type context after first argument
			je.inTypeContext = false
		}
	})
}

func (je *JavaEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(";", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// If there's an Init, don't emit "if " here; let Init be a separate statement
		if node.Init != nil {
			je.hasIfInit = true
			je.ifInitIndent = indent
			return
		}
		je.hasIfInit = false
		str := je.emitAsString("if ", indent)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitIfStmtInit(node ast.Stmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Init will be emitted as a regular statement
	})
}

func (je *JavaEmitter) PostVisitIfStmtInit(node ast.Stmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// After init, emit newline - the statement already has its own semicolon
		je.gir.emitToFileBuffer("\n", EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitIfStmtCond(node *ast.IfStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// If there was an Init, emit "if " now with proper indentation
		if je.hasIfInit {
			str := je.emitAsString("if (", je.ifInitIndent)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
			je.hasIfInit = false // Reset flag after use
		} else {
			str := je.emitAsString("(", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(") ", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Push a scope for for-loop init variables (these are scoped to the for statement)
		je.declaredVarsStack = append(je.declaredVarsStack, make(map[string]bool))

		je.isInfiniteLoop = node.Init == nil && node.Cond == nil && node.Post == nil
		if je.isInfiniteLoop {
			str := je.emitAsString("while (true) ", indent)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		} else {
			str := je.emitAsString("for (", indent)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PreVisitForStmtInit(node ast.Stmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.insideForInit = true
	})
}

func (je *JavaEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.insideForInit = false
		if je.isInfiniteLoop {
			return
		}
		str := je.emitAsString("; ", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitForStmtPost(node ast.Stmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.insideForPostCond = true
	})
}

func (je *JavaEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.insideForPostCond = false
		if je.isInfiniteLoop {
			return
		}
		str := je.emitAsString(") ", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitIfStmtElse(node *ast.IfStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(" else ", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isInfiniteLoop {
			return
		}
		str := je.emitAsString("; ", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.isInfiniteLoop = false
		// Pop the for-loop scope
		if len(je.declaredVarsStack) > 0 {
			je.declaredVarsStack = je.declaredVarsStack[:len(je.declaredVarsStack)-1]
		}
	})
}

func (je *JavaEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Push a scope for range loop variables
		je.declaredVarsStack = append(je.declaredVarsStack, make(map[string]bool))

		je.rangeStmtIndent = indent
		je.isKeyValueRange = false
		je.rangeKeyName = ""
		je.rangeValueName = ""
		je.rangeCollectionExpr = ""

		// Determine if key-value or index-only range
		// Handle key (index)
		if node.Key != nil {
			if keyIdent, ok := node.Key.(*ast.Ident); ok {
				if keyIdent.Name != "_" {
					je.rangeKeyName = keyIdent.Name
				}
			}
		}
		// Handle value
		if node.Value != nil {
			if valIdent, ok := node.Value.(*ast.Ident); ok {
				if valIdent.Name != "_" {
					je.rangeValueName = valIdent.Name
					je.isKeyValueRange = true
				}
			}
		}

		str := je.emitAsString("for (", indent)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitRangeStmtKey(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressRangeEmit = true
	})
}

func (je *JavaEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitRangeStmtValue(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressRangeEmit = false
	})
}

func (je *JavaEmitter) PreVisitRangeStmtX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.captureRangeExpr = true
		je.rangeCollectionExpr = ""
	})
}

func (je *JavaEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.captureRangeExpr = false

		if je.isKeyValueRange && je.rangeKeyName != "" {
			// For key-value range: for (int key = 0; key < collection.size(); key++)
			je.gir.emitToFileBuffer(fmt.Sprintf("int %s = 0; %s < %s.size(); %s++) ",
				je.rangeKeyName, je.rangeKeyName, je.rangeCollectionExpr, je.rangeKeyName), EmptyVisitMethod)
			je.pendingRangeValueDecl = true
			je.pendingValueName = je.rangeValueName
			je.pendingCollectionExpr = je.rangeCollectionExpr
			je.pendingKeyName = je.rangeKeyName
		} else if je.rangeKeyName != "" && je.rangeValueName == "" {
			// Index-only range: for (int i = 0; i < collection.size(); i++)
			je.gir.emitToFileBuffer(fmt.Sprintf("int %s = 0; %s < %s.size(); %s++) ",
				je.rangeKeyName, je.rangeKeyName, je.rangeCollectionExpr, je.rangeKeyName), EmptyVisitMethod)
		} else if je.rangeValueName != "" {
			// Value-only range (key is "_" or not used): for (var val : collection)
			je.gir.emitToFileBuffer(fmt.Sprintf("var %s : %s) ", je.rangeValueName, je.rangeCollectionExpr), EmptyVisitMethod)
		} else {
			// Neither key nor value specified: for (var __item : collection)
			je.gir.emitToFileBuffer(fmt.Sprintf("var __item : %s) ", je.rangeCollectionExpr), EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.isKeyValueRange = false
		// Pop the range loop scope
		if len(je.declaredVarsStack) > 0 {
			je.declaredVarsStack = je.declaredVarsStack[:len(je.declaredVarsStack)-1]
		}
	})
}

func (je *JavaEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		if !je.insideForPostCond {
			str := je.emitAsString(node.Tok.String(), 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
			str = je.emitAsString(";", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		} else {
			str := je.emitAsString(node.Tok.String(), 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PreVisitCompositeLitType(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("new ", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isArray {
			str := je.emitAsString("(Arrays.asList", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		} else {
			str := je.emitAsString("(", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PreVisitCompositeLitElts(node []ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isArray {
			str := je.emitAsString("(", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isArray {
			str := je.emitAsString("))", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
			je.isArray = false
			je.sliceLitElemCast = "" // Reset after processing slice literal
		} else {
			str := je.emitAsString(")", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PreVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if index > 0 {
			str := je.emitAsString(", ", 0)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
		// Emit cast prefix for byte/short slice literal elements
		if je.sliceLitElemCast != "" && je.isArray {
			// Only apply cast to BasicLit (numeric literals)
			if _, ok := node.(*ast.BasicLit); ok {
				je.gir.emitToFileBuffer(je.sliceLitElemCast, EmptyVisitMethod)
			}
		}
	})
}

func (je *JavaEmitter) PreVisitSliceExprX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Capture the X expression name for later use (for .size() call when High is nil)
		je.sliceExprXName = exprToString(node)
	})
}

func (je *JavaEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(".subList(", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitSliceExprXBegin(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress the second X traversal
		je.suppressRangeEmit = true
	})
}

func (je *JavaEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressRangeEmit = false
	})
}

func (je *JavaEmitter) PreVisitSliceExprLow(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// If Low is nil (like a[:high]), emit 0
		if node == nil {
			je.gir.emitToFileBuffer("0", EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(", ", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitSliceExprXEnd(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress the third X traversal
		je.suppressRangeEmit = true
	})
}

func (je *JavaEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressRangeEmit = false
	})
}

func (je *JavaEmitter) PreVisitSliceExprHigh(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// If High is nil (like a[low:]), emit X.size()
		if node == nil {
			je.gir.emitToFileBuffer(je.sliceExprXName+".size()", EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(")", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		je.sliceExprXName = "" // Reset
	})
}

func (je *JavaEmitter) PreVisitFuncLit(node *ast.FuncLit, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inLambdaParams = true
		je.emitToken("(", LeftParen, 0)
	})
}

func (je *JavaEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inLambdaParams = false
		je.emitToken(") -> ", RightParen, 0)
	})
}

func (je *JavaEmitter) PreVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if index > 0 {
			je.emitToken(", ", Comma, 0)
		}
		// Emit parameter name only for lambda (Java uses type inference)
		if len(node.Names) > 0 {
			je.gir.emitToFileBuffer(node.Names[0].Name, EmptyVisitMethod)
		}
		// Set type context to suppress type emission
		je.inTypeContext = true
	})
}

func (je *JavaEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inTypeContext = false
	})
}

func (je *JavaEmitter) PreVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitFuncLitTypeResult(node *ast.Field, index int, indent int) {
	// Lambda return types are inferred in Java
}

func (je *JavaEmitter) PostVisitFuncLitTypeResult(node *ast.Field, index int, indent int) {
}

func (je *JavaEmitter) PreVisitInterfaceType(node *ast.InterfaceType, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Skip if we're inside a slice make call (the type is handled separately)
		if je.isSliceMakeCall {
			return
		}
		str := je.emitAsString("Object", indent)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitKeyValueExprKey(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress struct field key - Java uses positional constructor args
		je.suppressKeyValueKey = true
	})
}

func (je *JavaEmitter) PostVisitKeyValueExprKey(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressKeyValueKey = false
	})
}

func (je *JavaEmitter) PreVisitKeyValueExprValue(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(node.Op.String(), 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitParenExpr(node *ast.ParenExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.gir.emitToFileBuffer("(", EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.gir.emitToFileBuffer(")", EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitGenDeclConstName(node *ast.Ident, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("public static final ", indent+2)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)

		// Try to infer type from value
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			if obj := je.pkg.TypesInfo.Defs[node]; obj != nil {
				typeName := getJavaTypeName(obj.Type())
				je.gir.emitToFileBuffer(typeName+" ", EmptyVisitMethod)
			}
		}
		str = je.emitAsString(node.Name+" = ", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.gir.emitToFileBuffer(";\n", EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("switch ", indent)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		je.emitToken("(", LeftParen, 0)
	})
}

func (je *JavaEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("}\n", indent)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.emitToken(")", RightParen, 0)
		str := je.emitAsString(" {\n", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Add break after each case unless it's a fallthrough
		str := je.emitAsString("break;\n", indent+2)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if len(node) == 0 {
			str := je.emitAsString("default:\n", indent)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PreVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if index == 0 {
			str := je.emitAsString("case ", indent)
			je.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (je *JavaEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(":\n", 0)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (je *JavaEmitter) PreVisitTypeAssertExprType(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isTypeAssertCommaOk {
			return
		}
		je.emitToken("(", LeftParen, 0)
	})
}

func (je *JavaEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isTypeAssertCommaOk {
			return
		}
		je.emitToken(")", RightParen, 0)
	})
}

func (je *JavaEmitter) PreVisitTypeAssertExprX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(strings.ToLower(node.Tok.String())+";", indent)
		je.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

// Build file generation

func (je *JavaEmitter) GeneratePomXml() error {
	if je.LinkRuntime == "" {
		return nil
	}

	pomPath := filepath.Join(je.OutputDir, "pom.xml")
	file, err := os.Create(pomPath)
	if err != nil {
		return fmt.Errorf("failed to create pom.xml: %w", err)
	}
	defer file.Close()

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
                            <mainClass>Api</mainClass>
                        </manifest>
                    </archive>
                </configuration>
            </plugin>
            <plugin>
                <groupId>org.codehaus.mojo</groupId>
                <artifactId>exec-maven-plugin</artifactId>
                <version>3.1.0</version>
                <configuration>
                    <mainClass>Api</mainClass>
                </configuration>
            </plugin>
        </plugins>
    </build>
</project>
`, je.OutputName)

	_, err = file.WriteString(pom)
	if err != nil {
		return fmt.Errorf("failed to write pom.xml: %w", err)
	}

	DebugLogPrintf("Generated pom.xml at %s", pomPath)
	return nil
}

func (je *JavaEmitter) CopyRuntimePackages() error {
	if je.LinkRuntime == "" {
		return nil
	}

	for name, variant := range je.RuntimePackages {
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

		runtimeSrcPath := filepath.Join(je.LinkRuntime, name, "java", srcFileName)
		content, err := os.ReadFile(runtimeSrcPath)
		if err != nil {
			DebugLogPrintf("Skipping Java runtime for %s: %v", name, err)
			continue
		}

		dstFileName := capName + "Runtime.java"
		dstPath := filepath.Join(je.OutputDir, dstFileName)
		if err := os.WriteFile(dstPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", dstFileName, err)
		}
		DebugLogPrintf("Copied %s from %s to %s", dstFileName, runtimeSrcPath, dstPath)
	}

	return nil
}
