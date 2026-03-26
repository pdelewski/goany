//  ____  ___    _    _  _ _   _
// / ___|/ _ \  / \  | \| | \_/ |
//| |  _| | | |/ _ \ |  \ |\_/| |
//| |_| | |_| / ___ \| |\  |  | |
// \____|\___/_/   \_\_| \_|  |_|

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
	var checkSema bool
	flag.BoolVar(&compiler.DebugMode, "debug", false, "Enable debug output")
	flag.BoolVar(&optimizeMoves, "optimize-moves", false, "Enable move optimizations to reduce struct cloning")
	flag.BoolVar(&optimizeRefs, "optimize-refs", false, "Enable reference optimization for read-only parameters")
	flag.BoolVar(&checkSema, "check-sema", false, "Check syntax and semantics only, no transpilation")
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
	// Scan allPkgs (not just top-level pkgs) so transitive runtime imports are detected
	runtimePackages := detectRuntimePackages(allPkgs)
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
	useJs := backendSet["js"]             // JS is opt-in, not included in "all"
	useJava := backendSet["java"] // Java is opt-in, not included in "all"
	useGo := backendSet["go"]             // Go is opt-in, not included in "all"

	// Build shared frontend passes
	var frontendPasses []compiler.FrontendPass

	// Compute backend set for the canonicalize pass
	var enabledBackends compiler.BackendSet
	if useCpp {
		enabledBackends |= compiler.BackendCpp
	}
	if useCs {
		enabledBackends |= compiler.BackendCSharp
	}
	if useRust {
		enabledBackends |= compiler.BackendRust
	}
	if useJs {
		enabledBackends |= compiler.BackendJs
	}
	if useJava {
		enabledBackends |= compiler.BackendJava
	}
	if useGo {
		enabledBackends |= compiler.BackendGo
	}

	// Pass 1: Syntax checking (rejects unsupported Go constructs)
	syntaxChecker := &compiler.SyntaxChecker{Emitter: &compiler.BaseEmitter{}}
	syntaxPass := &compiler.BasePass{PassName: "SyntaxCheck", Emitter: syntaxChecker}
	frontendPasses = append(frontendPasses, syntaxPass)

	// Pass 2: Semantic analysis (first run - on original AST)
	semaChecker := &compiler.SemaChecker{Emitter: &compiler.BaseEmitter{}}
	sema := &compiler.BasePass{PassName: "Sema", Emitter: semaChecker}
	frontendPasses = append(frontendPasses, sema)

	// Pass 3: Canonicalize (rewrite AST patterns that lack 1:1 equivalents)
	canonicalize := &compiler.CanonicalizePass{Backends: enabledBackends}
	frontendPasses = append(frontendPasses, canonicalize)

	// Pass 4: Method receiver lowering
	methodLowering := &compiler.MethodReceiverLoweringPass{}
	frontendPasses = append(frontendPasses, methodLowering)

	// Pass 4: Semantic analysis (after method receiver lowering)
	semaChecker2 := &compiler.SemaChecker{Emitter: &compiler.BaseEmitter{}}
	sema2 := &compiler.BasePass{PassName: "Sema", Emitter: semaChecker2}
	frontendPasses = append(frontendPasses, sema2)

	// Pass 5: Pointer-to-array transformation
	ptrTransform := &compiler.PointerTransformPass{}
	frontendPasses = append(frontendPasses, ptrTransform)

	// Pass 6: Semantic analysis (after pointer transform)
	semaChecker3 := &compiler.SemaChecker{Emitter: &compiler.BaseEmitter{}}
	sema3 := &compiler.BasePass{PassName: "Sema", Emitter: semaChecker3}
	frontendPasses = append(frontendPasses, sema3)

	// If check-sema mode, run passes and exit
	if checkSema {
		passManager := &compiler.PassManager{
			Pkgs:           allPkgs,
			FrontendPasses: frontendPasses,
		}
		passManager.RunPasses()

		// Print success message
		green := "\033[32m"
		bold := "\033[1m"
		reset := "\033[0m"
		checkmark := "✓"
		fmt.Printf("\n%s%s%s Syntax and semantic checks passed!%s\n", bold, green, checkmark, reset)
		fmt.Printf("%s  Checked:%s %d package(s)\n", green, reset, len(allPkgs))
		return
	}

	// Build backend pipelines
	var backendPipelines []compiler.BackendPipeline
	var programFiles []string

	if useCpp {
		cppRefOpt := &compiler.RefOptPass{Tag: compiler.TagCpp, Enabled: true}
		backendPipelines = append(backendPipelines, compiler.BackendPipeline{
			CodeGen: &compiler.BasePass{PassName: "CppGen", Emitter: &compiler.CppEmitter{
				Emitter:         &compiler.BaseEmitter{},
				Output:          output + ".cpp",
				LinkRuntime:     linkRuntime,
				RuntimePackages: runtimePackages,
				OutputDir:       outputDir,
				OutputName:      outputName,
				OptimizeMoves:   optimizeMoves,
				OptimizeRefs:    optimizeRefs,
				CppRefOptPass:   cppRefOpt,
			}},
			IRPasses: []compiler.IRPass{
				&compiler.CloneMovePass{Tag: compiler.TagCpp, Enabled: true},
				cppRefOpt,
			},
		})
		programFiles = append(programFiles, "cpp")
	}
	if useCs {
		csRefOpt := &compiler.RefOptPass{Tag: compiler.TagCSharp, Enabled: true}
		backendPipelines = append(backendPipelines, compiler.BackendPipeline{
			CodeGen: &compiler.BasePass{PassName: "CsGen", Emitter: &compiler.CSharpEmitter{
				Emitter:         &compiler.BaseEmitter{},
				Output:          output + ".cs",
				LinkRuntime:     linkRuntime,
				RuntimePackages: runtimePackages,
				OutputDir:       outputDir,
				OutputName:      outputName,
				OptimizeRefs:    optimizeRefs,
				CsRefOptPass:    csRefOpt,
			}},
			IRPasses: []compiler.IRPass{csRefOpt},
		})
		programFiles = append(programFiles, "cs")
	}
	if useRust {
		rustRefOpt := &compiler.RefOptPass{Tag: compiler.TagRust, Enabled: true}
		backendPipelines = append(backendPipelines, compiler.BackendPipeline{
			CodeGen: &compiler.BasePass{PassName: "RustGen", Emitter: &compiler.RustEmitter{
				Emitter:         &compiler.BaseEmitter{},
				Output:          output + ".rs",
				LinkRuntime:     linkRuntime,
				RuntimePackages: runtimePackages,
				OutputDir:       outputDir,
				OutputName:      outputName,
				Opt: compiler.RustOptState{
					OptimizeMoves: optimizeMoves,
					OptimizeRefs:  optimizeRefs,
					RefOptPass:    rustRefOpt,
				},
			}},
			IRPasses: []compiler.IRPass{
				&compiler.CloneMovePass{Tag: compiler.TagRust, Enabled: true},
				rustRefOpt,
			},
		})
		programFiles = append(programFiles, "rs")
	}
	if useJs {
		backendPipelines = append(backendPipelines, compiler.BackendPipeline{
			CodeGen: &compiler.BasePass{PassName: "JsGen", Emitter: &compiler.JSEmitter{
				Emitter:         &compiler.BaseEmitter{},
				Output:          output + ".js",
				LinkRuntime:     linkRuntime,
				RuntimePackages: runtimePackages,
				OutputDir:       outputDir,
				OutputName:      outputName,
			}},
		})
		programFiles = append(programFiles, "js")
	}
	if useJava {
		backendPipelines = append(backendPipelines, compiler.BackendPipeline{
			CodeGen: &compiler.BasePass{PassName: "JavaGen", Emitter: &compiler.JavaEmitter{
				Emitter:         &compiler.BaseEmitter{},
				Output:          output + ".java",
				LinkRuntime:     linkRuntime,
				RuntimePackages: runtimePackages,
				OutputDir:       outputDir,
				OutputName:      outputName,
			}},
		})
		programFiles = append(programFiles, "java")
	}
	if useGo {
		backendPipelines = append(backendPipelines, compiler.BackendPipeline{
			CodeGen: &compiler.BasePass{PassName: "GoGen", Emitter: &compiler.GoEmitter{
				Emitter:         &compiler.BaseEmitter{},
				Output:          output + "_gen.go",
				LinkRuntime:     linkRuntime,
				RuntimePackages: runtimePackages,
				Pkgs:            allPkgs,
				OutputDir:       outputDir,
				OutputName:      outputName,
			}},
		})
		programFiles = append(programFiles, "go")
	}
	passManager := &compiler.PassManager{
		Pkgs:           allPkgs,
		FrontendPasses: frontendPasses,
		Backends:       backendPipelines,
	}

	passManager.RunPasses()

	// Format generated files
	// Use astyle for C++/C#/Java, rustfmt for Rust
	hasAstyleFiles := useCpp || useCs || useJava
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
		if useJava {
			// Format all generated .java files (main + package files)
			javaFiles, _ := filepath.Glob(filepath.Join(outputDir, "*.java"))
			for _, filePath := range javaFiles {
				err = compiler.FormatFile(filePath, astyleOptions)
				if err != nil {
					log.Fatalf("Failed to format %s: %v", filePath, err)
				}
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
