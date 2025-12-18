package utils

import (
	"errors"
	"testing"
)

func TestStripPortFromAddr(t *testing.T) {
	addr := StripPortFromAddr("127.0.0.1:2323")
	if addr != "127.0.0.1" {
		t.Error(errors.New("Address is wrong"))
	}
}
