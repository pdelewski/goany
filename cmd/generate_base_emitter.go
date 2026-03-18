//go:build ignore

// generate_base_emitter.go - Tool for automatically generating base_emitter.go
//
// This tool reads the Emitter interface from emitter.go and generates empty
// implementations for all interface methods in the BaseEmitter struct.
//
// Usage:
//
//	go run cmd/generate_base_emitter.go
//
// The tool is integrated with go generate. Run `go generate ./...` to regenerate
// base_emitter.go whenever the Emitter interface changes.
//
// The generated base_emitter.go provides a foundation that other emitters
// (like CSharpEmitter, RustEmitter) can embed and override specific methods.
package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	// Read the emitter.go file
	file, err := os.Open("emitter.go")
	if err != nil {
		fmt.Printf("Error opening emitter.go: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	// Parse the interface methods
	methods := parseEmitterInterface(file)

	// Extract Pre/Post visit method names for constants
	visitConstants := extractVisitConstants(methods)

	// Generate the base_emitter.go file
	generateBaseEmitter(methods, visitConstants)
}

func parseEmitterInterface(file *os.File) []string {
	scanner := bufio.NewScanner(file)
	var methods []string
	inInterface := false

	// Regex to match method signatures
	methodRegex := regexp.MustCompile(`^\s*([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)\s*(.*)$`)

	for scanner.Scan() {
		line := scanner.Text()

		// Check if we're entering the Emitter interface
		if strings.Contains(line, "type Emitter interface") {
			inInterface = true
			continue
		}

		// Check if we're exiting the interface
		if inInterface && strings.Contains(line, "}") {
			break
		}

		// Skip comments and empty lines
		if inInterface && (strings.HasPrefix(strings.TrimSpace(line), "//") || strings.TrimSpace(line) == "") {
			continue
		}

		// Extract method signatures
		if inInterface {
			matches := methodRegex.FindStringSubmatch(line)
			if len(matches) >= 3 {
				methodName := matches[1]
				params := matches[2]
				returnType := strings.TrimSpace(matches[3])

				// Format the method signature for BaseEmitter
				signature := formatMethodSignature(methodName, params, returnType)
				methods = append(methods, signature)
			}
		}
	}

	return methods
}

func formatMethodSignature(methodName, params, returnType string) string {
	// Add receiver for BaseEmitter
	receiver := "(v *BaseEmitter) "

	// Handle return types
	var returnClause string
	if returnType != "" {
		returnClause = " " + returnType
	}

	// Format the complete method signature
	return fmt.Sprintf("func %s%s(%s)%s", receiver, methodName, params, returnClause)
}

func extractVisitConstants(methods []string) []string {
	var constants []string

	for _, method := range methods {
		// Extract method name from signature
		methodNameRegex := regexp.MustCompile(`func \(v \*BaseEmitter\) ([A-Za-z0-9_]+)\(`)
		matches := methodNameRegex.FindStringSubmatch(method)
		if len(matches) >= 2 {
			methodName := matches[1]
			// Only include Pre/Post visit methods
			if strings.HasPrefix(methodName, "PreVisit") || strings.HasPrefix(methodName, "PostVisit") {
				constants = append(constants, methodName)
			}
		}
	}

	return constants
}

func generateBaseEmitter(methods []string, visitConstants []string) {
	// Create/overwrite base_emitter.go
	output, err := os.Create("base_emitter.go")
	if err != nil {
		fmt.Printf("Error creating base_emitter.go: %v\n", err)
		os.Exit(1)
	}
	defer output.Close()

	// Write the header
	fmt.Fprintf(output, `package compiler

import (
	"go/ast"
	"go/token"
	"golang.org/x/tools/go/packages"
	"os"
)

// VisitMethod represents a visit method identifier
type VisitMethod string

// Visit method name constants
const (
	EmptyVisitMethod VisitMethod = ""
`)

	// Write the visit constants
	for _, constant := range visitConstants {
		fmt.Fprintf(output, "\t%s VisitMethod = \"%s\"\n", constant, constant)
	}

	fmt.Fprintf(output, `)

type BaseEmitter struct{
	fb *IRForestBuilder
}

`)

	// Write each method with empty implementation
	for _, method := range methods {
		// Check if method has return type
		if strings.Contains(method, ") *os.File") {
			fmt.Fprintf(output, "%s { return nil }\n", method)
		} else if strings.Contains(method, ") *IRForestBuilder") {
			fmt.Fprintf(output, "%s {\n\tif v.fb == nil {\n\t\tv.fb = NewIRForestBuilder()\n\t}\n\treturn v.fb\n}\n", method)
		} else {
			fmt.Fprintf(output, "%s {}\n", method)
		}
	}
}
