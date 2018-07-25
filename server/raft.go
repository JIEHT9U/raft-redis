package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

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
	commitC     chan *string             // entries committed to log (k,v)
	errorC      chan<- error             // errors from raft session

	id        int      // client ID for raft session
	peers     []string // raft peer URLs
	join      bool     // node is joining an existing cluster
	waldir    string   // path to WAL directory
	snapdir   string   // path to snapshot directory
	lastIndex uint64   // index of log at start

	confState     raftpb.ConfState
	snapshotIndex uint64
	appliedIndex  uint64

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
	var nents = []raftpb.Entry{}
	if len(ents) == 0 {
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
	raft.SetLogger(raft.Logger(s.logger))

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
		ElectionTick:    10,
		HeartbeatTick:   1,
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

			// s.logger.Debug("Log Recive Data s.raft.node.Ready!!")

			PrePrint, err := json.MarshalIndent(rd, "", "  ")
			if err != nil {
				return err
			}
			s.logger.Debug("rc.node.Ready:", string(PrePrint))

			if err := s.raft.wal.Save(rd.HardState, rd.Entries); err != nil {
				return err
			}

			if !raft.IsEmptySnap(rd.Snapshot) {
				s.saveSnap(rd.Snapshot)
				s.raft.raftStorage.ApplySnapshot(rd.Snapshot)
				s.publishSnapshot(rd.Snapshot)
			}
			s.raft.raftStorage.Append(rd.Entries)
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
			s.raft.node.Advance()

		case <-ctx.Done():
			return nil
		}
	}

}

// publishEntries writes committed log entries to commit channel and returns
// whether all entries could be published.
func (s *Server) publishEntries(ctx context.Context, ents []raftpb.Entry) error {
	for i := range ents {
		switch ents[i].Type {
		case raftpb.EntryNormal:
			if len(ents[i].Data) == 0 {
				// ignore empty messages
				s.logger.Debug("Recive emty MSG")
				break
			}
			srt := string(ents[i].Data)
			select {
			case s.raft.commitC <- &srt:
			case <-ctx.Done():
				return ctx.Err()
			}

		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			if err := cc.Unmarshal(ents[i].Data); err != nil {
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
		s.raft.appliedIndex = ents[i].Index

		// special nil commit to signal replay has finished
		if ents[i].Index == s.raft.lastIndex {
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
