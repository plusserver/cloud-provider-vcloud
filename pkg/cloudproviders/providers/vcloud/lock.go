package vcloud

import (
	"fmt"
	"sync"
)

type keyLock struct {
	lock sync.Mutex
	keys map[string]*sync.Mutex
}

func newKeyLock() *keyLock {
	return &keyLock{keys: map[string]*sync.Mutex{}}
}

// Lock locks the key
func (l *keyLock) Lock(key string) {
	l.lock.Lock()
	lock := l.keys[key]
	if lock == nil {
		lock = &sync.Mutex{}
		l.keys[key] = lock
	}
	l.lock.Unlock()

	lock.Lock()
}

// Unlock unlocks the key
func (l *keyLock) Unlock(key string) {
	l.lock.Lock()
	defer l.lock.Unlock()

	lock := l.keys[key]
	if lock == nil {
		panic(fmt.Sprintf("unlock of unknown keyLock %s", key))
	}
	lock.Unlock()
}
