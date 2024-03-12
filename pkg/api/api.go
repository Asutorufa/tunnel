package api

import (
	"context"
	"io"
	"log/slog"
	"net"

	"github.com/Asutorufa/tunnel/pkg/protomsg"
	"github.com/Asutorufa/yuhaiin/pkg/net/netapi"
	"github.com/Asutorufa/yuhaiin/pkg/net/proxy/simple"
	"github.com/Asutorufa/yuhaiin/pkg/net/proxy/socks5/server"
	"github.com/Asutorufa/yuhaiin/pkg/protos/config/listener"
	"github.com/Asutorufa/yuhaiin/pkg/utils/relay"
)

type Tunnel interface {
	OpenStream(context.Context, *protomsg.Request) (net.Conn, error)
	Close() error
}

func Forward(api Tunnel, Rule map[string]protomsg.Target) {
	for h, v := range Rule {
		go func(h string, v protomsg.Target) {
			if err := forward(api, h, v); err != nil {
				slog.Error("forward failed", "host", h, "target", v, "err", err)
			}
		}(h, v)
	}
}

func forward(api Tunnel, host string, t protomsg.Target) error {
	lis, err := net.Listen("tcp", host)
	if err != nil {
		return err
	}

	slog.Debug("new server", "host", lis.Addr(), "target", t)

	for {
		conn, err := lis.Accept()
		if err != nil {
			return err
		}

		go func() {
			defer conn.Close()

			remote, err := api.OpenStream(context.TODO(), &protomsg.Request{
				Type: protomsg.Type_Connection,
				Payload: &protomsg.Request_Connect{
					Connect: &protomsg.Connect{
						Target:  t.UUID,
						Address: t.Address,
						Port:    uint32(t.Port),
					},
				},
			})
			if err != nil {
				slog.Error("open  stream failed", "host", host, "target", t, "err", err)
				return
			}
			defer remote.Close()

			relay.Relay(remote, conn)
		}()
	}
}

func Socks5Server(host string, api Tunnel) (io.Closer, error) {
	lis, err := simple.NewServer(&listener.Inbound_Tcpudp{
		Tcpudp: &listener.Tcpudp{
			Host: host,
		},
	})
	if err != nil {
		return nil, err
	}

	server, err := server.NewServer(&listener.Inbound_Socks5{
		Socks5: &listener.Socks5{
			Host: host,
			Udp:  false,
		},
	})(lis)
	if err != nil {
		return nil, err
	}

	go func() {
		for {
			sm, err := server.AcceptStream()
			if err != nil {
				break
			}

			go Stream(context.TODO(), api, sm)
		}
	}()

	return lis, nil
}

func Stream(ctx context.Context, api Tunnel, t *netapi.StreamMeta) {
	defer t.Src.Close()

	conn, err := api.OpenStream(ctx, &protomsg.Request{
		Type: protomsg.Type_Connection,
		Payload: &protomsg.Request_Connect{
			Connect: &protomsg.Connect{
				Target:  t.Address.Hostname(),
				Address: "127.0.0.1",
				Port:    uint32(t.Address.Port().Port()),
			},
		},
	})
	if err != nil {
		slog.Error("open stream failed", "target", t, "err", err)
		return
	}

	defer conn.Close()

	relay.Relay(conn, t.Src)
}
