package main

import (
	"fmt"
	"runtime/fs"
)

func main() {
	testFile := "/tmp/fs-demo-test.txt"
	testDir := "/tmp/fs-demo-testdir"
	testContent := "Hello from fs-demo!"

	// Test WriteFile
	writeErr := fs.WriteFile(testFile, testContent)
	if writeErr != "" {
		panic("WriteFile error: " + writeErr)
	}
	fmt.Println("WriteFile: OK")

	// Test Exists
	if !fs.Exists(testFile) {
		panic("Exists: FAIL - file should exist")
	}
	fmt.Println("Exists: OK")

	// Test ReadFile
	content, readErr := fs.ReadFile(testFile)
	if readErr != "" {
		panic("ReadFile error: " + readErr)
	}
	if content != testContent {
		panic("ReadFile: FAIL - content mismatch")
	}
	fmt.Println("ReadFile: OK")

	// Test Mkdir
	mkdirErr := fs.Mkdir(testDir)
	if mkdirErr != "" {
		panic("Mkdir error: " + mkdirErr)
	}
	if !fs.Exists(testDir) {
		panic("Mkdir: FAIL - directory should exist")
	}
	fmt.Println("Mkdir: OK")

	// Test Remove (file)
	removeErr := fs.Remove(testFile)
	if removeErr != "" {
		panic("Remove error: " + removeErr)
	}
	if fs.Exists(testFile) {
		panic("Remove: FAIL - file should not exist")
	}
	fmt.Println("Remove: OK")

	// Test RemoveAll (directory)
	removeAllErr := fs.RemoveAll(testDir)
	if removeAllErr != "" {
		panic("RemoveAll error: " + removeAllErr)
	}
	if fs.Exists(testDir) {
		panic("RemoveAll: FAIL - directory should not exist")
	}
	fmt.Println("RemoveAll: OK")

	// ========== Chunked Read Tests ==========
	chunkTestFile := "/tmp/fs-demo-chunk-test.txt"
	chunkContent := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"

	// Create test file for chunked reading
	chunkWriteErr := fs.WriteFile(chunkTestFile, chunkContent)
	if chunkWriteErr != "" {
		panic("ChunkWrite error: " + chunkWriteErr)
	}

	// Test Open
	handle, openErr := fs.Open(chunkTestFile)
	if openErr != "" {
		panic("Open error: " + openErr)
	}
	fmt.Println("Open: OK")

	// Test Size
	size, sizeErr := fs.Size(handle)
	if sizeErr != "" {
		panic("Size error: " + sizeErr)
	}
	if size != 26 {
		panic("Size: FAIL - expected 26")
	}
	fmt.Println("Size: OK")

	// Test Read (first 5 bytes: "ABCDE")
	data, bytesRead, readChunkErr := fs.Read(handle, 5)
	if readChunkErr != "" {
		panic("Read error: " + readChunkErr)
	}
	if bytesRead != 5 || len(data) != 5 {
		panic("Read chunk 1: FAIL - wrong bytes read")
	}
	if data[0] != 65 || data[4] != 69 {
		panic("Read chunk 1: FAIL - wrong content")
	}
	fmt.Println("Read chunk 1: OK")

	// Test Read (next 5 bytes: "FGHIJ")
	data2, bytesRead2, readChunkErr2 := fs.Read(handle, 5)
	if readChunkErr2 != "" {
		panic("Read error: " + readChunkErr2)
	}
	if bytesRead2 != 5 || data2[0] != 70 || data2[4] != 74 {
		panic("Read chunk 2: FAIL")
	}
	fmt.Println("Read chunk 2: OK")

	// Test Seek (go back to start)
	newPos, seekErr := fs.Seek(handle, 0, fs.SeekStart)
	if seekErr != "" {
		panic("Seek error: " + seekErr)
	}
	if newPos != 0 {
		panic("Seek to start: FAIL")
	}
	fmt.Println("Seek to start: OK")

	// Test ReadAt (read "KLMNO" at offset 10)
	dataAt, bytesReadAt, readAtErr := fs.ReadAt(handle, 10, 5)
	if readAtErr != "" {
		panic("ReadAt error: " + readAtErr)
	}
	if bytesReadAt != 5 || dataAt[0] != 75 || dataAt[4] != 79 {
		panic("ReadAt: FAIL")
	}
	fmt.Println("ReadAt: OK")

	// Test Close
	closeErr := fs.Close(handle)
	if closeErr != "" {
		panic("Close error: " + closeErr)
	}
	fmt.Println("Close: OK")

	// Cleanup chunk test file
	fs.Remove(chunkTestFile)

	fmt.Println("All fs tests passed!")
}
