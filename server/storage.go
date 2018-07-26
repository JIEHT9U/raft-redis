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
	ErrTimeExpired = errors.New("key time expired")
	ErrKeyNotFound = errors.New("key not found")
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

func (st *storages) lpush(key string, value string) error {

	dataBytes, hash, err := convertToBytesAndHash(value)
	if err != nil {
		return err
	}

	if s, ok := st.data[key]; ok {
		s.linkedList.AddFirst(hash, dataBytes)
		return nil
	}

	st.data[key] = storage{linkedList: list.Create().AddFirst(hash, dataBytes)}

	return nil
}

func (st *storages) rpush(key string, value string) error {

	dataBytes, hash, err := convertToBytesAndHash(value)
	if err != nil {
		return err
	}

	if s, ok := st.data[key]; ok {
		s.linkedList.AddLast(hash, dataBytes)
		return nil
	}

	st.data[key] = storage{linkedList: list.Create().AddLast(hash, dataBytes)}

	return nil
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

func (st *storages) get(key string) ([]byte, error) {

	if value, ok := st.data[key]; ok {

		lastExp, err := checkKeyExpire(value.expired)
		if err != nil {
			return nil, err
		}
		return []byte(fmt.Sprintf("%s %d", value.str, lastExp)), nil
	}
	return nil, ErrKeyNotFound
}

func (st *storages) del(key string) {
	delete(st.data, key)
}
