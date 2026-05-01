package runtime

import (
	"hash/fnv"
	"reflect"
	"sync"
)

// WireNamer lets a message type freeze its wire identifier across Go-type
// renames or package moves. Optional: if a Message implements WireNamer,
// the framework hashes WireName() instead of the Go type name.
type WireNamer interface {
	WireName() string
}

// WireID returns the deterministic 64-bit wire identifier for message
// type M. Use this when you need the id without a Message instance —
// e.g. inside a generic registration helper. The id is the FNV-1a 64-bit
// hash of either M.WireName() (if M implements WireNamer) or
// reflect.TypeOf(*new(M)).String() (the Go type name, fully qualified).
func WireID[M Message]() uint64 {
	var zero M
	t := reflect.TypeOf(zero)
	if t == nil {
		// M is an interface or otherwise non-instantiable; reject explicitly.
		panic("runtime: WireID called with non-instantiable type parameter")
	}
	// Allocate a non-nil instance to call any optional WireName() method.
	// For pointer types, `var zero M` is typed nil and would panic on a
	// method call — reflect.New(t.Elem()) produces a fresh *T.
	var probe any
	if t.Kind() == reflect.Pointer {
		probe = reflect.New(t.Elem()).Interface()
	} else {
		probe = zero
	}
	if wn, ok := probe.(WireNamer); ok {
		return hashString(wn.WireName())
	}
	return hashString(t.String())
}

// wireIDOf returns the wire id for a runtime-typed Message. Used on the
// send path. Cached per reflect.Type so the cost is a single sync.Map
// load after the first hash.
func wireIDOf(m Message) uint64 {
	t := reflect.TypeOf(m)
	if cached, ok := wireIDCache.Load(t); ok {
		return cached.(uint64)
	}
	if wn, ok := m.(WireNamer); ok {
		id := hashString(wn.WireName())
		wireIDCache.Store(t, id)
		return id
	}
	id := hashString(t.String())
	wireIDCache.Store(t, id)
	return id
}

var wireIDCache sync.Map // map[reflect.Type]uint64

func hashString(s string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return h.Sum64()
}
