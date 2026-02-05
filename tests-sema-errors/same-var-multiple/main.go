package main

// ERROR: Same variable used multiple times in expression (Rust move semantics)
func process(a []int, b []int) []int {
	return append(a, b...)
}

func main() {
	data := []int{1, 2, 3}
	result := process(data, data) // error: data used twice, moved twice
	_ = result
}
