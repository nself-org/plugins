// Purpose: tests for image pinning in Map: an untagged, undigested image is
// refused, a pinned one is mapped with pullPolicy IfNotPresent.
//
// Inputs: inline models.
//
// Outputs: test results.
//
// Constraints: no docker, no testdata.
package values

import "testing"

const fakeDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"

const noTagReason = "image has no tag or digest (k8s would pull a moving latest); pin a tag or digest in compose"

func TestMapImagePinning(t *testing.T) {
	cases := []struct {
		name       string
		svc        string
		image      string
		wantKind   string // "" means refused
		wantTag    string
		wantDigest string
		wantPolicy string
	}{
		{"untagged", "worker", "busybox", "", "", "", ""},
		{"untagged with registry port", "worker", "localhost:5000/app", "", "", "", ""},
		{"tagged", "worker", "busybox:1.36", KindDeployment, "1.36", "", PullIfNotPresent},
		{"digest only", "worker", "busybox@" + fakeDigest, KindDeployment, "", fakeDigest, PullIfNotPresent},
		{"tag and digest", "worker", "busybox:1.36@" + fakeDigest, KindDeployment, "1.36", fakeDigest, PullIfNotPresent},
		{"statefulset", "postgres", "postgres:16", KindStatefulSet, "16", "", PullIfNotPresent},
		{"job", "migrate", "busybox:1.36", KindJob, "1.36", "", PullIfNotPresent},
		{"ingress has no policy", "nginx", "nginx:1.27", KindIngress, "1.27", "", ""},
		{"untagged ingress is not refused", "nginx", "nginx", KindIngress, "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cs := ComposeSvc{Image: c.image}
			if c.wantKind == KindJob {
				cs.Restart = "no"
			}
			v, _, err := Map(&Model{Name: "p", Services: map[string]ComposeSvc{c.svc: cs}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if c.wantKind == "" {
				if len(v.Services) != 0 || len(v.Unsupported) != 1 || v.Unsupported[0].Name != c.svc || v.Unsupported[0].Reason != noTagReason {
					t.Errorf("services=%v unsupported=%+v, want %s refused with %q", v.Services, v.Unsupported, c.svc, noTagReason)
				}
				return
			}
			got, ok := v.Services[c.svc]
			if !ok || len(v.Unsupported) != 0 {
				t.Fatalf("services=%v unsupported=%+v, want %s mapped", v.Services, v.Unsupported, c.svc)
			}
			if got.Kind != c.wantKind || got.Tag != c.wantTag || got.Digest != c.wantDigest || got.PullPolicy != c.wantPolicy {
				t.Errorf("service = %+v", got)
			}
		})
	}
}

// TestMapInterpolatedImageStillErrors: an unresolved "${" in an untagged image
// stays an error, not a silent refusal.
func TestMapInterpolatedImageStillErrors(t *testing.T) {
	m := &Model{Name: "p", Services: map[string]ComposeSvc{"worker": {Image: "${REGISTRY}/app"}}}
	if _, _, err := Map(m, nil); err == nil {
		t.Error("expected an error for an unresolved interpolation")
	}
}

// TestCheckRefusedServiceIsAccountedFor: an untagged service is listed
// unsupported, so Check reports no diff for it.
func TestCheckRefusedServiceIsAccountedFor(t *testing.T) {
	m := &Model{Name: "p", Services: map[string]ComposeSvc{"worker": {Image: "busybox"}}}
	v, _, err := Map(m, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := MarshalValues(v)
	if err != nil {
		t.Fatal(err)
	}
	if diffs, err := Check(b, m); err != nil || len(diffs) != 0 {
		t.Errorf("diffs=%v err=%v, want none", diffs, err)
	}
}
