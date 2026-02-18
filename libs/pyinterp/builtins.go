package pyinterp

import "fmt"

// IsBuiltin checks if a name is a builtin function
func IsBuiltin(name string) bool {
	if name == "print" {
		return true
	}
	if name == "len" {
		return true
	}
	if name == "range" {
		return true
	}
	if name == "str" {
		return true
	}
	if name == "int" {
		return true
	}
	if name == "float" {
		return true
	}
	if name == "bool" {
		return true
	}
	if name == "abs" {
		return true
	}
	if name == "min" {
		return true
	}
	if name == "max" {
		return true
	}
	if name == "type" {
		return true
	}
	if name == "append" {
		return true
	}
	if name == "input" {
		return true
	}
	return false
}

// CallBuiltin calls a builtin function with the given arguments
func CallBuiltin(name string, args []Value) Value {
	if name == "print" {
		return builtinPrint(args)
	}
	if name == "len" {
		return builtinLen(args)
	}
	if name == "range" {
		return builtinRange(args)
	}
	if name == "str" {
		return builtinStr(args)
	}
	if name == "int" {
		return builtinInt(args)
	}
	if name == "float" {
		return builtinFloat(args)
	}
	if name == "bool" {
		return builtinBool(args)
	}
	if name == "abs" {
		return builtinAbs(args)
	}
	if name == "min" {
		return builtinMin(args)
	}
	if name == "max" {
		return builtinMax(args)
	}
	if name == "type" {
		return builtinType(args)
	}
	if name == "append" {
		return builtinAppend(args)
	}
	if name == "input" {
		return builtinInput(args)
	}
	return NewNone()
}

// builtinPrint implements print(*args)
func builtinPrint(args []Value) Value {
	output := ""
	for i := 0; i < len(args); i++ {
		if i > 0 {
			output = output + " "
		}
		output = output + ValueToString(args[i])
	}
	fmt.Println(output)
	return NewNone()
}

// builtinLen implements len(obj)
func builtinLen(args []Value) Value {
	if len(args) < 1 {
		return NewInt(0)
	}
	return NewInt(ValueLen(args[0]))
}

// builtinRange implements range(stop) or range(start, stop) or range(start, stop, step)
func builtinRange(args []Value) Value {
	start := 0
	end := 0
	step := 1

	if len(args) == 1 {
		if args[0].Type == ValueInt {
			end = args[0].IntVal
		}
	} else if len(args) == 2 {
		if args[0].Type == ValueInt {
			start = args[0].IntVal
		}
		if args[1].Type == ValueInt {
			end = args[1].IntVal
		}
	} else if len(args) >= 3 {
		if args[0].Type == ValueInt {
			start = args[0].IntVal
		}
		if args[1].Type == ValueInt {
			end = args[1].IntVal
		}
		if args[2].Type == ValueInt {
			step = args[2].IntVal
		}
	}

	if step == 0 {
		step = 1
	}

	return NewRange(start, end, step)
}

// builtinStr implements str(obj)
func builtinStr(args []Value) Value {
	if len(args) < 1 {
		return NewString("")
	}
	return NewString(ValueToString(args[0]))
}

// builtinInt implements int(obj)
func builtinInt(args []Value) Value {
	if len(args) < 1 {
		return NewInt(0)
	}
	v := args[0]
	if v.Type == ValueInt {
		return v
	}
	if v.Type == ValueFloat {
		return NewInt(int(v.FloatVal))
	}
	if v.Type == ValueBool {
		if v.BoolVal {
			return NewInt(1)
		}
		return NewInt(0)
	}
	if v.Type == ValueString {
		// Parse string to int
		result := 0
		negative := false
		s := v.StrVal
		startIdx := 0
		if len(s) > 0 && s[0] == '-' {
			negative = true
			startIdx = 1
		}
		for i := startIdx; i < len(s); i++ {
			ch := s[i]
			if ch >= '0' && ch <= '9' {
				result = result*10 + int(ch-'0')
			} else {
				break
			}
		}
		if negative {
			result = -result
		}
		return NewInt(result)
	}
	return NewInt(0)
}

// builtinFloat implements float(obj)
func builtinFloat(args []Value) Value {
	if len(args) < 1 {
		return NewFloat(0.0)
	}
	v := args[0]
	if v.Type == ValueFloat {
		return v
	}
	if v.Type == ValueInt {
		return NewFloat(float64(v.IntVal))
	}
	if v.Type == ValueBool {
		if v.BoolVal {
			return NewFloat(1.0)
		}
		return NewFloat(0.0)
	}
	if v.Type == ValueString {
		// Simple string to float parsing
		result := 0.0
		negative := false
		s := v.StrVal
		startIdx := 0
		if len(s) > 0 && s[0] == '-' {
			negative = true
			startIdx = 1
		}
		// Parse integer part
		i := startIdx
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			result = result*10 + float64(s[i]-'0')
			i++
		}
		// Parse decimal part
		if i < len(s) && s[i] == '.' {
			i++
			factor := 0.1
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				result = result + float64(s[i]-'0')*factor
				factor = factor / 10
				i++
			}
		}
		if negative {
			result = -result
		}
		return NewFloat(result)
	}
	return NewFloat(0.0)
}

// builtinBool implements bool(obj)
func builtinBool(args []Value) Value {
	if len(args) < 1 {
		return NewBool(false)
	}
	return NewBool(ValueToBool(args[0]))
}

// builtinAbs implements abs(num)
func builtinAbs(args []Value) Value {
	if len(args) < 1 {
		return NewInt(0)
	}
	v := args[0]
	if v.Type == ValueInt {
		if v.IntVal < 0 {
			return NewInt(-v.IntVal)
		}
		return v
	}
	if v.Type == ValueFloat {
		if v.FloatVal < 0 {
			return NewFloat(-v.FloatVal)
		}
		return v
	}
	return NewInt(0)
}

// builtinMin implements min(a, b, ...)
func builtinMin(args []Value) Value {
	if len(args) < 1 {
		return NewNone()
	}
	// If single list argument, find min in list
	if len(args) == 1 && args[0].Type == ValueList {
		list := args[0].ListVal
		if len(list) == 0 {
			return NewNone()
		}
		listMin := list[0]
		i := 1
		for i < len(list) {
			if ValueCompare(list[i], listMin) < 0 {
				listMin = list[i]
			}
			i = i + 1
		}
		return listMin
	}
	// Multiple arguments
	argsMin := args[0]
	j := 1
	for j < len(args) {
		if ValueCompare(args[j], argsMin) < 0 {
			argsMin = args[j]
		}
		j = j + 1
	}
	return argsMin
}

// builtinMax implements max(a, b, ...)
func builtinMax(args []Value) Value {
	if len(args) < 1 {
		return NewNone()
	}
	// If single list argument, find max in list
	if len(args) == 1 && args[0].Type == ValueList {
		list := args[0].ListVal
		if len(list) == 0 {
			return NewNone()
		}
		listMax := list[0]
		i := 1
		for i < len(list) {
			if ValueCompare(list[i], listMax) > 0 {
				listMax = list[i]
			}
			i = i + 1
		}
		return listMax
	}
	// Multiple arguments
	argsMax := args[0]
	j := 1
	for j < len(args) {
		if ValueCompare(args[j], argsMax) > 0 {
			argsMax = args[j]
		}
		j = j + 1
	}
	return argsMax
}

// builtinType implements type(obj)
func builtinType(args []Value) Value {
	if len(args) < 1 {
		return NewString("NoneType")
	}
	return NewString(ValueTypeName(args[0].Type))
}

// builtinAppend implements append(list, item)
func builtinAppend(args []Value) Value {
	if len(args) < 2 {
		if len(args) == 1 {
			return args[0]
		}
		return NewEmptyList()
	}
	return ListAppend(args[0], args[1])
}

// builtinInput implements input(prompt)
// Note: Returns empty string as stdin reading is not supported in cross-compiled targets
func builtinInput(args []Value) Value {
	// Print prompt if provided
	if len(args) > 0 {
		fmt.Print(ValueToString(args[0]))
	}
	// Return empty string - stdin not supported in goany targets
	return NewString("")
}

// RangeToList converts a range to a list of integers
func RangeToList(r Value) []Value {
	var result []Value
	if r.Type != ValueRange {
		return result
	}
	start := r.RangeStart
	end := r.RangeEnd
	step := r.RangeStep

	if step > 0 {
		i := start
		for i < end {
			result = append(result, NewInt(i))
			i = i + step
		}
	} else if step < 0 {
		i := start
		for i > end {
			result = append(result, NewInt(i))
			i = i + step
		}
	}
	return result
}
