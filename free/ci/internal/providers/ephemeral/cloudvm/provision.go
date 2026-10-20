package cloudvm

// Purpose: Provision, the create call of the cloud VM provider (Epic D13): check the recipe and
// the infra CLI, adopt a machine that already carries the intent label, otherwise make the
// enrollment offer, render the user data into a private 0600 file and run
// `nself infra server provision` through the funnel.
// Inputs:  a burst.Intent (token, spec, the three lifecycle labels).
// Outputs: the created machine as a burst.Resource.
// Constraints: the intent row and the lifetime reservation exist before this runs (P7-SCHED-08);
// the recipe (type, location, image, enroll URL, token file, pinned agent for the VM's arch) comes from
// the spend policy at call time; `provision --help` must offer --user-data-file (CI D26) or the call is
// E698; the pairing code reaches the VM only through the 0600 user-data file, which is removed on every
// exit; a failed create withdraws the offer; a created machine that answers without user data
// or without the lifecycle labels is destroyed at once, because it could never join or be found.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/burst"
	"github.com/nself-org/plugins/free/ci/internal/schedpolicy"
)

var (
	tokenShape = regexp.MustCompile(`^[0-9A-Za-z]{8,48}$`)
	labelShape = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)
)

// recipe is the cloud VM recipe read from the policy.
type recipe struct {
	typ, location, image, enrollURL, tokenFile string
	agent                                      schedpolicy.AgentBinary
}

// archOf is the CPU architecture of a Hetzner server type: the cax line is arm64.
func archOf(serverType string) string {
	if strings.HasPrefix(strings.ToLower(serverType), "cax") {
		return "arm64"
	}
	return "amd64"
}

func recipeOf(cv schedpolicy.CloudVM, spec burst.Spec) (recipe, error) {
	for _, f := range []struct{ key, val string }{{schedpolicy.KeyCloudType, cv.Type}, {schedpolicy.KeyCloudLocation, cv.Location}, {schedpolicy.KeyCloudImage, cv.Image},
		{schedpolicy.KeyCloudEnrollURL, cv.EnrollURL}, {schedpolicy.KeyCloudTokenFile, cv.TokenFile}} {
		if f.val == "" {
			return recipe{}, fail(ReasonRecipe, nil, "%s is not set", f.key)
		}
	}
	arch := archOf(cv.Type)
	if pf := spec.Platform; pf != "" && pf != "linux" && pf != "linux/"+arch {
		return recipe{}, fail(ReasonPlatform, nil, "platform %q does not fit server type %s (%s)", pf, cv.Type, arch)
	}
	bin, ok := cv.Agent[arch]
	if !ok || bin.URL == "" || bin.SHA256 == "" {
		return recipe{}, fail(ReasonAgent, nil, "no pinned agent url and sha256 for %s (burst.cloudvm.agent.%s)", arch, arch)
	}
	return recipe{typ: cv.Type, location: cv.Location, image: cv.Image, enrollURL: cv.EnrollURL, tokenFile: cv.TokenFile, agent: bin}, nil
}

// checkIntent refuses an intent whose labels are not the three lifecycle labels with usable values.
func checkIntent(in burst.Intent) error {
	if !tokenShape.MatchString(in.Token) || in.Labels[burst.LabelIntent] != in.Token || in.Labels[burst.LabelRole] != burst.RoleEphemeral || len(in.Labels) != 3 {
		return fail(ReasonRecipe, nil, "the intent must carry exactly the role, coordinator_id and intent labels")
	}
	for k, v := range in.Labels {
		if !labelShape.MatchString(v) || len(v) > burst.MaxLabelValue || k != burst.LabelRole && k != burst.LabelCoordinator && k != burst.LabelIntent {
			return fail(ReasonRecipe, nil, "label %s has a value the provider cannot take", k)
		}
	}
	return nil
}

// probeUserData asks the installed nself whether provision takes --user-data-file (cached per binary).
func (p *Provider) probeUserData(ctx context.Context) error {
	exe, err := p.nself()
	if err != nil {
		return err
	}
	p.mu.Lock()
	ok := p.probed[exe]
	p.mu.Unlock()
	if ok {
		return nil
	}
	out, code, rerr := p.cfg.Runner.Run(ctx, []string{exe, "infra", "server", "provision", "--help"}, nil, nil)
	if cerr := ctx.Err(); cerr != nil {
		return fail(ReasonInfra, errUnavailable, "the user-data probe ended with the caller's context")
	}
	if rerr != nil || code != 0 || !bytes.Contains(out, []byte("--user-data-file")) {
		return fail(ReasonNoUserData, nil, "`nself infra server provision --help` does not list --user-data-file; upgrade nself and the infra plugin")
	}
	p.mu.Lock()
	p.probed[exe] = true
	p.mu.Unlock()
	return nil
}

// writeUserData writes the rendered document into a fresh 0600 file in a 0700 directory.
func (p *Provider) writeUserData(doc []byte) (path string, cleanup func(), err error) {
	if p.cfg.Home == "" {
		return "", nil, fail(ReasonUserData, nil, "no CI home for the user-data directory")
	}
	dir := filepath.Join(p.cfg.Home, "burst-userdata")
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return "", nil, fail(ReasonUserData, nil, "the user-data directory cannot be made")
	}
	if err = os.Chmod(dir, 0o700); err != nil {
		return "", nil, fail(ReasonUserData, nil, "the user-data directory cannot be made private")
	}
	f, err := os.CreateTemp(dir, "ud-*")
	if err != nil {
		return "", nil, fail(ReasonUserData, nil, "the user-data file cannot be created")
	}
	cleanup = func() { _ = os.Remove(f.Name()) }
	if _, err = f.Write(doc); err == nil {
		err = f.Chmod(0o600)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		cleanup()
		return "", nil, fail(ReasonUserData, nil, "the user-data file cannot be written")
	}
	return f.Name(), cleanup, nil
}

// Provision implements burst.Provider.
func (p *Provider) Provision(ctx context.Context, in burst.Intent) (burst.Resource, error) {
	if err := ctx.Err(); err != nil {
		return burst.Resource{}, fail(ReasonInfra, errUnavailable, "the caller's context ended before the create")
	}
	if err := checkIntent(in); err != nil {
		return burst.Resource{}, err
	}
	pol, err := p.cfg.Spend(ctx)
	if err != nil {
		return burst.Resource{}, fail(ReasonRecipe, err, "the spend policy could not be read")
	}
	rc, err := recipeOf(pol.Burst.CloudVM, in.Spec)
	if err != nil {
		return burst.Resource{}, err
	}
	if err = p.probeUserData(ctx); err != nil {
		return burst.Resource{}, err
	}
	tok, err := readTokenFile(rc.tokenFile)
	if err != nil {
		return burst.Resource{}, err
	}
	found, err := p.list(ctx, tok, map[string]string{burst.LabelIntent: in.Token})
	if err != nil {
		return burst.Resource{}, err
	}
	for _, r := range found {
		if r.Labels[burst.LabelRole] == burst.RoleEphemeral && r.Labels[burst.LabelCoordinator] == in.Labels[burst.LabelCoordinator] {
			p.remember(r.ID, in.Token)
			return r, nil
		}
	}
	offer, err := p.offer(ctx, in.Token)
	if err != nil {
		return burst.Resource{}, err
	}
	res, err := p.create(ctx, tok, rc, in, offer.Code, offer.RootFingerprint)
	if err != nil {
		p.withdraw(ctx, in.Token)
		return burst.Resource{}, err
	}
	p.remember(res.ID, in.Token)
	return res, nil
}

func (p *Provider) remember(id, token string) {
	p.mu.Lock()
	p.byID[id] = token
	p.mu.Unlock()
}

// create renders the user data and runs the provision child.
func (p *Provider) create(ctx context.Context, tok string, rc recipe, in burst.Intent, code, rootFP string) (burst.Resource, error) {
	doc, err := Render(UserData{EnrollURL: rc.enrollURL, RootFingerprint: rootFP, AgentURL: rc.agent.URL, AgentSHA256: strings.ToLower(rc.agent.SHA256), PairingCode: code})
	if err != nil {
		return burst.Resource{}, err
	}
	path, cleanup, err := p.writeUserData(doc)
	if err != nil {
		return burst.Resource{}, err
	}
	defer cleanup()
	args := []string{"--name", NodeName(in.Token), "--type", rc.typ, "--location", rc.location, "--image", rc.image}
	keys := make([]string, 0, len(in.Labels))
	for k := range in.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "--label", k+"="+in.Labels[k])
	}
	args = append(args, "--user-data-file", path)
	data, err := p.run(ctx, tok, "provision", args...)
	if err != nil {
		return burst.Resource{}, err
	}
	var ans struct {
		Server   wireServer `json:"server"`
		UserData bool       `json:"user_data"`
	}
	if jerr := json.Unmarshal(data, &ans); jerr != nil || ans.Server.ID <= 0 {
		return burst.Resource{}, fail(ReasonAnswer, nil, "the provision answer has no server")
	}
	res := ans.Server.resource(p.cfg.Now())
	if !ans.UserData || !labelsCarry(res.Labels, in.Labels) {
		// Not joinable or not findable: delete it rather than let it bill until the sweep.
		derr := p.destroy(context.WithoutCancel(ctx), tok, res.ID)
		return burst.Resource{}, fail(ReasonAnswer, derr, "the created server %s carries no user data or lacks the lifecycle labels; it was deleted (%v)", res.ID, derr == nil)
	}
	return res, nil
}

func labelsCarry(got, want map[string]string) bool {
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

// destroy runs the destroy child for one server id; not found is success.
func (p *Provider) destroy(ctx context.Context, tok, id string) error {
	_, err := p.run(ctx, tok, "destroy", "--id", id, "--force-no-backup", "--release-ip")
	if err != nil && !errors.Is(err, errNotFound) {
		return err
	}
	return nil
}
