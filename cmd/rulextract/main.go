// rulextract walks Go source files in tests/ and examples/ directories,
// extracts structural signatures of AST constructs, and reports them.
//
// Output formats:
//   -format text     (default) human-readable on screen
//   -format json     JSON array
//   -format markdown markdown table
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Rule represents a single extracted construct signature with its source location.
type Rule struct {
	Category   string `json:"category"`
	Signature  string `json:"signature"`
	File       string `json:"file"`
	Line       int    `json:"line"`
	SourceCode string `json:"source_code"`
}

// signature produces a structural description of an AST node,
// abstracting away concrete values while preserving shape.
func signature(fset *token.FileSet, node ast.Node) string {
	if node == nil {
		return "_"
	}
	switch n := node.(type) {
	// ---- Literals & identifiers ----
	case *ast.Ident:
		return "ident"
	case *ast.BasicLit:
		switch n.Kind {
		case token.INT:
			return "int"
		case token.FLOAT:
			return "float"
		case token.STRING:
			return "string"
		case token.CHAR:
			return "char"
		default:
			return strings.ToLower(n.Kind.String())
		}

	// ---- Expressions ----
	case *ast.BinaryExpr:
		return signature(fset, n.X) + " " + n.Op.String() + " " + signature(fset, n.Y)
	case *ast.UnaryExpr:
		return n.Op.String() + signature(fset, n.X)
	case *ast.ParenExpr:
		return "(" + signature(fset, n.X) + ")"
	case *ast.CallExpr:
		args := make([]string, len(n.Args))
		for i, a := range n.Args {
			args[i] = signature(fset, a)
		}
		return signature(fset, n.Fun) + "(" + strings.Join(args, ", ") + ")"
	case *ast.SelectorExpr:
		return signature(fset, n.X) + ".field"
	case *ast.IndexExpr:
		return signature(fset, n.X) + "[" + signature(fset, n.Index) + "]"
	case *ast.SliceExpr:
		low := signature(fset, n.Low)
		high := signature(fset, n.High)
		if n.Max != nil {
			return signature(fset, n.X) + "[" + low + ":" + high + ":" + signature(fset, n.Max) + "]"
		}
		return signature(fset, n.X) + "[" + low + ":" + high + "]"
	case *ast.TypeAssertExpr:
		if n.Type == nil {
			return signature(fset, n.X) + ".(type)"
		}
		return signature(fset, n.X) + ".(" + signature(fset, n.Type) + ")"
	case *ast.KeyValueExpr:
		return signature(fset, n.Key) + ": " + signature(fset, n.Value)
	case *ast.StarExpr:
		return "*" + signature(fset, n.X)
	case *ast.CompositeLit:
		elts := make([]string, len(n.Elts))
		for i, e := range n.Elts {
			elts[i] = signature(fset, e)
		}
		if n.Type != nil {
			return signature(fset, n.Type) + "{" + strings.Join(elts, ", ") + "}"
		}
		return "{" + strings.Join(elts, ", ") + "}"
	case *ast.FuncLit:
		return "func" + signature(fset, n.Type)

	// ---- Type expressions ----
	case *ast.ArrayType:
		if n.Len != nil {
			return "[" + signature(fset, n.Len) + "]" + signature(fset, n.Elt)
		}
		return "[]" + signature(fset, n.Elt)
	case *ast.MapType:
		return "map[" + signature(fset, n.Key) + "]" + signature(fset, n.Value)
	case *ast.StructType:
		return "struct{...}"
	case *ast.InterfaceType:
		if n.Methods == nil || len(n.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{...}"
	case *ast.FuncType:
		params := fieldListSig(fset, n.Params)
		results := fieldListSig(fset, n.Results)
		if results != "" {
			return "(" + params + ") " + results
		}
		return "(" + params + ")"
	case *ast.ChanType:
		return "chan " + signature(fset, n.Value)
	case *ast.Ellipsis:
		return "..." + signature(fset, n.Elt)

	// ---- Statements ----
	case *ast.AssignStmt:
		lhs := exprListSig(fset, n.Lhs)
		rhs := exprListSig(fset, n.Rhs)
		return lhs + " " + n.Tok.String() + " " + rhs
	case *ast.IncDecStmt:
		return signature(fset, n.X) + n.Tok.String()
	case *ast.ReturnStmt:
		if len(n.Results) == 0 {
			return "return"
		}
		return "return " + exprListSig(fset, n.Results)
	case *ast.ExprStmt:
		return signature(fset, n.X)
	case *ast.BranchStmt:
		if n.Label != nil {
			return n.Tok.String() + " label"
		}
		return n.Tok.String()
	case *ast.EmptyStmt:
		return "_"

	default:
		return fmt.Sprintf("<%T>", node)
	}
}

func exprListSig(fset *token.FileSet, exprs []ast.Expr) string {
	parts := make([]string, len(exprs))
	for i, e := range exprs {
		parts[i] = signature(fset, e)
	}
	return strings.Join(parts, ", ")
}

// shallowSig returns a one-level-deep description of an expression,
// without recursing into sub-expressions. Used for AssignStmt RHS, ReturnStmt, etc.
// where the full expression tree creates too many unique variants.
func shallowSig(fset *token.FileSet, node ast.Expr) string {
	if node == nil {
		return "_"
	}
	switch n := node.(type) {
	case *ast.Ident:
		return "ident"
	case *ast.BasicLit:
		switch n.Kind {
		case token.INT:
			return "int"
		case token.FLOAT:
			return "float"
		case token.STRING:
			return "string"
		case token.CHAR:
			return "char"
		default:
			return strings.ToLower(n.Kind.String())
		}
	case *ast.BinaryExpr:
		return "expr " + n.Op.String() + " expr"
	case *ast.UnaryExpr:
		return n.Op.String() + "expr"
	case *ast.CallExpr:
		return "call()"
	case *ast.SelectorExpr:
		return "ident.field"
	case *ast.IndexExpr:
		return "expr[expr]"
	case *ast.SliceExpr:
		return "expr[:]"
	case *ast.CompositeLit:
		if n.Type != nil {
			return signature(fset, n.Type) + "{...}"
		}
		return "{...}"
	case *ast.FuncLit:
		return "func(){...}"
	case *ast.TypeAssertExpr:
		if n.Type == nil {
			return "expr.(type)"
		}
		return "expr.(" + signature(fset, n.Type) + ")"
	case *ast.ParenExpr:
		return "(expr)"
	case *ast.StarExpr:
		return "*expr"
	case *ast.KeyValueExpr:
		return "key: value"
	default:
		return "expr"
	}
}

func shallowExprListSig(fset *token.FileSet, exprs []ast.Expr) string {
	parts := make([]string, len(exprs))
	for i, e := range exprs {
		parts[i] = shallowSig(fset, e)
	}
	return strings.Join(parts, ", ")
}

// argSig classifies a function argument at the highest level:
// ident, literal type, call, or just expr.
func argSig(fset *token.FileSet, node ast.Expr) string {
	if node == nil {
		return "_"
	}
	switch n := node.(type) {
	case *ast.Ident:
		return "ident"
	case *ast.BasicLit:
		switch n.Kind {
		case token.INT:
			return "int"
		case token.FLOAT:
			return "float"
		case token.STRING:
			return "string"
		case token.CHAR:
			return "char"
		default:
			return "literal"
		}
	case *ast.CallExpr:
		return "call()"
	case *ast.SelectorExpr:
		return "ident.field"
	case *ast.CompositeLit:
		if n.Type != nil {
			return signature(fset, n.Type) + "{...}"
		}
		return "{...}"
	case *ast.FuncLit:
		return "func(){...}"
	case *ast.UnaryExpr:
		return n.Op.String() + "expr"
	default:
		return "expr"
	}
}

func fieldListSig(fset *token.FileSet, fl *ast.FieldList) string {
	if fl == nil || len(fl.List) == 0 {
		return ""
	}
	parts := make([]string, len(fl.List))
	for i, f := range fl.List {
		parts[i] = signature(fset, f.Type)
	}
	return strings.Join(parts, ", ")
}

// fileLineCache caches file contents for source line lookups.
type fileLineCache struct {
	cache map[string][]string
}

func newFileLineCache() *fileLineCache {
	return &fileLineCache{cache: make(map[string][]string)}
}

func (c *fileLineCache) getLine(absPath string, lineNum int) string {
	lines, ok := c.cache[absPath]
	if !ok {
		data, err := os.ReadFile(absPath)
		if err != nil {
			return ""
		}
		lines = strings.Split(string(data), "\n")
		c.cache[absPath] = lines
	}
	if lineNum < 1 || lineNum > len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[lineNum-1])
}

// extractRules walks an AST file and collects rules for interesting constructs.
func extractRules(fset *token.FileSet, file *ast.File, filename string, absPath string, lineCache *fileLineCache) []Rule {
	var rules []Rule

	addRule := func(category, sig string, pos token.Pos) {
		p := fset.Position(pos)
		src := lineCache.getLine(absPath, p.Line)
		rules = append(rules, Rule{category, sig, filename, p.Line, src})
	}

	ast.Inspect(file, func(n ast.Node) bool {
		if n == nil {
			return false
		}

		switch node := n.(type) {
		case *ast.ForStmt:
			init := signature(fset, node.Init)
			cond := signature(fset, node.Cond)
			post := signature(fset, node.Post)
			sig := "for " + init + "; " + cond + "; " + post
			addRule("ForStmt", sig, node.Pos())

		case *ast.RangeStmt:
			key := signature(fset, node.Key)
			value := signature(fset, node.Value)
			x := signature(fset, node.X)
			tok := node.Tok.String()
			if value == "_" {
				addRule("RangeStmt", "for "+key+" "+tok+" range "+x, node.Pos())
			} else {
				addRule("RangeStmt", "for "+key+", "+value+" "+tok+" range "+x, node.Pos())
			}

		case *ast.IfStmt:
			init := "_"
			if node.Init != nil {
				switch s := node.Init.(type) {
				case *ast.AssignStmt:
					init = shallowExprListSig(fset, s.Lhs) + " " + s.Tok.String() + " " + shallowExprListSig(fset, s.Rhs)
				default:
					init = "stmt"
				}
			}
			cond := shallowSig(fset, node.Cond)
			hasElse := "no-else"
			if node.Else != nil {
				switch node.Else.(type) {
				case *ast.IfStmt:
					hasElse = "else-if"
				case *ast.BlockStmt:
					hasElse = "else"
				}
			}
			var sig string
			if init == "_" {
				sig = "if " + cond + " [" + hasElse + "]"
			} else {
				sig = "if " + init + "; " + cond + " [" + hasElse + "]"
			}
			addRule("IfStmt", sig, node.Pos())

		case *ast.SwitchStmt:
			init := "_"
			if node.Init != nil {
				init = signature(fset, node.Init)
			}
			tag := "_"
			if node.Tag != nil {
				tag = signature(fset, node.Tag)
			}
			var sig string
			if init == "_" {
				sig = "switch " + tag
			} else {
				sig = "switch " + init + "; " + tag
			}
			addRule("SwitchStmt", sig, node.Pos())

		case *ast.TypeSwitchStmt:
			addRule("TypeSwitchStmt", "type switch", node.Pos())

		case *ast.SliceExpr:
			low := signature(fset, node.Low)
			high := signature(fset, node.High)
			x := typeDescr(fset, node.X)
			if node.Max != nil {
				max := signature(fset, node.Max)
				addRule("SliceExpr", x+"["+low+":"+high+":"+max+"]", node.Pos())
			} else {
				addRule("SliceExpr", x+"["+low+":"+high+"]", node.Pos())
			}

		case *ast.IndexExpr:
			x := typeDescr(fset, node.X)
			idx := signature(fset, node.Index)
			addRule("IndexExpr", x+"["+idx+"]", node.Pos())

		case *ast.AssignStmt:
			lhsParts := make([]string, len(node.Lhs))
			for i, e := range node.Lhs {
				lhsParts[i] = shallowSig(fset, e)
			}
			rhsParts := make([]string, len(node.Rhs))
			for i, e := range node.Rhs {
				rhsParts[i] = argSig(fset, e)
			}
			sig := strings.Join(lhsParts, ", ") + " " + node.Tok.String() + " " + strings.Join(rhsParts, ", ")
			addRule("AssignStmt", sig, node.Pos())

		case *ast.ReturnStmt:
			if len(node.Results) == 0 {
				addRule("ReturnStmt", "return", node.Pos())
			} else {
				parts := make([]string, len(node.Results))
				for i, e := range node.Results {
					parts[i] = argSig(fset, e)
				}
				addRule("ReturnStmt", "return "+strings.Join(parts, ", "), node.Pos())
			}

		case *ast.BranchStmt:
			addRule("BranchStmt", signature(fset, node), node.Pos())

		case *ast.CallExpr:
			fun := shallowSig(fset, node.Fun)
			args := make([]string, len(node.Args))
			for i, a := range node.Args {
				args[i] = argSig(fset, a)
			}
			addRule("CallExpr", fun+"("+strings.Join(args, ", ")+")", node.Pos())

		case *ast.FuncDecl:
			recv := ""
			if node.Recv != nil && len(node.Recv.List) > 0 {
				recv = "(" + signature(fset, node.Recv.List[0].Type) + ") "
			}
			params := fieldListSig(fset, node.Type.Params)
			results := fieldListSig(fset, node.Type.Results)
			var sig string
			if results != "" {
				sig = "func " + recv + "ident(" + params + ") " + results
			} else {
				sig = "func " + recv + "ident(" + params + ")"
			}
			addRule("FuncDecl", sig, node.Pos())

		case *ast.FuncLit:
			params := fieldListSig(fset, node.Type.Params)
			results := fieldListSig(fset, node.Type.Results)
			var sig string
			if results != "" {
				sig = "func(" + params + ") " + results
			} else {
				sig = "func(" + params + ")"
			}
			addRule("FuncLit", sig, node.Pos())

		case *ast.GenDecl:
			switch node.Tok {
			case token.CONST:
				for _, spec := range node.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					typeSig := "_"
					if vs.Type != nil {
						typeSig = signature(fset, vs.Type)
					}
					valSig := "_"
					if len(vs.Values) > 0 {
						valSig = exprListSig(fset, vs.Values)
					}
					addRule("ConstDecl", "const ident "+typeSig+" = "+valSig, vs.Pos())
				}
			case token.VAR:
				for _, spec := range node.Specs {
					vs, ok := spec.(*ast.ValueSpec)
					if !ok {
						continue
					}
					typeSig := "_"
					if vs.Type != nil {
						typeSig = signature(fset, vs.Type)
					}
					valSig := "_"
					if len(vs.Values) > 0 {
						valSig = exprListSig(fset, vs.Values)
					}
					addRule("VarDecl", "var ident "+typeSig+" = "+valSig, vs.Pos())
				}
			case token.TYPE:
				for _, spec := range node.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					addRule("TypeDecl", "type ident "+signature(fset, ts.Type), ts.Pos())
				}
			}

		case *ast.CompositeLit:
			typeSig := "_"
			if node.Type != nil {
				typeSig = signature(fset, node.Type)
			}
			nElts := len(node.Elts)
			hasKV := false
			if nElts > 0 {
				if _, ok := node.Elts[0].(*ast.KeyValueExpr); ok {
					hasKV = true
				}
			}
			style := "positional"
			if hasKV {
				style = "keyed"
			}
			if nElts == 0 {
				style = "empty"
			}
			addRule("CompositeLit", typeSig+"{"+style+"}", node.Pos())

		case *ast.TypeAssertExpr:
			if node.Type == nil {
				addRule("TypeAssert", "ident.(type)", node.Pos())
			} else {
				addRule("TypeAssert", "ident.("+signature(fset, node.Type)+")", node.Pos())
			}

		case *ast.GoStmt:
			addRule("GoStmt", "go "+signature(fset, node.Call), node.Pos())
		case *ast.DeferStmt:
			addRule("DeferStmt", "defer "+signature(fset, node.Call), node.Pos())
		case *ast.SelectStmt:
			addRule("SelectStmt", "select", node.Pos())
		case *ast.SendStmt:
			addRule("SendStmt", signature(fset, node.Chan)+" <- "+signature(fset, node.Value), node.Pos())

		case *ast.MapType:
			addRule("MapType", "map["+signature(fset, node.Key)+"]"+signature(fset, node.Value), node.Pos())

		case *ast.ArrayType:
			if node.Len != nil {
				addRule("ArrayType", "["+signature(fset, node.Len)+"]"+signature(fset, node.Elt), node.Pos())
			} else {
				addRule("SliceType", "[]"+signature(fset, node.Elt), node.Pos())
			}

		case *ast.InterfaceType:
			if node.Methods == nil || len(node.Methods.List) == 0 {
				addRule("InterfaceType", "interface{}", node.Pos())
			} else {
				addRule("InterfaceType", "interface{...}", node.Pos())
			}

		case *ast.IncDecStmt:
			addRule("IncDecStmt", signature(fset, node), node.Pos())

		case *ast.UnaryExpr:
			addRule("UnaryExpr", node.Op.String()+typeDescr(fset, node.X), node.Pos())

		case *ast.BinaryExpr:
			addRule("BinaryExpr", "expr "+node.Op.String()+" expr", node.Pos())
		}

		return true
	})

	return rules
}

// typeDescr gives a short type description for the expression (ident, selector, index, etc.)
func typeDescr(fset *token.FileSet, expr ast.Expr) string {
	if expr == nil {
		return "_"
	}
	switch expr.(type) {
	case *ast.Ident:
		return "ident"
	case *ast.SelectorExpr:
		return "ident.field"
	case *ast.IndexExpr:
		return "ident[expr]"
	case *ast.CallExpr:
		return "call"
	case *ast.SliceExpr:
		return "slice"
	default:
		return "expr"
	}
}

func collectGoFiles(root string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if info.IsDir() {
			base := filepath.Base(path)
			// skip hidden dirs, vendor, build artifacts
			if strings.HasPrefix(base, ".") || base == "vendor" || base == "build" || base == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func makeRelative(path, root string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

// dedup returns unique signatures per category, keeping the first occurrence.
type location struct {
	Loc    string // file:line
	Source string // actual source code
}

type uniqueRule struct {
	Category  string
	Signature string
	Locs      []location // all locations with source
}

func dedup(rules []Rule) []uniqueRule {
	type key struct {
		cat string
		sig string
	}
	seen := make(map[key]*uniqueRule)
	var order []key
	for _, r := range rules {
		k := key{r.Category, r.Signature}
		loc := location{
			Loc:    fmt.Sprintf("%s:%d", r.File, r.Line),
			Source: r.SourceCode,
		}
		if u, ok := seen[k]; ok {
			u.Locs = append(u.Locs, loc)
		} else {
			u := &uniqueRule{r.Category, r.Signature, []location{loc}}
			seen[k] = u
			order = append(order, k)
		}
	}
	// sort by category then signature
	sort.Slice(order, func(i, j int) bool {
		if order[i].cat != order[j].cat {
			return order[i].cat < order[j].cat
		}
		return order[i].sig < order[j].sig
	})
	result := make([]uniqueRule, len(order))
	for i, k := range order {
		result[i] = *seen[k]
	}
	return result
}

func outputText(rules []uniqueRule) {
	currentCat := ""
	for _, r := range rules {
		if r.Category != currentCat {
			if currentCat != "" {
				fmt.Println()
			}
			fmt.Printf("=== %s ===\n", r.Category)
			currentCat = r.Category
		}
		fmt.Printf("  %s\n", r.Signature)
		if len(r.Locs) <= 3 {
			for _, l := range r.Locs {
				fmt.Printf("    -> %s\n", l.Loc)
				if l.Source != "" {
					fmt.Printf("       %s\n", l.Source)
				}
			}
		} else {
			for _, l := range r.Locs[:2] {
				fmt.Printf("    -> %s\n", l.Loc)
				if l.Source != "" {
					fmt.Printf("       %s\n", l.Source)
				}
			}
			fmt.Printf("    -> ... and %d more\n", len(r.Locs)-2)
		}
	}
}

func outputJSON(rules []uniqueRule) {
	type jsonLoc struct {
		Location string `json:"location"`
		Source   string `json:"source"`
	}
	type jsonRule struct {
		Category  string    `json:"category"`
		Signature string    `json:"signature"`
		Count     int       `json:"count"`
		Locations []jsonLoc `json:"locations"`
	}
	out := make([]jsonRule, len(rules))
	for i, r := range rules {
		locs := make([]jsonLoc, len(r.Locs))
		for j, l := range r.Locs {
			locs[j] = jsonLoc{l.Loc, l.Source}
		}
		out[i] = jsonRule{r.Category, r.Signature, len(r.Locs), locs}
	}
	data, _ := json.MarshalIndent(out, "", "  ")
	fmt.Println(string(data))
}

func outputMarkdown(rules []uniqueRule) {
	fmt.Println("# Supported Construct Variants")
	fmt.Println()
	fmt.Println("Extracted from tests and examples.")
	fmt.Println()

	currentCat := ""
	for _, r := range rules {
		if r.Category != currentCat {
			if currentCat != "" {
				fmt.Println()
			}
			fmt.Printf("## %s\n\n", r.Category)
			fmt.Println("| Signature | Count | Example Location | Source |")
			fmt.Println("|-----------|-------|------------------|--------|")
			currentCat = r.Category
		}
		loc := r.Locs[0]
		src := loc.Source
		if len(src) > 80 {
			src = src[:77] + "..."
		}
		fmt.Printf("| `%s` | %d | %s | `%s` |\n", r.Signature, len(r.Locs), loc.Loc, src)
	}
}

func main() {
	var format string
	var root string
	flag.StringVar(&format, "format", "text", "Output format: text, json, markdown")
	flag.StringVar(&root, "root", "", "Project root directory (default: auto-detect)")
	flag.Parse()

	// Auto-detect root: look for go.mod in parent directories
	if root == "" {
		// Default: assume we're in cmd/rulextract, go up to project root
		wd, _ := os.Getwd()
		root = wd
		for {
			if _, err := os.Stat(filepath.Join(root, "tests")); err == nil {
				break
			}
			parent := filepath.Dir(root)
			if parent == root {
				fmt.Fprintln(os.Stderr, "Cannot find project root (looking for tests/ directory)")
				os.Exit(1)
			}
			root = parent
		}
	}

	dirs := []string{
		filepath.Join(root, "tests"),
		filepath.Join(root, "examples"),
	}

	var allRules []Rule
	fset := token.NewFileSet()
	lineCache := newFileLineCache()

	for _, dir := range dirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}
		files, err := collectGoFiles(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error walking %s: %v\n", dir, err)
			continue
		}
		for _, f := range files {
			parsed, err := parser.ParseFile(fset, f, nil, parser.ParseComments)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Parse error %s: %v\n", f, err)
				continue
			}
			relPath := makeRelative(f, root)
			rules := extractRules(fset, parsed, relPath, f, lineCache)
			allRules = append(allRules, rules...)
		}
	}

	unique := dedup(allRules)

	switch format {
	case "json":
		outputJSON(unique)
	case "markdown":
		outputMarkdown(unique)
	default:
		outputText(unique)
	}
}
