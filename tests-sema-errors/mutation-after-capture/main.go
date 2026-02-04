package main

// ERROR: Mutation of variable after closure capture
func main() {
	data := []int{1, 2, 3}

	fn := func() {
		_ = data // closure captures data
	}

	data = append(data, 4) // error: mutating after capture
	fn()
}
