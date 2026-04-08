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

// BuildChainGraph creates a linear chain of n nodes: 0 → 1 → 2 → ... → n-1
func BuildChainGraph(n int) []*GNode {
	nodes := make([]*GNode, 0)
	for i := 0; i < n; i++ {
		nd := NewGNode(i)
		nodes = append(nodes, nd)
	}
	for i := 0; i < n-1; i++ {
		AddEdge(nodes[i], nodes[i+1])
	}
	return nodes
}

// BuildGridGraph creates an n×n grid graph with edges right and down.
// Node value = row*n + col. Total nodes = n*n, edges = 2*n*(n-1).
func BuildGridGraph(n int) []*GNode {
	nodes := make([]*GNode, 0)
	total := n * n
	for i := 0; i < total; i++ {
		nd := NewGNode(i)
		nodes = append(nodes, nd)
	}
	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			idx := row*n + col
			if col+1 < n {
				AddEdge(nodes[idx], nodes[idx+1])
			}
			if row+1 < n {
				AddEdge(nodes[idx], nodes[idx+n])
			}
		}
	}
	return nodes
}

// CountReachable returns how many nodes are reachable from start via BFS.
func CountReachable(start *GNode) int {
	result := BFS(start)
	return len(result)
}

// BuildSparseGraph creates n nodes with deterministic pseudo-random edges.
// Each node i gets edges to (i*7+3)%n, (i*13+7)%n, (i*31+11)%n — giving
// a connected-ish sparse directed graph with ~3 edges per node.
func BuildSparseGraph(n int) []*GNode {
	nodes := make([]*GNode, 0)
	for i := 0; i < n; i++ {
		nd := NewGNode(i)
		nodes = append(nodes, nd)
	}
	for i := 0; i < n; i++ {
		t1 := (i*7 + 3) % n
		t2 := (i*13 + 7) % n
		t3 := (i*31 + 11) % n
		if t1 != i {
			AddEdge(nodes[i], nodes[t1])
		}
		if t2 != i && t2 != t1 {
			AddEdge(nodes[i], nodes[t2])
		}
		if t3 != i && t3 != t1 && t3 != t2 {
			AddEdge(nodes[i], nodes[t3])
		}
	}
	return nodes
}

// AllPairsPathCount checks HasPath between all pairs in a subset of nodes.
// Returns number of reachable pairs.
func AllPairsPathCount(nodes []*GNode, subset []int) int {
	count := 0
	for i := 0; i < len(subset); i++ {
		for j := 0; j < len(subset); j++ {
			if i != j {
				if HasPath(nodes[subset[i]], nodes[subset[j]]) {
					count = count + 1
				}
			}
		}
	}
	return count
}

// SumBFSFromStarts runs BFS from numStarts evenly-spaced starting nodes
// and sums up traversal lengths. Each BFS is O(n^2) due to containsVal.
func SumBFSFromStarts(nodes []*GNode, numStarts int) int {
	total := 0
	n := len(nodes)
	step := n / numStarts
	if step < 1 {
		step = 1
	}
	for i := 0; i < n; i = i + step {
		bfs := BFS(nodes[i])
		total = total + len(bfs)
	}
	return total
}

func main() {
	// --- Original small graph test ---
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

	// --- Benchmark 1: BFS from every node on sparse graph (heavy) ---
	// 1000 nodes, ~3000 edges. BFS from all 1000 nodes per round.
	// Each BFS is O(n^2) due to containsVal. 1000 BFS * 5 rounds = 5000 BFS calls.
	sparseSize := 1000
	sparse := BuildSparseGraph(sparseSize)
	totalReachable := 0
	rounds := 10
	for r := 0; r < rounds; r++ {
		sum := SumBFSFromStarts(sparse, sparseSize)
		totalReachable = totalReachable + sum
	}
	fmt.Print("Sparse(")
	fmt.Printf("%d", sparseSize)
	fmt.Print(") SumBFS x ")
	fmt.Printf("%d", rounds)
	fmt.Print(": total=")
	fmt.Printf("%d", totalReachable)
	fmt.Println("")

	// --- Benchmark 2: All-pairs path on 80-node subset (heavy) ---
	// 80*79 = 6320 HasPath calls, each traversing up to 1000 nodes.
	subsetSize := 100
	subset := make([]int, 0)
	for i := 0; i < subsetSize; i++ {
		idx := (i * 10) % sparseSize
		subset = append(subset, idx)
	}
	pairCount := AllPairsPathCount(sparse, subset)
	fmt.Print("AllPairs(")
	fmt.Printf("%d", subsetSize)
	fmt.Print("): paths=")
	fmt.Printf("%d", pairCount)
	fmt.Println("")

	// --- Benchmark 3: Rebuild grid graph many times (pointer allocation heavy) ---
	// Each rebuild creates 2500 new GNode pointers + 4800 edges + BFS traversal.
	gridSize := 50
	rebuildCount := 200
	gridTotal := 0
	for r := 0; r < rebuildCount; r++ {
		g := BuildGridGraph(gridSize)
		reachable := CountReachable(g[0])
		gridTotal = gridTotal + reachable
	}
	fmt.Print("GridRebuild(")
	fmt.Printf("%d", gridSize)
	fmt.Print("x")
	fmt.Printf("%d", gridSize)
	fmt.Print(") x ")
	fmt.Printf("%d", rebuildCount)
	fmt.Print(": total=")
	fmt.Printf("%d", gridTotal)
	fmt.Println("")

	// --- Benchmark 4: Repeated DFS on chain (deep stack operations) ---
	chainSize := 2000
	chain := BuildChainGraph(chainSize)
	dfsTotal := 0
	dfsRounds := 1000
	for r := 0; r < dfsRounds; r++ {
		dfs := DFS(chain[0])
		dfsTotal = dfsTotal + len(dfs)
	}
	fmt.Print("ChainDFS(")
	fmt.Printf("%d", chainSize)
	fmt.Print(") x ")
	fmt.Printf("%d", dfsRounds)
	fmt.Print(": total=")
	fmt.Printf("%d", dfsTotal)
	fmt.Println("")

	// --- Verify correctness ---
	sparseReachable := CountReachable(sparse[0])
	verifyGrid := BuildGridGraph(gridSize)
	gridReachable := CountReachable(verifyGrid[0])
	chainReachable := CountReachable(chain[0])
	expectedGrid := gridSize * gridSize

	if sparseReachable == sparseSize {
		fmt.Println("PASS: sparse reachability")
	} else {
		fmt.Print("FAIL: sparse reachable=")
		fmt.Printf("%d", sparseReachable)
		fmt.Println("")
	}
	if gridReachable == expectedGrid {
		fmt.Println("PASS: grid reachability")
	} else {
		fmt.Print("FAIL: grid reachable=")
		fmt.Printf("%d", gridReachable)
		fmt.Println("")
	}
	if chainReachable == chainSize {
		fmt.Println("PASS: chain reachability")
	} else {
		fmt.Print("FAIL: chain reachable=")
		fmt.Printf("%d", chainReachable)
		fmt.Println("")
	}
}
