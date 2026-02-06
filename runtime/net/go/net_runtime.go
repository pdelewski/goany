package gonet

import (
	"net"
	"sync"
	"time"
)

// Connection and listener handle management
var (
	connections      = make(map[int]net.Conn)
	listeners        = make(map[int]net.Listener)
	nextConnHandle   = 1
	nextListenHandle = 1
	connMutex        sync.Mutex
	listenMutex      sync.Mutex
)

// Dial connects to the address on the named network.
func Dial(network string, address string) (int, string) {
	conn, err := net.Dial(network, address)
	if err != nil {
		return -1, err.Error()
	}

	connMutex.Lock()
	handle := nextConnHandle
	nextConnHandle++
	connections[handle] = conn
	connMutex.Unlock()

	return handle, ""
}

// Listen announces on the local network address.
func Listen(network string, address string) (int, string) {
	listener, err := net.Listen(network, address)
	if err != nil {
		return -1, err.Error()
	}

	listenMutex.Lock()
	handle := nextListenHandle
	nextListenHandle++
	listeners[handle] = listener
	listenMutex.Unlock()

	return handle, ""
}

// Accept waits for and returns the next connection to the listener.
func Accept(listener int) (int, string) {
	listenMutex.Lock()
	l, ok := listeners[listener]
	listenMutex.Unlock()

	if !ok {
		return -1, "invalid listener handle"
	}

	conn, err := l.Accept()
	if err != nil {
		return -1, err.Error()
	}

	connMutex.Lock()
	handle := nextConnHandle
	nextConnHandle++
	connections[handle] = conn
	connMutex.Unlock()

	return handle, ""
}

// Read reads up to size bytes from the connection.
func Read(conn int, size int) ([]byte, int, string) {
	connMutex.Lock()
	c, ok := connections[conn]
	connMutex.Unlock()

	if !ok {
		return nil, 0, "invalid connection handle"
	}

	buf := make([]byte, size)
	n, err := c.Read(buf)
	if err != nil {
		return nil, 0, err.Error()
	}

	return buf[:n], n, ""
}

// Write writes data to the connection.
func Write(conn int, data []byte) (int, string) {
	connMutex.Lock()
	c, ok := connections[conn]
	connMutex.Unlock()

	if !ok {
		return 0, "invalid connection handle"
	}

	n, err := c.Write(data)
	if err != nil {
		return n, err.Error()
	}

	return n, ""
}

// Close closes the connection.
func Close(conn int) string {
	connMutex.Lock()
	c, ok := connections[conn]
	if !ok {
		connMutex.Unlock()
		return "invalid connection handle"
	}
	delete(connections, conn)
	connMutex.Unlock()

	err := c.Close()
	if err != nil {
		return err.Error()
	}
	return ""
}

// CloseListener closes the listener.
func CloseListener(listener int) string {
	listenMutex.Lock()
	l, ok := listeners[listener]
	if !ok {
		listenMutex.Unlock()
		return "invalid listener handle"
	}
	delete(listeners, listener)
	listenMutex.Unlock()

	err := l.Close()
	if err != nil {
		return err.Error()
	}
	return ""
}

// SetReadTimeout sets read timeout in milliseconds for a connection.
func SetReadTimeout(conn int, timeoutMs int) string {
	connMutex.Lock()
	c, ok := connections[conn]
	connMutex.Unlock()

	if !ok {
		return "invalid connection handle"
	}

	var deadline time.Time
	if timeoutMs > 0 {
		deadline = time.Now().Add(time.Duration(timeoutMs) * time.Millisecond)
	}

	err := c.SetReadDeadline(deadline)
	if err != nil {
		return err.Error()
	}
	return ""
}
