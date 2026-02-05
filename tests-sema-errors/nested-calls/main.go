package main

// ERROR: Nested function calls share non-Copy variable (Rust move semantics)
func process(x []int, y []int) []int {
	return append(x, y...)
}

func transform(x []int) []int {
	return x
}

func main() {
	data := []int{1, 2, 3}
	result := process(data, transform(data)) // error: data used directly AND in nested call
	_ = result
}
