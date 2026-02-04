package main

// ERROR: Struct embedding is not supported
type Base struct {
	X int
}

type Derived struct {
	Base // error: embedding not allowed
	Y int
}

func main() {
}
