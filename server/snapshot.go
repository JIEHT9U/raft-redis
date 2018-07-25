package server

import (
	"fmt"

	"github.com/coreos/etcd/etcdserver/api/snap"
	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/coreos/etcd/wal/walpb"
)

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

//Пробуем заргузить снапшот
func (s *Server) loadSnapshot() (*raftpb.Snapshot, error) {
	snapshot, err := s.raft.snapshotter.Load()
	if err != nil && err != snap.ErrNoSnapshot {
		return nil, fmt.Errorf("error loading snapshot (%v)", err)
	}
	return snapshot, nil
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

func (s *Server) maybeTriggerSnapshot() error {
	if s.raft.appliedIndex-s.raft.snapshotIndex <= s.raft.snapCount {
		return nil
	}

	s.logger.Infof("start snapshot [applied index: %d | last snapshot index: %d]", s.raft.appliedIndex, s.raft.snapshotIndex)
	data, err := s.getSnapshot()
	if err != nil {
		return err
	}
	snap, err := s.raft.raftStorage.CreateSnapshot(s.raft.appliedIndex, &s.raft.confState, data)
	if err != nil {
		return err
	}
	if err := s.saveSnap(snap); err != nil {
		return err
	}

	compactIndex := uint64(1)
	if s.raft.appliedIndex > snapshotCatchUpEntriesN {
		compactIndex = s.raft.appliedIndex - snapshotCatchUpEntriesN
	}
	if err := s.raft.raftStorage.Compact(compactIndex); err != nil {
		return err
	}

	s.logger.Infof("compacted log at index %d", compactIndex)
	s.raft.snapshotIndex = s.raft.appliedIndex
	return nil
}
