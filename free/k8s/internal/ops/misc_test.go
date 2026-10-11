// Purpose: unit tests of the pure parts: Compare, the fingerprint SQL, the target
// name rules, the pod picker and the kubectl argv.
//
// Inputs: none (no process, no docker).
//
// Outputs: test results.
//
// Constraints: table-driven.
package ops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompareIsDeterministic(t *testing.T) {
	want := []Fingerprint{{"a", "gone", 1, "1"}, {"a", "rows", 5, "1"}, {"b", "sum", 2, "9"}, {"b", "same", 1, "1"}}
	got := []Fingerprint{{"z", "extra", 1, "1"}, {"b", "same", 1, "1"}, {"b", "sum", 2, "8"}, {"a", "rows", 4, "1"}}
	d := Compare(want, got)
	var lines []string
	for _, x := range d {
		lines = append(lines, x.String())
	}
	if g := strings.Join(lines, "|"); g != "a.gone missing|a.rows rows 5 != 4|b.sum content differs|z.extra extra" {
		t.Errorf("diffs = %s", g)
	}
	if len(Compare(want, want)) != 0 {
		t.Error("equal lists differ")
	}
}

func TestFingerprintSQLQuotesEveryIdentifierInSQL(t *testing.T) {
	for _, must := range []string{"%I.%I", "%L::text", "relkind = 'r'", "'pg_catalog', 'information_schema'", `pg\_toast%`, `pg\_temp\_%`, "ORDER BY n.nspname, c.relname", `\gexec`,
		"count(*)", "md5(nself_fp_row::text)", "::bit(64)::bigint"} {
		if !strings.Contains(fingerprintSQL, must) {
			t.Errorf("fingerprint SQL lacks %q", must)
		}
	}
	// Names come from the catalog, never from Go: the SQL has no placeholder for a Go string.
	if strings.Contains(fingerprintSQL, "%s") {
		t.Error("fingerprint SQL splices a Go string verb")
	}
	for _, pin := range []string{"TimeZone = 'UTC'", "DateStyle = 'ISO, MDY'", "IntervalStyle = 'postgres'", "extra_float_digits = 3", "bytea_output = 'hex'"} {
		if !strings.Contains(sessionPrelude, pin) {
			t.Errorf("session prelude lacks %q", pin)
		}
	}
}

func TestParseFingerprintsRejectsGarbage(t *testing.T) {
	for _, l := range []string{"nope", `{"schema":"s"}`, `{"schema":"s","table":"t","rows":1}`} {
		if _, err := parseFingerprints([]string{l}); err == nil {
			t.Errorf("accepted %q", l)
		}
	}
	fps, err := parseFingerprints([]string{"", `{"schema":"b","table":"x","rows":1,"sum":"1"}`, `{"schema":"a","table":"y","rows":1,"sum":"1"}`})
	if err != nil || len(fps) != 2 || fps[0].Schema != "a" {
		t.Errorf("fps = %+v, err = %v", fps, err)
	}
}

func TestPgDumpArgvValidatesSnapshot(t *testing.T) {
	for _, ok := range []string{"00000003-0000001B-1", "00000003-0000001B", "0A-1F-12"} {
		if _, valid := pgDumpArgv(ok); !valid {
			t.Errorf("rejected %q", ok)
		}
	}
	for _, bad := range []string{"", "x", "00000003-0000001b-1", "1-1-1-1", "1-1; id", "1-1\n", "$(id)-1"} {
		if _, valid := pgDumpArgv(bad); valid {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestResolveTarget(t *testing.T) {
	now := time.Date(2026, 10, 11, 4, 5, 6, 0, time.UTC)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("# c\nexport PROJECT_NAME=\"from-dotenv\"\nBACKUP_DIR='bk'\nOTHER=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	tests := []struct {
		name string
		o    TargetOptions
		path string
		proj string
		code string
	}{
		{"dotenv", TargetOptions{ProjectDir: dir, Getenv: env(nil), Now: now}, filepath.Join(dir, "bk", "from-dotenv_full_20261011_040506.dump"), "from-dotenv", ""},
		{"process env wins", TargetOptions{ProjectDir: dir, Getenv: env(map[string]string{"PROJECT_NAME": "p1", "BACKUP_DIR": "/abs"}), Now: now}, "/abs/p1_full_20261011_040506.dump", "p1", ""},
		{"tag", TargetOptions{ProjectDir: dir, Tag: "pre-upgrade_1", Getenv: env(map[string]string{"PROJECT_NAME": "p"}), Now: now}, filepath.Join(dir, "bk", "p_full_20261011_040506_pre-upgrade_1.dump"), "p", ""},
		{"output exact", TargetOptions{ProjectDir: dir, Output: "/o/x.dump", Getenv: env(nil), Now: now}, "/o/x.dump", "from-dotenv", ""},
		{"bad tag", TargetOptions{ProjectDir: dir, Tag: "a/b", Getenv: env(nil), Now: now}, "", "", CodeTarget},
		{"bad tag with output", TargetOptions{ProjectDir: dir, Output: "/o/x", Tag: "..;x", Getenv: env(nil), Now: now}, "", "", CodeTarget},
		{"bad name", TargetOptions{ProjectDir: dir, Getenv: env(map[string]string{"PROJECT_NAME": "../x"}), Now: now}, "", "", CodeTarget},
		{"no name", TargetOptions{ProjectDir: t.TempDir(), Getenv: env(nil), Now: now}, "", "", CodeTarget},
		{"output dir", TargetOptions{ProjectDir: dir, Output: "/o/", Getenv: env(nil), Now: now}, "", "", CodeTarget},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, proj, err := ResolveTarget(tc.o)
			if tc.code != "" {
				wantCode(t, err, tc.code)
				return
			}
			if err != nil || p != tc.path || proj != tc.proj {
				t.Errorf("got %q %q %v, want %q %q", p, proj, err, tc.path, tc.proj)
			}
		})
	}
}

const podDoc = `{"items":[
 {"metadata":{"name":"postgres-0"},"status":{"phase":"Running","containerStatuses":[{"name":"postgres","ready":true}]}},
 {"metadata":{"name":"old"},"status":{"phase":"Succeeded","containerStatuses":[{"name":"postgres","ready":false}]}},
 {"metadata":{"name":"unready"},"status":{"phase":"Running","containerStatuses":[{"name":"postgres","ready":false}]}}]}`

func TestPickPod(t *testing.T) {
	if p, err := pickPod([]byte(podDoc)); err != nil || p != "postgres-0" {
		t.Errorf("pod = %q, %v", p, err)
	}
	for name, doc := range map[string]string{
		"none":      `{"items":[]}`,
		"unready":   `{"items":[{"metadata":{"name":"a"},"status":{"phase":"Running","containerStatuses":[{"name":"postgres","ready":false}]}}]}`,
		"two":       `{"items":[` + strings.Repeat(`{"metadata":{"name":"a"},"status":{"phase":"Running","containerStatuses":[{"name":"postgres","ready":true}]}},`, 1) + `{"metadata":{"name":"b"},"status":{"phase":"Running","containerStatuses":[{"name":"postgres","ready":true}]}}]}`,
		"not json":  `<html>`,
		"wrong ctr": `{"items":[{"metadata":{"name":"a"},"status":{"phase":"Running","containerStatuses":[{"name":"sidecar","ready":true}]}}]}`,
	} {
		_, err := pickPod([]byte(doc))
		oe := wantCode(t, err, CodePodNotFound)
		if oe.Class != ClassUser {
			t.Errorf("%s: class %s", name, oe.Class)
		}
	}
	_, err := pickPod([]byte(`{"items":[]}`))
	if !strings.Contains(err.Error(), "found 0") {
		t.Errorf("message = %v", err)
	}
}

func TestKubectlArgv(t *testing.T) {
	if g := strings.Join(getPodsArgv("/k/cfg", "ns", "rel"), " "); g != "get pods --kubeconfig /k/cfg -n ns -l app.kubernetes.io/name=postgres,app.kubernetes.io/instance=rel -o json" {
		t.Errorf("get argv = %s", g)
	}
	if g := strings.Join(execPrefix("", "", "postgres-0"), " "); g != "exec -i postgres-0 -c postgres --" {
		t.Errorf("exec prefix = %s", g)
	}
	if g := strings.Join(execPrefix("/k", "n", "p"), " "); g != "exec -i --kubeconfig /k -n n p -c postgres --" {
		t.Errorf("exec prefix = %s", g)
	}
}

func TestNewKubectlMissingBinary(t *testing.T) {
	old := lookPath
	defer func() { lookPath = old }()
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	_, err := NewKubectl(t.Context(), "", "", "nself")
	oe := wantCode(t, err, CodeKubectlMissing)
	if oe.Class != ClassInfra {
		t.Errorf("class = %s", oe.Class)
	}
}

func TestCleanEnvDropsNselfVariables(t *testing.T) {
	got := cleanEnv([]string{"PATH=/bin", "NSELF_PLUGIN_LICENSE_KEY=x", "HOME=/h", "NSELF_X=1"})
	if strings.Join(got, ",") != "PATH=/bin,HOME=/h" {
		t.Errorf("env = %v", got)
	}
}
