package containers

import "fmt"

// BinaryTreeNode represents a node in the binary tree using an array-based approach
type BinaryTreeNode struct {
	value int // The value of the node
	left  int // The index of the left child (-1 if no left child)
	right int // The index of the right child (-1 if no right child)
}

// BinaryTree represents the array-based binary tree
type BinaryTree struct {
	nodes []BinaryTreeNode // The array storing the tree nodes
	root  int              // The index of the root node (-1 if the tree is empty)
}

// NewBinaryTree creates a new empty binary tree and returns it as a value
func NewBinaryTree() BinaryTree {
	return BinaryTree{
		nodes: []BinaryTreeNode{}, // Initialize with an empty slice of nodes
		root:  -1,                 // -1 indicates the tree is empty
	}
}

// Insert inserts a value into the binary tree, maintaining a complete binary tree, and returns the modified tree
func Insert(t BinaryTree, value int) BinaryTree {
	newNode := BinaryTreeNode{
		value: value,
		left:  -1, // No left child
		right: -1, // No right child
	}

	// Add the new node to the array
	t.nodes = append(t.nodes, newNode)

	// If the tree is empty, set the root to the new node's index
	if t.root == -1 {
		t.root = 0
	} else {
		// Otherwise, insert the node as a child of the existing tree
		insertIndex := len(t.nodes) - 1
		parentIndex := (insertIndex - 1) / 2 // Find parent index

		if insertIndex%2 == 1 {
			// Left child
			// TODO original code  code was about mutating this in-place
			// this is problematic now from c# perspective
			// as it causes Cannot modify the return value of 'List<Api.ListNode>.this[int]' because it is not a variable
			// to fix that we have to discover this case and transform code as below
			tmp := t.nodes[parentIndex]
			tmp.left = insertIndex
			t.nodes[parentIndex] = tmp
		} else {
			// Right child
			// TODO original code  code was about mutating this in-place
			// this is problematic now from c# perspective
			// as it causes Cannot modify the return value of 'List<Api.ListNode>.this[int]' because it is not a variable
			// to fix that we have to discover this case and transform code as below
			tmp := t.nodes[parentIndex]
			tmp.right = insertIndex // Update the parent's right child index
			t.nodes[parentIndex] = tmp
		}
	}

	return t // Return the updated tree
}

// Print prints the binary tree in a level-order traversal
func PrintTree(t BinaryTree) {
	if t.root == -1 {
		fmt.Println("The tree is empty.")
		return
	}

	// Start from the root and print nodes in level-order (breadth-first)
	queue := []int{t.root}
	for len(queue) > 0 {
		index := queue[0]
		queue = queue[1:]
		node := t.nodes[index]
		fmt.Printf("%d:", node.value)

		if node.left != -1 {
			queue = append(queue, node.left)
		}
		if node.right != -1 {
			queue = append(queue, node.right)
		}
	}
	fmt.Println()
}

// Remove removes a node by value in the binary tree and returns the modified tree
// This implementation removes the last node and replaces the target node's value
func RemoveFromTree(t BinaryTree, value int) BinaryTree {
	if t.root == -1 {
		fmt.Println("The tree is empty.")
		return t
	}

	// Find the index of the node to remove
	indexToRemove := -1
	i := 0
	for {
		if i >= len(t.nodes) {
			break
		}
		if t.nodes[i].value == value {
			indexToRemove = i
			break
		}
		i = i + 1
	}

	if indexToRemove == -1 {
		fmt.Println("Value not found in the tree.")
		return t
	}

	// Get the last node index
	lastNodeIndex := len(t.nodes) - 1

	// If the node to remove is not the last node, replace its value with the last node's value
	if indexToRemove != lastNodeIndex {
		tmp := t.nodes[indexToRemove]
		tmp.value = t.nodes[lastNodeIndex].value
		t.nodes[indexToRemove] = tmp
	}

	// Remove the last node from the array by creating a new slice without the last element
	newNodes := []BinaryTreeNode{}
	j := 0
	for {
		if j >= lastNodeIndex {
			break
		}
		newNodes = append(newNodes, t.nodes[j])
		j = j + 1
	}
	t.nodes = newNodes

	// If the tree is now empty, reset root
	if len(t.nodes) == 0 {
		t.root = -1
	}

	// Update parent's child pointer if the last node was moved
	if lastNodeIndex > 0 && lastNodeIndex != indexToRemove {
		parentIndex := (lastNodeIndex - 1) / 2
		if lastNodeIndex%2 == 1 {
			// Was left child - clear parent's left pointer
			tmp := t.nodes[parentIndex]
			tmp.left = -1
			t.nodes[parentIndex] = tmp
		} else {
			// Was right child - clear parent's right pointer
			tmp := t.nodes[parentIndex]
			tmp.right = -1
			t.nodes[parentIndex] = tmp
		}
	} else if lastNodeIndex > 0 && lastNodeIndex == indexToRemove {
		// The removed node was the last node, update its parent
		parentIndex := (lastNodeIndex - 1) / 2
		if lastNodeIndex%2 == 1 {
			tmp := t.nodes[parentIndex]
			tmp.left = -1
			t.nodes[parentIndex] = tmp
		} else {
			tmp := t.nodes[parentIndex]
			tmp.right = -1
			t.nodes[parentIndex] = tmp
		}
	}

	return t
}

// TreeInOrder returns values in in-order traversal (left, root, right) using iterative approach
func TreeInOrder(t BinaryTree) []int {
	result := []int{}
	if t.root == -1 {
		return result
	}
	stk := []int{}
	currIndex := t.root
	for currIndex != -1 || len(stk) > 0 {
		for currIndex != -1 {
			stk = append(stk, currIndex)
			currIndex = t.nodes[currIndex].left
		}
		currIndex = stk[len(stk)-1]
		stk = stk[:len(stk)-1]
		result = append(result, t.nodes[currIndex].value)
		currIndex = t.nodes[currIndex].right
	}
	return result
}

// TreePreOrder returns values in pre-order traversal (root, left, right) using iterative approach
func TreePreOrder(t BinaryTree) []int {
	result := []int{}
	if t.root == -1 {
		return result
	}
	stk := []int{t.root}
	for len(stk) > 0 {
		currIndex := stk[len(stk)-1]
		stk = stk[:len(stk)-1]
		result = append(result, t.nodes[currIndex].value)
		if t.nodes[currIndex].right != -1 {
			stk = append(stk, t.nodes[currIndex].right)
		}
		if t.nodes[currIndex].left != -1 {
			stk = append(stk, t.nodes[currIndex].left)
		}
	}
	return result
}

// TreePostOrder returns values in post-order traversal (left, right, root) using two-stack approach
func TreePostOrder(t BinaryTree) []int {
	result := []int{}
	if t.root == -1 {
		return result
	}
	stk1 := []int{t.root}
	stk2 := []int{}
	for len(stk1) > 0 {
		currIndex := stk1[len(stk1)-1]
		stk1 = stk1[:len(stk1)-1]
		stk2 = append(stk2, currIndex)
		if t.nodes[currIndex].left != -1 {
			stk1 = append(stk1, t.nodes[currIndex].left)
		}
		if t.nodes[currIndex].right != -1 {
			stk1 = append(stk1, t.nodes[currIndex].right)
		}
	}
	i := len(stk2)
	for i > 0 {
		i = i - 1
		result = append(result, t.nodes[stk2[i]].value)
	}
	return result
}

// TreeLevelOrder returns values in level-order (BFS) traversal
func TreeLevelOrder(t BinaryTree) []int {
	result := []int{}
	if t.root == -1 {
		return result
	}
	queue := []int{t.root}
	for len(queue) > 0 {
		currIndex := queue[0]
		queue = queue[1:]
		result = append(result, t.nodes[currIndex].value)
		if t.nodes[currIndex].left != -1 {
			queue = append(queue, t.nodes[currIndex].left)
		}
		if t.nodes[currIndex].right != -1 {
			queue = append(queue, t.nodes[currIndex].right)
		}
	}
	return result
}

// TreeSize returns the number of reachable nodes in the tree
func TreeSize(t BinaryTree) int {
	if t.root == -1 {
		return 0
	}
	queue := []int{t.root}
	count := 0
	for len(queue) > 0 {
		currIndex := queue[0]
		queue = queue[1:]
		count = count + 1
		if t.nodes[currIndex].left != -1 {
			queue = append(queue, t.nodes[currIndex].left)
		}
		if t.nodes[currIndex].right != -1 {
			queue = append(queue, t.nodes[currIndex].right)
		}
	}
	return count
}

// TreeHeight returns the height of the tree using BFS level counting
func TreeHeight(t BinaryTree) int {
	if t.root == -1 {
		return 0
	}
	queue := []int{t.root}
	height := 0
	for len(queue) > 0 {
		levelSize := len(queue)
		i := 0
		for i < levelSize {
			currIndex := queue[0]
			queue = queue[1:]
			if t.nodes[currIndex].left != -1 {
				queue = append(queue, t.nodes[currIndex].left)
			}
			if t.nodes[currIndex].right != -1 {
				queue = append(queue, t.nodes[currIndex].right)
			}
			i = i + 1
		}
		height = height + 1
	}
	return height
}

// TreeContains returns true if the value exists in the tree (linear BFS search)
func TreeContains(t BinaryTree, value int) bool {
	if t.root == -1 {
		return false
	}
	queue := []int{t.root}
	for len(queue) > 0 {
		currIndex := queue[0]
		queue = queue[1:]
		if t.nodes[currIndex].value == value {
			return true
		}
		if t.nodes[currIndex].left != -1 {
			queue = append(queue, t.nodes[currIndex].left)
		}
		if t.nodes[currIndex].right != -1 {
			queue = append(queue, t.nodes[currIndex].right)
		}
	}
	return false
}

// TreeMin returns the minimum value in the tree (BFS scan)
func TreeMin(t BinaryTree) int {
	if t.root == -1 {
		return -1
	}
	minVal := t.nodes[t.root].value
	queue := []int{t.root}
	for len(queue) > 0 {
		currIndex := queue[0]
		queue = queue[1:]
		if t.nodes[currIndex].value < minVal {
			minVal = t.nodes[currIndex].value
		}
		if t.nodes[currIndex].left != -1 {
			queue = append(queue, t.nodes[currIndex].left)
		}
		if t.nodes[currIndex].right != -1 {
			queue = append(queue, t.nodes[currIndex].right)
		}
	}
	return minVal
}

// TreeMax returns the maximum value in the tree (BFS scan)
func TreeMax(t BinaryTree) int {
	if t.root == -1 {
		return -1
	}
	maxVal := t.nodes[t.root].value
	queue := []int{t.root}
	for len(queue) > 0 {
		currIndex := queue[0]
		queue = queue[1:]
		if t.nodes[currIndex].value > maxVal {
			maxVal = t.nodes[currIndex].value
		}
		if t.nodes[currIndex].left != -1 {
			queue = append(queue, t.nodes[currIndex].left)
		}
		if t.nodes[currIndex].right != -1 {
			queue = append(queue, t.nodes[currIndex].right)
		}
	}
	return maxVal
}

// TreeToSlice returns the tree values as a flat slice in level-order
func TreeToSlice(t BinaryTree) []int {
	return TreeLevelOrder(t)
}
