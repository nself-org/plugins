// Purpose: regressions for private database diagnostics and whole-row alias collisions.
// Inputs: synthetic messages; NSELF_K8S_PG_TEST=1 enables throwaway PostgreSQL.
// Outputs: test results. Constraints: no production data or services.
package ops

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestHumanDetailDoesNotExposeUnquotedDatabaseValues(t *testing.T) {
	for _, detail := range []string{
		"pg_restore: error: ERROR: customer token unquoted-private-value rejected",
		"pg_dump: error: ERROR: duplicate key unquoted-private-value",
		"pg_restore: error: ERROR: invalid input syntax unquoted-private-value",
		"pg_dump: error: connection to server failed unquoted-private-value",
	} {
		got := (&Error{Detail: detail}).HumanDetail()
		switch got {
		case "database operation failed", "database constraint violation", "database input rejected", "database connection failed":
		default:
			t.Errorf("unsafe diagnostic %q for %q", got, detail)
		}
	}
	if got := (&Error{}).HumanDetail(); got != "" {
		t.Errorf("empty detail = %q", got)
	}
}

func TestIntegrationFingerprintAliasCannotShadowAtIdentifierLimit(t *testing.T) {
	if os.Getenv("NSELF_K8S_PG_TEST") != "1" {
		t.Skip("set NSELF_K8S_PG_TEST=1 to run against postgres")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	ex := startPostgres(ctx, t)
	sql(ctx, t, ex, `CREATE TABLE public.alias_limit (id int);
DO $$ DECLARE base text := 'nself_fp_' || 'public.alias_limit'::regclass::oid::text;
BEGIN
  EXECUTE format('ALTER TABLE public.alias_limit ADD COLUMN %I text DEFAULT ''constant''', rpad(base, 63, '_'));
  FOR i IN 0..3 LOOP
    EXECUTE format('ALTER TABLE public.alias_limit ADD COLUMN %I text DEFAULT ''constant''', base || '_' || i);
  END LOOP;
END $$;
INSERT INTO public.alias_limit (id) VALUES (1);`)
	before, err := ReadFingerprints(ctx, ex)
	if err != nil {
		t.Fatal(err)
	}
	sql(ctx, t, ex, `UPDATE public.alias_limit SET id = 2;`)
	after, err := ReadFingerprints(ctx, ex)
	if err != nil {
		t.Fatal(err)
	}
	if d := Compare(before, after); len(d) != 1 || d[0].Kind != DiffSum || !strings.EqualFold(d[0].Table, "alias_limit") {
		t.Fatalf("non-alias column mutation was missed: %v", d)
	}
}
