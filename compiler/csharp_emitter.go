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

type csMixedIndexOp struct {
	accessType   string // "map" or "slice"
	keyExpr      string // Key/index expression
	keyCastPfx   string // Cast prefix for map key
	keyCastSfx   string // Cast suffix for map key
	valueCsType  string // C# type of the value at this level
	tempVarName  string // Temp variable name (only for map access)
	mapVarExpr   string // The expression to call hashMapGet on
}

// getCsTypeName converts a Go type to its C# type name
func getCsTypeName(t types.Type) string {
	// Handle slice types - recursively get element type
	if slice, ok := t.(*types.Slice); ok {
		elemType := getCsTypeName(slice.Elem())
		return "List<" + elemType + ">"
	}
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
	// Handle map types
	if _, ok := t.Underlying().(*types.Map); ok {
		return "hmap.HashMap"
	}
	// Handle pointer types (after pointer lowering, stored as int pool index)
	if _, ok := t.(*types.Pointer); ok {
		return "int"
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

// exprToCsString converts a simple expression (BasicLit, Ident, IndexExpr, SelectorExpr) to its C# string representation
func exprToCsString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Value // Keep quotes for strings
	case *ast.Ident:
		return e.Name
	case *ast.IndexExpr:
		xStr := exprToCsString(e.X)
		indexStr := exprToCsString(e.Index)
		if xStr != "" && indexStr != "" {
			return xStr + "[" + indexStr + "]"
		}
		return ""
	case *ast.SelectorExpr:
		xStr := exprToCsString(e.X)
		if xStr != "" {
			return xStr + "." + e.Sel.Name
		}
		return ""
	default:
		return ""
	}
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

func trimBeforeChar(s string, ch byte) string {
	pos := strings.IndexByte(s, ch)
	if pos == -1 {
		return s // character not found
	}
	return s[pos+1:]
}

// CSharpEmitter implements the Emitter interface using a shift/reduce architecture
// for C# code generation.
type CSharpEmitter struct {
	fs              *FragmentStack
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string
	RuntimePackages map[string]string
	OptimizeRefs    bool
	RefOptCount     int
	file            *os.File
	Emitter
	pkg            *packages.Package
	currentPackage string
	indent         int
	numFuncResults int
	// Map assignment detection (same as JS/C++ pattern)
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
	// Reference optimization
	refOptReadOnly            *ReadOnlyAnalysis
	refOptCurrentFunc         string
	refOptCurrentPkg          string
	refOptCurrentRefParams    map[string]bool
	refOptCurrentMutRefParams map[string]bool
	refOptCalleeReadOnly      [][]bool
	refOptCalleeMutRef        [][]bool
	currentParamIndex         int
	// C#-specific
	forwardDecl      bool
	nestedMapCounter int
	typeAliasMap     map[string]string
	aliases          map[string]Alias
	currentAliasName string
	rangeVarCounter int
	funcReturnType  types.Type // Current function's return type (for narrowing casts)
}

func (e *CSharpEmitter) SetFile(file *os.File) { e.file = file }
func (e *CSharpEmitter) GetFile() *os.File     { return e.file }

// csRefOptFuncKey converts a C# function name to the analysis key format (pkg.Func).
func (e *CSharpEmitter) csRefOptFuncKey(csFuncName string) string {
	key := strings.ReplaceAll(csFuncName, ".", ".")
	if !strings.Contains(key, ".") {
		key = e.refOptCurrentPkg + "." + key
	}
	return key
}

// csIsRefOptArg checks if the current call's argument at the given index is read-only.
func (e *CSharpEmitter) csIsRefOptArg(index int) bool {
	if !e.OptimizeRefs || len(e.refOptCalleeReadOnly) == 0 {
		return false
	}
	flags := e.refOptCalleeReadOnly[len(e.refOptCalleeReadOnly)-1]
	if flags == nil || index >= len(flags) {
		return false
	}
	return flags[index]
}

// csIsMutRefOptArg checks if the current call's argument at the given index is mut-ref.
func (e *CSharpEmitter) csIsMutRefOptArg(index int) bool {
	if !e.OptimizeRefs || len(e.refOptCalleeMutRef) == 0 {
		return false
	}
	flags := e.refOptCalleeMutRef[len(e.refOptCalleeMutRef)-1]
	if flags == nil || index >= len(flags) {
		return false
	}
	return flags[index]
}

// csIndent returns indentation string for the given level.
func csIndent(indent int) string {
	return strings.Repeat("  ", indent/2)
}

// csDefaultForGoType returns C# default value for a Go type.
func csDefaultForGoType(t types.Type) string {
	if t == nil {
		return "default"
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
		elemType := getCsTypeName(u.Elem())
		return fmt.Sprintf("new List<%s>()", elemType)
	case *types.Map:
		return "default"
	case *types.Struct:
		if named, ok := t.(*types.Named); ok {
			return fmt.Sprintf("new %s()", named.Obj().Name())
		}
		return "default"
	}
	return "default"
}

// csGetTypeName extends getCsTypeName with function/signature type support.
func csGetTypeName(t types.Type) string {
	if t == nil {
		return "object"
	}
	if sig, ok := t.Underlying().(*types.Signature); ok {
		params := sig.Params()
		results := sig.Results()
		if results.Len() == 0 {
			// Action<P1, P2, ...>
			if params.Len() == 0 {
				return "Action"
			}
			var pTypes []string
			for i := 0; i < params.Len(); i++ {
				pTypes = append(pTypes, csGetTypeName(params.At(i).Type()))
			}
			return fmt.Sprintf("Action<%s>", strings.Join(pTypes, ", "))
		}
		// Func<P1, P2, ..., R>
		var pTypes []string
		for i := 0; i < params.Len(); i++ {
			pTypes = append(pTypes, csGetTypeName(params.At(i).Type()))
		}
		for i := 0; i < results.Len(); i++ {
			pTypes = append(pTypes, csGetTypeName(results.At(i).Type()))
		}
		return fmt.Sprintf("Func<%s>", strings.Join(pTypes, ", "))
	}
	return getCsTypeName(t)
}

// qualifiedCsTypeName returns the C# type name with package prefix for cross-package struct types.
func (e *CSharpEmitter) qualifiedCsTypeName(t types.Type) string {
	if t == nil {
		return "object"
	}
	// Handle function types: Func<>/Action<> with qualified param types
	if sig, ok := t.Underlying().(*types.Signature); ok {
		params := sig.Params()
		results := sig.Results()
		if results.Len() == 0 {
			if params.Len() == 0 {
				return "Action"
			}
			var pTypes []string
			for i := 0; i < params.Len(); i++ {
				pTypes = append(pTypes, e.qualifiedCsTypeName(params.At(i).Type()))
			}
			return fmt.Sprintf("Action<%s>", strings.Join(pTypes, ", "))
		}
		var pTypes []string
		for i := 0; i < params.Len(); i++ {
			pTypes = append(pTypes, e.qualifiedCsTypeName(params.At(i).Type()))
		}
		for i := 0; i < results.Len(); i++ {
			pTypes = append(pTypes, e.qualifiedCsTypeName(results.At(i).Type()))
		}
		return fmt.Sprintf("Func<%s>", strings.Join(pTypes, ", "))
	}
	// For named struct types from other packages, add the package prefix
	if named, ok := t.(*types.Named); ok {
		if _, isStruct := named.Underlying().(*types.Struct); isStruct {
			name := named.Obj().Name()
			if named.Obj().Pkg() != nil {
				pkgName := named.Obj().Pkg().Name()
				if pkgName != e.currentPackage && pkgName != "main" {
					return pkgName + "." + name
				}
			}
			return name
		}
	}
	// Handle slice of cross-package structs: List<From> → List<ast.From>
	if slice, ok := t.(*types.Slice); ok {
		elemType := e.qualifiedCsTypeName(slice.Elem())
		return "List<" + elemType + ">"
	}
	// Handle map type
	if _, ok := t.Underlying().(*types.Map); ok {
		return "hmap.HashMap"
	}
	return getCsTypeName(t)
}

// csLowerBuiltin maps Go stdlib selectors to C# equivalents.
func csLowerBuiltin(selector string) string {
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
	case "panic":
		return "GoanyRuntime.goany_panic"
	}
	return selector
}

// convertGoTypeToCSharp converts a Go type string to C# syntax.
func convertGoTypeToCSharp(goType string) string {
	result := goType

	if strings.HasPrefix(result, "[]") {
		elementType := result[2:]
		elementType = convertGoTypeToCSharp(elementType)
		return "List<" + elementType + ">"
	}

	if strings.HasPrefix(result, "map[") {
		bracketEnd := strings.Index(result, "]")
		if bracketEnd > 4 {
			keyType := result[4:bracketEnd]
			valueType := result[bracketEnd+1:]
			keyType = convertGoTypeToCSharp(keyType)
			valueType = convertGoTypeToCSharp(valueType)
			return "Dictionary<" + keyType + ", " + valueType + ">"
		}
	}

	if strings.Contains(result, "/") {
		lastSlash := strings.LastIndex(result, "/")
		result = result[lastSlash+1:]
	}

	if csType, exists := csTypesMap[result]; exists {
		return csType
	}

	return result
}

// csMixedOp represents a single map or slice access in a chained index expression.
type csMixedOp struct {
	accessType   string // "map" or "slice"
	keyExpr      string
	valueCsType  string
	keyCastPfx   string
	keyCastSfx   string
	mapVarExpr   string
	tempVarName  string
}

// analyzeLhsIndexChainCs walks a chain of IndexExpr, returning operations and
// whether there's an intermediate map access that needs read-modify-write.
func (e *CSharpEmitter) analyzeLhsIndexChainCs(expr ast.Expr) (ops []csMixedOp, hasIntermediateMap bool) {
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
		var exprType types.Type
		tv := e.pkg.TypesInfo.Types[ie.X]
		if tv.Type != nil {
			exprType = tv.Type
		} else if ident, ok2 := ie.X.(*ast.Ident); ok2 {
			if obj := e.pkg.TypesInfo.ObjectOf(ident); obj != nil {
				exprType = obj.Type()
			}
		}
		if exprType == nil {
			continue
		}
		isLast := (i == len(chain)-1)
		if mapType, ok := exprType.Underlying().(*types.Map); ok {
			op := csMixedOp{
				accessType:  "map",
				keyExpr:     exprToCsString(ie.Index),
				valueCsType: e.qualifiedCsTypeName(mapType.Elem()),
			}
			op.keyCastPfx, op.keyCastSfx = getCsKeyCast(mapType.Key())
			if !isLast {
				hasIntermediateMap = true
			}
			ops = append(ops, op)
		} else {
			op := csMixedOp{
				accessType: "slice",
				keyExpr:    exprToCsString(ie.Index),
			}
			ops = append(ops, op)
		}
	}
	return ops, hasIntermediateMap
}

// isMapTypeExpr checks if an expression has map type via TypesInfo.
func (e *CSharpEmitter) isMapTypeExpr(expr ast.Expr) bool {
	if e.pkg == nil || e.pkg.TypesInfo == nil {
		return false
	}
	tv := e.pkg.TypesInfo.Types[expr]
	if tv.Type != nil {
		_, ok := tv.Type.Underlying().(*types.Map)
		if ok {
			return true
		}
	}
	// Fallback: for identifiers, try ObjectOf/Uses
	if ident, ok := expr.(*ast.Ident); ok {
		if obj := e.pkg.TypesInfo.ObjectOf(ident); obj != nil {
			_, isMap := obj.Type().Underlying().(*types.Map)
			return isMap
		}
	}
	return false
}

// getExprGoType returns the Go type for an expression, or nil.
func (e *CSharpEmitter) getExprGoType(expr ast.Expr) types.Type {
	if e.pkg == nil || e.pkg.TypesInfo == nil {
		return nil
	}
	tv := e.pkg.TypesInfo.Types[expr]
	return tv.Type
}

// getMapKeyTypeConst returns the key type constant for a map's key type.
func (e *CSharpEmitter) getMapKeyTypeConst(mapType *ast.MapType) int {
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
					e.structKeyTypes[structName] = structName
				}
				return 100
			}
		}
	}
	return 1
}

// csMapKeyTypeConst returns the key type constant from types.Map.
func csMapKeyTypeConst(t *types.Map) int {
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

// ============================================================
// Program / Package
// ============================================================

func (e *CSharpEmitter) PreVisitProgram(indent int) {
	var err error
	e.file, err = os.Create(e.Output)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}

	e.fs = NewFragmentStack(e.GetGoFIR())
	e.typeAliasMap = make(map[string]string)
	e.aliases = make(map[string]Alias)

	// Write C# header
	e.file.WriteString("using System;\nusing System.Collections;\nusing System.Collections.Generic;\n\n")

	// Include panic runtime
	e.file.WriteString("// GoAny panic runtime\n")
	e.file.WriteString(goanyrt.PanicCsSource)
	e.file.WriteString("\n")

	// Write SliceBuiltins and Formatter classes
	e.file.WriteString(`public static class SliceBuiltins
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
                        i++;
                        continue;
                    case 'v':
                        converted += "{" + argIndex + "}";
                        formattedArgs.Add(args[argIndex]);
                        argIndex++;
                        i++;
                        continue;
                    case 'x':
                        converted += "{" + argIndex + ":x}";
                        formattedArgs.Add(args[argIndex]);
                        argIndex++;
                        i++;
                        continue;
                    case 'c':
                        converted += "{" + argIndex + "}";
                        object arg = args[argIndex];
                        if (arg is sbyte sb)
                            formattedArgs.Add((char)sb);
                        else if (arg is int iVal)
                            formattedArgs.Add((char)iVal);
                        else if (arg is char cVal)
                            formattedArgs.Add(cVal);
                        else
                            throw new ArgumentException($"Argument {argIndex} for %c must be a char, int, or sbyte");
                        argIndex++;
                        i++;
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
                        i++;
                        continue;
                    case 'v':
                        converted += "{" + argIndex + "}";
                        formattedArgs.Add(args[argIndex]);
                        argIndex++;
                        i++;
                        continue;
                    case 'x':
                        converted += "{" + argIndex + ":x}";
                        formattedArgs.Add(args[argIndex]);
                        argIndex++;
                        i++;
                        continue;
                    case 'c':
                        converted += "{" + argIndex + "}";
                        object arg = args[argIndex];
                        if (arg is sbyte sb)
                            formattedArgs.Add((char)sb);
                        else if (arg is int iVal)
                            formattedArgs.Add((char)iVal);
                        else if (arg is char cVal)
                            formattedArgs.Add(cVal);
                        else
                            throw new ArgumentException($"Argument {argIndex} for %c must be a char, int, or sbyte");
                        argIndex++;
                        i++;
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

`)

	// Runtime packages are copied to separate .cs files by CopyRuntimePackages() in PostVisitProgram
}


func (e *CSharpEmitter) PostVisitProgram(indent int) {
	// Reduce everything from program marker
	tokens := e.fs.Reduce(string(PreVisitProgram))
	// Write all accumulated code
	for _, t := range tokens {
		e.file.WriteString(t.Serialize())
	}
	e.file.Close()

	if e.OptimizeRefs && e.RefOptCount > 0 {
		fmt.Printf("  C#: %d copy(ies) removed by reference optimization\n", e.RefOptCount)
	}

	// Replace placeholder struct key functions with working implementations
	if len(e.structKeyTypes) > 0 {
		e.replaceStructKeyFunctions()
	}

	// Generate .NET project files if link-runtime is enabled
	if e.LinkRuntime != "" {
		if err := e.GenerateCsproj(); err != nil {
			log.Printf("Warning: %v", err)
		}
		if err := e.CopyRuntimePackages(); err != nil {
			log.Printf("Warning: %v", err)
		}
	}
}

// replaceStructKeyFunctions replaces placeholder hash/equality functions for struct keys.
func (e *CSharpEmitter) replaceStructKeyFunctions() {
	content, err := os.ReadFile(e.Output)
	if err != nil {
		log.Printf("Warning: could not read file for struct key replacement: %v", err)
		return
	}

	newContent := string(content)

	hashPattern := regexp.MustCompile(`(?s)public static int hashStructKey\(object key\)\s*\{\s*return 0;\s*\}`)
	newHashBody := `public static int hashStructKey(object key)
    {
        var h = key.GetHashCode();
        if (h < 0) h = -h;
        return h;
    }`
	newContent = hashPattern.ReplaceAllString(newContent, newHashBody)

	equalPattern := regexp.MustCompile(`(?s)public static bool structKeysEqual\(object a, object b\)\s*\{\s*return false;\s*\}`)
	newEqualBody := `public static bool structKeysEqual(object a, object b)
    {
        return a.Equals(b);
    }`
	newContent = equalPattern.ReplaceAllString(newContent, newEqualBody)

	if err := os.WriteFile(e.Output, []byte(newContent), 0644); err != nil {
		log.Printf("Warning: could not write struct key replacements: %v", err)
	}
}

func (e *CSharpEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	e.pkg = pkg
	name := pkg.Name
	e.refOptCurrentPkg = pkg.Name
	if name == "main" {
		e.currentPackage = "MainClass"
	} else {
		e.currentPackage = name
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
	}
	e.fs.PushTree(TokenTree(Keyword, TagStmt,
		LeafTag(Keyword, "public static class ", TagCSharp),
		Leaf(Identifier, e.currentPackage),
		Leaf(WhiteSpace, " {\n\n"),
	))
}

func (e *CSharpEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	e.fs.PushCode("}\n")
}

// ============================================================
// Literals and Identifiers
// ============================================================

func (e *CSharpEmitter) PreVisitBasicLit(node *ast.BasicLit, indent int) {
	val := node.Value
	if node.Kind == token.STRING && len(val) > 1 && val[0] == '`' {
		// Raw string literal -> C# verbatim string
		content := val[1 : len(val)-1]
		val = "@\"" + content + "\""
	}
	// Convert Go character literals to numeric values
	if node.Kind == token.CHAR && len(val) >= 3 && val[0] == '\'' {
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
			val = fmt.Sprintf("(int)'%s'", inner)
		}
	}
	e.fs.Push(val, TagLiteral, nil)
}

func (e *CSharpEmitter) PreVisitIdent(node *ast.Ident, indent int) {
	name := node.Name
	// Map Go builtins
	switch name {
	case "true", "false":
		e.fs.Push(name, TagLiteral, nil)
		return
	case "nil":
		e.fs.Push("default", TagLiteral, nil)
		return
	case "string":
		e.fs.Push("string", TagType, nil)
		return
	}
	// Check csTypesMap for type mappings
	if csType, ok := csTypesMap[name]; ok {
		e.fs.Push(csType, TagType, nil)
		return
	}
	// Check typeAliasMap
	if underlyingType, ok := e.typeAliasMap[name]; ok {
		e.fs.Push(underlyingType, TagType, nil)
		return
	}
	// Check if this is a reference to another package
	if e.pkg != nil && e.pkg.TypesInfo != nil {
		if obj := e.pkg.TypesInfo.Uses[node]; obj != nil {
			if obj.Pkg() != nil && obj.Pkg().Name() != e.currentPackage && obj.Pkg().Name() != "main" {
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

func (e *CSharpEmitter) PostVisitBinaryExprLeft(node ast.Expr, indent int) {
	left := e.fs.ReduceToCode(string(PreVisitBinaryExprLeft))
	e.fs.PushCode(left)
}

func (e *CSharpEmitter) PostVisitBinaryExprRight(node ast.Expr, indent int) {
	right := e.fs.ReduceToCode(string(PreVisitBinaryExprRight))
	e.fs.PushCode(right)
}

func (e *CSharpEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitBinaryExpr))
	left := ""
	right := ""
	if len(tokens) >= 1 {
		left = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		right = tokens[1].Serialize()
	}
	op := node.Op.String()
	expr := fmt.Sprintf("%s %s %s", left, op, right)
	// For arithmetic ops on narrow types, C# promotes to int — add narrowing cast
	goType := e.getExprGoType(node)
	if goType != nil {
		if basic, ok := goType.Underlying().(*types.Basic); ok {
			switch basic.Kind() {
			case types.Int8:
				expr = fmt.Sprintf("(sbyte)(%s)", expr)
			case types.Uint8:
				expr = fmt.Sprintf("(byte)(%s)", expr)
			case types.Int16:
				expr = fmt.Sprintf("(short)(%s)", expr)
			case types.Uint16:
				expr = fmt.Sprintf("(ushort)(%s)", expr)
			}
		}
	}
	e.fs.PushCode(expr)
}

// ============================================================
// Call Expressions
// ============================================================

func (e *CSharpEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	funCode := e.fs.ReduceToCode(string(PreVisitCallExprFun))

	// Push callee's read-only and mut-ref parameter flags for ref optimization
	if e.OptimizeRefs && e.refOptReadOnly != nil {
		funcKey := e.csRefOptFuncKey(funCode)
		if flags, ok := e.refOptReadOnly.ReadOnly[funcKey]; ok {
			e.refOptCalleeReadOnly = append(e.refOptCalleeReadOnly, flags)
		} else {
			e.refOptCalleeReadOnly = append(e.refOptCalleeReadOnly, nil)
		}
		if flags, ok := e.refOptReadOnly.MutRef[funcKey]; ok {
			e.refOptCalleeMutRef = append(e.refOptCalleeMutRef, flags)
		} else {
			e.refOptCalleeMutRef = append(e.refOptCalleeMutRef, nil)
		}
	}

	e.fs.PushCode(funCode)
}

func (e *CSharpEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {
	argCode := e.fs.ReduceToCode(string(PreVisitCallExprArg))

	// Reference optimization: add in/ref at call site.
	// Only for simple identifiers — C# requires lvalues for in/ref.
	// Index expressions (list[i]), field accesses, and calls return rvalues.
	if _, isIdent := node.(*ast.Ident); isIdent {
		if e.csIsRefOptArg(index) {
			argCode = "in " + argCode
		} else if e.csIsMutRefOptArg(index) {
			argCode = "ref " + argCode
		}
	}

	e.fs.PushCode(argCode)
}

func (e *CSharpEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	argTokens := e.fs.Reduce(string(PreVisitCallExprArgs))
	var args []string
	for _, t := range argTokens {
		if t.Serialize() != "" {
			args = append(args, t.Serialize())
		}
	}
	e.fs.PushCode(strings.Join(args, ", "))
}

func (e *CSharpEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {
	// Pop callee ref-opt flags (pushed in PostVisitCallExprFun)
	if e.OptimizeRefs {
		if len(e.refOptCalleeReadOnly) > 0 {
			e.refOptCalleeReadOnly = e.refOptCalleeReadOnly[:len(e.refOptCalleeReadOnly)-1]
		}
		if len(e.refOptCalleeMutRef) > 0 {
			e.refOptCalleeMutRef = e.refOptCalleeMutRef[:len(e.refOptCalleeMutRef)-1]
		}
	}

	tokens := e.fs.Reduce(string(PreVisitCallExpr))
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
	case "len", "SliceBuiltins.Length":
		// len(x) — for maps use hmap.hashMapLen(x), otherwise SliceBuiltins.Length(x)
		if len(node.Args) > 0 && e.isMapTypeExpr(node.Args[0]) {
			e.fs.PushTree(TokenTree(Identifier, TagExpr,
				Leaf(Identifier, "hmap.hashMapLen"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.PushTree(TokenTree(Identifier, TagExpr,
				Leaf(Identifier, "SliceBuiltins.Length"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(RightParen, ")"),
			))
		}
		return
	case "append", "SliceBuiltins.Append":
		e.fs.PushTree(TokenTree(Identifier, TagExpr,
			Leaf(Identifier, "SliceBuiltins.Append"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
		return
	case "delete":
		// delete(m, k) -> m = hmap.hashMapDelete(m, k)
		if len(node.Args) >= 2 {
			mapName := exprToCsString(node.Args[0])
			e.fs.PushTree(TokenTree(Identifier, TagExpr,
				Leaf(Identifier, mapName),
				Leaf(Assignment, " = "),
				Leaf(Identifier, "hmap.hashMapDelete"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.PushTree(TokenTree(Identifier, TagExpr,
				Leaf(Identifier, "hmap.hashMapDelete"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, argsStr),
				Leaf(RightParen, ")"),
			))
		}
		return
	case "make":
		if len(node.Args) >= 1 {
			if mapType, ok := node.Args[0].(*ast.MapType); ok {
				keyTypeConst := e.getMapKeyTypeConst(mapType)
				e.fs.PushTree(TokenTree(Identifier, TagExpr,
					Leaf(Identifier, "hmap.newHashMap"),
					Leaf(LeftParen, "("),
					Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)),
					Leaf(RightParen, ")"),
				))
				return
			}
			if _, ok := node.Args[0].(*ast.ArrayType); ok {
				// make([]T, n) -> new List<T>(new T[n])
				// Get element type
				elemType := "object"
				if e.pkg != nil && e.pkg.TypesInfo != nil {
					if tv, ok := e.pkg.TypesInfo.Types[node.Args[0]]; ok && tv.Type != nil {
						if slice, ok := tv.Type.(*types.Slice); ok {
							elemType = e.qualifiedCsTypeName(slice.Elem())
						}
					}
				}
				parts := strings.SplitN(argsStr, ", ", 2)
				if len(parts) >= 2 {
					e.fs.PushTree(TokenTree(Identifier, TagExpr,
						LeafTag(Keyword, "new ", TagCSharp),
						Leaf(Identifier, "List"),
						Leaf(LeftAngle, "<"),
						Leaf(Identifier, elemType),
						Leaf(RightAngle, ">"),
						Leaf(LeftParen, "("),
						LeafTag(Keyword, "new ", TagCSharp),
						Leaf(Identifier, elemType),
						Leaf(LeftBracket, "["),
						Leaf(Identifier, parts[1]),
						Leaf(RightBracket, "]"),
						Leaf(RightParen, ")"),
					))
				} else {
					e.fs.PushTree(TokenTree(Identifier, TagExpr,
						LeafTag(Keyword, "new ", TagCSharp),
						Leaf(Identifier, "List"),
						Leaf(LeftAngle, "<"),
						Leaf(Identifier, elemType),
						Leaf(RightAngle, ">"),
						Leaf(LeftParen, "("),
						Leaf(RightParen, ")"),
					))
				}
				return
			}
		}
		e.fs.PushTree(TokenTree(Identifier, TagExpr,
			Leaf(Identifier, "make"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, argsStr),
			Leaf(RightParen, ")"),
		))
		return
	}

	// Check if this is a type conversion (e.g., int(x), string(x), int8(x))
	if ident, ok := node.Fun.(*ast.Ident); ok {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if obj := e.pkg.TypesInfo.ObjectOf(ident); obj != nil {
				if _, isTypeName := obj.(*types.TypeName); isTypeName {
					csType := e.qualifiedCsTypeName(obj.Type())
					if csType == "string" {
						e.fs.PushTree(TokenTree(Identifier, TagExpr,
							Leaf(Identifier, "Convert.ToString"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, argsStr),
							Leaf(RightParen, ")"),
						))
					} else {
						e.fs.PushTree(TokenTree(Identifier, TagExpr,
							Leaf(LeftParen, "("),
							Leaf(Identifier, csType),
							Leaf(RightParen, ")"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, argsStr),
							Leaf(RightParen, ")"),
						))
					}
					return
				}
			}
		}
	}

	// Lower builtins (fmt.Println -> Console.WriteLine, etc.)
	lowered := csLowerBuiltin(funName)
	if lowered != funName {
		funName = lowered
	}

	e.fs.PushTree(TokenTree(Identifier, TagExpr,
		Leaf(Identifier, funName),
		Leaf(LeftParen, "("),
		Leaf(Identifier, argsStr),
		Leaf(RightParen, ")"),
	))
}

// ============================================================
// Selector Expressions (a.b)
// ============================================================

func (e *CSharpEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitSelectorExprX))
	e.fs.PushCode(xCode)
}

func (e *CSharpEmitter) PostVisitSelectorExprSel(node *ast.Ident, indent int) {
	e.fs.Reduce(string(PreVisitSelectorExprSel))
	e.fs.PushCode(node.Name)
}

func (e *CSharpEmitter) PostVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSelectorExpr))
	xCode := ""
	selCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		selCode = tokens[1].Serialize()
	}

	if xCode == "os" && selCode == "Args" {
		e.fs.PushCode("goany_os_args")
		return
	}

	// Check if selector is a type alias
	if _, isAlias := e.typeAliasMap[selCode]; isAlias {
		e.fs.PushCode(e.typeAliasMap[selCode])
		return
	}

	// Lower builtins: fmt.Println -> Console.WriteLine
	loweredX := csLowerBuiltin(xCode)
	loweredSel := csLowerBuiltin(selCode)

	if loweredX == "" {
		e.fs.PushCode(loweredSel)
	} else {
		e.fs.PushCode(loweredX + "." + loweredSel)
	}
}

// ============================================================
// Index Expressions (a[i])
// ============================================================

func (e *CSharpEmitter) PostVisitIndexExprX(node *ast.IndexExpr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitIndexExprX))
	e.fs.PushCode(xCode)
	e.lastIndexXCode = xCode
}

func (e *CSharpEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	idxCode := e.fs.ReduceToCode(string(PreVisitIndexExprIndex))
	e.fs.PushCode(idxCode)
	e.lastIndexKeyCode = idxCode
}

func (e *CSharpEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitIndexExpr))
	xCode := ""
	idxCode := ""
	if len(tokens) >= 1 {
		xCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		idxCode = tokens[1].Serialize()
	}

	if e.isMapTypeExpr(node.X) {
		mapGoType := e.getExprGoType(node.X)
		valType := "object"
		pfx := ""
		sfx := ""
		if mapGoType != nil {
			if mapUnderlying, ok := mapGoType.Underlying().(*types.Map); ok {
				valType = e.qualifiedCsTypeName(mapUnderlying.Elem())
				pfx, sfx = getCsKeyCast(mapUnderlying.Key())
			}
		}
		tree := TokenTree(Identifier, TagExpr,
			Leaf(LeftParen, "("),
			Leaf(LeftParen, "("),
			Leaf(Identifier, valType),
			Leaf(RightParen, ")"),
			Leaf(Identifier, "hmap.hashMapGet"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, xCode),
			Leaf(Comma, ", "),
			Leaf(Identifier, pfx+idxCode+sfx),
			Leaf(RightParen, ")"),
			Leaf(RightParen, ")"),
		)
		tree.GoType = e.getExprGoType(node)
		e.fs.PushTree(tree)
	} else {
		// Check for string indexing
		xType := e.getExprGoType(node.X)
		if xType != nil {
			if basic, ok := xType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
				e.fs.PushTree(TokenTree(Identifier, TagExpr,
					Leaf(LeftParen, "("),
					Leaf(Identifier, "int"),
					Leaf(RightParen, ")"),
					Leaf(Identifier, xCode),
					Leaf(LeftBracket, "["),
					Leaf(Identifier, idxCode),
					Leaf(RightBracket, "]"),
				))
				return
			}
		}
		e.fs.PushTree(TokenTree(Identifier, TagExpr,
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

func (e *CSharpEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitUnaryExpr))
	op := node.Op.String()
	if op == "^" {
		e.fs.PushCode("~" + xCode)
	} else {
		e.fs.PushCode(op + xCode)
	}
}

// ============================================================
// Paren Expressions
// ============================================================

func (e *CSharpEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {
	inner := e.fs.ReduceToCode(string(PreVisitParenExpr))
	e.fs.PushCode("(" + inner + ")")
}

// ============================================================
// Composite Literals
// ============================================================

func (e *CSharpEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitCompositeLitType))
}

func (e *CSharpEmitter) PostVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	eltCode := e.fs.ReduceToCode(string(PreVisitCompositeLitElt))
	e.fs.PushCode(eltCode)
}

func (e *CSharpEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	eltTokens := e.fs.Reduce(string(PreVisitCompositeLitElts))
	for _, t := range eltTokens {
		if t.Serialize() != "" {
			e.fs.Push(t.Serialize(), TagLiteral, nil)
		}
	}
}

func (e *CSharpEmitter) PostVisitCompositeLit(node *ast.CompositeLit, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCompositeLit))
	var elts []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			elts = append(elts, t.Serialize())
		}
	}
	eltsStr := strings.Join(elts, ", ")

	litType := e.getExprGoType(node)
	if litType == nil {
		e.fs.PushCode("new List<object> {" + eltsStr + "}")
		return
	}

	switch u := litType.Underlying().(type) {
	case *types.Struct:
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
				kvMap := make(map[string]string)
				for _, elt := range elts {
					parts := strings.SplitN(elt, ": ", 2)
					if len(parts) == 2 {
						key := parts[0]
						if dotIdx := strings.LastIndex(key, "."); dotIdx >= 0 {
							key = key[dotIdx+1:]
						}
						kvMap[key] = parts[1]
					}
				}
				var children []Token
				children = append(children, LeafTag(Keyword, "new ", TagCSharp))
				children = append(children, Leaf(Identifier, typeName))
				children = append(children, Leaf(WhiteSpace, " "))
				children = append(children, Leaf(LeftBrace, "{ "))
				first := true
				for i := 0; i < u.NumFields(); i++ {
					fieldName := u.Field(i).Name()
					if val, ok := kvMap[fieldName]; ok {
						if !first {
							children = append(children, Leaf(Comma, ", "))
						}
						children = append(children, Leaf(Identifier, fieldName))
						children = append(children, Leaf(Assignment, " = "))
						children = append(children, Leaf(Identifier, val))
						first = false
					}
				}
				children = append(children, Leaf(WhiteSpace, " "))
				children = append(children, Leaf(RightBrace, "}"))
				e.fs.PushTree(TokenTree(Identifier, TagExpr, children...))
				return
			}
		}
		// Positional struct literal
		var children []Token
		children = append(children, LeafTag(Keyword, "new ", TagCSharp))
		children = append(children, Leaf(Identifier, typeName))
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(LeftBrace, "{ "))
		for i, elt := range elts {
			if i > 0 {
				children = append(children, Leaf(Comma, ", "))
			}
			if i < u.NumFields() {
				children = append(children, Leaf(Identifier, u.Field(i).Name()))
				children = append(children, Leaf(Assignment, " = "))
				children = append(children, Leaf(Identifier, elt))
			}
		}
		children = append(children, Leaf(WhiteSpace, " "))
		children = append(children, Leaf(RightBrace, "}"))
		e.fs.PushTree(TokenTree(Identifier, TagExpr, children...))
	case *types.Slice:
		elemType := e.qualifiedCsTypeName(u.Elem())
		e.fs.PushTree(TokenTree(Identifier, TagExpr,
			LeafTag(Keyword, "new ", TagCSharp),
			Leaf(Identifier, "List"),
			Leaf(LeftAngle, "<"),
			Leaf(Identifier, elemType),
			Leaf(RightAngle, ">"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftBrace, "{"),
			Leaf(Identifier, eltsStr),
			Leaf(RightBrace, "}"),
		))
	case *types.Map:
		keyTypeConst := csMapKeyTypeConst(u)
		if keyTypeConst == 100 {
			if e.structKeyTypes == nil {
				e.structKeyTypes = make(map[string]string)
			}
			if named, ok := u.Key().(*types.Named); ok {
				e.structKeyTypes[named.Obj().Name()] = named.Obj().Name()
			}
		}
		if len(elts) == 0 {
			e.fs.PushTree(TokenTree(Identifier, TagExpr,
				Leaf(Identifier, "hmap.newHashMap"),
				Leaf(LeftParen, "("),
				Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)),
				Leaf(RightParen, ")"),
			))
		} else {
			pfx, sfx := getCsKeyCast(u.Key())
			e.nestedMapCounter++
			tmpVar := fmt.Sprintf("_m%d", e.nestedMapCounter)
			// Wrap in a lambda that creates and initializes
			var children []Token
			children = append(children, Leaf(LeftParen, "("))
			children = append(children, Leaf(LeftParen, "("))
			children = append(children, Leaf(RightParen, ")"))
			children = append(children, Leaf(WhiteSpace, " "))
			children = append(children, Leaf(BinaryOperator, "=> "))
			children = append(children, Leaf(LeftBrace, "{ "))
			children = append(children, LeafTag(Keyword, "var ", TagCSharp))
			children = append(children, Leaf(Identifier, tmpVar))
			children = append(children, Leaf(Assignment, " = "))
			children = append(children, Leaf(Identifier, "hmap.newHashMap"))
			children = append(children, Leaf(LeftParen, "("))
			children = append(children, Leaf(NumberLiteral, fmt.Sprintf("%d", keyTypeConst)))
			children = append(children, Leaf(RightParen, ")"))
			children = append(children, Leaf(Semicolon, "; "))
			for _, elt := range elts {
				parts := strings.SplitN(elt, ": ", 2)
				if len(parts) == 2 {
					children = append(children, Leaf(Identifier, "hmap.hashMapSet"))
					children = append(children, Leaf(LeftParen, "("))
					children = append(children, Leaf(Identifier, tmpVar))
					children = append(children, Leaf(Comma, ", "))
					children = append(children, Leaf(Identifier, pfx+parts[0]+sfx))
					children = append(children, Leaf(Comma, ", "))
					children = append(children, Leaf(Identifier, parts[1]))
					children = append(children, Leaf(RightParen, ")"))
					children = append(children, Leaf(Semicolon, "; "))
				}
			}
			children = append(children, Leaf(ReturnKeyword, "return "))
			children = append(children, Leaf(Identifier, tmpVar))
			children = append(children, Leaf(Semicolon, "; "))
			children = append(children, Leaf(RightBrace, "}"))
			children = append(children, Leaf(RightParen, ")"))
			children = append(children, Leaf(LeftParen, "("))
			children = append(children, Leaf(RightParen, ")"))
			e.fs.PushTree(TokenTree(Identifier, TagExpr, children...))
		}
	default:
		elemType := "object"
		if slice, ok := litType.(*types.Slice); ok {
			elemType = e.qualifiedCsTypeName(slice.Elem())
		}
		e.fs.PushTree(TokenTree(Identifier, TagExpr,
			LeafTag(Keyword, "new ", TagCSharp),
			Leaf(Identifier, "List"),
			Leaf(LeftAngle, "<"),
			Leaf(Identifier, elemType),
			Leaf(RightAngle, ">"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftBrace, "{"),
			Leaf(Identifier, eltsStr),
			Leaf(RightBrace, "}"),
		))
	}
}

// ============================================================
// KeyValue Expressions (for composite literals)
// ============================================================

func (e *CSharpEmitter) PostVisitKeyValueExprKey(node ast.Expr, indent int) {
	keyCode := e.fs.ReduceToCode(string(PreVisitKeyValueExprKey))
	e.fs.PushCode(keyCode)
}

func (e *CSharpEmitter) PostVisitKeyValueExprValue(node ast.Expr, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitKeyValueExprValue))
	e.fs.PushCode(valCode)
}

func (e *CSharpEmitter) PostVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitKeyValueExpr))
	keyCode := ""
	valCode := ""
	if len(tokens) >= 1 {
		keyCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		valCode = tokens[1].Serialize()
	}
	e.fs.PushCode(keyCode + ": " + valCode)
}

// ============================================================
// Slice Expressions (a[lo:hi])
// ============================================================

func (e *CSharpEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitSliceExprX))
	e.fs.PushCode(xCode)
}

func (e *CSharpEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitSliceExprXBegin))
}

func (e *CSharpEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	lowCode := e.fs.ReduceToCode(string(PreVisitSliceExprLow))
	e.fs.PushCode(lowCode)
}

func (e *CSharpEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitSliceExprXEnd))
}

func (e *CSharpEmitter) PostVisitSliceExprHigh(node ast.Expr, indent int) {
	highCode := e.fs.ReduceToCode(string(PreVisitSliceExprHigh))
	e.fs.PushCode(highCode)
}

func (e *CSharpEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSliceExpr))
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

	// Check if slicing a string
	xType := e.getExprGoType(node.X)
	isString := false
	if xType != nil {
		if basic, ok := xType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
			isString = true
		}
	}

	if isString {
		if highCode == "" {
			e.fs.PushTree(TokenTree(Identifier, TagExpr,
				Leaf(Identifier, xCode),
				Leaf(Dot, "."),
				Leaf(Identifier, "Substring"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.PushTree(TokenTree(Identifier, TagExpr,
				Leaf(Identifier, xCode),
				Leaf(Dot, "."),
				Leaf(Identifier, "Substring"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(Comma, ", "),
				Leaf(Identifier, highCode),
				Leaf(BinaryOperator, " - "),
				Leaf(Identifier, lowCode),
				Leaf(RightParen, ")"),
			))
		}
	} else {
		if highCode == "" {
			e.fs.PushTree(TokenTree(Identifier, TagExpr,
				Leaf(Identifier, xCode),
				Leaf(Dot, "."),
				Leaf(Identifier, "GetRange"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(Comma, ", "),
				Leaf(Identifier, xCode),
				Leaf(Dot, "."),
				Leaf(Identifier, "Count"),
				Leaf(BinaryOperator, " - "),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(RightParen, ")"),
				Leaf(RightParen, ")"),
			))
		} else {
			e.fs.PushTree(TokenTree(Identifier, TagExpr,
				Leaf(Identifier, xCode),
				Leaf(Dot, "."),
				Leaf(Identifier, "GetRange"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(Comma, ", "),
				Leaf(LeftParen, "("),
				Leaf(Identifier, highCode),
				Leaf(RightParen, ")"),
				Leaf(BinaryOperator, " - "),
				Leaf(LeftParen, "("),
				Leaf(Identifier, lowCode),
				Leaf(RightParen, ")"),
				Leaf(RightParen, ")"),
			))
		}
	}
}

// ============================================================
// Array Type
// ============================================================

func (e *CSharpEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	typeTokens := e.fs.Reduce(string(PreVisitArrayType))
	if len(typeTokens) == 0 {
		typeTokens = []Token{Leaf(Identifier, "object")}
	}
	children := []Token{Leaf(Identifier, "List"), Leaf(LeftAngle, "<")}
	children = append(children, typeTokens...)
	children = append(children, Leaf(RightAngle, ">"))
	e.fs.PushTree(TokenTree(TypeKeyword, TagType, children...))
}

// ============================================================
// Map Type
// ============================================================

func (e *CSharpEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitMapKeyType))
}

func (e *CSharpEmitter) PostVisitMapValueType(node ast.Expr, indent int) {
	e.fs.Reduce(string(PreVisitMapValueType))
}

func (e *CSharpEmitter) PostVisitMapType(node *ast.MapType, indent int) {
	e.fs.Reduce(string(PreVisitMapType))
	e.fs.Push("hmap.HashMap", TagType, nil)
}

// ============================================================
// Function Type (Func<>/Action<>)
// ============================================================

func (e *CSharpEmitter) PostVisitFuncTypeResult(node *ast.Field, index int, indent int) {
	resultCode := e.fs.ReduceToCode(string(PreVisitFuncTypeResult))
	e.fs.PushCode(resultCode)
}

func (e *CSharpEmitter) PostVisitFuncTypeResults(node *ast.FieldList, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncTypeResults))
	var resultTypes []string
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			resultTypes = append(resultTypes, s)
		}
	}
	e.fs.PushCode(strings.Join(resultTypes, ", "))
}

func (e *CSharpEmitter) PostVisitFuncTypeParam(node *ast.Field, index int, indent int) {
	paramCode := e.fs.ReduceToCode(string(PreVisitFuncTypeParam))
	e.fs.PushCode(paramCode)
}

func (e *CSharpEmitter) PostVisitFuncTypeParams(node *ast.FieldList, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncTypeParams))
	var paramTypes []string
	for _, t := range tokens {
		s := t.Serialize()
		if s != "" {
			paramTypes = append(paramTypes, s)
		}
	}
	e.fs.PushCode(strings.Join(paramTypes, ", "))
}

func (e *CSharpEmitter) PostVisitFuncType(node *ast.FuncType, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncType))
	// tokens: [0] = result types (if any), [1] = param types
	resultTypes := ""
	paramTypes := ""
	if node.Results != nil && node.Results.NumFields() > 0 {
		if len(tokens) >= 1 {
			resultTypes = tokens[0].Serialize()
		}
		if len(tokens) >= 2 {
			paramTypes = tokens[1].Serialize()
		}
		// Func<params, result>
		var children []Token
		if paramTypes != "" {
			children = []Token{
				Leaf(Identifier, "Func"),
				Leaf(LeftAngle, "<"),
				Leaf(Identifier, paramTypes),
				Leaf(Comma, ", "),
				Leaf(Identifier, resultTypes),
				Leaf(RightAngle, ">"),
			}
		} else {
			children = []Token{
				Leaf(Identifier, "Func"),
				Leaf(LeftAngle, "<"),
				Leaf(Identifier, resultTypes),
				Leaf(RightAngle, ">"),
			}
		}
		e.fs.PushTree(TokenTree(TypeKeyword, TagExpr, children...))
	} else {
		if len(tokens) >= 1 {
			paramTypes = tokens[0].Serialize()
		}
		// Action<params>
		if paramTypes != "" {
			children := []Token{
				Leaf(Identifier, "Action"),
				Leaf(LeftAngle, "<"),
				Leaf(Identifier, paramTypes),
				Leaf(RightAngle, ">"),
			}
			e.fs.PushTree(TokenTree(TypeKeyword, TagExpr, children...))
		} else {
			e.fs.PushCode("Action")
		}
	}
}

// ============================================================
// Function Literals (closures)
// ============================================================

func (e *CSharpEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	e.fs.Reduce(string(PreVisitFuncLitTypeParam))
	// Push parameter type and names
	typeStr := "object"
	if e.pkg != nil && e.pkg.TypesInfo != nil && len(node.Names) > 0 {
		if obj := e.pkg.TypesInfo.Defs[node.Names[0]]; obj != nil {
			typeStr = e.qualifiedCsTypeName(obj.Type())
		}
	}
	for _, name := range node.Names {
		tree := TokenTree(Identifier, TagIdent,
			Leaf(Identifier, typeStr),
			Leaf(WhiteSpace, " "),
			Leaf(Identifier, name.Name),
		)
		e.fs.PushTree(tree)
	}
}

func (e *CSharpEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncLitTypeParams))
	var paramNames []string
	for _, t := range tokens {
		if t.Tag == TagIdent && t.Serialize() != "" {
			paramNames = append(paramNames, t.Serialize())
		}
	}
	paramsStr := strings.Join(paramNames, ", ")
	if paramsStr == "" {
		paramsStr = " "
	}
	e.fs.PushCode(paramsStr)
}

func (e *CSharpEmitter) PostVisitFuncLitTypeResults(node *ast.FieldList, indent int) {
	e.fs.Reduce(string(PreVisitFuncLitTypeResults))
}

func (e *CSharpEmitter) PostVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitFuncLitBody))
	e.fs.PushCode(bodyCode)
}

func (e *CSharpEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncLit))
	paramsCode := ""
	bodyCode := ""
	if len(tokens) >= 1 {
		paramsCode = strings.TrimSpace(tokens[0].Serialize())
	}
	if len(tokens) >= 2 {
		bodyCode = tokens[1].Serialize()
	}
	e.fs.PushTree(TokenTree(Identifier, TagExpr,
		Leaf(LeftParen, "("),
		Leaf(Identifier, paramsCode),
		Leaf(RightParen, ")"),
		Leaf(BinaryOperator, " => "),
		Leaf(Identifier, bodyCode),
	))
}

// ============================================================
// Type Assertions
// ============================================================

func (e *CSharpEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	typeCode := e.fs.ReduceToCode(string(PreVisitTypeAssertExprType))
	e.fs.PushCode(typeCode)
}

func (e *CSharpEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitTypeAssertExprX))
	e.fs.PushCode(xCode)
}

func (e *CSharpEmitter) PostVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitTypeAssertExpr))
	typeCode := ""
	xCode := ""
	// Visitor order: Type is pushed first, then X
	if len(tokens) >= 1 {
		typeCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		xCode = tokens[1].Serialize()
	}
	// C# type assertion: (Type)x
	if typeCode != "" {
		e.fs.PushTree(TokenTree(Identifier, TagExpr,
			Leaf(LeftParen, "("),
			Leaf(Identifier, typeCode),
			Leaf(RightParen, ")"),
			Leaf(Identifier, xCode),
		))
	} else {
		e.fs.PushCode(xCode)
	}
}

// ============================================================
// Star Expressions (dereference — pass through in C#)
// ============================================================

func (e *CSharpEmitter) PostVisitStarExpr(node *ast.StarExpr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitStarExpr))
	e.fs.PushCode(xCode)
}

// ============================================================
// Interface Type
// ============================================================

func (e *CSharpEmitter) PostVisitInterfaceType(node *ast.InterfaceType, indent int) {
	e.fs.Reduce(string(PreVisitInterfaceType))
	e.fs.Push("object", TagType, nil)
}

// ============================================================
// Function Declarations
// ============================================================

func (e *CSharpEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	e.numFuncResults = 0
	e.funcReturnType = nil
	if node.Type.Results != nil {
		e.numFuncResults = node.Type.Results.NumFields()
		// Track single return type for narrowing casts
		if e.numFuncResults == 1 && e.pkg != nil && e.pkg.TypesInfo != nil {
			field := node.Type.Results.List[0]
			if tv, ok := e.pkg.TypesInfo.Types[field.Type]; ok && tv.Type != nil {
				e.funcReturnType = tv.Type
			}
		}
	}
}

func (e *CSharpEmitter) PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	typeCode := e.fs.ReduceToCode(string(PreVisitFuncDeclSignatureTypeResultsList))
	e.fs.PushCode(typeCode)
}

func (e *CSharpEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeResults))
	var resultTypes []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			resultTypes = append(resultTypes, t.Serialize())
		}
	}
	if len(resultTypes) == 0 {
		e.fs.Push("void", TagType, nil)
	} else if len(resultTypes) == 1 {
		e.fs.Push(resultTypes[0], TagType, nil)
	} else {
		e.fs.Push("("+strings.Join(resultTypes, ", ")+")", TagType, nil)
	}
}

func (e *CSharpEmitter) PostVisitFuncDeclName(node *ast.Ident, indent int) {
	e.fs.Reduce(string(PreVisitFuncDeclName))
	name := node.Name
	if name == "main" {
		name = "Main"
	}
	e.fs.Push(name, TagIdent, nil)
	e.refOptCurrentFunc = e.refOptCurrentPkg + "." + node.Name
	e.currentParamIndex = 0
	e.refOptCurrentRefParams = make(map[string]bool)
	e.refOptCurrentMutRefParams = make(map[string]bool)
}

func (e *CSharpEmitter) PostVisitFuncDeclSignatureTypeParamsListType(node ast.Expr, argName *ast.Ident, index int, indent int) {
	typeCode := e.fs.ReduceToCode(string(PreVisitFuncDeclSignatureTypeParamsListType))
	e.fs.PushCode(typeCode)
}

func (e *CSharpEmitter) PostVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeParamsArgName))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *CSharpEmitter) PostVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeParamsList))
	// tokens: type (TagExpr), then names (TagIdent)
	typeStr := ""
	var names []string
	for _, t := range tokens {
		if t.Tag == TagExpr && typeStr == "" {
			typeStr = t.Serialize()
		} else if t.Tag == TagIdent {
			names = append(names, t.Serialize())
		}
	}
	paramIdx := e.currentParamIndex
	for _, name := range names {
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
		if isRefOpt {
			e.fs.PushTree(TokenTree(Identifier, TagIdent,
				LeafTag(Keyword, "in ", TagCSharp),
				Leaf(Identifier, typeStr),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, name),
			))
			e.refOptCurrentRefParams[name] = true
			e.RefOptCount++
		} else if isMutRefOpt {
			e.fs.PushTree(TokenTree(Identifier, TagIdent,
				LeafTag(Keyword, "ref ", TagCSharp),
				Leaf(Identifier, typeStr),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, name),
			))
			e.refOptCurrentMutRefParams[name] = true
			e.RefOptCount++
		} else {
			e.fs.PushTree(TokenTree(Identifier, TagIdent,
				Leaf(Identifier, typeStr),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, name),
			))
		}
		paramIdx++
	}
	e.currentParamIndex = paramIdx
}

func (e *CSharpEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignatureTypeParams))
	var paramDecls []string
	for _, t := range tokens {
		if t.Tag == TagIdent {
			paramDecls = append(paramDecls, t.Serialize())
		}
	}
	e.fs.PushCode(strings.Join(paramDecls, ", "))
}

func (e *CSharpEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDeclSignature))
	returnType := "void"
	funcName := ""
	paramsStr := ""
	for _, t := range tokens {
		if t.Tag == TagType && returnType == "void" {
			returnType = t.Serialize()
		} else if t.Tag == TagIdent && funcName == "" {
			funcName = t.Serialize()
		} else if t.Tag == TagExpr {
			paramsStr = t.Serialize()
		}
	}

	if funcName == "Main" {
		sig := fmt.Sprintf("\npublic static %s %s(string[] args)", returnType, funcName)
		e.fs.PushCode(sig)
	} else {
		sig := fmt.Sprintf("\npublic static %s %s(%s)", returnType, funcName, paramsStr)
		e.fs.PushCode(sig)
	}
}

func (e *CSharpEmitter) PostVisitFuncDeclBody(node *ast.BlockStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitFuncDeclBody))
	e.fs.PushCode(bodyCode)
}

func (e *CSharpEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	tokens := e.fs.Reduce(string(PreVisitFuncDecl))
	sigCode := ""
	bodyCode := ""
	if len(tokens) >= 1 {
		sigCode = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		bodyCode = tokens[1].Serialize()
	}
	if node.Name.Name == "main" && strings.HasPrefix(bodyCode, "{\n") {
		bodyCode = "{\n" + csIndent(2) + "List<string> goany_os_args = new List<string>();\n" + csIndent(2) + "goany_os_args.Add(\"program\");\n" + csIndent(2) + "goany_os_args.AddRange(args);\n" + bodyCode[2:]
	}
	e.fs.PushCode(sigCode + " " + bodyCode + "\n")
}

// ============================================================
// Forward Declaration Signatures (suppressed)
// ============================================================

func (e *CSharpEmitter) PreVisitFuncDeclSignatures(indent int) {
	e.forwardDecl = true
}

func (e *CSharpEmitter) PostVisitFuncDeclSignatures(indent int) {
	e.fs.Reduce(string(PreVisitFuncDeclSignatures))
	e.forwardDecl = false
}

// ============================================================
// Block Statements
// ============================================================

func (e *CSharpEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	e.indent = indent
}

func (e *CSharpEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	itemCode := e.fs.ReduceToCode(string(PreVisitBlockStmtList))
	e.fs.PushCode(itemCode)
}

func (e *CSharpEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitBlockStmt))
	var children []Token
	children = append(children, Leaf(LeftBrace, "{\n"))
	for _, t := range tokens {
		if t.Serialize() != "" {
			children = append(children, t)
		}
	}
	children = append(children, Leaf(Identifier, csIndent(indent/2)))
	children = append(children, Leaf(RightBrace, "}"))
	e.fs.PushTree(TokenTree(Identifier, TagExpr, children...))
}

// ============================================================
// Assignment Statements
// ============================================================

func (e *CSharpEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	e.indent = indent
	e.mapAssignVar = ""
	e.mapAssignKey = ""
}

func (e *CSharpEmitter) PostVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
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

func (e *CSharpEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmtLhs))
	var lhsExprs []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			lhsExprs = append(lhsExprs, t.Serialize())
		}
	}
	e.fs.PushCode(strings.Join(lhsExprs, ", "))
}

func (e *CSharpEmitter) PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {
	rhsCode := e.fs.ReduceToCode(string(PreVisitAssignStmtRhsExpr))
	e.fs.PushCode(rhsCode)
}

func (e *CSharpEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmtRhs))
	var rhsExprs []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			rhsExprs = append(rhsExprs, t.Serialize())
		}
	}
	e.fs.PushCode(strings.Join(rhsExprs, ", "))
}

func (e *CSharpEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitAssignStmt))
	lhsStr := ""
	rhsStr := ""
	if len(tokens) >= 1 {
		lhsStr = tokens[0].Serialize()
	}
	if len(tokens) >= 2 {
		rhsStr = tokens[1].Serialize()
	}

	ind := csIndent(indent / 2)

	// Pointer alias elimination: emit comment instead of assignment
	if len(node.Lhs) == 1 {
		if lhsIdent, ok := node.Lhs[0].(*ast.Ident); ok {
			if comment, ok := PtrLocalComments[lhsIdent.Pos()]; ok {
				e.fs.PushTree(TokenTree(LineComment, TagExpr,
					Leaf(WhiteSpace, ind),
					Leaf(LineComment, comment),
					Leaf(NewLine, "\n"),
				))
				return
			}
		}
	}

	tokStr := node.Tok.String()

	// Mixed index chain: nested map/slice assignments like m["outer"]["inner"] = v
	if len(node.Lhs) == 1 {
		if _, isIndex := node.Lhs[0].(*ast.IndexExpr); isIndex && e.pkg != nil && e.pkg.TypesInfo != nil {
			ops, hasIntermediateMap := e.analyzeLhsIndexChainCs(node.Lhs[0])
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

				// Assign temp var names
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
				var children []Token

				// Prologue: extract temp variables for intermediate map accesses
				for i, op := range ops {
					if op.accessType == "map" && i < lastIdx {
						key := op.keyExpr
						if op.keyCastPfx != "" {
							key = op.keyCastPfx + key + op.keyCastSfx
						}
						children = append(children,
							Leaf(WhiteSpace, ind),
							LeafTag(Keyword, "var ", TagCSharp),
							Leaf(Identifier, op.tempVarName),
							Leaf(Assignment, " = "),
							Leaf(LeftParen, "("),
							Leaf(Identifier, op.valueCsType),
							Leaf(RightParen, ")"),
							Leaf(Identifier, "hmap.hashMapGet"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, op.mapVarExpr),
							Leaf(Comma, ", "),
							Leaf(Identifier, key),
							Leaf(RightParen, ")"),
							Leaf(Semicolon, ";"),
							Leaf(NewLine, "\n"),
						)
					}
				}

				// Assignment
				lastOp := ops[lastIdx]
				if lastOp.accessType == "map" {
					key := lastOp.keyExpr
					if lastOp.keyCastPfx != "" {
						key = lastOp.keyCastPfx + key + lastOp.keyCastSfx
					}
					children = append(children,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, lastOp.mapVarExpr),
						Leaf(Assignment, " = "),
						Leaf(Identifier, "hmap.hashMapSet"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, lastOp.mapVarExpr),
						Leaf(Comma, ", "),
						Leaf(Identifier, key),
						Leaf(Comma, ", "),
						Leaf(Identifier, rhsStr),
						Leaf(RightParen, ")"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					)
				} else {
					children = append(children,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, currentVar),
						Leaf(WhiteSpace, " "),
						Leaf(Assignment, tokStr),
						Leaf(WhiteSpace, " "),
						Leaf(Identifier, rhsStr),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					)
				}

				// Epilogue: write back intermediate maps in reverse
				for i := lastIdx - 1; i >= 0; i-- {
					op := ops[i]
					if op.accessType == "map" {
						key := op.keyExpr
						if op.keyCastPfx != "" {
							key = op.keyCastPfx + key + op.keyCastSfx
						}
						children = append(children,
							Leaf(WhiteSpace, ind),
							Leaf(Identifier, op.mapVarExpr),
							Leaf(Assignment, " = "),
							Leaf(Identifier, "hmap.hashMapSet"),
							Leaf(LeftParen, "("),
							Leaf(Identifier, op.mapVarExpr),
							Leaf(Comma, ", "),
							Leaf(Identifier, key),
							Leaf(Comma, ", "),
							Leaf(Identifier, op.tempVarName),
							Leaf(RightParen, ")"),
							Leaf(Semicolon, ";"),
							Leaf(NewLine, "\n"),
						)
					}
				}
				e.fs.PushTree(TokenTree(Identifier, TagExpr, children...))
				e.mapAssignVar = ""
				e.mapAssignKey = ""
				return
			}
		}
	}

	// Map assignment: m[k] = v -> hmap.hashMapSet(m, k, v)
	if e.mapAssignVar != "" && e.mapAssignKey != "" {
		mapGoType := e.getExprGoType(node.Lhs[0].(*ast.IndexExpr).X)
		pfx := ""
		sfx := ""
		if mapGoType != nil {
			if mapUnderlying, ok := mapGoType.Underlying().(*types.Map); ok {
				pfx, sfx = getCsKeyCast(mapUnderlying.Key())
			}
		}
		e.fs.PushTree(TokenTree(Identifier, TagExpr,
			Leaf(WhiteSpace, ind),
			Leaf(Identifier, e.mapAssignVar),
			Leaf(Assignment, " = "),
			Leaf(Identifier, "hmap.hashMapSet"),
			Leaf(LeftParen, "("),
			Leaf(Identifier, e.mapAssignVar),
			Leaf(Comma, ", "),
			Leaf(Identifier, pfx+e.mapAssignKey+sfx),
			Leaf(Comma, ", "),
			Leaf(Identifier, rhsStr),
			Leaf(RightParen, ")"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
		e.mapAssignVar = ""
		e.mapAssignKey = ""
		return
	}

	// C# struct value-type writeback: p[0].field = val
	// C# List<T> indexer returns a copy for value types (structs), causing CS1612.
	// Rewrite to: var __tmp = p[0]; __tmp.field = val; p[0] = __tmp;
	if len(node.Lhs) == 1 && tokStr != ":=" {
		if selExpr, ok := node.Lhs[0].(*ast.SelectorExpr); ok {
			if _, ok := selExpr.X.(*ast.IndexExpr); ok {
				elemType := e.getExprGoType(selExpr.X)
				if elemType != nil {
					if _, isStruct := elemType.Underlying().(*types.Struct); isStruct {
						fieldName := selExpr.Sel.Name
						// Compute index code from lhsStr by stripping ".fieldName"
						suffix := "." + fieldName
						indexCode := lhsStr
						if strings.HasSuffix(lhsStr, suffix) {
							indexCode = lhsStr[:len(lhsStr)-len(suffix)]
						}
						tmpVar := fmt.Sprintf("__struct_tmp_%d", e.nestedMapCounter)
						e.nestedMapCounter++
						e.fs.PushTree(TokenTree(Keyword, TagExpr,
							Leaf(WhiteSpace, ind),
							LeafTag(Keyword, "var ", TagCSharp),
							Leaf(Identifier, tmpVar),
							Leaf(Assignment, " = "),
							Leaf(Identifier, indexCode),
							Leaf(Semicolon, ";"),
							Leaf(NewLine, "\n"),
						))
						e.fs.PushTree(TokenTree(Identifier, TagExpr,
							Leaf(WhiteSpace, ind),
							Leaf(Identifier, tmpVar),
							Leaf(Dot, "."),
							Leaf(Identifier, fieldName),
							Leaf(WhiteSpace, " "),
							Leaf(Assignment, tokStr),
							Leaf(WhiteSpace, " "),
							Leaf(Identifier, rhsStr),
							Leaf(Semicolon, ";"),
							Leaf(NewLine, "\n"),
						))
						e.fs.PushTree(TokenTree(Identifier, TagExpr,
							Leaf(WhiteSpace, ind),
							Leaf(Identifier, indexCode),
							Leaf(Assignment, " = "),
							Leaf(Identifier, tmpVar),
							Leaf(Semicolon, ";"),
							Leaf(NewLine, "\n"),
						))
						return
					}
				}
			}
		}
	}

	// Comma-ok map read: val, ok := m[key]
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if indexExpr, ok := node.Rhs[0].(*ast.IndexExpr); ok {
			if e.isMapTypeExpr(indexExpr.X) {
				valName := exprToString(node.Lhs[0])
				okName := exprToString(node.Lhs[1])
				mapName := exprToString(indexExpr.X)
				keyStr := exprToString(indexExpr.Index)

				mapGoType := e.getExprGoType(indexExpr.X)
				valType := "object"
				pfx := ""
				sfx := ""
				zeroVal := "default"
				if mapGoType != nil {
					if mapUnderlying, ok2 := mapGoType.Underlying().(*types.Map); ok2 {
						valType = e.qualifiedCsTypeName(mapUnderlying.Elem())
						pfx, sfx = getCsKeyCast(mapUnderlying.Key())
						zeroVal = csDefaultForGoType(mapUnderlying.Elem())
					}
				}
				if tokStr == ":=" {
					e.fs.PushTree(TokenTree(Keyword, TagExpr,
						Leaf(WhiteSpace, ind),
						LeafTag(Keyword, "var ", TagCSharp),
						Leaf(Identifier, okName),
						Leaf(Assignment, " = "),
						Leaf(Identifier, "hmap.hashMapContains"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, mapName),
						Leaf(Comma, ", "),
						Leaf(Identifier, pfx+keyStr+sfx),
						Leaf(RightParen, ")"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
					e.fs.PushTree(TokenTree(Keyword, TagExpr,
						Leaf(WhiteSpace, ind),
						LeafTag(Keyword, "var ", TagCSharp),
						Leaf(Identifier, valName),
						Leaf(Assignment, " = "),
						Leaf(Identifier, okName),
						Leaf(Identifier, " ? "),
						Leaf(LeftParen, "("),
						Leaf(Identifier, valType),
						Leaf(RightParen, ")"),
						Leaf(Identifier, "hmap.hashMapGet"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, mapName),
						Leaf(Comma, ", "),
						Leaf(Identifier, pfx+keyStr+sfx),
						Leaf(RightParen, ")"),
						Leaf(Identifier, " : "),
						Leaf(Identifier, zeroVal),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
				} else {
					e.fs.PushTree(TokenTree(Identifier, TagExpr,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, okName),
						Leaf(Assignment, " = "),
						Leaf(Identifier, "hmap.hashMapContains"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, mapName),
						Leaf(Comma, ", "),
						Leaf(Identifier, pfx+keyStr+sfx),
						Leaf(RightParen, ")"),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
					e.fs.PushTree(TokenTree(Identifier, TagExpr,
						Leaf(WhiteSpace, ind),
						Leaf(Identifier, valName),
						Leaf(Assignment, " = "),
						Leaf(Identifier, okName),
						Leaf(Identifier, " ? "),
						Leaf(LeftParen, "("),
						Leaf(Identifier, valType),
						Leaf(RightParen, ")"),
						Leaf(Identifier, "hmap.hashMapGet"),
						Leaf(LeftParen, "("),
						Leaf(Identifier, mapName),
						Leaf(Comma, ", "),
						Leaf(Identifier, pfx+keyStr+sfx),
						Leaf(RightParen, ")"),
						Leaf(Identifier, " : "),
						Leaf(Identifier, zeroVal),
						Leaf(Semicolon, ";"),
						Leaf(NewLine, "\n"),
					))
				}
				return
			}
		}
	}

	// Comma-ok type assertion: val, ok := x.(Type)
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if typeAssert, ok := node.Rhs[0].(*ast.TypeAssertExpr); ok {
			valName := exprToString(node.Lhs[0])
			okName := exprToString(node.Lhs[1])
			assertType := ""
			if typeAssert.Type != nil {
				assertType = exprToString(typeAssert.Type)
				if csType, ok := csTypesMap[assertType]; ok {
					assertType = csType
				}
			}
			xExpr := exprToString(typeAssert.X)
			if tokStr == ":=" {
				e.fs.PushTree(TokenTree(Keyword, TagExpr,
					Leaf(WhiteSpace, ind),
					LeafTag(Keyword, "var ", TagCSharp),
					Leaf(Identifier, okName),
					Leaf(Assignment, " = "),
					Leaf(Identifier, xExpr),
					LeafTag(Keyword, " is ", TagCSharp),
					Leaf(Identifier, assertType),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
				e.fs.PushTree(TokenTree(Keyword, TagExpr,
					Leaf(WhiteSpace, ind),
					LeafTag(Keyword, "var ", TagCSharp),
					Leaf(Identifier, valName),
					Leaf(Assignment, " = "),
					Leaf(Identifier, okName),
					Leaf(Identifier, " ? "),
					Leaf(LeftParen, "("),
					Leaf(Identifier, assertType),
					Leaf(RightParen, ")"),
					Leaf(Identifier, xExpr),
					Leaf(Identifier, " : "),
					LeafTag(Keyword, "default", TagCSharp),
					Leaf(LeftParen, "("),
					Leaf(Identifier, assertType),
					Leaf(RightParen, ")"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
			} else {
				e.fs.PushTree(TokenTree(Identifier, TagExpr,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, okName),
					Leaf(Assignment, " = "),
					Leaf(Identifier, xExpr),
					LeafTag(Keyword, " is ", TagCSharp),
					Leaf(Identifier, assertType),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
				e.fs.PushTree(TokenTree(Identifier, TagExpr,
					Leaf(WhiteSpace, ind),
					Leaf(Identifier, valName),
					Leaf(Assignment, " = "),
					Leaf(Identifier, okName),
					Leaf(Identifier, " ? "),
					Leaf(LeftParen, "("),
					Leaf(Identifier, assertType),
					Leaf(RightParen, ")"),
					Leaf(Identifier, xExpr),
					Leaf(Identifier, " : "),
					LeafTag(Keyword, "default", TagCSharp),
					Leaf(LeftParen, "("),
					Leaf(Identifier, assertType),
					Leaf(RightParen, ")"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				))
			}
			return
		}
	}

	// Multi-value return: a, b := func() -> var (a, b) = func()
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
		destructured := "(" + strings.Join(lhsParts, ", ") + ")"
		if tokStr == ":=" {
			e.fs.PushTree(TokenTree(Keyword, TagExpr,
				Leaf(WhiteSpace, ind),
				LeafTag(Keyword, "var ", TagCSharp),
				Leaf(Identifier, destructured),
				Leaf(Assignment, " = "),
				Leaf(Identifier, rhsStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		} else {
			e.fs.PushTree(TokenTree(Identifier, TagExpr,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, destructured),
				Leaf(Assignment, " = "),
				Leaf(Identifier, rhsStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		}
		return
	}

	// Check if LHS needs narrowing cast (sbyte, short, byte, ushort)
	narrowCast := ""
	if len(node.Lhs) == 1 {
		if lhsType := e.getExprGoType(node.Lhs[0]); lhsType != nil {
			if basic, ok := lhsType.Underlying().(*types.Basic); ok {
				switch basic.Kind() {
				case types.Int8:
					narrowCast = "(sbyte)"
				case types.Int16:
					narrowCast = "(short)"
				case types.Uint8:
					narrowCast = "(byte)"
				case types.Uint16:
					narrowCast = "(ushort)"
				}
			}
		}
	}

	switch tokStr {
	case ":=":
		if narrowCast != "" {
			e.fs.PushTree(TokenTree(Keyword, TagExpr,
				Leaf(WhiteSpace, ind),
				LeafTag(Keyword, "var ", TagCSharp),
				Leaf(Identifier, lhsStr),
				Leaf(Assignment, " = "),
				Leaf(Identifier, narrowCast),
				Leaf(LeftParen, "("),
				Leaf(Identifier, rhsStr),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		} else {
			e.fs.PushTree(TokenTree(Keyword, TagExpr,
				Leaf(WhiteSpace, ind),
				LeafTag(Keyword, "var ", TagCSharp),
				Leaf(Identifier, lhsStr),
				Leaf(Assignment, " = "),
				Leaf(Identifier, rhsStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		}
	case "+=", "-=", "*=", "/=", "%=", "&=", "|=", "^=", "<<=", ">>=":
		e.fs.PushTree(TokenTree(Identifier, TagExpr,
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
		if narrowCast != "" {
			e.fs.PushTree(TokenTree(Identifier, TagExpr,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, lhsStr),
				Leaf(Assignment, " = "),
				Leaf(Identifier, narrowCast),
				Leaf(LeftParen, "("),
				Leaf(Identifier, rhsStr),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		} else {
			e.fs.PushTree(TokenTree(Identifier, TagExpr,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, lhsStr),
				Leaf(Assignment, " = "),
				Leaf(Identifier, rhsStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			))
		}
	}
}

// ============================================================
// Declaration Statements (var x int, var y = 5)
// ============================================================

func (e *CSharpEmitter) PreVisitDeclStmt(node *ast.DeclStmt, indent int) {
	e.indent = indent
}

func (e *CSharpEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	tokens := e.fs.Reduce(string(PreVisitDeclStmtValueSpecType))
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
	e.fs.Push(typeStr, TagType, goType)
}

func (e *CSharpEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	e.fs.Reduce(string(PreVisitDeclStmtValueSpecNames))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *CSharpEmitter) PostVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitDeclStmtValueSpecValue))
	e.fs.Push(valCode, TagExpr, nil)
}

func (e *CSharpEmitter) PostVisitDeclStmt(node *ast.DeclStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitDeclStmt))
	ind := csIndent(indent / 2)

	var children []Token
	i := 0
	for i < len(tokens) {
		typeStr := ""
		var goType types.Type
		nameStr := ""
		valueStr := ""

		if i < len(tokens) && tokens[i].Tag == TagType {
			typeStr = tokens[i].Serialize()
			goType = tokens[i].GoType
			i++
		}
		if i < len(tokens) && tokens[i].Tag == TagIdent {
			nameStr = tokens[i].Serialize()
			i++
		}
		if i < len(tokens) && tokens[i].Tag == TagExpr {
			valueStr = tokens[i].Serialize()
			i++
		}

		if nameStr == "" {
			continue
		}

		if valueStr != "" {
			children = append(children,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, typeStr),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, nameStr),
				Leaf(Assignment, " = "),
				Leaf(Identifier, valueStr),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		} else {
			// No initializer - generate default value
			defaultVal := "default"
			if goType != nil {
				if _, isSlice := goType.Underlying().(*types.Slice); isSlice {
					defaultVal = fmt.Sprintf("new %s()", typeStr)
				} else if _, isMap := goType.Underlying().(*types.Map); isMap {
					if e.pkg != nil && e.pkg.TypesInfo != nil {
						// Find the map type to get key type constant
						for _, spec := range node.Decl.(*ast.GenDecl).Specs {
							if vs, ok := spec.(*ast.ValueSpec); ok {
								if mapType, ok := vs.Type.(*ast.MapType); ok {
									keyConst := e.getMapKeyTypeConst(mapType)
									defaultVal = fmt.Sprintf("hmap.newHashMap(%d)", keyConst)
								}
							}
						}
					}
				} else if _, isStruct := goType.Underlying().(*types.Struct); isStruct {
					defaultVal = fmt.Sprintf("new %s()", typeStr)
				} else {
					defaultVal = csDefaultForGoType(goType)
				}
			}
			children = append(children,
				Leaf(WhiteSpace, ind),
				Leaf(Identifier, typeStr),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, nameStr),
				Leaf(Assignment, " = "),
				Leaf(Identifier, defaultVal),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		}
	}
	e.fs.PushTree(TokenTree(Identifier, TagExpr, children...))
}

// ============================================================
// Return Statements
// ============================================================

func (e *CSharpEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	e.indent = indent
}

func (e *CSharpEmitter) PostVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	resultCode := e.fs.ReduceToCode(string(PreVisitReturnStmtResult))
	e.fs.PushCode(resultCode)
}

func (e *CSharpEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitReturnStmt))
	ind := csIndent(indent / 2)

	if len(tokens) == 0 {
		e.fs.PushCode(ind + "return;\n")
	} else if len(tokens) == 1 {
		retExpr := tokens[0].Serialize()
		// Add narrowing cast if return type is narrower than int
		if e.funcReturnType != nil {
			if basic, ok := e.funcReturnType.Underlying().(*types.Basic); ok {
				switch basic.Kind() {
				case types.Int8:
					retExpr = fmt.Sprintf("(sbyte)(%s)", retExpr)
				case types.Int16:
					retExpr = fmt.Sprintf("(short)(%s)", retExpr)
				case types.Uint8:
					retExpr = fmt.Sprintf("(byte)(%s)", retExpr)
				case types.Uint16:
					retExpr = fmt.Sprintf("(ushort)(%s)", retExpr)
				}
			}
		}
		e.fs.PushTree(TokenTree(ReturnKeyword, TagExpr,
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return "),
			Leaf(Identifier, retExpr),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	} else {
		// Multi-value return: return (a, b)
		var vals []string
		for _, t := range tokens {
			vals = append(vals, t.Serialize())
		}
		e.fs.PushTree(TokenTree(ReturnKeyword, TagExpr,
			Leaf(WhiteSpace, ind),
			Leaf(ReturnKeyword, "return "),
			Leaf(LeftParen, "("),
			Leaf(Identifier, strings.Join(vals, ", ")),
			Leaf(RightParen, ")"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		))
	}
}

// ============================================================
// Expression Statements
// ============================================================

func (e *CSharpEmitter) PreVisitExprStmt(node *ast.ExprStmt, indent int) {
	e.indent = indent
}

func (e *CSharpEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitExprStmtX))
	e.fs.PushCode(xCode)
}

func (e *CSharpEmitter) PostVisitExprStmt(node *ast.ExprStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitExprStmt))
	code := ""
	if len(tokens) >= 1 {
		code = tokens[0].Serialize()
	}
	ind := csIndent(indent / 2)
	e.fs.PushCode(ind + code + ";\n")
}

// ============================================================
// If Statements
// ============================================================

func (e *CSharpEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	e.indent = indent
	e.ifInitStack = append(e.ifInitStack, "")
	e.ifCondStack = append(e.ifCondStack, "")
	e.ifBodyStack = append(e.ifBodyStack, "")
	e.ifElseStack = append(e.ifElseStack, "")
}

func (e *CSharpEmitter) PostVisitIfStmtInit(node ast.Stmt, indent int) {
	e.ifInitStack[len(e.ifInitStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtInit))
}

func (e *CSharpEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	e.ifCondStack[len(e.ifCondStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtCond))
}

func (e *CSharpEmitter) PostVisitIfStmtBody(node *ast.IfStmt, indent int) {
	e.ifBodyStack[len(e.ifBodyStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtBody))
}

func (e *CSharpEmitter) PostVisitIfStmtElse(node *ast.IfStmt, indent int) {
	e.ifElseStack[len(e.ifElseStack)-1] = e.fs.ReduceToCode(string(PreVisitIfStmtElse))
}

func (e *CSharpEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	e.fs.Reduce(string(PreVisitIfStmt))
	ind := csIndent(indent / 2)

	n := len(e.ifInitStack)
	initCode := e.ifInitStack[n-1]
	condCode := e.ifCondStack[n-1]
	bodyCode := e.ifBodyStack[n-1]
	elseCode := e.ifElseStack[n-1]
	e.ifInitStack = e.ifInitStack[:n-1]
	e.ifCondStack = e.ifCondStack[:n-1]
	e.ifBodyStack = e.ifBodyStack[:n-1]
	e.ifElseStack = e.ifElseStack[:n-1]

	var children []Token
	if initCode != "" {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(LeftBrace, "{\n"),
			Leaf(Identifier, initCode),
		)
	}
	children = append(children,
		Leaf(WhiteSpace, ind),
		Leaf(IfKeyword, "if "),
		Leaf(LeftParen, "("),
		Leaf(Identifier, condCode),
		Leaf(RightParen, ") "),
		Leaf(Identifier, bodyCode),
	)
	if elseCode != "" {
		trimmed := strings.TrimLeft(elseCode, " \t\n")
		if strings.HasPrefix(trimmed, "if ") || strings.HasPrefix(trimmed, "if(") {
			children = append(children,
				Leaf(ElseKeyword, " else "),
				Leaf(Identifier, trimmed),
			)
		} else {
			children = append(children,
				Leaf(ElseKeyword, " else "),
				Leaf(Identifier, elseCode),
			)
		}
	}
	children = append(children, Leaf(NewLine, "\n"))
	if initCode != "" {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(RightBrace, "}\n"),
		)
	}
	e.fs.PushTree(TokenTree(IfKeyword, TagExpr, children...))
}

// ============================================================
// For Statements
// ============================================================

func (e *CSharpEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	e.indent = indent
	e.forInitStack = append(e.forInitStack, "")
	e.forCondStack = append(e.forCondStack, "")
	e.forPostStack = append(e.forPostStack, "")
}

func (e *CSharpEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	initCode := e.fs.ReduceToCode(string(PreVisitForStmtInit))
	initCode = strings.TrimRight(initCode, ";\n \t")
	initCode = strings.TrimLeft(initCode, " \t")
	e.forInitStack[len(e.forInitStack)-1] = initCode
}

func (e *CSharpEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	e.forCondStack[len(e.forCondStack)-1] = e.fs.ReduceToCode(string(PreVisitForStmtCond))
}

func (e *CSharpEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	postCode := e.fs.ReduceToCode(string(PreVisitForStmtPost))
	postCode = strings.TrimRight(postCode, ";\n \t")
	postCode = strings.TrimLeft(postCode, " \t")
	e.forPostStack[len(e.forPostStack)-1] = postCode
}

func (e *CSharpEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {
	bodyCode := e.fs.ReduceToCode(string(PreVisitForStmt))
	ind := csIndent(indent / 2)

	n := len(e.forInitStack)
	initCode := e.forInitStack[n-1]
	condCode := e.forCondStack[n-1]
	postCode := e.forPostStack[n-1]
	e.forInitStack = e.forInitStack[:n-1]
	e.forCondStack = e.forCondStack[:n-1]
	e.forPostStack = e.forPostStack[:n-1]

	if node.Init == nil && node.Cond == nil && node.Post == nil {
		e.fs.PushTree(TokenTree(WhileKeyword, TagExpr,
			Leaf(WhiteSpace, ind),
			Leaf(WhileKeyword, "while "),
			Leaf(LeftParen, "("),
			Leaf(BooleanLiteral, "true"),
			Leaf(RightParen, ") "),
			Leaf(Identifier, bodyCode),
			Leaf(NewLine, "\n"),
		))
		return
	}

	if node.Init == nil && node.Post == nil && node.Cond != nil {
		e.fs.PushTree(TokenTree(WhileKeyword, TagExpr,
			Leaf(WhiteSpace, ind),
			Leaf(WhileKeyword, "while "),
			Leaf(LeftParen, "("),
			Leaf(Identifier, condCode),
			Leaf(RightParen, ") "),
			Leaf(Identifier, bodyCode),
			Leaf(NewLine, "\n"),
		))
		return
	}

	e.fs.PushTree(TokenTree(ForKeyword, TagExpr,
		Leaf(WhiteSpace, ind),
		Leaf(ForKeyword, "for "),
		Leaf(LeftParen, "("),
		Leaf(Identifier, initCode),
		Leaf(Semicolon, "; "),
		Leaf(Identifier, condCode),
		Leaf(Semicolon, "; "),
		Leaf(Identifier, postCode),
		Leaf(RightParen, ") "),
		Leaf(Identifier, bodyCode),
		Leaf(NewLine, "\n"),
	))
}

// ============================================================
// Range Statements
// ============================================================

func (e *CSharpEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	e.indent = indent
}

func (e *CSharpEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	keyCode := e.fs.ReduceToCode(string(PreVisitRangeStmtKey))
	e.fs.Push(keyCode, TagIdent, nil)
}

func (e *CSharpEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	valCode := e.fs.ReduceToCode(string(PreVisitRangeStmtValue))
	e.fs.Push(valCode, TagIdent, nil)
}

func (e *CSharpEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitRangeStmtX))
	e.fs.PushCode(xCode)
}

func (e *CSharpEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitRangeStmt))
	ind := csIndent(indent / 2)

	keyCode := ""
	valCode := ""
	xCode := ""
	bodyCode := ""

	idx := 0
	if node.Key != nil {
		if idx < len(tokens) && tokens[idx].Tag == TagIdent {
			keyCode = tokens[idx].Serialize()
			idx++
		}
	}
	if node.Value != nil {
		if idx < len(tokens) && tokens[idx].Tag == TagIdent {
			valCode = tokens[idx].Serialize()
			idx++
		}
	}
	if idx < len(tokens) {
		xCode = tokens[idx].Serialize()
		idx++
	}
	if idx < len(tokens) {
		bodyCode = tokens[idx].Serialize()
	}
	if node.Key == nil && valCode != "" {
		keyCode = "_"
	}

	isMap := false
	if node.X != nil {
		isMap = e.isMapTypeExpr(node.X)
	}

	if isMap {
		// Map range: iterate using hashMapKeys
		mapGoType := e.getExprGoType(node.X)
		pfx := ""
		sfx := ""
		valType := "object"
		keyType := "object"
		if mapGoType != nil {
			if mapUnderlying, ok := mapGoType.Underlying().(*types.Map); ok {
				pfx, sfx = getCsKeyCast(mapUnderlying.Key())
				valType = e.qualifiedCsTypeName(mapUnderlying.Elem())
				keyType = e.qualifiedCsTypeName(mapUnderlying.Key())
			}
		}
		_ = pfx
		_ = sfx
		keysVar := fmt.Sprintf("_keys%d", e.rangeVarCounter)
		loopIdx := fmt.Sprintf("_mi%d", e.rangeVarCounter)
		e.rangeVarCounter++
		if valCode != "" && valCode != "_" {
			var children []Token
			children = append(children, Leaf(WhiteSpace, ind), Leaf(LeftBrace, "{\n"))
			children = append(children,
				Leaf(WhiteSpace, ind+"  "),
				LeafTag(Keyword, "var ", TagCSharp),
				Leaf(Identifier, keysVar),
				Leaf(Assignment, " = "),
				Leaf(Identifier, "hmap.hashMapKeys"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, xCode),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
			children = append(children,
				Leaf(WhiteSpace, ind+"  "),
				Leaf(ForKeyword, "for "),
				Leaf(LeftParen, "("),
				LeafTag(Keyword, "var ", TagCSharp),
				Leaf(Identifier, loopIdx),
				Leaf(Assignment, " = "),
				Leaf(NumberLiteral, "0"),
				Leaf(Semicolon, "; "),
				Leaf(Identifier, loopIdx),
				Leaf(ComparisonOperator, " < "),
				Leaf(Identifier, keysVar),
				Leaf(Dot, "."),
				Leaf(Identifier, "Count"),
				Leaf(Semicolon, "; "),
				Leaf(Identifier, loopIdx),
				Leaf(UnaryOperator, "++"),
				Leaf(RightParen, ") "),
				Leaf(LeftBrace, "{\n"),
			)
			if keyCode != "_" {
				children = append(children,
					Leaf(WhiteSpace, ind+"    "),
					LeafTag(Keyword, "var ", TagCSharp),
					Leaf(Identifier, keyCode),
					Leaf(Assignment, " = "),
					Leaf(LeftParen, "("),
					Leaf(Identifier, keyType),
					Leaf(RightParen, ")"),
					Leaf(Identifier, keysVar),
					Leaf(LeftBracket, "["),
					Leaf(Identifier, loopIdx),
					Leaf(RightBracket, "]"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				)
			}
			children = append(children,
				Leaf(WhiteSpace, ind+"    "),
				LeafTag(Keyword, "var ", TagCSharp),
				Leaf(Identifier, valCode),
				Leaf(Assignment, " = "),
				Leaf(LeftParen, "("),
				Leaf(Identifier, valType),
				Leaf(RightParen, ")"),
				Leaf(Identifier, "hmap.hashMapGet"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, xCode),
				Leaf(Comma, ", "),
				Leaf(Identifier, keysVar),
				Leaf(LeftBracket, "["),
				Leaf(Identifier, loopIdx),
				Leaf(RightBracket, "]"),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
			children = append(children,
				Leaf(WhiteSpace, ind+"    "),
				Leaf(Identifier, bodyCode),
				Leaf(NewLine, "\n"),
			)
			children = append(children, Leaf(WhiteSpace, ind+"  "), Leaf(RightBrace, "}\n"))
			children = append(children, Leaf(WhiteSpace, ind), Leaf(RightBrace, "}\n"))
			e.fs.PushTree(TokenTree(ForKeyword, TagExpr, children...))
		} else {
			var children []Token
			children = append(children, Leaf(WhiteSpace, ind), Leaf(LeftBrace, "{\n"))
			children = append(children,
				Leaf(WhiteSpace, ind+"  "),
				LeafTag(Keyword, "var ", TagCSharp),
				Leaf(Identifier, keysVar),
				Leaf(Assignment, " = "),
				Leaf(Identifier, "hmap.hashMapKeys"),
				Leaf(LeftParen, "("),
				Leaf(Identifier, xCode),
				Leaf(RightParen, ")"),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
			children = append(children,
				Leaf(WhiteSpace, ind+"  "),
				Leaf(ForKeyword, "for "),
				Leaf(LeftParen, "("),
				LeafTag(Keyword, "var ", TagCSharp),
				Leaf(Identifier, loopIdx),
				Leaf(Assignment, " = "),
				Leaf(NumberLiteral, "0"),
				Leaf(Semicolon, "; "),
				Leaf(Identifier, loopIdx),
				Leaf(ComparisonOperator, " < "),
				Leaf(Identifier, keysVar),
				Leaf(Dot, "."),
				Leaf(Identifier, "Count"),
				Leaf(Semicolon, "; "),
				Leaf(Identifier, loopIdx),
				Leaf(UnaryOperator, "++"),
				Leaf(RightParen, ") "),
				Leaf(LeftBrace, "{\n"),
			)
			if keyCode != "_" {
				children = append(children,
					Leaf(WhiteSpace, ind+"    "),
					LeafTag(Keyword, "var ", TagCSharp),
					Leaf(Identifier, keyCode),
					Leaf(Assignment, " = "),
					Leaf(LeftParen, "("),
					Leaf(Identifier, keyType),
					Leaf(RightParen, ")"),
					Leaf(Identifier, keysVar),
					Leaf(LeftBracket, "["),
					Leaf(Identifier, loopIdx),
					Leaf(RightBracket, "]"),
					Leaf(Semicolon, ";"),
					Leaf(NewLine, "\n"),
				)
			}
			children = append(children,
				Leaf(WhiteSpace, ind+"    "),
				Leaf(Identifier, bodyCode),
				Leaf(NewLine, "\n"),
			)
			children = append(children, Leaf(WhiteSpace, ind+"  "), Leaf(RightBrace, "}\n"))
			children = append(children, Leaf(WhiteSpace, ind), Leaf(RightBrace, "}\n"))
			e.fs.PushTree(TokenTree(ForKeyword, TagExpr, children...))
		}
		return
	}

	// Check if ranging over string (affects .Count vs .Length)
	xType := e.getExprGoType(node.X)
	lenExpr := xCode + ".Count"
	if xType != nil {
		if basic, ok := xType.Underlying().(*types.Basic); ok && basic.Kind() == types.String {
			lenExpr = xCode + ".Length"
		}
	}

	// If range expression is an inline composite literal, emit a temp variable
	if _, isCompLit := node.X.(*ast.CompositeLit); isCompLit {
		tmpVar := fmt.Sprintf("_range%d", e.rangeVarCounter)
		e.rangeVarCounter++
		var children []Token
		children = append(children, Leaf(WhiteSpace, ind), Leaf(LeftBrace, "{\n"))
		children = append(children,
			Leaf(WhiteSpace, ind+"  "),
			LeafTag(Keyword, "var ", TagCSharp),
			Leaf(Identifier, tmpVar),
			Leaf(Assignment, " = "),
			Leaf(Identifier, xCode),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		)
		xCode = tmpVar
		lenExpr = xCode + ".Count"
		if valCode != "" && valCode != "_" {
			loopVar := keyCode
			if loopVar == "_" || loopVar == "" {
				loopVar = fmt.Sprintf("_i%d", e.rangeVarCounter)
				e.rangeVarCounter++
			}
			valDecl := fmt.Sprintf("%s      var %s = %s[%s];\n", ind, valCode, xCode, loopVar)
			bodyWithDecl := strings.Replace(bodyCode, "{\n", "{\n"+valDecl, 1)
			children = append(children,
				Leaf(WhiteSpace, ind+"  "),
				Leaf(ForKeyword, "for "),
				Leaf(LeftParen, "("),
				LeafTag(Keyword, "var ", TagCSharp),
				Leaf(Identifier, loopVar),
				Leaf(Assignment, " = "),
				Leaf(NumberLiteral, "0"),
				Leaf(Semicolon, "; "),
				Leaf(Identifier, loopVar),
				Leaf(ComparisonOperator, " < "),
				Leaf(Identifier, lenExpr),
				Leaf(Semicolon, "; "),
				Leaf(Identifier, loopVar),
				Leaf(UnaryOperator, "++"),
				Leaf(RightParen, ") "),
				Leaf(Identifier, bodyWithDecl),
				Leaf(NewLine, "\n"),
			)
		} else {
			loopVar := keyCode
			if loopVar == "_" || loopVar == "" {
				loopVar = fmt.Sprintf("_i%d", e.rangeVarCounter)
				e.rangeVarCounter++
			}
			children = append(children,
				Leaf(WhiteSpace, ind+"  "),
				Leaf(ForKeyword, "for "),
				Leaf(LeftParen, "("),
				LeafTag(Keyword, "var ", TagCSharp),
				Leaf(Identifier, loopVar),
				Leaf(Assignment, " = "),
				Leaf(NumberLiteral, "0"),
				Leaf(Semicolon, "; "),
				Leaf(Identifier, loopVar),
				Leaf(ComparisonOperator, " < "),
				Leaf(Identifier, lenExpr),
				Leaf(Semicolon, "; "),
				Leaf(Identifier, loopVar),
				Leaf(UnaryOperator, "++"),
				Leaf(RightParen, ") "),
				Leaf(Identifier, bodyCode),
				Leaf(NewLine, "\n"),
			)
		}
		children = append(children, Leaf(WhiteSpace, ind), Leaf(RightBrace, "}\n"))
		e.fs.PushTree(TokenTree(ForKeyword, TagExpr, children...))
		return
	}

	// Slice/string range
	if valCode != "" && valCode != "_" {
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = fmt.Sprintf("_i%d", e.rangeVarCounter)
			e.rangeVarCounter++
		}

		valDecl := fmt.Sprintf("%s    var %s = %s[%s];\n", ind, valCode, xCode, loopVar)
		bodyWithDecl := strings.Replace(bodyCode, "{\n", "{\n"+valDecl, 1)

		e.fs.PushTree(TokenTree(ForKeyword, TagExpr,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for "),
			Leaf(LeftParen, "("),
			LeafTag(Keyword, "var ", TagCSharp),
			Leaf(Identifier, loopVar),
			Leaf(Assignment, " = "),
			Leaf(NumberLiteral, "0"),
			Leaf(Semicolon, "; "),
			Leaf(Identifier, loopVar),
			Leaf(ComparisonOperator, " < "),
			Leaf(Identifier, lenExpr),
			Leaf(Semicolon, "; "),
			Leaf(Identifier, loopVar),
			Leaf(UnaryOperator, "++"),
			Leaf(RightParen, ") "),
			Leaf(Identifier, bodyWithDecl),
			Leaf(NewLine, "\n"),
		))
	} else {
		loopVar := keyCode
		if loopVar == "_" || loopVar == "" {
			loopVar = fmt.Sprintf("_i%d", e.rangeVarCounter)
			e.rangeVarCounter++
		}
		e.fs.PushTree(TokenTree(ForKeyword, TagExpr,
			Leaf(WhiteSpace, ind),
			Leaf(ForKeyword, "for "),
			Leaf(LeftParen, "("),
			LeafTag(Keyword, "var ", TagCSharp),
			Leaf(Identifier, loopVar),
			Leaf(Assignment, " = "),
			Leaf(NumberLiteral, "0"),
			Leaf(Semicolon, "; "),
			Leaf(Identifier, loopVar),
			Leaf(ComparisonOperator, " < "),
			Leaf(Identifier, lenExpr),
			Leaf(Semicolon, "; "),
			Leaf(Identifier, loopVar),
			Leaf(UnaryOperator, "++"),
			Leaf(RightParen, ") "),
			Leaf(Identifier, bodyCode),
			Leaf(NewLine, "\n"),
		))
	}
}

// ============================================================
// Switch / Case Statements
// ============================================================

func (e *CSharpEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	e.indent = indent
}

func (e *CSharpEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	tagCode := e.fs.ReduceToCode(string(PreVisitSwitchStmtTag))
	e.fs.PushCode(tagCode)
}

func (e *CSharpEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	tokens := e.fs.Reduce(string(PreVisitSwitchStmt))
	ind := csIndent(indent / 2)

	tagCode := ""
	idx := 0
	if idx < len(tokens) {
		tagCode = tokens[idx].Serialize()
		idx++
	}

	var children []Token
	children = append(children,
		Leaf(WhiteSpace, ind),
		Leaf(SwitchKeyword, "switch "),
		Leaf(LeftParen, "("),
		Leaf(Identifier, tagCode),
		Leaf(RightParen, ") "),
		Leaf(LeftBrace, "{\n"),
	)
	for i := idx; i < len(tokens); i++ {
		children = append(children, tokens[i])
	}
	children = append(children, Leaf(WhiteSpace, ind), Leaf(RightBrace, "}\n"))
	e.fs.PushTree(TokenTree(SwitchKeyword, TagExpr, children...))
}

func (e *CSharpEmitter) PreVisitCaseClause(node *ast.CaseClause, indent int) {
	e.indent = indent
}

func (e *CSharpEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	exprCode := e.fs.ReduceToCode(string(PreVisitCaseClauseListExpr))
	e.fs.PushCode(exprCode)
}

func (e *CSharpEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCaseClauseList))
	var exprs []string
	for _, t := range tokens {
		if t.Serialize() != "" {
			exprs = append(exprs, t.Serialize())
		}
	}
	e.fs.PushCode(strings.Join(exprs, ", "))
}

func (e *CSharpEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	tokens := e.fs.Reduce(string(PreVisitCaseClause))
	ind := csIndent(indent / 2)

	var children []Token
	idx := 0
	if len(node.List) == 0 {
		children = append(children,
			Leaf(WhiteSpace, ind),
			Leaf(DefaultKeyword, "default"),
			Leaf(Colon, ":\n"),
		)
	} else {
		caseExprs := ""
		if idx < len(tokens) {
			caseExprs = tokens[idx].Serialize()
			idx++
		}
		vals := strings.Split(caseExprs, ", ")
		for _, v := range vals {
			children = append(children,
				Leaf(WhiteSpace, ind),
				Leaf(CaseKeyword, "case "),
				Leaf(Identifier, v),
				Leaf(Colon, ":\n"),
			)
		}
	}
	for i := idx; i < len(tokens); i++ {
		children = append(children, tokens[i])
	}
	bodyStr := TokenTree(CaseKeyword, TagExpr, children...).Serialize()
	if !strings.Contains(bodyStr, "return ") && !strings.Contains(bodyStr, "break;") {
		children = append(children,
			Leaf(WhiteSpace, ind+"  "),
			Leaf(BreakKeyword, "break"),
			Leaf(Semicolon, ";"),
			Leaf(NewLine, "\n"),
		)
	}
	e.fs.PushTree(TokenTree(CaseKeyword, TagExpr, children...))
}

// ============================================================
// Inc/Dec Statements
// ============================================================

func (e *CSharpEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	xCode := e.fs.ReduceToCode(string(PreVisitIncDecStmt))
	ind := csIndent(indent / 2)
	e.fs.PushTree(TokenTree(Identifier, TagExpr,
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

func (e *CSharpEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	ind := csIndent(indent / 2)
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

func (e *CSharpEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	typeCode := e.fs.ReduceToCode(string(PreVisitGenStructFieldType))
	e.fs.PushCode(typeCode)
}

func (e *CSharpEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	e.fs.Reduce(string(PreVisitGenStructFieldName))
	e.fs.Push(node.Name, TagIdent, nil)
}

func (e *CSharpEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {
	tokens := e.fs.Reduce(string(PreVisitGenStructInfo))

	if node.Struct == nil {
		return
	}

	// Collect field types and names
	type fieldInfo struct {
		typeName string
		name     string
	}
	var fields []fieldInfo
	i := 0
	for i < len(tokens) {
		if tokens[i].Tag == TagExpr {
			fi := fieldInfo{typeName: tokens[i].Serialize()}
			i++
			if i < len(tokens) && tokens[i].Tag == TagIdent {
				fi.name = tokens[i].Serialize()
				i++
			}
			fields = append(fields, fi)
		} else if tokens[i].Tag == TagIdent {
			// Field name without explicit type token
			fields = append(fields, fieldInfo{typeName: "object", name: tokens[i].Serialize()})
			i++
		} else {
			i++
		}
	}

	hasStringField := false
	for _, f := range fields {
		if f.typeName == "string" {
			hasStringField = true
		}
	}
	var children []Token
	children = append(children,
		LeafTag(Keyword, "public ", TagCSharp),
		Leaf(StructKeyword, "struct "),
		Leaf(Identifier, node.Name),
		Leaf(WhiteSpace, " "),
		Leaf(LeftBrace, "{\n"),
	)
	if hasStringField {
		children = append(children,
			Leaf(WhiteSpace, "  "),
			LeafTag(Keyword, "public ", TagCSharp),
			Leaf(Identifier, node.Name),
			Leaf(LeftParen, "("),
			Leaf(RightParen, ")"),
			Leaf(WhiteSpace, " "),
			Leaf(LeftBrace, "{"),
			Leaf(RightBrace, "}\n"),
		)
	}
	for _, f := range fields {
		if f.typeName == "string" {
			children = append(children,
				Leaf(WhiteSpace, "  "),
				LeafTag(Keyword, "public ", TagCSharp),
				Leaf(Identifier, f.typeName),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, f.name),
				Leaf(Assignment, " = "),
				Leaf(StringLiteral, "\"\""),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		} else {
			children = append(children,
				Leaf(WhiteSpace, "  "),
				LeafTag(Keyword, "public ", TagCSharp),
				Leaf(Identifier, f.typeName),
				Leaf(WhiteSpace, " "),
				Leaf(Identifier, f.name),
				Leaf(Semicolon, ";"),
				Leaf(NewLine, "\n"),
			)
		}
	}
	children = append(children, Leaf(RightBrace, "}\n"), Leaf(NewLine, "\n"))
	e.fs.PushTree(TokenTree(StructKeyword, TagExpr, children...))
}

func (e *CSharpEmitter) PostVisitGenStructInfos(node []GenTypeInfo, indent int) {
	// Structs are already pushed to the stack
}

// ============================================================
// Constants (GenDeclConst)
// ============================================================

func (e *CSharpEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	valTokens := e.fs.Reduce(string(PreVisitGenDeclConstName))
	valCode := ""
	for _, t := range valTokens {
		valCode += t.Serialize()
	}
	if valCode == "" {
		valCode = "0"
	}

	// Determine the type from type info
	constType := "int"
	if e.pkg != nil && e.pkg.TypesInfo != nil {
		if obj := e.pkg.TypesInfo.Defs[node]; obj != nil {
			// Use Underlying() to resolve named types (e.g., type ExprKind int → int)
			ut := obj.Type().Underlying()
			resolved := getCsTypeName(ut)
			// Handle untyped constants (e.g., const X = 1 without explicit type)
			if resolved == "object" {
				if basic, ok := ut.(*types.Basic); ok {
					if basic.Info()&types.IsInteger != 0 {
						resolved = "int"
					} else if basic.Info()&types.IsFloat != 0 {
						resolved = "double"
					} else if basic.Info()&types.IsString != 0 {
						resolved = "string"
					} else if basic.Info()&types.IsBoolean != 0 {
						resolved = "bool"
					}
				}
			}
			constType = resolved
		}
	}

	name := node.Name
	// Use const for basic types, static readonly for others
	e.fs.PushTree(TokenTree(Keyword, TagExpr,
		LeafTag(Keyword, "public const ", TagCSharp),
		Leaf(Identifier, constType),
		Leaf(WhiteSpace, " "),
		Leaf(Identifier, name),
		Leaf(Assignment, " = "),
		Leaf(Identifier, valCode),
		Leaf(Semicolon, ";"),
		Leaf(NewLine, "\n"),
	))
}

func (e *CSharpEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {
	// Let const tokens flow through
}

// ============================================================
// Type Aliases
// ============================================================

func (e *CSharpEmitter) PreVisitTypeAliasName(node *ast.Ident, indent int) {
	// Store the alias name so PostVisitTypeAliasType can use it
	e.currentAliasName = node.Name
}

func (e *CSharpEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	// Reduce all tokens from the alias name marker onwards (consumes both name and type tokens)
	e.fs.Reduce(string(PreVisitTypeAliasName))

	// Store the alias mapping: aliasName -> underlyingType (converted to C# syntax)
	if e.currentAliasName != "" {
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if tv, ok := e.pkg.TypesInfo.Types[node]; ok && tv.Type != nil {
				underlyingType := tv.Type.String()
				underlyingType = convertGoTypeToCSharp(underlyingType)
				if e.typeAliasMap == nil {
					e.typeAliasMap = make(map[string]string)
				}
				e.typeAliasMap[e.currentAliasName] = underlyingType
			}
		}
	}
	e.currentAliasName = ""
}

// ============================================================
// GenerateCsproj generates the .csproj file for the C# backend.
// ============================================================

func (e *CSharpEmitter) GenerateCsproj() error {
	if e.LinkRuntime == "" {
		return nil
	}

	csprojPath := filepath.Join(e.OutputDir, e.OutputName+".csproj")
	file, err := os.Create(csprojPath)
	if err != nil {
		return fmt.Errorf("failed to create .csproj: %w", err)
	}
	defer file.Close()

	graphicsBackend := e.RuntimePackages["graphics"]
	if graphicsBackend == "" {
		graphicsBackend = "none"
	}

	var csproj string
	switch graphicsBackend {
	case "none":
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
		csproj = `<Project Sdk="Microsoft.NET.Sdk">

  <PropertyGroup>
    <OutputType>Exe</OutputType>
    <TargetFramework>net9.0</TargetFramework>
    <ImplicitUsings>enable</ImplicitUsings>
    <Nullable>enable</Nullable>
    <AllowUnsafeBlocks>true</AllowUnsafeBlocks>
  </PropertyGroup>

  <Target Name="CompileTigr" BeforeTargets="Build">
    <Exec Command="cc -shared -o $(OutputPath)libtigr.dylib tigr.c screen_helper.c -framework OpenGL -framework Cocoa -framework CoreGraphics"
          Condition="$([MSBuild]::IsOSPlatform('OSX'))"
          WorkingDirectory="$(ProjectDir)" />
    <Exec Command="gcc -shared -fPIC -o $(OutputPath)libtigr.so tigr.c screen_helper.c -lGL -lX11 -lm"
          Condition="$([MSBuild]::IsOSPlatform('Linux'))"
          WorkingDirectory="$(ProjectDir)" />
    <Exec Command="gcc -shared -o $(OutputPath)tigr.dll tigr.c screen_helper.c -lopengl32 -lgdi32 -luser32 -lshell32 -ladvapi32"
          Condition="$([MSBuild]::IsOSPlatform('Windows'))"
          WorkingDirectory="$(ProjectDir)" />
  </Target>

  <Target Name="EnsureOutputDir" BeforeTargets="CompileTigr">
    <MakeDir Directories="$(OutputPath)" />
  </Target>

</Project>
`
	default:
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

// CopyRuntimePackages copies runtime .cs files for all detected runtime packages.
func (e *CSharpEmitter) CopyRuntimePackages() error {
	if e.LinkRuntime == "" {
		return nil
	}
	for name, variant := range e.RuntimePackages {
		if variant == "none" {
			continue
		}
		capName := strings.ToUpper(name[:1]) + name[1:]

		var srcFileName string
		if variant != "" {
			capVariant := strings.ToUpper(variant[:1]) + variant[1:]
			srcFileName = capName + "Runtime" + capVariant + ".cs"
		} else {
			srcFileName = capName + "Runtime.cs"
		}

		runtimeSrcPath := filepath.Join(e.LinkRuntime, name, "csharp", srcFileName)
		content, err := os.ReadFile(runtimeSrcPath)
		if err != nil {
			DebugLogPrintf("Skipping C# runtime for %s: %v", name, err)
			continue
		}

		dstFileName := capName + "Runtime.cs"
		dstPath := filepath.Join(e.OutputDir, dstFileName)
		if err := os.WriteFile(dstPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", dstFileName, err)
		}
		DebugLogPrintf("Copied %s from %s to %s", dstFileName, runtimeSrcPath, dstPath)

		if name == "graphics" && variant == "tigr" {
			for _, extraFile := range []string{"tigr.c", "tigr.h", "screen_helper.c"} {
				src := filepath.Join(e.LinkRuntime, "graphics", "cpp", extraFile)
				dst := filepath.Join(e.OutputDir, extraFile)
				data, err := os.ReadFile(src)
				if err != nil {
					return fmt.Errorf("failed to read %s from %s: %w", extraFile, src, err)
				}
				if err := os.WriteFile(dst, data, 0644); err != nil {
					return fmt.Errorf("failed to write %s: %w", extraFile, err)
				}
				DebugLogPrintf("Copied %s to %s", extraFile, dst)
			}
		}
	}
	return nil
}

// csDefaultForTypeStr returns the C# default value for a Go type name string.
func (e *CSharpEmitter) csDefaultForTypeStr(typeStr string) string {
	switch typeStr {
	case "int", "sbyte", "short", "long",
		"byte", "ushort", "uint", "ulong",
		"float", "double":
		return "0"
	case "string":
		return `""`
	case "bool":
		return "false"
	case "hmap.HashMap":
		return "default"
	}
	if strings.HasPrefix(typeStr, "List<") {
		return "new " + typeStr + "()"
	}
	return "default"
}

// csDefaultForASTType returns the C# default value for a Go AST type expression.
func (e *CSharpEmitter) csDefaultForASTType(expr ast.Expr) string {
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
		if e.pkg != nil && e.pkg.TypesInfo != nil {
			if obj := e.pkg.TypesInfo.Uses[t]; obj != nil {
				if named, ok := obj.Type().(*types.Named); ok {
					if _, isStruct := named.Underlying().(*types.Struct); isStruct {
						return fmt.Sprintf("new %s()", t.Name)
					}
				}
			}
		}
		return "default"
	case *ast.ArrayType:
		return "default"
	case *ast.MapType:
		return "default"
	case *ast.SelectorExpr:
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
		return "default"
	case *ast.InterfaceType:
		return "default"
	case *ast.FuncType:
		return "default"
	}
	return "default"
}

// analyzeLhsIndexChainCs2 walks an IndexExpr chain for mixed nested assignments.
func (e *CSharpEmitter) analyzeLhsIndexChainCs2(expr ast.Expr) (ops []csMixedIndexOp, hasIntermediateMap bool) {
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
			op := csMixedIndexOp{
				accessType:  "map",
				keyExpr:     exprToCsString(ie.Index),
				valueCsType: e.qualifiedCsTypeName(mapType.Elem()),
			}
			op.keyCastPfx, op.keyCastSfx = getCsKeyCast(mapType.Key())
			if !isLast {
				hasIntermediateMap = true
			}
			ops = append(ops, op)
		} else {
			op := csMixedIndexOp{
				accessType: "slice",
				keyExpr:    exprToCsString(ie.Index),
			}
			ops = append(ops, op)
		}
	}
	return ops, hasIntermediateMap
}
