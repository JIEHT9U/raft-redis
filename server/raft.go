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

// publishEntries writes committed log entries to commit channel and returns
// whether all entries could be published.
// func (s *Server) publishEntries(ents []raftpb.Entry) bool {
// 	for i := range ents {
// 		switch ents[i].Type {
// 		case raftpb.EntryNormal:
// 			if len(ents[i].Data) == 0 {
// 				// ignore empty messages
// 				break
// 			}
// 			str := string(ents[i].Data)
// 			select {
// 			case s.raft.commitC <- &str:
// 			case <-s.raft.stopc:
// 				return false
// 			}

// 		case raftpb.EntryConfChange:
// 			var cc raftpb.ConfChange
// 			cc.Unmarshal(ents[i].Data)
// 			s.raft.confState = *s.raft.node.ApplyConfChange(cc)
// 			switch cc.Type {
// 			case raftpb.ConfChangeAddNode:
// 				if len(cc.Context) > 0 {
// 					s.raft.transport.AddPeer(types.ID(cc.NodeID), []string{string(cc.Context)})
// 				}
// 			case raftpb.ConfChangeRemoveNode:
// 				if cc.NodeID == uint64(s.raft.id) {
// 					log.Println("I've been removed from the cluster! Shutting down.")
// 					return false
// 				}
// 				s.raft.transport.RemovePeer(types.ID(cc.NodeID))
// 			}
// 		}

// 		// after commit, update appliedIndex
// 		s.raft.appliedIndex = ents[i].Index

// 		// special nil commit to signal replay has finished
// 		if ents[i].Index == s.raft.lastIndex {
// 			select {
// 			case s.raft.commitC <- nil:
// 			case <-s.raft.stopc:
// 				return false
// 			}
// 		}
// 	}
// 	return true
// }

func creteSnapDirIfNotExist(path string) error {
	if !fileutil.Exist(path) {
		if err := os.Mkdir(path, 0750); err != nil {
			return fmt.Errorf("raftexample: cannot create dir for snapshot (%v)", err)
		}
	}
	return nil
}

//InitRaft init default RAFT param
func (s *Server) InitRaft() error {
	var err error

	//Создаём дерикротию где будут храниться snapshots
	if err = creteSnapDirIfNotExist(s.raft.snapdir); err != nil {
		return err
	}

	raft.SetLogger(raft.Logger(s.logger))

	//Signals when snapshotter is ready
	// s.raft.snapshotter = snap.New(s.logger, s.raft.snapdir)
	// s.raft.snapshotterReady <- s.raft.snapshotter

	if s.raft.wal, err = s.replayWAL(); err != nil {
		return err
	}

	rpeers := make([]raft.Peer, len(s.raft.peers))
	for i := range rpeers {
		rpeers[i] = raft.Peer{ID: uint64(i + 1)}
	}

	c := &raft.Config{
		ID:              uint64(s.raft.id),
		ElectionTick:    10,
		HeartbeatTick:   1,
		Storage:         s.raft.raftStorage,
		MaxSizePerMsg:   1024 * 1024,
		MaxInflightMsgs: 256,
	}

	if wal.Exist(s.raft.waldir) {
		s.logger.Debugf("Restart Node waldir %s exist", s.raft.waldir)
		s.raft.node = raft.RestartNode(c)
	} else {
		s.logger.Debug("Start Node waldir %s don't exist", s.raft.waldir)

		if s.raft.join {
			rpeers = nil
		}
		s.raft.node = raft.StartNode(c, rpeers)
	}

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

func (s *Server) serveChannels() error {

	snap, err := s.raft.raftStorage.Snapshot()
	if err != nil {
		return err
	}

	PrePrint, _ := json.MarshalIndent(snap, "", "  ")
	s.logger.Debug("snap:", string(PrePrint))
	return nil

	// s.raft.confState = snap.Metadata.ConfState
	// s.raft.snapshotIndex = snap.Metadata.Index
	// s.raft.appliedIndex = snap.Metadata.Index

	// defer s.raft.wal.Close()

	// ticker := time.NewTicker(100 * time.Millisecond)
	// defer ticker.Stop()

	// // event loop on raft state machine updates
	// for {
	// 	select {
	// 	case <-ticker.C:
	// 		s.raft.node.Tick()

	// 	// store raft entries to wal, then publish over commit channel
	// 	case rd := <-s.raft.node.Ready():
	// 		s.raft.wal.Save(rd.HardState, rd.Entries)
	// 		if !raft.IsEmptySnap(rd.Snapshot) {
	// 			s.saveSnap(rd.Snapshot)
	// 			s.raft.raftStorage.ApplySnapshot(rd.Snapshot)
	// 			s.publishSnapshot(rd.Snapshot)
	// 		}
	// 		s.raft.raftStorage.Append(rd.Entries)
	// 		s.raft.transport.Send(rd.Messages)
	// 		nents, err := s.entriesToApply(rd.CommittedEntries)
	// 		if err != nil {
	// 			return err
	// 		}
	// 		if ok := s.publishEntries(nents); !ok {
	// 			s.stop()
	// 			return err
	// 		}
	// 		if err := s.maybeTriggerSnapshot(); err != nil {
	// 			return err
	// 		}
	// 		s.raft.node.Advance()

	// 	case err := <-s.raft.transport.ErrorC:
	// 		s.writeError(err)
	// 		return err

	// 	case <-s.raft.stopc:
	// 		s.stop()
	// 		return err
	// 	}
	// }
}

func (s *Server) Process(ctx context.Context, m raftpb.Message) error {
	return s.raft.node.Step(ctx, m)
}
func (s *Server) IsIDRemoved(id uint64) bool                           { return false }
func (s *Server) ReportUnreachable(id uint64)                          {}
func (s *Server) ReportSnapshot(id uint64, status raft.SnapshotStatus) {}
