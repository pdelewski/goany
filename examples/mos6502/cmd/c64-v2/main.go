package main

import (
	"mos6502lib/assembler"
	"mos6502lib/basic"
	"mos6502lib/cpu"
	"mos6502lib/font"
	"runtime/graphics"
)

// C64-style text screen constants
// Screen is 40 characters wide x 25 characters tall (like real C64)
const TextCols = 40
const TextRows = 25

// Text screen memory starts at $0400 (like real C64)
const TextScreenBase = 0x0400

// C64State holds all mutable state for the C64 emulator.
// By threading this through RunLoopWithState, we avoid closure capture
// and eliminate cloning in Rust's FnMut closures.
type C64State struct {
	C             cpu.CPU
	CursorRow     int
	CursorCol     int
	BasicState    basic.BasicState
	InputStartRow int
	FontData      []uint8
	TextColor     graphics.Color
	BgColor       graphics.Color
	Scale         int32
}

// hexDigit converts a value 0-15 to a hex character string
func hexDigit(n int) string {
	if n == 0 {
		return "0"
	} else if n == 1 {
		return "1"
	} else if n == 2 {
		return "2"
	} else if n == 3 {
		return "3"
	} else if n == 4 {
		return "4"
	} else if n == 5 {
		return "5"
	} else if n == 6 {
		return "6"
	} else if n == 7 {
		return "7"
	} else if n == 8 {
		return "8"
	} else if n == 9 {
		return "9"
	} else if n == 10 {
		return "A"
	} else if n == 11 {
		return "B"
	} else if n == 12 {
		return "C"
	} else if n == 13 {
		return "D"
	} else if n == 14 {
		return "E"
	} else if n == 15 {
		return "F"
	}
	return "0"
}

// toHex2 converts a byte to 2-digit hex string
func toHex2(n int) string {
	high := (n >> 4) & 0x0F
	low := n & 0x0F
	return hexDigit(high) + hexDigit(low)
}

// toHex4 converts a 16-bit value to 4-digit hex string
func toHex4(n int) string {
	return toHex2((n>>8)&0xFF) + toHex2(n&0xFF)
}

// toHex converts an integer to a hex string with $ prefix
// Uses 2 digits for values <= 255, 4 digits otherwise
func toHex(n int) string {
	if n > 255 {
		return "$" + toHex4(n)
	}
	return "$" + toHex2(n)
}

// addStringToScreen generates assembly to write a string to screen memory
func addStringToScreen(lines []string, text string, row int, col int) []string {
	baseAddr := TextScreenBase + (row * TextCols) + col
	i := 0
	for {
		if i >= len(text) {
			break
		}
		charCode := int(text[i])
		addr := baseAddr + i
		lines = append(lines, "LDA #"+toHex(charCode))
		lines = append(lines, "STA "+toHex(addr))
		i = i + 1
	}
	return lines
}

// clearScreen generates assembly to fill screen memory with spaces
func clearScreen(lines []string) []string {
	// Fill all 1000 screen locations (40x25) with space character (0x20)
	// Load space character once, then just STA to each location
	lines = append(lines, "LDA #$20") // space character - load once
	addr := TextScreenBase
	i := 0
	for {
		if i >= TextCols*TextRows {
			break
		}
		lines = append(lines, "STA "+toHex(addr+i))
		i = i + 1
	}
	return lines
}

// charFromCodeMain converts a character code to a single-character string (goany compatible)
func charFromCodeMain(ch int) string {
	if ch == 32 {
		return " "
	} else if ch == 33 {
		return "!"
	} else if ch == 34 {
		return "\""
	} else if ch == 35 {
		return "#"
	} else if ch == 36 {
		return "$"
	} else if ch == 37 {
		return "%"
	} else if ch == 38 {
		return "&"
	} else if ch == 39 {
		return "'"
	} else if ch == 40 {
		return "("
	} else if ch == 41 {
		return ")"
	} else if ch == 42 {
		return "*"
	} else if ch == 43 {
		return "+"
	} else if ch == 44 {
		return ","
	} else if ch == 45 {
		return "-"
	} else if ch == 46 {
		return "."
	} else if ch == 47 {
		return "/"
	} else if ch == 48 {
		return "0"
	} else if ch == 49 {
		return "1"
	} else if ch == 50 {
		return "2"
	} else if ch == 51 {
		return "3"
	} else if ch == 52 {
		return "4"
	} else if ch == 53 {
		return "5"
	} else if ch == 54 {
		return "6"
	} else if ch == 55 {
		return "7"
	} else if ch == 56 {
		return "8"
	} else if ch == 57 {
		return "9"
	} else if ch == 58 {
		return ":"
	} else if ch == 59 {
		return ";"
	} else if ch == 60 {
		return "<"
	} else if ch == 61 {
		return "="
	} else if ch == 62 {
		return ">"
	} else if ch == 63 {
		return "?"
	} else if ch == 64 {
		return "@"
	} else if ch == 65 {
		return "A"
	} else if ch == 66 {
		return "B"
	} else if ch == 67 {
		return "C"
	} else if ch == 68 {
		return "D"
	} else if ch == 69 {
		return "E"
	} else if ch == 70 {
		return "F"
	} else if ch == 71 {
		return "G"
	} else if ch == 72 {
		return "H"
	} else if ch == 73 {
		return "I"
	} else if ch == 74 {
		return "J"
	} else if ch == 75 {
		return "K"
	} else if ch == 76 {
		return "L"
	} else if ch == 77 {
		return "M"
	} else if ch == 78 {
		return "N"
	} else if ch == 79 {
		return "O"
	} else if ch == 80 {
		return "P"
	} else if ch == 81 {
		return "Q"
	} else if ch == 82 {
		return "R"
	} else if ch == 83 {
		return "S"
	} else if ch == 84 {
		return "T"
	} else if ch == 85 {
		return "U"
	} else if ch == 86 {
		return "V"
	} else if ch == 87 {
		return "W"
	} else if ch == 88 {
		return "X"
	} else if ch == 89 {
		return "Y"
	} else if ch == 90 {
		return "Z"
	} else if ch == 91 {
		return "["
	} else if ch == 93 {
		return "]"
	} else if ch == 94 {
		return "^"
	} else if ch == 95 {
		return "_"
	} else if ch == 96 {
		return "`"
	} else if ch == 97 {
		return "a"
	} else if ch == 98 {
		return "b"
	} else if ch == 99 {
		return "c"
	} else if ch == 100 {
		return "d"
	} else if ch == 101 {
		return "e"
	} else if ch == 102 {
		return "f"
	} else if ch == 103 {
		return "g"
	} else if ch == 104 {
		return "h"
	} else if ch == 105 {
		return "i"
	} else if ch == 106 {
		return "j"
	} else if ch == 107 {
		return "k"
	} else if ch == 108 {
		return "l"
	} else if ch == 109 {
		return "m"
	} else if ch == 110 {
		return "n"
	} else if ch == 111 {
		return "o"
	} else if ch == 112 {
		return "p"
	} else if ch == 113 {
		return "q"
	} else if ch == 114 {
		return "r"
	} else if ch == 115 {
		return "s"
	} else if ch == 116 {
		return "t"
	} else if ch == 117 {
		return "u"
	} else if ch == 118 {
		return "v"
	} else if ch == 119 {
		return "w"
	} else if ch == 120 {
		return "x"
	} else if ch == 121 {
		return "y"
	} else if ch == 122 {
		return "z"
	} else if ch == 123 {
		return "{"
	} else if ch == 124 {
		return "|"
	} else if ch == 125 {
		return "}"
	} else if ch == 126 {
		return "~"
	}
	return ""
}

// digitToCharMain converts a digit 0-9 to its character
func digitToCharMain(d int) string {
	if d == 0 {
		return "0"
	} else if d == 1 {
		return "1"
	} else if d == 2 {
		return "2"
	} else if d == 3 {
		return "3"
	} else if d == 4 {
		return "4"
	} else if d == 5 {
		return "5"
	} else if d == 6 {
		return "6"
	} else if d == 7 {
		return "7"
	} else if d == 8 {
		return "8"
	} else if d == 9 {
		return "9"
	}
	return "0"
}

// intToString converts an integer to a string (goany compatible)
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	digits := ""
	for {
		if n == 0 {
			break
		}
		digit := n % 10
		digits = digitToCharMain(digit) + digits
		n = n / 10
	}
	if neg {
		digits = "-" + digits
	}
	return digits
}

// readLineFromScreen reads a line of text from screen memory
func readLineFromScreen(c cpu.CPU, row int) string {
	result := ""
	baseAddr := TextScreenBase + (row * TextCols)
	col := 0
	for {
		if col >= TextCols {
			break
		}
		ch := int(c.Memory[baseAddr+col])
		if ch >= 32 && ch <= 126 {
			result = result + charFromCodeMain(ch)
		}
		col = col + 1
	}
	// Trim trailing spaces
	end := len(result)
	for {
		if end <= 0 {
			break
		}
		if result[end-1] != ' ' {
			break
		}
		end = end - 1
	}
	if end <= 0 {
		return ""
	}
	// Build trimmed result manually (avoid string slicing)
	trimmed := ""
	i := 0
	for {
		if i >= end {
			break
		}
		trimmed = trimmed + charFromCodeMain(int(result[i]))
		i = i + 1
	}
	return trimmed
}

// printReady displays "READY." on the screen at the given row
func printReady(c cpu.CPU, row int) cpu.CPU {
	text := "READY."
	baseAddr := TextScreenBase + (row * TextCols)
	i := 0
	for {
		if i >= len(text) {
			break
		}
		c.Memory[baseAddr+i] = uint8(text[i])
		i = i + 1
	}
	return c
}

// scrollScreenUp scrolls the screen up by one row and clears the last row
func scrollScreenUp(c cpu.CPU) cpu.CPU {
	row := 0
	for {
		if row >= TextRows-1 {
			break
		}
		srcAddr := TextScreenBase + ((row + 1) * TextCols)
		dstAddr := TextScreenBase + (row * TextCols)
		col := 0
		for {
			if col >= TextCols {
				break
			}
			c.Memory[dstAddr+col] = c.Memory[srcAddr+col]
			col = col + 1
		}
		row = row + 1
	}
	// Clear the last row
	lastRowAddr := TextScreenBase + ((TextRows - 1) * TextCols)
	clearCol := 0
	for {
		if clearCol >= TextCols {
			break
		}
		c.Memory[lastRowAddr+clearCol] = 32 // space
		clearCol = clearCol + 1
	}
	return c
}

// createC64WelcomeScreen creates the classic C64 boot screen
func createC64WelcomeScreen() []uint8 {
	lines := []string{}

	// First, clear the screen with spaces
	lines = clearScreen(lines)

	// Classic C64 boot screen:
	//
	//     **** COMMODORE 64 BASIC V2 ****
	//
	//  64K RAM SYSTEM  38911 BASIC BYTES FREE
	//
	// READY.
	// _

	// Row 1: "    **** COMMODORE 64 BASIC V2 ****"
	lines = addStringToScreen(lines, "**** COMMODORE 64 BASIC V2 ****", 1, 4)

	// Row 3: " 64K RAM SYSTEM  38911 BASIC BYTES FREE"
	lines = addStringToScreen(lines, "64K RAM SYSTEM  38911 BASIC BYTES FREE", 3, 1)

	// Row 5: "READY."
	lines = addStringToScreen(lines, "READY.", 5, 0)

	// Row 7: Cursor - use underscore as cursor representation (blank line after READY.)
	// Row 7, col 0 = 0x0400 + (7 * 40) = 0x0400 + 280 = 0x0518
	lines = append(lines, "LDA #$5F") // underscore cursor (ASCII 95)
	lines = append(lines, "STA $0518")

	lines = append(lines, "BRK")
	return assembler.AssembleLines(lines)
}

// frame is the per-frame callback for RunLoopWithState.
// It receives the window and state by value, returns updated state.
// No closure capture needed — all state flows through parameters.
func frame(w graphics.Window, state C64State) (C64State, bool) {
	// Handle keyboard input
	key := graphics.GetLastKey()
	if key != 0 {
		// Clear old cursor BEFORE changing position
		oldCursorAddr := TextScreenBase + (state.CursorRow * TextCols) + state.CursorCol
		if state.C.Memory[oldCursorAddr] == 95 {
			state.C.Memory[oldCursorAddr] = 32 // clear old cursor
		}

		if key == 13 {
			// Enter - process the command
			line := readLineFromScreen(state.C, state.InputStartRow)

			// Move cursor to next line first
			state.CursorCol = 0
			state.CursorRow = state.CursorRow + 1
			if state.CursorRow >= TextRows {
				state.C = scrollScreenUp(state.C)
				state.CursorRow = TextRows - 1
				// Adjust inputStartRow if it was scrolled
				if state.InputStartRow > 0 {
					state.InputStartRow = state.InputStartRow - 1
				}
			}

			// Process the line if not empty
			if len(line) > 0 {
				// Check if it has a line number (store program line)
				if basic.HasLineNumber(line) {
					lineNum, rest := basic.ExtractLineNumber(line)
					state.BasicState = basic.StoreLine(state.BasicState, lineNum, rest)
				} else if basic.IsCommand(line, "RUN") {
					// Execute stored program
					lineCount := basic.GetLineCount(state.BasicState)
					if lineCount > 0 {
						state.BasicState = basic.SetCursor(state.BasicState, state.CursorRow, 0)
						code := basic.CompileProgram(state.BasicState)
						// Execute the program
						state.C = cpu.LoadProgram(state.C, code, 0xC000)
						state.C = cpu.SetPC(state.C, 0xC000)
						state.C = cpu.ClearHalted(state.C)
						state.C = cpu.Run(state.C, 1000000)
						// Read cursor row from zero page (set by PRINT statements)
						state.CursorRow = int(state.C.Memory[basic.GetCursorRowAddr()])
						for {
							if state.CursorRow < TextRows {
								break
							}
							state.C = scrollScreenUp(state.C)
							state.CursorRow = state.CursorRow - 1
						}
					}
					// Print READY.
					state.C = printReady(state.C, state.CursorRow)
					state.CursorRow = state.CursorRow + 1
					if state.CursorRow >= TextRows {
						state.C = scrollScreenUp(state.C)
						state.CursorRow = TextRows - 1
					}
				} else if basic.IsCommand(line, "LIST") {
					// List stored program
					i := 0
					listRow := state.CursorRow
					for {
						if i >= basic.GetLineCount(state.BasicState) {
							break
						}
						if listRow >= TextRows {
							break
						}
						pl := basic.GetLine(state.BasicState, i)
						// Format line number
						numStr := intToString(pl.LineNum)
						listLine := numStr + " " + pl.Text
						// Write to screen
						baseAddr := TextScreenBase + (listRow * TextCols)
						j := 0
						for {
							if j >= len(listLine) {
								break
							}
							if j >= TextCols {
								break
							}
							state.C.Memory[baseAddr+j] = uint8(listLine[j])
							j = j + 1
						}
						listRow = listRow + 1
						i = i + 1
					}
					state.CursorRow = listRow
					if state.CursorRow >= TextRows {
						state.C = scrollScreenUp(state.C)
						state.CursorRow = TextRows - 1
					}
					// Print READY.
					state.C = printReady(state.C, state.CursorRow)
					state.CursorRow = state.CursorRow + 1
					if state.CursorRow >= TextRows {
						state.C = scrollScreenUp(state.C)
						state.CursorRow = TextRows - 1
					}
				} else if basic.IsCommand(line, "NEW") {
					// Clear program
					state.BasicState = basic.ClearProgram(state.BasicState)
					// Print READY.
					state.C = printReady(state.C, state.CursorRow)
					state.CursorRow = state.CursorRow + 1
					if state.CursorRow >= TextRows {
						state.C = scrollScreenUp(state.C)
						state.CursorRow = TextRows - 1
					}
				} else if basic.IsCommand(line, "CLR") {
					// Clear screen and reset cursor to top-left
					state.BasicState = basic.SetCursor(state.BasicState, 0, 0)
					code := basic.CompileImmediate(state.BasicState, line)
					state.C = cpu.LoadProgram(state.C, code, 0xC000)
					state.C = cpu.SetPC(state.C, 0xC000)
					state.C = cpu.ClearHalted(state.C)
					state.C = cpu.Run(state.C, 100000)
					// Reset cursor to top of screen
					state.CursorRow = 0
					state.CursorCol = 0
					// Print READY. at top
					state.C = printReady(state.C, state.CursorRow)
					state.CursorRow = state.CursorRow + 1
				} else {
					// Immediate execution (PRINT, POKE, etc.)
					state.BasicState = basic.SetCursor(state.BasicState, state.CursorRow, 0)
					code := basic.CompileImmediate(state.BasicState, line)
					if len(code) > 1 { // More than just BRK
						state.C = cpu.LoadProgram(state.C, code, 0xC000)
						state.C = cpu.SetPC(state.C, 0xC000)
						state.C = cpu.ClearHalted(state.C)
						state.C = cpu.Run(state.C, 10000)
					}
					// For PRINT, move cursor down
					if basic.IsCommand(line, "PRINT") {
						state.CursorRow = state.CursorRow + 1
						if state.CursorRow >= TextRows {
							state.C = scrollScreenUp(state.C)
							state.CursorRow = TextRows - 1
						}
					}
					// Print READY. after command execution
					state.C = printReady(state.C, state.CursorRow)
					state.CursorRow = state.CursorRow + 1
					if state.CursorRow >= TextRows {
						state.C = scrollScreenUp(state.C)
						state.CursorRow = TextRows - 1
					}
				}
			}

			// Update input start row for next command
			state.InputStartRow = state.CursorRow
		} else if key == 8 {
			// Backspace - move back and clear
			if state.CursorCol > 0 {
				state.CursorCol = state.CursorCol - 1
				// Clear the character at cursor position
				addr := TextScreenBase + (state.CursorRow * TextCols) + state.CursorCol
				state.C.Memory[addr] = 32 // space
			} else if state.CursorRow > 0 {
				// At beginning of line - move to end of previous line
				state.CursorRow = state.CursorRow - 1
				state.CursorCol = TextCols - 1
			}
		} else if key >= 32 && key <= 126 {
			// Printable character
			addr := TextScreenBase + (state.CursorRow * TextCols) + state.CursorCol
			state.C.Memory[addr] = uint8(key)
			state.CursorCol = state.CursorCol + 1
			if state.CursorCol >= TextCols {
				state.CursorCol = 0
				state.CursorRow = state.CursorRow + 1
				if state.CursorRow >= TextRows {
					state.C = scrollScreenUp(state.C)
					state.CursorRow = TextRows - 1
				}
			}
		}
		// Draw cursor at new position
		cursorAddr := TextScreenBase + (state.CursorRow * TextCols) + state.CursorCol
		if state.C.Memory[cursorAddr] == 32 {
			state.C.Memory[cursorAddr] = 95 // underscore cursor
		}
	}

	// Clear screen with C64 dark blue background
	graphics.Clear(w, state.BgColor)

	// Render the text screen
	memAddr := TextScreenBase
	charY := 0
	for {
		if charY >= TextRows {
			break
		}
		charX := 0
		for {
			if charX >= TextCols {
				break
			}
			// Get character code from screen memory
			charCode := int(cpu.GetMemory(state.C, memAddr))
			memAddr = memAddr + 1

			// Only render printable non-space characters (33-127)
			// Skip spaces (32) as they have no pixels to render
			if charCode > 32 && charCode <= 127 {
				// Render 8x8 character bitmap
				baseScreenX := int32(charX * 8)
				baseScreenY := int32(charY * 8)
				pixelY := 0
				for {
					if pixelY >= 8 {
						break
					}
					// Get entire row byte once (reduces function calls from 64 to 8)
					rowByte := font.GetRow(state.FontData, charCode, pixelY)
					if rowByte != 0 {
						// Only iterate if row has any pixels
						mask := uint8(0x80)
						pixelX := 0
						for {
							if pixelX >= 8 {
								break
							}
							if (rowByte & mask) != 0 {
								screenX := (baseScreenX + int32(pixelX)) * state.Scale
								screenY := (baseScreenY + int32(pixelY)) * state.Scale
								graphics.FillRect(w, graphics.NewRect(screenX, screenY, state.Scale, state.Scale), state.TextColor)
							}
							mask = mask >> 1
							pixelX = pixelX + 1
						}
					}
					pixelY = pixelY + 1
				}
			}
			charX = charX + 1
		}
		charY = charY + 1
	}

	// Present frame
	graphics.Present(w)

	return state, true
}

func main() {
	// Create window (320x200 C64 resolution scaled up)
	scale := int32(4)
	windowWidth := int32(TextCols*8) * scale
	windowHeight := int32(TextRows*8) * scale
	w := graphics.CreateWindow("Commodore 64", windowWidth, windowHeight)

	// Create CPU
	c := cpu.NewCPU()

	// Load font data
	fontData := font.GetFontData()

	// Create the C64 welcome screen program
	program := createC64WelcomeScreen()

	// Load and run program
	c = cpu.LoadProgram(c, program, 0x0600)
	c = cpu.SetPC(c, 0x0600)
	c = cpu.ClearHalted(c)
	c = cpu.Run(c, 100000)

	// C64 colors: light blue text on dark blue background
	textColor := graphics.NewColor(134, 122, 222, 255) // C64 light blue
	bgColor := graphics.NewColor(64, 50, 133, 255)     // C64 dark blue

	// Initialize BASIC interpreter state
	basicState := basic.NewBasicState()
	basicState = basic.SetCursor(basicState, 7, 0)

	// Create initial state — all mutable data flows through RunLoopWithState
	state := C64State{
		C:             c,
		CursorRow:     7,
		CursorCol:     0,
		BasicState:    basicState,
		InputStartRow: 7,
		FontData:      fontData,
		TextColor:     textColor,
		BgColor:       bgColor,
		Scale:         scale,
	}

	// Main display loop using RunLoopWithState — state is threaded through
	// the frame function by value, eliminating closure capture and cloning
	graphics.RunLoopWithState(w, state, frame)

	graphics.CloseWindow(w)
}
