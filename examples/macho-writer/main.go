package main

import (
	"fmt"
	"libs/macho"
)

func main() {
	// arm64 machine code: mov w0, #42; ret
	code := []byte{
		0x40, 0x05, 0x80, 0x52, // mov w0, #42
		0xC0, 0x03, 0x5F, 0xD6, // ret
	}
	err := macho.WriteObjectFile("/tmp/tiny_arm64.o", code, "_main")
	if err != "" {
		fmt.Println("Error: " + err)
	} else {
		fmt.Println("Written: /tmp/tiny_arm64.o")
		fmt.Println("Link:    cc -o /tmp/tiny_arm64 /tmp/tiny_arm64.o")
		fmt.Println("Run:     /tmp/tiny_arm64; echo $?")
	}
}
