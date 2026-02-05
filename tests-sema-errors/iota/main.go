package main

// ERROR: Iota is not supported
const (
	A = iota // error: iota not allowed
	B
	C
)

func main() {
}
