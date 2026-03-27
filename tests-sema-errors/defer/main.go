package main

// ERROR: defer inside loop is not supported
func main() {
	for i := 0; i < 10; i++ {
		defer println(i) // error: defer inside loop
	}
}
