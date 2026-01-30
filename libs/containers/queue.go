package containers

import "fmt"

// Queue represents a queue using a slice
type Queue struct {
	items []int
}

// NewQueue creates a new empty queue
func NewQueue() Queue {
	return Queue{
		items: []int{},
	}
}

// Enqueue adds a value to the back of the queue
func Enqueue(q Queue, value int) Queue {
	q.items = append(q.items, value)
	return q
}

// Dequeue removes and returns the front value from the queue
func Dequeue(q Queue) (Queue, int) {
	value := q.items[0]
	newItems := []int{}
	i := 1
	for i < len(q.items) {
		newItems = append(newItems, q.items[i])
		i = i + 1
	}
	q.items = newItems
	return q, value
}

// QueuePeek returns the front value without removing it
func QueuePeek(q Queue) int {
	return q.items[0]
}

// QueueIsEmpty returns true if the queue has no elements
func QueueIsEmpty(q Queue) bool {
	return len(q.items) == 0
}

// QueueSize returns the number of elements in the queue
func QueueSize(q Queue) int {
	return len(q.items)
}

// PrintQueue prints the queue contents from front to back
func PrintQueue(q Queue) {
	i := 0
	for i < len(q.items) {
		fmt.Printf("%d ", q.items[i])
		i = i + 1
	}
	fmt.Println()
}
