package main

// ERROR: Self-referencing string concatenation (Rust borrow checker)
// Pattern: s += s + "..." is invalid because s is both borrowed (for +=)
// and moved (in the + expression) simultaneously.
func main() {
	s := "hello"
	s += s + " world" // error: s borrowed and moved in same statement
}
