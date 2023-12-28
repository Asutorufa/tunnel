package tunnel

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Asutorufa/yuhaiin/pkg/net/netapi"
	"github.com/Asutorufa/yuhaiin/pkg/net/proxy/socks5/server"
	"github.com/Asutorufa/yuhaiin/pkg/protos/config/listener"
	"github.com/Asutorufa/yuhaiin/pkg/utils/relay"
	"github.com/Asutorufa/yuhaiin/pkg/utils/syncmap"
	"google.golang.org/protobuf/proto"
)

//go:generate protoc --go_out=. --go_opt=paths=source_relative message.proto

type ServerM struct {
	devices *Devices
	chanm   *Chan
}

func NewServerM() *ServerM {
	return &ServerM{
		devices: &Devices{},
		chanm:   &Chan{},
	}
}

func (s *ServerM) Handle(c net.Conn) error {
	log.Println("new request from: ", c.RemoteAddr())

	req, err := getRequestReader(c)
	if err != nil {
		return err
	}

	switch req.GetType() {
	case Type_Register:
		if err := s.devices.RegisterDevice(req.GetDevice().Uuid, c); err != nil {
			return err
		}
		log.Println("new device:", req.GetDevice().Uuid)
		return nil
	case Type_Connection:
		return s.ConnectT(c, req)
	case Type_Response:
		log.Println("send resp to conn id", req.GetConnectResponse().Connid)
		s.chanm.SendChan(req.GetConnectResponse().Connid, c)
		return nil
	}

	return fmt.Errorf("unknown type: %d", req.GetType())
}

func (c *ServerM) forward(host string, t Target) error {
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
			err = c.ConnectT(conn, &Request{
				Type: Type_Connection,
				Payload: &Request_Connect{
					Connect: &Connect{
						Target:  t.UUID,
						Address: t.Address,
						Port:    uint32(t.Port),
					},
				},
			})
			if err != nil {
				log.Println(err)
			}

		}()
	}
}

func (c *ServerM) Forward(Rule map[string]Target) {
	for h, v := range Rule {
		go func(h string, v Target) {
			if err := c.forward(h, v); err != nil {
				log.Println(err)
			}
		}(h, v)
	}
}

func (c *ServerM) Socks5Server(host string) (io.Closer, error) {
	return server.NewServer(&listener.Opts[*listener.Protocol_Socks5]{
		Protocol: &listener.Protocol_Socks5{
			Socks5: &listener.Socks5{
				Host: host,
			},
		},
		Handler: c,
	}, false)
}

func (c *ServerM) Stream(ctx context.Context, t *netapi.StreamMeta) {
	defer t.Src.Close()

	err := c.ConnectT(t.Src, &Request{
		Type: Type_Connection,
		Payload: &Request_Connect{
			Connect: &Connect{
				Target:  t.Address.Hostname(),
				Address: "127.0.0.1",
				Port:    uint32(t.Address.Port().Port()),
			},
		},
	})
	if err != nil {
		log.Println(err)
	}
}

func (c *ServerM) Packet(ctx context.Context, pack *netapi.Packet) {}

func getRequestReader(c io.Reader) (*Request, error) {
	var length uint64
	if err := binary.Read(c, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(c, data); err != nil {
		return nil, err
	}

	return getRequest(data)
}

func getRequest(data []byte) (*Request, error) {
	var req Request

	if err := proto.Unmarshal(data, &req); err != nil {
		return nil, err
	}

	return &req, nil
}

type Chan struct {
	IDChan syncmap.SyncMap[uint64, chan net.Conn]
	ID     atomic.Uint64
}

func (c *Chan) NewChan() (uint64, chan net.Conn) {
	id := c.ID.Add(1)
	ch := make(chan net.Conn)
	c.IDChan.Store(id, ch)

	return id, ch
}

func (c *Chan) RemoveChan(id uint64) { c.IDChan.Delete(id) }

func (c *Chan) SendChan(id uint64, conn net.Conn) {
	ch, ok := c.IDChan.Load(id)
	if !ok {
		conn.Close()
		return
	}
	ch <- conn
}

type Devices struct {
	mu      sync.Mutex
	devices syncmap.SyncMap[string, *DeviceT]
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

	if err := SendOk(conn); err != nil {
		conn.Close()
		return err
	}

	go func() {
		defer func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			dv, ok := d.devices.LoadAndDelete(uuid)
			if ok {
				dv.conn.Close()
			}
		}()

		for {
			req, err := getRequestReader(conn)
			if err != nil {
				log.Println("get req failed", err, uuid)
				return
			}

			switch req.GetType() {
			case Type_Ping:
				if err := device.Keepalive(); err != nil {
					log.Println("send keepalive failed:", err)
					return
				}
			}
		}
	}()

	return nil
}

func (s *ServerM) ConnectT(c net.Conn, req *Request) error {
	defer c.Close()

	device, ok := s.devices.devices.Load(req.GetConnect().Target)
	if !ok {
		return fmt.Errorf("device %s is not exist", req.GetConnect().Target)
	}

	log.Println("new request target:", req.GetConnect())
	defer log.Println("close", req.GetConnect())

	id, ch := s.chanm.NewChan()
	defer s.chanm.RemoveChan(id)

	log.Println("get new chan", id)

	req.GetConnect().Id = id

	err := device.Connect(req)
	if err != nil {
		return err
	}

	log.Println("wait to get resp conn by", req.GetConnect().Id, id)

	select {
	case conn := <-ch:
		log.Println("get resp conn by", req.GetConnect().Id, id)
		relay.Relay(conn, c)
	case <-time.After(time.Second * 10):
		return fmt.Errorf("timeout")
	}

	return nil
}

// type DeviceQuicT struct {
// 	connection quic.Connection
// }

// func (d *DeviceQuicT) Keepalive() error {
// 	return d.connection.SendMessage([]byte{0x01})
// }

// func (d *DeviceQuicT) Connect(req *Request) (io.ReadWriteCloser, error) {
// 	conn, err := d.connection.OpenStream()
// 	if err != nil {
// 		return nil, err
// 	}

// 	if err := SendRequest(conn, req); err != nil {
// 		return nil, err
// 	}

// 	return conn, nil
// }

type DeviceT struct{ conn net.Conn }

func NewDevice(conn net.Conn) *DeviceT        { return &DeviceT{conn} }
func (d *DeviceT) Keepalive() error           { return SendPing(d.conn) }
func (d *DeviceT) Connect(req *Request) error { return SendRequest(d.conn, req) }
