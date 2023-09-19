package main

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log"
	"os"
	"time"
	"tunnel"
)

func main() {
	uuid := flag.String("uuid", "uuid", "uuid, -uuid xedsfd")
	server := flag.String("s", "127.0.0.1:8388", "server, -s 127.0.0.1:8388")
	socks5 := flag.String("s5", "", "socks5 proxy, -s5 127.0.0.1:1080")
	rule := flag.String("r", "rule.json", "rules, -r config.json")
	flag.Parse()

	c := &tunnel.Client{
		UUID:   *uuid,
		Server: *server,
		Socks5: *socks5,
	}
	data, err := os.ReadFile(*rule)
	if err == nil {
		if err := json.Unmarshal(data, &c.Rule); err != nil {
			log.Println(err)
		}
	} else {
		log.Println(err)
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
