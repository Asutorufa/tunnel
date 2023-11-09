package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"os"
	"tunnel"
)

func main() {
	host := flag.String("h", "127.0.0.1:8388", "host, -h 127.0.0.1:8388")
	rule := flag.String("r", "rule.json", "rules, -r config.json")
	socks5server := flag.String("s5server", "127.0.0.1:1081", "socks5 server, -s5server 127.0.0.1:1081")
	flag.Parse()

	lis, err := net.Listen("tcp", *host)
	if err != nil {
		panic(err)
	}

	var Rule map[string]tunnel.Target

	data, err := os.ReadFile(*rule)
	if err == nil {
		if err := json.Unmarshal(data, &Rule); err != nil {
			log.Println(err)
		}
		log.Println(Rule)
	} else {
		log.Println(err)
	}

	log.Println("new server", lis.Addr())

	s := tunnel.NewServerM()

	s.Forward(Rule)
	s5, err := s.Socks5Server(*socks5server)
	if err != nil {
		log.Println(err)
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
				log.Println(err)
				conn.Close()
			}
		}()
	}
}
