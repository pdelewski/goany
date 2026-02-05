package main

// ERROR: Variadic functions are not supported
func variadicFunc(args ...int) { // error: variadic not allowed
	_ = args
}

func main() {
}
