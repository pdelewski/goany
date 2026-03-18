package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	goanyrt "goany/runtime"

	"golang.org/x/tools/go/packages"
)

// JSEmitter implements the Emitter interface using a shift/reduce architecture.
// PreVisit methods push markers, PostVisit methods reduce and push results.
type JSEmitter struct {
	fs              *IRForestBuilder
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string
	RuntimePackages map[string]string
	file            *os.File
	Emitter
	pkg            *packages.Package
	currentPackage string
	inNamespace    bool
	indent         int
	numFuncResults int
	rangeVarCounter  int
	// Minimal state for map assignment detection
	lastIndexXCode   string
	lastIndexKeyCode string
	mapAssignVar     string
	mapAssignKey     string
	structKeyTypes   map[string]bool
	// For loop components (stacks for nesting support)
	forInitStack []string
	forCondStack []string
	forPostStack []string
	// If statement components (stacks for nesting support)
	ifInitStack []string
	ifCondStack []string
	ifBodyStack []string
	ifElseStack []string
}

func (e *JSEmitter) SetFile(file *os.File) { e.file = file }
func (e *JSEmitter) GetFile() *os.File     { return e.file }

// indentStr returns indentation string for the given level.
func jsIndent(indent int) string {
	return strings.Repeat("  ", indent)
}

// jsDefaultForGoType returns JS default value for a Go type.
func jsDefaultForGoType(t types.Type) string {
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
		return "[]"
	case *types.Map:
		return "null"
	case *types.Struct:
		// If the original type is named, produce new StructName()
		if named, ok := t.(*types.Named); ok {
			return fmt.Sprintf("new %s()", named.Obj().Name())
		}
		return "null"
	}
	return "null"
}

// jsMapKeyTypeConst returns the key type constant for a map's key type.
func jsMapKeyTypeConst(t *types.Map) int {
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

// lowerBuiltin maps Go stdlib selectors to JS equivalents.
func jsLowerBuiltin(selector string) string {
	switch selector {
	case "fmt":
		return ""
	case "Sprintf":
		return "stringFormat"
	case "Println":
		return "console.log"
	case "Printf":
		return "printf"
	case "Print":
		return "print"
	case "len":
		return "len"
	case "panic":
		return "goany_panic"
	}
	return selector
}

// isMapTypeExpr checks if an expression has map type via TypesInfo.
func (e *JSEmitter) isMapTypeExpr(expr ast.Expr) bool {
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
func (e *JSEmitter) getExprGoType(expr ast.Expr) types.Type {
	if e.pkg == nil || e.pkg.TypesInfo == nil {
		return nil
	}
	tv := e.pkg.TypesInfo.Types[expr]
	return tv.Type
}

// jsGraphicsRuntime is the inline Canvas-based graphics runtime for browser JS.
var jsGraphicsRuntime = `// Graphics runtime for Canvas
class Color {
  constructor(R = 0, G = 0, B = 0, A = 0) {
    this.R = R; this.G = G; this.B = B; this.A = A;
  }
}
class Rect {
  constructor(x = 0, y = 0, width = 0, height = 0) {
    this.x = x; this.y = y; this.width = width; this.height = height;
  }
}
const graphics = {
  Color: Color,
  Rect: Rect,
  canvas: null,
  ctx: null,
  running: true,
  keys: {},
  lastKey: 0,
  mouseX: 0,
  mouseY: 0,
  mouseDown: false,

  _setupEventListeners: function() {
    window.addEventListener('keydown', (e) => {
      this.keys[e.key] = true;
      if (e.key.length === 1) {
        this.lastKey = e.key.charCodeAt(0);
      } else {
        const specialKeys = {
          'Enter': 13, 'Backspace': 8, 'Tab': 9, 'Escape': 27,
          'ArrowUp': 38, 'ArrowDown': 40, 'ArrowLeft': 37, 'ArrowRight': 39,
          'Delete': 127, 'Space': 32
        };
        if (specialKeys[e.key]) { this.lastKey = specialKeys[e.key]; }
      }
    });
    window.addEventListener('keyup', (e) => { this.keys[e.key] = false; });
    this.canvas.addEventListener('mousemove', (e) => {
      const rect = this.canvas.getBoundingClientRect();
      this.mouseX = e.clientX - rect.left;
      this.mouseY = e.clientY - rect.top;
    });
    this.canvas.addEventListener('mousedown', () => { this.mouseDown = true; });
    this.canvas.addEventListener('mouseup', () => { this.mouseDown = false; });
  },

  CreateWindow: function(title, width, height) {
    this.canvas = document.createElement('canvas');
    this.canvas.width = width;
    this.canvas.height = height;
    this.ctx = this.canvas.getContext('2d');
    document.body.appendChild(this.canvas);
    document.title = title;
    this.windowObj = { canvas: this.canvas, width: width, height: height };
    this._setupEventListeners();
    return this.windowObj;
  },

  CreateWindowFullscreen: function(title, width, height) {
    document.body.style.margin = '0';
    document.body.style.padding = '0';
    document.body.style.overflow = 'hidden';
    this.canvas = document.createElement('canvas');
    this.canvas.width = window.innerWidth;
    this.canvas.height = window.innerHeight;
    this.canvas.style.display = 'block';
    this.ctx = this.canvas.getContext('2d');
    document.body.appendChild(this.canvas);
    document.title = title;
    this.windowObj = { canvas: this.canvas, width: this.canvas.width, height: this.canvas.height };
    window.addEventListener('resize', () => {
      this.canvas.width = window.innerWidth;
      this.canvas.height = window.innerHeight;
      this.windowObj.width = this.canvas.width;
      this.windowObj.height = this.canvas.height;
    });
    this._setupEventListeners();
    return this.windowObj;
  },

  NewColor: function(r, g, b, a) {
    return { R: r, G: g, B: b, A: a !== undefined ? a : 255 };
  },

  Red: function() { return { R: 255, G: 0, B: 0, A: 255 }; },
  Green: function() { return { R: 0, G: 255, B: 0, A: 255 }; },
  Blue: function() { return { R: 0, G: 0, B: 255, A: 255 }; },
  White: function() { return { R: 255, G: 255, B: 255, A: 255 }; },
  Black: function() { return { R: 0, G: 0, B: 0, A: 255 }; },

` + "  Clear: function(canvas, color) {\n    this.ctx.fillStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;\n    this.ctx.fillRect(0, 0, canvas.width, canvas.height);\n  },\n\n" +
	"  FillRect: function(canvas, rect, color) {\n    this.ctx.fillStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;\n    this.ctx.fillRect(rect.x, rect.y, rect.width, rect.height);\n  },\n\n" +
	"  DrawRect: function(canvas, rect, color) {\n    this.ctx.strokeStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;\n    this.ctx.strokeRect(rect.x, rect.y, rect.width, rect.height);\n  },\n\n" +
	`  NewRect: function(x, y, width, height) {
    return { x, y, width, height };
  },

` + "  FillCircle: function(canvas, centerX, centerY, radius, color) {\n    this.ctx.fillStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;\n    this.ctx.beginPath();\n    this.ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);\n    this.ctx.fill();\n  },\n\n" +
	"  DrawCircle: function(canvas, centerX, centerY, radius, color) {\n    this.ctx.strokeStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;\n    this.ctx.beginPath();\n    this.ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);\n    this.ctx.stroke();\n  },\n\n" +
	"  DrawPoint: function(canvas, x, y, color) {\n    this.ctx.fillStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;\n    this.ctx.fillRect(x, y, 1, 1);\n  },\n\n" +
	"  DrawLine: function(canvas, x1, y1, x2, y2, color) {\n    this.ctx.strokeStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;\n    this.ctx.beginPath();\n    this.ctx.moveTo(x1, y1);\n    this.ctx.lineTo(x2, y2);\n    this.ctx.stroke();\n  },\n\n" +
	"  SetPixel: function(canvas, x, y, color) {\n    this.ctx.fillStyle = `rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`;\n    this.ctx.fillRect(x, y, 1, 1);\n  },\n\n" +
	`  PollEvents: function(canvas) {
    return [canvas, this.running];
  },

  Update: function(canvas) {
    // Canvas updates automatically
  },

  KeyDown: function(canvas, key) {
    return this.keys[key] || false;
  },

  GetLastKey: function() {
    const key = this.lastKey;
    this.lastKey = 0;
    return key;
  },

  GetMousePos: function(canvas) {
    return [this.mouseX, this.mouseY];
  },

  GetMouse: function(canvas) {
    return [this.mouseX, this.mouseY, this.mouseDown ? 1 : 0];
  },

  GetWidth: function(w) {
    return w.width;
  },

  GetHeight: function(w) {
    return w.height;
  },

  GetScreenSize: function() {
    return [window.screen.width, window.screen.height];
  },

  MouseDown: function(canvas) {
    return this.mouseDown;
  },

  Closed: function(canvas) {
    return !this.running;
  },

  Free: function(canvas) {
    if (canvas && canvas.parentNode) {
      canvas.parentNode.removeChild(canvas);
    }
  },

  Present: function(canvas) {
    // Canvas updates automatically, no-op
  },

  CloseWindow: function(canvas) {
    // In browser context, don't immediately close
  },

  RunLoop: function(canvas, frameFunc) {
    const self = this;
    function loop() {
      if (!self.running) return;
      const result = frameFunc(canvas);
      if (result === false) { self.running = false; return; }
      requestAnimationFrame(loop);
    }
    requestAnimationFrame(loop);
  },

  RunLoopWithState: function(canvas, state, frameFunc) {
    const self = this;
    function loop() {
      if (!self.running) return;
      const result = frameFunc(canvas, state);
      state = result[0];
      if (result[1] === false) { self.running = false; return; }
      requestAnimationFrame(loop);
    }
    requestAnimationFrame(loop);
  }
};

`

// ============================================================
// Program / Package
// ============================================================

func (e *JSEmitter) PreVisitProgram(indent int) {
	var err error
	e.file, err = os.Create(e.Output)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}

	e.fs = NewIRForestBuilder(e.GetGoFIR())

	// Write JS header with runtime helpers
	e.file.WriteString(`// Generated JavaScript code
"use strict";

// Runtime helpers
function len(arr) {
  if (typeof arr === 'string') return arr.length;
  if (Array.isArray(arr)) return arr.length;
  return 0;
}

function append(arr, ...items) {
  if (arr == null) arr = [];
  for (const item of items) {
    if (item && typeof item === 'object' && !Array.isArray(item)) {
      arr.push({...item});
    } else {
      arr.push(item);
    }
  }
  return arr;
}

function stringFormat(fmt, ...args) {
  let i = 0;
  return fmt.replace(/%[sdvfxc%]/g, (match) => {
    if (match === '%%') return '%';
    if (i >= args.length) return match;
    const arg = args[i++];
    switch (match) {
      case '%s': return String(arg);
      case '%d': return parseInt(arg, 10);
      case '%f': return parseFloat(arg);
      case '%v': return String(arg);
      case '%x': return parseInt(arg, 10).toString(16);
      case '%c': return String.fromCharCode(arg);
      default: return arg;
    }
  });
}

function printf(fmt, ...args) {
  const str = stringFormat(fmt, ...args);
  if (typeof process !== 'undefined' && process.stdout) {
    process.stdout.write(str);
  } else {
    if (typeof window !== 'undefined') {
      window._printBuffer = (window._printBuffer || '') + str;
    }
  }
}

function print(...args) {
  const str = args.map(a => String(a)).join(' ');
  if (typeof process !== 'undefined' && process.stdout) {
    process.stdout.write(str);
  } else {
    if (typeof window !== 'undefined') {
      window._printBuffer = (window._printBuffer || '') + str;
    }
  }
}

function make(type, length, capacity) {
  if (Array.isArray(type)) {
    return new Array(length || 0).fill(type[0] === 'number' ? 0 : null);
  }
  return [];
}

// Type conversion functions
function int8(v) { return typeof v === 'string' ? v.charCodeAt(0) | 0 : v | 0; }
function int16(v) { return typeof v === 'string' ? v.charCodeAt(0) | 0 : v | 0; }
function int32(v) { return typeof v === 'string' ? v.charCodeAt(0) | 0 : v | 0; }
function int64(v) { return typeof v === 'string' ? v.charCodeAt(0) : v; }
function int(v) { return typeof v === 'string' ? v.charCodeAt(0) | 0 : v | 0; }
function uint8(v) { return typeof v === 'string' ? v.charCodeAt(0) & 0xFF : (v | 0) & 0xFF; }
function uint16(v) { return typeof v === 'string' ? v.charCodeAt(0) & 0xFFFF : (v | 0) & 0xFFFF; }
function uint32(v) { return typeof v === 'string' ? v.charCodeAt(0) >>> 0 : (v | 0) >>> 0; }
function uint64(v) { return typeof v === 'string' ? v.charCodeAt(0) : v; }
function float32(v) { return v; }
function float64(v) { return v; }
function string(v) { return String(v); }
function bool(v) { return Boolean(v); }

`)
	// Include panic runtime
	e.file.WriteString("// GoAny panic runtime\n")
	e.file.WriteString(goanyrt.PanicJsSource)
	e.file.WriteString("\n")

	// Include graphics runtime if graphics package is used
	if _, hasGraphics := e.RuntimePackages["graphics"]; hasGraphics {
		e.file.WriteString(jsGraphicsRuntime)
	}

	// Include runtime packages from files (convention: X/js/X_runtime.js)
	if e.LinkRuntime != "" {
		for name, variant := range e.RuntimePackages {
			if name == "graphics" || variant == "none" {
				continue
			}
			runtimePath := filepath.Join(e.LinkRuntime, name, "js", name+"_runtime.js")
			content, err := os.ReadFile(runtimePath)
			if err != nil {
				DebugLogPrintf("Skipping JsPrim runtime for %s: %v", name, err)
				continue
			}
			e.file.WriteString(fmt.Sprintf("// %s runtime\n", name))
			e.file.Write(content)
			e.file.WriteString("\n")
		}
	}
}

func (e *JSEmitter) PostVisitProgram(indent int) {
	// Reduce everything from program marker
	tokens := e.fs.CollectForest(string(PreVisitProgram))
	// Write all accumulated code
	for _, t := range tokens {
		e.file.WriteString(t.Serialize())
	}
	// Add main() call at the end
	e.file.WriteString("\n// Run main\nmain();\n")
	e.file.Close()

	// Replace placeholder struct key functions with working implementations
	if len(e.structKeyTypes) > 0 {
		e.replaceStructKeyFunctions()
	}

	// Create HTML wrapper if graphics runtime is enabled
	if _, hasGraphics := e.RuntimePackages["graphics"]; hasGraphics {
		e.createHTMLWrapper()
	}

	// Auto-install npm dependencies if needed
	e.ensureNpmDependencies()
}

// createHTMLWrapper generates an HTML file that loads the generated JS.
func (e *JSEmitter) createHTMLWrapper() {
	htmlFile := strings.TrimSuffix(e.Output, ".js") + ".html"
	f, err := os.Create(htmlFile)
	if err != nil {
		log.Printf("Warning: could not create HTML wrapper: %v", err)
		return
	}
	defer f.Close()

	jsFileName := filepath.Base(e.Output)
	f.WriteString(fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>%s</title>
  <style>
    body { margin: 0; display: flex; justify-content: center; align-items: center; min-height: 100vh; background: #1a1a1a; }
    canvas { border: 1px solid #333; }
  </style>
</head>
<body>
  <script src="%s"></script>
</body>
</html>
`, e.OutputName, jsFileName))
}

// replaceStructKeyFunctions replaces placeholder hash/equality functions for struct keys
func (e *JSEmitter) replaceStructKeyFunctions() {
	content, err := os.ReadFile(e.Output)
	if err != nil {
		log.Printf("Warning: could not read file for struct key replacement: %v", err)
		return
	}

	newContent := string(content)

	// Replace hashStructKey function (handles both ES6 method shorthand and function format)
	hashPattern := regexp.MustCompile(`(?s)hashStructKey\s*\(key\)\s*\{\s*return 0;\s*\}`)
	newHashBody := `hashStructKey(key) {
  let s = JSON.stringify(key);
  let h = 0;
  for (let i = 0; i < s.length; i++) {
    h = (h * 31 + s.charCodeAt(i)) | 0;
  }
  if (h < 0) h = -h;
  return h;
}`
	newContent = hashPattern.ReplaceAllString(newContent, newHashBody)

	// Replace structKeysEqual function
	equalPattern := regexp.MustCompile(`(?s)structKeysEqual\s*\(a,\s*b\)\s*\{\s*return false;\s*\}`)
	newEqualBody := `structKeysEqual(a, b) {
  return JSON.stringify(a) === JSON.stringify(b);
}`
	newContent = equalPattern.ReplaceAllString(newContent, newEqualBody)

	if err := os.WriteFile(e.Output, []byte(newContent), 0644); err != nil {
		log.Printf("Warning: could not write struct key replacements: %v", err)
	}
}

func (e *JSEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	e.pkg = pkg
	e.currentPackage = pkg.Name
	if pkg.Name != "main" {
		e.inNamespace = true
		// Write namespace opener directly - structs will be written before it
	}
}

func (e *JSEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	if pkg.Name != "main" {
		e.fs.AddTree(IRTree(PackageDeclaration, KindDecl, Leaf(Identifier, "};\n")))
		e.inNamespace = false
	}
	// Don't reduce package marker - let tokens flow up to program
}

// ============================================================
// Literals and Identifiers
// ============================================================

func (e *JSEmitter) PreVisitBasicLit(node *ast.BasicLit, indent int) {
	val := node.Value
	// Convert Go raw strings (backticks) to JS template literals
	// Go raw strings preserve backslashes literally, but JS template literals
	// interpret escape sequences like \n. We need to escape backslashes.
	if node.Kind == token.STRING && len(val) > 1 && val[0] == '`' {
		content := val[1 : len(val)-1]
		escaped := strings.ReplaceAll(content, "\\", "\\\\")
		val = "`" + escaped + "`"
	}
	// Convert Go character literals ('a') to numeric values for JS
	// In Go, 'a' is a rune (int32 = 97). In JS, 'a' is a string.
	if node.Kind == token.CHAR && len(val) >= 3 && val[0] == '\'' {
		// Handle escape sequences
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
			// Fallback: use charCodeAt
			val = fmt.Sprintf("'%s'.charCodeAt(0)", inner)
		}
	}
	e.fs.AddLeaf(val, TagLiteral, nil)
}

func (e *JSEmitter) PreVisitIdent(node *ast.Ident, indent int) {
	name := node.Name
	// Map Go builtins
	switch name {
	case "true", "false":
		e.fs.AddLeaf(name, TagLiteral, nil)
		return
	case "nil":
		e.fs.AddLeaf("null", TagLiteral, nil)
		return
	case "string":
		e.fs.AddLeaf("string", TagType, nil)
		return
	}
	// Check if this is a reference to a package identifier
	if e.pkg != nil && e.pkg.TypesInfo != nil {
		if obj := e.pkg.TypesInfo.Uses[node]; obj != nil {
			if obj.Pkg() != nil && obj.Pkg().Name() != e.currentPackage {
				// Qualified name from another package
				name = obj.Pkg().Name() + "." + name
			} else if obj.Pkg() != nil && e.inNamespace && obj.Parent() == obj.Pkg().Scope() {
				// Same package, inside a namespace, and it's a package-level declaration
				// (not a local variable/parameter) — need prefix for resolution
				name = obj.Pkg().Name() + "." + name
			}
		}
	}
	goType := e.getExprGoType(node)
	e.fs.AddLeaf(name, TagIdent, goType)
}

// ============================================================
// Binary Expressions
// ============================================================

func (e *JSEmitter) PostVisitBinaryExprLeft(node ast.Expr, indent int) {
	left := e.fs.CollectText(string(PreVisitBinaryExprLeft))
	e.fs.AddLeaf(left, KindExpr, nil)
}

func (e *JSEmitter) PostVisitBinaryExprRight(node ast.Expr, indent int) {
	right := e.fs.CollectText(string(PreVisitBinaryExprRight))
	e.fs.AddLeaf(right, KindExpr, nil)
}

func (e *JSEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBinaryExpr))
	left := ""
	right := ""
	if len(tokens) >= 1 {
		left = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		right = tokens[1].Serialize()
	}
	op := node.Op.String()

	// Check for integer division
	if op == "/" {
		leftType := e.getExprGoType(node.X)
		if leftType != nil {
			if basic, ok := leftType.Underlying().(*types.Basic); ok {
				if basic.Info()&types.IsInteger != 0 {
					e.fs.AddTree(IRTree(BinaryExpression, KindExpr,
						Leaf(Identifier, "Math.trunc"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, left),
						Leaf(WhiteSpace, " "),
						Leaf(BinaryOperator, op),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, right),
						Leaf(RightParen, ")"),
					))
					return
				}
			}
		}
	}

	// Check for string indexing: str[i] in Go returns byte, use charCodeAt in JS
	e.fs.AddTree(IRTree(BinaryExpression, KindExpr,
		Leaf(Identifier, left),
		Leaf(WhiteSpace, " "),
		Leaf(BinaryOperator, op),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, right),
	))
}

// ============================================================
// Call Expressions
// ============================================================

func (e *JSEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	funCode := e.fs.CollectText(string(PreVisitCallExprFun))
	e.fs.AddLeaf(funCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {
	argCode := e.fs.CollectText(string(PreVisitCallExprArg))
	e.fs.AddLeaf(argCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	argTokens := e.fs.CollectForest(string(PreVisitCallExprArgs))
	var args []string
	for _, t := range argTokens {
		s := t.Serialize()
		if s != "" {
			args = append(args, s)
		}
	}
	e.fs.AddLeaf(strings.Join(args, ", "), KindExpr, nil)
}

func (e *JSEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCallExpr))
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
	case "len":
		// len(x) → len(x) helper for slices/strings (null-safe), (x ? x.Size : 0) for maps
		if len(node.Args) > 0 && e.isMapTypeExpr(node.Args[0]) {
			e.fs.AddTree(IRTree(CallExpression, KindExpr,
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(WhiteSpace, " "),
				Leaf(BinaryOperator, "?"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, argsStr),
				Leaf(Dot, "."),
				Leaf(Identifier, "Size"),
				Leaf(WhiteSpace, " "),
				Leaf(Colon, ":"),
				Leaf(WhiteSpace, " "),
				Leaf(NumberLiteral, "0"),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.AddTree(IRTree(CallExpression, KindExpr,
				Leaf(Identifier, "len"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(RightParen, ")"),
			))
		}
		return
	case "append":
		// append(slice, items...) → append(slice, items...)
		e.fs.AddTree(IRTree(CallExpression, KindExpr,
			Leaf(Identifier, "append"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
		return
	case "delete":
		// delete(m, k) → hmap.hashMapDelete(m, k)
		e.fs.AddTree(IRTree(CallExpression, KindExpr,
			Leaf(Identifier, "hmap.hashMapDelete"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
		return
	case "make":
		// Detect make(map[K]V) vs make([]T, n)
		if len(node.Args) >= 1 {
			if mapType, ok := node.Args[0].(*ast.MapType); ok {
				keyTypeConst := 1
				if e.pkg != nil && e.pkg.TypesInfo != nil {
					if tv, ok2 := e.pkg.TypesInfo.Types[mapType.Key]; ok2 && tv.Type != nil {
						mapT := types.NewMap(tv.Type, nil)
						keyTypeConst = jsMapKeyTypeConst(mapT)
						if keyTypeConst == 100 {
							if e.structKeyTypes == nil {
								e.structKeyTypes = make(map[string]bool)
							}
							e.structKeyTypes[tv.Type.String()] = true
						}
					}
				}
				e.fs.AddTree(IRTree(CallExpression, KindExpr,
					Leaf(Identifier, "hmap.newHashMap"),
					Leaf(LeftParen, "("),
					Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)),
					Leaf(RightParen, ")"),
				))
				return
			}
			if _, ok := node.Args[0].(*ast.ArrayType); ok {
				// make([]T, n) → new Array(n).fill(default)
				parts := strings.SplitN(argsStr, ", ", 2)
				if len(parts) >= 2 {
					// First part is the type (already traversed), second is the size
					e.fs.AddTree(IRTree(CallExpression, KindExpr,
						Leaf(Identifier, "new Array"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, parts[1]),
						Leaf(RightParen, ")"),
						Leaf(Dot, "."),
						Leaf(Identifier, "fill"),
						Leaf(LeftParen, "("),
						Leaf(NumberLiteral, "0"),
						Leaf(RightParen, ")"),
					))
				} else {
					e.fs.AddTree(IRTree(CallExpression, KindExpr, Leaf(Identifier, "[]")))
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
	}

	// Lower builtins (fmt.Println → console.log, etc.)
	lowered := jsLowerBuiltin(funName)
	if lowered != funName {
		funName = lowered
	}

	e.fs.AddTree(IRTree(CallExpression, KindExpr,
		Leaf(Identifier, funName),
		Leaf(LeftParen, "("),
		Leaf(Identifier, argsStr),
		Leaf(RightParen, ")"),
	))
}

// ============================================================
// Selector Expressions (a.b)
// ============================================================

func (e *JSEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	xCode := e.fs.CollectText(string(PreVisitSelectorExprX))
	e.fs.AddLeaf(xCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitSelectorExprSel(node *ast.Ident, indent int) {
	// Discard the traversed ident, use node.Name directly
	e.fs.CollectForest(string(PreVisitSelectorExprSel))
	e.fs.AddLeaf(node.Name, KindExpr, nil)
}

func (e *JSEmitter) PostVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSelectorExpr))
	xCode := ""
	selCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		selCode = tokens[1].Serialize()
	}

	if xCode == "os" && selCode == "Args" {
		e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, "process.argv.slice(1)")))
		return
	}

	// Lower builtins: fmt.Println → console.log
	loweredX := jsLowerBuiltin(xCode)
	loweredSel := jsLowerBuiltin(selCode)

	if loweredX == "" {
		// Package selector like fmt is suppressed
		e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, loweredSel)))
	} else {
		e.fs.AddTree(IRTree(SelectorExpression, KindExpr, Leaf(Identifier, loweredX+"."+loweredSel)))
	}
}

// ============================================================
// Index Expressions (a[i])
// ============================================================

func (e *JSEmitter) PostVisitIndexExprX(node *ast.IndexExpr, indent int) {
	xCode := e.fs.CollectText(string(PreVisitIndexExprX))
	e.fs.AddLeaf(xCode, KindExpr, nil)
	// Save for potential map assignment detection
	e.lastIndexXCode = xCode
}

func (e *JSEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	idxCode := e.fs.CollectText(string(PreVisitIndexExprIndex))
	e.fs.AddLeaf(idxCode, KindExpr, nil)
	e.lastIndexKeyCode = idxCode
}

func (e *JSEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitIndexExpr))
	xCode := ""
	idxCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		idxCode = tokens[1].Serialize()
	}

	// Check if this is a map index (read)
	if e.isMapTypeExpr(node.X) {
		tree := IRTree(IndexExpression, KindExpr,
			Leaf(Identifier, "hmap.hashMapGet"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, xCode),
			Leaf(Comma, ","),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, idxCode),
			Leaf(RightParen, ")"),
		)
		tree.GoType = e.getExprGoType(node)
		e.fs.AddTree(tree)
	} else {
		// Check for string indexing: str[i] returns byte in Go
		xType := e.getExprGoType(node.X)
		if xType != nil {
			if basic, ok := xType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
				e.fs.AddTree(IRTree(IndexExpression, KindExpr,
					Leaf(Identifier, xCode),
					Leaf(Dot, "."),
					Leaf(Identifier, "charCodeAt"),
					Leaf(LeftParen, "("),
					Leaf(Identifier, idxCode),
					Leaf(RightParen, ")"),
				))
				return
			}
		}
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

func (e *JSEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	xCode := e.fs.CollectText(string(PreVisitUnaryExpr))
	op := node.Op.String()
	if op == "^" {
		// Go's ^ is bitwise NOT (like ~ in JS)
		e.fs.AddTree(IRTree(UnaryExpression, KindExpr, Leaf(Identifier, "~"+xCode)))
	} else {
		e.fs.AddTree(IRTree(UnaryExpression, KindExpr, Leaf(Identifier, op+xCode)))
	}
}

// ============================================================
// Paren Expressions
// ============================================================

func (e *JSEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {
	inner := e.fs.CollectText(string(PreVisitParenExpr))
	e.fs.AddTree(IRTree(ParenExpression, KindExpr, Leaf(Identifier, "("+inner+")")))
}

// ============================================================
// Composite Literals (struct{}, []int{}, map[K]V{})
// ============================================================

func (e *JSEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	// Discard type tokens for JS (no type annotations)
	e.fs.CollectForest(string(PreVisitCompositeLitType))
}

func (e *JSEmitter) PostVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	eltCode := e.fs.CollectText(string(PreVisitCompositeLitElt))
	e.fs.AddLeaf(eltCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	eltTokens := e.fs.CollectForest(string(PreVisitCompositeLitElts))
	// Push each element back as an individual token (don't join)
	for _, t := range eltTokens {
		s := t.Serialize()
		if s != "" {
			e.fs.AddLeaf(s, TagLiteral, nil)
		}
	}
}

func (e *JSEmitter) PostVisitCompositeLit(node *ast.CompositeLit, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCompositeLit))
	// Collect individual element tokens (each is TagLiteral from PostVisitCompositeLitElts)
	var elts []string
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			elts = append(elts, s)
		}
	}
	eltsStr := strings.Join(elts, ", ")

	// Determine type of composite literal
	litType := e.getExprGoType(node)
	if litType == nil {
		// Fallback: just use array syntax
		e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr, Leaf(Identifier, "["+eltsStr+"]")))
		return
	}

	switch u := litType.Underlying().(type) {
	case *types.Struct:
		// Struct literal: new StructName(fields...)
		typeName := ""
		if node.Type != nil {
			typeName = exprToString(node.Type)
		}
		if typeName == "" {
			typeName = "Object"
		}
		// Check if using named fields (KeyValueExpr)
		if len(node.Elts) > 0 {
			if _, isKV := node.Elts[0].(*ast.KeyValueExpr); isKV {
				// Named fields — each element token is "Key: Value"
				// Build a map from field name to value code using individual tokens
				kvMap := make(map[string]string)
				for _, elt := range elts {
					parts := strings.SplitN(elt, ": ", 2)
					if len(parts) == 2 {
						key := parts[0]
						// Strip package prefix from key (e.g. "types.ID" → "ID")
						if dotIdx := strings.LastIndex(key, "."); dotIdx >= 0 {
							key = key[dotIdx+1:]
						}
						kvMap[key] = parts[1]
					}
				}
				// Build args in struct field order, using defaults for missing fields
				var args []string
				for i := 0; i < u.NumFields(); i++ {
					fieldName := u.Field(i).Name()
					if val, ok := kvMap[fieldName]; ok {
						args = append(args, val)
					} else {
						args = append(args, jsDefaultForGoType(u.Field(i).Type()))
					}
				}
				e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr,
					Leaf(Identifier, "new"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, typeName),
					Leaf(LeftParen, "("),
					Leaf(Identifier, strings.Join(args, ", ")),
					Leaf(RightParen, ")"),
				))
				return
			}
		}
		e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr,
			Leaf(Identifier, "new"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, typeName),
			Leaf(LeftParen, "("),
			Leaf(Identifier, eltsStr),
			Leaf(RightParen, ")"),
		))
	case *types.Slice:
		e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr, Leaf(Identifier, "["+eltsStr+"]")))
	case *types.Map:
		// Map literal: IIFE with hashMapSet calls
		keyTypeConst := jsMapKeyTypeConst(u)
		if keyTypeConst == 100 {
			if e.structKeyTypes == nil {
				e.structKeyTypes = make(map[string]bool)
			}
			e.structKeyTypes[u.Key().String()] = true
		}
		if len(elts) == 0 {
			e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr,
				Leaf(Identifier, "hmap.newHashMap"),
				Leaf(LeftParen, "("),
				Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)),
				Leaf(RightParen, ")"),
			))
		} else {
			// Each element is "key: value" from KeyValueExpr
			var children []IRNode
			children = append(children, Leaf(LeftParen, "("), Leaf(LeftParen, "("), Leaf(RightParen, ")"), Leaf(WhiteSpace, " "), Leaf(Identifier, "=>"), Leaf(WhiteSpace, " "), Leaf(LeftBrace, "{"), Leaf(WhiteSpace, " "))
			children = append(children, Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, "_m"), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(Identifier, "hmap.newHashMap"), Leaf(LeftParen, "("), Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)), Leaf(RightParen, ")"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "))
			for _, elt := range elts {
				parts := strings.SplitN(elt, ": ", 2)
				if len(parts) == 2 {
					children = append(children, Leaf(Identifier, "hmap.hashMapSet"), Leaf(LeftParen, "("), Leaf(Identifier, "_m"), Leaf(Comma, ","), Leaf(WhiteSpace, " "), Leaf(Identifier, parts[0]), Leaf(Comma, ","), Leaf(WhiteSpace, " "), Leaf(Identifier, parts[1]), Leaf(RightParen, ")"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "))
				}
			}
			children = append(children, Leaf(ReturnKeyword, "return"), Leaf(WhiteSpace, " "), Leaf(Identifier, "_m"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "), Leaf(RightBrace, "}"), Leaf(RightParen, ")"), Leaf(LeftParen, "("), Leaf(RightParen, ")"))
			e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr, children...))
		}
	default:
		e.fs.AddTree(IRTree(CompositeLitExpression, KindExpr, Leaf(Identifier, "["+eltsStr+"]")))
	}
}

// ============================================================
// KeyValue Expressions (for composite literals)
// ============================================================

func (e *JSEmitter) PostVisitKeyValueExprKey(node ast.Expr, indent int) {
	keyCode := e.fs.CollectText(string(PreVisitKeyValueExprKey))
	e.fs.AddLeaf(keyCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitKeyValueExprValue(node ast.Expr, indent int) {
	valCode := e.fs.CollectText(string(PreVisitKeyValueExprValue))
	e.fs.AddLeaf(valCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
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

func (e *JSEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	xCode := e.fs.CollectText(string(PreVisitSliceExprX))
	e.fs.AddLeaf(xCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {
	// Discard the duplicated X traversal
	e.fs.CollectForest(string(PreVisitSliceExprXBegin))
}

func (e *JSEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	lowCode := e.fs.CollectText(string(PreVisitSliceExprLow))
	e.fs.AddLeaf(lowCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {
	// Discard the duplicated X traversal
	e.fs.CollectForest(string(PreVisitSliceExprXEnd))
}

func (e *JSEmitter) PostVisitSliceExprHigh(node ast.Expr, indent int) {
	highCode := e.fs.CollectText(string(PreVisitSliceExprHigh))
	e.fs.AddLeaf(highCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSliceExpr))
	xCode := ""
	lowCode := ""
	highCode := ""

	// Use AST node to determine which parts are present
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

	if highCode == "" {
		e.fs.AddTree(IRTree(SliceExpression, KindExpr,
			Leaf(Identifier, xCode),
			Leaf(Dot, "."),
			Leaf(Identifier, "slice"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, lowCode),
			Leaf(RightParen, ")"),
		))
	} else {
		e.fs.AddTree(IRTree(SliceExpression, KindExpr,
			Leaf(Identifier, xCode),
			Leaf(Dot, "."),
			Leaf(Identifier, "slice"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, lowCode),
			Leaf(Comma, ","),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, highCode),
			Leaf(RightParen, ")"),
		))
	}
}

// ============================================================
// Array Type (used in composite literals, make calls)
// ============================================================

func (e *JSEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	// Discard type info for JS
	e.fs.CollectForest(string(PreVisitArrayType))
	e.fs.AddLeaf("[]", TagType, nil)
}

// ============================================================
// Map Type
// ============================================================

func (e *JSEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitMapKeyType))
}

func (e *JSEmitter) PostVisitMapValueType(node ast.Expr, indent int) {
	e.fs.CollectForest(string(PreVisitMapValueType))
}

func (e *JSEmitter) PostVisitMapType(node *ast.MapType, indent int) {
	e.fs.CollectForest(string(PreVisitMapType))
	e.fs.AddLeaf("map", TagType, nil)
}

// ============================================================
// Function Literals (closures)
// ============================================================

func (e *JSEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	// Discard the type tokens (e.g., "int") that traverseExpression(param.Type) pushed
	e.fs.CollectForest(string(PreVisitFuncLitTypeParam))
	// Push the actual parameter names from node.Names
	for _, name := range node.Names {
		e.fs.AddLeaf(name.Name, TagIdent, nil)
	}
}

func (e *JSEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	// Collect parameter names
	tokens := e.fs.CollectForest(string(PreVisitFuncLitTypeParams))
	var paramNames []string
	for _, t := range tokens {
		if t.Kind == TagIdent && t.Serialize() != "" {
			paramNames = append(paramNames, t.Serialize())
		}
	}
	// Use a sentinel space for empty params so the token doesn't get dropped
	paramsStr := strings.Join(paramNames, ", ")
	if paramsStr == "" {
		paramsStr = " "
	}
	e.fs.AddLeaf(paramsStr, KindExpr, nil)
}

func (e *JSEmitter) PostVisitFuncLitTypeResults(node *ast.FieldList, indent int) {
	// Discard result types for JS
	e.fs.CollectForest(string(PreVisitFuncLitTypeResults))
}

func (e *JSEmitter) PostVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	bodyCode := e.fs.CollectText(string(PreVisitFuncLitBody))
	e.fs.AddLeaf(bodyCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncLit))
	paramsCode := ""
	bodyCode := ""
	if len(tokens) >= 1 {
		paramsCode = strings.TrimSpace(tokens[0].Serialize())
	}
	if len(tokens) >= 2 {
		bodyCode = tokens[1].Serialize()
	}
	e.fs.AddTree(IRTree(FuncLitExpression, KindExpr,
		Leaf(LeftParen, "("),
		Leaf(Identifier, paramsCode),
		Leaf(RightParen, ")"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, "=>"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, bodyCode),
	))
}

// ============================================================
// Type Assertions
// ============================================================

func (e *JSEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	// Discard type for JS (type assertions are no-ops at runtime)
	e.fs.CollectForest(string(PreVisitTypeAssertExprType))
}

func (e *JSEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	xCode := e.fs.CollectText(string(PreVisitTypeAssertExprX))
	e.fs.AddLeaf(xCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitTypeAssertExpr))
	xCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Serialize()
	}
	// In JS, type assertions are pass-through
	e.fs.AddTree(IRTree(TypeAssertExpression, KindExpr, Leaf(Identifier, xCode)))
}

// ============================================================
// Function Declarations
// ============================================================

func (e *JSEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	// Track number of results for multi-value return handling
	e.numFuncResults = 0
	if node.Type.Results != nil {
		e.numFuncResults = node.Type.Results.NumFields()
	}
}

func (e *JSEmitter) PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	// Discard result type tokens for JS
	e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeResultsList))
}

func (e *JSEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	// Discard all result type tokens for JS
	e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeResults))
}

func (e *JSEmitter) PostVisitFuncDeclName(node *ast.Ident, indent int) {
	// Discard traversed ident, use node.Name directly
	e.fs.CollectForest(string(PreVisitFuncDeclName))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *JSEmitter) PostVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {
	// Discard type tokens for JS
	e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeParamsListType))
}

func (e *JSEmitter) PostVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	// Discard traversed ident, push name directly
	e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeParamsArgName))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *JSEmitter) PostVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	// Collect param names from this field group
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeParamsList))
	for _, t := range tokens {
		if t.Kind == TagIdent {
			e.fs.AddLeaf(t.Serialize(), TagIdent, nil)
		}
	}
}

func (e *JSEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignatureTypeParams))
	var paramNames []string
	for _, t := range tokens {
		if t.Kind == TagIdent {
			paramNames = append(paramNames, t.Serialize())
		}
	}
	e.fs.AddLeaf(strings.Join(paramNames, ", "), KindExpr, nil)
}

func (e *JSEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDeclSignature))
	funcName := ""
	paramsStr := ""
	for _, t := range tokens {
		if t.Kind == TagIdent && funcName == "" {
			funcName = t.Serialize()
		} else if t.Kind == TagExpr {
			paramsStr = t.Serialize()
		}
	}

	if e.inNamespace {
		e.fs.AddTree(IRTree(FuncDeclaration, KindDecl,
			Leaf(NewLine, "\n"),
			Leaf(Identifier, funcName),
			Leaf(LeftParen, "("),
			Leaf(Identifier, paramsStr),
			Leaf(RightParen, ")"),
		))
	} else {
		e.fs.AddTree(IRTree(FuncDeclaration, KindDecl,
			Leaf(NewLine, "\n"),
			Leaf(FunctionKeyword, "function"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, funcName),
			Leaf(LeftParen, "("),
			Leaf(Identifier, paramsStr),
			Leaf(RightParen, ")"),
		))
	}
}

func (e *JSEmitter) PostVisitFuncDeclBody(node *ast.BlockStmt, indent int) {
	bodyCode := e.fs.CollectText(string(PreVisitFuncDeclBody))
	e.fs.AddLeaf(bodyCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitFuncDecl))
	sigCode := ""
	bodyCode := ""
	if len(tokens) >= 1 {
		sigCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		bodyCode = tokens[1].Serialize()
	}

	if e.inNamespace {
		e.fs.AddTree(IRTree(FuncDeclaration, KindDecl, Leaf(Identifier, sigCode+" "+bodyCode+",\n")))
	} else {
		e.fs.AddTree(IRTree(FuncDeclaration, KindDecl, Leaf(Identifier, sigCode+" "+bodyCode+"\n")))
	}
}

// ============================================================
// Forward Declaration Signatures (suppressed for JS)
// ============================================================

func (e *JSEmitter) PostVisitFuncDeclSignatures(indent int) {
	// Discard all forward declaration signatures for JS
	e.fs.CollectForest(string(PreVisitFuncDeclSignatures))
}

// ============================================================
// Block Statements
// ============================================================

func (e *JSEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	e.indent = indent
}

func (e *JSEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	itemCode := e.fs.CollectText(string(PreVisitBlockStmtList))
	e.fs.AddLeaf(itemCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitBlockStmt))
	var children []IRNode
	children = append(children, Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			children = append(children, t)
		}
	}
	children = append(children, Leaf(WhiteSpace, jsIndent(indent/2)), Leaf(RightBrace, "}"))
	e.fs.AddTree(IRTree(BlockStatement, KindStmt, children...))
}

// ============================================================
// Assignment Statements
// ============================================================

func (e *JSEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	e.indent = indent
	// Reset map assign state
	e.mapAssignVar = ""
	e.mapAssignKey = ""
}

func (e *JSEmitter) PostVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	lhsCode := e.fs.CollectText(string(PreVisitAssignStmtLhsExpr))

	// Detect map assignment: if lhs is m[k] on a map type
	if indexExpr, ok := node.(*ast.IndexExpr); ok {
		if e.isMapTypeExpr(indexExpr.X) {
			// Save map var and key for PostVisitAssignStmt to generate hashMapSet
			e.mapAssignVar = e.lastIndexXCode
			e.mapAssignKey = e.lastIndexKeyCode
			e.fs.AddLeaf(lhsCode, KindExpr, nil)
			return
		}
	}
	e.fs.AddLeaf(lhsCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
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

func (e *JSEmitter) PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {
	rhsCode := e.fs.CollectText(string(PreVisitAssignStmtRhsExpr))
	e.fs.AddLeaf(rhsCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitAssignStmtRhs))
	var rhsExprs []string
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			rhsExprs = append(rhsExprs, s)
		}
	}
	e.fs.AddLeaf(strings.Join(rhsExprs, ", "), KindExpr, nil)
}

func (e *JSEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitAssignStmt))
	lhsStr := ""
	rhsStr := ""
	if len(tokens) >= 1 {
		lhsStr = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		rhsStr = tokens[1].Serialize()
	}

	ind := jsIndent(indent / 2)

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

	// Map assignment: m[k] = v → hmap.hashMapSet(m, k, v)
	if e.mapAssignVar != "" && e.mapAssignKey != "" {
		e.fs.AddTree(IRTree(AssignStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(Identifier, "hmap.hashMapSet"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, e.mapAssignVar),
			Leaf(Comma, ","),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, e.mapAssignKey),
			Leaf(Comma, ","),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, rhsStr),
			Leaf(RightParen, ")"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
		e.mapAssignVar = ""
		e.mapAssignKey = ""
		return
	}

	// Comma-ok map read: val, ok := m[key] (must be checked before generic multi-value)
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if indexExpr, ok := node.Rhs[0].(*ast.IndexExpr); ok {
			if e.isMapTypeExpr(indexExpr.X) {
				valName := exprToString(node.Lhs[0])
				okName := exprToString(node.Lhs[1])
				mapName := exprToString(indexExpr.X)
				keyStr := exprToString(indexExpr.Index)
				// Get the zero value for the map's value type
				zeroVal := "null"
				mapGoType := e.getExprGoType(indexExpr.X)
				if mapGoType != nil {
					if mapUnderlying, ok2 := mapGoType.Underlying().(*types.Map); ok2 {
						zeroVal = jsDefaultForGoType(mapUnderlying.Elem())
					}
				}
				if tokStr == ":=" {
					e.fs.AddTree(IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, "let"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, okName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "hmap.hashMapContains"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, mapName),
						Leaf(Comma, ","),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, keyStr),
						Leaf(RightParen, ")"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
					e.fs.AddTree(IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, "let"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, valName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, okName),
						Leaf(WhiteSpace, " "),
						Leaf(BinaryOperator, "?"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "hmap.hashMapGet"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, mapName),
						Leaf(Comma, ","),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, keyStr),
						Leaf(RightParen, ")"),
						Leaf(WhiteSpace, " "),
						Leaf(Colon, ":"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, zeroVal),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
				} else {
					e.fs.AddTree(IRTree(AssignStatement, KindStmt,
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
						Leaf(Identifier, keyStr),
						Leaf(RightParen, ")"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
					e.fs.AddTree(IRTree(AssignStatement, KindStmt,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, valName),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, "="),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, okName),
						Leaf(WhiteSpace, " "),
						Leaf(BinaryOperator, "?"),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, "hmap.hashMapGet"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, mapName),
						Leaf(Comma, ","),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, keyStr),
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

	// Comma-ok type assertion: val, ok := x.(Type) (must be checked before generic multi-value)
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if _, ok := node.Rhs[0].(*ast.TypeAssertExpr); ok {
			valName := exprToString(node.Lhs[0])
			okName := exprToString(node.Lhs[1])
			// Type assertions are no-ops in JS; just check if value is not null/undefined
			if tokStr == ":=" {
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, "let"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, valName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, rhsStr),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, "let"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, okName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(LeftParen, "("),
					Leaf(Identifier, valName),
					Leaf(WhiteSpace, " "),
					Leaf(ComparisonOperator, "!=="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "null"),
					Leaf(WhiteSpace, " "),
					Leaf(LogicalOperator, "&&"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, valName),
					Leaf(WhiteSpace, " "),
					Leaf(ComparisonOperator, "!=="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "undefined"),
					Leaf(RightParen, ")"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
			} else {
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, valName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, rhsStr),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
				e.fs.AddTree(IRTree(AssignStatement, KindStmt,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, okName),
					Leaf(WhiteSpace, " "),
					Leaf(Assignment, "="),
					Leaf(WhiteSpace, " "),
					Leaf(LeftParen, "("),
					Leaf(Identifier, valName),
					Leaf(WhiteSpace, " "),
					Leaf(ComparisonOperator, "!=="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "null"),
					Leaf(WhiteSpace, " "),
					Leaf(LogicalOperator, "&&"),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, valName),
					Leaf(WhiteSpace, " "),
					Leaf(ComparisonOperator, "!=="),
					Leaf(WhiteSpace, " "),
					Leaf(Identifier, "undefined"),
					Leaf(RightParen, ")"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
			}
			return
		}
	}

	// Multi-value return: a, b := func() → let [a, b] = func()
	if len(node.Lhs) > 1 && len(node.Rhs) == 1 {
		lhsParts := make([]string, len(node.Lhs))
		for i, lhs := range node.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				if ident.Name == "_" {
					lhsParts[i] = "_"
				} else {
					lhsParts[i] = ident.Name
				}
			} else {
				lhsParts[i] = exprToString(lhs)
			}
		}
		destructured := "[" + strings.Join(lhsParts, ", ") + "]"
		if tokStr == ":=" {
			e.fs.AddTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, "let"),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, destructured),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, rhsStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		} else {
			e.fs.AddTree(IRTree(AssignStatement, KindStmt,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, destructured),
				Leaf(WhiteSpace, " "),
				Leaf(Assignment, "="),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, rhsStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		}
		return
	}

	switch tokStr {
	case ":=":
		e.fs.AddTree(IRTree(AssignStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(Identifier, "let"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, lhsStr),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, "="),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, rhsStr),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
		e.fs.AddTree(IRTree(AssignStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(Identifier, lhsStr),
			Leaf(WhiteSpace, " "),
			Leaf(Assignment, tokStr),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, rhsStr),
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
			Leaf(Identifier, rhsStr),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	}
}

// ============================================================
// Declaration Statements (var x int, var y = 5)
// ============================================================

func (e *JSEmitter) PreVisitDeclStmt(node *ast.DeclStmt, indent int) {
	e.indent = indent
}

func (e *JSEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	// Keep type info for default value determination
	tokens := e.fs.CollectForest(string(PreVisitDeclStmtValueSpecType))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Serialize()
	}
	// Also carry the Go type for struct default value generation
	var goType types.Type
	if e.pkg != nil && e.pkg.TypesInfo != nil && index < len(node.Names) {
		if obj := e.pkg.TypesInfo.Defs[node.Names[index]]; obj != nil {
			goType = obj.Type()
		}
	}
	e.fs.AddLeaf(typeStr, TagType, goType)
}

func (e *JSEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	// Discard traversed ident, use node.Name directly
	e.fs.CollectForest(string(PreVisitDeclStmtValueSpecNames))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *JSEmitter) PostVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {
	valCode := e.fs.CollectText(string(PreVisitDeclStmtValueSpecValue))
	e.fs.AddLeaf(valCode, TagExpr, nil)
}

func (e *JSEmitter) PostVisitDeclStmt(node *ast.DeclStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitDeclStmt))
	ind := jsIndent(indent / 2)

	// Parse tokens: types (TagType), names (TagIdent), values (TagExpr)
	// Pattern for each var: type, name, [value]
	// Multiple vars result in multiple type/name/value groups
	var children []IRNode
	i := 0
	for i < len(tokens) {
		typeStr := ""
		var goType types.Type
		nameStr := ""
		valueStr := ""

		// Read type
		if i < len(tokens) && tokens[i].Kind == TagType {
			typeStr = tokens[i].Serialize()
			goType = tokens[i].GoType
			i++
		}
		// Read name
		if i < len(tokens) && tokens[i].Kind == TagIdent {
			nameStr = tokens[i].Serialize()
			i++
		}
		// Read value (optional)
		if i < len(tokens) && tokens[i].Kind == TagExpr {
			valueStr = tokens[i].Serialize()
			i++
		}

		if nameStr == "" {
			continue
		}

		if valueStr != "" {
			children = append(children, Leaf(WhiteSpace, ind), Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, nameStr), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(Identifier, valueStr), Leaf(Semicolon, ";"), Leaf(NewLine, "\n"))
		} else {
			// No initializer - generate default value based on Go type or type string
			defaultVal := ""
			if goType != nil {
				if _, isStruct := goType.Underlying().(*types.Struct); isStruct {
					// Struct type: create with constructor
					defaultVal = fmt.Sprintf("new %s()", typeStr)
				} else {
					defaultVal = jsDefaultForGoType(goType)
				}
			} else {
				defaultVal = e.defaultForTypeStr(typeStr)
			}
			children = append(children, Leaf(WhiteSpace, ind), Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, nameStr), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(Identifier, defaultVal), Leaf(Semicolon, ";"), Leaf(NewLine, "\n"))
		}
	}
	e.fs.AddTree(IRTree(DeclStatement, KindStmt, children...))
}

// defaultForTypeStr returns the JS default value for a Go type name string.
func (e *JSEmitter) defaultForTypeStr(typeStr string) string {
	switch typeStr {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "byte", "rune":
		return "0"
	case "string":
		return `""`
	case "bool":
		return "false"
	case "[]":
		return "[]"
	case "map":
		return "null"
	}
	// Check if it's a slice type
	if strings.HasPrefix(typeStr, "[]") {
		return "[]"
	}
	// Default: zero value
	return "null"
}

// jsDefaultForASTType returns the JS default value for a Go AST type expression.
func (e *JSEmitter) jsDefaultForASTType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		switch t.Name {
		case "int", "int8", "int16", "int32", "int64",
			"uint", "uint8", "uint16", "uint32", "uint64",
			"float32", "float64", "byte", "rune":
			return "0"
		case "string":
			return `""`
		case "bool":
			return "false"
		}
		// Check if it's a struct type using TypesInfo
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if obj := e.pkg.TypesInfo.Uses[t]; obj != nil {
				if named, ok := obj.Type().(*types.Named); ok {
					if _, isStruct := named.Underlying().(*types.Struct); isStruct {
						return fmt.Sprintf("new %s()", t.Name)
					}
				}
			}
		}
		return "null"
	case *ast.ArrayType:
		return "[]"
	case *ast.MapType:
		return "null"
	case *ast.SelectorExpr:
		// e.g., pkg.StructType — check if it's a struct
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if tv, ok := e.pkg.TypesInfo.Types[expr]; ok {
				if named, ok := tv.Type.(*types.Named); ok {
					if _, isStruct := named.Underlying().(*types.Struct); isStruct {
						if ident, ok := t.X.(*ast.Ident); ok {
							return fmt.Sprintf("new %s.%s()", ident.Name, t.Sel.Name)
						}
					}
				}
			}
		}
		return "null"
	case *ast.InterfaceType:
		return "null"
	case *ast.FuncType:
		return "null"
	}
	return "null"
}

// ============================================================
// Return Statements
// ============================================================

func (e *JSEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	e.indent = indent
}

func (e *JSEmitter) PostVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	resultCode := e.fs.CollectText(string(PreVisitReturnStmtResult))
	e.fs.AddLeaf(resultCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitReturnStmt))
	ind := jsIndent(indent / 2)

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
		// Multi-value return: return [a, b]
		var vals []string
		for _, t := range tokens {
			vals = append(vals, t.Serialize())
		}
		e.fs.AddTree(IRTree(ReturnStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftBracket, "["),
			Leaf(Identifier, strings.Join(vals, ", ")),
			Leaf(RightBracket, "]"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	}
}

// ============================================================
// Expression Statements
// ============================================================

func (e *JSEmitter) PreVisitExprStmt(node *ast.ExprStmt, indent int) {
	e.indent = indent
}

func (e *JSEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	xCode := e.fs.CollectText(string(PreVisitExprStmtX))
	e.fs.AddLeaf(xCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitExprStmt(node *ast.ExprStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitExprStmt))
	code := ""
	if len(tokens) >= 1 {
		code = tokens[0].Serialize()
	}
	ind := jsIndent(indent / 2)
	e.fs.AddTree(IRTree(ExprStatement, KindStmt, Leaf(Identifier, ind+code+";\n")))
}

// ============================================================
// If Statements
// ============================================================

func (e *JSEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	e.indent = indent
	// Push new stack frame for nesting support
	e.ifInitStack = append(e.ifInitStack, "")
	e.ifCondStack = append(e.ifCondStack, "")
	e.ifBodyStack = append(e.ifBodyStack, "")
	e.ifElseStack = append(e.ifElseStack, "")
}

func (e *JSEmitter) PostVisitIfStmtInit(node ast.Stmt, indent int) {
	e.ifInitStack[len(e.ifInitStack)-1] = e.fs.CollectText(string(PreVisitIfStmtInit))
}

func (e *JSEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	e.ifCondStack[len(e.ifCondStack)-1] = e.fs.CollectText(string(PreVisitIfStmtCond))
}

func (e *JSEmitter) PostVisitIfStmtBody(node *ast.IfStmt, indent int) {
	e.ifBodyStack[len(e.ifBodyStack)-1] = e.fs.CollectText(string(PreVisitIfStmtBody))
}

func (e *JSEmitter) PostVisitIfStmtElse(node *ast.IfStmt, indent int) {
	e.ifElseStack[len(e.ifElseStack)-1] = e.fs.CollectText(string(PreVisitIfStmtElse))
}

func (e *JSEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	e.fs.CollectForest(string(PreVisitIfStmt))
	ind := jsIndent(indent / 2)

	// Pop stack frame
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
		// Wrap in block scope to avoid variable redeclaration conflicts
		children = append(children, Leaf(WhiteSpace, ind), Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
		children = append(children, Leaf(Identifier, initCode))
		children = append(children, Leaf(WhiteSpace, ind), Leaf(IfKeyword, "if"), Leaf(WhiteSpace, " "), Leaf(LeftParen, "("), Leaf(Identifier, condCode), Leaf(RightParen, ")"), Leaf(WhiteSpace, " "), Leaf(Identifier, bodyCode))
	} else {
		children = append(children, Leaf(WhiteSpace, ind), Leaf(IfKeyword, "if"), Leaf(WhiteSpace, " "), Leaf(LeftParen, "("), Leaf(Identifier, condCode), Leaf(RightParen, ")"), Leaf(WhiteSpace, " "), Leaf(Identifier, bodyCode))
	}
	if elseCode != "" {
		// Check if else is another if statement (else if chain)
		trimmed := strings.TrimLeft(elseCode, " \t\n")
		if strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "if(") {
			children = append(children, Leaf(WhiteSpace, " "), Leaf(ElseKeyword, "else"), Leaf(WhiteSpace, " "), Leaf(Identifier, trimmed))
		} else {
			children = append(children, Leaf(WhiteSpace, " "), Leaf(ElseKeyword, "else"), Leaf(WhiteSpace, " "), Leaf(Identifier, elseCode))
		}
	}
	children = append(children, Leaf(NewLine, "\n"))
	if initCode != "" {
		children = append(children, Leaf(WhiteSpace, ind), Leaf(RightBrace, "}"), Leaf(NewLine, "\n"))
	}
	e.fs.AddTree(IRTree(IfStatement, KindStmt, children...))
}

// ============================================================
// For Statements
// ============================================================

func (e *JSEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	e.indent = indent
	// Push new stack frame for nesting support
	e.forInitStack = append(e.forInitStack, "")
	e.forCondStack = append(e.forCondStack, "")
	e.forPostStack = append(e.forPostStack, "")
}

func (e *JSEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	initCode := e.fs.CollectText(string(PreVisitForStmtInit))
	initCode = strings.TrimRight(initCode, ";\n \t")
	initCode = strings.TrimLeft(initCode, " \t")
	e.forInitStack[len(e.forInitStack)-1] = initCode
}

func (e *JSEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	e.forCondStack[len(e.forCondStack)-1] = e.fs.CollectText(string(PreVisitForStmtCond))
}

func (e *JSEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	postCode := e.fs.CollectText(string(PreVisitForStmtPost))
	postCode = strings.TrimRight(postCode, ";\n \t")
	postCode = strings.TrimLeft(postCode, " \t")
	e.forPostStack[len(e.forPostStack)-1] = postCode
}

func (e *JSEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {
	bodyCode := e.fs.CollectText(string(PreVisitForStmt))
	ind := jsIndent(indent / 2)

	// Pop stack frame
	n := len(e.forInitStack)
	initCode := e.forInitStack[n-1]
	condCode := e.forCondStack[n-1]
	postCode := e.forPostStack[n-1]
	e.forInitStack = e.forInitStack[:n-1]
	e.forCondStack = e.forCondStack[:n-1]
	e.forPostStack = e.forPostStack[:n-1]

	// Infinite loop (no init, cond, post)
	if node.Init == nil && node.Cond == nil && node.Post == nil {
		e.fs.AddTree(IRTree(ForStatement, KindStmt,
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

	// While loop (only cond)
	if node.Init == nil && node.Post == nil && node.Cond != nil {
		e.fs.AddTree(IRTree(ForStatement, KindStmt,
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

	e.fs.AddTree(IRTree(ForStatement, KindStmt,
		Leaf(WhiteSpace, ind),
		Leaf(ForKeyword, "for"),
		Leaf(WhiteSpace, " "),
		Leaf(LeftParen, "("),
		Leaf(Identifier, initCode),
		Leaf(Semicolon, ";"),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, condCode),
		Leaf(Semicolon, ";"),
		Leaf(WhiteSpace, " "),
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

func (e *JSEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	e.indent = indent
}

func (e *JSEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	keyCode := e.fs.CollectText(string(PreVisitRangeStmtKey))
	e.fs.AddLeaf(keyCode, TagIdent, nil)
}

func (e *JSEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	valCode := e.fs.CollectText(string(PreVisitRangeStmtValue))
	e.fs.AddLeaf(valCode, TagIdent, nil)
}

func (e *JSEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	xCode := e.fs.CollectText(string(PreVisitRangeStmtX))
	e.fs.AddLeaf(xCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitRangeStmt))
	ind := jsIndent(indent / 2)

	keyCode := ""
	valCode := ""
	xCode := ""
	bodyCode := ""

	idx := 0
	// Use AST node to determine which tokens are present
	// Sema pass sets Key=nil when key is blank identifier (_)
	if node.Key != nil {
		// key (TagIdent)
		if idx < len(tokens) && tokens[idx].Kind == TagIdent {
			keyCode = tokens[idx].Serialize()
			idx++
		}
	}
	if node.Value != nil {
		// value (TagIdent)
		if idx < len(tokens) && tokens[idx].Kind == TagIdent {
			valCode = tokens[idx].Serialize()
			idx++
		}
	}
	// collection (TagExpr)
	if idx < len(tokens) {
		xCode = tokens[idx].Serialize()
		idx++
	}
	// body
	if idx < len(tokens) {
		bodyCode = tokens[idx].Serialize()
	}
	// When Key was nil (blank identifier), treat as value-only range
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
		if valCode != "" && valCode != "_" {
			// Key-value range on map
			var children []IRNode
			children = append(children, Leaf(WhiteSpace, ind), Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
			children = append(children, Leaf(WhiteSpace, ind+"  "), Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, "_keys"), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(Identifier, "hmap.hashMapKeys"), Leaf(LeftParen, "("), Leaf(Identifier, xCode), Leaf(RightParen, ")"), Leaf(Semicolon, ";"), Leaf(NewLine, "\n"))
			children = append(children, Leaf(WhiteSpace, ind+"  "), Leaf(ForKeyword, "for"), Leaf(WhiteSpace, " "), Leaf(LeftParen, "("), Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, "_i"), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(NumberLiteral, "0"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "), Leaf(Identifier, "_i"), Leaf(WhiteSpace, " "), Leaf(ComparisonOperator, "<"), Leaf(WhiteSpace, " "), Leaf(Identifier, "_keys"), Leaf(Dot, "."), Leaf(Identifier, "length"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "), Leaf(Identifier, "_i"), Leaf(UnaryOperator, "++"), Leaf(RightParen, ")"), Leaf(WhiteSpace, " "), Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
			if keyCode != "_" {
				children = append(children, Leaf(WhiteSpace, ind+"    "), Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, keyCode), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(Identifier, "_keys"), Leaf(LeftBracket, "["), Leaf(Identifier, "_i"), Leaf(RightBracket, "]"), Leaf(Semicolon, ";"), Leaf(NewLine, "\n"))
			}
			children = append(children, Leaf(WhiteSpace, ind+"    "), Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, valCode), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(Identifier, "hmap.hashMapGet"), Leaf(LeftParen, "("), Leaf(Identifier, xCode), Leaf(Comma, ","), Leaf(WhiteSpace, " "), Leaf(Identifier, "_keys"), Leaf(LeftBracket, "["), Leaf(Identifier, "_i"), Leaf(RightBracket, "]"), Leaf(RightParen, ")"), Leaf(Semicolon, ";"), Leaf(NewLine, "\n"))
			children = append(children, Leaf(WhiteSpace, ind+"    "), Leaf(Identifier, bodyCode), Leaf(NewLine, "\n"))
			children = append(children, Leaf(WhiteSpace, ind+"  "), Leaf(RightBrace, "}"), Leaf(NewLine, "\n"))
			children = append(children, Leaf(WhiteSpace, ind), Leaf(RightBrace, "}"), Leaf(NewLine, "\n"))
			e.fs.AddTree(IRTree(RangeStatement, KindStmt, children...))
		} else {
			// Key-only range on map
			var children []IRNode
			children = append(children, Leaf(WhiteSpace, ind), Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
			children = append(children, Leaf(WhiteSpace, ind+"  "), Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, "_keys"), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(Identifier, "hmap.hashMapKeys"), Leaf(LeftParen, "("), Leaf(Identifier, xCode), Leaf(RightParen, ")"), Leaf(Semicolon, ";"), Leaf(NewLine, "\n"))
			children = append(children, Leaf(WhiteSpace, ind+"  "), Leaf(ForKeyword, "for"), Leaf(WhiteSpace, " "), Leaf(LeftParen, "("), Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, "_i"), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(NumberLiteral, "0"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "), Leaf(Identifier, "_i"), Leaf(WhiteSpace, " "), Leaf(ComparisonOperator, "<"), Leaf(WhiteSpace, " "), Leaf(Identifier, "_keys"), Leaf(Dot, "."), Leaf(Identifier, "length"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "), Leaf(Identifier, "_i"), Leaf(UnaryOperator, "++"), Leaf(RightParen, ")"), Leaf(WhiteSpace, " "), Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
			if keyCode != "_" {
				children = append(children, Leaf(WhiteSpace, ind+"    "), Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, keyCode), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(Identifier, "_keys"), Leaf(LeftBracket, "["), Leaf(Identifier, "_i"), Leaf(RightBracket, "]"), Leaf(Semicolon, ";"), Leaf(NewLine, "\n"))
			}
			children = append(children, Leaf(WhiteSpace, ind+"    "), Leaf(Identifier, bodyCode), Leaf(NewLine, "\n"))
			children = append(children, Leaf(WhiteSpace, ind+"  "), Leaf(RightBrace, "}"), Leaf(NewLine, "\n"))
			children = append(children, Leaf(WhiteSpace, ind), Leaf(RightBrace, "}"), Leaf(NewLine, "\n"))
			e.fs.AddTree(IRTree(RangeStatement, KindStmt, children...))
		}
		return
	}

	// If range expression is an inline composite literal, emit a temp variable
	if _, isCompLit := node.X.(*ast.CompositeLit); isCompLit {
		tmpVar := fmt.Sprintf("_range%d", e.rangeVarCounter)
		e.rangeVarCounter++
		var children []IRNode
		children = append(children, Leaf(WhiteSpace, ind), Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
		children = append(children, Leaf(WhiteSpace, ind+"  "), Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, tmpVar), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(Identifier, xCode), Leaf(Semicolon, ";"), Leaf(NewLine, "\n"))
		xCode = tmpVar
		if valCode != "" && valCode != "_" {
			loopVar := keyCode
			if loopVar == "_" || loopVar == "" {
				loopVar = "_i"
			}
			valDecl := fmt.Sprintf("%s      let %s = %s[%s];\n", ind, valCode, xCode, loopVar)
			bodyWithDecl := strings.Replace(bodyCode, "{\n", "{\n"+valDecl, 1)
			children = append(children, Leaf(WhiteSpace, ind+"  "), Leaf(ForKeyword, "for"), Leaf(WhiteSpace, " "), Leaf(LeftParen, "("), Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, loopVar), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(NumberLiteral, "0"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "), Leaf(Identifier, loopVar), Leaf(WhiteSpace, " "), Leaf(ComparisonOperator, "<"), Leaf(WhiteSpace, " "), Leaf(Identifier, xCode), Leaf(Dot, "."), Leaf(Identifier, "length"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "), Leaf(Identifier, loopVar), Leaf(UnaryOperator, "++"), Leaf(RightParen, ")"), Leaf(WhiteSpace, " "), Leaf(Identifier, bodyWithDecl), Leaf(NewLine, "\n"))
		} else {
			loopVar := keyCode
			if loopVar == "_" || loopVar == "" {
				loopVar = "_i"
			}
			children = append(children, Leaf(WhiteSpace, ind+"  "), Leaf(ForKeyword, "for"), Leaf(WhiteSpace, " "), Leaf(LeftParen, "("), Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, loopVar), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(NumberLiteral, "0"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "), Leaf(Identifier, loopVar), Leaf(WhiteSpace, " "), Leaf(ComparisonOperator, "<"), Leaf(WhiteSpace, " "), Leaf(Identifier, xCode), Leaf(Dot, "."), Leaf(Identifier, "length"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "), Leaf(Identifier, loopVar), Leaf(UnaryOperator, "++"), Leaf(RightParen, ")"), Leaf(WhiteSpace, " "), Leaf(Identifier, bodyCode), Leaf(NewLine, "\n"))
		}
		children = append(children, Leaf(WhiteSpace, ind), Leaf(RightBrace, "}"), Leaf(NewLine, "\n"))
		e.fs.AddTree(IRTree(RangeStatement, KindStmt, children...))
		return
	}

	// Slice/string range
	if valCode != "" && valCode != "_" {
		// Key-value range on slice/string
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = "_i"
		}

		// Inject value declaration at start of body
		valDecl := fmt.Sprintf("%s    let %s = %s[%s];\n", ind, valCode, xCode, loopVar)
		// Insert value decl inside the body block
		bodyWithDecl := strings.Replace(bodyCode, "{\n", "{\n"+valDecl, 1)

		e.fs.AddTree(IRTree(RangeStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, loopVar), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(NumberLiteral, "0"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "), Leaf(Identifier, loopVar), Leaf(WhiteSpace, " "), Leaf(ComparisonOperator, "<"), Leaf(WhiteSpace, " "), Leaf(Identifier, xCode), Leaf(Dot, "."), Leaf(Identifier, "length"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "), Leaf(Identifier, loopVar), Leaf(UnaryOperator, "++"),
			Leaf(RightParen, ")"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, bodyWithDecl),
			Leaf(NewLine, "\n"),
		))
	} else {
		// Key-only range
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = "_i"
		}
		e.fs.AddTree(IRTree(RangeStatement, KindStmt,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftParen, "("),
			Leaf(Identifier, "let"), Leaf(WhiteSpace, " "), Leaf(Identifier, loopVar), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(NumberLiteral, "0"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "), Leaf(Identifier, loopVar), Leaf(WhiteSpace, " "), Leaf(ComparisonOperator, "<"), Leaf(WhiteSpace, " "), Leaf(Identifier, xCode), Leaf(Dot, "."), Leaf(Identifier, "length"), Leaf(Semicolon, ";"), Leaf(WhiteSpace, " "), Leaf(Identifier, loopVar), Leaf(UnaryOperator, "++"),
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

func (e *JSEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	e.indent = indent
}

func (e *JSEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	tagCode := e.fs.CollectText(string(PreVisitSwitchStmtTag))
	e.fs.AddLeaf(tagCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitSwitchStmt))
	ind := jsIndent(indent / 2)

	tagCode := ""
	idx := 0
	if idx < len(tokens) {
		tagCode = tokens[idx].Serialize()
		idx++
	}

	var children []IRNode
	children = append(children, Leaf(WhiteSpace, ind), Leaf(SwitchKeyword, "switch"), Leaf(WhiteSpace, " "), Leaf(LeftParen, "("), Leaf(Identifier, tagCode), Leaf(RightParen, ")"), Leaf(WhiteSpace, " "), Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
	for i := idx; i < len(tokens); i++ {
		children = append(children, Leaf(Identifier, tokens[i].Serialize()))
	}
	children = append(children, Leaf(WhiteSpace, ind), Leaf(RightBrace, "}"), Leaf(NewLine, "\n"))
	e.fs.AddTree(IRTree(SwitchStatement, KindStmt, children...))
}

func (e *JSEmitter) PreVisitCaseClause(node *ast.CaseClause, indent int) {
	e.indent = indent
}

func (e *JSEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	exprCode := e.fs.CollectText(string(PreVisitCaseClauseListExpr))
	e.fs.AddLeaf(exprCode, KindExpr, nil)
}

func (e *JSEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
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

func (e *JSEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitCaseClause))
	ind := jsIndent(indent / 2)

	var children []IRNode
	idx := 0
	if len(node.List) == 0 {
		// Default case: AddLeaf("") is a no-op (empty tokens are dropped),
		// so all tokens on the stack are body statements.
		children = append(children, Leaf(WhiteSpace, ind), Leaf(DefaultKeyword, "default"), Leaf(Colon, ":"), Leaf(NewLine, "\n"))
	} else {
		// Regular case: token[0] is case expressions, rest is body
		caseExprs := ""
		if idx < len(tokens) {
			caseExprs = tokens[idx].Serialize()
			idx++
		}
		vals := strings.Split(caseExprs, ", ")
		for _, v := range vals {
			children = append(children, Leaf(WhiteSpace, ind), Leaf(CaseKeyword, "case"), Leaf(WhiteSpace, " "), Leaf(Identifier, v), Leaf(Colon, ":"), Leaf(NewLine, "\n"))
		}
	}
	// Case body statements
	for i := idx; i < len(tokens); i++ {
		children = append(children, Leaf(Identifier, tokens[i].Serialize()))
	}
	// Add break unless the last statement is a return or break
	bodyStr := IRTree(CaseClauseStatement, KindStmt, children...).Serialize()
	if !strings.Contains(bodyStr, "return ") && !strings.Contains(bodyStr, "break;") {
		children = append(children, Leaf(WhiteSpace, ind+"  "), Leaf(BreakKeyword, "break"), Leaf(Semicolon, ";"), Leaf(NewLine, "\n"))
	}
	e.fs.AddTree(IRTree(CaseClauseStatement, KindStmt, children...))
}

// ============================================================
// Inc/Dec Statements
// ============================================================

func (e *JSEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	xCode := e.fs.CollectText(string(PreVisitIncDecStmt))
	ind := jsIndent(indent / 2)
	e.fs.AddTree(IRTree(IncDecStatement, KindStmt,
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

func (e *JSEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	ind := jsIndent(indent / 2)
	switch node.Tok {
	case token.BREAK:
		e.fs.AddTree(IRTree(BranchStatement, KindStmt, Leaf(Identifier, ind+"break;\n")))
	case token.CONTINUE:
		e.fs.AddTree(IRTree(BranchStatement, KindStmt, Leaf(Identifier, ind+"continue;\n")))
	}
}

// ============================================================
// Struct Declarations (GenStructInfo)
// ============================================================

func (e *JSEmitter) PostVisitGenStructInfos(node []GenTypeInfo, indent int) {
	// After all struct classes are emitted, open the namespace object if needed
	if e.inNamespace {
		var children []IRNode
		children = append(children, Leaf(NewLine, "\n"), Leaf(Identifier, "const"), Leaf(WhiteSpace, " "), Leaf(Identifier, e.currentPackage), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
		// Add struct class references into the namespace
		for _, ti := range node {
			if ti.Struct != nil {
				children = append(children, Leaf(WhiteSpace, "  "), Leaf(Identifier, ti.Name), Leaf(Colon, ":"), Leaf(WhiteSpace, " "), Leaf(Identifier, ti.Name), Leaf(Comma, ","), Leaf(NewLine, "\n"))
			}
		}
		e.fs.AddTree(IRTree(StructTypeNode, KindType, children...))
	}
}

func (e *JSEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	// Discard type tokens
	e.fs.CollectForest(string(PreVisitGenStructFieldType))
}

func (e *JSEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	// Discard traversed ident, push name directly
	e.fs.CollectForest(string(PreVisitGenStructFieldName))
	e.fs.AddLeaf(node.Name, TagIdent, nil)
}

func (e *JSEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {
	tokens := e.fs.CollectForest(string(PreVisitGenStructInfo))

	if node.Struct == nil {
		return
	}

	// Collect field names
	var fieldNames []string
	for _, t := range tokens {
		if t.Kind == TagIdent {
			fieldNames = append(fieldNames, t.Serialize())
		}
	}

	// Generate JS class with default parameter values
	// Build a map from field name to its default value based on AST type
	fieldDefaults := make(map[string]string)
	if node.Struct != nil {
		for _, field := range node.Struct.Fields.List {
			defVal := e.jsDefaultForASTType(field.Type)
			for _, name := range field.Names {
				fieldDefaults[name.Name] = defVal
			}
		}
	}

	var children []IRNode
	children = append(children, Leaf(ClassKeyword, "class"), Leaf(WhiteSpace, " "), Leaf(Identifier, node.Name), Leaf(WhiteSpace, " "), Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
	var ctorParams []string
	for _, name := range fieldNames {
		if defVal, ok := fieldDefaults[name]; ok {
			ctorParams = append(ctorParams, fmt.Sprintf("%s = %s", name, defVal))
		} else {
			ctorParams = append(ctorParams, name)
		}
	}
	children = append(children, Leaf(WhiteSpace, "  "), Leaf(Identifier, "constructor"), Leaf(LeftParen, "("), Leaf(Identifier, strings.Join(ctorParams, ", ")), Leaf(RightParen, ")"), Leaf(WhiteSpace, " "), Leaf(LeftBrace, "{"), Leaf(NewLine, "\n"))
	for _, name := range fieldNames {
		children = append(children, Leaf(WhiteSpace, "    "), Leaf(Identifier, "this"), Leaf(Dot, "."), Leaf(Identifier, name), Leaf(WhiteSpace, " "), Leaf(Assignment, "="), Leaf(WhiteSpace, " "), Leaf(Identifier, name), Leaf(Semicolon, ";"), Leaf(NewLine, "\n"))
	}
	children = append(children, Leaf(WhiteSpace, "  "), Leaf(RightBrace, "}"), Leaf(NewLine, "\n"))
	children = append(children, Leaf(RightBrace, "}"), Leaf(NewLine, "\n"), Leaf(NewLine, "\n"))

	// Classes are always pushed to the stack - they come before the namespace opener
	// (PostVisitGenStructInfos opens the namespace after all struct classes)
	e.fs.AddTree(IRTree(TypeKeyword, TagExpr, children...))
}

// ============================================================
// Constants (GenDeclConst)
// ============================================================

func (e *JSEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	valTokens := e.fs.CollectForest(string(PreVisitGenDeclConstName))
	valCode := ""
	for _, t := range valTokens {
		valCode += t.Serialize()
	}
	if valCode == "" {
		valCode = "0"
	}
	name := node.Name
	if e.inNamespace {
		e.fs.AddTree(IRTree(TypeKeyword, TagExpr,
			Leaf(Identifier, name),
			Leaf(Colon, ":"),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, valCode),
			Leaf(Comma, ","),
			Leaf(NewLine, "\n"),
		))
	} else {
		e.fs.AddTree(IRTree(TypeKeyword, TagExpr,
			Leaf(Identifier, "const"),
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
}

func (e *JSEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {
	// Let const tokens flow through
}

// ============================================================
// Type Aliases (suppressed for JS)
// ============================================================

func (e *JSEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	// Discard type alias - no output for JS
	e.fs.CollectForest(string(PreVisitTypeAliasName))
}

// ensureNpmDependencies generates package.json and runs npm install if runtime
// packages require third-party npm dependencies (e.g. net runtime needs deasync).
func (e *JSEmitter) ensureNpmDependencies() {
	if _, hasNet := e.RuntimePackages["net"]; !hasNet {
		return
	}
	pkgJsonPath := filepath.Join(e.OutputDir, "package.json")
	if _, err := os.Stat(pkgJsonPath); os.IsNotExist(err) {
		os.WriteFile(pkgJsonPath, []byte("{\"private\":true,\"dependencies\":{\"deasync\":\"^0.1.30\"}}\n"), 0644)
	}
	deasyncDir := filepath.Join(e.OutputDir, "node_modules", "deasync")
	if _, err := os.Stat(deasyncDir); os.IsNotExist(err) {
		cmd := exec.Command("npm", "install", "--no-audit", "--no-fund")
		cmd.Dir = e.OutputDir
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("Warning: npm install failed in %s: %v\n%s", e.OutputDir, err, out)
		}
	}
}
