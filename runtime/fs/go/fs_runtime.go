package gofs

import (
	"io"
	"os"
	"sync"
)

// File handle management
var (
	fileHandles   = make(map[int]*os.File)
	nextHandle    = 1
	handlesMutex  sync.Mutex
)

// ReadFile reads the entire file and returns its content as a string.
func ReadFile(path string) (string, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err.Error()
	}
	return string(data), ""
}

// WriteFile writes content to a file.
func WriteFile(path string, content string) string {
	err := os.WriteFile(path, []byte(content), 0644)
	if err != nil {
		return err.Error()
	}
	return ""
}

// Exists checks if a file or directory exists.
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Remove deletes a file.
func Remove(path string) string {
	err := os.Remove(path)
	if err != nil {
		return err.Error()
	}
	return ""
}

// Mkdir creates a directory.
func Mkdir(path string) string {
	err := os.Mkdir(path, 0755)
	if err != nil {
		return err.Error()
	}
	return ""
}

// MkdirAll creates a directory and all parent directories.
func MkdirAll(path string) string {
	err := os.MkdirAll(path, 0755)
	if err != nil {
		return err.Error()
	}
	return ""
}

// RemoveAll removes a file or directory and all its contents.
func RemoveAll(path string) string {
	err := os.RemoveAll(path)
	if err != nil {
		return err.Error()
	}
	return ""
}

// Open opens a file for reading and returns a handle.
func Open(path string) (int, string) {
	f, err := os.Open(path)
	if err != nil {
		return -1, err.Error()
	}

	handlesMutex.Lock()
	handle := nextHandle
	nextHandle++
	fileHandles[handle] = f
	handlesMutex.Unlock()

	return handle, ""
}

// Create creates or truncates a file for writing and returns a handle.
func Create(path string) (int, string) {
	f, err := os.Create(path)
	if err != nil {
		return -1, err.Error()
	}

	handlesMutex.Lock()
	handle := nextHandle
	nextHandle++
	fileHandles[handle] = f
	handlesMutex.Unlock()

	return handle, ""
}

// Close closes a file handle.
func Close(handle int) string {
	handlesMutex.Lock()
	f, ok := fileHandles[handle]
	if !ok {
		handlesMutex.Unlock()
		return "invalid file handle"
	}
	delete(fileHandles, handle)
	handlesMutex.Unlock()

	err := f.Close()
	if err != nil {
		return err.Error()
	}
	return ""
}

// Read reads up to size bytes from the file.
func Read(handle int, size int) ([]byte, int, string) {
	handlesMutex.Lock()
	f, ok := fileHandles[handle]
	handlesMutex.Unlock()

	if !ok {
		return nil, 0, "invalid file handle"
	}

	buf := make([]byte, size)
	n, err := f.Read(buf)
	if err != nil && err != io.EOF {
		return nil, 0, err.Error()
	}

	return buf[:n], n, ""
}

// ReadAt reads up to size bytes from the file at the given offset.
func ReadAt(handle int, offset int64, size int) ([]byte, int, string) {
	handlesMutex.Lock()
	f, ok := fileHandles[handle]
	handlesMutex.Unlock()

	if !ok {
		return nil, 0, "invalid file handle"
	}

	buf := make([]byte, size)
	n, err := f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil, 0, err.Error()
	}

	return buf[:n], n, ""
}

// Write writes data to the file.
func Write(handle int, data []byte) (int, string) {
	handlesMutex.Lock()
	f, ok := fileHandles[handle]
	handlesMutex.Unlock()

	if !ok {
		return 0, "invalid file handle"
	}

	n, err := f.Write(data)
	if err != nil {
		return n, err.Error()
	}

	return n, ""
}

// Seek sets the file position for the next read/write.
func Seek(handle int, offset int64, whence int) (int64, string) {
	handlesMutex.Lock()
	f, ok := fileHandles[handle]
	handlesMutex.Unlock()

	if !ok {
		return 0, "invalid file handle"
	}

	newOffset, err := f.Seek(offset, whence)
	if err != nil {
		return 0, err.Error()
	}

	return newOffset, ""
}

// Size returns the size of the file in bytes.
func Size(handle int) (int64, string) {
	handlesMutex.Lock()
	f, ok := fileHandles[handle]
	handlesMutex.Unlock()

	if !ok {
		return 0, "invalid file handle"
	}

	info, err := f.Stat()
	if err != nil {
		return 0, err.Error()
	}

	return info.Size(), ""
}
