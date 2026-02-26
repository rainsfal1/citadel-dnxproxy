package discovery

import "testing"

func TestParseDarwinARPLine(t *testing.T) {
	line := "? (192.168.1.5) at aa:bb:cc:dd:ee:ff on en0 ifscope [ethernet]"
	ip, mac, ok := parseDarwinARPLine(line)
	if !ok {
		t.Fatalf("expected parse ok")
	}
	if ip.String() != "192.168.1.5" {
		t.Fatalf("unexpected ip %s", ip.String())
	}
	if mac.String() != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("unexpected mac %s", mac.String())
	}
}

func TestParseDarwinARPLineIncomplete(t *testing.T) {
	line := "? (192.168.1.9) at <incomplete> on en0 ifscope [ethernet]"
	if _, _, ok := parseDarwinARPLine(line); ok {
		t.Fatalf("expected incomplete line to be ignored")
	}
}
