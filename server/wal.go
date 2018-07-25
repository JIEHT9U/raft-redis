package server

import (
	"fmt"
	"os"

	"github.com/coreos/etcd/raft/raftpb"
	"github.com/coreos/etcd/wal"
	"github.com/coreos/etcd/wal/walpb"
)

// openWAL returns a WAL ready for reading.
func (s *Server) openWAL(snapshot *raftpb.Snapshot) (*wal.WAL, error) {
	if !wal.Exist(s.raft.waldir) {
		if err := os.Mkdir(s.raft.waldir, 0750); err != nil {
			return nil, fmt.Errorf("cannot create dir for wal (%v)", err)
		}

		w, err := wal.Create(s.logger, s.raft.waldir, nil)
		if err != nil {
			return nil, fmt.Errorf("create wal error (%v)", err)
		}
		w.Close()
	}

	walsnap := walpb.Snapshot{}
	if snapshot != nil {
		walsnap.Index, walsnap.Term = snapshot.Metadata.Index, snapshot.Metadata.Term
	}
	s.logger.Infof("loading WAL at term %d and index %d", walsnap.Term, walsnap.Index)

	w, err := wal.Open(s.logger, s.raft.waldir, walsnap)
	if err != nil {
		return nil, fmt.Errorf("error loading wal (%v)", err)
	}

	return w, nil
}

// replayWAL  повторяет записи WAL в экземпляр плота.
func (s *Server) replayWAL() (*wal.WAL, error) {
	s.logger.Infof("replaying WAL of member %d", s.raft.id)

	////Пробуем заргузить снапшот
	snapshot, err := s.loadSnapshot()
	if err != nil {
		return nil, err
	}

	//WAL используется для возврата данных с диска в память
	//openWAL возвращает WALL, готовый для чтения.
	w, err := s.openWAL(snapshot)
	if err != nil {
		return nil, err
	}

	//читаем данные с диска c помошью WAL
	_, st, ents, err := w.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read WAL (%v)", err)
	}

	//Загружаем данные в storage если snapshot не пуст
	if snapshot != nil {
		s.raft.raftStorage.ApplySnapshot(*snapshot)

		s.raft.confState = snapshot.Metadata.ConfState
		s.raft.snapshotIndex = snapshot.Metadata.Index
		s.raft.appliedIndex = snapshot.Metadata.Index
	}
	s.raft.raftStorage.SetHardState(st)

	//добавьте в хранилище, чтобы raft стартовал в нужном месте в журнале
	s.raft.raftStorage.Append(ents)

	// send nil once lastIndex is published so client knows commit channel is current
	// отправить nil после опубликования последнего индекса, так что клиент знает, что канал фиксации текущий
	if len(ents) > 0 {
		s.raft.lastIndex = ents[len(ents)-1].Index
	} else {
		s.raft.commitC <- nil
	}
	return w, nil
}
