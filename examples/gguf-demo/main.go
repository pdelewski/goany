package main

import (
	"fmt"
	"libs/anyllm"
	"runtime/fs"
)

func main() {
	path := "model.gguf"
	if !fs.Exists(path) {
		fmt.Println("No model file found at: " + path)
		fmt.Println("Place a .gguf file as model.gguf to test")
	} else {
		file := anyllm.LoadGGUF(path)
		summary := anyllm.PrintSummary(file)
		fmt.Print(summary)
	}
}
