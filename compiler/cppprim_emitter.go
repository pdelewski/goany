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

// CPPPrimEmitter implements the Emitter interface using a shift/reduce architecture
// for C++ code generation. Mirrors the JSEmitter pattern from js_emitter.go.
type CPPPrimEmitter struct {
	fs              *FragmentStack
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string
	RuntimePackages map[string]string
	OptimizeMoves   bool
	MoveOptCount    int
	file            *os.File
	Emitter
	pkg            *packages.Package
	currentPackage string
	inNamespace    bool
	indent         int
	numFuncResults int
	// Map assignment detection (minimal, same as JS)
	lastIndexXCode   string
	lastIndexKeyCode string
	mapAssignVar     string
	mapAssignKey     string
	structKeyTypes   map[string]string
	// For loop components (stacks for nesting support)
	forInitStack []string
	forCondStack []string
	forPostStack []string
	// If statement components (stacks for nesting support)
	ifInitStack []string
	ifCondStack []string
	ifBodyStack []string
	ifElseStack []string
	// Move optimization guards
	currentAssignLhsNames     map[string]bool
	currentCallArgIdentsStack []map[string]int
	// C++-specific
	forwardDecl      bool
	funcLitDepth     int
	nestedMapCounter int
	pendingHashSpecs []pendingHashSpec
}

func (e *CPPPrimEmitter) SetFile(file *os.File) { e.file = file }
func (e *CPPPrimEmitter) GetFile() *os.File     { return e.file }

// canMoveArg checks if a call argument identifier can be moved instead of copied.
// A move is safe only when: (1) the variable is the LHS of the enclosing assignment
// (so it gets reassigned immediately), and (2) it doesn't appear multiple times
// in the same call's arguments.
func (e *CPPPrimEmitter) canMoveArg(varName string) bool {
	if !e.OptimizeMoves {
		return false
	}
	if e.funcLitDepth > 0 {
		return false
	}
	if e.currentAssignLhsNames == nil || !e.currentAssignLhsNames[varName] {
		return false
	}
	if len(e.currentCallArgIdentsStack) > 0 {
		outermostCounts := e.currentCallArgIdentsStack[0]
		if outermostCounts[varName] > 1 {
			return false
		}
	}
	e.MoveOptCount++
	return true
}

func (e *CPPPrimEmitter) collectCallArgIdentCounts(args []ast.Expr) map[string]int {
	counts := make(map[string]int)
	for _, arg := range args {
		e.countIdentsInExpr(arg, counts)
	}
	return counts
}

func (e *CPPPrimEmitter) countIdentsInExpr(expr ast.Expr, counts map[string]int) {
	if expr == nil {
		return
	}
	switch n := expr.(type) {
	case *ast.Ident:
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if obj := e.pkg.TypesInfo.ObjectOf(n); obj != nil {
				if _, isTypeName := obj.(*types.TypeName); isTypeName {
					return
				}
				if _, isPkgName := obj.(*types.PkgName); isPkgName {
					return
				}
			}
		}
		counts[n.Name]++
	case *ast.SelectorExpr:
		e.countIdentsInExpr(n.X, counts)
	case *ast.CallExpr:
		for _, arg := range n.Args {
			e.countIdentsInExpr(arg, counts)
		}
		e.countIdentsInExpr(n.Fun, counts)
	case *ast.IndexExpr:
		e.countIdentsInExpr(n.X, counts)
		e.countIdentsInExpr(n.Index, counts)
	case *ast.BinaryExpr:
		e.countIdentsInExpr(n.X, counts)
		e.countIdentsInExpr(n.Y, counts)
	case *ast.UnaryExpr:
		e.countIdentsInExpr(n.X, counts)
	case *ast.ParenExpr:
		e.countIdentsInExpr(n.X, counts)
	case *ast.TypeAssertExpr:
		e.countIdentsInExpr(n.X, counts)
	case *ast.CompositeLit:
		for _, elt := range n.Elts {
			e.countIdentsInExpr(elt, counts)
		}
	case *ast.KeyValueExpr:
		e.countIdentsInExpr(n.Value, counts)
	}
}

// analyzeLhsIndexChain walks a chain of IndexExpr from outermost to root variable,
// returning the operations and whether there's an intermediate map access that needs
// read-modify-write semantics.
func (e *CPPPrimEmitter) analyzeLhsIndexChain(expr ast.Expr) (ops []mixedIndexOp, hasIntermediateMap bool) {
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
	// Reverse: chain[0] = innermost (closest to root), chain[len-1] = outermost
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	hasIntermediateMap = false
	for i, node := range chain {
		ie := node.(*ast.IndexExpr)
		tv := e.pkg.TypesInfo.Types[ie.X]
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

// cppIndent returns indentation string for the given level.
func cppIndent(indent int) string {
	return strings.Repeat("    ", indent/4)
}

// isMapTypeExprCpp checks if an expression has map type via TypesInfo.
func (e *CPPPrimEmitter) isMapTypeExpr(expr ast.Expr) bool {
	if e.pkg == nil || e.pkg.TypesInfo == nil {
		return false
	}
	tv := e.pkg.TypesInfo.Types[expr]
	if tv.Type == nil {
		return false
	}
	_, ok := tv.Type.Underlying().(*types.Map)
	return ok
}

// getExprGoType returns the Go type for an expression, or nil.
func (e *CPPPrimEmitter) getExprGoType(expr ast.Expr) types.Type {
	if e.pkg == nil || e.pkg.TypesInfo == nil {
		return nil
	}
	tv := e.pkg.TypesInfo.Types[expr]
	return tv.Type
}

// cppLowerBuiltin maps Go stdlib selectors to C++ equivalents.
func cppLowerBuiltin(selector string) string {
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

// getMapKeyTypeConstPrim returns the key type constant for a map type AST node
func (e *CPPPrimEmitter) getMapKeyTypeConst(mapType *ast.MapType) int {
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
					qualifiedName := structName
					if structPkg := named.Obj().Pkg(); structPkg != nil {
						pkgName := structPkg.Name()
						if pkgName != "" && pkgName != "main" {
							qualifiedName = pkgName + "::" + structName
						}
					}
					e.structKeyTypes[structName] = qualifiedName
				}
				return 100
			}
		}
	}
	return 1
}

// exprContainsIdent checks if an expression references a given identifier
func (e *CPPPrimEmitter) exprContainsIdent(expr ast.Expr, name string) bool {
	if expr == nil {
		return false
	}
	switch n := expr.(type) {
	case *ast.Ident:
		return n.Name == name
	case *ast.SelectorExpr:
		return e.exprContainsIdent(n.X, name)
	case *ast.CallExpr:
		for _, arg := range n.Args {
			if e.exprContainsIdent(arg, name) {
				return true
			}
		}
		return e.exprContainsIdent(n.Fun, name)
	case *ast.IndexExpr:
		return e.exprContainsIdent(n.X, name) || e.exprContainsIdent(n.Index, name)
	case *ast.BinaryExpr:
		return e.exprContainsIdent(n.X, name) || e.exprContainsIdent(n.Y, name)
	case *ast.UnaryExpr:
		return e.exprContainsIdent(n.X, name)
	case *ast.ParenExpr:
		return e.exprContainsIdent(n.X, name)
	case *ast.TypeAssertExpr:
		return e.exprContainsIdent(n.X, name)
	case *ast.CompositeLit:
		for _, elt := range n.Elts {
			if e.exprContainsIdent(elt, name) {
				return true
			}
		}
	case *ast.KeyValueExpr:
		return e.exprContainsIdent(n.Value, name)
	}
	return false
}

// ============================================================
// Program / Package
// ============================================================

func (e *CPPPrimEmitter) PreVisitProgram(indent int) {
	var err error
	e.file, err = os.Create(e.Output)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}

	e.fs = NewFragmentStack(e.GetGoFIR())

	// Write C++ header
	e.file.WriteString("#include <vector>\n" +
		"#include <string>\n" +
		"#include <tuple>\n" +
		"#include <any>\n" +
		"#include <cstdint>\n" +
		"#include <functional>\n")
	e.file.WriteString("#include <cstdarg> // For va_start, etc.\n" +
		"#include <initializer_list>\n" +
		"#include <iostream>\n")

	// Include runtime headers
	if e.LinkRuntime != "" {
		for name, variant := range e.RuntimePackages {
			if variant == "none" {
				continue
			}
			fileName := name + "_runtime"
			if variant != "" {
				fileName += "_" + variant
			}
			e.file.WriteString(fmt.Sprintf("#include \"%s/cpp/%s.hpp\"\n", name, fileName))
		}
	}

	// Include panic runtime
	e.file.WriteString("\n// GoAny panic runtime\n")
	e.file.WriteString(goanyrt.PanicCppSource)
	e.file.WriteString("\n")

	// C++ helper definitions
	e.file.WriteString(`
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
	e.file.WriteString("\n\n")
}

func (e *CPPPrimEmitter) PostVisitProgram(indent int) {
	tokens := e.fs.Reduce(string(PreVisitProgram))
	for _, t := range tokens {
		e.file.WriteString(t.Content)
	}

	// Emit pending hash specializations for main package
	e.emitPendingHashSpecs("")

	e.file.Close()

	// Replace placeholder struct key functions
	if len(e.structKeyTypes) > 0 {
		e.replaceStructKeyFunctions()
	}

	// Generate Makefile
	if err := e.GenerateMakefile(); err != nil {
		log.Printf("Warning: %v", err)
	}

	if e.OptimizeMoves && e.MoveOptCount > 0 {
		fmt.Printf("  C++ (prim): %d copy(ies) replaced by std::move()\n", e.MoveOptCount)
	}
}

func (e *CPPPrimEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	e.pkg = pkg
	e.currentPackage = pkg.Name
	if e.structKeyTypes == nil {
		e.structKeyTypes = make(map[string]string)
	}
	if pkg.Name != "main" {
		e.inNamespace = true
		e.fs.PushCode(fmt.Sprintf("namespace %s\n{\n\n", pkg.Name))
	}
}

func (e *CPPPrimEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	if pkg.Name != "main" {
		e.fs.PushCode(fmt.Sprintf("} // namespace %s\n\n", pkg.Name))
		e.inNamespace = false

		// Emit pending hash specializations for this package
		var specsForPkg []pendingHashSpec
		var remaining []pendingHashSpec
		for _, spec := range e.pendingHashSpecs {
			if spec.pkgName == pkg.Name {
				specsForPkg = append(specsForPkg, spec)
			} else {
				remaining = append(remaining, spec)
			}
		}
		e.pendingHashSpecs = remaining
		for _, spec := range specsForPkg {
			e.emitHashSpec(spec)
		}
	}
}

// ============================================================
// Literals and Identifiers
// ============================================================

func (e *CPPPrimEmitter) PreVisitBasicLit(node *ast.BasicLit, indent int) {
	val := node.Value
	if node.Kind == token.STRING {
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			// Regular string
			e.fs.Push(val, TagLiteral, nil)
		} else if len(val) >= 2 && val[0] == '`' && val[len(val)-1] == '`' {
			// Raw string → C++ raw string
			content := val[1 : len(val)-1]
			e.fs.Push(fmt.Sprintf("R\"(%s)\"", content), TagLiteral, nil)
		} else {
			e.fs.Push(val, TagLiteral, nil)
		}
	} else {
		e.fs.Push(val, TagLiteral, nil)
	}
}

func (e *CPPPrimEmitter) PreVisitIdent(node *ast.Ident, indent int) {
	name := node.Name
	// Map Go builtins
	switch name {
	case "true", "false":
		e.fs.Push(name, TagLiteral, nil)
		return
	case "nil":
		e.fs.Push("{}", TagLiteral, nil)
		return
	}
	// Map Go types to C++ types
	if n, ok := cppTypesMap[name]; ok {
		e.fs.Push(n, TagType, nil)
		return
	}
	goType := e.getExprGoType(node)
	e.fs.Push(name, TagIdent, goType)
}

// ============================================================
// Binary Expressions
// ============================================================

func (e *CPPPrimEmitter) PostVisitBinaryExprLeft(node ast.Expr, indent int) {
	left := e.fs.ReduceToCode(string(PreVisitBinaryExprLeft))
	e.fs.PushCode(left)
}

func (e *CPPPrimEmitter) PostVisitBinaryExprRight(node ast.Expr, indent int) {
	right := e.fs.ReduceToCode(string(PreVisitBinaryExprRight))
	e.fs.PushCode(right)
}

func (e *CPPPrimEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitBinaryExpr))
	left := ""
	right := ""
	if len(tokens) >= 1 {
		left = tokens[0].Content
	}
	if len(tokens) >= 2 {
		right = tokens[1].Content
	}
	op := node.Op.String()
	e.fs.PushCode(fmt.Sprintf("%s %s %s", left, op, right))
}

// ============================================================
// Call Expressions
// ============================================================

func (e *CPPPrimEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	funCode := e.fs.ReduceToCode(string(PreVisitCallExprFun))
	e.fs.PushCode(funCode)
}

func (e *CPPPrimEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {
	argCode := e.fs.ReduceToCode(string(PreVisitCallExprArg))

	// Move optimization for struct arguments
	if e.OptimizeMoves && e.funcLitDepth == 0 {
		if ident, isIdent := node.(*ast.Ident); isIdent {
			tv := e.getExprGoType(node)
			if tv != nil {
				if named, ok := tv.(*types.Named); ok {
					if _, isStruct := named.Underlying().(*types.Struct); isStruct {
						if e.canMoveArg(ident.Name) {
							argCode = "std::move(" + argCode + ")"
						}
					}
				}
			}
		}
	}

	e.fs.PushCode(argCode)
}

func (e *CPPPrimEmitter) PreVisitCallExprArgs(node []ast.Expr, indent int) {
	e.currentCallArgIdentsStack = append(e.currentCallArgIdentsStack, e.collectCallArgIdentCounts(node))
}

func (e *CPPPrimEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	if len(e.currentCallArgIdentsStack) > 0 {
		e.currentCallArgIdentsStack = e.currentCallArgIdentsStack[:len(e.currentCallArgIdentsStack)-1]
	}
	argTokens := e.fs.Reduce(string(PreVisitCallExprArgs))
	var args []string
	for _, t := range argTokens {
		if t.Content != "" {
			args = append(args, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(args, ", "))
}

func (e *CPPPrimEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCallExpr))
	funName := ""
	argsStr := ""
	if len(tokens) >= 1 {
		funName = tokens[0].Content
	}
	if len(tokens) >= 2 {
		argsStr = tokens[1].Content
	}

	// Handle special built-in functions
	switch funName {
	case "len":
		// len(m) on maps → hmap::hashMapLen(m)
		if len(node.Args) > 0 && e.isMapTypeExpr(node.Args[0]) {
			e.fs.PushCode(fmt.Sprintf("hmap::hashMapLen(%s)", argsStr))
		} else {
			e.fs.PushCode(fmt.Sprintf("std::size(%s)", argsStr))
		}
		return
	case "append":
		e.fs.PushCode(fmt.Sprintf("append(%s)", argsStr))
		return
	case "delete":
		// delete(m, k) → m = hmap::hashMapDelete(m, k)
		if len(node.Args) >= 2 {
			mapName := exprToString(node.Args[0])
			// Get key cast
			castPfx, castSfx := "", ""
			if e.pkg != nil && e.pkg.TypesInfo != nil {
				tv := e.pkg.TypesInfo.Types[node.Args[0]]
				if tv.Type != nil {
					if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
						castPfx, castSfx = getCppKeyCast(mapType.Key())
					}
				}
			}
			parts := strings.SplitN(argsStr, ", ", 2)
			keyStr := ""
			if len(parts) >= 2 {
				keyStr = parts[1]
			}
			if castPfx != "" {
				keyStr = castPfx + keyStr + castSfx
			}
			e.fs.PushCode(fmt.Sprintf("%s = hmap::hashMapDelete(%s, %s)", mapName, mapName, keyStr))
		} else {
			e.fs.PushCode(fmt.Sprintf("delete(%s)", argsStr))
		}
		return
	case "make":
		if len(node.Args) >= 1 {
			if mapType, ok := node.Args[0].(*ast.MapType); ok {
				keyTypeConst := e.getMapKeyTypeConst(mapType)
				e.fs.PushCode(fmt.Sprintf("hmap::newHashMap(%d)", keyTypeConst))
				return
			}
			if _, ok := node.Args[0].(*ast.ArrayType); ok {
				// make([]T, n) → std::vector<CppType>(n)
				cppType := "std::vector<std::any>"
				if e.pkg != nil && e.pkg.TypesInfo != nil {
					if arrayType, ok := node.Args[0].(*ast.ArrayType); ok {
						if tv, ok2 := e.pkg.TypesInfo.Types[arrayType.Elt]; ok2 && tv.Type != nil {
							elemType := getCppTypeName(tv.Type)
							cppType = "std::vector<" + elemType + ">"
						}
					}
				}
				parts := strings.SplitN(argsStr, ", ", 2)
				if len(parts) >= 2 {
					e.fs.PushCode(fmt.Sprintf("%s(%s)", cppType, parts[1]))
				} else {
					e.fs.PushCode(cppType + "()")
				}
				return
			}
		}
		e.fs.PushCode(fmt.Sprintf("make(%s)", argsStr))
		return
	case "goany_panic":
		e.fs.PushCode(fmt.Sprintf("goany_panic(%s)", argsStr))
		return
	}

	// Lower builtins
	lowered := cppLowerBuiltin(funName)
	if lowered != funName {
		funName = lowered
	}

	e.fs.PushCode(fmt.Sprintf("%s(%s)", funName, argsStr))
}

// ============================================================
// Selector Expressions (a.b)
// ============================================================

func (e *CPPPrimEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitSelectorExprX))
	e.fs.PushCode(xCode)
}

func (e *CPPPrimEmitter) PostVisitSelectorExprSel(node *ast.Ident, indent int) {
	e.fs.Reduce(string(PreVisitSelectorExprSel))
	e.fs.PushCode(node.Name)
}

func (e *CPPPrimEmitter) PostVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSelectorExpr))
	xCode := ""
	selCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Content
	}
	if len(tokens) >= 2 {
		selCode = tokens[1].Content
	}

	loweredX := cppLowerBuiltin(xCode)
	loweredSel := cppLowerBuiltin(selCode)

	if loweredX == "" {
		// Package selector like fmt is suppressed
		e.fs.PushCode(loweredSel)
	} else {
		// Use :: for namespaces, . for struct fields
		scopeOp := "."
		if _, found := namespaces[xCode]; found {
			scopeOp = "::"
		}
		e.fs.PushCode(loweredX + scopeOp + loweredSel)
	}
}

// ============================================================
// Index Expressions (a[i])
// ============================================================

func (e *CPPPrimEmitter) PostVisitIndexExprX(node *ast.IndexExpr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitIndexExprX))
	e.fs.PushCode(xCode)
	e.lastIndexXCode = xCode
}

func (e *CPPPrimEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	idxCode := e.fs.ReduceToCode(string(PreVisitIndexExprIndex))
	e.fs.PushCode(idxCode)
	e.lastIndexKeyCode = idxCode
}

func (e *CPPPrimEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitIndexExpr))
	xCode := ""
	idxCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Content
	}
	if len(tokens) >= 2 {
		idxCode = tokens[1].Content
	}

	// Check if this is a map index
	if e.isMapTypeExpr(node.X) {
		mapGoType := e.getExprGoType(node.X)
		valueCppType := "std::any"
		castPfx, castSfx := "", ""
		if mapGoType != nil {
			if mapType, ok := mapGoType.Underlying().(*types.Map); ok {
				valueCppType = getCppTypeName(mapType.Elem())
				castPfx, castSfx = getCppKeyCast(mapType.Key())
			}
		}
		keyStr := idxCode
		if castPfx != "" {
			keyStr = castPfx + idxCode + castSfx
		}
		e.fs.PushCodeWithType(
			fmt.Sprintf("std::any_cast<%s>(hmap::hashMapGet(%s, %s))", valueCppType, xCode, keyStr),
			e.getExprGoType(node))
	} else {
		e.fs.PushCode(fmt.Sprintf("%s[%s]", xCode, idxCode))
	}
}

// ============================================================
// Unary Expressions
// ============================================================

func (e *CPPPrimEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitUnaryExpr))
	op := node.Op.String()
	if op == "^" {
		e.fs.PushCode("~" + xCode)
	} else {
		e.fs.PushCode("(" + op + xCode + ")")
	}
}

// ============================================================
// Paren Expressions
// ============================================================

func (e *CPPPrimEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {
	inner := e.fs.ReduceToCode(string(PreVisitParenExpr))
	e.fs.PushCode("(" + inner + ")")
}

// ============================================================
// Slice Expressions (a[lo:hi])
// ============================================================

func (e *CPPPrimEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitSliceExprX))
	e.fs.PushCode(xCode)
}

func (e *CPPPrimEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitSliceExprXBegin))
}

func (e *CPPPrimEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	lowCode := e.fs.ReduceToCode(string(PreVisitSliceExprLow))
	e.fs.PushCode(lowCode)
}

func (e *CPPPrimEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitSliceExprXEnd))
}

func (e *CPPPrimEmitter) PostVisitSliceExprHigh(node ast.Expr, indent int) {
	highCode := e.fs.ReduceToCode(string(PreVisitSliceExprHigh))
	e.fs.PushCode(highCode)
}

func (e *CPPPrimEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSliceExpr))
	xCode := ""
	lowCode := ""
	highCode := ""

	idx := 0
	if idx < len(tokens) {
		xCode = tokens[idx].Content
		idx++
	}
	if node.Low != nil && idx < len(tokens) {
		lowCode = tokens[idx].Content
		idx++
	}
	if node.High != nil && idx < len(tokens) {
		highCode = tokens[idx].Content
	}

	if lowCode == "" {
		lowCode = "0"
	}

	// C++ slice: std::vector<T>(x.begin() + low, x.begin() + high) or x.end()
	beginExpr := fmt.Sprintf("%s.begin() + %s", xCode, lowCode)
	endExpr := ""
	if highCode == "" {
		endExpr = fmt.Sprintf("%s.end()", xCode)
	} else {
		endExpr = fmt.Sprintf("%s.begin() + %s", xCode, highCode)
	}
	e.fs.PushCode(fmt.Sprintf("std::vector<std::remove_reference<decltype(%s[0])>::type>(%s, %s)", xCode, beginExpr, endExpr))
}

// ============================================================
// Composite Literals
// ============================================================

func (e *CPPPrimEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitCompositeLitType))
}

func (e *CPPPrimEmitter) PostVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	eltCode := e.fs.ReduceToCode(string(PreVisitCompositeLitElt))
	e.fs.PushCode(eltCode)
}

func (e *CPPPrimEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	eltTokens := e.fs.Reduce(string(PreVisitCompositeLitElts))
	for _, t := range eltTokens {
		if t.Content != "" {
			e.fs.Push(t.Content, TagLiteral, nil)
		}
	}
}

func (e *CPPPrimEmitter) PostVisitCompositeLit(node *ast.CompositeLit, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCompositeLit))
	var elts []string
	for _, t := range tokens {
		if t.Content != "" {
			elts = append(elts, t.Content)
		}
	}

	litType := e.getExprGoType(node)
	if litType == nil {
		// Try to get type name from AST
		typeName := ""
		if node.Type != nil {
			typeName = exprToString(node.Type)
			typeName = strings.ReplaceAll(typeName, ".", "::")
		}
		if typeName != "" {
			e.fs.PushCode(fmt.Sprintf("%s{%s}", typeName, strings.Join(elts, ", ")))
		} else {
			e.fs.PushCode("{" + strings.Join(elts, ", ") + "}")
		}
		return
	}

	switch u := litType.Underlying().(type) {
	case *types.Struct:
		typeName := ""
		if node.Type != nil {
			typeName = exprToString(node.Type)
		}
		if typeName == "" {
			typeName = "auto"
		}
		// Convert package.Type to package::Type for C++
		typeName = strings.ReplaceAll(typeName, ".", "::")
		// Check for KeyValueExpr (named fields)
		if len(node.Elts) > 0 {
			if _, isKV := node.Elts[0].(*ast.KeyValueExpr); isKV {
				// Build ordered args from KV pairs
				kvMap := make(map[string]string)
				for _, elt := range elts {
					parts := strings.SplitN(elt, " = ", 2)
					if len(parts) == 2 {
						key := strings.TrimPrefix(parts[0], ".")
						kvMap[key] = parts[1]
					}
				}
				var args []string
				for i := 0; i < u.NumFields(); i++ {
					fieldName := u.Field(i).Name()
					if val, ok := kvMap[fieldName]; ok {
						args = append(args, val)
					} else {
						args = append(args, "{}")
					}
				}
				e.fs.PushCode(fmt.Sprintf("%s{%s}", typeName, strings.Join(args, ", ")))
				return
			}
		}
		e.fs.PushCode(fmt.Sprintf("%s{%s}", typeName, strings.Join(elts, ", ")))

	case *types.Slice:
		elemCppType := getCppTypeName(u.Elem())
		e.fs.PushCode(fmt.Sprintf("std::vector<%s>{%s}", elemCppType, strings.Join(elts, ", ")))

	case *types.Map:
		keyTypeConst := 1
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			keyTypeConst = e.getMapKeyTypeConst(node.Type.(*ast.MapType))
		}
		if len(elts) == 0 {
			e.fs.PushCode(fmt.Sprintf("hmap::newHashMap(%d)", keyTypeConst))
		} else {
			// Map literal with elements
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("[&]() { auto _m = hmap::newHashMap(%d); ", keyTypeConst))
			castPfx, castSfx := "", ""
			if mapType, ok := litType.Underlying().(*types.Map); ok {
				castPfx, castSfx = getCppKeyCast(mapType.Key())
			}
			_ = u
			for _, elt := range elts {
				// Each element is ".key = value" from KeyValueExpr
				parts := strings.SplitN(elt, " = ", 2)
				if len(parts) == 2 {
					key := strings.TrimPrefix(parts[0], ".")
					valStr := parts[1]
					keyStr := key
					if castPfx != "" {
						keyStr = castPfx + key + castSfx
					}
					sb.WriteString(fmt.Sprintf("_m = hmap::hashMapSet(_m, %s, %s); ", keyStr, valStr))
				}
			}
			sb.WriteString("return _m; }()")
			e.fs.PushCode(sb.String())
		}

	default:
		e.fs.PushCode("{" + strings.Join(elts, ", ") + "}")
	}
}

// ============================================================
// KeyValue Expressions
// ============================================================

func (e *CPPPrimEmitter) PostVisitKeyValueExprKey(node ast.Expr, indent int) {
	keyCode := e.fs.ReduceToCode(string(PreVisitKeyValueExprKey))
	e.fs.PushCode(keyCode)
}

func (e *CPPPrimEmitter) PostVisitKeyValueExprValue(node ast.Expr, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitKeyValueExprValue))
	e.fs.PushCode(valCode)
}

func (e *CPPPrimEmitter) PostVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitKeyValueExpr))
	keyCode := ""
	valCode := ""
	if len(tokens) >= 1 {
		keyCode = tokens[0].Content
	}
	if len(tokens) >= 2 {
		valCode = tokens[1].Content
	}
	// C++ designated initializer style: .Key = Value
	e.fs.PushCode("." + keyCode + " = " + valCode)
}

// ============================================================
// Array Type
// ============================================================

func (e *CPPPrimEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	tokens := e.fs.Reduce(string(PreVisitArrayType))
	elemType := ""
	for _, t := range tokens {
		elemType += t.Content
	}
	if elemType == "" {
		elemType = "std::any"
	}
	e.fs.Push("std::vector<"+elemType+">", TagType, nil)
}

// ============================================================
// Map Type
// ============================================================

func (e *CPPPrimEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitMapKeyType))
}

func (e *CPPPrimEmitter) PostVisitMapValueType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitMapValueType))
}

func (e *CPPPrimEmitter) PostVisitMapType(node *ast.MapType, indent int) {
	e.fs.Reduce(string(PreVisitMapType))
	e.fs.Push("hmap::HashMap", TagType, nil)
}

// ============================================================
// Function Type
// ============================================================

func (e *CPPPrimEmitter) PostVisitFuncTypeResults(node *ast.FieldList, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncTypeResults))
	resultType := "void"
	if node != nil && len(tokens) > 0 {
		var parts []string
		for _, t := range tokens {
			if t.Content != "" {
				parts = append(parts, t.Content)
			}
		}
		resultType = strings.Join(parts, ", ")
	}
	e.fs.PushCode(resultType)
}

func (e *CPPPrimEmitter) PostVisitFuncTypeResult(node *ast.Field, index int, indent int) {
	code := e.fs.ReduceToCode(string(PreVisitFuncTypeResult))
	e.fs.PushCode(code)
}

func (e *CPPPrimEmitter) PostVisitFuncTypeParams(node *ast.FieldList, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncTypeParams))
	var parts []string
	for _, t := range tokens {
		if t.Content != "" {
			parts = append(parts, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(parts, ", "))
}

func (e *CPPPrimEmitter) PostVisitFuncTypeParam(node *ast.Field, index int, indent int) {
	code := e.fs.ReduceToCode(string(PreVisitFuncTypeParam))
	e.fs.PushCode(code)
}

func (e *CPPPrimEmitter) PostVisitFuncType(node *ast.FuncType, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncType))
	resultType := "void"
	paramsType := ""
	if len(tokens) >= 1 {
		resultType = tokens[0].Content
	}
	if len(tokens) >= 2 {
		paramsType = tokens[1].Content
	}
	e.fs.PushCode(fmt.Sprintf("std::function<%s(%s)>", resultType, paramsType))
}

// ============================================================
// Interface / Star / Type Assertions
// ============================================================

func (e *CPPPrimEmitter) PreVisitInterfaceType(node *ast.InterfaceType, indent int) {
	e.fs.Push("std::any", TagType, nil)
}

func (e *CPPPrimEmitter) PreVisitStarExpr(node *ast.StarExpr, indent int) {
	// Pointers not supported in ULang, but emit * for completeness
}

func (e *CPPPrimEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	typeCode := e.fs.ReduceToCode(string(PreVisitTypeAssertExprType))
	e.fs.PushCode(typeCode)
}

func (e *CPPPrimEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitTypeAssertExprX))
	e.fs.PushCode(xCode)
}

func (e *CPPPrimEmitter) PostVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitTypeAssertExpr))
	typeCode := ""
	xCode := ""
	if len(tokens) >= 1 {
		typeCode = tokens[0].Content
	}
	if len(tokens) >= 2 {
		xCode = tokens[1].Content
	}
	// Check if expression is already interface{} (std::any)
	needsAnyWrap := true
	if e.pkg != nil && e.pkg.TypesInfo != nil {
		tv := e.pkg.TypesInfo.Types[node.X]
		if tv.Type != nil {
			if iface, ok := tv.Type.Underlying().(*types.Interface); ok && iface.Empty() {
				needsAnyWrap = false
			}
		}
	}
	if needsAnyWrap {
		e.fs.PushCode(fmt.Sprintf("std::any_cast<%s>(std::any(%s))", typeCode, xCode))
	} else {
		e.fs.PushCode(fmt.Sprintf("std::any_cast<%s>(%s)", typeCode, xCode))
	}
}

// ============================================================
// Function Literals (closures)
// ============================================================

func (e *CPPPrimEmitter) PreVisitFuncLit(node *ast.FuncLit, indent int) {
	e.funcLitDepth++
}

func (e *CPPPrimEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	// Get type and name tokens
	tokens := e.fs.Reduce(string(PreVisitFuncLitTypeParam))
	var typeStr string
	for _, t := range tokens {
		if t.Content != "" {
			typeStr = t.Content
		}
	}
	// Push "type name" pairs
	for _, name := range node.Names {
		e.fs.Push(typeStr+" "+name.Name, TagIdent, nil)
	}
}

func (e *CPPPrimEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncLitTypeParams))
	var paramStrs []string
	for _, t := range tokens {
		if t.Tag == TagIdent && t.Content != "" {
			paramStrs = append(paramStrs, t.Content)
		}
	}
	paramsStr := strings.Join(paramStrs, ", ")
	if paramsStr == "" {
		paramsStr = " "
	}
	e.fs.PushCode(paramsStr)
}

func (e *CPPPrimEmitter) PostVisitFuncLitTypeResults(node *ast.FieldList, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncLitTypeResults))
	if node == nil || len(tokens) == 0 {
		e.fs.PushCode("void")
		return
	}
	var parts []string
	for _, t := range tokens {
		if t.Content != "" {
			parts = append(parts, t.Content)
		}
	}
	if len(parts) > 1 {
		e.fs.PushCode("std::tuple<" + strings.Join(parts, ", ") + ">")
	} else if len(parts) == 1 {
		e.fs.PushCode(parts[0])
	} else {
		e.fs.PushCode("void")
	}
}

func (e *CPPPrimEmitter) PostVisitFuncLitTypeResult(node *ast.Field, index int, indent int) {
	code := e.fs.ReduceToCode(string(PreVisitFuncLitTypeResult))
	e.fs.PushCode(code)
}

func (e *CPPPrimEmitter) PostVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitFuncLitBody))
	e.fs.PushCode(bodyCode)
}

func (e *CPPPrimEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	e.funcLitDepth--
	tokens := e.fs.Reduce(string(PreVisitFuncLit))
	paramsCode := ""
	resultCode := ""
	bodyCode := ""
	if len(tokens) >= 1 {
		paramsCode = strings.TrimSpace(tokens[0].Content)
	}
	if len(tokens) >= 2 {
		resultCode = tokens[1].Content
	}
	if len(tokens) >= 3 {
		bodyCode = tokens[2].Content
	}
	e.fs.PushCode(fmt.Sprintf("[&](%s)->%s%s", paramsCode, resultCode, bodyCode))
}

// ============================================================
// Function Declarations
// ============================================================

func (e *CPPPrimEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	e.numFuncResults = 0
	if node.Type.Results != nil {
		e.numFuncResults = node.Type.Results.NumFields()
	}
}

func (e *CPPPrimEmitter) PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	code := e.fs.ReduceToCode(string(PreVisitFuncDeclSignatureTypeResultsList))
	e.fs.PushCode(code)
}

func (e *CPPPrimEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeResults))
	if node.Type.Results != nil && len(node.Type.Results.List) > 0 {
		var parts []string
		for _, t := range tokens {
			if t.Content != "" {
				parts = append(parts, t.Content)
			}
		}
		if len(parts) > 1 {
			e.fs.PushCode("std::tuple<" + strings.Join(parts, ", ") + ">")
		} else if len(parts) == 1 {
			e.fs.PushCode(parts[0])
		} else {
			e.fs.PushCode("void")
		}
	} else if node.Name.Name == "main" {
		e.fs.PushCode("int")
	} else {
		e.fs.PushCode("void")
	}
}

func (e *CPPPrimEmitter) PostVisitFuncDeclName(node *ast.Ident, indent int) {
	e.fs.Reduce(string(PreVisitFuncDeclName))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *CPPPrimEmitter) PostVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {
	typeCode := e.fs.ReduceToCode(string(PreVisitFuncDeclSignatureTypeParamsListType))
	e.fs.PushCode(typeCode)
}

func (e *CPPPrimEmitter) PostVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeParamsArgName))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *CPPPrimEmitter) PostVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeParamsList))
	// Collect type and names
	typeStr := ""
	var names []string
	for _, t := range tokens {
		if t.Tag == TagExpr && typeStr == "" {
			typeStr = t.Content
		} else if t.Tag == TagIdent {
			names = append(names, t.Content)
		}
	}
	for _, name := range names {
		e.fs.Push(typeStr+" "+name, TagExpr, nil)
	}
}

func (e *CPPPrimEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeParams))
	var params []string
	for _, t := range tokens {
		if t.Tag == TagExpr && t.Content != "" {
			params = append(params, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(params, ", "))
}

func (e *CPPPrimEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignature))
	// Tokens: result-type, name, params
	resultType := ""
	funcName := ""
	paramsStr := ""
	for _, t := range tokens {
		if t.Tag == TagIdent && funcName == "" {
			funcName = t.Content
		} else if t.Tag == TagExpr {
			if resultType == "" {
				resultType = t.Content
			} else {
				paramsStr = t.Content
			}
		}
	}

	if e.forwardDecl {
		e.fs.PushCode(fmt.Sprintf("%s %s(%s);\n", resultType, funcName, paramsStr))
	} else {
		e.fs.PushCode(fmt.Sprintf("\n%s %s(%s)", resultType, funcName, paramsStr))
	}
}

func (e *CPPPrimEmitter) PostVisitFuncDeclBody(node *ast.BlockStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitFuncDeclBody))
	e.fs.PushCode(bodyCode)
}

func (e *CPPPrimEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDecl))
	sigCode := ""
	bodyCode := ""
	if len(tokens) >= 1 {
		sigCode = tokens[0].Content
	}
	if len(tokens) >= 2 {
		bodyCode = tokens[1].Content
	}
	e.fs.PushCode(sigCode + "\n" + bodyCode + "\n\n")
}

// Forward Declaration Signatures
func (e *CPPPrimEmitter) PreVisitFuncDeclSignatures(indent int) {
	e.forwardDecl = true
	e.fs.PushCode("// Forward declarations\n")
}

func (e *CPPPrimEmitter) PostVisitFuncDeclSignatures(indent int) {
	e.forwardDecl = false
	// Let forward decl tokens flow through
}

// ============================================================
// Block Statements
// ============================================================

func (e *CPPPrimEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	e.indent = indent
}

func (e *CPPPrimEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	itemCode := e.fs.ReduceToCode(string(PreVisitBlockStmtList))
	e.fs.PushCode(itemCode)
}

func (e *CPPPrimEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitBlockStmt))
	var sb strings.Builder
	sb.WriteString("{\n")
	for _, t := range tokens {
		if t.Content != "" {
			sb.WriteString(t.Content)
		}
	}
	sb.WriteString(cppIndent(indent) + "}")
	e.fs.PushCode(sb.String())
}

// ============================================================
// Expression Statements
// ============================================================

func (e *CPPPrimEmitter) PreVisitExprStmt(node *ast.ExprStmt, indent int) {
	e.indent = indent
}

func (e *CPPPrimEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitExprStmtX))
	e.fs.PushCode(xCode)
}

func (e *CPPPrimEmitter) PostVisitExprStmt(node *ast.ExprStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitExprStmt))
	code := ""
	if len(tokens) >= 1 {
		code = tokens[0].Content
	}
	ind := cppIndent(indent)
	e.fs.PushCode(ind + code + ";\n")
}

// ============================================================
// Assignment Statements
// ============================================================

func (e *CPPPrimEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	e.indent = indent
	e.mapAssignVar = ""
	e.mapAssignKey = ""
	// Capture LHS variable names for move optimization
	e.currentAssignLhsNames = make(map[string]bool)
	for _, lhs := range node.Lhs {
		if ident, ok := lhs.(*ast.Ident); ok {
			e.currentAssignLhsNames[ident.Name] = true
		}
	}
}

func (e *CPPPrimEmitter) PostVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	lhsCode := e.fs.ReduceToCode(string(PreVisitAssignStmtLhsExpr))

	if indexExpr, ok := node.(*ast.IndexExpr); ok {
		if e.isMapTypeExpr(indexExpr.X) {
			e.mapAssignVar = e.lastIndexXCode
			e.mapAssignKey = e.lastIndexKeyCode
			e.fs.PushCode(lhsCode)
			return
		}
	}
	e.fs.PushCode(lhsCode)
}

func (e *CPPPrimEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmtLhs))
	var lhsExprs []string
	for _, t := range tokens {
		if t.Content != "" {
			lhsExprs = append(lhsExprs, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(lhsExprs, ", "))
}

func (e *CPPPrimEmitter) PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {
	rhsCode := e.fs.ReduceToCode(string(PreVisitAssignStmtRhsExpr))
	e.fs.PushCode(rhsCode)
}

func (e *CPPPrimEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmtRhs))
	var rhsExprs []string
	for _, t := range tokens {
		if t.Content != "" {
			rhsExprs = append(rhsExprs, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(rhsExprs, ", "))
}

func (e *CPPPrimEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	e.currentAssignLhsNames = nil
	tokens := e.fs.Reduce(string(PreVisitAssignStmt))
	lhsStr := ""
	rhsStr := ""
	if len(tokens) >= 1 {
		lhsStr = tokens[0].Content
	}
	if len(tokens) >= 2 {
		rhsStr = tokens[1].Content
	}

	ind := cppIndent(indent)
	tokStr := node.Tok.String()

	// Check all-blank assignment: _ = expr (suppress)
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
		return
	}

	// Mixed index chain: e.g., mapOfSlices["first"][0] = 100
	// Needs read-modify-write when intermediate access is a map
	if len(node.Lhs) == 1 {
		if _, isIndex := node.Lhs[0].(*ast.IndexExpr); isIndex && e.pkg != nil && e.pkg.TypesInfo != nil {
			ops, hasIntermediateMap := e.analyzeLhsIndexChain(node.Lhs[0])
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
				rootVar := exprToString(rootExpr)

				// Assign temp var names to map accesses
				currentVar := rootVar
				for i := range ops {
					if ops[i].accessType == "map" {
						ops[i].mapVarExpr = currentVar
						ops[i].tempVarName = fmt.Sprintf("__nested_inner_%d", e.nestedMapCounter)
						e.nestedMapCounter++
						currentVar = ops[i].tempVarName
					} else {
						ops[i].mapVarExpr = currentVar
						currentVar = currentVar + "[" + ops[i].keyExpr + "]"
					}
				}

				lastIdx := len(ops) - 1
				var sb strings.Builder

				// Prologue: extract temp variables for INTERMEDIATE map accesses only
				for i, op := range ops {
					if op.accessType == "map" && i < lastIdx {
						key := op.keyExpr
						if op.keyCastPfx != "" {
							key = op.keyCastPfx + key + op.keyCastSfx
						}
						sb.WriteString(fmt.Sprintf("%sauto %s = std::any_cast<%s>(hmap::hashMapGet(%s, %s));\n",
							ind, op.tempVarName, op.valueCppType, op.mapVarExpr, key))
					}
				}

				// Assignment depends on whether the last op is a map or slice access
				lastOp := ops[lastIdx]
				if lastOp.accessType == "map" {
					// Last op is map: use hashMapSet directly
					key := lastOp.keyExpr
					if lastOp.keyCastPfx != "" {
						key = lastOp.keyCastPfx + key + lastOp.keyCastSfx
					}
					sb.WriteString(fmt.Sprintf("%s%s = hmap::hashMapSet(%s, %s, %s);\n",
						ind, lastOp.mapVarExpr, lastOp.mapVarExpr, key, rhsStr))
				} else {
					// Last op is slice: assign to slice element
					sb.WriteString(fmt.Sprintf("%s%s %s %s;\n", ind, currentVar, tokStr, rhsStr))
				}

				// Epilogue: write back INTERMEDIATE map entries in reverse
				for i := lastIdx - 1; i >= 0; i-- {
					op := ops[i]
					if op.accessType == "map" {
						key := op.keyExpr
						if op.keyCastPfx != "" {
							key = op.keyCastPfx + key + op.keyCastSfx
						}
						sb.WriteString(fmt.Sprintf("%s%s = hmap::hashMapSet(%s, %s, %s);\n",
							ind, op.mapVarExpr, op.mapVarExpr, key, op.tempVarName))
					}
				}
				e.fs.PushCode(sb.String())
				return
			}
		}
	}

	// Map assignment: m[k] = v → m = hmap::hashMapSet(m, k, v)
	if e.mapAssignVar != "" && e.mapAssignKey != "" {
		mapVar := e.mapAssignVar
		mapKey := e.mapAssignKey
		// Apply key cast
		mapGoType := e.getExprGoType(node.Lhs[0].(*ast.IndexExpr).X)
		if mapGoType != nil {
			if mapType, ok := mapGoType.Underlying().(*types.Map); ok {
				castPfx, castSfx := getCppKeyCast(mapType.Key())
				if castPfx != "" {
					mapKey = castPfx + mapKey + castSfx
				}
				// Wrap string values
				if basic, isBasic := mapType.Elem().Underlying().(*types.Basic); isBasic && basic.Kind() == types.String {
					rhsStr = "std::string(" + rhsStr + ")"
				}
			}
		}

		// Check for nested map: m[k1][k2] = v
		if outerIndex, isNested := node.Lhs[0].(*ast.IndexExpr).X.(*ast.IndexExpr); isNested {
			if e.isMapTypeExpr(outerIndex.X) {
				outerVar := exprToString(outerIndex.X)
				outerKey := exprToCppString(outerIndex.Index)
				outerMapType := e.getExprGoType(outerIndex.X)
				if outerMapType != nil {
					if omt, ok := outerMapType.Underlying().(*types.Map); ok {
						oCastPfx, oCastSfx := getCppKeyCast(omt.Key())
						if oCastPfx != "" {
							outerKey = oCastPfx + outerKey + oCastSfx
						}
					}
				}
				tempVar := fmt.Sprintf("__nested_inner_%d", e.nestedMapCounter)
				e.nestedMapCounter++
				e.fs.PushCode(fmt.Sprintf("%sauto %s = std::any_cast<hmap::HashMap>(hmap::hashMapGet(%s, %s));\n",
					ind, tempVar, outerVar, outerKey))
				e.fs.PushCode(fmt.Sprintf("%s%s = hmap::hashMapSet(%s, %s, %s);\n",
					ind, tempVar, tempVar, mapKey, rhsStr))
				e.fs.PushCode(fmt.Sprintf("%s%s = hmap::hashMapSet(%s, %s, %s);\n",
					ind, outerVar, outerVar, outerKey, tempVar))
				e.mapAssignVar = ""
				e.mapAssignKey = ""
				return
			}
		}

		e.fs.PushCode(fmt.Sprintf("%s%s = hmap::hashMapSet(%s, %s, %s);\n", ind, mapVar, mapVar, mapKey, rhsStr))
		e.mapAssignVar = ""
		e.mapAssignKey = ""
		return
	}

	// Comma-ok map read: val, ok := m[key]
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if indexExpr, ok := node.Rhs[0].(*ast.IndexExpr); ok {
			if e.isMapTypeExpr(indexExpr.X) {
				valName := exprToString(node.Lhs[0])
				okName := exprToString(node.Lhs[1])
				mapName := exprToString(indexExpr.X)
				keyStr := exprToCppString(indexExpr.Index)
				valueCppType := "std::any"
				mapGoType := e.getExprGoType(indexExpr.X)
				if mapGoType != nil {
					if mapType, ok2 := mapGoType.Underlying().(*types.Map); ok2 {
						valueCppType = getCppTypeName(mapType.Elem())
						castPfx, castSfx := getCppKeyCast(mapType.Key())
						if castPfx != "" {
							keyStr = castPfx + keyStr + castSfx
						}
					}
				}
				decl := ""
				if tokStr == ":=" {
					decl = "auto "
				}
				e.fs.PushCode(fmt.Sprintf("%s%s%s = hmap::hashMapContains(%s, %s);\n",
					ind, decl, okName, mapName, keyStr))
				e.fs.PushCode(fmt.Sprintf("%s%s%s = %s ? std::any_cast<%s>(hmap::hashMapGet(%s, %s)) : %s{};\n",
					ind, decl, valName, okName, valueCppType, mapName, keyStr, valueCppType))
				return
			}
		}
	}

	// Comma-ok type assertion: val, ok := x.(Type)
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if typeAssert, ok := node.Rhs[0].(*ast.TypeAssertExpr); ok {
			valName := exprToString(node.Lhs[0])
			okName := exprToString(node.Lhs[1])
			typeName := ""
			if e.pkg != nil && e.pkg.TypesInfo != nil {
				tv := e.pkg.TypesInfo.Types[typeAssert.Type]
				if tv.Type != nil {
					typeName = getCppTypeName(tv.Type)
				}
			}
			xExpr := exprToString(typeAssert.X)
			decl := ""
			if tokStr == ":=" {
				decl = "auto "
			}
			e.fs.PushCode(fmt.Sprintf("%s%s%s = (std::any(%s)).type() == typeid(%s);\n",
				ind, decl, okName, xExpr, typeName))
			e.fs.PushCode(fmt.Sprintf("%s%s%s = %s ? std::any_cast<%s>(std::any(%s)) : %s{};\n",
				ind, decl, valName, okName, typeName, xExpr, typeName))
			return
		}
	}

	// Multi-value return: a, b := func() → auto [a, b] = func()
	if len(node.Lhs) > 1 && len(node.Rhs) == 1 {
		lhsParts := make([]string, len(node.Lhs))
		for i, lhs := range node.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				lhsParts[i] = ident.Name
			} else {
				lhsParts[i] = exprToString(lhs)
			}
		}
		if tokStr == ":=" {
			e.fs.PushCode(fmt.Sprintf("%sauto [%s] = %s;\n", ind, strings.Join(lhsParts, ", "), rhsStr))
		} else {
			e.fs.PushCode(fmt.Sprintf("%sstd::tie(%s) = %s;\n", ind, strings.Join(lhsParts, ", "), rhsStr))
		}
		return
	}

	// Detect if LHS is interface{}/std::any for string wrapping
	assignLhsIsInterface := false
	if len(node.Lhs) == 1 && e.pkg != nil && e.pkg.TypesInfo != nil {
		if ident, ok := node.Lhs[0].(*ast.Ident); ok {
			if obj := e.pkg.TypesInfo.ObjectOf(ident); obj != nil {
				if iface, ok := obj.Type().Underlying().(*types.Interface); ok && iface.Empty() {
					assignLhsIsInterface = true
				}
			}
		}
	}

	// Wrap string literals assigned to interface{}/std::any
	if assignLhsIsInterface && len(node.Rhs) == 1 {
		if lit, ok := node.Rhs[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
			rhsStr = "std::string(" + rhsStr + ")"
		}
	}

	switch tokStr {
	case ":=":
		// Check if RHS is a string literal → use std::string instead of auto
		if len(node.Rhs) == 1 {
			if lit, ok := node.Rhs[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				e.fs.PushCode(fmt.Sprintf("%sstd::string %s = %s;\n", ind, lhsStr, rhsStr))
			} else {
				e.fs.PushCode(fmt.Sprintf("%sauto %s = %s;\n", ind, lhsStr, rhsStr))
			}
		} else {
			e.fs.PushCode(fmt.Sprintf("%sauto %s = %s;\n", ind, lhsStr, rhsStr))
		}
	case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
		e.fs.PushCode(fmt.Sprintf("%s%s %s %s;\n", ind, lhsStr, tokStr, rhsStr))
	default:
		e.fs.PushCode(fmt.Sprintf("%s%s = %s;\n", ind, lhsStr, rhsStr))
	}
}

// ============================================================
// Declaration Statements
// ============================================================

func (e *CPPPrimEmitter) PreVisitDeclStmt(node *ast.DeclStmt, indent int) {
	e.indent = indent
}

func (e *CPPPrimEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	tokens := e.fs.Reduce(string(PreVisitDeclStmtValueSpecType))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Content
	}
	var goType types.Type
	if e.pkg != nil && e.pkg.TypesInfo != nil && index < len(node.Names) {
		if obj := e.pkg.TypesInfo.Defs[node.Names[index]]; obj != nil {
			goType = obj.Type()
		}
	}
	e.fs.Push(typeStr, TagType, goType)
}

func (e *CPPPrimEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	e.fs.Reduce(string(PreVisitDeclStmtValueSpecNames))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *CPPPrimEmitter) PostVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitDeclStmtValueSpecValue))
	e.fs.Push(valCode, TagExpr, nil)
}

func (e *CPPPrimEmitter) PostVisitDeclStmt(node *ast.DeclStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitDeclStmt))
	ind := cppIndent(indent)

	var sb strings.Builder
	i := 0
	for i < len(tokens) {
		typeStr := ""
		nameStr := ""
		valueStr := ""
		var goType types.Type

		if i < len(tokens) && tokens[i].Tag == TagType {
			typeStr = tokens[i].Content
			goType = tokens[i].GoType
			i++
		}
		if i < len(tokens) && tokens[i].Tag == TagIdent {
			nameStr = tokens[i].Content
			i++
		}
		if i < len(tokens) && tokens[i].Tag == TagExpr {
			valueStr = tokens[i].Content
			i++
		}

		if nameStr == "" {
			continue
		}

		if valueStr != "" {
			sb.WriteString(fmt.Sprintf("%s%s %s = %s;\n", ind, typeStr, nameStr, valueStr))
		} else {
			// Check for map type → default init with newHashMap
			if goType != nil {
				if _, isMap := goType.Underlying().(*types.Map); isMap {
					// Get key type constant
					keyType := 1
					// Try to get from AST
					if genDecl, ok := node.Decl.(*ast.GenDecl); ok {
						for _, spec := range genDecl.Specs {
							if vSpec, ok := spec.(*ast.ValueSpec); ok {
								if mapAst, ok := vSpec.Type.(*ast.MapType); ok {
									keyType = e.getMapKeyTypeConst(mapAst)
								}
							}
						}
					}
					sb.WriteString(fmt.Sprintf("%s%s %s = hmap::newHashMap(%d);\n", ind, typeStr, nameStr, keyType))
					continue
				}
			}
			sb.WriteString(fmt.Sprintf("%s%s %s;\n", ind, typeStr, nameStr))
		}
	}
	e.fs.PushCode(sb.String())
}

// ============================================================
// Return Statements
// ============================================================

func (e *CPPPrimEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	e.indent = indent
}

func (e *CPPPrimEmitter) PostVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	resultCode := e.fs.ReduceToCode(string(PreVisitReturnStmtResult))

	// Move optimization for struct return values
	if e.OptimizeMoves && e.funcLitDepth == 0 && e.numFuncResults > 1 && index == 0 {
		if ident, ok := node.(*ast.Ident); ok {
			tv := e.getExprGoType(node)
			if tv != nil {
				if named, ok := tv.(*types.Named); ok {
					if _, isStruct := named.Underlying().(*types.Struct); isStruct {
						_ = ident
						resultCode = "std::move(" + resultCode + ")"
						e.MoveOptCount++
					}
				}
			}
		}
	}

	e.fs.PushCode(resultCode)
}

func (e *CPPPrimEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitReturnStmt))
	ind := cppIndent(indent)

	if len(tokens) == 0 {
		e.fs.PushCode(ind + "return;\n")
	} else if len(tokens) == 1 {
		e.fs.PushCode(fmt.Sprintf("%sreturn %s;\n", ind, tokens[0].Content))
	} else {
		var vals []string
		for _, t := range tokens {
			vals = append(vals, t.Content)
		}
		e.fs.PushCode(fmt.Sprintf("%sreturn std::make_tuple(%s);\n", ind, strings.Join(vals, ", ")))
	}
}

// ============================================================
// If Statements
// ============================================================

func (e *CPPPrimEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	e.indent = indent
	e.ifInitStack = append(e.ifInitStack, "")
	e.ifCondStack = append(e.ifCondStack, "")
	e.ifBodyStack = append(e.ifBodyStack, "")
	e.ifElseStack = append(e.ifElseStack, "")
}

func (e *CPPPrimEmitter) PostVisitIfStmtInit(node ast.Stmt, indent int) {
	e.ifInitStack[len(e.ifInitStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtInit))
}

func (e *CPPPrimEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	e.ifCondStack[len(e.ifCondStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtCond))
}

func (e *CPPPrimEmitter) PostVisitIfStmtBody(node *ast.IfStmt, indent int) {
	e.ifBodyStack[len(e.ifBodyStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtBody))
}

func (e *CPPPrimEmitter) PostVisitIfStmtElse(node *ast.IfStmt, indent int) {
	e.ifElseStack[len(e.ifElseStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtElse))
}

func (e *CPPPrimEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	e.fs.Reduce(string(PreVisitIfStmt))
	ind := cppIndent(indent)

	n := len(e.ifInitStack)
	initCode := e.ifInitStack[n-1]
	condCode := e.ifCondStack[n-1]
	bodyCode := e.ifBodyStack[n-1]
	elseCode := e.ifElseStack[n-1]
	e.ifInitStack = e.ifInitStack[:n-1]
	e.ifCondStack = e.ifCondStack[:n-1]
	e.ifBodyStack = e.ifBodyStack[:n-1]
	e.ifElseStack = e.ifElseStack[:n-1]

	var sb strings.Builder
	if initCode != "" {
		sb.WriteString(fmt.Sprintf("%s{\n", ind))
		sb.WriteString(initCode)
		sb.WriteString(fmt.Sprintf("%sif (%s)\n%s", ind, condCode, bodyCode))
	} else {
		sb.WriteString(fmt.Sprintf("%sif (%s)\n%s", ind, condCode, bodyCode))
	}
	if elseCode != "" {
		trimmed := strings.TrimLeft(elseCode, " \t\n")
		if strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "if(") {
			sb.WriteString("\nelse " + trimmed)
		} else {
			sb.WriteString("\nelse\n" + elseCode)
		}
	}
	sb.WriteString("\n")
	if initCode != "" {
		sb.WriteString(fmt.Sprintf("%s}\n", ind))
	}
	e.fs.PushCode(sb.String())
}

// ============================================================
// For Statements
// ============================================================

func (e *CPPPrimEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	e.indent = indent
	e.forInitStack = append(e.forInitStack, "")
	e.forCondStack = append(e.forCondStack, "")
	e.forPostStack = append(e.forPostStack, "")
}

func (e *CPPPrimEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	initCode := e.fs.ReduceToCode(string(PreVisitForStmtInit))
	initCode = strings.TrimRight(initCode, ";\n \t")
	initCode = strings.TrimLeft(initCode, " \t")
	e.forInitStack[len(e.forInitStack)-1] = initCode
}

func (e *CPPPrimEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	e.forCondStack[len(e.forCondStack)-1] = e.fs.ReduceToCode(string(PreVisitForStmtCond))
}

func (e *CPPPrimEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	postCode := e.fs.ReduceToCode(string(PreVisitForStmtPost))
	postCode = strings.TrimRight(postCode, ";\n \t")
	postCode = strings.TrimLeft(postCode, " \t")
	e.forPostStack[len(e.forPostStack)-1] = postCode
}

func (e *CPPPrimEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitForStmt))
	ind := cppIndent(indent)

	n := len(e.forInitStack)
	initCode := e.forInitStack[n-1]
	condCode := e.forCondStack[n-1]
	postCode := e.forPostStack[n-1]
	e.forInitStack = e.forInitStack[:n-1]
	e.forCondStack = e.forCondStack[:n-1]
	e.forPostStack = e.forPostStack[:n-1]

	if node.Init == nil && node.Cond == nil && node.Post == nil {
		e.fs.PushCode(fmt.Sprintf("%sfor (;;)\n%s\n", ind, bodyCode))
		return
	}
	if node.Init == nil && node.Post == nil && node.Cond != nil {
		e.fs.PushCode(fmt.Sprintf("%sfor (;%s;)\n%s\n", ind, condCode, bodyCode))
		return
	}
	e.fs.PushCode(fmt.Sprintf("%sfor (%s;%s;%s)\n%s\n", ind, initCode, condCode, postCode, bodyCode))
}

// ============================================================
// Range Statements
// ============================================================

func (e *CPPPrimEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	e.indent = indent
}

func (e *CPPPrimEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	keyCode := e.fs.ReduceToCode(string(PreVisitRangeStmtKey))
	e.fs.Push(keyCode, TagIdent, nil)
}

func (e *CPPPrimEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitRangeStmtValue))
	e.fs.Push(valCode, TagIdent, nil)
}

func (e *CPPPrimEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitRangeStmtX))
	e.fs.PushCode(xCode)
}

func (e *CPPPrimEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitRangeStmt))
	ind := cppIndent(indent)

	keyCode := ""
	valCode := ""
	xCode := ""
	bodyCode := ""

	idx := 0
	if node.Key != nil {
		if idx < len(tokens) && tokens[idx].Tag == TagIdent {
			keyCode = tokens[idx].Content
			idx++
		}
	}
	if node.Value != nil {
		if idx < len(tokens) && tokens[idx].Tag == TagIdent {
			valCode = tokens[idx].Content
			idx++
		}
	}
	if idx < len(tokens) {
		xCode = tokens[idx].Content
		idx++
	}
	if idx < len(tokens) {
		bodyCode = tokens[idx].Content
	}

	if node.Key == nil && valCode != "" {
		keyCode = "_"
	}

	// Key-value range
	if valCode != "" && valCode != "_" {
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = "_i"
		}

		valDecl := fmt.Sprintf("%s    auto %s = %s[%s];\n", ind, valCode, xCode, loopVar)
		bodyWithDecl := strings.Replace(bodyCode, "{\n", "{\n"+valDecl, 1)

		e.fs.PushCode(fmt.Sprintf("%sfor (size_t %s = 0; %s < %s.size(); %s++)\n%s\n",
			ind, loopVar, loopVar, xCode, loopVar, bodyWithDecl))
	} else {
		// Key-only range → for (auto key : collection)
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = "_i"
		}
		e.fs.PushCode(fmt.Sprintf("%sfor (auto %s : %s)\n%s\n",
			ind, loopVar, xCode, bodyCode))
	}
}

// ============================================================
// Switch / Case Statements
// ============================================================

func (e *CPPPrimEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	e.indent = indent
}

func (e *CPPPrimEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	tagCode := e.fs.ReduceToCode(string(PreVisitSwitchStmtTag))
	e.fs.PushCode(tagCode)
}

func (e *CPPPrimEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSwitchStmt))
	ind := cppIndent(indent)

	tagCode := ""
	idx := 0
	if idx < len(tokens) {
		tagCode = tokens[idx].Content
		idx++
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%sswitch (%s) {\n", ind, tagCode))
	for i := idx; i < len(tokens); i++ {
		sb.WriteString(tokens[i].Content)
	}
	sb.WriteString(ind + "}\n")
	e.fs.PushCode(sb.String())
}

func (e *CPPPrimEmitter) PreVisitCaseClause(node *ast.CaseClause, indent int) {
	e.indent = indent
}

func (e *CPPPrimEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	exprCode := e.fs.ReduceToCode(string(PreVisitCaseClauseListExpr))
	e.fs.PushCode(exprCode)
}

func (e *CPPPrimEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCaseClauseList))
	var exprs []string
	for _, t := range tokens {
		if t.Content != "" {
			exprs = append(exprs, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(exprs, ", "))
}

func (e *CPPPrimEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCaseClause))
	ind := cppIndent(indent)

	var sb strings.Builder
	idx := 0
	if len(node.List) == 0 {
		// Default case: PushCode("") is a no-op (empty tokens are dropped),
		// so all tokens on the stack are body statements.
		sb.WriteString(ind + "  default:\n")
	} else {
		// Regular case: token[0] is case expressions, rest is body
		caseExprs := ""
		if idx < len(tokens) {
			caseExprs = tokens[idx].Content
			idx++
		}
		vals := strings.Split(caseExprs, ", ")
		for _, v := range vals {
			sb.WriteString(fmt.Sprintf("%s  case %s:\n", ind, v))
		}
	}
	for i := idx; i < len(tokens); i++ {
		sb.WriteString(tokens[i].Content)
	}
	sb.WriteString(ind + "    break;\n")
	e.fs.PushCode(sb.String())
}

// ============================================================
// Inc/Dec Statements
// ============================================================

func (e *CPPPrimEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitIncDecStmt))
	ind := cppIndent(indent)
	e.fs.PushCode(fmt.Sprintf("%s%s%s;\n", ind, xCode, node.Tok.String()))
}

// ============================================================
// Branch Statements (break, continue)
// ============================================================

func (e *CPPPrimEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	ind := cppIndent(indent)
	switch node.Tok {
	case token.BREAK:
		e.fs.PushCode(ind + "break;\n")
	case token.CONTINUE:
		e.fs.PushCode(ind + "continue;\n")
	}
}

// ============================================================
// Struct Declarations
// ============================================================

func (e *CPPPrimEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	typeCode := e.fs.ReduceToCode(string(PreVisitGenStructFieldType))
	e.fs.PushCode(typeCode)
}

func (e *CPPPrimEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	e.fs.Reduce(string(PreVisitGenStructFieldName))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *CPPPrimEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {
	tokens := e.fs.Reduce(string(PreVisitGenStructInfo))

	if node.Struct == nil {
		return
	}

	// Collect type-name pairs
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("struct %s\n{\n", node.Name))

	i := 0
	for i < len(tokens) {
		typeStr := ""
		nameStr := ""
		if i < len(tokens) && tokens[i].Tag == TagExpr {
			typeStr = tokens[i].Content
			i++
		}
		if i < len(tokens) && tokens[i].Tag == TagIdent {
			nameStr = tokens[i].Content
			i++
		}
		if typeStr != "" && nameStr != "" {
			sb.WriteString(fmt.Sprintf("%s %s;\n", typeStr, nameStr))
		}
	}
	sb.WriteString("};\n\n")

	// Generate operator== and std::hash for hashable structs
	if node.Struct != nil && e.structHasOnlyPrimitiveFields(node.Name) {
		e.generateStructHashAndEquality(node)
	}

	e.fs.PushCode(sb.String())
}

func (e *CPPPrimEmitter) PostVisitGenStructInfos(node []GenTypeInfo, indent int) {
	// Let struct code flow through
}

// structHasOnlyPrimitiveFields checks if a struct has only hashable fields
func (e *CPPPrimEmitter) structHasOnlyPrimitiveFields(structName string) bool {
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

func (e *CPPPrimEmitter) isHashableType(t types.Type, visited map[string]bool) bool {
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
			if !e.isHashableType(structType.Field(i).Type(), visited) {
				return false
			}
		}
		return true
	}
	return false
}

func (e *CPPPrimEmitter) generateStructHashAndEquality(node GenTypeInfo) {
	structName := node.Name
	pkgName := ""
	if e.pkg != nil && e.pkg.Name != "main" {
		pkgName = e.pkg.Name
	}

	// Check if already in pendingHashSpecs (may have been added as a dependency)
	for _, spec := range e.pendingHashSpecs {
		if spec.structName == structName && spec.pkgName == pkgName {
			return
		}
	}

	var fieldNames []string
	if node.Struct.Fields != nil {
		for _, field := range node.Struct.Fields.List {
			for _, name := range field.Names {
				fieldNames = append(fieldNames, name.Name)
			}
		}
	}

	// Also generate operator== for dependency struct types (e.g., cross-package structs
	// used as fields) that wouldn't otherwise get operator== generated
	if node.Struct != nil && e.pkg != nil && e.pkg.TypesInfo != nil {
		e.ensureDependencyHashSpecs(node.Struct, make(map[string]bool))
	}

	e.pendingHashSpecs = append(e.pendingHashSpecs, pendingHashSpec{
		structName: structName,
		pkgName:    pkgName,
		fieldNames: fieldNames,
	})
}

// ensureDependencyHashSpecs generates operator== and hash for struct field types from other packages
func (e *CPPPrimEmitter) ensureDependencyHashSpecs(structType *ast.StructType, visited map[string]bool) {
	if structType.Fields == nil {
		return
	}
	for _, field := range structType.Fields.List {
		fieldType := e.pkg.TypesInfo.Types[field.Type].Type
		if fieldType == nil {
			continue
		}
		named, ok := fieldType.(*types.Named)
		if !ok {
			continue
		}
		depStruct, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}
		depName := named.Obj().Name()
		depPkg := ""
		if named.Obj().Pkg() != nil {
			pkgName := named.Obj().Pkg().Name()
			if pkgName != "main" {
				depPkg = pkgName
			}
		}
		key := depPkg + "::" + depName
		if visited[key] {
			continue
		}
		visited[key] = true

		// Check if already in pendingHashSpecs
		alreadyPending := false
		for _, spec := range e.pendingHashSpecs {
			if spec.structName == depName && spec.pkgName == depPkg {
				alreadyPending = true
				break
			}
		}
		if alreadyPending {
			continue
		}
		// Collect field names from the dependency struct
		var depFieldNames []string
		for i := 0; i < depStruct.NumFields(); i++ {
			depFieldNames = append(depFieldNames, depStruct.Field(i).Name())
		}
		e.pendingHashSpecs = append(e.pendingHashSpecs, pendingHashSpec{
			structName: depName,
			pkgName:    depPkg,
			fieldNames: depFieldNames,
		})
	}
}

func (e *CPPPrimEmitter) emitHashSpec(spec pendingHashSpec) {
	qualifiedName := spec.structName
	if spec.pkgName != "" {
		qualifiedName = spec.pkgName + "::" + spec.structName
	}

	var sb strings.Builder
	// operator==
	sb.WriteString(fmt.Sprintf("inline bool operator==(const %s& a, const %s& b) {\n", qualifiedName, qualifiedName))
	sb.WriteString("    return ")
	for i, fieldName := range spec.fieldNames {
		if i > 0 {
			sb.WriteString(" && ")
		}
		sb.WriteString(fmt.Sprintf("a.%s == b.%s", fieldName, fieldName))
	}
	if len(spec.fieldNames) == 0 {
		sb.WriteString("true")
	}
	sb.WriteString(";\n}\n\n")

	// std::hash
	sb.WriteString("namespace std {\n")
	sb.WriteString(fmt.Sprintf("    template<> struct hash<%s> {\n", qualifiedName))
	sb.WriteString(fmt.Sprintf("        size_t operator()(const %s& s) const {\n", qualifiedName))
	sb.WriteString("            size_t h = 0;\n")
	for _, fieldName := range spec.fieldNames {
		sb.WriteString(fmt.Sprintf("            h ^= std::hash<decltype(s.%s)>{}(s.%s) + 0x9e3779b9 + (h << 6) + (h >> 2);\n", fieldName, fieldName))
	}
	sb.WriteString("            return h;\n")
	sb.WriteString("        }\n")
	sb.WriteString("    };\n")
	sb.WriteString("}\n\n")

	e.fs.PushCode(sb.String())
}

func (e *CPPPrimEmitter) emitPendingHashSpecs(pkgName string) {
	var specsForPkg []pendingHashSpec
	var remaining []pendingHashSpec

	for _, spec := range e.pendingHashSpecs {
		if spec.pkgName == pkgName {
			specsForPkg = append(specsForPkg, spec)
		} else {
			remaining = append(remaining, spec)
		}
	}
	e.pendingHashSpecs = remaining

	for _, spec := range specsForPkg {
		qualifiedName := spec.structName
		if spec.pkgName != "" {
			qualifiedName = spec.pkgName + "::" + spec.structName
		}

		e.file.WriteString(fmt.Sprintf("inline bool operator==(const %s& a, const %s& b) {\n", qualifiedName, qualifiedName))
		e.file.WriteString("    return ")
		for i, fieldName := range spec.fieldNames {
			if i > 0 {
				e.file.WriteString(" && ")
			}
			e.file.WriteString(fmt.Sprintf("a.%s == b.%s", fieldName, fieldName))
		}
		if len(spec.fieldNames) == 0 {
			e.file.WriteString("true")
		}
		e.file.WriteString(";\n}\n\n")

		e.file.WriteString("namespace std {\n")
		e.file.WriteString(fmt.Sprintf("    template<> struct hash<%s> {\n", qualifiedName))
		e.file.WriteString(fmt.Sprintf("        size_t operator()(const %s& s) const {\n", qualifiedName))
		e.file.WriteString("            size_t h = 0;\n")
		for _, fieldName := range spec.fieldNames {
			e.file.WriteString(fmt.Sprintf("            h ^= std::hash<decltype(s.%s)>{}(s.%s) + 0x9e3779b9 + (h << 6) + (h >> 2);\n", fieldName, fieldName))
		}
		e.file.WriteString("            return h;\n")
		e.file.WriteString("        }\n")
		e.file.WriteString("    };\n")
		e.file.WriteString("}\n\n")
	}
}

// ============================================================
// Constants
// ============================================================

func (e *CPPPrimEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	valTokens := e.fs.Reduce(string(PreVisitGenDeclConstName))
	valCode := ""
	for _, t := range valTokens {
		valCode += t.Content
	}
	if valCode == "" {
		valCode = "0"
	}
	e.fs.PushCode(fmt.Sprintf("constexpr auto %s = %s;\n", node.Name, valCode))
}

func (e *CPPPrimEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {
	// Let const tokens flow through
}

// ============================================================
// Type Aliases
// ============================================================

func (e *CPPPrimEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitTypeAliasName))
	// Tokens should have name and type
	// The PreVisitTypeAliasName marker captures everything
	nameCode := ""
	typeCode := ""
	for _, t := range tokens {
		if t.Tag == TagIdent && nameCode == "" {
			nameCode = t.Content
		} else if t.Content != "" {
			typeCode += t.Content
		}
	}
	if nameCode != "" && typeCode != "" {
		e.fs.PushCode(fmt.Sprintf("using %s = %s;\n\n", nameCode, typeCode))
	}
}

// ============================================================
// Struct Key Functions
// ============================================================

func (e *CPPPrimEmitter) replaceStructKeyFunctions() {
	outputPath := e.Output
	content, err := os.ReadFile(outputPath)
	if err != nil {
		log.Printf("Warning: could not read file for struct key replacement: %v", err)
		return
	}

	var hashCode strings.Builder
	hashCode.WriteString("int hashStructKey(std::any key)\n{\n")
	for _, qualifiedName := range e.structKeyTypes {
		hashCode.WriteString(fmt.Sprintf("    if (key.type() == typeid(%s)) {\n", qualifiedName))
		hashCode.WriteString(fmt.Sprintf("        int h = static_cast<int>(std::hash<%s>{}(std::any_cast<%s>(key)));\n", qualifiedName, qualifiedName))
		hashCode.WriteString("        if (h < 0) h = -h;\n")
		hashCode.WriteString("        return h;\n")
		hashCode.WriteString("    }\n")
	}
	hashCode.WriteString("    return 0;\n}")

	var eqCode strings.Builder
	eqCode.WriteString("bool structKeysEqual(std::any a, std::any b)\n{\n")
	eqCode.WriteString("    if (a.type() != b.type()) return false;\n")
	for _, qualifiedName := range e.structKeyTypes {
		eqCode.WriteString(fmt.Sprintf("    if (a.type() == typeid(%s)) {\n", qualifiedName))
		eqCode.WriteString(fmt.Sprintf("        return std::any_cast<%s>(a) == std::any_cast<%s>(b);\n", qualifiedName, qualifiedName))
		eqCode.WriteString("    }\n")
	}
	eqCode.WriteString("    return false;\n}")

	newContent := string(content)

	hashPattern := regexp.MustCompile(`(?s)int hashStructKey\(std::any key\)\s*\{\s*return 0;\s*\}`)
	newContent = hashPattern.ReplaceAllString(newContent, "int hashStructKey(std::any key);\n")

	eqPattern := regexp.MustCompile(`(?s)bool structKeysEqual\(std::any a, std::any b\)\s*\{\s*return false;\s*\}`)
	newContent = eqPattern.ReplaceAllString(newContent, "bool structKeysEqual(std::any a, std::any b);\n")

	newContent += "\n// Struct map key support - implementations after struct definitions\n"
	newContent += "namespace hmap {\n"
	newContent += hashCode.String() + "\n"
	newContent += eqCode.String() + "\n"
	newContent += "} // namespace hmap\n"

	if err := os.WriteFile(outputPath, []byte(newContent), 0644); err != nil {
		log.Printf("Warning: could not write struct key replacements: %v", err)
	}
}

// ============================================================
// Makefile Generation
// ============================================================

func (e *CPPPrimEmitter) GenerateMakefile() error {
	if e.LinkRuntime == "" {
		return nil
	}

	makefilePath := filepath.Join(e.OutputDir, "Makefile.cppprim")
	file, err := os.Create(makefilePath)
	if err != nil {
		return fmt.Errorf("failed to create Makefile.cppprim: %w", err)
	}
	defer file.Close()

	absRuntimePath, err := filepath.Abs(e.LinkRuntime)
	if err != nil {
		absRuntimePath = e.LinkRuntime
	}

	graphicsBackend := e.RuntimePackages["graphics"]
	if graphicsBackend == "" {
		graphicsBackend = "none"
	}

	var makefile string

	switch graphicsBackend {
	case "sdl2":
		makefile = fmt.Sprintf(`CXX = g++
CXXFLAGS = -O3 -fwrapv -std=c++17 -I%s $(shell sdl2-config --cflags)
LDFLAGS = $(shell sdl2-config --libs)

TARGET = %s-cppprim
SRCS = %s.cppprim.cpp

all: $(TARGET)

$(TARGET): $(SRCS)
	$(CXX) $(CXXFLAGS) -o $@ $^ $(LDFLAGS)

clean:
	rm -f $(TARGET)

.PHONY: all clean
`, absRuntimePath, e.OutputName, e.OutputName)

	case "none":
		makefile = fmt.Sprintf(`CXX = g++
CXXFLAGS = -O3 -fwrapv -std=c++17 -I%s
LDFLAGS =

TARGET = %s-cppprim
SRCS = %s.cppprim.cpp

all: $(TARGET)

$(TARGET): $(SRCS)
	$(CXX) $(CXXFLAGS) -o $@ $^ $(LDFLAGS)

clean:
	rm -f $(TARGET)

.PHONY: all clean
`, absRuntimePath, e.OutputName, e.OutputName)

	default: // tigr
		makefile = fmt.Sprintf(`CXX = g++
CC = gcc
CXXFLAGS = -O3 -fwrapv -std=c++17 -I%s
CFLAGS = -O3

TARGET = %s-cppprim
SRCS = %s.cppprim.cpp
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
`, absRuntimePath, e.OutputName, e.OutputName, absRuntimePath, absRuntimePath)
	}

	_, hasHTTP := e.RuntimePackages["http"]
	_, hasNet := e.RuntimePackages["net"]
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
		return fmt.Errorf("failed to write Makefile.cppprim: %w", err)
	}

	DebugLogPrintf("Generated Makefile.cppprim at %s (graphics: %s)", makefilePath, graphicsBackend)
	return nil
}
