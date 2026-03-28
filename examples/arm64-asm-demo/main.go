package main

import (
	"fmt"
	"libs/arm64asm"
	"libs/macho"
)

func main() {
	// Recursive fibonacci: computes fib(10) = 55, returned as exit code
	asmText := `.global _main
_main:
    stp x29, x30, [sp, #-16]!
    mov x29, sp
    mov w0, #10
    bl fib
    ldp x29, x30, [sp], #16
    ret

fib:
    cmp w0, #1
    b.le fib_base
    stp x29, x30, [sp, #-32]!
    mov x29, sp
    str w0, [sp, #28]
    sub w0, w0, #1
    bl fib
    str w0, [sp, #24]
    ldr w0, [sp, #28]
    sub w0, w0, #2
    bl fib
    ldr w1, [sp, #24]
    add w0, w0, w1
    ldp x29, x30, [sp], #32
    ret
fib_base:
    ret
`

	code, err := arm64asm.Assemble(asmText)
	if err == "" {
		globals := arm64asm.GetGlobals(asmText)
		symbolName := "_main"
		if len(globals) > 0 {
			symbolName = globals[0]
		}

		objPath := "/tmp/fib_arm64.o"
		writeErr := macho.WriteObjectFile(objPath, code, symbolName)
		if writeErr == "" {
			fmt.Println("ARM64 assembler demo: recursive fibonacci")
			fmt.Println("Assembled " + fmt.Sprintf("%d", len(code)) + " bytes of machine code")
			fmt.Println("Written: " + objPath)
			fmt.Println("Link:    cc -o /tmp/fib_arm64 " + objPath)
			fmt.Println("Run:     /tmp/fib_arm64; echo $?")
			fmt.Println("Expected exit code: 55 (fib(10))")
		} else {
			fmt.Println("Write error: " + writeErr)
		}
	} else {
		fmt.Println("Assembly error: " + err)
	}
}
