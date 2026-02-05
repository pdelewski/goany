package main

// ERROR: Type switch is not supported
func main() {
	var x interface{}
	switch x.(type) { // error: type switch not allowed
	case int:
	}
}
