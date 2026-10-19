package slots

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"testing"
)

const helperEnv = "NSELF_SLOTS_HELPER_DIR"

// TestMain lets the test binary act as a slot-holding peer process.
func TestMain(m *testing.M) {
	if dir := os.Getenv(helperEnv); dir != "" {
		total, _ := strconv.Atoi(os.Getenv("NSELF_SLOTS_HELPER_TOTAL"))
		tb, err := Open(dir, total)
		if err != nil {
			os.Stdout.WriteString("error\n")
			os.Exit(3)
		}
		if _, err := tb.Acquire(Self("peer", "l")); err != nil {
			os.Stdout.WriteString("full\n")
			os.Exit(0)
		}
		os.Stdout.WriteString("held\n")
		_, _ = bufio.NewReader(os.Stdin).ReadByte() // hold the slot until the parent closes stdin
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestSlotsAcrossProcesses proves the cap holds between separate processes
// (two coordinators' agents on one host), not only between goroutines.
func TestSlotsAcrossProcesses(t *testing.T) {
	skipWindows(t)
	dir := t.TempDir()
	const total, peers = 2, 5
	type peer struct {
		cmd  *exec.Cmd
		out  *bufio.Reader
		stop func()
	}
	var ps []peer
	for range peers {
		cmd := exec.Command(os.Args[0], "-test.run=NONE")
		cmd.Env = append(os.Environ(), helperEnv+"="+dir, "NSELF_SLOTS_HELPER_TOTAL="+strconv.Itoa(total))
		in, _ := cmd.StdinPipe()
		out, _ := cmd.StdoutPipe()
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		ps = append(ps, peer{cmd, bufio.NewReader(out), func() { _ = in.Close() }})
	}
	held := 0
	for _, p := range ps {
		line, _ := p.out.ReadString('\n')
		switch line {
		case "held\n":
			held++
		case "full\n":
		default:
			t.Errorf("peer said %q", line)
		}
	}
	for _, p := range ps {
		p.stop()
		_ = p.cmd.Wait()
	}
	if held != total {
		t.Fatalf("%d peers held a slot, want %d", held, total)
	}
}
