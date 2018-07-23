package server

import (
	"github.com/JIEHT9U/raft-redis/list"
	"github.com/JIEHT9U/raft-redis/str"
	"github.com/JIEHT9U/raft-redis/vocabulary"
)

type storages struct {
	listStorage       map[string]list.Store
	vocabularyStorage map[string]vocabulary.Store
	stringsStorage    *str.Store
}

func (s *Server) readCommits() {

}
