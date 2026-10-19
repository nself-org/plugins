package main

// Purpose: the signer opens the run store read-only (D-1094): against a state.db
// older than this binary it refuses and leaves the schema as it found it.
// Inputs: none. Outputs: pass or fail.
// Constraints: unix only (shares the artifacts rig and fake cli with the key tests).

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/nself-org/plugins/free/ci/internal/signing"
)

func TestSignerDoesNotMigrateAnOlderStore(t *testing.T) {
	newArtRig(t)
	fakeNself(t)
	db, err := sql.Open("sqlite", "file:"+runsStorePath())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	// Make the database one migration behind: forget trust-policy/0001 and its tables.
	// A read-write open would apply the migration again; the signer must not.
	old := "DELETE FROM migration_log WHERE set_name='trust-policy'; UPDATE schema_meta SET schema_version=schema_version-1; DROP TABLE trust_policy_row; DROP TABLE trust_policy_pin"
	if _, err = db.Exec(old); err != nil {
		t.Fatal(err)
	}
	version := func() (v int) {
		if err := db.QueryRow("SELECT schema_version FROM schema_meta WHERE id=1").Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	before := version()
	raw, _ := json.Marshal(signing.Request{RunID: "run-x", Items: []signing.Item{{PayloadType: signing.PayloadStatement, PayloadB64: b64([]byte("{}"))}}})
	signerIn = bytes.NewReader(raw)
	defer func() { signerIn = nil }()
	code, out, _ := runArt(t, "signer", "--stdio")
	var resp signing.Response
	if code != 2 || json.Unmarshal([]byte(out), &resp) != nil || resp.Error == nil || resp.Error.Reason != signing.ReasonSignerFailed {
		t.Fatalf("an older store was not refused: exit %d, %q", code, out)
	}
	if after := version(); after != before {
		t.Fatalf("the signer migrated the store: schema_version %d -> %d", before, after)
	}
}
