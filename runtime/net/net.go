package net

import gonet "runtime/net/go"

// Dial connects to the address on the named network (e.g., "tcp").
// Returns (connection handle, error) where handle is -1 on error.
func Dial(network string, address string) (int, string) {
	return gonet.Dial(network, address)
}

// Listen announces on the local network address.
// Returns (listener handle, error) where handle is -1 on error.
func Listen(network string, address string) (int, string) {
	return gonet.Listen(network, address)
}

// Accept waits for and returns the next connection to the listener.
// Returns (connection handle, error) where handle is -1 on error.
func Accept(listener int) (int, string) {
	return gonet.Accept(listener)
}

// Read reads up to size bytes from the connection.
// Returns (data, bytesRead, error).
func Read(conn int, size int) ([]byte, int, string) {
	return gonet.Read(conn, size)
}

// Write writes data to the connection.
// Returns (bytesWritten, error).
func Write(conn int, data []byte) (int, string) {
	return gonet.Write(conn, data)
}

// Close closes the connection.
// Returns error (empty string on success).
func Close(conn int) string {
	return gonet.Close(conn)
}

// CloseListener closes the listener.
// Returns error (empty string on success).
func CloseListener(listener int) string {
	return gonet.CloseListener(listener)
}

// SetReadTimeout sets read timeout in milliseconds for a connection.
// Use 0 to disable timeout.
// Returns error (empty string on success).
func SetReadTimeout(conn int, timeoutMs int) string {
	return gonet.SetReadTimeout(conn, timeoutMs)
}
