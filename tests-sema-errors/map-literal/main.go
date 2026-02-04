package main

// ERROR: Map literals are not supported
// Inline map initialization is not allowed.
func main() {
	m := map[string]int{"a": 1, "b": 2} // error: map literals not allowed
	_ = m
}
