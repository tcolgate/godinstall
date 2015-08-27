package main

import (
	"sync"
)

// SafeMap is a thread safe map stolen from somwhere else
type SafeMap struct {
	sync.RWMutex
	bm map[interface{}]interface{}
}

// NewSafeMap creates a thread safe map
func NewSafeMap() *SafeMap {
	return &SafeMap{
		RWMutex: sync.RWMutex{},
		bm:      make(map[interface{}]interface{}),
	}
}

//Get from maps return the k's value
func (m *SafeMap) Get(k interface{}) interface{} {
	m.RLock()
	defer m.RUnlock()
	if val, ok := m.bm[k]; ok {
		return val
	}
	return nil
}

// Set maps the given key and value. Returns false
// if the key is already in the map and changes nothing.
func (m *SafeMap) Set(k interface{}, v interface{}) bool {
	m.Lock()
	defer m.Unlock()
	if val, ok := m.bm[k]; !ok {
		m.bm[k] = v
	} else if val != v {
		m.bm[k] = v
	} else {
		return false
	}
	return true
}

// Check returns true if k is exist in the map.
func (m *SafeMap) Check(k interface{}) bool {
	m.RLock()
	defer m.RUnlock()
	if _, ok := m.bm[k]; !ok {
		return false
	}
	return true
}

// Delete removes a key from a map
func (m *SafeMap) Delete(k interface{}) {
	m.Lock()
	defer m.Unlock()
	delete(m.bm, k)
}
