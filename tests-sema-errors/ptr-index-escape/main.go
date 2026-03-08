package main

import "fmt"

func modifyInt(p *int) {
	*p = 99
}

// ERROR: pointer to slice element passed as argument is not supported
func main() {
	arr := []int{1, 2, 3}
	p := &arr[1]
	modifyInt(p) // error: pointer to slice element cannot escape
	fmt.Println(*p)
}
