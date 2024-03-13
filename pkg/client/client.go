package tunnelclient

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/Asutorufa/tunnel/pkg/protomsg"
	"github.com/Asutorufa/yuhaiin/pkg/net/netapi"
	"github.com/Asutorufa/yuhaiin/pkg/protos/statistic"
	"github.com/Asutorufa/yuhaiin/pkg/utils/relay"
)

type Client struct {
	UUID     string
	Server   string
	S5Dialer netapi.Proxy
}

func (c *Client) OpenStream(ctx context.Context, t *protomsg.Request) (net.Conn, error) {
	remote, err := c.connectServer()
	if err != nil {
		return nil, err
	}

	err = protomsg.SendRequest(remote, t)
	if err != nil {
		remote.Close()
		return nil, err
	}

	return remote, nil
}

func (c *Client) Register() error {
	slog.Debug("try register to", "server", c.Server)

	conn, err := c.connectServer()
	if err != nil {
		return err
	}
	defer conn.Close()

	_ = conn.SetWriteDeadline(time.Now().Add(time.Minute))
	if err := protomsg.SendRegister(conn, c.UUID); err != nil {
		return err
	}
	_ = conn.SetWriteDeadline(time.Time{})

	slog.Debug("register success", "server", c.Server)

	go func() {
		ticker := time.NewTicker(time.Second * 15)
		defer ticker.Stop()

		for range ticker.C {
			if err := protomsg.SendPing(conn); err != nil {
				slog.Error("send ping failed", "err", err)
				conn.Close()
				return
			}
		}
	}()

	for {
		if err := c.handle(conn); err != nil {
			return err
		}
	}
}

func (c *Client) connectServer() (net.Conn, error) {
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

func (c *Client) handle(lis io.ReadWriter) error {
	req, err := protomsg.GetRequestReader(lis)
	if err != nil {
		return err
	}

	switch req.GetType() {
	case protomsg.Type_Connection:
		go func() {
			if err := c.handleConnect(req); err != nil {
				slog.Error("handle connect failed", "err", err)
			}
		}()

	case protomsg.Type_Ping:
	default:
		slog.Error("unknown request type", "type", req.GetType())
	}

	return nil
}

func (c *Client) handleConnect(req *protomsg.Request) error {
	port := req.GetConnect().Port
	address := req.GetConnect().GetAddress()
	if address == "" {
		address = "127.0.0.1"
	}

	slog.Debug("connect", "address", address, "port", port)

	remote, err := c.connectServer()
	if err != nil {
		return err
	}
	defer remote.Close()

	err = protomsg.SendRequest(remote, &protomsg.Request{
		Type: protomsg.Type_Response,
		Payload: &protomsg.Request_ConnectResponse{
			ConnectResponse: &protomsg.ConnectResponse{
				Uuid:   c.UUID,
				Connid: req.GetConnect().Id,
			},
		},
	})
	if err != nil {
		return err
	}

	conn, err := net.DialTimeout("tcp", net.JoinHostPort(address, fmt.Sprint(port)), time.Second*5)
	if err != nil {
		return err
	}
	defer conn.Close()

	relay.Relay(conn, remote)
	return nil
}

func (c *Client) Close() error {
	return nil
}
