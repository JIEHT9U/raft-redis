package init

import (
	"fmt"
	"net"
	"os"

	"github.com/alecthomas/kingpin"
)

//Params type
type Params struct {
	ListenAddr  *net.TCPAddr
	RaftPeers   string
	RaftJoin    bool
	NodeID      int
	RaftDataDir string
}

//Param return init param
func Param() (*Params, error) {
	init := &Params{}

	a := kingpin.New("go-redis", "Implementation redis on Go")
	a.HelpFlag.Short('h')

	a.Flag("listen.addr", "The address the go-redis listens on for incoming TCP connections").
		Envar("LISTEN_ADDR").
		Default("0.0.0.0:3000").
		TCPVar(&init.ListenAddr)

	a.Flag("initial-cluster", "comma separated cluster peers").
		Envar("INITIAL_CLUSTER").
		Default("http://127.0.0.1:9021").
		StringVar(&init.RaftPeers)

	a.Flag("raft-data-dir", "data dir").
		Envar("RAFT_DATA_DIR").
		Default("data").
		StringVar(&init.RaftDataDir)

	a.Flag("join", "join an existing cluster").
		Envar("JOIN").
		BoolVar(&init.RaftJoin)

	a.Flag("id", "node ID").
		Envar("ID").
		Default("1").
		IntVar(&init.NodeID)

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		a.Usage(os.Args[1:])
		return init, fmt.Errorf("error parsing commandline arguments: %v", err)
	}
	return init, nil
}
