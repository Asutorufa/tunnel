package tunnel

import (
	"encoding/binary"
	"log"
	"net"

	"google.golang.org/protobuf/proto"
)

func SendOk(c net.Conn) error {
	req := &Request{
		Type:    Type_Ok,
		Payload: &Request_Ok{Ok: &OkMsg{}},
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	err = binary.Write(c, binary.BigEndian, uint64(len(data)))
	if err != nil {
		return err
	}
	_, err = c.Write(data)
	return err
}

func SendError(c net.Conn, err error) error {
	if err == nil {
		return SendOk(c)
	}
	req := &Request{
		Type: Type_Error,
		Payload: &Request_Error{
			Error: &ErrorMsg{
				Msg: err.Error(),
			},
		},
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	err = binary.Write(c, binary.BigEndian, uint64(len(data)))
	if err != nil {
		return err
	}
	_, err = c.Write(data)
	return err

}

func SendConnect(c net.Conn, r *Connect) error {
	req := &Request{
		Type: Type_Connection,
		Payload: &Request_Connect{
			Connect: r,
		},
	}

	log.Println(r)

	data, err := proto.Marshal(req)
	if err != nil {
		return err
	}

	err = binary.Write(c, binary.BigEndian, uint64(len(data)))
	if err != nil {
		return err
	}
	_, err = c.Write(data)
	return err
}
