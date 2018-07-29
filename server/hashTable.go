package server

import (
	"bytes"
	"time"
)

func (st *storages) getHashTable(key string) (map[string][]byte, error) {
	if s, ok := st.data[key]; ok {
		if s.vocabulary == nil {
			return nil, ErrKeyHaveAnotherType
		}
		if _, err := checkKeyExpire(time.Now(), s.expired); err != nil {
			return nil, err
		}
		return s.vocabulary, nil
	}
	return nil, ErrKeyNotFound
}

func (st *storages) hset(key, field string, value string) error {

	ht, err := st.getHashTable(key)
	if err == ErrKeyNotFound {
		st.data[key] = storage{expired: -1, vocabulary: map[string][]byte{
			field: []byte(value),
		}}
		return nil
	}
	if err == nil {
		ht[field] = []byte(value)
		return nil
	}

	return err
}

func (st *storages) hget(key, field string) ([]byte, error) {

	ht, err := st.getHashTable(key)
	if err != nil {
		return nil, err
	}

	if val, ok := ht[field]; ok {
		return val, nil
	}

	return nil, ErrFildNotFound
}

func (st *storages) hgetall(key string) ([]byte, error) {

	ht, err := st.getHashTable(key)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	for key, value := range ht {
		if _, err := buf.WriteString("\n\r" + key + ":" + string(value)); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}
