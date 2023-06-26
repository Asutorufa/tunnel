package tunnel

import (
	"encoding/json"
	"testing"
)

func TestRule(t *testing.T) {
	r := map[string]Target{
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
