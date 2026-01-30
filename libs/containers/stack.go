package containers

import "fmt"

// Stack represents a stack using a slice
type Stack struct {
	items []int
}

// NewStack creates a new empty stack
func NewStack() Stack {
	return Stack{
		items: []int{},
	}
}

// Push adds a value to the top of the stack
func Push(s Stack, value int) Stack {
	s.items = append(s.items, value)
	return s
}

// Pop removes and returns the top value from the stack
func Pop(s Stack) (Stack, int) {
	length := len(s.items)
	value := s.items[length-1]
	newItems := []int{}
	i := 0
	for i < length-1 {
		newItems = append(newItems, s.items[i])
		i = i + 1
	}
	s.items = newItems
	return s, value
}

// Peek returns the top value without removing it
func Peek(s Stack) int {
	return s.items[len(s.items)-1]
}

// StackIsEmpty returns true if the stack has no elements
func StackIsEmpty(s Stack) bool {
	return len(s.items) == 0
}

// StackSize returns the number of elements in the stack
func StackSize(s Stack) int {
	return len(s.items)
}

// PrintStack prints the stack contents from top to bottom
func PrintStack(s Stack) {
	i := len(s.items)
	for i > 0 {
		i = i - 1
		fmt.Printf("%d ", s.items[i])
	}
	fmt.Println()
}
