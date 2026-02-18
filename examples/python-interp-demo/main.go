package main

import (
	"fmt"
	"libs/pyinterp"
)

func test(name string, code string) {
	fmt.Println("=== " + name + " ===")
	fmt.Println("Code:")
	fmt.Println(code)
	fmt.Println("Output:")
	pyinterp.Eval(code)
	fmt.Println()
}

func main() {
	// Test 1: Variables and arithmetic
	test("Test 1: Variables and Arithmetic",
		`x = 10
y = 20
print(x + y)`)

	// Test 2: Multiple operations
	test("Test 2: Multiple Operations",
		`a = 5
b = 3
print(a + b)
print(a - b)
print(a * b)
print(a / b)
print(a // b)
print(a % b)`)

	// Test 3: Float operations
	test("Test 3: Float Operations",
		`x = 3.14
y = 2.0
print(x + y)
print(x * y)`)

	// Test 4: String operations
	test("Test 4: String Operations",
		`s = 'Hello'
t = ' World'
print(s + t)
print(s * 3)`)

	// Test 5: Conditionals - if
	test("Test 5: If Statement",
		`x = 5
if x > 0:
    print('positive')`)

	// Test 6: Conditionals - if/else
	test("Test 6: If/Else Statement",
		`x = -3
if x > 0:
    print('positive')
else:
    print('not positive')`)

	// Test 7: Conditionals - if/elif/else
	test("Test 7: If/Elif/Else",
		`x = 0
if x > 0:
    print('positive')
elif x < 0:
    print('negative')
else:
    print('zero')`)

	// Test 8: While loop
	test("Test 8: While Loop",
		`i = 0
while i < 5:
    print(i)
    i = i + 1`)

	// Test 9: For loop with range
	test("Test 9: For Loop with Range",
		`for i in range(5):
    print(i)`)

	// Test 10: For loop with range(start, stop)
	test("Test 10: For Loop with Range(start, stop)",
		`for i in range(3, 7):
    print(i)`)

	// Test 11: For loop with range(start, stop, step)
	test("Test 11: For Loop with Range(start, stop, step)",
		`for i in range(0, 10, 2):
    print(i)`)

	// Test 12: Simple function
	test("Test 12: Simple Function",
		`def greet():
    print('Hello!')

greet()`)

	// Test 13: Function with parameters
	test("Test 13: Function with Parameters",
		`def add(a, b):
    print(a + b)

add(3, 4)`)

	// Test 14: Function with return
	test("Test 14: Function with Return",
		`def multiply(a, b):
    return a * b

result = multiply(5, 6)
print(result)`)

	// Test 15: Recursive function (factorial)
	test("Test 15: Recursive Function (Factorial)",
		`def factorial(n):
    if n <= 1:
        return 1
    return n * factorial(n - 1)

print(factorial(5))`)

	// Test 16: Recursive function (fibonacci)
	test("Test 16: Recursive Function (Fibonacci)",
		`def fib(n):
    if n <= 1:
        return n
    return fib(n - 1) + fib(n - 2)

print(fib(10))`)

	// Test 17: List creation and indexing
	test("Test 17: List Creation and Indexing",
		`items = [1, 2, 3, 4, 5]
print(items[0])
print(items[2])
print(items[-1])`)

	// Test 18: List length
	test("Test 18: List Length",
		`items = [10, 20, 30, 40]
print(len(items))`)

	// Test 19: List append
	test("Test 19: List Append",
		`items = [1, 2, 3]
items.append(4)
items.append(5)
print(items)`)

	// Test 20: For loop over list
	test("Test 20: For Loop Over List",
		`items = [10, 20, 30]
for item in items:
    print(item)`)

	// Test 21: Dict creation and access
	test("Test 21: Dict Creation and Access",
		`data = {'name': 'Alice', 'age': 30}
print(data['name'])
print(data['age'])`)

	// Test 22: Dict modification
	test("Test 22: Dict Modification",
		`data = {'x': 1}
data['y'] = 2
data['z'] = 3
print(data)`)

	// Test 23: Logical operators
	test("Test 23: Logical Operators",
		`print(True and True)
print(True and False)
print(True or False)
print(False or False)
print(not True)
print(not False)`)

	// Test 24: Comparison operators
	test("Test 24: Comparison Operators",
		`x = 5
print(x == 5)
print(x != 5)
print(x < 10)
print(x > 10)
print(x <= 5)
print(x >= 5)`)

	// Test 25: Augmented assignment
	test("Test 25: Augmented Assignment",
		`x = 10
x += 5
print(x)
x -= 3
print(x)
x *= 2
print(x)`)

	// Test 26: Break statement
	test("Test 26: Break Statement",
		`for i in range(10):
    if i == 5:
        break
    print(i)`)

	// Test 27: Continue statement
	test("Test 27: Continue Statement",
		`for i in range(10):
    if i % 2 == 0:
        continue
    print(i)`)

	// Test 28: Nested loops
	test("Test 28: Nested Loops",
		`for i in range(3):
    for j in range(3):
        print(i * 3 + j)`)

	// Test 29: Nested functions (closure)
	test("Test 29: Nested Functions (Closure)",
		`def outer(x):
    def inner(y):
        return x + y
    return inner(10)

print(outer(5))`)

	// Test 30: Built-in str()
	test("Test 30: Built-in str()",
		`x = 42
s = str(x)
print('The answer is ' + s)`)

	// Test 31: Built-in int()
	test("Test 31: Built-in int()",
		`s = '123'
x = int(s)
print(x + 1)`)

	// Test 32: Built-in len()
	test("Test 32: Built-in len()",
		`s = 'Hello'
print(len(s))
items = [1, 2, 3, 4, 5]
print(len(items))`)

	// Test 33: Built-in abs()
	test("Test 33: Built-in abs()",
		`print(abs(-5))
print(abs(5))
print(abs(-3.14))`)

	// Test 34: Built-in min() and max()
	test("Test 34: Built-in min() and max()",
		`print(min(3, 1, 4))
print(max(3, 1, 4))
print(min([5, 2, 8, 1]))
print(max([5, 2, 8, 1]))`)

	// Test 35: Built-in type()
	test("Test 35: Built-in type()",
		`print(type(42))
print(type(3.14))
print(type('hello'))
print(type([1, 2, 3]))
print(type({'a': 1}))`)

	// Test 36: String methods
	test("Test 36: String Methods",
		`s = 'Hello World'
print(s.upper())
print(s.lower())`)

	// Test 37: String strip
	test("Test 37: String Strip",
		`s = '  hello  '
print(s.strip())`)

	// Test 38: String split
	test("Test 38: String Split",
		`s = 'a,b,c,d'
parts = s.split(',')
print(parts)`)

	// Test 39: String join
	test("Test 39: String Join",
		`items = ['a', 'b', 'c']
result = '-'.join(items)
print(result)`)

	// Test 40: In operator
	test("Test 40: In Operator",
		`items = [1, 2, 3, 4, 5]
print(2 in items)
print(10 in items)
print('ell' in 'Hello')`)

	// Test 41: Boolean conversion
	test("Test 41: Boolean Conversion",
		`print(bool(0))
print(bool(1))
print(bool(''))
print(bool('hello'))
print(bool([]))
print(bool([1]))`)

	// Test 42: Ternary expression
	test("Test 42: Ternary Expression",
		`x = 10
result = 'big' if x > 5 else 'small'
print(result)`)

	// Test 43: Unary operators
	test("Test 43: Unary Operators",
		`x = 5
print(-x)
print(+x)`)

	// Test 44: Power operator
	test("Test 44: Power Operator",
		`print(2 ** 10)
print(3 ** 4)`)

	// Test 45: FizzBuzz
	test("Test 45: FizzBuzz",
		`for i in range(1, 16):
    if i % 15 == 0:
        print('FizzBuzz')
    elif i % 3 == 0:
        print('Fizz')
    elif i % 5 == 0:
        print('Buzz')
    else:
        print(i)`)

	// Test 46: Sum function
	test("Test 46: Sum Function",
		`def sum_list(items):
    total = 0
    for item in items:
        total = total + item
    return total

print(sum_list([1, 2, 3, 4, 5]))`)

	// Test 47: Find max function
	test("Test 47: Find Max Function",
		`def find_max(items):
    if len(items) == 0:
        return None
    max_val = items[0]
    for item in items:
        if item > max_val:
            max_val = item
    return max_val

print(find_max([3, 7, 2, 9, 1]))`)

	// Test 48: Bubble sort
	test("Test 48: Bubble Sort",
		`def bubble_sort(arr):
    n = len(arr)
    for i in range(n):
        for j in range(n - i - 1):
            if arr[j] > arr[j + 1]:
                temp = arr[j]
                arr[j] = arr[j + 1]
                arr[j + 1] = temp
    return arr

result = bubble_sort([64, 34, 25, 12, 22, 11, 90])
print(result)`)

	// Test 49: Dict keys and values
	test("Test 49: Dict Keys and Values",
		`data = {'a': 1, 'b': 2, 'c': 3}
print(data.keys())
print(data.values())`)

	// Test 50: Counter example
	test("Test 50: Counter Example",
		`def count_items(items):
    counts = {}
    for item in items:
        if item in counts:
            counts[item] = counts[item] + 1
        else:
            counts[item] = 1
    return counts

result = count_items(['a', 'b', 'a', 'c', 'b', 'a'])
print(result)`)

	fmt.Println("=== All Tests Completed ===")
}
