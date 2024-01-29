package tunnel

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"github.com/Asutorufa/yuhaiin/pkg/net/netapi"
	"github.com/Asutorufa/yuhaiin/pkg/net/proxy/simple"
	"github.com/Asutorufa/yuhaiin/pkg/net/proxy/socks5/server"
	"github.com/Asutorufa/yuhaiin/pkg/protos/config/listener"
	"github.com/Asutorufa/yuhaiin/pkg/protos/statistic"
	"github.com/Asutorufa/yuhaiin/pkg/utils/relay"
	"google.golang.org/protobuf/proto"
)

type Target struct {
	UUID    string `json:"uuid"`
	Address string `json:"address"`
	Port    uint16 `json:"port"`
}

type Client struct {
	UUID     string
	Server   string
	S5Dialer netapi.Proxy
	Rule     map[string]Target
}

func (c *Client) Socks5Server(host string) (io.Closer, error) {
	lis, err := simple.NewServer(&listener.Inbound_Tcpudp{
		Tcpudp: &listener.Tcpudp{
			Host: host,
		},
	})
	if err != nil {
		return nil, err
	}

	server, err := server.NewServer(
		&listener.Inbound_Socks5{
			Socks5: &listener.Socks5{
				Host: host,
				Udp:  false,
			},
		})(lis)
	if err != nil {
		lis.Close()
		return nil, err
	}

	go func() {
		for {
			sm, err := server.AcceptStream()
			if err != nil {
				break
			}

			go c.Stream(context.TODO(), sm)
		}
	}()

	return server, nil
}

func (c *Client) Stream(ctx context.Context, t *netapi.StreamMeta) {
	defer t.Src.Close()

	r, err := c.Connect(Target{
		UUID:    t.Address.Hostname(),
		Address: "127.0.0.1",
		Port:    t.Address.Port().Port(),
	})
	if err != nil {
		log.Println(err)
		return
	}
	defer r.Close()

	relay.Relay(r, t.Src)
}

func (c *Client) Packet(ctx context.Context, pack *netapi.Packet) {}

func (c *Client) forward(host string, t Target) error {
	lis, err := net.Listen("tcp", host)
	if err != nil {
		return err
	}

	log.Println("new server", lis.Addr(), "target", t)

	for {
		conn, err := lis.Accept()
		if err != nil {
			return err
		}

		go func() {
			defer conn.Close()

			remote, err := c.Connect(t)
			if err != nil {
				log.Println(err)
				return
			}
			defer remote.Close()

			relay.Relay(remote, conn)
		}()
	}
}

func (c *Client) Connect(t Target) (io.ReadWriteCloser, error) {
	remote, err := c.dial()
	if err != nil {
		return nil, err
	}

	err = SendConnect(remote, &Connect{Target: t.UUID, Address: t.Address, Port: uint32(t.Port)})
	if err != nil {
		remote.Close()
		return nil, err
	}

	return remote, nil
}

func (c *Client) Forward() {
	for h, v := range c.Rule {
		go func(h string, v Target) {
			if err := c.forward(h, v); err != nil {
				log.Println(err)
			}
		}(h, v)
	}
}

func (c *Client) Register() error {
	log.Println("try register to", c.Server)

	conn, err := c.dial()
	if err != nil {
		return err
	}
	defer conn.Close()

	conn.SetWriteDeadline(time.Now().Add(time.Minute))
	if err := SendRegister(func(req []byte) ([]byte, error) {
		err = binary.Write(conn, binary.BigEndian, uint64(len(req)))
		if err != nil {
			return nil, err
		}
		_, err = conn.Write(req)

		var length uint64
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			return nil, err
		}

		data := make([]byte, length)
		if _, err := io.ReadFull(conn, data); err != nil {
			return nil, err
		}
		return data, nil
	}, c.UUID); err != nil {
		return err
	}
	conn.SetWriteDeadline(time.Time{})

	go func() {
		ticker := time.NewTicker(time.Second * 15)
		defer ticker.Stop()

		for range ticker.C {
			if err := SendPing(conn); err != nil {
				log.Println("send ping failed: ", err)
				conn.Close()
				return
			}
		}
	}()

	for {
		err := c.handle(conn)
		if err != nil {
			if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
				return err
			} else {
				log.Println(err)
			}
		}
	}
}

func (c *Client) dial() (net.Conn, error) {
	if c.S5Dialer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()
		saddr, err := netapi.ParseAddress(statistic.Type_tcp, c.Server)
		if err != nil {
			return nil, err
		}
		return c.S5Dialer.Conn(ctx, saddr)
	}

	return net.DialTimeout("tcp", c.Server, time.Second*10)
}

func (c *Client) NewConn(id uint64) (net.Conn, error) {
	resp := &Request{
		Type: Type_Response,
		Payload: &Request_ConnectResponse{
			ConnectResponse: &ConnectResponse{
				Uuid:   c.UUID,
				Connid: id,
			},
		},
	}

	data, err := proto.Marshal(resp)
	if err != nil {
		return nil, err
	}

	conn, err := c.dial()
	if err != nil {
		return nil, err
	}

	err = binary.Write(conn, binary.BigEndian, uint64(len(data)))
	if err != nil {
		conn.Close()
		return nil, err
	}

	_, err = conn.Write(data)
	if err != nil {
		conn.Close()
	}

	return conn, err
}

func (c *Client) handle(lis io.ReadWriter) error {
_retry:
	req, err := getRequestReader(lis)
	if err != nil {
		return err
	}

	switch req.GetType() {
	case Type_Connection:
		go func() {
			if err := c.handleConnect(req); err != nil {
				log.Println("handle connect failed:", err)
			}
		}()

		goto _retry

	case Type_Ping:
		goto _retry
	}

	log.Printf("unknown type: %d\n", req.GetType())
	return nil
}

type Handler interface {
	handle(lis io.ReadWriter) error
	handleConnect(lis io.ReadWriteCloser, req *Request) error
}

func (c *Client) handleConnect(req *Request) error {
	port := req.GetConnect().Port
	address := req.GetConnect().GetAddress()
	if address == "" {
		address = "127.0.0.1"
	}

	log.Println("new request connect to", address, port)

	remote, err := c.NewConn(req.GetConnect().Id)
	if err != nil {
		return err
	}
	defer remote.Close()

	conn, err := (&net.Dialer{Timeout: time.Second * 4}).
		Dial("tcp", net.JoinHostPort(address, fmt.Sprint(port)))
	if err != nil {
		return err
	}
	defer conn.Close()

	relay.Relay(conn, remote)
	return nil
}

// type QuicClient struct {
// 	UUID   string
// 	Server string
// 	Socks5 string
// 	Rule   map[string]Target

// 	connection quic.Connection
// }

// func (c *QuicClient) Socks5Server(host string) (io.Closer, error) {
// 	return server.NewServer(&listener.Opts[*listener.Protocol_Socks5]{
// 		Protocol: &listener.Protocol_Socks5{
// 			Socks5: &listener.Socks5{
// 				Host: host,
// 			},
// 		},
// 		Handler: c,
// 	})
// }

// func (c *QuicClient) Stream(ctx context.Context, t *netapi.StreamMeta) {
// 	defer t.Src.Close()

// 	r, err := c.Connect(Target{
// 		UUID:    t.Address.Hostname(),
// 		Address: "127.0.0.1",
// 		Port:    t.Address.Port().Port(),
// 	})
// 	if err != nil {
// 		log.Println(err)
// 		return
// 	}
// 	defer r.Close()

// 	Relay(r, t.Src)
// }

// func (c *QuicClient) Packet(ctx context.Context, pack *netapi.Packet) { return }

// func (c *QuicClient) forward(host string, t Target) error {
// 	lis, err := net.Listen("tcp", host)
// 	if err != nil {
// 		return err
// 	}

// 	log.Println("new server", lis.Addr(), "target", t)

// 	for {
// 		conn, err := lis.Accept()
// 		if err != nil {
// 			return err
// 		}

// 		go func() {
// 			defer conn.Close()

// 			remote, err := c.Connect(t)
// 			if err != nil {
// 				log.Println(err)
// 				return
// 			}
// 			defer remote.Close()

// 			Relay(remote, conn)
// 		}()
// 	}
// }

// func (c *QuicClient) Connect(t Target) (io.ReadWriteCloser, error) {
// 	if c.connection == nil {
// 		return nil, net.ErrClosed
// 	}

// 	remote, err := c.connection.OpenStream()
// 	if err != nil {
// 		return nil, err
// 	}

// 	err = SendConnect(remote, &Connect{Target: t.UUID, Address: t.Address, Port: uint32(t.Port)})
// 	if err != nil {
// 		remote.Close()
// 		return nil, err
// 	}

// 	return remote, nil
// }

// func (c *QuicClient) Forward() {
// 	for h, v := range c.Rule {
// 		go func(h string, v Target) {
// 			if err := c.forward(h, v); err != nil {
// 				log.Println(err)
// 			}
// 		}(h, v)
// 	}
// }

// func (c *QuicClient) Register() error {
// 	log.Println("try register to", c.Server)

// 	conn, err := quic.DialAddr(context.TODO(),
// 		c.Server,
// 		&tls.Config{
// 			InsecureSkipVerify: true,
// 		},
// 		&quic.Config{
// 			KeepAlivePeriod: time.Second * 10,
// 		},
// 	)
// 	if err != nil {
// 		return err
// 	}
// 	defer conn.CloseWithError(0, "")

// 	if err := SendRegister(func(req []byte) ([]byte, error) {
// 		if err := conn.SendMessage(req); err != nil {
// 			return nil, err
// 		}
// 		return conn.ReceiveMessage(context.TODO())
// 	}, c.UUID); err != nil {
// 		return err
// 	}

// 	c.connection = conn

// 	for {
// 		stream, err := conn.AcceptStream(context.TODO())
// 		if err != nil {
// 			return err
// 		}

// 		go func() {
// 			defer stream.Close()
// 			req, err := getRequestReader(stream)
// 			if err != nil {
// 				log.Println(err)
// 				return
// 			}

// 			conn, err := (&net.Dialer{Timeout: time.Second * 4}).Dial("tcp",
// 				net.JoinHostPort(req.GetConnect().GetAddress(), fmt.Sprint(req.GetConnect().GetPort())))
// 			if err != nil {
// 				log.Println(err)
// 				return
// 			}
// 			defer conn.Close()

// 			Relay(conn, stream)
// 		}()

// 	}
// }
