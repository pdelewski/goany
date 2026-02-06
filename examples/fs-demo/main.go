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

	// ========== Chunked Read Tests ==========
	chunkTestFile := "/tmp/fs-demo-chunk-test.txt"
	chunkContent := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	// Create test file for chunked reading
	if allPassed {
		chunkWriteErr := fs.WriteFile(chunkTestFile, chunkContent)
		if chunkWriteErr != "" {
			fmt.Println("ChunkWrite error: " + chunkWriteErr)
			allPassed = false
		}
	}

	// Test Open
	if allPassed {
		handle, openErr := fs.Open(chunkTestFile)
		if openErr != "" {
			fmt.Println("Open error: " + openErr)
			allPassed = false
		} else {
			fmt.Println("Open: OK")

			// Test Size
			size, sizeErr := fs.Size(handle)
			if sizeErr != "" {
				fmt.Println("Size error: " + sizeErr)
				allPassed = false
			} else {
				if size == 26 {
					fmt.Println("Size: OK")
				} else {
					fmt.Println("Size: FAIL - expected 26")
					allPassed = false
				}
			}

			// Test Read (first 5 bytes: "ABCDE")
			if allPassed {
				data, bytesRead, readErr := fs.Read(handle, 5)
				if readErr != "" {
					fmt.Println("Read error: " + readErr)
					allPassed = false
				} else {
					if bytesRead == 5 && len(data) == 5 {
						if data[0] == 65 && data[4] == 69 {
							fmt.Println("Read chunk 1: OK")
						} else {
							fmt.Println("Read chunk 1: FAIL - wrong content")
							allPassed = false
						}
					} else {
						fmt.Println("Read chunk 1: FAIL - wrong bytes read")
						allPassed = false
					}
				}
			}

			// Test Read (next 5 bytes: "FGHIJ")
			if allPassed {
				data2, bytesRead2, readErr2 := fs.Read(handle, 5)
				if readErr2 != "" {
					fmt.Println("Read error: " + readErr2)
					allPassed = false
				} else {
					if bytesRead2 == 5 && data2[0] == 70 && data2[4] == 74 {
						fmt.Println("Read chunk 2: OK")
					} else {
						fmt.Println("Read chunk 2: FAIL")
						allPassed = false
					}
				}
			}

			// Test Seek (go back to start)
			if allPassed {
				newPos, seekErr := fs.Seek(handle, 0, fs.SeekStart)
				if seekErr != "" {
					fmt.Println("Seek error: " + seekErr)
					allPassed = false
				} else {
					if newPos == 0 {
						fmt.Println("Seek to start: OK")
					} else {
						fmt.Println("Seek to start: FAIL")
						allPassed = false
					}
				}
			}

			// Test ReadAt (read "KLMNO" at offset 10)
			if allPassed {
				dataAt, bytesReadAt, readAtErr := fs.ReadAt(handle, 10, 5)
				if readAtErr != "" {
					fmt.Println("ReadAt error: " + readAtErr)
					allPassed = false
				} else {
					if bytesReadAt == 5 && dataAt[0] == 75 && dataAt[4] == 79 {
						fmt.Println("ReadAt: OK")
					} else {
						fmt.Println("ReadAt: FAIL")
						allPassed = false
					}
				}
			}

			// Test Close
			closeErr := fs.Close(handle)
			if closeErr != "" {
				fmt.Println("Close error: " + closeErr)
				allPassed = false
			} else {
				fmt.Println("Close: OK")
			}
		}
	}

	// Cleanup chunk test file
	if allPassed {
		fs.Remove(chunkTestFile)
	}

	if allPassed {
		fmt.Println("All fs tests passed!")
	}
}
