package main

// ERROR: Non-empty interfaces are not supported
type Reader interface {
	Read() int // error: interface methods not allowed
}

func main() {
}
