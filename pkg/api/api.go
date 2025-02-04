package api

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strings"

	"github.com/Asutorufa/tunnel/pkg/protomsg"
	"github.com/Asutorufa/yuhaiin/pkg/net/netapi"
	"github.com/Asutorufa/yuhaiin/pkg/net/proxy/simple"
	"github.com/Asutorufa/yuhaiin/pkg/net/proxy/socks5"
	"github.com/Asutorufa/yuhaiin/pkg/protos/config/listener"
	"github.com/Asutorufa/yuhaiin/pkg/utils/relay"
	"google.golang.org/protobuf/proto"
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

type HandlerFunc func(*netapi.StreamMeta)

func (h HandlerFunc) HandleStream(s *netapi.StreamMeta) { h(s) }
func (h HandlerFunc) HandlePacket(*netapi.Packet)       {}

func Socks5Server(host string, api Tunnel) (io.Closer, error) {
	lis, err := simple.NewServer(listener.Tcpudp_builder{
		Host: proto.String(host),
	}.Build())
	if err != nil {
		return nil, err
	}

	server, err := socks5.NewServer(listener.Socks5_builder{
		Udp: proto.Bool(false),
	}.Build(), lis, HandlerFunc(func(s *netapi.StreamMeta) {
		go Stream(context.TODO(), api, s)
	}))
	if err != nil {
		return nil, err
	}

	return server, nil
}

func Stream(ctx context.Context, api Tunnel, t *netapi.StreamMeta) {
	defer t.Src.Close()

	var address, device string
	if i := strings.LastIndexByte(t.Address.Hostname(), '.'); i != -1 {
		address = t.Address.Hostname()[:i]
		device = t.Address.Hostname()[i+1:]
	} else {
		address = "127.0.0.1"
		device = t.Address.Hostname()
	}

	conn, err := api.OpenStream(ctx, &protomsg.Request{
		Type: protomsg.Type_Connection,
		Payload: &protomsg.Request_Connect{
			Connect: &protomsg.Connect{
				Target:  device,
				Address: address,
				Port:    uint32(t.Address.Port()),
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
