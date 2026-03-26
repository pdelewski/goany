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

// CppEmitter implements the Emitter interface using a shift/reduce architecture
// for C++ code generation. Mirrors the JSEmitter pattern from js_emitter.go.
type CppEmitter struct {
	fs              *IRForestBuilder
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string
	RuntimePackages map[string]string
	OptimizeMoves   bool
	OptimizeRefs    bool
	CppRefOptPass   *RefOptPass
	MoveOptCount    int
	blankCounter    int
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
	rangeVarCounter  int
	// For loop components (stacks for nesting support)
	forInitStack []string
	forCondStack []string
	forPostStack []string
	forCondNodes []IRNode
	forBodyNodes []IRNode
	// If statement components (stacks for nesting support)
	ifInitStack []string
	ifCondStack []string
	ifBodyStack []string
	ifElseStack []string
	// Parallel node stacks for tree preservation
	ifInitNodes []IRNode
	ifCondNodes []IRNode
	ifBodyNodes []IRNode
	ifElseNodes []IRNode
	// Move optimization guards
	currentAssignLhsNames     map[string]bool
	currentCallArgIdentsStack []map[string]int
	// Reference optimization
	refOptReadOnly        *ReadOnlyAnalysis
	refOptCurrentFunc     string
	refOptCurrentPkg      string
	currentParamIndex     int
	currentCalleeName     string
	currentCalleeKey      string
	calleeNameStack       []string
	calleeKeyStack        []string
	// C++-specific
	forwardDecl      bool
	funcLitDepth     int
	nestedMapCounter int
	pendingHashSpecs []pendingHashSpec
	outputs          []OutputEntry
}

func (e *CppEmitter) SetFile(file *os.File) { e.file = file }
func (e *CppEmitter) GetFile() *os.File     { return e.file }

// canMoveArg checks if a call argument identifier can be moved instead of copied.
// A move is safe only when: (1) the variable is the LHS of the enclosing assignment
// (so it gets reassigned immediately), and (2) it doesn't appear multiple times
// in the same call's arguments.
func (e *CppEmitter) canMoveArg(varName string) bool {
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

func (e *CppEmitter) collectCallArgIdentCounts(args []ast.Expr) map[string]int {
	counts := make(map[string]int)
	for _, arg := range args {
		e.countIdentsInExpr(arg, counts)
	}
	return counts
}

func (e *CppEmitter) countIdentsInExpr(expr ast.Expr, counts map[string]int) {
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
func (e *CppEmitter) analyzeLhsIndexChain(expr ast.Expr) (ops []mixedIndexOp, hasIntermediateMap bool) {
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
func (e *CppEmitter) isMapTypeExpr(expr ast.Expr) bool {
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
func (e *CppEmitter) getExprGoType(expr ast.Expr) types.Type {
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
func (e *CppEmitter) getMapKeyTypeConst(mapType *ast.MapType) int {
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
func (e *CppEmitter) exprContainsIdent(expr ast.Expr, name string) bool {
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

func (e *CppEmitter) PreVisitProgram(indent int) {
	e.fs = e.GetForestBuilder()

	// Write C++ header
	e.fs.AddTree(IRNode{Type: Preamble, Kind: KindDecl, Content: "#include <vector>\n" +
		"#include <string>\n" +
		"#include <tuple>\n" +
		"#include <any>\n" +
		"#include <cstdint>\n" +
		"#include <functional>\n"})
	e.fs.AddTree(IRNode{Type: Preamble, Kind: KindDecl, Content: "#include <cstdarg> // For va_start, etc.\n" +
		"#include <initializer_list>\n" +
		"#include <iostream>\n"})

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
			e.fs.AddTree(IRNode{Type: Preamble, Kind: KindDecl, Content: fmt.Sprintf("#include \"%s/cpp/%s.hpp\"\n", name, fileName)})
		}
	}

	// Include panic runtime
	e.fs.AddTree(IRNode{Type: Preamble, Kind: KindDecl, Content: "\n// GoAny panic runtime\n"})
	e.fs.AddTree(IRNode{Type: Preamble, Kind: KindDecl, Content: goanyrt.PanicCppSource})
	e.fs.AddTree(IRNode{Type: Preamble, Kind: KindDecl, Content: "\n"})

	// C++ helper definitions
	e.fs.AddTree(IRNode{Type: Preamble, Kind: KindDecl, Content: `
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
`})
	e.fs.AddTree(IRNode{Type: Preamble, Kind: KindDecl, Content: "\n\n"})
}

func (e *CppEmitter) PostVisitProgram(indent int) {
	// Emit pending hash specializations for main package into IR tree
	var remaining []pendingHashSpec
	for _, spec := range e.pendingHashSpecs {
		if spec.pkgName == "" {
			e.emitHashSpec(spec)
		} else {
			remaining = append(remaining, spec)
		}
	}
	e.pendingHashSpecs = remaining

	// Collect forest into single OutputEntry
	tokens := e.fs.CollectForest(string(PreVisitProgram))
	root := IRNode{Type: ScopeNode, Kind: KindDecl, Children: tokens}
	root.Content = root.Serialize()
	e.outputs = []OutputEntry{{Path: e.Output, Root: root}}
}

func (e *CppEmitter) GetOutputEntries() []OutputEntry { return e.outputs }

func (e *CppEmitter) PostFileEmit() {
	if len(e.structKeyTypes) > 0 {
		e.replaceStructKeyFunctions()
	}
	if err := e.GenerateMakefile(); err != nil {
		log.Printf("Warning: %v", err)
	}
	if e.OptimizeMoves && e.MoveOptCount > 0 {
		fmt.Printf("  C++: %d copy(ies) replaced by std::move()\n", e.MoveOptCount)
	}
	if e.CppRefOptPass != nil && e.CppRefOptPass.TransformCount > 0 {
		fmt.Printf("  C++: %d ref(s) optimized by RefOptPass\n", e.CppRefOptPass.TransformCount)
	}
}

func (e *CppEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	e.pkg = pkg
	e.currentPackage = pkg.Name
	e.refOptCurrentPkg = pkg.Name
	if e.structKeyTypes == nil {
		e.structKeyTypes = make(map[string]string)
	}
	if e.OptimizeRefs {
		pkgAnalysis := AnalyzeReadOnlyParams(pkg)
		if e.refOptReadOnly == nil {
			e.refOptReadOnly = pkgAnalysis
		} else {
			for k, v := range pkgAnalysis.ReadOnly {
				e.refOptReadOnly.ReadOnly[k] = v
			}
			for k, v := range pkgAnalysis.MutRef {
				e.refOptReadOnly.MutRef[k] = v
			}
			for k, v := range pkgAnalysis.FuncsAsValues {
				e.refOptReadOnly.FuncsAsValues[k] = v
			}
		}
		// Emit synthetic OptFuncParam nodes for cross-package functions
		for key, flags := range e.refOptReadOnly.ReadOnly {
			if strings.HasPrefix(key, pkg.Name+".") {
				continue
			}
			mutFlags := e.refOptReadOnly.MutRef[key]
			for i, ro := range flags {
				isMut := mutFlags != nil && i < len(mutFlags) && mutFlags[i]
				if !ro && !isMut {
					continue
				}
				e.fs.AddTree(IRNode{
					Type: Identifier,
					OptMeta: &OptMeta{
						Kind:       OptFuncParam,
						FuncKey:    key,
						ParamIndex: i,
						IsReadOnly: ro,
						IsMutRef:   isMut,
					},
				})
			}
		}
	}
	if pkg.Name != "main" {
		e.inNamespace = true
		e.fs.AddTree(IRTree(Keyword, TagExpr,
			LeafTag(Keyword, "namespace", TagCpp),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, pkg.Name),
			Leaf(NewLine, "\n"),
			Leaf(LeftBrace, "{"),
			Leaf(NewLine, "\n\n"),
		))
	}
}

func (e *CppEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	if pkg.Name != "main" {
		e.fs.AddTree(IRTree(PackageDeclaration, KindDecl,
			Leaf(RightBrace, "}"),
			Leaf(WhiteSpace, " "),
			Leaf(LineComment, "// namespace "+pkg.Name),
			Leaf(NewLine, "\n\n"),
		))
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

func (e *CppEmitter) PreVisitBasicLit(node *ast.BasicLit, indent int) {
	val := node.Value
	if node.Kind == token.STRING {
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			// Regular string
			e.fs.AddLeaf(val, TagLiteral, nil)
		} else if len(val) >= 2 && val[0] == '`' && val[len(val)-1] == '`' {
			// Raw string → C++ raw string
			content := val[1 : len(val)-1]
			e.fs.AddTree(IRTree(StringLiteral, TagLiteral,
				Leaf(StringLiteral, "R\"("),
				Leaf(StringLiteral, content),
				Leaf(StringLiteral, ")\""),
			))
		} else {
			e.fs.AddLeaf(val, TagLiteral, nil)
		}
	} else {
		e.fs.AddLeaf(val, TagLiteral, nil)
	}
}

func (e *CppEmitter) PreVisitIdent(node *ast.Ident, indent int) {
	name := node.Name
	// Map Go builtins
	switch name {
	case "true", "false":
		e.fs.AddLeaf(name, TagLiteral, nil)
		return
	case "nil":
		e.fs.AddLeaf("{}", TagLiteral, nil)
		return
	}
	// Map Go types to C++ types
	if n, ok := cppTypesMap[name]; ok {
		e.fs.AddLeaf(n, TagType, nil)
		return
	}
	goType := e.getExprGoType(node)
	e.fs.AddLeaf(name, TagIdent, goType)
}

// ============================================================
// Binary Expressions
// ============================================================

func (e *CppEmitter) PostVisitBinaryExprLeft(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBinaryExprLeft))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitBinaryExprRight(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBinaryExprRight))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBinaryExpr))
	var leftNode, rightNode IRNode
	if len(tokens) >= 1 {
		leftNode = tokens[0]
	}
	if len(tokens) >= 2 {
		rightNode = tokens[1]
	}
	op := node.Op.String()
	e.fs.AddTree(IRTree(BinaryExpression, KindExpr,
		leftNode,
		Leaf(WhiteSpace, " "),
		Leaf(BinaryOperator, op),
		Leaf(WhiteSpace, " "),
		rightNode,
	))
}

// ============================================================
// Call Expressions
// ============================================================

func (e *CppEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	funCode := e.fs.CollectText(string(PreVisitCallExprFun))

	// Save current callee state before overwriting (for nested calls)
	e.calleeNameStack = append(e.calleeNameStack, e.currentCalleeName)
	e.calleeKeyStack = append(e.calleeKeyStack, e.currentCalleeKey)

	if ident, ok := node.(*ast.Ident); ok {
		e.currentCalleeName = ident.Name
	} else if sel, ok := node.(*ast.SelectorExpr); ok {
		e.currentCalleeName = sel.Sel.Name
	} else {
		e.currentCalleeName = ""
	}
	if e.currentCalleeName != "" {
		e.currentCalleeKey = e.refOptCurrentPkg + "." + e.currentCalleeName
	} else {
		e.currentCalleeKey = ""
	}
	e.fs.AddLeaf(funCode, KindExpr, nil)
}

func (e *CppEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCallExprArg))
	argNode := collectToNode(tokens)

	// Annotate struct args for ownership transfer (CloneMovePass applies std::move)
	if e.OptimizeMoves && e.funcLitDepth == 0 {
		if ident, isIdent := node.(*ast.Ident); isIdent {
			tv := e.getExprGoType(node)
			if tv != nil {
				if named, ok := tv.(*types.Named); ok {
					if _, isStruct := named.Underlying().(*types.Struct); isStruct {
						if e.canMoveArg(ident.Name) {
							argNode.OptMeta = &OptMeta{CanTransfer: true}
							e.fs.AddTree(argNode)
							return
						}
					}
				}
			}
		}
	}

	e.fs.AddTree(argNode)
}

func (e *CppEmitter) PreVisitCallExprArgs(node []ast.Expr, indent int) {
	e.currentCallArgIdentsStack = append(e.currentCallArgIdentsStack, e.collectCallArgIdentCounts(node))
}

func (e *CppEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	if len(e.currentCallArgIdentsStack) > 0 {
		e.currentCallArgIdentsStack = e.currentCallArgIdentsStack[:len(e.currentCallArgIdentsStack)-1]
	}
	argTokens := e.fs.CollectForest(string(PreVisitCallExprArgs))
	first := true
	argIdx := 0
	for _, t := range argTokens {
		if t.Serialize() == "" {
			continue
		}
		if !first {
			e.fs.AddTree(IRNode{Type: Comma, Content: ", "})
		}
		t.Type = CallExpression
		isIdent := false
		if argIdx < len(node) {
			_, isIdent = node[argIdx].(*ast.Ident)
		}
		// Merge ownership flags from PostVisitCallExprArg
		existingMeta := t.OptMeta
		t.OptMeta = &OptMeta{
			Kind:       OptCallArg,
			CalleeName: e.currentCalleeName,
			FuncKey:    e.currentCalleeKey,
			ParamIndex: argIdx,
			IsIdentArg: isIdent,
		}
		if existingMeta != nil {
			t.OptMeta.CanTransfer = existingMeta.CanTransfer
		}
		e.fs.AddTree(t)
		first = false
		argIdx++
	}
}

func (e *CppEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {
	// Restore saved callee state (for nested calls)
	if n := len(e.calleeNameStack); n > 0 {
		e.currentCalleeName = e.calleeNameStack[n-1]
		e.calleeNameStack = e.calleeNameStack[:n-1]
		e.currentCalleeKey = e.calleeKeyStack[n-1]
		e.calleeKeyStack = e.calleeKeyStack[:n-1]
	} else {
		e.currentCalleeName = ""
		e.currentCalleeKey = ""
	}
	tokens := e.fs.CollectForest(string(PreVisitCallExpr))
	funName := ""
	argsStr := ""
	if len(tokens) >= 1 {
		funName = tokens[0].Serialize()
	}
	if len(tokens) > 1 {
		var sb strings.Builder
		for _, t := range tokens[1:] {
			sb.WriteString(t.Serialize())
		}
		argsStr = sb.String()
	}

	// Handle special built-in functions
	switch funName {
	case "len":
		// len(m) on maps → hmap::hashMapLen(m)
		if len(node.Args) > 0 && e.isMapTypeExpr(node.Args[0]) {
			e.fs.AddTree(IRTree(CallExpression, KindExpr,
				Leaf(Identifier, "hmap::hashMapLen"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.AddTree(IRTree(CallExpression, KindExpr,
				Leaf(Identifier, "std::size"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(RightParen, ")"),
			))
		}
		return
	case "append":
		e.fs.AddTree(IRTree(CallExpression, KindExpr,
			Leaf(Identifier, "append"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
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
			e.fs.AddTree(IRTree(CallExpression, KindExpr,
				Leaf(Identifier, mapName),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "hmap::hashMapDelete"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, mapName),
				Leaf(Comma, ", "),
				Leaf(Identifier, keyStr),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.AddTree(IRTree(CallExpression, KindExpr,
				Leaf(Identifier, "delete"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(RightParen, ")"),
			))
		}
		return
	case "min":
		e.fs.AddTree(IRTree(CallExpression, KindExpr,
			Leaf(Identifier, "std::min"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
		return
	case "max":
		e.fs.AddTree(IRTree(CallExpression, KindExpr,
			Leaf(Identifier, "std::max"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
		return
	case "clear":
		if len(node.Args) >= 1 {
			mapName := exprToString(node.Args[0])
			e.fs.AddTree(IRTree(CallExpression, KindExpr,
				Leaf(Identifier, mapName),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "hmap::hashMapClear"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, mapName),
				Leaf(RightParen, ")"),
			))
		}
		return
	case "make":
		if len(node.Args) >= 1 {
			if mapType, ok := node.Args[0].(*ast.MapType); ok {
				keyTypeConst := e.getMapKeyTypeConst(mapType)
				e.fs.AddTree(IRTree(CallExpression, KindExpr,
					Leaf(Identifier, "hmap::newHashMap"),
					Leaf(LeftParen, "("),
					Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)),
					Leaf(RightParen, ")"),
				))
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
					e.fs.AddTree(IRTree(CallExpression, KindExpr,
						Leaf(Identifier, cppType),
						Leaf(LeftParen, "("),
						Leaf(Identifier, parts[1]),
						Leaf(RightParen, ")"),
					))
				} else {
					e.fs.AddLeaf(cppType + "()", KindExpr, nil)
				}
				return
			}
		}
		e.fs.AddTree(IRTree(CallExpression, KindExpr,
			Leaf(Identifier, "make"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
		return
	case "goany_panic":
		e.fs.AddTree(IRTree(CallExpression, KindExpr,
			Leaf(Identifier, "goany_panic"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
		return
	}

	// Lower builtins
	lowered := cppLowerBuiltin(funName)
	if lowered != funName {
		funName = lowered
	}

	var callChildren []IRNode
	callChildren = append(callChildren, Leaf(Identifier, funName))
	callChildren = append(callChildren, Leaf(LeftParen, "("))
	for _, t := range tokens[1:] {
		callChildren = append(callChildren, t)
	}
	callChildren = append(callChildren, Leaf(RightParen, ")"))
	e.fs.AddTree(IRTree(CallExpression, KindExpr, callChildren...))
}

// ============================================================
// Selector Expressions (a.b)
// ============================================================

func (e *CppEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSelectorExprX))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitSelectorExprSel(node *ast.Ident, indent int) {
	e.fs.CollectForest(string(PreVisitSelectorExprSel))
	e.fs.AddLeaf(node.Name, KindExpr, nil)
}

func (e *CppEmitter) PostVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSelectorExpr))
	xCode := ""
	selCode := ""
	var xNode IRNode
	if len(tokens) >= 1 {
		xNode = tokens[0]
		xCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		selCode = tokens[1].Serialize()
	}

	if xCode == "os" && selCode == "Args" {
		e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, "goany_os_args")))
		return
	}

	loweredX := cppLowerBuiltin(xCode)
	loweredSel := cppLowerBuiltin(selCode)

	if loweredX == "" {
		// Package selector like fmt is suppressed
		e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, loweredSel)))
	} else {
		// Use :: for namespaces, . for struct fields
		if _, found := namespaces[xCode]; found {
			e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, loweredX+"::"+loweredSel)))
		} else if loweredX == xCode {
			// Preserve tree structure for field access to keep OptCallArg metadata
			e.fs.AddTree(IRTree(SelectorExpression, KindExpr,
				xNode,
				Leaf(Dot, "."),
				Leaf(Identifier, loweredSel),
			))
		} else {
			e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, loweredX+"."+loweredSel)))
		}
	}
}

// ============================================================
// Index Expressions (a[i])
// ============================================================

func (e *CppEmitter) PostVisitIndexExprX(node *ast.IndexExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIndexExprX))
	xNode := collectToNode(tokens)
	xNode.Kind = KindExpr
	e.fs.AddTree(xNode)
	e.lastIndexXCode = xNode.Serialize()
}

func (e *CppEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIndexExprIndex))
	idxNode := collectToNode(tokens)
	idxNode.Kind = KindExpr
	e.fs.AddTree(idxNode)
	e.lastIndexKeyCode = idxNode.Serialize()
}

func (e *CppEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIndexExpr))
	xCode := ""
	idxCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		idxCode = tokens[1].Serialize()
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
		tree := IRTree(IndexExpression, KindExpr,
			Leaf(Identifier, "std::any_cast"),
			Leaf(LeftAngle, "<"),
			Leaf(Identifier, valueCppType),
			Leaf(RightAngle, ">"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, "hmap::hashMapGet"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, xCode),
			Leaf(Comma, ", "),
			Leaf(Identifier, keyStr),
			Leaf(RightParen, ")"),
			Leaf(RightParen, ")"),
		)
		tree.GoType = e.getExprGoType(node)
		e.fs.AddTree(tree)
	} else {
		e.fs.AddTree(IRTree(IndexExpression, KindExpr,
			Leaf(Identifier, xCode),
			Leaf(LeftBracket, "["),
			Leaf(Identifier, idxCode),
			Leaf(RightBracket, "]"),
		))
	}
}

// ============================================================
// Unary Expressions
// ============================================================

func (e *CppEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitUnaryExpr))
	xNode := collectToNode(tokens)
	op := node.Op.String()
	if op == "^" {
		op = "~"
	}
	e.fs.AddTree(IRTree(UnaryExpression, KindExpr,
		Leaf(LeftParen, "("),
		Leaf(UnaryOperator, op),
		xNode,
		Leaf(RightParen, ")"),
	))
}

// ============================================================
// Paren Expressions
// ============================================================

func (e *CppEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitParenExpr))
	innerNode := collectToNode(tokens)
	e.fs.AddTree(IRTree(ParenExpression, KindExpr,
		Leaf(LeftParen, "("),
		innerNode,
		Leaf(RightParen, ")"),
	))
}

// ============================================================
// Slice Expressions (a[lo:hi])
// ============================================================

func (e *CppEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSliceExprX))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitSliceExprXBegin))
}

func (e *CppEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSliceExprLow))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitSliceExprXEnd))
}

func (e *CppEmitter) PostVisitSliceExprHigh(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSliceExprHigh))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSliceExpr))
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

	// C++ slice: std::vector<T>(x.begin() + low, x.begin() + high) or x.end()
	beginExpr := xCode + ".begin() + " + lowCode
	endExpr := ""
	if highCode == "" {
		endExpr = xCode + ".end()"
	} else {
		endExpr = xCode + ".begin() + " + highCode
	}
	e.fs.AddTree(IRTree(SliceExpression, KindExpr,
		Leaf(Identifier, "std::vector<std::remove_const<std::remove_reference<decltype("+xCode+"[0])>::type>::type>"),
		Leaf(LeftParen, "("),
		Leaf(Identifier, beginExpr),
		Leaf(Comma, ", "),
		Leaf(Identifier, endExpr),
		Leaf(RightParen, ")"),
	))
}

// ============================================================
// Composite Literals
// ============================================================

func (e *CppEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitCompositeLitType))
}

func (e *CppEmitter) PostVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCompositeLitElt))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	eltTokens := e.fs.CollectForest(string(PreVisitCompositeLitElts))
	for _, t := range eltTokens {
		if t.Serialize() != "" {
			e.fs.AddLeaf(t.Serialize(), TagLiteral, nil)
		}
	}
}

func (e *CppEmitter) PostVisitCompositeLit(node *ast.CompositeLit, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCompositeLit))
	var elts []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			elts = append(elts, t.Serialize())
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
			e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr,
				Leaf(Identifier, typeName),
				Leaf(LeftBrace, "{"),
				Leaf(Identifier, strings.Join(elts, ", ")),
				Leaf(RightBrace, "}"),
			))
		} else {
			e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr, Leaf(Identifier, "{"+strings.Join(elts, ", ")+"}")))
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
				e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr,
					Leaf(Identifier, typeName),
					Leaf(LeftBrace, "{"),
					Leaf(Identifier, strings.Join(args, ", ")),
					Leaf(RightBrace, "}"),
				))
				return
			}
		}
		e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr,
			Leaf(Identifier, typeName),
			Leaf(LeftBrace, "{"),
			Leaf(Identifier, strings.Join(elts, ", ")),
			Leaf(RightBrace, "}"),
		))

	case *types.Slice:
		elemCppType := getCppTypeName(u.Elem())
		e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr,
			Leaf(Identifier, "std::vector<"+elemCppType+">"),
			Leaf(LeftBrace, "{"),
			Leaf(Identifier, strings.Join(elts, ", ")),
			Leaf(RightBrace, "}"),
		))

	case *types.Map:
		keyTypeConst := 1
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			keyTypeConst = e.getMapKeyTypeConst(node.Type.(*ast.MapType))
		}
		if len(elts) == 0 {
			e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr,
				Leaf(Identifier, "hmap::newHashMap"),
				Leaf(LeftParen, "("),
				Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)),
				Leaf(RightParen, ")"),
			))
		} else {
			// Map literal with elements
			castPfx, castSfx := "", ""
			if mapType, ok := litType.Underlying().(*types.Map); ok {
				castPfx, castSfx = getCppKeyCast(mapType.Key())
			}
			_ = u
			var children []IRNode
			children = append(children,
				Leaf(LeftBracket, "["),
				Leaf(Identifier, "&"),
				Leaf(RightBracket, "]"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(LeftBrace, "{"),
				Leaf(WhiteSpace, " "),
				LeafTag(Keyword, "auto", TagCpp),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "_m"),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "hmap::newHashMap"),
				Leaf(LeftParen, "("),
				Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(WhiteSpace, " "),
			)
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
					children = append(children,
						Leaf(Identifier, "_m"),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "hmap::hashMapSet"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, "_m"),
						Leaf(Comma, ", "),
						Leaf(Identifier, keyStr),
						Leaf(Comma, ", "),
						Leaf(Identifier, valStr),
						Leaf(RightParen, ")"),
						Leaf(Semicolon, ";"),
						Leaf(WhiteSpace, " "),
					)
				}
			}
			children = append(children,
				Leaf(ReturnKeyword, "return"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "_m"),
				Leaf(Semicolon, ";"),
				Leaf(WhiteSpace, " "),
				Leaf(RightBrace, "}"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
			)
			e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr, children...))
		}

	default:
		e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr, Leaf(Identifier, "{"+strings.Join(elts, ", ")+"}")))
	}
}

// ============================================================
// KeyValue Expressions
// ============================================================

func (e *CppEmitter) PostVisitKeyValueExprKey(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitKeyValueExprKey))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitKeyValueExprValue(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitKeyValueExprValue))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitKeyValueExpr))
	keyCode := ""
	valCode := ""
	if len(tokens) >= 1 {
		keyCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		valCode = tokens[1].Serialize()
	}
	// C++ designated initializer style: .Key = Value
	e.fs.AddTree(IRTree(KeyValueExpression, KindExpr, Leaf(Identifier, "."+keyCode+" = "+valCode)))
}

// ============================================================
// Array Type
// ============================================================

func (e *CppEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	typeTokens := e.fs.CollectForest(string(PreVisitArrayType))
	if len(typeTokens) == 0 {
		typeTokens = []IRNode{Leaf(Identifier, "std::any")}
	}
	children := []IRNode{Leaf(Identifier, "std::vector"), Leaf(LeftAngle, "<")}
	children = append(children, typeTokens...)
	children = append(children, Leaf(RightAngle, ">"))
	e.fs.AddTree(IRTree(ArrayTypeNode, KindType, children...))
}

// ============================================================
// Map Type
// ============================================================

func (e *CppEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitMapKeyType))
}

func (e *CppEmitter) PostVisitMapValueType(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitMapValueType))
}

func (e *CppEmitter) PostVisitMapType(node *ast.MapType, indent int) {
	e.fs.CollectForest(string(PreVisitMapType))
	e.fs.AddTree(IRTree(MapTypeNode, KindType, Leaf(Identifier, "hmap::HashMap")))
}

// ============================================================
// Function Type
// ============================================================

func (e *CppEmitter) PostVisitFuncTypeResults(node *ast.FieldList, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncTypeResults))
	resultType := "void"
	if node != nil && len(tokens) > 0 {
		var parts []string
		for _, t := range tokens {
			s := t.Serialize()
			if s != "" {
				parts = append(parts, s)
			}
		}
		resultType = strings.Join(parts, ", ")
	}
	e.fs.AddLeaf(resultType, KindExpr, nil)
}

func (e *CppEmitter) PostVisitFuncTypeResult(node *ast.Field, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncTypeResult))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitFuncTypeParams(node *ast.FieldList, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncTypeParams))
	var parts []string
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			parts = append(parts, s)
		}
	}
	e.fs.AddLeaf(strings.Join(parts, ", "), KindExpr, nil)
}

func (e *CppEmitter) PostVisitFuncTypeParam(node *ast.Field, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncTypeParam))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitFuncType(node *ast.FuncType, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncType))
	resultType := "void"
	paramsType := ""
	if len(tokens) >= 1 {
		resultType = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		paramsType = tokens[1].Serialize()
	}
	children := []IRNode{
		Leaf(Identifier, "std::function"),
		Leaf(LeftAngle, "<"),
		Leaf(Identifier, resultType),
		Leaf(LeftParen, "("),
		Leaf(Identifier, paramsType),
		Leaf(RightParen, ")"),
		Leaf(RightAngle, ">"),
	}
	e.fs.AddTree(IRTree(FuncTypeExpression, KindType, children...))
}

// ============================================================
// Interface / Star / Type Assertions
// ============================================================

func (e *CppEmitter) PreVisitInterfaceType(node *ast.InterfaceType, indent int) {
	e.fs.AddLeaf("std::any", TagType, nil)
}

func (e *CppEmitter) PreVisitStarExpr(node *ast.StarExpr, indent int) {
	// Pointers not supported in ULang, but emit * for completeness
}

func (e *CppEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitTypeAssertExprType))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitTypeAssertExprX))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitTypeAssertExpr))
	typeCode := ""
	xCode := ""
	if len(tokens) >= 1 {
		typeCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		xCode = tokens[1].Serialize()
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
		e.fs.AddTree(IRTree(TypeAssertExpression, KindExpr,
			Leaf(Identifier, "std::any_cast"),
			Leaf(LeftAngle, "<"),
			Leaf(Identifier, typeCode),
			Leaf(RightAngle, ">"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, "std::any"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, xCode),
			Leaf(RightParen, ")"),
			Leaf(RightParen, ")"),
		))
	} else {
		e.fs.AddTree(IRTree(TypeAssertExpression, KindExpr,
			Leaf(Identifier, "std::any_cast"),
			Leaf(LeftAngle, "<"),
			Leaf(Identifier, typeCode),
			Leaf(RightAngle, ">"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, xCode),
			Leaf(RightParen, ")"),
		))
	}
}

// ============================================================
// Function Literals (closures)
// ============================================================

func (e *CppEmitter) PreVisitFuncLit(node *ast.FuncLit, indent int) {
	e.funcLitDepth++
}

func (e *CppEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	// Get type and name tokens
	tokens := e.fs.CollectForest(string(PreVisitFuncLitTypeParam))
	var typeStr string
	for _, t := range tokens {
		if t.Serialize() != "" {
			typeStr = t.Serialize()
		}
	}
	// Push "type name" pairs
	for _, name := range node.Names {
		e.fs.AddLeaf(typeStr+" "+name.Name, TagIdent, nil)
	}
}

func (e *CppEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncLitTypeParams))
	var paramStrs []string
	for _, t := range tokens {
		if t.Kind == TagIdent && t.Serialize() != "" {
			paramStrs = append(paramStrs, t.Serialize())
		}
	}
	paramsStr := strings.Join(paramStrs, ", ")
	if paramsStr == "" {
		paramsStr = " "
	}
	e.fs.AddLeaf(paramsStr, KindExpr, nil)
}

func (e *CppEmitter) PostVisitFuncLitTypeResults(node *ast.FieldList, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncLitTypeResults))
	if node == nil || len(tokens) == 0 {
		e.fs.AddLeaf("void", KindExpr, nil)
		return
	}
	var parts []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			parts = append(parts, t.Serialize())
		}
	}
	if len(parts) > 1 {
		e.fs.AddLeaf("std::tuple<" + strings.Join(parts, ", ") + ">", KindExpr, nil)
	} else if len(parts) == 1 {
		e.fs.AddLeaf(parts[0], KindExpr, nil)
	} else {
		e.fs.AddLeaf("void", KindExpr, nil)
	}
}

func (e *CppEmitter) PostVisitFuncLitTypeResult(node *ast.Field, index int, indent int) {
	code := e.fs.CollectText(string(PreVisitFuncLitTypeResult))
	e.fs.AddLeaf(code, KindExpr, nil)
}

func (e *CppEmitter) PostVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncLitBody))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	e.funcLitDepth--
	tokens := e.fs.CollectForest(string(PreVisitFuncLit))
	paramsCode := ""
	resultCode := ""
	bodyCode := ""
	if len(tokens) >= 1 {
		paramsCode = strings.TrimSpace(tokens[0].Serialize())
	}
	if len(tokens) >= 2 {
		resultCode = tokens[1].Serialize()
	}
	if len(tokens) >= 3 {
		bodyCode = tokens[2].Serialize()
	}
	e.fs.AddTree(IRTree(FuncLitExpression, KindExpr,
		Leaf(LeftBracket, "["),
		Leaf(Identifier, "&"),
		Leaf(RightBracket, "]"),
		Leaf(LeftParen, "("),
		Leaf(Identifier, paramsCode),
		Leaf(RightParen, ")"),
		Leaf(Identifier, "->"),
		Leaf(Identifier, resultCode),
		Leaf(Identifier, bodyCode),
	))
}

// ============================================================
// Function Declarations
// ============================================================

func (e *CppEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	e.numFuncResults = 0
	if node.Type.Results != nil {
		e.numFuncResults = node.Type.Results.NumFields()
	}
}

func (e *CppEmitter) PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	code := e.fs.CollectText(string(PreVisitFuncDeclSignatureTypeResultsList))
	e.fs.AddLeaf(code, KindExpr, nil)
}

func (e *CppEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeResults))
	if node.Type.Results != nil && len(node.Type.Results.List) > 0 {
		var parts []string
		for _, t := range tokens {
			if t.Serialize() != "" {
				parts = append(parts, t.Serialize())
			}
		}
		if len(parts) > 1 {
			e.fs.AddLeaf("std::tuple<" + strings.Join(parts, ", ") + ">", KindExpr, nil)
		} else if len(parts) == 1 {
			e.fs.AddLeaf(parts[0], KindExpr, nil)
		} else {
			e.fs.AddLeaf("void", KindExpr, nil)
		}
	} else if node.Name.Name == "main" {
		e.fs.AddLeaf("int", KindExpr, nil)
	} else {
		e.fs.AddLeaf("void", KindExpr, nil)
	}
}

func (e *CppEmitter) PostVisitFuncDeclName(node *ast.Ident, indent int) {
	e.fs.CollectForest(string(PreVisitFuncDeclName))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
	e.refOptCurrentFunc = e.refOptCurrentPkg + "." + node.Name
	e.currentParamIndex = 0
}

func (e *CppEmitter) PostVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {
	typeCode := e.fs.CollectText(string(PreVisitFuncDeclSignatureTypeParamsListType))
	e.fs.AddLeaf(typeCode, KindExpr, nil)
}

func (e *CppEmitter) PostVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeParamsArgName))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *CppEmitter) PostVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeParamsList))
	// Collect type and names
	typeStr := ""
	var names []string
	for _, t := range tokens {
		if t.Kind == TagExpr && typeStr == "" {
			typeStr = t.Serialize()
		} else if t.Kind == TagIdent {
			names = append(names, t.Serialize())
		}
	}
	goType := e.getExprGoType(node.Type)
	paramIdx := e.currentParamIndex
	for _, name := range names {
		optMeta := &OptMeta{
			Kind:          OptFuncParam,
			FuncKey:       e.refOptCurrentFunc,
			ParamIndex:    paramIdx,
			ParamName:     name,
			TypeStr:       typeStr,
			IsRefEligible: isRefOptEligibleType(goType),
		}
		isRefOpt := false
		isMutRefOpt := false
		if e.OptimizeRefs && e.refOptReadOnly != nil {
			if readOnlyFlags, ok := e.refOptReadOnly.ReadOnly[e.refOptCurrentFunc]; ok {
				if paramIdx >= 0 && paramIdx < len(readOnlyFlags) && readOnlyFlags[paramIdx] {
					isRefOpt = true
				}
			}
			if !isRefOpt {
				if mutRefFlags, ok := e.refOptReadOnly.MutRef[e.refOptCurrentFunc]; ok {
					if paramIdx >= 0 && paramIdx < len(mutRefFlags) && mutRefFlags[paramIdx] {
						isMutRefOpt = true
					}
				}
			}
		}
		optMeta.IsReadOnly = isRefOpt
		optMeta.IsMutRef = isMutRefOpt
		// Always emit base form; RefOptPass transforms to const T& / T&
		paramNode := IRTree(Identifier, TagIdent,
			Leaf(Identifier, typeStr),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, name),
		)
		paramNode.OptMeta = optMeta
		e.fs.AddTree(paramNode)
		paramIdx++
	}
	e.currentParamIndex = paramIdx
}

func (e *CppEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
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

func (e *CppEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignature))
	// Tokens: result-type, name, params
	resultType := ""
	funcName := ""
	var paramsToken IRNode
	hasParams := false
	for _, t := range tokens {
		if t.Kind == TagIdent && funcName == "" {
			funcName = t.Serialize()
		} else if t.Kind == TagExpr {
			if resultType == "" {
				resultType = t.Serialize()
			} else {
				paramsToken = t
				hasParams = true
			}
		}
	}
	_ = hasParams

	if e.forwardDecl {
		if funcName == "main" {
			e.fs.AddTree(IRTree(TypeKeyword, TagExpr,
				Leaf(Identifier, resultType),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, funcName),
				Leaf(LeftParen, "("),
				Leaf(Identifier, "int argc, char* argv[]"),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		} else {
			e.fs.AddTree(IRTree(TypeKeyword, TagExpr,
				Leaf(Identifier, resultType),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, funcName),
				Leaf(LeftParen, "("),
				paramsToken,
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		}
	} else {
		if funcName == "main" {
			e.fs.AddTree(IRTree(TypeKeyword, TagExpr,
				Leaf(NewLine, "\n"),
				Leaf(Identifier, resultType),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, funcName),
				Leaf(LeftParen, "("),
				Leaf(Identifier, "int argc, char* argv[]"),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.AddTree(IRTree(TypeKeyword, TagExpr,
				Leaf(NewLine, "\n"),
				Leaf(Identifier, resultType),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, funcName),
				Leaf(LeftParen, "("),
				paramsToken,
				Leaf(RightParen, ")"),
			))
		}
	}
}

func (e *CppEmitter) PostVisitFuncDeclBody(node *ast.BlockStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclBody))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDecl))
	var children []IRNode
	if len(tokens) >= 1 {
		children = append(children, tokens[0]) // sig as tree (preserves OptMeta)
	}
	if len(tokens) >= 2 {
		if node.Name.Name == "main" {
			bodyCode := tokens[1].Serialize()
			if strings.HasPrefix(bodyCode, "{\n") {
				bodyCode = "{\n" + cppIndent(1) + "std::vector<std::string> goany_os_args(argv, argv + argc);\n" + bodyCode[2:]
			}
			children = append(children, Leaf(NewLine, "\n"))
			children = append(children, Leaf(Identifier, bodyCode))
		} else {
			children = append(children, Leaf(NewLine, "\n"))
			children = append(children, tokens[1])
		}
	}
	children = append(children, Leaf(NewLine, "\n"))
	children = append(children, Leaf(NewLine, "\n"))
	e.fs.AddTree(IRTree(FuncDeclaration, KindDecl, children...))
}

// Forward Declaration Signatures
func (e *CppEmitter) PreVisitFuncDeclSignatures(indent int) {
	e.forwardDecl = true
	e.fs.AddLeaf("// Forward declarations\n", KindExpr, nil)
}

func (e *CppEmitter) PostVisitFuncDeclSignatures(indent int) {
	e.forwardDecl = false
	// Let forward decl tokens flow through
}

// ============================================================
// Block Statements
// ============================================================

func (e *CppEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	e.indent = indent
}

func (e *CppEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBlockStmtList))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBlockStmt))
	var children []IRNode
	children = append(children, Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
	for _, t := range tokens {
		if t.Serialize() != "" {
			children = append(children, t)
		}
	}
	children = append(children, Leaf(WhiteSpace, cppIndent(indent)), Leaf(RightBrace, "}"))
	e.fs.AddTree(IRTree(BlockStatement, KindStmt, children...))
}

// ============================================================
// Expression Statements
// ============================================================

func (e *CppEmitter) PreVisitExprStmt(node *ast.ExprStmt, indent int) {
	e.indent = indent
}

func (e *CppEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitExprStmtX))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitExprStmt(node *ast.ExprStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitExprStmt))
	ind := cppIndent(indent)
	var children []IRNode
	children = append(children, Leaf(WhiteSpace, ind))
	if len(tokens) >= 1 {
		children = append(children, tokens[0])
	}
	children = append(children, Leaf(Semicolon, ";\n"))
	e.fs.AddTree(IRTree(ExprStatement, KindStmt, children...))
}

// ============================================================
// Assignment Statements
// ============================================================

func (e *CppEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
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

func (e *CppEmitter) PostVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	lhsCode := e.fs.CollectText(string(PreVisitAssignStmtLhsExpr))

	if indexExpr, ok := node.(*ast.IndexExpr); ok {
		if e.isMapTypeExpr(indexExpr.X) {
			e.mapAssignVar = e.lastIndexXCode
			e.mapAssignKey = e.lastIndexKeyCode
			e.fs.AddLeaf(lhsCode, KindExpr, nil)
			return
		}
	}
	e.fs.AddLeaf(lhsCode, KindExpr, nil)
}

func (e *CppEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitAssignStmtLhs))
	var lhsExprs []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			lhsExprs = append(lhsExprs, t.Serialize())
		}
	}
	e.fs.AddLeaf(strings.Join(lhsExprs, ", "), KindExpr, nil)
}

func (e *CppEmitter) PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitAssignStmtRhsExpr))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitAssignStmtRhs))
	var nonEmpty []IRNode
	for _, t := range tokens {
		if t.Serialize() != "" {
			nonEmpty = append(nonEmpty, t)
		}
	}
	if len(nonEmpty) == 1 {
		e.fs.AddTree(nonEmpty[0])
	} else if len(nonEmpty) > 1 {
		var children []IRNode
		for i, t := range nonEmpty {
			if i > 0 {
				children = append(children, IRNode{Type: Comma, Content: ", "})
			}
			children = append(children, t)
		}
		e.fs.AddTree(IRTree(Identifier, KindExpr, children...))
	}
}

func (e *CppEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	e.currentAssignLhsNames = nil
	tokens := e.fs.CollectForest(string(PreVisitAssignStmt))
	lhsStr := ""
	rhsStr := ""
	if len(tokens) >= 1 {
		lhsStr = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		rhsStr = tokens[1].Serialize()
	}

	ind := cppIndent(indent)

	// Pointer alias elimination: emit comment instead of assignment
	if len(node.Lhs) == 1 {
		if lhsIdent, ok := node.Lhs[0].(*ast.Ident); ok {
			if comment, ok := PtrLocalComments[lhsIdent.Pos()]; ok {
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(LineComment, comment),
					Leaf(NewLine, "\n"),
				))
				return
			}
		}
	}

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

				// Prologue: extract temp variables for INTERMEDIATE map accesses only
				for i, op := range ops {
					if op.accessType == "map" && i < lastIdx {
						key := op.keyExpr
						if op.keyCastPfx != "" {
							key = op.keyCastPfx + key + op.keyCastSfx
						}
						e.fs.AddTree(IRTree(AssignStatement, KindStmt,
							Leaf(WhiteSpace, ind),
							LeafTag(Keyword, "auto", TagCpp),
							Leaf(WhiteSpace, " "),
							Leaf(Identifier, op.tempVarName),
							Leaf(WhiteSpace, " "),
							Leaf(Assignment, "="),
							Leaf(WhiteSpace, " "),
							Leaf(Identifier, "std::any_cast"),
							Leaf(LeftAngle, "<"),
							Leaf(Identifier, op.valueCppType),
							Leaf(RightAngle, ">"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, "hmap::hashMapGet"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, op.mapVarExpr),
							Leaf(Comma, ", "),
							Leaf(Identifier, key),
							Leaf(RightParen, ")"),
							Leaf(RightParen, ")"),
							Leaf(Semicolon, ";"),
							Leaf(NewLine, "\n"),
						))
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
					e.fs.AddTree(IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, lastOp.mapVarExpr),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "hmap::hashMapSet"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, lastOp.mapVarExpr),
						Leaf(Comma, ", "),
						Leaf(Identifier, key),
						Leaf(Comma, ", "),
						tokens[1],
						Leaf(RightParen, ")"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
				} else {
					// Last op is slice: assign to slice element
					e.fs.AddTree(IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, currentVar),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, tokStr),
						Leaf(WhiteSpace, " "),
						tokens[1],
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
				}

				// Epilogue: write back INTERMEDIATE map entries in reverse
				for i := lastIdx - 1; i >= 0; i-- {
					op := ops[i]
					if op.accessType == "map" {
						key := op.keyExpr
						if op.keyCastPfx != "" {
							key = op.keyCastPfx + key + op.keyCastSfx
						}
						e.fs.AddTree(IRTree(AssignStatement, KindStmt,
							Leaf(WhiteSpace, ind),
							Leaf(Identifier, op.mapVarExpr),
							Leaf(WhiteSpace, " "),
							Leaf(Assignment, "="),
							Leaf(WhiteSpace, " "),
							Leaf(Identifier, "hmap::hashMapSet"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, op.mapVarExpr),
							Leaf(Comma, ", "),
							Leaf(Identifier, key),
							Leaf(Comma, ", "),
							Leaf(Identifier, op.tempVarName),
							Leaf(RightParen, ")"),
							Leaf(Semicolon, ";"),
							Leaf(NewLine, "\n"),
						))
					}
				}
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

		// Compute rhsNode: preserve tree when rhsStr is unmodified
		rhsNode := Leaf(Identifier, rhsStr)
		if len(tokens) >= 2 && rhsStr == tokens[1].Serialize() {
			rhsNode = tokens[1]
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
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					LeafTag(Keyword, "auto", TagCpp),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, tempVar),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "std::any_cast"),
					Leaf(LeftAngle, "<"),
					Leaf(Identifier, "hmap::HashMap"),
					Leaf(RightAngle, ">"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, "hmap::hashMapGet"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, outerVar),
					Leaf(Comma, ", "),
					Leaf(Identifier, outerKey),
					Leaf(RightParen, ")"),
					Leaf(RightParen, ")"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, tempVar),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "hmap::hashMapSet"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, tempVar),
					Leaf(Comma, ", "),
					Leaf(Identifier, mapKey),
					Leaf(Comma, ", "),
					rhsNode,
					Leaf(RightParen, ")"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, outerVar),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "hmap::hashMapSet"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, outerVar),
					Leaf(Comma, ", "),
					Leaf(Identifier, outerKey),
					Leaf(Comma, ", "),
					Leaf(Identifier, tempVar),
					Leaf(RightParen, ")"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
				e.mapAssignVar = ""
				e.mapAssignKey = ""
				return
			}
		}

		e.fs.AddTree(IRTree(AssignStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(Identifier, mapVar),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, "="),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "hmap::hashMapSet"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, mapVar),
			Leaf(Comma, ", "),
			Leaf(Identifier, mapKey),
			Leaf(Comma, ", "),
			rhsNode,
			Leaf(RightParen, ")"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
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
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, decl+okName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "hmap::hashMapContains"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, mapName),
					Leaf(Comma, ", "),
					Leaf(Identifier, keyStr),
					Leaf(RightParen, ")"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, decl+valName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, okName),
					Leaf(WhiteSpace, " "),
					Leaf(BinaryOperator, "?"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "std::any_cast"),
					Leaf(LeftAngle, "<"),
					Leaf(Identifier, valueCppType),
					Leaf(RightAngle, ">"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, "hmap::hashMapGet"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, mapName),
					Leaf(Comma, ", "),
					Leaf(Identifier, keyStr),
					Leaf(RightParen, ")"),
					Leaf(RightParen, ")"),
					Leaf(WhiteSpace, " "),
					Leaf(Colon, ":"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, valueCppType),
					Leaf(LeftBrace, "{"),
					Leaf(RightBrace, "}"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
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
			e.fs.AddTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, decl+okName),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(LeftParen, "("),
				Leaf(Identifier, "std::any"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, xExpr),
				Leaf(RightParen, ")"),
				Leaf(RightParen, ")"),
				Leaf(Dot, "."),
				Leaf(Identifier, "type"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(ComparisonOperator, "=="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "typeid"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, typeName),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
			e.fs.AddTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, decl+valName),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, okName),
				Leaf(WhiteSpace, " "),
				Leaf(BinaryOperator, "?"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "std::any_cast"),
				Leaf(LeftAngle, "<"),
				Leaf(Identifier, typeName),
				Leaf(RightAngle, ">"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, "std::any"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, xExpr),
				Leaf(RightParen, ")"),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Colon, ":"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, typeName),
				Leaf(LeftBrace, "{"),
				Leaf(RightBrace, "}"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
			return
		}
	}

	// Multi-value return: a, b := func() → auto [a, b] = func()
	if len(node.Lhs) > 1 && len(node.Rhs) == 1 {
		lhsParts := make([]string, len(node.Lhs))
		tieParts := make([]string, len(node.Lhs))
		for i, lhs := range node.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				if ident.Name == "_" {
					lhsParts[i] = fmt.Sprintf("__blank_%d", e.blankCounter)
					e.blankCounter = e.blankCounter + 1
					tieParts[i] = "std::ignore"
				} else {
					lhsParts[i] = ident.Name
					tieParts[i] = ident.Name
				}
			} else {
				lhsParts[i] = exprToString(lhs)
				tieParts[i] = exprToString(lhs)
			}
		}
		if tokStr == ":=" {
			e.fs.AddTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				LeafTag(Keyword, "auto", TagCpp),
				Leaf(WhiteSpace, " "),
				Leaf(LeftBracket, "["),
				Leaf(Identifier, strings.Join(lhsParts, ", ")),
				Leaf(RightBracket, "]"),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				tokens[1],
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		} else {
			e.fs.AddTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, "std::tie"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, strings.Join(tieParts, ", ")),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				tokens[1],
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
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

	// Compute rhsNode: preserve tree when rhsStr is unmodified
	rhsNode := Leaf(Identifier, rhsStr)
	if len(tokens) >= 2 && rhsStr == tokens[1].Serialize() {
		rhsNode = tokens[1]
	}

	switch tokStr {
	case ":=":
		// Check if RHS is a string literal → use std::string instead of auto
		if len(node.Rhs) == 1 {
			if lit, ok := node.Rhs[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, "std::string"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, lhsStr),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					rhsNode,
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
			} else {
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					LeafTag(Keyword, "auto", TagCpp),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, lhsStr),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					rhsNode,
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
			}
		} else {
			e.fs.AddTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				LeafTag(Keyword, "auto", TagCpp),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, lhsStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				rhsNode,
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		}
	case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
		e.fs.AddTree(IRTree(AssignStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(Identifier, lhsStr),
			Leaf(WhiteSpace, " "),
			Leaf(BinaryOperator, tokStr),
			Leaf(WhiteSpace, " "),
			rhsNode,
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	default:
		e.fs.AddTree(IRTree(AssignStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(Identifier, lhsStr),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, "="),
			Leaf(WhiteSpace, " "),
			rhsNode,
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	}
}

// ============================================================
// Declaration Statements
// ============================================================

func (e *CppEmitter) PreVisitDeclStmt(node *ast.DeclStmt, indent int) {
	e.indent = indent
}

func (e *CppEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitDeclStmtValueSpecType))
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
	e.fs.AddLeaf(typeStr, TagType, goType)
}

func (e *CppEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	e.fs.CollectForest(string(PreVisitDeclStmtValueSpecNames))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *CppEmitter) PostVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitDeclStmtValueSpecValue))
	if len(tokens) == 1 {
		t := tokens[0]
		t.Kind = TagExpr
		e.fs.AddTree(t)
	} else {
		valCode := ""
		for _, t := range tokens {
			valCode += t.Serialize()
		}
		e.fs.AddLeaf(valCode, TagExpr, nil)
	}
}

func (e *CppEmitter) PostVisitDeclStmt(node *ast.DeclStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitDeclStmt))
	ind := cppIndent(indent)

	var children []IRNode
	i := 0
	for i < len(tokens) {
		typeStr := ""
		nameStr := ""
		valueStr := ""
		valueToken := Leaf(Identifier, "")
		var goType types.Type

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
			valueToken = tokens[i]
			i++
		}

		if nameStr == "" {
			continue
		}

		if valueStr != "" {
			valueNode := func() IRNode {
				if valueStr == valueToken.Serialize() {
					return valueToken
				}
				return Leaf(Identifier, valueStr)
			}()
			children = append(children,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, typeStr),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, nameStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				valueNode,
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
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
					children = append(children,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, typeStr),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, nameStr),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "hmap::newHashMap"),
						Leaf(LeftParen, "("),
						Leaf(NumberLiteral, fmt.Sprintf("%d", keyType)),
						Leaf(RightParen, ")"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					)
					continue
				}
			}
			children = append(children,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, typeStr),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, nameStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		}
	}
	if len(children) > 0 {
		e.fs.AddTree(IRTree(DeclStatement, KindStmt, children...))
	}
}

// ============================================================
// Return Statements
// ============================================================

func (e *CppEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	e.indent = indent
}

func (e *CppEmitter) PostVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitReturnStmtResult))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitReturnStmt))
	ind := cppIndent(indent)

	if len(tokens) == 0 {
		e.fs.AddTree(IRTree(ReturnStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	} else if len(tokens) == 1 {
		e.fs.AddTree(IRTree(ReturnStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return"),
			Leaf(WhiteSpace, " "),
			tokens[0],
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	} else {
		var vals []string
		for _, t := range tokens {
			vals = append(vals, t.Serialize())
		}
		e.fs.AddTree(IRTree(ReturnStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "std::make_tuple"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, strings.Join(vals, ", ")),
			Leaf(RightParen, ")"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	}
}

// ============================================================
// If Statements
// ============================================================

func (e *CppEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	e.indent = indent
	e.ifInitStack = append(e.ifInitStack, "")
	e.ifCondStack = append(e.ifCondStack, "")
	e.ifBodyStack = append(e.ifBodyStack, "")
	e.ifElseStack = append(e.ifElseStack, "")
	e.ifInitNodes = append(e.ifInitNodes, IRNode{})
	e.ifCondNodes = append(e.ifCondNodes, IRNode{})
	e.ifBodyNodes = append(e.ifBodyNodes, IRNode{})
	e.ifElseNodes = append(e.ifElseNodes, IRNode{})
}

func (e *CppEmitter) PostVisitIfStmtInit(node ast.Stmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIfStmtInit))
	e.ifInitNodes[len(e.ifInitNodes)-1] = collectToNode(tokens)
	e.ifInitStack[len(e.ifInitStack)-1] = e.ifInitNodes[len(e.ifInitNodes)-1].Serialize()
}

func (e *CppEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIfStmtCond))
	e.ifCondNodes[len(e.ifCondNodes)-1] = collectToNode(tokens)
	e.ifCondStack[len(e.ifCondStack)-1] = e.ifCondNodes[len(e.ifCondNodes)-1].Serialize()
}

func (e *CppEmitter) PostVisitIfStmtBody(node *ast.IfStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIfStmtBody))
	e.ifBodyNodes[len(e.ifBodyNodes)-1] = collectToNode(tokens)
	e.ifBodyStack[len(e.ifBodyStack)-1] = e.ifBodyNodes[len(e.ifBodyNodes)-1].Serialize()
}

func (e *CppEmitter) PostVisitIfStmtElse(node *ast.IfStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIfStmtElse))
	e.ifElseNodes[len(e.ifElseNodes)-1] = collectToNode(tokens)
	e.ifElseStack[len(e.ifElseStack)-1] = e.ifElseNodes[len(e.ifElseNodes)-1].Serialize()
}

func (e *CppEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	e.fs.CollectForest(string(PreVisitIfStmt))
	ind := cppIndent(indent)

	n := len(e.ifInitStack)
	initCode := e.ifInitStack[n-1]
	_ = e.ifCondStack[n-1]
	_ = e.ifBodyStack[n-1]
	elseCode := e.ifElseStack[n-1]
	initNode := e.ifInitNodes[n-1]
	condNode := e.ifCondNodes[n-1]
	bodyNode := e.ifBodyNodes[n-1]
	elseNode := e.ifElseNodes[n-1]
	e.ifInitStack = e.ifInitStack[:n-1]
	e.ifCondStack = e.ifCondStack[:n-1]
	e.ifBodyStack = e.ifBodyStack[:n-1]
	e.ifElseStack = e.ifElseStack[:n-1]
	e.ifInitNodes = e.ifInitNodes[:n-1]
	e.ifCondNodes = e.ifCondNodes[:n-1]
	e.ifBodyNodes = e.ifBodyNodes[:n-1]
	e.ifElseNodes = e.ifElseNodes[:n-1]

	var children []IRNode
	if initCode != "" {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(LeftBrace, "{"),
			Leaf(NewLine, "\n"),
			initNode,
			Leaf(WhiteSpace, ind),
			Leaf(IfKeyword, "if"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			condNode,
			Leaf(RightParen, ")"),
			Leaf(NewLine, "\n"),
			bodyNode,
		)
	} else {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(IfKeyword, "if"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			condNode,
			Leaf(RightParen, ")"),
			Leaf(NewLine, "\n"),
			bodyNode,
		)
	}
	if elseCode != "" {
		trimmed := strings.TrimLeft(elseCode, " \t\n")
		if strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "if(") {
			elseIfNode := stripLeadingWhitespace(elseNode)
			children = append(children,
				Leaf(NewLine, "\n"),
				Leaf(ElseKeyword, "else"),
				Leaf(WhiteSpace, " "),
				elseIfNode,
			)
		} else {
			children = append(children,
				Leaf(NewLine, "\n"),
				Leaf(ElseKeyword, "else"),
				Leaf(NewLine, "\n"),
				elseNode,
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
	e.fs.AddTree(IRTree(IfStatement, KindStmt, children...))
}

// ============================================================
// For Statements
// ============================================================

func (e *CppEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	e.indent = indent
	e.forInitStack = append(e.forInitStack, "")
	e.forCondStack = append(e.forCondStack, "")
	e.forPostStack = append(e.forPostStack, "")
	e.forCondNodes = append(e.forCondNodes, IRNode{})
	e.forBodyNodes = append(e.forBodyNodes, IRNode{})
}

func (e *CppEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitForStmtInit))
	initCode := collectToNode(tokens).Serialize()
	initCode = strings.TrimRight(initCode, ";\n \t")
	initCode = strings.TrimLeft(initCode, " \t")
	e.forInitStack[len(e.forInitStack)-1] = initCode
}

func (e *CppEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitForStmtCond))
	e.forCondNodes[len(e.forCondNodes)-1] = collectToNode(tokens)
	e.forCondStack[len(e.forCondStack)-1] = e.forCondNodes[len(e.forCondNodes)-1].Serialize()
}

func (e *CppEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitForStmtPost))
	postCode := collectToNode(tokens).Serialize()
	postCode = strings.TrimRight(postCode, ";\n \t")
	postCode = strings.TrimLeft(postCode, " \t")
	e.forPostStack[len(e.forPostStack)-1] = postCode
}

func (e *CppEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {
	bodyTokens := e.fs.CollectForest(string(PreVisitForStmt))
	bodyNode := collectToNode(bodyTokens)
	_ = bodyNode.Serialize()
	ind := cppIndent(indent)

	n := len(e.forInitStack)
	initCode := e.forInitStack[n-1]
	_ = e.forCondStack[n-1]
	condNode := e.forCondNodes[n-1]
	postCode := e.forPostStack[n-1]
	e.forInitStack = e.forInitStack[:n-1]
	e.forCondStack = e.forCondStack[:n-1]
	e.forPostStack = e.forPostStack[:n-1]
	e.forCondNodes = e.forCondNodes[:n-1]
	e.forBodyNodes = e.forBodyNodes[:n-1]

	if node.Init == nil && node.Cond == nil && node.Post == nil {
		e.fs.AddTree(IRTree(ForStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			Leaf(Semicolon, ";"),
			Leaf(Semicolon, ";"),
			Leaf(RightParen, ")"),
			Leaf(NewLine, "\n"),
			bodyNode,
			Leaf(NewLine, "\n"),
		))
		return
	}
	if node.Init == nil && node.Post == nil && node.Cond != nil {
		e.fs.AddTree(IRTree(ForStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			Leaf(Semicolon, ";"),
			condNode,
			Leaf(Semicolon, ";"),
			Leaf(RightParen, ")"),
			Leaf(NewLine, "\n"),
			bodyNode,
			Leaf(NewLine, "\n"),
		))
		return
	}
	e.fs.AddTree(IRTree(ForStatement, KindStmt,
		Leaf(WhiteSpace, ind),
		Leaf(ForKeyword, "for"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftParen, "("),
		Leaf(Identifier, initCode),
		Leaf(Semicolon, ";"),
		condNode,
		Leaf(Semicolon, ";"),
		Leaf(Identifier, postCode),
		Leaf(RightParen, ")"),
		Leaf(NewLine, "\n"),
		bodyNode,
		Leaf(NewLine, "\n"),
	))
}
// ============================================================
// Range Statements
// ============================================================

func (e *CppEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	e.indent = indent
}

func (e *CppEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitRangeStmtKey))
	for _, t := range tokens {
		t.Kind = TagIdent
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitRangeStmtValue))
	for _, t := range tokens {
		t.Kind = TagIdent
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitRangeStmtX))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitRangeStmt))
	ind := cppIndent(indent)

	keyCode := ""
	valCode := ""
	xCode := ""
	bodyNode := Leaf(Identifier, "")

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
		bodyNode = tokens[idx]
	}

	if node.Key == nil && valCode != "" {
		keyCode = "_"
	}

	// Check if ranging over a map
	isMap := false
	if node.X != nil {
		isMap = e.isMapTypeExpr(node.X)
	}

	if isMap {
		// Map range: iterate using hashMapKeys
		mapGoType := e.getExprGoType(node.X)
		valueCppType := "std::any"
		keyCppType := "std::any"
		if mapGoType != nil {
			if mapUnderlying, ok := mapGoType.Underlying().(*types.Map); ok {
				valueCppType = getCppTypeName(mapUnderlying.Elem())
				keyCppType = getCppTypeName(mapUnderlying.Key())
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
				Leaf(WhiteSpace, ind+"    "),
				LeafTag(Keyword, "auto", TagCpp),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, keysVar),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "hmap::hashMapKeys"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, xCode),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"    "),
				Leaf(ForKeyword, "for"),
				Leaf(WhiteSpace, " "),
				Leaf(LeftParen, "("),
				Leaf(Identifier, "size_t"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, loopIdx),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(NumberLiteral, "0"),
				Leaf(Semicolon, "; "),
				Leaf(Identifier, loopIdx),
				Leaf(WhiteSpace, " "),
				Leaf(ComparisonOperator, "<"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, keysVar),
				Leaf(Dot, "."),
				Leaf(Identifier, "size"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, "; "),
				Leaf(Identifier, loopIdx),
				Leaf(UnaryOperator, "++"),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(LeftBrace, "{"),
				Leaf(NewLine, "\n"),
			)
			if keyCode != "_" {
				children = append(children,
					Leaf(WhiteSpace, ind+"        "),
					LeafTag(Keyword, "auto", TagCpp),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, keyCode),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "std::any_cast"),
					Leaf(LeftAngle, "<"),
					Leaf(Identifier, keyCppType),
					Leaf(RightAngle, ">"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, keysVar),
					Leaf(LeftBracket, "["),
					Leaf(Identifier, loopIdx),
					Leaf(RightBracket, "]"),
					Leaf(RightParen, ")"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				)
			}
			children = append(children,
				Leaf(WhiteSpace, ind+"        "),
				LeafTag(Keyword, "auto", TagCpp),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, valCode),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "std::any_cast"),
				Leaf(LeftAngle, "<"),
				Leaf(Identifier, valueCppType),
				Leaf(RightAngle, ">"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, "hmap::hashMapGet"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, xCode),
				Leaf(Comma, ", "),
				Leaf(Identifier, keysVar),
				Leaf(LeftBracket, "["),
				Leaf(Identifier, loopIdx),
				Leaf(RightBracket, "]"),
				Leaf(RightParen, ")"),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"        "),
				bodyNode,
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"    "),
				Leaf(RightBrace, "}"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind),
				Leaf(RightBrace, "}"),
				Leaf(NewLine, "\n"),
			)
			e.fs.AddTree(IRTree(RangeStatement, KindStmt, children...))
		} else {
			var children []IRNode
			children = append(children,
				Leaf(WhiteSpace, ind),
				Leaf(LeftBrace, "{"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"    "),
				LeafTag(Keyword, "auto", TagCpp),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, keysVar),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, "hmap::hashMapKeys"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, xCode),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"    "),
				Leaf(ForKeyword, "for"),
				Leaf(WhiteSpace, " "),
				Leaf(LeftParen, "("),
				Leaf(Identifier, "size_t"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, loopIdx),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(NumberLiteral, "0"),
				Leaf(Semicolon, "; "),
				Leaf(Identifier, loopIdx),
				Leaf(WhiteSpace, " "),
				Leaf(ComparisonOperator, "<"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, keysVar),
				Leaf(Dot, "."),
				Leaf(Identifier, "size"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, "; "),
				Leaf(Identifier, loopIdx),
				Leaf(UnaryOperator, "++"),
				Leaf(RightParen, ")"),
				Leaf(WhiteSpace, " "),
				Leaf(LeftBrace, "{"),
				Leaf(NewLine, "\n"),
			)
			if keyCode != "_" {
				children = append(children,
					Leaf(WhiteSpace, ind+"        "),
					LeafTag(Keyword, "auto", TagCpp),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, keyCode),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "std::any_cast"),
					Leaf(LeftAngle, "<"),
					Leaf(Identifier, keyCppType),
					Leaf(RightAngle, ">"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, keysVar),
					Leaf(LeftBracket, "["),
					Leaf(Identifier, loopIdx),
					Leaf(RightBracket, "]"),
					Leaf(RightParen, ")"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				)
			}
			children = append(children,
				Leaf(WhiteSpace, ind+"        "),
				bodyNode,
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"    "),
				Leaf(RightBrace, "}"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind),
				Leaf(RightBrace, "}"),
				Leaf(NewLine, "\n"),
			)
			e.fs.AddTree(IRTree(RangeStatement, KindStmt, children...))
		}
		return
	}

	// If range expression is an inline composite literal, emit a temp variable
	if _, isCompLit := node.X.(*ast.CompositeLit); isCompLit {
		tmpVar := fmt.Sprintf("_range%d", e.rangeVarCounter)
		e.rangeVarCounter++
		origXCode := xCode
		xCode = tmpVar
		// Generate the loop inside the block
		if valCode != "" && valCode != "_" {
			loopVar := keyCode
			if loopVar == "_" || loopVar == "" {
				loopVar = "_i"
			}
			valDecl := fmt.Sprintf("%s        auto %s = %s[%s];\n", ind, valCode, xCode, loopVar)
			injectedBody := injectIntoBlock(bodyNode, Leaf(Identifier, valDecl))
			e.fs.AddTree(IRTree(RangeStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(LeftBrace, "{"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"    "),
				LeafTag(Keyword, "auto", TagCpp),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, tmpVar),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, origXCode),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"    "),
				Leaf(ForKeyword, "for"),
				Leaf(WhiteSpace, " "),
				Leaf(LeftParen, "("),
				Leaf(Identifier, "size_t"),
				Leaf(WhiteSpace, " "),
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
				Leaf(Identifier, xCode),
				Leaf(Dot, "."),
				Leaf(Identifier, "size"),
				Leaf(LeftParen, "("),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, "; "),
				Leaf(Identifier, loopVar),
				Leaf(UnaryOperator, "++"),
				Leaf(RightParen, ")"),
				Leaf(NewLine, "\n"),
				injectedBody,
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind),
				Leaf(RightBrace, "}"),
				Leaf(NewLine, "\n"),
			))
		} else {
			loopVar := keyCode
			if loopVar == "_" || loopVar == "" {
				loopVar = "_i"
			}
			e.fs.AddTree(IRTree(RangeStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(LeftBrace, "{"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"    "),
				LeafTag(Keyword, "auto", TagCpp),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, tmpVar),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, origXCode),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind+"    "),
				Leaf(ForKeyword, "for"),
				Leaf(WhiteSpace, " "),
				Leaf(LeftParen, "("),
				LeafTag(Keyword, "auto", TagCpp),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, loopVar),
				Leaf(WhiteSpace, " "),
				Leaf(Colon, ":"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, xCode),
				Leaf(RightParen, ")"),
				Leaf(NewLine, "\n"),
				bodyNode,
				Leaf(NewLine, "\n"),
				Leaf(WhiteSpace, ind),
				Leaf(RightBrace, "}"),
				Leaf(NewLine, "\n"),
			))
		}
		return
	}

	// Key-value range
	if valCode != "" && valCode != "_" {
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = "_i"
		}

		valDecl := fmt.Sprintf("%s    auto %s = %s[%s];\n", ind, valCode, xCode, loopVar)
		injectedBody := injectIntoBlock(bodyNode, Leaf(Identifier, valDecl))

		e.fs.AddTree(IRTree(RangeStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			Leaf(Identifier, "size_t"),
			Leaf(WhiteSpace, " "),
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
			Leaf(Identifier, xCode),
			Leaf(Dot, "."),
			Leaf(Identifier, "size"),
			Leaf(LeftParen, "("),
			Leaf(RightParen, ")"),
			Leaf(Semicolon, "; "),
			Leaf(Identifier, loopVar),
			Leaf(UnaryOperator, "++"),
			Leaf(RightParen, ")"),
			Leaf(NewLine, "\n"),
			injectedBody,
			Leaf(NewLine, "\n"),
		))
	} else {
		// Key-only range → for (auto key : collection)
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = "_i"
		}
		e.fs.AddTree(IRTree(RangeStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			LeafTag(Keyword, "auto", TagCpp),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, loopVar),
			Leaf(WhiteSpace, " "),
			Leaf(Colon, ":"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, xCode),
			Leaf(RightParen, ")"),
			Leaf(NewLine, "\n"),
			bodyNode,
			Leaf(NewLine, "\n"),
		))
	}
}

// ============================================================
// Switch / Case Statements
// ============================================================

func (e *CppEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	e.indent = indent
}

func (e *CppEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSwitchStmtTag))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSwitchStmt))
	ind := cppIndent(indent)

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
		children = append(children, tokens[i])
	}
	children = append(children,
		Leaf(WhiteSpace, ind),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
	)
	e.fs.AddTree(IRTree(SwitchStatement, KindStmt, children...))
}

func (e *CppEmitter) PreVisitCaseClause(node *ast.CaseClause, indent int) {
	e.indent = indent
}

func (e *CppEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCaseClauseListExpr))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *CppEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCaseClauseList))
	var exprs []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			exprs = append(exprs, t.Serialize())
		}
	}
	e.fs.AddLeaf(strings.Join(exprs, ", "), KindExpr, nil)
}

func (e *CppEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCaseClause))
	ind := cppIndent(indent)

	var children []IRNode
	idx := 0
	if len(node.List) == 0 {
		// Default case: AddLeaf("") is a no-op (empty tokens are dropped),
		// so all tokens on the stack are body statements.
		children = append(children,
			Leaf(WhiteSpace, ind+"  "),
			Leaf(DefaultKeyword, "default"),
			Leaf(Colon, ":"),
			Leaf(NewLine, "\n"),
		)
	} else {
		// Regular case: token[0] is case expressions, rest is body
		caseExprs := ""
		if idx < len(tokens) {
			caseExprs = tokens[idx].Serialize()
			idx++
		}
		vals := strings.Split(caseExprs, ", ")
		for _, v := range vals {
			children = append(children,
				Leaf(WhiteSpace, ind+"  "),
				Leaf(CaseKeyword, "case"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, v),
				Leaf(Colon, ":"),
				Leaf(NewLine, "\n"),
			)
		}
	}
	for i := idx; i < len(tokens); i++ {
		children = append(children, tokens[i])
	}
	children = append(children,
		Leaf(WhiteSpace, ind+"    "),
		Leaf(BreakKeyword, "break"),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
	)
	e.fs.AddTree(IRTree(CaseClauseStatement, KindStmt, children...))
}

// ============================================================
// Inc/Dec Statements
// ============================================================

func (e *CppEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIncDecStmt))
	xNode := collectToNode(tokens)
	ind := cppIndent(indent)
	e.fs.AddTree(IRTree(CaseClauseStatement, KindStmt,
		Leaf(WhiteSpace, ind),
		xNode,
		Leaf(UnaryOperator, node.Tok.String()),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
	))
}

// ============================================================
// Branch Statements (break, continue)
// ============================================================

func (e *CppEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	ind := cppIndent(indent)
	switch node.Tok {
	case token.BREAK:
		e.fs.AddTree(IRTree(BranchStatement, KindStmt, Leaf(Identifier, ind+"break;\n")))
	case token.CONTINUE:
		e.fs.AddTree(IRTree(BranchStatement, KindStmt, Leaf(Identifier, ind+"continue;\n")))
	}
}

// ============================================================
// Struct Declarations
// ============================================================

func (e *CppEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	typeCode := e.fs.CollectText(string(PreVisitGenStructFieldType))
	e.fs.AddLeaf(typeCode, KindExpr, nil)
}

func (e *CppEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	e.fs.CollectForest(string(PreVisitGenStructFieldName))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *CppEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitGenStructInfo))

	if node.Struct == nil {
		return
	}

	// Collect type-name pairs
	var children []IRNode
	children = append(children,
		Leaf(StructKeyword, "struct"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, node.Name),
		Leaf(NewLine, "\n"),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
	)

	i := 0
	for i < len(tokens) {
		typeStr := ""
		nameStr := ""
		if i < len(tokens) && tokens[i].Kind == TagExpr {
			typeStr = tokens[i].Serialize()
			i++
		}
		if i < len(tokens) && tokens[i].Kind == TagIdent {
			nameStr = tokens[i].Serialize()
			i++
		}
		if typeStr != "" && nameStr != "" {
			children = append(children,
				Leaf(Identifier, typeStr),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, nameStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		}
	}
	children = append(children,
		Leaf(RightBrace, "}"),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n\n"),
	)

	// Generate operator== and std::hash for hashable structs
	if node.Struct != nil && e.structHasOnlyPrimitiveFields(node.Name) {
		e.generateStructHashAndEquality(node)
	}

	e.fs.AddTree(IRTree(TypeKeyword, TagExpr, children...))
}

func (e *CppEmitter) PostVisitGenStructInfos(node []GenTypeInfo, indent int) {
	// Let struct code flow through
}

// structHasOnlyPrimitiveFields checks if a struct has only hashable fields
func (e *CppEmitter) structHasOnlyPrimitiveFields(structName string) bool {
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

func (e *CppEmitter) isHashableType(t types.Type, visited map[string]bool) bool {
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

func (e *CppEmitter) generateStructHashAndEquality(node GenTypeInfo) {
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
func (e *CppEmitter) ensureDependencyHashSpecs(structType *ast.StructType, visited map[string]bool) {
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

func (e *CppEmitter) emitHashSpec(spec pendingHashSpec) {
	qualifiedName := spec.structName
	if spec.pkgName != "" {
		qualifiedName = spec.pkgName + "::" + spec.structName
	}

	// operator==
	var eqChildren []IRNode
	eqChildren = append(eqChildren,
		Leaf(Keyword, "inline"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "bool"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "operator=="),
		Leaf(LeftParen, "("),
		LeafTag(Keyword, "const", TagCpp),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, qualifiedName),
		Leaf(Identifier, "&"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "a"),
		Leaf(Comma, ", "),
		LeafTag(Keyword, "const", TagCpp),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, qualifiedName),
		Leaf(Identifier, "&"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "b"),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "    "),
		Leaf(ReturnKeyword, "return"),
		Leaf(WhiteSpace, " "),
	)
	for i, fieldName := range spec.fieldNames {
		if i > 0 {
			eqChildren = append(eqChildren,
				Leaf(WhiteSpace, " "),
				Leaf(LogicalOperator, "&&"),
				Leaf(WhiteSpace, " "),
			)
		}
		eqChildren = append(eqChildren,
			Leaf(Identifier, "a"),
			Leaf(Dot, "."),
			Leaf(Identifier, fieldName),
			Leaf(WhiteSpace, " "),
			Leaf(ComparisonOperator, "=="),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "b"),
			Leaf(Dot, "."),
			Leaf(Identifier, fieldName),
		)
	}
	if len(spec.fieldNames) == 0 {
		eqChildren = append(eqChildren, Leaf(BooleanLiteral, "true"))
	}
	eqChildren = append(eqChildren,
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n\n"),
	)
	e.fs.AddTree(IRTree(TypeKeyword, TagExpr, eqChildren...))

	// std::hash
	var hashChildren []IRNode
	hashChildren = append(hashChildren,
		Leaf(Keyword, "namespace"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "std"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "    "),
		Leaf(Identifier, "template"),
		Leaf(LeftAngle, "<"),
		Leaf(RightAngle, ">"),
		Leaf(WhiteSpace, " "),
		Leaf(StructKeyword, "struct"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "hash"),
		Leaf(LeftAngle, "<"),
		Leaf(Identifier, qualifiedName),
		Leaf(RightAngle, ">"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "        "),
		Leaf(Identifier, "size_t"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "operator()"),
		Leaf(LeftParen, "("),
		LeafTag(Keyword, "const", TagCpp),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, qualifiedName),
		Leaf(Identifier, "&"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "s"),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		LeafTag(Keyword, "const", TagCpp),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "            "),
		Leaf(Identifier, "size_t"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "h"),
		Leaf(WhiteSpace, " "),
		Leaf(Assignment, "="),
		Leaf(WhiteSpace, " "),
		Leaf(NumberLiteral, "0"),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
	)
	for _, fieldName := range spec.fieldNames {
		hashChildren = append(hashChildren,
			Leaf(WhiteSpace, "            "),
			Leaf(Identifier, "h"),
			Leaf(WhiteSpace, " "),
			Leaf(BinaryOperator, "^="),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, "std::hash"),
			Leaf(LeftAngle, "<"),
			Leaf(Identifier, "decltype"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, "s"),
			Leaf(Dot, "."),
			Leaf(Identifier, fieldName),
			Leaf(RightParen, ")"),
			Leaf(RightAngle, ">"),
			Leaf(LeftBrace, "{"),
			Leaf(RightBrace, "}"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, "s"),
			Leaf(Dot, "."),
			Leaf(Identifier, fieldName),
			Leaf(RightParen, ")"),
			Leaf(WhiteSpace, " "),
			Leaf(BinaryOperator, "+"),
			Leaf(WhiteSpace, " "),
			Leaf(NumberLiteral, "0x9e3779b9"),
			Leaf(WhiteSpace, " "),
			Leaf(BinaryOperator, "+"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			Leaf(Identifier, "h"),
			Leaf(WhiteSpace, " "),
			Leaf(BinaryOperator, "<<"),
			Leaf(WhiteSpace, " "),
			Leaf(NumberLiteral, "6"),
			Leaf(RightParen, ")"),
			Leaf(WhiteSpace, " "),
			Leaf(BinaryOperator, "+"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			Leaf(Identifier, "h"),
			Leaf(WhiteSpace, " "),
			Leaf(BinaryOperator, ">>"),
			Leaf(WhiteSpace, " "),
			Leaf(NumberLiteral, "2"),
			Leaf(RightParen, ")"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		)
	}
	hashChildren = append(hashChildren,
		Leaf(WhiteSpace, "            "),
		Leaf(ReturnKeyword, "return"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "h"),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "        "),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n"),
		Leaf(WhiteSpace, "    "),
		Leaf(RightBrace, "}"),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
		Leaf(RightBrace, "}"),
		Leaf(NewLine, "\n\n"),
	)
	e.fs.AddTree(IRTree(TypeKeyword, TagExpr, hashChildren...))
}


// ============================================================
// Constants
// ============================================================

func (e *CppEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	valTokens := e.fs.CollectForest(string(PreVisitGenDeclConstName))
	valCode := ""
	for _, t := range valTokens {
		valCode += t.Serialize()
	}
	if valCode == "" {
		valCode = "0"
	}
	e.fs.AddTree(IRTree(TypeKeyword, TagExpr,
		LeafTag(Keyword, "constexpr", TagCpp),
		Leaf(WhiteSpace, " "),
		LeafTag(Keyword, "auto", TagCpp),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, node.Name),
		Leaf(WhiteSpace, " "),
		Leaf(Assignment, "="),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, valCode),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
	))
}

func (e *CppEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {
	// Let const tokens flow through
}

// ============================================================
// Type Aliases
// ============================================================

func (e *CppEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitTypeAliasName))
	// Tokens should have name and type
	// The PreVisitTypeAliasName marker captures everything
	nameCode := ""
	typeCode := ""
	for _, t := range tokens {
		if t.Kind == TagIdent && nameCode == "" {
			nameCode = t.Serialize()
		} else if t.Serialize() != "" {
			typeCode += t.Serialize()
		}
	}
	if nameCode != "" && typeCode != "" {
		e.fs.AddTree(IRTree(TypeKeyword, TagExpr,
			LeafTag(Keyword, "using", TagCpp),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, nameCode),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, "="),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, typeCode),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n\n"),
		))
	}
}

// ============================================================
// Struct Key Functions
// ============================================================

func (e *CppEmitter) replaceStructKeyFunctions() {
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

func (e *CppEmitter) GenerateMakefile() error {
	if e.LinkRuntime == "" {
		return nil
	}

	makefilePath := filepath.Join(e.OutputDir, "Makefile")
	file, err := os.Create(makefilePath)
	if err != nil {
		return fmt.Errorf("failed to create Makefile: %w", err)
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

TARGET = %s
SRCS = %s.cpp

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

TARGET = %s
SRCS = %s.cpp

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

	// Add math SIMD compilation if runtime/math is used
	if _, hasMath := e.RuntimePackages["math"]; hasMath {
		mathSrc := fmt.Sprintf("%s/math/c/math_simd.c", absRuntimePath)

		// Ensure CC is defined (needed for non-tigr cases)
		if !strings.Contains(makefile, "CC = ") {
			makefile = strings.Replace(makefile, "CXX = g++", "CXX = g++\nCC = gcc", 1)
		}
		if !strings.Contains(makefile, "CFLAGS = ") {
			makefile = strings.Replace(makefile, "CXXFLAGS =", "CFLAGS = -O3\nCXXFLAGS =", 1)
		}

		// Add MATH_SIMD_SRC variable after SRCS line
		mathVar := fmt.Sprintf("MATH_SIMD_SRC = %s", mathSrc)
		makefile = strings.Replace(makefile, "\nall:", "\n"+mathVar+"\n\nall:", 1)

		// Add math_simd.o as a prerequisite of $(TARGET)
		// This substring-matches both "$(TARGET): $(SRCS)" and "$(TARGET): $(SRCS) tigr.o ..."
		makefile = strings.Replace(makefile, "$(TARGET): $(SRCS)", "$(TARGET): $(SRCS) math_simd.o", 1)
		// For tigr case: also add math_simd.o to the explicit link command
		makefile = strings.Replace(makefile, "screen_helper.o $(LDFLAGS)", "screen_helper.o math_simd.o $(LDFLAGS)", 1)

		// Add math_simd.o build rule and clean before .PHONY
		mathRule := `
# Math SIMD runtime
MATH_CFLAGS = -O3 -march=native
math_simd.o: $(MATH_SIMD_SRC)
	$(CC) $(MATH_CFLAGS) -c $< -o $@

`
		makefile = strings.Replace(makefile, ".PHONY:", mathRule+".PHONY:", 1)
		makefile = strings.Replace(makefile, "rm -f $(TARGET)", "rm -f $(TARGET) math_simd.o", 1)

		// Add -lm for math library (exp, sqrt, etc.)
		if !strings.Contains(makefile, "-lm") {
			makefile = strings.Replace(makefile, "LDFLAGS =", "LDFLAGS = -lm", 1)
		}
	}

	_, err = file.WriteString(makefile)
	if err != nil {
		return fmt.Errorf("failed to write Makefile: %w", err)
	}

	DebugLogPrintf("Generated Makefile at %s (graphics: %s)", makefilePath, graphicsBackend)
	return nil
}
