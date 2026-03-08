package main

import "fmt"

type Person struct {
	name string
	age  int
}

// ERROR: pointer to struct field assigned to another variable is not supported
func main() {
	s := Person{name: "Alice", age: 30}
	p := &s.age
	q := p // error: p holds &s.field and is assigned to another variable
	_ = q
	fmt.Println(s.age)
}
