package main

import "fmt"

// ERROR: Taking address of a pointer creates **T which is not supported
func main() {
	x := 42
	p := &x
	pp := &p // error: &p creates **int
	_ = pp
	fmt.Println(x)
}
