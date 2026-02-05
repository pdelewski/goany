package main

import (
	"fmt"
	"runtime/http"
)

func helloHandler(req http.Request) http.Response {
	return http.Response{
		StatusCode: 200,
		Status:     "OK",
		Body:       "Hello, World!",
	}
}

func rootHandler(req http.Request) http.Response {
	return http.Response{
		StatusCode: 200,
		Status:     "OK",
		Body:       "Welcome to the HTTP server! Try /hello",
	}
}

func main() {
	http.HandleFunc("/hello", helloHandler)
	http.HandleFunc("/", rootHandler)

	fmt.Println("Starting server on :8089...")
	err := http.ListenAndServe(":8089")
	if err != "" {
		fmt.Println("Server error: " + err)
	}
}
