package main

// ERROR: Range over inline slice literal is not supported
func main() {
	for _, x := range []int{1, 2, 3} { // error: inline slice literal not allowed
		_ = x
	}
}
