package lexical

// KVPair is a key/value entry from a prefix scan.
type KVPair struct {
	Key   string
	Value []byte
}

// KV is the minimal key-value API used by the on-disk lexical index.
type KV interface {
	Put(key string, value []byte) error
	Get(key string) ([]byte, error)
	Delete(key string) error
	ScanPrefix(prefix string) ([]KVPair, error)
}
