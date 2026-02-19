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
	"strings"

	goanyrt "goany/runtime"

	"golang.org/x/tools/go/packages"
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

// keyCastState holds key cast values for nested map reads
type keyCastState struct {
	prefix    string
	suffix    string
	valueType string
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

type CPPEmitter struct {
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string            // Path to runtime directory (empty = disabled)
	RuntimePackages map[string]string // Detected runtime packages with variant (e.g. "graphics":"tigr", "http":"")
	OptimizeMoves   bool            // Enable move optimizations to reduce struct cloning
	MoveOptCount    int    // Count of copies replaced by std::move()
	file            *os.File
	Emitter
	pkg               *packages.Package
	insideForPostCond bool
	assignmentToken   string
	forwardDecl       bool
	// Key-value range loop support
	isKeyValueRange       bool
	rangeKeyName          string
	rangeValueName        string
	rangeCollectionExpr   string
	captureRangeExpr      bool
	suppressRangeEmit     bool
	rangeStmtIndent       int
	pendingRangeValueDecl bool // True when we need to emit value decl at start of block
	pendingValueName      string
	pendingCollectionExpr string
	pendingKeyName        string
	// Move optimization: add std::move() for struct arguments
	currentAssignLhsNames     map[string]bool  // LHS variable names of current assignment
	currentCallArgIdentsStack []map[string]int // Stack of identifier counts per nested call
	moveCurrentArg            bool             // Flag set in Pre, read in Post for std::move wrapping
	funcLitDepth              int              // Nesting depth of function literals (closures)
	currentReturnNode         *ast.ReturnStmt  // Current return statement being processed
	moveCurrentReturn         bool             // Flag for std::move wrapping in return
	// Map support
	isMapMakeCall          bool   // Inside make(map[K]V) call
	mapMakeKeyType         int    // Key type constant for make call
	isMapIndex             bool   // IndexExpr on a map (for read: m[k])
	mapIndexValueCppType   string // C++ type name for std::any_cast on hashMapGet result
	mapIndexKeyCastPrefix  string // Cast prefix for map key (e.g. "std::string(" or "(std::int64_t)(")
	mapIndexKeyCastSuffix  string // Cast suffix for map key (e.g. ")")
	isMapAssign            bool   // Assignment to map index (m[k] = v)
	mapAssignVarName       string // Variable name for map assignment
	mapAssignIndent        int    // Indent level for map assignment
	mapAssignKeyCastPrefix string // Cast prefix for map assign key
	mapAssignKeyCastSuffix string // Cast suffix for map assign key
	mapAssignValueIsString bool   // Value is string type
	// Nested map assignment: m["outer"]["inner"] = v
	isNestedMapAssign           bool   // Assignment to nested map index
	nestedMapOuterVarName       string // Outer map variable name (e.g., "m")
	nestedMapOuterKey           string // Outer map key expression (captured)
	nestedMapOuterKeyCastPrefix string // Cast prefix for outer key
	nestedMapOuterKeyCastSuffix string // Cast suffix for outer key
	captureOuterMapKey          bool   // Capturing outer map key expression
	capturedOuterMapKey         string // Captured outer key expression text
	nestedMapCounter            int    // Counter for unique nested map variable names
	nestedMapVarName            string // Generated variable name (e.g., "__nested_inner_0")
	nestedMapValueCppType       string // C++ type for the nested value extraction (e.g., "hmap::HashMap" or "std::vector<hmap::HashMap>")
	// Nested map read: m["outer"]["inner"] (read context)
	nestedMapReadDepth          int    // Depth of nested map reads (0 = not nested, 1+ = nested)
	nestedMapReadOuterKeyType   types.Type // Key type of the outer map for casting
	mapIndexKeyCastStack        []keyCastState // Stack to save/restore key cast values for nested reads
	// Mixed nested composites: []map[string]int, map[string][]int, etc.
	indexAccessTypeStack        []string // Stack of access types: "map" or "slice" for each IndexExpr level
	// Save/restore stack for suppressEmit during nested type traversal
	suppressEmitStack           []bool
	// General mixed index assignment: any combination of map[k] and slice[i] on LHS
	isMixedIndexAssign          bool              // Assignment involves map access below outermost level
	mixedIndexAssignOps         []mixedIndexOp    // Chain of operations from root to outermost
	mixedAssignIndent           int               // Indent level
	mixedAssignFinalTarget      string            // Final assignment target expression (e.g., "__temp[0]" or "__temp")
	captureMapKey          bool   // Capturing map key expression
	capturedMapKey         string // Captured key expression text
	isDeleteCall           bool   // Inside delete(m, k) call
	deleteMapVarName       string // Variable name for delete
	deleteKeyCastPrefix    string // Cast prefix for delete key
	deleteKeyCastSuffix    string // Cast suffix for delete key
	isMapLenCall           bool   // len() call on a map
	pendingMapInit         bool   // var m map[K]V needs default init
	pendingMapKeyType      int    // Key type for pending init
	structKeyTypes         map[string]string // Struct types used as map keys: name -> qualified C++ name (e.g., "::Point" or "::pkgname::MyStruct")
	pendingHashSpecs       []pendingHashSpec // Deferred hash specializations to emit after namespace close
	hasDeclValue           bool   // True if current declaration has an explicit initialization value
	suppressEmit           bool   // When true, emitToFile does nothing
	isSliceMakeCall        bool   // Inside make([]T, n) call
	sliceMakeCppType       string // C++ vector type, e.g. "std::vector<std::any>"
	assignLhsIsInterface   bool   // LHS of assignment is interface{}/std::any type
	// Comma-ok idiom: val, ok := m[key]
	isMapCommaOk          bool
	mapCommaOkValName     string
	mapCommaOkOkName      string
	mapCommaOkMapName     string
	mapCommaOkValueType   string
	mapCommaOkIsDecl      bool
	mapCommaOkIndent      int
	mapCommaOkKeyCastPrefix string
	mapCommaOkKeyCastSuffix string
	// Type assertion comma-ok: val, ok := x.(Type)
	isTypeAssertCommaOk      bool
	typeAssertCommaOkValName string
	typeAssertCommaOkOkName  string
	typeAssertCommaOkType    string
	typeAssertCommaOkIsDecl  bool
	typeAssertCommaOkIndent  int
}

// collectCallArgIdentCounts counts how many times each base identifier
// appears across all arguments of a function call
func (cppe *CPPEmitter) collectCallArgIdentCounts(args []ast.Expr) map[string]int {
	counts := make(map[string]int)
	for _, arg := range args {
		cppe.countIdentsInExpr(arg, counts)
	}
	return counts
}

// countIdentsInExpr recursively counts identifier occurrences in an expression
func (cppe *CPPEmitter) countIdentsInExpr(expr ast.Expr, counts map[string]int) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Ident:
		if cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
			if obj := cppe.pkg.TypesInfo.ObjectOf(e); obj != nil {
				if _, isTypeName := obj.(*types.TypeName); isTypeName {
					return
				}
				if _, isPkgName := obj.(*types.PkgName); isPkgName {
					return
				}
			}
		}
		counts[e.Name]++
	case *ast.SelectorExpr:
		cppe.countIdentsInExpr(e.X, counts)
	case *ast.CallExpr:
		for _, arg := range e.Args {
			cppe.countIdentsInExpr(arg, counts)
		}
		cppe.countIdentsInExpr(e.Fun, counts)
	case *ast.IndexExpr:
		cppe.countIdentsInExpr(e.X, counts)
		cppe.countIdentsInExpr(e.Index, counts)
	case *ast.BinaryExpr:
		cppe.countIdentsInExpr(e.X, counts)
		cppe.countIdentsInExpr(e.Y, counts)
	case *ast.UnaryExpr:
		cppe.countIdentsInExpr(e.X, counts)
	case *ast.ParenExpr:
		cppe.countIdentsInExpr(e.X, counts)
	case *ast.TypeAssertExpr:
		cppe.countIdentsInExpr(e.X, counts)
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			cppe.countIdentsInExpr(elt, counts)
		}
	case *ast.KeyValueExpr:
		cppe.countIdentsInExpr(e.Value, counts)
	}
}

// exprContainsIdent checks if an expression references a given identifier
func (cppe *CPPEmitter) exprContainsIdent(expr ast.Expr, name string) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == name
	case *ast.SelectorExpr:
		return cppe.exprContainsIdent(e.X, name)
	case *ast.CallExpr:
		for _, arg := range e.Args {
			if cppe.exprContainsIdent(arg, name) {
				return true
			}
		}
		return cppe.exprContainsIdent(e.Fun, name)
	case *ast.IndexExpr:
		return cppe.exprContainsIdent(e.X, name) || cppe.exprContainsIdent(e.Index, name)
	case *ast.BinaryExpr:
		return cppe.exprContainsIdent(e.X, name) || cppe.exprContainsIdent(e.Y, name)
	case *ast.UnaryExpr:
		return cppe.exprContainsIdent(e.X, name)
	case *ast.ParenExpr:
		return cppe.exprContainsIdent(e.X, name)
	case *ast.TypeAssertExpr:
		return cppe.exprContainsIdent(e.X, name)
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			if cppe.exprContainsIdent(elt, name) {
				return true
			}
		}
	case *ast.KeyValueExpr:
		return cppe.exprContainsIdent(e.Value, name)
	}
	return false
}

// canMoveArg checks if a call argument identifier can be moved instead of copied.
func (cppe *CPPEmitter) canMoveArg(varName string) bool {
	if !cppe.OptimizeMoves {
		return false
	}
	if cppe.funcLitDepth > 0 {
		return false
	}
	if cppe.currentAssignLhsNames == nil || !cppe.currentAssignLhsNames[varName] {
		return false
	}
	if len(cppe.currentCallArgIdentsStack) > 0 {
		outermostCounts := cppe.currentCallArgIdentsStack[0]
		if outermostCounts[varName] > 1 {
			return false
		}
	}
	cppe.MoveOptCount++
	return true
}

func (*CPPEmitter) lowerToBuiltins(selector string) string {
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
		return "std::size"
	case "panic":
		return "goany_panic"
	}
	return selector
}

// getMapKeyTypeConst returns the key type constant for a map type AST node
func (cppe *CPPEmitter) getMapKeyTypeConst(mapType *ast.MapType) int {
	if cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
		if tv, ok := cppe.pkg.TypesInfo.Types[mapType.Key]; ok && tv.Type != nil {
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
			// Check for struct type - use KeyTypeStruct (100)
			if _, isStruct := tv.Type.Underlying().(*types.Struct); isStruct {
				// Track this struct type for hash/equality generation
				if named, ok := tv.Type.(*types.Named); ok {
					if cppe.structKeyTypes == nil {
						cppe.structKeyTypes = make(map[string]string)
					}
					structName := named.Obj().Name()
					// Build C++ name - for structs in main package use just the name
					// For structs in other packages, use pkgname::StructName
					// Use the package where the struct is DEFINED, not the current package
					qualifiedName := structName
					if structPkg := named.Obj().Pkg(); structPkg != nil {
						pkgName := structPkg.Name()
						if pkgName != "" && pkgName != "main" {
							qualifiedName = pkgName + "::" + structName
						}
					}
					cppe.structKeyTypes[structName] = qualifiedName
				}
				return 100
			}
		}
	}
	return 1
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
			return named.Obj().Name()
		}
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

// analyzeLhsIndexChain walks an IndexExpr chain and returns operations from root to outermost.
// Returns true if any intermediate map access was found (requiring temp var extraction).
func (cppe *CPPEmitter) analyzeLhsIndexChain(expr ast.Expr) (ops []mixedIndexOp, hasIntermediateMap bool) {
	// Collect the chain from outermost to root
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
	// chain[0] is outermost, chain[len-1] is innermost (closest to root variable)
	// Reverse to get root to outermost order
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	// Now chain[0] is innermost (root[k1]), chain[len-1] is outermost ([kN])
	hasIntermediateMap = false
	for i, node := range chain {
		ie := node.(*ast.IndexExpr)
		tv := cppe.pkg.TypesInfo.Types[ie.X]
		if tv.Type == nil {
			continue
		}
		isLast := (i == len(chain)-1)
		if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
			op := mixedIndexOp{
				accessType:   "map",
				keyExpr:      exprToCppString(ie.Index),
				valueCppType: getCppTypeName(mapType.Elem()),
			}
			op.keyCastPfx, op.keyCastSfx = getCppKeyCast(mapType.Key())
			if !isLast {
				hasIntermediateMap = true
			}
			ops = append(ops, op)
		} else {
			op := mixedIndexOp{
				accessType: "slice",
				keyExpr:    exprToCppString(ie.Index),
			}
			ops = append(ops, op)
		}
	}
	return ops, hasIntermediateMap
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

func (cppe *CPPEmitter) emitToFile(s string) error {
	// When capturing range expression, append to buffer instead of file
	if cppe.captureRangeExpr {
		cppe.rangeCollectionExpr += s
		return nil
	}
	if cppe.captureMapKey {
		cppe.capturedMapKey += s
		return nil
	}
	if cppe.suppressEmit {
		return nil
	}
	// When suppressing range emit (key/value identifiers), skip
	if cppe.suppressRangeEmit {
		return nil
	}
	return emitToFile(cppe.file, s)
}

func (cppe *CPPEmitter) emitAsString(s string, indent int) string {
	return strings.Repeat(" ", indent) + s
}

func (cppe *CPPEmitter) SetFile(file *os.File) {
	cppe.file = file
}

func (cppe *CPPEmitter) GetFile() *os.File {
	return cppe.file
}

func (cppe *CPPEmitter) PreVisitProgram(indent int) {
	outputFile := cppe.Output
	var err error
	cppe.file, err = os.Create(outputFile)
	cppe.SetFile(cppe.file)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	_, err = cppe.file.WriteString("#include <vector>\n" +
		"#include <string>\n" +
		"#include <tuple>\n" +
		"#include <any>\n" +
		"#include <cstdint>\n" +
		"#include <functional>\n")
	cppe.file.WriteString(`#include <cstdarg> // For va_start, etc.
#include <initializer_list>
#include <iostream>
`)
	// Include runtime headers if link-runtime is enabled
	// Convention: X/cpp/X_runtime.hpp (no variant) or X/cpp/X_runtime_<variant>.hpp (with variant)
	if cppe.LinkRuntime != "" {
		for name, variant := range cppe.RuntimePackages {
			if variant == "none" {
				continue
			}
			fileName := name + "_runtime"
			if variant != "" {
				fileName += "_" + variant
			}
			cppe.file.WriteString(fmt.Sprintf("#include \"%s/cpp/%s.hpp\"\n", name, fileName))
		}
	}
	// Include panic runtime
	cppe.file.WriteString("\n// GoAny panic runtime\n")
	cppe.file.WriteString(goanyrt.PanicCppSource)
	cppe.file.WriteString("\n")

	cppe.file.WriteString(`
using int8 = int8_t;
using int16 = int16_t;
using int32 = int32_t;
using int64 = int64_t;
using uint8 = uint8_t;
using uint16 = uint16_t;
using uint32 = uint32_t;
using uint64 = uint64_t;
using float32 = float;
using float64 = double;

std::string string_format(const std::string fmt, ...) {
  int size =
      ((int)fmt.size()) * 2 + 50; // Use a rubric appropriate for your code
  std::string str;
  va_list ap;
  while (1) { // Maximum two passes on a POSIX system...
    str.resize(size);
    va_start(ap, fmt);
    int n = vsnprintf((char *)str.data(), size, fmt.c_str(), ap);
    va_end(ap);
    if (n > -1 && n < size) { // Everything worked
      str.resize(n);
      return str;
    }
    if (n > -1)     // Needed size returned
      size = n + 1; // For null char
    else
      size *= 2; // Guess at a larger size (OS specific)
  }
  return str;
}

void println() { printf("\n"); }

void println(std::int8_t val) { printf("%d\n", val); }

template<typename T>
void println(const T& val) { std::cout << val << std::endl;}

void printf(const signed char c) {
  std::cout << (int)c;
}

template<typename T>
void printf(const T& val) { std::cout << val;}

// Function to mimic Go's append behavior for std::vector
template <typename T>
std::vector<T>& append(std::vector<T> &vec,
                      const std::initializer_list<T> &elements) {
  std::vector<T>& result = vec;           // Create a copy of the original vector
  result.insert(result.end(), elements); // Append the elements
  return result;                         // Return the new vector
}

// Overload to allow appending another vector
template <typename T>
std::vector<T>& append( std::vector<T> &vec,
                      const std::vector<T> &elements) {
  std::vector<T> result = vec; // Create a copy of the original vector
  result.insert(result.end(), elements.begin(),
                elements.end()); // Append the elements
  return result;                 // Return the new vector
}

template <typename T>
std::vector<T>& append(std::vector<T> &vec, const T &element) {
  std::vector<T>& result = vec; // Create a copy of the original vector
  result.push_back(element);   // Append the single element
  return result;               // Return the new vector
}

// Specialization for appending const char* to vector of strings
std::vector<std::string>& append(std::vector<std::string> &vec, const char *element) {
  std::vector<std::string>& result = vec;
  result.push_back(std::string(element));
  return result;
}
`)
	cppe.file.WriteString("\n\n")
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}
	cppe.insideForPostCond = false
}

func (cppe *CPPEmitter) PostVisitProgram(indent int) {
	// Emit pending hash specializations for main package (no namespace)
	cppe.emitPendingHashSpecs("")

	cppe.file.Close()

	// Replace placeholder struct key functions with working implementations
	if len(cppe.structKeyTypes) > 0 {
		cppe.replaceStructKeyFunctions()
	}

	// Generate Makefile if link-runtime is enabled
	if err := cppe.GenerateMakefile(); err != nil {
		log.Printf("Warning: %v", err)
	}

	if cppe.OptimizeMoves && cppe.MoveOptCount > 0 {
		fmt.Printf("  C++: %d copy(ies) replaced by std::move()\n", cppe.MoveOptCount)
	}
}

// replaceStructKeyFunctions replaces placeholder hash/equality functions with struct-specific implementations
func (cppe *CPPEmitter) replaceStructKeyFunctions() {
	outputPath := cppe.OutputDir + "/" + cppe.OutputName + ".cpp"
	content, err := os.ReadFile(outputPath)
	if err != nil {
		log.Printf("Warning: could not read file for struct key replacement: %v", err)
		return
	}

	// Generate struct-specific hash function
	// Use qualified names (e.g., "::Point") to reference structs from within hmap namespace
	var hashCode strings.Builder
	hashCode.WriteString("int hashStructKey(std::any key)\n{\n")
	for _, qualifiedName := range cppe.structKeyTypes {
		hashCode.WriteString(fmt.Sprintf("    if (key.type() == typeid(%s)) {\n", qualifiedName))
		hashCode.WriteString(fmt.Sprintf("        int h = static_cast<int>(std::hash<%s>{}(std::any_cast<%s>(key)));\n", qualifiedName, qualifiedName))
		hashCode.WriteString("        if (h < 0) h = -h;\n")
		hashCode.WriteString("        return h;\n")
		hashCode.WriteString("    }\n")
	}
	hashCode.WriteString("    return 0;\n}")

	// Generate struct-specific equality function
	var eqCode strings.Builder
	eqCode.WriteString("bool structKeysEqual(std::any a, std::any b)\n{\n")
	eqCode.WriteString("    if (a.type() != b.type()) return false;\n")
	for _, qualifiedName := range cppe.structKeyTypes {
		eqCode.WriteString(fmt.Sprintf("    if (a.type() == typeid(%s)) {\n", qualifiedName))
		eqCode.WriteString(fmt.Sprintf("        return std::any_cast<%s>(a) == std::any_cast<%s>(b);\n", qualifiedName, qualifiedName))
		eqCode.WriteString("    }\n")
	}
	eqCode.WriteString("    return false;\n}")

	newContent := string(content)

	// Replace placeholder hashStructKey with a version that calls the real implementation defined later
	hashPattern := regexp.MustCompile(`(?s)int hashStructKey\(std::any key\)\s*\{\s*return 0;\s*\}`)
	newContent = hashPattern.ReplaceAllString(newContent, "int hashStructKey(std::any key);\n")

	// Replace placeholder structKeysEqual with a forward declaration
	eqPattern := regexp.MustCompile(`(?s)bool structKeysEqual\(std::any a, std::any b\)\s*\{\s*return false;\s*\}`)
	newContent = eqPattern.ReplaceAllString(newContent, "bool structKeysEqual(std::any a, std::any b);\n")

	// Append the real implementations at the end of the file (after all struct definitions)
	newContent += "\n// Struct map key support - implementations after struct definitions\n"
	newContent += "namespace hmap {\n"
	newContent += hashCode.String() + "\n"
	newContent += eqCode.String() + "\n"
	newContent += "} // namespace hmap\n"

	if err := os.WriteFile(outputPath, []byte(newContent), 0644); err != nil {
		log.Printf("Warning: could not write struct key replacements: %v", err)
	}
}

func (cppe *CPPEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	cppe.pkg = pkg
	name := pkg.Name
	if cppe.structKeyTypes == nil {
		cppe.structKeyTypes = make(map[string]string)
	}
	DebugLogPrintf("CPPEmitter: PreVisitPackage: %s (path: %s)", name, pkg.PkgPath)
	if name == "main" {
		return
	}
	str := cppe.emitAsString(fmt.Sprintf("namespace %s\n", name), 0)
	err := cppe.emitToFile(str)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}
	err = cppe.emitToFile("{\n\n")
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}
}

func (cppe *CPPEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	name := pkg.Name
	if name == "main" {
		return
	}
	str := cppe.emitAsString(fmt.Sprintf("} // namespace %s\n\n", name), 0)
	err := cppe.emitToFile(str)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}

	// Emit pending hash specializations for this package (now that namespace is closed)
	cppe.emitPendingHashSpecs(name)
}

func (cppe *CPPEmitter) PreVisitBasicLit(e *ast.BasicLit, indent int) {
	if e.Kind == token.STRING {
		// Wrap string literals in std::string() when assigned to interface{}/std::any
		// to avoid const char* being stored in std::any instead of std::string
		wrapStr := cppe.assignLhsIsInterface
		if wrapStr {
			cppe.emitToFile("std::string(")
		}
		// Use a local copy to avoid mutating the AST (which affects other emitters)
		value := e.Value
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			// Remove only the outer quotes, keep escaped content intact
			value = value[1 : len(value)-1]
			cppe.emitToFile(cppe.emitAsString(fmt.Sprintf("\"%s\"", value), 0))
		} else if len(value) >= 2 && value[0] == '`' && value[len(value)-1] == '`' {
			// Raw string literal - use C++ raw string
			value = value[1 : len(value)-1]
			cppe.emitToFile(cppe.emitAsString(fmt.Sprintf("R\"(%s)\"", value), 0))
		} else {
			cppe.emitToFile(cppe.emitAsString(fmt.Sprintf("\"%s\"", value), 0))
		}
		if wrapStr {
			cppe.emitToFile(")")
		}
	} else {
		cppe.emitToFile(cppe.emitAsString(e.Value, 0))
	}
}

func (cppe *CPPEmitter) PreVisitIdent(e *ast.Ident, indent int) {
	var str string
	name := e.Name
	name = cppe.lowerToBuiltins(name)
	// Handle map operation identifier replacements
	if cppe.isMapMakeCall && name == "make" {
		cppe.emitToFile("hmap::newHashMap")
		return
	}
	if cppe.isSliceMakeCall && name == "make" {
		cppe.emitToFile(cppe.sliceMakeCppType)
		return
	}
	if cppe.isDeleteCall && name == "delete" {
		cppe.emitToFile(cppe.deleteMapVarName + " = hmap::hashMapDelete")
		return
	}
	if cppe.isMapLenCall && name == "std::size" {
		cppe.emitToFile("hmap::hashMapLen")
		return
	}
	if name == "nil" {
		str = cppe.emitAsString("{}", indent)
		cppe.emitToFile(str)
	} else {
		if n, ok := cppTypesMap[name]; ok {
			str = cppe.emitAsString(n, indent)
			cppe.emitToFile(str)
		} else {
			str = cppe.emitAsString(name, indent)
			cppe.emitToFile(str)
		}
	}
}

func (cppe *CPPEmitter) PreVisitBinaryExprOperator(op token.Token, indent int) {
	str := cppe.emitAsString(op.String()+" ", 1)
	cppe.emitToFile(str)
}

// PreVisitCallExpr detects map operations: make(map[K]V), make([]T,n), delete, len(map)
func (cppe *CPPEmitter) PreVisitCallExpr(node *ast.CallExpr, indent int) {
	if cppe.forwardDecl {
		return
	}
	// Detect make(map[K]V) and make([]T, n) calls
	if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "make" {
		if len(node.Args) >= 1 {
			if mapType, ok := node.Args[0].(*ast.MapType); ok {
				cppe.isMapMakeCall = true
				cppe.mapMakeKeyType = cppe.getMapKeyTypeConst(mapType)
			} else if arrayType, ok := node.Args[0].(*ast.ArrayType); ok {
				// make([]T, n) → std::vector<CppType>(n)
				cppe.isSliceMakeCall = true
				cppe.sliceMakeCppType = "std::vector<std::any>" // default for interface{}
				if cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
					if tv, ok := cppe.pkg.TypesInfo.Types[arrayType.Elt]; ok && tv.Type != nil {
						cppType := getCppTypeName(tv.Type)
						cppe.sliceMakeCppType = "std::vector<" + cppType + ">"
					}
				}
			}
		}
	}
	// Detect delete(m, k) calls
	if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "delete" {
		if len(node.Args) >= 2 {
			cppe.isDeleteCall = true
			cppe.deleteMapVarName = exprToString(node.Args[0])
			cppe.deleteKeyCastPrefix = ""
			cppe.deleteKeyCastSuffix = ""
			if cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
				tv := cppe.pkg.TypesInfo.Types[node.Args[0]]
				if tv.Type != nil {
					if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
						cppe.deleteKeyCastPrefix, cppe.deleteKeyCastSuffix = getCppKeyCast(mapType.Key())
					}
				}
			}
		}
	}
	// Detect len(m) calls on maps
	if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "len" {
		if len(node.Args) >= 1 && cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
			tv := cppe.pkg.TypesInfo.Types[node.Args[0]]
			if tv.Type != nil {
				if _, ok := tv.Type.Underlying().(*types.Map); ok {
					cppe.isMapLenCall = true
				}
			}
		}
	}
}

func (cppe *CPPEmitter) PreVisitCallExprArgs(node []ast.Expr, indent int) {
	cppe.currentCallArgIdentsStack = append(cppe.currentCallArgIdentsStack, cppe.collectCallArgIdentCounts(node))
	str := cppe.emitAsString("(", 0)
	cppe.emitToFile(str)
	// For make(map[K]V), emit the key type constant as the argument
	if cppe.isMapMakeCall {
		cppe.emitToFile(fmt.Sprintf("%d", cppe.mapMakeKeyType))
	}
}
func (cppe *CPPEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	if len(cppe.currentCallArgIdentsStack) > 0 {
		cppe.currentCallArgIdentsStack = cppe.currentCallArgIdentsStack[:len(cppe.currentCallArgIdentsStack)-1]
	}
	str := cppe.emitAsString(")", 0)
	cppe.emitToFile(str)
	// For make([]T, n), no .fill needed in C++ (vector constructor handles it)
	if cppe.isSliceMakeCall {
		cppe.isSliceMakeCall = false
		cppe.sliceMakeCppType = ""
		cppe.suppressEmit = false
	}
	// Reset map call flags
	if cppe.isMapMakeCall {
		cppe.isMapMakeCall = false
	}
	if cppe.isDeleteCall {
		cppe.isDeleteCall = false
		cppe.deleteMapVarName = ""
		cppe.deleteKeyCastPrefix = ""
		cppe.deleteKeyCastSuffix = ""
	}
	if cppe.isMapLenCall {
		cppe.isMapLenCall = false
	}
}

func (cppe *CPPEmitter) PreVisitCallExprArg(node ast.Expr, index int, indent int) {
	if cppe.isSliceMakeCall {
		if index == 0 {
			// Suppress the first argument (the ArrayType) for make([]T, n)
			cppe.suppressEmit = true
			return
		} else if index == 1 {
			// Re-enable output for the length argument
			cppe.suppressEmit = false
			return
		}
	}
	if index > 0 {
		str := cppe.emitAsString(", ", 0)
		cppe.emitToFile(str)
	}
	// Wrap delete key argument with appropriate cast
	if cppe.isDeleteCall && cppe.deleteKeyCastPrefix != "" && index == 1 {
		cppe.emitToFile(cppe.deleteKeyCastPrefix)
	}
	// Check if this argument is a struct that can be moved
	cppe.moveCurrentArg = false
	if cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
		if ident, isIdent := node.(*ast.Ident); isIdent {
			tv := cppe.pkg.TypesInfo.Types[node]
			if tv.Type != nil {
				if named, ok := tv.Type.(*types.Named); ok {
					if _, isStruct := named.Underlying().(*types.Struct); isStruct {
						if cppe.canMoveArg(ident.Name) {
							cppe.moveCurrentArg = true
							str := cppe.emitAsString("std::move(", 0)
							cppe.emitToFile(str)
						}
					}
				}
			}
		}
	}
}

func (cppe *CPPEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {
	if cppe.isDeleteCall && cppe.deleteKeyCastSuffix != "" && index == 1 {
		cppe.emitToFile(cppe.deleteKeyCastSuffix)
	}
	if cppe.moveCurrentArg {
		str := cppe.emitAsString(")", 0)
		cppe.emitToFile(str)
		cppe.moveCurrentArg = false
	}
}

func (cppe *CPPEmitter) PreVisitParenExpr(node *ast.ParenExpr, indent int) {
	str := cppe.emitAsString("(", 0)
	cppe.emitToFile(str)
}
func (cppe *CPPEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {
	str := cppe.emitAsString(")", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitCompositeLitElts(node []ast.Expr, indent int) {
	str := cppe.emitAsString("{", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	str := cppe.emitAsString("}", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	if index > 0 {
		str := cppe.emitAsString(", ", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitArrayType(node ast.ArrayType, indent int) {
	str := cppe.emitAsString("std::vector<", indent)
	cppe.emitToFile(str)
}
func (cppe *CPPEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	str := cppe.emitAsString(">", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitMapType(node *ast.MapType, indent int) {
	// Skip when inside make(map[K]V) — already handled by make lowering
	if cppe.isMapMakeCall {
		return
	}
	cppe.emitToFile("hmap::HashMap")
}

func (cppe *CPPEmitter) PostVisitMapType(node *ast.MapType, indent int) {
}

// PreVisitMapKeyType suppresses output during map key type traversal
func (cppe *CPPEmitter) PreVisitMapKeyType(node ast.Expr, indent int) {
	cppe.suppressEmitStack = append(cppe.suppressEmitStack, cppe.suppressEmit)
	cppe.suppressEmit = true
}

// PostVisitMapKeyType restores output state after map key type traversal
func (cppe *CPPEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {
	if len(cppe.suppressEmitStack) > 0 {
		cppe.suppressEmit = cppe.suppressEmitStack[len(cppe.suppressEmitStack)-1]
		cppe.suppressEmitStack = cppe.suppressEmitStack[:len(cppe.suppressEmitStack)-1]
	} else {
		cppe.suppressEmit = false
	}
}

// PreVisitMapValueType suppresses output during map value type traversal
func (cppe *CPPEmitter) PreVisitMapValueType(node ast.Expr, indent int) {
	cppe.suppressEmitStack = append(cppe.suppressEmitStack, cppe.suppressEmit)
	cppe.suppressEmit = true
}

// PostVisitMapValueType restores output state after map value type traversal
func (cppe *CPPEmitter) PostVisitMapValueType(node ast.Expr, indent int) {
	if len(cppe.suppressEmitStack) > 0 {
		cppe.suppressEmit = cppe.suppressEmitStack[len(cppe.suppressEmitStack)-1]
		cppe.suppressEmitStack = cppe.suppressEmitStack[:len(cppe.suppressEmitStack)-1]
	} else {
		cppe.suppressEmit = false
	}
}

func (cppe *CPPEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	if ident, ok := node.(*ast.Ident); ok {
		if cppe.lowerToBuiltins(ident.Name) == "" {
			return
		}
		scopeOperator := "."
		if _, found := namespaces[ident.Name]; found {
			scopeOperator = "::"
		}
		str := cppe.emitAsString(scopeOperator, 0)
		cppe.emitToFile(str)
	} else {
		str := cppe.emitAsString(".", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitIndexExpr(node *ast.IndexExpr, indent int) {
	if cppe.forwardDecl {
		return
	}
	// Check if this is a map index (not a map assignment or comma-ok - those are handled separately)
	if !cppe.isMapAssign && !cppe.isMapCommaOk && cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
		tv := cppe.pkg.TypesInfo.Types[node.X]
		if tv.Type != nil {
			if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
				// Push "map" onto the access type stack
				cppe.indexAccessTypeStack = append(cppe.indexAccessTypeStack, "map")

				// If already in a map index (nested read), save current state
				if cppe.isMapIndex {
					cppe.mapIndexKeyCastStack = append(cppe.mapIndexKeyCastStack, keyCastState{
						prefix:    cppe.mapIndexKeyCastPrefix,
						suffix:    cppe.mapIndexKeyCastSuffix,
						valueType: cppe.mapIndexValueCppType,
					})
				}
				cppe.isMapIndex = true
				cppe.mapIndexKeyCastPrefix, cppe.mapIndexKeyCastSuffix = getCppKeyCast(mapType.Key())

				// Check if the value type is also a map (nested map read)
				// If so, cast to hmap::HashMap instead of the map type
				if _, isNestedMap := mapType.Elem().Underlying().(*types.Map); isNestedMap {
					cppe.mapIndexValueCppType = "hmap::HashMap"
					cppe.nestedMapReadDepth++
				} else {
					cppe.mapIndexValueCppType = getCppTypeName(mapType.Elem())
				}

				// Emit: std::any_cast<ValueType>(hmap::hashMapGet(
				cppe.emitToFile("std::any_cast<" + cppe.mapIndexValueCppType + ">(hmap::hashMapGet(")
			} else {
				// Slice or array access - push "slice" onto the stack
				switch tv.Type.Underlying().(type) {
				case *types.Slice, *types.Array:
					cppe.indexAccessTypeStack = append(cppe.indexAccessTypeStack, "slice")
				default:
					// For other types (like string indexing), treat as slice-like
					cppe.indexAccessTypeStack = append(cppe.indexAccessTypeStack, "slice")
				}
			}
		}
	}
}

func (cppe *CPPEmitter) PreVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	if cppe.isMapCommaOk {
		cppe.captureMapKey = true
		cppe.capturedMapKey = ""
		return
	}
	if cppe.isMapAssign {
		// Capture the key expression
		cppe.captureMapKey = true
		cppe.capturedMapKey = ""
		return
	}
	// Check the access type stack to determine if this is a map or slice access
	if len(cppe.indexAccessTypeStack) > 0 {
		accessType := cppe.indexAccessTypeStack[len(cppe.indexAccessTypeStack)-1]
		if accessType == "map" {
			if cppe.mapIndexKeyCastPrefix != "" {
				cppe.emitToFile(", " + cppe.mapIndexKeyCastPrefix)
			} else {
				cppe.emitToFile(", ")
			}
			return
		}
	}
	// Slice/array access - emit [
	str := cppe.emitAsString("[", 0)
	cppe.emitToFile(str)
}
func (cppe *CPPEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	if cppe.isMapCommaOk {
		cppe.captureMapKey = false
		return
	}
	if cppe.captureMapKey {
		cppe.captureMapKey = false
		return
	}
	// Check the access type stack to determine if this is a map or slice access
	if len(cppe.indexAccessTypeStack) > 0 {
		accessType := cppe.indexAccessTypeStack[len(cppe.indexAccessTypeStack)-1]
		// Pop from the stack
		cppe.indexAccessTypeStack = cppe.indexAccessTypeStack[:len(cppe.indexAccessTypeStack)-1]

		if accessType == "map" {
			if cppe.mapIndexKeyCastSuffix != "" {
				cppe.emitToFile(cppe.mapIndexKeyCastSuffix)
			}
			cppe.emitToFile("))")
			// Restore previous key cast state if we have saved state (nested reads)
			if len(cppe.mapIndexKeyCastStack) > 0 {
				state := cppe.mapIndexKeyCastStack[len(cppe.mapIndexKeyCastStack)-1]
				cppe.mapIndexKeyCastStack = cppe.mapIndexKeyCastStack[:len(cppe.mapIndexKeyCastStack)-1]
				cppe.mapIndexKeyCastPrefix = state.prefix
				cppe.mapIndexKeyCastSuffix = state.suffix
				cppe.mapIndexValueCppType = state.valueType
				// Keep isMapIndex true for the outer map read
				if cppe.nestedMapReadDepth > 0 {
					cppe.nestedMapReadDepth--
				}
			} else {
				cppe.isMapIndex = false
				cppe.mapIndexValueCppType = ""
				cppe.mapIndexKeyCastPrefix = ""
				cppe.mapIndexKeyCastSuffix = ""
			}
			return
		}
	}
	// Slice/array access - emit ]
	str := cppe.emitAsString("]", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	// Nothing needed - cleanup handled in PostVisitIndexExprIndex
}

func (cppe *CPPEmitter) PreVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	str := cppe.emitAsString("(", 0)
	str += cppe.emitAsString(node.Op.String(), 0)
	cppe.emitToFile(str)
}
func (cppe *CPPEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	str := cppe.emitAsString(")", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitSliceExpr(node *ast.SliceExpr, indent int) {
	str := cppe.emitAsString("std::vector<std::remove_reference<decltype(", indent)
	cppe.emitToFile(str)
}
func (cppe *CPPEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	str := cppe.emitAsString(")", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	str := cppe.emitAsString("[0]", 0)
	str += cppe.emitAsString(")>::type>(", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitSliceExprLow(node ast.Expr, indent int) {
	str := cppe.emitAsString(".begin() ", 0)
	cppe.emitToFile(str)
	if node != nil {
		str := cppe.emitAsString("+ ", 0)
		cppe.emitToFile(str)
	} else {
		DebugLogPrintf("Low index: <nil>\n")
	}
}

func (cppe *CPPEmitter) PreVisitSliceExprXEnd(node ast.Expr, indent int) {
	str := cppe.emitAsString(", ", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitSliceExprHigh(node ast.Expr, indent int) {
	if node != nil {
		str := cppe.emitAsString(".begin() ", 0)
		cppe.emitToFile(str)
		str = cppe.emitAsString("+ ", 0)
		cppe.emitToFile(str)
	} else {
		str := cppe.emitAsString(".end() ", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PostVisitSliceExprHigh(node ast.Expr, indent int) {

}

func (cppe *CPPEmitter) PreVisitFuncType(node *ast.FuncType, indent int) {
	str := cppe.emitAsString("std::function<", indent)
	cppe.emitToFile(str)
}
func (cppe *CPPEmitter) PostVisitFuncType(node *ast.FuncType, indent int) {
	str := cppe.emitAsString(">", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitFuncTypeResults(node *ast.FieldList, indent int) {
	if node == nil {
		str := cppe.emitAsString("void", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitFuncTypeResult(node *ast.Field, index int, indent int) {
	if index > 0 {
		str := cppe.emitAsString(", ", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	str := cppe.emitAsString(".", 0)
	cppe.emitToFile(str)
}
func (cppe *CPPEmitter) PostVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	str := cppe.emitAsString("\n", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitFuncTypeParams(node *ast.FieldList, indent int) {
	str := cppe.emitAsString("(", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitFuncTypeParams(node *ast.FieldList, indent int) {
	str := cppe.emitAsString(")", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitFuncTypeParam(node *ast.Field, index int, indent int) {
	if index > 0 {
		str := cppe.emitAsString(", ", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitKeyValueExprValue(node ast.Expr, indent int) {
	str := cppe.emitAsString("= ", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitFuncLit(node *ast.FuncLit, indent int) {
	cppe.funcLitDepth++
	str := cppe.emitAsString("[&](", indent)
	cppe.emitToFile(str)
}
func (cppe *CPPEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	cppe.funcLitDepth--
	str := cppe.emitAsString("}", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	str := cppe.emitAsString(")", 0)
	str += cppe.emitAsString("->", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	str := ""
	if index > 0 {
		str += cppe.emitAsString(", ", 0)
	}
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	str := cppe.emitAsString(" ", 0)
	str += cppe.emitAsString(node.Names[0].Name, indent)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	str := cppe.emitAsString("{\n", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitFuncLitTypeResults(node *ast.FieldList, indent int) {
	if node == nil {
		str := cppe.emitAsString("void", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitFuncLitTypeResult(node *ast.Field, index int, indent int) {
	if index > 0 {
		str := cppe.emitAsString(", ", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitTypeAssertExprType(node ast.Expr, indent int) {
	if cppe.isTypeAssertCommaOk {
		return
	}
	str := cppe.emitAsString("std::any_cast<", indent)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitTypeAssertExprX(node ast.Expr, indent int) {
	if cppe.isTypeAssertCommaOk {
		cppe.captureMapKey = true
		cppe.capturedMapKey = ""
		return
	}
	// Check if the expression is already interface{} (std::any) - skip the std::any() wrapper
	needsAnyWrap := true
	if cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
		tv := cppe.pkg.TypesInfo.Types[node]
		if tv.Type != nil {
			if iface, ok := tv.Type.Underlying().(*types.Interface); ok && iface.Empty() {
				needsAnyWrap = false
			}
		}
	}
	if needsAnyWrap {
		str := cppe.emitAsString(">(std::any(", 0)
		cppe.emitToFile(str)
	} else {
		str := cppe.emitAsString(">(", 0)
		cppe.emitToFile(str)
	}
}
func (cppe *CPPEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	if cppe.isTypeAssertCommaOk {
		cppe.captureMapKey = false
		return
	}
	// Match the wrapper from PreVisitTypeAssertExprX
	needsAnyWrap := true
	if cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
		tv := cppe.pkg.TypesInfo.Types[node]
		if tv.Type != nil {
			if iface, ok := tv.Type.Underlying().(*types.Interface); ok && iface.Empty() {
				needsAnyWrap = false
			}
		}
	}
	if needsAnyWrap {
		str := cppe.emitAsString("))", 0)
		cppe.emitToFile(str)
	} else {
		str := cppe.emitAsString(")", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitStarExpr(node *ast.StarExpr, indent int) {
	str := cppe.emitAsString("*", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitInterfaceType(node *ast.InterfaceType, indent int) {
	str := cppe.emitAsString("std::any", indent)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	str := cppe.emitAsString(";", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	// Track if there's an explicit initialization value
	cppe.hasDeclValue = index < len(node.Values)
	// Detect var m map[K]V declarations
	if len(node.Values) == 0 && node.Type != nil {
		if cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
			if typeAndValue, ok := cppe.pkg.TypesInfo.Types[node.Type]; ok {
				if _, isMap := typeAndValue.Type.Underlying().(*types.Map); isMap {
					if mapType, ok := node.Type.(*ast.MapType); ok {
						cppe.pendingMapInit = true
						cppe.pendingMapKeyType = cppe.getMapKeyTypeConst(mapType)
					}
				}
			}
		}
	}
}

func (cppe *CPPEmitter) PreVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	str := cppe.emitAsString(" ", 0)
	cppe.emitToFile(str)
}
func (cppe *CPPEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	if cppe.pendingMapInit {
		cppe.emitToFile(fmt.Sprintf(" = hmap::newHashMap(%d)", cppe.pendingMapKeyType))
		cppe.pendingMapInit = false
		cppe.pendingMapKeyType = 0
	}
	// Only emit semicolon if there's no value coming
	if !cppe.hasDeclValue {
		cppe.emitToFile(";")
	}
}

func (cppe *CPPEmitter) PreVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {
	cppe.emitToFile(" = ")
}

func (cppe *CPPEmitter) PostVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {
	cppe.emitToFile(";")
	cppe.hasDeclValue = false
}

func (cppe *CPPEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	str := cppe.emitAsString(node.Tok.String()+";", indent)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	str := cppe.emitAsString(node.Tok.String(), 0)
	if !cppe.insideForPostCond {
		str += cppe.emitAsString(";", 0)
	}
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitForStmtPost(node ast.Stmt, indent int) {
	if node != nil {
		cppe.insideForPostCond = true
	}
}
func (cppe *CPPEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	cppe.insideForPostCond = false
	str := cppe.emitAsString(")\n", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	// Check if all LHS are blank identifiers - if so, suppress the statement
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
		cppe.suppressRangeEmit = true
		return
	}
	// Detect comma-ok: val, ok := m[key]
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if indexExpr, ok := node.Rhs[0].(*ast.IndexExpr); ok {
			if cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
				tv := cppe.pkg.TypesInfo.Types[indexExpr.X]
				if tv.Type != nil {
					if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
						cppe.isMapCommaOk = true
						cppe.mapCommaOkValName = node.Lhs[0].(*ast.Ident).Name
						cppe.mapCommaOkOkName = node.Lhs[1].(*ast.Ident).Name
						cppe.mapCommaOkMapName = exprToString(indexExpr.X)
						cppe.mapCommaOkValueType = getCppTypeName(mapType.Elem())
						cppe.mapCommaOkIsDecl = (node.Tok == token.DEFINE)
						cppe.mapCommaOkIndent = indent
						cppe.mapCommaOkKeyCastPrefix, cppe.mapCommaOkKeyCastSuffix = getCppKeyCast(mapType.Key())
						cppe.suppressEmit = true
						return
					}
				}
			}
		}
	}
	// Detect type assertion comma-ok: val, ok := x.(Type)
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if typeAssert, ok := node.Rhs[0].(*ast.TypeAssertExpr); ok {
			if cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
				tv := cppe.pkg.TypesInfo.Types[typeAssert.Type]
				if tv.Type != nil {
					cppe.isTypeAssertCommaOk = true
					cppe.typeAssertCommaOkValName = node.Lhs[0].(*ast.Ident).Name
					cppe.typeAssertCommaOkOkName = node.Lhs[1].(*ast.Ident).Name
					cppe.typeAssertCommaOkType = getCppTypeName(tv.Type)
					cppe.typeAssertCommaOkIsDecl = (node.Tok == token.DEFINE)
					cppe.typeAssertCommaOkIndent = indent
					cppe.suppressEmit = true
					return
				}
			}
		}
	}
	// Detect map assignment: m[k] = v or nested m[k1][k2] = v
	if len(node.Lhs) == 1 {
		if indexExpr, ok := node.Lhs[0].(*ast.IndexExpr); ok {
			if cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
				tv := cppe.pkg.TypesInfo.Types[indexExpr.X]
				if tv.Type != nil {
					if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
						cppe.isMapAssign = true
						cppe.mapAssignIndent = indent
						cppe.mapAssignValueIsString = false
						cppe.mapAssignKeyCastPrefix, cppe.mapAssignKeyCastSuffix = getCppKeyCast(mapType.Key())
						valType := mapType.Elem()
						if basic, isBasic := valType.Underlying().(*types.Basic); isBasic && basic.Kind() == types.String {
							cppe.mapAssignValueIsString = true
						}

						// Check for nested index: m[k1][k2] = v or sliceOfMaps[0]["key"] = v
						if outerIndexExpr, isNested := indexExpr.X.(*ast.IndexExpr); isNested {
							// Check if outer expression is a map or a slice
							outerTv := cppe.pkg.TypesInfo.Types[outerIndexExpr.X]
							if outerTv.Type != nil {
								if outerMapType, ok := outerTv.Type.Underlying().(*types.Map); ok {
									// Nested map assignment: m[k1][k2] = v
									cppe.isNestedMapAssign = true
									cppe.nestedMapOuterVarName = exprToString(outerIndexExpr.X)
									// Extract outer key directly from AST
									cppe.nestedMapOuterKey = exprToCppString(outerIndexExpr.Index)
									cppe.nestedMapOuterKeyCastPrefix, cppe.nestedMapOuterKeyCastSuffix = getCppKeyCast(outerMapType.Key())
									cppe.nestedMapValueCppType = "hmap::HashMap"
									// Use temp variable for inner map with unique name
									cppe.nestedMapVarName = fmt.Sprintf("__nested_inner_%d", cppe.nestedMapCounter)
									cppe.nestedMapCounter++
									cppe.mapAssignVarName = cppe.nestedMapVarName
								} else {
									// Check if the slice is accessed from a map further up
									// e.g., mapVar[k1][sliceIdx]["key"] = v
									if deeperIndexExpr, isDeeper := outerIndexExpr.X.(*ast.IndexExpr); isDeeper {
										deeperTv := cppe.pkg.TypesInfo.Types[deeperIndexExpr.X]
										if deeperTv.Type != nil {
											if deeperMapType, ok := deeperTv.Type.Underlying().(*types.Map); ok {
												// map-slice-map assignment pattern
												cppe.isNestedMapAssign = true
												cppe.nestedMapOuterVarName = exprToString(deeperIndexExpr.X)
												cppe.nestedMapOuterKey = exprToCppString(deeperIndexExpr.Index)
												cppe.nestedMapOuterKeyCastPrefix, cppe.nestedMapOuterKeyCastSuffix = getCppKeyCast(deeperMapType.Key())
												cppe.nestedMapValueCppType = getCppTypeName(deeperMapType.Elem())
												cppe.nestedMapVarName = fmt.Sprintf("__nested_inner_%d", cppe.nestedMapCounter)
												cppe.nestedMapCounter++
												sliceIdx := exprToCppString(outerIndexExpr.Index)
												cppe.mapAssignVarName = cppe.nestedMapVarName + "[" + sliceIdx + "]"
											} else {
												cppe.isNestedMapAssign = false
												cppe.mapAssignVarName = exprToCppString(indexExpr.X)
											}
										} else {
											cppe.isNestedMapAssign = false
											cppe.mapAssignVarName = exprToCppString(indexExpr.X)
										}
									} else {
										// Simple slice-then-map: sliceOfMaps[0]["key"] = v
										cppe.isNestedMapAssign = false
										cppe.mapAssignVarName = exprToCppString(indexExpr.X)
									}
								}
							} else {
								// Fallback: treat as non-nested
								cppe.isNestedMapAssign = false
								cppe.mapAssignVarName = exprToCppString(indexExpr.X)
							}
						} else {
							cppe.isNestedMapAssign = false
							cppe.mapAssignVarName = exprToString(indexExpr.X)
						}
						// Suppress normal LHS output (the m[k] part will be captured)
						cppe.suppressEmit = true
						return
					}
					// Detect mixed index assignment: any pattern where intermediate map
					// accesses exist below a non-map outermost access
					// e.g., mapOfSlices["key"][idx] = v, nestedMaps["a"]["b"][idx] = v
					ops, hasIntermediateMap := cppe.analyzeLhsIndexChain(node.Lhs[0])
					if hasIntermediateMap && len(ops) > 0 {
						// Get root variable name
						rootExpr := node.Lhs[0]
						for {
							if ie, ok := rootExpr.(*ast.IndexExpr); ok {
								rootExpr = ie.X
							} else {
								break
							}
						}
						rootVar := exprToString(rootExpr)

						// Assign temp var names to map accesses and build the chain
						currentVar := rootVar
						for i := range ops {
							if ops[i].accessType == "map" {
								ops[i].mapVarExpr = currentVar
								ops[i].tempVarName = fmt.Sprintf("__nested_inner_%d", cppe.nestedMapCounter)
								cppe.nestedMapCounter++
								currentVar = ops[i].tempVarName
							} else {
								// Slice access on current variable
								ops[i].mapVarExpr = currentVar
								currentVar = currentVar + "[" + ops[i].keyExpr + "]"
							}
						}

						cppe.isMixedIndexAssign = true
						cppe.mixedIndexAssignOps = ops
						cppe.mixedAssignIndent = indent
						cppe.mixedAssignFinalTarget = currentVar
						cppe.suppressEmit = true
						return
					}
				}
			}
		}
	}
	// Detect if LHS is interface{}/std::any type (for string literal wrapping)
	cppe.assignLhsIsInterface = false
	if len(node.Lhs) == 1 && cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
		if ident, ok := node.Lhs[0].(*ast.Ident); ok {
			if obj := cppe.pkg.TypesInfo.ObjectOf(ident); obj != nil {
				if iface, ok := obj.Type().Underlying().(*types.Interface); ok && iface.Empty() {
					cppe.assignLhsIsInterface = true
				}
			}
		}
	}
	// Capture LHS variable names for move optimization
	cppe.currentAssignLhsNames = make(map[string]bool)
	for _, lhs := range node.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok {
			cppe.currentAssignLhsNames[ident.Name] = true
		}
	}
	str := cppe.emitAsString("", indent)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	cppe.currentAssignLhsNames = nil
	cppe.assignLhsIsInterface = false
	if cppe.isTypeAssertCommaOk {
		expr := cppe.capturedMapKey
		typeName := cppe.typeAssertCommaOkType
		decl := ""
		if cppe.typeAssertCommaOkIsDecl {
			decl = "auto "
		}
		indentStr := strings.Repeat("    ", cppe.typeAssertCommaOkIndent/4)
		cppe.suppressEmit = false
		cppe.emitToFile(fmt.Sprintf("%s%s%s = (std::any(%s)).type() == typeid(%s);\n",
			indentStr, decl, cppe.typeAssertCommaOkOkName, expr, typeName))
		cppe.emitToFile(fmt.Sprintf("%s%s%s = %s ? std::any_cast<%s>(std::any(%s)) : %s{};\n",
			indentStr, decl, cppe.typeAssertCommaOkValName, cppe.typeAssertCommaOkOkName,
			typeName, expr, typeName))
		cppe.isTypeAssertCommaOk = false
		cppe.capturedMapKey = ""
		cppe.captureMapKey = false
		return
	}
	if cppe.isMixedIndexAssign {
		// Finish the value assignment
		cppe.emitToFile(";\n")
		// Emit epilogue: set-back chain in reverse order for map accesses
		for i := len(cppe.mixedIndexAssignOps) - 1; i >= 0; i-- {
			op := cppe.mixedIndexAssignOps[i]
			if op.accessType == "map" {
				key := op.keyExpr
				if op.keyCastPfx != "" {
					key = op.keyCastPfx + key + op.keyCastSfx
				}
				epilogue := cppe.emitAsString(op.mapVarExpr+" = hmap::hashMapSet("+
					op.mapVarExpr+", "+key+", "+op.tempVarName+");\n", cppe.mixedAssignIndent)
				cppe.emitToFile(epilogue)
			}
		}
		// Reset state
		cppe.isMixedIndexAssign = false
		cppe.mixedIndexAssignOps = nil
		cppe.mixedAssignFinalTarget = ""
		return
	}
	if cppe.isMapCommaOk {
		key := cppe.capturedMapKey
		if cppe.mapCommaOkKeyCastPrefix != "" {
			key = cppe.mapCommaOkKeyCastPrefix + key + cppe.mapCommaOkKeyCastSuffix
		}
		decl := ""
		if cppe.mapCommaOkIsDecl {
			decl = "auto "
		}
		indentStr := strings.Repeat("    ", cppe.mapCommaOkIndent/4)
		cppe.suppressEmit = false
		cppe.emitToFile(fmt.Sprintf("%s%s%s = hmap::hashMapContains(%s, %s);\n",
			indentStr, decl, cppe.mapCommaOkOkName, cppe.mapCommaOkMapName, key))
		cppe.emitToFile(fmt.Sprintf("%s%s%s = %s ? std::any_cast<%s>(hmap::hashMapGet(%s, %s)) : %s{};\n",
			indentStr, decl, cppe.mapCommaOkValName, cppe.mapCommaOkOkName,
			cppe.mapCommaOkValueType, cppe.mapCommaOkMapName, key, cppe.mapCommaOkValueType))
		cppe.isMapCommaOk = false
		return
	}
	if cppe.isMapAssign {
		if cppe.mapAssignValueIsString {
			cppe.emitToFile(")")
		}
		cppe.emitToFile(");\n")

		// For nested map assignment, emit epilogue to set updated inner map back to outer
		if cppe.isNestedMapAssign {
			outerKey := cppe.nestedMapOuterKey
			if cppe.nestedMapOuterKeyCastPrefix != "" {
				outerKey = cppe.nestedMapOuterKeyCastPrefix + outerKey + cppe.nestedMapOuterKeyCastSuffix
			}
			epilogue := cppe.emitAsString(cppe.nestedMapOuterVarName+" = hmap::hashMapSet("+
				cppe.nestedMapOuterVarName+", "+outerKey+", "+cppe.nestedMapVarName+");\n", cppe.mapAssignIndent)
			cppe.emitToFile(epilogue)
			cppe.isNestedMapAssign = false
			cppe.nestedMapOuterVarName = ""
			cppe.nestedMapOuterKey = ""
			cppe.nestedMapOuterKeyCastPrefix = ""
			cppe.nestedMapOuterKeyCastSuffix = ""
		}

		cppe.isMapAssign = false
		cppe.mapAssignVarName = ""
		cppe.capturedMapKey = ""
		cppe.mapAssignKeyCastPrefix = ""
		cppe.mapAssignKeyCastSuffix = ""
		cppe.mapAssignValueIsString = false
		return
	}
	// Reset blank identifier suppression if it was set
	if cppe.suppressRangeEmit {
		cppe.suppressRangeEmit = false
		return
	}
	// Don't emit semicolon inside for loop post statement
	if !cppe.insideForPostCond {
		str := cppe.emitAsString(";", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	assignmentToken := node.Tok.String()
	if assignmentToken == ":=" && len(node.Lhs) == 1 {
		// Check if RHS is a string literal - use std::string instead of auto
		// to avoid const char* which breaks string concatenation
		if len(node.Rhs) == 1 {
			if lit, ok := node.Rhs[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				str := cppe.emitAsString("std::string ", indent)
				cppe.emitToFile(str)
			} else {
				str := cppe.emitAsString("auto ", indent)
				cppe.emitToFile(str)
			}
		} else {
			str := cppe.emitAsString("auto ", indent)
			cppe.emitToFile(str)
		}
	} else if assignmentToken == ":=" && len(node.Lhs) > 1 {
		str := cppe.emitAsString("auto [", indent)
		cppe.emitToFile(str)
	} else if assignmentToken == "=" && len(node.Lhs) > 1 {
		str := cppe.emitAsString("std::tie(", indent)
		cppe.emitToFile(str)
	}
	// Preserve compound assignment operators, convert := to =
	if assignmentToken != "+=" && assignmentToken != "-=" && assignmentToken != "*=" && assignmentToken != "/=" {
		assignmentToken = "="
	}
	cppe.assignmentToken = assignmentToken
}

func (cppe *CPPEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	if node.Tok.String() == ":=" && len(node.Lhs) > 1 {
		str := cppe.emitAsString("]", indent)
		cppe.emitToFile(str)
	} else if node.Tok.String() == "=" && len(node.Lhs) > 1 {
		str := cppe.emitAsString(")", indent)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	if cppe.isMixedIndexAssign {
		// Turn off suppression and emit the preamble + assignment
		cppe.suppressEmit = false
		// Emit preamble: extract temp variables for each map access
		for _, op := range cppe.mixedIndexAssignOps {
			if op.accessType == "map" {
				key := op.keyExpr
				if op.keyCastPfx != "" {
					key = op.keyCastPfx + key + op.keyCastSfx
				}
				preamble := cppe.emitAsString("auto "+op.tempVarName+" = std::any_cast<"+
					op.valueCppType+">(hmap::hashMapGet("+
					op.mapVarExpr+", "+key+"));\n", cppe.mixedAssignIndent)
				cppe.emitToFile(preamble)
			}
		}
		// Emit assignment: finalTarget = value
		str := cppe.emitAsString(cppe.mixedAssignFinalTarget+" = ", cppe.mixedAssignIndent)
		cppe.emitToFile(str)
		return
	}
	if cppe.isMapAssign {
		// Turn off suppression and emit the assignment
		cppe.suppressEmit = false
		key := cppe.capturedMapKey
		if cppe.mapAssignKeyCastPrefix != "" {
			key = cppe.mapAssignKeyCastPrefix + key + cppe.mapAssignKeyCastSuffix
		}

		if cppe.isNestedMapAssign {
			// For nested map: m["outer"]["inner"] = v
			// Generate:
			//   auto __nested_inner = std::any_cast<hmap::HashMap>(hmap::hashMapGet(m, outerKey));
			//   __nested_inner = hmap::hashMapSet(__nested_inner, innerKey, value)
			// (The epilogue in PostVisitAssignStmt will add the outer set)
			outerKey := cppe.nestedMapOuterKey
			if cppe.nestedMapOuterKeyCastPrefix != "" {
				outerKey = cppe.nestedMapOuterKeyCastPrefix + outerKey + cppe.nestedMapOuterKeyCastSuffix
			}
			// Emit preamble: extract inner value (map or slice of maps)
			castType := cppe.nestedMapValueCppType
			if castType == "" {
				castType = "hmap::HashMap"
			}
			preamble := cppe.emitAsString("auto "+cppe.nestedMapVarName+" = std::any_cast<"+castType+">(hmap::hashMapGet("+
				cppe.nestedMapOuterVarName+", "+outerKey+"));\n", cppe.mapAssignIndent)
			cppe.emitToFile(preamble)
		}

		str := cppe.emitAsString(cppe.mapAssignVarName+" = hmap::hashMapSet("+cppe.mapAssignVarName+", "+key+", ", cppe.mapAssignIndent)
		cppe.emitToFile(str)
		if cppe.mapAssignValueIsString {
			cppe.emitToFile("std::string(")
		}
		return
	}
	str := cppe.emitAsString(cppe.assignmentToken+" ", indent+1)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	if index > 0 {
		str := cppe.emitAsString(", ", indent)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	cppe.currentReturnNode = node
	str := cppe.emitAsString("return ", indent)
	cppe.emitToFile(str)
	if len(node.Results) > 1 {
		str := cppe.emitAsString("std::make_tuple(", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	if len(node.Results) > 1 {
		str := cppe.emitAsString(")", 0)
		cppe.emitToFile(str)
	}
	str := cppe.emitAsString(";", 0)
	cppe.emitToFile(str)
	cppe.currentReturnNode = nil
}

func (cppe *CPPEmitter) PreVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	if index > 0 {
		str := cppe.emitAsString(", ", 0)
		cppe.emitToFile(str)
	}
	// For multi-value returns, add std::move() for the first struct result
	// when later results don't reference the same identifier
	cppe.moveCurrentReturn = false
	if cppe.OptimizeMoves && cppe.pkg != nil && cppe.pkg.TypesInfo != nil && cppe.currentReturnNode != nil && len(cppe.currentReturnNode.Results) > 1 && index == 0 {
		if ident, ok := node.(*ast.Ident); ok {
			tv := cppe.pkg.TypesInfo.Types[node]
			if tv.Type != nil {
				if named, ok := tv.Type.(*types.Named); ok {
					if _, isStruct := named.Underlying().(*types.Struct); isStruct {
						// Only move if later results don't reference this identifier
						needsClone := false
						for i := 1; i < len(cppe.currentReturnNode.Results); i++ {
							if cppe.exprContainsIdent(cppe.currentReturnNode.Results[i], ident.Name) {
								needsClone = true
								break
							}
						}
						if !needsClone && cppe.funcLitDepth == 0 {
							cppe.moveCurrentReturn = true
							cppe.MoveOptCount++
							str := cppe.emitAsString("std::move(", 0)
							cppe.emitToFile(str)
						}
					}
				}
			}
		}
	}
}

func (cppe *CPPEmitter) PostVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	if cppe.moveCurrentReturn {
		str := cppe.emitAsString(")", 0)
		cppe.emitToFile(str)
		cppe.moveCurrentReturn = false
	}
}

func (cppe *CPPEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	if node.Init != nil {
		str := cppe.emitAsString("{\n", indent)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	if node.Init != nil {
		str := cppe.emitAsString("}\n", indent)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitIfStmtCond(node *ast.IfStmt, indent int) {
	str := cppe.emitAsString("if (", indent)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	str := cppe.emitAsString(")\n", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitIfStmtElse(node *ast.IfStmt, indent int) {
	str := cppe.emitAsString("else", 1)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	str := cppe.emitAsString("for (", indent)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	if node == nil {
		str := cppe.emitAsString(";", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	str := cppe.emitAsString(";", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	// Check if this is a key-value range (both Key and Value present)
	if node.Key != nil && node.Value != nil {
		cppe.isKeyValueRange = true
		cppe.rangeKeyName = node.Key.(*ast.Ident).Name
		cppe.rangeValueName = node.Value.(*ast.Ident).Name
		cppe.rangeCollectionExpr = ""
		cppe.suppressRangeEmit = true
		cppe.rangeStmtIndent = indent
		// Don't emit anything yet - we'll emit the complete for loop in PostVisitRangeStmtX
	} else {
		cppe.isKeyValueRange = false
		str := cppe.emitAsString("for (auto ", indent)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitRangeStmtKey(node ast.Expr, indent int) {
	// For key-value range, we've already captured the key name
	// For index-only range (for i := range), let it emit normally
}

func (cppe *CPPEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	// Nothing special needed here
}

func (cppe *CPPEmitter) PreVisitRangeStmtValue(node ast.Expr, indent int) {
	// For key-value range, we've already captured the value name, keep suppressing
}

func (cppe *CPPEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	if cppe.isKeyValueRange {
		// Stop suppressing, start capturing collection expression
		cppe.suppressRangeEmit = false
		cppe.captureRangeExpr = true
	} else {
		str := cppe.emitAsString(" : ", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitRangeStmtX(node ast.Expr, indent int) {
	// For key-value range, we're already in capture mode
}

func (cppe *CPPEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	if cppe.isKeyValueRange {
		// Stop capturing and emit the complete for loop
		cppe.captureRangeExpr = false
		collection := cppe.rangeCollectionExpr
		key := cppe.rangeKeyName
		value := cppe.rangeValueName
		indent := cppe.rangeStmtIndent

		// Emit: for (size_t key = 0; key < collection.size(); key++)
		str := cppe.emitAsString(fmt.Sprintf("for (size_t %s = 0; %s < %s.size(); %s++)\n", key, key, collection, key), indent)
		cppe.emitToFile(str)

		// Set pending value declaration to be emitted at start of body block
		cppe.pendingRangeValueDecl = true
		cppe.pendingValueName = value
		cppe.pendingCollectionExpr = collection
		cppe.pendingKeyName = key

		// Reset range state
		cppe.isKeyValueRange = false
		cppe.rangeKeyName = ""
		cppe.rangeValueName = ""
		cppe.rangeCollectionExpr = ""
	} else {
		str := cppe.emitAsString(")\n", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitRangeStmtBody(node *ast.BlockStmt, indent int) {
	// For key-value range, emit the value declaration at the start of the body
	// This is called from the base_pass when visiting the body
}

func (cppe *CPPEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	// Reset any range-related state
}
func (cppe *CPPEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	str := cppe.emitAsString("switch (", indent)
	cppe.emitToFile(str)
}
func (cppe *CPPEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	str := cppe.emitAsString("}", indent)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	str := cppe.emitAsString(") {\n", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	cppe.emitToFile("\n")
	str := cppe.emitAsString("break;\n", indent+4)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	if len(node) == 0 {
		str := cppe.emitAsString("default:\n", indent+2)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	str := cppe.emitAsString("case ", indent+2)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	str := cppe.emitAsString(":\n", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	str := cppe.emitAsString("{\n", indent)
	cppe.emitToFile(str)

	// If we have a pending value declaration from key-value range, emit it now
	if cppe.pendingRangeValueDecl {
		valueDecl := cppe.emitAsString(fmt.Sprintf("auto %s = %s[%s];\n",
			cppe.pendingValueName, cppe.pendingCollectionExpr, cppe.pendingKeyName), indent+2)
		cppe.emitToFile(valueDecl)
		cppe.pendingRangeValueDecl = false
		cppe.pendingValueName = ""
		cppe.pendingCollectionExpr = ""
		cppe.pendingKeyName = ""
	}
}

func (cppe *CPPEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	str := cppe.emitAsString("}", indent)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	str := cppe.emitAsString("\n", indent)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	str := cppe.emitAsString("\n\n", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitFuncDeclBody(node *ast.BlockStmt, indent int) {
	str := cppe.emitAsString("\n", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	if node.Type.Results != nil {
		if len(node.Type.Results.List) > 1 {
			str := cppe.emitAsString("std::tuple<", 0)
			err := cppe.emitToFile(str)
			if err != nil {
				fmt.Println("Error writing to file:", err)
				return
			}
		}
	}
}

func (cppe *CPPEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	str := cppe.emitAsString(")", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitFuncDeclName(node *ast.Ident, indent int) {
	DebugLogPrintf("CPPEmitter: PreVisitFuncDeclName: %s", node.Name)
	str := cppe.emitAsString(node.Name, 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	if node.Type.Results != nil {
		if len(node.Type.Results.List) > 1 {
			str := cppe.emitAsString(">", 0)
			cppe.emitToFile(str)
		}
	} else if node.Name.Name == "main" {
		str := cppe.emitAsString("int", 0)
		cppe.emitToFile(str)
	} else {
		str := cppe.emitAsString("void", 0)
		cppe.emitToFile(str)
	}
	str := cppe.emitAsString("", 1)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	if index > 0 {
		str := cppe.emitAsString(",", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	str := cppe.emitAsString("(", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	if index > 0 {
		str := cppe.emitAsString(", ", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	cppe.emitToFile(" ")
}

func (cppe *CPPEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	if cppe.forwardDecl {
		str := cppe.emitAsString(";\n", 0)
		cppe.emitToFile(str)
	}
}

func (cppe *CPPEmitter) PreVisitGenStructInfo(node GenTypeInfo, indent int) {
	str := cppe.emitAsString(fmt.Sprintf("struct %s\n", node.Name), 0)
	err := cppe.emitToFile(str)
	if err != nil {
		fmt.Println("Error writing to file:", err)
	}
	str = cppe.emitAsString("{\n", 0)
	err = cppe.emitToFile(str)
	if err != nil {
		fmt.Println("Error writing to file:", err)
	}
}
func (cppe *CPPEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {
	str := cppe.emitAsString("};\n\n", 0)
	err := cppe.emitToFile(str)
	if err != nil {
		fmt.Println("Error writing to file:", err)
	}

	// Generate operator== and std::hash for structs with primitive fields
	// This enables using them as map keys
	if node.Struct != nil && cppe.structHasOnlyPrimitiveFields(node.Name) {
		cppe.generateStructHashAndEquality(node)
	}
}

// structHasOnlyPrimitiveFields checks if a struct has only hashable fields (primitives or structs with primitive fields)
func (cppe *CPPEmitter) structHasOnlyPrimitiveFields(structName string) bool {
	for _, file := range cppe.pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if typeSpec.Name.Name == structName {
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								if structType.Fields != nil {
									for _, field := range structType.Fields.List {
										fieldType := cppe.pkg.TypesInfo.Types[field.Type].Type
										if !cppe.isHashableType(fieldType, make(map[string]bool)) {
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

// isHashableType checks if a type can have operator== and std::hash in C++ (recursively)
func (cppe *CPPEmitter) isHashableType(t types.Type, visited map[string]bool) bool {
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
	if _, isBasic := underlying.(*types.Basic); isBasic {
		return true
	}
	if structType, isStruct := underlying.(*types.Struct); isStruct {
		for i := 0; i < structType.NumFields(); i++ {
			if !cppe.isHashableType(structType.Field(i).Type(), visited) {
				return false
			}
		}
		return true
	}
	return false
}

// generateStructHashAndEquality stores struct info for deferred hash/equality generation
// The actual generation happens in emitPendingHashSpecs after namespace close
func (cppe *CPPEmitter) generateStructHashAndEquality(node GenTypeInfo) {
	structName := node.Name
	pkgName := ""
	if cppe.pkg != nil && cppe.pkg.Name != "main" {
		pkgName = cppe.pkg.Name
	}

	// Collect field names
	var fieldNames []string
	if node.Struct.Fields != nil {
		for _, field := range node.Struct.Fields.List {
			for _, name := range field.Names {
				fieldNames = append(fieldNames, name.Name)
			}
		}
	}

	// Store for later emission (after namespace close)
	cppe.pendingHashSpecs = append(cppe.pendingHashSpecs, pendingHashSpec{
		structName: structName,
		pkgName:    pkgName,
		fieldNames: fieldNames,
	})
}

// emitPendingHashSpecs emits all pending hash specializations for a package
// Called after namespace close so hash specs are at global scope
func (cppe *CPPEmitter) emitPendingHashSpecs(pkgName string) {
	var specsForPkg []pendingHashSpec
	var remaining []pendingHashSpec

	for _, spec := range cppe.pendingHashSpecs {
		if spec.pkgName == pkgName {
			specsForPkg = append(specsForPkg, spec)
		} else {
			remaining = append(remaining, spec)
		}
	}
	cppe.pendingHashSpecs = remaining

	for _, spec := range specsForPkg {
		// Fully qualified name for non-main packages
		qualifiedName := spec.structName
		if spec.pkgName != "" {
			qualifiedName = spec.pkgName + "::" + spec.structName
		}

		// Generate operator== at global scope
		cppe.emitToFile(fmt.Sprintf("inline bool operator==(const %s& a, const %s& b) {\n", qualifiedName, qualifiedName))
		cppe.emitToFile("    return ")
		for i, fieldName := range spec.fieldNames {
			if i > 0 {
				cppe.emitToFile(" && ")
			}
			cppe.emitToFile(fmt.Sprintf("a.%s == b.%s", fieldName, fieldName))
		}
		if len(spec.fieldNames) == 0 {
			cppe.emitToFile("true")
		}
		cppe.emitToFile(";\n}\n\n")

		// Generate std::hash specialization at global scope
		cppe.emitToFile("namespace std {\n")
		cppe.emitToFile(fmt.Sprintf("    template<> struct hash<%s> {\n", qualifiedName))
		cppe.emitToFile(fmt.Sprintf("        size_t operator()(const %s& s) const {\n", qualifiedName))
		cppe.emitToFile("            size_t h = 0;\n")
		for _, fieldName := range spec.fieldNames {
			cppe.emitToFile(fmt.Sprintf("            h ^= std::hash<decltype(s.%s)>{}(s.%s) + 0x9e3779b9 + (h << 6) + (h >> 2);\n", fieldName, fieldName))
		}
		cppe.emitToFile("            return h;\n")
		cppe.emitToFile("        }\n")
		cppe.emitToFile("    };\n")
		cppe.emitToFile("}\n\n")
	}
}

func (cppe *CPPEmitter) PreVisitFuncDeclSignatures(indent int) {
	// Generate forward function declarations
	str := cppe.emitAsString("// Forward declarations\n", 0)
	cppe.emitToFile(str)
	cppe.forwardDecl = true
}

func (cppe *CPPEmitter) PostVisitFuncDeclSignatures(indent int) {
	str := cppe.emitAsString("\n", 0)
	cppe.emitToFile(str)
	cppe.forwardDecl = false
}

func (cppe *CPPEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {
	str := cppe.emitAsString("\n", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	cppe.emitToFile(" ")
}

func (cppe *CPPEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	cppe.emitToFile(";\n")
}

func (cppe *CPPEmitter) PreVisitGenDeclConstName(node *ast.Ident, indent int) {
	str := cppe.emitAsString(fmt.Sprintf("constexpr auto %s = ", node.Name), 0)
	cppe.emitToFile(str)
}
func (cppe *CPPEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	str := cppe.emitAsString(";\n", 0)
	cppe.emitToFile(str)
}

func (cppe *CPPEmitter) PreVisitTypeAliasName(node *ast.Ident, indent int) {
	cppe.emitToFile(fmt.Sprintf("using "))
}

func (cppe *CPPEmitter) PostVisitTypeAliasName(node *ast.Ident, indent int) {
	cppe.emitToFile(" = ")
}

func (cppe *CPPEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	cppe.emitToFile(";\n\n")
}

// GenerateMakefile creates a Makefile for building the C++ project
func (cppe *CPPEmitter) GenerateMakefile() error {
	if cppe.LinkRuntime == "" {
		return nil
	}

	makefilePath := filepath.Join(cppe.OutputDir, "Makefile")
	file, err := os.Create(makefilePath)
	if err != nil {
		return fmt.Errorf("failed to create Makefile: %w", err)
	}
	defer file.Close()

	// Convert LinkRuntime to absolute path so Makefile works from any directory
	absRuntimePath, err := filepath.Abs(cppe.LinkRuntime)
	if err != nil {
		absRuntimePath = cppe.LinkRuntime // Fall back to original if Abs fails
	}

	var makefile string

	// Determine graphics backend from RuntimePackages
	graphicsBackend := cppe.RuntimePackages["graphics"]
	if graphicsBackend == "" {
		graphicsBackend = "none" // no graphics import detected
	}

	switch graphicsBackend {
	case "sdl2":
		makefile = fmt.Sprintf(`CXX = g++
CXXFLAGS = -O3 -fwrapv -std=c++17 -I%s $(shell sdl2-config --cflags)
LDFLAGS = $(shell sdl2-config --libs)

TARGET = %s
SRCS = %s.cpp

all: $(TARGET)

$(TARGET): $(SRCS)
	$(CXX) $(CXXFLAGS) -o $@ $^ $(LDFLAGS)

clean:
	rm -f $(TARGET)

.PHONY: all clean
`, absRuntimePath, cppe.OutputName, cppe.OutputName)

	case "none":
		makefile = fmt.Sprintf(`CXX = g++
CXXFLAGS = -O3 -fwrapv -std=c++17 -I%s
LDFLAGS =

TARGET = %s
SRCS = %s.cpp

all: $(TARGET)

$(TARGET): $(SRCS)
	$(CXX) $(CXXFLAGS) -o $@ $^ $(LDFLAGS)

clean:
	rm -f $(TARGET)

.PHONY: all clean
`, absRuntimePath, cppe.OutputName, cppe.OutputName)

	default: // tigr (default)
		makefile = fmt.Sprintf(`CXX = g++
CC = gcc
CXXFLAGS = -O3 -fwrapv -std=c++17 -I%s
CFLAGS = -O3

TARGET = %s
SRCS = %s.cpp
TIGR_SRC = %s/graphics/cpp/tigr.c
SCREEN_SRC = %s/graphics/cpp/screen_helper.c

# Platform-specific flags
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Darwin)
    LDFLAGS = -framework OpenGL -framework Cocoa -framework CoreGraphics
endif
ifeq ($(UNAME_S),Linux)
    LDFLAGS = -lGL -lX11
endif
# Windows (MinGW)
ifeq ($(OS),Windows_NT)
    LDFLAGS = -lopengl32 -lgdi32 -luser32 -lshell32 -ladvapi32
endif

all: $(TARGET)

tigr.o: $(TIGR_SRC)
	$(CC) $(CFLAGS) -c $< -o $@

screen_helper.o: $(SCREEN_SRC)
	$(CC) $(CFLAGS) -c $< -o $@

$(TARGET): $(SRCS) tigr.o screen_helper.o
	$(CXX) $(CXXFLAGS) -o $@ $(SRCS) tigr.o screen_helper.o $(LDFLAGS)

clean:
	rm -f $(TARGET) tigr.o screen_helper.o

.PHONY: all clean
`, absRuntimePath, cppe.OutputName, cppe.OutputName, absRuntimePath, absRuntimePath)
	}

	// Add platform-specific linker flags if HTTP or NET runtime is used
	// These need -pthread on Unix and -lws2_32 on Windows
	_, hasHTTP := cppe.RuntimePackages["http"]
	_, hasNet := cppe.RuntimePackages["net"]
	if hasHTTP || hasNet {
		networkFlags := `
# Network runtime linker flags
ifeq ($(OS),Windows_NT)
    LDFLAGS += -lws2_32 -pthread
else
    LDFLAGS += -pthread
endif
`
		makefile = strings.Replace(makefile, "\nall:", networkFlags+"\nall:", 1)
	}

	_, err = file.WriteString(makefile)
	if err != nil {
		return fmt.Errorf("failed to write Makefile: %w", err)
	}

	DebugLogPrintf("Generated Makefile at %s (graphics: %s)", makefilePath, graphicsBackend)
	return nil
}
