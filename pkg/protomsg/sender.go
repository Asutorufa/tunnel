package protomsg

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"

	"github.com/Asutorufa/yuhaiin/pkg/utils/pool"
	"google.golang.org/protobuf/proto"
)

func SendOk(c io.Writer) error {
	return SendRequest(c, &Request{
		Type:    Type_Ok,
		Payload: &Request_Ok{Ok: &OkMsg{}},
	})
}

func SendRequest(c io.Writer, req *Request) error {
	buf := pool.GetBuffer()
	defer pool.PutBuffer(buf)

	data, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	_ = binary.Write(buf, binary.BigEndian, uint64(len(data)))
	_, _ = buf.Write(data)

	_, err = c.Write(buf.Bytes())
	if err != nil {
		return err
	}

	return nil
}

func SendPing(c io.Writer) error {
	return SendRequest(c, &Request{
		Type:    Type_Ping,
		Payload: &Request_Ping{Ping: &PingMsg{}},
	})
}

func SendRegister(conn net.Conn, uuid string) error {
	err := SendRequest(conn, &Request{
		Type: Type_Register,
		Payload: &Request_Device{
			Device: &Device{
				Uuid: uuid,
			},
		},
	})
	if err != nil {
		return err
	}

	resp, err := GetRequestReader(conn)
	if err != nil {
		return err
	}

	if resp.Type == Type_Error {
		return errors.New(resp.GetError().GetMsg())
	}

	if resp.Type != Type_Ok {
		return fmt.Errorf("unknown type: %d", resp.Type)
	}

	return nil
}

func GetRequest(data []byte) (*Request, error) {
	var req Request

	if err := proto.Unmarshal(data, &req); err != nil {
		return nil, err
	}

	return &req, nil
}

func GetRequestReader(c io.Reader) (*Request, error) {
	var length uint64
	if err := binary.Read(c, binary.BigEndian, &length); err != nil {
		return nil, err
	}

	if length <= 0 || length > 0xffff {
		return nil, fmt.Errorf("invalid length: %d", length)
	}

	data := make([]byte, length)
	if _, err := io.ReadFull(c, data); err != nil {
		return nil, err
	}

	return GetRequest(data)
}

type Target struct {
	UUID    string `json:"uuid"`
	Address string `json:"address"`
	Port    uint16 `json:"port"`
}
