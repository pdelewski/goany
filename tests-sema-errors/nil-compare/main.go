package main

// ERROR: Nil comparison is not supported
func main() {
	var s []int
	if s == nil { // error: nil comparison not allowed
	}
}
