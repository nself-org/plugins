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
	half := strings.Repeat("x", 256<<10)
	cases := []struct {
		name    string
		fsys    fstest.MapFS
		wantErr string
	}{
		{"exactly at the limit", fstest.MapFS{
			"a.sql": {Data: []byte(half)}, "b.sql": {Data: []byte(half)},
		}, ""},
		{"one byte over names the file", fstest.MapFS{
			"a.sql": {Data: []byte(half)}, "b.sql": {Data: []byte(half + "y")},
		}, "initdb file b.sql takes the scripts over 512 KiB"},
		{"base64 size counts for gz", fstest.MapFS{
			"a.sql":    {Data: []byte(half)},
			"b.sql.gz": {Data: make([]byte, 200<<10)}, // 200 KiB raw is about 267 KiB stored
		}, "initdb file b.sql.gz"},
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

func TestInitDBTwoMountsIsAnError(t *testing.T) {
	m := initModel("/p/init")
	cs := m.Services["postgres"]
	cs.Volumes = append(cs.Volumes, ComposeMount{Type: "bind", Source: "/p/other", Target: "/docker-entrypoint-initdb.d/"})
	m.Services["postgres"] = cs
	_, _, err := MapWith(m, nil, memFS(fstest.MapFS{}))
	if err == nil || !strings.Contains(err.Error(), "service postgres: more than one bind mount") {
		t.Errorf("err = %v, want an error naming the service", err)
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

func TestLicenceKeyIsNeverAValue(t *testing.T) {
	const key = "fake-licence-0000"
	k, other := key, "3711"
	m := &Model{Name: "p", Services: map[string]ComposeSvc{
		"mux": {Image: "x/mux:1", Hostname: "plugin-mux", Environment: map[string]*string{
			"NSELF_PLUGIN_LICENSE_KEY": &k, "PORT": &other}},
	}}
	v, s, err := Map(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	svc := v.Services["mux"]
	if got := strings.Join(svc.Env, ","); got != "PORT" {
		t.Errorf("env names = %q, want only PORT", got)
	}
	if _, ok := s.Secrets["mux"]["NSELF_PLUGIN_LICENSE_KEY"]; ok {
		t.Error("the licence key env is in secrets.yaml")
	}
	if got := strings.Join(svc.NotMapped, "|"); got != "env:NSELF_PLUGIN_LICENSE_KEY (pass --license-secret)" {
		t.Errorf("notMapped = %q", got)
	}
	vb, _ := MarshalValues(v)
	sb, _ := MarshalSecrets(s)
	if strings.Contains(string(vb), key) || strings.Contains(string(sb), key) {
		t.Error("the licence key value reached values.yaml or secrets.yaml")
	}
}

func TestCheckComparesBaseDomainAndPluginMarker(t *testing.T) {
	b, m := fullValuesBytes(t)
	opts := fixtureOpts("full")
	// baseDomain differs.
	edited := strings.Replace(string(b), "baseDomain: example.test", "baseDomain: other.test", 1)
	diffs, err := CheckWith([]byte(edited), m, opts)
	if err != nil || len(diffs) != 1 || diffs[0].Service != ProjectDiff || !strings.Contains(diffs[0].Detail, "baseDomain") {
		t.Errorf("baseDomain: diffs = %v, err = %v", diffs, err)
	}
	// plugin marker differs: add one to hasura in values, compose has none.
	edited = strings.Replace(string(b), "  hasura:\n    kind: deployment\n", "  hasura:\n    kind: deployment\n    plugin: ghost\n", 1)
	if edited == string(b) {
		t.Fatal("fixture layout changed: hasura block not found")
	}
	diffs, err = CheckWith([]byte(edited), m, opts)
	if err != nil || len(diffs) != 1 || diffs[0].Service != "hasura" || !strings.Contains(diffs[0].Detail, "plugin marker") {
		t.Errorf("plugin marker: diffs = %v, err = %v", diffs, err)
	}
}

// TestLicenceEnvDroppedBeforeInterpolationCheck: the licence env is dropped even
// when its value is an unresolved interpolation; any other env with "${" still errors.
func TestLicenceEnvDroppedBeforeInterpolationCheck(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{"literal interpolation is dropped", map[string]string{"NSELF_PLUGIN_LICENSE_KEY": "${NSELF_PLUGIN_LICENSE_KEY}", "PORT": "1"}, false},
		{"resolved value is dropped", map[string]string{"NSELF_PLUGIN_LICENSE_KEY": "fake-licence-0000"}, false},
		{"other env still errors", map[string]string{"NSELF_PLUGIN_LICENSE_KEY": "${X}", "OTHER": "${Y}"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := map[string]*string{}
			for k, v := range c.env {
				v := v
				env[k] = &v
			}
			v, s, err := Map(&Model{Name: "p", Services: map[string]ComposeSvc{"mux": {Image: "x/mux:1", Environment: env}}}, nil)
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if err != nil {
				return
			}
			for _, n := range v.Services["mux"].Env {
				if n == "NSELF_PLUGIN_LICENSE_KEY" {
					t.Error("licence env name kept")
				}
			}
			if _, ok := s.Secrets["mux"]["NSELF_PLUGIN_LICENSE_KEY"]; ok {
				t.Error("licence env value kept")
			}
		})
	}
}
