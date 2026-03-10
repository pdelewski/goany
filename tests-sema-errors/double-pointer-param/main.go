package main

import "fmt"

// ERROR: Multi-level pointer in function parameter
func foo(pp **int) { // error: **int not supported
	fmt.Println(pp)
}

func main() {
	foo(nil)
}
