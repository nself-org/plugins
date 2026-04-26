package internal

import (
	"os"
	"testing"
)

// setEnv sets environment variables for the duration of the test and cleans
// them up on test exit.
func setEnv(t *testing.T, key, val string) {
	t.Helper()
	prev, set := os.LookupEnv(key)
	if err := os.Setenv(key, val); err != nil {
		t.Fatalf("setEnv: Setenv %s: %v", key, err)
	}
	t.Cleanup(func() {
		if set {
			os.Setenv(key, prev)
		} else {
			os.Unsetenv(key)
		}
	})
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	prev, set := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if set {
			os.Setenv(key, prev)
		}
	})
}

// TestLoadEnvJobs_Empty verifies that no jobs are returned when no CRON_JOB_*
// vars are set.
func TestLoadEnvJobs_Empty(t *testing.T) {
	// Clear any stray CRON_JOB_* vars from the test environment.
	for i := 1; i <= 20; i++ {
		n := itoa(i)
		unsetEnv(t, "CRON_JOB_"+n+"_SCHEDULE")
		unsetEnv(t, "CRON_JOB_"+n+"_COMMAND")
		unsetEnv(t, "CRON_JOB_"+n+"_NAME")
		unsetEnv(t, "CRON_JOB_"+n+"_PAYLOAD")
	}

	jobs := LoadEnvJobs()
	if len(jobs) != 0 {
		t.Errorf("LoadEnvJobs empty: want 0 jobs, got %d: %+v", len(jobs), jobs)
	}
}

// TestLoadEnvJobs_SingleJob verifies that a single fully-declared job is loaded.
func TestLoadEnvJobs_SingleJob(t *testing.T) {
	clearCronJobEnv(t)

	setEnv(t, "CRON_JOB_1_SCHEDULE", "0 3 * * *")
	setEnv(t, "CRON_JOB_1_COMMAND", "http://backup:8080/backup/run")
	setEnv(t, "CRON_JOB_1_NAME", "nightly-backup")

	jobs := LoadEnvJobs()
	if len(jobs) != 1 {
		t.Fatalf("LoadEnvJobs single: want 1 job, got %d", len(jobs))
	}
	j := jobs[0]
	if j.Schedule != "0 3 * * *" {
		t.Errorf("Schedule = %q, want %q", j.Schedule, "0 3 * * *")
	}
	if j.CallbackURL != "http://backup:8080/backup/run" {
		t.Errorf("CallbackURL = %q, want http://backup:8080/backup/run", j.CallbackURL)
	}
	if j.Name != "nightly-backup" {
		t.Errorf("Name = %q, want nightly-backup", j.Name)
	}
	if j.Payload != nil {
		t.Errorf("Payload should be nil when CRON_JOB_1_PAYLOAD is unset, got %q", *j.Payload)
	}
}

// TestLoadEnvJobs_DefaultName verifies that the name defaults to "env-job-<N>"
// when CRON_JOB_<N>_NAME is not set.
func TestLoadEnvJobs_DefaultName(t *testing.T) {
	clearCronJobEnv(t)

	setEnv(t, "CRON_JOB_3_SCHEDULE", "*/5 * * * *")
	setEnv(t, "CRON_JOB_3_COMMAND", "http://hasura:8080/healthz")

	jobs := LoadEnvJobs()
	if len(jobs) != 1 {
		t.Fatalf("LoadEnvJobs default name: want 1, got %d", len(jobs))
	}
	if jobs[0].Name != "env-job-3" {
		t.Errorf("Name = %q, want env-job-3", jobs[0].Name)
	}
}

// TestLoadEnvJobs_WithPayload verifies that CRON_JOB_<N>_PAYLOAD is loaded.
func TestLoadEnvJobs_WithPayload(t *testing.T) {
	clearCronJobEnv(t)

	payload := `{"bucket":"main"}`
	setEnv(t, "CRON_JOB_2_SCHEDULE", "0 0 * * *")
	setEnv(t, "CRON_JOB_2_COMMAND", "http://my-service:8080/tasks/regen")
	setEnv(t, "CRON_JOB_2_PAYLOAD", payload)

	jobs := LoadEnvJobs()
	if len(jobs) != 1 {
		t.Fatalf("LoadEnvJobs payload: want 1, got %d", len(jobs))
	}
	if jobs[0].Payload == nil {
		t.Fatal("Payload should not be nil")
	}
	if *jobs[0].Payload != payload {
		t.Errorf("Payload = %q, want %q", *jobs[0].Payload, payload)
	}
}

// TestLoadEnvJobs_MissingSchedule verifies that a job missing SCHEDULE is skipped.
func TestLoadEnvJobs_MissingSchedule(t *testing.T) {
	clearCronJobEnv(t)

	// Only COMMAND, no SCHEDULE → should be skipped.
	setEnv(t, "CRON_JOB_1_COMMAND", "http://hasura:8080/healthz")

	jobs := LoadEnvJobs()
	if len(jobs) != 0 {
		t.Errorf("LoadEnvJobs missing schedule: want 0 jobs, got %d", len(jobs))
	}
}

// TestLoadEnvJobs_MissingCommand verifies that a job missing COMMAND is skipped.
func TestLoadEnvJobs_MissingCommand(t *testing.T) {
	clearCronJobEnv(t)

	// Only SCHEDULE, no COMMAND → should be skipped.
	setEnv(t, "CRON_JOB_1_SCHEDULE", "0 3 * * *")

	jobs := LoadEnvJobs()
	if len(jobs) != 0 {
		t.Errorf("LoadEnvJobs missing command: want 0 jobs, got %d", len(jobs))
	}
}

// TestLoadEnvJobs_MultipleGaps verifies that non-contiguous job numbers load
// correctly (N=1 and N=5 with 2,3,4 unset).
func TestLoadEnvJobs_MultipleGaps(t *testing.T) {
	clearCronJobEnv(t)

	setEnv(t, "CRON_JOB_1_SCHEDULE", "0 3 * * *")
	setEnv(t, "CRON_JOB_1_COMMAND", "http://backup:8080/backup/run")
	setEnv(t, "CRON_JOB_5_SCHEDULE", "0 8 * * 1")
	setEnv(t, "CRON_JOB_5_COMMAND", "http://digest:8080/send")

	jobs := LoadEnvJobs()
	if len(jobs) != 2 {
		t.Fatalf("LoadEnvJobs gaps: want 2 jobs, got %d: %+v", len(jobs), jobs)
	}
}

// TestLoadEnvJobs_IndexPreserved verifies that the N field reflects the env var index.
func TestLoadEnvJobs_IndexPreserved(t *testing.T) {
	clearCronJobEnv(t)

	setEnv(t, "CRON_JOB_7_SCHEDULE", "0 3 * * *")
	setEnv(t, "CRON_JOB_7_COMMAND", "http://svc:8080/run")

	jobs := LoadEnvJobs()
	if len(jobs) != 1 {
		t.Fatalf("want 1 job, got %d", len(jobs))
	}
	if jobs[0].N != 7 {
		t.Errorf("N = %d, want 7", jobs[0].N)
	}
}

// clearCronJobEnv unsets all CRON_JOB_* variables for the test duration.
func clearCronJobEnv(t *testing.T) {
	t.Helper()
	for i := 1; i <= 20; i++ {
		n := itoa(i)
		unsetEnv(t, "CRON_JOB_"+n+"_SCHEDULE")
		unsetEnv(t, "CRON_JOB_"+n+"_COMMAND")
		unsetEnv(t, "CRON_JOB_"+n+"_NAME")
		unsetEnv(t, "CRON_JOB_"+n+"_PAYLOAD")
	}
}

// itoa converts an int to a decimal string without importing strconv.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 3)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
