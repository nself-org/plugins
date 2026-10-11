// Purpose: tests for the init-script mapping (D-0308) and the BASE_DOMAIN
// lookup (D-0311): which files are carried, what is skipped, where the
// contents go, and the size limit.
//
// Inputs: inline models over fstest.MapFS, and testdata/full.
//
// Outputs: test results.
//
// Constraints: no disk outside testdata, no docker.
package values

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func initModel(src string) *Model {
	return &Model{Name: "p", Services: map[string]ComposeSvc{
		"postgres": {Image: "postgres:16-alpine", Volumes: []ComposeMount{
			{Type: "bind", Source: src, Target: "/docker-entrypoint-initdb.d", ReadOnly: true},
		}},
	}}
}

func memFS(m fstest.MapFS) MapOptions {
	return MapOptions{InitFS: func(string) (fs.FS, error) { return m, nil }}
}

func TestInitDBMapping(t *testing.T) {
	cases := []struct {
		name        string
		fsys        fstest.MapFS
		wantNames   string
		wantSkipped string
		wantSecrets int
	}{
		{"sorted sql sh gz", fstest.MapFS{
			"20-b.sh":     {Data: []byte("echo b\n")},
			"10-a.sql":    {Data: []byte("select 1;\n")},
			"30-c.sql.gz": {Data: []byte{0x1f, 0x8b, 0x08, 0x00}},
		}, "10-a.sql 20-b.sh 30-c.sql.gz", "", 3},
		{"others are listed not carried", fstest.MapFS{
			"10-a.sql":     {Data: []byte("select 1;\n")},
			"notes.txt":    {Data: []byte("x")},
			"big.sql.xz":   {Data: []byte("x")},
			"sub/z.sql":    {Data: []byte("x")},
			"bad name.sql": {Data: []byte("x")},
		}, "10-a.sql", "initdb:bad name.sql initdb:big.sql.xz initdb:notes.txt initdb:sub", 1},
		{"dot files are ignored", fstest.MapFS{
			".gitkeep": {Data: nil},
			"a.sql":    {Data: []byte("select 1;")},
		}, "a.sql", "", 1},
		{"symlink is not regular", fstest.MapFS{
			"a.sql": {Data: []byte("x"), Mode: fs.ModeSymlink},
		}, "", "initdb:a.sql", 0},
		{"empty directory", fstest.MapFS{}, "", "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v, s, err := MapWith(initModel("/p/init"), nil, memFS(c.fsys))
			if err != nil {
				t.Fatal(err)
			}
			svc := v.Services["postgres"]
			if got := strings.Join(svc.InitDB, " "); got != c.wantNames {
				t.Errorf("initdb = %q, want %q", got, c.wantNames)
			}
			var skipped []string
			for _, n := range svc.NotMapped {
				if strings.HasPrefix(n, "initdb:") {
					skipped = append(skipped, n)
				}
				if strings.HasPrefix(n, "bind-mount:") {
					t.Errorf("the initdb mount is still listed notMapped: %v", svc.NotMapped)
				}
			}
			if got := strings.Join(skipped, " "); got != c.wantSkipped {
				t.Errorf("skipped = %q, want %q", got, c.wantSkipped)
			}
			if got := len(s.InitDB["postgres"]); got != c.wantSecrets {
				t.Errorf("secrets carry %d scripts, want %d", got, c.wantSecrets)
			}
		})
	}
}

func TestInitDBContentsOnlyInSecrets(t *testing.T) {
	const pw = "fake-initdb-0000"
	fsys := fstest.MapFS{
		"10-a.sql":    {Data: []byte("CREATE ROLE r PASSWORD '" + pw + "';\n")},
		"20-b.sql.gz": {Data: []byte{0x1f, 0x8b, 0x00, 0xff}},
	}
	v, s, err := MapWith(initModel("/p/init"), nil, memFS(fsys))
	if err != nil {
		t.Fatal(err)
	}
	vb, _ := MarshalValues(v)
	sb, _ := MarshalSecrets(s)
	if strings.Contains(string(vb), pw) {
		t.Error("an init script content reached values.yaml")
	}
	if !strings.Contains(string(sb), pw) {
		t.Error("secrets.yaml does not carry the init script content")
	}
	if got := s.InitDB["postgres"]["20-b.sql.gz"]; got != "H4sA/w==" {
		t.Errorf("gz script = %q, want base64 of its bytes", got)
	}
}

func TestInitDBLimitAndEncoding(t *testing.T) {
	big := strings.Repeat("x", 600<<10)
	cases := []struct {
		name    string
		fsys    fstest.MapFS
		wantErr string
	}{
		{"within limit", fstest.MapFS{"a.sql": {Data: []byte(big)}}, ""},
		{"over limit names the file", fstest.MapFS{
			"a.sql": {Data: []byte(big)}, "b.sql": {Data: []byte(big)},
		}, "initdb file b.sql"},
		{"not utf8", fstest.MapFS{"a.sql": {Data: []byte{0xff, 0xfe}}}, "initdb file a.sql is not valid UTF-8"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := MapWith(initModel("/p/init"), nil, memFS(c.fsys))
			switch {
			case c.wantErr == "" && err != nil:
				t.Fatal(err)
			case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
				t.Fatalf("err = %v, want it to contain %q", err, c.wantErr)
			}
			if err != nil && strings.Contains(err.Error(), "xxxx") {
				t.Error("the error echoes file content")
			}
		})
	}
}

func TestInitDBMissingDirectory(t *testing.T) {
	missing := MapOptions{InitFS: func(string) (fs.FS, error) { return errFS{}, nil }}
	v, _, err := MapWith(initModel("/p/init"), nil, missing)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(v.Services["postgres"].NotMapped, " "); got != "initdb:source-missing" {
		t.Errorf("notMapped = %q", got)
	}
}

// errFS reports every path as missing.
type errFS struct{}

func (errFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }

func TestInitDBFixtureAndCheck(t *testing.T) {
	m, v, s := mapFixture(t, "full")
	if got := strings.Join(v.Services["postgres"].InitDB, " "); got != "00-extensions.sql 10-auth.sql 20-hook.sh 30-seed.sql.gz" {
		t.Errorf("fixture initdb = %q", got)
	}
	if len(s.InitDB["postgres"]) != 4 {
		t.Errorf("fixture secrets carry %d scripts", len(s.InitDB["postgres"]))
	}
	// --check: dropping a name from values.yaml is a parity difference.
	v.Services["postgres"] = withoutInitDB(v.Services["postgres"])
	b, _ := MarshalValues(v)
	diffs, err := CheckWith(b, m, fixtureOpts("full"))
	if err != nil || len(diffs) != 1 || diffs[0].Service != "postgres" || !strings.Contains(diffs[0].Detail, "initdb") {
		t.Errorf("diffs = %v, err = %v, want one postgres initdb difference", diffs, err)
	}
}

func withoutInitDB(s Service) Service { s.InitDB = s.InitDB[:1]; return s }

func TestBaseDomain(t *testing.T) {
	env := func(v string) map[string]*string { return map[string]*string{"BASE_DOMAIN": &v} }
	svc := func(e map[string]*string) ComposeSvc { return ComposeSvc{Image: "x:1", Environment: e} }
	cases := []struct {
		name string
		in   map[string]ComposeSvc
		want string
	}{
		{"nginx wins", map[string]ComposeSvc{"nginx": svc(env("Example.Test")), "a": svc(env("other.test"))}, "example.test"},
		{"all agree", map[string]ComposeSvc{"a": svc(env("x.test")), "b": svc(env("x.test")), "c": svc(nil)}, "x.test"},
		{"conflict emits nothing", map[string]ComposeSvc{"a": svc(env("x.test")), "b": svc(env("y.test"))}, ""},
		{"unresolved ignored", map[string]ComposeSvc{"a": svc(env("${BASE_DOMAIN}"))}, ""},
		{"empty ignored", map[string]ComposeSvc{"a": svc(env(" "))}, ""},
		{"none", map[string]ComposeSvc{"a": svc(nil)}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := baseDomain(&Model{Services: c.in}); got != c.want {
				t.Errorf("baseDomain = %q, want %q", got, c.want)
			}
		})
	}
	_, v, _ := mapFixture(t, "full")
	if v.BaseDomain != "example.test" {
		t.Errorf("fixture baseDomain = %q", v.BaseDomain)
	}
}
