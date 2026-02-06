package main

import (
	"fmt"
	"runtime/net"
)

func main() {
	fmt.Println("=== TCP Echo Client ===")

	// Connect to echo server on localhost:9999
	conn, dialErr := net.Dial("tcp", "127.0.0.1:9999")
	if dialErr != "" {
		panic("Dial error: " + dialErr)
	}
	fmt.Println("Connected to server at 127.0.0.1:9999")

	// Send test message: "Hello Echo Server!"
	testMessage := []byte{
		72, 101, 108, 108, 111, 32, 69, 99, 104, 111, 32,
		83, 101, 114, 118, 101, 114, 33,
	}

	bytesWritten, writeErr := net.Write(conn, testMessage)
	if writeErr != "" {
		panic("Write error: " + writeErr)
	}
	fmt.Println("Sent bytes:")
	fmt.Println(bytesWritten)

	// Read echo response
	data, bytesRead, readErr := net.Read(conn, 1024)
	if readErr != "" {
		panic("Read error: " + readErr)
	}
	fmt.Println("Received bytes:")
	fmt.Println(bytesRead)

	// Verify echo matches: "Hello Echo Server!" = 18 bytes
	// H=72, e=101, l=108, l=108, o=111, space=32, E=69, c=99, h=104, o=111, space=32
	// S=83, e=101, r=114, v=118, e=101, r=114, !=33
	if bytesRead != 18 {
		panic("Echo response length mismatch")
	}
	if data[0] != 72 || data[5] != 32 || data[17] != 33 {
		panic("Echo response content mismatch")
	}
	fmt.Println("PASS: Echo response matches sent data")

	// Close connection
	closeErr := net.Close(conn)
	if closeErr != "" {
		panic("Close error: " + closeErr)
	}
	fmt.Println("Connection closed")

	fmt.Println("")
	fmt.Println("=== Echo client completed successfully ===")
}
