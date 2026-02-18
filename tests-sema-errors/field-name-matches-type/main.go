package main

// ERROR: Field name matches type name (causes C++ -Wchanges-meaning error)

type Value struct {
	Data int
}

type Container struct {
	Value Value // error: field name 'Value' matches type name 'Value'
}

func main() {
	c := Container{Value: Value{Data: 42}}
	_ = c
}
