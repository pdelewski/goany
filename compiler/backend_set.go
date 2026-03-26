package compiler

// BackendSet is a bitmask representing which transpiler backends are enabled.
// The canonicalize pass uses this to decide which transforms to apply:
// transforms that have 1:1 equivalents in a target language are skipped
// when only that backend is enabled.
type BackendSet uint8

const (
	BackendCpp    BackendSet = 1 << iota // C++ backend
	BackendCSharp                        // C# backend
	BackendRust                          // Rust backend
	BackendJs                            // JavaScript backend
	BackendJava                          // Java backend
	BackendGo                            // Go backend
	BackendAll    = BackendCpp | BackendCSharp | BackendRust | BackendJs | BackendJava | BackendGo
)

// Has returns true if the set includes the given backend(s).
func (bs BackendSet) Has(b BackendSet) bool {
	return bs&b != 0
}
