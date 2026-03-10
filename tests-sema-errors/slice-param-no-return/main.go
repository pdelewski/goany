package main

// ERROR: Slice parameter not returned
func process(items []int) {
	items[0] = 99
}

func main() {
	data := []int{1, 2, 3}
	process(data)
}
