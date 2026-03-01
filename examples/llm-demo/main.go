package main

import (
	"fmt"
	"libs/anyllm"
	"os"
	"runtime/fs"
)

func main() {
	modelPath := "model.gguf"
	prompt := "hello"
	maxTokens := 20
	temperature := 0.7
	topP := 0.9

	if len(os.Args) > 1 {
		modelPath = os.Args[1]
	}
	if len(os.Args) > 2 {
		prompt = os.Args[2]
	}
	if len(os.Args) > 3 {
		temperature = anyllm.ParseFloat(os.Args[3])
	}
	if len(os.Args) > 4 {
		topP = anyllm.ParseFloat(os.Args[4])
	}

	if !fs.Exists(modelPath) {
		fmt.Println("No model file found at: " + modelPath)
		fmt.Println("Usage: llm-demo <model.gguf> [prompt] [temperature] [top_p]")
	} else {
		file := anyllm.OpenGGUF(modelPath)
		if file.Error != "" {
			fmt.Println("Error loading model: " + file.Error)
		} else {
			cfg := anyllm.ExtractModelConfig(file)
			if cfg.Error != "" {
				fmt.Println("Error extracting config: " + cfg.Error)
			} else {
				fmt.Println("Model: " + cfg.Architecture)
				embStr := anyllm.IntToStringPub(cfg.EmbeddingLength)
				fmt.Println("Embedding: " + embStr)
				blockStr := anyllm.IntToStringPub(cfg.BlockCount)
				fmt.Println("Layers: " + blockStr)
				vocabStr := anyllm.IntToStringPub(cfg.VocabSize)
				fmt.Println("Vocab: " + vocabStr)
				fmt.Println("Prompt: " + prompt)

				chatFormat := anyllm.DetectChatFormatFromFile(file, cfg)
				gparams := anyllm.DefaultGenerateParams(chatFormat)
				gparams.Temperature = temperature
				gparams.TopP = topP

				fmt.Println("Generating...")

				tok := anyllm.LoadTokenizer(file)
				result := anyllm.GenerateWithParams(file, cfg, tok, prompt, maxTokens, gparams)
				fmt.Println(result)
			}
			anyllm.CloseGGUF(file)
		}
	}
}
