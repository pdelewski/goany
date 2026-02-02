package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
	"log"
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
	"float32": "float",
	"float64": "double",
	"any":     "std::any",
	"string":  "std::string",
}

type CPPEmitter struct {
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string // Path to runtime directory (empty = disabled)
	GraphicsRuntime string // Graphics backend: tigr (default), sdl2, none
	OptimizeMoves   bool   // Enable move optimizations to reduce struct cloning
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
	mapIndexKeyIsString    bool   // Map key type is string (needs std::string wrap)
	isMapAssign            bool   // Assignment to map index (m[k] = v)
	mapAssignVarName       string // Variable name for map assignment
	mapAssignIndent        int    // Indent level for map assignment
	mapAssignKeyIsString   bool   // Key is string type
	mapAssignValueIsString bool   // Value is string type
	captureMapKey          bool   // Capturing map key expression
	capturedMapKey         string // Captured key expression text
	isDeleteCall           bool   // Inside delete(m, k) call
	deleteMapVarName       string // Variable name for delete
	deleteKeyIsString      bool   // Delete key is string type
	isMapLenCall           bool   // len() call on a map
	pendingMapInit         bool   // var m map[K]V needs default init
	pendingMapKeyType      int    // Key type for pending init
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
	mapCommaOkKeyIsString bool
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
	if named, ok := t.(*types.Named); ok {
		if _, isStruct := named.Underlying().(*types.Struct); isStruct {
			return named.Obj().Name()
		}
	}
	return "std::any"
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
	// Include runtime header if link-runtime is enabled
	if cppe.LinkRuntime != "" {
		// Include graphics runtime based on selected backend
		graphicsBackend := cppe.GraphicsRuntime
		if graphicsBackend == "" {
			graphicsBackend = "tigr"
		}
		switch graphicsBackend {
		case "sdl2":
			cppe.file.WriteString("#include \"graphics/cpp/graphics_runtime_sdl2.hpp\"\n")
		case "tigr":
			cppe.file.WriteString("#include \"graphics/cpp/graphics_runtime_tigr.hpp\"\n")
		case "none":
			// No graphics runtime
		}
	}
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
	cppe.file.Close()

	// Generate Makefile if link-runtime is enabled
	if err := cppe.GenerateMakefile(); err != nil {
		log.Printf("Warning: %v", err)
	}

	if cppe.OptimizeMoves && cppe.MoveOptCount > 0 {
		fmt.Printf("  C++: %d copy(ies) replaced by std::move()\n", cppe.MoveOptCount)
	}
}

func (cppe *CPPEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	cppe.pkg = pkg
	name := pkg.Name
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
			if mapIdent, ok := node.Args[0].(*ast.Ident); ok {
				cppe.isDeleteCall = true
				cppe.deleteMapVarName = mapIdent.Name
				cppe.deleteKeyIsString = false
				// Check if map key type is string
				if cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
					tv := cppe.pkg.TypesInfo.Types[node.Args[0]]
					if tv.Type != nil {
						if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
							if basic, isBasic := mapType.Key().Underlying().(*types.Basic); isBasic && basic.Kind() == types.String {
								cppe.deleteKeyIsString = true
							}
						}
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
		cppe.deleteKeyIsString = false
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
	// Wrap delete key argument with std::string() for string-key maps
	if cppe.isDeleteCall && cppe.deleteKeyIsString && index == 1 {
		cppe.emitToFile("std::string(")
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
	if cppe.isDeleteCall && cppe.deleteKeyIsString && index == 1 {
		cppe.emitToFile(")")
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
	// Check if this is a map index (not a map assignment - that's handled separately)
	if !cppe.isMapAssign && cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
		tv := cppe.pkg.TypesInfo.Types[node.X]
		if tv.Type != nil {
			if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
				cppe.isMapIndex = true
				cppe.mapIndexValueCppType = getCppTypeName(mapType.Elem())
				keyType := mapType.Key()
				if basic, isBasic := keyType.Underlying().(*types.Basic); isBasic && basic.Kind() == types.String {
					cppe.mapIndexKeyIsString = true
				}
				// Emit: std::any_cast<ValueType>(hmap::hashMapGet(
				cppe.emitToFile("std::any_cast<" + cppe.mapIndexValueCppType + ">(hmap::hashMapGet(")
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
	if cppe.isMapIndex {
		// Emit ", " (or ", std::string(" for string keys) instead of "["
		if cppe.mapIndexKeyIsString {
			cppe.emitToFile(", std::string(")
		} else {
			cppe.emitToFile(", ")
		}
		return
	}
	if cppe.isMapAssign {
		// Capture the key expression
		cppe.captureMapKey = true
		cppe.capturedMapKey = ""
		return
	}
	str := cppe.emitAsString("[", 0)
	cppe.emitToFile(str)
}
func (cppe *CPPEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	if cppe.isMapCommaOk {
		cppe.captureMapKey = false
		return
	}
	if cppe.isMapIndex {
		// Close std::string wrapper if needed, then close hashMapGet and any_cast
		if cppe.mapIndexKeyIsString {
			cppe.emitToFile(")")
		}
		cppe.emitToFile("))")
		cppe.isMapIndex = false
		cppe.mapIndexValueCppType = ""
		cppe.mapIndexKeyIsString = false
		return
	}
	if cppe.captureMapKey {
		cppe.captureMapKey = false
		return
	}
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
	cppe.emitToFile(";")
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
						if ident, ok := indexExpr.X.(*ast.Ident); ok {
							cppe.mapCommaOkMapName = ident.Name
						}
						cppe.mapCommaOkValueType = getCppTypeName(mapType.Elem())
						cppe.mapCommaOkIsDecl = (node.Tok == token.DEFINE)
						cppe.mapCommaOkIndent = indent
						cppe.mapCommaOkKeyIsString = false
						keyType := mapType.Key()
						if basic, isBasic := keyType.Underlying().(*types.Basic); isBasic && basic.Kind() == types.String {
							cppe.mapCommaOkKeyIsString = true
						}
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
	// Detect map assignment: m[k] = v
	if len(node.Lhs) == 1 {
		if indexExpr, ok := node.Lhs[0].(*ast.IndexExpr); ok {
			if cppe.pkg != nil && cppe.pkg.TypesInfo != nil {
				tv := cppe.pkg.TypesInfo.Types[indexExpr.X]
				if tv.Type != nil {
					if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
						cppe.isMapAssign = true
						cppe.mapAssignIndent = indent
						cppe.mapAssignKeyIsString = false
						cppe.mapAssignValueIsString = false
						if ident, ok := indexExpr.X.(*ast.Ident); ok {
							cppe.mapAssignVarName = ident.Name
						}
						keyType := mapType.Key()
						if basic, isBasic := keyType.Underlying().(*types.Basic); isBasic && basic.Kind() == types.String {
							cppe.mapAssignKeyIsString = true
						}
						valType := mapType.Elem()
						if basic, isBasic := valType.Underlying().(*types.Basic); isBasic && basic.Kind() == types.String {
							cppe.mapAssignValueIsString = true
						}
						// Suppress normal LHS output (the m[k] part will be captured)
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
	if cppe.isMapCommaOk {
		key := cppe.capturedMapKey
		if cppe.mapCommaOkKeyIsString {
			key = "std::string(" + key + ")"
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
		cppe.isMapAssign = false
		cppe.mapAssignVarName = ""
		cppe.capturedMapKey = ""
		cppe.mapAssignKeyIsString = false
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
	if cppe.isMapAssign {
		// Turn off suppression and emit: m = hmap::hashMapSet(m, key, value)
		cppe.suppressEmit = false
		key := cppe.capturedMapKey
		if cppe.mapAssignKeyIsString {
			key = "std::string(" + key + ")"
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

	// Determine graphics backend (default to tigr)
	graphicsBackend := cppe.GraphicsRuntime
	if graphicsBackend == "" {
		graphicsBackend = "tigr"
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

	_, err = file.WriteString(makefile)
	if err != nil {
		return fmt.Errorf("failed to write Makefile: %w", err)
	}

	DebugLogPrintf("Generated Makefile at %s (graphics: %s)", makefilePath, graphicsBackend)
	return nil
}
