package hygiene

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"
)

// TestMain doubles as the helper process of the leak tests: HYGIENE_HELPER=listen opens a TCP
// listener, prints its port and sleeps, standing in for a daemonised job child.
func TestMain(m *testing.M) {
	if os.Getenv("HYGIENE_HELPER") == "listen" {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			os.Exit(2)
		}
		fmt.Println(l.Addr().(*net.TCPAddr).Port)
		time.Sleep(time.Hour)
		os.Exit(0)
	}
	os.Exit(m.Run())
}
