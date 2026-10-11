// Purpose: the integration test of Backup and Restore against a real postgres:16
// container, through the same Exec seam the cluster uses (docker exec instead of
// kubectl exec).
//
// Inputs: NSELF_K8S_PG_TEST=1 and a working docker. Skipped otherwise.
//
// Outputs: test results. The container (--rm, synthetic throwaway password given
// through the process environment, never on an argv) is always removed.
//
// Constraints: the fixture has the auth and storage schemas, odd identifiers, a
// jsonb, a float, a timestamptz and a bytea column, and one table of five million
// rows. The test proves: the fingerprints cover auth and storage, a restore after
// damage ends with equal fingerprints, and a change after the restore is seen.
package ops

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const fixtureSQL = `
CREATE SCHEMA auth;
CREATE SCHEMA storage;
CREATE SCHEMA "Odd Schema";
CREATE TABLE auth.users (id serial PRIMARY KEY, email text, meta jsonb, created timestamptz, ratio double precision, iv interval);
INSERT INTO auth.users (email, meta, created, ratio, iv) VALUES
  ('a@example.test', '{"b": 1, "a": [1, 2, {"z": null}]}', '2026-01-02 03:04:05.678+05', 0.1, '1 day 02:03:04'),
  ('b@example.test', '{}', '2026-10-11 00:00:00+00', 1e-7, '-5 months');
CREATE TABLE storage.files (id uuid PRIMARY KEY, data bytea, size real, ts timestamp);
INSERT INTO storage.files SELECT ('00000000-0000-0000-0000-' || lpad(g::text, 12, '0'))::uuid, decode(md5(g::text), 'hex'), g / 3.0, '2026-01-01'::timestamp + g * interval '1 minute' FROM generate_series(1, 500) g;
CREATE TABLE public."Weird ""name""" (id int, "Col ""x""" text);
INSERT INTO public."Weird ""name""" VALUES (1, E'tab\there\nnewline'), (2, NULL);
CREATE TABLE "Odd Schema"."t;DROP TABLE x" (a int);
INSERT INTO "Odd Schema"."t;DROP TABLE x" VALUES (1), (2), (3);
CREATE TABLE public.empty_t (a int);
CREATE TABLE public.shadow (id int, nself_fp_row text);
INSERT INTO public.shadow VALUES (1, 'a'), (2, 'a');
CREATE TABLE public.shadow2 (id int);
DO $$ BEGIN EXECUTE format('ALTER TABLE public.shadow2 ADD COLUMN %I text DEFAULT ''a''', 'nself_fp_' || 'public.shadow2'::regclass::oid::text); END $$;
INSERT INTO public.shadow2 (id) VALUES (1), (2);
CREATE TABLE public.big AS SELECT g::bigint AS id, (g %% 1000)::int AS v, md5(g::text) AS h FROM generate_series(1, 5000000) g;
`

func TestIntegrationBackupRestoreRealPostgres(t *testing.T) {
	if os.Getenv("NSELF_K8S_PG_TEST") != "1" {
		t.Skip("set NSELF_K8S_PG_TEST=1 to run against a postgres container (needs docker)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	ex := startPostgres(ctx, t)

	sql(ctx, t, ex, strings.ReplaceAll(fixtureSQL, "%%", "%"))
	// Timeouts a database may set: the sessions of this package must lift them. The
	// fingerprint of the 5M-row table takes longer than a second, and the holding session
	// idles in a transaction while pg_dump runs.
	sql(ctx, t, ex, `ALTER DATABASE postgres SET statement_timeout = '1s'; ALTER DATABASE postgres SET idle_in_transaction_session_timeout = '1s'; ALTER DATABASE postgres SET lock_timeout = '1s';`)
	dir := t.TempDir()
	res, err := Backup(ctx, BackupOptions{Exec: ex, Path: dir + "/it_full_20261011_000000.dump", Project: "it", Release: "nself", Now: time.Now})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	rows := map[string]int64{}
	for _, f := range res.Tables {
		rows[f.Schema+"."+f.Table] = f.Rows
	}
	for name, want := range map[string]int64{"auth.users": 2, "storage.files": 500, `public.Weird "name"`: 2,
		"Odd Schema.t;DROP TABLE x": 3, "public.empty_t": 0, "public.big": 5000000, "public.shadow": 2, "public.shadow2": 2} {
		if got, ok := rows[name]; !ok || got != want {
			t.Errorf("fingerprint %q = %d (present %v), want %d", name, got, ok, want)
		}
	}
	if res.Bytes == 0 || len(res.SHA256) != 64 {
		t.Errorf("result = %+v", res)
	}

	// Damage the database, then restore and verify.
	sql(ctx, t, ex, `DELETE FROM auth.users; UPDATE public.big SET v = v + 1 WHERE id <= 10; DROP TABLE "Odd Schema"."t;DROP TABLE x"; INSERT INTO storage.files VALUES ('ffffffff-0000-0000-0000-000000000000', 'x', 1, now());`)
	if before, err := ReadFingerprints(ctx, ex); err != nil || len(Compare(res.Tables, before)) == 0 {
		t.Fatalf("damage was not seen: %v", err)
	}
	rr, err := Restore(ctx, RestoreOptions{Exec: ex, File: res.File})
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if d := Compare(res.Tables, rr.Tables); len(d) != 0 {
		t.Errorf("restore result differs: %v", d)
	}

	// A change after the restore must show in the fingerprints.
	// A table with a column named nself_fp_row (or like the generated alias) is hashed whole,
	// so a change in another column must show.
	sql(ctx, t, ex, `UPDATE storage.files SET size = size + 1 WHERE id = '00000000-0000-0000-0000-000000000001'; UPDATE public.shadow SET id = 3 WHERE id = 1; UPDATE public.shadow2 SET id = 3 WHERE id = 1;`)
	after, err := ReadFingerprints(ctx, ex)
	if err != nil {
		t.Fatal(err)
	}
	d := Compare(res.Tables, after)
	kinds := map[string]string{}
	for _, x := range d {
		kinds[x.Schema+"."+x.Table] = x.Kind
	}
	if len(d) != 3 || kinds["storage.files"] != DiffSum || kinds["public.shadow"] != DiffSum || kinds["public.shadow2"] != DiffSum {
		t.Errorf("diffs after tampering = %v, want a content difference on storage.files, public.shadow and public.shadow2", d)
	}
}

// startPostgres runs a throwaway postgres:16-alpine and returns an Exec into it.
func startPostgres(ctx context.Context, t *testing.T) Exec {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not found")
	}
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	name := "nself-k8s-pgtest-" + hex.EncodeToString(b)
	// The password is a synthetic throwaway; it reaches docker through the
	// environment, never an argv.
	run := exec.CommandContext(ctx, "docker", "run", "-d", "--rm", "--name", name, "-e", "POSTGRES_PASSWORD", "postgres:16-alpine")
	run.Env = append(cleanEnv(os.Environ()), "POSTGRES_PASSWORD=throwaway-fixture-"+hex.EncodeToString(b))
	if out, err := run.CombinedOutput(); err != nil {
		t.Fatalf("docker run: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})
	ex := NewDocker(name)
	deadline := time.Now().Add(2 * time.Minute)
	for {
		var out, serr bytes.Buffer
		err := ex.Run(ctx, []string{"pg_isready", "-q", "-h", "127.0.0.1", "-U", "postgres"}, nil, &out, &serr)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("postgres never became ready: %v", err)
		}
		time.Sleep(time.Second)
	}
	return ex
}

// sql runs statements through the pod's psql.
func sql(ctx context.Context, t *testing.T, ex Exec, script string) {
	t.Helper()
	var out, serr bytes.Buffer
	if err := ex.Run(ctx, psqlArgv, strings.NewReader(script), &out, &serr); err != nil {
		t.Fatalf("psql: %v\n%s", err, serr.String())
	}
}
