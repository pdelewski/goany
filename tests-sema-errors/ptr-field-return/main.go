package main

type Person struct {
	name string
	age  int
}

// ERROR: address-of struct field returned from function is not supported
func getAgePtr(s Person) *int {
	return &s.age // error: &s.field cannot escape via return
}

func main() {
	s := Person{name: "Alice", age: 30}
	_ = getAgePtr(s)
}
