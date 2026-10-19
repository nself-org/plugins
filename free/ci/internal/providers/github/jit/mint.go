package jit

// Purpose: Mint, the generate-jitconfig call (Epic D6, review F5).
// Inputs:  a providers.RunnerRequest (its Scope.Repo is required, "owner/name"); at repo scope the
//          runner group is always 1, at org scope it is the one the operator mapped to the attempt's
//          trust tier (a GroupID that disagrees with the tier is refused).
// Outputs: the encoded JIT config as a providers.RunnerCredential (Kind github-jit-config, expiry
//          now+60 min). MintRunner also returns the runner id and name Deregister needs.
// Constraints: the name and the unique label are one fresh ULID per call, never reused; the labels
//          are the fixed set self-hosted, the operator's label, linux, the arch label and the unique
//          label. Nothing the caller passes is copied except an arch label (x64 or arm64), so a
//          demand for ubuntu-latest registers an ordinary nSelf runner and cannot pull hosted jobs
//          (E708 label.not_allowed is the demand handler's job, HOST-19). The config never reaches
//          a log, an error or the store; Minted prints without it.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
)

// Minted is one registration: the credential plus what teardown needs.
type Minted struct {
	RunnerID   string
	Name       string
	Labels     []string
	GroupID    int64
	Scope      providers.RunnerScope
	Credential providers.RunnerCredential
}

// Ref is the handle to pass to Deregister.
func (m Minted) Ref() providers.RunnerRef { return providers.RunnerRef{ID: m.RunnerID, Scope: m.Scope} }

// String, GoString, LogValue and MarshalJSON leave the credential out wherever a Minted is printed.
func (m Minted) String() string {
	return fmt.Sprintf("jit runner %s (id %s, group %d)", m.Name, m.RunnerID, m.GroupID)
}
func (m Minted) GoString() string { return m.String() }
func (m Minted) LogValue() slog.Value {
	return slog.GroupValue(slog.String("name", m.Name), slog.String("id", m.RunnerID), slog.Int64("group", m.GroupID))
}
func (m Minted) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		RunnerID string   `json:"runner_id"`
		Name     string   `json:"name"`
		Labels   []string `json:"labels"`
		GroupID  int64    `json:"group_id"`
	}{m.RunnerID, m.Name, m.Labels, m.GroupID})
}

// archLabel picks the one caller label that survives: x64 or arm64 (default X64).
func archLabel(asked []string) string {
	for _, l := range asked {
		switch strings.ToLower(l) {
		case "x64":
			return "X64"
		case "arm64":
			return "ARM64"
		}
	}
	return "X64"
}

// groupFor chooses the runner group. Repo scope: the default group. Org scope: the group the operator
// mapped to the attempt's trust tier. An explicit GroupID is only a cross-check: it must equal that
// tier's group, so a caller cannot move an untrusted attempt into a more trusted tier's group.
func (r *Registrar) groupFor(req providers.RunnerRequest) (int64, error) {
	if r.set.Scope == ScopeRepo {
		return DefaultGroup, nil
	}
	id, ok := r.set.Groups[req.Attempt.Trust]
	if !ok {
		return 0, providers.NewError(providers.Config, "E700", "jit.groups.tier", 0, time.Time{})
	}
	if req.GroupID > 0 && req.GroupID != id {
		return 0, providers.NewError(providers.Config, "E700", "jit.group.not_tier", 0, time.Time{})
	}
	return id, nil
}

type mintBody struct {
	Name       string   `json:"name"`
	GroupID    int64    `json:"runner_group_id"`
	Labels     []string `json:"labels"`
	WorkFolder string   `json:"work_folder"`
}

type mintReply struct {
	Runner struct {
		ID int64 `json:"id"`
	} `json:"runner"`
	Config string `json:"encoded_jit_config"`
}

// MintRunner registers one single-job runner and returns the credential with its teardown handle.
func (r *Registrar) MintRunner(ctx context.Context, req providers.RunnerRequest) (Minted, error) {
	t, err := r.resolve(req.Scope)
	if err != nil {
		return Minted{}, err
	}
	group, err := r.groupFor(req)
	if err != nil {
		return Minted{}, err
	}
	now := r.now()
	id, err := newULID(now, r.entropy)
	if err != nil {
		return Minted{}, providers.NewError(providers.Config, "E707", "mint.entropy", 0, time.Time{})
	}
	name := RunnerPrefix + id
	labels := []string{"self-hosted", r.set.Label, "linux", archLabel(req.Labels), name}
	tmpl, params := r.runnersPath(t)
	var out mintReply
	body := mintBody{Name: name, GroupID: group, Labels: labels, WorkFolder: workFolderName}
	if _, err := r.c.Do(ctx, "POST", tmpl+"/generate-jitconfig", params, body, &out); err != nil {
		return Minted{}, runnerErr(err, "mint")
	}
	if out.Config == "" || out.Runner.ID <= 0 {
		return Minted{}, providers.NewError(providers.Config, "E707", "mint.incomplete", 0, time.Time{})
	}
	return Minted{
		RunnerID: strconv.FormatInt(out.Runner.ID, 10), Name: name, Labels: labels, GroupID: group, Scope: req.Scope,
		Credential: providers.RunnerCredential{Kind: idCredential, Value: out.Config, ExpiresAt: now.Add(ConfigLifetime)},
	}, nil
}

// Mint implements providers.RunnerRegistrar.
func (r *Registrar) Mint(ctx context.Context, req providers.RunnerRequest) (providers.RunnerCredential, error) {
	m, err := r.MintRunner(ctx, req)
	return m.Credential, err
}
