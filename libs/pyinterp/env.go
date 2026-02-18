package pyinterp

// Variable represents a named variable in scope
type Variable struct {
	Name string
	Val  Value
}

// Environment represents a scope with variables
type Environment struct {
	Vars   []Variable
	Parent int // Index of parent env, -1 for global
}

// EnvStore stores all environments (index-based for goany compatibility)
type EnvStore struct {
	Envs []Environment
}

// NewEnvStore creates a new environment store with global scope
func NewEnvStore() EnvStore {
	store := EnvStore{
		Envs: []Environment{},
	}
	// Create global environment
	globalEnv := Environment{
		Vars:   []Variable{},
		Parent: -1,
	}
	store.Envs = append(store.Envs, globalEnv)
	return store
}

// NewEnv creates a new environment with the given parent
func NewEnv(store EnvStore, parentIdx int) (EnvStore, int) {
	env := Environment{
		Vars:   []Variable{},
		Parent: parentIdx,
	}
	store.Envs = append(store.Envs, env)
	return store, len(store.Envs) - 1
}

// envFindVar finds a variable in the current scope (not parent scopes)
func envFindVar(store EnvStore, envIdx int, name string) int {
	if envIdx < 0 || envIdx >= len(store.Envs) {
		return -1
	}
	env := store.Envs[envIdx]
	for i := 0; i < len(env.Vars); i++ {
		if env.Vars[i].Name == name {
			return i
		}
	}
	return -1
}

// EnvGet gets a variable value, searching up the scope chain
func EnvGet(store EnvStore, envIdx int, name string) (Value, bool) {
	currentIdx := envIdx
	for currentIdx >= 0 {
		varIdx := envFindVar(store, currentIdx, name)
		if varIdx >= 0 {
			return store.Envs[currentIdx].Vars[varIdx].Val, true
		}
		currentIdx = store.Envs[currentIdx].Parent
	}
	return NewNone(), false
}

// EnvSet sets a variable value, searching up the scope chain
// If the variable exists in any parent scope, update it there
// If not found, create it in the current scope
func EnvSet(store EnvStore, envIdx int, name string, val Value) EnvStore {
	// First search for existing variable in scope chain
	currentIdx := envIdx
	for currentIdx >= 0 {
		varIdx := envFindVar(store, currentIdx, name)
		if varIdx >= 0 {
			// Found it, update in place (C# compatible way)
			env := store.Envs[currentIdx]
			v := env.Vars[varIdx]
			v.Val = val
			env.Vars[varIdx] = v
			store.Envs[currentIdx] = env
			return store
		}
		currentIdx = store.Envs[currentIdx].Parent
	}
	// Not found, create in current scope
	return EnvDefine(store, envIdx, name, val)
}

// EnvDefine defines a new variable in the current scope
// (always creates in current scope, even if exists in parent)
func EnvDefine(store EnvStore, envIdx int, name string, val Value) EnvStore {
	if envIdx < 0 || envIdx >= len(store.Envs) {
		return store
	}
	// Check if already exists in current scope
	varIdx := envFindVar(store, envIdx, name)
	if varIdx >= 0 {
		// Update existing (C# compatible way)
		existEnv := store.Envs[envIdx]
		v := existEnv.Vars[varIdx]
		v.Val = val
		existEnv.Vars[varIdx] = v
		store.Envs[envIdx] = existEnv
		return store
	}
	// Add new variable
	newVar := Variable{Name: name, Val: val}
	currEnv := store.Envs[envIdx]
	currEnv.Vars = append(currEnv.Vars, newVar)
	store.Envs[envIdx] = currEnv
	return store
}

// EnvSetLocal sets a variable in the current scope only
// (doesn't search parent scopes, always sets in current)
func EnvSetLocal(store EnvStore, envIdx int, name string, val Value) EnvStore {
	if envIdx < 0 || envIdx >= len(store.Envs) {
		return store
	}
	varIdx := envFindVar(store, envIdx, name)
	if varIdx >= 0 {
		// C# compatible way
		existEnv := store.Envs[envIdx]
		v := existEnv.Vars[varIdx]
		v.Val = val
		existEnv.Vars[varIdx] = v
		store.Envs[envIdx] = existEnv
		return store
	}
	newVar := Variable{Name: name, Val: val}
	currEnv := store.Envs[envIdx]
	currEnv.Vars = append(currEnv.Vars, newVar)
	store.Envs[envIdx] = currEnv
	return store
}

// EnvHas checks if a variable exists in the scope chain
func EnvHas(store EnvStore, envIdx int, name string) bool {
	var val Value
	var found bool
	val, found = EnvGet(store, envIdx, name)
	// Silence unused variable warning
	if val.Type < -1 {
		return found
	}
	return found
}

// EnvGetParent returns the parent environment index
func EnvGetParent(store EnvStore, envIdx int) int {
	if envIdx < 0 || envIdx >= len(store.Envs) {
		return -1
	}
	return store.Envs[envIdx].Parent
}

// InitGlobalEnv initializes the global environment with builtins
func InitGlobalEnv(store EnvStore) EnvStore {
	// Add builtin functions
	store = EnvDefine(store, 0, "print", NewBuiltin("print"))
	store = EnvDefine(store, 0, "len", NewBuiltin("len"))
	store = EnvDefine(store, 0, "range", NewBuiltin("range"))
	store = EnvDefine(store, 0, "str", NewBuiltin("str"))
	store = EnvDefine(store, 0, "int", NewBuiltin("int"))
	store = EnvDefine(store, 0, "float", NewBuiltin("float"))
	store = EnvDefine(store, 0, "bool", NewBuiltin("bool"))
	store = EnvDefine(store, 0, "abs", NewBuiltin("abs"))
	store = EnvDefine(store, 0, "min", NewBuiltin("min"))
	store = EnvDefine(store, 0, "max", NewBuiltin("max"))
	store = EnvDefine(store, 0, "type", NewBuiltin("type"))
	store = EnvDefine(store, 0, "append", NewBuiltin("append"))
	store = EnvDefine(store, 0, "input", NewBuiltin("input"))

	// Add builtin constants
	store = EnvDefine(store, 0, "True", NewBool(true))
	store = EnvDefine(store, 0, "False", NewBool(false))
	store = EnvDefine(store, 0, "None", NewNone())

	return store
}
