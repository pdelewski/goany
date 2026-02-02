package hmap

// HashMap key type constants
const KeyTypeString int = 1
const KeyTypeInt int = 2
const KeyTypeBool int = 3

// HashMap implements a hash table with open addressing and linear probing.
// Keys and values are stored as interface{} to support all types.
// KeyType determines which hash/equality function to use.
type HashMap struct {
	Keys     []interface{}
	Values   []interface{}
	Occupied []bool
	Size     int
	Capacity int
	KeyType  int
}

func newHashMap(keyType int) HashMap {
	var m HashMap
	m.Capacity = 16
	m.Size = 0
	m.KeyType = keyType
	m.Keys = make([]interface{}, 16)
	m.Values = make([]interface{}, 16)
	m.Occupied = make([]bool, 16)
	return m
}

func hashString(s string) int {
	hash := 5381
	i := 0
	for i < len(s) {
		c := int(s[i])
		hash = hash*33 + c
		i = i + 1
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}

func hashInt(n int) int {
	h := n * 1103515245
	if h < 0 {
		h = -h
	}
	return h
}

func hashBool(b bool) int {
	if b {
		return 1
	}
	return 0
}

func hashMapHash(key interface{}, keyType int, capacity int) int {
	h := 0
	if keyType == KeyTypeString {
		h = hashString(key.(string))
	}
	if keyType == KeyTypeInt {
		h = hashInt(key.(int))
	}
	if keyType == KeyTypeBool {
		h = hashBool(key.(bool))
	}
	return h % capacity
}

func keysEqual(a interface{}, b interface{}, keyType int) bool {
	if keyType == KeyTypeString {
		return a.(string) == b.(string)
	}
	if keyType == KeyTypeInt {
		return a.(int) == b.(int)
	}
	if keyType == KeyTypeBool {
		return a.(bool) == b.(bool)
	}
	return false
}

func hashMapGrow(m HashMap) HashMap {
	oldKeys := m.Keys
	oldValues := m.Values
	oldOccupied := m.Occupied
	oldCapacity := m.Capacity

	newCap := oldCapacity * 2
	var grown HashMap
	grown.Capacity = newCap
	grown.Size = 0
	grown.KeyType = m.KeyType
	grown.Keys = make([]interface{}, newCap)
	grown.Values = make([]interface{}, newCap)
	grown.Occupied = make([]bool, newCap)

	i := 0
	for i < oldCapacity {
		if oldOccupied[i] {
			grown = hashMapSet(grown, oldKeys[i], oldValues[i])
		}
		i = i + 1
	}
	return grown
}

func hashMapSet(m HashMap, key interface{}, value interface{}) HashMap {
	// Check load factor - grow if > 75%
	if (m.Size+1)*4 > m.Capacity*3 {
		m = hashMapGrow(m)
	}

	idx := hashMapHash(key, m.KeyType, m.Capacity)
	i := 0
	for i < m.Capacity {
		pos := (idx + i) % m.Capacity
		if !m.Occupied[pos] {
			m.Keys[pos] = key
			m.Values[pos] = value
			m.Occupied[pos] = true
			m.Size = m.Size + 1
			return m
		}
		if keysEqual(m.Keys[pos], key, m.KeyType) {
			m.Values[pos] = value
			return m
		}
		i = i + 1
	}
	return m
}

func hashMapGet(m HashMap, key interface{}) interface{} {
	if m.Capacity == 0 {
		return nil
	}
	idx := hashMapHash(key, m.KeyType, m.Capacity)
	i := 0
	for i < m.Capacity {
		pos := (idx + i) % m.Capacity
		if !m.Occupied[pos] {
			return nil
		}
		if keysEqual(m.Keys[pos], key, m.KeyType) {
			return m.Values[pos]
		}
		i = i + 1
	}
	return nil
}

func hashMapDelete(m HashMap, key interface{}) HashMap {
	if m.Capacity == 0 {
		return m
	}
	idx := hashMapHash(key, m.KeyType, m.Capacity)
	i := 0
	for i < m.Capacity {
		pos := (idx + i) % m.Capacity
		if !m.Occupied[pos] {
			return m
		}
		if keysEqual(m.Keys[pos], key, m.KeyType) {
			// Found - remove and rehash subsequent entries
			m.Occupied[pos] = false
			m.Size = m.Size - 1
			// Rehash entries after the deleted one
			next := (pos + 1) % m.Capacity
			for m.Occupied[next] {
				rehashKey := m.Keys[next]
				rehashVal := m.Values[next]
				m.Occupied[next] = false
				m.Size = m.Size - 1
				m = hashMapSet(m, rehashKey, rehashVal)
				next = (next + 1) % m.Capacity
			}
			return m
		}
		i = i + 1
	}
	return m
}

func hashMapLen(m HashMap) int {
	return m.Size
}
