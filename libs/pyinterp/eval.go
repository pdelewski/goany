package pyinterp

import "libs/pyparser"

// Eval parses and evaluates Python code, returning the final value
func Eval(code string) Value {
	store := NewEnvStore()
	store = InitGlobalEnv(store)
	var finalStore EnvStore
	var result Value
	finalStore, result = EvalWithEnv(code, store, 0)
	// Silence unused variable warning
	if len(finalStore.Envs) < 0 {
		return result
	}
	return result
}

// EvalWithEnv evaluates code with a given environment
func EvalWithEnv(code string, store EnvStore, envIdx int) (EnvStore, Value) {
	ast := pyparser.Parse(code)
	result := evalNode(ast, store, envIdx)
	return result.store, result.val.Val
}

// evalResult holds the evaluation result with environment
type evalResult struct {
	store EnvStore
	val   Result
}

// newEvalResult creates an evalResult with a normal value
func newEvalResult(store EnvStore, val Value) evalResult {
	return evalResult{store: store, val: NewResult(val)}
}

// newEvalResultWithControl creates an evalResult with control flow
func newEvalResultWithControl(store EnvStore, res Result) evalResult {
	return evalResult{store: store, val: res}
}

// evalNode dispatches to the appropriate evaluator based on node type
func evalNode(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	if node.Type == pyparser.NodeModule {
		return evalModule(node, store, envIdx)
	}
	if node.Type == pyparser.NodeFunctionDef {
		return evalFunctionDef(node, store, envIdx)
	}
	if node.Type == pyparser.NodeReturn {
		return evalReturn(node, store, envIdx)
	}
	if node.Type == pyparser.NodeIf {
		return evalIf(node, store, envIdx)
	}
	if node.Type == pyparser.NodeFor {
		return evalFor(node, store, envIdx)
	}
	if node.Type == pyparser.NodeWhile {
		return evalWhile(node, store, envIdx)
	}
	if node.Type == pyparser.NodeAssign {
		return evalAssign(node, store, envIdx)
	}
	if node.Type == pyparser.NodeAugAssign {
		return evalAugAssign(node, store, envIdx)
	}
	if node.Type == pyparser.NodeExpr {
		return evalExpr(node, store, envIdx)
	}
	if node.Type == pyparser.NodeBinOp {
		return evalBinOp(node, store, envIdx)
	}
	if node.Type == pyparser.NodeUnaryOp {
		return evalUnaryOp(node, store, envIdx)
	}
	if node.Type == pyparser.NodeCompare {
		return evalCompare(node, store, envIdx)
	}
	if node.Type == pyparser.NodeCall {
		return evalCall(node, store, envIdx)
	}
	if node.Type == pyparser.NodeName {
		return evalName(node, store, envIdx)
	}
	if node.Type == pyparser.NodeNum {
		return evalNum(node, store, envIdx)
	}
	if node.Type == pyparser.NodeStr {
		return evalStr(node, store, envIdx)
	}
	if node.Type == pyparser.NodeBool {
		return evalBool(node, store, envIdx)
	}
	if node.Type == pyparser.NodeNone {
		return newEvalResult(store, NewNone())
	}
	if node.Type == pyparser.NodeList {
		return evalList(node, store, envIdx)
	}
	if node.Type == pyparser.NodeDict {
		return evalDict(node, store, envIdx)
	}
	if node.Type == pyparser.NodeSubscript {
		return evalSubscript(node, store, envIdx)
	}
	if node.Type == pyparser.NodePass {
		return newEvalResult(store, NewNone())
	}
	if node.Type == pyparser.NodeBreak {
		return newEvalResultWithControl(store, NewBreakResult())
	}
	if node.Type == pyparser.NodeContinue {
		return newEvalResultWithControl(store, NewContinueResult())
	}
	if node.Type == pyparser.NodeTernary {
		return evalTernary(node, store, envIdx)
	}
	return newEvalResult(store, NewNone())
}

// evalModule evaluates a module (sequence of statements)
func evalModule(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	var lastVal Value
	lastVal = NewNone()
	i := 0
	for i < len(node.Children) {
		result := evalNode(node.Children[i], store, envIdx)
		// Check for control flow BEFORE moving parts
		if result.val.Control != ControlNone {
			return result
		}
		store = result.store
		lastVal = result.val.Val
		i = i + 1
	}
	return newEvalResult(store, lastVal)
}

// evalFunctionDef evaluates a function definition
func evalFunctionDef(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	// node.Name is the function name
	// node.Children[0] is a NodeList containing parameters
	// node.Children[1] is a NodeList containing body statements

	name := node.Name
	var paramList []string

	// Extract parameter names from first child (parameters NodeList)
	if len(node.Children) > 0 {
		paramNode := node.Children[0]
		j := 0
		for j < len(paramNode.Children) {
			p := paramNode.Children[j]
			if p.Type == pyparser.NodeName {
				paramList = append(paramList, p.Name)
			} else if p.Type == pyparser.NodeDefault {
				// Parameter with default value - Name is in p.Name
				paramList = append(paramList, p.Name)
			}
			j = j + 1
		}
	}

	// Body is in node.Children[1] which is a NodeList
	var body []pyparser.Node
	if len(node.Children) > 1 {
		bodyNode := node.Children[1]
		body = bodyNode.Children
	}

	// Create function value with closure
	funcVal := NewFunc(name, paramList, body, envIdx)
	store = EnvDefine(store, envIdx, name, funcVal)

	return newEvalResult(store, NewNone())
}

// evalReturn evaluates a return statement
func evalReturn(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	if len(node.Children) == 0 {
		return newEvalResultWithControl(store, NewReturnResult(NewNone()))
	}
	result := evalNode(node.Children[0], store, envIdx)
	return newEvalResultWithControl(result.store, NewReturnResult(result.val.Val))
}

// evalIf evaluates an if/elif/else statement
func evalIf(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	// Children: [condition, body (NodeList), (elif nodes...), (else node)]
	if len(node.Children) < 2 {
		return newEvalResult(store, NewNone())
	}

	// Evaluate condition
	condResult := evalNode(node.Children[0], store, envIdx)
	store = condResult.store

	if ValueToBool(condResult.val.Val) {
		// Execute then-body (node.Children[1] is a NodeList)
		bodyNode := node.Children[1]
		i := 0
		for i < len(bodyNode.Children) {
			result := evalNode(bodyNode.Children[i], store, envIdx)
			if result.val.Control != ControlNone {
				return result
			}
			store = result.store
			i = i + 1
		}
		return newEvalResult(store, NewNone())
	}

	// Condition false - look for elif/else (starting from index 2)
	idx := 2
	for idx < len(node.Children) {
		child := node.Children[idx]
		if child.Type == pyparser.NodeElif {
			// Elif: Child 0 is condition, Child 1 is body (NodeList)
			if len(child.Children) >= 2 {
				elifCondResult := evalNode(child.Children[0], store, envIdx)
				store = elifCondResult.store
				if ValueToBool(elifCondResult.val.Val) {
					// Execute elif body
					elifBody := child.Children[1]
					j := 0
					for j < len(elifBody.Children) {
						result := evalNode(elifBody.Children[j], store, envIdx)
						if result.val.Control != ControlNone {
							return result
						}
						store = result.store
						j = j + 1
					}
					return newEvalResult(store, NewNone())
				}
			}
		} else if child.Type == pyparser.NodeElse {
			// Else: Child 0 is body (NodeList)
			if len(child.Children) >= 1 {
				elseBody := child.Children[0]
				j := 0
				for j < len(elseBody.Children) {
					result := evalNode(elseBody.Children[j], store, envIdx)
					if result.val.Control != ControlNone {
						return result
					}
					store = result.store
					j = j + 1
				}
			}
			return newEvalResult(store, NewNone())
		}
		idx = idx + 1
	}

	return newEvalResult(store, NewNone())
}

// evalWhile evaluates a while loop
func evalWhile(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	// Children: [condition, body (NodeList)]
	if len(node.Children) < 2 {
		return newEvalResult(store, NewNone())
	}

	// Body is in node.Children[1] which is a NodeList
	bodyNode := node.Children[1]
	bodyStatements := bodyNode.Children

	for {
		// Evaluate condition
		condResult := evalNode(node.Children[0], store, envIdx)
		store = condResult.store

		if !ValueToBool(condResult.val.Val) {
			break
		}

		// Execute body
		shouldBreak := false
		i := 0
		for i < len(bodyStatements) {
			result := evalNode(bodyStatements[i], store, envIdx)
			if result.val.Control == ControlReturn {
				return result
			}
			if result.val.Control == ControlBreak {
				shouldBreak = true
				break
			}
			if result.val.Control == ControlContinue {
				break
			}
			store = result.store
			i = i + 1
		}
		if shouldBreak {
			break
		}
	}

	return newEvalResult(store, NewNone())
}

// evalFor evaluates a for loop
func evalFor(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	// Children: [target, iterable, body (NodeList)]
	if len(node.Children) < 3 {
		return newEvalResult(store, NewNone())
	}

	targetNode := node.Children[0]
	iterableResult := evalNode(node.Children[1], store, envIdx)
	store = iterableResult.store
	iterable := iterableResult.val.Val

	// Get items to iterate over
	var items []Value
	if iterable.Type == ValueRange {
		items = RangeToList(iterable)
	} else if iterable.Type == ValueList {
		items = iterable.ListVal
	} else if iterable.Type == ValueString {
		// Iterate over characters - append works on nil slice
		i := 0
		for i < len(iterable.StrVal) {
			items = append(items, NewString(charToString(int(iterable.StrVal[i]))))
			i = i + 1
		}
	} else {
		return newEvalResult(store, NewNone())
	}

	// Body is in node.Children[2] which is a NodeList
	bodyNode := node.Children[2]
	bodyStatements := bodyNode.Children

	// Iterate
	idx := 0
	for idx < len(items) {
		// Assign to target variable
		if targetNode.Type == pyparser.NodeName {
			store = EnvSet(store, envIdx, targetNode.Name, items[idx])
		}

		// Execute body
		shouldBreak := false
		j := 0
		for j < len(bodyStatements) {
			result := evalNode(bodyStatements[j], store, envIdx)
			if result.val.Control == ControlReturn {
				return result
			}
			if result.val.Control == ControlBreak {
				shouldBreak = true
				break
			}
			if result.val.Control == ControlContinue {
				break
			}
			store = result.store
			j = j + 1
		}
		if shouldBreak {
			break
		}
		idx = idx + 1
	}

	return newEvalResult(store, NewNone())
}

// evalAssign evaluates an assignment statement
func evalAssign(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	// Children: [target, value]
	if len(node.Children) < 2 {
		return newEvalResult(store, NewNone())
	}

	target := node.Children[0]
	valueResult := evalNode(node.Children[1], store, envIdx)
	store = valueResult.store
	val := valueResult.val.Val

	if target.Type == pyparser.NodeName {
		// Simple variable assignment
		store = EnvSet(store, envIdx, target.Name, val)
	} else if target.Type == pyparser.NodeSubscript {
		// Subscript assignment: list[i] = val or dict[key] = val
		store = evalSubscriptAssign(target, val, store, envIdx)
	}

	return newEvalResult(store, val)
}

// evalSubscriptAssign handles subscript assignment (list[i] = x, dict[k] = v)
func evalSubscriptAssign(target pyparser.Node, val Value, store EnvStore, envIdx int) EnvStore {
	if len(target.Children) < 2 {
		return store
	}

	// Get the container
	containerResult := evalNode(target.Children[0], store, envIdx)
	store = containerResult.store
	container := containerResult.val.Val

	// Get the index/key
	indexResult := evalNode(target.Children[1], store, envIdx)
	store = indexResult.store
	index := indexResult.val.Val

	// Get the variable name to update
	varName := ""
	if target.Children[0].Type == pyparser.NodeName {
		varName = target.Children[0].Name
	}

	if varName == "" {
		return store
	}

	// Update based on container type
	if container.Type == ValueList && index.Type == ValueInt {
		newContainer := ListSet(container, index.IntVal, val)
		store = EnvSet(store, envIdx, varName, newContainer)
	} else if container.Type == ValueDict {
		newContainer := DictSet(container, index, val)
		store = EnvSet(store, envIdx, varName, newContainer)
	}

	return store
}

// evalAugAssign evaluates augmented assignment (+=, -=, etc.)
func evalAugAssign(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	// node.Op is the operator (+=, -=, etc.)
	// Children: [target, value]
	if len(node.Children) < 2 {
		return newEvalResult(store, NewNone())
	}

	target := node.Children[0]

	// Get current value
	var currentVal Value
	var found bool
	if target.Type == pyparser.NodeName {
		currentVal, found = EnvGet(store, envIdx, target.Name)
		if !found {
			currentVal = NewNone()
		}
	} else if target.Type == pyparser.NodeSubscript {
		result := evalSubscript(target, store, envIdx)
		store = result.store
		currentVal = result.val.Val
	} else {
		return newEvalResult(store, NewNone())
	}

	// Evaluate the right side
	valueResult := evalNode(node.Children[1], store, envIdx)
	store = valueResult.store
	rightVal := valueResult.val.Val

	// Apply operation
	var newVal Value
	op := node.Op
	if op == "+=" {
		newVal = applyBinOp("+", currentVal, rightVal)
	} else if op == "-=" {
		newVal = applyBinOp("-", currentVal, rightVal)
	} else if op == "*=" {
		newVal = applyBinOp("*", currentVal, rightVal)
	} else if op == "/=" {
		newVal = applyBinOp("/", currentVal, rightVal)
	} else if op == "//=" {
		newVal = applyBinOp("//", currentVal, rightVal)
	} else if op == "%=" {
		newVal = applyBinOp("%", currentVal, rightVal)
	} else if op == "**=" {
		newVal = applyBinOp("**", currentVal, rightVal)
	} else {
		newVal = currentVal
	}

	// Assign back
	if target.Type == pyparser.NodeName {
		store = EnvSet(store, envIdx, target.Name, newVal)
	} else if target.Type == pyparser.NodeSubscript {
		store = evalSubscriptAssign(target, newVal, store, envIdx)
	}

	return newEvalResult(store, newVal)
}

// evalExpr evaluates an expression statement
func evalExpr(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	if len(node.Children) == 0 {
		return newEvalResult(store, NewNone())
	}
	return evalNode(node.Children[0], store, envIdx)
}

// evalBinOp evaluates a binary operation
func evalBinOp(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	// Children: [left, right]
	if len(node.Children) < 2 {
		return newEvalResult(store, NewNone())
	}

	// Handle attribute access (obj.attr) - returns None for non-method attribute access
	// Method calls like obj.method() are handled in evalCall before reaching here
	if node.Op == "." {
		return newEvalResult(store, NewNone())
	}

	leftResult := evalNode(node.Children[0], store, envIdx)
	store = leftResult.store
	left := leftResult.val.Val

	// Short-circuit evaluation for and/or
	if node.Op == "and" {
		if !ValueToBool(left) {
			return newEvalResult(store, left)
		}
		andResult := evalNode(node.Children[1], store, envIdx)
		return newEvalResult(andResult.store, andResult.val.Val)
	}
	if node.Op == "or" {
		if ValueToBool(left) {
			return newEvalResult(store, left)
		}
		orResult := evalNode(node.Children[1], store, envIdx)
		return newEvalResult(orResult.store, orResult.val.Val)
	}

	// Evaluate right side
	rightResult := evalNode(node.Children[1], store, envIdx)
	store = rightResult.store
	right := rightResult.val.Val

	result := applyBinOp(node.Op, left, right)
	return newEvalResult(store, result)
}

// applyBinOp applies a binary operator to two values
func applyBinOp(op string, left Value, right Value) Value {
	// String concatenation
	if op == "+" && left.Type == ValueString && right.Type == ValueString {
		return NewString(left.StrVal + right.StrVal)
	}

	// String repetition
	if op == "*" && left.Type == ValueString && right.Type == ValueInt {
		result := ""
		for i := 0; i < right.IntVal; i++ {
			result = result + left.StrVal
		}
		return NewString(result)
	}

	// List concatenation
	if op == "+" && left.Type == ValueList && right.Type == ValueList {
		var items []Value
		i := 0
		for i < len(left.ListVal) {
			items = append(items, left.ListVal[i])
			i = i + 1
		}
		j := 0
		for j < len(right.ListVal) {
			items = append(items, right.ListVal[j])
			j = j + 1
		}
		return NewList(items)
	}

	// Numeric operations
	if (left.Type == ValueInt || left.Type == ValueFloat) &&
		(right.Type == ValueInt || right.Type == ValueFloat) {

		// Convert to floats for calculation
		var lf, rf float64
		if left.Type == ValueInt {
			lf = float64(left.IntVal)
		} else {
			lf = left.FloatVal
		}
		if right.Type == ValueInt {
			rf = float64(right.IntVal)
		} else {
			rf = right.FloatVal
		}

		// Determine if result should be int or float
		isFloat := left.Type == ValueFloat || right.Type == ValueFloat

		if op == "+" {
			if isFloat {
				return NewFloat(lf + rf)
			}
			return NewInt(left.IntVal + right.IntVal)
		}
		if op == "-" {
			if isFloat {
				return NewFloat(lf - rf)
			}
			return NewInt(left.IntVal - right.IntVal)
		}
		if op == "*" {
			if isFloat {
				return NewFloat(lf * rf)
			}
			return NewInt(left.IntVal * right.IntVal)
		}
		if op == "/" {
			if rf == 0 {
				return NewFloat(0) // Division by zero
			}
			return NewFloat(lf / rf) // Always returns float
		}
		if op == "//" {
			if rf == 0 {
				return NewInt(0)
			}
			return NewInt(int(lf / rf))
		}
		if op == "%" {
			if right.IntVal == 0 {
				return NewInt(0)
			}
			return NewInt(left.IntVal % right.IntVal)
		}
		if op == "**" {
			// Power operation
			result := 1.0
			for i := 0; i < int(rf); i++ {
				result = result * lf
			}
			if isFloat {
				return NewFloat(result)
			}
			return NewInt(int(result))
		}
	}

	return NewNone()
}

// evalUnaryOp evaluates a unary operation
func evalUnaryOp(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	if len(node.Children) < 1 {
		return newEvalResult(store, NewNone())
	}

	operandResult := evalNode(node.Children[0], store, envIdx)
	store = operandResult.store
	operand := operandResult.val.Val

	if node.Op == "-" {
		if operand.Type == ValueInt {
			return newEvalResult(store, NewInt(-operand.IntVal))
		}
		if operand.Type == ValueFloat {
			return newEvalResult(store, NewFloat(-operand.FloatVal))
		}
	}
	if node.Op == "+" {
		return newEvalResult(store, operand)
	}
	if node.Op == "not" {
		return newEvalResult(store, NewBool(!ValueToBool(operand)))
	}

	return newEvalResult(store, NewNone())
}

// evalCompare evaluates a comparison operation
func evalCompare(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	// Children: [left, right] or [left, right1, right2, ...] for chained
	if len(node.Children) < 2 {
		return newEvalResult(store, NewBool(false))
	}

	leftResult := evalNode(node.Children[0], store, envIdx)
	store = leftResult.store
	left := leftResult.val.Val

	rightResult := evalNode(node.Children[1], store, envIdx)
	store = rightResult.store
	right := rightResult.val.Val

	result := applyCompare(node.Op, left, right)
	return newEvalResult(store, NewBool(result))
}

// applyCompare applies a comparison operator
func applyCompare(op string, left Value, right Value) bool {
	if op == "==" {
		return ValueEquals(left, right)
	}
	if op == "!=" {
		return !ValueEquals(left, right)
	}
	if op == "<" {
		return ValueCompare(left, right) < 0
	}
	if op == ">" {
		return ValueCompare(left, right) > 0
	}
	if op == "<=" {
		return ValueCompare(left, right) <= 0
	}
	if op == ">=" {
		return ValueCompare(left, right) >= 0
	}
	if op == "in" {
		return valueIn(left, right)
	}
	if op == "not in" {
		return !valueIn(left, right)
	}
	if op == "is" {
		// For None comparison
		if left.Type == ValueNone && right.Type == ValueNone {
			return true
		}
		return ValueEquals(left, right)
	}
	if op == "is not" {
		if left.Type == ValueNone && right.Type == ValueNone {
			return false
		}
		return !ValueEquals(left, right)
	}
	return false
}

// valueIn checks if a value is in a collection
func valueIn(item Value, collection Value) bool {
	if collection.Type == ValueList {
		for i := 0; i < len(collection.ListVal); i++ {
			if ValueEquals(item, collection.ListVal[i]) {
				return true
			}
		}
		return false
	}
	if collection.Type == ValueString && item.Type == ValueString {
		// Check if substring
		for i := 0; i <= len(collection.StrVal)-len(item.StrVal); i++ {
			match := true
			for j := 0; j < len(item.StrVal); j++ {
				if collection.StrVal[i+j] != item.StrVal[j] {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
		return false
	}
	if collection.Type == ValueDict {
		for i := 0; i < len(collection.DictKeys); i++ {
			if ValueEquals(item, collection.DictKeys[i]) {
				return true
			}
		}
		return false
	}
	return false
}

// evalCall evaluates a function call
// evalArgsResult holds args evaluation results
type evalArgsResult struct {
	store EnvStore
	args  []Value
}

// evalArgsFromNode evaluates arguments from a call node
func evalArgsFromNode(node pyparser.Node, store EnvStore, envIdx int) evalArgsResult {
	var args []Value
	if len(node.Children) > 1 {
		argsNode := node.Children[1]
		i := 0
		for i < len(argsNode.Children) {
			argResult := evalNode(argsNode.Children[i], store, envIdx)
			store = argResult.store
			args = append(args, argResult.val.Val)
			i = i + 1
		}
	}
	return evalArgsResult{store: store, args: args}
}

func evalCall(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	// Children: [func, args (NodeList)]
	if len(node.Children) < 1 {
		return newEvalResult(store, NewNone())
	}

	funcNode := node.Children[0]

	// Check for method call (e.g., list.append)
	// Parser creates NodeBinOp with Op="." for attribute access
	if funcNode.Type == pyparser.NodeBinOp && funcNode.Op == "." {
		return evalMethodCall(node, store, envIdx)
	}

	// Get function value
	var funcVal Value
	if funcNode.Type == pyparser.NodeName {
		// Check for builtin first
		if IsBuiltin(funcNode.Name) {
			argsRes := evalArgsFromNode(node, store, envIdx)
			store = argsRes.store
			result := CallBuiltin(funcNode.Name, argsRes.args)
			return newEvalResult(store, result)
		}
		// Look up in environment
		var found bool
		funcVal, found = EnvGet(store, envIdx, funcNode.Name)
		if !found {
			return newEvalResult(store, NewNone())
		}
	} else {
		// Evaluate function expression
		funcResult := evalNode(funcNode, store, envIdx)
		store = funcResult.store
		funcVal = funcResult.val.Val
	}

	// Handle builtin function value
	if funcVal.Type == ValueBuiltin {
		argsRes := evalArgsFromNode(node, store, envIdx)
		store = argsRes.store
		result := CallBuiltin(funcVal.BuiltinName, argsRes.args)
		return newEvalResult(store, result)
	}

	// Handle user-defined function
	if funcVal.Type == ValueFunc {
		argsRes := evalArgsFromNode(node, store, envIdx)
		store = argsRes.store
		args := argsRes.args

		// Create new environment for function execution
		var newEnvIdx int
		store, newEnvIdx = NewEnv(store, funcVal.FuncEnv)

		// Bind parameters
		i := 0
		for i < len(funcVal.FuncParams) && i < len(args) {
			store = EnvDefine(store, newEnvIdx, funcVal.FuncParams[i], args[i])
			i = i + 1
		}

		// Execute function body
		j := 0
		for j < len(funcVal.FuncBody) {
			result := evalNode(funcVal.FuncBody[j], store, newEnvIdx)
			store = result.store
			if result.val.Control == ControlReturn {
				return newEvalResult(store, result.val.Val)
			}
			j = j + 1
		}
		return newEvalResult(store, NewNone())
	}

	return newEvalResult(store, NewNone())
}

// evalMethodCall evaluates a method call (e.g., list.append(x))
func evalMethodCall(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	funcNode := node.Children[0]
	// funcNode.Children[0] is the object
	// funcNode.Children[1] is the method name

	if len(funcNode.Children) < 2 {
		return newEvalResult(store, NewNone())
	}

	objResult := evalNode(funcNode.Children[0], store, envIdx)
	store = objResult.store
	obj := objResult.val.Val

	methodNameNode := funcNode.Children[1]
	methodName := ""
	if methodNameNode.Type == pyparser.NodeName {
		methodName = methodNameNode.Name
	}

	// Evaluate arguments from NodeList
	argsRes := evalArgsFromNode(node, store, envIdx)
	store = argsRes.store
	args := argsRes.args

	// Handle list methods
	if obj.Type == ValueList {
		if methodName == "append" && len(args) > 0 {
			// Get variable name to update
			if funcNode.Children[0].Type == pyparser.NodeName {
				varName := funcNode.Children[0].Name
				newList := ListAppend(obj, args[0])
				store = EnvSet(store, envIdx, varName, newList)
			}
			return newEvalResult(store, NewNone())
		}
		if methodName == "pop" {
			if len(obj.ListVal) > 0 {
				idx := len(obj.ListVal) - 1
				if len(args) > 0 && args[0].Type == ValueInt {
					idx = args[0].IntVal
					if idx < 0 {
						idx = len(obj.ListVal) + idx
					}
				}
				if idx >= 0 && idx < len(obj.ListVal) {
					poppedVal := obj.ListVal[idx]
					var newItems []Value
					i := 0
					for i < len(obj.ListVal) {
						if i != idx {
							newItems = append(newItems, obj.ListVal[i])
						}
						i = i + 1
					}
					if funcNode.Children[0].Type == pyparser.NodeName {
						varName := funcNode.Children[0].Name
						store = EnvSet(store, envIdx, varName, NewList(newItems))
					}
					return newEvalResult(store, poppedVal)
				}
			}
			return newEvalResult(store, NewNone())
		}
	}

	// Handle string methods
	if obj.Type == ValueString {
		if methodName == "upper" {
			result := ""
			for i := 0; i < len(obj.StrVal); i++ {
				ch := int(obj.StrVal[i])
				if ch >= 97 && ch <= 122 { // 'a' to 'z'
					result = result + charToString(ch-32)
				} else {
					result = result + charToString(ch)
				}
			}
			return newEvalResult(store, NewString(result))
		}
		if methodName == "lower" {
			result := ""
			for i := 0; i < len(obj.StrVal); i++ {
				ch := int(obj.StrVal[i])
				if ch >= 65 && ch <= 90 { // 'A' to 'Z'
					result = result + charToString(ch+32)
				} else {
					result = result + charToString(ch)
				}
			}
			return newEvalResult(store, NewString(result))
		}
		if methodName == "strip" {
			s := obj.StrVal
			start := 0
			end := len(s)
			for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n') {
				start++
			}
			for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n') {
				end--
			}
			return newEvalResult(store, NewString(substring(s, start, end)))
		}
		if methodName == "split" {
			sep := " "
			if len(args) > 0 && args[0].Type == ValueString {
				sep = args[0].StrVal
			}
			parts := stringSplit(obj.StrVal, sep)
			var items []Value
			i := 0
			for i < len(parts) {
				items = append(items, NewString(parts[i]))
				i = i + 1
			}
			return newEvalResult(store, NewList(items))
		}
		if methodName == "join" {
			if len(args) > 0 && args[0].Type == ValueList {
				result := ""
				i := 0
				for i < len(args[0].ListVal) {
					if i > 0 {
						result = result + obj.StrVal
					}
					result = result + ValueToString(args[0].ListVal[i])
					i = i + 1
				}
				return newEvalResult(store, NewString(result))
			}
			return newEvalResult(store, NewString(""))
		}
	}

	// Handle dict methods
	if obj.Type == ValueDict {
		if methodName == "keys" {
			return newEvalResult(store, NewList(obj.DictKeys))
		}
		if methodName == "values" {
			return newEvalResult(store, NewList(obj.DictVals))
		}
		if methodName == "get" {
			if len(args) > 0 {
				val := DictGet(obj, args[0])
				if val.Type == ValueNone && len(args) > 1 {
					return newEvalResult(store, args[1])
				}
				return newEvalResult(store, val)
			}
			return newEvalResult(store, NewNone())
		}
	}

	return newEvalResult(store, NewNone())
}

// stringSplit splits a string by separator
func stringSplit(s string, sep string) []string {
	result := []string{}
	current := ""
	i := 0
	for i < len(s) {
		if i+len(sep) <= len(s) {
			match := true
			for j := 0; j < len(sep); j++ {
				if s[i+j] != sep[j] {
					match = false
					break
				}
			}
			if match {
				result = append(result, current)
				current = ""
				i = i + len(sep)
				continue
			}
		}
		current = current + charToString(int(s[i]))
		i++
	}
	result = append(result, current)
	return result
}

// evalSubscript evaluates a subscript operation (list[i], dict[k], str[i])
func evalSubscript(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	if len(node.Children) < 2 {
		return newEvalResult(store, NewNone())
	}

	// Check for attribute access (obj.attr)
	if node.Op == "." {
		return evalAttribute(node, store, envIdx)
	}

	// Regular subscript
	containerResult := evalNode(node.Children[0], store, envIdx)
	store = containerResult.store
	container := containerResult.val.Val

	indexResult := evalNode(node.Children[1], store, envIdx)
	store = indexResult.store
	index := indexResult.val.Val

	if container.Type == ValueList && index.Type == ValueInt {
		return newEvalResult(store, ListGet(container, index.IntVal))
	}
	if container.Type == ValueString && index.Type == ValueInt {
		idx := index.IntVal
		if idx < 0 {
			idx = len(container.StrVal) + idx
		}
		if idx >= 0 && idx < len(container.StrVal) {
			return newEvalResult(store, NewString(charToString(int(container.StrVal[idx]))))
		}
		return newEvalResult(store, NewString(""))
	}
	if container.Type == ValueDict {
		return newEvalResult(store, DictGet(container, index))
	}

	return newEvalResult(store, NewNone())
}

// evalAttribute evaluates an attribute access (obj.attr)
func evalAttribute(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	// For now, just return None for non-method attribute access
	// Methods are handled in evalMethodCall
	return newEvalResult(store, NewNone())
}

// evalName evaluates a name/identifier
func evalName(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	val, found := EnvGet(store, envIdx, node.Name)
	if found {
		return newEvalResult(store, val)
	}
	// Check if it's a builtin name
	if IsBuiltin(node.Name) {
		return newEvalResult(store, NewBuiltin(node.Name))
	}
	return newEvalResult(store, NewNone())
}

// evalNum evaluates a numeric literal
func evalNum(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	// Check if it's a float
	isFloat := false
	for i := 0; i < len(node.Value); i++ {
		if node.Value[i] == '.' {
			isFloat = true
			break
		}
	}

	if isFloat {
		f := parseFloat(node.Value)
		return newEvalResult(store, NewFloat(f))
	}
	n := parseInt(node.Value)
	return newEvalResult(store, NewInt(n))
}

// parseInt parses an integer from a string
func parseInt(s string) int {
	result := 0
	negative := false
	startIdx := 0
	if len(s) > 0 && s[0] == '-' {
		negative = true
		startIdx = 1
	}
	for i := startIdx; i < len(s); i++ {
		ch := s[i]
		if ch >= '0' && ch <= '9' {
			result = result*10 + int(ch-'0')
		}
	}
	if negative {
		result = -result
	}
	return result
}

// parseFloat parses a float from a string
func parseFloat(s string) float64 {
	result := 0.0
	negative := false
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
	return result
}

// evalStr evaluates a string literal
func evalStr(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	return newEvalResult(store, NewString(node.Value))
}

// evalBool evaluates a boolean literal
func evalBool(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	if node.Value == "True" {
		return newEvalResult(store, NewBool(true))
	}
	return newEvalResult(store, NewBool(false))
}

// evalList evaluates a list literal
func evalList(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	var items []Value
	i := 0
	for i < len(node.Children) {
		itemResult := evalNode(node.Children[i], store, envIdx)
		store = itemResult.store
		items = append(items, itemResult.val.Val)
		i = i + 1
	}
	return newEvalResult(store, NewList(items))
}

// evalDict evaluates a dict literal
func evalDict(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	var keys []Value
	var vals []Value
	i := 0
	for i < len(node.Children) {
		entry := node.Children[i]
		if entry.Type == pyparser.NodeDictEntry && len(entry.Children) >= 2 {
			keyResult := evalNode(entry.Children[0], store, envIdx)
			store = keyResult.store
			valResult := evalNode(entry.Children[1], store, envIdx)
			store = valResult.store
			keys = append(keys, keyResult.val.Val)
			vals = append(vals, valResult.val.Val)
		}
		i = i + 1
	}
	return newEvalResult(store, NewDict(keys, vals))
}

// evalTernary evaluates a ternary/conditional expression (x if c else y)
func evalTernary(node pyparser.Node, store EnvStore, envIdx int) evalResult {
	// Children: [true_value, condition, false_value] (as parsed)
	if len(node.Children) < 3 {
		return newEvalResult(store, NewNone())
	}

	condResult := evalNode(node.Children[1], store, envIdx)
	store = condResult.store

	if ValueToBool(condResult.val.Val) {
		return evalNode(node.Children[0], store, envIdx)
	}
	return evalNode(node.Children[2], store, envIdx)
}
