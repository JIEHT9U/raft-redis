package str

import (
	"errors"
	"fmt"
	"time"
)

//ErrTimeExpired err
var (
	ErrTimeExpired = errors.New("key time expired")
	ErrKeyNotFound = errors.New("key not found")
)

//Store type
type Store struct {
	data map[string]str
}

//New return pointer Store struct
func New() *Store {
	return &Store{
		data: make(map[string]str),
	}
}

type str struct {
	data    string
	expires int64
}

//Set set value
func (s *Store) Set(key, value string, expireTime int64) {
	s.data[key] = str{data: value, expires: expireTime}
}

//Get set value
func (s *Store) Get(key string) ([]byte, error) {
	if value, ok := s.data[key]; ok {
		if value.expires < 0 {
			return []byte(fmt.Sprintf("%s %d", value.data, -1)), nil
		}

		if time.Now().UnixNano() < value.expires {
			return []byte(fmt.Sprintf("%s %d", value.data, time.Unix(0, value.expires-time.Now().UnixNano()).Second())), nil
		}
		return nil, ErrTimeExpired
	}
	return nil, ErrKeyNotFound
}

//Del set value
func (s *Store) Del(key string) {
	delete(s.data, key)
}
