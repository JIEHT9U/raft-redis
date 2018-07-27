package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/JIEHT9U/raft-redis/list"
	"github.com/JIEHT9U/raft-redis/vocabulary"
	"github.com/pkg/errors"
)

//ErrTimeExpired err
var (
	ErrTimeExpired        = errors.New("key time expired")
	ErrKeyNotFound        = errors.New("key not found")
	ErrKeyHaveAnotherType = errors.New("key have another type")
)

// type storages struct {
// 	listStorage       map[string]list.Store
// 	vocabularyStorage map[string]vocabulary.Store
// 	stringsStorage    *str.Store
// }

type storages struct {
	data map[string]storage
}

type storage struct {
	expired    int64
	linkedList *list.LinkedList
	vocabulary vocabulary.Store
	str        string
}

func convertToBytesAndHash(data string) ([]byte, string, error) {
	var buf bytes.Buffer
	var h = sha256.New()
	if err := gob.NewEncoder(&buf).Encode(data); err != nil {
		return nil, "", err
	}
	if _, err := h.Write(buf.Bytes()); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), hex.EncodeToString(h.Sum(nil)), nil
}

func convertToStrong(data []byte) (string, error) {
	var srt string
	if err := gob.NewDecoder(bytes.NewReader(data)).Decode(&srt); err != nil {
		return "", err
	}
	return srt, nil
}

func checkKeyExpire(exp int64) (int, error) {
	if exp < 0 {
		return -1, nil
	}
	if time.Now().UnixNano() < exp {
		return time.Unix(0, exp-time.Now().UnixNano()).Second(), nil
	}
	return 0, ErrTimeExpired
}

func (st *storages) set(key, value string, expireTime int64) {
	st.data[key] = storage{str: value, expired: expireTime}
}

func (st *storages) expire(key string, expireTime int64) error {
	if data, ok := st.data[key]; ok {
		st.data[key] = storage{
			expired:    expireTime,
			str:        data.str,
			linkedList: data.linkedList,
			vocabulary: data.vocabulary,
		}
		return nil
	}
	return ErrKeyNotFound
}

func (st *storages) get(key string) ([]byte, error) {
	if value, ok := st.data[key]; ok {
		lastExp, err := checkKeyExpire(value.expired)
		if err != nil {
			return nil, err
		}
		if value.str == "" {
			return nil, ErrKeyHaveAnotherType
		}
		return []byte(fmt.Sprintf("%s %d", value.str, lastExp)), nil
	}
	return nil, ErrKeyNotFound
}

func (st *storages) del(key string) {
	delete(st.data, key)
}
