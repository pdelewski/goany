package main

import (
	"fmt"
	"go/ast"
	"os"
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

// detectRuntimePackages scans all packages for "runtime/X" imports and returns
// a map of detected runtime package names with variant (e.g. {"http": "", "graphics": "tigr"}).
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

// packagesUseMap checks if any package in the list uses Go map types
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
