package server

import (
	"bytes"
	"strconv"

	"github.com/JIEHT9U/raft-redis/list"
	"github.com/pkg/errors"
)

func (st *storages) getLinkedList(key string) (*list.LinkedList, error) {
	if s, ok := st.data[key]; ok {
		if s.linkedList == nil {
			return nil, ErrKeyHaveAnotherType
		}
		if _, err := checkKeyExpire(s.expired); err != nil {
			return nil, err
		}
		return s.linkedList, nil
	}
	return nil, ErrKeyNotFound
}

func (st *storages) lget(key string, start, end int) ([]byte, error) {

	ll, err := st.getLinkedList(key)
	if err != nil {
		return nil, err
	}

	if ll.Count() <= 0 {
		return nil, errors.New("linked list empty")
	}

	var buf bytes.Buffer

	for n := range ll.Next() {
		str, err := convertToStrong(n.Value)
		if err != nil {
			return nil, err
		}
		if _, err := buf.WriteString(str + "\n\r"); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

func (st *storages) llen(key string) ([]byte, error) {
	ll, err := st.getLinkedList(key)
	if err != nil {
		return nil, err
	}
	return []byte("len:" + strconv.FormatInt(ll.Count(), 64)), nil
}

func (st *storages) rpush(key string, value string) error {

	dataBytes, hash, err := convertToBytesAndHash(value)
	if err != nil {
		return err
	}

	if s, ok := st.data[key]; ok {
		if s.linkedList == nil {
			return ErrKeyHaveAnotherType
		}
		s.linkedList.AddLast(hash, dataBytes)
		return nil
	}
	st.data[key] = storage{linkedList: list.Create().AddLast(hash, dataBytes)}
	return nil
}

func (st *storages) lpush(key string, value string) error {

	dataBytes, hash, err := convertToBytesAndHash(value)
	if err != nil {
		return err
	}

	if s, ok := st.data[key]; ok {
		if s.linkedList == nil {
			return ErrKeyHaveAnotherType
		}
		s.linkedList.AddFirst(hash, dataBytes)
		return nil
	}

	st.data[key] = storage{linkedList: list.Create().AddFirst(hash, dataBytes)}
	return nil
}
