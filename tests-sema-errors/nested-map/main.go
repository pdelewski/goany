package main

// ERROR: Nested maps are not supported
// Map values cannot themselves be maps.
func main() {
	m := make(map[string]map[string]int) // error: nested maps not allowed
	_ = m
}
