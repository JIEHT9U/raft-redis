package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
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
	"github.com/coreos/etcd/wal/walpb"

	"go.uber.org/zap"
)

// A key-value stream backed by raft
type raftNode struct {
	proposeC    <-chan string            // proposed messages (k,v)
	confChangeC <-chan raftpb.ConfChange // proposed cluster config changes
	commitC     chan<- *string           // entries committed to log (k,v)
	errorC      chan<- error             // errors from raft session

	id          int      // client ID for raft session
	peers       []string // raft peer URLs
	join        bool     // node is joining an existing cluster
	waldir      string   // path to WAL directory
	snapdir     string   // path to snapshot directory
	getSnapshot func() ([]byte, error)
	lastIndex   uint64 // index of log at start

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
	stopc     chan struct{} // signals proposal channel closed
	httpstopc chan struct{} // signals http server to shutdown
	httpdonec chan struct{} // signals http server shutdown complete
}

var defaultSnapshotCount uint64 = 10000

func (s *Server) saveSnap(snap raftpb.Snapshot) error {
	// must save the snapshot index to the WAL before saving the
	// snapshot to maintain the invariant that we only Open the
	// wal at previously-saved snapshot indexes.
	walSnap := walpb.Snapshot{
		Index: snap.Metadata.Index,
		Term:  snap.Metadata.Term,
	}
	if err := s.raft.wal.SaveSnapshot(walSnap); err != nil {
		return err
	}

	if err := s.raft.snapshotter.SaveSnap(snap); err != nil {
		return err
	}
	return s.raft.wal.ReleaseLockTo(snap.Metadata.Index)
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
func (s *Server) publishEntries(ents []raftpb.Entry) bool {
	for i := range ents {
		switch ents[i].Type {
		case raftpb.EntryNormal:
			if len(ents[i].Data) == 0 {
				// ignore empty messages
				break
			}
			str := string(ents[i].Data)
			select {
			case s.raft.commitC <- &str:
			case <-s.raft.stopc:
				return false
			}

		case raftpb.EntryConfChange:
			var cc raftpb.ConfChange
			cc.Unmarshal(ents[i].Data)
			s.raft.confState = *s.raft.node.ApplyConfChange(cc)
			switch cc.Type {
			case raftpb.ConfChangeAddNode:
				if len(cc.Context) > 0 {
					s.raft.transport.AddPeer(types.ID(cc.NodeID), []string{string(cc.Context)})
				}
			case raftpb.ConfChangeRemoveNode:
				if cc.NodeID == uint64(s.raft.id) {
					log.Println("I've been removed from the cluster! Shutting down.")
					return false
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
			case <-s.raft.stopc:
				return false
			}
		}
	}
	return true
}

func (s *Server) loadSnapshot() (*raftpb.Snapshot, error) {
	snapshot, err := s.raft.snapshotter.Load()
	if err != nil && err != snap.ErrNoSnapshot {
		return nil, fmt.Errorf("raftexample: error loading snapshot (%v)", err)
	}
	return snapshot, nil
}

// openWAL returns a WAL ready for reading.
func (s *Server) openWAL(snapshot *raftpb.Snapshot) (*wal.WAL, error) {
	if !wal.Exist(s.raft.waldir) {
		if err := os.Mkdir(s.raft.waldir, 0750); err != nil {
			return nil, fmt.Errorf("raftexample: cannot create dir for wal (%v)", err)
		}

		w, err := wal.Create(s.logger, s.raft.waldir, nil)
		if err != nil {
			return nil, fmt.Errorf("raftexample: create wal error (%v)", err)
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
		return nil, fmt.Errorf("raftexample: error loading wal (%v)", err)
	}

	return w, nil
}

// replayWAL replays WAL entries into the raft instance.
func (s *Server) replayWAL() (*wal.WAL, error) {
	s.logger.Infof("replaying WAL of member %d", s.raft.id)

	snapshot, err := s.loadSnapshot()
	if err != nil {
		return nil, err
	}

	w, err := s.openWAL(snapshot)
	if err != nil {
		return nil, err
	}

	_, st, ents, err := w.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("raftexample: failed to read WAL (%v)", err)
	}

	s.raft.raftStorage = raft.NewMemoryStorage()
	if snapshot != nil {
		s.raft.raftStorage.ApplySnapshot(*snapshot)
	}
	s.raft.raftStorage.SetHardState(st)

	// append to storage so raft starts at the right place in log
	s.raft.raftStorage.Append(ents)
	// send nil once lastIndex is published so client knows commit channel is current
	if len(ents) > 0 {
		s.raft.lastIndex = ents[len(ents)-1].Index
	} else {
		s.raft.commitC <- nil
	}
	return w, nil
}

func (s *Server) writeError(err error) {
	s.stopHTTP()
	close(s.raft.commitC)
	s.raft.errorC <- err
	close(s.raft.errorC)
	s.raft.node.Stop()
}

func creteSnapDirIfNotExist(path string) error {
	if !fileutil.Exist(path) {
		if err := os.Mkdir(path, 0750); err != nil {
			return fmt.Errorf("raftexample: cannot create dir for snapshot (%v)", err)
		}
	}
	return nil
}

func (s *Server) startRaft() error {
	var err error
	if err = creteSnapDirIfNotExist(s.raft.snapdir); err != nil {
		return err
	}
	s.raft.snapshotter = snap.New(s.logger, s.raft.snapdir)
	s.raft.snapshotterReady <- s.raft.snapshotter

	oldwal := wal.Exist(s.raft.waldir)

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

	if oldwal {
		s.raft.node = raft.RestartNode(c)
	} else {
		startPeers := rpeers
		if s.raft.join {
			startPeers = nil
		}
		s.raft.node = raft.StartNode(c, startPeers)
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

	s.raft.transport.Start()
	for i := range s.raft.peers {
		if i+1 != s.raft.id {
			s.raft.transport.AddPeer(types.ID(i+1), []string{s.raft.peers[i]})
		}
	}

	go s.serveRaft()
	go s.serveChannels()
	return nil
}

// stop closes http, closes all channels, and stops raft.
func (s *Server) stop() {
	s.stopHTTP()
	close(s.raft.commitC)
	close(s.raft.errorC)
	s.raft.node.Stop()
}

func (s *Server) stopHTTP() {
	s.raft.transport.Stop()
	close(s.raft.httpstopc)
	<-s.raft.httpdonec
}

func (s *Server) publishSnapshot(snapshotToSave raftpb.Snapshot) error {
	if raft.IsEmptySnap(snapshotToSave) {
		return nil
	}

	s.logger.Infof("publishing snapshot at index %d", s.raft.snapshotIndex)
	defer s.logger.Infof("finished publishing snapshot at index %d", s.raft.snapshotIndex)

	if snapshotToSave.Metadata.Index <= s.raft.appliedIndex {
		return fmt.Errorf("snapshot index [%d] should > progress.appliedIndex [%d] + 1", snapshotToSave.Metadata.Index, s.raft.appliedIndex)
	}
	s.raft.commitC <- nil // trigger kvstore to load snapshot

	s.raft.confState = snapshotToSave.Metadata.ConfState
	s.raft.snapshotIndex = snapshotToSave.Metadata.Index
	s.raft.appliedIndex = snapshotToSave.Metadata.Index
	return nil
}

var snapshotCatchUpEntriesN uint64 = 10000

func (s *Server) maybeTriggerSnapshot() {
	if s.raft.appliedIndex-s.raft.snapshotIndex <= s.raft.snapCount {
		return
	}

	s.logger.Infof("start snapshot [applied index: %d | last snapshot index: %d]", s.raft.appliedIndex, s.raft.snapshotIndex)
	data, err := s.raft.getSnapshot()
	if err != nil {
		log.Panic(err)
	}
	snap, err := s.raft.raftStorage.CreateSnapshot(s.raft.appliedIndex, &s.raft.confState, data)
	if err != nil {
		panic(err)
	}
	if err := s.saveSnap(snap); err != nil {
		panic(err)
	}

	compactIndex := uint64(1)
	if s.raft.appliedIndex > snapshotCatchUpEntriesN {
		compactIndex = s.raft.appliedIndex - snapshotCatchUpEntriesN
	}
	if err := s.raft.raftStorage.Compact(compactIndex); err != nil {
		panic(err)
	}

	s.logger.Infof("compacted log at index %d", compactIndex)
	s.raft.snapshotIndex = s.raft.appliedIndex
}

func (s *Server) serveChannels() {
	snap, err := s.raft.raftStorage.Snapshot()
	if err != nil {
		panic(err)
	}
	s.raft.confState = snap.Metadata.ConfState
	s.raft.snapshotIndex = snap.Metadata.Index
	s.raft.appliedIndex = snap.Metadata.Index

	defer s.raft.wal.Close()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	// send proposals over raft
	go func() {
		var confChangeCount uint64 = 0

		for s.raft.proposeC != nil && s.raft.confChangeC != nil {
			select {
			case prop, ok := <-s.raft.proposeC:
				if !ok {
					s.raft.proposeC = nil
				} else {
					// blocks until accepted by raft state machine
					s.raft.node.Propose(context.TODO(), []byte(prop))
				}

			case cc, ok := <-s.raft.confChangeC:
				if !ok {
					s.raft.confChangeC = nil
				} else {
					confChangeCount++
					cc.ID = confChangeCount
					s.raft.node.ProposeConfChange(context.TODO(), cc)
				}
			}
		}
		// client closed channel; shutdown raft if not already
		close(s.raft.stopc)
	}()

	// event loop on raft state machine updates
	for {
		select {
		case <-ticker.C:
			s.raft.node.Tick()

		// store raft entries to wal, then publish over commit channel
		case rd := <-s.raft.node.Ready():
			s.raft.wal.Save(rd.HardState, rd.Entries)
			if !raft.IsEmptySnap(rd.Snapshot) {
				s.saveSnap(rd.Snapshot)
				s.raft.raftStorage.ApplySnapshot(rd.Snapshot)
				s.publishSnapshot(rd.Snapshot)
			}
			s.raft.raftStorage.Append(rd.Entries)
			s.raft.transport.Send(rd.Messages)
			nents, err := s.entriesToApply(rd.CommittedEntries)
			if err != nil {
				return
			}
			if ok := s.publishEntries(nents); !ok {
				s.stop()
				return
			}
			s.maybeTriggerSnapshot()
			s.raft.node.Advance()

		case err := <-s.raft.transport.ErrorC:
			s.writeError(err)
			return

		case <-s.raft.stopc:
			s.stop()
			return
		}
	}
}

func (s *Server) serveRaft() error {
	url, err := url.Parse(s.raft.peers[s.raft.id-1])
	if err != nil {
		return fmt.Errorf("raftexample: Failed parsing URL (%v)", err)
	}

	ln, err := newStoppableListener(url.Host, s.raft.httpstopc)
	if err != nil {
		return fmt.Errorf("raftexample: Failed to listen rafthttp (%v)", err)
	}

	err = (&http.Server{Handler: s.raft.transport.Handler()}).Serve(ln)
	select {
	case <-s.raft.httpstopc:
	default:
		return fmt.Errorf("raftexample: Failed to serve rafthttp (%v)", err)
	}
	close(s.raft.httpdonec)
	return nil
}

func (s *Server) Process(ctx context.Context, m raftpb.Message) error {
	return s.raft.node.Step(ctx, m)
}
func (s *Server) IsIDRemoved(id uint64) bool                           { return false }
func (s *Server) ReportUnreachable(id uint64)                          {}
func (s *Server) ReportSnapshot(id uint64, status raft.SnapshotStatus) {}
