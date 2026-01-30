package containers

import "fmt"

// ListNode represents a node in the list using an array-based approach
type ListNode struct {
	value int // The value of the node
	next  int // The index of the next node (-1 if no next node)
}

// List represents the array-based list
type List struct {
	nodes []ListNode // The array storing the list nodes
	head  int        // The index of the head node (-1 if the list is empty)
}

// NewList creates a new empty list and returns it as a value
func NewList() List {
	return List{
		nodes: []ListNode{}, // Initialize with an empty slice of nodes
		head:  -1,           // -1 indicates the list is empty
	}
}

// Add adds a new value to the end of the list and returns the modified list
func Add(l List, value int) List {
	newNode := ListNode{
		value: value,
		next:  -1, // No next node as it will be the last one
	}

	// Add the new node to the array
	l.nodes = append(l.nodes, newNode)

	// If the list is empty, set the head to the new node's index
	if l.head == -1 {
		l.head = len(l.nodes) - 1
	} else {
		// Otherwise, find the last node and link it to the new node
		lastNodeIndex := l.head
		for l.nodes[lastNodeIndex].next != -1 {
			lastNodeIndex = l.nodes[lastNodeIndex].next
		}
		// TODO original code was about mutating this in-place l.nodes[lastNodeIndex].next = len(l.nodes) - 1
		// this is problematic now from c# perspective
		// as it causes Cannot modify the return value of 'List<Api.ListNode>.this[int]' because it is not a variable
		// to fix that we have to discover this case and transform code as below
		tmp := l.nodes[lastNodeIndex]
		tmp.next = len(l.nodes) - 1 // Set the next of the last node to the new node's index
		l.nodes[lastNodeIndex] = tmp
	}

	return l // Return the updated list
}

// Remove removes the first occurrence of a value in the list and returns the modified list
func Remove(l List, value int) List {
	if l.head == -1 {
		fmt.Println("The list is empty.")
		return l
	}

	// If the head node is the one to be removed
	if l.nodes[l.head].value == value {
		l.head = l.nodes[l.head].next
		return l
	}

	prevIndex := -1
	currIndex := l.head
	for currIndex != -1 {
		if l.nodes[currIndex].value == value {
			if prevIndex == -1 {
				l.head = l.nodes[currIndex].next
			} else {
				// TODO original code  code was about mutating this in-place l.nodes[currIndex].next = l.nodes[prevIndex]
				// this is problematic now from c# perspective
				// as it causes Cannot modify the return value of 'List<Api.ListNode>.this[int]' because it is not a variable
				// to fix that we have to discover this case and transform code as below
				tmp := l.nodes[prevIndex]
				tmp.next = l.nodes[currIndex].next
				l.nodes[prevIndex] = tmp // Update the previous node's next pointer
			}
			return l
		}
		prevIndex = currIndex
		currIndex = l.nodes[currIndex].next
	}

	fmt.Println("Value not found in the list.")
	return l
}

// Print prints the list
func PrintList(l List) {
	if l.head == -1 {
		fmt.Println("The list is empty.")
		return
	}

	currIndex := l.head
	for currIndex != -1 {
		fmt.Printf("%d -> ", l.nodes[currIndex].value)
		currIndex = l.nodes[currIndex].next
	}
	fmt.Println("nil")
}

// Size returns the number of elements in the list
func Size(l List) int {
	count := 0
	currIndex := l.head
	for currIndex != -1 {
		count = count + 1
		currIndex = l.nodes[currIndex].next
	}
	return count
}

// Find returns the logical position of the first occurrence of value, or -1 if not found
func Find(l List, value int) int {
	position := 0
	currIndex := l.head
	for currIndex != -1 {
		if l.nodes[currIndex].value == value {
			return position
		}
		position = position + 1
		currIndex = l.nodes[currIndex].next
	}
	return -1
}

// Contains returns true if the list contains the given value
func Contains(l List, value int) bool {
	return Find(l, value) >= 0
}

// Get returns the value at the given logical index, or -1 if out of bounds
func Get(l List, index int) int {
	position := 0
	currIndex := l.head
	for currIndex != -1 {
		if position == index {
			return l.nodes[currIndex].value
		}
		position = position + 1
		currIndex = l.nodes[currIndex].next
	}
	return -1
}

// ToSlice returns a slice containing all values in the list
func ToSlice(l List) []int {
	result := []int{}
	currIndex := l.head
	for currIndex != -1 {
		result = append(result, l.nodes[currIndex].value)
		currIndex = l.nodes[currIndex].next
	}
	return result
}

// FromSlice creates a new list from a slice of values
func FromSlice(values []int) List {
	l := NewList()
	i := 0
	for i < len(values) {
		l = Add(l, values[i])
		i = i + 1
	}
	return l
}

// InsertAt inserts a value at the given logical position
func InsertAt(l List, index int, value int) List {
	newNode := ListNode{
		value: value,
		next:  -1,
	}
	l.nodes = append(l.nodes, newNode)
	newIndex := len(l.nodes) - 1

	if index == 0 {
		tmp := l.nodes[newIndex]
		tmp.next = l.head
		l.nodes[newIndex] = tmp
		l.head = newIndex
		return l
	}

	position := 0
	currIndex := l.head
	for currIndex != -1 {
		if position == index-1 {
			tmp := l.nodes[newIndex]
			tmp.next = l.nodes[currIndex].next
			l.nodes[newIndex] = tmp
			tmp2 := l.nodes[currIndex]
			tmp2.next = newIndex
			l.nodes[currIndex] = tmp2
			return l
		}
		position = position + 1
		currIndex = l.nodes[currIndex].next
	}

	return l
}

// RemoveAt removes the element at the given logical position
func RemoveAt(l List, index int) List {
	if l.head == -1 {
		return l
	}

	if index == 0 {
		l.head = l.nodes[l.head].next
		return l
	}

	position := 0
	currIndex := l.head
	for currIndex != -1 {
		if position == index-1 {
			targetIndex := l.nodes[currIndex].next
			if targetIndex != -1 {
				tmp := l.nodes[currIndex]
				tmp.next = l.nodes[targetIndex].next
				l.nodes[currIndex] = tmp
			}
			return l
		}
		position = position + 1
		currIndex = l.nodes[currIndex].next
	}

	return l
}

// Reverse reverses the list in place using three-pointer reversal
func Reverse(l List) List {
	prevIndex := -1
	currIndex := l.head
	for currIndex != -1 {
		nextIndex := l.nodes[currIndex].next
		tmp := l.nodes[currIndex]
		tmp.next = prevIndex
		l.nodes[currIndex] = tmp
		prevIndex = currIndex
		currIndex = nextIndex
	}
	l.head = prevIndex
	return l
}
