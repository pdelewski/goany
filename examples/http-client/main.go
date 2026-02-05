package main

import (
	"fmt"
	"runtime/http"
)

func main() {
	resp, err := http.Get("http://httpbin.org/get")
	if err != "" {
		panic(err)
	}
	fmt.Println(resp.StatusCode)
	fmt.Println(resp.Body)
}
