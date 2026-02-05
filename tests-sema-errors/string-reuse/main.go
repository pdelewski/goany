package main

// ERROR: String variable reuse after concatenation (Rust move semantics)
// In Rust, strings are moved (not copied) when concatenated.
// After `a + b`, the variable `a` is consumed and cannot be used again.
func main() {
	a := "hello"
	b := " world"
	c := a + b // a is moved/consumed here
	d := a + c // error: a was already moved in previous line
	_ = d
}
