package compiler

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// ReadOnlyAnalysis holds the results of read-only parameter analysis.
type ReadOnlyAnalysis struct {
	ReadOnly      map[string][]bool // funcKey → per-param read-only flags
	FuncsAsValues map[string]bool   // functions used as callbacks
}

// AnalyzeReadOnlyParams walks all functions in a package and determines
// which parameters are read-only (eligible for &T in Rust).
// A parameter is read-only if: it's non-Copy, not mutated, not returned, and not
// assigned as a whole value to another variable.
func AnalyzeReadOnlyParams(pkg *packages.Package) *ReadOnlyAnalysis {
	result := &ReadOnlyAnalysis{
		ReadOnly:      make(map[string][]bool),
		FuncsAsValues: CollectFuncsUsedAsValues(pkg),
	}

	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !ok || funcDecl.Type == nil || funcDecl.Type.Params == nil {
				continue
			}

			key := pkg.Name + "." + funcDecl.Name.Name

			// Skip functions used as callbacks (their signature must match the expected type)
			if result.FuncsAsValues[funcDecl.Name.Name] {
				continue
			}

			params := funcDecl.Type.Params.List
			body := funcDecl.Body

			// Collect mutated variables
			mutatedVars := make(map[string]bool)
			if body != nil {
				for _, stmt := range body.List {
					CollectMutatedVarsInStmt(stmt, mutatedVars)
				}
			}

			// Collect returned variables
			returnedVars := make(map[string]bool)
			if body != nil {
				for _, stmt := range body.List {
					CollectReturnedVarsInStmt(stmt, returnedVars)
				}
			}

			// Collect variables assigned as whole values
			assignedFromVars := make(map[string]bool)
			if body != nil {
				for _, stmt := range body.List {
					CollectAssignedFromVarsInStmt(stmt, pkg, assignedFromVars)
				}
			}

			// Build read-only flags for each parameter
			var readOnly []bool
			for _, field := range params {
				for _, name := range field.Names {
					paramName := name.Name
					// Check type: only struct/slice types benefit from &T
					tv := pkg.TypesInfo.Types[field.Type]
					isEligible := isRefOptEligibleType(tv.Type)
					isReadOnly := isEligible &&
						!mutatedVars[paramName] &&
						!returnedVars[paramName] &&
						!assignedFromVars[paramName]
					readOnly = append(readOnly, isReadOnly)
				}
			}
			result.ReadOnly[key] = readOnly
		}
	}
	return result
}

// CollectFuncsUsedAsValues finds function names that are used as values (not calls)
// in any expression in the package. Functions passed as callbacks cannot have their
// signatures changed by the optimization.
func CollectFuncsUsedAsValues(pkg *packages.Package) map[string]bool {
	result := make(map[string]bool)
	for _, file := range pkg.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			callExpr, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			for _, arg := range callExpr.Args {
				if ident, ok := arg.(*ast.Ident); ok {
					obj := pkg.TypesInfo.ObjectOf(ident)
					if obj != nil {
						if _, isFunc := obj.Type().(*types.Signature); isFunc {
							result[ident.Name] = true
						}
					}
				}
			}
			return true
		})
	}
	return result
}

// CollectMutatedVarsInStmt recursively collects variables that are assigned to (mutated) in a statement.
func CollectMutatedVarsInStmt(stmt ast.Stmt, result map[string]bool) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		if s.Tok == token.ASSIGN || s.Tok == token.ADD_ASSIGN || s.Tok == token.SUB_ASSIGN ||
			s.Tok == token.MUL_ASSIGN || s.Tok == token.QUO_ASSIGN || s.Tok == token.REM_ASSIGN {
			for _, lhs := range s.Lhs {
				CollectMutatedVarsInExpr(lhs, result)
			}
		}
	case *ast.IncDecStmt:
		CollectMutatedVarsInExpr(s.X, result)
	case *ast.IfStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				CollectMutatedVarsInStmt(bodyStmt, result)
			}
		}
		if s.Else != nil {
			CollectMutatedVarsInStmt(s.Else, result)
		}
	case *ast.ForStmt:
		if s.Post != nil {
			CollectMutatedVarsInStmt(s.Post, result)
		}
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				CollectMutatedVarsInStmt(bodyStmt, result)
			}
		}
	case *ast.RangeStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				CollectMutatedVarsInStmt(bodyStmt, result)
			}
		}
	case *ast.BlockStmt:
		for _, bodyStmt := range s.List {
			CollectMutatedVarsInStmt(bodyStmt, result)
		}
	case *ast.SwitchStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				CollectMutatedVarsInStmt(bodyStmt, result)
			}
		}
	case *ast.CaseClause:
		for _, bodyStmt := range s.Body {
			CollectMutatedVarsInStmt(bodyStmt, result)
		}
	}
}

// CollectMutatedVarsInExpr recursively collects variables that are assigned to in an expression.
func CollectMutatedVarsInExpr(expr ast.Expr, result map[string]bool) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Ident:
		result[e.Name] = true
	case *ast.IndexExpr:
		CollectMutatedVarsInExpr(e.X, result)
	case *ast.SelectorExpr:
		CollectMutatedVarsInExpr(e.X, result)
	}
}

// CollectReturnedVarsInStmt recursively finds variables directly returned from the function.
func CollectReturnedVarsInStmt(stmt ast.Stmt, result map[string]bool) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		for _, expr := range s.Results {
			if ident, ok := expr.(*ast.Ident); ok {
				result[ident.Name] = true
			}
		}
	case *ast.IfStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				CollectReturnedVarsInStmt(bodyStmt, result)
			}
		}
		if s.Else != nil {
			CollectReturnedVarsInStmt(s.Else, result)
		}
	case *ast.ForStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				CollectReturnedVarsInStmt(bodyStmt, result)
			}
		}
	case *ast.RangeStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				CollectReturnedVarsInStmt(bodyStmt, result)
			}
		}
	case *ast.BlockStmt:
		for _, bodyStmt := range s.List {
			CollectReturnedVarsInStmt(bodyStmt, result)
		}
	case *ast.SwitchStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				CollectReturnedVarsInStmt(bodyStmt, result)
			}
		}
	case *ast.CaseClause:
		for _, bodyStmt := range s.Body {
			CollectReturnedVarsInStmt(bodyStmt, result)
		}
	}
}

// CollectUsedAsValueVarsInExpr finds the root variable of an expression used as a value.
func CollectUsedAsValueVarsInExpr(expr ast.Expr, result map[string]bool) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Ident:
		result[e.Name] = true
	case *ast.SelectorExpr:
		CollectUsedAsValueVarsInExpr(e.X, result)
	}
}

// CollectAssignedFromVarsInStmt recursively finds variables used as standalone values on the RHS of assignments,
// struct literal field values, or other contexts requiring ownership.
func CollectAssignedFromVarsInStmt(stmt ast.Stmt, pkg *packages.Package, result map[string]bool) {
	if stmt == nil {
		return
	}
	ast.Inspect(stmt, func(n ast.Node) bool {
		if compLit, ok := n.(*ast.CompositeLit); ok {
			for _, elt := range compLit.Elts {
				if kv, ok := elt.(*ast.KeyValueExpr); ok {
					CollectUsedAsValueVarsInExpr(kv.Value, result)
				} else {
					CollectUsedAsValueVarsInExpr(elt, result)
				}
			}
		}
		return true
	})
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		for _, rhs := range s.Rhs {
			CollectUsedAsValueVarsInExpr(rhs, result)
		}
	case *ast.IfStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				CollectAssignedFromVarsInStmt(bodyStmt, pkg, result)
			}
		}
		if s.Else != nil {
			CollectAssignedFromVarsInStmt(s.Else, pkg, result)
		}
	case *ast.ForStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				CollectAssignedFromVarsInStmt(bodyStmt, pkg, result)
			}
		}
	case *ast.RangeStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				CollectAssignedFromVarsInStmt(bodyStmt, pkg, result)
			}
		}
	case *ast.BlockStmt:
		for _, bodyStmt := range s.List {
			CollectAssignedFromVarsInStmt(bodyStmt, pkg, result)
		}
	case *ast.SwitchStmt:
		if s.Body != nil {
			for _, bodyStmt := range s.Body.List {
				CollectAssignedFromVarsInStmt(bodyStmt, pkg, result)
			}
		}
	case *ast.CaseClause:
		for _, bodyStmt := range s.Body {
			CollectAssignedFromVarsInStmt(bodyStmt, pkg, result)
		}
	}
}

// ExprContainsIdent checks if an expression references a given identifier.
func ExprContainsIdent(expr ast.Expr, name string) bool {
	if expr == nil {
		return false
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name == name
	case *ast.SelectorExpr:
		return ExprContainsIdent(e.X, name)
	case *ast.CallExpr:
		for _, arg := range e.Args {
			if ExprContainsIdent(arg, name) {
				return true
			}
		}
		return ExprContainsIdent(e.Fun, name)
	case *ast.IndexExpr:
		return ExprContainsIdent(e.X, name) || ExprContainsIdent(e.Index, name)
	case *ast.BinaryExpr:
		return ExprContainsIdent(e.X, name) || ExprContainsIdent(e.Y, name)
	case *ast.UnaryExpr:
		return ExprContainsIdent(e.X, name)
	case *ast.ParenExpr:
		return ExprContainsIdent(e.X, name)
	case *ast.TypeAssertExpr:
		return ExprContainsIdent(e.X, name)
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			if ExprContainsIdent(elt, name) {
				return true
			}
		}
	case *ast.KeyValueExpr:
		return ExprContainsIdent(e.Value, name)
	}
	return false
}

// CollectCallArgIdentCounts counts identifier occurrences across all call arguments.
func CollectCallArgIdentCounts(args []ast.Expr, pkg *packages.Package) map[string]int {
	counts := make(map[string]int)
	for _, arg := range args {
		CountIdentsInExpr(arg, pkg, counts)
	}
	return counts
}

// CountIdentsInExpr recursively counts identifier occurrences in an expression.
func CountIdentsInExpr(expr ast.Expr, pkg *packages.Package, counts map[string]int) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Ident:
		if pkg != nil && pkg.TypesInfo != nil {
			if obj := pkg.TypesInfo.ObjectOf(e); obj != nil {
				if _, isTypeName := obj.(*types.TypeName); isTypeName {
					return
				}
				if _, isPkgName := obj.(*types.PkgName); isPkgName {
					return
				}
			}
		}
		counts[e.Name]++
	case *ast.SelectorExpr:
		CountIdentsInExpr(e.X, pkg, counts)
	case *ast.CallExpr:
		for _, arg := range e.Args {
			CountIdentsInExpr(arg, pkg, counts)
		}
		CountIdentsInExpr(e.Fun, pkg, counts)
	case *ast.IndexExpr:
		CountIdentsInExpr(e.X, pkg, counts)
		CountIdentsInExpr(e.Index, pkg, counts)
	case *ast.BinaryExpr:
		CountIdentsInExpr(e.X, pkg, counts)
		CountIdentsInExpr(e.Y, pkg, counts)
	case *ast.UnaryExpr:
		CountIdentsInExpr(e.X, pkg, counts)
	case *ast.ParenExpr:
		CountIdentsInExpr(e.X, pkg, counts)
	case *ast.TypeAssertExpr:
		CountIdentsInExpr(e.X, pkg, counts)
	case *ast.CompositeLit:
		for _, elt := range e.Elts {
			CountIdentsInExpr(elt, pkg, counts)
		}
	case *ast.KeyValueExpr:
		CountIdentsInExpr(e.Value, pkg, counts)
	}
}

// SubtractIdentsInExpr removes identifier occurrences found in expr from counts.
func SubtractIdentsInExpr(expr ast.Expr, pkg *packages.Package, counts map[string]int) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *ast.Ident:
		if pkg != nil && pkg.TypesInfo != nil {
			if obj := pkg.TypesInfo.ObjectOf(e); obj != nil {
				if _, isTypeName := obj.(*types.TypeName); isTypeName {
					return
				}
				if _, isPkgName := obj.(*types.PkgName); isPkgName {
					return
				}
			}
		}
		counts[e.Name]--
	case *ast.SelectorExpr:
		SubtractIdentsInExpr(e.X, pkg, counts)
	case *ast.IndexExpr:
		SubtractIdentsInExpr(e.X, pkg, counts)
		SubtractIdentsInExpr(e.Index, pkg, counts)
	case *ast.BinaryExpr:
		SubtractIdentsInExpr(e.X, pkg, counts)
		SubtractIdentsInExpr(e.Y, pkg, counts)
	case *ast.UnaryExpr:
		SubtractIdentsInExpr(e.X, pkg, counts)
	case *ast.ParenExpr:
		SubtractIdentsInExpr(e.X, pkg, counts)
	case *ast.CallExpr:
		for _, arg := range e.Args {
			SubtractIdentsInExpr(arg, pkg, counts)
		}
		SubtractIdentsInExpr(e.Fun, pkg, counts)
	}
}

// ExprContainsSelectorPath checks if an expression contains a selector path
// that matches exactly or starts with the given prefix (for sub-field access).
func ExprContainsSelectorPath(expr ast.Expr, exact string, prefix string) bool {
	if expr == nil {
		return false
	}
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if found {
			return false
		}
		if sel, ok := n.(*ast.SelectorExpr); ok {
			path := exprToString(sel)
			if path == exact || strings.HasPrefix(path, prefix) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

// isRefOptEligibleType checks if a Go type is eligible for &T optimization in Rust.
// Only struct types and slice types benefit from pass-by-reference.
func isRefOptEligibleType(t types.Type) bool {
	if t == nil {
		return false
	}
	switch t.Underlying().(type) {
	case *types.Struct:
		return true
	case *types.Slice:
		return true
	case *types.Basic:
		return false
	}
	return false
}
