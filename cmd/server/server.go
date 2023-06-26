package main

import (
	"flag"
	"log"
	"net"
	"tunnel"
)

func main() {
	host := flag.String("h", "127.0.0.1:8388", "host, -h 127.0.0.1:8388")
	flag.Parse()

	lis, err := net.Listen("tcp", *host)
	if err != nil {
		panic(err)
	}

	log.Println("new server", lis.Addr())

	s := tunnel.NewServerM()
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
