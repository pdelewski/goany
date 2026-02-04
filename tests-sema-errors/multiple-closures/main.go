package main

// ERROR: Multiple closures capture same variable (Rust borrow checker)
func main() {
	data := []int{1, 2, 3}

	fn1 := func() {
		_ = data // first closure captures data
	}

	fn2 := func() {
		_ = data // error: second closure also captures data
	}

	fn1()
	fn2()
}
