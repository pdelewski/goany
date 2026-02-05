package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSemaErrors verifies that each case in tests-sema-errors/ produces an error or warning
func TestSemaErrors(t *testing.T) {
	// Find the tests-sema-errors directory
	testsDir := filepath.Join("..", "tests-sema-errors")
	if _, err := os.Stat(testsDir); os.IsNotExist(err) {
		t.Skip("tests-sema-errors directory not found")
	}

	// Get all subdirectories
	entries, err := os.ReadDir(testsDir)
	if err != nil {
		t.Fatalf("Failed to read tests-sema-errors: %v", err)
	}

	// Test cases that expect warnings instead of errors
	warningOnlyTests := map[string]bool{
		"const-no-type": true,
		"iota":          true,
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			sourceDir := filepath.Join(testsDir, name)
			outputFile := filepath.Join(t.TempDir(), "output")

			// Run the transpiler
			cmd := exec.Command("go", "run", ".", "-source", sourceDir, "-output", outputFile, "-backend", "rust")
			output, err := cmd.CombinedOutput()
			outputStr := string(output)

			// Check if this test expects a warning instead of an error
			if warningOnlyTests[name] {
				// For warning-only tests, check that a warning was emitted
				hasWarning := strings.Contains(outputStr, "Warning:")
				if !hasWarning {
					t.Errorf("Expected warning for %s, got: %s", name, outputStr)
				}
				t.Logf("%s: %s", name, strings.Split(outputStr, "\n")[0])
				return
			}

			// For error tests, we expect a non-zero exit code
			if err == nil {
				t.Errorf("Expected error for %s, but transpilation succeeded", name)
				return
			}

			// Check that it's a syntax or semantic error
			hasSyntaxError := strings.Contains(outputStr, "Syntax error")
			hasSemanticError := strings.Contains(outputStr, "Semantic error") || strings.Contains(outputStr, "Error:")

			if !hasSyntaxError && !hasSemanticError {
				t.Errorf("Expected syntax or semantic error for %s, got: %s", name, outputStr)
			}

			t.Logf("%s: %s", name, strings.Split(outputStr, "\n")[0])
		})
	}
}
