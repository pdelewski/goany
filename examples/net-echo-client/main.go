package main

import (
	"fmt"
	"runtime/net"
)

func main() {
	fmt.Println("=== TCP Echo Client ===")
	allPassed := true

	// Connect to echo server on localhost:9999
	conn, dialErr := net.Dial("tcp", "127.0.0.1:9999")
	if dialErr != "" {
		fmt.Println("FAIL: Dial error")
		fmt.Println(dialErr)
		allPassed = false
	}

	if allPassed {
		fmt.Println("Connected to server at 127.0.0.1:9999")

		// Send test message: "Hello Echo Server!"
		testMessage := []byte{
			72, 101, 108, 108, 111, 32, 69, 99, 104, 111, 32,
			83, 101, 114, 118, 101, 114, 33,
		}

		bytesWritten, writeErr := net.Write(conn, testMessage)
		if writeErr != "" {
			fmt.Println("FAIL: Write error")
			fmt.Println(writeErr)
			allPassed = false
		} else {
			fmt.Println("Sent bytes:")
			fmt.Println(bytesWritten)
		}
	}

	// Read echo response
	if allPassed {
		data, bytesRead, readErr := net.Read(conn, 1024)
		if readErr != "" {
			fmt.Println("FAIL: Read error")
			fmt.Println(readErr)
			allPassed = false
		} else {
			fmt.Println("Received bytes:")
			fmt.Println(bytesRead)

			// Verify echo matches: "Hello Echo Server!" = 18 bytes
			// H=72, e=101, l=108, l=108, o=111, space=32, E=69, c=99, h=104, o=111, space=32
			// S=83, e=101, r=114, v=118, e=101, r=114, !=33
			if bytesRead == 18 {
				if data[0] == 72 && data[5] == 32 && data[17] == 33 {
					fmt.Println("PASS: Echo response matches sent data")
				} else {
					fmt.Println("FAIL: Echo response content mismatch")
					allPassed = false
				}
			} else {
				fmt.Println("FAIL: Echo response length mismatch")
				allPassed = false
			}
		}
	}

	// Close connection
	if allPassed {
		closeErr := net.Close(conn)
		if closeErr != "" {
			fmt.Println("FAIL: Close error")
			fmt.Println(closeErr)
			allPassed = false
		} else {
			fmt.Println("Connection closed")
		}
	}

	if allPassed {
		fmt.Println("")
		fmt.Println("=== Echo client completed successfully ===")
	}
}
