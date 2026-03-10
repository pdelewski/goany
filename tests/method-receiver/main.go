package main

import "fmt"

type Counter struct {
	count int
}

// Pointer receiver - mutates state
func (c *Counter) Inc() {
	c.count += 1
}

// Pointer receiver with parameter
func (c *Counter) Add(n int) {
	c.count += n
}

// Value receiver - reads state
func (c Counter) Value() int {
	return c.count
}

// Pointer receiver - method calling another method on same receiver
func (c *Counter) Reset() {
	c.count = 0
}

func (c *Counter) ResetAndInc() {
	c.Reset()
	c.Inc()
}

func main() {
	c := Counter{count: 0}

	// Test pointer receiver mutation
	c.Inc()
	c.Inc()
	c.Inc()
	if c.Value() == 3 {
		fmt.Println("PASS: Inc and Value")
	} else {
		panic("FAIL: Inc and Value")
	}

	// Test method with parameter
	c.Add(7)
	if c.Value() == 10 {
		fmt.Println("PASS: Add")
	} else {
		panic("FAIL: Add")
	}

	// Test method calling another method (self-call)
	c.ResetAndInc()
	if c.Value() == 1 {
		fmt.Println("PASS: ResetAndInc")
	} else {
		panic("FAIL: ResetAndInc")
	}

	// Test multiple calls to same method
	c.Reset()
	c.Inc()
	c.Inc()
	c.Inc()
	c.Inc()
	c.Inc()
	if c.Value() == 5 {
		fmt.Println("PASS: Multiple calls")
	} else {
		panic("FAIL: Multiple calls")
	}
}
