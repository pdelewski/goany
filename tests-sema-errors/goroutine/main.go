package main

// ERROR: Goroutines are not supported
func main() {
	go func() {}() // error: go keyword not allowed
}
