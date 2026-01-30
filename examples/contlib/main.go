package main

import "fmt"
import "containers_tests/containers"

func main() {
	// === Linked List Demo ===
	fmt.Println("=== Linked List ===")

	list := containers.NewList()
	list = containers.Add(list, 10)
	list = containers.Add(list, 20)
	list = containers.Add(list, 30)
	list = containers.Add(list, 40)

	fmt.Println("List after adding 10, 20, 30, 40:")
	containers.PrintList(list)

	fmt.Printf("Size: %d\n", containers.Size(list))

	fmt.Printf("Find 30: position %d\n", containers.Find(list, 30))
	fmt.Printf("Find 99: position %d\n", containers.Find(list, 99))

	if containers.Contains(list, 20) {
		fmt.Println("Contains 20: true")
	} else {
		fmt.Println("Contains 20: false")
	}

	fmt.Printf("Get index 2: %d\n", containers.Get(list, 2))

	slice := containers.ToSlice(list)
	fmt.Printf("ToSlice: ")
	idx := 0
	for idx < len(slice) {
		fmt.Printf("%d ", slice[idx])
		idx = idx + 1
	}
	fmt.Println()

	values := []int{100, 200, 300}
	list2 := containers.FromSlice(values)
	fmt.Println("FromSlice [100, 200, 300]:")
	containers.PrintList(list2)

	list = containers.InsertAt(list, 2, 25)
	fmt.Println("After InsertAt index 2, value 25:")
	containers.PrintList(list)

	list = containers.RemoveAt(list, 2)
	fmt.Println("After RemoveAt index 2:")
	containers.PrintList(list)

	list = containers.Reverse(list)
	fmt.Println("After Reverse:")
	containers.PrintList(list)

	// Remove an element
	list = containers.Remove(list, 20)
	fmt.Println("After Remove 20:")
	containers.PrintList(list)

	// === Binary Search Tree Demo ===
	fmt.Println("=== Binary Search Tree ===")

	bst := containers.NewBST()
	bst = containers.BSTInsert(bst, 50)
	bst = containers.BSTInsert(bst, 30)
	bst = containers.BSTInsert(bst, 70)
	bst = containers.BSTInsert(bst, 20)
	bst = containers.BSTInsert(bst, 40)
	bst = containers.BSTInsert(bst, 60)
	bst = containers.BSTInsert(bst, 80)

	fmt.Println("BST after inserting 50, 30, 70, 20, 40, 60, 80:")
	containers.PrintBST(bst)

	if containers.BSTSearch(bst, 40) {
		fmt.Println("Search 40: found")
	} else {
		fmt.Println("Search 40: not found")
	}

	if containers.BSTSearch(bst, 99) {
		fmt.Println("Search 99: found")
	} else {
		fmt.Println("Search 99: not found")
	}

	fmt.Printf("Min: %d\n", containers.BSTMin(bst))
	fmt.Printf("Max: %d\n", containers.BSTMax(bst))

	inorder := containers.BSTInOrder(bst)
	fmt.Printf("InOrder: ")
	idx = 0
	for idx < len(inorder) {
		fmt.Printf("%d ", inorder[idx])
		idx = idx + 1
	}
	fmt.Println()

	preorder := containers.BSTPreOrder(bst)
	fmt.Printf("PreOrder: ")
	idx = 0
	for idx < len(preorder) {
		fmt.Printf("%d ", preorder[idx])
		idx = idx + 1
	}
	fmt.Println()

	postorder := containers.BSTPostOrder(bst)
	fmt.Printf("PostOrder: ")
	idx = 0
	for idx < len(postorder) {
		fmt.Printf("%d ", postorder[idx])
		idx = idx + 1
	}
	fmt.Println()

	fmt.Printf("Height: %d\n", containers.BSTHeight(bst))
	fmt.Printf("BSTSize: %d\n", containers.BSTSize(bst))

	// Remove leaf node
	bst = containers.BSTRemove(bst, 20)
	fmt.Println("After removing 20 (leaf):")
	containers.PrintBST(bst)

	// Remove node with one child
	bst = containers.BSTRemove(bst, 30)
	fmt.Println("After removing 30 (one child):")
	containers.PrintBST(bst)

	// Remove node with two children
	bst = containers.BSTRemove(bst, 50)
	fmt.Println("After removing 50 (two children):")
	containers.PrintBST(bst)

	// === Stack Demo ===
	fmt.Println("=== Stack ===")

	stk := containers.NewStack()
	stk = containers.Push(stk, 10)
	stk = containers.Push(stk, 20)
	stk = containers.Push(stk, 30)

	fmt.Println("Stack after pushing 10, 20, 30:")
	containers.PrintStack(stk)

	fmt.Printf("Peek: %d\n", containers.Peek(stk))
	fmt.Printf("StackSize: %d\n", containers.StackSize(stk))

	if containers.StackIsEmpty(stk) {
		fmt.Println("IsEmpty: true")
	} else {
		fmt.Println("IsEmpty: false")
	}

	sval := 0
	stk, sval = containers.Pop(stk)
	fmt.Printf("Popped: %d\n", sval)
	fmt.Println("Stack after pop:")
	containers.PrintStack(stk)

	stk, sval = containers.Pop(stk)
	fmt.Printf("Popped: %d\n", sval)
	stk, sval = containers.Pop(stk)
	fmt.Printf("Popped: %d\n", sval)

	if containers.StackIsEmpty(stk) {
		fmt.Println("IsEmpty after popping all: true")
	} else {
		fmt.Println("IsEmpty after popping all: false")
	}

	// === Queue Demo ===
	fmt.Println("=== Queue ===")

	que := containers.NewQueue()
	que = containers.Enqueue(que, 10)
	que = containers.Enqueue(que, 20)
	que = containers.Enqueue(que, 30)

	fmt.Println("Queue after enqueueing 10, 20, 30:")
	containers.PrintQueue(que)

	fmt.Printf("QueuePeek: %d\n", containers.QueuePeek(que))
	fmt.Printf("QueueSize: %d\n", containers.QueueSize(que))

	if containers.QueueIsEmpty(que) {
		fmt.Println("QueueIsEmpty: true")
	} else {
		fmt.Println("QueueIsEmpty: false")
	}

	qval := 0
	que, qval = containers.Dequeue(que)
	fmt.Printf("Dequeued: %d\n", qval)
	fmt.Println("Queue after dequeue:")
	containers.PrintQueue(que)

	que, qval = containers.Dequeue(que)
	fmt.Printf("Dequeued: %d\n", qval)
	que, qval = containers.Dequeue(que)
	fmt.Printf("Dequeued: %d\n", qval)

	if containers.QueueIsEmpty(que) {
		fmt.Println("QueueIsEmpty after dequeueing all: true")
	} else {
		fmt.Println("QueueIsEmpty after dequeueing all: false")
	}

	// === Complete Binary Tree Demo (existing) ===
	fmt.Println("=== Complete Binary Tree ===")

	tree := containers.NewBinaryTree()
	tree = containers.Insert(tree, 10)
	tree = containers.Insert(tree, 20)
	tree = containers.Insert(tree, 30)
	tree = containers.Insert(tree, 40)
	tree = containers.Insert(tree, 50)

	fmt.Println("Tree after inserting 10, 20, 30, 40, 50:")
	containers.PrintTree(tree)

	tree = containers.RemoveFromTree(tree, 30)
	fmt.Println("Tree after removing 30:")
	containers.PrintTree(tree)

	tree = containers.RemoveFromTree(tree, 10)
	fmt.Println("Tree after removing 10:")
	containers.PrintTree(tree)
}
