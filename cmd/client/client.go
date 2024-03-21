package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net"
	"os"
	"time"

	"github.com/Asutorufa/tunnel/pkg/api"
	tunnelclient "github.com/Asutorufa/tunnel/pkg/client"
	"github.com/Asutorufa/tunnel/pkg/protomsg"
	"github.com/Asutorufa/yuhaiin/pkg/net/netapi"
	"github.com/Asutorufa/yuhaiin/pkg/net/proxy/socks5"
)

func main() {
	slog.SetLogLoggerLevel(slog.LevelDebug)
	uuid := flag.String("uuid", "uuid", "uuid, -uuid xedsfd")
	server := flag.String("s", "127.0.0.1:8388", "server, -s 127.0.0.1:8388")
	socks5host := flag.String("s5", "", "socks5 proxy, -s5 127.0.0.1:1080")
	rule := flag.String("r", "rule.json", "rules, -r config.json")
	socks5server := flag.String("s5server", "127.0.0.1:1081", "socks5 server, -s5server 127.0.0.1:1081")
	flag.Parse()

	var ruleT map[string]protomsg.Target
	data, err := os.ReadFile(*rule)
	if err == nil {
		if err := json.Unmarshal(data, &ruleT); err != nil {
			slog.Error("unmarshal rule", "err", err)
		}
	} else {
		slog.Error("read rule failed", "err", err)
	}

	var p netapi.Proxy
	host, port, err := net.SplitHostPort(*socks5host)
	if err != nil {
		slog.Error("split proxy host port", "err", err)
	} else {
		p = socks5.Dial(host, port, "", "")
	}

	c := &tunnelclient.Client{UUID: *uuid, Server: *server, S5Dialer: p}

	s, err := api.Socks5Server(*socks5server, c)
	if err != nil {
		slog.Error("new socks5server failed", "err", err)
	} else {
		defer s.Close()
	}

	api.Forward(c, ruleT)

	for {
		start := time.Now()
		if err := c.Register(); err != nil {
			if !errors.Is(err, io.EOF) {
				slog.Error("register failed", "err", err)
			}
		}

		if time.Since(start) < time.Second*5 {
			time.Sleep(5 * time.Second)
		}
	}
}
