package main

// ERROR: Address-of operator is not supported
func main() {
	x := 5
	p := &x // error: taking address not allowed
	_ = p
}
