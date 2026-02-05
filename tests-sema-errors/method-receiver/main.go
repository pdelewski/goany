package main

// ERROR: Method receivers are not supported
type MyType struct {
	Value int
}

func (m MyType) GetValue() int { // error: method receiver not allowed
	return m.Value
}

func main() {
}
