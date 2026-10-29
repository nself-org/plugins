//go:build security

package redteam

// Purpose: red team attacks on the publication profile (TM-36): every free-text field of an
// evidence document is filled, one at a time, with private data in many spellings; the real
// attest.Publish for a public repository must remove it from the document that leaves the machine.
// Inputs:  a reflectively populated model.Evidence. Outputs: pass or fail.
// Constraints: pure, no network; the telemetry half (zero requests) is not attackable here.

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/attest"
	"github.com/nself-org/plugins/free/ci/internal/model"
)

// coreMarkers are the categories the control names: RFC1918 addresses, runner and host names, home
// paths, user names, private URLs.
var coreMarkers = []string{"10.1.2.3", "192.168.7.8", "172.16.5.4", "buildbox.local", "/Users/aliadmin/", "/home/aliadmin/", "ali@corp.example", "http://10.9.9.9/", "fd12:3456::1"}

// exoticMarkers are rarer spellings of the same data.
var exoticMarkers = []string{"0x0a000001", "167772161", "/private/var/folders/zz", "::ffff:10.0.0.9"}

// freeTextField reports a field that carries prose or a path from job output or the host.
func freeTextField(path string) bool {
	for _, suf := range []string{".Excerpt", ".Reason", ".Diagnosis", ".Remediation", ".Source", ".LogRef", "Runner.ID", "Runner.Platform", "Artifacts[0].Path", "Retries[0].Excerpt"} {
		if strings.HasSuffix(path, suf) {
			return true
		}
	}
	return strings.Contains(path, "ToolVersions{")
}

func hostile(markers []string) string {
	return "see " + strings.Join(markers, " | ") + " (10.1.2.3) ip=10.1.2.3:8080"
}

func fill(v reflect.Value, depth int) {
	if depth > 8 {
		return
	}
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		fill(v.Elem(), depth+1)
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanSet() {
				fill(v.Field(i), depth+1)
			}
		}
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return
		}
		v.Set(reflect.MakeSlice(v.Type(), 1, 1))
		fill(v.Index(0), depth+1)
	case reflect.Map:
		if v.Type().Key().Kind() == reflect.String {
			v.Set(reflect.MakeMap(v.Type()))
			e := reflect.New(v.Type().Elem()).Elem()
			fill(e, depth+1)
			v.SetMapIndex(reflect.ValueOf("k").Convert(v.Type().Key()), e)
		}
	case reflect.String:
		v.SetString("x")
	}
}

// leaves lists settable string leaves with their paths.
func leaves(v reflect.Value, path string, out *[]func(string), names *[]string) {
	switch v.Kind() {
	case reflect.Ptr:
		if !v.IsNil() {
			leaves(v.Elem(), path, out, names)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Field(i).CanSet() {
				leaves(v.Field(i), path+"."+v.Type().Field(i).Name, out, names)
			}
		}
	case reflect.Slice:
		if v.Type().Elem().Kind() != reflect.Uint8 {
			for i := 0; i < v.Len(); i++ {
				leaves(v.Index(i), fmt.Sprintf("%s[%d]", path, i), out, names)
			}
		}
	case reflect.Map:
		for _, k := range v.MapKeys() {
			k := k
			if v.Type().Elem().Kind() == reflect.String {
				*out = append(*out, func(s string) { v.SetMapIndex(k, reflect.ValueOf(s).Convert(v.Type().Elem())) })
				*names = append(*names, path+"{"+k.String()+"}")
			}
		}
	case reflect.String:
		*out = append(*out, func(s string) { v.SetString(s) })
		*names = append(*names, path)
	}
}

func TestB_Publish_NoPrivateDataInAnyTextField(t *testing.T) {
	leakingIn(t, "B11", coreMarkers, true)
}

// The rarer spellings (hex or decimal IPv4, macOS per-user temp paths) in a check excerpt.
func TestB_Publish_ExoticSpellingsInExcerpt(t *testing.T) {
	leakingIn(t, "B12", exoticMarkers, false)
}

func leakingIn(t *testing.T, id string, markers []string, allFreeText bool) {
	t.Helper()
	leaking := map[string][]string{}
	var base model.Evidence
	fill(reflect.ValueOf(&base).Elem(), 0)
	var probe []func(string)
	var names []string
	leaves(reflect.ValueOf(&base).Elem(), "Evidence", &probe, &names)
	if len(names) < 20 {
		t.Fatalf("fixture error: only %d string leaves found", len(names))
	}
	for i := range names {
		if !freeTextField(names[i]) || (!allFreeText && !strings.HasSuffix(names[i], "Checks[0].Excerpt")) {
			continue
		}
		var ev model.Evidence
		fill(reflect.ValueOf(&ev).Elem(), 0)
		var set []func(string)
		var nm []string
		leaves(reflect.ValueOf(&ev).Elem(), "Evidence", &set, &nm)
		set[i](hostile(markers))
		out, err := json.Marshal(attest.Publish(ev, attest.Public))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range markers {
			if strings.Contains(strings.ToLower(string(out)), strings.ToLower(strings.ReplaceAll(m, `\`, `\\`))) || strings.Contains(string(out), m) {
				leaking[names[i]] = append(leaking[names[i]], m)
			}
		}
	}
	if len(leaking) > 0 {
		keys := make([]string, 0, len(leaking))
		for k := range leaking {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			t.Errorf("published document keeps private data from %s: %v", k, leaking[k])
		}
	}
}

func TestB_Publish_UnknownVisibilityScrubs(t *testing.T) {
	ev := model.Evidence{RepoID: "r", Runner: model.RunnerRef{ID: "buildbox.local"}, Checks: []model.Check{{Excerpt: "10.1.2.3 at /Users/aliadmin/x"}}}
	for _, v := range []attest.Visibility{"", "PRIVATE", "Private", " private", "private ", "internal", "public"} {
		out, _ := json.Marshal(attest.Publish(ev, v))
		if strings.Contains(string(out), "10.1.2.3") || strings.Contains(string(out), "buildbox.local") || strings.Contains(string(out), "/Users/aliadmin") {
			t.Errorf("visibility %q published private data", v)
		}
	}
}
