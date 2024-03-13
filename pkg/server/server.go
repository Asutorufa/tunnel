package tunnelserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Asutorufa/tunnel/pkg/protomsg"
	"github.com/Asutorufa/yuhaiin/pkg/utils/relay"
	"github.com/Asutorufa/yuhaiin/pkg/utils/syncmap"
)

type Server struct {
	devices *Devices
	*Chan
}

func NewServer() *Server {
	return &Server{
		devices: &Devices{},
		Chan:    &Chan{},
	}
}

func (s *Server) Handle(c net.Conn) error {
	req, err := protomsg.GetRequestReader(c)
	if err != nil {
		return err
	}

	slog.Debug("new request", "type", req.GetType(), "remoteAddr", c.RemoteAddr())

	switch req.GetType() {
	case protomsg.Type_Register:
		return s.devices.RegisterDevice(req.GetDevice().Uuid, c)
	case protomsg.Type_Connection:
		defer c.Close()
		remote, err := s.OpenStream(context.TODO(), req)
		if err != nil {
			return err
		}
		defer remote.Close()
		relay.Relay(remote, c)
		return nil
	case protomsg.Type_Response:
		s.SendChan(req.GetConnectResponse().Connid, c)
		return nil
	}

	return fmt.Errorf("unknown type: %d", req.GetType())
}

func (s *Server) OpenStream(ctx context.Context, req *protomsg.Request) (net.Conn, error) {
	device, ok := s.devices.devices.Load(req.GetConnect().Target)
	if !ok {
		return nil, fmt.Errorf("device %s is not exist", req.GetConnect().Target)
	}

	id, ch := s.NewChan()
	defer s.RemoveChan(id)

	req.GetConnect().Id = id

	slog.Debug("new request", "target", req.GetConnect(), "chan_id", id)

	err := device.Connect(req)
	if err != nil {
		return nil, err
	}

	select {
	case conn := <-ch:
		return conn, nil
	case <-time.After(time.Second * 10):
		return nil, fmt.Errorf("timeout")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (s *Server) Close() error {
	return nil
}

type Chan struct {
	IDChan syncmap.SyncMap[uint64, chan net.Conn]
	ID     atomic.Uint64
}

func (c *Chan) NewChan() (uint64, chan net.Conn) {
	id := c.ID.Add(1)
	ch := make(chan net.Conn, 2)
	c.IDChan.Store(id, ch)

	return id, ch
}

func (c *Chan) RemoveChan(id uint64) { c.IDChan.Delete(id) }

func (c *Chan) SendChan(id uint64, conn net.Conn) {
	slog.Debug("send resp to conn id", "conn_id", id)
	ch, ok := c.IDChan.Load(id)
	if !ok {
		conn.Close()
		return
	}
	ch <- conn
}

type Devices struct {
	mu      sync.Mutex
	devices syncmap.SyncMap[string, *Device]
}

func (d *Devices) RegisterDevice(uuid string, conn net.Conn) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	dd, ok := d.devices.LoadAndDelete(uuid)
	if ok {
		dd.conn.Close()
	}

	device := NewDevice(conn)

	d.devices.Store(uuid, device)

	if err := protomsg.SendOk(conn); err != nil {
		conn.Close()
		return err
	}

	slog.Debug("new device", "uuid", uuid)

	go func() {
		defer func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			dv, ok := d.devices.LoadAndDelete(uuid)
			if ok {
				slog.Debug("delete device", "uuid", uuid)
				dv.conn.Close()
			}
			_ = conn.Close()
		}()

		for {
			req, err := protomsg.GetRequestReader(conn)
			if err != nil {
				slog.Error("get req failed", "err", err, "uuid", uuid)
				return
			}

			switch req.GetType() {
			case protomsg.Type_Ping:
				if err := device.Keepalive(); err != nil {
					slog.Error("send keepalive failed:", "err", err, "uuid", uuid)
					return
				}
			}
		}
	}()

	return nil
}

type Device struct{ conn net.Conn }

func NewDevice(conn net.Conn) *Device                 { return &Device{conn} }
func (d *Device) Keepalive() error                    { return protomsg.SendPing(d.conn) }
func (d *Device) Connect(req *protomsg.Request) error { return protomsg.SendRequest(d.conn, req) }
