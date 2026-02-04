package main

// ERROR: Defer statements are not supported
func main() {
	defer println("cleanup") // error: defer not allowed
}
