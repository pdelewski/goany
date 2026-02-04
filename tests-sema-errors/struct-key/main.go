package main

// ERROR: Structs as map keys are not supported
// Only primitive types (string, int, bool, etc.) can be map keys.
type Point struct {
	X, Y int
}

func main() {
	m := make(map[Point]string) // error: struct key type not allowed
	_ = m
}
