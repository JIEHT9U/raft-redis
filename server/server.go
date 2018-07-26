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
	"strconv"
	"strings"
	"time"

	i "github.com/JIEHT9U/raft-redis/init"
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
	getSnap    chan storages
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
func New(initParam *i.Params, logger *zap.Logger, shutdown <-chan struct{}) *Server {
	snapDir := fmt.Sprintf("%s/%d/snap", initParam.RaftDataDir, initParam.NodeID)

	return &Server{
		getSnap:    make(chan storages),
		listenAddr: initParam.ListenAddr,
		logger:     logger.Sugar(),
		shutdown:   shutdown,
		st: storages{
			data: make(map[string]storage),
		},

		requests: make(chan request, 1),
		raft: &raftNode{
			electionTick:     10,
			heartbeatTick:    1,
			id:               initParam.NodeID,
			join:             initParam.RaftJoin,
			confChangeC:      make(chan raftpb.ConfChange),
			commitC:          make(chan *string, 1),
			peers:            strings.Split(initParam.RaftPeers, ","),
			waldir:           fmt.Sprintf("%s/%d/wal", initParam.RaftDataDir, initParam.NodeID),
			snapCount:        10000,
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

	for {
		select {

		case cc := <-s.raft.confChangeC:

			PrePrint, _ := json.MarshalIndent(cc, "", "  ")
			s.logger.Debug("s.raft.confChangeC:", string(PrePrint))

			confChangeCount++
			cc.ID = confChangeCount
			if err := s.raft.node.ProposeConfChange(context.TODO(), cc); err != nil {
				return err
			}

		case requests := <-s.requests:
			switch requests.cmd.Actions {
			case set, del, lpush, rpush, expire:
				if err := responceWraper(requests.response, nil, s.proposeCMD(requests.cmd)); err != nil {
					s.logger.Error(err)
				}
			case get:
				data, err := s.st.get(requests.cmd.Key)
				if err == ErrTimeExpired {
					var err = s.proposeCMD(cmd{Key: requests.cmd.Key, Actions: del})
					if err == nil {
						err = ErrKeyNotFound
					}
					if err := responceWraper(requests.response, nil, err); err != nil {
						s.logger.Error(err)
					}
					continue
				}
				if err := responceWraper(requests.response, data, err); err != nil {
					s.logger.Error(err)
				}
			case llen:
				if data, ok := s.st.data[requests.cmd.Key]; ok {
					if err := responceWraper(requests.response, []byte("len:"+strconv.FormatInt(data.linkedList.Count(), 64)), nil); err != nil {
						s.logger.Error(err)
					}
				}
				if err := responceWraper(requests.response, nil, ErrKeyNotFound); err != nil {
					s.logger.Error(err)
				}
			case lget:
				if data, ok := s.st.data[requests.cmd.Key]; ok {
					data.linkedList.Next()
					if err := responceWraper(requests.response, []byte("len:"+strconv.FormatInt(data.linkedList.Count(), 64)), nil); err != nil {
						s.logger.Error(err)
					}
				}
				if err := responceWraper(requests.response, nil, ErrKeyNotFound); err != nil {
					s.logger.Error(err)
				}
			default:
				if err := responceWraper(requests.response, nil, fmt.Errorf("Unknown command %s", requests.cmd.Actions)); err != nil {
					s.logger.Error(err)
				}
			}
		case s.getSnap <- s.st:
		case msg := <-s.raft.commitC:
			if err := s.receiveCommitMSG(msg); err != nil {
				return err
			}
		case <-ctx.Done():
			return nil
		}
	}
}

func (s *Server) proposeCMD(cmd cmd) error {
	var buf bytes.Buffer
	if err := gob.NewEncoder(&buf).Encode(cmd); err != nil {
		return err
	}

	ctx, cansel := context.WithTimeout(context.Background(), time.Second*2)
	defer cansel()
	return s.raft.node.Propose(ctx, buf.Bytes())
}

func (s *Server) receiveCommitMSG(msg *string) error {
	if msg == nil {
		return s.loadFromSnapshot()
	}

	var cmd cmd
	if err := gob.NewDecoder(bytes.NewBufferString(*msg)).Decode(&cmd); err != nil {
		return fmt.Errorf("could not decode message (%v)", err)
	}

	return s.applyCMD(cmd)
}

func (s *Server) applyCMD(cmd cmd) error {

	switch cmd.Actions {
	case set:
		s.st.set(cmd.Key, cmd.Values[0], cmd.Expire)
		return nil
	case del:
		s.st.del(cmd.Key)
		return nil
	case lpush:
		return s.st.lpush(cmd.Key, cmd.Values[0])
	case rpush:
		return s.st.rpush(cmd.Key, cmd.Values[0])
	case expire:
		return s.st.expire(cmd.Key, cmd.Expire)
	default:
		return fmt.Errorf("Undefined cmd %s", cmd.Actions)
	}
}

func (s *Server) loadFromSnapshot() error {
	snapshot, err := s.raft.snapshotter.Load()
	if err == snap.ErrNoSnapshot {
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "load from snapshot")
	}
	s.logger.Infof("loading snapshot at term %d and index %d", snapshot.Metadata.Term, snapshot.Metadata.Index)
	return json.Unmarshal(snapshot.Data, &s.st)
}

func (s *Server) getSnapshot() ([]byte, error) {
	s.logger.Debug("get snapshot")
	select {
	case storages := <-s.getSnap:
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(storages); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case <-time.After(time.Second * 1):
		return nil, errors.New("Error time out getSnapshot")
	}
}

func responceWraper(response chan resp, data []byte, err error) error {
	select {
	case response <- resp{data: data, err: err}:
		return nil
	default:
		return fmt.Errorf("Error send response MSG [%s]", string(data))
	}
}

func (s *Server) createRAFTListener() (*net.TCPListener, error) {

	url, err := url.Parse(s.raft.peers[s.raft.id-1])
	if err != nil {
		return nil, fmt.Errorf("Failed parsing URL (%v)", err)
	}

	ln, err := net.Listen("tcp", url.Host)
	if err != nil {
		return nil, err
	}
	return ln.(*net.TCPListener), nil
}
