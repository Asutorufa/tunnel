package tunnelclient

import (
	"encoding/json"
	"testing"

	"github.com/Asutorufa/tunnel/pkg/protomsg"
)

func TestRule(t *testing.T) {
	r := map[string]protomsg.Target{
		"127.0.0.1:56022": {
			UUID:    "5600g",
			Address: "127.0.0.1",
			Port:    22,
		},
	}

	data, err := json.Marshal(r)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	t.Log(string(data))
}
