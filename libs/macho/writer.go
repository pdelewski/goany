package macho

import (
	"runtime/fs"
)

// SectionFlagCode returns S_ATTR_PURE_INSTRUCTIONS | S_ATTR_SOME_INSTRUCTIONS
// Computed at runtime to avoid 32-bit int overflow in C++ transpilation
func SectionFlagCode() int {
	return (0x80 << 24) | (0x04 << 8)
}

// writeUint16LE appends a 16-bit little-endian value to buf
func writeUint16LE(buf []byte, val int) []byte {
	buf = append(buf, byte(val&0xFF))
	buf = append(buf, byte((val>>8)&0xFF))
	return buf
}

// writeUint32LE appends a 32-bit little-endian value to buf
func writeUint32LE(buf []byte, val int) []byte {
	buf = append(buf, byte(val&0xFF))
	buf = append(buf, byte((val>>8)&0xFF))
	buf = append(buf, byte((val>>16)&0xFF))
	buf = append(buf, byte((val>>24)&0xFF))
	return buf
}

// writeUint64LE appends a 64-bit little-endian value to buf
func writeUint64LE(buf []byte, val int64) []byte {
	buf = append(buf, byte(val&0xFF))
	buf = append(buf, byte((val>>8)&0xFF))
	buf = append(buf, byte((val>>16)&0xFF))
	buf = append(buf, byte((val>>24)&0xFF))
	buf = append(buf, byte((val>>32)&0xFF))
	buf = append(buf, byte((val>>40)&0xFF))
	buf = append(buf, byte((val>>48)&0xFF))
	buf = append(buf, byte((val>>56)&0xFF))
	return buf
}

// writeFixedString appends a string padded with zeros to exactly n bytes
func writeFixedString(buf []byte, s string, n int) []byte {
	i := 0
	for i < n {
		if i < len(s) {
			buf = append(buf, byte(s[i]))
		} else {
			buf = append(buf, byte(0))
		}
		i = i + 1
	}
	return buf
}

// writeZeros appends n zero bytes to buf
func writeZeros(buf []byte, n int) []byte {
	i := 0
	for i < n {
		buf = append(buf, byte(0))
		i = i + 1
	}
	return buf
}

// writeByte appends a single byte to buf
func writeByte(buf []byte, val int) []byte {
	buf = append(buf, byte(val&0xFF))
	return buf
}

// BuildArm64Object builds a complete Mach-O .o (MH_OBJECT) for arm64
// from raw machine code and an exported symbol name.
func BuildArm64Object(code []byte, symbolName string) []byte {
	codeSize := len(code)
	headerSize := 32
	segCmdSize := 72 + 80 // segment header + 1 section
	symtabCmdSize := 24
	loadCmdsSize := segCmdSize + symtabCmdSize
	codeOffset := headerSize + loadCmdsSize
	symtabOffset := codeOffset + codeSize
	strtabOffset := symtabOffset + 16
	strtabSize := 1 + len(symbolName) + 1

	buf := []byte{}

	// === Mach-O Header (32 bytes) ===
	buf = writeUint32LE(buf, MachOMagic64)
	buf = writeUint32LE(buf, CPUTypeARM64)
	buf = writeUint32LE(buf, 0) // cpusubtype
	buf = writeUint32LE(buf, MHObject)
	buf = writeUint32LE(buf, 2) // ncmds
	buf = writeUint32LE(buf, loadCmdsSize)
	buf = writeUint32LE(buf, MHSubsectionsViaSymbols)
	buf = writeUint32LE(buf, 0) // reserved

	// === LC_SEGMENT_64 (152 bytes = 72 + 80) ===
	buf = writeUint32LE(buf, LCSegment64)
	buf = writeUint32LE(buf, segCmdSize)
	buf = writeFixedString(buf, "", 16)           // segname (empty for .o)
	buf = writeUint64LE(buf, 0)                   // vmaddr
	buf = writeUint64LE(buf, int64(codeSize))     // vmsize
	buf = writeUint64LE(buf, int64(codeOffset))   // fileoff
	buf = writeUint64LE(buf, int64(codeSize))     // filesize
	buf = writeUint32LE(buf, 7)                   // maxprot (rwx)
	buf = writeUint32LE(buf, 7)                   // initprot (rwx)
	buf = writeUint32LE(buf, 1)                   // nsects
	buf = writeUint32LE(buf, 0)                   // flags

	// Section: __text in __TEXT (80 bytes)
	buf = writeFixedString(buf, "__text", 16)     // sectname
	buf = writeFixedString(buf, "__TEXT", 16)     // segname
	buf = writeUint64LE(buf, 0)                   // addr
	buf = writeUint64LE(buf, int64(codeSize))     // size
	buf = writeUint32LE(buf, codeOffset)          // offset
	buf = writeUint32LE(buf, 2)                   // align (2^2 = 4 bytes)
	buf = writeUint32LE(buf, 0)                   // reloff
	buf = writeUint32LE(buf, 0)                   // nreloc
	buf = writeUint32LE(buf, SectionFlagCode())   // flags
	buf = writeUint32LE(buf, 0)                   // reserved1
	buf = writeUint32LE(buf, 0)                   // reserved2
	buf = writeUint32LE(buf, 0)                   // reserved3

	// === LC_SYMTAB (24 bytes) ===
	buf = writeUint32LE(buf, LCSymtab)
	buf = writeUint32LE(buf, symtabCmdSize)
	buf = writeUint32LE(buf, symtabOffset)
	buf = writeUint32LE(buf, 1) // nsyms
	buf = writeUint32LE(buf, strtabOffset)
	buf = writeUint32LE(buf, strtabSize)

	// === Machine code ===
	i := 0
	for i < codeSize {
		buf = append(buf, code[i])
		i = i + 1
	}

	// === Symbol table (nlist_64: 16 bytes) ===
	buf = writeUint32LE(buf, 1)          // n_strx (index 1 in string table)
	buf = writeByte(buf, NlistExtSect)   // n_type (N_SECT | N_EXT)
	buf = writeByte(buf, 1)              // n_sect (section 1)
	buf = writeUint16LE(buf, 0)          // n_desc
	buf = writeUint64LE(buf, 0)          // n_value

	// === String table ===
	buf = writeByte(buf, 0) // initial null byte (index 0)
	j := 0
	for j < len(symbolName) {
		buf = append(buf, byte(symbolName[j]))
		j = j + 1
	}
	buf = writeByte(buf, 0) // terminating null byte

	return buf
}

// WriteObjectFile builds a Mach-O .o and writes it to disk.
// Returns an error string (empty on success).
func WriteObjectFile(path string, code []byte, symbolName string) string {
	data := BuildArm64Object(code, symbolName)
	handle, err := fs.Create(path)
	if err != "" {
		return err
	}
	_, writeErr := fs.Write(handle, data)
	if writeErr != "" {
		fs.Close(handle)
		return writeErr
	}
	closeErr := fs.Close(handle)
	return closeErr
}
