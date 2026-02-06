package fs

import gofs "runtime/fs/go"

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
