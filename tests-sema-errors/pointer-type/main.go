package main

// ERROR: Pointer types are not supported
func main() {
	var p *int // error: pointer type not allowed
	_ = p
}
