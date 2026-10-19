package records_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/artifacts/records"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestPolicyFromEnv(t *testing.T) {
	p, err := records.PolicyFromEnv(envOf(nil))
	if err != nil || p.MaxFile != 2<<30 || p.MaxTotal != 20<<30 || p.ProjectShare != 50 || p.MinFreeBytes != 0 || p.MinFreePercent != 0 {
		t.Fatalf("defaults: %+v %v", p, err)
	}
	p, err = records.PolicyFromEnv(envOf(map[string]string{"NSELF_CI_ARTIFACT_MAX_FILE": "512MiB", "NSELF_CI_ARTIFACT_MAX_BYTES": "3GB", "NSELF_CI_STORE_MIN_FREE": "7GiB", "NSELF_CI_PROJECT_SHARE": "30"}))
	if err != nil || p.MaxFile != 512<<20 || p.MaxTotal != 3e9 || p.MinFreeBytes != 7<<30 || p.ProjectShare != 30 {
		t.Fatalf("explicit: %+v %v", p, err)
	}
	p, err = records.PolicyFromEnv(envOf(map[string]string{"NSELF_CI_STORE_MIN_FREE": "15%", "NSELF_CI_ARTIFACT_MAX_FILE": "1048576"}))
	if err != nil || p.MinFreePercent != 15 || p.MinFreeBytes != 0 || p.MaxFile != 1<<20 {
		t.Fatalf("percent: %+v %v", p, err)
	}
	for k, v := range map[string]string{"NSELF_CI_ARTIFACT_MAX_FILE": "0", "NSELF_CI_ARTIFACT_MAX_BYTES": "lots", "NSELF_CI_STORE_MIN_FREE": "-1", "NSELF_CI_PROJECT_SHARE": "0"} {
		if _, err := records.PolicyFromEnv(envOf(map[string]string{k: v})); !records.IsReason(err, records.ReasonConfig) {
			t.Fatalf("%s=%s: %v", k, v, err)
		}
	}
	for _, v := range []string{"150%", "x%", "9223372036854775807GiB"} {
		if _, err := records.PolicyFromEnv(envOf(map[string]string{"NSELF_CI_STORE_MIN_FREE": v})); !records.IsReason(err, records.ReasonConfig) {
			t.Fatalf("min free %s: %v", v, err)
		}
	}
	for _, v := range []string{"101", "x", "-5"} {
		if _, err := records.PolicyFromEnv(envOf(map[string]string{"NSELF_CI_PROJECT_SHARE": v})); !records.IsReason(err, records.ReasonConfig) {
			t.Fatalf("share %s: %v", v, err)
		}
	}
	if _, err := records.PolicyFromEnv(envOf(map[string]string{"NSELF_CI_ARTIFACT_MAX_BYTES": "5%"})); !records.IsReason(err, records.ReasonConfig) {
		t.Fatal("a percent is not a byte cap")
	}
}

func TestDiskFloor(t *testing.T) {
	gib := uint64(1 << 30)
	at := func(free, total uint64) records.Policy {
		return records.Policy{Statfs: func(string) (uint64, uint64, error) { return free, total, nil }}
	}
	// default floor = larger of 5 GiB and 10 %
	if err := at(6*gib, 40*gib).CheckDisk(0); err != nil {
		t.Fatalf("6 of 40 GiB is above the 5 GiB floor: %v", err)
	}
	if err := at(5*gib-1, 40*gib).CheckDisk(0); !records.IsReason(err, records.ReasonDiskFloor) {
		t.Fatalf("just below 5 GiB: %v", err)
	}
	if err := at(50*gib, 1000*gib).CheckDisk(0); !records.IsReason(err, records.ReasonDiskFloor) {
		t.Fatalf("10 %% of 1000 GiB is 100 GiB: %v", err)
	}
	if err := at(101*gib, 1000*gib).CheckDisk(0); err != nil {
		t.Fatalf("%v", err)
	}
	// the bytes about to land count
	if err := at(6*gib, 40*gib).CheckDisk(int64(gib) + 1); !records.IsReason(err, records.ReasonDiskFloor) {
		t.Fatalf("floor plus write: %v", err)
	}
	if err := at(6*gib, 40*gib).CheckDisk(int64(gib)); err != nil {
		t.Fatalf("exactly at the floor: %v", err)
	}
	if err := at(6*gib, 40*gib).CheckDisk(-5); err != nil {
		t.Fatalf("a negative size is zero: %v", err)
	}
	// configured floors: bytes and percent, the larger wins
	p := at(10*gib, 100*gib)
	p.MinFreeBytes, p.MinFreePercent = 2*int64(gib), 20
	if err := p.CheckDisk(0); !records.IsReason(err, records.ReasonDiskFloor) {
		t.Fatalf("20 %% of 100 GiB beats 2 GiB: %v", err)
	}
	p.MinFreePercent = 5
	if err := p.CheckDisk(0); err != nil {
		t.Fatalf("5 %% = 5 GiB below 10 GiB free: %v", err)
	}
	p.MinFreeBytes = 11 * int64(gib)
	if err := p.CheckDisk(0); !records.IsReason(err, records.ReasonDiskFloor) {
		t.Fatalf("11 GiB bytes floor: %v", err)
	}
	// an unreadable volume refuses
	bad := records.Policy{Statfs: func(string) (uint64, uint64, error) { return 0, 0, errors.New("io") }}
	if err := bad.CheckDisk(0); !records.IsReason(err, records.ReasonDiskFloor) {
		t.Fatalf("unreadable: %v", err)
	}
	// the real statfs works on a real directory
	real := records.Policy{StorePath: t.TempDir(), MinFreeBytes: 1}
	if err := real.CheckDisk(0); err != nil {
		t.Fatalf("real volume: %v", err)
	}
}

func TestCheckFileAndCaps(t *testing.T) {
	p := records.Policy{MaxFile: 100, MaxTotal: 1000, ProjectShare: 50}
	if err := p.CheckFile("a", 100); err != nil {
		t.Fatal(err)
	}
	if err := p.CheckFile("a", 101); !records.IsReason(err, records.ReasonFileCap) {
		t.Fatal(err)
	}
	if w, err := p.CheckCaps(records.Usage{Total: 100}, 100); err != nil || w != "" {
		t.Fatalf("20%%: %q %v", w, err)
	}
	if w, err := p.CheckCaps(records.Usage{Total: 700}, 100); err != nil || !strings.Contains(w, "80%") || !strings.Contains(w, "`nself ci artifacts prune`") || strings.Contains(w, "--release") {
		t.Fatalf("80%% without release bytes: %q %v", w, err)
	}
	if w, err := p.CheckCaps(records.Usage{Total: 790, Release: 10}, 0); err != nil || w != "" {
		t.Fatalf("79%% must not warn: %q %v", w, err)
	}
	if w, err := p.CheckCaps(records.Usage{Total: 700, Release: 5}, 100); err != nil || !strings.Contains(w, "prune --release") {
		t.Fatalf("release hint: %q %v", w, err)
	}
	if _, err := p.CheckCaps(records.Usage{Total: 900}, 100); err != nil {
		t.Fatalf("exactly full fits: %v", err)
	}
	if _, err := p.CheckCaps(records.Usage{Total: 900}, 101); !records.IsReason(err, records.ReasonSizeCap) {
		t.Fatal(err)
	}
	// the project share applies to TEAM only
	if _, err := p.CheckCaps(records.Usage{Total: 600, Project: 600}, 10); err != nil {
		t.Fatalf("PERSONAL has no share: %v", err)
	}
	p.Team = true
	if _, err := p.CheckCaps(records.Usage{Total: 600, Project: 490}, 10); err != nil {
		t.Fatalf("at the share: %v", err)
	}
	if _, err := p.CheckCaps(records.Usage{Total: 600, Project: 490}, 11); !records.IsReason(err, records.ReasonSizeCap) || !strings.Contains(err.Error(), "NSELF_CI_PROJECT_SHARE") {
		t.Fatalf("over the share: %v", err)
	}
	if _, err := p.CheckCaps(records.Usage{Total: 1000, Project: 0}, 1); !records.IsReason(err, records.ReasonSizeCap) {
		t.Fatalf("whole store full while the project is not: %v", err)
	}
	// tiny caps do not divide by zero
	tiny := records.Policy{MaxTotal: 10}
	if w, err := tiny.CheckCaps(records.Usage{Total: 9}, 0); err != nil || w == "" {
		t.Fatalf("tiny: %q %v", w, err)
	}
}

func TestRetentionAndTotalBoundaries(t *testing.T) {
	if _, _, err := records.RetentionDuration("36500d"); err != nil {
		t.Fatalf("36500d: %v", err)
	}
	if _, _, err := records.RetentionDuration("36501d"); err == nil {
		t.Fatal("36501d")
	}
}

func TestStoreDir(t *testing.T) {
	t.Setenv("NSELF_CI_HOME", "/ci/home")
	if got := records.StoreDir(""); got != "/ci/home" {
		t.Fatal(got)
	}
	if got := records.StoreDir("/explicit"); got != "/explicit" {
		t.Fatal(got)
	}
	t.Setenv("NSELF_CI_HOME", "")
	t.Setenv("HOME", "/users/x")
	if got := records.StoreDir(""); got != "/users/x/.nself/ci" {
		t.Fatal(got)
	}
}
