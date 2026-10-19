package provenance_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/provenance"
)

func TestParseRoundTrip(t *testing.T) {
	st, err := provenance.Parse(mustBuild(t, releaseFacts()))
	if err != nil {
		t.Fatal(err)
	}
	if st.Type != provenance.StatementType || st.PredicateType != provenance.PredicateType || len(st.Subject) != 2 {
		t.Fatalf("%+v", st)
	}
	if st.SubjectDigests()["image/nself"] != hex64("d") || st.Subject[0].Name != "image/nself" {
		t.Fatal("subjects")
	}
	if _, ok := st.Byproduct("nope"); ok {
		t.Fatal("unknown byproduct found")
	}
	if _, ok := st.Dependency("nope"); ok {
		t.Fatal("unknown dependency found")
	}
	if (provenance.Statement{}).GitCommit() != "" {
		t.Fatal("empty statement has a commit")
	}
}

func TestParseAcceptsUnknownAdditiveFields(t *testing.T) {
	good := string(mustBuild(t, releaseFacts()))
	extra := strings.Replace(good, `"predicateType"`, `"extra":{"x":1},"predicateType"`, 1)
	if _, err := provenance.Parse([]byte(extra)); err != nil {
		t.Fatal(err)
	}
}

func TestParseRefuses(t *testing.T) {
	good := string(mustBuild(t, releaseFacts()))
	digest := hex64("d")
	for name, doc := range map[string]string{
		"empty":                 "",
		"not json":              "{",
		"wrong type":            strings.Replace(good, "https://in-toto.io/Statement/v1", "https://in-toto.io/Statement/v0.1", 1),
		"wrong predicate type":  strings.Replace(good, "https://slsa.dev/provenance/v1", "https://slsa.dev/provenance/v0.2", 1),
		"repeated key":          strings.Replace(good, `"subject"`, `"subject":[],"subject"`, 1),
		"case variant key":      strings.Replace(good, `"subject"`, `"Subject":[],"subject"`, 1),
		"no subject":            strings.Replace(good, `"subject":[`, `"subject":[],"x":[`, 1),
		"upper-case digest":     strings.Replace(good, digest, strings.ToUpper(digest), 1),
		"short digest":          strings.Replace(good, digest, "abc", 1),
		"repeated subject":      strings.Replace(good, `"name":"image/nself"`, `"name":"nself-linux-amd64"`, 1),
		"empty subject name":    strings.Replace(good, `"name":"image/nself"`, `"name":""`, 1),
		"no build type":         strings.Replace(good, provenance.BuildTypeRelease, "", 1),
		"no builder":            strings.Replace(good, `"builder":{"id":`, `"builder":{"x":`, 1),
		"no invocation id":      strings.Replace(good, `"invocationId"`, `"x"`, 1),
		"digest-less byproduct": strings.Replace(good, `{"digest":{"sha256":"`+hex64("6")+`"},"name":"evidence.json"}`, `{"digest":{},"name":"evidence.json"}`, 1),
		"trailing data":         good + "{}",
		"digest not a string":   strings.Replace(good, `"sha256":"`+digest+`"`, `"sha256":5`, 1),
		"no external params":    strings.Replace(good, `"externalParameters"`, `"x"`, 1),
		"oversize":              good + strings.Repeat(" ", 4<<20),
	} {
		if _, err := provenance.Parse([]byte(doc)); !errors.Is(err, provenance.ErrInvalid) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
