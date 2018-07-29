package server

import (
	"bytes"
	"fmt"
	"strconv"
	"time"

	"github.com/JIEHT9U/raft-redis/list"
	"github.com/pkg/errors"
)

func (st *storages) getLinkedList(key string) (*list.LinkedList, error) {
	if s, ok := st.data[key]; ok {
		if s.linkedList == nil {
			return nil, ErrKeyHaveAnotherType
		}
		if _, err := checkKeyExpire(time.Now(), s.expired); err != nil {
			return nil, err
		}
		return s.linkedList, nil
	}
	return nil, ErrKeyNotFound
}

func (st *storages) lget(key string, start, end string) ([]byte, error) {

	startInt, err := strconv.Atoi(start)
	if err != nil {
		return nil, err
	}

	endInt, err := strconv.Atoi(end)
	if err != nil {
		return nil, err
	}

	ll, err := st.getLinkedList(key)
	if err != nil {
		return nil, err
	}

	if ll.Count() <= 0 {
		return nil, errors.New("linked list empty")
	}

	var buf bytes.Buffer
	var starPositions int
	for n := range ll.Next() {
		if starPositions >= startInt && chechEndPos(starPositions, endInt) {
			str, err := convertToStrong(n.Value)
			if err != nil {
				return nil, err
			}
			if _, err := buf.WriteString("\n\r" + str); err != nil {
				return nil, err
			}
		}
		starPositions++
	}
	return buf.Bytes(), nil
}

func chechEndPos(starPositions, end int) bool {
	if end < 0 {
		return true
	}
	return starPositions <= end
}

func (st *storages) llen(key string) ([]byte, error) {
	ll, err := st.getLinkedList(key)
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("len: %d", ll.Count())), nil
}

func (st *storages) rpush(key string, value []string) error {

	if s, ok := st.data[key]; ok {
		if s.linkedList == nil {
			return ErrKeyHaveAnotherType
		}

		for _, val := range value {
			dataBytes, hash, err := convertToBytesAndHash(val)
			if err != nil {
				return err
			}
			s.linkedList.AddLast(hash, dataBytes)
		}

		return nil
	}

	ll := list.Create()
	for _, val := range value {
		dataBytes, hash, err := convertToBytesAndHash(val)
		if err != nil {
			return err
		}
		ll.AddLast(hash, dataBytes)
	}

	st.data[key] = storage{linkedList: ll, expired: -1}
	return nil
}

func (st *storages) lpush(key string, value []string) error {

	if s, ok := st.data[key]; ok {
		if s.linkedList == nil {
			return ErrKeyHaveAnotherType
		}

		for _, val := range value {
			dataBytes, hash, err := convertToBytesAndHash(val)
			if err != nil {
				return err
			}
			s.linkedList.AddFirst(hash, dataBytes)
		}

		return nil
	}

	ll := list.Create()
	for _, val := range value {
		dataBytes, hash, err := convertToBytesAndHash(val)
		if err != nil {
			return err
		}
		ll.AddFirst(hash, dataBytes)
	}

	st.data[key] = storage{linkedList: ll, expired: -1}
	return nil
}
