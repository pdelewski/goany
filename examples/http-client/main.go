package main

import (
	"fmt"
	"runtime/http"
)

func main() {
	resp, err := http.Get("http://localhost:8089/hello")
	if err != "" {
		panic(err)
	}
	fmt.Println(resp.StatusCode)
	fmt.Println(resp.Body)
}
