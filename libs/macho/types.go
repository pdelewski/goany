package macho

// Mach-O magic numbers
const MachOMagic64 int = 0xCF | (0xFA << 8) | (0xED << 16) | (0xFE << 24)

// Fat/universal binary magic (big-endian 0xCAFEBABE read as LE uint32)
const FatMagic int = 0xCA | (0xFE << 8) | (0xBA << 16) | (0xBE << 24)

// CPU types (decimal to avoid hex > 0x7FFFFFFF issues)
const CPUTypeX86_64 int = 16777223  // 0x01000007
const CPUTypeARM64 int = 16777228   // 0x0100000C

// File types
const MHObject int = 1
const MHExecute int = 2
const MHDylib int = 6
const MHBundle int = 8
const MHDsym int = 10

// Load command types
const LCSymtab int = 2
const LCDysymtab int = 11
const LCLoadDylib int = 12
const LCSegment64 int = 25
const LCUUID int = 27
const LCBuildVersion int = 50

// VM protection flags
const VMProtRead int = 1
const VMProtWrite int = 2
const VMProtExecute int = 4

// MachOHeader holds the Mach-O file header
type MachOHeader struct {
	Magic      int
	CPUType    int
	CPUSubtype int
	FileType   int
	NCmds      int
	SizeOfCmds int
	Flags      int
	Reserved   int
}

// Section64 holds a Mach-O 64-bit section
type Section64 struct {
	SectName  string
	SegName   string
	Addr      int64
	Size      int64
	Offset    int
	Align     int
	RelOff    int
	NReloc    int
	Flags     int
	Reserved1 int
	Reserved2 int
	Reserved3 int
}

// Segment64 holds a Mach-O 64-bit segment
type Segment64 struct {
	Name     string
	VMAddr   int64
	VMSize   int64
	FileOff  int64
	FileSize int64
	MaxProt  int
	InitProt int
	NSects   int
	Flags    int
	Sections []Section64
}

// SymtabCommand holds symtab load command data
type SymtabCommand struct {
	SymOff  int
	NSyms   int
	StrOff  int
	StrSize int
}

// Symbol holds a resolved symbol
type Symbol struct {
	Name  string
	NType int
	NSect int
	NDesc int
	Value int64
}

// DylibInfo holds dynamic library information
type DylibInfo struct {
	Name          string
	Timestamp     int
	CurrentVer    int
	CompatVer     int
}

// EntryPoint holds the entry point information
type EntryPoint struct {
	EntryOff  int64
	StackSize int64
	Present   bool
}

// UUIDInfo holds the UUID information
type UUIDInfo struct {
	UUID    string
	Present bool
}

// LoadCommandInfo holds basic info about a load command
type LoadCommandInfo struct {
	Cmd     int
	CmdSize int
}

// MachOFile holds the complete parsed Mach-O file
type MachOFile struct {
	Header       MachOHeader
	Segments     []Segment64
	Symtab       SymtabCommand
	HasSymtab    bool
	Symbols      []Symbol
	Dylibs       []DylibInfo
	Entry        EntryPoint
	UUID         UUIDInfo
	LoadCommands []LoadCommandInfo
	Handle       int
	Error        string
}
