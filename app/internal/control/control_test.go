package control

import (
	"path/filepath"
	"testing"
)

func TestListenAfterFailedDial(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "control.sock")
	if c, err := Dial(sock); err == nil {
		c.Close()
		t.Fatal("dial with no server succeeded")
	}
	ln, err := Listen(sock)
	if err != nil {
		t.Fatalf("listen after failed dial: %v", err)
	}
	defer ln.Close()
	c, err := Dial(sock)
	if err != nil {
		t.Fatalf("dial after listen: %v", err)
	}
	c.Close()
}
