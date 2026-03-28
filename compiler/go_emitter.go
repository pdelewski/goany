package compiler

import (
	"go/ast"
	"go/token"
	"go/types"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// goStdlibPackages is a set of common Go standard library package names.
// Used to distinguish stdlib selectors (keep qualified) from user-package selectors (strip prefix).
var goStdlibPackages = map[string]bool{
	"fmt": true, "os": true, "io": true, "log": true,
	"strings": true, "strconv": true, "math": true, "sort": true,
	"unicode": true, "bytes": true, "errors": true, "sync": true,
	"time": true, "path": true, "filepath": true, "regexp": true,
	"encoding": true, "json": true, "xml": true, "csv": true,
	"net": true, "http": true, "bufio": true, "context": true,
	"reflect": true, "runtime": true, "testing": true,
}

// GoEmitter implements the Emitter interface for Go→Go round-trip emission.
// It preserves Go types and syntax, producing valid (non-idiomatic) Go output
// after MethodReceiverLowering and PointerLowering frontend passes.
type GoEmitter struct {
	fs              *IRForestBuilder
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string
	RuntimePackages map[string]string
	Pkgs            []*packages.Package // All packages for pre-scan
	file            *os.File
	Emitter
	pkg            *packages.Package
	currentPackage string
	skipPackage    bool
	usedImports    map[string]bool
	declaredFuncs   map[string]bool
	declaredConsts  map[string]bool
	declaredStructs map[string]bool
	// nameRenames maps "pkg::name" -> renamed name for collision resolution
	// (covers both functions and constants with same name but different signatures/values)
	nameRenames map[string]string
	indent         int
	numFuncResults int
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
	ifInitNodes []IRNode
	ifCondNodes []IRNode
	ifBodyNodes []IRNode
	ifElseNodes []IRNode
	outputs     []OutputEntry
}

func (e *GoEmitter) SetFile(file *os.File) { e.file = file }
func (e *GoEmitter) GetFile() *os.File     { return e.file }

// goIndent returns indentation string for the given level.
func goIndent(indent int) string {
	return strings.Repeat("\t", indent)
}

// goDefaultForGoType returns Go default value for a Go type.
func goDefaultForGoType(t types.Type) string {
	if t == nil {
		return ""
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
		return "nil"
	case *types.Map:
		return "nil"
	case *types.Struct:
		if named, ok := t.(*types.Named); ok {
			return named.Obj().Name() + "{}"
		}
		return ""
	case *types.Interface:
		return "nil"
	case *types.Pointer:
		return "nil"
	}
	return ""
}

// goDefaultForTypeStr returns Go default value for a Go type name string.
func goDefaultForTypeStr(typeStr string) string {
	switch typeStr {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "byte", "rune":
		return "0"
	case "string":
		return `""`
	case "bool":
		return "false"
	}
	if strings.HasPrefix(typeStr, "[]") || strings.HasPrefix(typeStr, "map[") {
		return "nil"
	}
	return typeStr + "{}"
}

// getExprGoType returns the Go type for an expression, or nil.
func (e *GoEmitter) getExprGoType(expr ast.Expr) types.Type {
	if e.pkg == nil || e.pkg.TypesInfo == nil {
		return nil
	}
	tv := e.pkg.TypesInfo.Types[expr]
	return tv.Type
}

// isMapTypeExpr checks if an expression has map type via TypesInfo.
func (e *GoEmitter) isMapTypeExpr(expr ast.Expr) bool {
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

// ============================================================
// Program / Package
// ============================================================

func (e *GoEmitter) PreVisitProgram(indent int) {
	e.fs = e.GetForestBuilder()
	e.usedImports = make(map[string]bool)
	e.declaredFuncs = make(map[string]bool)
	e.declaredConsts = make(map[string]bool)
	e.declaredStructs = make(map[string]bool)
	e.nameRenames = e.preScanCollisions()
}

// preScanCollisions scans all packages for function and constant name collisions
// with different signatures/values. Returns a rename map: "pkg::name" -> "pkg_name"
// for symbols that need renaming to avoid redeclaration errors after flattening.
func (e *GoEmitter) preScanCollisions() map[string]string {
	renames := make(map[string]string)
	if len(e.Pkgs) == 0 {
		return renames
	}

	type entry struct {
		pkg string
		sig string // signature for funcs, value for constants
	}

	funcMap := make(map[string][]entry)
	constMap := make(map[string][]entry)

	for _, pkg := range e.Pkgs {
		if pkg.Name == "hmap" {
			continue
		}
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					if d.Recv != nil {
						continue // skip methods
					}
					funcMap[d.Name.Name] = append(funcMap[d.Name.Name],
						entry{pkg.Name, goFuncSignature(d)})
				case *ast.GenDecl:
					if d.Tok != token.CONST {
						continue
					}
					for _, spec := range d.Specs {
						vs, ok := spec.(*ast.ValueSpec)
						if !ok {
							continue
						}
						for i, name := range vs.Names {
							val := ""
							if i < len(vs.Values) {
								val = types.ExprString(vs.Values[i])
							}
							constMap[name.Name] = append(constMap[name.Name],
								entry{pkg.Name, val})
						}
					}
				}
			}
		}
	}

	// Detect collisions: same name, different signatures/values
	checkCollisions := func(m map[string][]entry) {
		for name, entries := range m {
			if len(entries) <= 1 {
				continue
			}
			allSame := true
			for i := 1; i < len(entries); i++ {
				if entries[i].sig != entries[0].sig {
					allSame = false
					break
				}
			}
			if allSame {
				continue
			}
			// Different: rename non-main versions
			for _, e := range entries {
				if e.pkg != "main" {
					renames[e.pkg+"::"+name] = e.pkg + "_" + name
				}
			}
		}
	}

	checkCollisions(funcMap)
	checkCollisions(constMap)

	return renames
}

// goFuncSignature returns a string representation of a function's parameter
// and result types for collision detection.
func goFuncSignature(fd *ast.FuncDecl) string {
	var b strings.Builder
	b.WriteString("(")
	if fd.Type.Params != nil {
		for i, field := range fd.Type.Params.List {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(types.ExprString(field.Type))
		}
	}
	b.WriteString(")")
	if fd.Type.Results != nil {
		b.WriteString("(")
		for i, field := range fd.Type.Results.List {
			if i > 0 {
				b.WriteString(",")
			}
			b.WriteString(types.ExprString(field.Type))
		}
		b.WriteString(")")
	}
	return b.String()
}

func (e *GoEmitter) PostVisitProgram(indent int) {
	tokens := e.fs.CollectForest(string(PreVisitProgram))

	// Build preamble with package declaration and used imports
	// Runtime packages are kept as imports (with go.mod replace directives)
	// rather than embedded inline, since this is Go→Go.
	var preamble strings.Builder
	preamble.WriteString("package main\n\n")
	if len(e.usedImports) > 0 {
		preamble.WriteString("import (\n")
		// Emit imports in sorted order for determinism
		var imports []string
		for pkg := range e.usedImports {
			imports = append(imports, pkg)
		}
		sort.Strings(imports)
		for _, pkg := range imports {
			preamble.WriteString("\t\"" + pkg + "\"\n")
		}
		preamble.WriteString(")\n\n")
	}

	// Prepend preamble node
	preambleNode := IRNode{Type: Preamble, Kind: KindDecl, Content: preamble.String()}
	allTokens := make([]IRNode, 0, 1+len(tokens))
	allTokens = append(allTokens, preambleNode)
	allTokens = append(allTokens, tokens...)

	root := IRNode{Type: ScopeNode, Kind: KindDecl, Children: allTokens}
	root.Content = root.Serialize()
	e.outputs = []OutputEntry{{Path: e.Output, Root: root}}
}

func (e *GoEmitter) GetOutputEntries() []OutputEntry { return e.outputs }

func (e *GoEmitter) PostFileEmit() {
	// Generate go.mod with replace directives for runtime packages
	e.generateGoMod()

	// Run go mod tidy to resolve transitive dependencies
	outputDir := e.OutputDir
	if outputDir == "" {
		outputDir = filepath.Dir(e.Output)
	}
	tidyCmd := exec.Command("go", "mod", "tidy")
	tidyCmd.Dir = outputDir
	if err := tidyCmd.Run(); err != nil {
		log.Printf("Warning: go mod tidy failed in %s: %v", outputDir, err)
	}

	// Format with gofmt
	cmd := exec.Command("gofmt", "-w", e.Output)
	if err := cmd.Run(); err != nil {
		log.Printf("Warning: gofmt failed for %s: %v", e.Output, err)
	}
}

// generateGoMod creates a go.mod file in the output directory with replace
// directives for any runtime packages used by the generated code.
func (e *GoEmitter) generateGoMod() {
	outputDir := e.OutputDir
	if outputDir == "" {
		outputDir = filepath.Dir(e.Output)
	}
	modName := e.OutputName
	if modName == "" {
		modName = "generated"
	}

	var b strings.Builder
	b.WriteString("module " + modName + "\n\ngo 1.21\n")

	// Add require + replace for each runtime package import
	if e.LinkRuntime != "" {
		runtimeAbs, err := filepath.Abs(e.LinkRuntime)
		if err != nil {
			runtimeAbs = e.LinkRuntime
		}
		for pkg := range e.usedImports {
			if !strings.HasPrefix(pkg, "runtime/") {
				continue
			}
			pkgName := strings.TrimPrefix(pkg, "runtime/")
			localPath := filepath.Join(runtimeAbs, pkgName)
			b.WriteString("\nrequire " + pkg + " v0.0.0")
			b.WriteString("\nreplace " + pkg + " => " + localPath + "\n")
		}
	}

	goModPath := filepath.Join(outputDir, "go.mod")
	if err := os.WriteFile(goModPath, []byte(b.String()), 0644); err != nil {
		log.Printf("Warning: failed to write go.mod: %v", err)
	}
}

func (e *GoEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	e.pkg = pkg
	e.currentPackage = pkg.Name
	// Skip hmap package — Go uses native maps
	e.skipPackage = pkg.Name == "hmap"
}

func (e *GoEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	if e.skipPackage {
		// Discard all tokens emitted during hmap package processing
		e.fs.CollectForest(string(PreVisitPackage))
		e.skipPackage = false
	}
}

// ============================================================
// Literals and Identifiers
// ============================================================

func (e *GoEmitter) PreVisitBasicLit(node *ast.BasicLit, indent int) {
	if e.skipPackage {
		return
	}
	e.fs.AddLeaf(node.Value, TagLiteral, nil)
}

func (e *GoEmitter) PreVisitIdent(node *ast.Ident, indent int) {
	if e.skipPackage {
		return
	}
	name := node.Name
	switch name {
	case "true", "false", "nil":
		e.fs.AddLeaf(name, TagLiteral, nil)
		return
	}
	// Apply renames for collision resolution (functions and constants)
	if renamed, ok := e.nameRenames[e.currentPackage+"::"+name]; ok {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if obj := e.pkg.TypesInfo.Uses[node]; obj != nil {
				switch obj.(type) {
				case *types.Func, *types.Const:
					goType := e.getExprGoType(node)
					e.fs.AddLeaf(renamed, TagIdent, goType)
					return
				}
			}
		}
	}
	goType := e.getExprGoType(node)
	e.fs.AddLeaf(name, TagIdent, goType)
}

// ============================================================
// Binary Expressions
// ============================================================

func (e *GoEmitter) PostVisitBinaryExprLeft(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBinaryExprLeft))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitBinaryExprRight(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBinaryExprRight))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
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

func (e *GoEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	funCode := e.fs.CollectText(string(PreVisitCallExprFun))
	e.fs.AddLeaf(funCode, KindExpr, nil)
}

func (e *GoEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCallExprArg))
	argNode := collectToNode(tokens)
	e.fs.AddTree(argNode)
}

func (e *GoEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	argTokens := e.fs.CollectForest(string(PreVisitCallExprArgs))
	first := true
	for _, t := range argTokens {
		s := t.Serialize()
		if s == "" {
			continue
		}
		if !first {
			e.fs.AddTree(IRNode{Type: Comma, Content: ", "})
		}
		t.Type = CallExpression
		e.fs.AddTree(t)
		first = false
	}
}

func (e *GoEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCallExpr))
	funName := ""
	if len(tokens) >= 1 {
		funName = tokens[0].Serialize()
	}

	// All Go builtins pass through unchanged
	var callChildren []IRNode
	callChildren = append(callChildren, Leaf(Identifier, funName))
	callChildren = append(callChildren, Leaf(LeftParen, "("))
	if len(tokens) > 1 {
		for _, t := range tokens[1:] {
			callChildren = append(callChildren, t)
		}
	}
	callChildren = append(callChildren, Leaf(RightParen, ")"))
	e.fs.AddTree(IRTree(CallExpression, KindExpr, callChildren...))
}

// ============================================================
// Selector Expressions (a.b)
// ============================================================

func (e *GoEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSelectorExprX))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitSelectorExprSel(node *ast.Ident, indent int) {
	e.fs.CollectForest(string(PreVisitSelectorExprSel))
	e.fs.AddLeaf(node.Name, KindExpr, nil)
}

func (e *GoEmitter) PostVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSelectorExpr))
	xCode := ""
	selCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		selCode = tokens[1].Serialize()
	}

	// Check if X is a package name
	if ident, ok := node.X.(*ast.Ident); ok {
		pkgNameStr := ident.Name
		// Check runtime packages first (takes priority over stdlib — e.g. "http" is
		// both a stdlib name and a goany runtime package)
		if _, isRuntimePkg := e.RuntimePackages[pkgNameStr]; isRuntimePkg {
			e.usedImports["runtime/"+pkgNameStr] = true
			e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, xCode+"."+selCode)))
			return
		}
		if goStdlibPackages[pkgNameStr] {
			// Standard library: keep qualified (e.g. fmt.Println)
			e.usedImports[pkgNameStr] = true
			e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, xCode+"."+selCode)))
			return
		}
		// Check if it's a user-defined package via TypesInfo
		isUserPkg := false
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if obj := e.pkg.TypesInfo.Uses[ident]; obj != nil {
				if _, ok := obj.(*types.PkgName); ok {
					isUserPkg = true
				}
			}
		}
		// Also heuristic: if xCode matches a known imported non-stdlib package name
		if !isUserPkg && e.pkg != nil {
			for _, imp := range e.pkg.Imports {
				if imp.Name == pkgNameStr && strings.Contains(imp.PkgPath, ".") {
					isUserPkg = true
					break
				}
			}
		}
		if isUserPkg {
			// User-defined package: strip prefix (flattened to package main)
			e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, selCode)))
			return
		}
	}

	// Not a package selector (struct field, etc.)
	e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, xCode+"."+selCode)))
}

// ============================================================
// Index Expressions (a[i])
// ============================================================

func (e *GoEmitter) PostVisitIndexExprX(node *ast.IndexExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIndexExprX))
	xNode := collectToNode(tokens)
	xNode.Kind = KindExpr
	e.fs.AddTree(xNode)
}

func (e *GoEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIndexExprIndex))
	idxNode := collectToNode(tokens)
	idxNode.Kind = KindExpr
	e.fs.AddTree(idxNode)
}

func (e *GoEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIndexExpr))
	xCode := ""
	idxCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		idxCode = tokens[1].Serialize()
	}
	// Native Go indexing for all types (slices, maps, arrays)
	e.fs.AddTree(IRTree(IndexExpression, KindExpr,
		Leaf(Identifier, xCode),
		Leaf(LeftBracket, "["),
		Leaf(Identifier, idxCode),
		Leaf(RightBracket, "]"),
	))
}

// ============================================================
// Unary Expressions
// ============================================================

func (e *GoEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitUnaryExpr))
	xNode := collectToNode(tokens)
	op := node.Op.String()
	e.fs.AddTree(IRTree(UnaryExpression, KindExpr,
		Leaf(UnaryOperator, op),
		xNode,
	))
}

// ============================================================
// Paren Expressions
// ============================================================

func (e *GoEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitParenExpr))
	innerNode := collectToNode(tokens)
	e.fs.AddTree(IRTree(ParenExpression, KindExpr,
		Leaf(LeftParen, "("),
		innerNode,
		Leaf(RightParen, ")"),
	))
}

// ============================================================
// Composite Literals
// ============================================================

func (e *GoEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	// Keep type for Go (unlike JS which discards)
	tokens := e.fs.CollectForest(string(PreVisitCompositeLitType))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Serialize()
	}
	e.fs.AddLeaf(typeStr, TagType, nil)
}

func (e *GoEmitter) PostVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCompositeLitElt))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	eltTokens := e.fs.CollectForest(string(PreVisitCompositeLitElts))
	for _, t := range eltTokens {
		s := t.Serialize()
		if s != "" {
			e.fs.AddLeaf(s, TagLiteral, nil)
		}
	}
}

func (e *GoEmitter) PostVisitCompositeLit(node *ast.CompositeLit, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCompositeLit))

	// First token is type (TagType), rest are elements (TagLiteral)
	typeStr := ""
	var elts []string
	for _, t := range tokens {
		if t.Kind == TagType && typeStr == "" {
			typeStr = t.Serialize()
		} else {
			s := t.Serialize()
			if s != "" {
				elts = append(elts, s)
			}
		}
	}
	eltsStr := strings.Join(elts, ", ")

	e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr,
		Leaf(Identifier, typeStr),
		Leaf(LeftBrace, "{"),
		Leaf(Identifier, eltsStr),
		Leaf(RightBrace, "}"),
	))
}

// ============================================================
// KeyValue Expressions
// ============================================================

func (e *GoEmitter) PostVisitKeyValueExprKey(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitKeyValueExprKey))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitKeyValueExprValue(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitKeyValueExprValue))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitKeyValueExpr))
	keyCode := ""
	valCode := ""
	if len(tokens) >= 1 {
		keyCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		valCode = tokens[1].Serialize()
	}
	e.fs.AddTree(IRTree(KeyValueExpression, KindExpr, Leaf(Identifier, keyCode+": "+valCode)))
}

// ============================================================
// Slice Expressions (a[lo:hi])
// ============================================================

func (e *GoEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSliceExprX))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitSliceExprXBegin))
}

func (e *GoEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSliceExprLow))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitSliceExprXEnd))
}

func (e *GoEmitter) PostVisitSliceExprHigh(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSliceExprHigh))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
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

	// Go native slice syntax: x[low:high]
	e.fs.AddTree(IRTree(SliceExpression, KindExpr,
		Leaf(Identifier, xCode),
		Leaf(LeftBracket, "["),
		Leaf(Identifier, lowCode),
		Leaf(Colon, ":"),
		Leaf(Identifier, highCode),
		Leaf(RightBracket, "]"),
	))
}

// ============================================================
// Array Type
// ============================================================

func (e *GoEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitArrayType))
	elemType := ""
	for _, t := range tokens {
		elemType += t.Serialize()
	}
	e.fs.AddLeaf("[]"+elemType, TagType, nil)
}

// ============================================================
// Map Type
// ============================================================

func (e *GoEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitMapKeyType))
	for _, t := range tokens {
		t.Kind = TagType
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitMapValueType(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitMapValueType))
	for _, t := range tokens {
		t.Kind = TagType
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitMapType(node *ast.MapType, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitMapType))
	keyType := ""
	valType := ""
	if len(tokens) >= 1 {
		keyType = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		valType = tokens[1].Serialize()
	}
	e.fs.AddLeaf("map["+keyType+"]"+valType, TagType, nil)
}

// ============================================================
// Channel Type
// ============================================================

func (e *GoEmitter) PostVisitChanType(node *ast.ChanType, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitChanType))
	elemType := ""
	for _, t := range tokens {
		elemType += t.Serialize()
	}
	switch node.Dir {
	case ast.SEND:
		e.fs.AddLeaf("chan<- "+elemType, TagType, nil)
	case ast.RECV:
		e.fs.AddLeaf("<-chan "+elemType, TagType, nil)
	default:
		e.fs.AddLeaf("chan "+elemType, TagType, nil)
	}
}

// ============================================================
// Ellipsis (variadic)
// ============================================================

func (e *GoEmitter) PostVisitEllipsis(node *ast.Ellipsis, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitEllipsis))
	elemType := ""
	for _, t := range tokens {
		elemType += t.Serialize()
	}
	e.fs.AddLeaf("..."+elemType, TagType, nil)
}

// ============================================================
// Star Expressions (*x or *Type)
// ============================================================

func (e *GoEmitter) PostVisitStarExprX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitStarExprX))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitStarExpr(node *ast.StarExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitStarExpr))
	// After PointerLowering, *T is lowered to pool index (int).
	// Discard the inner type and emit "int".
	_ = tokens
	e.fs.AddLeaf("int", TagType, nil)
}

// ============================================================
// Interface Type
// ============================================================

func (e *GoEmitter) PostVisitInterfaceType(node *ast.InterfaceType, indent int) {
	e.fs.CollectForest(string(PreVisitInterfaceType))
	e.fs.AddLeaf("interface{}", TagType, nil)
}

// ============================================================
// Function Literals (closures)
// ============================================================

func (e *GoEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncLitTypeParam))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Serialize()
	}
	// Push each name with its type
	for _, name := range node.Names {
		e.fs.AddLeaf(name.Name+" "+typeStr, TagIdent, nil)
	}
}

func (e *GoEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncLitTypeParams))
	var params []string
	for _, t := range tokens {
		if t.Kind == TagIdent && t.Serialize() != "" {
			params = append(params, t.Serialize())
		}
	}
	paramsStr := strings.Join(params, ", ")
	if paramsStr == "" {
		paramsStr = " "
	}
	e.fs.AddLeaf(paramsStr, KindExpr, nil)
}

func (e *GoEmitter) PostVisitFuncLitTypeResult(node *ast.Field, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncLitTypeResult))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Serialize()
	}
	e.fs.AddLeaf(typeStr, TagType, nil)
}

func (e *GoEmitter) PostVisitFuncLitTypeResults(node *ast.FieldList, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncLitTypeResults))
	var types []string
	for _, t := range tokens {
		if t.Kind == TagType {
			s := t.Serialize()
			if s != "" {
				types = append(types, s)
			}
		}
	}
	if len(types) == 0 {
		return
	}
	if len(types) == 1 {
		e.fs.AddLeaf(types[0], TagType, nil)
	} else {
		e.fs.AddLeaf("("+strings.Join(types, ", ")+")", TagType, nil)
	}
}

func (e *GoEmitter) PostVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncLitBody))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncLit))
	paramsCode := ""
	retType := ""
	bodyCode := ""

	idx := 0
	// Params
	if idx < len(tokens) {
		paramsCode = strings.TrimSpace(tokens[idx].Serialize())
		idx++
	}
	// Check for return type (TagType)
	if idx < len(tokens) && tokens[idx].Kind == TagType {
		retType = tokens[idx].Serialize()
		idx++
	}
	// Body
	if idx < len(tokens) {
		bodyCode = tokens[idx].Serialize()
	}

	var children []IRNode
	children = append(children, Leaf(FunctionKeyword, "func"))
	children = append(children, Leaf(LeftParen, "("))
	children = append(children, Leaf(Identifier, paramsCode))
	children = append(children, Leaf(RightParen, ")"))
	if retType != "" {
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(Identifier, retType))
	}
	children = append(children, Leaf(WhiteSpace, " "))
	children = append(children, Leaf(Identifier, bodyCode))
	e.fs.AddTree(IRTree(FuncLitExpression, KindExpr, children...))
}

// ============================================================
// Type Assertions
// ============================================================

func (e *GoEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitTypeAssertExprX))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitTypeAssertExprType))
	for _, t := range tokens {
		t.Kind = TagType
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitTypeAssertExpr))
	// Traversal order: Type first, then X
	typeCode := ""
	xCode := ""
	if len(tokens) >= 1 {
		typeCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		xCode = tokens[1].Serialize()
	}
	// Go native type assertion: x.(Type)
	e.fs.AddTree(IRTree(TypeAssertExpression, KindExpr,
		Leaf(Identifier, xCode),
		Leaf(Dot, "."),
		Leaf(LeftParen, "("),
		Leaf(Identifier, typeCode),
		Leaf(RightParen, ")"),
	))
}

// ============================================================
// Function Type (for func parameters/types)
// ============================================================

func (e *GoEmitter) PostVisitFuncTypeParam(node *ast.Field, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncTypeParam))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Serialize()
	}
	for _, name := range node.Names {
		e.fs.AddLeaf(name.Name+" "+typeStr, TagIdent, nil)
	}
	if len(node.Names) == 0 {
		e.fs.AddLeaf(typeStr, TagType, nil)
	}
}

func (e *GoEmitter) PostVisitFuncTypeParams(node *ast.FieldList, indent int) {
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

func (e *GoEmitter) PostVisitFuncTypeResult(node *ast.Field, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncTypeResult))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Serialize()
	}
	e.fs.AddLeaf(typeStr, TagType, nil)
}

func (e *GoEmitter) PostVisitFuncTypeResults(node *ast.FieldList, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncTypeResults))
	var types []string
	for _, t := range tokens {
		if t.Kind == TagType {
			s := t.Serialize()
			if s != "" {
				types = append(types, s)
			}
		}
	}
	if len(types) == 0 {
		return
	}
	if len(types) == 1 {
		e.fs.AddLeaf(types[0], TagType, nil)
	} else {
		e.fs.AddLeaf("("+strings.Join(types, ", ")+")", TagType, nil)
	}
}

func (e *GoEmitter) PostVisitFuncType(node *ast.FuncType, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncType))
	paramsCode := ""
	retCode := ""
	for _, t := range tokens {
		if t.Kind == KindExpr {
			paramsCode = t.Serialize()
		} else if t.Kind == TagType {
			retCode = t.Serialize()
		}
	}
	result := "func(" + paramsCode + ")"
	if retCode != "" {
		result += " " + retCode
	}
	e.fs.AddLeaf(result, TagType, nil)
}

// ============================================================
// Function Declarations
// ============================================================

func (e *GoEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	e.numFuncResults = 0
	if node.Type.Results != nil {
		e.numFuncResults = node.Type.Results.NumFields()
	}
}

func (e *GoEmitter) PostVisitFuncDeclName(node *ast.Ident, indent int) {
	e.fs.CollectForest(string(PreVisitFuncDeclName))
	name := node.Name
	// Apply rename for collision resolution
	if renamed, ok := e.nameRenames[e.currentPackage+"::"+name]; ok {
		name = renamed
	}
	e.fs.AddLeaf(name, TagIdent, nil)
}

func (e *GoEmitter) PostVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {
	// Keep type for Go
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeParamsListType))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Serialize()
	}
	e.fs.AddLeaf(typeStr, TagType, nil)
}

func (e *GoEmitter) PostVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeParamsArgName))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *GoEmitter) PostVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeParamsList))
	typeStr := ""
	var names []string
	for _, t := range tokens {
		if t.Kind == TagIdent {
			names = append(names, t.Serialize())
		} else if t.Kind == TagType {
			typeStr = t.Serialize()
		}
	}
	// Emit each name with type
	for _, name := range names {
		e.fs.AddLeaf(name+" "+typeStr, TagIdent, nil)
	}
}

func (e *GoEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeParams))
	var params []string
	for _, t := range tokens {
		if t.Kind == TagIdent {
			params = append(params, t.Serialize())
		}
	}
	e.fs.AddLeaf(strings.Join(params, ", "), KindExpr, nil)
}

func (e *GoEmitter) PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeResultsList))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Serialize()
	}
	e.fs.AddLeaf(typeStr, TagType, nil)
}

func (e *GoEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeResults))
	var types []string
	for _, t := range tokens {
		if t.Kind == TagType {
			s := t.Serialize()
			if s != "" {
				types = append(types, s)
			}
		}
	}
	if len(types) == 0 {
		return
	}
	if len(types) == 1 {
		e.fs.AddLeaf(types[0], TagType, nil)
	} else {
		e.fs.AddLeaf("("+strings.Join(types, ", ")+")", TagType, nil)
	}
}

func (e *GoEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignature))
	funcName := ""
	paramsStr := ""
	retStr := ""
	for _, t := range tokens {
		if t.Kind == TagIdent && funcName == "" {
			funcName = t.Serialize()
		} else if t.Kind == TagExpr {
			paramsStr = t.Serialize()
		} else if t.Kind == TagType {
			retStr = t.Serialize()
		}
	}

	var children []IRNode
	children = append(children, Leaf(NewLine, "\n"))
	children = append(children, Leaf(FunctionKeyword, "func"))
	children = append(children, Leaf(WhiteSpace, " "))
	children = append(children, Leaf(Identifier, funcName))
	children = append(children, Leaf(LeftParen, "("))
	children = append(children, Leaf(Identifier, paramsStr))
	children = append(children, Leaf(RightParen, ")"))
	if retStr != "" {
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(Identifier, retStr))
	}
	e.fs.AddTree(IRTree(FuncDeclaration, KindDecl, children...))
}

func (e *GoEmitter) PostVisitFuncDeclBody(node *ast.BlockStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclBody))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDecl))

	// Use renamed name if collision was detected
	funcName := node.Name.Name
	if renamed, ok := e.nameRenames[e.currentPackage+"::"+funcName]; ok {
		funcName = renamed
	}

	// Deduplicate: skip if a function with this name was already emitted
	if e.declaredFuncs[funcName] {
		return
	}
	e.declaredFuncs[funcName] = true

	var children []IRNode
	if len(tokens) >= 1 {
		children = append(children, tokens[0]) // signature
	}
	children = append(children, Leaf(WhiteSpace, " "))
	if len(tokens) >= 2 {
		children = append(children, tokens[1]) // body
	}
	children = append(children, Leaf(NewLine, "\n"))
	e.fs.AddTree(IRTree(FuncDeclaration, KindDecl, children...))
}

// ============================================================
// Forward Declaration Signatures (suppressed)
// ============================================================

func (e *GoEmitter) PostVisitFuncDeclSignatures(indent int) {
	e.fs.CollectForest(string(PreVisitFuncDeclSignatures))
}

// ============================================================
// Block Statements
// ============================================================

func (e *GoEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	e.indent = indent
}

func (e *GoEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBlockStmtList))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBlockStmt))
	var children []IRNode
	children = append(children, Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			children = append(children, t)
		}
	}
	children = append(children, Leaf(WhiteSpace, goIndent(indent/2)), Leaf(RightBrace, "}"))
	e.fs.AddTree(IRTree(BlockStatement, KindStmt, children...))
}

// ============================================================
// Assignment Statements
// ============================================================

func (e *GoEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	e.indent = indent
}

func (e *GoEmitter) PostVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	lhsCode := e.fs.CollectText(string(PreVisitAssignStmtLhsExpr))
	e.fs.AddLeaf(lhsCode, KindExpr, nil)
}

func (e *GoEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitAssignStmtLhs))
	var lhsExprs []string
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			lhsExprs = append(lhsExprs, s)
		}
	}
	e.fs.AddLeaf(strings.Join(lhsExprs, ", "), KindExpr, nil)
}

func (e *GoEmitter) PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitAssignStmtRhsExpr))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
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

func (e *GoEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitAssignStmt))
	lhsStr := ""
	rhsStr := ""
	if len(tokens) >= 1 {
		lhsStr = tokens[0].Serialize()
	}
	rhsNode := Leaf(Identifier, "")
	if len(tokens) >= 2 {
		rhsStr = tokens[1].Serialize()
		rhsNode = tokens[1]
	}

	ind := goIndent(indent / 2)

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

	// Comma-ok type assertion: val, ok := x.(Type) — Go native
	// Comma-ok map read: val, ok := m[key] — Go native
	// Multi-value: a, b := func() — Go native
	// All pass through as-is

	_ = rhsStr
	e.fs.AddTree(IRTree(AssignStatement, KindStmt,
		Leaf(WhiteSpace, ind),
		Leaf(Identifier, lhsStr),
		Leaf(WhiteSpace, " "),
		Leaf(Assignment, tokStr),
		Leaf(WhiteSpace, " "),
		rhsNode,
		Leaf(NewLine, "\n"),
	))
}

// ============================================================
// Declaration Statements (var x int, var y = 5)
// ============================================================

func (e *GoEmitter) PreVisitDeclStmt(node *ast.DeclStmt, indent int) {
	e.indent = indent
}

func (e *GoEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
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

func (e *GoEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	e.fs.CollectForest(string(PreVisitDeclStmtValueSpecNames))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *GoEmitter) PostVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {
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

func (e *GoEmitter) PostVisitDeclStmt(node *ast.DeclStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitDeclStmt))
	ind := goIndent(indent / 2)

	var children []IRNode
	i := 0
	for i < len(tokens) {
		typeStr := ""
		nameStr := ""
		valueStr := ""
		var valueToken IRNode

		if i < len(tokens) && tokens[i].Kind == TagType {
			typeStr = tokens[i].Serialize()
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
			valueNode := Leaf(Identifier, valueStr)
			if valueStr == valueToken.Serialize() {
				valueNode = valueToken
			}
			children = append(children,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, "var"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, nameStr),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, typeStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				valueNode,
				Leaf(NewLine, "\n"),
			)
		} else {
			// No initializer — Go auto-initializes to zero value
			children = append(children,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, "var"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, nameStr),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, typeStr),
				Leaf(NewLine, "\n"),
			)
		}
	}
	e.fs.AddTree(IRTree(DeclStatement, KindStmt, children...))
}

// ============================================================
// Return Statements
// ============================================================

func (e *GoEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	e.indent = indent
}

func (e *GoEmitter) PostVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitReturnStmtResult))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitReturnStmt))
	ind := goIndent(indent / 2)

	if len(tokens) == 0 {
		e.fs.AddTree(IRTree(ReturnStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return"),
			Leaf(NewLine, "\n"),
		))
	} else if len(tokens) == 1 {
		e.fs.AddTree(IRTree(ReturnStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return"),
			Leaf(WhiteSpace, " "),
			tokens[0],
			Leaf(NewLine, "\n"),
		))
	} else {
		// Multi-value return: return a, b (native Go)
		var children []IRNode
		children = append(children, Leaf(WhiteSpace, ind), Leaf(ReturnKeyword, "return"), Leaf(WhiteSpace, " "))
		for i, t := range tokens {
			if i > 0 {
				children = append(children, Leaf(Comma, ","), Leaf(WhiteSpace, " "))
			}
			children = append(children, t)
		}
		children = append(children, Leaf(NewLine, "\n"))
		e.fs.AddTree(IRTree(ReturnStatement, KindStmt, children...))
	}
}

// ============================================================
// Expression Statements
// ============================================================

func (e *GoEmitter) PreVisitExprStmt(node *ast.ExprStmt, indent int) {
	e.indent = indent
}

func (e *GoEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitExprStmtX))
	for _, t := range tokens {
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitExprStmt(node *ast.ExprStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitExprStmt))
	ind := goIndent(indent / 2)
	var children []IRNode
	children = append(children, Leaf(WhiteSpace, ind))
	if len(tokens) >= 1 {
		children = append(children, tokens[0])
	}
	children = append(children, Leaf(NewLine, "\n"))
	e.fs.AddTree(IRTree(ExprStatement, KindStmt, children...))
}

// ============================================================
// If Statements
// ============================================================

func (e *GoEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
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

func (e *GoEmitter) PostVisitIfStmtInit(node ast.Stmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIfStmtInit))
	n := len(e.ifInitNodes)
	e.ifInitNodes[n-1] = collectToNode(tokens)
	e.ifInitStack[n-1] = e.ifInitNodes[n-1].Serialize()
}

func (e *GoEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIfStmtCond))
	n := len(e.ifCondNodes)
	e.ifCondNodes[n-1] = collectToNode(tokens)
	e.ifCondStack[n-1] = e.ifCondNodes[n-1].Serialize()
}

func (e *GoEmitter) PostVisitIfStmtBody(node *ast.IfStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIfStmtBody))
	n := len(e.ifBodyNodes)
	e.ifBodyNodes[n-1] = collectToNode(tokens)
	e.ifBodyStack[n-1] = e.ifBodyNodes[n-1].Serialize()
}

func (e *GoEmitter) PostVisitIfStmtElse(node *ast.IfStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIfStmtElse))
	n := len(e.ifElseNodes)
	e.ifElseNodes[n-1] = collectToNode(tokens)
	e.ifElseStack[n-1] = e.ifElseNodes[n-1].Serialize()
}

func (e *GoEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	e.fs.CollectForest(string(PreVisitIfStmt))
	ind := goIndent(indent / 2)

	n := len(e.ifInitStack)
	initCode := e.ifInitStack[n-1]
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

	// Strip trailing whitespace/newlines from init for clean formatting
	initStr := strings.TrimRight(initCode, " \t\n;")

	var children []IRNode
	children = append(children, Leaf(WhiteSpace, ind), Leaf(IfKeyword, "if"), Leaf(WhiteSpace, " "))
	if initStr != "" {
		// Trim leading indent from init
		initStr = strings.TrimLeft(initStr, " \t")
		children = append(children, Leaf(Identifier, initStr), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "))
		_ = initNode
	}
	children = append(children, condNode, Leaf(WhiteSpace, " "), bodyNode)

	if elseCode != "" {
		trimmed := strings.TrimLeft(elseCode, " \t\n")
		if strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "if\t") {
			elseIfNode := stripLeadingWhitespace(elseNode)
			children = append(children, Leaf(WhiteSpace, " "), Leaf(ElseKeyword, "else"), Leaf(WhiteSpace, " "), elseIfNode)
		} else {
			children = append(children, Leaf(WhiteSpace, " "), Leaf(ElseKeyword, "else"), Leaf(WhiteSpace, " "), elseNode)
		}
	}
	children = append(children, Leaf(NewLine, "\n"))
	e.fs.AddTree(IRTree(IfStatement, KindStmt, children...))
}

// ============================================================
// For Statements
// ============================================================

func (e *GoEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	e.indent = indent
	e.forInitStack = append(e.forInitStack, "")
	e.forCondStack = append(e.forCondStack, "")
	e.forPostStack = append(e.forPostStack, "")
	e.forCondNodes = append(e.forCondNodes, IRNode{})
	e.forBodyNodes = append(e.forBodyNodes, IRNode{})
}

func (e *GoEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitForStmtInit))
	initCode := collectToNode(tokens).Serialize()
	initCode = strings.TrimRight(initCode, ";\n \t")
	initCode = strings.TrimLeft(initCode, " \t")
	e.forInitStack[len(e.forInitStack)-1] = initCode
}

func (e *GoEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitForStmtCond))
	e.forCondNodes[len(e.forCondNodes)-1] = collectToNode(tokens)
	e.forCondStack[len(e.forCondStack)-1] = e.forCondNodes[len(e.forCondNodes)-1].Serialize()
}

func (e *GoEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitForStmtPost))
	postCode := collectToNode(tokens).Serialize()
	postCode = strings.TrimRight(postCode, ";\n \t")
	postCode = strings.TrimLeft(postCode, " \t")
	e.forPostStack[len(e.forPostStack)-1] = postCode
}

func (e *GoEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {
	bodyTokens := e.fs.CollectForest(string(PreVisitForStmt))
	bodyNode := collectToNode(bodyTokens)
	ind := goIndent(indent / 2)

	n := len(e.forInitStack)
	initCode := e.forInitStack[n-1]
	condNode := e.forCondNodes[n-1]
	postCode := e.forPostStack[n-1]
	e.forInitStack = e.forInitStack[:n-1]
	e.forCondStack = e.forCondStack[:n-1]
	e.forPostStack = e.forPostStack[:n-1]
	e.forCondNodes = e.forCondNodes[:n-1]
	e.forBodyNodes = e.forBodyNodes[:n-1]

	// Infinite loop
	if node.Init == nil && node.Cond == nil && node.Post == nil {
		e.fs.AddTree(IRTree(ForStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for"),
			Leaf(WhiteSpace, " "),
			bodyNode,
			Leaf(NewLine, "\n"),
		))
		return
	}

	// Condition-only loop
	if node.Init == nil && node.Post == nil && node.Cond != nil {
		e.fs.AddTree(IRTree(ForStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for"),
			Leaf(WhiteSpace, " "),
			condNode,
			Leaf(WhiteSpace, " "),
			bodyNode,
			Leaf(NewLine, "\n"),
		))
		return
	}

	// Full for loop: for init; cond; post { body }
	e.fs.AddTree(IRTree(ForStatement, KindStmt,
		Leaf(WhiteSpace, ind),
		Leaf(ForKeyword, "for"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, initCode),
		Leaf(Semicolon, ";"),
		Leaf(WhiteSpace, " "),
		condNode,
		Leaf(Semicolon, ";"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, postCode),
		Leaf(WhiteSpace, " "),
		bodyNode,
		Leaf(NewLine, "\n"),
	))
}

// ============================================================
// Range Statements
// ============================================================

func (e *GoEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	e.indent = indent
}

func (e *GoEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitRangeStmtKey))
	for _, t := range tokens {
		t.Kind = TagIdent
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitRangeStmtValue))
	for _, t := range tokens {
		t.Kind = TagIdent
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitRangeStmtX))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitRangeStmt))
	ind := goIndent(indent / 2)

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

	// When Key was nil (blank identifier from SemaChecker), use _
	if node.Key == nil {
		keyCode = "_"
	}

	tokStr := node.Tok.String()

	var children []IRNode
	children = append(children, Leaf(WhiteSpace, ind), Leaf(ForKeyword, "for"), Leaf(WhiteSpace, " "))

	if valCode != "" {
		children = append(children, Leaf(Identifier, keyCode), Leaf(Comma, ","), Leaf(WhiteSpace, " "), Leaf(Identifier, valCode))
	} else if keyCode != "" {
		children = append(children, Leaf(Identifier, keyCode))
	}

	children = append(children, Leaf(WhiteSpace, " "), Leaf(Assignment, tokStr), Leaf(WhiteSpace, " "), Leaf(Identifier, "range"), Leaf(WhiteSpace, " "), Leaf(Identifier, xCode), Leaf(WhiteSpace, " "), bodyNode, Leaf(NewLine, "\n"))
	e.fs.AddTree(IRTree(RangeStatement, KindStmt, children...))
}

// ============================================================
// Switch / Case Statements
// ============================================================

func (e *GoEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	e.indent = indent
}

func (e *GoEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSwitchStmtTag))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSwitchStmt))
	ind := goIndent(indent / 2)

	tagCode := ""
	idx := 0
	if idx < len(tokens) {
		tagCode = tokens[idx].Serialize()
		idx++
	}

	var children []IRNode
	children = append(children, Leaf(WhiteSpace, ind), Leaf(SwitchKeyword, "switch"), Leaf(WhiteSpace, " "), Leaf(Identifier, tagCode), Leaf(WhiteSpace, " "), Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
	for i := idx; i < len(tokens); i++ {
		children = append(children, tokens[i])
	}
	children = append(children, Leaf(WhiteSpace, ind), Leaf(RightBrace, "}"), Leaf(NewLine, "\n"))
	e.fs.AddTree(IRTree(SwitchStatement, KindStmt, children...))
}

func (e *GoEmitter) PreVisitCaseClause(node *ast.CaseClause, indent int) {
	e.indent = indent
}

func (e *GoEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCaseClauseListExpr))
	for _, t := range tokens {
		t.Kind = KindExpr
		e.fs.AddTree(t)
	}
}

func (e *GoEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCaseClauseList))
	var exprs []string
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			exprs = append(exprs, s)
		}
	}
	e.fs.AddLeaf(strings.Join(exprs, ", "), KindExpr, nil)
}

func (e *GoEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCaseClause))
	ind := goIndent(indent / 2)

	var children []IRNode
	idx := 0
	if len(node.List) == 0 {
		children = append(children, Leaf(WhiteSpace, ind), Leaf(DefaultKeyword, "default"), Leaf(Colon, ":"), Leaf(NewLine, "\n"))
	} else {
		caseExprs := ""
		if idx < len(tokens) {
			caseExprs = tokens[idx].Serialize()
			idx++
		}
		children = append(children, Leaf(WhiteSpace, ind), Leaf(CaseKeyword, "case"), Leaf(WhiteSpace, " "), Leaf(Identifier, caseExprs), Leaf(Colon, ":"), Leaf(NewLine, "\n"))
	}
	// Case body statements
	for i := idx; i < len(tokens); i++ {
		children = append(children, tokens[i])
	}
	// No explicit break in Go (unlike JS)
	e.fs.AddTree(IRTree(CaseClauseStatement, KindStmt, children...))
}

// ============================================================
// Inc/Dec Statements
// ============================================================

func (e *GoEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIncDecStmt))
	xNode := collectToNode(tokens)
	ind := goIndent(indent / 2)
	e.fs.AddTree(IRTree(IncDecStatement, KindStmt,
		Leaf(WhiteSpace, ind),
		xNode,
		Leaf(UnaryOperator, node.Tok.String()),
		Leaf(NewLine, "\n"),
	))
}

// ============================================================
// Branch Statements (break, continue)
// ============================================================

func (e *GoEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	ind := goIndent(indent / 2)
	switch node.Tok {
	case token.BREAK:
		e.fs.AddTree(IRTree(BranchStatement, KindStmt, Leaf(Identifier, ind+"break\n")))
	case token.CONTINUE:
		e.fs.AddTree(IRTree(BranchStatement, KindStmt, Leaf(Identifier, ind+"continue\n")))
	}
}

// ============================================================
// Defer Statements
// ============================================================

func (e *GoEmitter) PostVisitDeferStmt(node *ast.DeferStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitDeferStmt))
	ind := goIndent(indent / 2)
	callNode := collectToNode(tokens)
	e.fs.AddTree(IRTree(DeferStatement, KindStmt,
		Leaf(WhiteSpace, ind),
		Leaf(Identifier, "defer"),
		Leaf(WhiteSpace, " "),
		callNode,
		Leaf(NewLine, "\n"),
	))
}

// ============================================================
// Go Statements
// ============================================================

func (e *GoEmitter) PostVisitGoStmt(node *ast.GoStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitGoStmt))
	ind := goIndent(indent / 2)
	callNode := collectToNode(tokens)
	e.fs.AddTree(IRTree(GoStatement, KindStmt,
		Leaf(WhiteSpace, ind),
		Leaf(Identifier, "go"),
		Leaf(WhiteSpace, " "),
		callNode,
		Leaf(NewLine, "\n"),
	))
}

// ============================================================
// Struct Declarations (GenStructInfo)
// ============================================================

func (e *GoEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitGenStructFieldType))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Serialize()
	}
	e.fs.AddLeaf(typeStr, TagType, nil)
}

func (e *GoEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	e.fs.CollectForest(string(PreVisitGenStructFieldName))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *GoEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitGenStructInfo))

	if node.Struct == nil {
		return
	}

	// Collect field pairs (type, name) — traversal order is Type then Name
	type fieldPair struct {
		name string
		typ  string
	}
	var fields []fieldPair
	i := 0
	for i < len(tokens) {
		typ := ""
		name := ""
		if i < len(tokens) && tokens[i].Kind == TagType {
			typ = tokens[i].Serialize()
			i++
		}
		if i < len(tokens) && tokens[i].Kind == TagIdent {
			name = tokens[i].Serialize()
			i++
		}
		if name != "" {
			fields = append(fields, fieldPair{name, typ})
		}
	}

	var children []IRNode
	children = append(children, Leaf(NewLine, "\n"))
	children = append(children, Leaf(TypeKeyword, "type"))
	children = append(children, Leaf(WhiteSpace, " "))
	children = append(children, Leaf(Identifier, node.Name))
	children = append(children, Leaf(WhiteSpace, " "))
	children = append(children, Leaf(StructKeyword, "struct"))
	children = append(children, Leaf(WhiteSpace, " "))
	children = append(children, Leaf(LeftBrace, "{"))
	children = append(children, Leaf(NewLine, "\n"))
	for _, f := range fields {
		children = append(children, Leaf(WhiteSpace, "\t"))
		children = append(children, Leaf(Identifier, f.name))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(Identifier, f.typ))
		children = append(children, Leaf(NewLine, "\n"))
	}
	children = append(children, Leaf(RightBrace, "}"))
	children = append(children, Leaf(NewLine, "\n"))
	e.fs.AddTree(IRTree(StructTypeNode, KindType, children...))
}

// ============================================================
// Constants
// ============================================================

func (e *GoEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitGenDeclConstName))
	// tokens contain the value expression (traversed between Pre and Post)
	valCode := ""
	for _, t := range tokens {
		valCode += t.Serialize()
	}
	// Apply rename for collision resolution
	name := node.Name
	if renamed, ok := e.nameRenames[e.currentPackage+"::"+name]; ok {
		name = renamed
	}
	e.fs.AddLeaf(name, TagIdent, nil)
	if valCode != "" {
		e.fs.AddLeaf(valCode, TagExpr, nil)
	}
}

func (e *GoEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitGenDeclConst))

	var children []IRNode
	i := 0
	for i < len(tokens) {
		nameStr := ""
		valStr := ""
		if i < len(tokens) && tokens[i].Kind == TagIdent {
			nameStr = tokens[i].Serialize()
			i++
		}
		if i < len(tokens) && tokens[i].Kind == TagExpr {
			valStr = tokens[i].Serialize()
			i++
		}
		if nameStr != "" && e.declaredConsts[nameStr] {
			// Skip duplicate constant
			continue
		}
		if nameStr != "" {
			e.declaredConsts[nameStr] = true
		}
		if nameStr != "" && valStr != "" {
			children = append(children,
				Leaf(NewLine, "\n"),
				Leaf(Identifier, "const"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, nameStr),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, valStr),
				Leaf(NewLine, "\n"),
			)
		} else if nameStr != "" {
			// Constant without explicit value (iota continuation)
			children = append(children,
				Leaf(NewLine, "\n"),
				Leaf(Identifier, "const"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, nameStr),
				Leaf(NewLine, "\n"),
			)
		}
	}
	if len(children) > 0 {
		e.fs.AddTree(IRTree(DeclStatement, KindDecl, children...))
	}
}

// ============================================================
// Type Aliases
// ============================================================

func (e *GoEmitter) PostVisitTypeAliasName(node *ast.Ident, indent int) {
	e.fs.CollectForest(string(PreVisitTypeAliasName))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *GoEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitTypeAliasType))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Serialize()
	}
	e.fs.AddLeaf(typeStr, TagType, nil)
}

func (e *GoEmitter) PostVisitGenStructInfos(node []GenTypeInfo, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitGenStructInfos))

	// Process tokens: StructTypeNode trees pass through,
	// TagIdent+TagType pairs from type aliases become `type Name Type`
	i := 0
	for i < len(tokens) {
		if tokens[i].Type == StructTypeNode {
			// Deduplicate: extract struct name from Children[3] (NewLine, type, space, name)
			structName := ""
			if len(tokens[i].Children) > 3 {
				structName = tokens[i].Children[3].Content
			}
			if structName != "" && e.declaredStructs[structName] {
				i++
				continue
			}
			if structName != "" {
				e.declaredStructs[structName] = true
			}
			e.fs.AddTree(tokens[i])
			i++
		} else if tokens[i].Kind == TagIdent {
			nameStr := tokens[i].Serialize()
			i++
			typeStr := ""
			if i < len(tokens) && tokens[i].Kind == TagType {
				typeStr = tokens[i].Serialize()
				i++
			}
			if nameStr != "" && typeStr != "" {
				e.fs.AddTree(IRTree(DeclStatement, KindDecl,
					Leaf(NewLine, "\n"),
					Leaf(TypeKeyword, "type"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, nameStr),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, typeStr),
					Leaf(NewLine, "\n"),
				))
			}
		} else {
			e.fs.AddTree(tokens[i])
			i++
		}
	}
}
