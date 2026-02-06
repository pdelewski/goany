package fs

import gofs "runtime/fs/go"

// Seek whence constants
const (
	SeekStart   = 0 // Seek from file start
	SeekCurrent = 1 // Seek from current position
	SeekEnd     = 2 // Seek from file end
)

// ReadFile reads the entire file and returns its content as a string.
// Returns (content, error) where error is empty string on success.
func ReadFile(path string) (string, string) {
	return gofs.ReadFile(path)
}

// WriteFile writes content to a file, creating it if it doesn't exist
// or overwriting it if it does. Returns error (empty string on success).
func WriteFile(path string, content string) string {
	return gofs.WriteFile(path, content)
}

// Exists checks if a file or directory exists at the given path.
func Exists(path string) bool {
	return gofs.Exists(path)
}

// Remove deletes a file. Returns error (empty string on success).
func Remove(path string) string {
	return gofs.Remove(path)
}

// Mkdir creates a directory. Returns error (empty string on success).
func Mkdir(path string) string {
	return gofs.Mkdir(path)
}

// MkdirAll creates a directory and all parent directories.
// Returns error (empty string on success).
func MkdirAll(path string) string {
	return gofs.MkdirAll(path)
}

// RemoveAll removes a file or directory and all its contents.
// Returns error (empty string on success).
func RemoveAll(path string) string {
	return gofs.RemoveAll(path)
}

// Open opens a file for reading and returns a handle.
// Returns (handle, error) where handle is -1 on error.
func Open(path string) (int, string) {
	return gofs.Open(path)
}

// Create creates or truncates a file for writing and returns a handle.
// Returns (handle, error) where handle is -1 on error.
func Create(path string) (int, string) {
	return gofs.Create(path)
}

// Close closes a file handle.
// Returns error (empty string on success).
func Close(handle int) string {
	return gofs.Close(handle)
}

// Read reads up to size bytes from the file.
// Returns (data, bytesRead, error).
func Read(handle int, size int) ([]byte, int, string) {
	return gofs.Read(handle, size)
}

// ReadAt reads up to size bytes from the file at the given offset.
// Does not change the file position.
// Returns (data, bytesRead, error).
func ReadAt(handle int, offset int64, size int) ([]byte, int, string) {
	return gofs.ReadAt(handle, offset, size)
}

// Write writes data to the file.
// Returns (bytesWritten, error).
func Write(handle int, data []byte) (int, string) {
	return gofs.Write(handle, data)
}

// Seek sets the file position for the next read/write.
// whence: SeekStart (0), SeekCurrent (1), or SeekEnd (2).
// Returns (newOffset, error).
func Seek(handle int, offset int64, whence int) (int64, string) {
	return gofs.Seek(handle, offset, whence)
}

// Size returns the size of the file in bytes.
// Returns (size, error).
func Size(handle int) (int64, string) {
	return gofs.Size(handle)
}
