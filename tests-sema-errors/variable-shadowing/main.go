package main

// ERROR: Variable shadowing (C# does not allow shadowing within same function)
// In C#, you cannot declare a variable with the same name in a nested scope
// if a variable with that name exists in an outer scope of the same function.
func main() {
	col := 0 // outer scope variable
	for {
		if col >= 10 {
			break
		}
		col := 1 // error: shadows outer 'col' - C# doesn't allow this
		_ = col
	}
	_ = col
}
