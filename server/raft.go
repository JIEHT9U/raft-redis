package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	ll "github.com/JIEHT9U/raft-redis/logger"
	"github.com/coreos/etcd/etcdserver/api/rafthttp"
	"github.com/coreos/etcd/etcdserver/api/snap"
	stats "github.com/coreos/etcd/etcdserver/api/v2stats"
	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/coreos/etcd/pkg/types"
	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/coreos/etcd/wal"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

// A key-value stream backed by raft
type raftNode struct {
	confChangeC <-chan raftpb.ConfChange // proposed cluster config changes
	commitC     chan *string             // entries committed to log

	// electionTick - количество вызовов Node.Tick, которые должны проходить между
	// выборы. То есть, если follower не получает никакого сообщения от
	// лидер текущего термина до истечения срока действия ElectionTick, он станет
	// кандидат и начать выборы. Выделение должно быть больше, чем
	// HeartbeatTick. Мы предлагаем ElectionTick = 10 * HeartbeatTick, чтобы избежать
	// ненужное переключение лидера.
	electionTick int
	// heartbeatTick - количество вызовов Node.Tick, которые должны проходить между
	// сердцебиение. То есть лидер посылает сообщения о сердцебиении для поддержания своих
	// лидерство каждый тибетский HeartbeatTick.
	heartbeatTick int
	id            int      // client ID for raft session
	peers         []string // raft peer URLs
	join          bool     // node is joining an existing cluster
	waldir        string   // path to WAL directory
	snapdir       string   // path to snapshot directory
	lastIndex     uint64   // index of log at start

	confState     raftpb.ConfState
	snapshotIndex uint64

	// appliedIndex последний применяемый индекс. Его следует устанавливать только при перезапуске
	// RAFT. RAFT не будет возвращать записи в приложение, меньшие или равные
	// appliedIndex. Если appliedIndex не используется при перезапуске, Raft может вернуться назад
	// применяемые записи. Это очень зависимая от приложения конфигурация.
	appliedIndex uint64

	// raft backing for the commit/error channel
	node        raft.Node
	raftStorage *raft.MemoryStorage
	wal         *wal.WAL

	snapshotter      *snap.Snapshotter
	snapshotterReady chan *snap.Snapshotter // signals when snapshotter is ready

	snapCount uint64
	transport *rafthttp.Transport
}

func (s *Server) entriesToApply(ents []raftpb.Entry) ([]raftpb.Entry, error) {
	var nents = make([]raftpb.Entry, 0)
	if len(ents) <= 0 {
		return nents, nil
	}
	firstIdx := ents[0].Index
	if firstIdx > s.raft.appliedIndex+1 {
		return nil, fmt.Errorf("first index of committed entry[%d] should <= progress.appliedIndex[%d]+1", firstIdx, s.raft.appliedIndex)
	}
	if s.raft.appliedIndex-firstIdx+1 < uint64(len(ents)) {
		nents = ents[s.raft.appliedIndex-firstIdx+1:]
	}
	return nents, nil
}

func creteSnapDirIfNotExist(path string) error {
	if !fileutil.Exist(path) {
		if err := os.MkdirAll(path, 0750); err != nil {
			return fmt.Errorf("cannot create dir for snapshot (%v)", err)
		}
	}
	return nil
}

//InitRaft init default RAFT param
func (s *Server) InitRaft() error {
	var oldwal = wal.Exist(s.raft.waldir)
	var err error

	//Создаём дерикротию где будут храниться snapshots
	if err = creteSnapDirIfNotExist(s.raft.snapdir); err != nil {
		return err
	}
	raft.SetLogger(raft.Logger(ll.NewZapLoggerRaft(s.logger)))
	// raft.SetLogger(raft.Logger(s.logger))

	//Signals when snapshotter is ready
	s.raft.snapshotterReady <- s.raft.snapshotter

	if s.raft.wal, err = s.replayWAL(); err != nil {
		return err
	}

	return s.startNode(oldwal)
}

func (s *Server) startNode(walExist bool) error {
	var rpeers = make([]raft.Peer, len(s.raft.peers))
	var c = &raft.Config{
		ID:              uint64(s.raft.id),
		ElectionTick:    s.raft.electionTick,
		HeartbeatTick:   s.raft.heartbeatTick,
		Storage:         s.raft.raftStorage,
		MaxSizePerMsg:   1024 * 1024,
		MaxInflightMsgs: 256,
	}

	if walExist {
		s.logger.Debugf("Restart Node waldir %s exist", s.raft.waldir)
		s.raft.node = raft.RestartNode(c)
		return nil
	}

	if len(rpeers) <= 0 {
		return errors.New("Required minimum one peers")
	}

	for i := range rpeers {
		rpeers[i] = raft.Peer{ID: uint64(i + 1)}
	}

	s.logger.Debugf("Start new node waldir %s don't exist", s.raft.waldir)
	if s.raft.join {
		rpeers = nil
	}
	s.raft.node = raft.StartNode(c, rpeers)

	return s.initRaftTransport()
}

func (s *Server) initRaftTransport() error {
	s.raft.transport = &rafthttp.Transport{
		Logger:      zap.NewExample(),
		ID:          types.ID(s.raft.id),
		ClusterID:   0x1000,
		Raft:        s,
		ServerStats: stats.NewServerStats("", ""),
		LeaderStats: stats.NewLeaderStats(strconv.Itoa(s.raft.id)),
		ErrorC:      make(chan error),
	}

	if err := s.raft.transport.Start(); err != nil {
		return errors.Wrap(err, "Error start RAFT transport")
	}

	//Добавляем оставшиеся ноды в кластер
	for i := range s.raft.peers {
		if i+1 != s.raft.id {
			s.raft.transport.AddPeer(types.ID(i+1), []string{s.raft.peers[i]})
		}
	}
	return nil
}

func (s *Server) runRAFTListener(ln *net.TCPListener) error {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return err
	}
	tc.SetKeepAlive(true)
	tc.SetKeepAlivePeriod(3 * time.Minute)

	return (&http.Server{
		Handler: s.raft.transport.Handler(),
	}).Serve(ln)
}

func (s *Server) serveChannels(ctx context.Context) error {
	defer s.logger.Debug("defer serveChannels")

	defer s.raft.wal.Close()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// event loop on raft state machine updates
	for {
		select {
		case <-ticker.C:
			s.raft.node.Tick()

		// store raft entries to wal, then publish over commit channel
		case rd := <-s.raft.node.Ready():
			if err := s.raft.wal.Save(rd.HardState, rd.Entries); err != nil {
				return err
			}

			//Если приепришел  не пустой
			if !raft.IsEmptySnap(rd.Snapshot) {
				//Сохраняем Snapshot на диск
				if err := s.saveSnap(rd.Snapshot); err != nil {
					return err
				}
				//Применяем Snapshot к Storage
				if err := s.raft.raftStorage.ApplySnapshot(rd.Snapshot); err != nil {
					return err
				}
				if err := s.publishSnapshot(rd.Snapshot); err != nil {
					return err
				}
			}

			//Вросим запиши в Raft Storage
			if err := s.raft.raftStorage.Append(rd.Entries); err != nil {
				return err
			}
			s.raft.transport.Send(rd.Messages)
			nents, err := s.entriesToApply(rd.CommittedEntries)
			if err != nil {
				return err
			}
			if err := s.publishEntries(ctx, nents); err != nil {
				return err
			}
			if err := s.maybeTriggerSnapshot(); err != nil {
				return err
			}
			// Advance уведомляет Node, что приложение сохраняет прогресс до последней готовности.
			// Он подготавливает Node к возврату следующей доступной Ready.
			//
			// Обычно приложение должно вызывать Advance после применения записей в последней Ready.
			//
			// Тем не менее, в качестве оптимизации приложение может называть Advance, когда оно применяет
			// команды. Например. когда последняя Ready содержит моментальный снимок, приложение может принимать
			// долгое время применять данные моментальных снимков. Для продолжения приема Ready без блокировки плота
			// progress, он может вызвать Advance перед тем, как закончить применение последней готовой.
			s.raft.node.Advance()

		case err := <-s.raft.transport.ErrorC:
			return err

		case <-ctx.Done():
			return nil
		}
	}

}

// publishEntries writes committed log entries to commit channel and returns
// whether all entries could be published.
func (s *Server) publishEntries(ctx context.Context, ents []raftpb.Entry) error {
	for _, val := range ents {
		switch val.Type {
		case raftpb.EntryNormal:
			if len(val.Data) == 0 {
				// ignore empty messages
				break
			}
			srt := string(val.Data)
			select {
			case s.raft.commitC <- &srt:
			case <-ctx.Done():
				return ctx.Err()
			}

		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			if err := cc.Unmarshal(val.Data); err != nil {
				return err
			}
			s.raft.confState = *s.raft.node.ApplyConfChange(cc)

			switch cc.Type {
			case raftpb.ConfChangeAddNode:
				if len(cc.Context) > 0 {
					s.raft.transport.AddPeer(types.ID(cc.NodeID), []string{string(cc.Context)})
				}
			case raftpb.ConfChangeRemoveNode:
				if cc.NodeID == uint64(s.raft.id) {
					return errors.New("I've been removed from the cluster! Shutting down")
				}
				s.raft.transport.RemovePeer(types.ID(cc.NodeID))
			}
		}

		// after commit, update appliedIndex
		s.raft.appliedIndex = val.Index

		// special nil commit to signal replay has finished
		if val.Index == s.raft.lastIndex {
			select {
			case s.raft.commitC <- nil:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return nil
}

func (s *Server) Process(ctx context.Context, m raftpb.Message) error {
	return s.raft.node.Step(ctx, m)
}
func (s *Server) IsIDRemoved(id uint64) bool                           { return false }
func (s *Server) ReportUnreachable(id uint64)                          {}
func (s *Server) ReportSnapshot(id uint64, status raft.SnapshotStatus) {}
