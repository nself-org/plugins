package secrets

// Purpose: parse a job secret reference (Epic D5 grammar) into class, environment and name.
// Inputs:  the ref strings of a job (`Job.Secrets`).
// Outputs: Ref values with the environment variable name each one is delivered as.
// Constraints: pure; no value is ever involved. A bad ref error echoes at most 64 printable
// bytes of the ref (a ref is a name, never a value) and never the store contents. Two refs that
// deliver to the same variable are E674.

import (
	"regexp"
	"strings"

	"github.com/nself-org/plugins/free/ci/internal/model"
)

var (
	namePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)
	envPattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)
)

// Ref is one parsed secret reference.
type Ref struct {
	Raw   string
	Class model.SecretClass
	// Env is the deployment environment of an environment/ or deploy/ ref, else "".
	Env string
	// Name is the secret name and the environment variable it is delivered as.
	Name string
}

// EnvName is the variable the value is delivered as.
func (r Ref) EnvName() string { return r.Name }

// StoreFile is the file argument `config secrets recipients --file` takes for the ref's class:
// project -> ci, environment/<env> -> ci-<env>, release -> release, deploy/<env> -> <env>.
func (r Ref) StoreFile() string { return storeFile(r.Class, r.Env) }

func storeFile(class model.SecretClass, env string) string {
	switch class {
	case "project":
		return "ci"
	case "environment":
		return "ci-" + env
	case "release":
		return "release"
	case "deploy":
		return env
	}
	return ""
}

// reservedEnv lists the environment names no ref may use (they would alias another store file).
func reservedEnv(env string) bool {
	return env == "ci" || env == "release" || env == "team" || strings.HasPrefix(env, "ci-")
}

// echo bounds how much of a bad ref an error repeats.
func echo(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= 64 {
			b.WriteString("...")
			break
		}
		if r < 0x20 || r > 0x7e {
			b.WriteByte('?')
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// Parse reads one ref.
func Parse(s string) (Ref, error) { return parseRef(s, false) }

// ParseSigner parses a reference for in-process signer use only. It permits reserved
// variable names; the returned ref must never be used for job delivery.
func ParseSigner(s string) (Ref, error) { return parseRef(s, true) }

func parseRef(s string, signer bool) (Ref, error) {
	parts := strings.Split(s, "/")
	shown := "<not echoed>" // a literal secret pasted into `secrets:` must not reach an error or a log
	switch parts[0] {
	case "project", "team", "release", "environment", "deploy":
		shown = parts[0] + "/..."
		if namePattern.MatchString(parts[len(parts)-1]) {
			shown = echo(s)
		}
	}
	bad := func(why string) (Ref, error) { return Ref{}, errRef("invalid secret ref %q: %s", shown, why) }
	r := Ref{Raw: s}
	switch parts[0] {
	case "project", "team", "release":
		if len(parts) != 2 {
			return bad("expected " + parts[0] + "/NAME")
		}
		r.Class, r.Name = model.SecretClass(parts[0]), parts[1]
	case "environment", "deploy":
		if len(parts) != 3 {
			return bad("expected " + parts[0] + "/<env>/NAME")
		}
		if !envPattern.MatchString(parts[1]) || reservedEnv(parts[1]) {
			return bad("invalid environment name")
		}
		r.Class, r.Env, r.Name = model.SecretClass(parts[0]), parts[1], parts[2]
	default:
		return bad("unknown class")
	}
	if !namePattern.MatchString(r.Name) {
		return bad("NAME must match ^[A-Z][A-Z0-9_]{0,127}$")
	}
	if !signer && reservedName(r.Name) {
		return bad("NAME is reserved: it would change how the process or the docker client runs")
	}
	return r, nil
}

// ParseAll parses every ref and rejects two refs that deliver the same variable.
func ParseAll(raw []string) ([]Ref, error) {
	out := make([]Ref, 0, len(raw))
	seen := map[string]string{}
	for _, s := range raw {
		r, err := Parse(s)
		if err != nil {
			return nil, err
		}
		if prev, dup := seen[r.Name]; dup {
			return nil, errRef("refs %q and %q both deliver %s", echo(prev), echo(s), r.Name)
		}
		seen[r.Name] = s
		out = append(out, r)
	}
	return out, nil
}

// Classes returns the distinct classes of refs in first-seen order.
func Classes(refs []Ref) []model.SecretClass {
	var out []model.SecretClass
	seen := map[model.SecretClass]bool{}
	for _, r := range refs {
		if !seen[r.Class] {
			seen[r.Class] = true
			out = append(out, r.Class)
		}
	}
	return out
}

// ParseClass reads the `--class` argument of `nodes secrets grant|revoke`: project, release,
// environment/<env> or deploy/<env>. `team` is node-local and never granted.
func ParseClass(spec string) (model.SecretClass, string, error) {
	bad := func(why string) (model.SecretClass, string, error) {
		return "", "", errRef("invalid class %q: %s", echo(spec), why)
	}
	class, env, hasEnv := strings.Cut(spec, "/")
	switch model.SecretClass(class) {
	case "project", "release":
		if hasEnv {
			return bad("no environment for this class")
		}
	case "environment", "deploy":
		if !hasEnv || !envPattern.MatchString(env) || reservedEnv(env) {
			return bad("expected " + class + "/<env>")
		}
	case "team":
		return bad("team secrets are node-local and are never granted")
	default:
		return bad("unknown class")
	}
	return model.SecretClass(class), env, nil
}

// StoreFileOf is the `--file` argument of `config secrets recipients` for a class and environment.
func StoreFileOf(class model.SecretClass, env string) string { return storeFile(class, env) }

// reservedName lists variables a secret may not be delivered as: container secrets travel in the docker
// CLI's own environment, and any process would honour these, so a ref must not be able to redirect them.
func reservedName(n string) bool {
	switch n {
	case "PATH", "HOME", "USER", "TMPDIR", "SHELL", "IFS", "ENV", "BASH_ENV", "PWD", "OLDPWD", "XDG_RUNTIME_DIR", "SSH_AUTH_SOCK", "SECRETS_AGE_KEY_PATH", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
		return true
	}
	for _, p := range []string{"DOCKER_", "BUILDKIT_", "LD_", "DYLD_", "NSELF_CI_", "GIT_", "BASH_FUNC_", "SSL_CERT"} {
		if strings.HasPrefix(n, p) {
			return true
		}
	}
	return false
}
