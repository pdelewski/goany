# Python Parser Library

A Python syntax parser written in Go, designed to work with the goany transpiler for cross-language compilation to C++, Rust, JavaScript, and Java.

## Overview

This library parses Python source code and produces an Abstract Syntax Tree (AST). It supports approximately 95-97% of Python syntax, covering all common constructs used in typical Python programs.

## Usage

```go
import "libs/pyparser"

func main() {
    code := `def greet(name):
    return "Hello, " + name`

    ast := pyparser.Parse(code)
    fmt.Println(pyparser.PrintAST(ast))
}
```

## Supported Features

### Literals
| Feature | Example | Status |
|---------|---------|--------|
| Integers | `42`, `1_000_000` | Supported |
| Floats | `3.14`, `1e-10` | Supported |
| Hex/Octal/Binary | `0xFF`, `0o77`, `0b1010` | Supported |
| Complex numbers | `3+4j`, `2.5j` | Supported |
| Strings | `"hello"`, `'world'` | Supported |
| Triple-quoted | `"""multi-line"""` | Supported |
| Raw strings | `r"\n is literal"` | Supported |
| Byte strings | `b"bytes"` | Supported |
| F-strings | `f"Hello {name}"` | Supported |
| Booleans | `True`, `False` | Supported |
| None | `None` | Supported |
| Ellipsis | `...` | Supported |

### Data Structures
| Feature | Example | Status |
|---------|---------|--------|
| Lists | `[1, 2, 3]` | Supported |
| Dicts | `{"a": 1, "b": 2}` | Supported |
| Sets | `{1, 2, 3}` | Supported |
| Tuples | `(1, 2, 3)` | Supported |

### Operators
| Category | Operators | Status |
|----------|-----------|--------|
| Arithmetic | `+`, `-`, `*`, `/`, `//`, `%`, `**`, `@` | Supported |
| Comparison | `==`, `!=`, `<`, `>`, `<=`, `>=` | Supported |
| Identity/Membership | `is`, `is not`, `in`, `not in` | Supported |
| Logical | `and`, `or`, `not` | Supported |
| Bitwise | `&`, `\|`, `^`, `~`, `<<`, `>>` | Supported |
| Assignment | `=`, `+=`, `-=`, `*=`, `/=`, etc. | Supported |
| Walrus | `:=` | Supported |

### Control Flow
| Feature | Example | Status |
|---------|---------|--------|
| If/Elif/Else | `if x > 0: ...` | Supported |
| For loops | `for i in range(10): ...` | Supported |
| While loops | `while condition: ...` | Supported |
| Loop else | `for x in y: ... else: ...` | Supported |
| Break/Continue | `break`, `continue` | Supported |
| Pass | `pass` | Supported |
| Match/Case | `match x: case 1: ...` | Supported |

### Exception Handling
| Feature | Example | Status |
|---------|---------|--------|
| Try/Except | `try: ... except: ...` | Supported |
| Multiple except types | `except (TypeError, ValueError): ...` | Supported |
| Except with alias | `except Error as e: ...` | Supported |
| Try/Else/Finally | `try: ... else: ... finally: ...` | Supported |
| Raise | `raise ValueError("msg")` | Supported |
| Raise from | `raise NewError from original` | Supported |
| Assert | `assert condition, "message"` | Supported |

### Functions
| Feature | Example | Status |
|---------|---------|--------|
| Definition | `def func(): ...` | Supported |
| Parameters | `def func(a, b): ...` | Supported |
| Default values | `def func(a=1): ...` | Supported |
| *args/**kwargs | `def func(*args, **kwargs): ...` | Supported |
| Type hints | `def func(x: int) -> str: ...` | Supported |
| Positional-only | `def func(a, /, b): ...` | Supported |
| Keyword-only | `def func(a, *, b): ...` | Supported |
| Lambda | `lambda x: x + 1` | Supported |
| Decorators | `@decorator` | Supported |
| Return/Yield | `return x`, `yield x` | Supported |

### Classes
| Feature | Example | Status |
|---------|---------|--------|
| Definition | `class Foo: ...` | Supported |
| Inheritance | `class Foo(Bar): ...` | Supported |
| Multiple inheritance | `class Foo(A, B, C): ...` | Supported |
| Decorators | `@dataclass` | Supported |

### Comprehensions
| Feature | Example | Status |
|---------|---------|--------|
| List comprehension | `[x for x in items]` | Supported |
| Dict comprehension | `{k: v for k, v in items}` | Supported |
| Set comprehension | `{x for x in items}` | Supported |
| Generator expression | `(x for x in items)` | Supported |
| With condition | `[x for x in items if x > 0]` | Supported |

### Imports
| Feature | Example | Status |
|---------|---------|--------|
| Import | `import os` | Supported |
| Import as | `import numpy as np` | Supported |
| From import | `from os import path` | Supported |
| From import as | `from os import path as p` | Supported |

### Other
| Feature | Example | Status |
|---------|---------|--------|
| With statement | `with open(f) as file: ...` | Supported |
| Ternary | `x if condition else y` | Supported |
| Subscript | `a[0]`, `a[1:3]`, `a[::2]` | Supported |
| Attribute access | `obj.attr` | Supported |
| Unpacking | `a, b = 1, 2` | Supported |
| Starred unpacking | `first, *rest = items` | Supported |
| Global/Nonlocal | `global x`, `nonlocal y` | Supported |
| Del | `del x` | Supported |
| Async/Await | `async def`, `await` | Supported |

### Match Patterns (Python 3.10+)
| Pattern | Example | Status |
|---------|---------|--------|
| Literal | `case 1:` | Supported |
| Wildcard | `case _:` | Supported |
| Capture | `case x:` | Supported |
| Sequence | `case [a, b]:` | Supported |
| OR patterns | `case 1 \| 2 \| 3:` | Supported |
| Guard | `case x if x > 0:` | Supported |

## Limitations

### Simplified Support
- **Nested comprehensions**: Only single `for` clause supported
- **Nested tuple unpacking**: Simplified to avoid complexity
- **Complex match patterns**: Class patterns have limited support

### Not Supported
- **Unicode identifiers**: Only ASCII identifiers
- **Type parameter syntax** (Python 3.12+): `def func[T](x: T)`
- **`except*`** (Python 3.11+): Exception groups

## Architecture

The parser consists of four main components:

1. **tokens.go** - Token type definitions
2. **lexer.go** - Tokenizer/lexer implementation
3. **ast.go** - AST node definitions
4. **parser.go** - Recursive descent parser
5. **printer.go** - AST pretty printer

## AST Node Structure

```go
type Node struct {
    Type     int      // Node type constant
    Name     string   // For identifiers, function names
    Value    string   // For literals
    Op       string   // For operators
    Children []Node   // Child nodes
    Line     int      // Source line number
}
```
ą
## Example Output

Input:
```python
def add(a, b):
    return a + b
```

AST Output:
```
Module
  FunctionDef name=add
    List
      Name name=a
      Name name=b
    List
      Return
        BinOp op=+
          Name name=a
          Name name=b
```
