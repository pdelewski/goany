package containers

import "fmt"

// BSTNode represents a node in the binary search tree
type BSTNode struct {
	value int
	left  int
	right int
}

// BST represents an array-based binary search tree
type BST struct {
	nodes []BSTNode
	root  int
}

// NewBST creates a new empty binary search tree
func NewBST() BST {
	return BST{
		nodes: []BSTNode{},
		root:  -1,
	}
}

// BSTInsert inserts a value into the BST maintaining the ordering property
func BSTInsert(t BST, value int) BST {
	newNode := BSTNode{
		value: value,
		left:  -1,
		right: -1,
	}
	t.nodes = append(t.nodes, newNode)
	newIndex := len(t.nodes) - 1

	if t.root == -1 {
		t.root = newIndex
		return t
	}

	currIndex := t.root
	for currIndex != -1 {
		if value < t.nodes[currIndex].value {
			if t.nodes[currIndex].left == -1 {
				tmp := t.nodes[currIndex]
				tmp.left = newIndex
				t.nodes[currIndex] = tmp
				return t
			}
			currIndex = t.nodes[currIndex].left
		} else {
			if t.nodes[currIndex].right == -1 {
				tmp := t.nodes[currIndex]
				tmp.right = newIndex
				t.nodes[currIndex] = tmp
				return t
			}
			currIndex = t.nodes[currIndex].right
		}
	}

	return t
}

// BSTSearch returns true if the value exists in the BST
func BSTSearch(t BST, value int) bool {
	currIndex := t.root
	for currIndex != -1 {
		if value == t.nodes[currIndex].value {
			return true
		}
		if value < t.nodes[currIndex].value {
			currIndex = t.nodes[currIndex].left
		} else {
			currIndex = t.nodes[currIndex].right
		}
	}
	return false
}

// BSTMin returns the minimum value in the BST
func BSTMin(t BST) int {
	if t.root == -1 {
		return -1
	}
	currIndex := t.root
	for t.nodes[currIndex].left != -1 {
		currIndex = t.nodes[currIndex].left
	}
	return t.nodes[currIndex].value
}

// BSTMax returns the maximum value in the BST
func BSTMax(t BST) int {
	if t.root == -1 {
		return -1
	}
	currIndex := t.root
	for t.nodes[currIndex].right != -1 {
		currIndex = t.nodes[currIndex].right
	}
	return t.nodes[currIndex].value
}

// BSTInOrder returns values in in-order traversal (iterative)
func BSTInOrder(t BST) []int {
	result := []int{}
	if t.root == -1 {
		return result
	}
	stk := []int{}
	currIndex := t.root
	for {
		if currIndex == -1 && len(stk) == 0 {
			break
		}
		for {
			if currIndex == -1 {
				break
			}
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

// BSTPreOrder returns values in pre-order traversal (iterative)
func BSTPreOrder(t BST) []int {
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

// BSTPostOrder returns values in post-order traversal (iterative, two-stack)
func BSTPostOrder(t BST) []int {
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

// BSTHeight returns the height of the BST using BFS level counting
func BSTHeight(t BST) int {
	if t.root == -1 {
		return 0
	}
	queue := []int{t.root}
	height := 0
	for {
		if len(queue) == 0 {
			break
		}
		levelSize := len(queue)
		i := 0
		for {
			if i >= levelSize {
				break
			}
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

// BSTSize returns the number of reachable nodes in the BST
func BSTSize(t BST) int {
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

// BSTRemove removes a value from the BST
func BSTRemove(t BST, value int) BST {
	if t.root == -1 {
		return t
	}

	// Find the node and its parent
	parentIndex := -1
	currIndex := t.root
	isLeftChild := false
	for currIndex != -1 {
		if value == t.nodes[currIndex].value {
			break
		}
		parentIndex = currIndex
		if value < t.nodes[currIndex].value {
			currIndex = t.nodes[currIndex].left
			isLeftChild = true
		} else {
			currIndex = t.nodes[currIndex].right
			isLeftChild = false
		}
	}

	if currIndex == -1 {
		fmt.Println("Value not found in BST.")
		return t
	}

	leftChild := t.nodes[currIndex].left
	rightChild := t.nodes[currIndex].right

	// Case 1: Leaf node (no children)
	if leftChild == -1 && rightChild == -1 {
		if parentIndex == -1 {
			t.root = -1
		} else if isLeftChild {
			tmpA := t.nodes[parentIndex]
			tmpA.left = -1
			t.nodes[parentIndex] = tmpA
		} else {
			tmpB := t.nodes[parentIndex]
			tmpB.right = -1
			t.nodes[parentIndex] = tmpB
		}
		return t
	}

	// Case 2: One child
	if leftChild == -1 || rightChild == -1 {
		childIndex := leftChild
		if leftChild == -1 {
			childIndex = rightChild
		}
		if parentIndex == -1 {
			t.root = childIndex
		} else if isLeftChild {
			tmpC := t.nodes[parentIndex]
			tmpC.left = childIndex
			t.nodes[parentIndex] = tmpC
		} else {
			tmpD := t.nodes[parentIndex]
			tmpD.right = childIndex
			t.nodes[parentIndex] = tmpD
		}
		return t
	}

	// Case 3: Two children - find in-order successor
	successorParent := currIndex
	successorIndex := rightChild
	for t.nodes[successorIndex].left != -1 {
		successorParent = successorIndex
		successorIndex = t.nodes[successorIndex].left
	}

	// Copy successor value to current node
	tmpE := t.nodes[currIndex]
	tmpE.value = t.nodes[successorIndex].value
	t.nodes[currIndex] = tmpE

	// Remove successor (has at most one right child)
	successorRight := t.nodes[successorIndex].right
	if successorParent == currIndex {
		tmpF := t.nodes[successorParent]
		tmpF.right = successorRight
		t.nodes[successorParent] = tmpF
	} else {
		tmpG := t.nodes[successorParent]
		tmpG.left = successorRight
		t.nodes[successorParent] = tmpG
	}

	return t
}

// PrintBST prints the BST values in order
func PrintBST(t BST) {
	values := BSTInOrder(t)
	i := 0
	for i < len(values) {
		fmt.Printf("%d ", values[i])
		i = i + 1
	}
	fmt.Println()
}
