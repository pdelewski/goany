package main

import "fmt"

type Item struct {
	name string
}

// ERROR: Map with pointer values is not supported
func main() {
	m := make(map[string]*Item) // error: pointer values in maps
	m["a"] = &Item{name: "test"}
	fmt.Println(m)
}
