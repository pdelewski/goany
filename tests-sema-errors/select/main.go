package main

// ERROR: Select statements are not supported
func main() {
	ch := make(chan int)
	select { // error: select not allowed
	case <-ch:
	}
}
