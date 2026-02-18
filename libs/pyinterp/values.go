package pyinterp

import "libs/pyparser"

// Value type constants
const ValueNone int = 0
const ValueBool int = 1
const ValueInt int = 2
const ValueFloat int = 3
const ValueString int = 4
const ValueList int = 5
const ValueDict int = 6
const ValueFunc int = 7
const ValueBuiltin int = 8
const ValueRange int = 9

// Control flow constants
const ControlNone int = 0
const ControlReturn int = 1
const ControlBreak int = 2
const ControlContinue int = 3

// Value represents a Python value at runtime
type Value struct {
	Type        int
	BoolVal     bool
	IntVal      int
	FloatVal    float64
	StrVal      string
	ListVal     []Value
	DictKeys    []Value
	DictVals    []Value
	FuncName    string
	FuncParams  []string
	FuncBody    []pyparser.Node
	FuncEnv     int
	BuiltinName string
	RangeStart  int
	RangeEnd    int
	RangeStep   int
}

// Result wraps a Value with control flow information
type Result struct {
	Val     Value
	Control int
}

// NewNone creates a None value
func NewNone() Value {
	return Value{Type: ValueNone}
}

// NewBool creates a boolean value
func NewBool(b bool) Value {
	return Value{Type: ValueBool, BoolVal: b}
}

// NewInt creates an integer value
func NewInt(i int) Value {
	return Value{Type: ValueInt, IntVal: i}
}

// NewFloat creates a float value
func NewFloat(f float64) Value {
	return Value{Type: ValueFloat, FloatVal: f}
}

// NewString creates a string value
func NewString(s string) Value {
	return Value{Type: ValueString, StrVal: s}
}

// NewList creates a list value
func NewList(items []Value) Value {
	return Value{Type: ValueList, ListVal: items}
}

// NewEmptyList creates an empty list value
func NewEmptyList() Value {
	var items []Value
	return Value{Type: ValueList, ListVal: items}
}

// NewDict creates a dict value
func NewDict(keys []Value, vals []Value) Value {
	return Value{Type: ValueDict, DictKeys: keys, DictVals: vals}
}

// NewFunc creates a function value
func NewFunc(name string, paramList []string, body []pyparser.Node, envIdx int) Value {
	return Value{
		Type:       ValueFunc,
		FuncName:   name,
		FuncParams: paramList,
		FuncBody:   body,
		FuncEnv:    envIdx,
	}
}

// NewBuiltin creates a builtin function value
func NewBuiltin(name string) Value {
	return Value{Type: ValueBuiltin, BuiltinName: name}
}

// NewRange creates a range value
func NewRange(start int, end int, step int) Value {
	return Value{
		Type:       ValueRange,
		RangeStart: start,
		RangeEnd:   end,
		RangeStep:  step,
	}
}

// NewResult creates a normal result (no control flow)
func NewResult(val Value) Result {
	return Result{Val: val, Control: ControlNone}
}

// NewReturnResult creates a return control flow result
func NewReturnResult(val Value) Result {
	return Result{Val: val, Control: ControlReturn}
}

// NewBreakResult creates a break control flow result
func NewBreakResult() Result {
	return Result{Val: NewNone(), Control: ControlBreak}
}

// NewContinueResult creates a continue control flow result
func NewContinueResult() Result {
	return Result{Val: NewNone(), Control: ControlContinue}
}

// ValueTypeName returns the type name as a string
func ValueTypeName(t int) string {
	if t == ValueNone {
		return "NoneType"
	}
	if t == ValueBool {
		return "bool"
	}
	if t == ValueInt {
		return "int"
	}
	if t == ValueFloat {
		return "float"
	}
	if t == ValueString {
		return "str"
	}
	if t == ValueList {
		return "list"
	}
	if t == ValueDict {
		return "dict"
	}
	if t == ValueFunc {
		return "function"
	}
	if t == ValueBuiltin {
		return "builtin_function"
	}
	if t == ValueRange {
		return "range"
	}
	return "unknown"
}

// charToString converts a character code to a single-character string
// Uses lookup table for goany compatibility (no string(byte) conversion)
func charToString(ch int) string {
	if ch == 9 {
		return "\t"
	} else if ch == 10 {
		return "\n"
	} else if ch == 13 {
		return "\r"
	} else if ch == 32 {
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
	} else if ch == 92 {
		return "\\"
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

// substring extracts a substring from start to end (exclusive)
func substring(s string, start int, end int) string {
	result := ""
	for i := start; i < end && i < len(s); i++ {
		result = result + charToString(int(s[i]))
	}
	return result
}

// intToString converts an integer to string without fmt
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	negative := false
	if n < 0 {
		negative = true
		n = -n
	}
	result := ""
	for n > 0 {
		digit := n % 10
		result = charToString(48+digit) + result // 48 is '0' in ASCII
		n = n / 10
	}
	if negative {
		result = "-" + result
	}
	return result
}

// floatToString converts a float to string (simple version)
func floatToString(f float64) string {
	if f == 0.0 {
		return "0.0"
	}
	negative := false
	if f < 0 {
		negative = true
		f = -f
	}
	// Get integer part
	intPart := int(f)
	fracPart := f - float64(intPart)

	result := intToString(intPart)
	result = result + "."

	// Get fractional part (6 decimal places)
	for i := 0; i < 6; i++ {
		fracPart = fracPart * 10
		digit := int(fracPart)
		result = result + charToString(48+digit) // 48 is '0' in ASCII
		fracPart = fracPart - float64(digit)
	}

	// Remove trailing zeros (48 is '0', 46 is '.')
	for len(result) > 0 && int(result[len(result)-1]) == 48 {
		result = substring(result, 0, len(result)-1)
	}
	if len(result) > 0 && int(result[len(result)-1]) == 46 {
		result = result + "0"
	}

	if negative {
		result = "-" + result
	}
	return result
}

// ValueToString converts a value to its string representation
func ValueToString(v Value) string {
	if v.Type == ValueNone {
		return "None"
	}
	if v.Type == ValueBool {
		if v.BoolVal {
			return "True"
		}
		return "False"
	}
	if v.Type == ValueInt {
		return intToString(v.IntVal)
	}
	if v.Type == ValueFloat {
		return floatToString(v.FloatVal)
	}
	if v.Type == ValueString {
		// Return copy of string (for Rust compatibility)
		return "" + v.StrVal
	}
	if v.Type == ValueList {
		result := "["
		for i := 0; i < len(v.ListVal); i++ {
			if i > 0 {
				result = result + ", "
			}
			item := v.ListVal[i]
			if item.Type == ValueString {
				result = result + "'" + ValueToString(item) + "'"
			} else {
				result = result + ValueToString(item)
			}
		}
		result = result + "]"
		return result
	}
	if v.Type == ValueDict {
		result := "{"
		for i := 0; i < len(v.DictKeys); i++ {
			if i > 0 {
				result = result + ", "
			}
			key := v.DictKeys[i]
			val := v.DictVals[i]
			if key.Type == ValueString {
				result = result + "'" + ValueToString(key) + "'"
			} else {
				result = result + ValueToString(key)
			}
			result = result + ": "
			if val.Type == ValueString {
				result = result + "'" + ValueToString(val) + "'"
			} else {
				result = result + ValueToString(val)
			}
		}
		result = result + "}"
		return result
	}
	if v.Type == ValueFunc {
		return "<function " + v.FuncName + ">"
	}
	if v.Type == ValueBuiltin {
		return "<built-in function " + v.BuiltinName + ">"
	}
	if v.Type == ValueRange {
		return "range(" + intToString(v.RangeStart) + ", " + intToString(v.RangeEnd) + ")"
	}
	return "<unknown>"
}

// ValueToBool returns the truthiness of a value
func ValueToBool(v Value) bool {
	if v.Type == ValueNone {
		return false
	}
	if v.Type == ValueBool {
		return v.BoolVal
	}
	if v.Type == ValueInt {
		return v.IntVal != 0
	}
	if v.Type == ValueFloat {
		return v.FloatVal != 0.0
	}
	if v.Type == ValueString {
		return len(v.StrVal) > 0
	}
	if v.Type == ValueList {
		return len(v.ListVal) > 0
	}
	if v.Type == ValueDict {
		return len(v.DictKeys) > 0
	}
	return true
}

// ValueEquals checks if two values are equal
func ValueEquals(a Value, b Value) bool {
	if a.Type != b.Type {
		// Special case: int and float comparison
		if a.Type == ValueInt && b.Type == ValueFloat {
			return float64(a.IntVal) == b.FloatVal
		}
		if a.Type == ValueFloat && b.Type == ValueInt {
			return a.FloatVal == float64(b.IntVal)
		}
		return false
	}
	if a.Type == ValueNone {
		return true
	}
	if a.Type == ValueBool {
		return a.BoolVal == b.BoolVal
	}
	if a.Type == ValueInt {
		return a.IntVal == b.IntVal
	}
	if a.Type == ValueFloat {
		return a.FloatVal == b.FloatVal
	}
	if a.Type == ValueString {
		return a.StrVal == b.StrVal
	}
	if a.Type == ValueList {
		if len(a.ListVal) != len(b.ListVal) {
			return false
		}
		for i := 0; i < len(a.ListVal); i++ {
			if !ValueEquals(a.ListVal[i], b.ListVal[i]) {
				return false
			}
		}
		return true
	}
	return false
}

// ValueCompare compares two values, returns -1, 0, or 1
func ValueCompare(a Value, b Value) int {
	// Handle numeric comparisons
	if (a.Type == ValueInt || a.Type == ValueFloat) && (b.Type == ValueInt || b.Type == ValueFloat) {
		var aFloat float64
		var bFloat float64
		if a.Type == ValueInt {
			aFloat = float64(a.IntVal)
		} else {
			aFloat = a.FloatVal
		}
		if b.Type == ValueInt {
			bFloat = float64(b.IntVal)
		} else {
			bFloat = b.FloatVal
		}
		if aFloat < bFloat {
			return -1
		}
		if aFloat > bFloat {
			return 1
		}
		return 0
	}
	// String comparison
	if a.Type == ValueString && b.Type == ValueString {
		return compareStrings(a.StrVal, b.StrVal)
	}
	return 0
}

// compareStrings compares two strings lexicographically
func compareStrings(s1 string, s2 string) int {
	len1 := len(s1)
	len2 := len(s2)
	minLen := len1
	if len2 < minLen {
		minLen = len2
	}
	i := 0
	for i < minLen {
		c1 := int(s1[i])
		c2 := int(s2[i])
		if c1 < c2 {
			return -1
		}
		if c1 > c2 {
			return 1
		}
		i = i + 1
	}
	if len1 < len2 {
		return -1
	}
	if len1 > len2 {
		return 1
	}
	return 0
}

// ValueLen returns the length of a value (string, list, dict)
func ValueLen(v Value) int {
	if v.Type == ValueString {
		return len(v.StrVal)
	}
	if v.Type == ValueList {
		return len(v.ListVal)
	}
	if v.Type == ValueDict {
		return len(v.DictKeys)
	}
	return 0
}

// ListAppend appends an item to a list and returns a new list
func ListAppend(list Value, item Value) Value {
	if list.Type != ValueList {
		return list
	}
	var newItems []Value
	i := 0
	for i < len(list.ListVal) {
		newItems = append(newItems, list.ListVal[i])
		i = i + 1
	}
	newItems = append(newItems, item)
	return NewList(newItems)
}

// ListGet gets an item from a list by index
func ListGet(list Value, index int) Value {
	if list.Type != ValueList {
		return NewNone()
	}
	if index < 0 {
		index = len(list.ListVal) + index
	}
	if index < 0 || index >= len(list.ListVal) {
		return NewNone()
	}
	return list.ListVal[index]
}

// ListSet sets an item in a list by index and returns a new list
func ListSet(list Value, index int, val Value) Value {
	if list.Type != ValueList {
		return list
	}
	if index < 0 {
		index = len(list.ListVal) + index
	}
	if index < 0 || index >= len(list.ListVal) {
		return list
	}
	var newItems []Value
	i := 0
	for i < len(list.ListVal) {
		if i == index {
			newItems = append(newItems, val)
		} else {
			newItems = append(newItems, list.ListVal[i])
		}
		i = i + 1
	}
	return NewList(newItems)
}

// DictGet gets a value from a dict by key
func DictGet(dict Value, key Value) Value {
	if dict.Type != ValueDict {
		return NewNone()
	}
	for i := 0; i < len(dict.DictKeys); i++ {
		if ValueEquals(dict.DictKeys[i], key) {
			return dict.DictVals[i]
		}
	}
	return NewNone()
}

// DictSet sets a value in a dict and returns a new dict
func DictSet(dict Value, key Value, val Value) Value {
	if dict.Type != ValueDict {
		return dict
	}
	// Check if key exists
	keyIndex := -1
	i := 0
	for i < len(dict.DictKeys) {
		if ValueEquals(dict.DictKeys[i], key) {
			keyIndex = i
		}
		i = i + 1
	}
	if keyIndex >= 0 {
		// Update existing key
		var updKeys []Value
		var updVals []Value
		j := 0
		for j < len(dict.DictKeys) {
			updKeys = append(updKeys, dict.DictKeys[j])
			if j == keyIndex {
				updVals = append(updVals, val)
			} else {
				updVals = append(updVals, dict.DictVals[j])
			}
			j = j + 1
		}
		return NewDict(updKeys, updVals)
	}
	// Add new key
	var addKeys []Value
	var addVals []Value
	k := 0
	for k < len(dict.DictKeys) {
		addKeys = append(addKeys, dict.DictKeys[k])
		addVals = append(addVals, dict.DictVals[k])
		k = k + 1
	}
	addKeys = append(addKeys, key)
	addVals = append(addVals, val)
	return NewDict(addKeys, addVals)
}
