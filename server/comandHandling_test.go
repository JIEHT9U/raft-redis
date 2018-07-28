package server

import (
	"net"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"
)

func Test_command_String(t *testing.T) {
	tests := []struct {
		name string
		c    command
		want string
	}{
		{name: "SET", c: set, want: "SET"},
		{name: "GET", c: get, want: "GET"},
		{name: "DEL", c: del, want: "DEL"},
		{name: "HGET", c: hget, want: "HGET"},
		{name: "HSET", c: hset, want: "HSET"},
		{name: "HGETALL", c: hgetall, want: "HGETALL"},
		{name: "LLEN", c: llen, want: "LLEN"},
		{name: "LGET", c: lget, want: "LGET"},
		{name: "RPUSH", c: rpush, want: "RPUSH"},
		{name: "LPUSH", c: lpush, want: "LPUSH"},
		{name: "EXPIRE", c: expire, want: "EXPIRE"},
		{name: "TTL", c: ttl, want: "TTL"},
		{name: "TTL", c: command(22), want: "error map cmd to string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.c.String(); got != tt.want {
				t.Errorf("command.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func imitaionReqChan() chan request {
	req := make(chan request)
	go func() {
		for r := range req {
			r.response <- resp{data: nil, err: nil}
		}
	}()
	return req
}

func imitaionReqChanWaiter() chan request {
	req := make(chan request)
	go func() {
		for r := range req {
			time.Sleep(time.Second * 4)
			select {
			case r.response <- resp{data: nil, err: nil}:
			default:
				// close(r.response)
			}
		}
	}()
	return req
}

func TestServer_commandHandling(t *testing.T) {

	type fields struct {
		listenAddr *net.TCPAddr
		logger     *zap.SugaredLogger
		requests   chan request
		getSnap    chan storages
		raft       *raftNode
		st         storages
	}
	type args struct {
		cmds []string
	}

	req := imitaionReqChan()
	reqWait := imitaionReqChanWaiter()
	// defer close(req)

	tests := []struct {
		name    string
		fields  fields
		args    args
		want    []byte
		wantErr bool
	}{
		{
			name:    "Expected at least 2 arguments",
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Expected at least 2 arguments",
			fields:  fields{requests: req},
			args:    args{cmds: []string{"set"}},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Err command Parse",
			args:    args{cmds: []string{"sdfsdfs", "sdsdf"}},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Error send data requests channel is full",
			fields:  fields{requests: make(chan request)},
			args:    args{cmds: []string{"set", "t", "value"}},
			want:    nil,
			wantErr: true,
		},
		{
			name:    "Success",
			fields:  fields{requests: req},
			args:    args{cmds: []string{"set", "t", "value"}},
			want:    nil,
			wantErr: false,
		},
		{
			name:    "Error get data from channel resp time out",
			fields:  fields{requests: reqWait},
			args:    args{cmds: []string{"set", "t", "value"}},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Server{
				listenAddr: tt.fields.listenAddr,
				logger:     tt.fields.logger,
				requests:   tt.fields.requests,
				getSnap:    tt.fields.getSnap,
				raft:       tt.fields.raft,
				st:         tt.fields.st,
			}
			got, err := s.commandHandling(tt.args.cmds)
			if (err != nil) != tt.wantErr {
				t.Errorf("Server.commandHandling() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Server.commandHandling() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_commandParse(t *testing.T) {
	type args struct {
		cmds []string
	}
	tests := []struct {
		name    string
		args    args
		wantCmd cmd
		wantErr bool
	}{
		{
			name:    "Error parse Command Type empty input args",
			args:    args{cmds: []string{""}},
			wantCmd: cmd{},
			wantErr: true,
		},
		{
			name:    "Error parse Command Type undefined cmd",
			args:    args{cmds: []string{"sfsdf", "sdfsdf", "sfgdfg"}},
			wantCmd: cmd{},
			wantErr: true,
		},
		{
			name:    "expire Err Expected 3 arguments",
			args:    args{cmds: []string{"expire", "t"}},
			wantCmd: cmd{Actions: expire},
			wantErr: true,
		},
		{
			name:    "expire Err parse dudation",
			args:    args{cmds: []string{"expire", "t", "dudation"}},
			wantCmd: cmd{Actions: expire, Key: "t"},
			wantErr: true,
		},
		{
			name:    "del Expected  2 arguments",
			args:    args{cmds: []string{"del"}},
			wantCmd: cmd{Actions: del},
			wantErr: true,
		},
		{
			name:    "get Expected  2 arguments",
			args:    args{cmds: []string{"get"}},
			wantCmd: cmd{Actions: get},
			wantErr: true,
		},
		{
			name:    "llen Expected  2 arguments",
			args:    args{cmds: []string{"llen"}},
			wantCmd: cmd{Actions: llen},
			wantErr: true,
		},
		{
			name:    "ttl Expected  2 arguments",
			args:    args{cmds: []string{"ttl"}},
			wantCmd: cmd{Actions: ttl},
			wantErr: true,
		},
		{
			name:    "hgetall Expected  2 arguments",
			args:    args{cmds: []string{"hgetall"}},
			wantCmd: cmd{Actions: hgetall},
			wantErr: true,
		},
		{
			name:    "del",
			args:    args{cmds: []string{"del", "t"}},
			wantCmd: cmd{Actions: del, Key: "t"},
			wantErr: false,
		},
		{
			name:    "get",
			args:    args{cmds: []string{"get", "t"}},
			wantCmd: cmd{Actions: get, Key: "t"},
			wantErr: false,
		},
		{
			name:    "llen",
			args:    args{cmds: []string{"llen", "t"}},
			wantCmd: cmd{Actions: llen, Key: "t"},
			wantErr: false,
		},
		{
			name:    "ttl",
			args:    args{cmds: []string{"ttl", "t"}},
			wantCmd: cmd{Actions: ttl, Key: "t"},
			wantErr: false,
		},
		{
			name:    "hgetall",
			args:    args{cmds: []string{"hgetall", "t"}},
			wantCmd: cmd{Actions: hgetall, Key: "t"},
			wantErr: false,
		},
		{
			name:    "lget Err Expected 4 arguments",
			args:    args{cmds: []string{"lget"}},
			wantCmd: cmd{Actions: lget},
			wantErr: true,
		},
		{
			name:    "lget",
			args:    args{cmds: []string{"lget", "t", "0", "-1"}},
			wantCmd: cmd{Actions: lget, Expire: 0, Key: "t", Values: []string{"0", "-1"}},
			wantErr: false,
		},
		{
			name:    "Expected at list 3 arguments lpush",
			args:    args{cmds: []string{"lpush", "t"}},
			wantCmd: cmd{Actions: lpush},
			wantErr: true,
		},
		{
			name:    "Expected at list 3 arguments rpush",
			args:    args{cmds: []string{"rpush", "t"}},
			wantCmd: cmd{Actions: rpush},
			wantErr: true,
		},
		{
			name:    "Expected at list 3 arguments set",
			args:    args{cmds: []string{"set", "t"}},
			wantCmd: cmd{Actions: set},
			wantErr: true,
		},
		{
			name:    "Expected at list 3 arguments hset",
			args:    args{cmds: []string{"hset", "t"}},
			wantCmd: cmd{Actions: hset},
			wantErr: true,
		},
		{
			name:    "Expected at list 3 arguments hget",
			args:    args{cmds: []string{"hget", "t"}},
			wantCmd: cmd{Actions: hget},
			wantErr: true,
		},
		{
			name:    "lpush",
			args:    args{cmds: []string{"lpush", "t", "value"}},
			wantCmd: cmd{Actions: lpush, Key: "t", Values: []string{"value"}},
			wantErr: false,
		},
		{
			name:    "rpush",
			args:    args{cmds: []string{"rpush", "t", "value"}},
			wantCmd: cmd{Actions: rpush, Key: "t", Values: []string{"value"}},
			wantErr: false,
		},
		{
			name:    "set",
			args:    args{cmds: []string{"set", "t", "value"}},
			wantCmd: cmd{Actions: set, Key: "t", Values: []string{"value"}},
			wantErr: false,
		},
		{
			name:    "hset",
			args:    args{cmds: []string{"hset", "t", "value"}},
			wantCmd: cmd{Actions: hset, Key: "t", Values: []string{"value"}},
			wantErr: false,
		},
		{
			name:    "hget",
			args:    args{cmds: []string{"hget", "t", "value"}},
			wantCmd: cmd{Actions: hget, Key: "t", Values: []string{"value"}},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCmd, err := commandParse(tt.args.cmds)
			if (err != nil) != tt.wantErr {
				t.Errorf("commandParse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(gotCmd, tt.wantCmd) {
				t.Errorf("commandParse() = %v, want %v", gotCmd, tt.wantCmd)
			}
		})
	}
}

func Test_parseCommandType(t *testing.T) {
	type args struct {
		cmd string
	}
	tests := []struct {
		name    string
		args    args
		want    command
		wantErr bool
	}{
		{name: "set", args: args{cmd: "set"}, want: set, wantErr: false},
		{name: "get", args: args{cmd: "get"}, want: get, wantErr: false},
		{name: "del", args: args{cmd: "del"}, want: del, wantErr: false},
		{name: "hget", args: args{cmd: "hget"}, want: hget, wantErr: false},
		{name: "hset", args: args{cmd: "hset"}, want: hset, wantErr: false},
		{name: "hgetall", args: args{cmd: "hgetall"}, want: hgetall, wantErr: false},
		{name: "llen", args: args{cmd: "llen"}, want: llen, wantErr: false},
		{name: "lget", args: args{cmd: "lget"}, want: lget, wantErr: false},
		{name: "rpush", args: args{cmd: "rpush"}, want: rpush, wantErr: false},
		{name: "lpush", args: args{cmd: "lpush"}, want: lpush, wantErr: false},
		{name: "expire", args: args{cmd: "expire"}, want: expire, wantErr: false},
		{name: "ttl", args: args{cmd: "ttl"}, want: ttl, wantErr: false},
		{name: "Unknown command", args: args{cmd: "sfsdgsg"}, want: command(0), wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCommandType(tt.args.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseCommandType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseCommandType() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseDudation(t *testing.T) {
	type args struct {
		expire      string
		currentTime time.Time
	}

	timeNow := time.Now()

	tests := []struct {
		name    string
		args    args
		want    int64
		wantErr bool
	}{
		{name: "0", args: args{expire: "0", currentTime: timeNow}, want: -1, wantErr: false},
		{name: "Error convert string to int", args: args{expire: "sfsdf", currentTime: timeNow}, want: 0, wantErr: true},
		{name: "30s", args: args{expire: "30", currentTime: timeNow}, want: timeNow.Add(time.Duration(30) * time.Second).UnixNano(), wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseDudation(timeNow, tt.args.expire)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDudation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseDudation() = %v, want %v", got, tt.want)
			}
		})
	}
}
