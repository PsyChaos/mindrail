// Package nativealloc is a test helper for observing actual Tree-sitter C
// allocations through its official SetAllocator API. Production does not import it.
package nativealloc

/*
#include <stdlib.h>
*/
import "C"

import (
	"sync"
	"unsafe"

	ts "github.com/tree-sitter/go-tree-sitter"
)

// Tracker must be installed only while no other goroutine uses Tree-sitter.
// Call Restore after all measured objects have been closed.
type Tracker struct {
	mu   sync.Mutex
	live map[unsafe.Pointer]uint
}

func Install() *Tracker {
	t := &Tracker{live: make(map[unsafe.Pointer]uint)}
	ts.SetAllocator(t.malloc, t.calloc, t.realloc, t.free)
	return t
}

func (t *Tracker) Restore() { ts.SetAllocator(nil, nil, nil, nil) }

func (t *Tracker) Live() (count int, bytes uint) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, size := range t.live {
		bytes += size
	}
	return len(t.live), bytes
}

func (t *Tracker) malloc(size uint) unsafe.Pointer {
	p := C.malloc(C.size_t(size))
	t.mu.Lock()
	if p != nil {
		t.live[p] = size
	}
	t.mu.Unlock()
	return p
}

func (t *Tracker) calloc(num, size uint) unsafe.Pointer {
	p := C.calloc(C.size_t(num), C.size_t(size))
	t.mu.Lock()
	if p != nil {
		t.live[p] = num * size
	}
	t.mu.Unlock()
	return p
}

func (t *Tracker) realloc(old unsafe.Pointer, size uint) unsafe.Pointer {
	t.mu.Lock()
	defer t.mu.Unlock()
	p := C.realloc(old, C.size_t(size))
	if p != nil || size == 0 {
		delete(t.live, old)
	}
	if p != nil {
		t.live[p] = size
	}
	return p
}

func (t *Tracker) free(p unsafe.Pointer) {
	t.mu.Lock()
	// The binding also frees callback C strings allocated directly with libc;
	// those are not registered by ts_set_allocator and simply have no entry.
	delete(t.live, p)
	t.mu.Unlock()
	C.free(p)
}
