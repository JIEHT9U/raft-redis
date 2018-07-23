package server

import (
	"bufio"
	"fmt"
	"net"
	"strings"

	"context"

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
			proposeC:    make(chan string),
			confChangeC: make(chan raftpb.ConfChange),
			commitC:     make(chan *string),
			errorC:      make(chan error),
			id:          initParam.NodeID,
			peers:       strings.Split(initParam.RaftPeers, ","),
			join:        initParam.RaftJoin,
			waldir:      fmt.Sprintf("raftexample-%d", initParam.NodeID),
			snapdir:     fmt.Sprintf("raftexample-%d-snap", initParam.NodeID),
			getSnapshot: func() ([]byte, error) { return nil, nil },
			snapCount:   defaultSnapshotCount,
			//Может потом удалю из за ненадобности
			stopc:     make(chan struct{}),
			httpstopc: make(chan struct{}),
			httpdonec: make(chan struct{}),

			snapshotterReady: make(chan *snap.Snapshotter, 1),
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

	//RAFT
	g.Add(func() error {
		s.logger.Info("Start RAFT")
		return s.startRaft()
	}, func(err error) {
		if err != nil {
			s.logger.Errorf("Error RAFT loop %s", err)
		} else {
			s.logger.Info("Exit RAFT loop")
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
	// defer close(s.requests)  !!!!!!!!!
	for {
		select {
		case cmd := <-s.requests:
			switch cmd.cmd.actions {
			case set:
				s.st.stringsStorage.Set(cmd.cmd.key, cmd.cmd.values[0], cmd.cmd.expire)
				if err := responceWraper(cmd.response, []byte("success"), nil); err != nil {
					s.logger.Error(err)
				}
				continue
			case get:
				data, err := s.st.stringsStorage.Get(cmd.cmd.key)
				if err := responceWraper(cmd.response, data, err); err != nil {
					s.logger.Error(err)
				}
				continue
			case del:
				s.st.stringsStorage.Del(cmd.cmd.key)
				if err := responceWraper(cmd.response, []byte("success"), nil); err != nil {
					s.logger.Error(err)
				}
				continue
			}

		case <-ctx.Done():
			return nil
		}
	}
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
