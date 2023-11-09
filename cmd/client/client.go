package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"net"
	"os"
	"time"
	"tunnel"

	s5c "github.com/Asutorufa/yuhaiin/pkg/net/proxy/socks5/client"
)

type client interface {
	Socks5Server(host string) (io.Closer, error)
	Register() error
	Forward()
}

func main() {
	uuid := flag.String("uuid", "uuid", "uuid, -uuid xedsfd")
	server := flag.String("s", "127.0.0.1:8388", "server, -s 127.0.0.1:8388")
	socks5 := flag.String("s5", "", "socks5 proxy, -s5 127.0.0.1:1080")
	rule := flag.String("r", "rule.json", "rules, -r config.json")
	quic := flag.Bool("quic", false, "quic, -quic")
	socks5server := flag.String("s5server", "127.0.0.1:1081", "socks5 server, -s5server 127.0.0.1:1081")
	flag.Parse()

	var ruleT map[string]tunnel.Target
	data, err := os.ReadFile(*rule)
	if err == nil {
		if err := json.Unmarshal(data, &ruleT); err != nil {
			log.Println(err)
		}
	} else {
		log.Println(err)
	}

	var c client

	if *quic {
		// c = &tunnel.QuicClient{UUID: *uuid, Server: *server, Socks5: *socks5, Rule: ruleT}
	} else {
		host, port, _ := net.SplitHostPort(*socks5)
		c = &tunnel.Client{UUID: *uuid, Server: *server, S5Dialer: s5c.Dial(host, port, "", ""), Rule: ruleT}
	}

	s, err := c.Socks5Server(*socks5server)
	if err != nil {
		log.Println(err)
	} else {
		defer s.Close()
	}

	c.Forward()

	for {
		start := time.Now()
		if err := c.Register(); err != nil {
			if !errors.Is(err, io.EOF) {
				log.Println(err)
			}
		}

		if time.Since(start) < time.Second*5 {
			time.Sleep(5 * time.Second)
		}
	}
}
