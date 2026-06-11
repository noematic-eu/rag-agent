package lexical

import (
	"strings"
)

func diskGetOptional(kv KV, key string) ([]byte, error) {
	data, err := kv.Get(key)
	if err != nil {
		if diskIsMissingKey(err) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

func diskIsMissingKey(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found")
}
