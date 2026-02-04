package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/packages"
)

var destTypes = []string{"sbyte", "short", "int", "long", "byte", "ushort", "uint", "ulong", "object", "string", "float", "double"}

var csTypesMap = map[string]string{
	"int8":    destTypes[0],
	"int16":   destTypes[1],
	"int32":   destTypes[2],
	"int64":   destTypes[3],
	"uint8":   destTypes[4],
	"uint16":  destTypes[5],
	"uint32":  destTypes[6],
	"uint64":  destTypes[7],
	"any":     destTypes[8],
	"string":  destTypes[9],
	"float32": destTypes[10],
	"float64": destTypes[11],
}

type AliasRepr struct {
	PackageName string // Package name of the alias
	TypeName    string
}

type Alias struct {
	PackageName    string
	representation []AliasRepr // Representation of the alias
	UnderlyingType string      // Underlying type of the alias as string for now  It's type to what the alias points to
}

type CSharpEmitter struct {
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string // Path to runtime directory (empty = disabled)
	GraphicsRuntime string // Graphics backend: tigr (default), sdl2, none
	file            *os.File
	BaseEmitter
	pkg               *packages.Package
	insideForPostCond bool
	assignmentToken   string
	forwardDecls      bool
	shouldGenerate    bool
	numFuncResults    int
	aliases           map[string]Alias
	currentPackage    string
	isArray           bool
	arrayType         string
	isTuple           bool
	isInfiniteLoop    bool // Track if current for loop is infinite (no init, cond, post)
	// Key-value range loop support
	isKeyValueRange            bool
	rangeKeyName               string
	rangeValueName             string
	rangeCollectionExpr        string
	captureRangeExpr           bool
	suppressRangeEmit          bool
	rangeStmtIndent            int
	pendingRangeValueDecl      bool
	pendingValueName           string
	pendingCollectionExpr      string
	pendingKeyName             string
	inTypeContext              bool              // Track if we're in a type context (don't add .Api. for types)
	apiClassOpened             bool              // Track if we've opened the Api class (for functions)
	suppressTypeAliasEmit      bool              // Suppress emission during type alias handling
	currentAliasName           string            // Current type alias name being processed
	typeAliasMap               map[string]string // Maps alias names to underlying type names
	suppressTypeAliasSelectorX bool              // Suppress X part emission for type alias selectors
	// Map support
	isMapMakeCall     bool
	mapMakeKeyType    int
	isMapIndex        bool
	mapIndexValueType string
	isMapAssign       bool
	mapAssignVarName  string
	mapAssignIndent   int
	captureMapKey     bool
	capturedMapKey    string
	suppressMapEmit   bool
	isDeleteCall      bool
	deleteMapVarName  string
	mapKeyCastPrefix  string // C# cast prefix for map keys (e.g. "(long)(")
	mapKeyCastSuffix  string // C# cast suffix for map keys (e.g. ")")
	isMapLenCall      bool
	pendingMapInit    bool
	pendingMapKeyType int
	// Comma-ok idiom: val, ok := m[key]
	isMapCommaOk      bool
	mapCommaOkValName string
	mapCommaOkOkName  string
	mapCommaOkMapName string
	mapCommaOkValType string
	mapCommaOkIsDecl  bool
	mapCommaOkIndent  int
	// Type assertion comma-ok: val, ok := x.(Type)
	isTypeAssertCommaOk      bool
	typeAssertCommaOkValName string
	typeAssertCommaOkOkName  string
	typeAssertCommaOkType    string
	typeAssertCommaOkIsDecl  bool
	typeAssertCommaOkIndent  int
	// Slice make support (for transpiled hashmap runtime)
	isSliceMakeCall bool
	sliceMakeElemType string
}

func (*CSharpEmitter) lowerToBuiltins(selector string) string {
	switch selector {
	case "fmt":
		return ""
	case "Sprintf":
		return "Formatter.Sprintf"
	case "Println":
		return "Console.WriteLine"
	case "Printf":
		return "Formatter.Printf"
	case "Print":
		return "Formatter.Printf"
	case "len":
		return "SliceBuiltins.Length"
	case "append":
		return "SliceBuiltins.Append"
	}
	return selector
}

// =============================================================================
// TYPE ALIAS HANDLING FOR C#
// =============================================================================
//
// Go allows type aliases like:
//     type ExprKind int           // simple alias
//     type AST []Statement        // complex alias (slice)
//
// In C#, type aliases use "using X = T;" syntax, but these CANNOT be placed
// inside a class. Since we use "public static class" for packages (to avoid
// the .Api. workaround), we cannot emit using aliases inside the class body.
//
// SOLUTION: Replace all usages of type aliases with their underlying C# type.
//
// ALGORITHM:
//
// 1. COLLECTION PHASE (during AST traversal of type alias definitions):
//    - PreVisitTypeAliasName: Store alias name, set suppressTypeAliasEmit=true
//    - PostVisitTypeAliasType: Get underlying Go type from type info,
//      convert to C# syntax, store in typeAliasMap[aliasName] = csharpType
//    - The suppressTypeAliasEmit flag prevents any output during this phase
//
// 2. CONVERSION (convertGoTypeToCSharp function):
//    - Slice types: "[]T" -> "List<T>" (recursive for nested types)
//    - Map types: "map[K]V" -> "Dictionary<K, V>"
//    - Package paths: "uql/ast.Statement" -> "ast.Statement" (strip path prefix)
//    - Basic types: "int8" -> "sbyte", etc. (via csTypesMap)
//
// 3. REPLACEMENT PHASE (during code generation):
//    - PreVisitIdent: If identifier is in typeAliasMap, emit underlying type
//    - PreVisitSelectorExpr: If selector (e.g., ast.AST) refers to alias,
//      set suppressTypeAliasSelectorX=true to skip package prefix
//    - This transforms "ast.AST" into just "List<ast.Statement>"
//
// 4. SUPPRESSION FLAGS:
//    - suppressTypeAliasEmit: Blocks PreVisitIdent, PreVisitArrayType,
//      PostVisitArrayType during type alias definition processing
//    - suppressTypeAliasSelectorX: Blocks package name and dot emission
//      when a type alias is used as pkg.AliasName
//
// EXAMPLE:
//
//   Go source:
//     type AST []Statement
//     func Parse() (AST, int8) { var result AST; return result, 0 }
//
//   Generated C#:
//     // No "using AST = ..." - alias definition produces no output
//     public static (List<ast.Statement>, sbyte) Parse() {
//         List<ast.Statement> result = default;
//         return (result, 0);
//     }
//
// =============================================================================

// convertGoTypeToCSharp converts a Go type string to C# syntax
// Handles: slices ([]T -> List<T>), package paths (pkg/subpkg.Type -> subpkg.Type), basic type mappings
func (cse *CSharpEmitter) convertGoTypeToCSharp(goType string) string {
	result := goType

	// Handle slice types: []T -> List<T>
	if strings.HasPrefix(result, "[]") {
		elementType := result[2:]
		elementType = cse.convertGoTypeToCSharp(elementType) // Recursive for nested types
		return "List<" + elementType + ">"
	}

	// Handle map types: map[K]V -> Dictionary<K, V>
	if strings.HasPrefix(result, "map[") {
		// Find the key type (between [ and ])
		bracketEnd := strings.Index(result, "]")
		if bracketEnd > 4 {
			keyType := result[4:bracketEnd]
			valueType := result[bracketEnd+1:]
			keyType = cse.convertGoTypeToCSharp(keyType)
			valueType = cse.convertGoTypeToCSharp(valueType)
			return "Dictionary<" + keyType + ", " + valueType + ">"
		}
	}

	// Strip package path prefixes (e.g., uql/ast.Statement -> ast.Statement)
	if strings.Contains(result, "/") {
		lastSlash := strings.LastIndex(result, "/")
		result = result[lastSlash+1:]
	}

	// Apply basic type mappings
	if csType, exists := csTypesMap[result]; exists {
		return csType
	}

	return result
}

func (cse *CSharpEmitter) emitAsString(s string, indent int) string {
	return strings.Repeat(" ", indent) + s
}

// Helper function to determine token type for C# specific content
func (cse *CSharpEmitter) getTokenType(content string) TokenType {
	// Check for C# keywords
	switch content {
	case "using", "namespace", "class", "public", "private", "protected", "static", "override", "virtual", "sealed", "readonly", "var":
		return CSharpKeyword
	case "if", "else", "for", "while", "switch", "case", "default", "break", "continue", "return":
		return IfKeyword // Will be refined based on actual keyword
	case "(":
		return LeftParen
	case ")":
		return RightParen
	case "{":
		return LeftBrace
	case "}":
		return RightBrace
	case "[":
		return LeftBracket
	case "]":
		return RightBracket
	case ";":
		return Semicolon
	case ",":
		return Comma
	case ".":
		return Dot
	case "=", "+=", "-=", "*=", "/=":
		return Assignment
	case "+", "-", "*", "/", "%":
		return ArithmeticOperator
	case "==", "!=", "<", ">", "<=", ">=":
		return ComparisonOperator
	case "&&", "||", "!":
		return LogicalOperator
	case " ", "\t":
		return WhiteSpace
	case "\n":
		return NewLine
	}

	// Check if it's a number
	if len(content) > 0 && (content[0] >= '0' && content[0] <= '9') {
		return NumberLiteral
	}

	// Check if it's a string literal
	if len(content) >= 2 && content[0] == '"' && content[len(content)-1] == '"' {
		return StringLiteral
	}

	// Default to identifier
	return Identifier
}

// Helper function to emit token
func (cse *CSharpEmitter) emitToken(content string, tokenType TokenType, indent int) {
	if cse.suppressMapEmit {
		return
	}
	if cse.captureMapKey {
		cse.capturedMapKey += cse.emitAsString(content, indent)
		return
	}
	token := CreateToken(tokenType, cse.emitAsString(content, indent))
	_ = cse.gir.emitTokenToFileBuffer(token, EmptyVisitMethod)
}

// getMapKeyTypeConst returns the hashmap key type constant for a MapType node
func (cse *CSharpEmitter) getMapKeyTypeConst(mapType *ast.MapType) int {
	if ident, ok := mapType.Key.(*ast.Ident); ok {
		switch ident.Name {
		case "string":
			return 1 // KeyTypeString
		case "int":
			return 2 // KeyTypeInt
		case "bool":
			return 3 // KeyTypeBool
		case "int8":
			return 4 // KeyTypeInt8
		case "int16":
			return 5 // KeyTypeInt16
		case "int32":
			return 6 // KeyTypeInt32
		case "int64":
			return 7 // KeyTypeInt64
		case "uint8":
			return 8 // KeyTypeUint8
		case "uint16":
			return 9 // KeyTypeUint16
		case "uint32":
			return 10 // KeyTypeUint32
		case "uint64":
			return 11 // KeyTypeUint64
		case "float32":
			return 12 // KeyTypeFloat32
		case "float64":
			return 13 // KeyTypeFloat64
		}
	}
	return 0
}

// getCsTypeName converts a Go type to its C# type name
func getCsTypeName(t types.Type) string {
	if basicType, ok := t.(*types.Basic); ok {
		switch basicType.Kind() {
		case types.Int, types.Int32:
			return "int"
		case types.Int8:
			return "sbyte"
		case types.Int16:
			return "short"
		case types.Int64:
			return "long"
		case types.Uint8:
			return "byte"
		case types.Uint16:
			return "ushort"
		case types.Uint32:
			return "uint"
		case types.Uint64:
			return "ulong"
		case types.String:
			return "string"
		case types.Bool:
			return "bool"
		case types.Float32:
			return "float"
		case types.Float64:
			return "double"
		default:
			return "object"
		}
	}
	if iface, ok := t.(*types.Interface); ok && iface.Empty() {
		return "object"
	}
	if named, ok := t.(*types.Named); ok {
		if _, isStruct := named.Underlying().(*types.Struct); isStruct {
			return named.Obj().Name()
		}
	}
	return "object"
}

// getCsKeyCast returns prefix/suffix to cast a map key expression to the correct C# type.
func getCsKeyCast(keyType types.Type) (string, string) {
	if basic, isBasic := keyType.Underlying().(*types.Basic); isBasic {
		switch basic.Kind() {
		case types.Int8:
			return "(sbyte)(", ")"
		case types.Int16:
			return "(short)(", ")"
		case types.Int32:
			return "(int)(", ")"
		case types.Int64:
			return "(long)(", ")"
		case types.Uint8:
			return "(byte)(", ")"
		case types.Uint16:
			return "(ushort)(", ")"
		case types.Uint32:
			return "(uint)(", ")"
		case types.Uint64:
			return "(ulong)(", ")"
		case types.Float32:
			return "(float)(", ")"
		}
	}
	return "", ""
}

func (cse *CSharpEmitter) SetFile(file *os.File) {
	cse.file = file
}

func (cse *CSharpEmitter) GetFile() *os.File {
	return cse.file
}

func (cse *CSharpEmitter) executeIfNotForwardDecls(fn func()) {
	if cse.forwardDecls {
		return
	}
	fn()
}

func (cse *CSharpEmitter) PreVisitProgram(indent int) {
	cse.aliases = make(map[string]Alias)
	cse.typeAliasMap = make(map[string]string)
	outputFile := cse.Output
	cse.shouldGenerate = true
	var err error
	cse.file, err = os.Create(outputFile)
	cse.SetFile(cse.file)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}
	_, err = cse.file.WriteString("using System;\nusing System.Collections;\nusing System.Collections.Generic;\n\n")
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}
	builtin := `public static class SliceBuiltins
{
  public static List<T> Append<T>(this List<T> list, T element)
  {
    if (list == null) list = new List<T>();
    list.Add(element);
    return list;
  }

  public static List<T> Append<T>(this List<T> list, params T[] elements)
  {
    if (list == null) list = new List<T>();
    list.AddRange(elements);
    return list;
  }

  public static List<T> Append<T>(this List<T> list, List<T> elements)
  {
    if (list == null) list = new List<T>();
    list.AddRange(elements);
    return list;
  }

  // Fix: Ensure Length works for collections and not generic T
  public static int Length<T>(ICollection<T> collection)
  {
    return collection == null ? 0 : collection.Count;
  }
  public static int Length(string s)
  {
    return s == null ? 0 : s.Length;
  }
}
public class Formatter {
    public static void Printf(string format, params object[] args)
    {
        int argIndex = 0;
        string converted = "";
        List<object> formattedArgs = new List<object>();

        for (int i = 0; i < format.Length; i++)
        {
            if (format[i] == '%' && i + 1 < format.Length)
            {
                char next = format[i + 1];
                switch (next)
                {
                    case 'd':
                    case 's':
                    case 'f':
                        converted += "{" + argIndex + "}";
                        formattedArgs.Add(args[argIndex]);
                        argIndex++;
                        i++; // skip format char
                        continue;
                    case 'c':
                        converted += "{" + argIndex + "}";
                        object arg = args[argIndex];
                        if (arg is sbyte sb)
                            formattedArgs.Add((char)sb); // sbyte to char
                        else if (arg is int iVal)
                            formattedArgs.Add((char)iVal);
                        else if (arg is char cVal)
                            formattedArgs.Add(cVal);
                        else
                            throw new ArgumentException($"Argument {argIndex} for %c must be a char, int, or sbyte");
                        argIndex++;
                        i++; // skip format char
                        continue;
                }
            }

            converted += format[i];
        }

        converted = converted
            .Replace(@"\n", "\n")
            .Replace(@"\t", "\t")
            .Replace(@"\\", "\\");

        Console.Write(string.Format(converted, formattedArgs.ToArray()));
    }

    public static string Sprintf(string format, params object[] args)
     {
        int argIndex = 0;
        string converted = "";
        List<object> formattedArgs = new List<object>();

        for (int i = 0; i < format.Length; i++)
        {
            if (format[i] == '%' && i + 1 < format.Length)
            {
                char next = format[i + 1];
                switch (next)
                {
                    case 'd':
                    case 's':
                    case 'f':
                        converted += "{" + argIndex + "}";
                        formattedArgs.Add(args[argIndex]);
                        argIndex++;
                        i++; // skip format char
                        continue;
                    case 'c':
                        converted += "{" + argIndex + "}";
                        object arg = args[argIndex];
                        if (arg is sbyte sb)
                            formattedArgs.Add((char)sb); // sbyte to char
                        else if (arg is int iVal)
                            formattedArgs.Add((char)iVal);
                        else if (arg is char cVal)
                            formattedArgs.Add(cVal);
                        else
                            throw new ArgumentException($"Argument {argIndex} for %c must be a char, int, or sbyte");
                        argIndex++;
                        i++; // skip format char
                        continue;
                }
            }

            converted += format[i];
        }
        converted = converted
            .Replace(@"\n", "\n")
            .Replace(@"\t", "\t")
            .Replace(@"\\", "\\");

        return string.Format(converted, formattedArgs.ToArray());
    }
}

`
	str := cse.emitAsString(builtin, indent)
	_ = cse.gir.emitToFileBuffer(str, EmptyVisitMethod)

	cse.insideForPostCond = false
}

func (cse *CSharpEmitter) PostVisitProgram(indent int) {
	emitTokensToFile(cse.file, cse.gir.tokenSlice)
	cse.file.Close()

	// Generate .NET project files if link-runtime is enabled
	if cse.LinkRuntime != "" {
		if err := cse.GenerateCsproj(); err != nil {
			log.Printf("Warning: %v", err)
		}
		if err := cse.CopyGraphicsRuntime(); err != nil {
			log.Printf("Warning: %v", err)
		}
	}
}

func (cse *CSharpEmitter) PreVisitFuncDeclSignatures(indent int) {
	cse.forwardDecls = true
}

func (cse *CSharpEmitter) PostVisitFuncDeclSignatures(indent int) {
	cse.forwardDecls = false
}

func (cse *CSharpEmitter) PreVisitFuncDeclName(node *ast.Ident, indent int) {
	cse.executeIfNotForwardDecls(func() {
		var str string
		if node.Name == "main" {
			str = cse.emitAsString(fmt.Sprintf("Main"), 0)
		} else {
			str = cse.emitAsString(fmt.Sprintf("%s", node.Name), 0)
		}
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken("{", LeftBrace, 1)
		str := cse.emitAsString("\n", 1)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)

		// If we have a pending value declaration from key-value range, emit it now
		if cse.pendingRangeValueDecl {
			valueDecl := cse.emitAsString(fmt.Sprintf("var %s = %s[%s];\n",
				cse.pendingValueName, cse.pendingCollectionExpr, cse.pendingKeyName), indent+2)
			cse.gir.emitToFileBuffer(valueDecl, EmptyVisitMethod)
			cse.pendingRangeValueDecl = false
			cse.pendingValueName = ""
			cse.pendingCollectionExpr = ""
			cse.pendingKeyName = ""
		}
	})
}

func (cse *CSharpEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken("}", RightBrace, 1)
		cse.isArray = false
	})
}

func (cse *CSharpEmitter) PreVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken("(", LeftParen, 0)
	})
}

func (cse *CSharpEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken(")", RightParen, 0)
	})
}

func (cse *CSharpEmitter) PreVisitIdent(e *ast.Ident, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if !cse.shouldGenerate {
			return
		}
		// Skip emission during type alias handling
		if cse.suppressTypeAliasEmit {
			return
		}
		// Skip emission during key-value range key/value visits
		if cse.suppressRangeEmit {
			return
		}
		// Skip emission of type name in type conversions (already emitted in PreVisitCallExprFun)
		if csSuppressTypeCastIdent {
			return
		}
		// Skip package name emission for type alias selector expressions
		// (the X part, not the Sel part - Sel will be emitted with the underlying type)
		if cse.suppressTypeAliasSelectorX {
			// Check if this is the package name (X part) - we need to let the Sel through
			if _, isAlias := cse.typeAliasMap[e.Name]; !isAlias {
				return // Skip the package name
			}
		}
		// Capture to buffer during range collection expression visit
		if cse.captureRangeExpr {
			cse.rangeCollectionExpr += e.Name
			return
		}
		var str string
		name := e.Name
		// Map operation identifier replacements
		if cse.isMapMakeCall && name == "make" {
			name = "hmap.newHashMap"
		} else if cse.isSliceMakeCall && name == "make" {
			name = "new "
		} else if cse.isDeleteCall && name == "delete" {
			name = cse.deleteMapVarName + " = hmap.hashMapDelete"
		} else if cse.isMapLenCall && name == "len" {
			name = "hmap.hashMapLen"
		} else {
			name = cse.lowerToBuiltins(name)
		}
		if name == "nil" {
			str = cse.emitAsString("default", indent)
		} else {
			if n, ok := csTypesMap[name]; ok {
				str = cse.emitAsString(n, indent)
			} else if underlyingType, ok := cse.typeAliasMap[name]; ok {
				// Replace type alias with its underlying type
				str = cse.emitAsString(underlyingType, indent)
			} else {
				str = cse.emitAsString(name, indent)
			}
		}

		cse.emitToken(str, Identifier, 0)
	})
}

// isTypeConversion tracks if current call expression is a type conversion
var csIsTypeConversion bool
var csSuppressTypeCastIdent bool

func (cse *CSharpEmitter) PreVisitCallExpr(node *ast.CallExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Map operations
		if ident, ok := node.Fun.(*ast.Ident); ok {
			// make(map[K]V) or make([]T, n)
			if ident.Name == "make" && len(node.Args) >= 1 {
				if mapType, ok := node.Args[0].(*ast.MapType); ok {
					cse.isMapMakeCall = true
					cse.mapMakeKeyType = cse.getMapKeyTypeConst(mapType)
					csIsTypeConversion = false
					return
				}
				if _, ok := node.Args[0].(*ast.ArrayType); ok && len(node.Args) >= 2 {
					cse.isSliceMakeCall = true
					cse.sliceMakeElemType = "object" // default
					if cse.pkg != nil && cse.pkg.TypesInfo != nil {
						tv := cse.pkg.TypesInfo.Types[node.Args[0]]
						if tv.Type != nil {
							if sliceType, ok := tv.Type.Underlying().(*types.Slice); ok {
								cse.sliceMakeElemType = getCsTypeName(sliceType.Elem())
							}
						}
					}
					csIsTypeConversion = false
					return
				}
			}

			// delete(m, k)
			if ident.Name == "delete" && len(node.Args) >= 2 {
				cse.isDeleteCall = true
				cse.deleteMapVarName = exprToString(node.Args[0])
				cse.mapKeyCastPrefix = ""
				cse.mapKeyCastSuffix = ""
				if cse.pkg != nil && cse.pkg.TypesInfo != nil {
					tv := cse.pkg.TypesInfo.Types[node.Args[0]]
					if tv.Type != nil {
						if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
							cse.mapKeyCastPrefix, cse.mapKeyCastSuffix = getCsKeyCast(mapType.Key())
						}
					}
				}
				csIsTypeConversion = false
				return
			}

			// len(m) where m is a map
			if ident.Name == "len" && len(node.Args) >= 1 {
				if cse.pkg != nil && cse.pkg.TypesInfo != nil {
					tv := cse.pkg.TypesInfo.Types[node.Args[0]]
					if tv.Type != nil {
						if _, ok := tv.Type.Underlying().(*types.Map); ok {
							cse.isMapLenCall = true
							csIsTypeConversion = false
							return
						}
					}
				}
			}
		}

		// Check if this is a type conversion (Fun is an Ident that's a type name)
		if ident, ok := node.Fun.(*ast.Ident); ok {
			// Check if it's a known type
			if _, isType := csTypesMap[ident.Name]; isType {
				csIsTypeConversion = true
				return
			}
			// Also check destTypes directly (for "int", "string", etc.)
			for _, t := range destTypes {
				if ident.Name == t {
					csIsTypeConversion = true
					return
				}
			}
		}
		csIsTypeConversion = false
	})
}

func (cse *CSharpEmitter) PreVisitCallExprFun(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// If this is a type conversion, emit cast syntax: (type)
		if csIsTypeConversion {
			if ident, ok := node.(*ast.Ident); ok {
				typeName := ident.Name
				// Map Go type to C# type
				if mapped, ok := csTypesMap[typeName]; ok {
					typeName = mapped
				}
				cse.emitToken("(", LeftParen, 0)
				cse.emitToken(typeName, Identifier, 0)
				cse.emitToken(")", RightParen, 0)
				// Suppress the normal Ident emission for the type name
				csSuppressTypeCastIdent = true
			}
		}
	})
}

func (cse *CSharpEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Clear the suppression flag after the Fun expression is traversed
		csSuppressTypeCastIdent = false
	})
}

func (cse *CSharpEmitter) PreVisitCallExprArgs(node []ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if cse.isMapMakeCall {
			// Emit (keyTypeConst instead of just (
			cse.emitToken(fmt.Sprintf("(%d", cse.mapMakeKeyType), LeftParen, 0)
			return
		}
		if cse.isSliceMakeCall {
			// Don't emit "(" - it will be emitted later in PreVisitCallExprArg
			return
		}
		cse.emitToken("(", LeftParen, 0)
	})
}

func (cse *CSharpEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if cse.isSliceMakeCall {
			cse.emitToken("])", RightParen, 0)
			cse.isSliceMakeCall = false
			cse.sliceMakeElemType = ""
			cse.isArray = false // Reset array flag set by ArrayType traversal
			return
		}
		cse.emitToken(")", RightParen, 0)
		// Reset map-related call flags
		cse.isMapMakeCall = false
		if cse.isDeleteCall {
			cse.mapKeyCastPrefix = ""
			cse.mapKeyCastSuffix = ""
		}
		cse.isDeleteCall = false
		cse.deleteMapVarName = ""
		cse.isMapLenCall = false
	})
}

func (cse *CSharpEmitter) PreVisitBasicLit(e *ast.BasicLit, indent int) {
	cse.executeIfNotForwardDecls(func() {
		var str string
		if e.Kind == token.STRING {
			// Use a local copy to avoid mutating the AST (which affects other emitters)
			value := e.Value
			if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
				// Remove only the outer quotes, keep escaped content intact
				value = value[1 : len(value)-1]
				// Use regular C# string (not verbatim) to preserve escape sequences
				str = (cse.emitAsString(fmt.Sprintf("\"%s\"", value), 0))
			} else if len(value) >= 2 && value[0] == '`' && value[len(value)-1] == '`' {
				// Raw string literal - use C# verbatim string
				value = value[1 : len(value)-1]
				str = (cse.emitAsString(fmt.Sprintf("@\"%s\"", value), 0))
			} else {
				str = (cse.emitAsString(fmt.Sprintf("\"%s\"", value), 0))
			}
			cse.emitToken(str, StringLiteral, 0)
		} else {
			str = (cse.emitAsString(e.Value, 0))
			cse.emitToken(str, NumberLiteral, 0)
		}
	})
}

func (cse *CSharpEmitter) PreVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Reset isArray flag at the start of each variable declaration
		// This prevents stale state from previous declarations affecting this one
		cse.isArray = false
		// Set type context flag - the variable type will be visited next
		cse.inTypeContext = true
		// Detect var m map[K]V declarations (no initialization value)
		if len(node.Values) == 0 && node.Type != nil {
			if cse.pkg != nil && cse.pkg.TypesInfo != nil {
				if typeAndValue, ok := cse.pkg.TypesInfo.Types[node.Type]; ok {
					if _, isMap := typeAndValue.Type.Underlying().(*types.Map); isMap {
						if mapType, ok := node.Type.(*ast.MapType); ok {
							cse.pendingMapInit = true
							cse.pendingMapKeyType = cse.getMapKeyTypeConst(mapType)
						}
					}
				}
			}
		}
	})
}

func (cse *CSharpEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Clear type context flag - type has been visited
		cse.inTypeContext = false
		pointerAndPosition := SearchPointerIndexReverse(PreVisitDeclStmtValueSpecType, cse.gir.pointerAndIndexVec)
		if pointerAndPosition != nil {
			for aliasName, alias := range cse.aliases {
				if alias.UnderlyingType == cse.pkg.TypesInfo.Types[node.Type].Type.Underlying().String() {
					cse.gir.tokenSlice, _ = RewriteTokensBetween(cse.gir.tokenSlice, pointerAndPosition.Index, len(cse.gir.tokenSlice), []string{aliasName})
				}
			}
		}
	})
}

func (cse *CSharpEmitter) PreVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString(" ", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		var str string
		if cse.pendingMapInit {
			str += fmt.Sprintf(" = hmap.newHashMap(%d);", cse.pendingMapKeyType)
			cse.pendingMapInit = false
			cse.pendingMapKeyType = 0
		} else if cse.isArray {
			str += " = new "
			str += strings.TrimSpace(cse.arrayType)
			str += "();"
			cse.isArray = false
		} else {
			str += " = default;"
		}
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitGenStructFieldType(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("public ", indent+2)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.gir.emitToFileBuffer(" ", EmptyVisitMethod)
		// clean array marker as we should generate
		// initializer only for expression statements
		// not for struct fields
		cse.isArray = false
	})
}

func (cse *CSharpEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.gir.emitToFileBuffer(";\n", EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	cse.executeIfNotForwardDecls(func() {
		name := pkg.Name
		cse.pkg = pkg
		var packageName string
		if name == "main" {
			packageName = "MainClass"
		} else {
			//packageName = capitalizeFirst(name)
			packageName = name
		}
		// Use a static class instead of namespace so we can have both types and functions
		// This allows pkg.Type and pkg.Function without needing .Api.
		str := cse.emitAsString(fmt.Sprintf("public static class %s {\n\n", packageName), indent)
		err := cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		err = cse.gir.emitToFileBufferString("", pkg.Name)
		cse.currentPackage = packageName
		cse.apiClassOpened = true // Class is already open (no separate Api class needed)
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
		}
	})
}

func (cse *CSharpEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Note: Type aliases (using X = T) are not emitted here because they can't be inside a class.
		// They would need to be at namespace or file level, but since we use static classes,
		// we skip alias emission. The actual types will be used directly where needed.

		// Close the package class
		err := cse.gir.emitToFileBuffer("}\n", EmptyVisitMethod)
		if err != nil {
			fmt.Println("Error writing to file:", err)
			return
		}
	})
}

func (cse *CSharpEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.isArray = false
	})
}

func (cse *CSharpEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("\n", indent)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("\n\n", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitGenStructInfo(node GenTypeInfo, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString(fmt.Sprintf("public struct %s\n", node.Name), indent+2)
		str += cse.emitAsString("{\n", indent+2)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("};\n\n", indent+2)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitArrayType(node ast.ArrayType, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Skip emission during type alias handling
		if cse.suppressTypeAliasEmit {
			return
		}
		str := cse.emitAsString("List", indent)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		str = cse.emitAsString("<", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}
func (cse *CSharpEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Skip emission during type alias handling
		if cse.suppressTypeAliasEmit {
			return
		}
		str := cse.emitAsString(">", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)

		// Don't set isArray when inside a slice make call
		if cse.isSliceMakeCall {
			return
		}

		pointerAndPosition := SearchPointerIndexReverse(PreVisitArrayType, cse.gir.pointerAndIndexVec)
		if pointerAndPosition != nil {
			tokens, _ := ExtractTokens(pointerAndPosition.Index, cse.gir.tokenSlice)
			cse.isArray = true
			cse.arrayType = strings.Join(tokens, "")
		}
	})
}

func (cse *CSharpEmitter) PreVisitMapType(node *ast.MapType, indent int) {
	// Skip when inside make(map[K]V) — already handled by make lowering
	if cse.isMapMakeCall {
		return
	}
	cse.executeIfNotForwardDecls(func() {
		cse.gir.emitToFileBuffer("hmap.HashMap", EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PostVisitMapType(node *ast.MapType, indent int) {
}

// PreVisitMapKeyType suppresses output during map key type traversal
func (cse *CSharpEmitter) PreVisitMapKeyType(node ast.Expr, indent int) {
	cse.suppressMapEmit = true
}

// PostVisitMapKeyType re-enables output after map key type traversal
func (cse *CSharpEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {
	cse.suppressMapEmit = false
}

// PreVisitMapValueType suppresses output during map value type traversal
func (cse *CSharpEmitter) PreVisitMapValueType(node ast.Expr, indent int) {
	cse.suppressMapEmit = true
}

// PostVisitMapValueType re-enables output after map value type traversal
func (cse *CSharpEmitter) PostVisitMapValueType(node ast.Expr, indent int) {
	cse.suppressMapEmit = false
}

func (cse *CSharpEmitter) PreVisitFuncType(node *ast.FuncType, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// All types within FuncType are type references
		cse.inTypeContext = true
		var str string
		if node.Results != nil {
			str = cse.emitAsString("Func", indent)
		} else {
			str = cse.emitAsString("Action", indent)
		}
		str += cse.emitAsString("<", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}
func (cse *CSharpEmitter) PostVisitFuncType(node *ast.FuncType, indent int) {
	cse.executeIfNotForwardDecls(func() {
		pointerAndPosition := SearchPointerIndexReverse(PreVisitFuncType, cse.gir.pointerAndIndexVec)
		if pointerAndPosition != nil && cse.numFuncResults > 0 {
			// For function types with return values, we need to reorder tokens
			// to move return type to the end (C# syntax requirement)
			tokens, _ := ExtractTokens(pointerAndPosition.Index, cse.gir.tokenSlice)
			if len(tokens) > 2 {
				// Find and move return type to end with comma separator
				var reorderedTokens []string
				reorderedTokens = append(reorderedTokens, tokens[0]) // "Func<" or "Action<"
				if len(tokens) > 3 {
					// Skip return type (index 1) and add parameters first
					reorderedTokens = append(reorderedTokens, tokens[2:]...)
					reorderedTokens = append(reorderedTokens, ",")
					reorderedTokens = append(reorderedTokens, tokens[1]) // Add return type at end
				}
				cse.gir.tokenSlice, _ = RewriteTokensBetween(cse.gir.tokenSlice, pointerAndPosition.Index, len(cse.gir.tokenSlice), reorderedTokens)
			}
		}

		str := cse.emitAsString(">", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		// Clear type context flag
		cse.inTypeContext = false
	})
}

func (cse *CSharpEmitter) PreVisitFuncTypeParam(node *ast.Field, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if index > 0 {
			str := cse.emitAsString(", ", 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PreVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Check if the selector is a type alias - if so, suppress package prefix emission
		if node.Sel != nil {
			if _, isAlias := cse.typeAliasMap[node.Sel.Name]; isAlias {
				cse.suppressTypeAliasSelectorX = true
			}
		}
	})
}

func (cse *CSharpEmitter) PostVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Reset the flag after processing the selector expression
		cse.suppressTypeAliasSelectorX = false
	})
}

func (cse *CSharpEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Skip emitting the dot if we're in a type alias selector
		if cse.suppressTypeAliasSelectorX {
			return
		}
		// Skip emitting the dot when map operations suppress emission
		if cse.suppressMapEmit {
			return
		}
		var str string
		scopeOperator := "."
		if ident, ok := node.(*ast.Ident); ok {
			if cse.lowerToBuiltins(ident.Name) == "" {
				return
			}
			// No need to add .Api. - everything is in the package class directly
		}

		str = cse.emitAsString(scopeOperator, 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitFuncTypeResults(node *ast.FieldList, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if node != nil {
			cse.numFuncResults = len(node.List)
		}
	})
}

func (cse *CSharpEmitter) PreVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if index > 0 {
			str := cse.emitAsString(", ", 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
		// Set type context flag - the parameter type will be visited next
		cse.inTypeContext = true
	})
}

func (cse *CSharpEmitter) PreVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Clear type context flag - type has been visited, now emitting name
		cse.inTypeContext = false
		cse.gir.emitToFileBuffer(" ", EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if index > 0 {
			cse.emitToken(",", Comma, 0)
		}
		// Set type context flag - the return type will be visited next
		cse.inTypeContext = true
	})
}

func (cse *CSharpEmitter) PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Clear type context flag - type has been visited
		cse.inTypeContext = false
		pointerAndPosition := SearchPointerIndexReverse(PreVisitFuncDeclSignatureTypeResultsList, cse.gir.pointerAndIndexVec)
		if pointerAndPosition != nil {
			adjustment := 0
			// Check for comma after the type to adjust index
			if cse.gir.tokenSlice[pointerAndPosition.Index].Content == "," {
				adjustment = 1
			}
			for aliasName, alias := range cse.aliases {
				if alias.UnderlyingType == cse.pkg.TypesInfo.Types[node.Type].Type.Underlying().String() {
					cse.gir.tokenSlice, _ = RewriteTokensBetween(cse.gir.tokenSlice, pointerAndPosition.Index+adjustment, len(cse.gir.tokenSlice), []string{aliasName})
				}
			}
		}
	})
}

func (cse *CSharpEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Functions are directly in the package class (no separate Api class)
		str := cse.emitAsString("public static ", indent+2)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		if node.Type.Results != nil {
			if len(node.Type.Results.List) > 1 {
				cse.emitToken("(", LeftParen, 0)
			}
		} else {
			str := cse.emitAsString("void", 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if node.Type.Results != nil {
			if len(node.Type.Results.List) > 1 {
				cse.emitToken(")", RightParen, 0)
			}
		}

		str := cse.emitAsString("", 1)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitTypeAliasName(node *ast.Ident, indent int) {
	// Note: Type aliases (using X = T;) are not supported inside static classes.
	// We collect the alias name here and will map it to the underlying type.
	cse.suppressTypeAliasEmit = true
	cse.currentAliasName = node.Name
}

func (cse *CSharpEmitter) PostVisitTypeAliasName(node *ast.Ident, indent int) {
	// Skipped - see PreVisitTypeAliasName
}

func (cse *CSharpEmitter) PreVisitTypeAliasType(node ast.Expr, indent int) {
	// Skipped - see PreVisitTypeAliasName
}

func ConvertToAliasRepr(types []string, pkgName []string) []AliasRepr {
	var result []AliasRepr
	for i, t := range types {
		result = append(result, AliasRepr{
			PackageName: pkgName[i], // or derive if format is pkg.Type
			TypeName:    t,
		})
	}
	return result
}

func ParseNestedTypes(s string) []string {
	var result []string
	s = strings.TrimSpace(s)

	for strings.HasPrefix(s, "List<") {
		result = append(result, "List")
		s = strings.TrimPrefix(s, "List<")
		s = strings.TrimSuffix(s, ">")
	}

	// Add the final inner type (e.g., "int", "string", "MyType")
	s = strings.TrimSpace(s)
	if s != "" {
		result = append(result, s)
	}

	return result
}

func (cse *CSharpEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	// Store the alias mapping: aliasName -> underlyingType (converted to C# syntax)
	if cse.currentAliasName != "" {
		// Get the underlying type from the type info
		if tv, ok := cse.pkg.TypesInfo.Types[node]; ok && tv.Type != nil {
			underlyingType := tv.Type.String()
			// Convert Go type to C# syntax (handles slices, package paths, basic types)
			underlyingType = cse.convertGoTypeToCSharp(underlyingType)
			if cse.typeAliasMap == nil {
				cse.typeAliasMap = make(map[string]string)
			}
			cse.typeAliasMap[cse.currentAliasName] = underlyingType
		}
	}
	// Reset flags
	cse.suppressTypeAliasEmit = false
	cse.currentAliasName = ""
}

func (cse *CSharpEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("return ", indent)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)

		if len(node.Results) == 1 {
			tv := cse.pkg.TypesInfo.Types[node.Results[0]]
			//pos := cse.pkg.Fset.Position(node.Pos())
			//fmt.Printf("@@Type: %s %s:%d:%d\n", tv.Type, pos.Filename, pos.Line, pos.Column)
			if tv.Type != nil {
				if typeVal, ok := csTypesMap[tv.Type.String()]; ok {
					if !cse.isTuple && tv.Type.String() != "func()" {
						cse.emitToken("(", LeftParen, 0)
						str := cse.emitAsString(typeVal, 0)
						cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
						cse.emitToken(")", RightParen, 0)
					}
				}
			}
		}
		if len(node.Results) > 1 {
			cse.emitToken("(", LeftParen, 0)
		}
	})
}

func (cse *CSharpEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if len(node.Results) > 1 {
			cse.emitToken(")", RightParen, 0)
		}
		str := cse.emitAsString(";", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if index > 0 {
			str := cse.emitAsString(", ", 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		pointerAndPosition := SearchPointerIndexReverse(PreVisitCallExpr, cse.gir.pointerAndIndexVec)
		if pointerAndPosition != nil {
			tokens, _ := ExtractTokens(pointerAndPosition.Index, cse.gir.tokenSlice)
			for _, t := range destTypes {
				if len(tokens) >= 2 && tokens[0] == t && tokens[1] == "(" {
					cse.gir.tokenSlice, _ = RewriteTokens(cse.gir.tokenSlice, pointerAndPosition.Index, []string{tokens[0], tokens[1]}, []string{"(", t, ")", "("})
				}
			}
		}
	})
}

func (cse *CSharpEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
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
		cse.suppressRangeEmit = true
		return
	}

	// Detect comma-ok: val, ok := m[key]
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if indexExpr, ok := node.Rhs[0].(*ast.IndexExpr); ok {
			if cse.pkg != nil && cse.pkg.TypesInfo != nil {
				tv := cse.pkg.TypesInfo.Types[indexExpr.X]
				if tv.Type != nil {
					if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
						cse.isMapCommaOk = true
						cse.mapCommaOkValName = node.Lhs[0].(*ast.Ident).Name
						cse.mapCommaOkOkName = node.Lhs[1].(*ast.Ident).Name
						cse.mapCommaOkMapName = exprToString(indexExpr.X)
						cse.mapCommaOkValType = getCsTypeName(mapType.Elem())
						cse.mapCommaOkIsDecl = (node.Tok == token.DEFINE)
						cse.mapCommaOkIndent = indent
						cse.mapKeyCastPrefix, cse.mapKeyCastSuffix = getCsKeyCast(mapType.Key())
						cse.suppressMapEmit = true
						return
					}
				}
			}
		}
	}
	// Detect type assertion comma-ok: val, ok := x.(Type)
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if typeAssert, ok := node.Rhs[0].(*ast.TypeAssertExpr); ok {
			if cse.pkg != nil && cse.pkg.TypesInfo != nil {
				tv := cse.pkg.TypesInfo.Types[typeAssert.Type]
				if tv.Type != nil {
					cse.isTypeAssertCommaOk = true
					cse.typeAssertCommaOkValName = node.Lhs[0].(*ast.Ident).Name
					cse.typeAssertCommaOkOkName = node.Lhs[1].(*ast.Ident).Name
					cse.typeAssertCommaOkType = getCsTypeName(tv.Type)
					cse.typeAssertCommaOkIsDecl = (node.Tok == token.DEFINE)
					cse.typeAssertCommaOkIndent = indent
					cse.suppressMapEmit = true
					return
				}
			}
		}
	}
	// Detect map assignment: m[k] = v -> m = hmap.hashMapSet(m, k, v)
	if len(node.Lhs) == 1 {
		if indexExpr, ok := node.Lhs[0].(*ast.IndexExpr); ok {
			if cse.pkg != nil && cse.pkg.TypesInfo != nil {
				tv := cse.pkg.TypesInfo.Types[indexExpr.X]
				if tv.Type != nil {
					if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
						cse.isMapAssign = true
						cse.mapAssignIndent = indent
						cse.suppressMapEmit = true
						cse.assignmentToken = "="
						cse.mapAssignVarName = exprToString(indexExpr.X)
						cse.mapKeyCastPrefix, cse.mapKeyCastSuffix = getCsKeyCast(mapType.Key())
						return // Skip normal indent emission
					}
				}
			}
		}
	}

	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("", indent)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	// Reset blank identifier suppression if it was set
	if cse.suppressRangeEmit {
		cse.suppressRangeEmit = false
		return
	}
	// Handle type assertion comma-ok: val, ok := x.(Type)
	if cse.isTypeAssertCommaOk {
		cse.executeIfNotForwardDecls(func() {
			expr := cse.capturedMapKey
			typeName := cse.typeAssertCommaOkType
			decl := ""
			if cse.typeAssertCommaOkIsDecl {
				decl = "var "
			}
			indentStr := cse.emitAsString("", cse.typeAssertCommaOkIndent)
			cse.suppressMapEmit = false
			cse.gir.emitToFileBuffer(fmt.Sprintf("%s%s%s = %s is %s;",
				indentStr, decl, cse.typeAssertCommaOkOkName, expr, typeName), EmptyVisitMethod)
			cse.gir.emitToFileBuffer(fmt.Sprintf("\n%s%s%s = %s ? (%s)%s : default(%s);",
				indentStr, decl, cse.typeAssertCommaOkValName, cse.typeAssertCommaOkOkName,
				typeName, expr, typeName), EmptyVisitMethod)
		})
		cse.isTypeAssertCommaOk = false
		cse.capturedMapKey = ""
		cse.captureMapKey = false
		cse.suppressMapEmit = false
		return
	}
	// Handle comma-ok: val, ok := m[key]
	if cse.isMapCommaOk {
		cse.executeIfNotForwardDecls(func() {
			key := cse.mapKeyCastPrefix + cse.capturedMapKey + cse.mapKeyCastSuffix
			decl := ""
			if cse.mapCommaOkIsDecl {
				decl = "var "
			}
			indentStr := cse.emitAsString("", cse.mapCommaOkIndent)
			cse.suppressMapEmit = false
			valType := cse.mapCommaOkValType
			mapName := cse.mapCommaOkMapName
			cse.gir.emitToFileBuffer(fmt.Sprintf("%s%s%s = hmap.hashMapContains(%s, %s);",
				indentStr, decl, cse.mapCommaOkOkName, mapName, key), EmptyVisitMethod)
			cse.gir.emitToFileBuffer(fmt.Sprintf("\n%s%s%s = %s ? (%s)hmap.hashMapGet(%s, %s) : default(%s);",
				indentStr, decl, cse.mapCommaOkValName, cse.mapCommaOkOkName,
				valType, mapName, key, valType), EmptyVisitMethod)
		})
		cse.isMapCommaOk = false
		cse.capturedMapKey = ""
		cse.captureMapKey = false
		cse.suppressMapEmit = false
		cse.mapKeyCastPrefix = ""
		cse.mapKeyCastSuffix = ""
		return
	}
	// Close hashMapSet call for map assignment
	if cse.isMapAssign {
		cse.executeIfNotForwardDecls(func() {
			str := cse.emitAsString(");", 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		})
		cse.isMapAssign = false
		cse.suppressMapEmit = false
		cse.captureMapKey = false
		cse.mapAssignVarName = ""
		cse.mapKeyCastPrefix = ""
		cse.mapKeyCastSuffix = ""
		return
	}
	cse.executeIfNotForwardDecls(func() {
		// Don't emit semicolon inside for loop post statement
		if !cse.insideForPostCond {
			str := cse.emitAsString(";", 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PreVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if cse.isMapAssign {
			// Emit: varName = hmap.hashMapSet(varName, capturedKey,
			key := cse.mapKeyCastPrefix + strings.TrimSpace(cse.capturedMapKey) + cse.mapKeyCastSuffix
			str := cse.emitAsString(cse.mapAssignVarName+" = hmap.hashMapSet("+cse.mapAssignVarName+", "+key+", ", cse.mapAssignIndent)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
			return
		}
		opTokenType := cse.getTokenType(cse.assignmentToken)
		cse.emitToken(cse.assignmentToken, opTokenType, indent+1)
		cse.emitToken(" ", WhiteSpace, 0)
	})
}

func (cse *CSharpEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.isTuple = false
	})
}

func (cse *CSharpEmitter) PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		pointerAndPosition := SearchPointerIndexReverse(PreVisitAssignStmtRhsExpr, cse.gir.pointerAndIndexVec)
		rewritten := false
		if pointerAndPosition != nil {
			tokens, _ := ExtractTokens(pointerAndPosition.Index, cse.gir.tokenSlice)
			for _, t := range destTypes {
				if len(tokens) >= 2 && tokens[0] == t && tokens[1] == "(" {
					cse.gir.tokenSlice, _ = RewriteTokens(cse.gir.tokenSlice, pointerAndPosition.Index, []string{tokens[0], tokens[1]}, []string{"(", t, ")", "("})
				}
			}
		}

		if !rewritten {
			tv := cse.pkg.TypesInfo.Types[node]
			//pos := cse.pkg.Fset.Position(node.Pos())
			//fmt.Printf("@@Type: %s %s:%d:%d\n", tv.Type, pos.Filename, pos.Line, pos.Column)
			if tv.Type != nil {
				if typeVal, ok := csTypesMap[tv.Type.String()]; ok {
					if !cse.isTuple && tv.Type.String() != "func()" {
						cse.gir.tokenSlice, _ = RewriteTokens(cse.gir.tokenSlice, pointerAndPosition.Index, []string{}, []string{"(", typeVal, ")"})
					}
				}
			}
		}
	})
}

func (cse *CSharpEmitter) PreVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if cse.isMapAssign || cse.isMapCommaOk || cse.isTypeAssertCommaOk {
			return // Skip for map assignment or comma-ok
		}
		if index > 0 {
			str := cse.emitAsString(", ", indent)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PreVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if cse.isMapAssign || cse.isMapCommaOk || cse.isTypeAssertCommaOk {
			return // Skip for map assignment or comma-ok
		}
		assignmentToken := node.Tok.String()
		if assignmentToken == ":=" && len(node.Lhs) == 1 {
			str := cse.emitAsString("var ", indent)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		} else if assignmentToken == ":=" && len(node.Lhs) > 1 {
			str := cse.emitAsString("var ", indent)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
			cse.emitToken("(", LeftParen, 0)
		} else if assignmentToken == "=" && len(node.Lhs) > 1 {
			cse.emitToken("(", LeftParen, indent)
			cse.isTuple = true
		}
		// Preserve compound assignment operators, convert := to =
		if assignmentToken != "+=" && assignmentToken != "-=" && assignmentToken != "*=" && assignmentToken != "/=" {
			assignmentToken = "="
		}
		cse.assignmentToken = assignmentToken
	})
}

func (cse *CSharpEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if cse.isMapAssign || cse.isMapCommaOk || cse.isTypeAssertCommaOk {
			return // Skip for map assignment or comma-ok
		}
		if node.Tok.String() == ":=" && len(node.Lhs) > 1 {
			cse.emitToken(")", RightParen, indent)
		} else if node.Tok.String() == "=" && len(node.Lhs) > 1 {
			cse.emitToken(")", RightParen, indent)
		}
	})
}

func (cse *CSharpEmitter) PreVisitIndexExpr(node *ast.IndexExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if cse.isMapAssign || cse.isMapCommaOk {
			return // Map assignment or comma-ok - suppressed
		}
		// Detect map index read: m[k] -> (ValueType)hmap.hashMapGet(m, k)
		if cse.pkg != nil && cse.pkg.TypesInfo != nil {
			tv := cse.pkg.TypesInfo.Types[node.X]
			if tv.Type != nil {
				if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
					cse.isMapIndex = true
					cse.mapIndexValueType = getCsTypeName(mapType.Elem())
					cse.mapKeyCastPrefix, cse.mapKeyCastSuffix = getCsKeyCast(mapType.Key())
					// Emit (ValueType)hmap.hashMapGet(
					str := cse.emitAsString("("+cse.mapIndexValueType+")hmap.hashMapGet(", 0)
					cse.emitToken(str, Identifier, 0)
				}
			}
		}
	})
}
func (cse *CSharpEmitter) PostVisitIndexExprX(node *ast.IndexExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if cse.isMapIndex {
			if cse.mapKeyCastPrefix != "" {
				cse.emitToken(", "+cse.mapKeyCastPrefix, Comma, 0)
			} else {
				cse.emitToken(", ", Comma, 0)
			}
		}
	})
}
func (cse *CSharpEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.isMapIndex = false
		// Don't reset cast fields if isMapAssign - they're used in PreVisitAssignStmtRhs
		if !cse.isMapAssign && !cse.isMapCommaOk {
			cse.mapKeyCastPrefix = ""
			cse.mapKeyCastSuffix = ""
		}
	})
}
func (cse *CSharpEmitter) PreVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if cse.isMapAssign {
			// Start capturing key expression
			cse.suppressMapEmit = false
			cse.captureMapKey = true
			cse.capturedMapKey = ""
			return
		}
		if cse.isMapCommaOk {
			// Start capturing key expression for comma-ok
			cse.suppressMapEmit = false
			cse.captureMapKey = true
			cse.capturedMapKey = ""
			return
		}
		if cse.isMapIndex {
			return // Don't emit "[" for map read
		}
		cse.emitToken("[", LeftBracket, 0)
	})
}
func (cse *CSharpEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if cse.isMapAssign {
			// Stop capturing key expression
			cse.captureMapKey = false
			return
		}
		if cse.isMapCommaOk {
			// Stop capturing key expression, re-suppress
			cse.captureMapKey = false
			cse.suppressMapEmit = true
			return
		}
		if cse.isMapIndex {
			if cse.mapKeyCastSuffix != "" {
				cse.emitToken(cse.mapKeyCastSuffix, RightParen, 0)
			}
			cse.emitToken(")", RightParen, 0)
			return
		}
		cse.emitToken("]", RightBracket, 0)
	})
}

func (cse *CSharpEmitter) PreVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken("(", LeftParen, 1)
	})
}
func (cse *CSharpEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken(")", RightParen, 1)
	})
}

func (cse *CSharpEmitter) PreVisitBinaryExprOperator(op token.Token, indent int) {
	cse.executeIfNotForwardDecls(func() {
		opTokenType := cse.getTokenType(op.String())
		cse.emitToken(op.String(), opTokenType, 1)
		cse.emitToken(" ", WhiteSpace, 0)
	})
}

func (cse *CSharpEmitter) PreVisitCallExprArg(node ast.Expr, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// For slice make: suppress first arg (ArrayType), emit "(new ElemType[" before second arg
		if cse.isSliceMakeCall {
			if index == 0 {
				return // Suppress first arg separator
			}
			if index == 1 {
				str := cse.emitAsString("(new "+cse.sliceMakeElemType+"[", 0)
				cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
				return
			}
		}
		if index > 0 {
			str := cse.emitAsString(", ", 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
		// Wrap delete key argument with appropriate cast
		if cse.isDeleteCall && cse.mapKeyCastPrefix != "" && index == 1 {
			str := cse.emitAsString(cse.mapKeyCastPrefix, 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if cse.isDeleteCall && cse.mapKeyCastSuffix != "" && index == 1 {
			str := cse.emitAsString(cse.mapKeyCastSuffix, 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString(";", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if node.Init != nil {
			str := cse.emitAsString("{\n", indent)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if node.Init != nil {
			str := cse.emitAsString("}\n", indent)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PreVisitIfStmtCond(node *ast.IfStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("if ", 1)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		cse.emitToken("(", LeftParen, 0)
	})
}

func (cse *CSharpEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken(")", RightParen, 0)
		str := cse.emitAsString("\n", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("for ", indent)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		cse.emitToken("(", LeftParen, 0)
	})
}

func (cse *CSharpEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if node == nil {
			str := cse.emitAsString(";", 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PreVisitForStmtPost(node ast.Stmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if node != nil {
			cse.insideForPostCond = true
		}
	})
}

func (cse *CSharpEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.insideForPostCond = false
		cse.emitToken(")", RightParen, 0)
		str := cse.emitAsString("\n", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitIfStmtElse(node *ast.IfStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("else", 1)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString(";", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.insideForPostCond = false
	})
}

func (cse *CSharpEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Check if this is a key-value range (both Key and Value present)
		if node.Key != nil && node.Value != nil {
			cse.isKeyValueRange = true
			cse.rangeKeyName = node.Key.(*ast.Ident).Name
			cse.rangeValueName = node.Value.(*ast.Ident).Name
			cse.rangeCollectionExpr = ""
			cse.suppressRangeEmit = true
			cse.rangeStmtIndent = indent
			// Don't emit anything yet - we'll emit the complete for loop in PostVisitRangeStmtX
		} else {
			cse.isKeyValueRange = false
			str := cse.emitAsString("foreach ", indent)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
			cse.emitToken("(", LeftParen, 0)
			str = cse.emitAsString("var ", 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PreVisitRangeStmtKey(node ast.Expr, indent int) {
	// For key-value range, we've already captured the key name
}

func (cse *CSharpEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	// Nothing special needed here
}

func (cse *CSharpEmitter) PreVisitRangeStmtValue(node ast.Expr, indent int) {
	// For key-value range, we've already captured the value name
}

func (cse *CSharpEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if cse.isKeyValueRange {
			// Stop suppressing, start capturing collection expression
			cse.suppressRangeEmit = false
			cse.captureRangeExpr = true
		} else {
			str := cse.emitAsString(" in ", 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PreVisitRangeStmtX(node ast.Expr, indent int) {
	// For key-value range, we're already in capture mode
}

func (cse *CSharpEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if cse.isKeyValueRange {
			// Stop capturing and emit the complete for loop
			cse.captureRangeExpr = false
			collection := cse.rangeCollectionExpr
			key := cse.rangeKeyName
			value := cse.rangeValueName
			indent := cse.rangeStmtIndent

			// Emit: for (int key = 0; key < collection.Count; key++)
			str := cse.emitAsString(fmt.Sprintf("for (int %s = 0; %s < %s.Count; %s++)\n", key, key, collection, key), indent)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)

			// Set pending value declaration to be emitted at start of body block
			cse.pendingRangeValueDecl = true
			cse.pendingValueName = value
			cse.pendingCollectionExpr = collection
			cse.pendingKeyName = key

			// Reset range state
			cse.isKeyValueRange = false
			cse.rangeKeyName = ""
			cse.rangeValueName = ""
			cse.rangeCollectionExpr = ""
		} else {
			cse.emitToken(")", RightParen, 0)
			str := cse.emitAsString("\n", 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	// Reset any range-related state
}

func (cse *CSharpEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString(node.Tok.String(), 0)
		if !cse.insideForPostCond {
			str += cse.emitAsString(";", 0)
		}
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitCompositeLitType(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("new ", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		// Set type context flag - the composite literal type will be visited next
		cse.inTypeContext = true
	})
}

func (cse *CSharpEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Clear type context flag - type has been visited
		cse.inTypeContext = false
		pointerAndPosition := SearchPointerIndexReverse(PreVisitCompositeLitType, cse.gir.pointerAndIndexVec)
		if pointerAndPosition != nil {
			// TODO not very effective
			// go through all aliases and check if the underlying type matches
			for aliasName, alias := range cse.aliases {
				if alias.UnderlyingType == cse.pkg.TypesInfo.Types[node].Type.Underlying().String() {
					const newKeywordIndex = 1
					cse.gir.tokenSlice, _ = RewriteTokensBetween(cse.gir.tokenSlice, pointerAndPosition.Index+newKeywordIndex, len(cse.gir.tokenSlice), []string{aliasName})
				}
			}
		}
	})
}

func (cse *CSharpEmitter) PreVisitCompositeLitElts(node []ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("{", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("}", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if index > 0 {
			str := cse.emitAsString(", ", 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

// LIMITATION: C# slice syntax (arr[low..high]) has issues with the current emitter.
// The slice expression `arr[:high]` may emit incorrectly as `arr[....high]` (4 dots instead of 2).
// Workaround: Instead of using slice syntax like `arr = arr[:n]`, use a manual loop:
//
//	newArr := []T{}
//	i := 0
//	for {
//	    if i >= n { break }
//	    newArr = append(newArr, arr[i])
//	    i = i + 1
//	}
//	arr = newArr
//
// This limitation is tracked and should be fixed in a future version.
func (cse *CSharpEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken("[", LeftBracket, 0)
	})
}

func (cse *CSharpEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken("]", RightBracket, 0)
	})
}

func (cse *CSharpEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.gir.emitToFileBuffer("..", EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitFuncLit(node *ast.FuncLit, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken("(", LeftParen, indent)
	})
}
func (cse *CSharpEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	// Note: Don't emit } here - PostVisitBlockStmt will handle it
}

func (cse *CSharpEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken(")", RightParen, 0)
		str := cse.emitAsString("=>", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := ""
		if index > 0 {
			str += cse.emitAsString(", ", 0)
		}
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		// Set type context flag - the parameter type will be visited next
		cse.inTypeContext = true
	})
}

func (cse *CSharpEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Clear type context flag - type has been visited
		cse.inTypeContext = false
		str := cse.emitAsString(" ", 0)
		str += cse.emitAsString(node.Names[0].Name, indent)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	// Note: Don't emit { here - PreVisitBlockStmt will handle it
}

func (cse *CSharpEmitter) PreVisitFuncLitTypeResult(node *ast.Field, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.shouldGenerate = false
	})
}

func (cse *CSharpEmitter) PostVisitFuncLitTypeResult(node *ast.Field, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.shouldGenerate = true
	})
}

func (cse *CSharpEmitter) PreVisitInterfaceType(node *ast.InterfaceType, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("object", indent)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitKeyValueExprValue(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("= ", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken("(", LeftParen, 0)
		str := cse.emitAsString(node.Op.String(), 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}
func (cse *CSharpEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken(")", RightParen, 0)
	})
}

func trimBeforeChar(s string, ch byte) string {
	pos := strings.IndexByte(s, ch)
	if pos == -1 {
		return s // character not found
	}
	return s[pos+1:]
}

func (cse *CSharpEmitter) PreVisitGenDeclConstName(node *ast.Ident, indent int) {
	cse.executeIfNotForwardDecls(func() {
		// Constants are directly in the package class (no separate Api class)
		// TODO dummy implementation
		// not very well performed
		for constIdent, obj := range cse.pkg.TypesInfo.Defs {
			if obj == nil {
				continue
			}
			if con, ok := obj.(*types.Const); ok {
				if constIdent.Name != node.Name {
					continue
				}
				constType := con.Type().String()
				constType = strings.TrimPrefix(constType, "untyped ")
				if constType == cse.pkg.TypesInfo.Defs[node].Type().String() {
					constType = trimBeforeChar(constType, '.')
				}
				// Map Go types to C# types (e.g., int8 -> sbyte)
				if mappedType, ok := csTypesMap[constType]; ok {
					constType = mappedType
				}
				// Check if it's a type alias and replace with underlying type
				if underlyingType, ok := cse.typeAliasMap[constType]; ok {
					constType = underlyingType
				}
				str := cse.emitAsString(fmt.Sprintf("public const %s %s = ", constType, node.Name), indent+2)

				cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
			}
		}
	})
}
func (cse *CSharpEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString(";\n", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}
func (cse *CSharpEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("\n", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitSliceExprXBegin(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.shouldGenerate = false
	})
}

func (cse *CSharpEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.shouldGenerate = true
	})
}

func (cse *CSharpEmitter) PreVisitSliceExprXEnd(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.shouldGenerate = false
	})
}

func (cse *CSharpEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.shouldGenerate = true
	})
}

func (cse *CSharpEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("switch ", indent)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		cse.emitToken("(", LeftParen, 0)
	})
}
func (cse *CSharpEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("}", indent)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken(")", RightParen, 0)
		str := cse.emitAsString(" ", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		str = cse.emitAsString("{\n", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	cse.executeIfNotForwardDecls(func() {
		cse.gir.emitToFileBuffer("\n", EmptyVisitMethod)
		str := cse.emitAsString("break;\n", indent+4)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	cse.executeIfNotForwardDecls(func() {
		if len(node) == 0 {
			str := cse.emitAsString("default:\n", indent+2)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		}
	})
}

func (cse *CSharpEmitter) PreVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString("case ", indent+2)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
		tv := cse.pkg.TypesInfo.Types[node]
		if typeVal, ok := csTypesMap[tv.Type.String()]; ok {
			cse.emitToken("(", LeftParen, 0)
			str = cse.emitAsString(typeVal, 0)
			cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
			cse.emitToken(")", RightParen, 0)
		}
	})
}

func (cse *CSharpEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString(":\n", 0)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

func (cse *CSharpEmitter) PreVisitTypeAssertExprType(node ast.Expr, indent int) {
	if cse.isTypeAssertCommaOk {
		return
	}
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken("(", LeftParen, indent)
	})
}

func (cse *CSharpEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	if cse.isTypeAssertCommaOk {
		return
	}
	cse.executeIfNotForwardDecls(func() {
		cse.emitToken(")", RightParen, indent)
	})
}

func (cse *CSharpEmitter) PreVisitTypeAssertExprX(node ast.Expr, indent int) {
	if cse.isTypeAssertCommaOk {
		cse.suppressMapEmit = false
		cse.captureMapKey = true
		cse.capturedMapKey = ""
		return
	}
}

func (cse *CSharpEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	if cse.isTypeAssertCommaOk {
		cse.captureMapKey = false
		cse.suppressMapEmit = true
		return
	}
}

func (cse *CSharpEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	cse.executeIfNotForwardDecls(func() {
		str := cse.emitAsString(node.Tok.String()+";", indent)
		cse.gir.emitToFileBuffer(str, EmptyVisitMethod)
	})
}

// GenerateCsproj creates a .csproj file for building the C# project
func (cse *CSharpEmitter) GenerateCsproj() error {
	if cse.LinkRuntime == "" {
		return nil
	}

	csprojPath := filepath.Join(cse.OutputDir, cse.OutputName+".csproj")
	file, err := os.Create(csprojPath)
	if err != nil {
		return fmt.Errorf("failed to create .csproj: %w", err)
	}
	defer file.Close()

	// Determine graphics backend
	graphicsBackend := cse.GraphicsRuntime
	if graphicsBackend == "" {
		graphicsBackend = "tigr" // Default to tigr for C#
	}

	var csproj string
	switch graphicsBackend {
	case "none":
		// No graphics dependencies
		csproj = `<Project Sdk="Microsoft.NET.Sdk">

  <PropertyGroup>
    <OutputType>Exe</OutputType>
    <TargetFramework>net9.0</TargetFramework>
    <ImplicitUsings>enable</ImplicitUsings>
    <Nullable>enable</Nullable>
    <AllowUnsafeBlocks>true</AllowUnsafeBlocks>
  </PropertyGroup>

</Project>
`
	case "tigr":
		// tigr graphics - compile tigr.c to native library at build time
		csproj = `<Project Sdk="Microsoft.NET.Sdk">

  <PropertyGroup>
    <OutputType>Exe</OutputType>
    <TargetFramework>net9.0</TargetFramework>
    <ImplicitUsings>enable</ImplicitUsings>
    <Nullable>enable</Nullable>
    <AllowUnsafeBlocks>true</AllowUnsafeBlocks>
  </PropertyGroup>

  <!-- Compile tigr.c to native library before build -->
  <Target Name="CompileTigr" BeforeTargets="Build">
    <!-- macOS -->
    <Exec Command="cc -shared -o $(OutputPath)libtigr.dylib tigr.c screen_helper.c -framework OpenGL -framework Cocoa -framework CoreGraphics"
          Condition="$([MSBuild]::IsOSPlatform('OSX'))"
          WorkingDirectory="$(ProjectDir)" />
    <!-- Linux -->
    <Exec Command="gcc -shared -fPIC -o $(OutputPath)libtigr.so tigr.c screen_helper.c -lGL -lX11 -lm"
          Condition="$([MSBuild]::IsOSPlatform('Linux'))"
          WorkingDirectory="$(ProjectDir)" />
    <!-- Windows (MinGW) -->
    <Exec Command="gcc -shared -o $(OutputPath)tigr.dll tigr.c screen_helper.c -lopengl32 -lgdi32 -luser32 -lshell32 -ladvapi32"
          Condition="$([MSBuild]::IsOSPlatform('Windows'))"
          WorkingDirectory="$(ProjectDir)" />
  </Target>

  <!-- Ensure output directory exists before compiling tigr -->
  <Target Name="EnsureOutputDir" BeforeTargets="CompileTigr">
    <MakeDir Directories="$(OutputPath)" />
  </Target>

</Project>
`
	default:
		// SDL2 graphics
		csproj = `<Project Sdk="Microsoft.NET.Sdk">

  <PropertyGroup>
    <OutputType>Exe</OutputType>
    <TargetFramework>net9.0</TargetFramework>
    <ImplicitUsings>enable</ImplicitUsings>
    <Nullable>enable</Nullable>
    <AllowUnsafeBlocks>true</AllowUnsafeBlocks>
  </PropertyGroup>

  <ItemGroup>
    <PackageReference Include="Sayers.SDL2.Core" Version="1.0.11" />
  </ItemGroup>

  <!-- Copy native SDL2 library on macOS (Homebrew installation) -->
  <Target Name="CopyNativeSDL2" AfterTargets="Build" Condition="$([MSBuild]::IsOSPlatform('OSX'))">
    <PropertyGroup>
      <SDL2HomebrewPath Condition="Exists('/opt/homebrew/lib/libSDL2.dylib')">/opt/homebrew/lib/libSDL2.dylib</SDL2HomebrewPath>
      <SDL2HomebrewPath Condition="$(SDL2HomebrewPath) == '' And Exists('/usr/local/lib/libSDL2.dylib')">/usr/local/lib/libSDL2.dylib</SDL2HomebrewPath>
    </PropertyGroup>
    <Copy SourceFiles="$(SDL2HomebrewPath)" DestinationFolder="$(OutputPath)" Condition="$(SDL2HomebrewPath) != ''" />
    <Warning Text="SDL2 native library not found. Install with: brew install sdl2" Condition="$(SDL2HomebrewPath) == ''" />
  </Target>

</Project>
`
	}

	_, err = file.WriteString(csproj)
	if err != nil {
		return fmt.Errorf("failed to write .csproj: %w", err)
	}

	DebugLogPrintf("Generated .csproj at %s (graphics: %s)", csprojPath, graphicsBackend)
	return nil
}

// CopyGraphicsRuntime copies the graphics runtime file from the runtime directory
func (cse *CSharpEmitter) CopyGraphicsRuntime() error {
	if cse.LinkRuntime == "" {
		return nil
	}

	// Determine graphics backend
	graphicsBackend := cse.GraphicsRuntime
	if graphicsBackend == "" {
		graphicsBackend = "tigr"
	}

	// Select the appropriate runtime file based on backend
	var runtimeFileName string
	switch graphicsBackend {
	case "tigr":
		runtimeFileName = "GraphicsRuntimeTigr.cs"
	case "sdl2":
		runtimeFileName = "GraphicsRuntimeSDL2.cs"
	default:
		// For "none" or unknown, use SDL2 runtime as fallback
		runtimeFileName = "GraphicsRuntimeSDL2.cs"
	}

	// Source path: LinkRuntime points to runtime directory, graphics runtime is in graphics/csharp/
	runtimeSrcPath := filepath.Join(cse.LinkRuntime, "graphics", "csharp", runtimeFileName)
	graphicsCs, err := os.ReadFile(runtimeSrcPath)
	if err != nil {
		return fmt.Errorf("failed to read graphics runtime from %s: %w", runtimeSrcPath, err)
	}

	// Destination path
	graphicsPath := filepath.Join(cse.OutputDir, "GraphicsRuntime.cs")
	if err := os.WriteFile(graphicsPath, graphicsCs, 0644); err != nil {
		return fmt.Errorf("failed to write GraphicsRuntime.cs: %w", err)
	}

	DebugLogPrintf("Copied GraphicsRuntime.cs from %s to %s", runtimeSrcPath, graphicsPath)

	// For tigr backend, also copy tigr.c and tigr.h
	if graphicsBackend == "tigr" {
		// Copy tigr.c
		tigrCSrc := filepath.Join(cse.LinkRuntime, "graphics", "cpp", "tigr.c")
		tigrCDst := filepath.Join(cse.OutputDir, "tigr.c")
		tigrCContent, err := os.ReadFile(tigrCSrc)
		if err != nil {
			return fmt.Errorf("failed to read tigr.c from %s: %w", tigrCSrc, err)
		}
		if err := os.WriteFile(tigrCDst, tigrCContent, 0644); err != nil {
			return fmt.Errorf("failed to write tigr.c: %w", err)
		}
		DebugLogPrintf("Copied tigr.c to %s", tigrCDst)

		// Copy tigr.h
		tigrHSrc := filepath.Join(cse.LinkRuntime, "graphics", "cpp", "tigr.h")
		tigrHDst := filepath.Join(cse.OutputDir, "tigr.h")
		tigrHContent, err := os.ReadFile(tigrHSrc)
		if err != nil {
			return fmt.Errorf("failed to read tigr.h from %s: %w", tigrHSrc, err)
		}
		if err := os.WriteFile(tigrHDst, tigrHContent, 0644); err != nil {
			return fmt.Errorf("failed to write tigr.h: %w", err)
		}
		DebugLogPrintf("Copied tigr.h to %s", tigrHDst)

		// Copy screen_helper.c (platform-specific screen size detection)
		shSrc := filepath.Join(cse.LinkRuntime, "graphics", "cpp", "screen_helper.c")
		shDst := filepath.Join(cse.OutputDir, "screen_helper.c")
		shContent, err := os.ReadFile(shSrc)
		if err != nil {
			return fmt.Errorf("failed to read screen_helper.c from %s: %w", shSrc, err)
		}
		if err := os.WriteFile(shDst, shContent, 0644); err != nil {
			return fmt.Errorf("failed to write screen_helper.c: %w", err)
		}
		DebugLogPrintf("Copied screen_helper.c to %s", shDst)
	}

	return nil
}
