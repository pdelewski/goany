package main

import (
	"fmt"
	"runtime/net"
)

func main() {
	fmt.Println("=== TCP Echo Server ===")
	allPassed := true

	// Start listening on port 9999
	listener, listenErr := net.Listen("tcp", "127.0.0.1:9999")
	if listenErr != "" {
		fmt.Println("FAIL: Listen error")
		fmt.Println(listenErr)
		allPassed = false
	}

	if allPassed {
		fmt.Println("Server listening on 127.0.0.1:9999")
		fmt.Println("Waiting for client connection...")

		// Accept one client connection
		conn, acceptErr := net.Accept(listener)
		if acceptErr != "" {
			fmt.Println("FAIL: Accept error")
			fmt.Println(acceptErr)
			allPassed = false
		}

		if allPassed {
			fmt.Println("Client connected")

			// Read data from client
			data, bytesRead, readErr := net.Read(conn, 1024)
			if readErr != "" {
				fmt.Println("FAIL: Read error")
				fmt.Println(readErr)
				allPassed = false
			}

			if allPassed && bytesRead > 0 {
				fmt.Println("Received bytes:")
				fmt.Println(bytesRead)

				// Echo data back to client
				bytesWritten, writeErr := net.Write(conn, data)
				if writeErr != "" {
					fmt.Println("FAIL: Write error")
					fmt.Println(writeErr)
					allPassed = false
				} else {
					fmt.Println("Echoed bytes:")
					fmt.Println(bytesWritten)
				}
			}

			// Close client connection
			closeErr := net.Close(conn)
			if closeErr != "" {
				fmt.Println("FAIL: Close connection error")
				fmt.Println(closeErr)
				allPassed = false
			}
		}

		// Close listener
		closeListenerErr := net.CloseListener(listener)
		if closeListenerErr != "" {
			fmt.Println("FAIL: Close listener error")
			fmt.Println(closeListenerErr)
			allPassed = false
		}
	}

	if allPassed {
		fmt.Println("")
		fmt.Println("=== Echo server completed successfully ===")
	}
}
