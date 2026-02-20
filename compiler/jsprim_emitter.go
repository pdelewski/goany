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

// JSPrimEmitter implements the Emitter interface using a shift/reduce architecture.
// PreVisit methods push markers, PostVisit methods reduce and push results.
type JSPrimEmitter struct {
	fs              *FragmentStack
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

func (e *JSPrimEmitter) SetFile(file *os.File) { e.file = file }
func (e *JSPrimEmitter) GetFile() *os.File     { return e.file }

// indentStr returns indentation string for the given level.
func jsprimIndent(indent int) string {
	return strings.Repeat("  ", indent)
}

// concatTokenContents concatenates Content fields of tokens.
func concatTokenContents(tokens []Token) string {
	var sb strings.Builder
	for _, t := range tokens {
		sb.WriteString(t.Content)
	}
	return sb.String()
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
func jsprimLowerBuiltin(selector string) string {
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
func (e *JSPrimEmitter) isMapTypeExpr(expr ast.Expr) bool {
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
func (e *JSPrimEmitter) getExprGoType(expr ast.Expr) types.Type {
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

func (e *JSPrimEmitter) PreVisitProgram(indent int) {
	var err error
	e.file, err = os.Create(e.Output)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}

	e.fs = NewFragmentStack(e.GetGoFIR())

	// Write JS header with runtime helpers
	e.file.WriteString(`// Generated JavaScript code (jsprim backend)
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

func (e *JSPrimEmitter) PostVisitProgram(indent int) {
	// Reduce everything from program marker
	tokens := e.fs.Reduce(string(PreVisitProgram))
	// Write all accumulated code
	for _, t := range tokens {
		e.file.WriteString(t.Content)
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
}

// createHTMLWrapper generates an HTML file that loads the generated JS.
func (e *JSPrimEmitter) createHTMLWrapper() {
	htmlFile := strings.TrimSuffix(e.Output, ".jsprim.js") + ".jsprim.html"
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
func (e *JSPrimEmitter) replaceStructKeyFunctions() {
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

func (e *JSPrimEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	e.pkg = pkg
	e.currentPackage = pkg.Name
	if pkg.Name != "main" {
		e.inNamespace = true
		// Write namespace opener directly - structs will be written before it
	}
}

func (e *JSPrimEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	if pkg.Name != "main" {
		e.fs.PushCode("};\n")
		e.inNamespace = false
	}
	// Don't reduce package marker - let tokens flow up to program
}

// ============================================================
// Literals and Identifiers
// ============================================================

func (e *JSPrimEmitter) PreVisitBasicLit(node *ast.BasicLit, indent int) {
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
	e.fs.Push(val, TagLiteral, nil)
}

func (e *JSPrimEmitter) PreVisitIdent(node *ast.Ident, indent int) {
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
		e.fs.Push("string", TagType, nil)
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
	e.fs.Push(name, TagIdent, goType)
}


// ============================================================
// Binary Expressions
// ============================================================

func (e *JSPrimEmitter) PostVisitBinaryExprLeft(node ast.Expr, indent int) {
	left := e.fs.ReduceToCode(string(PreVisitBinaryExprLeft))
	e.fs.PushCode(left)
}

func (e *JSPrimEmitter) PostVisitBinaryExprRight(node ast.Expr, indent int) {
	right := e.fs.ReduceToCode(string(PreVisitBinaryExprRight))
	e.fs.PushCode(right)
}

func (e *JSPrimEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
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

	// Check for integer division
	if op == "/" {
		leftType := e.getExprGoType(node.X)
		if leftType != nil {
			if basic, ok := leftType.Underlying().(*types.Basic); ok {
				if basic.Info()&types.IsInteger != 0 {
					e.fs.PushCode(fmt.Sprintf("Math.trunc(%s %s %s)", left, op, right))
					return
				}
			}
		}
	}

	// Check for string indexing: str[i] in Go returns byte, use charCodeAt in JS
	e.fs.PushCode(fmt.Sprintf("%s %s %s", left, op, right))
}

// ============================================================
// Call Expressions
// ============================================================

func (e *JSPrimEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	funCode := e.fs.ReduceToCode(string(PreVisitCallExprFun))
	e.fs.PushCode(funCode)
}

func (e *JSPrimEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {
	argCode := e.fs.ReduceToCode(string(PreVisitCallExprArg))
	e.fs.PushCode(argCode)
}

func (e *JSPrimEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	argTokens := e.fs.Reduce(string(PreVisitCallExprArgs))
	var args []string
	for _, t := range argTokens {
		if t.Content != "" {
			args = append(args, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(args, ", "))
}

func (e *JSPrimEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {
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
		// len(x) → len(x) helper for slices/strings (null-safe), (x ? x.Size : 0) for maps
		if len(node.Args) > 0 && e.isMapTypeExpr(node.Args[0]) {
			e.fs.PushCode(fmt.Sprintf("(%s ? %s.Size : 0)", argsStr, argsStr))
		} else {
			e.fs.PushCode(fmt.Sprintf("len(%s)", argsStr))
		}
		return
	case "append":
		// append(slice, items...) → append(slice, items...)
		e.fs.PushCode(fmt.Sprintf("append(%s)", argsStr))
		return
	case "delete":
		// delete(m, k) → hmap.hashMapDelete(m, k)
		e.fs.PushCode(fmt.Sprintf("hmap.hashMapDelete(%s)", argsStr))
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
				e.fs.PushCode(fmt.Sprintf("hmap.newHashMap(%d)", keyTypeConst))
				return
			}
			if _, ok := node.Args[0].(*ast.ArrayType); ok {
				// make([]T, n) → new Array(n).fill(default)
				parts := strings.SplitN(argsStr, ", ", 2)
				if len(parts) >= 2 {
					// First part is the type (already traversed), second is the size
					e.fs.PushCode(fmt.Sprintf("new Array(%s).fill(0)", parts[1]))
				} else {
					e.fs.PushCode("[]")
				}
				return
			}
		}
		e.fs.PushCode(fmt.Sprintf("make(%s)", argsStr))
		return
	}

	// Lower builtins (fmt.Println → console.log, etc.)
	lowered := jsprimLowerBuiltin(funName)
	if lowered != funName {
		funName = lowered
	}

	e.fs.PushCode(fmt.Sprintf("%s(%s)", funName, argsStr))
}

// ============================================================
// Selector Expressions (a.b)
// ============================================================

func (e *JSPrimEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitSelectorExprX))
	e.fs.PushCode(xCode)
}

func (e *JSPrimEmitter) PostVisitSelectorExprSel(node *ast.Ident, indent int) {
	// Discard the traversed ident, use node.Name directly
	e.fs.Reduce(string(PreVisitSelectorExprSel))
	e.fs.PushCode(node.Name)
}

func (e *JSPrimEmitter) PostVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSelectorExpr))
	xCode := ""
	selCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Content
	}
	if len(tokens) >= 2 {
		selCode = tokens[1].Content
	}

	// Lower builtins: fmt.Println → console.log
	loweredX := jsprimLowerBuiltin(xCode)
	loweredSel := jsprimLowerBuiltin(selCode)

	if loweredX == "" {
		// Package selector like fmt is suppressed
		e.fs.PushCode(loweredSel)
	} else {
		e.fs.PushCode(loweredX + "." + loweredSel)
	}
}

// ============================================================
// Index Expressions (a[i])
// ============================================================

func (e *JSPrimEmitter) PostVisitIndexExprX(node *ast.IndexExpr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitIndexExprX))
	e.fs.PushCode(xCode)
	// Save for potential map assignment detection
	e.lastIndexXCode = xCode
}

func (e *JSPrimEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	idxCode := e.fs.ReduceToCode(string(PreVisitIndexExprIndex))
	e.fs.PushCode(idxCode)
	e.lastIndexKeyCode = idxCode
}

func (e *JSPrimEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitIndexExpr))
	xCode := ""
	idxCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Content
	}
	if len(tokens) >= 2 {
		idxCode = tokens[1].Content
	}

	// Check if this is a map index (read)
	if e.isMapTypeExpr(node.X) {
		e.fs.PushCodeWithType(fmt.Sprintf("hmap.hashMapGet(%s, %s)", xCode, idxCode), e.getExprGoType(node))
	} else {
		// Check for string indexing: str[i] returns byte in Go
		xType := e.getExprGoType(node.X)
		if xType != nil {
			if basic, ok := xType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
				e.fs.PushCode(fmt.Sprintf("%s.charCodeAt(%s)", xCode, idxCode))
				return
			}
		}
		e.fs.PushCode(fmt.Sprintf("%s[%s]", xCode, idxCode))
	}
}

// ============================================================
// Unary Expressions
// ============================================================

func (e *JSPrimEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitUnaryExpr))
	op := node.Op.String()
	if op == "^" {
		// Go's ^ is bitwise NOT (like ~ in JS)
		e.fs.PushCode("~" + xCode)
	} else {
		e.fs.PushCode(op + xCode)
	}
}

// ============================================================
// Paren Expressions
// ============================================================

func (e *JSPrimEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {
	inner := e.fs.ReduceToCode(string(PreVisitParenExpr))
	e.fs.PushCode("(" + inner + ")")
}

// ============================================================
// Composite Literals (struct{}, []int{}, map[K]V{})
// ============================================================

func (e *JSPrimEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	// Discard type tokens for JS (no type annotations)
	e.fs.Reduce(string(PreVisitCompositeLitType))
}

func (e *JSPrimEmitter) PostVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	eltCode := e.fs.ReduceToCode(string(PreVisitCompositeLitElt))
	e.fs.PushCode(eltCode)
}

func (e *JSPrimEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	eltTokens := e.fs.Reduce(string(PreVisitCompositeLitElts))
	// Push each element back as an individual token (don't join)
	for _, t := range eltTokens {
		if t.Content != "" {
			e.fs.Push(t.Content, TagLiteral, nil)
		}
	}
}

func (e *JSPrimEmitter) PostVisitCompositeLit(node *ast.CompositeLit, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCompositeLit))
	// Collect individual element tokens (each is TagLiteral from PostVisitCompositeLitElts)
	var elts []string
	for _, t := range tokens {
		if t.Content != "" {
			elts = append(elts, t.Content)
		}
	}
	eltsStr := strings.Join(elts, ", ")

	// Determine type of composite literal
	litType := e.getExprGoType(node)
	if litType == nil {
		// Fallback: just use array syntax
		e.fs.PushCode("[" + eltsStr + "]")
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
				e.fs.PushCode(fmt.Sprintf("new %s(%s)", typeName, strings.Join(args, ", ")))
				return
			}
		}
		e.fs.PushCode(fmt.Sprintf("new %s(%s)", typeName, eltsStr))
	case *types.Slice:
		e.fs.PushCode("[" + eltsStr + "]")
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
			e.fs.PushCode(fmt.Sprintf("hmap.newHashMap(%d)", keyTypeConst))
		} else {
			// Each element is "key: value" from KeyValueExpr
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("(() => { let _m = hmap.newHashMap(%d); ", keyTypeConst))
			for _, elt := range elts {
				parts := strings.SplitN(elt, ": ", 2)
				if len(parts) == 2 {
					sb.WriteString(fmt.Sprintf("hmap.hashMapSet(_m, %s, %s); ", parts[0], parts[1]))
				}
			}
			sb.WriteString("return _m; })()")
			e.fs.PushCode(sb.String())
		}
	default:
		e.fs.PushCode("[" + eltsStr + "]")
	}
}

// ============================================================
// KeyValue Expressions (for composite literals)
// ============================================================

func (e *JSPrimEmitter) PostVisitKeyValueExprKey(node ast.Expr, indent int) {
	keyCode := e.fs.ReduceToCode(string(PreVisitKeyValueExprKey))
	e.fs.PushCode(keyCode)
}

func (e *JSPrimEmitter) PostVisitKeyValueExprValue(node ast.Expr, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitKeyValueExprValue))
	e.fs.PushCode(valCode)
}

func (e *JSPrimEmitter) PostVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitKeyValueExpr))
	keyCode := ""
	valCode := ""
	if len(tokens) >= 1 {
		keyCode = tokens[0].Content
	}
	if len(tokens) >= 2 {
		valCode = tokens[1].Content
	}
	e.fs.PushCode(keyCode + ": " + valCode)
}

// ============================================================
// Slice Expressions (a[lo:hi])
// ============================================================

func (e *JSPrimEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitSliceExprX))
	e.fs.PushCode(xCode)
}

func (e *JSPrimEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {
	// Discard the duplicated X traversal
	e.fs.Reduce(string(PreVisitSliceExprXBegin))
}

func (e *JSPrimEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	lowCode := e.fs.ReduceToCode(string(PreVisitSliceExprLow))
	e.fs.PushCode(lowCode)
}

func (e *JSPrimEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {
	// Discard the duplicated X traversal
	e.fs.Reduce(string(PreVisitSliceExprXEnd))
}

func (e *JSPrimEmitter) PostVisitSliceExprHigh(node ast.Expr, indent int) {
	highCode := e.fs.ReduceToCode(string(PreVisitSliceExprHigh))
	e.fs.PushCode(highCode)
}

func (e *JSPrimEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSliceExpr))
	xCode := ""
	lowCode := ""
	highCode := ""

	// Use AST node to determine which parts are present
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

	if highCode == "" {
		e.fs.PushCode(fmt.Sprintf("%s.slice(%s)", xCode, lowCode))
	} else {
		e.fs.PushCode(fmt.Sprintf("%s.slice(%s, %s)", xCode, lowCode, highCode))
	}
}

// ============================================================
// Array Type (used in composite literals, make calls)
// ============================================================

func (e *JSPrimEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	// Discard type info for JS
	e.fs.Reduce(string(PreVisitArrayType))
	e.fs.Push("[]", TagType, nil)
}

// ============================================================
// Map Type
// ============================================================

func (e *JSPrimEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitMapKeyType))
}

func (e *JSPrimEmitter) PostVisitMapValueType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitMapValueType))
}

func (e *JSPrimEmitter) PostVisitMapType(node *ast.MapType, indent int) {
	e.fs.Reduce(string(PreVisitMapType))
	e.fs.Push("map", TagType, nil)
}

// ============================================================
// Function Literals (closures)
// ============================================================

func (e *JSPrimEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	// Discard the type tokens (e.g., "int") that traverseExpression(param.Type) pushed
	e.fs.Reduce(string(PreVisitFuncLitTypeParam))
	// Push the actual parameter names from node.Names
	for _, name := range node.Names {
		e.fs.Push(name.Name, TagIdent, nil)
	}
}

func (e *JSPrimEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	// Collect parameter names
	tokens := e.fs.Reduce(string(PreVisitFuncLitTypeParams))
	var paramNames []string
	for _, t := range tokens {
		if t.Tag == TagIdent && t.Content != "" {
			paramNames = append(paramNames, t.Content)
		}
	}
	// Use a sentinel space for empty params so the token doesn't get dropped
	paramsStr := strings.Join(paramNames, ", ")
	if paramsStr == "" {
		paramsStr = " "
	}
	e.fs.PushCode(paramsStr)
}

func (e *JSPrimEmitter) PostVisitFuncLitTypeResults(node *ast.FieldList, indent int) {
	// Discard result types for JS
	e.fs.Reduce(string(PreVisitFuncLitTypeResults))
}

func (e *JSPrimEmitter) PostVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitFuncLitBody))
	e.fs.PushCode(bodyCode)
}

func (e *JSPrimEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncLit))
	paramsCode := ""
	bodyCode := ""
	if len(tokens) >= 1 {
		paramsCode = strings.TrimSpace(tokens[0].Content)
	}
	if len(tokens) >= 2 {
		bodyCode = tokens[1].Content
	}
	e.fs.PushCode(fmt.Sprintf("(%s) => %s", paramsCode, bodyCode))
}

// ============================================================
// Type Assertions
// ============================================================

func (e *JSPrimEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	// Discard type for JS (type assertions are no-ops at runtime)
	e.fs.Reduce(string(PreVisitTypeAssertExprType))
}

func (e *JSPrimEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitTypeAssertExprX))
	e.fs.PushCode(xCode)
}

func (e *JSPrimEmitter) PostVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitTypeAssertExpr))
	xCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Content
	}
	// In JS, type assertions are pass-through
	e.fs.PushCode(xCode)
}

// ============================================================
// Function Declarations
// ============================================================

func (e *JSPrimEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	// Track number of results for multi-value return handling
	e.numFuncResults = 0
	if node.Type.Results != nil {
		e.numFuncResults = node.Type.Results.NumFields()
	}
}

func (e *JSPrimEmitter) PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	// Discard result type tokens for JS
	e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeResultsList))
}

func (e *JSPrimEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	// Discard all result type tokens for JS
	e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeResults))
}

func (e *JSPrimEmitter) PostVisitFuncDeclName(node *ast.Ident, indent int) {
	// Discard traversed ident, use node.Name directly
	e.fs.Reduce(string(PreVisitFuncDeclName))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *JSPrimEmitter) PostVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {
	// Discard type tokens for JS
	e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeParamsListType))
}

func (e *JSPrimEmitter) PostVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	// Discard traversed ident, push name directly
	e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeParamsArgName))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *JSPrimEmitter) PostVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	// Collect param names from this field group
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeParamsList))
	for _, t := range tokens {
		if t.Tag == TagIdent {
			e.fs.Push(t.Content, TagIdent, nil)
		}
	}
}

func (e *JSPrimEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeParams))
	var paramNames []string
	for _, t := range tokens {
		if t.Tag == TagIdent {
			paramNames = append(paramNames, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(paramNames, ", "))
}

func (e *JSPrimEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignature))
	funcName := ""
	paramsStr := ""
	for _, t := range tokens {
		if t.Tag == TagIdent && funcName == "" {
			funcName = t.Content
		} else if t.Tag == TagExpr {
			paramsStr = t.Content
		}
	}

	if e.inNamespace {
		e.fs.PushCode(fmt.Sprintf("\n%s(%s)", funcName, paramsStr))
	} else {
		e.fs.PushCode(fmt.Sprintf("\nfunction %s(%s)", funcName, paramsStr))
	}
}

func (e *JSPrimEmitter) PostVisitFuncDeclBody(node *ast.BlockStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitFuncDeclBody))
	e.fs.PushCode(bodyCode)
}

func (e *JSPrimEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDecl))
	sigCode := ""
	bodyCode := ""
	if len(tokens) >= 1 {
		sigCode = tokens[0].Content
	}
	if len(tokens) >= 2 {
		bodyCode = tokens[1].Content
	}

	if e.inNamespace {
		e.fs.PushCode(sigCode + " " + bodyCode + ",\n")
	} else {
		e.fs.PushCode(sigCode + " " + bodyCode + "\n")
	}
}

// ============================================================
// Forward Declaration Signatures (suppressed for JS)
// ============================================================

func (e *JSPrimEmitter) PostVisitFuncDeclSignatures(indent int) {
	// Discard all forward declaration signatures for JS
	e.fs.Reduce(string(PreVisitFuncDeclSignatures))
}

// ============================================================
// Block Statements
// ============================================================

func (e *JSPrimEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	e.indent = indent
}

func (e *JSPrimEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	itemCode := e.fs.ReduceToCode(string(PreVisitBlockStmtList))
	e.fs.PushCode(itemCode)
}

func (e *JSPrimEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitBlockStmt))
	var sb strings.Builder
	sb.WriteString("{\n")
	for _, t := range tokens {
		if t.Content != "" {
			sb.WriteString(t.Content)
		}
	}
	sb.WriteString(jsprimIndent(indent/2) + "}")
	e.fs.PushCode(sb.String())
}

// ============================================================
// Assignment Statements
// ============================================================

func (e *JSPrimEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	e.indent = indent
	// Reset map assign state
	e.mapAssignVar = ""
	e.mapAssignKey = ""
}

func (e *JSPrimEmitter) PostVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	lhsCode := e.fs.ReduceToCode(string(PreVisitAssignStmtLhsExpr))

	// Detect map assignment: if lhs is m[k] on a map type
	if indexExpr, ok := node.(*ast.IndexExpr); ok {
		if e.isMapTypeExpr(indexExpr.X) {
			// Save map var and key for PostVisitAssignStmt to generate hashMapSet
			e.mapAssignVar = e.lastIndexXCode
			e.mapAssignKey = e.lastIndexKeyCode
			e.fs.PushCode(lhsCode)
			return
		}
	}
	e.fs.PushCode(lhsCode)
}

func (e *JSPrimEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmtLhs))
	var lhsExprs []string
	for _, t := range tokens {
		if t.Content != "" {
			lhsExprs = append(lhsExprs, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(lhsExprs, ", "))
}

func (e *JSPrimEmitter) PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {
	rhsCode := e.fs.ReduceToCode(string(PreVisitAssignStmtRhsExpr))
	e.fs.PushCode(rhsCode)
}

func (e *JSPrimEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmtRhs))
	var rhsExprs []string
	for _, t := range tokens {
		if t.Content != "" {
			rhsExprs = append(rhsExprs, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(rhsExprs, ", "))
}

func (e *JSPrimEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmt))
	lhsStr := ""
	rhsStr := ""
	if len(tokens) >= 1 {
		lhsStr = tokens[0].Content
	}
	if len(tokens) >= 2 {
		rhsStr = tokens[1].Content
	}

	ind := jsprimIndent(indent / 2)
	tokStr := node.Tok.String()

	// Map assignment: m[k] = v → hmap.hashMapSet(m, k, v)
	if e.mapAssignVar != "" && e.mapAssignKey != "" {
		e.fs.PushCode(fmt.Sprintf("%shmap.hashMapSet(%s, %s, %s);\n", ind, e.mapAssignVar, e.mapAssignKey, rhsStr))
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
					e.fs.PushCode(fmt.Sprintf("%slet %s = hmap.hashMapContains(%s, %s);\n", ind, okName, mapName, keyStr))
					e.fs.PushCode(fmt.Sprintf("%slet %s = %s ? hmap.hashMapGet(%s, %s) : %s;\n", ind, valName, okName, mapName, keyStr, zeroVal))
				} else {
					e.fs.PushCode(fmt.Sprintf("%s%s = hmap.hashMapContains(%s, %s);\n", ind, okName, mapName, keyStr))
					e.fs.PushCode(fmt.Sprintf("%s%s = %s ? hmap.hashMapGet(%s, %s) : %s;\n", ind, valName, okName, mapName, keyStr, zeroVal))
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
				e.fs.PushCode(fmt.Sprintf("%slet %s = %s;\n", ind, valName, rhsStr))
				e.fs.PushCode(fmt.Sprintf("%slet %s = (%s !== null && %s !== undefined);\n", ind, okName, valName, valName))
			} else {
				e.fs.PushCode(fmt.Sprintf("%s%s = %s;\n", ind, valName, rhsStr))
				e.fs.PushCode(fmt.Sprintf("%s%s = (%s !== null && %s !== undefined);\n", ind, okName, valName, valName))
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
			e.fs.PushCode(fmt.Sprintf("%slet %s = %s;\n", ind, destructured, rhsStr))
		} else {
			e.fs.PushCode(fmt.Sprintf("%s%s = %s;\n", ind, destructured, rhsStr))
		}
		return
	}

	switch tokStr {
	case ":=":
		e.fs.PushCode(fmt.Sprintf("%slet %s = %s;\n", ind, lhsStr, rhsStr))
	case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
		e.fs.PushCode(fmt.Sprintf("%s%s %s %s;\n", ind, lhsStr, tokStr, rhsStr))
	default:
		e.fs.PushCode(fmt.Sprintf("%s%s = %s;\n", ind, lhsStr, rhsStr))
	}
}

// ============================================================
// Declaration Statements (var x int, var y = 5)
// ============================================================

func (e *JSPrimEmitter) PreVisitDeclStmt(node *ast.DeclStmt, indent int) {
	e.indent = indent
}

func (e *JSPrimEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	// Keep type info for default value determination
	tokens := e.fs.Reduce(string(PreVisitDeclStmtValueSpecType))
	typeStr := ""
	for _, t := range tokens {
		typeStr += t.Content
	}
	// Also carry the Go type for struct default value generation
	var goType types.Type
	if e.pkg != nil && e.pkg.TypesInfo != nil && index < len(node.Names) {
		if obj := e.pkg.TypesInfo.Defs[node.Names[index]]; obj != nil {
			goType = obj.Type()
		}
	}
	e.fs.Push(typeStr, TagType, goType)
}

func (e *JSPrimEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	// Discard traversed ident, use node.Name directly
	e.fs.Reduce(string(PreVisitDeclStmtValueSpecNames))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *JSPrimEmitter) PostVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitDeclStmtValueSpecValue))
	e.fs.Push(valCode, TagExpr, nil)
}

func (e *JSPrimEmitter) PostVisitDeclStmt(node *ast.DeclStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitDeclStmt))
	ind := jsprimIndent(indent / 2)

	// Parse tokens: types (TagType), names (TagIdent), values (TagExpr)
	// Pattern for each var: type, name, [value]
	// Multiple vars result in multiple type/name/value groups
	var sb strings.Builder
	i := 0
	for i < len(tokens) {
		typeStr := ""
		var goType types.Type
		nameStr := ""
		valueStr := ""

		// Read type
		if i < len(tokens) && tokens[i].Tag == TagType {
			typeStr = tokens[i].Content
			goType = tokens[i].GoType
			i++
		}
		// Read name
		if i < len(tokens) && tokens[i].Tag == TagIdent {
			nameStr = tokens[i].Content
			i++
		}
		// Read value (optional)
		if i < len(tokens) && tokens[i].Tag == TagExpr {
			valueStr = tokens[i].Content
			i++
		}

		if nameStr == "" {
			continue
		}

		if valueStr != "" {
			sb.WriteString(fmt.Sprintf("%slet %s = %s;\n", ind, nameStr, valueStr))
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
			sb.WriteString(fmt.Sprintf("%slet %s = %s;\n", ind, nameStr, defaultVal))
		}
	}
	e.fs.PushCode(sb.String())
}

// defaultForTypeStr returns the JS default value for a Go type name string.
func (e *JSPrimEmitter) defaultForTypeStr(typeStr string) string {
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

// jsprimDefaultForASTType returns the JS default value for a Go AST type expression.
func (e *JSPrimEmitter) jsprimDefaultForASTType(expr ast.Expr) string {
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

func (e *JSPrimEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	e.indent = indent
}

func (e *JSPrimEmitter) PostVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	resultCode := e.fs.ReduceToCode(string(PreVisitReturnStmtResult))
	e.fs.PushCode(resultCode)
}

func (e *JSPrimEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitReturnStmt))
	ind := jsprimIndent(indent / 2)

	if len(tokens) == 0 {
		e.fs.PushCode(ind + "return;\n")
	} else if len(tokens) == 1 {
		e.fs.PushCode(fmt.Sprintf("%sreturn %s;\n", ind, tokens[0].Content))
	} else {
		// Multi-value return: return [a, b]
		var vals []string
		for _, t := range tokens {
			vals = append(vals, t.Content)
		}
		e.fs.PushCode(fmt.Sprintf("%sreturn [%s];\n", ind, strings.Join(vals, ", ")))
	}
}

// ============================================================
// Expression Statements
// ============================================================

func (e *JSPrimEmitter) PreVisitExprStmt(node *ast.ExprStmt, indent int) {
	e.indent = indent
}

func (e *JSPrimEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitExprStmtX))
	e.fs.PushCode(xCode)
}

func (e *JSPrimEmitter) PostVisitExprStmt(node *ast.ExprStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitExprStmt))
	code := ""
	if len(tokens) >= 1 {
		code = tokens[0].Content
	}
	ind := jsprimIndent(indent / 2)
	e.fs.PushCode(ind + code + ";\n")
}

// ============================================================
// If Statements
// ============================================================

func (e *JSPrimEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	e.indent = indent
	// Push new stack frame for nesting support
	e.ifInitStack = append(e.ifInitStack, "")
	e.ifCondStack = append(e.ifCondStack, "")
	e.ifBodyStack = append(e.ifBodyStack, "")
	e.ifElseStack = append(e.ifElseStack, "")
}

func (e *JSPrimEmitter) PostVisitIfStmtInit(node ast.Stmt, indent int) {
	e.ifInitStack[len(e.ifInitStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtInit))
}

func (e *JSPrimEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	e.ifCondStack[len(e.ifCondStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtCond))
}

func (e *JSPrimEmitter) PostVisitIfStmtBody(node *ast.IfStmt, indent int) {
	e.ifBodyStack[len(e.ifBodyStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtBody))
}

func (e *JSPrimEmitter) PostVisitIfStmtElse(node *ast.IfStmt, indent int) {
	e.ifElseStack[len(e.ifElseStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtElse))
}

func (e *JSPrimEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	e.fs.Reduce(string(PreVisitIfStmt))
	ind := jsprimIndent(indent / 2)

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

	var sb strings.Builder
	if initCode != "" {
		// Wrap in block scope to avoid variable redeclaration conflicts
		sb.WriteString(fmt.Sprintf("%s{\n", ind))
		sb.WriteString(initCode)
		sb.WriteString(fmt.Sprintf("%sif (%s) %s", ind, condCode, bodyCode))
	} else {
		sb.WriteString(fmt.Sprintf("%sif (%s) %s", ind, condCode, bodyCode))
	}
	if elseCode != "" {
		// Check if else is another if statement (else if chain)
		trimmed := strings.TrimLeft(elseCode, " \t\n")
		if strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "if(") {
			sb.WriteString(" else " + trimmed)
		} else {
			sb.WriteString(" else " + elseCode)
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

func (e *JSPrimEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	e.indent = indent
	// Push new stack frame for nesting support
	e.forInitStack = append(e.forInitStack, "")
	e.forCondStack = append(e.forCondStack, "")
	e.forPostStack = append(e.forPostStack, "")
}

func (e *JSPrimEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	initCode := e.fs.ReduceToCode(string(PreVisitForStmtInit))
	initCode = strings.TrimRight(initCode, ";\n \t")
	initCode = strings.TrimLeft(initCode, " \t")
	e.forInitStack[len(e.forInitStack)-1] = initCode
}

func (e *JSPrimEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	e.forCondStack[len(e.forCondStack)-1] = e.fs.ReduceToCode(string(PreVisitForStmtCond))
}

func (e *JSPrimEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	postCode := e.fs.ReduceToCode(string(PreVisitForStmtPost))
	postCode = strings.TrimRight(postCode, ";\n \t")
	postCode = strings.TrimLeft(postCode, " \t")
	e.forPostStack[len(e.forPostStack)-1] = postCode
}

func (e *JSPrimEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitForStmt))
	ind := jsprimIndent(indent / 2)

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
		e.fs.PushCode(fmt.Sprintf("%swhile (true) %s\n", ind, bodyCode))
		return
	}

	// While loop (only cond)
	if node.Init == nil && node.Post == nil && node.Cond != nil {
		e.fs.PushCode(fmt.Sprintf("%swhile (%s) %s\n", ind, condCode, bodyCode))
		return
	}

	e.fs.PushCode(fmt.Sprintf("%sfor (%s; %s; %s) %s\n", ind, initCode, condCode, postCode, bodyCode))
}

// ============================================================
// Range Statements
// ============================================================

func (e *JSPrimEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	e.indent = indent
}

func (e *JSPrimEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	keyCode := e.fs.ReduceToCode(string(PreVisitRangeStmtKey))
	e.fs.Push(keyCode, TagIdent, nil)
}

func (e *JSPrimEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitRangeStmtValue))
	e.fs.Push(valCode, TagIdent, nil)
}

func (e *JSPrimEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitRangeStmtX))
	e.fs.PushCode(xCode)
}

func (e *JSPrimEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitRangeStmt))
	ind := jsprimIndent(indent / 2)

	keyCode := ""
	valCode := ""
	xCode := ""
	bodyCode := ""

	idx := 0
	// Use AST node to determine which tokens are present
	// Sema pass sets Key=nil when key is blank identifier (_)
	if node.Key != nil {
		// key (TagIdent)
		if idx < len(tokens) && tokens[idx].Tag == TagIdent {
			keyCode = tokens[idx].Content
			idx++
		}
	}
	if node.Value != nil {
		// value (TagIdent)
		if idx < len(tokens) && tokens[idx].Tag == TagIdent {
			valCode = tokens[idx].Content
			idx++
		}
	}
	// collection (TagExpr)
	if idx < len(tokens) {
		xCode = tokens[idx].Content
		idx++
	}
	// body
	if idx < len(tokens) {
		bodyCode = tokens[idx].Content
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
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("%s{\n", ind))
			sb.WriteString(fmt.Sprintf("%s  let _keys = hmap.hashMapKeys(%s);\n", ind, xCode))
			sb.WriteString(fmt.Sprintf("%s  for (let _i = 0; _i < _keys.length; _i++) {\n", ind))
			if keyCode != "_" {
				sb.WriteString(fmt.Sprintf("%s    let %s = _keys[_i];\n", ind, keyCode))
			}
			sb.WriteString(fmt.Sprintf("%s    let %s = hmap.hashMapGet(%s, _keys[_i]);\n", ind, valCode, xCode))
			sb.WriteString(fmt.Sprintf("%s    %s\n", ind, bodyCode))
			sb.WriteString(fmt.Sprintf("%s  }\n", ind))
			sb.WriteString(fmt.Sprintf("%s}\n", ind))
			e.fs.PushCode(sb.String())
		} else {
			// Key-only range on map
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("%s{\n", ind))
			sb.WriteString(fmt.Sprintf("%s  let _keys = hmap.hashMapKeys(%s);\n", ind, xCode))
			sb.WriteString(fmt.Sprintf("%s  for (let _i = 0; _i < _keys.length; _i++) {\n", ind))
			if keyCode != "_" {
				sb.WriteString(fmt.Sprintf("%s    let %s = _keys[_i];\n", ind, keyCode))
			}
			sb.WriteString(fmt.Sprintf("%s    %s\n", ind, bodyCode))
			sb.WriteString(fmt.Sprintf("%s  }\n", ind))
			sb.WriteString(fmt.Sprintf("%s}\n", ind))
			e.fs.PushCode(sb.String())
		}
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

		e.fs.PushCode(fmt.Sprintf("%sfor (let %s = 0; %s < %s.length; %s++) %s\n",
			ind, loopVar, loopVar, xCode, loopVar, bodyWithDecl))
	} else {
		// Key-only range
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = "_i"
		}
		e.fs.PushCode(fmt.Sprintf("%sfor (let %s = 0; %s < %s.length; %s++) %s\n",
			ind, loopVar, loopVar, xCode, loopVar, bodyCode))
	}
}

// ============================================================
// Switch / Case Statements
// ============================================================

func (e *JSPrimEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	e.indent = indent
}

func (e *JSPrimEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	tagCode := e.fs.ReduceToCode(string(PreVisitSwitchStmtTag))
	e.fs.PushCode(tagCode)
}

func (e *JSPrimEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSwitchStmt))
	ind := jsprimIndent(indent / 2)

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

func (e *JSPrimEmitter) PreVisitCaseClause(node *ast.CaseClause, indent int) {
	e.indent = indent
}

func (e *JSPrimEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	exprCode := e.fs.ReduceToCode(string(PreVisitCaseClauseListExpr))
	e.fs.PushCode(exprCode)
}

func (e *JSPrimEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCaseClauseList))
	var exprs []string
	for _, t := range tokens {
		if t.Content != "" {
			exprs = append(exprs, t.Content)
		}
	}
	e.fs.PushCode(strings.Join(exprs, ", "))
}

func (e *JSPrimEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCaseClause))
	ind := jsprimIndent(indent / 2)

	caseExprs := ""
	idx := 0
	if idx < len(tokens) {
		caseExprs = tokens[idx].Content
		idx++
	}

	var sb strings.Builder
	if len(node.List) == 0 {
		// Default case
		sb.WriteString(ind + "default:\n")
	} else {
		// Regular case - handle multiple values
		vals := strings.Split(caseExprs, ", ")
		for _, v := range vals {
			sb.WriteString(fmt.Sprintf("%scase %s:\n", ind, v))
		}
	}
	// Case body statements
	for i := idx; i < len(tokens); i++ {
		sb.WriteString(tokens[i].Content)
	}
	// Add break unless the last statement is a return or break
	bodyStr := sb.String()
	if !strings.Contains(bodyStr, "return ") && !strings.Contains(bodyStr, "break;") {
		sb.WriteString(ind + "  break;\n")
	}
	e.fs.PushCode(sb.String())
}

// ============================================================
// Inc/Dec Statements
// ============================================================

func (e *JSPrimEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitIncDecStmt))
	ind := jsprimIndent(indent / 2)
	e.fs.PushCode(fmt.Sprintf("%s%s%s;\n", ind, xCode, node.Tok.String()))
}

// ============================================================
// Branch Statements (break, continue)
// ============================================================

func (e *JSPrimEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	ind := jsprimIndent(indent / 2)
	switch node.Tok {
	case token.BREAK:
		e.fs.PushCode(ind + "break;\n")
	case token.CONTINUE:
		e.fs.PushCode(ind + "continue;\n")
	}
}

// ============================================================
// Struct Declarations (GenStructInfo)
// ============================================================


func (e *JSPrimEmitter) PostVisitGenStructInfos(node []GenTypeInfo, indent int) {
	// After all struct classes are emitted, open the namespace object if needed
	if e.inNamespace {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("\nconst %s = {\n", e.currentPackage))
		// Add struct class references into the namespace
		for _, ti := range node {
			if ti.Struct != nil {
				sb.WriteString(fmt.Sprintf("  %s: %s,\n", ti.Name, ti.Name))
			}
		}
		e.fs.PushCode(sb.String())
	}
}

func (e *JSPrimEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	// Discard type tokens
	e.fs.Reduce(string(PreVisitGenStructFieldType))
}

func (e *JSPrimEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	// Discard traversed ident, push name directly
	e.fs.Reduce(string(PreVisitGenStructFieldName))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *JSPrimEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {
	tokens := e.fs.Reduce(string(PreVisitGenStructInfo))

	if node.Struct == nil {
		return
	}

	// Collect field names
	var fieldNames []string
	for _, t := range tokens {
		if t.Tag == TagIdent {
			fieldNames = append(fieldNames, t.Content)
		}
	}

	// Generate JS class with default parameter values
	// Build a map from field name to its default value based on AST type
	fieldDefaults := make(map[string]string)
	if node.Struct != nil {
		for _, field := range node.Struct.Fields.List {
			defVal := e.jsprimDefaultForASTType(field.Type)
			for _, name := range field.Names {
				fieldDefaults[name.Name] = defVal
			}
		}
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("class %s {\n", node.Name))
	var ctorParams []string
	for _, name := range fieldNames {
		if defVal, ok := fieldDefaults[name]; ok {
			ctorParams = append(ctorParams, fmt.Sprintf("%s = %s", name, defVal))
		} else {
			ctorParams = append(ctorParams, name)
		}
	}
	sb.WriteString("  constructor(" + strings.Join(ctorParams, ", ") + ") {\n")
	for _, name := range fieldNames {
		sb.WriteString(fmt.Sprintf("    this.%s = %s;\n", name, name))
	}
	sb.WriteString("  }\n")
	sb.WriteString("}\n\n")

	// Classes are always pushed to the stack - they come before the namespace opener
	// (PostVisitGenStructInfos opens the namespace after all struct classes)
	e.fs.PushCode(sb.String())
}

// ============================================================
// Constants (GenDeclConst)
// ============================================================

func (e *JSPrimEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	valTokens := e.fs.Reduce(string(PreVisitGenDeclConstName))
	valCode := ""
	for _, t := range valTokens {
		valCode += t.Content
	}
	if valCode == "" {
		valCode = "0"
	}
	name := node.Name
	if e.inNamespace {
		e.fs.PushCode(fmt.Sprintf("%s: %s,\n", name, valCode))
	} else {
		e.fs.PushCode(fmt.Sprintf("const %s = %s;\n", name, valCode))
	}
}

func (e *JSPrimEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {
	// Let const tokens flow through
}

// ============================================================
// Type Aliases (suppressed for JS)
// ============================================================

func (e *JSPrimEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	// Discard type alias - no output for JS
	e.fs.Reduce(string(PreVisitTypeAliasName))
}

