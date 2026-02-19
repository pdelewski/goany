package main

// ERROR: Structs with non-comparable fields cannot be used as map keys
type BadKey struct {
	Values []int // slice field - not supported as map key
}

func main() {
	m := make(map[BadKey]string) // error: slice field makes struct non-comparable
	_ = m
}
