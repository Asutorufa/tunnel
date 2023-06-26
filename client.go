package tunnel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"

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
	conn, err := c.dial()
	if err != nil {
		return err
	}

	req := &Request{
		Type: Type_Register,
		Payload: &Request_Device{
			Device: &Device{
				Uuid: c.UUID,
			},
		},
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	binary.Write(conn, binary.BigEndian, uint64(len(data)))
	_, err = conn.Write(data)
	if err != nil {
		return err
	}

	resp, err := getRequest(conn)
	if err != nil {
		return err
	}

	if resp.Type == Type_Error {
		return errors.New(resp.GetError().GetMsg())
	}

	if resp.Type != Type_Ok {
		return fmt.Errorf("unknown type: %d", resp.Type)
	}

	for {
		log.Println("start handle")
		err := c.handle(conn)
		if errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) {
			return err
		} else {
			log.Println(err)
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

	binary.Write(conn, binary.BigEndian, uint64(len(data)))
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
		go func() {
			port := req.GetConnect().Port
			address := req.GetConnect().GetAddress()
			if address == "" {
				address = "127.0.0.1"
			}
			conn, err := net.Dial("tcp", net.JoinHostPort(address, fmt.Sprint(port)))
			if err != nil {
				log.Println(err)
				return
			}
			defer conn.Close()

			if err := SendOk(lis); err != nil {
				log.Println(err)
				return
			}

			log.Println("send ok to", req.GetConnect().Id)

			remote, err := c.NewConn(req.GetConnect().Id)
			if err != nil {
				SendError(lis, err)
				log.Println(err)
				return
			}
			defer remote.Close()

			Relay(conn, remote)
		}()

		return nil
	}

	return fmt.Errorf("unknown type: %d", req.GetType())
}
