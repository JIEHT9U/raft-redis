package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/JIEHT9U/raft-redis/list"
	"github.com/pkg/errors"
)

//ErrTimeExpired err
var (
	ErrTimeExpired        = errors.New("key time expired")
	ErrKeyNotFound        = errors.New("key not found")
	ErrFildNotFound       = errors.New("fild not found")
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
	vocabulary map[string][]byte
	str        []byte
}

func convertToBytesAndHash(data ...interface{}) ([]byte, string, error) {
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

func checkKeyExpire(current time.Time, exp int64) (string, error) {
	if exp < 0 {
		return "infinity", nil
	}
	if time.Unix(0, exp).After(current) {
		return time.Unix(0, exp).Sub(current).String(), nil
	}
	return "", ErrTimeExpired
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

func (st *storages) ttl(key string) ([]byte, error) {
	if value, ok := st.data[key]; ok {
		lastExp, err := checkKeyExpire(time.Now(), value.expired)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("TTL %s", lastExp)), nil
	}
	return nil, ErrKeyNotFound
}

func (st *storages) del(key string) error {
	if _, ok := st.data[key]; ok {
		delete(st.data, key)
		return nil
	}
	return ErrKeyNotFound
}
