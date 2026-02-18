package compiler

/*
#cgo CXXFLAGS: -I./astyle -DASTYLE_LIB -std=c++17
#cgo LDFLAGS: -L./astyle -lastyle -lstdc++

#ifdef __cplusplus
extern "C" {
#endif

#include <stdlib.h>

// Forward declarations from astyle
typedef void (*fpError)(int errorNumber, const char* errorMessage);
typedef char* (*fpAlloc)(unsigned long memoryNeeded);

char* AStyleMain(const char* pSourceIn, const char* pOptions, fpError fpErrorHandler, fpAlloc fpMemoryAlloc);
const char* AStyleGetVersion(void);

// Error handler callback
void errorHandler(int errorNumber, const char* errorMessage) {
    // For now, we'll ignore errors or could log them
}

// Memory allocation callback
char* memoryAlloc(unsigned long memoryNeeded) {
    return (char*)malloc(memoryNeeded);
}

#ifdef __cplusplus
}
#endif
*/
import "C"

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"unsafe"
)

// FormatCodeWithAStyle formats code using the astyle C library
func FormatCodeWithAStyle(sourceCode, options string) (string, error) {
	// Convert Go strings to C strings
	cSourceCode := C.CString(sourceCode)
	defer C.free(unsafe.Pointer(cSourceCode))

	cOptions := C.CString(options)
	defer C.free(unsafe.Pointer(cOptions))

	// Call AStyleMain with our callbacks
	cResult := C.AStyleMain(
		cSourceCode,
		cOptions,
		C.fpError(C.errorHandler),
		C.fpAlloc(C.memoryAlloc),
	)

	if cResult == nil {
		return "", fmt.Errorf("astyle formatting failed")
	}

	// Convert result back to Go string
	result := C.GoString(cResult)

	// Free the memory allocated by astyle
	C.free(unsafe.Pointer(cResult))

	return result, nil
}

// extractVerbatimStrings extracts C# verbatim strings (@"...") from code,
// replacing them with placeholders. Returns the modified code and a map of
// placeholders to original strings.
func extractVerbatimStrings(code string) (string, map[string]string) {
	placeholders := make(map[string]string)
	counter := 0

	// Find all verbatim strings including those spanning multiple lines
	result := code
	i := 0
	for i < len(result) {
		// Look for @" pattern
		if i < len(result)-1 && result[i] == '@' && result[i+1] == '"' {
			start := i
			i += 2 // Skip @"

			// Find the closing quote (handle escaped quotes "")
			for i < len(result) {
				if result[i] == '"' {
					// Check if it's an escaped quote ""
					if i+1 < len(result) && result[i+1] == '"' {
						i += 2 // Skip both quotes
						continue
					}
					// Found the closing quote
					i++
					break
				}
				i++
			}

			// Extract the verbatim string
			verbatimStr := result[start:i]

			// Only replace if it contains newlines (multi-line string)
			if strings.Contains(verbatimStr, "\n") {
				placeholder := fmt.Sprintf("__VERBATIM_STRING_%d__", counter)
				counter++
				placeholders[placeholder] = verbatimStr

				// Replace in result
				result = result[:start] + placeholder + result[i:]
				i = start + len(placeholder)
			}
		} else {
			i++
		}
	}

	return result, placeholders
}

// restoreVerbatimStrings restores the original verbatim strings from placeholders
func restoreVerbatimStrings(code string, placeholders map[string]string) string {
	result := code
	for placeholder, original := range placeholders {
		result = strings.Replace(result, placeholder, original, 1)
	}
	return result
}

// isCSharpFile checks if the file is a C# file
func isCSharpFile(filePath string) bool {
	return strings.HasSuffix(strings.ToLower(filePath), ".cs")
}

// FormatFile formats a single file using astyle
func FormatFile(filePath, options string) error {
	// Read the file
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %v", filePath, err)
	}

	sourceCode := string(content)
	var placeholders map[string]string

	// For C# files, preserve verbatim strings before formatting
	if isCSharpFile(filePath) {
		sourceCode, placeholders = extractVerbatimStrings(sourceCode)
	}

	// Format the content
	formattedContent, err := FormatCodeWithAStyle(sourceCode, options)
	if err != nil {
		return fmt.Errorf("failed to format file %s: %v", filePath, err)
	}

	// For C# files, restore verbatim strings after formatting
	if isCSharpFile(filePath) && len(placeholders) > 0 {
		formattedContent = restoreVerbatimStrings(formattedContent, placeholders)
	}

	// Write the formatted content back to the file
	err = ioutil.WriteFile(filePath, []byte(formattedContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write formatted file %s: %v", filePath, err)
	}

	DebugLogPrintf("Successfully formatted: %s\n", filePath)
	return nil
}

// Suppress unused import warning
var _ = regexp.Compile

// GetAStyleVersion returns the version of astyle library
func GetAStyleVersion() string {
	cVersion := C.AStyleGetVersion()
	return C.GoString(cVersion)
}
