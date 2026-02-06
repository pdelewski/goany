package main

import (
	"fmt"
	"runtime/net"
)

func main() {
	fmt.Println("=== TCP Echo Server ===")

	// Start listening on port 9999
	listener, listenErr := net.Listen("tcp", "127.0.0.1:9999")
	if listenErr != "" {
		panic("Listen error: " + listenErr)
	}

	fmt.Println("Server listening on 127.0.0.1:9999")
	fmt.Println("Waiting for client connection...")

	// Accept one client connection
	conn, acceptErr := net.Accept(listener)
	if acceptErr != "" {
		panic("Accept error: " + acceptErr)
	}

	fmt.Println("Client connected")

	// Read data from client
	data, bytesRead, readErr := net.Read(conn, 1024)
	if readErr != "" {
		panic("Read error: " + readErr)
	}

	if bytesRead > 0 {
		fmt.Println("Received bytes:")
		fmt.Println(bytesRead)

		// Echo data back to client
		bytesWritten, writeErr := net.Write(conn, data)
		if writeErr != "" {
			panic("Write error: " + writeErr)
		}
		fmt.Println("Echoed bytes:")
		fmt.Println(bytesWritten)
	}

	// Close client connection
	closeErr := net.Close(conn)
	if closeErr != "" {
		panic("Close connection error: " + closeErr)
	}

	// Close listener
	closeListenerErr := net.CloseListener(listener)
	if closeListenerErr != "" {
		panic("Close listener error: " + closeListenerErr)
	}

	fmt.Println("")
	fmt.Println("=== Echo server completed successfully ===")
}
