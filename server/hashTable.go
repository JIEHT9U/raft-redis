package server

import "bytes"

func (st *storages) getHashTable(key string) (map[string][]byte, error) {
	if s, ok := st.data[key]; ok {
		if s.vocabulary == nil {
			return nil, ErrKeyHaveAnotherType
		}
		if _, err := checkKeyExpire(s.expired); err != nil {
			return nil, err
		}
		return s.vocabulary, nil
	}
	return nil, ErrKeyNotFound
}

func (st *storages) hset(key, field string, value string) error {
	_, hash, err := convertToBytesAndHash(field)
	if err != nil {
		return nil
	}

	ht, err := st.getHashTable(key)
	if err == ErrKeyNotFound {
		st.data[key] = storage{expired: -1, vocabulary: map[string][]byte{
			hash: []byte(value),
		}}
		return nil
	}
	if err == nil {
		ht[hash] = []byte(value)
	}

	return err
}

func (st *storages) hget(key, field string) ([]byte, error) {
	_, hash, err := convertToBytesAndHash(field)
	if err != nil {
		return nil, nil
	}

	ht, err := st.getHashTable(key)
	if err != nil {
		return nil, err
	}

	if val, ok := ht[hash]; ok {
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
	if _, err := buf.WriteString("\n\r"); err != nil {
		return nil, err
	}
	for key, value := range ht {
		str, err := convertToStrong(value)
		if err != nil {
			return nil, err
		}
		if _, err := buf.WriteString(key + ":" + str + "\n\r"); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}
