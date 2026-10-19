package android

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type gh struct {
	assets     string
	uploadExit int
	uploadOut  string
	calls      [][]string
}

func (g *gh) run(_ context.Context, argv ...string) (string, int, error) {
	g.calls = append(g.calls, argv)
	if argv[2] == "view" {
		return g.assets, 0, nil
	}
	return g.uploadOut, g.uploadExit, nil
}

func TestUploadArgv(t *testing.T) {
	g := &gh{assets: "x.apk\n"}
	if err := Upload(context.Background(), g.run, "o", "r", "v1", []string{"/rel/a.apk", "/rel/b.apk"}); err != nil {
		t.Fatal(err)
	}
	if len(g.calls) != 2 {
		t.Fatalf("calls %v", g.calls)
	}
	if got := strings.Join(g.calls[1], " "); got != "gh release upload v1 /rel/a.apk /rel/b.apk --repo o/r" {
		t.Fatalf("upload argv %q", got)
	}
	if strings.Contains(strings.Join(g.calls[0], " "), "clobber") {
		t.Fatal("clobber in the check")
	}
}

func TestUploadRefusesExistingAsset(t *testing.T) {
	g := &gh{assets: "a.apk\nz.apk\n"}
	err := Upload(context.Background(), g.run, "o", "r", "v1", []string{"/rel/a.apk"})
	if !errors.Is(err, ErrAssetExists) || !strings.Contains(err.Error(), "a.apk") || len(g.calls) != 1 {
		t.Fatalf("err %v after %d calls", err, len(g.calls))
	}
	// gh's own refusal (the asset appeared between the check and the upload) is mapped too
	g = &gh{uploadExit: 1, uploadOut: "HTTP 422: a.apk already exists\n"}
	if err = Upload(context.Background(), g.run, "o", "r", "v1", []string{"/rel/a.apk"}); !errors.Is(err, ErrAssetExists) {
		t.Fatalf("gh already-exists: %v", err)
	}
	g = &gh{uploadExit: 1, uploadOut: "network down\nmore\n"}
	if err = Upload(context.Background(), g.run, "o", "r", "v1", []string{"/rel/a.apk"}); err == nil || errors.Is(err, ErrAssetExists) || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("other failure: %v", err)
	}
}

func TestUploadChecksArguments(t *testing.T) {
	g := &gh{}
	for _, c := range []struct{ owner, repo, tag, apk string }{
		{"--o", "r", "v1", "/a.apk"}, {"o", "r;x", "v1", "/a.apk"}, {"o", "r", "-v1", "/a.apk"}, {"o", "r", "v1", "a.apk"}, {"o", "r", "v1", "/dir/-a.apk"}, {"", "r", "v1", "/a.apk"},
	} {
		if err := Upload(context.Background(), g.run, c.owner, c.repo, c.tag, []string{c.apk}); err == nil {
			t.Errorf("accepted %+v", c)
		}
	}
	if err := Upload(context.Background(), g.run, "o", "r", "v1", nil); err == nil {
		t.Error("accepted no APK")
	}
	if len(g.calls) != 0 {
		t.Fatalf("gh was called for a bad argument: %v", g.calls)
	}
}

func TestParseRemote(t *testing.T) {
	for url, want := range map[string]string{
		"https://github.com/acme/app.git":      "acme/app",
		"https://github.com/acme/app":          "acme/app",
		"git@github.com:acme/app.git":          "acme/app",
		"ssh://git@github.com/acme/app.git":    "acme/app",
		"https://x@github.com/acme/my.app.git": "acme/my.app",
		"app":                                  "",
		"https://github.com/-bad/app":          "",
	} {
		o, r, ok := ParseRemote(url)
		got := ""
		if ok {
			got = o + "/" + r
		}
		if got != want {
			t.Errorf("%s -> %q, want %q", url, got, want)
		}
	}
}
