package main

import "fmt"

type GNode struct {
	Value int
	Adj   []*GNode
}

func NewGNode(value int) *GNode {
	return &GNode{Value: value}
}

func AddEdge(from *GNode, to *GNode) {
	from.Adj = append(from.Adj, to)
}

func containsVal(arr []int, val int) bool {
	for i := 0; i < len(arr); i++ {
		if arr[i] == val {
			return true
		}
	}
	return false
}

func BFS(start *GNode) []int {
	result := make([]int, 0)
	visited := make([]int, 0)
	queue := make([]*GNode, 0)
	queue = append(queue, start)
	visited = append(visited, start.Value)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		result = append(result, current.Value)
		for i := 0; i < len(current.Adj); i++ {
			neighbor := current.Adj[i]
			if !containsVal(visited, neighbor.Value) {
				visited = append(visited, neighbor.Value)
				queue = append(queue, neighbor)
			}
		}
	}
	return result
}

func DFS(start *GNode) []int {
	result := make([]int, 0)
	visited := make([]int, 0)
	stack := make([]*GNode, 0)
	stack = append(stack, start)
	visited = append(visited, start.Value)
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		result = append(result, current.Value)
		for i := 0; i < len(current.Adj); i++ {
			neighbor := current.Adj[i]
			if !containsVal(visited, neighbor.Value) {
				visited = append(visited, neighbor.Value)
				stack = append(stack, neighbor)
			}
		}
	}
	return result
}

func HasPath(from *GNode, to *GNode) bool {
	visited := make([]int, 0)
	stack := make([]*GNode, 0)
	stack = append(stack, from)
	visited = append(visited, from.Value)
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if current.Value == to.Value {
			return true
		}
		for i := 0; i < len(current.Adj); i++ {
			neighbor := current.Adj[i]
			if !containsVal(visited, neighbor.Value) {
				visited = append(visited, neighbor.Value)
				stack = append(stack, neighbor)
			}
		}
	}
	return false
}

func printArr(arr []int) {
	fmt.Print("[")
	for i := 0; i < len(arr); i++ {
		if i > 0 {
			fmt.Print(" ")
		}
		fmt.Printf("%d", arr[i])
	}
	fmt.Println("]")
}

func main() {
	n1 := NewGNode(1)
	n2 := NewGNode(2)
	n3 := NewGNode(3)
	n4 := NewGNode(4)
	n5 := NewGNode(5)
	n6 := NewGNode(6)

	AddEdge(n1, n2)
	AddEdge(n1, n3)
	AddEdge(n2, n4)
	AddEdge(n3, n4)
	AddEdge(n3, n5)
	AddEdge(n5, n6)

	bfsResult := BFS(n1)
	fmt.Print("BFS from 1: ")
	printArr(bfsResult)

	dfsResult := DFS(n1)
	fmt.Print("DFS from 1: ")
	printArr(dfsResult)

	if HasPath(n1, n6) {
		fmt.Println("HasPath(1, 6): true")
	} else {
		fmt.Println("HasPath(1, 6): false")
	}

	if HasPath(n6, n1) {
		fmt.Println("HasPath(6, 1): true")
	} else {
		fmt.Println("HasPath(6, 1): false")
	}
}
