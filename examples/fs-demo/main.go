package main

import (
	"fmt"
	"runtime/fs"
)

func main() {
	testFile := "/tmp/fs-demo-test.txt"
	testDir := "/tmp/fs-demo-testdir"
	testContent := "Hello from fs-demo!"
	allPassed := true

	// Test WriteFile
	writeErr := fs.WriteFile(testFile, testContent)
	if writeErr != "" {
		fmt.Println("WriteFile error: " + writeErr)
		allPassed = false
	} else {
		fmt.Println("WriteFile: OK")
	}

	// Test Exists
	if allPassed {
		if fs.Exists(testFile) {
			fmt.Println("Exists: OK")
		} else {
			fmt.Println("Exists: FAIL - file should exist")
			allPassed = false
		}
	}

	// Test ReadFile
	if allPassed {
		content, readErr := fs.ReadFile(testFile)
		if readErr != "" {
			fmt.Println("ReadFile error: " + readErr)
			allPassed = false
		} else {
			if content == testContent {
				fmt.Println("ReadFile: OK")
			} else {
				fmt.Println("ReadFile: FAIL - content mismatch")
				allPassed = false
			}
		}
	}

	// Test Mkdir
	if allPassed {
		mkdirErr := fs.Mkdir(testDir)
		if mkdirErr != "" {
			fmt.Println("Mkdir error: " + mkdirErr)
			allPassed = false
		} else {
			if fs.Exists(testDir) {
				fmt.Println("Mkdir: OK")
			} else {
				fmt.Println("Mkdir: FAIL - directory should exist")
				allPassed = false
			}
		}
	}

	// Test Remove (file)
	if allPassed {
		removeErr := fs.Remove(testFile)
		if removeErr != "" {
			fmt.Println("Remove error: " + removeErr)
			allPassed = false
		} else {
			if !fs.Exists(testFile) {
				fmt.Println("Remove: OK")
			} else {
				fmt.Println("Remove: FAIL - file should not exist")
				allPassed = false
			}
		}
	}

	// Test RemoveAll (directory)
	if allPassed {
		removeAllErr := fs.RemoveAll(testDir)
		if removeAllErr != "" {
			fmt.Println("RemoveAll error: " + removeAllErr)
			allPassed = false
		} else {
			if !fs.Exists(testDir) {
				fmt.Println("RemoveAll: OK")
			} else {
				fmt.Println("RemoveAll: FAIL - directory should not exist")
				allPassed = false
			}
		}
	}

	if allPassed {
		fmt.Println("All fs tests passed!")
	}
}
