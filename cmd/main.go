package main

import (
	"flag"
	"fmt"
	"go/ast"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"goany/compiler"
	goanyrt "goany/runtime"

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

// packagesUseMap checks if any package in the list uses Go map types
// detectRuntimePackages scans all packages for "runtime/X" imports and returns
// a map of detected runtime package names (e.g. {"http": true, "json": true}).
func detectRuntimePackages(pkgs []*packages.Package) map[string]string {
	result := make(map[string]string)
	for _, pkg := range pkgs {
		for _, file := range pkg.Syntax {
			for _, imp := range file.Imports {
				if imp.Path == nil {
					continue
				}
				path := imp.Path.Value
				if len(path) > 2 {
					path = path[1 : len(path)-1] // strip quotes
				}
				if strings.HasPrefix(path, "runtime/") {
					name := path[len("runtime/"):]
					// Take first component only (e.g. "graphics" from "runtime/graphics/go/tigr")
					if idx := strings.Index(name, "/"); idx >= 0 {
						name = name[:idx]
					}
					result[name] = "" // default variant (empty)
				}
			}
		}
	}
	return result
}

func packagesUseMap(pkgs []*packages.Package) bool {
	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil {
			continue
		}
		for _, file := range pkg.Syntax {
			found := false
			ast.Inspect(file, func(n ast.Node) bool {
				if found {
					return false
				}
				switch n.(type) {
				case *ast.MapType:
					found = true
					return false
				}
				return true
			})
			if found {
				return true
			}
		}
	}
	return false
}

// loadRuntimeHashmap loads the embedded runtime/std/hashmap.go as a *packages.Package
// by writing it to a temp directory and using packages.Load
func loadRuntimeHashmap() (*packages.Package, error) {
	tmpDir, err := os.MkdirTemp("", "goany-runtime-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write go.mod
	err = os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte("module runtime/hmap\n\ngo 1.21\n"), 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to write go.mod: %v", err)
	}

	// Write hashmap.go from embedded source
	err = os.WriteFile(filepath.Join(tmpDir, "hashmap.go"), []byte(goanyrt.HashmapGoSource), 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to write hashmap.go: %v", err)
	}

	cfg := &packages.Config{
		Mode:  packages.LoadSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports | packages.NeedModule,
		Dir:   tmpDir,
		Tests: false,
	}

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("failed to load runtime hashmap package: %v", err)
	}

	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found in runtime hashmap")
	}

	// Check for errors
	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			return nil, fmt.Errorf("errors loading runtime hashmap: %v", pkg.Errors)
		}
	}

	return pkgs[0], nil
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
	var optimizeRefs bool
	var checkSyntax bool
	flag.BoolVar(&compiler.DebugMode, "debug", false, "Enable debug output")
	flag.BoolVar(&optimizeMoves, "optimize-moves", false, "Enable move optimizations to reduce struct cloning")
	flag.BoolVar(&optimizeRefs, "optimize-refs", false, "Enable reference optimization for read-only parameters")
	flag.BoolVar(&checkSyntax, "check-syntax", true, "Enable GoAny syntax validation (default: true)")
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

	// Verify that source packages have a go.mod file (Module != nil)
	// Without go.mod, packages would be silently skipped as "stdlib"
	for _, pkg := range pkgs {
		if pkg.Module == nil {
			fmt.Printf("\033[31m\033[1mError: go.mod file required\033[0m\n")
			fmt.Printf("  \033[36m-->\033[0m %s\n", sourceDir)
			fmt.Printf("  The source directory must be a Go module (contain go.mod).\n")
			fmt.Printf("  Without go.mod, packages cannot be properly validated.\n")
			fmt.Println()
			fmt.Printf("  \033[32mTo fix:\033[0m Run 'go mod init <module-name>' in the source directory.\n")
			fmt.Println()
			os.Exit(1)
		}
	}

	// Collect all imported packages recursively (excluding standard library)
	allPkgs := collectAllPackages(pkgs)

	// If any package uses map types, load and prepend the runtime hashmap package
	if packagesUseMap(allPkgs) {
		compiler.DebugLogPrintf("Map usage detected, loading runtime hashmap package")
		hashmapPkg, err := loadRuntimeHashmap()
		if err != nil {
			log.Fatalf("Failed to load runtime hashmap: %v", err)
		}
		// Prepend so hashmap functions are defined before user code
		allPkgs = append([]*packages.Package{hashmapPkg}, allPkgs...)
		compiler.DebugLogPrintf("Runtime hashmap package loaded: %s", hashmapPkg.Name)
	}

	// Detect runtime package usage (e.g. runtime/http, runtime/graphics, etc.)
	runtimePackages := detectRuntimePackages(pkgs)
	// Apply graphics variant from CLI flag
	if _, ok := runtimePackages["graphics"]; ok {
		runtimePackages["graphics"] = graphicsRuntime
	}
	for name, variant := range runtimePackages {
		if variant != "" {
			compiler.DebugLogPrintf("Runtime package detected: %s (variant: %s)", name, variant)
		} else {
			compiler.DebugLogPrintf("Runtime package detected: %s", name)
		}
	}

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
	var passes []compiler.Pass

	// Pass 1: Syntax checking (rejects unsupported Go constructs)
	if checkSyntax {
		syntaxChecker := &compiler.SyntaxChecker{Emitter: &compiler.BaseEmitter{}}
		syntaxPass := &compiler.BasePass{PassName: "SyntaxCheck", Emitter: syntaxChecker}
		passes = append(passes, syntaxPass)
	}

	// Pass 2: Semantic analysis (always runs)
	semaChecker := &compiler.SemaChecker{Emitter: &compiler.BaseEmitter{}}
	sema := &compiler.BasePass{PassName: "Sema", Emitter: semaChecker}
	passes = append(passes, sema)
	var programFiles []string

	if useCpp {
		cppBackend := &compiler.BasePass{PassName: "CppGen", Emitter: &compiler.CPPEmitter{
			Emitter:         &compiler.BaseEmitter{},
			Output:          output + ".cpp",
			LinkRuntime:     linkRuntime,
			RuntimePackages: runtimePackages,
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
			RuntimePackages: runtimePackages,
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
			RuntimePackages: runtimePackages,
			OutputDir:       outputDir,
			OutputName:      outputName,
			OptimizeMoves:   optimizeMoves,
			OptimizeRefs:    optimizeRefs,
		}}
		passes = append(passes, rustBackend)
		programFiles = append(programFiles, "rs")
	}
	if useJs {
		jsBackend := &compiler.BasePass{PassName: "JsGen", Emitter: &compiler.JSEmitter{
			Emitter:         &compiler.BaseEmitter{},
			Output:          output + ".js",
			LinkRuntime:     linkRuntime,
			RuntimePackages: runtimePackages,
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
