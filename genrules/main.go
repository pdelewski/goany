// genrules generates precise construct patterns from tests and examples.
//
// Usage:
//
//	go run ./genrules -collect -output patterns.json
//	go run ./genrules -check file.go -patterns patterns.json
//
// The tool walks all .go files in tests/ and examples/, parses them with
// full type information, and collects precise signatures for every construct
// used. These signatures can then be used to validate that user code only
// uses constructs that have been tested.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"goany/compiler"
	"golang.org/x/tools/go/packages"
)

// Pattern represents a precise construct signature
type Pattern struct {
	Kind    string            `json:"kind"`    // AST node type (e.g., "ForStmt", "RangeStmt")
	Attrs   map[string]string `json:"attrs"`   // Attributes that define this variant
	Example string            `json:"example"` // Example location where this was seen
}

// PatternKey returns a unique string key for this pattern
func (p Pattern) Key() string {
	var parts []string
	parts = append(parts, p.Kind)

	// Sort attribute keys for consistent ordering
	var keys []string
	for k := range p.Attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, p.Attrs[k]))
	}
	return strings.Join(parts, ":")
}

// NormalizedKey returns a key with only type-significant attributes.
// This allows matching constructs regardless of specific types used,
// except where types actually matter for correctness.
func (p Pattern) NormalizedKey() string {
	significant, hasConfig := compiler.TypeSignificantAttrs[p.Kind]

	var parts []string
	parts = append(parts, p.Kind)

	// Sort attribute keys for consistent ordering
	var keys []string
	for k := range p.Attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		// If no config for this kind, include all attrs (conservative)
		// If config exists, only include significant attrs
		if !hasConfig || significant[k] {
			parts = append(parts, fmt.Sprintf("%s=%s", k, p.Attrs[k]))
		}
	}
	return strings.Join(parts, ":")
}

// PatternCollector collects precise patterns from Go source code
type PatternCollector struct {
	patterns  map[string]Pattern // key -> pattern
	fset      *token.FileSet
	typesInfo *types.Info // Optional type info when available
}

func NewPatternCollector() *PatternCollector {
	return &PatternCollector{
		patterns: make(map[string]Pattern),
	}
}

func (pc *PatternCollector) addPattern(kind string, attrs map[string]string, pos token.Pos) {
	p := Pattern{
		Kind:  kind,
		Attrs: attrs,
	}
	if pc.fset != nil && pos.IsValid() {
		position := pc.fset.Position(pos)
		p.Example = fmt.Sprintf("%s:%d", filepath.Base(position.Filename), position.Line)
	}

	key := p.Key()
	if _, exists := pc.patterns[key]; !exists {
		pc.patterns[key] = p
	}
}

func (pc *PatternCollector) typeCategory(t types.Type) string {
	if t == nil {
		return "unknown"
	}

	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Name()
	case *types.Slice:
		return "slice"
	case *types.Array:
		return "array"
	case *types.Map:
		return "map"
	case *types.Struct:
		return "struct"
	case *types.Pointer:
		return "pointer"
	case *types.Interface:
		return "interface"
	case *types.Signature:
		return "func"
	case *types.Chan:
		return "chan"
	default:
		return "other"
	}
}

func (pc *PatternCollector) Visit(node ast.Node) ast.Visitor {
	if node == nil {
		return pc
	}

	switch n := node.(type) {
	case *ast.ForStmt:
		pc.collectForStmt(n)

	case *ast.RangeStmt:
		pc.collectRangeStmt(n)

	case *ast.IfStmt:
		pc.collectIfStmt(n)

	case *ast.SwitchStmt:
		pc.collectSwitchStmt(n)

	case *ast.TypeSwitchStmt:
		pc.collectTypeSwitchStmt(n)

	case *ast.AssignStmt:
		pc.collectAssignStmt(n)

	case *ast.CallExpr:
		pc.collectCallExpr(n)

	case *ast.CompositeLit:
		pc.collectCompositeLit(n)

	case *ast.MapType:
		pc.collectMapType(n)

	case *ast.IndexExpr:
		pc.collectIndexExpr(n)

	case *ast.SliceExpr:
		pc.collectSliceExpr(n)

	case *ast.UnaryExpr:
		pc.collectUnaryExpr(n)

	case *ast.BinaryExpr:
		pc.collectBinaryExpr(n)

	case *ast.FuncDecl:
		pc.collectFuncDecl(n)

	case *ast.FuncLit:
		pc.collectFuncLit(n)

	case *ast.ReturnStmt:
		pc.collectReturnStmt(n)

	case *ast.BranchStmt:
		pc.collectBranchStmt(n)

	case *ast.IncDecStmt:
		pc.collectIncDecStmt(n)

	case *ast.DeferStmt:
		pc.addPattern("DeferStmt", map[string]string{}, n.Pos())

	case *ast.GoStmt:
		pc.addPattern("GoStmt", map[string]string{}, n.Pos())

	case *ast.SendStmt:
		pc.addPattern("SendStmt", map[string]string{}, n.Pos())

	case *ast.SelectStmt:
		pc.addPattern("SelectStmt", map[string]string{}, n.Pos())

	case *ast.TypeAssertExpr:
		pc.collectTypeAssertExpr(n)

	case *ast.StarExpr:
		pc.collectStarExpr(n)

	case *ast.ArrayType:
		pc.collectArrayType(n)

	case *ast.StructType:
		pc.collectStructType(n)

	case *ast.InterfaceType:
		pc.collectInterfaceType(n)

	case *ast.ChanType:
		pc.collectChanType(n)

	case *ast.FuncType:
		pc.collectFuncType(n)
	}

	return pc
}

func (pc *PatternCollector) collectForStmt(n *ast.ForStmt) {
	attrs := map[string]string{
		"has_init": fmt.Sprintf("%t", n.Init != nil),
		"has_cond": fmt.Sprintf("%t", n.Cond != nil),
		"has_post": fmt.Sprintf("%t", n.Post != nil),
	}

	// Capture init statement type
	if n.Init != nil {
		switch init := n.Init.(type) {
		case *ast.AssignStmt:
			attrs["init_type"] = init.Tok.String()
		case *ast.DeclStmt:
			attrs["init_type"] = "decl"
		default:
			attrs["init_type"] = fmt.Sprintf("%T", n.Init)
		}
	}

	// Capture post statement type
	if n.Post != nil {
		switch post := n.Post.(type) {
		case *ast.IncDecStmt:
			attrs["post_type"] = post.Tok.String()
		case *ast.AssignStmt:
			attrs["post_type"] = post.Tok.String()
		default:
			attrs["post_type"] = fmt.Sprintf("%T", n.Post)
		}
	}

	pc.addPattern("ForStmt", attrs, n.Pos())
}

func (pc *PatternCollector) collectRangeStmt(n *ast.RangeStmt) {
	attrs := map[string]string{
		"tok": n.Tok.String(),
	}

	// Key variable status
	if n.Key == nil {
		attrs["key"] = "none"
	} else if ident, ok := n.Key.(*ast.Ident); ok && ident.Name == "_" {
		attrs["key"] = "blank"
	} else {
		attrs["key"] = "used"
	}

	// Value variable status
	if n.Value == nil {
		attrs["value"] = "none"
	} else if ident, ok := n.Value.(*ast.Ident); ok && ident.Name == "_" {
		attrs["value"] = "blank"
	} else {
		attrs["value"] = "used"
	}

	// Collection type being ranged over
	if pc.typesInfo != nil {
		if tv, ok := pc.typesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["collection_type"] = pc.typeCategory(tv.Type)
		}
	}

	pc.addPattern("RangeStmt", attrs, n.Pos())
}

func (pc *PatternCollector) collectIfStmt(n *ast.IfStmt) {
	attrs := map[string]string{
		"has_init": fmt.Sprintf("%t", n.Init != nil),
		"has_else": fmt.Sprintf("%t", n.Else != nil),
	}

	if n.Else != nil {
		switch n.Else.(type) {
		case *ast.IfStmt:
			attrs["else_type"] = "else_if"
		case *ast.BlockStmt:
			attrs["else_type"] = "else_block"
		}
	}

	pc.addPattern("IfStmt", attrs, n.Pos())
}

func (pc *PatternCollector) collectSwitchStmt(n *ast.SwitchStmt) {
	attrs := map[string]string{
		"has_init": fmt.Sprintf("%t", n.Init != nil),
		"has_tag":  fmt.Sprintf("%t", n.Tag != nil),
	}

	// Count cases and check for default
	hasDefault := false
	caseCount := 0
	for _, stmt := range n.Body.List {
		if cc, ok := stmt.(*ast.CaseClause); ok {
			caseCount++
			if cc.List == nil {
				hasDefault = true
			}
		}
	}
	attrs["case_count"] = fmt.Sprintf("%d", caseCount)
	attrs["has_default"] = fmt.Sprintf("%t", hasDefault)

	pc.addPattern("SwitchStmt", attrs, n.Pos())
}

func (pc *PatternCollector) collectTypeSwitchStmt(n *ast.TypeSwitchStmt) {
	attrs := map[string]string{
		"has_init": fmt.Sprintf("%t", n.Init != nil),
	}
	pc.addPattern("TypeSwitchStmt", attrs, n.Pos())
}

func (pc *PatternCollector) collectAssignStmt(n *ast.AssignStmt) {
	attrs := map[string]string{
		"tok":       n.Tok.String(),
		"lhs_count": fmt.Sprintf("%d", len(n.Lhs)),
		"rhs_count": fmt.Sprintf("%d", len(n.Rhs)),
	}

	// Check for comma-ok idiom (v, ok := m[key] or v, ok := x.(T))
	if len(n.Lhs) == 2 && len(n.Rhs) == 1 {
		switch rhs := n.Rhs[0].(type) {
		case *ast.IndexExpr:
			// Check if it's map access
			if pc.typesInfo != nil {
				if tv, ok := pc.typesInfo.Types[rhs.X]; ok && tv.Type != nil {
					if _, isMap := tv.Type.Underlying().(*types.Map); isMap {
						attrs["comma_ok"] = "map"
					}
				}
			}
		case *ast.TypeAssertExpr:
			attrs["comma_ok"] = "type_assert"
		}
	}

	// Check RHS types
	if len(n.Rhs) == 1 && pc.typesInfo != nil {
		if tv, ok := pc.typesInfo.Types[n.Rhs[0]]; ok && tv.Type != nil {
			attrs["rhs_type"] = pc.typeCategory(tv.Type)
		}
	}

	pc.addPattern("AssignStmt", attrs, n.Pos())
}

func (pc *PatternCollector) collectCallExpr(n *ast.CallExpr) {
	attrs := map[string]string{
		"arg_count": fmt.Sprintf("%d", len(n.Args)),
	}

	switch fn := n.Fun.(type) {
	case *ast.Ident:
		attrs["func_type"] = "ident"
		attrs["func_name"] = fn.Name

		// Check for builtins
		switch fn.Name {
		case "make":
			attrs["builtin"] = "make"
			if len(n.Args) > 0 {
				if pc.typesInfo != nil {
					if tv, ok := pc.typesInfo.Types[n.Args[0]]; ok && tv.Type != nil {
						attrs["make_type"] = pc.typeCategory(tv.Type)
					}
				}
				// Also check AST for type
				switch n.Args[0].(type) {
				case *ast.ArrayType:
					attrs["make_ast"] = "slice"
				case *ast.MapType:
					attrs["make_ast"] = "map"
				case *ast.ChanType:
					attrs["make_ast"] = "chan"
				}
			}
		case "len", "cap", "append", "copy", "delete", "close", "panic", "recover", "print", "println", "new", "complex", "real", "imag":
			attrs["builtin"] = fn.Name
		}

	case *ast.SelectorExpr:
		attrs["func_type"] = "selector"
		attrs["method"] = fn.Sel.Name
		if ident, ok := fn.X.(*ast.Ident); ok {
			attrs["receiver"] = ident.Name
		}

		// Check if it's a method call on a type
		if pc.typesInfo != nil {
			if tv, ok := pc.typesInfo.Types[fn.X]; ok && tv.Type != nil {
				attrs["receiver_type"] = pc.typeCategory(tv.Type)
			}
		}

	case *ast.FuncLit:
		attrs["func_type"] = "literal"

	case *ast.IndexExpr:
		attrs["func_type"] = "generic"
	}

	// Check if variadic call
	if n.Ellipsis.IsValid() {
		attrs["variadic"] = "true"
	}

	pc.addPattern("CallExpr", attrs, n.Pos())
}

func (pc *PatternCollector) collectCompositeLit(n *ast.CompositeLit) {
	attrs := map[string]string{}

	// Check if elements are keyed
	hasKeys := false
	for _, elt := range n.Elts {
		if _, ok := elt.(*ast.KeyValueExpr); ok {
			hasKeys = true
			break
		}
	}
	attrs["keyed"] = fmt.Sprintf("%t", hasKeys)
	attrs["elt_count"] = fmt.Sprintf("%d", len(n.Elts))

	// Type of composite literal
	if pc.typesInfo != nil {
		if tv, ok := pc.typesInfo.Types[n]; ok && tv.Type != nil {
			attrs["type"] = pc.typeCategory(tv.Type)
		}
	}

	// Also check AST type
	switch n.Type.(type) {
	case *ast.ArrayType:
		attrs["ast_type"] = "array_or_slice"
	case *ast.MapType:
		attrs["ast_type"] = "map"
	case *ast.StructType:
		attrs["ast_type"] = "anon_struct"
	case *ast.Ident:
		attrs["ast_type"] = "named"
	case *ast.SelectorExpr:
		attrs["ast_type"] = "qualified"
	}

	pc.addPattern("CompositeLit", attrs, n.Pos())
}

func (pc *PatternCollector) collectMapType(n *ast.MapType) {
	attrs := map[string]string{}

	// Key type
	if pc.typesInfo != nil {
		if tv, ok := pc.typesInfo.Types[n.Key]; ok && tv.Type != nil {
			attrs["key_type"] = pc.typeCategory(tv.Type)
		}
		if tv, ok := pc.typesInfo.Types[n.Value]; ok && tv.Type != nil {
			attrs["value_type"] = pc.typeCategory(tv.Type)

			// Check for nested map
			if _, isMap := tv.Type.Underlying().(*types.Map); isMap {
				attrs["nested"] = "true"
			}
		}
	}

	// Also check AST for nested map
	if _, isMap := n.Value.(*ast.MapType); isMap {
		attrs["nested_ast"] = "true"
	}

	pc.addPattern("MapType", attrs, n.Pos())
}

func (pc *PatternCollector) collectIndexExpr(n *ast.IndexExpr) {
	attrs := map[string]string{}

	if pc.typesInfo != nil {
		if tv, ok := pc.typesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["collection_type"] = pc.typeCategory(tv.Type)
		}
		if tv, ok := pc.typesInfo.Types[n.Index]; ok && tv.Type != nil {
			attrs["index_type"] = pc.typeCategory(tv.Type)
		}
	}

	pc.addPattern("IndexExpr", attrs, n.Pos())
}

func (pc *PatternCollector) collectSliceExpr(n *ast.SliceExpr) {
	attrs := map[string]string{
		"has_low":  fmt.Sprintf("%t", n.Low != nil),
		"has_high": fmt.Sprintf("%t", n.High != nil),
		"has_max":  fmt.Sprintf("%t", n.Max != nil),
		"slice3":   fmt.Sprintf("%t", n.Slice3),
	}

	if pc.typesInfo != nil {
		if tv, ok := pc.typesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["collection_type"] = pc.typeCategory(tv.Type)
		}
	}

	pc.addPattern("SliceExpr", attrs, n.Pos())
}

func (pc *PatternCollector) collectUnaryExpr(n *ast.UnaryExpr) {
	attrs := map[string]string{
		"op": n.Op.String(),
	}

	if pc.typesInfo != nil {
		if tv, ok := pc.typesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["operand_type"] = pc.typeCategory(tv.Type)
		}
	}

	pc.addPattern("UnaryExpr", attrs, n.Pos())
}

func (pc *PatternCollector) collectBinaryExpr(n *ast.BinaryExpr) {
	attrs := map[string]string{
		"op": n.Op.String(),
	}

	if pc.typesInfo != nil {
		if tv, ok := pc.typesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["left_type"] = pc.typeCategory(tv.Type)
		}
		if tv, ok := pc.typesInfo.Types[n.Y]; ok && tv.Type != nil {
			attrs["right_type"] = pc.typeCategory(tv.Type)
		}
	}

	pc.addPattern("BinaryExpr", attrs, n.Pos())
}

func (pc *PatternCollector) collectFuncDecl(n *ast.FuncDecl) {
	attrs := map[string]string{
		"has_recv": fmt.Sprintf("%t", n.Recv != nil),
	}

	if n.Recv != nil && len(n.Recv.List) > 0 {
		// Check receiver type
		recv := n.Recv.List[0]
		switch t := recv.Type.(type) {
		case *ast.StarExpr:
			attrs["recv_ptr"] = "true"
		case *ast.Ident:
			attrs["recv_ptr"] = "false"
			_ = t
		}
	}

	if n.Type != nil {
		if n.Type.Params != nil {
			attrs["param_count"] = fmt.Sprintf("%d", len(n.Type.Params.List))
		}
		if n.Type.Results != nil {
			attrs["result_count"] = fmt.Sprintf("%d", len(n.Type.Results.List))
		} else {
			attrs["result_count"] = "0"
		}
	}

	pc.addPattern("FuncDecl", attrs, n.Pos())
}

func (pc *PatternCollector) collectFuncLit(n *ast.FuncLit) {
	attrs := map[string]string{}

	if n.Type != nil {
		if n.Type.Params != nil {
			attrs["param_count"] = fmt.Sprintf("%d", len(n.Type.Params.List))
		}
		if n.Type.Results != nil {
			attrs["result_count"] = fmt.Sprintf("%d", len(n.Type.Results.List))
		} else {
			attrs["result_count"] = "0"
		}
	}

	pc.addPattern("FuncLit", attrs, n.Pos())
}

func (pc *PatternCollector) collectReturnStmt(n *ast.ReturnStmt) {
	attrs := map[string]string{
		"result_count": fmt.Sprintf("%d", len(n.Results)),
	}
	pc.addPattern("ReturnStmt", attrs, n.Pos())
}

func (pc *PatternCollector) collectBranchStmt(n *ast.BranchStmt) {
	attrs := map[string]string{
		"tok":       n.Tok.String(),
		"has_label": fmt.Sprintf("%t", n.Label != nil),
	}
	pc.addPattern("BranchStmt", attrs, n.Pos())
}

func (pc *PatternCollector) collectIncDecStmt(n *ast.IncDecStmt) {
	attrs := map[string]string{
		"tok": n.Tok.String(),
	}

	if pc.typesInfo != nil {
		if tv, ok := pc.typesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["operand_type"] = pc.typeCategory(tv.Type)
		}
	}

	pc.addPattern("IncDecStmt", attrs, n.Pos())
}

func (pc *PatternCollector) collectTypeAssertExpr(n *ast.TypeAssertExpr) {
	attrs := map[string]string{
		"type_switch": fmt.Sprintf("%t", n.Type == nil),
	}

	if pc.typesInfo != nil {
		if n.Type != nil {
			if tv, ok := pc.typesInfo.Types[n.Type]; ok && tv.Type != nil {
				attrs["assert_type"] = pc.typeCategory(tv.Type)
			}
		}
	}

	pc.addPattern("TypeAssertExpr", attrs, n.Pos())
}

func (pc *PatternCollector) collectStarExpr(n *ast.StarExpr) {
	attrs := map[string]string{}

	if pc.typesInfo != nil {
		if tv, ok := pc.typesInfo.Types[n.X]; ok && tv.Type != nil {
			attrs["operand_type"] = pc.typeCategory(tv.Type)
		}
	}

	pc.addPattern("StarExpr", attrs, n.Pos())
}

func (pc *PatternCollector) collectArrayType(n *ast.ArrayType) {
	attrs := map[string]string{
		"is_slice": fmt.Sprintf("%t", n.Len == nil),
	}

	if pc.typesInfo != nil {
		if tv, ok := pc.typesInfo.Types[n.Elt]; ok && tv.Type != nil {
			attrs["elt_type"] = pc.typeCategory(tv.Type)
		}
	}

	pc.addPattern("ArrayType", attrs, n.Pos())
}

func (pc *PatternCollector) collectStructType(n *ast.StructType) {
	attrs := map[string]string{}
	if n.Fields != nil {
		attrs["field_count"] = fmt.Sprintf("%d", len(n.Fields.List))
	}
	pc.addPattern("StructType", attrs, n.Pos())
}

func (pc *PatternCollector) collectInterfaceType(n *ast.InterfaceType) {
	attrs := map[string]string{}
	if n.Methods != nil {
		attrs["method_count"] = fmt.Sprintf("%d", len(n.Methods.List))
	}
	pc.addPattern("InterfaceType", attrs, n.Pos())
}

func (pc *PatternCollector) collectChanType(n *ast.ChanType) {
	attrs := map[string]string{}
	switch n.Dir {
	case ast.SEND:
		attrs["dir"] = "send"
	case ast.RECV:
		attrs["dir"] = "recv"
	default:
		attrs["dir"] = "both"
	}
	pc.addPattern("ChanType", attrs, n.Pos())
}

func (pc *PatternCollector) collectFuncType(n *ast.FuncType) {
	attrs := map[string]string{}
	if n.Params != nil {
		attrs["param_count"] = fmt.Sprintf("%d", len(n.Params.List))
	}
	if n.Results != nil {
		attrs["result_count"] = fmt.Sprintf("%d", len(n.Results.List))
	}
	pc.addPattern("FuncType", attrs, n.Pos())
}

func (pc *PatternCollector) ProcessPackage(dir string, verbose bool) error {
	// Find all directories with .go files and process each individually
	var dirsToProcess []string

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if !info.IsDir() {
			return nil
		}

		name := info.Name()
		if strings.HasPrefix(name, ".") || name == "vendor" || name == "node_modules" {
			return filepath.SkipDir
		}

		// Skip sema-errors
		if strings.Contains(path, "sema-errors") {
			return filepath.SkipDir
		}

		// Check if this directory has .go files
		entries, _ := os.ReadDir(path)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
				dirsToProcess = append(dirsToProcess, path)
				break
			}
		}

		return nil
	})

	// Process each directory individually
	for _, d := range dirsToProcess {
		if verbose {
			fmt.Printf("  Loading %s... ", d)
		}

		loaded, err := pc.tryLoadPackageWithTypes(d)
		if err != nil || !loaded {
			if verbose {
				if err != nil {
					fmt.Printf("failed (%v), using AST-only\n", err)
				} else {
					fmt.Printf("no type info, using AST-only\n")
				}
			}
			pc.processDirectoryFilesAST(d)
		} else {
			if verbose {
				fmt.Println("OK (with type info)")
			}
		}
	}

	return nil
}

// tryLoadPackageWithTypes attempts to load a package with full type information
// Returns true if successful, false if we should fall back to AST-only
func (pc *PatternCollector) tryLoadPackageWithTypes(dir string) (bool, error) {
	// Check if there's a go.mod in this directory or any parent
	// If not, packages.Load might fail
	hasGoMod := false
	checkDir := dir
	for checkDir != "/" && checkDir != "." {
		if _, err := os.Stat(filepath.Join(checkDir, "go.mod")); err == nil {
			hasGoMod = true
			break
		}
		checkDir = filepath.Dir(checkDir)
	}

	if !hasGoMod {
		return false, nil
	}

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
			packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports | packages.NeedDeps,
		Dir:   dir,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, ".")
	if err != nil {
		return false, err
	}

	if len(pkgs) == 0 {
		return false, nil
	}

	for _, pkg := range pkgs {
		// Skip packages with errors
		if len(pkg.Errors) > 0 {
			continue
		}

		if pkg.Fset == nil || len(pkg.Syntax) == 0 || pkg.TypesInfo == nil {
			continue
		}

		pc.fset = pkg.Fset
		pc.typesInfo = pkg.TypesInfo

		for _, file := range pkg.Syntax {
			ast.Walk(pc, file)
		}

		return true, nil
	}

	return false, nil
}

func (pc *PatternCollector) processDirectoryFilesAST(dir string) {
	fset := token.NewFileSet()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			continue
		}
		pc.fset = fset
		pc.typesInfo = nil
		ast.Walk(pc, file)
	}
}


func (pc *PatternCollector) GetPatterns() []Pattern {
	var patterns []Pattern
	for _, p := range pc.patterns {
		patterns = append(patterns, p)
	}

	// Sort by key for consistent output
	sort.Slice(patterns, func(i, j int) bool {
		return patterns[i].Key() < patterns[j].Key()
	})

	return patterns
}

func (pc *PatternCollector) PrintSummary() {
	// Group patterns by kind
	byKind := make(map[string]int)
	for _, p := range pc.patterns {
		byKind[p.Kind]++
	}

	var kinds []string
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	fmt.Printf("Collected %d unique patterns:\n", len(pc.patterns))
	for _, kind := range kinds {
		fmt.Printf("  - %s: %d variants\n", kind, byKind[kind])
	}
}

// PatternDatabase stores collected patterns for checking
type PatternDatabase struct {
	Patterns           map[string]Pattern `json:"patterns"`
	normalizedPatterns map[string]bool    // built on load, not serialized
}

func LoadPatternDatabase(filename string) (*PatternDatabase, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var db PatternDatabase
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, err
	}

	// Build normalized pattern index
	db.buildNormalizedIndex()

	return &db, nil
}

func (db *PatternDatabase) buildNormalizedIndex() {
	db.normalizedPatterns = make(map[string]bool)
	for _, p := range db.Patterns {
		normalizedKey := p.NormalizedKey()
		db.normalizedPatterns[normalizedKey] = true
	}
}

func (db *PatternDatabase) Save(filename string) error {
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

// HasPattern checks if a pattern is supported using two-level matching.
// It uses normalized keys which strip type-specific attributes except
// where types actually matter (e.g., map key types, range collection types).
func (db *PatternDatabase) HasPattern(p Pattern) bool {
	normalizedKey := p.NormalizedKey()
	return db.normalizedPatterns[normalizedKey]
}

// GenerateDoc creates a human-readable language reference document
// All content is derived from analyzing the patterns - nothing is hardcoded.
func (db *PatternDatabase) GenerateDoc() string {
	var sb strings.Builder

	// Collect and analyze all patterns
	byKind := make(map[string][]Pattern)
	for _, p := range db.Patterns {
		byKind[p.Kind] = append(byKind[p.Kind], p)
	}

	// Pre-collect data needed for examples
	assignOps := make(map[string]bool)
	for _, p := range byKind["AssignStmt"] {
		if tok := p.Attrs["tok"]; tok != "" {
			assignOps[tok] = true
		}
	}

	binaryOps := make(map[string]bool)
	for _, p := range byKind["BinaryExpr"] {
		if op := p.Attrs["op"]; op != "" {
			binaryOps[op] = true
		}
	}

	unaryOps := make(map[string]bool)
	for _, p := range byKind["UnaryExpr"] {
		if op := p.Attrs["op"]; op != "" {
			unaryOps[op] = true
		}
	}

	incDecOps := make(map[string]bool)
	for _, p := range byKind["IncDecStmt"] {
		if tok := p.Attrs["tok"]; tok != "" {
			incDecOps[tok] = true
		}
	}

	builtins := make(map[string]bool)
	makeTargets := make(map[string]bool)
	for _, p := range byKind["CallExpr"] {
		if b := p.Attrs["builtin"]; b != "" {
			builtins[b] = true
			if b == "make" {
				if t := p.Attrs["make_ast"]; t != "" {
					makeTargets[t] = true
				}
			}
		}
	}

	rangeTypes := make(map[string]bool)
	for _, p := range byKind["RangeStmt"] {
		if ct := p.Attrs["collection_type"]; ct != "" {
			rangeTypes[ct] = true
		}
	}

	mapKeyTypes := make(map[string]bool)
	for _, p := range byKind["MapType"] {
		if kt := p.Attrs["key_type"]; kt != "" {
			mapKeyTypes[kt] = true
		}
	}

	sb.WriteString("# GoAny Language Reference\n\n")
	sb.WriteString("This document is auto-generated from pattern analysis.\n\n")

	// === OPERATORS ===
	sb.WriteString("## Operators\n\n")

	// Binary operators with examples
	if len(binaryOps) > 0 {
		sb.WriteString("### Binary Operators\n\n")

		// Arithmetic
		arithOps := []string{}
		for _, op := range []string{"+", "-", "*", "/", "%"} {
			if binaryOps[op] {
				arithOps = append(arithOps, op)
			}
		}
		if len(arithOps) > 0 {
			sb.WriteString("**Arithmetic:** `" + strings.Join(arithOps, "`, `") + "`\n")
			sb.WriteString("```go\n")
			if binaryOps["+"] {
				sb.WriteString("a + b\n")
			}
			if binaryOps["-"] {
				sb.WriteString("a - b\n")
			}
			if binaryOps["*"] {
				sb.WriteString("a * b\n")
			}
			if binaryOps["/"] {
				sb.WriteString("a / b\n")
			}
			if binaryOps["%"] {
				sb.WriteString("a % b\n")
			}
			sb.WriteString("```\n\n")
		}

		// Comparison
		cmpOps := []string{}
		for _, op := range []string{"==", "!=", "<", ">", "<=", ">="} {
			if binaryOps[op] {
				cmpOps = append(cmpOps, op)
			}
		}
		if len(cmpOps) > 0 {
			sb.WriteString("**Comparison:** `" + strings.Join(cmpOps, "`, `") + "`\n")
			sb.WriteString("```go\n")
			if binaryOps["=="] {
				sb.WriteString("a == b\n")
			}
			if binaryOps["!="] {
				sb.WriteString("a != b\n")
			}
			if binaryOps["<"] {
				sb.WriteString("a < b\n")
			}
			if binaryOps[">"] {
				sb.WriteString("a > b\n")
			}
			sb.WriteString("```\n\n")
		}

		// Logical
		logicOps := []string{}
		for _, op := range []string{"&&", "||"} {
			if binaryOps[op] {
				logicOps = append(logicOps, op)
			}
		}
		if len(logicOps) > 0 || unaryOps["!"] {
			sb.WriteString("**Logical:** ")
			all := logicOps
			if unaryOps["!"] {
				all = append(all, "!")
			}
			sb.WriteString("`" + strings.Join(all, "`, `") + "`\n")
			sb.WriteString("```go\n")
			if binaryOps["&&"] {
				sb.WriteString("a && b\n")
			}
			if binaryOps["||"] {
				sb.WriteString("a || b\n")
			}
			if unaryOps["!"] {
				sb.WriteString("!a\n")
			}
			sb.WriteString("```\n\n")
		}

		// Bitwise
		bitOps := []string{}
		for _, op := range []string{"&", "|", "^", "<<", ">>"} {
			if binaryOps[op] {
				bitOps = append(bitOps, op)
			}
		}
		if len(bitOps) > 0 {
			sb.WriteString("**Bitwise:** `" + strings.Join(bitOps, "`, `") + "`\n")
			sb.WriteString("```go\n")
			if binaryOps["&"] {
				sb.WriteString("a & b   // AND\n")
			}
			if binaryOps["|"] {
				sb.WriteString("a | b   // OR\n")
			}
			if binaryOps["^"] {
				sb.WriteString("a ^ b   // XOR\n")
			}
			if binaryOps["<<"] {
				sb.WriteString("a << n  // left shift\n")
			}
			if binaryOps[">>"] {
				sb.WriteString("a >> n  // right shift\n")
			}
			sb.WriteString("```\n\n")
		}
	}

	// Assignment operators with examples
	if len(assignOps) > 0 {
		sb.WriteString("### Assignment\n\n")
		var ops []string
		for op := range assignOps {
			ops = append(ops, op)
		}
		sort.Strings(ops)
		sb.WriteString("**Operators:** `" + strings.Join(ops, "`, `") + "`\n")
		sb.WriteString("```go\n")
		if assignOps[":="] {
			sb.WriteString("x := 10      // short declaration\n")
		}
		if assignOps["="] {
			sb.WriteString("x = 20       // assignment\n")
		}
		if assignOps["+="] {
			sb.WriteString("x += 5       // add and assign\n")
		}
		if assignOps["-="] {
			sb.WriteString("x -= 3       // subtract and assign\n")
		}
		sb.WriteString("```\n\n")
	}

	// Increment/Decrement with examples
	if len(incDecOps) > 0 {
		sb.WriteString("### Increment/Decrement\n\n")
		sb.WriteString("```go\n")
		if incDecOps["++"] {
			sb.WriteString("i++\n")
		}
		if incDecOps["--"] {
			sb.WriteString("i--\n")
		}
		sb.WriteString("```\n\n")
	}

	// === CONTROL FLOW ===
	sb.WriteString("## Control Flow\n\n")

	// For loops with examples
	if patterns, ok := byKind["ForStmt"]; ok && len(patterns) > 0 {
		sb.WriteString("### For Loop\n\n")

		hasFullFor := false
		hasCondOnly := false
		hasInfinite := false
		for _, p := range patterns {
			hasInit := p.Attrs["has_init"] == "true"
			hasCond := p.Attrs["has_cond"] == "true"
			hasPost := p.Attrs["has_post"] == "true"
			if hasInit && hasCond && hasPost {
				hasFullFor = true
			} else if !hasInit && hasCond && !hasPost {
				hasCondOnly = true
			} else if !hasInit && !hasCond && !hasPost {
				hasInfinite = true
			}
		}

		sb.WriteString("```go\n")
		if hasFullFor {
			sb.WriteString("// C-style for loop\n")
			sb.WriteString("for i := 0; i < 10; i++ {\n")
			sb.WriteString("    // ...\n")
			sb.WriteString("}\n\n")
		}
		if hasCondOnly {
			sb.WriteString("// Condition-only (while-style)\n")
			sb.WriteString("for x > 0 {\n")
			sb.WriteString("    x--\n")
			sb.WriteString("}\n\n")
		}
		if hasInfinite {
			sb.WriteString("// Infinite loop\n")
			sb.WriteString("for {\n")
			sb.WriteString("    if done {\n")
			sb.WriteString("        break\n")
			sb.WriteString("    }\n")
			sb.WriteString("}\n")
		}
		sb.WriteString("```\n\n")
	}

	// Range loops with examples
	if len(rangeTypes) > 0 {
		sb.WriteString("### Range Loop\n\n")
		sb.WriteString("**Supported collection types:** ")
		var typeList []string
		for t := range rangeTypes {
			typeList = append(typeList, "`"+t+"`")
		}
		sort.Strings(typeList)
		sb.WriteString(strings.Join(typeList, ", ") + "\n\n")

		sb.WriteString("```go\n")
		if rangeTypes["slice"] {
			sb.WriteString("// Range over slice\n")
			sb.WriteString("for i, v := range items {\n")
			sb.WriteString("    // i = index, v = value\n")
			sb.WriteString("}\n\n")
			sb.WriteString("for i := range items {\n")
			sb.WriteString("    // index only\n")
			sb.WriteString("}\n\n")
			sb.WriteString("for _, v := range items {\n")
			sb.WriteString("    // value only\n")
			sb.WriteString("}\n")
		}
		if rangeTypes["string"] {
			sb.WriteString("\n// Range over string\n")
			sb.WriteString("for i, r := range str {\n")
			sb.WriteString("    // i = byte index, r = rune\n")
			sb.WriteString("}\n")
		}
		sb.WriteString("```\n\n")

		if !rangeTypes["map"] {
			sb.WriteString("**Not supported:** `for k, v := range map` - use index-based access instead\n\n")
		}
	}

	// If statements with examples
	if patterns, ok := byKind["IfStmt"]; ok && len(patterns) > 0 {
		sb.WriteString("### If Statement\n\n")

		hasInit := false
		hasElse := false
		hasElseIf := false
		for _, p := range patterns {
			if p.Attrs["has_init"] == "true" {
				hasInit = true
			}
			if p.Attrs["has_else"] == "true" {
				hasElse = true
			}
			if p.Attrs["else_type"] == "else_if" {
				hasElseIf = true
			}
		}

		sb.WriteString("```go\n")
		sb.WriteString("if x > 0 {\n")
		sb.WriteString("    // ...\n")
		sb.WriteString("}\n")
		if hasElse {
			sb.WriteString("\nif x > 0 {\n")
			sb.WriteString("    // ...\n")
			sb.WriteString("} else {\n")
			sb.WriteString("    // ...\n")
			sb.WriteString("}\n")
		}
		if hasElseIf {
			sb.WriteString("\nif x > 0 {\n")
			sb.WriteString("    // ...\n")
			sb.WriteString("} else if x < 0 {\n")
			sb.WriteString("    // ...\n")
			sb.WriteString("} else {\n")
			sb.WriteString("    // ...\n")
			sb.WriteString("}\n")
		}
		if hasInit {
			sb.WriteString("\n// With init statement\n")
			sb.WriteString("if v := getValue(); v > 0 {\n")
			sb.WriteString("    // v is scoped to if block\n")
			sb.WriteString("}\n")
		}
		sb.WriteString("```\n\n")
	}

	// Switch with examples
	if patterns, ok := byKind["SwitchStmt"]; ok && len(patterns) > 0 {
		sb.WriteString("### Switch Statement\n\n")

		hasTag := false
		hasDefault := false
		for _, p := range patterns {
			if p.Attrs["has_tag"] == "true" {
				hasTag = true
			}
			if p.Attrs["has_default"] == "true" {
				hasDefault = true
			}
		}

		sb.WriteString("```go\n")
		if hasTag {
			sb.WriteString("switch value {\n")
			sb.WriteString("case 1:\n")
			sb.WriteString("    // ...\n")
			sb.WriteString("case 2, 3:\n")
			sb.WriteString("    // multiple values\n")
			if hasDefault {
				sb.WriteString("default:\n")
				sb.WriteString("    // ...\n")
			}
			sb.WriteString("}\n")
		}
		sb.WriteString("```\n\n")
	}

	// Branch statements with examples
	if patterns, ok := byKind["BranchStmt"]; ok && len(patterns) > 0 {
		toks := make(map[string]bool)
		for _, p := range patterns {
			if tok := p.Attrs["tok"]; tok != "" {
				toks[tok] = true
			}
		}

		sb.WriteString("### Break and Continue\n\n")
		sb.WriteString("```go\n")
		sb.WriteString("for i := 0; i < 10; i++ {\n")
		if toks["continue"] {
			sb.WriteString("    if i%2 == 0 {\n")
			sb.WriteString("        continue  // skip even numbers\n")
			sb.WriteString("    }\n")
		}
		if toks["break"] {
			sb.WriteString("    if i > 5 {\n")
			sb.WriteString("        break     // exit loop\n")
			sb.WriteString("    }\n")
		}
		sb.WriteString("}\n")
		sb.WriteString("```\n\n")
	}

	// === FUNCTIONS ===
	sb.WriteString("## Functions\n\n")

	if patterns, ok := byKind["FuncDecl"]; ok && len(patterns) > 0 {
		hasMethod := false
		hasPtrRecv := false
		for _, p := range patterns {
			if p.Attrs["has_recv"] == "true" {
				hasMethod = true
			}
			if p.Attrs["recv_ptr"] == "true" {
				hasPtrRecv = true
			}
		}

		sb.WriteString("### Function Declarations\n\n")
		sb.WriteString("```go\n")
		sb.WriteString("func add(a, b int) int {\n")
		sb.WriteString("    return a + b\n")
		sb.WriteString("}\n\n")
		sb.WriteString("// Multiple return values\n")
		sb.WriteString("func divide(a, b int) (int, bool) {\n")
		sb.WriteString("    if b == 0 {\n")
		sb.WriteString("        return 0, false\n")
		sb.WriteString("    }\n")
		sb.WriteString("    return a / b, true\n")
		sb.WriteString("}\n")
		if hasMethod {
			sb.WriteString("\n// Method\n")
			sb.WriteString("func (p Point) Distance() float64 {\n")
			sb.WriteString("    return math.Sqrt(p.X*p.X + p.Y*p.Y)\n")
			sb.WriteString("}\n")
		}
		if hasPtrRecv {
			sb.WriteString("\n// Method with pointer receiver\n")
			sb.WriteString("func (p *Point) Scale(factor int) {\n")
			sb.WriteString("    p.X *= factor\n")
			sb.WriteString("    p.Y *= factor\n")
			sb.WriteString("}\n")
		}
		sb.WriteString("```\n\n")
	}

	// Built-in functions with examples
	if len(builtins) > 0 {
		sb.WriteString("### Built-in Functions\n\n")
		sb.WriteString("```go\n")
		if builtins["len"] {
			sb.WriteString("n := len(slice)     // length of slice, string, or map\n")
		}
		if builtins["append"] {
			sb.WriteString("s = append(s, x)    // append to slice\n")
		}
		if makeTargets["slice"] {
			sb.WriteString("s := make([]int, 5) // create slice with length\n")
		}
		if makeTargets["map"] {
			sb.WriteString("m := make(map[string]int)  // create map\n")
		}
		if builtins["delete"] {
			sb.WriteString("delete(m, key)      // delete from map\n")
		}
		sb.WriteString("```\n\n")
	}

	// === TYPES ===
	sb.WriteString("## Types\n\n")

	// Slices with examples
	if patterns, ok := byKind["ArrayType"]; ok && len(patterns) > 0 {
		hasSlice := false
		for _, p := range patterns {
			if p.Attrs["is_slice"] == "true" {
				hasSlice = true
			}
		}

		if hasSlice {
			sb.WriteString("### Slices\n\n")
			sb.WriteString("```go\n")
			sb.WriteString("// Create\n")
			sb.WriteString("s := []int{1, 2, 3}\n")
			if makeTargets["slice"] {
				sb.WriteString("s := make([]int, 5)\n")
			}
			sb.WriteString("\n// Access\n")
			sb.WriteString("x := s[0]\n")
			sb.WriteString("s[1] = 10\n")
			if builtins["append"] {
				sb.WriteString("\n// Append\n")
				sb.WriteString("s = append(s, 4)\n")
			}
			sb.WriteString("```\n\n")
		}
	}

	// Slice expressions with examples
	if patterns, ok := byKind["SliceExpr"]; ok && len(patterns) > 0 {
		sb.WriteString("### Slice Expressions\n\n")
		sb.WriteString("```go\n")
		for _, p := range patterns {
			hasLow := p.Attrs["has_low"] == "true"
			hasHigh := p.Attrs["has_high"] == "true"
			hasMax := p.Attrs["has_max"] == "true"
			if hasLow && hasHigh && hasMax {
				sb.WriteString("s[1:3:5]  // slice with capacity\n")
			} else if hasLow && hasHigh {
				sb.WriteString("s[1:3]    // elements 1, 2\n")
			} else if hasLow {
				sb.WriteString("s[1:]     // from index 1 to end\n")
			} else if hasHigh {
				sb.WriteString("s[:3]     // from start to index 2\n")
			}
		}
		sb.WriteString("```\n\n")
	}

	// Maps with examples
	if len(mapKeyTypes) > 0 {
		sb.WriteString("### Maps\n\n")
		sb.WriteString("**Supported key types:** ")
		var ktList []string
		for kt := range mapKeyTypes {
			ktList = append(ktList, "`"+kt+"`")
		}
		sort.Strings(ktList)
		sb.WriteString(strings.Join(ktList, ", ") + "\n\n")

		sb.WriteString("```go\n")
		if makeTargets["map"] {
			sb.WriteString("// Create\n")
			sb.WriteString("m := make(map[string]int)\n\n")
		}
		sb.WriteString("// Set\n")
		sb.WriteString("m[\"key\"] = 42\n\n")
		sb.WriteString("// Get\n")
		sb.WriteString("v := m[\"key\"]\n\n")
		sb.WriteString("// Check existence\n")
		sb.WriteString("v, ok := m[\"key\"]\n")
		sb.WriteString("if ok {\n")
		sb.WriteString("    // key exists\n")
		sb.WriteString("}\n")
		if builtins["delete"] {
			sb.WriteString("\n// Delete\n")
			sb.WriteString("delete(m, \"key\")\n")
		}
		if builtins["len"] {
			sb.WriteString("\n// Length\n")
			sb.WriteString("n := len(m)\n")
		}
		sb.WriteString("```\n\n")

		// Map restrictions
		sb.WriteString("**Restrictions:**\n")
		hasNested := false
		hasStructKey := false
		for _, p := range byKind["MapType"] {
			if p.Attrs["nested"] == "true" {
				hasNested = true
			}
			if p.Attrs["key_type"] == "struct" {
				hasStructKey = true
			}
		}
		hasMapLiteral := false
		for _, p := range byKind["CompositeLit"] {
			if p.Attrs["ast_type"] == "map" {
				hasMapLiteral = true
			}
		}
		if !hasMapLiteral {
			sb.WriteString("- No map literals (`map[K]V{...}`)\n")
		}
		if !hasNested {
			sb.WriteString("- No nested maps\n")
		}
		if !rangeTypes["map"] {
			sb.WriteString("- No range over map\n")
		}
		if !hasStructKey {
			sb.WriteString("- No struct keys\n")
		}
		sb.WriteString("\n")
	}

	// Structs with examples
	if patterns, ok := byKind["StructType"]; ok && len(patterns) > 0 {
		sb.WriteString("### Structs\n\n")
		sb.WriteString("```go\n")
		sb.WriteString("// Define\n")
		sb.WriteString("type Point struct {\n")
		sb.WriteString("    X int\n")
		sb.WriteString("    Y int\n")
		sb.WriteString("}\n\n")
		sb.WriteString("// Create\n")
		sb.WriteString("p := Point{X: 10, Y: 20}\n")
		sb.WriteString("p := Point{10, 20}\n\n")
		sb.WriteString("// Access\n")
		sb.WriteString("x := p.X\n")
		sb.WriteString("p.Y = 30\n")
		sb.WriteString("```\n\n")
	}

	// === NOT SUPPORTED ===
	sb.WriteString("## Not Supported\n\n")

	notSupported := []string{}

	if _, ok := byKind["GoStmt"]; !ok {
		notSupported = append(notSupported, "Goroutines (`go func() {...}()`)")
	}
	if _, ok := byKind["DeferStmt"]; !ok {
		notSupported = append(notSupported, "`defer`")
	}
	if _, ok := byKind["SelectStmt"]; !ok {
		notSupported = append(notSupported, "`select`")
	}
	if _, ok := byKind["ChanType"]; !ok {
		notSupported = append(notSupported, "Channels (`chan T`)")
	}
	if !rangeTypes["map"] && len(byKind["RangeStmt"]) > 0 {
		notSupported = append(notSupported, "`for k, v := range map`")
	}
	if !makeTargets["slice"] && builtins["make"] {
		notSupported = append(notSupported, "`make([]T, n)` for slices")
	}

	for _, ns := range notSupported {
		sb.WriteString("- " + ns + "\n")
	}

	return sb.String()
}

// GenerateReport creates a concise report of supported constructs
func (db *PatternDatabase) GenerateReport() string {
	var sb strings.Builder

	// Group patterns by kind
	byKind := make(map[string][]Pattern)
	for _, p := range db.Patterns {
		byKind[p.Kind] = append(byKind[p.Kind], p)
	}

	// Sort kinds
	var kinds []string
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)

	sb.WriteString("# Supported Constructs Report\n\n")

	// First section: All normalized rules (what actually gets matched)
	sb.WriteString("## All Normalized Rules\n\n")
	sb.WriteString("These are the actual patterns used for matching (type-insignificant attributes stripped):\n\n")

	normalizedByKind := make(map[string]map[string]bool)
	for _, p := range db.Patterns {
		if normalizedByKind[p.Kind] == nil {
			normalizedByKind[p.Kind] = make(map[string]bool)
		}
		normalizedByKind[p.Kind][p.NormalizedKey()] = true
	}

	for _, kind := range kinds {
		sb.WriteString(fmt.Sprintf("### %s\n", kind))
		var rules []string
		for rule := range normalizedByKind[kind] {
			rules = append(rules, rule)
		}
		sort.Strings(rules)
		for _, rule := range rules {
			sb.WriteString(fmt.Sprintf("  %s\n", rule))
		}
		sb.WriteString("\n")
	}

	// Second section: Summary view
	sb.WriteString("## Summary\n\n")

	for _, kind := range kinds {
		patterns := byKind[kind]
		sb.WriteString(fmt.Sprintf("### %s\n", kind))

		switch kind {
		case "ForStmt":
			db.reportForStmt(&sb, patterns)
		case "RangeStmt":
			db.reportRangeStmt(&sb, patterns)
		case "IfStmt":
			db.reportIfStmt(&sb, patterns)
		case "SwitchStmt":
			db.reportSwitchStmt(&sb, patterns)
		case "MapType":
			db.reportMapType(&sb, patterns)
		case "BinaryExpr":
			db.reportBinaryExpr(&sb, patterns)
		case "UnaryExpr":
			db.reportUnaryExpr(&sb, patterns)
		case "CallExpr":
			db.reportCallExpr(&sb, patterns)
		case "AssignStmt":
			db.reportAssignStmt(&sb, patterns)
		case "BranchStmt":
			db.reportBranchStmt(&sb, patterns)
		case "IndexExpr":
			db.reportIndexExpr(&sb, patterns)
		case "SliceExpr":
			db.reportSliceExpr(&sb, patterns)
		case "CompositeLit":
			db.reportCompositeLit(&sb, patterns)
		case "FuncDecl":
			db.reportFuncDecl(&sb, patterns)
		default:
			sb.WriteString(fmt.Sprintf("  %d variant(s)\n", len(patterns)))
		}
		sb.WriteString("\n")
	}

	// Add "Not Supported" section
	sb.WriteString("## Not Supported\n")
	notSupported := []string{}
	for _, kind := range []string{"GoStmt", "DeferStmt", "SelectStmt", "SendStmt", "ChanType"} {
		if _, exists := byKind[kind]; !exists {
			notSupported = append(notSupported, kind)
		}
	}
	// Check for range over map
	hasRangeMap := false
	for _, p := range byKind["RangeStmt"] {
		if p.Attrs["collection_type"] == "map" {
			hasRangeMap = true
			break
		}
	}
	if !hasRangeMap {
		notSupported = append(notSupported, "range over map")
	}
	// Check for nested maps
	hasNestedMap := false
	for _, p := range byKind["MapType"] {
		if p.Attrs["nested"] == "true" {
			hasNestedMap = true
			break
		}
	}
	if !hasNestedMap {
		notSupported = append(notSupported, "nested maps")
	}
	// Check for struct map keys
	hasStructKey := false
	for _, p := range byKind["MapType"] {
		if p.Attrs["key_type"] == "struct" {
			hasStructKey = true
			break
		}
	}
	if !hasStructKey {
		notSupported = append(notSupported, "struct as map key")
	}
	// Check for map literals
	hasMapLiteral := false
	for _, p := range byKind["CompositeLit"] {
		if p.Attrs["ast_type"] == "map" {
			hasMapLiteral = true
			break
		}
	}
	if !hasMapLiteral {
		notSupported = append(notSupported, "map literals")
	}

	for _, ns := range notSupported {
		sb.WriteString(fmt.Sprintf("  - %s\n", ns))
	}

	return sb.String()
}

func (db *PatternDatabase) reportForStmt(sb *strings.Builder, patterns []Pattern) {
	variants := make(map[string]bool)
	for _, p := range patterns {
		hasInit := p.Attrs["has_init"] == "true"
		hasCond := p.Attrs["has_cond"] == "true"
		hasPost := p.Attrs["has_post"] == "true"

		if hasInit && hasCond && hasPost {
			variants["for init; cond; post {}"] = true
		} else if !hasInit && hasCond && !hasPost {
			variants["for cond {}"] = true
		} else if !hasInit && !hasCond && !hasPost {
			variants["for {}"] = true
		} else {
			variants[fmt.Sprintf("for (init=%v, cond=%v, post=%v)", hasInit, hasCond, hasPost)] = true
		}
	}
	for v := range variants {
		sb.WriteString(fmt.Sprintf("  - %s\n", v))
	}
}

func (db *PatternDatabase) reportRangeStmt(sb *strings.Builder, patterns []Pattern) {
	types := make(map[string]bool)
	for _, p := range patterns {
		if ct := p.Attrs["collection_type"]; ct != "" {
			types[ct] = true
		}
	}
	sb.WriteString("  collection types: ")
	var typeList []string
	for t := range types {
		typeList = append(typeList, t)
	}
	sort.Strings(typeList)
	sb.WriteString(strings.Join(typeList, ", "))
	sb.WriteString("\n")
}

func (db *PatternDatabase) reportIfStmt(sb *strings.Builder, patterns []Pattern) {
	hasInit := false
	hasElse := false
	hasElseIf := false
	for _, p := range patterns {
		if p.Attrs["has_init"] == "true" {
			hasInit = true
		}
		if p.Attrs["has_else"] == "true" {
			hasElse = true
		}
		if p.Attrs["else_type"] == "else_if" {
			hasElseIf = true
		}
	}
	sb.WriteString(fmt.Sprintf("  if-init: %v, else: %v, else-if: %v\n", hasInit, hasElse, hasElseIf))
}

func (db *PatternDatabase) reportSwitchStmt(sb *strings.Builder, patterns []Pattern) {
	hasTag := false
	hasDefault := false
	for _, p := range patterns {
		if p.Attrs["has_tag"] == "true" {
			hasTag = true
		}
		if p.Attrs["has_default"] == "true" {
			hasDefault = true
		}
	}
	sb.WriteString(fmt.Sprintf("  switch-tag: %v, default-case: %v\n", hasTag, hasDefault))
}

func (db *PatternDatabase) reportMapType(sb *strings.Builder, patterns []Pattern) {
	keyTypes := make(map[string]bool)
	for _, p := range patterns {
		if kt := p.Attrs["key_type"]; kt != "" {
			keyTypes[kt] = true
		}
	}
	sb.WriteString("  key types: ")
	var typeList []string
	for t := range keyTypes {
		typeList = append(typeList, t)
	}
	sort.Strings(typeList)
	sb.WriteString(strings.Join(typeList, ", "))
	sb.WriteString("\n")
}

func (db *PatternDatabase) reportBinaryExpr(sb *strings.Builder, patterns []Pattern) {
	ops := make(map[string]bool)
	for _, p := range patterns {
		if op := p.Attrs["op"]; op != "" {
			ops[op] = true
		}
	}
	sb.WriteString("  operators: ")
	var opList []string
	for op := range ops {
		opList = append(opList, op)
	}
	sort.Strings(opList)
	sb.WriteString(strings.Join(opList, ", "))
	sb.WriteString("\n")
}

func (db *PatternDatabase) reportUnaryExpr(sb *strings.Builder, patterns []Pattern) {
	ops := make(map[string]bool)
	for _, p := range patterns {
		if op := p.Attrs["op"]; op != "" {
			ops[op] = true
		}
	}
	sb.WriteString("  operators: ")
	var opList []string
	for op := range ops {
		opList = append(opList, op)
	}
	sort.Strings(opList)
	sb.WriteString(strings.Join(opList, ", "))
	sb.WriteString("\n")
}

func (db *PatternDatabase) reportCallExpr(sb *strings.Builder, patterns []Pattern) {
	builtins := make(map[string]bool)
	methods := make(map[string]bool)
	for _, p := range patterns {
		if b := p.Attrs["builtin"]; b != "" {
			builtins[b] = true
		}
		if r := p.Attrs["receiver"]; r != "" {
			if m := p.Attrs["method"]; m != "" {
				methods[r+"."+m] = true
			}
		}
	}
	if len(builtins) > 0 {
		sb.WriteString("  builtins: ")
		var list []string
		for b := range builtins {
			list = append(list, b)
		}
		sort.Strings(list)
		sb.WriteString(strings.Join(list, ", "))
		sb.WriteString("\n")
	}
	if len(methods) > 0 {
		sb.WriteString(fmt.Sprintf("  package calls: %d unique\n", len(methods)))
	}
}

func (db *PatternDatabase) reportAssignStmt(sb *strings.Builder, patterns []Pattern) {
	toks := make(map[string]bool)
	hasCommaOk := false
	for _, p := range patterns {
		if tok := p.Attrs["tok"]; tok != "" {
			toks[tok] = true
		}
		if p.Attrs["comma_ok"] != "" {
			hasCommaOk = true
		}
	}
	sb.WriteString("  tokens: ")
	var tokList []string
	for t := range toks {
		tokList = append(tokList, t)
	}
	sort.Strings(tokList)
	sb.WriteString(strings.Join(tokList, ", "))
	sb.WriteString(fmt.Sprintf(", comma-ok: %v\n", hasCommaOk))
}

func (db *PatternDatabase) reportBranchStmt(sb *strings.Builder, patterns []Pattern) {
	toks := make(map[string]bool)
	for _, p := range patterns {
		if tok := p.Attrs["tok"]; tok != "" {
			toks[tok] = true
		}
	}
	var tokList []string
	for t := range toks {
		tokList = append(tokList, t)
	}
	sort.Strings(tokList)
	sb.WriteString("  " + strings.Join(tokList, ", ") + "\n")
}

func (db *PatternDatabase) reportIndexExpr(sb *strings.Builder, patterns []Pattern) {
	types := make(map[string]bool)
	for _, p := range patterns {
		if ct := p.Attrs["collection_type"]; ct != "" {
			types[ct] = true
		}
	}
	sb.WriteString("  collection types: ")
	var typeList []string
	for t := range types {
		typeList = append(typeList, t)
	}
	sort.Strings(typeList)
	sb.WriteString(strings.Join(typeList, ", "))
	sb.WriteString("\n")
}

func (db *PatternDatabase) reportSliceExpr(sb *strings.Builder, patterns []Pattern) {
	variants := make(map[string]bool)
	for _, p := range patterns {
		hasLow := p.Attrs["has_low"] == "true"
		hasHigh := p.Attrs["has_high"] == "true"
		hasMax := p.Attrs["has_max"] == "true"

		if hasLow && hasHigh && hasMax {
			variants["a[low:high:max]"] = true
		} else if hasLow && hasHigh {
			variants["a[low:high]"] = true
		} else if hasLow {
			variants["a[low:]"] = true
		} else if hasHigh {
			variants["a[:high]"] = true
		} else {
			variants["a[:]"] = true
		}
	}
	for v := range variants {
		sb.WriteString(fmt.Sprintf("  - %s\n", v))
	}
}

func (db *PatternDatabase) reportCompositeLit(sb *strings.Builder, patterns []Pattern) {
	types := make(map[string]bool)
	for _, p := range patterns {
		if t := p.Attrs["ast_type"]; t != "" {
			types[t] = true
		} else if t := p.Attrs["type"]; t != "" {
			types[t] = true
		}
	}
	sb.WriteString("  types: ")
	var typeList []string
	for t := range types {
		typeList = append(typeList, t)
	}
	sort.Strings(typeList)
	sb.WriteString(strings.Join(typeList, ", "))
	sb.WriteString("\n")
}

func (db *PatternDatabase) reportFuncDecl(sb *strings.Builder, patterns []Pattern) {
	hasMethod := false
	hasPtrRecv := false
	for _, p := range patterns {
		if p.Attrs["has_recv"] == "true" {
			hasMethod = true
		}
		if p.Attrs["recv_ptr"] == "true" {
			hasPtrRecv = true
		}
	}
	sb.WriteString(fmt.Sprintf("  methods: %v, pointer-receiver: %v\n", hasMethod, hasPtrRecv))
}

// PatternChecker checks a file against a pattern database
type PatternChecker struct {
	db         *PatternDatabase
	collector  *PatternCollector
	violations []Violation
}

type Violation struct {
	Pattern  Pattern
	Location string
	Message  string
}

func NewPatternChecker(db *PatternDatabase) *PatternChecker {
	return &PatternChecker{
		db:        db,
		collector: NewPatternCollector(),
	}
}

func (pch *PatternChecker) CheckPackage(dir string, verbose bool) ([]Violation, error) {
	if err := pch.collector.ProcessPackage(dir, verbose); err != nil {
		return nil, err
	}

	var violations []Violation
	for _, p := range pch.collector.GetPatterns() {
		if !pch.db.HasPattern(p) {
			violations = append(violations, Violation{
				Pattern:  p,
				Location: p.Example,
				Message:  fmt.Sprintf("unsupported construct: %s", p.Key()),
			})
		}
	}

	return violations, nil
}

// generatePatternsGo generates a Go source file with compiled patterns
func generatePatternsGo(db *PatternDatabase, outputFile string) error {
	var sb strings.Builder

	sb.WriteString(`// Code generated by genrules. DO NOT EDIT.
package compiler

// GetBuiltinPatterns returns the patterns compiled from tests/examples
func GetBuiltinPatterns() map[string]Pattern {
	return map[string]Pattern{
`)

	// Sort pattern keys for consistent output
	var keys []string
	for k := range db.Patterns {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		p := db.Patterns[key]
		// Write each pattern
		sb.WriteString(fmt.Sprintf("\t\t%q: {\n", key))
		sb.WriteString(fmt.Sprintf("\t\t\tKind: %q,\n", p.Kind))
		sb.WriteString("\t\t\tAttrs: map[string]string{\n")

		// Sort attrs for consistent output
		var attrKeys []string
		for ak := range p.Attrs {
			attrKeys = append(attrKeys, ak)
		}
		sort.Strings(attrKeys)

		for _, ak := range attrKeys {
			sb.WriteString(fmt.Sprintf("\t\t\t\t%q: %q,\n", ak, p.Attrs[ak]))
		}
		sb.WriteString("\t\t\t},\n")
		sb.WriteString(fmt.Sprintf("\t\t\tExample: %q,\n", p.Example))
		sb.WriteString("\t\t},\n")
	}

	sb.WriteString(`	}
}
`)

	return os.WriteFile(outputFile, []byte(sb.String()), 0644)
}

func main() {
	var (
		collect     = flag.Bool("collect", false, "Collect patterns from tests and examples")
		check       = flag.String("check", "", "Check a directory against pattern database")
		report      = flag.Bool("report", false, "Generate technical report of patterns")
		doc         = flag.Bool("doc", false, "Generate human-readable language reference")
		outputFile  = flag.String("output", "", "Output file for patterns (JSON)")
		outputGo    = flag.String("output-go", "", "Output file for patterns (Go source)")
		patternsDB  = flag.String("patterns", "patterns.json", "Pattern database file")
		verbose     = flag.Bool("v", false, "Verbose output")
		projectRoot = flag.String("root", ".", "Project root directory")
	)
	flag.Parse()

	if *collect {
		collector := NewPatternCollector()

		// Process tests directory
		testsDir := filepath.Join(*projectRoot, "tests")
		if _, err := os.Stat(testsDir); err == nil {
			if *verbose {
				fmt.Printf("Processing %s...\n", testsDir)
			}
			if err := collector.ProcessPackage(testsDir, *verbose); err != nil {
				fmt.Fprintf(os.Stderr, "Warning processing tests: %v\n", err)
			}
		}

		// Process examples directory
		examplesDir := filepath.Join(*projectRoot, "examples")
		if _, err := os.Stat(examplesDir); err == nil {
			if *verbose {
				fmt.Printf("Processing %s...\n", examplesDir)
			}
			if err := collector.ProcessPackage(examplesDir, *verbose); err != nil {
				fmt.Fprintf(os.Stderr, "Warning processing examples: %v\n", err)
			}
		}

		if *verbose {
			collector.PrintSummary()
		}

		// Build database
		db := &PatternDatabase{
			Patterns: collector.patterns,
		}

		// Output Go source file if requested
		if *outputGo != "" {
			if err := generatePatternsGo(db, *outputGo); err != nil {
				fmt.Fprintf(os.Stderr, "Error generating Go patterns: %v\n", err)
				os.Exit(1)
			}
			if *verbose {
				fmt.Printf("Generated Go patterns to %s\n", *outputGo)
			}
		}

		// Output JSON file if requested
		if *outputFile != "" {
			if err := db.Save(*outputFile); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving patterns: %v\n", err)
				os.Exit(1)
			}
			if *verbose {
				fmt.Printf("Saved patterns to %s\n", *outputFile)
			}
		}

		// If neither output specified, print JSON to stdout
		if *outputFile == "" && *outputGo == "" {
			data, _ := json.MarshalIndent(db, "", "  ")
			fmt.Println(string(data))
		}

	} else if *report {
		db, err := LoadPatternDatabase(*patternsDB)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading pattern database: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(db.GenerateReport())

	} else if *doc {
		db, err := LoadPatternDatabase(*patternsDB)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading pattern database: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(db.GenerateDoc())

	} else if *check != "" {
		db, err := LoadPatternDatabase(*patternsDB)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading pattern database: %v\n", err)
			os.Exit(1)
		}

		checker := NewPatternChecker(db)
		violations, err := checker.CheckPackage(*check, *verbose)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error checking package: %v\n", err)
			os.Exit(1)
		}

		if len(violations) == 0 {
			fmt.Println("OK: all constructs are supported")
		} else {
			fmt.Printf("Found %d unsupported constructs:\n", len(violations))
			for _, v := range violations {
				fmt.Printf("  %s: %s\n", v.Location, v.Message)
			}
			os.Exit(1)
		}

	} else {
		flag.Usage()
		os.Exit(1)
	}
}
