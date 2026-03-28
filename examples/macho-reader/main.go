package main

import (
	"fmt"
	"libs/binreader"
	"libs/macho"
	"os"
)

func int64ToHex(n int64) string {
	if n == 0 {
		return "0"
	}
	hexChars := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "a", "b", "c", "d", "e", "f"}
	result := ""
	val := n
	for val > 0 {
		digit := val % 16
		result = hexChars[int(digit)] + result
		val = val / 16
	}
	return result
}

func main() {
	path := "/usr/bin/true"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	fmt.Println("=== Mach-O Reader ===")
	fmt.Println("File: " + path)
	fmt.Println("")

	f := macho.ParseFile(path)
	if f.Error != "" {
		fmt.Println("Error: " + f.Error)
		fmt.Println("FAIL")
	} else {
		// Print header
		fmt.Println("--- Header ---")
		fmt.Println("  CPU Type: " + macho.CPUTypeName(f.Header.CPUType))
		fmt.Println("  File Type: " + macho.FileTypeName(f.Header.FileType))
		fmt.Println("  Load Commands: " + binreader.IntToString(f.Header.NCmds))
		fmt.Println("  Size of Commands: " + binreader.IntToString(f.Header.SizeOfCmds))
		fmt.Println("")

		// Print segments and sections
		fmt.Println("--- Segments ---")
		i := 0
		for i < len(f.Segments) {
			seg := f.Segments[i]
			fmt.Println("  " + seg.Name)
			fmt.Println("    VM Addr: 0x" + int64ToHex(seg.VMAddr))
			fmt.Println("    VM Size: 0x" + int64ToHex(seg.VMSize))
			fmt.Println("    File Offset: 0x" + int64ToHex(seg.FileOff))
			fmt.Println("    File Size: 0x" + int64ToHex(seg.FileSize))
			fmt.Println("    Max Prot: " + macho.ProtString(seg.MaxProt))
			fmt.Println("    Init Prot: " + macho.ProtString(seg.InitProt))
			fmt.Println("    Sections: " + binreader.IntToString(seg.NSects))
			j := 0
			for j < len(seg.Sections) {
				sec := seg.Sections[j]
				fmt.Println("      " + sec.SectName + " (in " + sec.SegName + ")")
				fmt.Println("        Addr: 0x" + int64ToHex(sec.Addr) + "  Size: 0x" + int64ToHex(sec.Size))
				j = j + 1
			}
			i = i + 1
		}
		fmt.Println("")

		// Print dylibs
		if len(f.Dylibs) > 0 {
			fmt.Println("--- Dynamic Libraries ---")
			di := 0
			for di < len(f.Dylibs) {
				dl := f.Dylibs[di]
				fmt.Println("  " + dl.Name)
				fmt.Println("    Version: " + macho.FormatVersion(dl.CurrentVer))
				fmt.Println("    Compat: " + macho.FormatVersion(dl.CompatVer))
				di = di + 1
			}
			fmt.Println("")
		}

		// Print entry point
		if f.Entry.Present {
			fmt.Println("--- Entry Point ---")
			fmt.Println("  Offset: 0x" + int64ToHex(f.Entry.EntryOff))
			fmt.Println("  Stack Size: " + binreader.Int64ToString(f.Entry.StackSize))
			fmt.Println("")
		}

		// Print UUID
		if f.UUID.Present {
			fmt.Println("--- UUID ---")
			fmt.Println("  " + f.UUID.UUID)
			fmt.Println("")
		}

		// Print load commands summary
		fmt.Println("--- Load Commands ---")
		lci := 0
		for lci < len(f.LoadCommands) {
			lc := f.LoadCommands[lci]
			fmt.Println("  " + macho.CmdName(lc.Cmd) + " (size: " + binreader.IntToString(lc.CmdSize) + ")")
			lci = lci + 1
		}
		fmt.Println("")

		// Print first N symbols
		if len(f.Symbols) > 0 {
			fmt.Println("--- Symbols (first 50) ---")
			maxSyms := 50
			if len(f.Symbols) < maxSyms {
				maxSyms = len(f.Symbols)
			}
			si := 0
			for si < maxSyms {
				sym := f.Symbols[si]
				fmt.Println("  " + sym.Name + "  value=0x" + int64ToHex(sym.Value))
				si = si + 1
			}
			if len(f.Symbols) > 50 {
				fmt.Println("  ... (" + binreader.IntToString(len(f.Symbols)-50) + " more)")
			}
			fmt.Println("")
		}

		// PASS/FAIL checks
		fmt.Println("--- Checks ---")
		allPassed := true

		// Check 1: Valid CPU type
		if f.Header.CPUType == macho.CPUTypeX86_64 || f.Header.CPUType == macho.CPUTypeARM64 {
			fmt.Println("  PASS: Valid CPU type (" + macho.CPUTypeName(f.Header.CPUType) + ")")
		} else {
			fmt.Println("  FAIL: Unknown CPU type")
			allPassed = false
		}

		// Check 2: File type is MH_EXECUTE
		if f.Header.FileType == macho.MHExecute {
			fmt.Println("  PASS: File type is MH_EXECUTE")
		} else {
			fmt.Println("  FAIL: File type is not MH_EXECUTE (" + macho.FileTypeName(f.Header.FileType) + ")")
			allPassed = false
		}

		// Check 3: Has __TEXT segment
		hasText := false
		ti := 0
		for ti < len(f.Segments) {
			if f.Segments[ti].Name == "__TEXT" {
				hasText = true
			}
			ti = ti + 1
		}
		if hasText {
			fmt.Println("  PASS: Has __TEXT segment")
		} else {
			fmt.Println("  FAIL: Missing __TEXT segment")
			allPassed = false
		}

		// Check 4: Load command count matches header
		if len(f.LoadCommands) == f.Header.NCmds {
			fmt.Println("  PASS: Load command count matches header (" + binreader.IntToString(f.Header.NCmds) + ")")
		} else {
			fmt.Println("  FAIL: Load command count mismatch (header=" + binreader.IntToString(f.Header.NCmds) + " actual=" + binreader.IntToString(len(f.LoadCommands)) + ")")
			allPassed = false
		}

		fmt.Println("")
		if allPassed {
			fmt.Println("PASS")
		} else {
			fmt.Println("FAIL")
		}

		macho.CloseFile(f)
	}
}
