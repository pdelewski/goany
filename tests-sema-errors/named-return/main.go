package main

// ERROR: Named return values are not supported
func namedReturn() (result int) { // error: named return not allowed
	return 0
}

func main() {
}
