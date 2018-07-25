package server

import (
	"bufio"
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	i "github.com/JIEHT9U/raft-redis/init"
	"github.com/JIEHT9U/raft-redis/list"
	"github.com/JIEHT9U/raft-redis/str"
	"github.com/JIEHT9U/raft-redis/vocabulary"
	"github.com/coreos/etcd/etcdserver/api/snap"
	"github.com/coreos/etcd/raft"
	"github.com/coreos/etcd/raft/raftpb"
	"github.com/oklog/run"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

//Server type
type Server struct {
	listenAddr *net.TCPAddr
	logger     *zap.SugaredLogger
	shutdown   <-chan struct{}
	requests   chan request
	raft       *raftNode
	st         storages
}

type request struct {
	cmd      cmd
	response chan resp
}

type resp struct {
	err  error
	data []byte
}

//New return new type *Server
func New(initParam *i.Params, logger *zap.SugaredLogger, shutdown <-chan struct{}) *Server {
	snapDir := fmt.Sprintf("%s/%d/snap", initParam.RaftDataDir, initParam.NodeID)

	return &Server{
		listenAddr: initParam.ListenAddr,
		logger:     logger,
		shutdown:   shutdown,
		st: storages{
			listStorage:       make(map[string]list.Store),
			vocabularyStorage: make(map[string]vocabulary.Store),
			stringsStorage:    str.New(),
		},
		requests: make(chan request, 1),
		raft: &raftNode{
			id:               initParam.NodeID,
			join:             initParam.RaftJoin,
			confChangeC:      make(chan raftpb.ConfChange),
			commitC:          make(chan *string, 1),
			peers:            strings.Split(initParam.RaftPeers, ","),
			waldir:           fmt.Sprintf("%s/%d/wal", initParam.RaftDataDir, initParam.NodeID),
			snapCount:        defaultSnapshotCount,
			snapdir:          snapDir,
			snapshotterReady: make(chan *snap.Snapshotter, 1),
			snapshotter:      snap.New(logger, snapDir),

			//Создаем новый raft Storage куда будут загружены данные из снапшота
			raftStorage: raft.NewMemoryStorage(),
		},
	}
}

//Run running main process
func (s *Server) Run() error {
	var g run.Group

	listener, err := net.Listen("tcp", s.listenAddr.String())
	if err != nil {
		return errors.Wrap(err, "Error starting TCP server.")
	}
	g.Add(func() error {
		s.logger.Infof("TCP listener %s", s.listenAddr.String())
		return s.runTCPListener(listener)
	}, func(err error) {
		s.errorsWraps(err, "Error stop TCP listener", "Stop TCP listener")
		listener.Close()
	})

	ctx, cansel := context.WithCancel(context.Background())
	g.Add(func() error {
		s.logger.Info("Start main loop")
		return s.runServer(ctx)
	}, func(err error) {
		s.errorsWraps(err, "Error exit main loop", "Exit main loop")
		cansel()
	})

	//initRaft
	raftListener, err := s.createRAFTListener()
	if err != nil {
		return errors.Wrap(err, "Error create RAFT Listener")
	}
	g.Add(func() error {
		return s.runRAFTListener(raftListener)
	}, func(err error) {
		s.errorsWraps(err, "Error RAFT Listener", "Exit RAFT Listener")
		raftListener.Close()
	})

	g.Add(func() error {
		s.logger.Info("Start serveChannels")
		return s.serveChannels(ctx)
	}, func(err error) {
		s.errorsWraps(err, "Error serveChannels loop", "Exit serveChannels loop")
		cansel()
	})

	//Shutdown
	g.Add(func() error {
		<-s.shutdown
		s.logger.Info("Resive Shutdown signal")
		return nil
	}, func(err error) {})

	return g.Run()
}

func (s *Server) errorsWraps(err error, e, str string) {
	if err != nil {
		s.logger.Errorf("%s %s", e, err)
	} else {
		s.logger.Info(str)
	}
}

func (s *Server) runTCPListener(l net.Listener) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return errors.Wrap(err, "Error accepting")
		}
		go s.connHandler(conn)
	}
}

func (s *Server) connHandler(conn net.Conn) {
	defer conn.Close()
	var remoteAddr = strings.TrimSpace(conn.RemoteAddr().String())

	s.logger.Infof("Start new connections %s", remoteAddr)

	if _, err := fmt.Fprintf(conn, "[%s] > %s\n\r", remoteAddr, "Success server connection"); err != nil {
		s.logger.Errorf("Error write err msg clinet [%s]", err)
		return
	}

	scanner := bufio.NewScanner(conn)

	for scanner.Scan() {
		switch line := scanner.Text(); line {
		case "exit":
			s.logger.Info("Close connection resive 'exit'")
			return
		default:
			out, err := s.commandHandling(strings.Fields(line))
			if err != nil {
				s.logger.Error(err)
				if _, err := fmt.Fprintf(conn, "[%s] ✘ > %s\n\r", remoteAddr, err.Error()); err != nil {
					s.logger.Errorf("Error write err msg clinet [%s]", err)
					return
				}
				continue
			}
			if _, err := fmt.Fprintf(conn, "[%s] > %s\n\r", remoteAddr, out); err != nil {
				s.logger.Errorf("Error write err msg clinet [%s]", err)
				return
			}
			continue
		}
	}

	if err := scanner.Err(); err != nil {
		s.logger.Error(errors.Wrap(err, "Scanner error"))
		return
	}
}

func (s *Server) runServer(ctx context.Context) error {
	var confChangeCount uint64

	// defer close(s.requests)  !!!!!!!!!

	for {
		select {

		// send proposals over raft
		case cc := <-s.raft.confChangeC:
			confChangeCount++
			cc.ID = confChangeCount
			if err := s.raft.node.ProposeConfChange(context.TODO(), cc); err != nil {
				return err
			}

		case cmd := <-s.requests:

			switch cmd.cmd.actions {
			case set:
				var buf bytes.Buffer
				if err := gob.NewEncoder(&buf).Encode(cmd.cmd); err != nil {
					return err
				}

				ctx, cansel := context.WithTimeout(context.Background(), time.Second*2)
				err := s.raft.node.Propose(ctx, buf.Bytes())
				cansel()

				if err := responceWraper(cmd.response, nil, err); err != nil {
					s.logger.Error(err)
				}
			}

			//END RAFT

			// switch cmd.cmd.actions {
			// case set:
			// 	s.st.stringsStorage.Set(cmd.cmd.key, cmd.cmd.values[0], cmd.cmd.expire)
			// 	if err := responceWraper(cmd.response, []byte("success"), nil); err != nil {
			// 		s.logger.Error(err)
			// 	}
			// 	continue
			// case get:
			// 	data, err := s.st.stringsStorage.Get(cmd.cmd.key)
			// 	if err := responceWraper(cmd.response, data, err); err != nil {
			// 		s.logger.Error(err)
			// 	}
			// 	continue
			// case del:
			// 	s.st.stringsStorage.Del(cmd.cmd.key)
			// 	if err := responceWraper(cmd.response, []byte("success"), nil); err != nil {
			// 		s.logger.Error(err)
			// 	}
			// 	continue
			// }

		case msg := <-s.raft.commitC:
			if err := s.receiveCommitMSG(msg); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func (s *Server) receiveCommitMSG(msg *string) error {

	return nil
}

func (s *Server) loadFromSnapshot(msg *string) error {
	snapshot, err := s.raft.snapshotter.Load()
	if err == snap.ErrNoSnapshot {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "load from snapshot")
	}
	s.logger.Infof("loading snapshot at term %d and index %d", snapshot.Metadata.Term, snapshot.Metadata.Index)
	return s.recoverFromSnapshot(snapshot.Data)
}

func (s *Server) recoverFromSnapshot(snapshot []byte) error {
	if err := json.Unmarshal(snapshot, &s.st); err != nil {
		return err
	}
	return nil
}

//GetSnapshot func
func (s *Server) getSnapshot() ([]byte, error) {
	return []byte{}, nil
}

func responceWraper(response chan resp, data []byte, err error) error {
	select {
	case response <- resp{data: data, err: err}:
		return nil
	default:
		close(response)
		return fmt.Errorf("Error send response MSG [%s]", string(data))
	}
}

func (s *Server) createRAFTListener() (*net.TCPListener, error) {

	url, err := url.Parse(s.raft.peers[s.raft.id-1])
	if err != nil {
		return nil, fmt.Errorf("raftexample: Failed parsing URL (%v)", err)
	}

	ln, err := net.Listen("tcp", url.Host)
	if err != nil {
		return nil, err
	}
	return ln.(*net.TCPListener), nil
}
