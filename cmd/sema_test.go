package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// SemaTestCase represents a test case for unsupported construct detection
type SemaTestCase struct {
	Name           string
	Code           string
	ExpectedError  string
}

var semaTestCases = []SemaTestCase{
	{
		Name: "nil_comparison_eq",
		Code: `package main

func main() {
	var a []int
	if a == nil {
	}
}
`,
		ExpectedError: "nil comparison",
	},
	{
		Name: "nil_comparison_neq",
		Code: `package main

func main() {
	var a []int
	if a != nil {
	}
}
`,
		ExpectedError: "nil comparison",
	},
	{
		Name: "struct_field_init_order",
		Code: `package main

type Person struct {
	Name string
	Age  int
	City string
}

func main() {
	p := Person{
		Age:  30,
		Name: "Alice",
		City: "NYC",
	}
	_ = p
}
`,
		ExpectedError: "struct field initialization order does not match declaration order",
	},
}

// SemaValidTestCase represents code that SHOULD compile successfully
type SemaValidTestCase struct {
	Name string
	Code string
}

var semaValidTestCases = []SemaValidTestCase{
	// Patterns now handled by LangSemaLoweringPass transforms (previously sema errors)
	{
		Name: "iota_now_valid",
		Code: `package main

const (
	A = iota
	B
	C
)

func main() {
}
`,
	},
	{
		Name: "string_variable_reuse_after_concat_now_valid",
		Code: `package main

func main() {
	indent := "  "
	result := indent + "hello"
	result = result + indent
	_ = result
}
`,
	},
	{
		Name: "string_plusequal_self_concat_now_valid",
		Code: `package main

func main() {
	result := "hello"
	indent := "  "
	result += result + indent
	_ = result
}
`,
	},
	{
		Name: "same_var_multiple_times_in_expr_string_now_valid",
		Code: `package main

func process(s string) string { return s }

func main() {
	x := "hello"
	result := process(x) + process(x)
	_ = result
}
`,
	},
	{
		Name: "slice_self_assignment_now_valid",
		Code: `package main

func main() {
	slice := []string{"a", "b", "c", "d", "e"}
	i := 0
	j := 1
	slice[i] = slice[j]
	_ = slice
}
`,
	},
	{
		Name: "multiple_closures_capture_same_var_now_valid",
		Code: `package main

func main() {
	x := "hello"
	fn1 := func() string { return x }
	fn2 := func() string { return x }
	_ = fn1
	_ = fn2
}
`,
	},
	{
		Name: "variable_shadowing_in_nested_block_now_valid",
		Code: `package main

func main() {
	col := 0
	for {
		if col >= 10 {
			break
		}
		col := 1
		_ = col
	}
	_ = col
}
`,
	},
	{
		Name: "variable_shadowing_in_if_block_now_valid",
		Code: `package main

func main() {
	x := 5
	if true {
		x := 10
		_ = x
	}
	_ = x
}
`,
	},
	{
		Name: "string_reassign_then_reuse",
		Code: `package main

func main() {
	x := "hello"
	x = x + " world"
	y := x + "!"
	_ = y
}
`,
	},
	{
		Name: "string_plusequal_then_reuse",
		Code: `package main

func main() {
	x := "hello"
	x += " world"
	y := x + "!"
	_ = y
}
`,
	},
	{
		Name: "struct_field_init_correct_order",
		Code: `package main

type Person struct {
	Name string
	Age  int
	City string
}

func main() {
	p := Person{
		Name: "Alice",
		Age:  30,
		City: "NYC",
	}
	_ = p
}
`,
	},
	// Valid Rust ownership patterns
	{
		Name: "same_var_int_multiple_times_ok",
		Code: `package main

func add(a int, b int) int { return a + b }

func main() {
	x := 5
	result := add(x, x)
	_ = result
}
`,
	},
	{
		Name: "slice_different_slices_assignment_ok",
		Code: `package main

func main() {
	slice1 := []int{1, 2, 3}
	slice2 := []int{4, 5, 6}
	slice1[0] = slice2[0]
	_ = slice1
}
`,
	},
	{
		Name: "slice_temp_variable_ok",
		Code: `package main

func main() {
	slice := []int{1, 2, 3, 4, 5}
	tmp := slice[1]
	slice[0] = tmp
	_ = slice
}
`,
	},
	{
		Name: "single_closure_capture_ok",
		Code: `package main

func main() {
	x := "hello"
	fn := func() string { return x }
	_ = fn
}
`,
	},
	{
		Name: "variable_reassignment_ok",
		Code: `package main

func main() {
	col := 0
	for {
		if col >= 10 {
			break
		}
		col = col + 1
	}
	_ = col
}
`,
	},
	{
		Name: "different_var_names_in_nested_scope_ok",
		Code: `package main

func main() {
	col := 0
	for {
		if col >= 10 {
			break
		}
		innerCol := 1
		_ = innerCol
		col = col + 1
	}
	_ = col
}
`,
	},
}

func TestSemaValidConstructs(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	for _, tc := range semaValidTestCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			runSemaValidTest(t, wd, tc)
		})
	}
}

func runSemaValidTest(t *testing.T, wd string, tc SemaValidTestCase) {
	// Create temporary directory for test
	testDir := filepath.Join(os.TempDir(), "sema_valid_test_"+tc.Name)
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}
	defer os.RemoveAll(testDir)

	// Create go.mod
	goMod := filepath.Join(testDir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module sematest\ngo 1.24.4\n"), 0644); err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	// Create main.go with test code
	mainGo := filepath.Join(testDir, "main.go")
	if err := os.WriteFile(mainGo, []byte(tc.Code), 0644); err != nil {
		t.Fatalf("Failed to write main.go: %v", err)
	}

	// Run the compiler - it should succeed
	cmd := exec.Command("go", "run", ".", "--source="+testDir, "--output="+filepath.Join(testDir, "out"))
	cmd.Dir = wd
	output, err := cmd.CombinedOutput()

	// We expect the command to succeed
	if err != nil {
		t.Fatalf("Expected compilation to succeed for %s, but it failed.\nOutput: %s", tc.Name, output)
	}

	t.Logf("Correctly accepted %s", tc.Name)
}

func TestSemaUnsupportedConstructs(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get working directory: %v", err)
	}

	for _, tc := range semaTestCases {
		tc := tc
		t.Run(tc.Name, func(t *testing.T) {
			runSemaTest(t, wd, tc)
		})
	}
}

func runSemaTest(t *testing.T, wd string, tc SemaTestCase) {
	// Create temporary directory for test
	testDir := filepath.Join(os.TempDir(), "sema_test_"+tc.Name)
	if err := os.MkdirAll(testDir, 0755); err != nil {
		t.Fatalf("Failed to create test dir: %v", err)
	}
	defer os.RemoveAll(testDir)

	// Create go.mod
	goMod := filepath.Join(testDir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module sematest\ngo 1.24.4\n"), 0644); err != nil {
		t.Fatalf("Failed to write go.mod: %v", err)
	}

	// Create main.go with test code
	mainGo := filepath.Join(testDir, "main.go")
	if err := os.WriteFile(mainGo, []byte(tc.Code), 0644); err != nil {
		t.Fatalf("Failed to write main.go: %v", err)
	}

	// Run the compiler - it should fail
	cmd := exec.Command("go", "run", ".", "--source="+testDir, "--output="+filepath.Join(testDir, "out"))
	cmd.Dir = wd
	output, err := cmd.CombinedOutput()

	// We expect the command to fail
	if err == nil {
		t.Fatalf("Expected compilation to fail for %s, but it succeeded.\nOutput: %s", tc.Name, output)
	}

	// Check that the expected error message is in the output
	if !strings.Contains(string(output), tc.ExpectedError) {
		t.Fatalf("Expected error containing %q for %s, but got:\n%s", tc.ExpectedError, tc.Name, output)
	}

	t.Logf("Correctly rejected %s with error: %s", tc.Name, tc.ExpectedError)
}
