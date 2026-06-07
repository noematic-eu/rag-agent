package f4kvs

/*
#cgo CFLAGS: -I${SRCDIR}/include
#cgo LDFLAGS: -L${SRCDIR}/../../lib -lf4kvs_ffi -Wl,-rpath,${SRCDIR}/../../lib
#include "f4kvs.h"
#include <stdlib.h>
*/
import "C"

import (
	"errors"
	"fmt"
	"unsafe"
)

const (
	resultSuccess         = C.F4KVS_SUCCESS
	resultInvalidArgument = C.F4KVS_ERROR_INVALID_ARGUMENT
	resultNotFound        = C.F4KVS_ERROR_NOT_FOUND
	resultStorage         = C.F4KVS_ERROR_STORAGE
)

var (
	ErrNotFound = errors.New("f4kvs: key not found")
)

// KVPair is a key/value entry returned by prefix scans.
type KVPair struct {
	Key   string
	Value []byte
}

// Store wraps the f4kvs LSM engine via C FFI.
type Store struct {
	handle *C.F4KvsEngine
}

// Open opens a persistent f4kvs store at the given data directory.
func Open(path string) (*Store, error) {
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	handle := C.f4kvs_engine_open(cPath)
	if handle == nil {
		return nil, fmt.Errorf("f4kvs: open failed: %s", lastError())
	}

	return &Store{handle: handle}, nil
}

// Put stores a binary value for key.
func (s *Store) Put(key string, value []byte) error {
	if s == nil || s.handle == nil {
		return errors.New("f4kvs: store is closed")
	}

	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))

	var cValue *C.uint8_t
	if len(value) > 0 {
		cValue = (*C.uint8_t)(unsafe.Pointer(&value[0]))
	}

	result := C.f4kvs_engine_put_bytes(s.handle, cKey, cValue, C.size_t(len(value)))
	return resultError(result, "put")
}

// Get loads a binary value for key.
func (s *Store) Get(key string) ([]byte, error) {
	if s == nil || s.handle == nil {
		return nil, errors.New("f4kvs: store is closed")
	}

	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))

	var valueOut *C.uint8_t
	var valueLen C.size_t

	result := C.f4kvs_engine_get_bytes(s.handle, cKey, &valueOut, &valueLen)
	if result == resultNotFound {
		return nil, ErrNotFound
	}
	if result != resultSuccess {
		return nil, fmt.Errorf("f4kvs: get failed: %s", lastError())
	}
	if valueOut == nil || valueLen == 0 {
		return []byte{}, nil
	}

	value := C.GoBytes(unsafe.Pointer(valueOut), C.int(valueLen))
	C.f4kvs_bytes_free(valueOut)
	return value, nil
}

// ScanPrefix returns all key/value pairs whose keys start with prefix.
func (s *Store) ScanPrefix(prefix string) ([]KVPair, error) {
	if s == nil || s.handle == nil {
		return nil, errors.New("f4kvs: store is closed")
	}

	cPrefix := C.CString(prefix)
	defer C.free(unsafe.Pointer(cPrefix))

	var scanResult C.F4KvsScanResult
	result := C.f4kvs_engine_scan_prefix(s.handle, cPrefix, &scanResult)
	if result != resultSuccess {
		return nil, fmt.Errorf("f4kvs: scan prefix failed: %s", lastError())
	}
	defer C.f4kvs_scan_result_free(&scanResult)

	if scanResult.count == 0 || scanResult.pairs == nil {
		return nil, nil
	}

	pairs := unsafe.Slice(scanResult.pairs, scanResult.count)
	out := make([]KVPair, 0, len(pairs))
	for _, pair := range pairs {
		key := C.GoString(pair.key)
		value := C.GoBytes(unsafe.Pointer(pair.value), C.int(pair.value_len))
		out = append(out, KVPair{Key: key, Value: value})
	}

	return out, nil
}

// Delete removes a key from the store.
func (s *Store) Delete(key string) error {
	if s == nil || s.handle == nil {
		return errors.New("f4kvs: store is closed")
	}

	cKey := C.CString(key)
	defer C.free(unsafe.Pointer(cKey))

	result := C.f4kvs_engine_delete(s.handle, cKey)
	if result == resultNotFound {
		return ErrNotFound
	}
	return resultError(result, "delete")
}

// Compact runs LSM compaction (Badger Flatten equivalent).
func (s *Store) Compact() error {
	if s == nil || s.handle == nil {
		return errors.New("f4kvs: store is closed")
	}

	result := C.f4kvs_engine_compact(s.handle)
	return resultError(result, "compact")
}

// Close shuts down and frees the underlying engine.
func (s *Store) Close() error {
	if s == nil || s.handle == nil {
		return nil
	}

	if result := C.f4kvs_engine_close(s.handle); result != resultSuccess {
		return fmt.Errorf("f4kvs: close failed: %s", lastError())
	}

	C.f4kvs_engine_free(s.handle)
	s.handle = nil
	return nil
}

func resultError(result C.F4KvsResult, op string) error {
	if result == resultSuccess {
		return nil
	}
	if result == resultNotFound {
		return ErrNotFound
	}
	return fmt.Errorf("f4kvs: %s failed: %s", op, lastError())
}

func lastError() string {
	ptr := C.f4kvs_get_last_error()
	if ptr == nil {
		return "unknown error"
	}
	return C.GoString(ptr)
}
