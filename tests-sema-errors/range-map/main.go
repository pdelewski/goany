package main

// ERROR: Range over maps is not supported
// Map iteration order is undefined and varies across languages.
func main() {
	m := make(map[string]int)
	m["a"] = 1
	for k, v := range m { // error: range over maps not allowed
		_ = k
		_ = v
	}
}
