package server

import (
	"bufio"
	"context"
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
	snapDir := fmt.Sprintf("%s-%d-snap", initParam.RaftDataDir, initParam.NodeID)

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
			confChangeC:      make(chan raftpb.ConfChange),
			commitC:          make(chan *string),
			errorC:           make(chan error),
			id:               initParam.NodeID,
			peers:            strings.Split(initParam.RaftPeers, ","),
			join:             initParam.RaftJoin,
			snapdir:          snapDir,
			waldir:           fmt.Sprintf("%s-%d", initParam.RaftDataDir, initParam.NodeID),
			snapCount:        defaultSnapshotCount,
			snapshotterReady: make(chan *snap.Snapshotter, 1),
			snapshotter:      snap.New(logger, snapDir),
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
		s.logger.Info("Stop TCP listener")
		listener.Close()
	})

	ctx, cansel := context.WithCancel(context.Background())
	g.Add(func() error {
		s.logger.Info("Start main loop")
		return s.runServer(ctx)
	}, func(err error) {
		if err != nil {
			s.logger.Errorf("Error exit main loop %s", err)
		} else {
			s.logger.Info("Exit main loop")
		}
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
		if err != nil {
			s.logger.Errorf("Error RAFT Listener %s", err)
		} else {
			s.logger.Info("Exit RAFT Listener")
		}
		raftListener.Close()
	})

	g.Add(func() error {
		s.logger.Info("Start serveChannels")
		return s.serveChannels()
	}, func(err error) {
		if err != nil {
			s.logger.Errorf("Error serveChannels loop %s", err)
		} else {
			s.logger.Info("Exit serveChannels loop")
		}
	})

	//Shutdown
	g.Add(func() error {
		<-s.shutdown
		s.logger.Info("Resive Shutdown signal")
		return nil
	}, func(err error) {})

	return g.Run()
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

		case cc, ok := <-s.raft.confChangeC:
			if !ok {
				s.raft.confChangeC = nil
			} else {
				confChangeCount++
				cc.ID = confChangeCount
				s.raft.node.ProposeConfChange(context.TODO(), cc)
			}

			//Propose
		case cmd := <-s.requests:
			s.logger.Debug("cmd:", cmd.cmd)
			//blocks до тех пор, пока их не примет машина
			ctx, _ := context.WithTimeout(context.Background(), time.Second*2)
			err := s.raft.node.Propose(ctx, []byte(cmd.cmd.values[0]))
			if err != nil {
				s.logger.Error(err)
			}

			if err := responceWraper(cmd.response, []byte("OLOLOLO"), nil); err != nil {
				s.logger.Error(err)
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

		case <-s.raft.commitC:
			s.logger.Debug("Log Recive Data !!")
		case <-ctx.Done():
			return nil
		}
	}
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
