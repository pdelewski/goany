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
)

var jsTypesMap = map[string]string{
	"int8":   "number",
	"int16":  "number",
	"int32":  "number",
	"int64":  "number",
	"int":    "number",
	"uint8":  "number",
	"uint16": "number",
	"uint32": "number",
	"uint64": "number",
	"float32": "number",
	"float64": "number",
	"bool":   "boolean",
	"string": "string",
	"any":    "any",
}

type JSEmitter struct {
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string // Path to runtime directory (empty = disabled)
	GraphicsRuntime string // Graphics backend for browser
	file            *os.File
	Emitter
	pkg                   *packages.Package
	insideForPostCond     bool
	assignmentToken       string
	forwardDecl           bool
	currentPackage        string
	// Key-value range loop support
	isKeyValueRange       bool
	rangeKeyName          string
	rangeValueName        string
	rangeCollectionExpr   string
	captureRangeExpr      bool
	suppressRangeEmit     bool
	rangeStmtIndent       int
	pendingRangeValueDecl bool  // Emit value declaration in next block
	// For loop support
	sawIncrement          bool
	sawDecrement          bool
	forLoopInclusive      bool
	forLoopReverse        bool
	isInfiniteLoop        bool
	// Multi-value support
	inMultiValueReturn    bool
	multiValueReturnIndex int
	numFuncResults        int
	// Type suppression for JavaScript (no type annotations)
	suppressTypeEmit      bool
	// For loop init section (suppress semicolon after assignment)
	insideForInit         bool
	// Pending slice/struct/basic type initialization
	pendingSliceInit      bool
	pendingStructInit     bool
	pendingStructType     *types.Struct // Store struct type for full initialization
	pendingBasicInit      *types.Basic  // Store basic type for proper initialization
	// Namespace handling for non-main packages
	inNamespace           bool
	// Track when inside a function literal (closure) where 'this' has different meaning
	inFuncLit             bool
	// Integer division handling
	intDivision           bool
	// String indexing - use charCodeAt for string[i] to return number like Go
	isStringIndex         bool
	// Map support
	isMapMakeCall    bool   // Inside make(map[K]V) call
	mapMakeKeyType   int    // Key type constant for make call
	isMapIndex       bool   // IndexExpr on a map (for read: m[k])
	isMapAssign      bool   // Assignment to map index (m[k] = v)
	mapAssignVarName string // Variable name for map assignment
	mapAssignIndent  int    // Indent level for map assignment
	captureMapKey    bool   // Capturing map key expression
	capturedMapKey   string // Captured key expression text
	isDeleteCall     bool   // Inside delete(m, k) call
	deleteMapVarName string // Variable name for delete
	isMapLenCall     bool   // len() call on a map
	pendingMapInit   bool   // var m map[K]V needs default init
	pendingMapKeyType int   // Key type for pending init
	// Comma-ok idiom: val, ok := m[key]
	isMapCommaOk      bool
	mapCommaOkValName string
	mapCommaOkOkName  string
	mapCommaOkMapName string
	mapCommaOkZeroVal string
	mapCommaOkIsDecl  bool
	mapCommaOkIndent  int
	// Type assertion comma-ok: val, ok := x.(Type)
	isTypeAssertCommaOk      bool
	typeAssertCommaOkValName string
	typeAssertCommaOkOkName  string
	typeAssertCommaOkIsDecl  bool
	typeAssertCommaOkIndent  int
	// Slice make support: make([]T, n) → new Array(n).fill(default)
	isSliceMakeCall  bool   // Inside make([]T, n) call
	sliceMakeDefault string // Default fill value for the slice element type
	suppressEmit     bool   // When true, emitToFile does nothing
}

// getMapKeyTypeConst returns the key type constant for a map type AST node
func (jse *JSEmitter) getMapKeyTypeConst(mapType *ast.MapType) int {
	if jse.pkg != nil && jse.pkg.TypesInfo != nil {
		if tv, ok := jse.pkg.TypesInfo.Types[mapType.Key]; ok && tv.Type != nil {
			if basic, isBasic := tv.Type.Underlying().(*types.Basic); isBasic {
				switch basic.Kind() {
				case types.String:
					return 1 // KEY_TYPE_STRING
				case types.Int:
					return 2 // KEY_TYPE_INT
				case types.Bool:
					return 3 // KEY_TYPE_BOOL
				case types.Int8:
					return 4 // KEY_TYPE_INT8
				case types.Int16:
					return 5 // KEY_TYPE_INT16
				case types.Int32:
					return 6 // KEY_TYPE_INT32
				case types.Int64:
					return 7 // KEY_TYPE_INT64
				case types.Uint8:
					return 8 // KEY_TYPE_UINT8
				case types.Uint16:
					return 9 // KEY_TYPE_UINT16
				case types.Uint32:
					return 10 // KEY_TYPE_UINT32
				case types.Uint64:
					return 11 // KEY_TYPE_UINT64
				case types.Float32:
					return 12 // KEY_TYPE_FLOAT32
				case types.Float64:
					return 13 // KEY_TYPE_FLOAT64
				}
			}
		}
	}
	return 1 // Default to string
}

func (*JSEmitter) lowerToBuiltins(selector string) string {
	switch selector {
	case "fmt":
		return ""
	case "Sprintf":
		return "stringFormat"
	case "Println":
		return "console.log"
	case "Printf":
		return "printf" // Uses process.stdout.write (no newline)
	case "Print":
		return "print" // Uses process.stdout.write (no newline)
	case "len":
		return "len"
	}
	return selector
}

func (jse *JSEmitter) emitToFile(s string) error {
	if jse.captureRangeExpr {
		jse.rangeCollectionExpr += s
		return nil
	}
	if jse.captureMapKey {
		jse.capturedMapKey += s
		return nil
	}
	if jse.suppressEmit {
		return nil
	}
	if jse.suppressRangeEmit {
		return nil
	}
	return emitToFile(jse.file, s)
}

func (jse *JSEmitter) emitAsString(s string, indent int) string {
	return strings.Repeat("  ", indent) + s
}

func (jse *JSEmitter) SetFile(file *os.File) {
	jse.file = file
}

func (jse *JSEmitter) GetFile() *os.File {
	return jse.file
}

func (jse *JSEmitter) PreVisitProgram(indent int) {
	outputFile := jse.Output
	var err error
	jse.file, err = os.Create(outputFile)
	jse.SetFile(jse.file)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}

	// Write JavaScript header with runtime helpers
	jse.file.WriteString(`// Generated JavaScript code
"use strict";

// Runtime helpers
function len(arr) {
  if (typeof arr === 'string') return arr.length;
  if (Array.isArray(arr)) return arr.length;
  return 0;
}

function append(arr, ...items) {
  // Handle nil/undefined slices like Go does
  if (arr == null) arr = [];
  // Clone plain objects to preserve Go's value semantics for structs
  // Use push for O(1) amortized instead of spread which is O(n)
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

// printf - like fmt.Printf (no newline)
function printf(fmt, ...args) {
  const str = stringFormat(fmt, ...args);
  if (typeof process !== 'undefined' && process.stdout) {
    process.stdout.write(str);
  } else {
    // Browser fallback - accumulate output
    if (typeof window !== 'undefined') {
      window._printBuffer = (window._printBuffer || '') + str;
    }
  }
}

// print - like fmt.Print (no newline)
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
// Handle string-to-int conversion for character codes (Go rune semantics)
function int8(v) { return typeof v === 'string' ? v.charCodeAt(0) | 0 : v | 0; }
function int16(v) { return typeof v === 'string' ? v.charCodeAt(0) | 0 : v | 0; }
function int32(v) { return typeof v === 'string' ? v.charCodeAt(0) | 0 : v | 0; }
function int64(v) { return typeof v === 'string' ? v.charCodeAt(0) : v; }  // BigInt not used for simplicity
function int(v) { return typeof v === 'string' ? v.charCodeAt(0) | 0 : v | 0; }
function uint8(v) { return typeof v === 'string' ? v.charCodeAt(0) & 0xFF : (v | 0) & 0xFF; }
function uint16(v) { return typeof v === 'string' ? v.charCodeAt(0) & 0xFFFF : (v | 0) & 0xFFFF; }
function uint32(v) { return typeof v === 'string' ? v.charCodeAt(0) >>> 0 : (v | 0) >>> 0; }
function uint64(v) { return typeof v === 'string' ? v.charCodeAt(0) : v; }  // BigInt not used for simplicity
function float32(v) { return v; }
function float64(v) { return v; }
function string(v) { return String(v); }
function bool(v) { return Boolean(v); }

`)

	// Include graphics runtime if enabled
	if jse.LinkRuntime != "" {
		jse.file.WriteString(`// Graphics runtime for Canvas
const graphics = {
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
      // Store ASCII code for GetLastKey
      if (e.key.length === 1) {
        this.lastKey = e.key.charCodeAt(0);
      } else {
        // Map special keys to ASCII codes
        const specialKeys = {
          'Enter': 13,
          'Backspace': 8,
          'Tab': 9,
          'Escape': 27,
          'ArrowUp': 38,
          'ArrowDown': 40,
          'ArrowLeft': 37,
          'ArrowRight': 39,
          'Delete': 127,
          'Space': 32
        };
        if (specialKeys[e.key]) {
          this.lastKey = specialKeys[e.key];
        }
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

    this.windowObj = {
      canvas: this.canvas,
      width: width,
      height: height
    };

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

    this.windowObj = {
      canvas: this.canvas,
      width: this.canvas.width,
      height: this.canvas.height
    };

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

  // Color helper functions
  Red: function() { return { R: 255, G: 0, B: 0, A: 255 }; },
  Green: function() { return { R: 0, G: 255, B: 0, A: 255 }; },
  Blue: function() { return { R: 0, G: 0, B: 255, A: 255 }; },
  White: function() { return { R: 255, G: 255, B: 255, A: 255 }; },
  Black: function() { return { R: 0, G: 0, B: 0, A: 255 }; },

  Clear: function(canvas, color) {
    this.ctx.fillStyle = ` + "`rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`" + `;
    this.ctx.fillRect(0, 0, canvas.width, canvas.height);
  },

  FillRect: function(canvas, rect, color) {
    this.ctx.fillStyle = ` + "`rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`" + `;
    this.ctx.fillRect(rect.x, rect.y, rect.width, rect.height);
  },

  DrawRect: function(canvas, rect, color) {
    this.ctx.strokeStyle = ` + "`rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`" + `;
    this.ctx.strokeRect(rect.x, rect.y, rect.width, rect.height);
  },

  NewRect: function(x, y, width, height) {
    return { x, y, width, height };
  },

  FillCircle: function(canvas, centerX, centerY, radius, color) {
    this.ctx.fillStyle = ` + "`rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`" + `;
    this.ctx.beginPath();
    this.ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);
    this.ctx.fill();
  },

  DrawCircle: function(canvas, centerX, centerY, radius, color) {
    this.ctx.strokeStyle = ` + "`rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`" + `;
    this.ctx.beginPath();
    this.ctx.arc(centerX, centerY, radius, 0, Math.PI * 2);
    this.ctx.stroke();
  },

  DrawPoint: function(canvas, x, y, color) {
    this.ctx.fillStyle = ` + "`rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`" + `;
    this.ctx.fillRect(x, y, 1, 1);
  },

  DrawLine: function(canvas, x1, y1, x2, y2, color) {
    this.ctx.strokeStyle = ` + "`rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`" + `;
    this.ctx.beginPath();
    this.ctx.moveTo(x1, y1);
    this.ctx.lineTo(x2, y2);
    this.ctx.stroke();
  },

  SetPixel: function(canvas, x, y, color) {
    this.ctx.fillStyle = ` + "`rgba(${color.R}, ${color.G}, ${color.B}, ${color.A / 255})`" + `;
    this.ctx.fillRect(x, y, 1, 1);
  },

  PollEvents: function(canvas) {
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
    this.lastKey = 0;  // Clear after reading (like native backends)
    return key;
  },

  GetMousePos: function(canvas) {
    return [this.mouseX, this.mouseY];
  },

  GetMouse: function(canvas) {
    // Returns [x, y, buttons] like other backends
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
    // In browser context, don't immediately close - RunLoop is async
    // The canvas will remain until the page is closed
  },

  RunLoop: function(canvas, frameFunc) {
    const self = this;
    function loop() {
      if (!self.running) return;
      // Poll events (update internal state)
      const result = frameFunc(canvas);
      if (result === false) {
        self.running = false;
        return;
      }
      requestAnimationFrame(loop);
    }
    requestAnimationFrame(loop);
  },

  RunLoopWithState: function(canvas, state, frameFunc) {
    const self = this;
    function loop() {
      if (!self.running) return;
      // Call frameFunc with canvas and state, returns [newState, shouldContinue]
      const result = frameFunc(canvas, state);
      state = result[0];
      if (result[1] === false) {
        self.running = false;
        return;
      }
      requestAnimationFrame(loop);
    }
    requestAnimationFrame(loop);
  }
};

`)
	}
}

func (jse *JSEmitter) PostVisitProgram(indent int) {
	// Add main() call at the end
	jse.file.WriteString("\n// Run main\nmain();\n")
	jse.file.Close()

	// Create HTML wrapper if graphics runtime is enabled
	if jse.LinkRuntime != "" {
		jse.createHTMLWrapper()
	}
}

func (jse *JSEmitter) createHTMLWrapper() {
	htmlFile := strings.TrimSuffix(jse.Output, ".js") + ".html"
	f, err := os.Create(htmlFile)
	if err != nil {
		fmt.Println("Error creating HTML file:", err)
		return
	}
	defer f.Close()

	jsFileName := filepath.Base(jse.Output)
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
`, jse.OutputName, jsFileName))
}

func (jse *JSEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	jse.pkg = pkg
	jse.currentPackage = pkg.Name
	// For non-main packages, create a namespace object
	if pkg.Name != "main" {
		jse.inNamespace = true
		jse.emitToFile(fmt.Sprintf("\nconst %s = {\n", pkg.Name))
	}
}

func (jse *JSEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	// Close the namespace object for non-main packages
	if pkg.Name != "main" {
		jse.emitToFile("};\n")
		jse.inNamespace = false
	}
}

// PreVisitFuncDecl handles function declarations
func (jse *JSEmitter) PreVisitFuncDecl(node *ast.FuncDecl, indent int) {
	if jse.forwardDecl {
		return
	}
	if jse.inNamespace {
		// Inside a namespace object, use method syntax
		str := jse.emitAsString("\n", indent)
		jse.emitToFile(str)
	} else {
		str := jse.emitAsString("\nfunction ", indent)
		jse.emitToFile(str)
	}
}

func (jse *JSEmitter) PreVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	if jse.forwardDecl {
		return
	}
	// Count results for multi-value returns
	if node.Type.Results != nil {
		jse.numFuncResults = len(node.Type.Results.List)
	} else {
		jse.numFuncResults = 0
	}
}

// Suppress return type emission (JavaScript doesn't have return types)
func (jse *JSEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	jse.suppressTypeEmit = true
}

func (jse *JSEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	jse.suppressTypeEmit = false
}

func (jse *JSEmitter) PreVisitFuncDeclName(node *ast.Ident, indent int) {
	if jse.forwardDecl {
		return
	}
	if jse.inNamespace {
		// Method syntax: name: function
		jse.emitToFile(node.Name + ": function")
	} else {
		jse.emitToFile(node.Name)
	}
}

func (jse *JSEmitter) PreVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile("(")
}

func (jse *JSEmitter) PreVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	if jse.forwardDecl {
		return
	}
	if index > 0 {
		jse.emitToFile(", ")
	}
}

// Suppress parameter type emission (JavaScript doesn't have type annotations)
func (jse *JSEmitter) PreVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {
	jse.suppressTypeEmit = true
}

func (jse *JSEmitter) PostVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {
	jse.suppressTypeEmit = false
}

func (jse *JSEmitter) PreVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	// Arg name is emitted via PreVisitIdent when traversed
}

func (jse *JSEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile(") ")
}

func (jse *JSEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	// Nothing to do - closing paren is in PostVisitFuncDeclSignatureTypeParams
}

func (jse *JSEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	if jse.forwardDecl {
		return
	}
	if jse.inNamespace {
		// Add comma after method in namespace object
		jse.emitToFile(",\n")
	} else {
		jse.emitToFile("\n")
	}
}

// JavaScript doesn't need forward declarations (function hoisting handles this)
func (jse *JSEmitter) PreVisitFuncDeclSignatures(indent int) {
	jse.forwardDecl = true
}

func (jse *JSEmitter) PostVisitFuncDeclSignatures(indent int) {
	jse.forwardDecl = false
}

// Block statements
func (jse *JSEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile("{\n")
	// Emit pending range value declaration
	if jse.pendingRangeValueDecl {
		str := jse.emitAsString("let "+jse.rangeValueName+" = "+jse.rangeCollectionExpr+"["+jse.rangeKeyName+"];\n", indent+1)
		jse.emitToFile(str)
		jse.pendingRangeValueDecl = false
	}
}

func (jse *JSEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	str := jse.emitAsString("}\n", indent)
	jse.emitToFile(str)
}

// Assignment statements
func (jse *JSEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	// Detect comma-ok: val, ok := m[key]
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if indexExpr, ok := node.Rhs[0].(*ast.IndexExpr); ok {
			if jse.pkg != nil && jse.pkg.TypesInfo != nil {
				tv := jse.pkg.TypesInfo.Types[indexExpr.X]
				if tv.Type != nil {
					if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
						jse.isMapCommaOk = true
						jse.mapCommaOkValName = node.Lhs[0].(*ast.Ident).Name
						jse.mapCommaOkOkName = node.Lhs[1].(*ast.Ident).Name
						jse.mapCommaOkMapName = exprToString(indexExpr.X)
						jse.mapCommaOkIsDecl = (node.Tok == token.DEFINE)
						jse.mapCommaOkIndent = indent
						// Determine JS zero value for the map value type
						jse.mapCommaOkZeroVal = "null"
						if basic, isBasic := mapType.Elem().Underlying().(*types.Basic); isBasic {
							switch basic.Kind() {
							case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
								types.Uint8, types.Uint16, types.Uint32, types.Uint64,
								types.Float32, types.Float64:
								jse.mapCommaOkZeroVal = "0"
							case types.String:
								jse.mapCommaOkZeroVal = "\"\""
							case types.Bool:
								jse.mapCommaOkZeroVal = "false"
							}
						}
						jse.suppressEmit = true
						return
					}
				}
			}
		}
	}
	// Detect type assertion comma-ok: val, ok := x.(Type)
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if _, ok := node.Rhs[0].(*ast.TypeAssertExpr); ok {
			jse.isTypeAssertCommaOk = true
			jse.typeAssertCommaOkValName = node.Lhs[0].(*ast.Ident).Name
			jse.typeAssertCommaOkOkName = node.Lhs[1].(*ast.Ident).Name
			jse.typeAssertCommaOkIsDecl = (node.Tok == token.DEFINE)
			jse.typeAssertCommaOkIndent = indent
			jse.suppressEmit = true
			return
		}
	}
	// Check for map assignment: m[k] = v
	if len(node.Lhs) == 1 {
		if indexExpr, ok := node.Lhs[0].(*ast.IndexExpr); ok {
			if jse.pkg != nil && jse.pkg.TypesInfo != nil {
				tv := jse.pkg.TypesInfo.Types[indexExpr.X]
				if tv.Type != nil {
					if _, ok := tv.Type.Underlying().(*types.Map); ok {
						jse.isMapAssign = true
						jse.mapAssignIndent = indent
						jse.mapAssignVarName = exprToString(indexExpr.X)
						jse.suppressRangeEmit = true // Suppress LHS output
						return
					}
				}
			}
		}
	}
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
		jse.suppressRangeEmit = true // Reuse this flag to suppress entire statement
	}
}

func (jse *JSEmitter) PreVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	assignmentToken := node.Tok.String()
	if assignmentToken == ":=" && len(node.Lhs) == 1 {
		str := jse.emitAsString("let ", indent)
		jse.emitToFile(str)
	} else if assignmentToken == ":=" && len(node.Lhs) > 1 {
		str := jse.emitAsString("let [", indent)
		jse.emitToFile(str)
	} else if len(node.Lhs) > 1 {
		// Multi-value assignment (not declaration) needs destructuring
		str := jse.emitAsString("[", indent)
		jse.emitToFile(str)
	} else {
		str := jse.emitAsString("", indent)
		jse.emitToFile(str)
	}
	// Convert := to =, preserve compound operators
	if assignmentToken != "+=" && assignmentToken != "-=" && assignmentToken != "*=" && assignmentToken != "/=" {
		assignmentToken = "="
	}
	jse.assignmentToken = assignmentToken
}

func (jse *JSEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	// Close destructuring bracket for multi-value assignments
	if len(node.Lhs) > 1 {
		jse.emitToFile("]")
	}
}

func (jse *JSEmitter) PreVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	if jse.isMapAssign {
		// Turn off suppression and emit: m = hmap.hashMapSet(m, capturedKey,
		jse.suppressRangeEmit = false
		str := jse.emitAsString(jse.mapAssignVarName+" = hmap.hashMapSet("+jse.mapAssignVarName+", "+jse.capturedMapKey+", ", jse.mapAssignIndent)
		jse.emitToFile(str)
		return
	}
	jse.emitToFile(" " + jse.assignmentToken + " ")
}

func (jse *JSEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	// Handle type assertion comma-ok: val, ok := x.(Type)
	if jse.isTypeAssertCommaOk {
		expr := jse.capturedMapKey
		decl := ""
		if jse.typeAssertCommaOkIsDecl {
			decl = "let "
		}
		indentStr := jse.emitAsString("", jse.typeAssertCommaOkIndent)
		jse.suppressEmit = false
		jse.emitToFile(fmt.Sprintf("%s%s%s = true;\n", indentStr, decl, jse.typeAssertCommaOkOkName))
		jse.emitToFile(fmt.Sprintf("%s%s%s = %s;\n", indentStr, decl, jse.typeAssertCommaOkValName, expr))
		jse.isTypeAssertCommaOk = false
		jse.capturedMapKey = ""
		jse.captureMapKey = false
		return
	}
	// Handle comma-ok: val, ok := m[key]
	if jse.isMapCommaOk {
		key := jse.capturedMapKey
		decl := ""
		if jse.mapCommaOkIsDecl {
			decl = "let "
		}
		indentStr := jse.emitAsString("", jse.mapCommaOkIndent)
		jse.suppressEmit = false
		mapName := jse.mapCommaOkMapName
		zeroVal := jse.mapCommaOkZeroVal
		jse.emitToFile(fmt.Sprintf("%s%s%s = hmap.hashMapContains(%s, %s);\n",
			indentStr, decl, jse.mapCommaOkOkName, mapName, key))
		jse.emitToFile(fmt.Sprintf("%s%s%s = %s ? hmap.hashMapGet(%s, %s) : %s;\n",
			indentStr, decl, jse.mapCommaOkValName, jse.mapCommaOkOkName,
			mapName, key, zeroVal))
		jse.isMapCommaOk = false
		jse.capturedMapKey = ""
		jse.captureMapKey = false
		return
	}
	// Handle map assignment: close hashMapSet call
	if jse.isMapAssign {
		jse.emitToFile(");\n")
		jse.isMapAssign = false
		jse.mapAssignVarName = ""
		jse.capturedMapKey = ""
		return
	}
	// Check if this was a blank identifier assignment - reset suppression
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
		jse.suppressRangeEmit = false
		return // Don't emit anything for blank identifier assignments
	}
	if jse.forwardDecl {
		return
	}
	// Don't emit semicolon inside for loop init or post conditions
	if !jse.insideForPostCond && !jse.insideForInit {
		jse.emitToFile(";\n")
	}
}

func (jse *JSEmitter) PreVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	if jse.forwardDecl {
		return
	}
	if index > 0 {
		jse.emitToFile(", ")
	}
}

// Expression statements
func (jse *JSEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile(";\n")
}

// Identifiers
func (jse *JSEmitter) PreVisitIdent(node *ast.Ident, indent int) {
	if jse.forwardDecl {
		return
	}
	// Skip type emissions in JavaScript
	if jse.suppressTypeEmit {
		return
	}
	name := node.Name
	// Handle map operation identifier replacements
	if jse.isMapMakeCall && name == "make" {
		jse.emitToFile("hmap.newHashMap")
		return
	}
	if jse.isSliceMakeCall && name == "make" {
		jse.emitToFile("new Array")
		return
	}
	if jse.isDeleteCall && name == "delete" {
		jse.emitToFile(jse.deleteMapVarName + " = hmap.hashMapDelete")
		return
	}
	if jse.isMapLenCall && name == "len" {
		jse.emitToFile("hmap.hashMapLen")
		return
	}
	// Apply builtin lowering
	lowered := jse.lowerToBuiltins(name)
	// If lowered to empty string, don't emit (e.g., "fmt" package)
	if lowered == "" {
		return
	}
	// Handle special identifiers
	switch lowered {
	case "true", "false":
		jse.emitToFile(lowered)
	case "nil":
		jse.emitToFile("null")
	default:
		// Check if this identifier refers to a constant or function in the current namespace
		if jse.inNamespace && jse.pkg != nil && jse.pkg.TypesInfo != nil {
			if obj := jse.pkg.TypesInfo.Uses[node]; obj != nil {
				needsThis := false
				// Check for constants
				if _, isConst := obj.(*types.Const); isConst {
					if obj.Pkg() != nil && obj.Pkg().Name() == jse.currentPackage {
						needsThis = true
					}
				}
				// Check for functions
				if _, isFunc := obj.(*types.Func); isFunc {
					if obj.Pkg() != nil && obj.Pkg().Name() == jse.currentPackage {
						needsThis = true
					}
				}
				if needsThis {
					// Inside function literals, use package name instead of 'this'
					// because 'this' has different meaning inside closures
					if jse.inFuncLit {
						jse.emitToFile(jse.currentPackage + "." + lowered)
					} else {
						jse.emitToFile("this." + lowered)
					}
					return
				}
			}
		}
		jse.emitToFile(lowered)
	}
}

// Basic literals
func (jse *JSEmitter) PreVisitBasicLit(node *ast.BasicLit, indent int) {
	if jse.forwardDecl {
		return
	}
	switch node.Kind {
	case token.STRING:
		// Handle raw strings
		if strings.HasPrefix(node.Value, "`") {
			// Convert to template literal
			jse.emitToFile(node.Value)
		} else {
			jse.emitToFile(node.Value)
		}
	case token.CHAR:
		// Convert char literal to integer (Go rune semantics)
		// node.Value is like 'a' or '\n', we need to convert to integer
		val := node.Value
		if len(val) >= 2 && val[0] == '\'' && val[len(val)-1] == '\'' {
			inner := val[1 : len(val)-1]
			// Handle escape sequences
			var charCode int
			if len(inner) == 1 {
				charCode = int(inner[0])
			} else if len(inner) == 2 && inner[0] == '\\' {
				switch inner[1] {
				case 'n':
					charCode = 10
				case 't':
					charCode = 9
				case 'r':
					charCode = 13
				case '\\':
					charCode = 92
				case '\'':
					charCode = 39
				case '0':
					charCode = 0
				default:
					charCode = int(inner[1])
				}
			} else {
				// Fallback to emitting as string
				jse.emitToFile(node.Value)
				return
			}
			jse.emitToFile(fmt.Sprintf("%d", charCode))
		} else {
			jse.emitToFile(node.Value)
		}
	default:
		jse.emitToFile(node.Value)
	}
}

// Binary expressions
func (jse *JSEmitter) PreVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	if jse.forwardDecl {
		return
	}
	// Check for integer division
	if node.Op == token.QUO {
		// Check if both operands are integer types
		if jse.pkg != nil && jse.pkg.TypesInfo != nil {
			leftType := jse.pkg.TypesInfo.TypeOf(node.X)
			rightType := jse.pkg.TypesInfo.TypeOf(node.Y)
			if leftType != nil && rightType != nil {
				leftBasic, leftIsBasic := leftType.Underlying().(*types.Basic)
				rightBasic, rightIsBasic := rightType.Underlying().(*types.Basic)
				if leftIsBasic && rightIsBasic {
					if (leftBasic.Info()&types.IsInteger) != 0 && (rightBasic.Info()&types.IsInteger) != 0 {
						jse.intDivision = true
					}
				}
			}
		}
	}
	jse.emitToFile("(")
}

func (jse *JSEmitter) PreVisitBinaryExprOperator(op token.Token, indent int) {
	if jse.forwardDecl {
		return
	}
	opStr := op.String()
	// Handle Go operators that need conversion
	switch opStr {
	case "&&":
		jse.emitToFile(" && ")
	case "||":
		jse.emitToFile(" || ")
	default:
		jse.emitToFile(" " + opStr + " ")
	}
}

func (jse *JSEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	if jse.forwardDecl {
		return
	}
	// Only add | 0 for the actual division operation, not nested expressions
	if node.Op == token.QUO && jse.intDivision {
		// Use bitwise OR to convert to integer (truncate towards zero)
		jse.emitToFile(" | 0)")
		jse.intDivision = false
	} else {
		jse.emitToFile(")")
	}
}

// Unary expressions
func (jse *JSEmitter) PreVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile(node.Op.String())
}

// Call expressions
func (jse *JSEmitter) PreVisitCallExpr(node *ast.CallExpr, indent int) {
	if jse.forwardDecl {
		return
	}
	// Detect make(map[K]V) and make([]T, n) calls
	if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "make" {
		if len(node.Args) >= 1 {
			if mapType, ok := node.Args[0].(*ast.MapType); ok {
				jse.isMapMakeCall = true
				jse.mapMakeKeyType = jse.getMapKeyTypeConst(mapType)
			} else if arrayType, ok := node.Args[0].(*ast.ArrayType); ok {
				// make([]T, n) → new Array(n).fill(default)
				jse.isSliceMakeCall = true
				jse.sliceMakeDefault = "null"
				// Determine default based on element type
				if jse.pkg != nil && jse.pkg.TypesInfo != nil {
					if tv, ok := jse.pkg.TypesInfo.Types[arrayType.Elt]; ok && tv.Type != nil {
						if basic, isBasic := tv.Type.Underlying().(*types.Basic); isBasic {
							info := basic.Info()
							if info&types.IsInteger != 0 || info&types.IsFloat != 0 {
								jse.sliceMakeDefault = "0"
							} else if info&types.IsBoolean != 0 {
								jse.sliceMakeDefault = "false"
							} else if info&types.IsString != 0 {
								jse.sliceMakeDefault = `""`
							}
						}
					}
				}
			}
		}
	}
	// Detect delete(m, k) calls
	if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "delete" {
		if len(node.Args) >= 2 {
			jse.isDeleteCall = true
			jse.deleteMapVarName = exprToString(node.Args[0])
		}
	}
	// Detect len(m) calls on maps
	if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "len" {
		if len(node.Args) >= 1 && jse.pkg != nil && jse.pkg.TypesInfo != nil {
			tv := jse.pkg.TypesInfo.Types[node.Args[0]]
			if tv.Type != nil {
				if _, ok := tv.Type.Underlying().(*types.Map); ok {
					jse.isMapLenCall = true
				}
			}
		}
	}
}

func (jse *JSEmitter) PreVisitCallExprFun(node ast.Expr, indent int) {
	// Don't emit here - the function name will be emitted by PreVisitIdent
	// through traverseExpression
}

func (jse *JSEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile("(")
}

func (jse *JSEmitter) PreVisitCallExprArgs(node []ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	// For make(map[K]V), emit the key type constant as the argument
	if jse.isMapMakeCall {
		jse.emitToFile(fmt.Sprintf("%d", jse.mapMakeKeyType))
	}
}

func (jse *JSEmitter) PreVisitCallExprArg(node ast.Expr, index int, indent int) {
	if jse.forwardDecl {
		return
	}
	if jse.isSliceMakeCall {
		if index == 0 {
			// Suppress the first argument (the ArrayType) for make([]T, n)
			jse.suppressEmit = true
			return
		} else if index == 1 {
			// Re-enable output for the length argument, no comma needed since arg 0 was suppressed
			jse.suppressEmit = false
			return
		}
	}
	if index > 0 {
		jse.emitToFile(", ")
	}
}

func (jse *JSEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile(")")
	// For make([]T, n), append .fill(default)
	if jse.isSliceMakeCall {
		jse.emitToFile(fmt.Sprintf(".fill(%s)", jse.sliceMakeDefault))
		jse.isSliceMakeCall = false
		jse.sliceMakeDefault = ""
		jse.suppressEmit = false // Safety reset
	}
	// Reset map call flags
	if jse.isMapMakeCall {
		jse.isMapMakeCall = false
	}
	if jse.isDeleteCall {
		jse.isDeleteCall = false
		jse.deleteMapVarName = ""
	}
	if jse.isMapLenCall {
		jse.isMapLenCall = false
	}
}

// Return statements
func (jse *JSEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	str := jse.emitAsString("return ", indent)
	jse.emitToFile(str)
	if len(node.Results) > 1 {
		jse.emitToFile("[")
		jse.inMultiValueReturn = true
		jse.multiValueReturnIndex = 0
	}
}

func (jse *JSEmitter) PreVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	if jse.forwardDecl {
		return
	}
	if index > 0 {
		jse.emitToFile(", ")
	}
}

func (jse *JSEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	if len(node.Results) > 1 {
		jse.emitToFile("]")
		jse.inMultiValueReturn = false
	}
	jse.emitToFile(";\n")
}

// If statements
func (jse *JSEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	if node.Init != nil {
		str := jse.emitAsString("{\n", indent)
		jse.emitToFile(str)
	} else {
		str := jse.emitAsString("if (", indent)
		jse.emitToFile(str)
	}
}

func (jse *JSEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	if node.Init != nil {
		str := jse.emitAsString("}\n", indent)
		jse.emitToFile(str)
	}
}

func (jse *JSEmitter) PreVisitIfStmtCond(node *ast.IfStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	if node.Init != nil {
		str := jse.emitAsString("if (", indent)
		jse.emitToFile(str)
	}
}

func (jse *JSEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile(") ")
}

func (jse *JSEmitter) PreVisitIfStmtElse(node *ast.IfStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	str := jse.emitAsString(" else ", indent)
	jse.emitToFile(str)
}

// For statements
func (jse *JSEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	// Check if infinite loop
	jse.isInfiniteLoop = node.Init == nil && node.Cond == nil && node.Post == nil
	if jse.isInfiniteLoop {
		str := jse.emitAsString("while (true) ", indent)
		jse.emitToFile(str)
	} else {
		str := jse.emitAsString("for (", indent)
		jse.emitToFile(str)
	}
}

func (jse *JSEmitter) PreVisitForStmtInit(node ast.Stmt, indent int) {
	if jse.forwardDecl || jse.isInfiniteLoop {
		return
	}
	jse.insideForInit = true
}

func (jse *JSEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	if jse.forwardDecl || jse.isInfiniteLoop {
		return
	}
	jse.insideForInit = false
	jse.emitToFile("; ")
}

func (jse *JSEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	if jse.forwardDecl || jse.isInfiniteLoop {
		return
	}
	jse.emitToFile("; ")
}

func (jse *JSEmitter) PreVisitForStmtPost(node ast.Stmt, indent int) {
	if jse.forwardDecl || jse.isInfiniteLoop {
		return
	}
	jse.insideForPostCond = true
}

func (jse *JSEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	if jse.forwardDecl || jse.isInfiniteLoop {
		return
	}
	jse.insideForPostCond = false
	jse.emitToFile(") ")
}

// Range statements
func (jse *JSEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	// Handle different range patterns
	// Note: Go AST sets Key=nil when using blank identifier _, so we check Value first
	if node.Value != nil {
		// for key, value := range collection OR for _, value := range collection
		// When key is blank (_), Go AST has Key=nil, not Key=Ident{Name:"_"}
		jse.isKeyValueRange = true
		if node.Key == nil {
			// Key is nil (blank identifier _) - use synthetic index
			jse.rangeKeyName = "_idx"
		} else if keyIdent, ok := node.Key.(*ast.Ident); ok {
			if keyIdent.Name == "_" {
				// Explicit blank key
				jse.rangeKeyName = "_idx"
				DebugLogPrintf("JSEmitter: Range key is blank _, using _idx")
			} else {
				jse.rangeKeyName = keyIdent.Name
				DebugLogPrintf("JSEmitter: Range key is %s", keyIdent.Name)
			}
		} else {
			// Key is not a simple identifier, use synthetic index
			jse.rangeKeyName = "_idx"
			DebugLogPrintf("JSEmitter: Range key not ident, using _idx")
		}
		if valIdent, ok := node.Value.(*ast.Ident); ok {
			jse.rangeValueName = valIdent.Name
		}
		jse.rangeCollectionExpr = ""
		jse.suppressRangeEmit = true
		jse.rangeStmtIndent = indent
	} else if node.Key != nil {
		// for i := range collection (index-only)
		jse.isKeyValueRange = false
		if keyIdent, ok := node.Key.(*ast.Ident); ok {
			if keyIdent.Name == "_" {
				jse.rangeKeyName = "_idx"
			} else {
				jse.rangeKeyName = keyIdent.Name
			}
		} else {
			jse.rangeKeyName = "_idx"
		}
		jse.rangeCollectionExpr = ""
		jse.suppressRangeEmit = true
		jse.rangeStmtIndent = indent
	} else {
		DebugLogPrintf("JSEmitter: Range has nil Key and nil Value")
	}
}

func (jse *JSEmitter) PreVisitRangeStmtKey(node ast.Expr, indent int) {
	// Key is captured in PreVisitRangeStmt, suppress emission
	jse.suppressRangeEmit = true
}

func (jse *JSEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	// Keep suppressing
}

func (jse *JSEmitter) PreVisitRangeStmtValue(node ast.Expr, indent int) {
	// Value is captured in PreVisitRangeStmt, suppress emission
}

func (jse *JSEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	// Stop suppressing, start capturing collection expression
	jse.suppressRangeEmit = false
	jse.captureRangeExpr = true
}

func (jse *JSEmitter) PreVisitRangeStmtX(node ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	// Already in capture mode from PostVisitRangeStmtValue
}

func (jse *JSEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.captureRangeExpr = false
	collection := jse.rangeCollectionExpr
	key := jse.rangeKeyName
	rangeIndent := jse.rangeStmtIndent

	if jse.isKeyValueRange {
		// Emit: for (let key = 0; key < collection.length; key++)
		str := jse.emitAsString(fmt.Sprintf("for (let %s = 0; %s < %s.length; %s++) ", key, key, collection, key), rangeIndent)
		jse.emitToFile(str)
		// Set flag to emit value declaration in the block
		if jse.rangeValueName != "" && jse.rangeValueName != "_" {
			jse.pendingRangeValueDecl = true
		}
	} else {
		// Index-only: for (let i = 0; i < collection.length; i++)
		str := jse.emitAsString(fmt.Sprintf("for (let %s = 0; %s < %s.length; %s++) ", key, key, collection, key), rangeIndent)
		jse.emitToFile(str)
	}
}

func (jse *JSEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.isKeyValueRange = false
	jse.rangeKeyName = ""
	jse.rangeValueName = ""
	jse.rangeCollectionExpr = ""
}

// Increment/Decrement statements
func (jse *JSEmitter) PreVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	if !jse.insideForPostCond {
		str := jse.emitAsString("", indent)
		jse.emitToFile(str)
	}
}

func (jse *JSEmitter) PostVisitIncDecStmtX(node ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
}

func (jse *JSEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile(node.Tok.String())
	if !jse.insideForPostCond {
		jse.emitToFile(";\n")
	}
}

// Index expressions (array access)
func (jse *JSEmitter) PreVisitIndexExpr(node *ast.IndexExpr, indent int) {
	if jse.forwardDecl {
		return
	}
	// Check if indexing a map (for read access: m[k])
	if !jse.isMapAssign && !jse.isMapCommaOk && jse.pkg != nil && jse.pkg.TypesInfo != nil {
		tv := jse.pkg.TypesInfo.Types[node.X]
		if tv.Type != nil {
			if _, ok := tv.Type.Underlying().(*types.Map); ok {
				jse.isMapIndex = true
				jse.emitToFile("hmap.hashMapGet(")
				return
			}
		}
	}
}

func (jse *JSEmitter) PreVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	if jse.forwardDecl {
		return
	}
	// Map read: emit ", " between map expr and key
	if jse.isMapIndex {
		jse.emitToFile(", ")
		return
	}
	// Map assignment: start capturing the key expression
	if jse.isMapAssign {
		jse.captureMapKey = true
		jse.capturedMapKey = ""
		return
	}
	// Comma-ok: start capturing the key expression
	if jse.isMapCommaOk {
		jse.captureMapKey = true
		jse.capturedMapKey = ""
		return
	}
	// Check if indexing a string - need to use charCodeAt() to get byte value like Go
	jse.isStringIndex = false
	if jse.pkg != nil && jse.pkg.TypesInfo != nil {
		tv := jse.pkg.TypesInfo.Types[node.X]
		if tv.Type != nil {
			if basic, ok := tv.Type.(*types.Basic); ok && basic.Kind() == types.String {
				jse.isStringIndex = true
				jse.emitToFile(".charCodeAt(")
				return
			}
		}
	}
	jse.emitToFile("[")
}

func (jse *JSEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	if jse.forwardDecl {
		return
	}
	if jse.isMapIndex {
		jse.emitToFile(")")
		jse.isMapIndex = false
		return
	}
	if jse.captureMapKey {
		jse.captureMapKey = false
		return
	}
	if jse.isStringIndex {
		jse.emitToFile(")")
		jse.isStringIndex = false
	} else {
		jse.emitToFile("]")
	}
}

// Composite literals (arrays, objects)
func (jse *JSEmitter) PreVisitCompositeLit(node *ast.CompositeLit, indent int) {
	if jse.forwardDecl {
		return
	}
	// Check if it's a struct or array
	if node.Type != nil {
		switch node.Type.(type) {
		case *ast.ArrayType:
			jse.emitToFile("[")
			return
		case *ast.Ident, *ast.SelectorExpr:
			// Check if this named type is actually a slice type alias
			if jse.pkg != nil && jse.pkg.TypesInfo != nil {
				if typeAndValue, ok := jse.pkg.TypesInfo.Types[node]; ok {
					underlying := typeAndValue.Type.Underlying()
					if _, isSlice := underlying.(*types.Slice); isSlice {
						jse.emitToFile("[")
						return
					}
				}
			}
			// Struct initialization - use object literal syntax
			// Check if it has named fields (KeyValueExpr)
			if len(node.Elts) > 0 {
				if _, hasKeys := node.Elts[0].(*ast.KeyValueExpr); hasKeys {
					jse.emitToFile("{")
					return
				}
			}
			// Empty struct or positional values
			jse.emitToFile("{")
			return
		}
	}
	jse.emitToFile("[")
}

// Suppress type emission in composite literals (already handled in PreVisitCompositeLit)
func (jse *JSEmitter) PreVisitCompositeLitType(node ast.Expr, indent int) {
	jse.suppressTypeEmit = true
}

func (jse *JSEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	jse.suppressTypeEmit = false
}

func (jse *JSEmitter) PreVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	if jse.forwardDecl {
		return
	}
	if index > 0 {
		jse.emitToFile(", ")
	}
}

func (jse *JSEmitter) PostVisitCompositeLit(node *ast.CompositeLit, indent int) {
	if jse.forwardDecl {
		return
	}
	if node.Type != nil {
		switch node.Type.(type) {
		case *ast.Ident, *ast.SelectorExpr:
			// Check if this named type is actually a slice type alias
			if jse.pkg != nil && jse.pkg.TypesInfo != nil {
				if typeAndValue, ok := jse.pkg.TypesInfo.Types[node]; ok {
					underlying := typeAndValue.Type.Underlying()
					if _, isSlice := underlying.(*types.Slice); isSlice {
						jse.emitToFile("]")
						return
					}
					// If it's a struct, emit default values for missing fields
					if structType, isStruct := underlying.(*types.Struct); isStruct {
						// Collect specified field names
						specifiedFields := make(map[string]bool)
						for _, elt := range node.Elts {
							if kv, ok := elt.(*ast.KeyValueExpr); ok {
								if ident, ok := kv.Key.(*ast.Ident); ok {
									specifiedFields[ident.Name] = true
								}
							}
						}
						// Emit defaults for missing fields
						needsComma := len(node.Elts) > 0
						for i := 0; i < structType.NumFields(); i++ {
							field := structType.Field(i)
							if !specifiedFields[field.Name()] {
								if needsComma {
									jse.emitToFile(", ")
								}
								needsComma = true
								// Emit field with default value
								jse.emitToFile(field.Name() + ": ")
								jse.emitDefaultValue(field.Type())
							}
						}
					}
				}
			}
			// Close struct/object literal
			jse.emitToFile("}")
			return
		}
	}
	jse.emitToFile("]")
}

// emitDefaultValue emits the JavaScript default value for a Go type
func (jse *JSEmitter) emitDefaultValue(t types.Type) {
	switch underlying := t.Underlying().(type) {
	case *types.Basic:
		info := underlying.Info()
		if info&types.IsInteger != 0 || info&types.IsFloat != 0 {
			jse.emitToFile("0")
		} else if info&types.IsBoolean != 0 {
			jse.emitToFile("false")
		} else if info&types.IsString != 0 {
			jse.emitToFile(`""`)
		} else {
			jse.emitToFile("null")
		}
	case *types.Slice:
		jse.emitToFile("[]")
	case *types.Struct:
		// Recursively initialize all struct fields
		jse.emitToFile("{")
		for i := 0; i < underlying.NumFields(); i++ {
			if i > 0 {
				jse.emitToFile(", ")
			}
			field := underlying.Field(i)
			jse.emitToFile(field.Name() + ": ")
			jse.emitDefaultValue(field.Type())
		}
		jse.emitToFile("}")
	case *types.Pointer:
		jse.emitToFile("null")
	default:
		jse.emitToFile("null")
	}
}

// Selector expressions (field access)
func (jse *JSEmitter) PreVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	if jse.forwardDecl {
		return
	}
}

func (jse *JSEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	// Don't emit dot when suppressing type emission
	if jse.suppressTypeEmit {
		return
	}
	// Only emit dot if X was not lowered to empty (e.g., fmt -> "")
	if ident, ok := node.(*ast.Ident); ok {
		if jse.lowerToBuiltins(ident.Name) == "" {
			return
		}
		// If X is a known namespace/package, don't emit dot
		// Constants and functions are accessible globally or via the namespace object
		if _, found := namespaces[ident.Name]; found {
			// For cross-package access, emit the dot to access namespace object
			// e.g., lexer.GetTokens() works because GetTokens is a method of the lexer object
			jse.emitToFile(".")
			return
		}
	}
	jse.emitToFile(".")
}

func (jse *JSEmitter) PreVisitSelectorExprSel(node *ast.Ident, indent int) {
	// Selector name is emitted via PreVisitIdent when Sel is traversed
}

// Type specifications (structs)
func (jse *JSEmitter) PreVisitTypeSpec(node *ast.TypeSpec, indent int) {
	if jse.forwardDecl {
		return
	}
	// Check if it's a struct type
	if structType, ok := node.Type.(*ast.StructType); ok {
		str := jse.emitAsString("class "+node.Name.Name+" {\n", indent)
		jse.emitToFile(str)

		// Generate constructor
		str = jse.emitAsString("constructor(", indent+1)
		jse.emitToFile(str)

		// Collect field names
		var fieldNames []string
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				fieldNames = append(fieldNames, name.Name)
			}
		}
		jse.emitToFile(strings.Join(fieldNames, ", "))
		jse.emitToFile(") {\n")

		// Initialize fields
		for _, name := range fieldNames {
			str = jse.emitAsString("this."+name+" = "+name+";\n", indent+2)
			jse.emitToFile(str)
		}

		str = jse.emitAsString("}\n", indent+1)
		jse.emitToFile(str)
		str = jse.emitAsString("}\n\n", indent)
		jse.emitToFile(str)
	} else if _, ok := node.Type.(*ast.ArrayType); ok {
		// Type alias for array - just emit a comment
		str := jse.emitAsString("// type "+node.Name.Name+" = array\n", indent)
		jse.emitToFile(str)
	}
}

// Struct field type and name handling - suppress type emissions
func (jse *JSEmitter) PreVisitGenStructFieldType(node ast.Expr, indent int) {
	jse.suppressTypeEmit = true
}

func (jse *JSEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	jse.suppressTypeEmit = false
}

func (jse *JSEmitter) PreVisitGenStructFieldName(node *ast.Ident, indent int) {
	// Field names are handled in PreVisitTypeSpec, suppress here
	jse.suppressTypeEmit = true
}

func (jse *JSEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	jse.suppressTypeEmit = false
}

// Type alias handling - suppress in JavaScript (no type aliases needed)
func (jse *JSEmitter) PreVisitTypeAliasName(node *ast.Ident, indent int) {
	jse.suppressTypeEmit = true
}

func (jse *JSEmitter) PostVisitTypeAliasName(node *ast.Ident, indent int) {
	// Keep suppressed until after the type
}

func (jse *JSEmitter) PreVisitTypeAliasType(node ast.Expr, indent int) {
	// Type still suppressed
}

func (jse *JSEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	jse.suppressTypeEmit = false
}

// Declaration statements (var a int, var b []string, etc.)
func (jse *JSEmitter) PreVisitDeclStmt(node *ast.DeclStmt, indent int) {
	if jse.forwardDecl {
		return
	}
}

func (jse *JSEmitter) PreVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	// Suppress type emission - JavaScript doesn't need type annotations
	jse.suppressTypeEmit = true
	// Emit "let " for variable declaration
	str := jse.emitAsString("let ", indent)
	jse.emitToFile(str)
	// Check if we need to add default initialization
	if len(node.Values) == 0 && node.Type != nil {
		// Use type info to detect actual type (handles type aliases)
		if jse.pkg != nil && jse.pkg.TypesInfo != nil {
			if typeAndValue, ok := jse.pkg.TypesInfo.Types[node.Type]; ok {
				underlying := typeAndValue.Type.Underlying()
				if _, isMap := underlying.(*types.Map); isMap {
					if mapType, ok := node.Type.(*ast.MapType); ok {
						jse.pendingMapInit = true
						jse.pendingMapKeyType = jse.getMapKeyTypeConst(mapType)
					}
					return
				}
				if _, isSlice := underlying.(*types.Slice); isSlice {
					jse.pendingSliceInit = true
					return
				}
				if structType, isStruct := underlying.(*types.Struct); isStruct {
					jse.pendingStructInit = true
					jse.pendingStructType = structType
					return
				}
				// Handle basic types (string, int, bool, etc.)
				if basicType, isBasic := underlying.(*types.Basic); isBasic {
					jse.pendingBasicInit = basicType
					return
				}
			}
		}
		// Fallback to AST-based detection
		switch t := node.Type.(type) {
		case *ast.ArrayType:
			// Slice/array type - initialize to []
			jse.pendingSliceInit = true
		case *ast.Ident:
			// Custom type (struct) - initialize to {} unless it's a built-in type
			if !isBuiltinType(t.Name) {
				jse.pendingStructInit = true
			}
		case *ast.SelectorExpr:
			// External package type (struct) - initialize to {}
			jse.pendingStructInit = true
		}
	}
}

func (jse *JSEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	jse.suppressTypeEmit = false
}

func (jse *JSEmitter) PreVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	// Variable name is emitted via PreVisitIdent when traversed
}

func (jse *JSEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	if jse.forwardDecl {
		return
	}
	if jse.pendingMapInit {
		jse.emitToFile(fmt.Sprintf(" = hmap.newHashMap(%d)", jse.pendingMapKeyType))
		jse.pendingMapInit = false
		jse.pendingMapKeyType = 0
	} else if jse.pendingSliceInit {
		jse.emitToFile(" = []")
		jse.pendingSliceInit = false
	} else if jse.pendingStructInit {
		if jse.pendingStructType != nil {
			// Emit full struct with all fields initialized
			jse.emitToFile(" = {")
			for i := 0; i < jse.pendingStructType.NumFields(); i++ {
				if i > 0 {
					jse.emitToFile(", ")
				}
				field := jse.pendingStructType.Field(i)
				jse.emitToFile(field.Name() + ": ")
				jse.emitDefaultValue(field.Type())
			}
			jse.emitToFile("}")
			jse.pendingStructType = nil
		} else {
			jse.emitToFile(" = {}")
		}
		jse.pendingStructInit = false
	} else if jse.pendingBasicInit != nil {
		// Emit default value for basic types (string, int, bool, etc.)
		info := jse.pendingBasicInit.Info()
		if info&types.IsInteger != 0 || info&types.IsFloat != 0 {
			jse.emitToFile(" = 0")
		} else if info&types.IsBoolean != 0 {
			jse.emitToFile(" = false")
		} else if info&types.IsString != 0 {
			jse.emitToFile(` = ""`)
		}
		jse.pendingBasicInit = nil
	}
	jse.emitToFile(";\n")
}

// isBuiltinType returns true if the type name is a Go built-in type
func isBuiltinType(name string) bool {
	switch name {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "complex64", "complex128",
		"bool", "string", "byte", "rune", "error", "any":
		return true
	}
	return false
}

func (jse *JSEmitter) PostVisitDeclStmt(node *ast.DeclStmt, indent int) {
	if jse.forwardDecl {
		return
	}
}

// Variable declarations
func (jse *JSEmitter) PreVisitGenDeclVar(node *ast.GenDecl, indent int) {
	if jse.forwardDecl {
		return
	}
}

func (jse *JSEmitter) PreVisitValueSpec(node *ast.ValueSpec, indent int) {
	if jse.forwardDecl {
		return
	}
	str := jse.emitAsString("let ", indent)
	jse.emitToFile(str)
}

func (jse *JSEmitter) PreVisitValueSpecName(node *ast.Ident, index int, indent int) {
	if jse.forwardDecl {
		return
	}
	if index > 0 {
		jse.emitToFile(", ")
	}
	jse.emitToFile(node.Name)
}

func (jse *JSEmitter) PreVisitValueSpecValue(node ast.Expr, index int, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile(" = ")
}

func (jse *JSEmitter) PostVisitValueSpec(node *ast.ValueSpec, indent int) {
	if jse.forwardDecl {
		return
	}
	// If no value, initialize with default
	if len(node.Values) == 0 {
		if node.Type != nil && jse.pkg != nil && jse.pkg.TypesInfo != nil {
			if typeAndValue, ok := jse.pkg.TypesInfo.Types[node.Type]; ok {
				jse.emitToFile(" = ")
				jse.emitDefaultValue(typeAndValue.Type)
			}
		}
	}
	jse.emitToFile(";\n")
}

// Constant declarations
func (jse *JSEmitter) PreVisitGenDeclConst(node *ast.GenDecl, indent int) {
	if jse.forwardDecl {
		return
	}
}

func (jse *JSEmitter) PreVisitGenDeclConstName(node *ast.Ident, indent int) {
	if jse.forwardDecl {
		return
	}
	if jse.inNamespace {
		// Inside a namespace object, use property syntax
		str := jse.emitAsString(node.Name+": ", 0)
		jse.emitToFile(str)
	} else {
		str := jse.emitAsString("const "+node.Name+" = ", 0)
		jse.emitToFile(str)
	}
}

func (jse *JSEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	if jse.forwardDecl {
		return
	}
	if jse.inNamespace {
		jse.emitToFile(",\n")
	} else {
		jse.emitToFile(";\n")
	}
}

// Parenthesized expressions
func (jse *JSEmitter) PreVisitParenExpr(node *ast.ParenExpr, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile("(")
}

func (jse *JSEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile(")")
}

// Break and continue
func (jse *JSEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	str := jse.emitAsString(node.Tok.String()+";\n", indent)
	jse.emitToFile(str)
}

// Switch statements
func (jse *JSEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	str := jse.emitAsString("switch (", indent)
	jse.emitToFile(str)
}

func (jse *JSEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile(") {\n")
}

func (jse *JSEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	if jse.forwardDecl {
		return
	}
	str := jse.emitAsString("}\n", indent)
	jse.emitToFile(str)
}

func (jse *JSEmitter) PreVisitCaseClause(node *ast.CaseClause, indent int) {
	if jse.forwardDecl {
		return
	}
	if len(node.List) == 0 {
		str := jse.emitAsString("default:\n", indent)
		jse.emitToFile(str)
	} else {
		str := jse.emitAsString("case ", indent)
		jse.emitToFile(str)
	}
}

func (jse *JSEmitter) PreVisitCaseClauseExpr(node ast.Expr, index int, indent int) {
	if jse.forwardDecl {
		return
	}
	if index > 0 {
		jse.emitToFile(", ")
	}
}

func (jse *JSEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	if len(node) > 0 {
		jse.emitToFile(":\n")
	}
}

func (jse *JSEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	if jse.forwardDecl {
		return
	}
	// Add break if no fallthrough (Go's default behavior)
	str := jse.emitAsString("break;\n", indent+1)
	jse.emitToFile(str)
}

// Key-value expressions (struct field initialization)
func (jse *JSEmitter) PreVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	if jse.forwardDecl {
		return
	}
}

func (jse *JSEmitter) PreVisitKeyValueExprValue(node ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile(": ")
}

// Slice expressions
// Go: a[low:high] => JS: a.slice(low, high)
// Go: a[low:] => JS: a.slice(low)
// Go: a[:high] => JS: a.slice(0, high)
func (jse *JSEmitter) PreVisitSliceExpr(node *ast.SliceExpr, indent int) {
	if jse.forwardDecl {
		return
	}
}

func (jse *JSEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	// After the array name, emit .slice(
	jse.emitToFile(".slice(")
}

func (jse *JSEmitter) PreVisitSliceExprXBegin(node ast.Expr, indent int) {
	// Suppress the second X visit - it's for Go's internal slice bounds
	jse.suppressRangeEmit = true
}

func (jse *JSEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {
	jse.suppressRangeEmit = false
}

func (jse *JSEmitter) PreVisitSliceExprLow(node ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	// If Low is nil (like a[:high]), emit 0
	if node == nil {
		jse.emitToFile("0")
	}
}

func (jse *JSEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	// We'll add comma in PreVisitSliceExprHigh if High is not nil
}

func (jse *JSEmitter) PreVisitSliceExprXEnd(node ast.Expr, indent int) {
	// Suppress the third X visit
	jse.suppressRangeEmit = true
}

func (jse *JSEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {
	jse.suppressRangeEmit = false
}

func (jse *JSEmitter) PreVisitSliceExprHigh(node ast.Expr, indent int) {
	if jse.forwardDecl {
		return
	}
	// If High is not nil, emit comma before it
	if node != nil {
		jse.emitToFile(", ")
	}
}

func (jse *JSEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.emitToFile(")")
}

// Type assertions (limited support)
func (jse *JSEmitter) PreVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	if jse.forwardDecl {
		return
	}
	// In JavaScript, just return the value (dynamic typing)
}

func (jse *JSEmitter) PostVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	if jse.forwardDecl {
		return
	}
	// No-op for JavaScript
}

func (jse *JSEmitter) PreVisitTypeAssertExprType(node ast.Expr, indent int) {
	// Suppress type emission - JavaScript has no type assertions
	jse.suppressTypeEmit = true
}

func (jse *JSEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	jse.suppressTypeEmit = false
}

func (jse *JSEmitter) PreVisitTypeAssertExprX(node ast.Expr, indent int) {
	if jse.isTypeAssertCommaOk {
		jse.captureMapKey = true
		jse.capturedMapKey = ""
		return
	}
}

func (jse *JSEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	if jse.isTypeAssertCommaOk {
		jse.captureMapKey = false
		return
	}
}

// Function literals (closures)
func (jse *JSEmitter) PreVisitFuncLit(node *ast.FuncLit, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.inFuncLit = true
	jse.emitToFile("function(")
}

func (jse *JSEmitter) PreVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	// Start of parameters - suppress type emission
	jse.suppressTypeEmit = true
}

func (jse *JSEmitter) PreVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	if jse.forwardDecl {
		return
	}
	if index > 0 {
		jse.emitToFile(", ")
	}
	for i, name := range node.Names {
		if i > 0 {
			jse.emitToFile(", ")
		}
		jse.emitToFile(name.Name)
	}
}

func (jse *JSEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	if jse.forwardDecl {
		return
	}
	jse.suppressTypeEmit = false
	jse.emitToFile(") ")
}

func (jse *JSEmitter) PreVisitFuncLitTypeResults(node *ast.FieldList, indent int) {
	// Suppress return type emission - JavaScript doesn't have return type annotations
	jse.suppressTypeEmit = true
}

func (jse *JSEmitter) PostVisitFuncLitTypeResults(node *ast.FieldList, indent int) {
	jse.suppressTypeEmit = false
}

func (jse *JSEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	jse.inFuncLit = false
}

// Helper to check if type needs special handling
func (jse *JSEmitter) getJSType(goType string) string {
	if jsType, ok := jsTypesMap[goType]; ok {
		return jsType
	}
	return goType
}

// mapGoTypeToJS converts Go types to JavaScript type comments
func (jse *JSEmitter) mapGoTypeToJS(t types.Type) string {
	if t == nil {
		return "any"
	}
	switch underlying := t.Underlying().(type) {
	case *types.Basic:
		return jse.getJSType(underlying.Name())
	case *types.Slice:
		return "Array"
	case *types.Struct:
		return "Object"
	default:
		return "any"
	}
}
