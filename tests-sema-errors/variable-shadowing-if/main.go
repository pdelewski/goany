package main

// ERROR: Variable shadowing in if block
func main() {
	x := 5 // outer scope
	if true {
		x := 10 // error: shadows outer 'x'
		_ = x
	}
	_ = x
}
