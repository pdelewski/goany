package compiler

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	goanyrt "goany/runtime"

	"golang.org/x/tools/go/packages"
)

// Java type mapping - note Java has no unsigned types
var javaTypesMap = map[string]string{
	"int8":    "byte",
	"int16":   "short",
	"int32":   "int",
	"int64":   "long",
	"uint8":   "int",    // No unsigned in Java, use wider signed
	"uint16":  "int",    // No unsigned in Java
	"uint32":  "long",   // No unsigned in Java
	"uint64":  "long",   // No unsigned in Java (would need BigInteger for full range)
	"int":     "int",
	"byte":    "byte",
	"rune":    "int",
	"any":     "Object",
	"string":  "String",
	"float32": "float",
	"float64": "double",
	"bool":    "boolean",
}

// javaBoxedTypes maps primitive types to their boxed versions for generics
var javaBoxedTypes = map[string]string{
	"byte":    "Byte",
	"short":   "Short",
	"int":     "Integer",
	"long":    "Long",
	"float":   "Float",
	"double":  "Double",
	"boolean": "Boolean",
	"char":    "Character",
}

// javaKeyCastState holds key cast values for nested map reads
type javaKeyCastState struct {
	prefix    string
	suffix    string
	valueType string
}

// mapIndexStackEntry holds state for nested map index expressions
type mapIndexStackEntry struct {
	isMapIndex        bool
	mapIndexValueType string
	mapReadVarName    string
	mapKeyCastPrefix  string
	mapKeyCastSuffix  string
}

type javaMixedIndexOp struct {
	accessType   string
	keyExpr      string
	keyCastPfx   string
	keyCastSfx   string
	valueJavaType string
	tempVarName  string
	mapVarExpr   string
}

// structLitState holds state for partial struct literal initialization
type structLitState struct {
	isStructLit       bool               // True when in a struct literal with key-value pairs
	structLitType     types.Type         // The struct type being constructed
	fieldOrder        []string           // Field names in order
	fieldTypes        map[string]string  // Field name -> Java type
	currentKey        string             // Current key being processed
	capturing         bool               // True when capturing elements to buffer
	buffer            *strings.Builder   // Buffer for current element (pointer to avoid copy issues)
	fieldBuffers      map[string]string  // Field name -> captured value string
}

type JavaEmitter struct {
	Output          string
	OutputDir       string
	OutputName      string
	LinkRuntime     string
	RuntimePackages map[string]string
	file            *os.File
	BaseEmitter
	pkg                        *packages.Package
	// Track files per package for separate file generation
	mainFile                   *os.File           // The main output file
	packageFiles               map[string]*os.File // Package name -> file
	currentPkgName             string             // Current package being processed
	insideForPostCond          bool
	assignmentToken            string
	forwardDecls               bool
	shouldGenerate             bool
	numFuncResults             int
	currentPackage             string
	isArray                    bool
	arrayType                  string
	isTuple                    bool
	isInfiniteLoop             bool
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
	inTypeContext              bool
	inArrayTypeContext         bool  // Track when we're inside ArrayList<...> to use boxed types
	inCompositeLitType         bool  // Track when we're inside a composite literal type (new Type(...))
	inPackageQualifiedExpr     bool  // Track when we're in a package-qualified expression (pkg.Type)
	// Struct literal handling for partial initialization (stack-based for nesting)
	structLitStack []structLitState // Stack for nested struct literals
	apiClassOpened             bool
	suppressTypeAliasEmit      bool
	currentAliasName           string
	typeAliasMap               map[string]string
	suppressTypeAliasSelectorX bool
	// Map support
	isMapMakeCall     bool
	mapMakeKeyType    int
	isMapIndex        bool
	mapIndexValueType string
	mapIndexStack     []mapIndexStackEntry // Stack for nested map index expressions
	isMapAssign       bool
	mapAssignVarName  string
	mapAssignIndent   int
	isNestedMapAssign      bool
	nestedMapRootVar       string             // Root variable for nested map (e.g., "m" for m["a"]["b"])
	nestedMapLevels        []nestedMapLevel   // All levels from root to the assignment level
	nestedMapAssignKey     string             // The final key being assigned to
	nestedMapAssignIndent  int
	// Slice-of-map assignment: sliceOfMaps[i][j]...["key"] = v (supports any depth)
	isSliceOfMapAssign    bool
	sliceOfMapSliceVar    string   // The root slice variable name
	sliceOfMapSliceIndices []string // All slice indices from root to map
	sliceOfMapMapKey      string   // The map key expression
	sliceOfMapIndent      int
	captureMapKey         bool
	capturedMapKey        string
	suppressMapEmit       bool
	isDeleteCall          bool
	deleteMapVarName      string
	mapKeyCastPrefix      string
	mapKeyCastSuffix      string
	mapKeyCastStack       []javaKeyCastState
	isMapLenCall          bool
	pendingMapInit        bool
	pendingMapKeyType     int
	pendingStructInit     bool   // Whether to add = new Type() for struct variable
	pendingStructTypeName string // Name of the struct type to initialize
	pendingSliceInit      bool   // Whether to add = new ArrayList<>() for slice variable
	pendingSliceElemType  string // Element type for the slice initialization
	pendingPrimitiveInit    bool   // Whether to add = default for primitive variable
	pendingPrimitiveDefault string // Default value for primitive ("0", "false", etc.)
	isMapCommaOk          bool
	mapCommaOkValName     string
	mapCommaOkOkName      string
	mapCommaOkMapName     string
	mapCommaOkValType     string
	mapCommaOkIsDecl      bool
	mapCommaOkIndent      int
	isTypeAssertCommaOk        bool
	typeAssertCommaOkValName   string
	typeAssertCommaOkOkName    string
	typeAssertCommaOkXName     string
	typeAssertCommaOkType      string
	typeAssertCommaOkBoxedType string
	typeAssertCommaOkIsDecl    bool
	typeAssertCommaOkIndent    int
	isSliceMakeCall     bool
	sliceMakeElemType   string
	isAppendCall        bool     // Track when we're in an append call
	appendStructType    string   // Type name for struct copy constructor
	indexAccessTypeStack    []string
	suppressMapEmitStack    []bool
	isMixedIndexAssign      bool
	mixedIndexOps           []javaMixedIndexOp
	mixedBaseVarName        string
	mixedAssignIndent       int
	// fmt package suppression
	suppressFmtPackage bool
	// struct field key suppression in composite literals
	suppressKeyValueKey bool
	// multiple return type suppression
	suppressMultiReturnTypes bool
	// flag to track if we're inside for loop init (to suppress semicolon)
	insideForInit bool
	// flag to suppress lambda parameter types (Java uses type inference)
	inLambdaParams bool
	// flag to suppress LHS emission during map assignment
	suppressMapAssignLhs bool
	// flag to suppress map index X emission (for map reads)
	suppressMapIndexX bool
	// stored map variable name for read
	mapReadVarName string
	// Multi-return function support
	currentFuncName       string              // Name of current function being processed
	multiReturnFuncs      map[string][]string // Map of func name to return type names
	isMultiReturnAssign   bool                // Flag for multi-return assignment
	multiReturnLhsNames   []string            // LHS variable names for unpacking
	multiReturnLhsTypes   []string            // LHS variable Java types for casting (runtime calls)
	multiReturnIsDecl     bool                // Whether it's a := declaration
	multiReturnIndent     int                 // Indent for the assignment
	resultClassDefs       []string            // Collected result class definitions
	multiReturnCounter    int                 // Counter for unique result variable names
	// If-init handling (Java doesn't support if init; cond)
	hasIfInit     bool // Flag for current if statement having an init
	ifInitIndent  int  // Stored indent for if statement with init
	// Unsupported indexed function call (x[0](args))
	isUnsupportedIndexedCall bool
	unsupportedCallIndent    int
	// Track when we're inside call expression arguments (for method reference detection)
	inCallExprArgs bool
	// Track when we're visiting the Fun part of a CallExpr (being called, not passed as value)
	inCallExprFunDepth int
	// Call expression parameter types for byte/short casting
	callExprParamTypes []string
	// Declaration statement value tracking
	hasDeclValue bool // True if current declaration has an explicit initialization value
	// Slice index assignment (x[i] = value -> x.set(i, value))
	isSliceIndexAssign  bool
	sliceIndexVarName   string
	sliceIndexIndent    int
	sliceIndexLhsExpr   *ast.IndexExpr // Track the actual LHS index expression
	// Multi-return type reuse - map signature to class name
	resultClassRegistry map[string]string
	// String comparison handling
	isStringComparison     bool   // Flag for string == or != comparison
	stringComparisonOp     string // "==" or "!="
	stringComparisonLhsExpr string // LHS expression as string for .equals() call
	// HashMap value type tracking - map variable name to value type
	mapVarValueTypes map[string]string
	// Variable shadowing - track declared variables per scope
	declaredVarsStack []map[string]bool
	// Track current function parameters (for lambda shadowing detection)
	funcParamsStack []map[string]bool
	// Track map value type for current map get operation
	currentMapGetValueType string
	// Slice expression handling
	sliceExprXName string // Store the X expression for a[low:high] to use in size() call
	// Byte/short casting for slice literals
	sliceLitElemCast string // Cast prefix for slice literal elements (e.g., "(byte)" or "(short)")
	// Byte/short arithmetic casting
	needsSmallIntCast bool   // Whether assignment RHS needs byte/short cast
	smallIntCastType  string // The type to cast to ("byte" or "short")
	// Functional interface call tracking
	isFuncVarCall        bool   // Whether current call is a function variable (needs .accept() etc.)
	funcFieldCallMethod  string // Method to call on function field (e.g., "apply" for BiFunction)
	// Closure-captured mutable variable tracking (for Java's "effectively final" requirement)
	closureCapturedMutVars  map[string]bool   // Variables that are captured by closures and mutated
	closureCapturedVarType  map[string]string // Captured variable name -> Java array element type
	inClosureLit            bool              // Track if we're inside a FuncLit body
	closureLitDepth         int               // Depth of nested closures (for scoping)
	isClosureCapturedDecl   bool              // True if current declaration is for a closure-captured variable
	closureCapturedDeclName string            // Name of current closure-captured variable being declared
	inDeclName              bool              // True when emitting a declaration variable name
	suppressClosureTypeEmit bool              // Suppress type emission for closure-captured variables (we emit Object[] instead)
	inAssignLhs             bool              // True when processing LHS of assignment (skip cast for captured vars)
	inSelectorX             bool              // True when processing X part of a SelectorExpr (need cast for field access)
}

// Helper functions

func (je *JavaEmitter) convertGoTypeToJava(goType string) string {
	if javaType, ok := javaTypesMap[goType]; ok {
		return javaType
	}
	return goType
}

func (je *JavaEmitter) getBoxedType(primitiveType string) string {
	if boxed, ok := javaBoxedTypes[primitiveType]; ok {
		return boxed
	}
	return primitiveType
}

// isVarDeclared checks if a variable is already declared in any active scope
func (je *JavaEmitter) isVarDeclared(name string) bool {
	for _, scope := range je.declaredVarsStack {
		if scope[name] {
			return true
		}
	}
	return false
}

// isFuncParam checks if a name is a function parameter in any active scope
func (je *JavaEmitter) isFuncParam(name string) bool {
	for _, scope := range je.funcParamsStack {
		if scope[name] {
			return true
		}
	}
	return false
}

// addFuncParam adds a parameter name to the current function parameter scope
func (je *JavaEmitter) addFuncParam(name string) {
	if len(je.funcParamsStack) > 0 {
		je.funcParamsStack[len(je.funcParamsStack)-1][name] = true
	}
}

// analyzeClosureCapturedMutVars analyzes a function body to find variables that are:
// 1. Declared in the outer scope (not inside a closure)
// 2. Captured by a closure (referenced inside a FuncLit)
// 3. Assigned somewhere (either in the outer scope or inside the closure)
// These variables need to be wrapped in single-element arrays for Java's "effectively final" requirement.
func (je *JavaEmitter) analyzeClosureCapturedMutVars(body *ast.BlockStmt) {
	if body == nil {
		return
	}

	je.closureCapturedMutVars = make(map[string]bool)
	je.closureCapturedVarType = make(map[string]string)

	// Step 1: Find all variable declarations at the outer level (not inside closures)
	outerVars := make(map[string]bool)
	outerVarTypes := make(map[string]types.Type)

	// Step 2: Find all assignments (to identify mutated variables)
	assignedVars := make(map[string]bool)

	// Step 3: Find all identifiers inside closures (captured variables)
	capturedVars := make(map[string]bool)

	// Helper to collect identifiers from an expression
	var collectIdents func(expr ast.Expr, inClosure bool)
	collectIdents = func(expr ast.Expr, inClosure bool) {
		if expr == nil {
			return
		}
		switch e := expr.(type) {
		case *ast.Ident:
			if inClosure {
				capturedVars[e.Name] = true
			}
		case *ast.BinaryExpr:
			collectIdents(e.X, inClosure)
			collectIdents(e.Y, inClosure)
		case *ast.UnaryExpr:
			collectIdents(e.X, inClosure)
		case *ast.CallExpr:
			collectIdents(e.Fun, inClosure)
			for _, arg := range e.Args {
				collectIdents(arg, inClosure)
			}
		case *ast.IndexExpr:
			collectIdents(e.X, inClosure)
			collectIdents(e.Index, inClosure)
		case *ast.SelectorExpr:
			collectIdents(e.X, inClosure)
		case *ast.ParenExpr:
			collectIdents(e.X, inClosure)
		case *ast.SliceExpr:
			collectIdents(e.X, inClosure)
			collectIdents(e.Low, inClosure)
			collectIdents(e.High, inClosure)
			collectIdents(e.Max, inClosure)
		case *ast.CompositeLit:
			for _, elt := range e.Elts {
				collectIdents(elt, inClosure)
			}
		case *ast.KeyValueExpr:
			collectIdents(e.Value, inClosure)
		case *ast.FuncLit:
			// Nested closure - recurse with inClosure=true
			// But first, collect parameter names to exclude them from captured variables
			lambdaParams := make(map[string]bool)
			if e.Type != nil && e.Type.Params != nil {
				for _, field := range e.Type.Params.List {
					for _, name := range field.Names {
						lambdaParams[name.Name] = true
					}
				}
			}
			if e.Body != nil {
				// Create a new collectIdents that excludes lambda parameters
				closureCollectIdents := func(expr ast.Expr, inClosure bool) {
					collectIdentsExcluding(expr, inClosure, lambdaParams, capturedVars, collectIdents)
				}
				analyzeClosureBodyWithCollector(e.Body, true, &assignedVars, capturedVars, closureCollectIdents)
			}
		}
	}

	// Walk the body
	for _, stmt := range body.List {
		walkStmtForClosureAnalysis(stmt, false, outerVars, outerVarTypes, &assignedVars, &capturedVars, collectIdents, je.pkg)
	}

	// Find variables that are: (1) outer vars, (2) captured, (3) assigned
	for varName := range outerVars {
		if capturedVars[varName] && assignedVars[varName] {
			je.closureCapturedMutVars[varName] = true
			// Get the Java type for the array wrapper
			if varType, ok := outerVarTypes[varName]; ok && varType != nil {
				je.closureCapturedVarType[varName] = getJavaTypeName(varType)
			}
		}
	}
}

// collectIdentsExcluding collects identifiers but excludes lambda parameters
func collectIdentsExcluding(expr ast.Expr, inClosure bool, excludeVars map[string]bool, capturedVars map[string]bool, baseCollect func(ast.Expr, bool)) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Ident:
		// Only mark as captured if NOT in the exclude list (lambda parameters)
		if inClosure && !excludeVars[e.Name] {
			capturedVars[e.Name] = true
		}
	case *ast.BinaryExpr:
		collectIdentsExcluding(e.X, inClosure, excludeVars, capturedVars, baseCollect)
		collectIdentsExcluding(e.Y, inClosure, excludeVars, capturedVars, baseCollect)
	case *ast.UnaryExpr:
		collectIdentsExcluding(e.X, inClosure, excludeVars, capturedVars, baseCollect)
	case *ast.CallExpr:
		collectIdentsExcluding(e.Fun, inClosure, excludeVars, capturedVars, baseCollect)
		for _, arg := range e.Args {
			collectIdentsExcluding(arg, inClosure, excludeVars, capturedVars, baseCollect)
		}
	case *ast.IndexExpr:
		collectIdentsExcluding(e.X, inClosure, excludeVars, capturedVars, baseCollect)
		collectIdentsExcluding(e.Index, inClosure, excludeVars, capturedVars, baseCollect)
	case *ast.SelectorExpr:
		collectIdentsExcluding(e.X, inClosure, excludeVars, capturedVars, baseCollect)
	case *ast.ParenExpr:
		collectIdentsExcluding(e.X, inClosure, excludeVars, capturedVars, baseCollect)
	case *ast.SliceExpr:
		collectIdentsExcluding(e.X, inClosure, excludeVars, capturedVars, baseCollect)
		collectIdentsExcluding(e.Low, inClosure, excludeVars, capturedVars, baseCollect)
		collectIdentsExcluding(e.High, inClosure, excludeVars, capturedVars, baseCollect)
		collectIdentsExcluding(e.Max, inClosure, excludeVars, capturedVars, baseCollect)
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			collectIdentsExcluding(elt, inClosure, excludeVars, capturedVars, baseCollect)
		}
	case *ast.KeyValueExpr:
		collectIdentsExcluding(e.Value, inClosure, excludeVars, capturedVars, baseCollect)
	case *ast.FuncLit:
		// Nested closure inside - need to add its parameters to exclude list too
		nestedExclude := make(map[string]bool)
		for k, v := range excludeVars {
			nestedExclude[k] = v
		}
		if e.Type != nil && e.Type.Params != nil {
			for _, field := range e.Type.Params.List {
				for _, name := range field.Names {
					nestedExclude[name.Name] = true
				}
			}
		}
		if e.Body != nil {
			nestedCollect := func(expr ast.Expr, inClosure bool) {
				collectIdentsExcluding(expr, inClosure, nestedExclude, capturedVars, baseCollect)
			}
			analyzeClosureBodyWithCollector(e.Body, true, nil, capturedVars, nestedCollect)
		}
	}
}

// analyzeClosureBodyWithCollector analyzes statements inside a closure body with a custom collector
func analyzeClosureBodyWithCollector(body *ast.BlockStmt, inClosure bool, assignedVars *map[string]bool, capturedVars map[string]bool, collectIdents func(ast.Expr, bool)) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		walkStmtInClosureWithCollector(stmt, inClosure, assignedVars, capturedVars, collectIdents)
	}
}

// walkStmtInClosureWithCollector walks a statement inside a closure with a custom collector
func walkStmtInClosureWithCollector(stmt ast.Stmt, inClosure bool, assignedVars *map[string]bool, capturedVars map[string]bool, collectIdents func(ast.Expr, bool)) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		// Track assigned variables (only if assignedVars is provided)
		for _, lhs := range s.Lhs {
			if assignedVars != nil {
				if ident, ok := lhs.(*ast.Ident); ok {
					(*assignedVars)[ident.Name] = true
				}
			}
			collectIdents(lhs, inClosure)
		}
		for _, rhs := range s.Rhs {
			collectIdents(rhs, inClosure)
		}
	case *ast.ExprStmt:
		collectIdents(s.X, inClosure)
	case *ast.ReturnStmt:
		for _, result := range s.Results {
			collectIdents(result, inClosure)
		}
	case *ast.IfStmt:
		collectIdents(s.Cond, inClosure)
		if s.Body != nil {
			analyzeClosureBodyWithCollector(s.Body, inClosure, assignedVars, capturedVars, collectIdents)
		}
		if s.Else != nil {
			walkStmtInClosureWithCollector(s.Else, inClosure, assignedVars, capturedVars, collectIdents)
		}
	case *ast.ForStmt:
		walkStmtInClosureWithCollector(s.Init, inClosure, assignedVars, capturedVars, collectIdents)
		collectIdents(s.Cond, inClosure)
		walkStmtInClosureWithCollector(s.Post, inClosure, assignedVars, capturedVars, collectIdents)
		if s.Body != nil {
			analyzeClosureBodyWithCollector(s.Body, inClosure, assignedVars, capturedVars, collectIdents)
		}
	case *ast.RangeStmt:
		collectIdents(s.X, inClosure)
		if s.Body != nil {
			analyzeClosureBodyWithCollector(s.Body, inClosure, assignedVars, capturedVars, collectIdents)
		}
	case *ast.BlockStmt:
		analyzeClosureBodyWithCollector(s, inClosure, assignedVars, capturedVars, collectIdents)
	case *ast.IncDecStmt:
		if assignedVars != nil {
			if ident, ok := s.X.(*ast.Ident); ok {
				(*assignedVars)[ident.Name] = true
			}
		}
		collectIdents(s.X, inClosure)
	}
}

// analyzeClosureBody analyzes statements inside a closure body
func analyzeClosureBody(body *ast.BlockStmt, inClosure bool, assignedVars *map[string]bool, capturedVars *map[string]bool, collectIdents func(ast.Expr, bool)) {
	if body == nil {
		return
	}
	for _, stmt := range body.List {
		walkStmtInClosure(stmt, inClosure, assignedVars, capturedVars, collectIdents)
	}
}

// walkStmtInClosure walks a statement inside a closure to find captured and assigned variables
func walkStmtInClosure(stmt ast.Stmt, inClosure bool, assignedVars *map[string]bool, capturedVars *map[string]bool, collectIdents func(ast.Expr, bool)) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		// Track assigned variables
		for _, lhs := range s.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				(*assignedVars)[ident.Name] = true
			}
			collectIdents(lhs, inClosure)
		}
		for _, rhs := range s.Rhs {
			collectIdents(rhs, inClosure)
		}
	case *ast.ExprStmt:
		collectIdents(s.X, inClosure)
	case *ast.ReturnStmt:
		for _, result := range s.Results {
			collectIdents(result, inClosure)
		}
	case *ast.IfStmt:
		collectIdents(s.Cond, inClosure)
		if s.Body != nil {
			analyzeClosureBody(s.Body, inClosure, assignedVars, capturedVars, collectIdents)
		}
		if s.Else != nil {
			walkStmtInClosure(s.Else, inClosure, assignedVars, capturedVars, collectIdents)
		}
	case *ast.ForStmt:
		walkStmtInClosure(s.Init, inClosure, assignedVars, capturedVars, collectIdents)
		collectIdents(s.Cond, inClosure)
		walkStmtInClosure(s.Post, inClosure, assignedVars, capturedVars, collectIdents)
		if s.Body != nil {
			analyzeClosureBody(s.Body, inClosure, assignedVars, capturedVars, collectIdents)
		}
	case *ast.RangeStmt:
		collectIdents(s.X, inClosure)
		if s.Body != nil {
			analyzeClosureBody(s.Body, inClosure, assignedVars, capturedVars, collectIdents)
		}
	case *ast.BlockStmt:
		analyzeClosureBody(s, inClosure, assignedVars, capturedVars, collectIdents)
	case *ast.IncDecStmt:
		if ident, ok := s.X.(*ast.Ident); ok {
			(*assignedVars)[ident.Name] = true
		}
		collectIdents(s.X, inClosure)
	}
}

// walkStmtForClosureAnalysis walks a statement at the outer level to find declarations and closures
func walkStmtForClosureAnalysis(stmt ast.Stmt, inClosure bool, outerVars map[string]bool, outerVarTypes map[string]types.Type, assignedVars *map[string]bool, capturedVars *map[string]bool, collectIdents func(ast.Expr, bool), pkg *packages.Package) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.DeclStmt:
		// Variable declarations
		if genDecl, ok := s.Decl.(*ast.GenDecl); ok && genDecl.Tok == token.VAR {
			for _, spec := range genDecl.Specs {
				if valueSpec, ok := spec.(*ast.ValueSpec); ok {
					for _, name := range valueSpec.Names {
						if !inClosure {
							outerVars[name.Name] = true
							// Get type info
							if pkg != nil && pkg.TypesInfo != nil {
								if obj := pkg.TypesInfo.Defs[name]; obj != nil {
									outerVarTypes[name.Name] = obj.Type()
								}
							}
						}
					}
					// Check RHS for closures
					for _, value := range valueSpec.Values {
						collectIdents(value, inClosure)
					}
				}
			}
		}
	case *ast.AssignStmt:
		// Track declared variables (short declarations)
		if s.Tok == token.DEFINE {
			for _, lhs := range s.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if !inClosure {
						outerVars[ident.Name] = true
						// Get type info from RHS
						if pkg != nil && pkg.TypesInfo != nil {
							if obj := pkg.TypesInfo.Defs[ident]; obj != nil {
								outerVarTypes[ident.Name] = obj.Type()
							}
						}
					}
				}
			}
		}
		// Track assigned variables
		for _, lhs := range s.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				(*assignedVars)[ident.Name] = true
			}
		}
		// Check RHS for closures
		for _, rhs := range s.Rhs {
			collectIdents(rhs, inClosure)
		}
	case *ast.ExprStmt:
		collectIdents(s.X, inClosure)
	case *ast.ReturnStmt:
		for _, result := range s.Results {
			collectIdents(result, inClosure)
		}
	case *ast.IfStmt:
		if s.Init != nil {
			walkStmtForClosureAnalysis(s.Init, inClosure, outerVars, outerVarTypes, assignedVars, capturedVars, collectIdents, pkg)
		}
		collectIdents(s.Cond, inClosure)
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				walkStmtForClosureAnalysis(bodyStmt, inClosure, outerVars, outerVarTypes, assignedVars, capturedVars, collectIdents, pkg)
			}
		}
		if s.Else != nil {
			walkStmtForClosureAnalysis(s.Else, inClosure, outerVars, outerVarTypes, assignedVars, capturedVars, collectIdents, pkg)
		}
	case *ast.ForStmt:
		// For loop init variables are scoped to the for loop, NOT outer vars
		// So we only collect assignments and identifiers, but don't add to outerVars
		if s.Init != nil {
			walkForLoopInitForClosureAnalysis(s.Init, inClosure, assignedVars, capturedVars, collectIdents)
		}
		collectIdents(s.Cond, inClosure)
		if s.Post != nil {
			walkForLoopInitForClosureAnalysis(s.Post, inClosure, assignedVars, capturedVars, collectIdents)
		}
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				walkStmtForClosureAnalysis(bodyStmt, inClosure, outerVars, outerVarTypes, assignedVars, capturedVars, collectIdents, pkg)
			}
		}
	case *ast.RangeStmt:
		// Range loop variables (Key, Value) are scoped to the range loop, NOT outer vars
		collectIdents(s.X, inClosure)
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				walkStmtForClosureAnalysis(bodyStmt, inClosure, outerVars, outerVarTypes, assignedVars, capturedVars, collectIdents, pkg)
			}
		}
	case *ast.BlockStmt:
		for _, bodyStmt := range s.List {
			walkStmtForClosureAnalysis(bodyStmt, inClosure, outerVars, outerVarTypes, assignedVars, capturedVars, collectIdents, pkg)
		}
	case *ast.IncDecStmt:
		if ident, ok := s.X.(*ast.Ident); ok {
			(*assignedVars)[ident.Name] = true
		}
		collectIdents(s.X, inClosure)
	}
}

// walkForLoopInitForClosureAnalysis walks a for loop init/post statement
// to find assignments and closures, but does NOT add to outerVars
// because for loop init variables are scoped to the for loop
func walkForLoopInitForClosureAnalysis(stmt ast.Stmt, inClosure bool, assignedVars *map[string]bool, capturedVars *map[string]bool, collectIdents func(ast.Expr, bool)) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		// Track assigned variables (for both := and =)
		for _, lhs := range s.Lhs {
			if ident, ok := lhs.(*ast.Ident); ok {
				(*assignedVars)[ident.Name] = true
			}
		}
		// Check RHS for closures
		for _, rhs := range s.Rhs {
			collectIdents(rhs, inClosure)
		}
	case *ast.IncDecStmt:
		if ident, ok := s.X.(*ast.Ident); ok {
			(*assignedVars)[ident.Name] = true
		}
		collectIdents(s.X, inClosure)
	case *ast.ExprStmt:
		collectIdents(s.X, inClosure)
	}
}

// javaExprToString converts an ast.Expr to its Java string representation.
// This handles Java-specific transformations like slice expressions -> subList.
func javaExprToString(expr ast.Expr) string {
	return javaExprToStringWithClosures(expr, nil, nil)
}

// javaExprToStringWithClosures converts an ast.Expr to its Java string representation
// with support for closure-captured mutable variables.
func javaExprToStringWithClosures(expr ast.Expr, closureCapturedMutVars map[string]bool, closureCapturedVarType map[string]string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		name := e.Name
		// Handle closure-captured variables - add cast and [0] suffix
		if closureCapturedMutVars != nil && closureCapturedMutVars[name] {
			capturedType := closureCapturedVarType[name]
			if capturedType != "" {
				return fmt.Sprintf("((%s)%s[0])", capturedType, name)
			}
			return fmt.Sprintf("%s[0]", name)
		}
		return name
	case *ast.SelectorExpr:
		// For selector expressions on closure-captured vars, we need to cast first
		// e.g., menuState.MenuBarH -> ((gui.MenuState)menuState[0]).MenuBarH
		return javaExprToStringWithClosures(e.X, closureCapturedMutVars, closureCapturedVarType) + "." + e.Sel.Name
	case *ast.BasicLit:
		value := e.Value
		// Handle raw strings (backticks) for Java
		if len(value) >= 2 && value[0] == '`' && value[len(value)-1] == '`' {
			inner := value[1 : len(value)-1]
			inner = strings.ReplaceAll(inner, "\\", "\\\\")
			inner = strings.ReplaceAll(inner, "\"", "\\\"")
			inner = strings.ReplaceAll(inner, "\n", "\\n")
			inner = strings.ReplaceAll(inner, "\r", "\\r")
			inner = strings.ReplaceAll(inner, "\t", "\\t")
			return "\"" + inner + "\""
		}
		return value
	case *ast.IndexExpr:
		// Array/slice index: x[i] -> x.get(i)
		return javaExprToStringWithClosures(e.X, closureCapturedMutVars, closureCapturedVarType) + ".get(" + javaExprToStringWithClosures(e.Index, closureCapturedMutVars, closureCapturedVarType) + ")"
	case *ast.SliceExpr:
		// Slice expression: x[low:high] -> new ArrayList<>(x.subList(low, high))
		xStr := javaExprToStringWithClosures(e.X, closureCapturedMutVars, closureCapturedVarType)
		lowStr := "0"
		if e.Low != nil {
			lowStr = javaExprToStringWithClosures(e.Low, closureCapturedMutVars, closureCapturedVarType)
		}
		highStr := xStr + ".size()"
		if e.High != nil {
			highStr = javaExprToStringWithClosures(e.High, closureCapturedMutVars, closureCapturedVarType)
		}
		return fmt.Sprintf("new ArrayList<>(%s.subList(%s, %s))", xStr, lowStr, highStr)
	case *ast.CallExpr:
		// Check for type conversion: float64(x) -> (double)(x)
		if ident, ok := e.Fun.(*ast.Ident); ok {
			if javaType, isType := javaTypesMap[ident.Name]; isType {
				// This is a type conversion, not a function call
				if len(e.Args) == 1 {
					return "(" + javaType + ")(" + javaExprToStringWithClosures(e.Args[0], closureCapturedMutVars, closureCapturedVarType) + ")"
				}
			}
		}
		// Regular function call: f(args) -> f(args)
		var args []string
		for _, arg := range e.Args {
			args = append(args, javaExprToStringWithClosures(arg, closureCapturedMutVars, closureCapturedVarType))
		}
		return javaExprToStringWithClosures(e.Fun, closureCapturedMutVars, closureCapturedVarType) + "(" + strings.Join(args, ", ") + ")"
	case *ast.UnaryExpr:
		return e.Op.String() + javaExprToStringWithClosures(e.X, closureCapturedMutVars, closureCapturedVarType)
	case *ast.BinaryExpr:
		return javaExprToStringWithClosures(e.X, closureCapturedMutVars, closureCapturedVarType) + " " + e.Op.String() + " " + javaExprToStringWithClosures(e.Y, closureCapturedMutVars, closureCapturedVarType)
	case *ast.ParenExpr:
		return "(" + javaExprToStringWithClosures(e.X, closureCapturedMutVars, closureCapturedVarType) + ")"
	case *ast.CompositeLit:
		// Composite literal: Type{elts} -> new Type(elts)
		typeStr := javaExprToStringWithClosures(e.Type, closureCapturedMutVars, closureCapturedVarType)
		var elts []string
		for _, elt := range e.Elts {
			elts = append(elts, javaExprToStringWithClosures(elt, closureCapturedMutVars, closureCapturedVarType))
		}
		return "new " + typeStr + "(" + strings.Join(elts, ", ") + ")"
	case *ast.KeyValueExpr:
		// Key-value in composite literal - just return value for Java constructor args
		return javaExprToStringWithClosures(e.Value, closureCapturedMutVars, closureCapturedVarType)
	case *ast.ArrayType:
		// []Type -> ArrayList<Type>
		return "ArrayList<" + javaExprToStringWithClosures(e.Elt, closureCapturedMutVars, closureCapturedVarType) + ">"
	default:
		return ""
	}
}

// declareVar marks a variable as declared in the current scope
func (je *JavaEmitter) declareVar(name string) {
	if len(je.declaredVarsStack) > 0 {
		je.declaredVarsStack[len(je.declaredVarsStack)-1][name] = true
	}
}

// nestedMapLevel represents one level of a nested map access
type nestedMapLevel struct {
	keyExpr   string
	valueType string // Java type name of the map's value
}

// collectNestedMapLevels walks a nested IndexExpr and returns:
// - rootVar: the root variable name (e.g., "m" for m["a"]["b"])
// - levels: list of key expressions from outermost to innermost
// This function is used for handling nested map assignments like m["a"]["b"] = v
func (je *JavaEmitter) collectNestedMapLevels(expr ast.Expr) (rootVar string, levels []nestedMapLevel) {
	for {
		indexExpr, ok := expr.(*ast.IndexExpr)
		if !ok {
			rootVar = exprToString(expr)
			return
		}
		// Get the value type of this map access
		var valueType string
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			tv := je.pkg.TypesInfo.Types[indexExpr.X]
			if tv.Type != nil {
				if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
					valueType = getJavaTypeName(mapType.Elem())
				}
			}
		}
		levels = append([]nestedMapLevel{{
			keyExpr:   exprToString(indexExpr.Index),
			valueType: valueType,
		}}, levels...)
		expr = indexExpr.X
	}
}

// generateNestedMapGetExpr generates a Java expression for reading a nested map
// e.g., m["a"]["b"] -> ((Integer)hashMapGet(((HashMap)hashMapGet(m, "a")), "b"))
func (je *JavaEmitter) generateNestedMapGetExpr(rootVar string, levels []nestedMapLevel) string {
	result := rootVar
	for _, level := range levels {
		boxedType := toBoxedType(level.valueType)
		result = fmt.Sprintf("((%s)hashMapGet(%s, %s))", boxedType, result, level.keyExpr)
	}
	return result
}

// sliceIndexLevel represents one level of slice indexing
type sliceIndexLevel struct {
	indexExpr string // The index expression (e.g., "0", "i")
}

// collectSliceIndices walks a chain of IndexExprs and collects all slice indices
// For expr = slice[0][1], returns rootExpr="slice", indices=["0", "1"]
// It stops when it finds a non-IndexExpr or when the type is not a slice
// If the root is a map access, rootExpr will be the Java expression for the map get
func (je *JavaEmitter) collectSliceIndices(expr ast.Expr) (rootExpr string, indices []string) {
	for {
		indexExpr, ok := expr.(*ast.IndexExpr)
		if !ok {
			rootExpr = exprToString(expr)
			return
		}
		// Check if X is a slice type
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			tv := je.pkg.TypesInfo.Types[indexExpr.X]
			if tv.Type != nil {
				if _, isSlice := tv.Type.Underlying().(*types.Slice); !isSlice {
					// Not a slice - check if it's a map access
					if mapType, isMap := tv.Type.Underlying().(*types.Map); isMap {
						// The root is a map access, generate proper Java expression
						// e.g., m[1] -> ((ArrayList<HashMap>)hashMapGet(m, 1))
						mapVar := exprToString(indexExpr.X)
						keyExpr := exprToString(indexExpr.Index)
						elemType := getJavaTypeName(mapType.Elem())
						boxedElemType := toBoxedType(elemType)
						rootExpr = fmt.Sprintf("((%s)hashMapGet(%s, %s))", boxedElemType, mapVar, keyExpr)
						return
					}
					// Not a slice or map, stop here
					rootExpr = exprToString(expr)
					return
				}
			}
		}
		// Prepend the index (we're walking from outer to inner, but we want inner to outer)
		indices = append([]string{exprToString(indexExpr.Index)}, indices...)
		expr = indexExpr.X
	}
}

// generateSliceGetChain generates the Java expression for getting through multiple slice levels
// e.g., slice, ["0", "1"] -> slice.get(0).get(1)
func (je *JavaEmitter) generateSliceGetChain(rootVar string, indices []string) string {
	result := rootVar
	for _, idx := range indices {
		result = fmt.Sprintf("%s.get(%s)", result, idx)
	}
	return result
}

func (je *JavaEmitter) emitAsString(s string, indent int) string {
	if indent > 0 {
		return strings.Repeat("    ", indent) + s
	}
	return s
}

func (je *JavaEmitter) getTokenType(content string) TokenType {
	switch content {
	case "{":
		return LeftBrace
	case "}":
		return RightBrace
	case "(":
		return LeftParen
	case ")":
		return RightParen
	case "[":
		return LeftBracket
	case "]":
		return RightBracket
	case ";":
		return Semicolon
	case ",":
		return Comma
	case "+", "-", "*", "/", "%", "=", "==", "!=", "<", ">", "<=", ">=", "&&", "||", "!":
		return BinaryOperator
	default:
		return Identifier
	}
}

func (je *JavaEmitter) emitToken(content string, tokenType TokenType, indent int) {
	je.emitToPackage(content)
}

// emitToPackage writes output to the appropriate destination:
// - When capturing struct literal elements: writes to the capture buffer of nearest capturing state
// - For non-main packages: writes directly to the package's separate file
// - For main package: writes to the main buffer via gir.emitToFileBuffer
func (je *JavaEmitter) emitToPackage(content string) {
	// If capturing struct literal elements, find nearest capturing state and redirect to its buffer
	// We search from the top of the stack downward to find the innermost capturing state
	for i := len(je.structLitStack) - 1; i >= 0; i-- {
		if je.structLitStack[i].capturing {
			je.structLitStack[i].buffer.WriteString(content)
			return
		}
	}
	if je.currentPkgName != "main" && je.currentPkgName != "" {
		if pkgFile, exists := je.packageFiles[je.currentPkgName]; exists {
			pkgFile.WriteString(content)
			return
		}
	}
	je.gir.emitToFileBuffer(content, EmptyVisitMethod)
}

func (je *JavaEmitter) getMapKeyTypeConst(mapType *ast.MapType) int {
	if je.pkg == nil || je.pkg.TypesInfo == nil {
		return 1 // Default to string
	}
	tv := je.pkg.TypesInfo.Types[mapType.Key]
	if tv.Type == nil {
		return 1 // Default to string
	}
	switch tv.Type.Underlying().String() {
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
	default:
		return 1 // Default to string
	}
}


// isJavaOuterClassPackage checks if a package name is an outer class that needs prefixing.
// Uses the global namespaces map from base_pass.go, excluding "main" and "hmap".
// "hmap" is a utility package that's inlined in the main file, not a separate outer class.
func isJavaOuterClassPackage(pkgName string) bool {
	if pkgName == "main" || pkgName == "hmap" {
		return false
	}
	_, exists := namespaces[pkgName]
	return exists
}

func getJavaTypeName(t types.Type) string {
	// First, check if this is a named type
	if named, ok := t.(*types.Named); ok {
		// Check if the underlying type is a basic type (type alias like `type ExprKind int`)
		// In that case, use the underlying type since Java doesn't have type aliases
		if basic, ok := named.Underlying().(*types.Basic); ok {
			// Use the underlying basic type
			switch basic.Kind() {
			case types.Int, types.Int32:
				return "int"
			case types.Int8:
				return "byte"
			case types.Int16:
				return "short"
			case types.Int64:
				return "long"
			case types.Uint8:
				return "byte"
			case types.Uint, types.Uint16, types.Uint32:
				return "int"
			case types.Uint64:
				return "long"
			case types.Float32:
				return "float"
			case types.Float64:
				return "double"
			case types.Bool:
				return "boolean"
			case types.String:
				return "String"
			}
		}
		// Check if the underlying type is a slice (type alias like `type AST []Statement`)
		// In that case, resolve to ArrayList<elemType>
		if sliceType, ok := named.Underlying().(*types.Slice); ok {
			elemType := getJavaTypeName(sliceType.Elem())
			return fmt.Sprintf("ArrayList<%s>", toBoxedType(elemType))
		}
		// Check if the underlying type is a map (type alias like `type Table map[K]V`)
		if mapType, ok := named.Underlying().(*types.Map); ok {
			keyType := getJavaTypeName(mapType.Key())
			valType := getJavaTypeName(mapType.Elem())
			return fmt.Sprintf("HashMap<%s, %s>", toBoxedType(keyType), toBoxedType(valType))
		}
		// Otherwise, it's a struct or other named type - use the name
		// If the type's package is an outer class package, prefix with package name
		typeName := named.Obj().Name()
		if pkg := named.Obj().Pkg(); pkg != nil {
			pkgName := pkg.Name()
			if isJavaOuterClassPackage(pkgName) {
				return pkgName + "." + typeName
			}
		}
		return typeName
	}

	switch ut := t.Underlying().(type) {
	case *types.Basic:
		switch ut.Kind() {
		case types.Int, types.Int32, types.UntypedInt, types.UntypedRune:
			return "int"
		case types.Int8:
			return "byte"
		case types.Int16:
			return "short"
		case types.Int64:
			return "long"
		case types.Uint8:
			return "byte"
		case types.Uint, types.Uint16, types.Uint32:
			return "int"
		case types.Uint64:
			return "long"
		case types.Float32:
			return "float"
		case types.Float64, types.UntypedFloat:
			return "double"
		case types.Bool, types.UntypedBool:
			return "boolean"
		case types.String, types.UntypedString:
			return "String"
		default:
			return "Object"
		}
	case *types.Slice:
		elemType := getJavaTypeName(ut.Elem())
		return fmt.Sprintf("ArrayList<%s>", toBoxedType(elemType))
	case *types.Map:
		return "HashMap"
	case *types.Pointer:
		return getJavaTypeName(ut.Elem())
	case *types.Signature:
		// Function types -> BiFunction, Function, etc.
		numParams := ut.Params().Len()
		hasReturn := ut.Results().Len() > 0

		if !hasReturn {
			switch numParams {
			case 0:
				return "Runnable"
			case 1:
				p1 := getJavaTypeName(ut.Params().At(0).Type())
				return fmt.Sprintf("Consumer<%s>", toBoxedType(p1))
			case 2:
				p1 := getJavaTypeName(ut.Params().At(0).Type())
				p2 := getJavaTypeName(ut.Params().At(1).Type())
				return fmt.Sprintf("BiConsumer<%s, %s>", toBoxedType(p1), toBoxedType(p2))
			}
		} else {
			returnType := getJavaTypeName(ut.Results().At(0).Type())
			switch numParams {
			case 0:
				return fmt.Sprintf("Supplier<%s>", toBoxedType(returnType))
			case 1:
				p1 := getJavaTypeName(ut.Params().At(0).Type())
				return fmt.Sprintf("Function<%s, %s>", toBoxedType(p1), toBoxedType(returnType))
			case 2:
				p1 := getJavaTypeName(ut.Params().At(0).Type())
				p2 := getJavaTypeName(ut.Params().At(1).Type())
				return fmt.Sprintf("BiFunction<%s, %s, %s>", toBoxedType(p1), toBoxedType(p2), toBoxedType(returnType))
			}
		}
		return "Object"
	default:
		return "Object"
	}
}

func toBoxedType(t string) string {
	if boxed, ok := javaBoxedTypes[t]; ok {
		return boxed
	}
	return t
}

// sanitizeJavaIdentifier converts a string to a valid Java identifier
// by replacing invalid characters (like hyphens) with underscores
// and removing the .java extension if present
func sanitizeJavaIdentifier(name string) string {
	// Remove .java extension if present
	if strings.HasSuffix(name, ".java") {
		name = strings.TrimSuffix(name, ".java")
	}
	return strings.ReplaceAll(name, "-", "_")
}

func getJavaDefaultValue(javaType string) string {
	switch javaType {
	case "int":
		return "0"
	case "long":
		return "0L"
	case "double":
		return "0.0"
	case "float":
		return "0.0f"
	case "boolean":
		return "false"
	case "char":
		return "'\\0'"
	case "byte":
		return "(byte)0"
	case "short":
		return "(short)0"
	case "String":
		return "\"\""
	default:
		return "null"
	}
}

// getJavaDefaultValueForStruct returns the default value for a field type,
// initializing struct types with new instances (to match Go value semantics)
func getJavaDefaultValueForStruct(javaType string) string {
	// Primitives and String
	switch javaType {
	case "int":
		return "0"
	case "long":
		return "0L"
	case "double":
		return "0.0"
	case "float":
		return "0.0f"
	case "boolean":
		return "false"
	case "char":
		return "'\\0'"
	case "byte":
		return "(byte)0"
	case "short":
		return "(short)0"
	case "String":
		return "\"\""
	}
	// Collections, functional interfaces, and built-in reference types should be null
	if strings.HasPrefix(javaType, "ArrayList<") ||
		strings.HasPrefix(javaType, "HashMap<") ||
		isJavaFunctionalInterface(javaType) ||
		isJavaBuiltinReferenceType(javaType) {
		return "null"
	}
	// Other types are assumed to be structs - initialize them
	return fmt.Sprintf("new %s()", javaType)
}

// isJavaPrimitiveType returns true if the type is a Java primitive type
func isJavaPrimitiveType(javaType string) bool {
	switch javaType {
	case "int", "long", "double", "float", "boolean", "char", "byte", "short", "String":
		return true
	default:
		return false
	}
}

// isJavaBuiltinReferenceType returns true if the type is a Java built-in reference type
// that should not be treated as a custom struct (no copy constructor available)
func isJavaBuiltinReferenceType(javaType string) bool {
	switch javaType {
	case "Object", "Integer", "Long", "Double", "Float", "Boolean", "Character", "Byte", "Short":
		return true
	default:
		return false
	}
}

// isJavaFunctionalInterface returns true if the type is a Java functional interface
// that should be copied by reference (not by calling a copy constructor)
func isJavaFunctionalInterface(javaType string) bool {
	// Check for common functional interface types from java.util.function
	if strings.HasPrefix(javaType, "BiFunction<") ||
		strings.HasPrefix(javaType, "Function<") ||
		strings.HasPrefix(javaType, "Consumer<") ||
		strings.HasPrefix(javaType, "BiConsumer<") ||
		strings.HasPrefix(javaType, "Supplier<") ||
		strings.HasPrefix(javaType, "Predicate<") ||
		strings.HasPrefix(javaType, "BiPredicate<") ||
		strings.HasPrefix(javaType, "Runnable") {
		return true
	}
	return false
}

// needsByteCast returns true if a value needs to be cast to byte/short
// This is needed because Java integer literals are 'int' by default
func needsByteCast(fieldType, value string) bool {
	if fieldType != "byte" && fieldType != "short" {
		return false
	}
	// Skip if already cast
	if strings.HasPrefix(value, "(byte)") || strings.HasPrefix(value, "(short)") {
		return false
	}
	// Check for numeric literals (positive or negative)
	trimmed := strings.TrimSpace(value)
	if len(trimmed) == 0 {
		return false
	}
	// Skip if it's null
	if trimmed == "null" {
		return false
	}
	// Check if it's a number literal
	firstChar := trimmed[0]
	if firstChar >= '0' && firstChar <= '9' {
		return true
	}
	// Check for negative number
	if firstChar == '-' && len(trimmed) > 1 && trimmed[1] >= '0' && trimmed[1] <= '9' {
		return true
	}
	// Check if it looks like a constant identifier (starts with letter/underscore)
	// Constants like TokenTypeSemicolon or ast.PgOrderAsc need casting
	if (firstChar >= 'A' && firstChar <= 'Z') || (firstChar >= 'a' && firstChar <= 'z') || firstChar == '_' {
		// Not a method call (no parens) - allow dots for package-qualified constants
		if !strings.Contains(trimmed, "(") {
			return true
		}
	}
	return false
}

func getJavaKeyCast(keyType types.Type) (string, string) {
	switch keyType.Underlying().String() {
	case "int64":
		return "(long)(", ")"
	case "uint64":
		return "(long)(", ")"
	default:
		// No cast needed for other types - they can be boxed directly
		return "", ""
	}
}

// getJavaTypeFromExpr converts an AST expression to a Java type string (fallback when TypesInfo unavailable)
func (je *JavaEmitter) getJavaTypeFromExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		// Check type alias map first
		if javaType, ok := je.typeAliasMap[e.Name]; ok {
			return javaType
		}
		// Check standard Go to Java type mapping
		if javaType, ok := javaTypesMap[e.Name]; ok {
			return javaType
		}
		// Return as-is (for struct names, etc.)
		return e.Name
	case *ast.ArrayType:
		elemType := je.getJavaTypeFromExpr(e.Elt)
		return fmt.Sprintf("ArrayList<%s>", toBoxedType(elemType))
	case *ast.MapType:
		return "HashMap"
	case *ast.StarExpr:
		return je.getJavaTypeFromExpr(e.X)
	case *ast.SelectorExpr:
		// For package.Type, just use the type name
		return e.Sel.Name
	case *ast.FuncType:
		return je.getFunctionalInterfaceType(e)
	default:
		return "Object"
	}
}

// getFunctionalInterfaceType maps a Go function type to a Java functional interface
func (je *JavaEmitter) getFunctionalInterfaceType(funcType *ast.FuncType) string {
	numParams := 0
	if funcType.Params != nil {
		for _, field := range funcType.Params.List {
			if len(field.Names) == 0 {
				numParams++
			} else {
				numParams += len(field.Names)
			}
		}
	}

	hasReturn := funcType.Results != nil && len(funcType.Results.List) > 0

	// Map to Java functional interfaces
	if !hasReturn {
		switch numParams {
		case 0:
			return "Runnable"
		case 1:
			paramType := je.getFuncParamType(funcType, 0)
			return fmt.Sprintf("Consumer<%s>", toBoxedType(paramType))
		case 2:
			p1 := je.getFuncParamType(funcType, 0)
			p2 := je.getFuncParamType(funcType, 1)
			return fmt.Sprintf("BiConsumer<%s, %s>", toBoxedType(p1), toBoxedType(p2))
		default:
			return "Object" // No standard interface for 3+ params
		}
	} else {
		returnType := je.getFuncReturnType(funcType)
		switch numParams {
		case 0:
			return fmt.Sprintf("Supplier<%s>", toBoxedType(returnType))
		case 1:
			paramType := je.getFuncParamType(funcType, 0)
			return fmt.Sprintf("Function<%s, %s>", toBoxedType(paramType), toBoxedType(returnType))
		case 2:
			p1 := je.getFuncParamType(funcType, 0)
			p2 := je.getFuncParamType(funcType, 1)
			return fmt.Sprintf("BiFunction<%s, %s, %s>", toBoxedType(p1), toBoxedType(p2), toBoxedType(returnType))
		default:
			return "Object"
		}
	}
}

// getFuncParamType gets the Java type of a function parameter at the given index
func (je *JavaEmitter) getFuncParamType(funcType *ast.FuncType, index int) string {
	if funcType.Params == nil {
		return "Object"
	}
	idx := 0
	for _, field := range funcType.Params.List {
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		if index < idx+count {
			return je.getJavaTypeFromExpr(field.Type)
		}
		idx += count
	}
	return "Object"
}

// getFuncReturnType gets the Java type of a function's return value
func (je *JavaEmitter) getFuncReturnType(funcType *ast.FuncType) string {
	if funcType.Results == nil || len(funcType.Results.List) == 0 {
		return "Void"
	}
	// For simplicity, just get the first return type
	return je.getJavaTypeFromExpr(funcType.Results.List[0].Type)
}

// getFunctionalInterfaceMethod returns the method name to call on a functional interface
// based on the function signature
func (je *JavaEmitter) getFunctionalInterfaceMethod(sig *types.Signature) string {
	numParams := sig.Params().Len()
	hasReturn := sig.Results().Len() > 0

	if !hasReturn {
		switch numParams {
		case 0:
			return "run" // Runnable
		case 1, 2:
			return "accept" // Consumer, BiConsumer
		}
	} else {
		switch numParams {
		case 0:
			return "get" // Supplier
		case 1, 2:
			return "apply" // Function, BiFunction
		}
	}
	return ""
}

func (je *JavaEmitter) SetFile(file *os.File) {
	je.file = file
}

func (je *JavaEmitter) GetFile() *os.File {
	return je.file
}

func (je *JavaEmitter) executeIfNotForwardDecls(fn func()) {
	if je.forwardDecls {
		return
	}
	fn()
}

func (je *JavaEmitter) lowerToBuiltins(name string) string {
	switch name {
	case "println":
		return "System.out.println"
	case "print":
		return "System.out.print"
	case "len":
		return "SliceBuiltins.Length"
	case "append":
		return "SliceBuiltins.Append"
	case "panic":
		return "GoanyPanic.goPanic"
	case "true":
		return "true"
	case "false":
		return "false"
	// Handle fmt package functions
	case "Printf":
		return "Formatter.Printf"
	case "Sprintf":
		return "Formatter.Sprintf"
	case "Println":
		return "System.out.println"
	case "Print":
		return "System.out.print"
	default:
		return name
	}
}

// writeJavaBoilerplate writes imports and panic runtime to the main output file
func (je *JavaEmitter) writeJavaBoilerplate() {
	imports := `import java.util.*;
import java.util.function.*;

`
	je.file.WriteString(imports)
	je.file.WriteString("// GoAny panic runtime\n")
	je.file.WriteString(goanyrt.PanicJavaSource)
	je.file.WriteString("\n")
}

// Main visitor methods

func (je *JavaEmitter) PreVisitProgram(indent int) {
	je.typeAliasMap = make(map[string]string)
	je.multiReturnFuncs = make(map[string][]string)
	je.resultClassDefs = nil
	je.resultClassRegistry = make(map[string]string)
	je.mapVarValueTypes = make(map[string]string)
	je.declaredVarsStack = []map[string]bool{make(map[string]bool)}
	je.funcParamsStack = []map[string]bool{make(map[string]bool)}
	je.packageFiles = make(map[string]*os.File)
	// Runtime packages are added to namespaces so they get proper type prefixing
	for pkgName := range je.RuntimePackages {
		namespaces[pkgName] = struct{}{}
	}

	// Sanitize output name for Java (replace hyphens with underscores, strip .java extension)
	je.OutputName = sanitizeJavaIdentifier(je.OutputName)
	// Rebuild the output path with sanitized name
	je.Output = filepath.Join(je.OutputDir, je.OutputName+".java")

	outputFile := je.Output
	je.shouldGenerate = true
	var err error
	je.file, err = os.Create(outputFile)
	je.SetFile(je.file)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return
	}

	// Write imports and panic runtime
	je.writeJavaBoilerplate()

	// Builtin helper classes
	builtin := `class SliceBuiltins {
    public static <T> ArrayList<T> Append(ArrayList<T> list, T element) {
        if (list == null) list = new ArrayList<T>();
        list.add(element);
        return list;
    }

    @SafeVarargs
    public static <T> ArrayList<T> Append(ArrayList<T> list, T... elements) {
        if (list == null) list = new ArrayList<T>();
        for (T e : elements) list.add(e);
        return list;
    }

    public static <T> ArrayList<T> Append(ArrayList<T> list, ArrayList<T> elements) {
        if (list == null) list = new ArrayList<T>();
        if (elements != null) list.addAll(elements);
        return list;
    }

    public static <T> int Length(List<T> list) {
        return list == null ? 0 : list.size();
    }

    public static int Length(String s) {
        return s == null ? 0 : s.length();
    }

    public static <T> int Length(T[] arr) {
        return arr == null ? 0 : arr.length;
    }

    // Create a slice with n elements initialized to null (like Go's make([]T, n))
    @SuppressWarnings("unchecked")
    public static <T> ArrayList<T> MakeSlice(int size) {
        ArrayList<T> list = new ArrayList<T>(size);
        for (int i = 0; i < size; i++) {
            list.add(null);
        }
        return list;
    }

    // Create a boolean slice with n elements initialized to false
    public static ArrayList<Boolean> MakeBoolSlice(int size) {
        ArrayList<Boolean> list = new ArrayList<Boolean>(size);
        for (int i = 0; i < size; i++) {
            list.add(false);
        }
        return list;
    }
}

class Formatter {
    public static void Printf(String format, Object... args) {
        int argIndex = 0;
        StringBuilder converted = new StringBuilder();
        List<Object> formattedArgs = new ArrayList<>();

        for (int i = 0; i < format.length(); i++) {
            if (format.charAt(i) == '%' && i + 1 < format.length()) {
                char next = format.charAt(i + 1);
                switch (next) {
                    case 'd':
                    case 's':
                    case 'f':
                        converted.append("%").append(next);
                        formattedArgs.add(args[argIndex]);
                        argIndex++;
                        i++;
                        continue;
                    case 'c':
                        converted.append("%c");
                        Object arg = args[argIndex];
                        if (arg instanceof Byte) {
                            formattedArgs.add((char)(byte)(Byte)arg);
                        } else if (arg instanceof Integer) {
                            formattedArgs.add((char)(int)(Integer)arg);
                        } else if (arg instanceof Character) {
                            formattedArgs.add(arg);
                        } else {
                            throw new IllegalArgumentException("Argument for %c must be char, int, or byte");
                        }
                        argIndex++;
                        i++;
                        continue;
                }
            }
            converted.append(format.charAt(i));
        }

        String result = converted.toString()
            .replace("\\n", "\n")
            .replace("\\t", "\t");

        System.out.printf(result, formattedArgs.toArray());
    }

    public static String Sprintf(String format, Object... args) {
        int argIndex = 0;
        StringBuilder converted = new StringBuilder();
        List<Object> formattedArgs = new ArrayList<>();

        for (int i = 0; i < format.length(); i++) {
            if (format.charAt(i) == '%' && i + 1 < format.length()) {
                char next = format.charAt(i + 1);
                switch (next) {
                    case 'd':
                    case 's':
                    case 'f':
                        converted.append("%").append(next);
                        formattedArgs.add(args[argIndex]);
                        argIndex++;
                        i++;
                        continue;
                    case 'c':
                        converted.append("%c");
                        Object arg = args[argIndex];
                        if (arg instanceof Byte) {
                            formattedArgs.add((char)(byte)(Byte)arg);
                        } else if (arg instanceof Integer) {
                            formattedArgs.add((char)(int)(Integer)arg);
                        } else if (arg instanceof Character) {
                            formattedArgs.add(arg);
                        } else {
                            throw new IllegalArgumentException("Argument for %c must be char, int, or byte");
                        }
                        argIndex++;
                        i++;
                        continue;
                }
            }
            converted.append(format.charAt(i));
        }

        String result = converted.toString()
            .replace("\\n", "\n")
            .replace("\\t", "\t");

        return String.format(result, formattedArgs.toArray());
    }
}

`
	str := je.emitAsString(builtin, indent)
	je.emitToPackage(str)

	je.insideForPostCond = false
}

func (je *JavaEmitter) PostVisitProgram(indent int) {
	// Close the main class
	if je.apiClassOpened {
		je.emitToPackage("}\n")
	}

	emitTokensToFile(je.file, je.gir.tokenSlice)
	je.file.Close()

	// Generate build files if link-runtime is enabled
	if je.LinkRuntime != "" {
		if err := je.GeneratePomXml(); err != nil {
			log.Printf("Warning: %v", err)
		}
		if err := je.CopyRuntimePackages(); err != nil {
			log.Printf("Warning: %v", err)
		}
	}
}

func (je *JavaEmitter) PreVisitFuncDecl(node *ast.FuncDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Analyze function body for closure-captured mutable variables
		// This must be done before any code is emitted
		if node.Body != nil {
			je.analyzeClosureCapturedMutVars(node.Body)
		}
	})
}

func (je *JavaEmitter) PostVisitFuncDecl(node *ast.FuncDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Clear closure analysis state after function is processed
		je.closureCapturedMutVars = nil
		je.closureCapturedVarType = nil

		str := je.emitAsString("\n\n", 0)
		je.emitToPackage(str)

		// Pop the function parameter scope
		if len(je.funcParamsStack) > 0 {
			je.funcParamsStack = je.funcParamsStack[:len(je.funcParamsStack)-1]
		}
	})
}

func (je *JavaEmitter) PreVisitFuncDeclSignatures(indent int) {
	je.forwardDecls = true
}

func (je *JavaEmitter) PostVisitFuncDeclSignatures(indent int) {
	je.forwardDecls = false
}

func (je *JavaEmitter) PreVisitFuncDeclName(node *ast.Ident, indent int) {
	je.executeIfNotForwardDecls(func() {
		var str string
		if node.Name == "main" {
			str = je.emitAsString("main", 0)
		} else {
			str = je.emitAsString(node.Name, 0)
		}
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitBlockStmt(node *ast.BlockStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Push new scope for variable tracking
		je.declaredVarsStack = append(je.declaredVarsStack, make(map[string]bool))

		je.emitToken("{", LeftBrace, 1)
		str := je.emitAsString("\n", 1)
		je.emitToPackage(str)

		if je.pendingRangeValueDecl {
			valueDecl := je.emitAsString(fmt.Sprintf("var %s = %s.get(%s);\n",
				je.pendingValueName, je.pendingCollectionExpr, je.pendingKeyName), indent+2)
			je.emitToPackage(valueDecl)
			// Track this variable in current scope
			if len(je.declaredVarsStack) > 0 {
				je.declaredVarsStack[len(je.declaredVarsStack)-1][je.pendingValueName] = true
			}
			je.pendingRangeValueDecl = false
			je.pendingValueName = ""
			je.pendingCollectionExpr = ""
			je.pendingKeyName = ""
		}
	})
}

func (je *JavaEmitter) PostVisitBlockStmt(node *ast.BlockStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("}", indent)
		je.emitToPackage(str)
		// Pop scope for variable tracking
		if len(je.declaredVarsStack) > 0 {
			je.declaredVarsStack = je.declaredVarsStack[:len(je.declaredVarsStack)-1]
		}
		// Note: Don't reset isArray here - it's reset in PostVisitCompositeLitElts
	})
}

func (je *JavaEmitter) PreVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Push a new function parameter scope for lambda shadowing detection
		je.funcParamsStack = append(je.funcParamsStack, make(map[string]bool))

		je.emitToken("(", LeftParen, 0)
		// Add String[] args for main function
		if node.Name.Name == "main" {
			je.emitToPackage("String[] args")
		}
	})
}

func (je *JavaEmitter) PostVisitFuncDeclSignatureTypeParams(node *ast.FuncDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.emitToken(")", RightParen, 0)
	})
}

// isTypeConversion tracks if current call expression is a type conversion
var javaIsTypeConversion bool
var javaSuppressTypeCastIdent bool

func (je *JavaEmitter) PreVisitIdent(e *ast.Ident, indent int) {
	je.executeIfNotForwardDecls(func() {
		if !je.shouldGenerate {
			return
		}
		// Suppress type emission for closure-captured variables (we use Object[] instead)
		if je.suppressClosureTypeEmit {
			return
		}
		if je.suppressTypeAliasEmit {
			return
		}
		if je.suppressRangeEmit {
			return
		}
		if javaSuppressTypeCastIdent {
			return
		}
		if je.suppressTypeAliasSelectorX {
			// When suppressTypeAliasSelectorX is set, we already emitted the resolved type
			// in PreVisitDeclStmtValueSpecType, so suppress all identifiers in this context
			return
		}
		if je.captureRangeExpr {
			je.rangeCollectionExpr += e.Name
			return
		}
		// Suppress package identifier (e.g., fmt, types) - we emit just the type/function name
		if je.suppressFmtPackage {
			return
		}
		// Suppress struct field key in composite literal (Java uses positional args)
		if je.suppressKeyValueKey {
			return
		}
		// Suppress identifier when we're in a function variable call (already emitted with .accept etc.)
		if je.isFuncVarCall {
			return
		}
		// Suppress individual return types when function has multiple return values
		if je.suppressMultiReturnTypes {
			return
		}
		// Suppress type identifiers in make([]Type, size) - handled separately
		if je.isSliceMakeCall && je.inTypeContext {
			return
		}
		// Suppress type identifiers in lambda parameters (Java uses type inference)
		if je.inLambdaParams && je.inTypeContext {
			return
		}
		// Suppress map key/value type identifiers during make(map[K]V) call
		// But don't suppress "make" itself as it needs to be replaced
		if je.isMapMakeCall && je.suppressMapEmit && e.Name != "make" {
			return
		}
		// Also suppress when inside map type declarations (inTypeContext and suppressMapEmit)
		// But don't suppress composite literal types - we need the type name for "new Type(...)"
		if je.inTypeContext && je.suppressMapEmit && !je.inCompositeLitType {
			return
		}
		// Suppress LHS identifiers during map assignment (handled in PreVisitAssignStmtRhs)
		if je.suppressMapAssignLhs {
			return
		}
		// Suppress map variable during map index read (handled in PostVisitIndexExprX)
		if je.suppressMapIndexX {
			return
		}
		// Suppress all identifiers during map comma-ok (handled in PostVisitAssignStmt)
		if je.isMapCommaOk {
			return
		}
		// Suppress all identifiers during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		// Suppress all identifiers during type assert comma-ok (handled in PostVisitAssignStmt)
		if je.isTypeAssertCommaOk {
			return
		}
		// Suppress for unsupported indexed function call (handled in PostVisitCallExpr)
		if je.isUnsupportedIndexedCall {
			return
		}

		var str string
		name := e.Name

		// Check if this identifier is a function being passed as an argument (not being called)
		// In Java, we need to emit method references (ClassName::methodName) instead of just the name
		// Only convert if: we're inside call arguments AND not in the Fun part of a nested call
		if je.inCallExprArgs && je.inCallExprFunDepth == 0 && je.pkg != nil && je.pkg.TypesInfo != nil {
			if obj := je.pkg.TypesInfo.Uses[e]; obj != nil {
				if _, isFunc := obj.Type().(*types.Signature); isFunc {
					// This is a function reference - emit as method reference
					className := je.OutputName
					// Sanitize class name (replace hyphens with underscores)
					className = strings.ReplaceAll(className, "-", "_")
					str = je.emitAsString(className+"::"+name, indent)
					je.emitToken(str, Identifier, 0)
					return
				}
			}
		}

		// Map operation identifier replacements
		if je.isMapMakeCall && name == "make" {
			name = "newHashMap"
		} else if je.isSliceMakeCall && name == "make" {
			name = "" // Suppress "make" - we emit SliceBuiltins.MakeSlice in PreVisitCallExprArg
		} else if je.isDeleteCall && name == "delete" {
			name = je.deleteMapVarName + " = hashMapDelete"
		} else if je.isMapLenCall && name == "len" {
			name = "hashMapLen"
		} else {
			name = je.lowerToBuiltins(name)
		}

		if name == "nil" {
			str = je.emitAsString("null", indent)
		} else {
			if n, ok := javaTypesMap[name]; ok {
				// Use boxed types when inside ArrayList<...>
				if je.inArrayTypeContext {
					n = toBoxedType(n)
				}
				str = je.emitAsString(n, indent)
			} else if underlyingType, ok := je.typeAliasMap[name]; ok {
				// Use boxed types when inside ArrayList<...>
				if je.inArrayTypeContext {
					underlyingType = toBoxedType(underlyingType)
				}
				str = je.emitAsString(underlyingType, indent)
			} else {
				// Check if this identifier is a type from an outer class package
				// If so, prefix with the package name (but not if we're already in a package-qualified expression)
				prefixedName := name
				if je.inTypeContext && !je.inPackageQualifiedExpr && je.pkg != nil && je.pkg.TypesInfo != nil {
					if obj := je.pkg.TypesInfo.Uses[e]; obj != nil {
						if typeName, isTypeName := obj.(*types.TypeName); isTypeName {
							if pkg := typeName.Pkg(); pkg != nil {
								if isJavaOuterClassPackage(pkg.Name()) {
									prefixedName = pkg.Name() + "." + name
								}
							}
						}
					}
				}
				str = je.emitAsString(prefixedName, indent)
			}
		}

		// Add cast and [0] suffix for closure-captured mutable variables (not during declaration, not in type context)
		isCapturedAccess := false
		capturedType := ""
		if !je.inDeclName && !je.inTypeContext && je.closureCapturedMutVars != nil {
			if je.closureCapturedMutVars[e.Name] {
				isCapturedAccess = true
				capturedType = je.closureCapturedVarType[e.Name]
				// Emit cast prefix for read access and field access (even on LHS for field access)
				// Direct LHS assignment: tokens[0] = ... (no cast needed)
				// Field access on LHS: ((Type)ctx[0]).Field = ... (cast needed for field access)
				// Read access: ((ArrayList<Token>)tokens[0]) (cast needed)
				needsCast := !je.inAssignLhs || je.inSelectorX
				if capturedType != "" && needsCast {
					je.emitToken("(("+capturedType+")", LeftParen, 0)
				}
			}
		}

		je.emitToken(str, Identifier, 0)

		if isCapturedAccess {
			je.emitToken("[0]", LeftBracket, 0)
			// Emit cast suffix for read access and field access (even on LHS for field access)
			needsCast := !je.inAssignLhs || je.inSelectorX
			if capturedType != "" && needsCast {
				je.emitToken(")", RightParen, 0)
			}
		}
	})
}

func (je *JavaEmitter) PreVisitCallExpr(node *ast.CallExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		// Check for append call with struct argument that's a variable reference
		// (not a composite literal or constructor call)
		if ident, ok := node.Fun.(*ast.Ident); ok && ident.Name == "append" && len(node.Args) >= 2 {
			je.isAppendCall = true
			je.appendStructType = ""
			// Only apply copy constructor for variable references (Ident), not for
			// composite literals or other expressions that already create new structs
			if _, isIdent := node.Args[1].(*ast.Ident); isIdent {
				// Check if the second argument is a struct type (needs copy constructor)
				if je.pkg != nil && je.pkg.TypesInfo != nil {
					tv := je.pkg.TypesInfo.Types[node.Args[1]]
					if tv.Type != nil {
						if _, isStruct := tv.Type.Underlying().(*types.Struct); isStruct {
							je.appendStructType = getJavaTypeName(tv.Type)
						}
					}
				}
			}
		}
		// Map operations
		if ident, ok := node.Fun.(*ast.Ident); ok {
			if ident.Name == "make" && len(node.Args) >= 1 {
				if mapType, ok := node.Args[0].(*ast.MapType); ok {
					je.isMapMakeCall = true
					je.mapMakeKeyType = je.getMapKeyTypeConst(mapType)
					je.suppressMapEmit = true // Suppress map type key/value emission
					javaIsTypeConversion = false
					return
				}
				if _, ok := node.Args[0].(*ast.ArrayType); ok && len(node.Args) >= 2 {
					je.isSliceMakeCall = true
					je.sliceMakeElemType = "Object"
					if je.pkg != nil && je.pkg.TypesInfo != nil {
						tv := je.pkg.TypesInfo.Types[node.Args[0]]
						if tv.Type != nil {
							if sliceType, ok := tv.Type.Underlying().(*types.Slice); ok {
								je.sliceMakeElemType = getJavaTypeName(sliceType.Elem())
							}
						}
					}
					javaIsTypeConversion = false
					return
				}
			}

			if ident.Name == "delete" && len(node.Args) >= 2 {
				je.isDeleteCall = true
				je.deleteMapVarName = exprToString(node.Args[0])
				je.mapKeyCastPrefix = ""
				je.mapKeyCastSuffix = ""
				if je.pkg != nil && je.pkg.TypesInfo != nil {
					tv := je.pkg.TypesInfo.Types[node.Args[0]]
					if tv.Type != nil {
						if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
							je.mapKeyCastPrefix, je.mapKeyCastSuffix = getJavaKeyCast(mapType.Key())
						}
					}
				}
				javaIsTypeConversion = false
				return
			}

			if ident.Name == "len" && len(node.Args) >= 1 {
				if je.pkg != nil && je.pkg.TypesInfo != nil {
					tv := je.pkg.TypesInfo.Types[node.Args[0]]
					if tv.Type != nil {
						if _, ok := tv.Type.Underlying().(*types.Map); ok {
							je.isMapLenCall = true
							javaIsTypeConversion = false
							return
						}
					}
				}
			}
		}

		// Check if calling a function variable (functional interface)
		// e.g., f(10, 20) where f is a BiConsumer -> f.accept(10, 20)
		// Only applies to variables, not function declarations
		if ident, ok := node.Fun.(*ast.Ident); ok {
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				if obj := je.pkg.TypesInfo.ObjectOf(ident); obj != nil {
					// Check if this is a variable (not a function declaration)
					if _, isVar := obj.(*types.Var); isVar {
						if sig, isFunc := obj.Type().Underlying().(*types.Signature); isFunc {
							// This is a function variable call - transform to functional interface method
							methodName := je.getFunctionalInterfaceMethod(sig)
							if methodName != "" {
								je.emitToPackage(fmt.Sprintf("%s.%s(", ident.Name, methodName))
								je.isFuncVarCall = true
								return
							}
						}
					}
				}
			}
		}

		// Check if calling a struct field that is a function type (BiFunction)
		// e.g., visitor.PreVisitFrom(state, expr) -> visitor.PreVisitFrom.apply(state, expr)
		if selExpr, ok := node.Fun.(*ast.SelectorExpr); ok {
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				// Get the type of the selection
				if sel := je.pkg.TypesInfo.Selections[selExpr]; sel != nil {
					// Check if the field type is a function signature
					if sig, isFunc := sel.Type().Underlying().(*types.Signature); isFunc {
						// This is a function field call - transform to functional interface method
						methodName := je.getFunctionalInterfaceMethod(sig)
						if methodName != "" {
							// Set flag to add .apply() after selector but before args
							je.funcFieldCallMethod = methodName
						}
					}
				}
			}
		}

		// Check for unsupported indexed function call: x[0](args)
		// Java doesn't support calling functions from slice index directly
		if indexExpr, ok := node.Fun.(*ast.IndexExpr); ok {
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				tv := je.pkg.TypesInfo.Types[indexExpr.X]
				if tv.Type != nil {
					if sliceType, ok := tv.Type.Underlying().(*types.Slice); ok {
						if _, isFunc := sliceType.Elem().Underlying().(*types.Signature); isFunc {
							je.isUnsupportedIndexedCall = true
							je.unsupportedCallIndent = indent
							return
						}
					}
				}
			}
		}

		// Check for type conversion
		if ident, ok := node.Fun.(*ast.Ident); ok {
			if _, isType := javaTypesMap[ident.Name]; isType {
				javaIsTypeConversion = true
				return
			}
		}

		// Get parameter types for byte/short casting in arguments
		je.callExprParamTypes = nil
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			tv := je.pkg.TypesInfo.Types[node.Fun]
			if tv.Type != nil {
				if sig, ok := tv.Type.Underlying().(*types.Signature); ok {
					params := sig.Params()
					for i := 0; i < params.Len(); i++ {
						je.callExprParamTypes = append(je.callExprParamTypes, getJavaTypeName(params.At(i).Type()))
					}
				}
			}
		}

		javaIsTypeConversion = false
	})
}

func (je *JavaEmitter) PreVisitCallExprFun(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Mark that we're in the Fun part of a CallExpr (the function being called)
		je.inCallExprFunDepth++
		// Suppress during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		// Suppress for unsupported indexed function call
		if je.isUnsupportedIndexedCall {
			return
		}
		if javaIsTypeConversion {
			if ident, ok := node.(*ast.Ident); ok {
				typeName := ident.Name
				if mapped, ok := javaTypesMap[typeName]; ok {
					typeName = mapped
				}
				je.emitToken("(", LeftParen, 0)
				je.emitToken(typeName, Identifier, 0)
				je.emitToken(")", RightParen, 0)
				javaSuppressTypeCastIdent = true
			}
		}
	})
}

func (je *JavaEmitter) PostVisitCallExprFun(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inCallExprFunDepth--
		javaSuppressTypeCastIdent = false
	})
}

func (je *JavaEmitter) PreVisitCallExprArgs(node []ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Mark that we're inside call expression arguments (for method reference detection)
		je.inCallExprArgs = true
		// Suppress for unsupported indexed function call
		if je.isUnsupportedIndexedCall {
			return
		}
		if je.isMultiReturnAssign {
			return
		}
		if je.isMapMakeCall {
			je.emitToken(fmt.Sprintf("(%d", je.mapMakeKeyType), LeftParen, 0)
			return
		}
		if je.isSliceMakeCall {
			return
		}
		// Suppress opening paren for func var call (already emitted with f.accept()
		if je.isFuncVarCall {
			return
		}
		// For struct field function calls, emit .apply( (or .accept(, etc.)
		if je.funcFieldCallMethod != "" {
			je.emitToPackage("." + je.funcFieldCallMethod + "(")
			return
		}
		je.emitToken("(", LeftParen, 0)
	})
}

func (je *JavaEmitter) PostVisitCallExprArgs(node []ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress for unsupported indexed function call
		if je.isUnsupportedIndexedCall {
			return
		}
		if je.isMultiReturnAssign {
			return
		}
		if je.isSliceMakeCall {
			je.emitToken(")", RightParen, 0)
			je.isSliceMakeCall = false
			je.sliceMakeElemType = ""
			je.isArray = false
			return
		}
		je.emitToken(")", RightParen, 0)
		// Reset functional interface call flags
		je.isFuncVarCall = false
		je.funcFieldCallMethod = ""
		if je.isMapMakeCall {
			je.suppressMapEmit = false
		}
		je.isMapMakeCall = false
		if je.isDeleteCall {
			je.mapKeyCastPrefix = ""
			je.mapKeyCastSuffix = ""
		}
		je.isDeleteCall = false
		je.deleteMapVarName = ""
		je.isMapLenCall = false
		je.inCallExprArgs = false
	})
}

func (je *JavaEmitter) PreVisitBasicLit(e *ast.BasicLit, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress literals during map assignment LHS (handled in PreVisitAssignStmtRhs)
		if je.suppressMapAssignLhs {
			return
		}
		// Suppress literals during map comma-ok (handled in PostVisitAssignStmt)
		if je.isMapCommaOk {
			return
		}
		// Suppress literals during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		// Suppress literals during type assert comma-ok (handled in PostVisitAssignStmt)
		if je.isTypeAssertCommaOk {
			return
		}
		// Suppress for unsupported indexed function call (handled in PostVisitCallExpr)
		if je.isUnsupportedIndexedCall {
			return
		}
		var str string
		if e.Kind == token.STRING {
			value := e.Value
			if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
				value = value[1 : len(value)-1]
				str = je.emitAsString(fmt.Sprintf("\"%s\"", value), 0)
			} else if len(value) >= 2 && value[0] == '`' && value[len(value)-1] == '`' {
				// Raw string literal - Java doesn't have verbatim strings, escape as needed
				value = value[1 : len(value)-1]
				// Escape backslashes, quotes, and newlines for Java
				value = strings.ReplaceAll(value, "\\", "\\\\")
				value = strings.ReplaceAll(value, "\"", "\\\"")
				value = strings.ReplaceAll(value, "\n", "\\n")
				value = strings.ReplaceAll(value, "\r", "\\r")
				value = strings.ReplaceAll(value, "\t", "\\t")
				str = je.emitAsString(fmt.Sprintf("\"%s\"", value), 0)
			} else {
				str = je.emitAsString(fmt.Sprintf("\"%s\"", value), 0)
			}
			je.emitToken(str, StringLiteral, 0)
		} else if e.Kind == token.CHAR {
			str = je.emitAsString(e.Value, 0)
			je.emitToken(str, NumberLiteral, 0)
		} else if e.Kind == token.INT {
			// Check if integer literal exceeds Java int max (2147483647)
			value := e.Value
			// Parse the integer value to check if it needs L suffix
			if len(value) > 0 {
				// Remove any underscores (Go allows _ in numeric literals)
				cleanValue := strings.ReplaceAll(value, "_", "")
				// Check if it's a large number (simple heuristic: > 10 digits or compare)
				if len(cleanValue) >= 10 {
					// Parse as int64 to check
					if n, err := strconv.ParseInt(cleanValue, 0, 64); err == nil {
						if n > 2147483647 || n < -2147483648 {
							value = value + "L"
						}
					}
				}
			}
			str = je.emitAsString(value, 0)
			je.emitToken(str, NumberLiteral, 0)
		} else {
			str = je.emitAsString(e.Value, 0)
			je.emitToken(str, NumberLiteral, 0)
		}
	})
}

func (je *JavaEmitter) PreVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.isArray = false
		je.inTypeContext = true
		// Track if there's an explicit initialization value
		je.hasDeclValue = index < len(node.Values)
		// Check if this variable is a closure-captured mutable variable
		je.isClosureCapturedDecl = false
		je.closureCapturedDeclName = ""
		je.suppressClosureTypeEmit = false
		if index < len(node.Names) && je.closureCapturedMutVars != nil {
			varName := node.Names[index].Name
			if je.closureCapturedMutVars[varName] {
				je.isClosureCapturedDecl = true
				je.closureCapturedDeclName = varName
				// For closure-captured variables, emit Object[] instead of the actual type
				// (Java doesn't allow generic array creation like ArrayList<T>[])
				je.emitToPackage("Object[]")
				je.suppressClosureTypeEmit = true
				je.inTypeContext = false // Prevent normal type emission
			}
		}
		if len(node.Values) == 0 && node.Type != nil {
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				if typeAndValue, ok := je.pkg.TypesInfo.Types[node.Type]; ok {
					if _, isMap := typeAndValue.Type.Underlying().(*types.Map); isMap {
						if mapType, ok := node.Type.(*ast.MapType); ok {
							je.pendingMapInit = true
							je.pendingMapKeyType = je.getMapKeyTypeConst(mapType)
						}
					}
					// Check for struct types that need initialization
					if _, isStruct := typeAndValue.Type.Underlying().(*types.Struct); isStruct {
						// Use getJavaTypeName which handles package prefixing
						je.pendingStructInit = true
						je.pendingStructTypeName = getJavaTypeName(typeAndValue.Type)
					}
					// Check for slice types - initialize to empty ArrayList
					if sliceType, isSlice := typeAndValue.Type.Underlying().(*types.Slice); isSlice {
						je.pendingSliceInit = true
						je.pendingSliceElemType = toBoxedType(getJavaTypeName(sliceType.Elem()))
					}
					// Check for primitive types - initialize to default value
					if basic, isBasic := typeAndValue.Type.Underlying().(*types.Basic); isBasic {
						switch basic.Kind() {
						case types.Int, types.Int8, types.Int16, types.Int32, types.Int64,
							types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64:
							je.pendingPrimitiveInit = true
							je.pendingPrimitiveDefault = "0"
						case types.Float32, types.Float64:
							je.pendingPrimitiveInit = true
							je.pendingPrimitiveDefault = "0.0"
						case types.Bool:
							je.pendingPrimitiveInit = true
							je.pendingPrimitiveDefault = "false"
						case types.String:
							je.pendingPrimitiveInit = true
							je.pendingPrimitiveDefault = "\"\""
						}
					}
					// Check if it's a type alias from another package - resolve to underlying type
					if _, ok := node.Type.(*ast.SelectorExpr); ok {
						underlyingType := typeAndValue.Type.Underlying()
						if basic, ok := underlyingType.(*types.Basic); ok {
							// Emit the underlying basic type directly
							javaType := getJavaTypeName(basic)
							je.emitToPackage(javaType)
							je.suppressTypeAliasSelectorX = true
							je.inTypeContext = false // Skip normal type emission
						} else if _, ok := underlyingType.(*types.Slice); ok {
							// Slice type alias - emit ArrayList<elemType> directly
							javaType := getJavaTypeName(typeAndValue.Type)
							je.emitToPackage(javaType)
							je.suppressTypeAliasSelectorX = true
							je.inTypeContext = false // Skip normal type emission
						} else if _, ok := underlyingType.(*types.Map); ok {
							// Map type alias - emit HashMap<K,V> directly
							javaType := getJavaTypeName(typeAndValue.Type)
							je.emitToPackage(javaType)
							je.suppressTypeAliasSelectorX = true
							je.inTypeContext = false // Skip normal type emission
						}
					}
				}
			}
		}
	})
}

func (je *JavaEmitter) PostVisitDeclStmtValueSpecType(node *ast.ValueSpec, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inTypeContext = false
		je.suppressTypeAliasSelectorX = false
		je.suppressClosureTypeEmit = false // Reset closure type suppression
		je.isArray = false                 // Reset array flag after declaration type
		// For closure-captured mutable variables, we already emitted Object[] in PreVisit
		// (skipped normal type emission), so nothing to do here
	})
}

func (je *JavaEmitter) PreVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Just emit space before name - the name itself is emitted by PreVisitIdent
		str := je.emitAsString(" ", 0)
		je.emitToPackage(str)
		// Set flag to indicate we're in a declaration name (don't add [0] to the name)
		je.inDeclName = true
	})
}

func (je *JavaEmitter) PostVisitDeclStmtValueSpecNames(node *ast.Ident, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inDeclName = false // Reset declaration name context
		if je.pendingMapInit {
			if je.isClosureCapturedDecl {
				je.emitToPackage(fmt.Sprintf(" = {newHashMap(%d)}", je.pendingMapKeyType))
			} else {
				je.emitToPackage(fmt.Sprintf(" = newHashMap(%d)", je.pendingMapKeyType))
			}
			je.pendingMapInit = false
			je.pendingMapKeyType = 0
		}
		if je.pendingStructInit {
			if je.isClosureCapturedDecl {
				je.emitToPackage(fmt.Sprintf(" = {new %s()}", je.pendingStructTypeName))
			} else {
				je.emitToPackage(fmt.Sprintf(" = new %s()", je.pendingStructTypeName))
			}
			je.pendingStructInit = false
			je.pendingStructTypeName = ""
		}
		if je.pendingSliceInit {
			if je.isClosureCapturedDecl {
				je.emitToPackage(fmt.Sprintf(" = {new ArrayList<%s>()}", je.pendingSliceElemType))
			} else {
				je.emitToPackage(fmt.Sprintf(" = new ArrayList<%s>()", je.pendingSliceElemType))
			}
			je.pendingSliceInit = false
			je.pendingSliceElemType = ""
		}
		if je.pendingPrimitiveInit {
			if je.isClosureCapturedDecl {
				je.emitToPackage(fmt.Sprintf(" = {%s}", je.pendingPrimitiveDefault))
			} else {
				je.emitToPackage(fmt.Sprintf(" = %s", je.pendingPrimitiveDefault))
			}
			je.pendingPrimitiveInit = false
			je.pendingPrimitiveDefault = ""
		}
		// Only emit semicolon if there's no value coming
		if !je.hasDeclValue {
			je.emitToPackage(";\n")
			// Reset closure-captured decl flag
			je.isClosureCapturedDecl = false
			je.closureCapturedDeclName = ""
		}
	})
}

func (je *JavaEmitter) PreVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isClosureCapturedDecl {
			je.emitToPackage(" = {")
		} else {
			je.emitToPackage(" = ")
		}
	})
}

func (je *JavaEmitter) PostVisitDeclStmtValueSpecValue(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isClosureCapturedDecl {
			je.emitToPackage("};\n")
			je.isClosureCapturedDecl = false
			je.closureCapturedDeclName = ""
		} else {
			je.emitToPackage(";\n")
		}
		je.hasDeclValue = false
	})
}

func (je *JavaEmitter) PreVisitGenStructFieldType(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("public ", indent+4)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PostVisitGenStructFieldType(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(" ", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PostVisitGenStructFieldName(node *ast.Ident, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.emitToPackage(";\n")
	})
}

func (je *JavaEmitter) PreVisitPackage(pkg *packages.Package, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.pkg = pkg
		je.currentPkgName = pkg.Name

		// For non-main packages, create a separate file
		// Exception: "hmap" is a utility package used by generated code, so keep it inline
		if pkg.Name != "main" && pkg.Name != "hmap" {
			// Check for naming conflict with main output file
			if pkg.Name == je.OutputName {
				// Naming conflict - rename main output to avoid collision
				// Close and remove the current main file, create a new one with "_main" suffix
				oldOutput := je.Output
				je.file.Close()
				os.Remove(oldOutput) // Remove the old file

				je.OutputName = je.OutputName + "_main"
				je.Output = filepath.Join(je.OutputDir, je.OutputName+".java")
				var err error
				je.file, err = os.Create(je.Output)
				if err != nil {
					fmt.Println("Error creating renamed main file:", err)
					return
				}
				je.SetFile(je.file)
				// Re-write imports and helpers to the new main file
				je.writeJavaBoilerplate()
			}

			// Create separate file for this package
			pkgFileName := filepath.Join(je.OutputDir, pkg.Name+".java")
			pkgFile, err := os.Create(pkgFileName)
			if err != nil {
				fmt.Println("Error creating package file:", err)
				return
			}
			je.packageFiles[pkg.Name] = pkgFile

			// Write imports and class header to package file
			imports := "import java.util.*;\nimport java.util.function.*;\n\n"
			pkgFile.WriteString(imports)
			pkgFile.WriteString(fmt.Sprintf("public class %s {\n\n", pkg.Name))

			// Package is already in namespaces (added by base_pass.go), so type prefixing works
			return
		}

		// For main package, use the main output file
		// Only create class if not already opened
		if je.apiClassOpened {
			return
		}
		var className string
		if je.OutputName != "" {
			// Use the output file name as the class name, sanitized for Java
			className = sanitizeJavaIdentifier(je.OutputName)
		} else {
			className = "Main"
		}
		str := je.emitAsString(fmt.Sprintf("public class %s {\n\n", className), indent)
		je.emitToPackage(str)
		_ = je.gir.emitToFileBufferString("", pkg.Name)
		je.currentPackage = className
		je.apiClassOpened = true
	})
}

func (je *JavaEmitter) PostVisitPackage(pkg *packages.Package, indent int) {
	je.executeIfNotForwardDecls(func() {
		// For non-main packages, close the class and file
		if pkg.Name != "main" {
			if pkgFile, exists := je.packageFiles[pkg.Name]; exists {
				pkgFile.WriteString("}\n") // Close class
				pkgFile.Close()

				// Format the package file
				pkgFileName := filepath.Join(je.OutputDir, pkg.Name+".java")
				if err := FormatFile(pkgFileName, "--style=webkit"); err != nil {
					log.Printf("Warning: failed to format %s: %v", pkgFileName, err)
				}
			}
			return
		}
		// Don't close the main class here - it will be closed in PostVisitProgram
	})
}

func (je *JavaEmitter) PostVisitFuncDeclSignature(node *ast.FuncDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.isArray = false
	})
}

func (je *JavaEmitter) PostVisitBlockStmtList(node ast.Stmt, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("\n", indent)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitGenStructInfo(node GenTypeInfo, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(fmt.Sprintf("public static class %s {\n", node.Name), indent+2)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PostVisitGenStructInfo(node GenTypeInfo, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Generate constructor for the struct
		if node.Struct != nil && node.Struct.Fields != nil {
			var fields []struct {
				name     string
				javaType string
			}

			// Collect field names and types
			for _, field := range node.Struct.Fields.List {
				// Get Java type for the field
				javaType := "Object"
				if je.pkg != nil && je.pkg.TypesInfo != nil {
					tv := je.pkg.TypesInfo.Types[field.Type]
					if tv.Type != nil {
						javaType = getJavaTypeName(tv.Type)
					}
				} else {
					// Fallback: try to get type from AST
					javaType = je.getJavaTypeFromExpr(field.Type)
				}

				// Handle multiple field names (e.g., `a, b int`)
				for _, name := range field.Names {
					fields = append(fields, struct {
						name     string
						javaType string
					}{name: name.Name, javaType: javaType})
				}
			}

			// Generate default (no-arg) constructor that initializes fields to defaults
			// Use getJavaDefaultValueForStruct to initialize nested structs (Go value semantics)
			defaultConstructor := fmt.Sprintf("        public %s() {\n", node.Name)
			for _, f := range fields {
				defaultConstructor += fmt.Sprintf("            this.%s = %s;\n", f.name, getJavaDefaultValueForStruct(f.javaType))
			}
			defaultConstructor += "        }\n"
			je.emitToPackage(defaultConstructor)

			// Generate all-args constructor
			constructorCode := fmt.Sprintf("        public %s(", node.Name)
			for i, f := range fields {
				if i > 0 {
					constructorCode += ", "
				}
				constructorCode += fmt.Sprintf("%s %s", f.javaType, f.name)
			}
			constructorCode += ") {\n"
			for _, f := range fields {
				// For struct types, convert null to new instance (Go value semantics)
				if !isJavaPrimitiveType(f.javaType) &&
					!strings.HasPrefix(f.javaType, "ArrayList<") &&
					!strings.HasPrefix(f.javaType, "HashMap<") &&
					!isJavaFunctionalInterface(f.javaType) &&
					!isJavaBuiltinReferenceType(f.javaType) {
					constructorCode += fmt.Sprintf("            this.%s = %s != null ? %s : new %s();\n", f.name, f.name, f.name, f.javaType)
				} else {
					constructorCode += fmt.Sprintf("            this.%s = %s;\n", f.name, f.name)
				}
			}
			constructorCode += "        }\n"
			je.emitToPackage(constructorCode)

			// Generate copy constructor for value semantics when appending to slices
			copyConstructor := fmt.Sprintf("        public %s(%s other) {\n", node.Name, node.Name)
			for _, f := range fields {
				// Deep copy ArrayList/reference types, shallow copy primitives
				if strings.HasPrefix(f.javaType, "ArrayList<") {
					copyConstructor += fmt.Sprintf("            this.%s = other.%s != null ? new ArrayList<>(other.%s) : null;\n", f.name, f.name, f.name)
				} else if strings.HasPrefix(f.javaType, "HashMap<") {
					copyConstructor += fmt.Sprintf("            this.%s = other.%s != null ? new HashMap<>(other.%s) : null;\n", f.name, f.name, f.name)
				} else if isJavaPrimitiveType(f.javaType) {
					copyConstructor += fmt.Sprintf("            this.%s = other.%s;\n", f.name, f.name)
				} else if isJavaFunctionalInterface(f.javaType) || isJavaBuiltinReferenceType(f.javaType) {
					// Functional interfaces (lambdas) and built-in reference types can be copied by reference
					copyConstructor += fmt.Sprintf("            this.%s = other.%s;\n", f.name, f.name)
				} else {
					// For other reference types (structs), use copy constructor if available
					copyConstructor += fmt.Sprintf("            this.%s = other.%s != null ? new %s(other.%s) : null;\n", f.name, f.name, f.javaType, f.name)
				}
			}
			copyConstructor += "        }\n"
			je.emitToPackage(copyConstructor)
		}

		str := je.emitAsString("}\n\n", indent+2)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitArrayType(node ast.ArrayType, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.suppressTypeAliasEmit || je.suppressMapEmit || je.suppressMultiReturnTypes || je.isSliceMakeCall || je.suppressClosureTypeEmit {
			return
		}
		je.inArrayTypeContext = true // Track for boxed type conversion
		// Detect byte/short element types for literal casting
		if ident, ok := node.Elt.(*ast.Ident); ok {
			switch ident.Name {
			case "int8", "byte":
				je.sliceLitElemCast = "(byte)"
			case "int16":
				je.sliceLitElemCast = "(short)"
			default:
				je.sliceLitElemCast = ""
			}
		} else {
			je.sliceLitElemCast = ""
		}
		str := je.emitAsString("ArrayList", indent)
		je.emitToPackage(str)
		str = je.emitAsString("<", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PostVisitArrayType(node ast.ArrayType, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.suppressTypeAliasEmit || je.suppressMapEmit || je.suppressMultiReturnTypes || je.isSliceMakeCall || je.suppressClosureTypeEmit {
			return
		}
		je.inArrayTypeContext = false // Clear boxed type context
		str := je.emitAsString(">", 0)
		je.emitToPackage(str)

		if je.isSliceMakeCall {
			return
		}

		pointerAndPosition := SearchPointerIndexReverse(PreVisitArrayType, je.gir.pointerAndIndexVec)
		if pointerAndPosition != nil {
			tokens, _ := ExtractTokens(pointerAndPosition.Index, je.gir.tokenSlice)
			je.isArray = true
			je.arrayType = strings.Join(tokens, "")
			je.gir.pointerAndIndexVec = RemovePointerEntryReverse(je.gir.pointerAndIndexVec, PreVisitArrayType)
		}
	})
}

func (je *JavaEmitter) PreVisitMapType(node *ast.MapType, indent int) {
	if je.isMapMakeCall {
		return
	}
	// Also suppress HashMap emission when inside slice make call type argument
	if je.isSliceMakeCall && je.inTypeContext {
		return
	}
	je.executeIfNotForwardDecls(func() {
		if je.suppressMapEmit {
			return
		}
		// Use HashMap without prefix since it's defined as an inner class
		je.emitToPackage("HashMap")
	})
}

func (je *JavaEmitter) PostVisitMapType(node *ast.MapType, indent int) {
}

func (je *JavaEmitter) PreVisitMapKeyType(node ast.Expr, indent int) {
	je.suppressMapEmitStack = append(je.suppressMapEmitStack, je.suppressMapEmit)
	je.suppressMapEmit = true
	je.inTypeContext = true // Ensure type context is set
}

func (je *JavaEmitter) PostVisitMapKeyType(node ast.Expr, indent int) {
	je.inTypeContext = false
	if len(je.suppressMapEmitStack) > 0 {
		je.suppressMapEmit = je.suppressMapEmitStack[len(je.suppressMapEmitStack)-1]
		je.suppressMapEmitStack = je.suppressMapEmitStack[:len(je.suppressMapEmitStack)-1]
	} else {
		je.suppressMapEmit = false
	}
}

func (je *JavaEmitter) PreVisitMapValueType(node ast.Expr, indent int) {
	je.suppressMapEmitStack = append(je.suppressMapEmitStack, je.suppressMapEmit)
	je.suppressMapEmit = true
	je.inTypeContext = true // Ensure type context is set
}

func (je *JavaEmitter) PostVisitMapValueType(node ast.Expr, indent int) {
	je.inTypeContext = false
	if len(je.suppressMapEmitStack) > 0 {
		je.suppressMapEmit = je.suppressMapEmitStack[len(je.suppressMapEmitStack)-1]
		je.suppressMapEmitStack = je.suppressMapEmitStack[:len(je.suppressMapEmitStack)-1]
	} else {
		je.suppressMapEmit = false
	}
}

func (je *JavaEmitter) PreVisitFuncType(node *ast.FuncType, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inTypeContext = true
		// Map Go function types to Java functional interfaces
		funcInterfaceType := je.getFunctionalInterfaceType(node)
		str := je.emitAsString(funcInterfaceType, indent)
		je.emitToPackage(str)
		// Suppress all type parameter emissions (we already emitted the full type)
		je.suppressMultiReturnTypes = true
	})
}

func (je *JavaEmitter) PostVisitFuncType(node *ast.FuncType, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inTypeContext = false
		je.numFuncResults = 0
		je.suppressMultiReturnTypes = false
	})
}

func (je *JavaEmitter) PreVisitFuncTypeParam(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress parameter emissions for function types
	})
}

func (je *JavaEmitter) PreVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Track that we're in selector X position (for closure-captured field access)
		je.inSelectorX = true

		// Check if X is a package - suppress it since all code is in one Java file
		// Do this first before any early returns
		if ident, ok := node.X.(*ast.Ident); ok {
			if ident.Name == "fmt" {
				je.suppressFmtPackage = true
			}
			// Check if this is a package-qualified reference (not a struct field access)
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				if uses, ok := je.pkg.TypesInfo.Uses[ident]; ok {
					if _, isPkg := uses.(*types.PkgName); isPkg {
						// Check if this is a runtime package or outer class package - if so, keep the prefix
						// because these packages define a class with that name (e.g., fs, http, types)
						if !isJavaOuterClassPackage(ident.Name) {
							// Not a runtime or outer class package - suppress the package name
							je.suppressFmtPackage = true // reuse this flag for package suppression
						} else {
							// We're in a package-qualified expression (types.Plan, http.Response)
							// Set flag to prevent double-prefixing in PreVisitIdent
							je.inPackageQualifiedExpr = true
						}
					}
				}
			}
		}

		if je.isMultiReturnAssign {
			return
		}
		// Suppress during map operations (handled elsewhere)
		if je.suppressMapAssignLhs || je.isMapCommaOk || je.suppressMapIndexX {
			return
		}
		// Suppress during type assert comma-ok (handled in PostVisitAssignStmt)
		if je.isTypeAssertCommaOk {
			return
		}
		// Skip if we've already resolved a type alias
		if je.suppressTypeAliasSelectorX && je.inTypeContext {
			return
		}
		if je.captureRangeExpr {
			// X will be captured by PreVisitIdent, dot will be added by PostVisitSelectorExprX
			return
		}
	})
}

func (je *JavaEmitter) PostVisitSelectorExpr(node *ast.SelectorExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressFmtPackage = false
		je.inPackageQualifiedExpr = false
	})
}

func (je *JavaEmitter) PostVisitSelectorExprX(node ast.Expr, indent int) {
	// Reset selector X flag (we're done processing X, now processing Sel)
	je.inSelectorX = false

	je.executeIfNotForwardDecls(func() {
		if je.isMultiReturnAssign {
			return
		}
		if je.suppressTypeAliasSelectorX {
			return
		}
		// Add dot to captured range expression (after X is captured by PreVisitIdent)
		if je.captureRangeExpr {
			je.rangeCollectionExpr += "."
			return
		}
		// Skip package prefix (use types/functions directly since all in one file)
		// Reset the flag so the type/function name is emitted
		if je.suppressFmtPackage {
			je.suppressFmtPackage = false // Reset so Sel identifier is emitted
			return                        // Don't emit dot
		}
		// Don't emit dot for package-qualified type aliases that were resolved
		if je.inTypeContext && je.suppressTypeAliasSelectorX {
			return
		}
		// Suppress dot during map operations (handled in PreVisitAssignStmtRhs or PostVisitIndexExprX)
		if je.suppressMapAssignLhs || je.isMapCommaOk || je.suppressMapIndexX {
			return
		}
		// Suppress dot during type assert comma-ok (handled in PostVisitAssignStmt)
		if je.isTypeAssertCommaOk {
			return
		}
		// Suppress dot in lambda parameter types (Java uses type inference)
		if je.inLambdaParams && je.inTypeContext {
			return
		}
		// Suppress dot when suppressing multi-return types
		if je.suppressMultiReturnTypes {
			return
		}
		// Suppress dot when suppressing closure type emission
		if je.suppressClosureTypeEmit {
			return
		}
		str := je.emitAsString(".", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitFuncTypeResults(node *ast.FieldList, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.suppressMultiReturnTypes {
			return
		}
		if node != nil && len(node.List) > 0 {
			je.numFuncResults = len(node.List)
			je.emitToken(", ", Comma, 0)
		}
	})
}

func (je *JavaEmitter) PreVisitFuncDeclSignatureTypeParamsList(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if index > 0 {
			str := je.emitAsString(", ", 0)
			je.emitToPackage(str)
		}
	})
}

func (je *JavaEmitter) PreVisitFuncDeclSignatureTypeParamsArgName(node *ast.Ident, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inTypeContext = false
		je.emitToPackage(" ")
		// Track this parameter name for lambda conflict detection
		if node != nil {
			je.addFuncParam(node.Name)
		}
	})
}

func (je *JavaEmitter) PreVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.suppressMultiReturnTypes {
			return
		}
		if index > 0 {
			str := je.emitAsString(", ", 0)
			je.emitToPackage(str)
		}
	})
}

func (je *JavaEmitter) PostVisitFuncDeclSignatureTypeResultsList(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.isTuple = true
	})
}

func (je *JavaEmitter) PreVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.isTuple = false
		je.numFuncResults = 0
		je.suppressMultiReturnTypes = false
		je.currentFuncName = node.Name.Name
		if node.Type.Results != nil {
			je.numFuncResults = len(node.Type.Results.List)
		}

		var resultClassName string
		if je.numFuncResults > 1 {
			// Collect return types
			var returnTypes []string
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				for _, field := range node.Type.Results.List {
					tv := je.pkg.TypesInfo.Types[field.Type]
					if tv.Type != nil {
						returnTypes = append(returnTypes, getJavaTypeName(tv.Type))
					} else {
						returnTypes = append(returnTypes, "Object")
					}
				}
			} else {
				for range node.Type.Results.List {
					returnTypes = append(returnTypes, "Object")
				}
			}

			// Create signature key for registry
			signature := strings.Join(returnTypes, ",")

			// Check if we already have a class for this signature
			if existingClass, ok := je.resultClassRegistry[signature]; ok {
				// Reuse existing class
				resultClassName = existingClass
			} else {
				// Create new class with first function's name
				resultClassName = node.Name.Name + "Result"
				je.resultClassRegistry[signature] = resultClassName

				// Generate and emit the result class definition
				classCode := fmt.Sprintf("\n    public static class %s {\n", resultClassName)
				for i, typeName := range returnTypes {
					classCode += fmt.Sprintf("        public %s _%d;\n", typeName, i)
				}
				// Constructor
				classCode += fmt.Sprintf("        public %s(", resultClassName)
				for i, typeName := range returnTypes {
					if i > 0 {
						classCode += ", "
					}
					classCode += fmt.Sprintf("%s _%d", typeName, i)
				}
				classCode += ") {\n"
				for i := range returnTypes {
					classCode += fmt.Sprintf("            this._%d = _%d;\n", i, i)
				}
				classCode += "        }\n    }\n\n"
				je.emitToPackage(classCode)
			}

			// Store for later use at call sites (use the shared class name)
			je.multiReturnFuncs[node.Name.Name] = returnTypes
		}

		// Now emit the method signature
		str := je.emitAsString("public static ", indent+2)
		je.emitToPackage(str)

		if je.numFuncResults == 0 {
			str = je.emitAsString("void ", 0)
			je.emitToPackage(str)
		} else if je.numFuncResults > 1 {
			// Get the result class name from registry based on signature
			var returnTypes []string
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				for _, field := range node.Type.Results.List {
					tv := je.pkg.TypesInfo.Types[field.Type]
					if tv.Type != nil {
						returnTypes = append(returnTypes, getJavaTypeName(tv.Type))
					} else {
						returnTypes = append(returnTypes, "Object")
					}
				}
			}
			signature := strings.Join(returnTypes, ",")
			resultClassName = je.resultClassRegistry[signature]

			// Emit the result class name as return type
			str = je.emitAsString(resultClassName+" ", 0)
			je.emitToPackage(str)
			// Suppress individual return type emissions
			je.suppressMultiReturnTypes = true
		}
	})
}

func (je *JavaEmitter) PostVisitFuncDeclSignatureTypeResults(node *ast.FuncDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressMultiReturnTypes = false
		if je.numFuncResults == 1 {
			str := je.emitAsString(" ", 0)
			je.emitToPackage(str)
		}
	})
}

func (je *JavaEmitter) PreVisitTypeAliasName(node *ast.Ident, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressTypeAliasEmit = true
		je.currentAliasName = node.Name
	})
}

func (je *JavaEmitter) PostVisitTypeAliasName(node *ast.Ident, indent int) {
}

func (je *JavaEmitter) PreVisitTypeAliasType(node ast.Expr, indent int) {
	// Type aliases are recorded but not emitted in Java
}

func (je *JavaEmitter) PostVisitTypeAliasType(node ast.Expr, indent int) {
	if je.currentAliasName != "" {
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			tv := je.pkg.TypesInfo.Types[node]
			if tv.Type != nil {
				underlyingType := getJavaTypeName(tv.Type)
				je.typeAliasMap[je.currentAliasName] = underlyingType
			}
		}
	}
	je.suppressTypeAliasEmit = false
	je.currentAliasName = ""
}

func (je *JavaEmitter) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("return ", indent)
		je.emitToPackage(str)

		if len(node.Results) > 1 {
			// Look up the shared result class name from the registry
			var resultClassName string
			if returnTypes, ok := je.multiReturnFuncs[je.currentFuncName]; ok {
				signature := strings.Join(returnTypes, ",")
				if className, ok := je.resultClassRegistry[signature]; ok {
					resultClassName = className
				} else {
					resultClassName = je.currentFuncName + "Result"
				}
			} else {
				resultClassName = je.currentFuncName + "Result"
			}
			je.emitToken(fmt.Sprintf("new %s(", resultClassName), LeftParen, 0)
		}
	})
}

func (je *JavaEmitter) PostVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		if len(node.Results) > 1 {
			je.emitToken(")", RightParen, 0)
		}
		str := je.emitAsString(";", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitReturnStmtResult(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if index > 0 {
			str := je.emitAsString(", ", 0)
			je.emitToPackage(str)
		}
		// Add cast for byte/short return values when returning literals
		if returnTypes, ok := je.multiReturnFuncs[je.currentFuncName]; ok {
			if index < len(returnTypes) {
				// Check if this is a numeric literal (including unary minus like -1)
				isNumericLit := false
				if _, isLit := node.(*ast.BasicLit); isLit {
					isNumericLit = true
				} else if unary, isUnary := node.(*ast.UnaryExpr); isUnary {
					// Handle -1, +1, etc.
					if _, isLit := unary.X.(*ast.BasicLit); isLit {
						isNumericLit = true
					}
				}
				if isNumericLit {
					switch returnTypes[index] {
					case "byte":
						je.emitToPackage("(byte)")
					case "short":
						je.emitToPackage("(short)")
					}
				}
			}
		}
	})
}

func (je *JavaEmitter) PostVisitCallExpr(node *ast.CallExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isUnsupportedIndexedCall {
			// Emit a comment for unsupported indexed function call
			indentStr := strings.Repeat("    ", je.unsupportedCallIndent)
			je.emitToPackage(fmt.Sprintf("%s/* TODO: Unsupported indexed function call: %s */",
				indentStr, exprToString(node)))
			je.isUnsupportedIndexedCall = false
		}
		// Clear parameter types after call expression
		je.callExprParamTypes = nil
		// Clear append call flags
		je.isAppendCall = false
		je.appendStructType = ""
	})
}

func (je *JavaEmitter) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	// Check if all LHS are blank identifiers
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
		je.suppressRangeEmit = true
		return
	}

	// Detect comma-ok map access
	if len(node.Lhs) == 2 && len(node.Rhs) == 1 {
		if indexExpr, ok := node.Rhs[0].(*ast.IndexExpr); ok {
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				tv := je.pkg.TypesInfo.Types[indexExpr.X]
				if tv.Type != nil {
					if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
						je.isMapCommaOk = true
						je.mapCommaOkValName = node.Lhs[0].(*ast.Ident).Name
						je.mapCommaOkOkName = node.Lhs[1].(*ast.Ident).Name
						je.mapCommaOkMapName = exprToString(indexExpr.X)
						je.mapCommaOkValType = getJavaTypeName(mapType.Elem())
						je.mapCommaOkIsDecl = (node.Tok == token.DEFINE)
						je.mapCommaOkIndent = indent
						je.mapKeyCastPrefix, je.mapKeyCastSuffix = getJavaKeyCast(mapType.Key())
						je.suppressMapEmit = true
						return
					}
				}
			}
		}
		// Detect type assert comma-ok: val, ok := x.(Type)
		if typeAssert, ok := node.Rhs[0].(*ast.TypeAssertExpr); ok {
			je.isTypeAssertCommaOk = true
			je.typeAssertCommaOkValName = node.Lhs[0].(*ast.Ident).Name
			je.typeAssertCommaOkOkName = node.Lhs[1].(*ast.Ident).Name
			je.typeAssertCommaOkXName = exprToString(typeAssert.X)
			je.typeAssertCommaOkIsDecl = (node.Tok == token.DEFINE)
			je.typeAssertCommaOkIndent = indent
			// Get the type being asserted
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				tv := je.pkg.TypesInfo.Types[typeAssert.Type]
				if tv.Type != nil {
					je.typeAssertCommaOkType = getJavaTypeName(tv.Type)
					je.typeAssertCommaOkBoxedType = toBoxedType(je.typeAssertCommaOkType)
				}
			}
			return
		}
	}

	// Detect multi-return function call assignment
	if len(node.Lhs) > 1 && len(node.Rhs) == 1 {
		if callExpr, ok := node.Rhs[0].(*ast.CallExpr); ok {
			funcName := ""
			if ident, ok := callExpr.Fun.(*ast.Ident); ok {
				funcName = ident.Name
			} else if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
				funcName = sel.Sel.Name
			}
			if _, isMultiReturn := je.multiReturnFuncs[funcName]; isMultiReturn || funcName != "" {
				// Collect LHS variable names and types
				je.multiReturnLhsNames = nil
				je.multiReturnLhsTypes = nil
				for _, lhs := range node.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						je.multiReturnLhsNames = append(je.multiReturnLhsNames, ident.Name)
						// Get the type of the LHS variable
						if je.pkg != nil && je.pkg.TypesInfo != nil {
							if tv, ok := je.pkg.TypesInfo.Types[ident]; ok {
								javaType := getJavaTypeName(tv.Type)
								je.multiReturnLhsTypes = append(je.multiReturnLhsTypes, javaType)
							} else if def := je.pkg.TypesInfo.Defs[ident]; def != nil {
								javaType := getJavaTypeName(def.Type())
								je.multiReturnLhsTypes = append(je.multiReturnLhsTypes, javaType)
							} else {
								je.multiReturnLhsTypes = append(je.multiReturnLhsTypes, "")
							}
						} else {
							je.multiReturnLhsTypes = append(je.multiReturnLhsTypes, "")
						}
					}
				}
				je.isMultiReturnAssign = true
				je.multiReturnIsDecl = (node.Tok == token.DEFINE)
				je.multiReturnIndent = indent
				return
			}
		}
	}

	// Detect map assignment (including nested maps)
	if len(node.Lhs) == 1 {
		if indexExpr, ok := node.Lhs[0].(*ast.IndexExpr); ok {
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				tv := je.pkg.TypesInfo.Types[indexExpr.X]
				if tv.Type != nil {
					if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
						je.suppressMapEmit = true
						je.suppressMapAssignLhs = true // Suppress LHS emission
						je.assignmentToken = "="
						je.mapKeyCastPrefix, je.mapKeyCastSuffix = getJavaKeyCast(mapType.Key())

						// Check if this is a nested map assignment (e.g., m["a"]["b"] = v)
						// Only handle as nested map if the inner expression is also a map access
						if innerIndex, isNestedIndex := indexExpr.X.(*ast.IndexExpr); isNestedIndex {
							// Check if the inner IndexExpr is accessing a map (not a slice)
							innerTv := je.pkg.TypesInfo.Types[innerIndex.X]
							if innerTv.Type != nil {
								if _, isInnerMap := innerTv.Type.Underlying().(*types.Map); isInnerMap {
									// Nested map assignment (map[K1]map[K2]V)
									je.isNestedMapAssign = true
									je.nestedMapAssignIndent = indent
									rootVar, levels := je.collectNestedMapLevels(indexExpr.X)
									je.nestedMapRootVar = rootVar
									je.nestedMapLevels = levels
									je.nestedMapAssignKey = exprToString(indexExpr.Index)
									return
								}
								// Inner is a slice access with map element (e.g., sliceOfMaps[0][1]["a"] = v)
								// This needs special handling for any depth of slice nesting
								if _, isInnerSlice := innerTv.Type.Underlying().(*types.Slice); isInnerSlice {
									je.isSliceOfMapAssign = true
									// Collect all slice indices from the chain
									rootVar, indices := je.collectSliceIndices(indexExpr.X)
									je.sliceOfMapSliceVar = rootVar
									je.sliceOfMapSliceIndices = indices
									je.sliceOfMapMapKey = exprToString(indexExpr.Index)
									je.sliceOfMapIndent = indent
									je.suppressMapEmit = true
									je.suppressMapAssignLhs = true
									return
								}
							}
						}

						// Simple map assignment
						je.isMapAssign = true
						je.mapAssignIndent = indent
						je.mapAssignVarName = exprToString(indexExpr.X)
						return
					}
					// Detect slice index assignment: x[i] = value -> x.set(i, value)
					if _, ok := tv.Type.Underlying().(*types.Slice); ok {
						je.isSliceIndexAssign = true
						je.sliceIndexVarName = exprToString(indexExpr.X)
						je.sliceIndexIndent = indent
						je.sliceIndexLhsExpr = indexExpr // Track the specific LHS index expr
						return
					}
				}
			}
		}
	}

	// Detect byte/short arithmetic: a = a + 5 where a is byte/short
	// In Java, byte/short arithmetic produces int, so we need to cast back
	if len(node.Lhs) == 1 && len(node.Rhs) == 1 {
		if ident, ok := node.Lhs[0].(*ast.Ident); ok {
			// Check if RHS contains a binary expression (arithmetic)
			if _, isBinary := node.Rhs[0].(*ast.BinaryExpr); isBinary {
				if je.pkg != nil && je.pkg.TypesInfo != nil {
					if obj := je.pkg.TypesInfo.ObjectOf(ident); obj != nil {
						typeName := obj.Type().String()
						switch typeName {
						case "int8", "byte":
							je.needsSmallIntCast = true
							je.smallIntCastType = "byte"
						case "int16":
							je.needsSmallIntCast = true
							je.smallIntCastType = "short"
						}
					}
				}
			}
		}
	}

	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("", indent)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PostVisitAssignStmt(node *ast.AssignStmt, indent int) {
	// Handle map comma-ok
	if je.isMapCommaOk {
		// Check if val and ok are already declared
		valVarDecl := ""
		okVarDecl := ""
		if je.mapCommaOkIsDecl {
			if !je.isVarDeclared(je.mapCommaOkOkName) {
				okVarDecl = "var "
				je.declareVar(je.mapCommaOkOkName)
			}
			if !je.isVarDeclared(je.mapCommaOkValName) {
				valVarDecl = "var "
				je.declareVar(je.mapCommaOkValName)
			}
		}
		// Add cast for the value type
		boxedValType := toBoxedType(je.mapCommaOkValType)
		keyExpr := exprToString(node.Rhs[0].(*ast.IndexExpr).Index)
		// First emit ok variable
		code := fmt.Sprintf("%s%s = hashMapContains(%s, %s%s%s);\n",
			okVarDecl, je.mapCommaOkOkName,
			je.mapCommaOkMapName,
			je.mapKeyCastPrefix, keyExpr, je.mapKeyCastSuffix)
		// Then emit val with ternary to provide default value when key not found
		// This avoids NPE when auto-unboxing null for comparisons
		defaultVal := getJavaDefaultValue(je.mapCommaOkValType)
		code += fmt.Sprintf("%s%s = %s ? (%s)hashMapGet(%s, %s%s%s) : %s;\n",
			valVarDecl, je.mapCommaOkValName,
			je.mapCommaOkOkName,
			boxedValType,
			je.mapCommaOkMapName,
			je.mapKeyCastPrefix, keyExpr, je.mapKeyCastSuffix,
			defaultVal)
		je.emitToPackage(code)
		je.isMapCommaOk = false
		je.suppressMapEmit = false
		return
	}

	// Handle type assert comma-ok: val, ok := x.(Type)
	if je.isTypeAssertCommaOk {
		// Check if val and ok are already declared
		valVarDecl := ""
		okVarDecl := ""
		if je.typeAssertCommaOkIsDecl {
			if !je.isVarDeclared(je.typeAssertCommaOkOkName) {
				okVarDecl = "var "
				je.declareVar(je.typeAssertCommaOkOkName)
			}
			if !je.isVarDeclared(je.typeAssertCommaOkValName) {
				valVarDecl = "var "
				je.declareVar(je.typeAssertCommaOkValName)
			}
		}
		indentStr := strings.Repeat("    ", je.typeAssertCommaOkIndent)
		// Generate: var ok = x instanceof Type;
		// Generate: var val = ok ? (Type)x : defaultValue;
		code := fmt.Sprintf("%s%s%s = %s instanceof %s;\n",
			indentStr, okVarDecl, je.typeAssertCommaOkOkName,
			je.typeAssertCommaOkXName, je.typeAssertCommaOkBoxedType)

		// Get default value for the type
		defaultVal := getJavaDefaultValue(je.typeAssertCommaOkType)
		code += fmt.Sprintf("%s%s%s = %s ? (%s)%s : %s;\n",
			indentStr, valVarDecl, je.typeAssertCommaOkValName,
			je.typeAssertCommaOkOkName,
			je.typeAssertCommaOkType, je.typeAssertCommaOkXName,
			defaultVal)

		je.emitToPackage(code)
		je.isTypeAssertCommaOk = false
		return
	}

	// Handle multi-return function call assignment
	if je.isMultiReturnAssign {
		callExpr := node.Rhs[0].(*ast.CallExpr)
		funcName := ""
		isRuntimeCall := false // Track if this is a runtime package call that returns Object[]
		if ident, ok := callExpr.Fun.(*ast.Ident); ok {
			funcName = ident.Name
		} else if sel, ok := callExpr.Fun.(*ast.SelectorExpr); ok {
			// Check if X is a package - if so, use function name with or without prefix
			if xIdent, ok := sel.X.(*ast.Ident); ok {
				if je.pkg != nil && je.pkg.TypesInfo != nil {
					if uses, ok := je.pkg.TypesInfo.Uses[xIdent]; ok {
						if _, isPkg := uses.(*types.PkgName); isPkg {
							// It's a package reference - check if it's an outer class package
							if isJavaOuterClassPackage(xIdent.Name) {
								// Runtime or outer class package - keep the package prefix (e.g., fs.ReadFile, types.AddLiteralToPlan)
								funcName = xIdent.Name + "." + sel.Sel.Name
								// Check if this runtime package returns Object[] (fs, http, net)
								// Graphics package uses proper result classes (like GetMouseResult), not Object[]
								if xIdent.Name == "fs" || xIdent.Name == "http" || xIdent.Name == "net" {
									isRuntimeCall = true
								}
							} else {
								// Regular package - just use the function name
								funcName = sel.Sel.Name
							}
						} else {
							funcName = exprToString(sel.X) + "." + sel.Sel.Name
						}
					} else {
						funcName = exprToString(sel.X) + "." + sel.Sel.Name
					}
				} else {
					funcName = exprToString(sel.X) + "." + sel.Sel.Name
				}
			} else {
				funcName = exprToString(sel.X) + "." + sel.Sel.Name
			}
		}

		// Generate function call arguments using Java-specific expression conversion
		// Get parameter types for byte/short casting
		var paramTypes []string
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			tv := je.pkg.TypesInfo.Types[callExpr.Fun]
			if tv.Type != nil {
				if sig, ok := tv.Type.Underlying().(*types.Signature); ok {
					params := sig.Params()
					for i := 0; i < params.Len(); i++ {
						paramTypes = append(paramTypes, getJavaTypeName(params.At(i).Type()))
					}
				}
			}
		}
		var args []string
		for i, arg := range callExpr.Args {
			// Use closure-aware expression converter
			argStr := javaExprToStringWithClosures(arg, je.closureCapturedMutVars, je.closureCapturedVarType)
			// Add cast for byte/short parameters
			if i < len(paramTypes) && needsByteCast(paramTypes[i], argStr) {
				argStr = fmt.Sprintf("(%s)(%s)", paramTypes[i], argStr)
			}
			args = append(args, argStr)
		}
		argsStr := strings.Join(args, ", ")

		// Use unique result variable name to avoid redeclaration
		je.multiReturnCounter++
		resultVarName := fmt.Sprintf("_result%d", je.multiReturnCounter)

		indentStr := strings.Repeat("    ", je.multiReturnIndent)
		code := fmt.Sprintf("%svar %s = %s(%s);\n", indentStr, resultVarName, funcName, argsStr)

		// Unpack fields with variable shadowing check
		// Runtime functions return Object[] (use array indexing with cast), user functions return tuple (use ._N fields)
		for i, name := range je.multiReturnLhsNames {
			if name == "_" {
				continue // Skip blank identifiers
			}
			var valueExpr string
			if isRuntimeCall {
				// For runtime calls, we need to cast the Object[] element to the correct type
				javaType := ""
				if i < len(je.multiReturnLhsTypes) {
					javaType = je.multiReturnLhsTypes[i]
				}
				if javaType != "" && javaType != "Object" {
					// Use boxed type for cast, then auto-unbox
					boxedType := toBoxedType(javaType)
					valueExpr = fmt.Sprintf("(%s)%s[%d]", boxedType, resultVarName, i)
				} else {
					valueExpr = fmt.Sprintf("%s[%d]", resultVarName, i)
				}
			} else {
				valueExpr = fmt.Sprintf("%s._%d", resultVarName, i)
			}
			// Check if this LHS variable is closure-captured
			lhsName := name
			isClosureCaptured := je.closureCapturedMutVars != nil && je.closureCapturedMutVars[name]
			if isClosureCaptured {
				lhsName = name + "[0]"
			}
			if je.multiReturnIsDecl && !je.isVarDeclared(name) && !isClosureCaptured {
				code += fmt.Sprintf("%svar %s = %s;\n", indentStr, name, valueExpr)
				je.declareVar(name)
			} else {
				code += fmt.Sprintf("%s%s = %s;\n", indentStr, lhsName, valueExpr)
			}
		}

		je.emitToPackage(code)
		je.isMultiReturnAssign = false
		je.multiReturnLhsNames = nil
		je.multiReturnLhsTypes = nil
		return
	}

	// Handle map assignment
	if je.isMapAssign {
		je.isMapAssign = false
		je.suppressMapEmit = false
		return
	}

	// Handle nested map assignment
	if je.isNestedMapAssign {
		// Close all the nested hashMapSet calls
		// For n levels, we have n+1 hashMapSet calls, so we need n+1 closing parens
		// Plus 1 for the semicolon
		closingParens := strings.Repeat(")", len(je.nestedMapLevels)+1) + ";"
		je.emitToPackage(closingParens)
		je.isNestedMapAssign = false
		je.nestedMapRootVar = ""
		je.nestedMapLevels = nil
		je.nestedMapAssignKey = ""
		je.suppressMapEmit = false
		return
	}

	// Handle slice-of-map assignment
	if je.isSliceOfMapAssign {
		// Close the hashMapSet and set calls: ));
		je.emitToPackage("));")
		je.isSliceOfMapAssign = false
		je.sliceOfMapSliceVar = ""
		je.sliceOfMapSliceIndices = nil
		je.sliceOfMapMapKey = ""
		je.suppressMapEmit = false
		return
	}

	// Handle slice index assignment
	if je.isSliceIndexAssign {
		je.isSliceIndexAssign = false
		je.sliceIndexLhsExpr = nil
		return
	}

	if je.suppressRangeEmit {
		je.suppressRangeEmit = false
		return
	}

	// Don't emit semicolon if we're inside for loop init or post (handled elsewhere)
	if je.insideForInit || je.insideForPostCond {
		return
	}

	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(";", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	if je.isMapCommaOk || je.isTypeAssertCommaOk || je.isMultiReturnAssign {
		return
	}
	je.executeIfNotForwardDecls(func() {
		if je.isMapAssign {
			je.suppressMapAssignLhs = false // Reset LHS suppression
			indexExpr := node.Lhs[0].(*ast.IndexExpr)
			keyStr := exprToString(indexExpr.Index)
			je.emitToPackage(fmt.Sprintf("%s = hashMapSet(%s, %s%s%s, ",
				je.mapAssignVarName, je.mapAssignVarName,
				je.mapKeyCastPrefix, keyStr, je.mapKeyCastSuffix))
			return
		}
		// Handle nested map assignment: m["a"]["b"] = v
		// Generates: m = hashMapSet(m, "a", hashMapSet(((HashMap)hashMapGet(m, "a")), "b", v));
		if je.isNestedMapAssign {
			je.suppressMapAssignLhs = false // Reset LHS suppression

			// Build the nested hashMapSet/hashMapGet expression
			// For m["a"]["b"] = v with levels=[{"a", "HashMap"}] and finalKey="b":
			// Output: m = hashMapSet(m, "a", hashMapSet(((HashMap)hashMapGet(m, "a")), "b", v));

			var code string
			rootVar := je.nestedMapRootVar
			levels := je.nestedMapLevels
			finalKey := je.nestedMapAssignKey

			// Start with outermost: rootVar = hashMapSet(rootVar, levels[0].key,
			code = fmt.Sprintf("%s = hashMapSet(%s, %s, ", rootVar, rootVar, levels[0].keyExpr)

			// For each intermediate level (1 to n-1), wrap with hashMapSet(hashMapGet(...), key,
			for i := 1; i < len(levels); i++ {
				// Get expression for levels[0..i-1]
				getExpr := je.generateNestedMapGetExpr(rootVar, levels[:i])
				code += fmt.Sprintf("hashMapSet(%s, %s, ", getExpr, levels[i].keyExpr)
			}

			// Innermost: hashMapSet(getExpr(all levels), finalKey, [value will be emitted next]
			getExpr := je.generateNestedMapGetExpr(rootVar, levels)
			code += fmt.Sprintf("hashMapSet(%s, %s, ", getExpr, finalKey)

			je.emitToPackage(code)
			return
		}
		// Handle slice-of-map assignment: sliceOfMaps[i][j]...["key"] = v
		// For slice[0][1]["x"] = v, generates:
		// slice.get(0).set(1, hashMapSet(slice.get(0).get(1), "x", v));
		if je.isSliceOfMapAssign {
			je.suppressMapAssignLhs = false // Reset LHS suppression
			indices := je.sliceOfMapSliceIndices
			rootVar := je.sliceOfMapSliceVar

			var code string
			if len(indices) == 1 {
				// Simple case: slice[0]["key"] = v
				code = fmt.Sprintf("%s.set(%s, hashMapSet(%s.get(%s), %s, ",
					rootVar, indices[0],
					rootVar, indices[0],
					je.sliceOfMapMapKey)
			} else {
				// Multi-level case: slice[0][1]...["key"] = v
				// First, generate the get chain for all but the last index
				getChainBeforeLast := je.generateSliceGetChain(rootVar, indices[:len(indices)-1])
				// Then, generate the full get chain for the map access
				fullGetChain := je.generateSliceGetChain(rootVar, indices)
				// Generate: getChainBeforeLast.set(lastIdx, hashMapSet(fullGetChain, key,
				lastIdx := indices[len(indices)-1]
				code = fmt.Sprintf("%s.set(%s, hashMapSet(%s, %s, ",
					getChainBeforeLast, lastIdx,
					fullGetChain,
					je.sliceOfMapMapKey)
			}
			je.emitToPackage(code)
			return
		}
		// For slice index assignment: emit ", " between index and value
		if je.isSliceIndexAssign {
			je.emitToPackage(", ")
			return
		}
		str := je.emitAsString(" ", 0)
		je.emitToPackage(str)
		if node.Tok == token.DEFINE {
			if je.isClosureCapturedDecl {
				str = je.emitAsString("= {", 0)
			} else {
				str = je.emitAsString("= ", 0)
			}
		} else {
			str = je.emitAsString(node.Tok.String()+" ", 0)
		}
		je.emitToPackage(str)
		// Emit cast prefix for byte/short arithmetic
		if je.needsSmallIntCast {
			je.emitToPackage(fmt.Sprintf("(%s)(", je.smallIntCastType))
		}
	})
}

func (je *JavaEmitter) PostVisitAssignStmtRhs(node *ast.AssignStmt, indent int) {
	// Close closure-captured variable array wrapper
	if je.isClosureCapturedDecl {
		je.emitToPackage("}")
		je.isClosureCapturedDecl = false
		je.closureCapturedDeclName = ""
	}
	// Close byte/short arithmetic cast
	if je.needsSmallIntCast {
		je.emitToPackage(")")
		je.needsSmallIntCast = false
		je.smallIntCastType = ""
	}
	if je.isMapAssign {
		je.emitToPackage(");\n")
	}
	// For slice index assignment: emit closing );\n
	if je.isSliceIndexAssign {
		je.emitToPackage(");\n")
	}
}

func (je *JavaEmitter) PostVisitAssignStmtRhsExpr(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitAssignStmtLhsExpr(node ast.Expr, index int, indent int) {
	if index > 0 && !je.isMapCommaOk && !je.isTypeAssertCommaOk && !je.isMultiReturnAssign {
		je.executeIfNotForwardDecls(func() {
			str := je.emitAsString(", ", 0)
			je.emitToPackage(str)
		})
	}
}

func (je *JavaEmitter) PreVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	// Track that we're in assignment LHS (skip cast for closure-captured vars)
	je.inAssignLhs = true
	if je.isMapCommaOk || je.isTypeAssertCommaOk || je.isMapAssign || je.isMultiReturnAssign {
		return
	}
	je.executeIfNotForwardDecls(func() {
		if node.Tok == token.DEFINE {
			// Check if any LHS variable is already declared
			allNew := true
			for _, lhs := range node.Lhs {
				if ident, ok := lhs.(*ast.Ident); ok {
					if ident.Name != "_" && je.isVarDeclared(ident.Name) {
						allNew = false
						break
					}
				}
			}
			if allNew {
				// Check if any LHS variable is a closure-captured mutable variable
				isClosureCaptured := false
				if je.closureCapturedMutVars != nil && len(node.Lhs) == 1 {
					if ident, ok := node.Lhs[0].(*ast.Ident); ok {
						if je.closureCapturedMutVars[ident.Name] {
							isClosureCaptured = true
							je.isClosureCapturedDecl = true
							je.closureCapturedDeclName = ident.Name
						}
					}
				}

				// Check if RHS is a function literal - if so, emit the functional interface type
				// instead of var (Java can't infer lambda types from var)
				isFuncLit := false
				var funcLitType string
				if len(node.Rhs) == 1 {
					if funcLit, ok := node.Rhs[0].(*ast.FuncLit); ok {
						isFuncLit = true
						// Determine the functional interface type based on params and return
						numParams := 0
						hasReturn := false
						if funcLit.Type.Params != nil {
							numParams = len(funcLit.Type.Params.List)
						}
						if funcLit.Type.Results != nil && len(funcLit.Type.Results.List) > 0 {
							hasReturn = true
						}
						if !hasReturn {
							funcLitType = "Runnable"
						} else {
							funcLitType = "Supplier<Object>"
						}
						_ = numParams // Future: could use Consumer, Function, etc. for params
					}
				}

				if isClosureCaptured {
					// Use Object[] for closure-captured mutable variables
					str := je.emitAsString("Object[] ", 0)
					je.emitToPackage(str)
					// Set inDeclName to prevent [0] suffix on the variable name
					je.inDeclName = true
				} else if isFuncLit {
					str := je.emitAsString(funcLitType+" ", 0)
					je.emitToPackage(str)
				} else {
					str := je.emitAsString("var ", 0)
					je.emitToPackage(str)
				}
				// Mark variables as declared
				for _, lhs := range node.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok {
						if ident.Name != "_" {
							je.declareVar(ident.Name)
						}
					}
				}
			}
		}
	})
}

func (je *JavaEmitter) PostVisitAssignStmtLhs(node *ast.AssignStmt, indent int) {
	// Reset assignment LHS flag
	je.inAssignLhs = false
	// Reset declaration name flag (for closure-captured short var declarations)
	je.inDeclName = false
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitIndexExpr(node *ast.IndexExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isMapAssign || je.isMapCommaOk || je.isNestedMapAssign || je.isSliceOfMapAssign {
			return
		}
		// Push current map index state to stack before processing this expression
		// This allows nested index expressions to restore the outer state when done
		je.mapIndexStack = append(je.mapIndexStack, mapIndexStackEntry{
			isMapIndex:        je.isMapIndex,
			mapIndexValueType: je.mapIndexValueType,
			mapReadVarName:    je.mapReadVarName,
			mapKeyCastPrefix:  je.mapKeyCastPrefix,
			mapKeyCastSuffix:  je.mapKeyCastSuffix,
		})
		// Reset state for this expression
		je.isMapIndex = false
		je.mapIndexValueType = ""
		je.mapReadVarName = ""
		je.mapKeyCastPrefix = ""
		je.mapKeyCastSuffix = ""

		// Check if this is a map index
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			tv := je.pkg.TypesInfo.Types[node.X]
			if tv.Type != nil {
				if mapType, ok := tv.Type.Underlying().(*types.Map); ok {
					je.isMapIndex = true
					je.mapIndexValueType = getJavaTypeName(mapType.Elem())
					je.mapKeyCastPrefix, je.mapKeyCastSuffix = getJavaKeyCast(mapType.Key())
					// For nested map access, emit ((Type)hashMapGet( BEFORE X is traversed
					// This way the X expression (which could be another IndexExpr) is emitted inside
					boxedType := toBoxedType(je.mapIndexValueType)
					je.emitToPackage(fmt.Sprintf("((%s)hashMapGet(", boxedType))
					// Store the map variable name - but don't suppress X emission for nested cases
					je.mapReadVarName = exprToString(node.X)
					// Only suppress X if it's a simple identifier (not another IndexExpr)
					if _, isIndexExpr := node.X.(*ast.IndexExpr); !isIndexExpr {
						je.suppressMapIndexX = true
					}
				}
			}
		}
	})
}

func (je *JavaEmitter) PostVisitIndexExprX(node *ast.IndexExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isMapAssign || je.isMapCommaOk || je.isNestedMapAssign || je.isSliceOfMapAssign {
			return
		}
		if je.isUnsupportedIndexedCall {
			return
		}
		if je.isMapIndex {
			je.suppressMapIndexX = false // Reset
			je.currentMapGetValueType = je.mapIndexValueType
			// The ((Type)hashMapGet( was already emitted in PreVisitIndexExpr
			// Check if X was a nested IndexExpr (already emitted) or a simple ident (needs emitting)
			if _, isIndexExpr := node.X.(*ast.IndexExpr); isIndexExpr {
				// X was a nested IndexExpr, it has been emitted, just add comma
				je.emitToPackage(", ")
			} else {
				// X was a simple expression, emit it then comma
				je.emitToPackage(je.mapReadVarName + ", ")
			}
			return
		}
		// Check if base is a string - use charAt instead of get
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			tv := je.pkg.TypesInfo.Types[node.X]
			if tv.Type != nil {
				if basic, ok := tv.Type.Underlying().(*types.Basic); ok {
					if basic.Kind() == types.String {
						je.emitToken(".charAt(", LeftBracket, 0)
						return
					}
				}
			}
		}
		// For slice index assignment: x[i] = v -> x.set(i, v)
		// Only the actual LHS index expression should use .set(), nested indices use .get()
		if je.isSliceIndexAssign && node == je.sliceIndexLhsExpr {
			je.emitToken(".set(", LeftBracket, 0)
			return
		}
		je.emitToken(".get(", LeftBracket, 0)
	})
}

func (je *JavaEmitter) PostVisitIndexExpr(node *ast.IndexExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isMapAssign || je.isMapCommaOk || je.isNestedMapAssign || je.isSliceOfMapAssign {
			return
		}
		// Reset map read state
		je.suppressMapIndexX = false
		je.mapReadVarName = ""
	})
}

func (je *JavaEmitter) PreVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isMapAssign || je.isMapCommaOk || je.isNestedMapAssign || je.isSliceOfMapAssign {
			return
		}
		if je.isUnsupportedIndexedCall {
			return
		}
		if je.isMapIndex {
			// Map variable already emitted in PostVisitIndexExprX, just emit key cast if needed
			if je.mapKeyCastPrefix != "" {
				je.emitToPackage(je.mapKeyCastPrefix)
			}
		}
	})
}

func (je *JavaEmitter) PostVisitIndexExprIndex(node *ast.IndexExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isMapAssign || je.isMapCommaOk || je.isNestedMapAssign || je.isSliceOfMapAssign {
			return
		}
		if je.isUnsupportedIndexedCall {
			return
		}
		if je.isMapIndex {
			if je.mapKeyCastSuffix != "" {
				je.emitToPackage(je.mapKeyCastSuffix)
			}
			// Close both hashMapGet() and the cast wrapper
			je.emitToPackage("))")
		} else {
			// For slice index assignment, don't emit ) here - it will be emitted after the value
			// Only suppress for the actual LHS index expression
			if je.isSliceIndexAssign && node == je.sliceIndexLhsExpr {
				// Don't pop stack here - let the outer handler deal with it
			} else {
				je.emitToken(")", RightBracket, 0)
			}
		}

		// Pop and restore previous map index state from stack
		if len(je.mapIndexStack) > 0 {
			prevState := je.mapIndexStack[len(je.mapIndexStack)-1]
			je.mapIndexStack = je.mapIndexStack[:len(je.mapIndexStack)-1]
			je.isMapIndex = prevState.isMapIndex
			je.mapIndexValueType = prevState.mapIndexValueType
			je.mapReadVarName = prevState.mapReadVarName
			je.mapKeyCastPrefix = prevState.mapKeyCastPrefix
			je.mapKeyCastSuffix = prevState.mapKeyCastSuffix
		}
		je.currentMapGetValueType = ""
	})
}

func (je *JavaEmitter) PreVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		// Check for string comparison (== or !=)
		if node.Op == token.EQL || node.Op == token.NEQ {
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				// Check if LHS is a string
				if tv, ok := je.pkg.TypesInfo.Types[node.X]; ok {
					if tv.Type != nil && tv.Type.String() == "string" {
						je.isStringComparison = true
						je.stringComparisonOp = node.Op.String()
						je.stringComparisonLhsExpr = exprToString(node.X)
						// For !=, emit "!"
						if node.Op == token.NEQ {
							je.emitToPackage("!")
						}
						// Emit opening paren to wrap the LHS for proper precedence
						// This ensures (String)a becomes ((String)a).equals() not (String)(a.equals())
						je.emitToPackage("(")
						return
					}
				}
			}
		}
	})
}

func (je *JavaEmitter) PostVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		// Close .equals() for string comparison
		if je.isStringComparison {
			je.emitToPackage(")")
			je.isStringComparison = false
			je.stringComparisonOp = ""
			je.stringComparisonLhsExpr = ""
		}
	})
}

func (je *JavaEmitter) PreVisitBinaryExprOperator(op token.Token, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		// For string comparison, close LHS paren and emit .equals(
		if je.isStringComparison {
			je.emitToPackage(").equals(")
			return
		}
		str := je.emitAsString(" "+op.String()+" ", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitCallExprArg(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isUnsupportedIndexedCall {
			return
		}
		if je.isMultiReturnAssign {
			return
		}
		if je.isSliceMakeCall {
			if index == 0 {
				// Skip the type argument - mark that we're in the type arg
				je.inTypeContext = true
				return
			}
			if index == 1 {
				// Use MakeSlice helper to create ArrayList with actual elements
				if je.sliceMakeElemType == "boolean" || je.sliceMakeElemType == "Boolean" {
					je.emitToPackage("SliceBuiltins.MakeBoolSlice(")
				} else {
					je.emitToPackage("SliceBuiltins.<"+toBoxedType(je.sliceMakeElemType)+">MakeSlice(")
				}
				return
			}
		}
		if index > 0 {
			str := je.emitAsString(", ", 0)
			je.emitToPackage(str)
		}
		// For append call with struct argument, wrap with copy constructor
		if je.isAppendCall && index > 0 && je.appendStructType != "" {
			je.emitToPackage("new " + je.appendStructType + "(")
		}
		// Add cast for byte/short parameters when passing integer literals/constants
		if index < len(je.callExprParamTypes) {
			paramType := je.callExprParamTypes[index]
			// Check if argument needs cast (integer literal or constant to byte/short)
			needsCast := false
			if _, isLit := node.(*ast.BasicLit); isLit {
				needsCast = true
			} else if unary, isUnary := node.(*ast.UnaryExpr); isUnary {
				// Handle -1, +1, etc.
				if _, isLit := unary.X.(*ast.BasicLit); isLit {
					needsCast = true
				}
			} else if ident, isIdent := node.(*ast.Ident); isIdent {
				// Check if it's a constant
				if je.pkg != nil && je.pkg.TypesInfo != nil {
					if obj := je.pkg.TypesInfo.ObjectOf(ident); obj != nil {
						if _, isConst := obj.(*types.Const); isConst {
							needsCast = true
						}
					}
				}
			}
			if needsCast {
				switch paramType {
				case "byte":
					je.emitToPackage("(byte)(")
				case "short":
					je.emitToPackage("(short)(")
				}
			}
		}
	})
}

func (je *JavaEmitter) PostVisitCallExprArg(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isSliceMakeCall && index == 0 {
			// Clear type context after first argument
			je.inTypeContext = false
		}
		// Close copy constructor for append with struct
		if je.isAppendCall && index > 0 && je.appendStructType != "" {
			je.emitToPackage(")")
		}
		// Close cast for byte/short parameters
		if index < len(je.callExprParamTypes) {
			paramType := je.callExprParamTypes[index]
			// Check if argument was cast
			needsCast := false
			if _, isLit := node.(*ast.BasicLit); isLit {
				needsCast = true
			} else if unary, isUnary := node.(*ast.UnaryExpr); isUnary {
				if _, isLit := unary.X.(*ast.BasicLit); isLit {
					needsCast = true
				}
			} else if ident, isIdent := node.(*ast.Ident); isIdent {
				if je.pkg != nil && je.pkg.TypesInfo != nil {
					if obj := je.pkg.TypesInfo.ObjectOf(ident); obj != nil {
						if _, isConst := obj.(*types.Const); isConst {
							needsCast = true
						}
					}
				}
			}
			if needsCast && (paramType == "byte" || paramType == "short") {
				je.emitToPackage(")")
			}
		}
	})
}

func (je *JavaEmitter) PostVisitExprStmtX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(";", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// If there's an Init, don't emit "if " here; let Init be a separate statement
		if node.Init != nil {
			je.hasIfInit = true
			je.ifInitIndent = indent
			return
		}
		je.hasIfInit = false
		str := je.emitAsString("if ", indent)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PostVisitIfStmt(node *ast.IfStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitIfStmtInit(node ast.Stmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Init will be emitted as a regular statement
	})
}

func (je *JavaEmitter) PostVisitIfStmtInit(node ast.Stmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// After init, emit newline - the statement already has its own semicolon
		je.emitToPackage("\n")
	})
}

func (je *JavaEmitter) PreVisitIfStmtCond(node *ast.IfStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// If there was an Init, emit "if " now with proper indentation
		if je.hasIfInit {
			str := je.emitAsString("if (", je.ifInitIndent)
			je.emitToPackage(str)
			je.hasIfInit = false // Reset flag after use
		} else {
			str := je.emitAsString("(", 0)
			je.emitToPackage(str)
		}
	})
}

func (je *JavaEmitter) PostVisitIfStmtCond(node *ast.IfStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(") ", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitForStmt(node *ast.ForStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Push a scope for for-loop init variables (these are scoped to the for statement)
		je.declaredVarsStack = append(je.declaredVarsStack, make(map[string]bool))

		je.isInfiniteLoop = node.Init == nil && node.Cond == nil && node.Post == nil
		if je.isInfiniteLoop {
			str := je.emitAsString("while (true) ", indent)
			je.emitToPackage(str)
		} else {
			str := je.emitAsString("for (", indent)
			je.emitToPackage(str)
		}
	})
}

func (je *JavaEmitter) PreVisitForStmtInit(node ast.Stmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.insideForInit = true
	})
}

func (je *JavaEmitter) PostVisitForStmtInit(node ast.Stmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.insideForInit = false
		if je.isInfiniteLoop {
			return
		}
		str := je.emitAsString("; ", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitForStmtPost(node ast.Stmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.insideForPostCond = true
	})
}

func (je *JavaEmitter) PostVisitForStmtPost(node ast.Stmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.insideForPostCond = false
		if je.isInfiniteLoop {
			return
		}
		str := je.emitAsString(") ", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitIfStmtElse(node *ast.IfStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(" else ", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PostVisitForStmtCond(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isInfiniteLoop {
			return
		}
		str := je.emitAsString("; ", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PostVisitForStmt(node *ast.ForStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.isInfiniteLoop = false
		// Pop the for-loop scope
		if len(je.declaredVarsStack) > 0 {
			je.declaredVarsStack = je.declaredVarsStack[:len(je.declaredVarsStack)-1]
		}
	})
}

func (je *JavaEmitter) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Push a scope for range loop variables
		je.declaredVarsStack = append(je.declaredVarsStack, make(map[string]bool))

		je.rangeStmtIndent = indent
		je.isKeyValueRange = false
		je.rangeKeyName = ""
		je.rangeValueName = ""
		je.rangeCollectionExpr = ""

		// Determine if key-value or index-only range
		// Handle key (index)
		if node.Key != nil {
			if keyIdent, ok := node.Key.(*ast.Ident); ok {
				if keyIdent.Name != "_" {
					je.rangeKeyName = keyIdent.Name
				}
			}
		}
		// Handle value
		if node.Value != nil {
			if valIdent, ok := node.Value.(*ast.Ident); ok {
				if valIdent.Name != "_" {
					je.rangeValueName = valIdent.Name
					je.isKeyValueRange = true
				}
			}
		}

		str := je.emitAsString("for (", indent)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitRangeStmtKey(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressRangeEmit = true
	})
}

func (je *JavaEmitter) PostVisitRangeStmtKey(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitRangeStmtValue(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PostVisitRangeStmtValue(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressRangeEmit = false
	})
}

func (je *JavaEmitter) PreVisitRangeStmtX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.captureRangeExpr = true
		je.rangeCollectionExpr = ""
	})
}

func (je *JavaEmitter) PostVisitRangeStmtX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.captureRangeExpr = false

		// Check if ranging over a string - need to use .toCharArray() for Java
		collectionExpr := je.rangeCollectionExpr
		isStringRange := false
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			if t := je.pkg.TypesInfo.TypeOf(node); t != nil {
				if basic, ok := t.Underlying().(*types.Basic); ok {
					if basic.Kind() == types.String {
						isStringRange = true
						collectionExpr = je.rangeCollectionExpr + ".toCharArray()"
					}
				}
			}
		}

		if je.isKeyValueRange && je.rangeKeyName != "" {
			// For key-value range: for (int key = 0; key < collection.size(); key++)
			sizeMethod := ".size()"
			if isStringRange {
				sizeMethod = ".length()"
			}
			je.emitToPackage(fmt.Sprintf("int %s = 0; %s < %s%s; %s++) ",
				je.rangeKeyName, je.rangeKeyName, je.rangeCollectionExpr, sizeMethod, je.rangeKeyName))
			je.pendingRangeValueDecl = true
			je.pendingValueName = je.rangeValueName
			je.pendingCollectionExpr = je.rangeCollectionExpr
			je.pendingKeyName = je.rangeKeyName
		} else if je.rangeKeyName != "" && je.rangeValueName == "" {
			// Index-only range: for (int i = 0; i < collection.size(); i++)
			sizeMethod := ".size()"
			if isStringRange {
				sizeMethod = ".length()"
			}
			je.emitToPackage(fmt.Sprintf("int %s = 0; %s < %s%s; %s++) ",
				je.rangeKeyName, je.rangeKeyName, je.rangeCollectionExpr, sizeMethod, je.rangeKeyName))
		} else if je.rangeValueName != "" {
			// Value-only range (key is "_" or not used): for (var val : collection)
			je.emitToPackage(fmt.Sprintf("var %s : %s) ", je.rangeValueName, collectionExpr))
		} else {
			// Neither key nor value specified: for (var __item : collection)
			je.emitToPackage(fmt.Sprintf("var __item : %s) ", collectionExpr))
		}
	})
}

func (je *JavaEmitter) PostVisitRangeStmt(node *ast.RangeStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.isKeyValueRange = false
		// Pop the range loop scope
		if len(je.declaredVarsStack) > 0 {
			je.declaredVarsStack = je.declaredVarsStack[:len(je.declaredVarsStack)-1]
		}
	})
}

func (je *JavaEmitter) PostVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		if !je.insideForPostCond {
			str := je.emitAsString(node.Tok.String(), 0)
			je.emitToPackage(str)
			str = je.emitAsString(";", 0)
			je.emitToPackage(str)
		} else {
			str := je.emitAsString(node.Tok.String(), 0)
			je.emitToPackage(str)
		}
	})
}

func (je *JavaEmitter) PreVisitCompositeLit(node *ast.CompositeLit, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Push a new state to the stack for this composite literal
		state := structLitState{
			isStructLit:  false,
			buffer:       &strings.Builder{},
			fieldBuffers: make(map[string]string),
		}

		// Check if any element is a key-value expression (partial struct initialization)
		hasKeyValue := false
		for _, elt := range node.Elts {
			if _, ok := elt.(*ast.KeyValueExpr); ok {
				hasKeyValue = true
				break
			}
		}

		if hasKeyValue && je.pkg != nil && je.pkg.TypesInfo != nil {
			// Get the type of the composite literal
			tv := je.pkg.TypesInfo.Types[node]
			if tv.Type != nil {
				// Check if it's a struct type
				if structType, ok := tv.Type.Underlying().(*types.Struct); ok {
					state.isStructLit = true
					state.structLitType = tv.Type
					state.fieldOrder = make([]string, structType.NumFields())
					state.fieldTypes = make(map[string]string)

					// Build the field order and types
					for i := 0; i < structType.NumFields(); i++ {
						field := structType.Field(i)
						state.fieldOrder[i] = field.Name()
						state.fieldTypes[field.Name()] = getJavaTypeName(field.Type())
					}
				}
			}
		}

		je.structLitStack = append(je.structLitStack, state)
	})
}

func (je *JavaEmitter) PostVisitCompositeLit(node *ast.CompositeLit, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Pop from the struct literal stack
		if len(je.structLitStack) > 0 {
			je.structLitStack = je.structLitStack[:len(je.structLitStack)-1]
		}
	})
}

func (je *JavaEmitter) PreVisitCompositeLitType(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("new ", 0)
		je.emitToPackage(str)
		// Set type context so type names get proper package prefix
		je.inTypeContext = true
		je.inCompositeLitType = true

		// Check if the type is a package-qualified slice/map type alias
		// If so, emit the resolved type directly and suppress normal selector emission
		if _, ok := node.(*ast.SelectorExpr); ok {
			if je.pkg != nil && je.pkg.TypesInfo != nil {
				typeAndValue := je.pkg.TypesInfo.Types[node]
				if typeAndValue.Type != nil {
					underlyingType := typeAndValue.Type.Underlying()
					if _, ok := underlyingType.(*types.Slice); ok {
						// Slice type alias - emit ArrayList<elemType> directly
						javaType := getJavaTypeName(typeAndValue.Type)
						je.emitToPackage(javaType)
						je.suppressTypeAliasSelectorX = true
						je.inTypeContext = false // Skip normal type emission
					} else if _, ok := underlyingType.(*types.Map); ok {
						// Map type alias - emit HashMap<K,V> directly
						javaType := getJavaTypeName(typeAndValue.Type)
						je.emitToPackage(javaType)
						je.suppressTypeAliasSelectorX = true
						je.inTypeContext = false // Skip normal type emission
					}
				}
			}
		}
	})
}

func (je *JavaEmitter) PostVisitCompositeLitType(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inTypeContext = false
		je.inCompositeLitType = false
		je.suppressTypeAliasSelectorX = false // Reset after composite literal type
		if je.isArray {
			str := je.emitAsString("(Arrays.asList", 0)
			je.emitToPackage(str)
		} else {
			str := je.emitAsString("(", 0)
			je.emitToPackage(str)
		}
	})
}

func (je *JavaEmitter) PreVisitCompositeLitElts(node []ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isArray {
			str := je.emitAsString("(", 0)
			je.emitToPackage(str)
		}
		// Enable capturing mode for struct literals with key-value pairs (use stack)
		if len(je.structLitStack) > 0 {
			top := &je.structLitStack[len(je.structLitStack)-1]
			if top.isStructLit {
				top.capturing = true
				top.buffer.Reset()
			}
		}
	})
}

func (je *JavaEmitter) PostVisitCompositeLitElts(node []ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isArray {
			str := je.emitAsString("))", 0)
			je.emitToPackage(str)
			je.isArray = false
			je.sliceLitElemCast = "" // Reset after processing slice literal
		} else if len(je.structLitStack) > 0 {
			top := &je.structLitStack[len(je.structLitStack)-1]
			if top.isStructLit && top.capturing {
				// Stop capturing and emit the properly ordered constructor arguments
				top.capturing = false

				// Build the argument list in field order with default values for missing fields
				var args []string
				for _, fieldName := range top.fieldOrder {
					fieldType := top.fieldTypes[fieldName]
					if val, exists := top.fieldBuffers[fieldName]; exists {
						// Add cast for byte/short fields if the value needs it
						if needsByteCast(fieldType, val) {
							val = fmt.Sprintf("(%s)(%s)", fieldType, val)
						}
						args = append(args, val)
					} else {
						// Field not provided - emit default value based on type
						defaultVal := getJavaDefaultValue(fieldType)
						args = append(args, defaultVal)
					}
				}

				// Emit all arguments with commas
				str := je.emitAsString(strings.Join(args, ", "), 0)
				je.emitToPackage(str)
				str = je.emitAsString(")", 0)
				je.emitToPackage(str)
			} else {
				str := je.emitAsString(")", 0)
				je.emitToPackage(str)
			}
		} else {
			str := je.emitAsString(")", 0)
			je.emitToPackage(str)
		}
	})
}

func (je *JavaEmitter) PreVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		// When capturing struct literal elements (check stack), reset buffer for each element
		// and skip comma emission (we'll handle commas in PostVisitCompositeLitElts)
		if len(je.structLitStack) > 0 {
			top := &je.structLitStack[len(je.structLitStack)-1]
			if top.capturing {
				top.buffer.Reset()
				return
			}
		}
		if index > 0 {
			str := je.emitAsString(", ", 0)
			je.emitToPackage(str)
		}
		// Emit cast prefix for byte/short slice literal elements
		if je.sliceLitElemCast != "" && je.isArray {
			// Only apply cast to BasicLit (numeric literals)
			if _, ok := node.(*ast.BasicLit); ok {
				je.emitToPackage(je.sliceLitElemCast)
			}
		}
	})
}

func (je *JavaEmitter) PostVisitCompositeLitElt(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		// When capturing struct literal elements (check stack), save the buffer for the current key
		if len(je.structLitStack) > 0 {
			top := &je.structLitStack[len(je.structLitStack)-1]
			if top.capturing && top.currentKey != "" {
				top.fieldBuffers[top.currentKey] = top.buffer.String()
				top.currentKey = "" // Reset for next element
			}
		}
	})
}

func (je *JavaEmitter) PreVisitKeyValueExpr(node *ast.KeyValueExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Capture the key name for struct literal element mapping (use stack)
		if len(je.structLitStack) > 0 {
			top := &je.structLitStack[len(je.structLitStack)-1]
			if top.capturing {
				if ident, ok := node.Key.(*ast.Ident); ok {
					top.currentKey = ident.Name
				}
			}
		}
	})
}

func (je *JavaEmitter) PreVisitSliceExprX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		// Capture the X expression name for later use (for .size() call when High is nil)
		je.sliceExprXName = exprToString(node)
		// Wrap subList with new ArrayList<>() to convert List back to ArrayList
		je.emitToPackage("new ArrayList<>(")
	})
}

func (je *JavaEmitter) PostVisitSliceExprX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		str := je.emitAsString(".subList(", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitSliceExprXBegin(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress the second X traversal
		je.suppressRangeEmit = true
	})
}

func (je *JavaEmitter) PostVisitSliceExprXBegin(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressRangeEmit = false
	})
}

func (je *JavaEmitter) PreVisitSliceExprLow(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		// If Low is nil (like a[:high]), emit 0
		if node == nil {
			je.emitToPackage("0")
		}
	})
}

func (je *JavaEmitter) PostVisitSliceExprLow(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		str := je.emitAsString(", ", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitSliceExprXEnd(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress the third X traversal
		je.suppressRangeEmit = true
	})
}

func (je *JavaEmitter) PostVisitSliceExprXEnd(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressRangeEmit = false
	})
}

func (je *JavaEmitter) PreVisitSliceExprHigh(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		// If High is nil (like a[low:]), emit X.size()
		if node == nil {
			je.emitToPackage(je.sliceExprXName+".size()")
		}
	})
}

func (je *JavaEmitter) PostVisitSliceExpr(node *ast.SliceExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress during multi-return assignment (handled in PostVisitAssignStmt)
		if je.isMultiReturnAssign {
			return
		}
		// Close both subList() and the ArrayList wrapper
		str := je.emitAsString("))", 0)
		je.emitToPackage(str)
		je.sliceExprXName = "" // Reset
	})
}

func (je *JavaEmitter) PreVisitFuncLit(node *ast.FuncLit, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inLambdaParams = true
		je.emitToken("(", LeftParen, 0)
	})
}

func (je *JavaEmitter) PostVisitFuncLit(node *ast.FuncLit, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PostVisitFuncLitTypeParams(node *ast.FieldList, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inLambdaParams = false
		je.emitToken(") -> ", RightParen, 0)
	})
}

func (je *JavaEmitter) PreVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if index > 0 {
			je.emitToken(", ", Comma, 0)
		}
		// Emit parameter name only for lambda (Java uses type inference)
		if len(node.Names) > 0 {
			paramName := node.Names[0].Name
			// Check if parameter name conflicts with an outer scope variable (Java doesn't allow shadowing in lambdas)
			if je.isVarDeclared(paramName) || je.isFuncParam(paramName) {
				// Append underscore to make the name unique
				paramName = paramName + "_"
			}
			je.emitToPackage(paramName)
		}
		// Set type context to suppress type emission
		je.inTypeContext = true
	})
}

func (je *JavaEmitter) PostVisitFuncLitTypeParam(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.inTypeContext = false
	})
}

func (je *JavaEmitter) PreVisitFuncLitBody(node *ast.BlockStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitFuncLitTypeResult(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Lambda return types are inferred in Java - suppress emission
		je.shouldGenerate = false
	})
}

func (je *JavaEmitter) PostVisitFuncLitTypeResult(node *ast.Field, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.shouldGenerate = true
	})
}

func (je *JavaEmitter) PreVisitInterfaceType(node *ast.InterfaceType, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Skip if we're inside a slice make call (the type is handled separately)
		if je.isSliceMakeCall {
			return
		}
		str := je.emitAsString("Object", indent)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitKeyValueExprKey(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Suppress struct field key - Java uses positional constructor args
		je.suppressKeyValueKey = true
	})
}

func (je *JavaEmitter) PostVisitKeyValueExprKey(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.suppressKeyValueKey = false
	})
}

func (je *JavaEmitter) PreVisitKeyValueExprValue(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(node.Op.String(), 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PostVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitParenExpr(node *ast.ParenExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.emitToPackage("(")
	})
}

func (je *JavaEmitter) PostVisitParenExpr(node *ast.ParenExpr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.emitToPackage(")")
	})
}

func (je *JavaEmitter) PreVisitGenDeclConstName(node *ast.Ident, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("public static final ", indent+2)
		je.emitToPackage(str)

		// Try to infer type from value
		if je.pkg != nil && je.pkg.TypesInfo != nil {
			if obj := je.pkg.TypesInfo.Defs[node]; obj != nil {
				typeName := getJavaTypeName(obj.Type())
				je.emitToPackage(typeName+" ")
			}
		}
		str = je.emitAsString(node.Name+" = ", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PostVisitGenDeclConstName(node *ast.Ident, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.emitToPackage(";\n")
	})
}

func (je *JavaEmitter) PostVisitGenDeclConst(node *ast.GenDecl, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("switch ", indent)
		je.emitToPackage(str)
		je.emitToken("(", LeftParen, 0)
	})
}

func (je *JavaEmitter) PostVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString("}\n", indent)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PostVisitSwitchStmtTag(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		je.emitToken(")", RightParen, 0)
		str := je.emitAsString(" {\n", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PostVisitCaseClause(node *ast.CaseClause, indent int) {
	je.executeIfNotForwardDecls(func() {
		// Add break after each case unless it's a fallthrough
		str := je.emitAsString("break;\n", indent+2)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PostVisitCaseClauseList(node []ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if len(node) == 0 {
			str := je.emitAsString("default:\n", indent)
			je.emitToPackage(str)
		}
	})
}

func (je *JavaEmitter) PreVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		if index == 0 {
			str := je.emitAsString("case ", indent)
			je.emitToPackage(str)
		}
	})
}

func (je *JavaEmitter) PostVisitCaseClauseListExpr(node ast.Expr, index int, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(":\n", 0)
		je.emitToPackage(str)
	})
}

func (je *JavaEmitter) PreVisitTypeAssertExprType(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isTypeAssertCommaOk {
			return
		}
		je.emitToken("(", LeftParen, 0)
		// Set type context so type names get proper package prefix
		je.inTypeContext = true
	})
}

func (je *JavaEmitter) PostVisitTypeAssertExprType(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
		if je.isTypeAssertCommaOk {
			return
		}
		je.inTypeContext = false
		je.emitToken(")", RightParen, 0)
	})
}

func (je *JavaEmitter) PreVisitTypeAssertExprX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PostVisitTypeAssertExprX(node ast.Expr, indent int) {
	je.executeIfNotForwardDecls(func() {
	})
}

func (je *JavaEmitter) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	je.executeIfNotForwardDecls(func() {
		str := je.emitAsString(strings.ToLower(node.Tok.String())+";", indent)
		je.emitToPackage(str)
	})
}

// Build file generation

func (je *JavaEmitter) GeneratePomXml() error {
	if je.LinkRuntime == "" {
		return nil
	}

	pomPath := filepath.Join(je.OutputDir, "pom.xml")
	file, err := os.Create(pomPath)
	if err != nil {
		return fmt.Errorf("failed to create pom.xml: %w", err)
	}
	defer file.Close()

	pom := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0"
         xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
         xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 http://maven.apache.org/xsd/maven-4.0.0.xsd">
    <modelVersion>4.0.0</modelVersion>

    <groupId>com.goany</groupId>
    <artifactId>%s</artifactId>
    <version>1.0-SNAPSHOT</version>
    <packaging>jar</packaging>

    <properties>
        <maven.compiler.source>17</maven.compiler.source>
        <maven.compiler.target>17</maven.compiler.target>
        <project.build.sourceEncoding>UTF-8</project.build.sourceEncoding>
    </properties>

    <build>
        <sourceDirectory>${project.basedir}</sourceDirectory>
        <plugins>
            <plugin>
                <groupId>org.apache.maven.plugins</groupId>
                <artifactId>maven-compiler-plugin</artifactId>
                <version>3.11.0</version>
                <configuration>
                    <source>17</source>
                    <target>17</target>
                </configuration>
            </plugin>
            <plugin>
                <groupId>org.apache.maven.plugins</groupId>
                <artifactId>maven-jar-plugin</artifactId>
                <version>3.3.0</version>
                <configuration>
                    <archive>
                        <manifest>
                            <mainClass>Api</mainClass>
                        </manifest>
                    </archive>
                </configuration>
            </plugin>
            <plugin>
                <groupId>org.codehaus.mojo</groupId>
                <artifactId>exec-maven-plugin</artifactId>
                <version>3.1.0</version>
                <configuration>
                    <mainClass>Api</mainClass>
                </configuration>
            </plugin>
        </plugins>
    </build>
</project>
`, je.OutputName)

	_, err = file.WriteString(pom)
	if err != nil {
		return fmt.Errorf("failed to write pom.xml: %w", err)
	}

	DebugLogPrintf("Generated pom.xml at %s", pomPath)
	return nil
}

func (je *JavaEmitter) CopyRuntimePackages() error {
	if je.LinkRuntime == "" {
		return nil
	}

	for name, variant := range je.RuntimePackages {
		if variant == "none" {
			continue
		}

		capName := strings.ToUpper(name[:1]) + name[1:]

		var srcFileName string
		if variant != "" {
			capVariant := strings.ToUpper(variant[:1]) + variant[1:]
			srcFileName = capName + "Runtime" + capVariant + ".java"
		} else {
			srcFileName = capName + "Runtime.java"
		}

		runtimeSrcPath := filepath.Join(je.LinkRuntime, name, "java", srcFileName)
		content, err := os.ReadFile(runtimeSrcPath)
		if err != nil {
			DebugLogPrintf("Skipping Java runtime for %s: %v", name, err)
			continue
		}

		dstFileName := capName + "Runtime.java"
		dstPath := filepath.Join(je.OutputDir, dstFileName)
		if err := os.WriteFile(dstPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", dstFileName, err)
		}
		DebugLogPrintf("Copied %s from %s to %s", dstFileName, runtimeSrcPath, dstPath)

		// For graphics with tigr variant, also copy native JNI files
		if name == "graphics" && variant == "tigr" {
			// Copy JNI C file
			jniSrc := filepath.Join(je.LinkRuntime, "graphics", "java", "graphics_jni.c")
			if jniContent, err := os.ReadFile(jniSrc); err == nil {
				jniDst := filepath.Join(je.OutputDir, "graphics_jni.c")
				if err := os.WriteFile(jniDst, jniContent, 0644); err != nil {
					return fmt.Errorf("failed to write graphics_jni.c: %w", err)
				}
				DebugLogPrintf("Copied graphics_jni.c")
			}

			// Copy Makefile
			makeSrc := filepath.Join(je.LinkRuntime, "graphics", "java", "Makefile")
			if makeContent, err := os.ReadFile(makeSrc); err == nil {
				makeDst := filepath.Join(je.OutputDir, "Makefile")
				if err := os.WriteFile(makeDst, makeContent, 0644); err != nil {
					return fmt.Errorf("failed to write Makefile: %w", err)
				}
				DebugLogPrintf("Copied Makefile")
			}

			// Copy TIGR files from cpp directory
			tigrFiles := []string{"tigr.c", "tigr.h", "screen_helper.c"}
			for _, file := range tigrFiles {
				src := filepath.Join(je.LinkRuntime, "graphics", "cpp", file)
				if fileContent, err := os.ReadFile(src); err == nil {
					dst := filepath.Join(je.OutputDir, file)
					if err := os.WriteFile(dst, fileContent, 0644); err != nil {
						return fmt.Errorf("failed to write %s: %w", file, err)
					}
					DebugLogPrintf("Copied %s", file)
				}
			}

			// Generate run.sh script with correct platform-specific flags
			runScript := `#!/bin/bash
# Run script for Java graphics application
# Automatically adds required JVM flags for each platform

MAIN_CLASS="${1:-` + je.OutputName + `}"

# Detect OS and set appropriate flags
case "$(uname -s)" in
    Darwin)
        # macOS requires -XstartOnFirstThread for GUI applications
        java -XstartOnFirstThread --enable-native-access=ALL-UNNAMED -Djava.library.path=. "$MAIN_CLASS"
        ;;
    Linux)
        java --enable-native-access=ALL-UNNAMED -Djava.library.path=. "$MAIN_CLASS"
        ;;
    MINGW*|MSYS*|CYGWIN*)
        java --enable-native-access=ALL-UNNAMED -Djava.library.path=. "$MAIN_CLASS"
        ;;
    *)
        echo "Unknown OS: $(uname -s)"
        java --enable-native-access=ALL-UNNAMED -Djava.library.path=. "$MAIN_CLASS"
        ;;
esac
`
			runScriptPath := filepath.Join(je.OutputDir, "run.sh")
			if err := os.WriteFile(runScriptPath, []byte(runScript), 0755); err != nil {
				return fmt.Errorf("failed to write run.sh: %w", err)
			}
			DebugLogPrintf("Generated run.sh")
		}
	}

	return nil
}
