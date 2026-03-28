package main

import (
	"fmt"
	"libs/macho"
	"os"
)

func main() {
	// arm64 machine code: mov w0, #42; ret
	code := []byte{
		0x40, 0x05, 0x80, 0x52, // mov w0, #42
		0xC0, 0x03, 0x5F, 0xD6, // ret
	}
	objPath := "/tmp/tiny_arm64.o"
	if len(os.Args) > 1 {
		objPath = os.Args[1]
	}
	err := macho.WriteObjectFile(objPath, code, "_main")
	if err != "" {
		fmt.Println("Error: " + err)
	} else {
		fmt.Println("Written: " + objPath)
		fmt.Println("Link:    cc -o /tmp/tiny_arm64 " + objPath)
		fmt.Println("Run:     /tmp/tiny_arm64; echo $?")
	}
}
