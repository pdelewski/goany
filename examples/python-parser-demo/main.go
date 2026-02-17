package main

import (
	"fmt"
	"libs/pyparser"
)

func main() {
	// Test 1: Simple function definition
	code1 := `def add(a, b):
    return a + b`

	fmt.Println("=== Test 1: Function Definition ===")
	ast1 := pyparser.Parse(code1)
	fmt.Println(pyparser.PrintAST(ast1))

	// Test 2: If statement with else
	code2 := `if x > 0:
    y = x
else:
    y = 0`

	fmt.Println("=== Test 2: If Statement ===")
	ast2 := pyparser.Parse(code2)
	fmt.Println(pyparser.PrintAST(ast2))

	// Test 3: For loop with range
	code3 := `for i in range(10):
    x = x + i`

	fmt.Println("=== Test 3: For Loop ===")
	ast3 := pyparser.Parse(code3)
	fmt.Println(pyparser.PrintAST(ast3))

	// Test 4: While loop
	code4 := `while n > 0:
    n = n - 1`

	fmt.Println("=== Test 4: While Loop ===")
	ast4 := pyparser.Parse(code4)
	fmt.Println(pyparser.PrintAST(ast4))

	// Test 5: List operations
	code5 := `items = [1, 2, 3]
x = items[0]`

	fmt.Println("=== Test 5: List Operations ===")
	ast5 := pyparser.Parse(code5)
	fmt.Println(pyparser.PrintAST(ast5))

	// Test 6: Arithmetic expression
	code6 := `result = (a + b) * c - d / e`

	fmt.Println("=== Test 6: Arithmetic Expression ===")
	ast6 := pyparser.Parse(code6)
	fmt.Println(pyparser.PrintAST(ast6))

	// Test 7: Comparison operators
	code7 := `if x == 10:
    pass`

	fmt.Println("=== Test 7: Comparison ===")
	ast7 := pyparser.Parse(code7)
	fmt.Println(pyparser.PrintAST(ast7))

	// Test 8: Logical operators
	code8 := `if x > 0 and y < 10:
    z = 1`

	fmt.Println("=== Test 8: Logical Operators ===")
	ast8 := pyparser.Parse(code8)
	fmt.Println(pyparser.PrintAST(ast8))

	// Test 9: Function call with arguments
	code9 := `print(x, y, z)`

	fmt.Println("=== Test 9: Function Call ===")
	ast9 := pyparser.Parse(code9)
	fmt.Println(pyparser.PrintAST(ast9))

	// Test 10: Nested if with elif
	code10 := `if x > 0:
    y = 1
elif x < 0:
    y = -1
else:
    y = 0`

	fmt.Println("=== Test 10: If-Elif-Else ===")
	ast10 := pyparser.Parse(code10)
	fmt.Println(pyparser.PrintAST(ast10))

	// Test 11: Dict literal
	code11 := `data = {'name': 'John', 'age': 30}`

	fmt.Println("=== Test 11: Dict Literal ===")
	ast11 := pyparser.Parse(code11)
	fmt.Println(pyparser.PrintAST(ast11))

	// Test 12: Import statements
	code12 := `import os
from math import sqrt, pow`

	fmt.Println("=== Test 12: Import Statements ===")
	ast12 := pyparser.Parse(code12)
	fmt.Println(pyparser.PrintAST(ast12))

	// Test 13: Lambda expression
	code13 := `f = lambda x, y: x + y`

	fmt.Println("=== Test 13: Lambda Expression ===")
	ast13 := pyparser.Parse(code13)
	fmt.Println(pyparser.PrintAST(ast13))

	// Test 14: *args and **kwargs
	code14 := `def func(*args, **kwargs):
    pass`

	fmt.Println("=== Test 14: *args/**kwargs ===")
	ast14 := pyparser.Parse(code14)
	fmt.Println(pyparser.PrintAST(ast14))

	// Test 15: Decorator
	code15 := `@decorator
def func():
    pass`

	fmt.Println("=== Test 15: Decorator ===")
	ast15 := pyparser.Parse(code15)
	fmt.Println(pyparser.PrintAST(ast15))

	fmt.Println("=== All Tests Completed ===")
}
