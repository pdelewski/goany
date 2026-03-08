package main

import "fmt"

type Person struct {
	name string
	age  int
}

func modifyInt(p *int) {
	*p = 99
}

// ERROR: pointer to struct field passed as argument is not supported
func main() {
	s := Person{name: "Alice", age: 30}
	p := &s.age
	modifyInt(p) // error: p holds &s.field and is passed to a function
	fmt.Println(s.age)
}
