package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func joinStrings(strs []string, sep string) string {
	return strings.Join(strs, sep)
}

type TestCase struct {
	Name          string
	SourceDir     string
	CppEnabled    bool
	DotnetEnabled bool
	RustEnabled   bool
	JsEnabled     bool
	JsRunnable    bool // Can run with Node.js (false for graphics apps that need browser)
	JavaEnabled   bool
	JavaRunnable  bool // Can run standalone (false for apps that need special setup)
}

const runtimePath = "../runtime"

var e2eTestCases = []TestCase{
	{"lang-constructs", "../tests/lang-constructs", true, true, true, true, true, true, true},
	{"containers", "../examples/containers", true, true, true, true, true, true, false},
	{"uql", "../examples/uql", true, true, true, true, true, true, false},
	{"ast-demo", "../examples/ast-demo", true, true, true, true, true, true, false},
	{"graphics-minimal", "../examples/graphics-minimal", true, true, true, true, false, true, false},          // JS transpile only (needs browser), Java transpile only (needs native lib)
	{"graphics-demo", "../examples/graphics-demo", true, true, true, true, false, true, false},               // JS transpile only (needs browser), Java transpile only (needs native lib)
	{"gui-demo", "../examples/gui-demo", true, true, true, true, false, true, false},                          // JS transpile only (needs browser), Java transpile only (needs native lib)
	{"mos6502-graphic", "../examples/mos6502/cmd/graphic", true, true, true, true, false, true, false},        // JS transpile only (needs browser), Java transpile only (needs native lib)
	{"mos6502-text", "../examples/mos6502/cmd/text", true, true, true, true, false, true, false},              // JS transpile only (needs browser), Java transpile only (needs native lib)
	{"mos6502-textscroll", "../examples/mos6502/cmd/textscroll", true, true, true, true, false, false, false}, // JS transpile only (needs browser), Java disabled (symbol issues)
	{"mos6502-c64", "../examples/mos6502/cmd/c64", true, true, true, true, false, true, false},                // JS transpile only (needs browser), Java transpile only (needs native lib)
	{"mos6502-c64-v2", "../examples/mos6502/cmd/c64-v2", true, true, true, true, false, false, false},         // JS transpile only (needs browser), Java disabled (method reference type issue)
	{"http-client", "../examples/http/client", true, true, true, true, false, true, false},
	{"http-server", "../examples/http/server", true, true, true, true, false, true, false},
	{"fs-demo", "../examples/fs-demo", true, true, true, true, true, true, true},
	{"net-demo", "../examples/net/demo", true, true, true, true, false, true, false},              // JS transpile only (needs deasync npm package)
	{"net-echo-server", "../examples/net/echo-server", true, true, true, true, false, true, false}, // Server example - transpile/compile only
	{"net-echo-client", "../examples/net/echo-client", true, true, true, true, false, true, false}, // Client example - transpile/compile only
}

func TestE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E tests in short mode")
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	// Create build directory
	buildDir := filepath.Join(wd, "build")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		t.Fatalf("Failed to create build directory: %v", err)
	}

	// Clean up build directory at the end
	t.Cleanup(func() {
		os.RemoveAll(buildDir)
	})

	for _, tc := range e2eTestCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			runE2ETest(t, wd, buildDir, tc)
		})
	}
}

func runE2ETest(t *testing.T, wd, buildDir string, tc TestCase) {
	outputDir := filepath.Join(buildDir, tc.Name)

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}

	// Ensure cleanup runs even if test fails
	t.Cleanup(func() {
		os.RemoveAll(outputDir)
	})

	// Step 1: Generate code using go run with -link-runtime
	// Output path includes the name so files are created in the subdirectory
	t.Logf("Generating code for %s", tc.Name)
	outputPath := filepath.Join(outputDir, tc.Name)
	args := []string{
		"run", ".",
		fmt.Sprintf("--source=%s", tc.SourceDir),
		fmt.Sprintf("--output=%s", outputPath),
		fmt.Sprintf("--link-runtime=%s", runtimePath),
		"--optimize-moves",
		"--optimize-refs",
	}
	// Add opt-in backends (JS and Java are not included in "all")
	optInBackends := []string{}
	if tc.JsEnabled {
		optInBackends = append(optInBackends, "js")
	}
	if tc.JavaEnabled {
		optInBackends = append(optInBackends, "java")
	}
	if len(optInBackends) > 0 {
		args = append(args, fmt.Sprintf("--backend=all,%s", joinStrings(optInBackends, ",")))
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = wd
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Code generation failed: %v\nOutput: %s", err, output)
	}
	t.Logf("Code generation output: %s", output)

	// Step 2: Compile C++ using make
	if tc.CppEnabled {
		t.Logf("Compiling C++ for %s", tc.Name)
		cmd = exec.Command("make")
		cmd.Dir = outputDir
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("C++ compilation failed: %v\nOutput: %s", err, output)
		}
		t.Logf("C++ compilation output: %s", output)
	}

	// Step 3: Compile C# using dotnet build
	if tc.DotnetEnabled {
		t.Logf("Compiling C# for %s", tc.Name)
		cmd = exec.Command("dotnet", "build")
		cmd.Dir = outputDir
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("C# compilation failed: %v\nOutput: %s", err, output)
		}
		t.Logf("C# compilation output: %s", output)
	}

	// Step 4: Compile Rust using cargo build --release (matches C++ -O3)
	if tc.RustEnabled {
		t.Logf("Compiling Rust for %s", tc.Name)
		cmd = exec.Command("cargo", "build", "--release")
		cmd.Dir = outputDir
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Rust compilation failed: %v\nOutput: %s", err, output)
		}
		t.Logf("Rust compilation output: %s", output)
	}

	// Step 5: Run JavaScript using node (only if runnable - graphics apps need browser)
	if tc.JsEnabled && tc.JsRunnable {
		jsFile := filepath.Join(outputDir, tc.Name+".js")
		t.Logf("Running JavaScript for %s", tc.Name)
		cmd = exec.Command("node", jsFile)
		cmd.Dir = outputDir
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("JavaScript execution failed: %v\nOutput: %s", err, output)
		}
		t.Logf("JavaScript execution output: %s", output)
	} else if tc.JsEnabled {
		t.Logf("Skipping JavaScript execution for %s (requires browser)", tc.Name)
	}

	// Step 6: Compile and run Java
	if tc.JavaEnabled {
		// Java file names are sanitized (hyphens replaced with underscores)
		javaName := strings.ReplaceAll(tc.Name, "-", "_")
		// Find all Java files in the output directory (main file + runtime files)
		javaFiles, _ := filepath.Glob(filepath.Join(outputDir, "*.java"))
		t.Logf("Compiling Java for %s (files: %v)", tc.Name, javaFiles)
		// Compile all Java files together
		cmd = exec.Command("javac", javaFiles...)
		cmd.Dir = outputDir
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Java compilation failed: %v\nOutput: %s", err, output)
		}
		t.Logf("Java compilation output: %s", output)

		if tc.JavaRunnable {
			// The class name is the sanitized file name (without extension)
			t.Logf("Running Java for %s", tc.Name)
			cmd = exec.Command("java", javaName)
			cmd.Dir = outputDir
			output, err = cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("Java execution failed: %v\nOutput: %s", err, output)
			}
			t.Logf("Java execution output: %s", output)
		}
	}

	t.Logf("Done with %s", tc.Name)
}
