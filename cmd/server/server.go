package main

import (
	"encoding/json"
	"flag"
	"log/slog"
	"net"
	"os"

	"github.com/Asutorufa/tunnel/pkg/api"
	"github.com/Asutorufa/tunnel/pkg/protomsg"
	tunnelserver "github.com/Asutorufa/tunnel/pkg/server"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)

	host := flag.String("h", "127.0.0.1:8388", "host, -h 127.0.0.1:8388")
	rule := flag.String("r", "rule.json", "rules, -r config.json")
	socks5server := flag.String("s5server", "127.0.0.1:1081", "socks5 server, -s5server 127.0.0.1:1081")
	flag.Parse()

	lis, err := net.Listen("tcp", *host)
	if err != nil {
		panic(err)
	}

	var Rule map[string]protomsg.Target

	data, err := os.ReadFile(*rule)
	if err == nil {
		if err := json.Unmarshal(data, &Rule); err != nil {
			slog.Error("Unmarshal rule  failed", "err", err)
		}
		slog.Debug("rule", "rule", Rule)
	} else {
		slog.Error("read rule failed", "err", err)
	}

	slog.Debug("new server", "host", lis.Addr())

	s := tunnelserver.NewServer()

	api.Forward(s, Rule)
	s5, err := api.Socks5Server(*socks5server, s)
	if err != nil {
		slog.Error("new socks5 server failed", "err", err)
	} else {
		defer s5.Close()
	}

	for {
		conn, err := lis.Accept()
		if err != nil {
			return
		}

		go func() {
			if err := s.Handle(conn); err != nil {
				slog.Error("handle failed", "err", err)
				conn.Close()
			}
		}()
	}
}
