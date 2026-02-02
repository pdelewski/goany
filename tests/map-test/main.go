package main

import "fmt"

func main() {
	// String keys
	m := make(map[string]int)
	m["hello"] = 1
	m["world"] = 2
	fmt.Println(m["hello"])
	fmt.Println(m["world"])
	fmt.Println(len(m))
	delete(m, "hello")
	fmt.Println(len(m))

	// Int keys
	m2 := make(map[int]string)
	m2[42] = "answer"
	m2[7] = "lucky"
	fmt.Println(m2[42])
	fmt.Println(len(m2))

	// Bool keys
	m3 := make(map[bool]int)
	m3[true] = 1
	m3[false] = 0
	fmt.Println(m3[true])
	fmt.Println(m3[false])
}
