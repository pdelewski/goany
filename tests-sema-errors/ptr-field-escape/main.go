package main

import "fmt"

type Person struct {
	name string
	age  int
}

func modifyInt(p *int) {
	*p = 99
}

// ERROR: address-of struct field passed as argument is not supported
func main() {
	s := Person{name: "Alice", age: 30}
	modifyInt(&s.age) // error: &s.field cannot escape
	fmt.Println(s.age)
}
