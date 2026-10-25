package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const head = "services:\n  svc:\n    image: x\n"

func TestPortBind(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		rc   int
		want []string // substrings that must appear in stdout
	}{
		{"loopback short", head + "    ports:\n      - \"127.0.0.1:3902:3902\"\n", 0, nil},
		{"loopback tcp suffix", head + "    ports:\n      - 127.0.0.1:3902:3902/tcp\n", 0, nil},
		{"no ports", head, 0, nil},
		{"expose only", head + "    expose:\n      - \"3902\"\n", 0, nil},
		{"null service", "services:\n  svc:\n", 0, nil},
		{"all interfaces", head + "    ports:\n      - \"3902:3902\"\n", 1, []string{`service svc publishes "3902:3902" on all interfaces`}},
		{"any ip", head + "    ports:\n      - \"0.0.0.0:3902:3902\"\n", 1, []string{`publishes "0.0.0.0:3902:3902" on 0.0.0.0`}},
		{"container only string", head + "    ports:\n      - \"3902\"\n", 1, []string{`publishes "3902" on all interfaces`}},
		{"container only int", head + "    ports:\n      - 3902\n", 1, []string{`publishes "3902" on all interfaces`}},
		{"udp suffix", head + "    ports:\n      - \"53:53/udp\"\n", 1, []string{"on all interfaces"}},
		{"env in port only", head + "    ports:\n      - \"${NSELF_HTTP_PORT:-80}:80\"\n", 1, []string{"on all interfaces"}},
		{"env host ip", head + "    ports:\n      - \"${PLUGIN_BIND:-127.0.0.1}:3902:3902\"\n", 1, []string{"not a literal 127.0.0.1"}},
		{"env host ip with loopback ports", head + "    ports:\n      - \"127.0.0.1:${P:-3902}:3902\"\n", 0, nil},
		{"ipv6 any", head + "    ports:\n      - \"[::]:3902:3902\"\n", 1, []string{"on ::"}},
		{"ipv6 loopback is not 127.0.0.1", head + "    ports:\n      - \"[::1]:3902:3902\"\n", 1, []string{"on ::1"}},
		{"unparseable mapping", head + "    ports:\n      - \"::1:3902:3902\"\n", 1, []string{"cannot be verified"}},
		{"long no host_ip", head + "    ports:\n      - target: 3902\n        published: 3902\n", 1, []string{"target 3902 published 3902 on all interfaces"}},
		{"long no published no host_ip", head + "    ports:\n      - target: 3902\n", 1, []string{"on all interfaces"}},
		{"long any host_ip", head + "    ports:\n      - target: 80\n        host_ip: 0.0.0.0\n", 1, []string{"on 0.0.0.0"}},
		{"long env host_ip", head + "    ports:\n      - target: 80\n        host_ip: ${B:-127.0.0.1}\n", 1, []string{"not a literal 127.0.0.1"}},
		{"long loopback", head + "    ports:\n      - target: 80\n        published: 8080\n        host_ip: 127.0.0.1\n", 0, nil},
		{"host network", head + "    network_mode: host\n", 1, []string{"network_mode host"}},
		{"bridge network_mode", head + "    network_mode: bridge\n", 0, nil},
		{"ports not a list", head + "    ports: \"3902:3902\"\n", 1, []string{"not a list"}},
		{"ports mapping not a list", head + "    ports:\n      a: b\n", 1, []string{"not a list"}},
		{"ports entry of odd type", head + "    ports:\n      - [1, 2]\n", 1, []string{"cannot be verified"}},
		{"flow sequence", head + "    ports: [\"3902:3902\"]\n", 1, []string{"on all interfaces"}},
		{"service not a mapping", "services:\n  svc: nope\n", 1, []string{"not a mapping"}},
		{"second violation reported with first ok", "services:\n  a:\n    ports: [\"127.0.0.1:1:1\"]\n  b:\n    ports: [\"2:2\"]\n", 1, []string{"service b "}},
		{"multi doc, violation in second", "x: 1\n---\n" + head + "    ports: [\"127.0.0.1:1:1\"]\n---\nservices:\n  late:\n    ports: [\"9:9\"]\n", 1, []string{"service late "}},
		{"anchor and merge key", "x-base: &base\n  ports:\n    - \"3902:3902\"\nservices:\n  svc:\n    <<: *base\n    image: x\n", 1, []string{"service svc publishes \"3902:3902\""}},
		{"anchor loopback", "x-base: &base\n  ports:\n    - \"127.0.0.1:3902:3902\"\nservices:\n  svc:\n    <<: *base\n", 0, nil},
		{"alias of a ports list", "x-p: &p\n  - \"3902:3902\"\nservices:\n  svc:\n    ports: *p\n", 1, []string{"on all interfaces"}},
		{"helm values skipped", "services:\n  auth:\n    kind: deployment\n    ports:\n      - port: 4000\n        protocol: TCP\n", 0, nil},
		{"k8s style skipped", "kind: Deployment\nspec:\n  template:\n    spec:\n      containers:\n        - ports:\n            - containerPort: 80\n", 0, nil},
		{"top-level ports without services skipped", "ports:\n  - containerPort: 80\n", 0, nil},
		{"unparseable skipped", "services: [\n  : bad\n", 0, nil},
		{"helm template skipped", "services:\n  - {{ .Values.x }}\n", 0, nil},
		{"services not a mapping skipped", "services:\n  - a\n", 0, nil},
		{"scalar document skipped", "just text\n", 0, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := filepath.Join(t.TempDir(), "c.yml")
			if err := os.WriteFile(f, []byte(c.yaml), 0o644); err != nil {
				t.Fatal(err)
			}
			var out, errb bytes.Buffer
			rc := run([]string{f}, &out, &errb)
			if rc != c.rc {
				t.Fatalf("rc = %d, want %d\nstdout: %s\nstderr: %s", rc, c.rc, out.String(), errb.String())
			}
			for _, w := range c.want {
				if !strings.Contains(out.String(), w) {
					t.Errorf("stdout lacks %q:\n%s", w, out.String())
				}
			}
			if c.rc == 0 && out.Len() != 0 {
				t.Errorf("unexpected output: %s", out.String())
			}
			if c.rc == 1 && !strings.HasPrefix(out.String(), f+": service ") {
				t.Errorf("line not prefixed with the file: %s", out.String())
			}
		})
	}
}

func TestRequire(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("good.yml", head+"    ports: [\"127.0.0.1:1:1\"]\n")
	bad := write("bad.yml", head+"    ports: [\"1:1\"]\n")
	helm := write("values.yaml", "services:\n  auth:\n    ports:\n      - port: 4000\n")
	k8s := write("k8s.yaml", "ports:\n  - containerPort: 80\n")
	broken := write("broken.yml", "services: [\n  : bad\n")
	noSvc := write("nosvc.yml", "version: '3'\n")
	listSvc := write("listsvc.yml", "services:\n  - a\n")
	cases := []struct {
		name string
		args []string
		rc   int
	}{
		{"required good", []string{"-require", good}, 0},
		{"required bad", []string{"-require", bad}, 1},
		{"required plus others", []string{"-require", good, k8s, broken}, 0},
		{"required long entry without target is checked", []string{"-require", helm}, 1},
		{"required non-compose", []string{"-require", k8s}, 2},
		{"required unparseable", []string{"-require", broken}, 2},
		{"required without services", []string{"-require", noSvc}, 2},
		{"required services not a mapping", []string{"-require", listSvc}, 2},
		{"required missing file", []string{"-require", filepath.Join(dir, "nope.yml")}, 2},
		{"required wins over plain, deduplicated", []string{"-require", bad, bad}, 1},
		{"require without value", []string{"-require"}, 2},
		{"no files", nil, 2},
		{"unknown flag", []string{"-x", good}, 2},
		{"plain missing file", []string{filepath.Join(dir, "nope.yml")}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var out, errb bytes.Buffer
			if rc := run(c.args, &out, &errb); rc != c.rc {
				t.Fatalf("rc = %d, want %d\nstdout: %s\nstderr: %s", rc, c.rc, out.String(), errb.String())
			}
		})
	}
	// A file given both ways reports its violation once.
	var out, errb bytes.Buffer
	run([]string{"-require", bad, bad}, &out, &errb)
	if n := strings.Count(out.String(), "\n"); n != 1 {
		t.Errorf("got %d lines, want 1:\n%s", n, out.String())
	}
}

func TestSplitPorts(t *testing.T) {
	cases := map[string]int{
		"80": 1, "80:80": 2, "1.2.3.4:80:80": 3, "${A:-80}:80": 2, "${A:-${B:-1}}:80": 2,
		"[::1]:80:80": 3, "${A:-1.2.3.4}:80:80": 3,
	}
	for in, n := range cases {
		if got := splitPorts(in); len(got) != n {
			t.Errorf("splitPorts(%q) = %q, want %d parts", in, got, n)
		}
	}
}

// -allow (D-0626): a listed container port passes on any host IP; every other port of the same service still fails.
func TestAllow(t *testing.T) {
	cases := []struct {
		name  string
		ports string
		args  []string
		rc    int
		want  string // substring that must appear in stdout
	}{
		{"listed ports pass", `["${HTTP:-80}:80", "${HTTPS:-443}:443"]`, []string{"-allow", "80", "-allow", "443"}, 0, ""},
		{"unlisted port beside a listed one fails", `["80:80", "8444:8444"]`, []string{"-allow", "80"}, 1, `"8444:8444"`},
		{"without -allow the same entry fails", `["80:80"]`, nil, 1, "on all interfaces"},
		{"match is on the container port, not the host port", `["80:8080"]`, []string{"-allow", "80"}, 1, `"80:8080"`},
		{"container-only form", `["443"]`, []string{"-allow", "443"}, 0, ""},
		{"tcp suffix", `["80:80/tcp"]`, []string{"-allow", "80"}, 0, ""},
		{"explicit host ip", `["0.0.0.0:80:80"]`, []string{"-allow", "80"}, 0, ""},
		{"range is not a listed port", `["80-90:80-90"]`, []string{"-allow", "80"}, 1, "80-90"},
		{"variable container port is not a listed port", `["80:${P:-80}"]`, []string{"-allow", "80"}, 1, "80:${P:-80}"},
		{"long syntax", "\n      - target: 80\n        published: 80\n", []string{"-allow", "80"}, 0, ""},
		{"long syntax unlisted target", "\n      - target: 81\n        published: 80\n", []string{"-allow", "80"}, 1, "target 81"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := filepath.Join(t.TempDir(), "c.yml")
			if err := os.WriteFile(f, []byte(head+"    ports: "+c.ports+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var out, errb bytes.Buffer
			if rc := run(append(append([]string{}, c.args...), f), &out, &errb); rc != c.rc {
				t.Fatalf("rc = %d, want %d\nstdout: %s\nstderr: %s", rc, c.rc, out.String(), errb.String())
			}
			if c.want != "" && !strings.Contains(out.String(), c.want) {
				t.Errorf("stdout lacks %q:\n%s", c.want, out.String())
			}
		})
	}
	t.Run("allow never excuses network_mode host", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "c.yml")
		if err := os.WriteFile(f, []byte(head+"    network_mode: host\n    ports: [\"80:80\"]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		var out, errb bytes.Buffer
		if rc := run([]string{"-allow", "80", f}, &out, &errb); rc != 1 || !strings.Contains(out.String(), "network_mode host") {
			t.Fatalf("rc = %d\n%s", rc, out.String())
		}
	})
	for _, bad := range [][]string{{"-allow"}, {"-allow", "x"}, {"-allow", "0"}, {"-allow", "70000"}, {"-allow", "80-90"}} {
		var out, errb bytes.Buffer
		if rc := run(append(bad, "f.yml"), &out, &errb); rc != 2 {
			t.Errorf("run(%v) = %d, want 2", bad, rc)
		}
	}
}
