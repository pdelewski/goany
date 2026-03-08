package main

type Person struct {
	name string
	age  int
}

// ERROR: pointer to struct field returned from function is not supported
func getAgePtr(s Person) *int {
	p := &s.age
	return p // error: p holds &s.field and is returned
}

func main() {
	s := Person{name: "Alice", age: 30}
	_ = getAgePtr(s)
}
