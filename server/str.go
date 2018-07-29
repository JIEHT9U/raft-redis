package server

import "time"

func (st *storages) getSTR(key string) ([]byte, error) {
	if s, ok := st.data[key]; ok {
		if s.str == nil {
			return nil, ErrKeyHaveAnotherType
		}
		if _, err := checkKeyExpire(time.Now(), s.expired); err != nil {
			return nil, err
		}
		return s.str, nil
	}
	return nil, ErrKeyNotFound
}

func (st *storages) set(key, value string) error {
	_, err := st.getSTR(key)
	if err == ErrKeyNotFound {
		st.data[key] = storage{str: []byte(value), expired: -1}
		return nil
	}
	if err == nil {
		st.data[key] = storage{str: []byte(value), expired: -1}
	}
	return err
}

func (st *storages) get(key string) ([]byte, error) {
	data, err := st.getSTR(key)
	if err != nil {
		return nil, err
	}
	return data, nil
}
