//go:build linux

package amneziawg

import (
	"context"
	"go.uber.org/zap"
	"os"
	"syscall"
	"testing"
	"time"
)

type userspaceOnlyLinks struct{ linkManager }

func (userspaceOnlyLinks) Create(string) error { return syscall.EOPNOTSUPP }

// Run only in an isolated Linux network namespace/container with NET_ADMIN.
func TestLinuxUserspaceLifecycle(t *testing.T) {
	if os.Getenv("VPN_MANAGER_LINUX_TEST") != "1" {
		t.Skip("requires isolated Linux namespace")
	}
	cfg := validTestConfig()
	cfg.H1 = "100-200"
	cfg.H2 = "300-400"
	cfg.H3 = "500-600"
	cfg.H4 = "700-800"
	cfg.JunkPacketCount = 3
	cfg.JunkPacketMinSize = 50
	cfg.JunkPacketMaxSize = 100
	cfg.InitJunkPackets = 105
	cfg.ResponseJunkPackets = 97
	c, err := NewController("isolated integration", cfg, zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	c.links = userspaceOnlyLinks{c.links}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for attempt := 0; attempt < 2; attempt++ {
		if err := c.Connect(ctx); err != nil {
			t.Fatal(err)
		}
		dev, err := c.client.Device(ctx, c.ifaceName)
		if err != nil {
			t.Fatal(err)
		}
		if dev.H1 != "100-200" || dev.Jc != 3 || dev.S1 != 105 || dev.S2 != 97 {
			t.Fatalf("obfuscation not applied: H1=%q Jc=%d S1=%d S2=%d", dev.H1, dev.Jc, dev.S1, dev.S2)
		}
		if err := c.Disconnect(ctx); err != nil {
			t.Fatal(err)
		}
		exists, err := c.links.Exists(c.ifaceName)
		if err != nil || exists {
			t.Fatalf("interface leaked: exists=%v err=%v", exists, err)
		}
	}
}
