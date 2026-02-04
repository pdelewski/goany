package main

// ERROR: Channel types are not supported
func main() {
	ch := make(chan int) // error: channel type not allowed
	_ = ch
}
