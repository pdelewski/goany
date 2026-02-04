package main

// ERROR: Collection mutation during iteration
func main() {
	items := []int{1, 2, 3}
	for i := 0; i < len(items); i++ {
		items = append(items, 4) // error: mutation during iteration
	}
}
