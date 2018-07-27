package server

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	e "github.com/pkg/errors"
)

//SET key 30 value

type command uint

const (
	set    command = iota
	get    command = iota
	del    command = iota
	hmset  command = iota
	hget   command = iota
	llen   command = iota
	lget   command = iota
	rpush  command = iota
	lpush  command = iota
	expire command = iota
	ttl    command = iota
)

var cmdMapToString = map[command]string{
	set:    "SET",
	get:    "GET",
	del:    "DEL",
	hmset:  "HMSET",
	hget:   "HGET",
	llen:   "LLEN",
	lget:   "LGET",
	rpush:  "RPUSH",
	lpush:  "LPUSH",
	expire: "EXPIRE",
	ttl:    "TTL",
}

func (c command) String() string {
	if val, ok := cmdMapToString[c]; ok {
		return val
	}
	return "error map cmd to string"
}

type cmd struct {
	Actions command
	Key     string
	Expire  int64
	Values  []string
}

func (s *Server) commandHandling(cmds []string) ([]byte, error) {

	if len(cmds) < 2 {
		return nil, errors.New("Expected at least 2 arguments")
	}

	cmd, err := commandParse(cmds)
	if err != nil {
		return nil, err
	}

	resp := make(chan resp, 1)
	defer close(resp)

	select {
	case s.requests <- request{cmd: cmd, response: resp}:
	case <-time.After(time.Second * 2):
		return nil, errors.New("Error send data requests channel is full")
	}

	if data, ok := <-resp; ok {
		return data.data, data.err
	}
	return nil, errors.New("Error get data from channel resp")
}

func commandParse(cmds []string) (cmd cmd, err error) {
	var cmdLen = len(cmds)

	if cmd.Actions, err = parseCommandType(cmds[0]); err != nil {
		return cmd, err
	}

	switch cmd.Actions {
	case del, get, llen, ttl:
		if cmdLen != 2 {
			return cmd, errors.New("Expected 2 arguments")
		}
		cmd.Key = cmds[1]
	default:
		switch cmd.Actions {
		case set:
			if cmdLen != 4 {
				return cmd, errors.New("Expected 4 arguments")
			}
			cmd.Values = cmds[3:4]

		case lpush, rpush:
			if cmdLen != 3 {
				return cmd, errors.New("Expected 3 arguments")
			}
			cmd.Key = cmds[1]
			cmd.Values = cmds[2:3]
			return cmd, nil
		case expire:
			if cmdLen != 3 {
				return cmd, errors.New("Expected 3 arguments")
			}
			cmd.Key = cmds[1]
			if cmd.Expire, err = parseDudation(cmds[2]); err != nil {
				return cmd, e.Wrap(err, "Invalid expiration time value")
			}
			return cmd, nil
		case lget:
			if cmdLen != 4 {
				return cmd, errors.New("Expected 4 arguments")
			}
			cmd.Key = cmds[1]
			cmd.Values = cmds[2:4]
			return cmd, nil
		default:
			if cmdLen < 4 {
				return cmd, errors.New("Expected at list 4 arguments")
			}
			cmd.Values = cmds[3:]
		}

		if cmd.Expire, err = parseDudation(cmds[2]); err != nil {
			return cmd, e.Wrap(err, "Invalid expiration time value")
		}
		cmd.Key = cmds[1]
	}

	return cmd, nil
}

//set(key string, value interface{}, expiration time.Duration)
func parseCommandType(cmd string) (command, error) {
	switch strings.ToLower(cmd) {
	// String storage
	case "set":
		return set, nil
	case "get":
		return get, nil
	case "del":
		return del, nil
	// HMAP storage
	case "hmset":
		return hmset, nil
	case "hget":
		return hget, nil
	// LinkedList storage
	case "rpush":
		return rpush, nil
	case "lpush":
		return lpush, nil
	case "llen":
		return llen, nil
	case "lget":
		return lget, nil
	case "expire":
		return expire, nil
	case "ttl":
		return ttl, nil
	default:
		return 0, fmt.Errorf("Unknown command %s", cmd)
	}
}

func parseDudation(expire string) (int64, error) {
	sec, err := strconv.ParseInt(expire, 10, 64)
	if err != nil {
		return 0, err
	}
	if sec < 0 {
		return -1, nil
	}
	return time.Now().Add(time.Duration(sec) * time.Second).UnixNano(), nil
}

/* func parseKey(key string) []byte {
	h := sha256.New()
	h.Write([]byte(key))
	return h.Sum(nil)
} */
