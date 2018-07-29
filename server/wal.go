// WAL используется для возврата данных с диска в память (например, в raftStorage) когда узел запускается, перезапускается.
// Все запросы, которые поступают к мастеру, записываются в WAL и raftStorage.

// Позже, когда они совершены (предложение принято кворумом), они отправляются по каналу фиксации,
// который затем обновляет хранилище KV.

// WAL также имеет возможность хранить моментальные снимки, где при повторном воспроизведении вы
// можете открыть WAL из определенного моментального снимка и прочитать записи с этого момента.

// Файлы WAL имеют 64 М каждый, а также хранят HardState отдельно от записей.

package server

import (
	"fmt"
	"os"

	"github.com/coreos/etcd/raft/raftpb"
	"github.com/coreos/etcd/wal"
	"github.com/coreos/etcd/wal/walpb"
	"go.uber.org/zap"
)

// openWAL returns a WAL ready for reading.
func (s *Server) openWAL(snapshot *raftpb.Snapshot) (*wal.WAL, error) {
	if !wal.Exist(s.raft.waldir) {
		if err := os.Mkdir(s.raft.waldir, 0750); err != nil {
			return nil, fmt.Errorf("cannot create dir for wal (%v)", err)
		}

		w, err := wal.Create(zap.NewExample(), s.raft.waldir, nil)
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

	w, err := wal.Open(zap.NewExample(), s.raft.waldir, walsnap)
	if err != nil {
		return nil, fmt.Errorf("error loading wal (%v)", err)
	}

	return w, nil
}

// replayWAL  повторяет записи WAL в экземпляр плота.
func (s *Server) replayWAL() (*wal.WAL, error) {
	s.logger.Infof("replaying WAL of member %d", s.raft.id)

	//Пробуем загрузить снапшот
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

	if err := s.raft.raftStorage.SetHardState(st); err != nil {
		return nil, err
	}

	//добавьте в хранилище, чтобы raft стартовал в нужном месте в журнале
	if err := s.raft.raftStorage.Append(ents); err != nil {
		return nil, err
	}

	// send nil once lastIndex is published so client knows commit channel is current
	// отправить nil после опубликования последнего индекса, так что клиент знает, что канал фиксации текущий
	if len(ents) > 0 {
		s.raft.lastIndex = ents[len(ents)-1].Index
	} else {
		s.raft.commitC <- nil
	}
	return w, nil
}
