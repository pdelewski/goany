package main

import (
	"fmt"
	"runtime/net"
)

func main() {
	fmt.Println("=== TCP Client Demo ===")

	// Connect to httpbin.org on port 80 using raw TCP
	conn, dialErr := net.Dial("tcp", "httpbin.org:80")
	if dialErr != "" {
		panic("Dial error: " + dialErr)
	}
	fmt.Println("Connected to httpbin.org:80")

	// Send a simple HTTP GET request manually using raw bytes
	// GET /get HTTP/1.1\r\nHost: httpbin.org\r\nConnection: close\r\n\r\n
	requestBytes := []byte{
		71, 69, 84, 32, 47, 103, 101, 116, 32, 72, 84, 84, 80, 47, 49, 46, 49, 13, 10,
		72, 111, 115, 116, 58, 32, 104, 116, 116, 112, 98, 105, 110, 46, 111, 114, 103, 13, 10,
		67, 111, 110, 110, 101, 99, 116, 105, 111, 110, 58, 32, 99, 108, 111, 115, 101, 13, 10,
		13, 10,
	}

	bytesWritten, writeErr := net.Write(conn, requestBytes)
	if writeErr != "" {
		panic("Write error: " + writeErr)
	}
	fmt.Println("Sent bytes:")
	fmt.Println(bytesWritten)

	// Read the response and count total bytes received
	totalBytes := 0
	foundHTTP := false
	for {
		data, bytesRead, readErr := net.Read(conn, 1024)
		if bytesRead == 0 {
			break
		}
		if readErr != "" {
			// EOF or connection closed is expected
			break
		}
		totalBytes = totalBytes + bytesRead

		// Check if response starts with HTTP (72=H, 84=T, 84=T, 80=P)
		if bytesRead >= 4 && data[0] == 72 && data[1] == 84 && data[2] == 84 && data[3] == 80 {
			foundHTTP = true
		}
	}

	// Close the connection
	closeErr := net.Close(conn)
	if closeErr != "" {
		panic("Close error: " + closeErr)
	}
	fmt.Println("Connection closed")

	// Check we received data
	fmt.Println("Total bytes received:")
	fmt.Println(totalBytes)
	if totalBytes <= 100 {
		panic("Did not receive enough data")
	}
	fmt.Println("PASS: Received substantial response")

	// Check we found HTTP in response
	if !foundHTTP {
		panic("Response does not start with HTTP")
	}
	fmt.Println("PASS: Response starts with HTTP")

	fmt.Println("")
	fmt.Println("=== All TCP tests passed ===")
}
