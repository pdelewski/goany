package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"goany/compiler"

	"golang.org/x/tools/go/packages"
)

// collectAllPackages recursively collects all imported packages (excluding standard library and runtime)
// Packages are returned in dependency order: dependencies first, main package last
func collectAllPackages(pkgs []*packages.Package) []*packages.Package {
	visited := make(map[string]bool)
	var result []*packages.Package

	var collect func(pkg *packages.Package)
	collect = func(pkg *packages.Package) {
		if pkg == nil || visited[pkg.PkgPath] {
			return
		}
		visited[pkg.PkgPath] = true

		// Skip standard library packages (they don't have a module)
		// User packages always have Module != nil
		if pkg.Module == nil {
			compiler.DebugLogPrintf("Skipping stdlib package: %s", pkg.PkgPath)
			return
		}

		// Skip runtime packages (they are part of the transpiler runtime, not user code)
		if strings.HasPrefix(pkg.Module.Path, "runtime/") {
			compiler.DebugLogPrintf("Skipping runtime package: %s (module: %s)", pkg.PkgPath, pkg.Module.Path)
			return
		}

		// First, recursively collect imports (dependencies come before dependents)
		for _, importedPkg := range pkg.Imports {
			collect(importedPkg)
		}

		// Then add this package (after its dependencies)
		compiler.DebugLogPrintf("Adding package: %s (module: %s)", pkg.PkgPath, pkg.Module.Path)
		result = append(result, pkg)
	}

	for _, pkg := range pkgs {
		collect(pkg)
	}

	compiler.DebugLogPrintf("Total packages to transpile: %d", len(result))
	return result
}

func main() {
	var sourceDir string
	var output string
	var backend string
	var linkRuntime string
	var graphicsRuntime string
	flag.StringVar(&sourceDir, "source", "", "Source directory")
	flag.StringVar(&output, "output", "", "Output program name (can include path, e.g., ./build/project)")
	flag.StringVar(&backend, "backend", "all", "Backend to use: all, cpp, cs, rust, js (comma-separated for multiple)")
	flag.StringVar(&linkRuntime, "link-runtime", "", "Path to runtime for linking (generates Makefile with -I flag)")
	flag.StringVar(&graphicsRuntime, "graphics-runtime", "tigr", "Graphics runtime: tigr (default), sdl2, none")
	var optimizeMoves bool
	flag.BoolVar(&compiler.DebugMode, "debug", false, "Enable debug output")
	flag.BoolVar(&optimizeMoves, "optimize-moves", false, "Enable move optimizations to reduce struct cloning")
	flag.Parse()
	if sourceDir == "" {
		fmt.Println("Please provide a source directory")
		return
	}

	// Parse output directory and name
	outputDir := filepath.Dir(output)
	outputName := filepath.Base(output)

	// Create output directory if it doesn't exist
	if outputDir != "." && outputDir != "" {
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Fatalf("Failed to create output directory: %v", err)
		}
	}

	// Note: We allow overwriting existing build files (Cargo.toml, Makefile, etc.)
	// to support iterative development
	cfg := &packages.Config{
		Mode:  packages.LoadSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports | packages.NeedModule,
		Dir:   sourceDir,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		fmt.Println("Error loading packages:", err)
		return
	}

	if len(pkgs) == 0 {
		fmt.Println("No packages found")
		return
	}

	// Collect all imported packages recursively (excluding standard library)
	allPkgs := collectAllPackages(pkgs)

	// Parse backend selection
	backends := strings.Split(strings.ToLower(backend), ",")
	backendSet := make(map[string]bool)
	for _, b := range backends {
		backendSet[strings.TrimSpace(b)] = true
	}
	useAll := backendSet["all"]
	useCpp := useAll || backendSet["cpp"]
	useCs := useAll || backendSet["cs"]
	useRust := useAll || backendSet["rust"]
	useJs := backendSet["js"] // JS is opt-in, not included in "all"

	// Build passes list
	sema := &compiler.BasePass{PassName: "Sema", Emitter: &compiler.SemaChecker{Emitter: &compiler.BaseEmitter{}}}
	passes := []compiler.Pass{sema}
	var programFiles []string

	if useCpp {
		cppBackend := &compiler.BasePass{PassName: "CppGen", Emitter: &compiler.CPPEmitter{
			Emitter:         &compiler.BaseEmitter{},
			Output:          output + ".cpp",
			LinkRuntime:     linkRuntime,
			GraphicsRuntime: graphicsRuntime,
			OutputDir:       outputDir,
			OutputName:      outputName,
			OptimizeMoves:   optimizeMoves,
		}}
		passes = append(passes, cppBackend)
		programFiles = append(programFiles, "cpp")
	}
	if useCs {
		csBackend := &compiler.BasePass{PassName: "CsGen", Emitter: &compiler.CSharpEmitter{
			BaseEmitter:     compiler.BaseEmitter{},
			Output:          output + ".cs",
			LinkRuntime:     linkRuntime,
			GraphicsRuntime: graphicsRuntime,
			OutputDir:       outputDir,
			OutputName:      outputName,
		}}
		passes = append(passes, csBackend)
		programFiles = append(programFiles, "cs")
	}
	if useRust {
		rustBackend := &compiler.BasePass{PassName: "RustGen", Emitter: &compiler.RustEmitter{
			BaseEmitter:     compiler.BaseEmitter{},
			Output:          output + ".rs",
			LinkRuntime:     linkRuntime,
			GraphicsRuntime: graphicsRuntime,
			OutputDir:       outputDir,
			OutputName:      outputName,
			OptimizeMoves:   optimizeMoves,
		}}
		passes = append(passes, rustBackend)
		programFiles = append(programFiles, "rs")
	}
	if useJs {
		jsBackend := &compiler.BasePass{PassName: "JsGen", Emitter: &compiler.JSEmitter{
			Emitter:         &compiler.BaseEmitter{},
			Output:          output + ".js",
			LinkRuntime:     linkRuntime,
			GraphicsRuntime: graphicsRuntime,
			OutputDir:       outputDir,
			OutputName:      outputName,
		}}
		passes = append(passes, jsBackend)
		programFiles = append(programFiles, "js")
	}

	passManager := &compiler.PassManager{
		Pkgs:   allPkgs,
		Passes: passes,
	}

	passManager.RunPasses()

	// Format generated files
	// Use astyle for C++/C#, rustfmt for Rust
	hasAstyleFiles := useCpp || useCs
	if hasAstyleFiles {
		compiler.DebugLogPrintf("Using astyle version: %s\n", compiler.GetAStyleVersion())
		const astyleOptions = "--style=webkit"

		if useCpp {
			filePath := fmt.Sprintf("%s.cpp", output)
			err = compiler.FormatFile(filePath, astyleOptions)
			if err != nil {
				log.Fatalf("Failed to format %s: %v", filePath, err)
			}
		}
		if useCs {
			filePath := fmt.Sprintf("%s.cs", output)
			err = compiler.FormatFile(filePath, astyleOptions)
			if err != nil {
				log.Fatalf("Failed to format %s: %v", filePath, err)
			}
		}
	}

	// Use rustfmt for Rust files
	if useRust {
		var rustFile string
		if linkRuntime != "" {
			// For Cargo projects, the file is in src/main.rs
			rustFile = filepath.Join(outputDir, "src", "main.rs")
		} else {
			rustFile = fmt.Sprintf("%s.rs", output)
		}
		cmd := exec.Command("rustfmt", rustFile)
		if err := cmd.Run(); err != nil {
			// rustfmt not available or failed - just log warning, don't fail
			log.Printf("Warning: rustfmt failed for %s: %v (install with: rustup component add rustfmt)", rustFile, err)
		} else {
			compiler.DebugLogPrintf("Successfully formatted: %s", rustFile)
		}
	}

	// Use prettier for JS files
	if useJs {
		jsFile := fmt.Sprintf("%s.js", output)
		cmd := exec.Command("npx", "prettier", "--write", jsFile)
		if err := cmd.Run(); err != nil {
			// prettier not available or failed - just log warning, don't fail
			log.Printf("Warning: prettier failed for %s: %v (install with: npm install -g prettier)", jsFile, err)
		} else {
			compiler.DebugLogPrintf("Successfully formatted: %s", jsFile)
		}
	}

	// Print colorful success message
	green := "\033[32m"
	bold := "\033[1m"
	reset := "\033[0m"
	checkmark := "✓"

	fmt.Printf("\n%s%s%s Transpilation successful!%s\n", bold, green, checkmark, reset)
	fmt.Printf("%s  Generated:%s %s\n", green, reset, strings.Join(programFiles, ", "))
}
