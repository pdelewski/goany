package gofs

import (
	"os"
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
