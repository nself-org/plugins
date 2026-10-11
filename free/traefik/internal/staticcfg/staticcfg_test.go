package staticcfg

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/nself-org/plugins/free/traefik/config"
)

func parsed(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := yaml.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func at(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur = mm[k]
	}
	return cur
}

func TestDisabledIsTheBaseWithTheHeader(t *testing.T) {
	out, err := Render(config.Base, Upload{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), Header) {
		t.Fatalf("no generated header: %q", out[:60])
	}
	if !reflect.DeepEqual(parsed(t, out), parsed(t, config.Base)) {
		t.Error("disabled output differs from the base")
	}
	if at(parsed(t, out), "entryPoints", "upload") != nil {
		t.Error("disabled output has an upload entrypoint")
	}
}

func TestEnabledAddsExactlyTheUploadEntrypoint(t *testing.T) {
	out, err := Render(config.Base, Upload{Enabled: true, ReadTimeout: 600 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	got, base := parsed(t, out), parsed(t, config.Base)
	want := map[string]any{
		"address": ":8444",
		"http":    map[string]any{"encodedCharacters": map[string]any{"allowEncodedSlash": false}},
		"transport": map[string]any{"respondingTimeouts": map[string]any{
			"readTimeout": "600s", "writeTimeout": "0s", "idleTimeout": "65s"}},
	}
	if !reflect.DeepEqual(at(got, "entryPoints", "upload"), want) {
		t.Errorf("upload entrypoint = %v, want %v", at(got, "entryPoints", "upload"), want)
	}
	// everything else equals the base
	delete(got["entryPoints"].(map[string]any), "upload")
	if !reflect.DeepEqual(got, base) {
		t.Error("enabling upload changed something besides entryPoints.upload")
	}
	for _, ep := range []string{"web", "websecure"} {
		if v := at(got, "entryPoints", ep, "transport", "respondingTimeouts", "readTimeout"); v != "60s" {
			t.Errorf("%s readTimeout %v, want 60s", ep, v)
		}
	}
	if !strings.Contains(string(out), "nginx client_header_timeout") {
		t.Error("the base comments were lost")
	}
}

func TestOutputNeverAddsProvidersAPIOrResolvers(t *testing.T) {
	for _, up := range []Upload{{}, {Enabled: true, ReadTimeout: time.Hour}} {
		out, err := Render(config.Base, up)
		if err != nil {
			t.Fatal(err)
		}
		m := parsed(t, out)
		prov, _ := m["providers"].(map[string]any)
		if len(prov) != 1 || prov["file"] == nil {
			t.Errorf("providers = %v, want the file provider only", prov)
		}
		if m["api"] != nil || m["certificatesResolvers"] != nil {
			t.Error("api or certificatesResolvers present")
		}
	}
}

func TestReadTimeoutBounds(t *testing.T) {
	for _, c := range []struct {
		d  time.Duration
		ok bool
	}{{59 * time.Second, false}, {60 * time.Second, true}, {24 * time.Hour, true}, {24*time.Hour + time.Second, false}, {90*time.Second + time.Millisecond, false}, {0, false}} {
		_, err := Render(config.Base, Upload{Enabled: true, ReadTimeout: c.d})
		if (err == nil) != c.ok {
			t.Errorf("%s: err %v, want ok=%v", c.d, err, c.ok)
		}
	}
	if _, err := Render(config.Base, Upload{ReadTimeout: time.Second}); err != nil {
		t.Errorf("a disabled upload ignores its timeout: %v", err)
	}
}

func TestEveryTimeoutIsFormattedInSeconds(t *testing.T) {
	out, _ := Render(config.Base, Upload{Enabled: true, ReadTimeout: 2 * time.Hour})
	if v := at(parsed(t, out), "entryPoints", "upload", "transport", "respondingTimeouts", "readTimeout"); v != "7200s" {
		t.Errorf("readTimeout %v, want 7200s", v)
	}
}
