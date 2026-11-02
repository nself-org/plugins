package diagnose

import "testing"

func TestResourceOutputCannotAuthorizeInfraRetry(t *testing.T) {
	for name, in := range map[string]Input{
		"bare exit 137":               {ExitCode: 137},
		"shell Killed line after 137": {ExitCode: 137, Tail: "make\nsh: line 1:  4242 Killed   cc1\n"},
		"kernel oom line in output":   {ExitCode: 1, Tail: "build\nOut of memory: Killed process 4242 (cc1)\n"},
		"allocation refusal":          {ExitCode: 1, Tail: "cannot allocate memory"},
		"fork refusal":                {ExitCode: 1, Tail: "sh: can't fork"},
		"fork unavailable":            {ExitCode: 1, Tail: "fork: Resource temporarily unavailable"},
		"pids counter in output":      {ExitCode: 1, Tail: "pids.events: max=8"},
		"prepare memory":              {ExitCode: PrepareExit, Tail: "sh: line 1:  4242 Killed npm ci\n" + PrepareFailed},
	} {
		t.Run(name, func(t *testing.T) {
			d := Classify(in)
			if d.Class != "code" || d.Signature != SigCode || d.Retryable || d.Remediation == "" {
				t.Fatalf("job output alone: got %+v, want nonretryable code with remediation", d)
			}
			in.Reason = "resource_limit"
			d = Classify(in)
			if d.Class != "infra" || d.Signature != SigOOM || !d.Retryable || d.Remediation == "" {
				t.Fatalf("host evidence overridden by job output: %+v", d)
			}
		})
	}
}

func TestHostResourceEvidencePrecedesJobMarkers(t *testing.T) {
	for _, in := range []Input{
		{Reason: "resource_limit", ExitCode: PinExit, Tail: PinMismatch},
		{Reason: "resource_limit", ExitCode: PrepareExit, Tail: "npm error code ENOSPC\n" + PrepareFailed},
	} {
		d := Classify(in)
		if d.Class != "infra" || d.Signature != SigOOM || !d.Retryable {
			t.Fatalf("host resource evidence overridden by job marker: %+v", d)
		}
	}
}

func TestHostErrorsStillAuthorizeInfraRetry(t *testing.T) {
	for text, sig := range map[string]string{
		"no space left on device":                     SigDisk,
		"temporary failure in name resolution":        SigDNS,
		"Cannot connect to the Docker daemon":         SigDocker,
		"executable file not found":                   SigTool,
		"Get https://proxy.golang.org/x: i/o timeout": SigNetwork,
	} {
		d := Classify(Input{ExecClass: "infra", ExitCode: -1, Err: text})
		if d.Class != "infra" || d.Signature != sig || !d.Retryable || d.Remediation == "" {
			t.Errorf("host error %q: got %+v, want retryable %s", text, d, sig)
		}
	}
}
