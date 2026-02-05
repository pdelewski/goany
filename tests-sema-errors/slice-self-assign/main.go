package main

// ERROR: Slice self-assignment (Rust borrow checker)
func main() {
	items := []int{1, 2, 3}
	items = append(items, items[0]) // error: items borrowed and mutated simultaneously
}
