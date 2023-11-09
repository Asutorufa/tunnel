package tunnel

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

func SendError(c net.Conn, err error) error {
	if err == nil {
		return SendOk(c)
	}

	return SendRequest(c, &Request{
		Type: Type_Error,
		Payload: &Request_Error{
			Error: &ErrorMsg{
				Msg: err.Error(),
			},
		},
	})
}

func SendConnect(c io.Writer, r *Connect) error {
	return SendRequest(c, &Request{
		Type: Type_Connection,
		Payload: &Request_Connect{
			Connect: r,
		},
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

func SendRegister(f func(req []byte) ([]byte, error), uuid string) error {
	req := &Request{
		Type: Type_Register,
		Payload: &Request_Device{
			Device: &Device{
				Uuid: uuid,
			},
		},
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	respdata, err := f(data)
	if err != nil {
		return err
	}

	resp, err := getRequest(respdata)
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
