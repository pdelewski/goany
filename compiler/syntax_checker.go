// syntax_checker.go provides GoAny syntax validation.
// It validates that all constructs in user code conform to GoAny's
// supported syntax (a subset of Go), using rules generated from tests/examples.
package compiler

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"strings"

	"golang.org/x/tools/go/packages"
)

// SyntaxChecker validates constructs against GoAny syntax rules
type SyntaxChecker struct {
	Emitter
	pkg       *packages.Package
	patternDB *PatternDatabase
}

// SetPatternDatabase sets the pattern database for validation
func (sc *SyntaxChecker) SetPatternDatabase(db *PatternDatabase) {
	sc.patternDB = db
}

// checkSyntax validates a construct and reports error if not supported
func (sc *SyntaxChecker) checkSyntax(p Pattern, pos token.Pos) {
	if sc.patternDB == nil {
		return
	}
	// Skip syntax checking for runtime packages (trusted internal code)
	if sc.pkg != nil && sc.pkg.Module != nil {
		modPath := sc.pkg.Module.Path
		if modPath == "runtime/hmap" || modPath == "runtime/std" {
			return
		}
	}
	if !sc.patternDB.HasPattern(p) {
		sc.reportUnsupportedConstruct(p, pos)
	}
}

// reportUnsupportedConstruct reports an unsupported construct and exits
func (sc *SyntaxChecker) reportUnsupportedConstruct(p Pattern, pos token.Pos) {
	var filename string
	var line, col int

	if sc.pkg != nil && sc.pkg.Fset != nil && pos.IsValid() {
		position := sc.pkg.Fset.Position(pos)
		filename = position.Filename
		line = position.Line
		col = position.Column
	}

	fmt.Printf("\033[31m\033[1mSyntax error: unsupported construct\033[0m\n")

	// Show the source code if we have location info
	if filename != "" && line > 0 {
		fmt.Printf("  \033[36m-->\033[0m %s:%d:%d\n", filename, line, col)

		// Read and display the source line
		if sourceLine := readSourceLine(filename, line); sourceLine != "" {
			fmt.Printf("   \033[90m%4d |\033[0m %s\n", line, sourceLine)

			// Add caret pointing to the column
			if col > 0 {
				padding := strings.Repeat(" ", col-1)
				fmt.Printf("   \033[90m     |\033[0m \033[31m%s^\033[0m\n", padding)
			}
		}
	}

	fmt.Printf("  \033[33mConstruct:\033[0m %s\n", p.NormalizedKey())
	fmt.Println("  This construct is not part of GoAny's supported syntax.")
	fmt.Println()
	os.Exit(-1)
}

// readSourceLine reads a specific line from a file
func readSourceLine(filename string, lineNum int) string {
	file, err := os.Open(filename)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	currentLine := 0
	for scanner.Scan() {
		currentLine++
		if currentLine == lineNum {
			return scanner.Text()
		}
	}
	return ""
}

func (sc *SyntaxChecker) PreVisitPackage(pkg *packages.Package, indent int) {
	sc.pkg = pkg
}

func (sc *SyntaxChecker) PreVisitForStmt(node *ast.ForStmt, indent int) {
	sc.checkSyntax(ExtractForStmtPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitRangeStmt(node *ast.RangeStmt, indent int) {
	sc.checkSyntax(ExtractRangeStmtPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitIfStmt(node *ast.IfStmt, indent int) {
	sc.checkSyntax(ExtractIfStmtPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitSwitchStmt(node *ast.SwitchStmt, indent int) {
	sc.checkSyntax(ExtractSwitchStmtPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitTypeSwitchStmt(node *ast.TypeSwitchStmt, indent int) {
	sc.checkSyntax(ExtractTypeSwitchStmtPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitAssignStmt(node *ast.AssignStmt, indent int) {
	sc.checkSyntax(ExtractAssignStmtPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitCallExpr(node *ast.CallExpr, indent int) {
	sc.checkSyntax(ExtractCallExprPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitCompositeLit(node *ast.CompositeLit, indent int) {
	sc.checkSyntax(ExtractCompositeLitPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitMapType(node *ast.MapType, indent int) {
	sc.checkSyntax(ExtractMapTypePattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitIndexExpr(node *ast.IndexExpr, indent int) {
	sc.checkSyntax(ExtractIndexExprPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitSliceExpr(node *ast.SliceExpr, indent int) {
	sc.checkSyntax(ExtractSliceExprPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitUnaryExpr(node *ast.UnaryExpr, indent int) {
	sc.checkSyntax(ExtractUnaryExprPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitBinaryExpr(node *ast.BinaryExpr, indent int) {
	sc.checkSyntax(ExtractBinaryExprPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitFuncDecl(node *ast.FuncDecl, indent int) {
	sc.checkSyntax(ExtractFuncDeclPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitFuncLit(node *ast.FuncLit, indent int) {
	sc.checkSyntax(ExtractFuncLitPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitReturnStmt(node *ast.ReturnStmt, indent int) {
	sc.checkSyntax(ExtractReturnStmtPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	sc.checkSyntax(ExtractBranchStmtPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitIncDecStmt(node *ast.IncDecStmt, indent int) {
	sc.checkSyntax(ExtractIncDecStmtPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitTypeAssertExpr(node *ast.TypeAssertExpr, indent int) {
	sc.checkSyntax(ExtractTypeAssertExprPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitStarExpr(node *ast.StarExpr, indent int) {
	sc.checkSyntax(ExtractStarExprPattern(node, sc.pkg), node.Pos())
}

// PreVisitArrayType uses value type (not pointer) per Emitter interface
func (sc *SyntaxChecker) PreVisitArrayType(node ast.ArrayType, indent int) {
	sc.checkSyntax(ExtractArrayTypePattern(&node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitStructType(node *ast.StructType, indent int) {
	sc.checkSyntax(ExtractStructTypePattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitInterfaceType(node *ast.InterfaceType, indent int) {
	sc.checkSyntax(ExtractInterfaceTypePattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitChanType(node *ast.ChanType, indent int) {
	sc.checkSyntax(ExtractChanTypePattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitFuncType(node *ast.FuncType, indent int) {
	sc.checkSyntax(ExtractFuncTypePattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitDeferStmt(node *ast.DeferStmt, indent int) {
	sc.checkSyntax(ExtractDeferStmtPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitGoStmt(node *ast.GoStmt, indent int) {
	sc.checkSyntax(ExtractGoStmtPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitSendStmt(node *ast.SendStmt, indent int) {
	sc.checkSyntax(ExtractSendStmtPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitSelectStmt(node *ast.SelectStmt, indent int) {
	sc.checkSyntax(ExtractSelectStmtPattern(node, sc.pkg), node.Pos())
}

func (sc *SyntaxChecker) PreVisitEllipsis(node *ast.Ellipsis, indent int) {
	sc.checkSyntax(ExtractEllipsisPattern(node, sc.pkg), node.Pos())
}
