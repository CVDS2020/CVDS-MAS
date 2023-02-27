package sip

import (
	"encoding/json"
	"gitee.com/sy_183/common/assert"
	"testing"
)

func TestMessage(t *testing.T) {
	t.Log(string(assert.Must(json.Marshal(new(Message)))))
}
