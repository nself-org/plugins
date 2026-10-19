package verify

// Purpose: load the ci-release trust roots (Epic CACHE D16, TRUST D11): the
// operator-scope files and the same two files read from the protected default
// branch at the pinned tip (`git show <pin>:.nself/trust/ci-release.keys`).
// Inputs:  Options.
// Outputs: a Verifier of purpose ci-release over the union of the keys, minus
// every revoked id.
// Constraints: the working tree and the released revision are never trusted: a
// working-tree copy that differs from the pinned one only produces a warning.
// A key must be listed by the operator or by the pinned tip; a key added only
// in the released revision is not a trust root. A trust file that does not
// parse fails the verification (fail closed). Git runs through the exec funnel.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/nself-org/cli/sdk/go/v2/signing"

	"github.com/nself-org/plugins/free/ci/internal/evidence"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

const (
	keysPath    = ".nself/trust/ci-release.keys"
	revokedPath = ".nself/trust/ci-release.revoked"
	maxTrust    = 1 << 20
)

var commitRe = regexp.MustCompile(`^([0-9a-f]{40}|[0-9a-f]{64})$`)

func (o Options) capture() evidence.Capture {
	if o.Capture != nil {
		return o.Capture
	}
	return report.Capture
}

// pinned reads path at the pinned commit: bytes and true, or false when the
// pinned tip has no such file.
func (o Options) pinned(ctx context.Context, path string) ([]byte, bool, error) {
	if o.Pin == "" {
		return nil, false, nil
	}
	if !commitRe.MatchString(o.Pin) || o.Root == "" {
		return nil, false, errors.New("the pinned default-branch commit is not a full commit id, or there is no repository")
	}
	run := o.capture()
	spec := o.Pin + ":" + path
	if _, exit, err := run(ctx, "git", "-c", "core.fsmonitor=false", "-C", o.Root, "cat-file", "-e", o.Pin+"^{commit}"); err != nil || exit != 0 {
		return nil, false, fmt.Errorf("the pinned default-branch commit %.12s is not in the repository", o.Pin)
	}
	out, exit, err := run(ctx, "git", "-c", "core.fsmonitor=false", "-C", o.Root, "cat-file", "-s", spec)
	if err != nil || exit != 0 {
		return nil, false, nil // the commit exists, so the file is absent there: no keys from it
	}
	if n, perr := strconv.Atoi(strings.TrimSpace(out)); perr != nil || n > maxTrust {
		return nil, false, fmt.Errorf("%s at the pinned tip is not a trust file of at most %d bytes", path, maxTrust)
	}
	body, exit, err := run(ctx, "git", "-c", "core.fsmonitor=false", "-C", o.Root, "show", spec)
	if err != nil || exit != 0 {
		return nil, false, fmt.Errorf("cannot read %s at the pinned tip", path)
	}
	return []byte(body), true, nil
}

// operatorFile reads an operator-scope file; a missing one is empty.
func operatorFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err == nil && len(b) > maxTrust {
		err = fmt.Errorf("%s is larger than %d bytes", path, maxTrust)
	}
	return b, err
}

// trustVerifier builds the ci-release verifier and records warnings.
func (r *run) trustVerifier() (*signing.Verifier, error) {
	o := r.opt
	if o.PinError != "" {
		return nil, fmt.Errorf("the protected default branch could not be read: %s", o.PinError)
	}
	var keys []signing.Key
	var revoked []string
	seen := map[string]bool{}
	parseKeys := func(data []byte) error {
		ks, err := signing.ParseKeysFile(bytes.NewReader(data), signing.PurposeCIRelease)
		for _, k := range ks {
			if !seen[k.ID] {
				seen[k.ID] = true
				keys = append(keys, k)
			}
		}
		return err
	}
	parseRevoked := func(data []byte) error {
		ids, err := signing.ParseRevokedFile(bytes.NewReader(data))
		revoked = append(revoked, ids...)
		return err
	}
	sources := []struct {
		label string
		get   func() ([]byte, error)
		parse func([]byte) error
	}{
		{"operator keys", func() ([]byte, error) { return operatorFile(o.OperatorKeys) }, parseKeys},
		{"operator revoked", func() ([]byte, error) { return operatorFile(o.OperatorRevoked) }, parseRevoked},
		{"protected-branch keys", func() ([]byte, error) { b, _, err := o.pinned(r.ctx, keysPath); return b, err }, parseKeys},
		{"protected-branch revoked", func() ([]byte, error) { b, _, err := o.pinned(r.ctx, revokedPath); return b, err }, parseRevoked},
	}
	for _, s := range sources {
		data, err := s.get()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.label, err)
		}
		if len(data) == 0 {
			continue
		}
		if err = s.parse(data); err != nil {
			return nil, fmt.Errorf("%s: %w", s.label, err)
		}
	}
	r.warnWorkingTree(keysPath)
	r.warnWorkingTree(revokedPath)
	var opts []signing.Option
	if o.Revoked != nil {
		opts = append(opts, signing.WithRevoked(o.Revoked))
	}
	return signing.NewVerifier(signing.PurposeCIRelease, keys, revoked, opts...)
}

// warnWorkingTree notes a working-tree trust file that differs from the pinned one.
func (r *run) warnWorkingTree(path string) {
	if r.opt.Root == "" {
		return
	}
	local, err := operatorFile(filepath.Join(r.opt.Root, filepath.FromSlash(path)))
	if err != nil || local == nil {
		return
	}
	pinned, _, _ := r.opt.pinned(r.ctx, path)
	if !bytes.Equal(local, pinned) {
		r.res.Warnings = append(r.res.Warnings, "the working tree's "+path+" differs from the protected default branch at the pinned tip and is ignored")
	}
}
