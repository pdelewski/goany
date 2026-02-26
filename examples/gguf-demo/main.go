package main

import (
	"fmt"
	"libs/anyllm"
	"os"
	"runtime/fs"
)

func main() {
	path := "model.gguf"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	if !fs.Exists(path) {
		fmt.Println("No model file found at: " + path)
		fmt.Println("Place a .gguf file as model.gguf to test")
	} else {
		file := anyllm.OpenGGUF(path)
		if file.Error != "" {
			fmt.Println("Error: " + file.Error)
		} else {
			summary := anyllm.PrintSummary(file)
			fmt.Print(summary)

			// Print first 10 dequantized values of the first tensor
			if len(file.Tensors) > 0 {
				fmt.Println("\n--- First Tensor Data ---")
				tensorName := file.Tensors[0].Name
				tensorType := anyllm.GGMLTypeName(file.Tensors[0].DType)
				fmt.Println("Tensor: " + tensorName + " [" + tensorType + "]")
				vals := anyllm.ReadTensor(file, 0)
				if len(vals) > 0 {
					count := len(vals)
					if count > 10 {
						count = 10
					}
					i := 0
					for i < count {
						fmt.Println(vals[i])
						i = i + 1
					}
				}
			}

			anyllm.CloseGGUF(file)
		}
	}
}
