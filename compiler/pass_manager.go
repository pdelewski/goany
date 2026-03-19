package compiler

import (
	"fmt"
	"go/ast"
	"os"

	"golang.org/x/tools/go/packages"
)

type FrontendPass interface {
	ProLog()
	Name() string
	Visitors(pkg *packages.Package) []ast.Visitor
	PreVisit(visitor ast.Visitor)
	PostVisit(visitor ast.Visitor, visited map[string]struct{})
	EpiLog()
}

type IRPass interface {
	Name() string
	Transform(root IRNode) IRNode
}

type BackendPipeline struct {
	CodeGen  FrontendPass // emitter pass (go/ast → IR)
	IRPasses []IRPass     // IR → IR optimization chain
}

type OutputEntry struct {
	Path string
	Root IRNode
}

type PassManager struct {
	Pkgs           []*packages.Package
	FrontendPasses []FrontendPass    // shared: syntax, sema, lowering
	Backends       []BackendPipeline // per-backend: codegen + optimizations
}

func (pm *PassManager) runFrontendPass(pass FrontendPass, passNum, totalPasses int) {
	DebugPrintf("Running pass: %s\n", pass.Name())
	if !DebugMode {
		fmt.Printf("\r[%d/%d] %s...", passNum, totalPasses, pass.Name())
	}
	visited := make(map[string]struct{})
	pass.ProLog()
	for _, pkg := range pm.Pkgs {
		DebugPrintf("Package: %s\n", pkg.Name)
		DebugPrintf("Types Topological Sort: %v\n", pkg.TypesInfo)
		visitors := pass.Visitors(pkg)

		for _, visitor := range visitors {
			pass.PreVisit(visitor)
		}

		for _, visitor := range visitors {
			for _, file := range pkg.Syntax {
				ast.Walk(visitor, file)
			}
		}
		for _, visitor := range visitors {
			pass.PostVisit(visitor, visited)
		}
	}
	pass.EpiLog()
}

func (pm *PassManager) RunPasses() {
	totalPasses := len(pm.FrontendPasses) + len(pm.Backends)
	passNum := 0

	// Phase 1: Run shared frontend passes (syntax, sema, lowering)
	for _, pass := range pm.FrontendPasses {
		passNum++
		pm.runFrontendPass(pass, passNum, totalPasses)
	}

	// Phase 2: Run each backend pipeline
	for _, backend := range pm.Backends {
		passNum++
		// Run the codegen frontend pass (emitter walks AST → IR)
		pm.runFrontendPass(backend.CodeGen, passNum, totalPasses)

		emitter := backend.CodeGen.(*BasePass).Emitter
		entries := emitter.GetOutputEntries()

		for _, entry := range entries {
			root := entry.Root
			for _, irPass := range backend.IRPasses {
				root = irPass.Transform(root)
			}
			file, err := os.Create(entry.Path)
			if err != nil {
				fmt.Printf("Error creating output file %s: %v\n", entry.Path, err)
				continue
			}
			file.WriteString(root.Serialize())
			file.Close()
		}

		// Report IR pass stats
		for _, irPass := range backend.IRPasses {
			if rop, ok := irPass.(*RefOptPass); ok && rop.TransformCount > 0 {
				lang := ""
				switch rop.Tag {
				case TagRust:
					lang = "Rust"
				case TagCpp:
					lang = "C++"
				case TagCSharp:
					lang = "C#"
				}
				fmt.Printf("  %s: %d ref(s) optimized by RefOptPass\n", lang, rop.TransformCount)
			}
		}

		emitter.PostFileEmit()
	}

	// Clear the progress line and show completion
	if !DebugMode {
		fmt.Printf("\r[%d/%d] Done.                    \n", totalPasses, totalPasses)
	}
}
