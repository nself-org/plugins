package store

import (
	"context"
	"database/sql"
	"testing"
)

func TestAgePinMigrationPreservesExistingRecipient(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE node(id TEXT PRIMARY KEY);
 CREATE TABLE capability_snapshot(node_id TEXT,doc_json TEXT);
 INSERT INTO node VALUES('pinned'),('empty');
 INSERT INTO capability_snapshot VALUES('pinned','{"identity":{"age_recipient":{"value":"age1existing"}}}'),('empty','{}');`)
	if err != nil {
		t.Fatal(err)
	}
	migration, err := nodeMigrations.ReadFile("migrations/node-registry/0002_age_pin.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(migration)); err != nil {
		t.Fatal(err)
	}
	var pin *string
	if err = db.QueryRow(`SELECT age_recipient FROM node WHERE id='pinned'`).Scan(&pin); err != nil || pin == nil || *pin != "age1existing" {
		t.Fatalf("migrated pin: %v", err)
	}
	if err = db.QueryRow(`SELECT age_recipient FROM node WHERE id='empty'`).Scan(&pin); err != nil || pin != nil {
		t.Fatalf("uninitialised pin: %v", err)
	}
}

func TestNodePinAgeRecipientKeepsFirstValue(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	n := NodeRecord{ID: "node", Name: "node", State: "offline", Lifecycle: []byte(`{}`), Trust: []byte(`{}`), DeployHost: []byte(`{}`), Capability: []byte(`{}`)}
	if err := s.NodePut(ctx, n, "operator"); err != nil {
		t.Fatal(err)
	}
	for _, presented := range []string{"age1first", "age1changed"} {
		if got, err := s.NodePinAgeRecipient(ctx, n.ID, presented); err != nil || got != "age1first" {
			t.Fatalf("pin changed: %s %v", got, err)
		}
	}
	if pin, err := s.NodeAgeRecipient(ctx, n.ID); err != nil || pin == nil || *pin != "age1first" {
		t.Fatalf("read pin: %v", err)
	}
	if _, err := s.NodePinAgeRecipient(ctx, "missing", "age1first"); err == nil {
		t.Fatal("missing node accepted")
	}
}
