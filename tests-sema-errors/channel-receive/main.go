package main

// ERROR: Channel receive is not supported
func main() {
	var ch chan int
	v := <-ch // error: channel receive operator not allowed
	_ = v
}
