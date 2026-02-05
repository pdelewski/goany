// syntax_checker.go provides GoAny syntax validation.
// It rejects Go constructs that are not supported by the GoAny transpiler.
// This is a simple handwritten checker that explicitly lists unsupported constructs.
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
	pkg *packages.Package
}

// reportSyntaxError reports an unsupported construct and exits
func (sc *SyntaxChecker) reportSyntaxError(pos token.Pos, construct, description string) {
	var filename string
	var line, col int

	if sc.pkg != nil && sc.pkg.Fset != nil && pos.IsValid() {
		position := sc.pkg.Fset.Position(pos)
		filename = position.Filename
		line = position.Line
		col = position.Column
	}

	fmt.Printf("\033[31m\033[1mSyntax error: %s\033[0m\n", construct)

	if filename != "" && line > 0 {
		fmt.Printf("  \033[36m-->\033[0m %s:%d:%d\n", filename, line, col)

		if sourceLine := readSourceLine(filename, line); sourceLine != "" {
			fmt.Printf("   \033[90m%4d |\033[0m %s\n", line, sourceLine)

			if col > 0 {
				padding := strings.Repeat(" ", col-1)
				fmt.Printf("   \033[90m     |\033[0m \033[31m%s^\033[0m\n", padding)
			}
		}
	}

	fmt.Printf("  %s\n", description)
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

// --- Unsupported Go constructs ---

func (sc *SyntaxChecker) PreVisitGoStmt(node *ast.GoStmt, indent int) {
	sc.reportSyntaxError(node.Pos(), "unsupported construct",
		"Goroutines (go keyword) are not supported.\n  GoAny targets languages without built-in goroutine equivalents.")
}

func (sc *SyntaxChecker) PreVisitDeferStmt(node *ast.DeferStmt, indent int) {
	sc.reportSyntaxError(node.Pos(), "unsupported construct",
		"Defer statements are not supported.\n  Defer has no direct equivalent in all target languages.")
}

func (sc *SyntaxChecker) PreVisitSelectStmt(node *ast.SelectStmt, indent int) {
	sc.reportSyntaxError(node.Pos(), "unsupported construct",
		"Select statements are not supported.\n  Select requires channel support which is not available.")
}

func (sc *SyntaxChecker) PreVisitSendStmt(node *ast.SendStmt, indent int) {
	sc.reportSyntaxError(node.Pos(), "unsupported construct",
		"Channel send statements are not supported.\n  Channels are not available in GoAny.")
}

func (sc *SyntaxChecker) PreVisitChanType(node *ast.ChanType, indent int) {
	sc.reportSyntaxError(node.Pos(), "unsupported construct",
		"Channel types are not supported.\n  Channels have no equivalent in target languages.")
}

func (sc *SyntaxChecker) PreVisitEllipsis(node *ast.Ellipsis, indent int) {
	sc.reportSyntaxError(node.Pos(), "unsupported construct",
		"Variadic parameters (...T) are not supported.\n  Use a slice parameter instead.")
}

func (sc *SyntaxChecker) PreVisitBranchStmt(node *ast.BranchStmt, indent int) {
	if node.Label != nil {
		sc.reportSyntaxError(node.Pos(), "unsupported construct",
			fmt.Sprintf("Labeled %s statements are not supported.\n  Use structured control flow instead.", node.Tok))
	}
}
