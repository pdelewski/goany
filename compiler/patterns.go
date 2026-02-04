//go:generate go run ../genrules/main.go -collect -root .. -output-go patterns_data.go

// patterns.go provides pattern matching infrastructure for semantic checking.
// Patterns are collected from tests/examples using genrules and used to validate
// that user code only uses constructs that have been tested.
package compiler

import (
	"encoding/json"
	"fmt"
	"go/types"
	"os"
	"sort"
	"strings"
)

// Pattern represents a precise construct signature
type Pattern struct {
	Kind    string            `json:"kind"`    // AST node type (e.g., "ForStmt", "RangeStmt")
	Attrs   map[string]string `json:"attrs"`   // Attributes that define this variant
	Example string            `json:"example"` // Example location where this was seen
}

// Key returns a unique string key for this pattern (includes all attributes)
func (p Pattern) Key() string {
	var parts []string
	parts = append(parts, p.Kind)

	// Sort attribute keys for consistent ordering
	var keys []string
	for k := range p.Attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, p.Attrs[k]))
	}
	return strings.Join(parts, ":")
}

// TypeSignificantAttrs defines which STRUCTURAL attributes matter for each construct kind.
// Attributes not listed here will be stripped during normalized matching.
// Type-related checks (map key types, range over maps, etc.) are handled by the semantic checker.
// IMPORTANT: This is the single source of truth - genrules imports this to ensure consistency.
var TypeSignificantAttrs = map[string]map[string]bool{
	// RangeStmt: structural attrs only (type checks in sema.go)
	"RangeStmt": {
		"tok":   true,
		"key":   true,
		"value": true,
		// collection_type moved to sema - checks range over maps
	},
	// MapType: structural only - type checks (key_type, nested) handled by sema.go
	"MapType": {
		// key_type, nested, nested_ast moved to sema
	},
	// IndexExpr: structural only - all collection types (slice, map, array) are valid
	"IndexExpr": {
		// collection_type removed - slices, maps, arrays all supported
	},
	// AssignStmt: structural attrs matter, but not rhs_type (except comma_ok)
	"AssignStmt": {
		"tok":       true,
		"lhs_count": true,
		"rhs_count": true,
		"comma_ok":  true,
		// rhs_type intentionally omitted
	},
	// BinaryExpr: operator matters, operand types don't
	"BinaryExpr": {
		"op": true,
		// left_type, right_type intentionally omitted
	},
	// UnaryExpr: operator matters, operand type doesn't
	"UnaryExpr": {
		"op": true,
		// operand_type intentionally omitted
	},
	// CallExpr: function type and builtin status matter, but not specific names
	"CallExpr": {
		"func_type": true,
		"builtin":   true,
		// func_name, method, receiver intentionally omitted - don't require patterns for every function/method name
		// arg_count, make_type, receiver_type, variadic intentionally omitted for flexibility
	},
	// IncDecStmt: tok matters, operand_type doesn't
	"IncDecStmt": {
		"tok": true,
		// operand_type intentionally omitted
	},
	// ForStmt: structural attrs matter
	"ForStmt": {
		"has_init":  true,
		"has_cond":  true,
		"has_post":  true,
		"init_type": true,
		"post_type": true,
	},
	// IfStmt: structural attrs matter
	"IfStmt": {
		"has_init":  true,
		"has_else":  true,
		"else_type": true,
	},
	// SwitchStmt: structural attrs matter
	"SwitchStmt": {
		"has_init":    true,
		"has_tag":     true,
		"has_default": true,
		// case_count intentionally omitted - don't limit number of cases
	},
	// FuncDecl: structural attrs matter
	"FuncDecl": {
		"has_recv": true,
		"recv_ptr": true,
		// param_count, result_count intentionally omitted
	},
	// FuncLit: no structural attrs matter - any function literal is ok
	"FuncLit": {
		// param_count, result_count intentionally omitted
	},
	// ReturnStmt: allow any result count
	"ReturnStmt": {
		// result_count intentionally omitted
	},
	// BranchStmt: tok and label matter
	"BranchStmt": {
		"tok":       true,
		"has_label": true,
	},
	// CompositeLit: structural attrs only - map literal check moved to sema.go
	"CompositeLit": {
		"ast_type": true, // structural: AST node type (array_or_slice, map, named, etc.)
		"keyed":    true, // structural: whether using key-value syntax
		// type (runtime type category) moved to sema - checks map literals
		// elt_count intentionally omitted
	},
	// SliceExpr: structural attrs matter
	"SliceExpr": {
		"has_low":  true,
		"has_high": true,
		"has_max":  true,
		"slice3":   true,
		// collection_type intentionally omitted
	},
	// ArrayType: is_slice and elt_is_composite matter
	"ArrayType": {
		"is_slice":         true,
		"elt_is_composite": true, // structural: whether element is slice/array/map
		// elt_type intentionally omitted (specific type doesn't matter)
	},
	// TypeAssertExpr: type_switch matters
	"TypeAssertExpr": {
		"type_switch": true,
		// assert_type intentionally omitted
	},
	// These are always unsupported - keep all attrs to ensure no match
	"GoStmt":     {},
	"DeferStmt":  {},
	"SelectStmt": {},
	"SendStmt":   {},
	"ChanType":   {"dir": true},
	"Ellipsis":   {},
}

// NormalizedKey returns a key with only type-significant attributes.
// This allows matching constructs regardless of specific types used,
// except where types actually matter for correctness.
func (p Pattern) NormalizedKey() string {
	significant, hasConfig := TypeSignificantAttrs[p.Kind]

	var parts []string
	parts = append(parts, p.Kind)

	// Sort attribute keys for consistent ordering
	var keys []string
	for k := range p.Attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		// If no config for this kind, include all attrs (conservative)
		// If config exists, only include significant attrs
		if !hasConfig || significant[k] {
			parts = append(parts, fmt.Sprintf("%s=%s", k, p.Attrs[k]))
		}
	}
	return strings.Join(parts, ":")
}

// PatternDatabase stores collected patterns for checking
type PatternDatabase struct {
	Patterns           map[string]Pattern `json:"patterns"`
	normalizedPatterns map[string]bool    // built on load, not serialized
}

// LoadPatternDatabase loads a pattern database from a JSON file
func LoadPatternDatabase(filename string) (*PatternDatabase, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var db PatternDatabase
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, err
	}

	// Build normalized pattern index
	db.buildNormalizedIndex()

	return &db, nil
}

// GetBuiltinPatternDatabase returns the pattern database compiled from tests/examples
func GetBuiltinPatternDatabase() *PatternDatabase {
	db := &PatternDatabase{
		Patterns: GetBuiltinPatterns(),
	}
	db.buildNormalizedIndex()
	return db
}

func (db *PatternDatabase) buildNormalizedIndex() {
	db.normalizedPatterns = make(map[string]bool)
	for _, p := range db.Patterns {
		normalizedKey := p.NormalizedKey()
		db.normalizedPatterns[normalizedKey] = true
	}
}

// HasPattern checks if a pattern is supported using two-level matching.
// It uses normalized keys which strip type-specific attributes except
// where types actually matter (e.g., map key types, range collection types).
func (db *PatternDatabase) HasPattern(p Pattern) bool {
	normalizedKey := p.NormalizedKey()
	return db.normalizedPatterns[normalizedKey]
}

// typeCategory returns a category string for a type
func typeCategory(t types.Type) string {
	if t == nil {
		return "unknown"
	}

	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Name()
	case *types.Slice:
		return "slice"
	case *types.Array:
		return "array"
	case *types.Map:
		return "map"
	case *types.Struct:
		return "struct"
	case *types.Pointer:
		return "pointer"
	case *types.Interface:
		return "interface"
	case *types.Signature:
		return "func"
	case *types.Chan:
		return "chan"
	default:
		return "other"
	}
}
