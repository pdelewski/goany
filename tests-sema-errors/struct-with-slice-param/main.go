package main

// ERROR: Struct containing slice not returned
type State struct {
	items []int
	name  string
}

func process(s State) {
	s.items[0] = 99
}

func main() {
	s := State{items: []int{1, 2, 3}, name: "test"}
	process(s)
}
