package tokens

import (
	"bytes"
	"strings"
	"testing"
)

func TestHashRandomAndDevice(t *testing.T) {
	if !bytes.Equal(Hash("x"), Hash("x")) || bytes.Equal(Hash("x"), Hash("y")) {
		t.Fatal("Hash is not a stable digest")
	}
	a, _ := Random(24)
	b, _ := Random(24)
	if a == b || len(a) != 32 {
		t.Fatalf("Random: %q %q", a, b)
	}
	id, cred, err := NewDevice()
	if err != nil || !strings.HasPrefix(id, "dev_") || !strings.HasPrefix(cred, "avdc_") {
		t.Fatalf("NewDevice: %q %q %v", id, cred, err)
	}
}
