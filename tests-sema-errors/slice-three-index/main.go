package main

// ERROR: Three-index slice expression is not supported
func main() {
	s := []int{1, 2, 3, 4, 5}
	sub := s[1:3:4] // error: full slice expression with capacity not allowed
	_ = sub
}
