package tunnel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"golang.org/x/net/proxy"
	"google.golang.org/protobuf/proto"
)

type Target struct {
	UUID    string `json:"uuid"`
	Address string `json:"address"`
	Port    uint16 `json:"port"`
}

type Client struct {
	UUID   string
	Server string
	Socks5 string
	Rule   map[string]Target
}

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

			remote, err := c.dial()
			if err != nil {
				log.Println(err)
				return
			}
			defer remote.Close()

			err = SendConnect(remote, &Connect{
				Target:  t.UUID,
				Address: t.Address,
				Port:    uint32(t.Port),
			})
			if err != nil {
				log.Println(err)
				return
			}

			Relay(remote, conn)
		}()
	}
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

	if err := SendRegister(conn, c.UUID); err != nil {
		return err
	}

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
	if c.Socks5 != "" {
		dialer, err := proxy.SOCKS5("tcp", c.Socks5, nil, nil)
		if err != nil {
			return nil, err
		}

		return dialer.Dial("tcp", c.Server)
	}
	return net.Dial("tcp", c.Server)
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

func (c *Client) handle(lis net.Conn) error {
	req, err := getRequest(lis)
	if err != nil {
		return err
	}

	switch req.GetType() {
	case Type_Connection:
		if err := SendOk(lis); err != nil {
			return err
		}
		go func() {
			if err := c.handleConnect(req); err != nil {
				log.Println("handle connect failed:", err)
			}
		}()

		return nil

	case Type_Ping:
		if err := SendOk(lis); err != nil {
			return err
		}
		return nil
	}

	return fmt.Errorf("unknown type: %d", req.GetType())
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

	conn, err := (&net.Dialer{Timeout: time.Second * 4}).Dial("tcp", net.JoinHostPort(address, fmt.Sprint(port)))
	if err != nil {
		return err
	}
	defer conn.Close()

	Relay(conn, remote)
	return nil
}
