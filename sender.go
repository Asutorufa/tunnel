package tunnel

import (
	"encoding/binary"
	"net"

	"google.golang.org/protobuf/proto"
)

func SendOk(c net.Conn) error {
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

func SendConnect(c net.Conn, r *Connect) error {
	return SendRequest(c, &Request{
		Type: Type_Connection,
		Payload: &Request_Connect{
			Connect: r,
		},
	})
}

func SendRequest(c net.Conn, req *Request) error {
	data, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	err = binary.Write(c, binary.BigEndian, uint64(len(data)))
	if err != nil {
		return err
	}

	_, err = c.Write(data)
	if err != nil {
		return err
	}

	return nil
}

func SendPing(c net.Conn) error {
	return SendRequest(c, &Request{
		Type:    Type_Ping,
		Payload: &Request_Ping{Ping: &PingMsg{}},
	})
}
