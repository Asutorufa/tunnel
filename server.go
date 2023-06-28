package tunnel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/proto"
)

//go:generate protoc --go_out=. --go_opt=paths=source_relative message.proto

type ServerM struct {
	devices *Devices
	chanm   *Chan
}

func NewServerM() *ServerM {
	return &ServerM{
		devices: &Devices{
			devices: make(map[string]*DeviceT),
		},
		chanm: &Chan{
			IDChan: make(map[uint64]chan net.Conn),
		},
	}
}
func (s *ServerM) Handle(c net.Conn) error {
	log.Println("new request from: ", c.RemoteAddr())

	req, err := getRequest(c)
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
		go s.chanm.SendChan(req.GetConnectResponse().Connid, c)
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

func getRequest(c net.Conn) (*Request, error) {
	var length uint64
	if err := binary.Read(c, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(c, data); err != nil {
		return nil, err
	}

	var req Request

	if err := proto.Unmarshal(data, &req); err != nil {
		return nil, err
	}

	return &req, nil
}

type Chan struct {
	mu     sync.RWMutex
	IDChan map[uint64]chan net.Conn
	ID     atomic.Uint64
}

func (c *Chan) NewChan() (uint64, chan net.Conn) {
	id := c.ID.Add(1)

	ch := make(chan net.Conn)

	c.mu.Lock()
	c.IDChan[id] = ch
	c.mu.Unlock()

	return id, ch
}

func (c *Chan) RemoveChan(id uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.IDChan, id)
}

func (c *Chan) SendChan(id uint64, conn net.Conn) {
	c.mu.RLock()
	ch, ok := c.IDChan[id]
	if !ok {
		conn.Close()
		return
	}
	c.mu.RUnlock()

	ch <- conn
}

type Devices struct {
	mu      sync.RWMutex
	devices map[string]*DeviceT
}

func (d *Devices) RegisterDevice(uuid string, conn net.Conn) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	dd, ok := d.devices[uuid]
	if ok {
		dd.conn.Close()
		delete(d.devices, uuid)
	}

	device := NewDevice(conn)

	d.devices[uuid] = device

	if err := SendOk(conn); err != nil {
		conn.Close()
		return err
	}

	go func() {
		ticker := time.NewTicker(time.Second * 15)
		defer ticker.Stop()

		defer func() {
			d.mu.Lock()
			defer d.mu.Unlock()
			conn.Close()

			if device == d.devices[uuid] {
				delete(d.devices, uuid)
			}
		}()

		for range ticker.C {
			if err := device.Keepalive(); err != nil {
				log.Println("send keepalive failed:", err)
				return
			}
		}
	}()

	return nil
}

func (d *Devices) Get(uuid string) (*DeviceT, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	dd, ok := d.devices[uuid]
	return dd, ok
}

func (s *ServerM) ConnectT(c net.Conn, req *Request) error {
	defer c.Close()

	device, ok := s.devices.Get(req.GetConnect().Target)
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
		Relay(conn, c)
	case <-time.After(time.Second * 10):
		return fmt.Errorf("timeout")
	}

	return nil
}

type DeviceT struct {
	mu   sync.Mutex
	conn net.Conn
}

func NewDevice(conn net.Conn) *DeviceT {
	return &DeviceT{
		conn: conn,
	}
}

func (d *DeviceT) Keepalive() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if err := SendPing(d.conn); err != nil {
		return err
	}

	resp, err := getRequest(d.conn)
	if err != nil {
		return err
	}

	if resp.GetType() == Type_Ok {
		return nil
	}

	if resp.GetType() == Type_Error {
		return errors.New(resp.GetError().GetMsg())
	}

	return fmt.Errorf("unknown type: %d", resp.GetType())
}

func (d *DeviceT) Connect(req *Request) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	err := SendRequest(d.conn, req)
	if err != nil {
		return err
	}

	log.Println("wait", req.GetConnect().Id, req.GetConnect().Target, "response")

	resp, err := getRequest(d.conn)
	if err != nil {
		return err
	}

	if resp.GetType() == Type_Ok {
		return nil
	}

	if resp.GetType() == Type_Error {
		return errors.New(resp.GetError().GetMsg())
	}

	return fmt.Errorf("unknown type: %d", resp.GetType())
}

func Relay(s, c net.Conn) {
	go func() {
		io.Copy(s, c)
		s.Close()
	}()

	io.Copy(c, s)
	c.Close()
}
