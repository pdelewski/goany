package main

// ERROR: Structs with non-primitive fields cannot be used as map keys
// Only structs with primitive fields (int, string, bool, floats) are allowed.
type Inner struct {
	X int
}

type Outer struct {
	I Inner // struct field - not a primitive type
}

func main() {
	m := make(map[Outer]string) // error: struct with non-primitive field as key
	_ = m
}
