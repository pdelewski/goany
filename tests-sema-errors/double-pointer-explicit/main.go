package main

import "fmt"

// ERROR: Multi-level pointer types are not supported
func main() {
	var pp **int // error: **int not supported
	_ = pp
	fmt.Println("hello")
}
