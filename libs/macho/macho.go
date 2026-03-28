package macho

import (
	"libs/binreader"
	"runtime/fs"
)

// readAtBytes wraps fs.ReadAt and returns a properly-typed []byte slice
// (needed because fs.ReadAt returns native byte[] in C# but transpiler expects List<byte>)
func readAtBytes(handle int, offset int64, size int) ([]byte, int, string) {
	rawData, bytesRead, err := fs.ReadAt(handle, offset, size)
	if err != "" {
		empty := []byte{}
		return empty, 0, err
	}
	result := []byte{}
	i := 0
	for i < bytesRead {
		result = append(result, rawData[i])
		i = i + 1
	}
	return result, bytesRead, ""
}

// LCMain returns the LC_MAIN load command value (0x80000028)
// Computed at runtime to avoid 32-bit int overflow in C++ transpilation
func LCMain() int {
	return 0x28 | (0x80 << 24)
}

// byteToHex converts a byte value (0-255) to a 2-char hex string with zero-pad
func byteToHex(b int) string {
	hexChars := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "a", "b", "c", "d", "e", "f"}
	hi := (b >> 4) & 0x0F
	lo := b & 0x0F
	return hexChars[hi] + hexChars[lo]
}

// intToHex converts an int to a hex string (no prefix)
func intToHex(n int) string {
	if n == 0 {
		return "0"
	}
	hexChars := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "a", "b", "c", "d", "e", "f"}
	result := ""
	val := n
	for val > 0 {
		digit := val % 16
		result = hexChars[digit] + result
		val = val / 16
	}
	return result
}

// int64ToHex converts an int64 to a hex string (no prefix)
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

// formatUUID formats 16 bytes as a UUID string (8-4-4-4-12)
func formatUUID(data []byte) string {
	result := ""
	i := 0
	for i < 16 {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			result += "-"
		}
		result += byteToHex(int(data[i]))
		i = i + 1
	}
	return result
}

// CPUTypeName returns a human-readable CPU type name
func CPUTypeName(cpuType int) string {
	if cpuType == CPUTypeX86_64 {
		return "x86_64"
	} else if cpuType == CPUTypeARM64 {
		return "arm64"
	}
	return "unknown(" + binreader.IntToString(cpuType) + ")"
}

// FileTypeName returns a human-readable file type name
func FileTypeName(ft int) string {
	if ft == MHObject {
		return "MH_OBJECT"
	} else if ft == MHExecute {
		return "MH_EXECUTE"
	} else if ft == MHDylib {
		return "MH_DYLIB"
	} else if ft == MHBundle {
		return "MH_BUNDLE"
	} else if ft == MHDsym {
		return "MH_DSYM"
	}
	return "unknown(" + binreader.IntToString(ft) + ")"
}

// CmdName returns a human-readable load command name
func CmdName(cmd int) string {
	if cmd == LCSegment64 {
		return "LC_SEGMENT_64"
	} else if cmd == LCSymtab {
		return "LC_SYMTAB"
	} else if cmd == LCDysymtab {
		return "LC_DYSYMTAB"
	} else if cmd == LCLoadDylib {
		return "LC_LOAD_DYLIB"
	} else if cmd == LCMain() {
		return "LC_MAIN"
	} else if cmd == LCUUID {
		return "LC_UUID"
	} else if cmd == LCBuildVersion {
		return "LC_BUILD_VERSION"
	}
	return "LC_(" + binreader.IntToString(cmd) + ")"
}

// ProtString returns a protection string like "r-x", "rw-", etc.
func ProtString(prot int) string {
	r := "-"
	w := "-"
	x := "-"
	if (prot & VMProtRead) != 0 {
		r = "r"
	}
	if (prot & VMProtWrite) != 0 {
		w = "w"
	}
	if (prot & VMProtExecute) != 0 {
		x = "x"
	}
	return r + w + x
}

// FormatVersion formats a packed Mach-O version (X.Y.Z packed in 32 bits)
func FormatVersion(v int) string {
	major := (v >> 16) & 0xFFFF
	minor := (v >> 8) & 0xFF
	patch := v & 0xFF
	return binreader.IntToString(major) + "." + binreader.IntToString(minor) + "." + binreader.IntToString(patch)
}

// parseHeader reads the Mach-O 64-bit header (32 bytes)
func parseHeader(rs binreader.ReadState) (binreader.ReadState, MachOHeader) {
	hdr := MachOHeader{}
	rs2, magic := binreader.ReadUint32LE(rs)
	if rs2.Err != "" {
		return rs2, hdr
	}
	hdr.Magic = magic
	rs3, cpuType := binreader.ReadUint32LE(rs2)
	if rs3.Err != "" {
		return rs3, hdr
	}
	hdr.CPUType = cpuType
	rs4, cpuSubtype := binreader.ReadUint32LE(rs3)
	if rs4.Err != "" {
		return rs4, hdr
	}
	hdr.CPUSubtype = cpuSubtype
	rs5, fileType := binreader.ReadUint32LE(rs4)
	if rs5.Err != "" {
		return rs5, hdr
	}
	hdr.FileType = fileType
	rs6, ncmds := binreader.ReadUint32LE(rs5)
	if rs6.Err != "" {
		return rs6, hdr
	}
	hdr.NCmds = ncmds
	rs7, sizeOfCmds := binreader.ReadUint32LE(rs6)
	if rs7.Err != "" {
		return rs7, hdr
	}
	hdr.SizeOfCmds = sizeOfCmds
	rs8, flags := binreader.ReadUint32LE(rs7)
	if rs8.Err != "" {
		return rs8, hdr
	}
	hdr.Flags = flags
	rs9, reserved := binreader.ReadUint32LE(rs8)
	if rs9.Err != "" {
		return rs9, hdr
	}
	hdr.Reserved = reserved
	return rs9, hdr
}

// parseSection64 reads a single section_64 structure (80 bytes)
func parseSection64(rs binreader.ReadState) (binreader.ReadState, Section64) {
	sec := Section64{}
	rs2, sectName := binreader.ReadFixedString(rs, 16)
	if rs2.Err != "" {
		return rs2, sec
	}
	sec.SectName = sectName
	rs3, segName := binreader.ReadFixedString(rs2, 16)
	if rs3.Err != "" {
		return rs3, sec
	}
	sec.SegName = segName
	rs4, addr := binreader.ReadUint64LE(rs3)
	if rs4.Err != "" {
		return rs4, sec
	}
	sec.Addr = addr
	rs5, size := binreader.ReadUint64LE(rs4)
	if rs5.Err != "" {
		return rs5, sec
	}
	sec.Size = size
	rs6, offset := binreader.ReadUint32LE(rs5)
	if rs6.Err != "" {
		return rs6, sec
	}
	sec.Offset = offset
	rs7, align := binreader.ReadUint32LE(rs6)
	if rs7.Err != "" {
		return rs7, sec
	}
	sec.Align = align
	rs8, relOff := binreader.ReadUint32LE(rs7)
	if rs8.Err != "" {
		return rs8, sec
	}
	sec.RelOff = relOff
	rs9, nReloc := binreader.ReadUint32LE(rs8)
	if rs9.Err != "" {
		return rs9, sec
	}
	sec.NReloc = nReloc
	rs10, flags := binreader.ReadUint32LE(rs9)
	if rs10.Err != "" {
		return rs10, sec
	}
	sec.Flags = flags
	rs11, r1 := binreader.ReadUint32LE(rs10)
	if rs11.Err != "" {
		return rs11, sec
	}
	sec.Reserved1 = r1
	rs12, r2 := binreader.ReadUint32LE(rs11)
	if rs12.Err != "" {
		return rs12, sec
	}
	sec.Reserved2 = r2
	rs13, r3 := binreader.ReadUint32LE(rs12)
	if rs13.Err != "" {
		return rs13, sec
	}
	sec.Reserved3 = r3
	return rs13, sec
}

// parseSegment64 reads a segment_64 load command (after cmd/cmdsize already read)
func parseSegment64(rs binreader.ReadState) (binreader.ReadState, Segment64) {
	seg := Segment64{}
	seg.Sections = []Section64{}
	rs2, name := binreader.ReadFixedString(rs, 16)
	if rs2.Err != "" {
		return rs2, seg
	}
	seg.Name = name
	rs3, vmAddr := binreader.ReadUint64LE(rs2)
	if rs3.Err != "" {
		return rs3, seg
	}
	seg.VMAddr = vmAddr
	rs4, vmSize := binreader.ReadUint64LE(rs3)
	if rs4.Err != "" {
		return rs4, seg
	}
	seg.VMSize = vmSize
	rs5, fileOff := binreader.ReadUint64LE(rs4)
	if rs5.Err != "" {
		return rs5, seg
	}
	seg.FileOff = fileOff
	rs6, fileSize := binreader.ReadUint64LE(rs5)
	if rs6.Err != "" {
		return rs6, seg
	}
	seg.FileSize = fileSize
	rs7, maxProt := binreader.ReadUint32LE(rs6)
	if rs7.Err != "" {
		return rs7, seg
	}
	seg.MaxProt = maxProt
	rs8, initProt := binreader.ReadUint32LE(rs7)
	if rs8.Err != "" {
		return rs8, seg
	}
	seg.InitProt = initProt
	rs9, nsects := binreader.ReadUint32LE(rs8)
	if rs9.Err != "" {
		return rs9, seg
	}
	seg.NSects = nsects
	rs10, flags := binreader.ReadUint32LE(rs9)
	if rs10.Err != "" {
		return rs10, seg
	}
	seg.Flags = flags
	// Read sections
	rsS := rs10
	si := 0
	for si < nsects {
		rsS2, sec := parseSection64(rsS)
		if rsS2.Err != "" {
			return rsS2, seg
		}
		seg.Sections = append(seg.Sections, sec)
		rsS = rsS2
		si = si + 1
	}
	return rsS, seg
}

// parseSymtabCmd reads the symtab command fields (after cmd/cmdsize)
func parseSymtabCmd(rs binreader.ReadState) (binreader.ReadState, SymtabCommand) {
	st := SymtabCommand{}
	rs2, symOff := binreader.ReadUint32LE(rs)
	if rs2.Err != "" {
		return rs2, st
	}
	st.SymOff = symOff
	rs3, nSyms := binreader.ReadUint32LE(rs2)
	if rs3.Err != "" {
		return rs3, st
	}
	st.NSyms = nSyms
	rs4, strOff := binreader.ReadUint32LE(rs3)
	if rs4.Err != "" {
		return rs4, st
	}
	st.StrOff = strOff
	rs5, strSize := binreader.ReadUint32LE(rs4)
	if rs5.Err != "" {
		return rs5, st
	}
	st.StrSize = strSize
	return rs5, st
}

// parseDylibCmd reads a dylib load command (after cmd/cmdsize)
func parseDylibCmd(rs binreader.ReadState, handle int, cmdStartPos int64) (binreader.ReadState, DylibInfo) {
	dl := DylibInfo{}
	// Read name offset (relative to start of load command)
	rs2, nameOffset := binreader.ReadUint32LE(rs)
	if rs2.Err != "" {
		return rs2, dl
	}
	rs3, timestamp := binreader.ReadUint32LE(rs2)
	if rs3.Err != "" {
		return rs3, dl
	}
	dl.Timestamp = timestamp
	rs4, curVer := binreader.ReadUint32LE(rs3)
	if rs4.Err != "" {
		return rs4, dl
	}
	dl.CurrentVer = curVer
	rs5, compatVer := binreader.ReadUint32LE(rs4)
	if rs5.Err != "" {
		return rs5, dl
	}
	dl.CompatVer = compatVer
	// Read dylib name from the file at cmdStartPos + nameOffset
	namePos := cmdStartPos + int64(nameOffset)
	nameData, bytesRead, err := readAtBytes(handle, namePos, 256)
	if err != "" || bytesRead == 0 {
		dl.Name = "?"
		return rs5, dl
	}
	dl.Name = binreader.ExtractString(nameData, 0)
	return rs5, dl
}

// parseEntryPointCmd reads the entry point command (after cmd/cmdsize)
func parseEntryPointCmd(rs binreader.ReadState) (binreader.ReadState, EntryPoint) {
	ep := EntryPoint{}
	ep.Present = true
	rs2, entryOff := binreader.ReadUint64LE(rs)
	if rs2.Err != "" {
		return rs2, ep
	}
	ep.EntryOff = entryOff
	rs3, stackSize := binreader.ReadUint64LE(rs2)
	if rs3.Err != "" {
		return rs3, ep
	}
	ep.StackSize = stackSize
	return rs3, ep
}

// parseUUIDCmd reads the UUID command (after cmd/cmdsize)
func parseUUIDCmd(rs binreader.ReadState) (binreader.ReadState, UUIDInfo) {
	ui := UUIDInfo{}
	rs2, data := binreader.ReadBytes(rs, 16)
	if rs2.Err != "" {
		return rs2, ui
	}
	ui.UUID = formatUUID(data)
	ui.Present = true
	return rs2, ui
}

// resolveSymbols reads symbols from the symbol table using ReadAt
func resolveSymbols(handle int, st SymtabCommand) []Symbol {
	symbols := []Symbol{}
	if st.NSyms == 0 || st.StrSize == 0 {
		return symbols
	}
	// Read the string table
	strData, strRead, strErr := readAtBytes(handle, int64(st.StrOff), st.StrSize)
	if strErr != "" || strRead == 0 {
		return symbols
	}
	// Read symbol entries (nlist_64 is 16 bytes each)
	nlistSize := 16
	totalBytes := st.NSyms * nlistSize
	symData, symRead, symErr := readAtBytes(handle, int64(st.SymOff), totalBytes)
	if symErr != "" || symRead == 0 {
		return symbols
	}
	i := 0
	for i < st.NSyms {
		off := i * nlistSize
		if off+nlistSize > symRead {
			break
		}
		// nlist_64: uint32 n_strx, uint8 n_type, uint8 n_sect, uint16 n_desc, uint64 n_value
		strx := int(symData[off]) | (int(symData[off+1]) << 8) | (int(symData[off+2]) << 16) | (int(symData[off+3]) << 24)
		nType := int(symData[off+4])
		nSect := int(symData[off+5])
		nDesc := int(symData[off+6]) | (int(symData[off+7]) << 8)
		b0 := int64(symData[off+8])
		b1 := int64(symData[off+9])
		b2 := int64(symData[off+10])
		b3 := int64(symData[off+11])
		b4 := int64(symData[off+12])
		b5 := int64(symData[off+13])
		b6 := int64(symData[off+14])
		b7 := int64(symData[off+15])
		value := b0 | (b1 << 8) | (b2 << 16) | (b3 << 24) | (b4 << 32) | (b5 << 40) | (b6 << 48) | (b7 << 56)
		sym := Symbol{}
		if strx >= 0 && strx < st.StrSize {
			sym.Name = binreader.ExtractString(strData, strx)
		} else {
			sym.Name = "?"
		}
		sym.NType = nType
		sym.NSect = nSect
		sym.NDesc = nDesc
		sym.Value = value
		symbols = append(symbols, sym)
		i = i + 1
	}
	return symbols
}

// ParseFile parses a Mach-O file at the given path
func ParseFile(path string) MachOFile {
	result := MachOFile{}
	result.Segments = []Segment64{}
	result.Symbols = []Symbol{}
	result.Dylibs = []DylibInfo{}
	result.LoadCommands = []LoadCommandInfo{}

	handle, err := fs.Open(path)
	if err != "" {
		result.Error = err
		return result
	}
	result.Handle = handle

	rs := binreader.NewReadState(handle)

	// Check for fat/universal binary
	rs2, magic := binreader.ReadUint32LE(rs)
	if rs2.Err != "" {
		result.Error = rs2.Err
		fs.Close(handle)
		return result
	}

	// If fat binary, find the best architecture and seek to it
	if magic == FatMagic {
		// Fat header uses big-endian
		// Re-read from start as big-endian
		rsF := binreader.NewReadState(handle)
		rsF = binreader.SkipBytes(rsF, 4) // skip magic
		rsF2, nArch := binreader.ReadUint32BE(rsF)
		if rsF2.Err != "" {
			result.Error = rsF2.Err
			fs.Close(handle)
			return result
		}
		if nArch == 0 {
			result.Error = "fat binary has no architectures"
			fs.Close(handle)
			return result
		}
		// Read all architecture entries: cpu_type(4) + cpu_subtype(4) + offset(4) + size(4) + align(4)
		bestOffset := 0
		foundAny := false
		rsA := rsF2
		ai := 0
		for ai < nArch {
			rsA2, cpuType := binreader.ReadUint32BE(rsA)
			if rsA2.Err != "" {
				result.Error = rsA2.Err
				fs.Close(handle)
				return result
			}
			rsA3, _ := binreader.ReadUint32BE(rsA2) // cpu_subtype
			if rsA3.Err != "" {
				result.Error = rsA3.Err
				fs.Close(handle)
				return result
			}
			rsA4, sliceOff := binreader.ReadUint32BE(rsA3) // offset
			if rsA4.Err != "" {
				result.Error = rsA4.Err
				fs.Close(handle)
				return result
			}
			rsA5, _ := binreader.ReadUint32BE(rsA4) // size
			if rsA5.Err != "" {
				result.Error = rsA5.Err
				fs.Close(handle)
				return result
			}
			rsA6, _ := binreader.ReadUint32BE(rsA5) // align
			if rsA6.Err != "" {
				result.Error = rsA6.Err
				fs.Close(handle)
				return result
			}
			// Prefer arm64, fall back to first slice
			if !foundAny {
				bestOffset = sliceOff
				foundAny = true
			}
			if cpuType == CPUTypeARM64 {
				bestOffset = sliceOff
			}
			rsA = rsA6
			ai = ai + 1
		}
		// Seek to the best slice offset
		rs2 = binreader.NewReadState(handle)
		rs2 = binreader.SkipBytes(rs2, int64(bestOffset))
		if rs2.Err != "" {
			result.Error = rs2.Err
			fs.Close(handle)
			return result
		}
	} else {
		// Not a fat binary, rewind
		rs2 = binreader.NewReadState(handle)
	}

	// Parse header
	rs3, hdr := parseHeader(rs2)
	if rs3.Err != "" {
		result.Error = rs3.Err
		fs.Close(handle)
		return result
	}
	if hdr.Magic != MachOMagic64 {
		result.Error = "not a 64-bit Mach-O file"
		fs.Close(handle)
		return result
	}
	result.Header = hdr

	// Parse load commands
	rsLC := rs3
	ci := 0
	for ci < hdr.NCmds {
		cmdStartPos := rsLC.Pos
		rsLC2, cmd := binreader.ReadUint32LE(rsLC)
		if rsLC2.Err != "" {
			result.Error = rsLC2.Err
			fs.Close(handle)
			return result
		}
		rsLC3, cmdSize := binreader.ReadUint32LE(rsLC2)
		if rsLC3.Err != "" {
			result.Error = rsLC3.Err
			fs.Close(handle)
			return result
		}
		lci := LoadCommandInfo{}
		lci.Cmd = cmd
		lci.CmdSize = cmdSize
		result.LoadCommands = append(result.LoadCommands, lci)

		if cmd == LCSegment64 {
			rsLC4, seg := parseSegment64(rsLC3)
			if rsLC4.Err != "" {
				result.Error = rsLC4.Err
				fs.Close(handle)
				return result
			}
			result.Segments = append(result.Segments, seg)
			// Skip any remaining bytes in the load command
			consumed := rsLC4.Pos - cmdStartPos
			remaining := int64(cmdSize) - consumed
			if remaining > 0 {
				rsLC4 = binreader.SkipBytes(rsLC4, remaining)
			}
			rsLC = rsLC4
		} else if cmd == LCSymtab {
			rsLC4, st := parseSymtabCmd(rsLC3)
			if rsLC4.Err != "" {
				result.Error = rsLC4.Err
				fs.Close(handle)
				return result
			}
			result.Symtab = st
			result.HasSymtab = true
			consumed := rsLC4.Pos - cmdStartPos
			remaining := int64(cmdSize) - consumed
			if remaining > 0 {
				rsLC4 = binreader.SkipBytes(rsLC4, remaining)
			}
			rsLC = rsLC4
		} else if cmd == LCLoadDylib {
			rsLC4, dl := parseDylibCmd(rsLC3, handle, cmdStartPos)
			if rsLC4.Err != "" {
				result.Error = rsLC4.Err
				fs.Close(handle)
				return result
			}
			result.Dylibs = append(result.Dylibs, dl)
			consumed := rsLC4.Pos - cmdStartPos
			remaining := int64(cmdSize) - consumed
			if remaining > 0 {
				rsLC4 = binreader.SkipBytes(rsLC4, remaining)
			}
			rsLC = rsLC4
		} else if cmd == LCMain() {
			rsLC4, ep := parseEntryPointCmd(rsLC3)
			if rsLC4.Err != "" {
				result.Error = rsLC4.Err
				fs.Close(handle)
				return result
			}
			result.Entry = ep
			consumed := rsLC4.Pos - cmdStartPos
			remaining := int64(cmdSize) - consumed
			if remaining > 0 {
				rsLC4 = binreader.SkipBytes(rsLC4, remaining)
			}
			rsLC = rsLC4
		} else if cmd == LCUUID {
			rsLC4, ui := parseUUIDCmd(rsLC3)
			if rsLC4.Err != "" {
				result.Error = rsLC4.Err
				fs.Close(handle)
				return result
			}
			result.UUID = ui
			consumed := rsLC4.Pos - cmdStartPos
			remaining := int64(cmdSize) - consumed
			if remaining > 0 {
				rsLC4 = binreader.SkipBytes(rsLC4, remaining)
			}
			rsLC = rsLC4
		} else {
			// Skip unknown load command
			skipAmount := int64(cmdSize) - 8 // already read cmd + cmdSize (8 bytes)
			if skipAmount > 0 {
				rsLC = binreader.SkipBytes(rsLC3, skipAmount)
			} else {
				rsLC = rsLC3
			}
		}
		ci = ci + 1
	}

	// Resolve symbols
	if result.HasSymtab {
		result.Symbols = resolveSymbols(handle, result.Symtab)
	}

	return result
}

// CloseFile closes the file handle of a parsed Mach-O file
func CloseFile(f MachOFile) {
	if f.Handle > 0 {
		fs.Close(f.Handle)
	}
}
