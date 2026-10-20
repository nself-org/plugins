package cloudvm

// Purpose: the cloud-init user data of one burst VM (Epic D13, review F9): minimal and hardened.
// Inputs:  UserData, the closed list of values the VM needs: the enroll URL, the root
//          fingerprint, the agent URL and its sha256, and the pairing code.
// Outputs: a #cloud-config document that turns password logins and root login off, masks sshd, writes
//          the pairing code to a 0600 file under /run, downloads the pinned agent, checks its sha256,
//          joins with the code read from that file (never argv, env or a log), deletes the file and runs `pull`.
// Constraints: the field list is closed (Fields); every value is checked against a narrow shape
//          before it is rendered and then escaped for its place (a YAML double-quoted scalar, or a
//          single-quoted shell word), so a value can never add a line, a key or a command; the
//          agent URL has no userinfo, query or fragment; the render fails closed (E698 user_data)
//          and the document is under the provider's 32 KiB limit.

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
	"text/template"

	"github.com/nself-org/plugins/free/ci/internal/pairing"
)

// maxUserDataBytes is the provider limit (Hetzner OpenAPI: user_data is at most 32 KiB).
const maxUserDataBytes = 32768

// Fields is the closed list of values the user data may carry.
var Fields = []string{"enroll_url", "root_fingerprint", "agent_url", "agent_sha256", "pairing_code"}

// UserData holds the values of the closed list.
type UserData struct {
	EnrollURL, RootFingerprint, AgentURL, AgentSHA256, PairingCode string
}

var (
	fingerprintRe = regexp.MustCompile(`^SHA256:[A-Za-z0-9+/]{43}$`)
	sha256Re      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	urlTextRe     = regexp.MustCompile(`^[A-Za-z0-9._~:/%@!$&*+,;=-]{1,512}$`)
)

// httpsURL checks a plain https URL without userinfo, query or fragment.
func httpsURL(raw string) bool {
	if !urlTextRe.MatchString(raw) || strings.ContainsAny(raw, "?#@'\"\\ ") {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && !u.ForceQuery
}

// validate refuses a value outside its shape.
func (d UserData) validate() error {
	switch {
	case !httpsURL(d.EnrollURL):
		return fail(ReasonUserData, nil, "enroll_url is not a plain https URL")
	case !fingerprintRe.MatchString(d.RootFingerprint):
		return fail(ReasonUserData, nil, "root_fingerprint is not a SHA256 fingerprint")
	case !httpsURL(d.AgentURL):
		return fail(ReasonUserData, nil, "agent_url is not a plain https URL")
	case !sha256Re.MatchString(d.AgentSHA256):
		return fail(ReasonUserData, nil, "agent_sha256 is not 64 lower-case hex digits")
	case !pairing.ValidShape(d.PairingCode):
		return fail(ReasonUserData, nil, "pairing_code has the wrong shape")
	}
	return nil
}

// yamlQ renders s as a YAML double-quoted scalar (a JSON string is one).
func yamlQ(s string) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// shQ renders s as one single-quoted shell word.
func shQ(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// script is the join script written to /run/nself-ci/join.sh. Flags come before the URL: the agent
// parses flags first. The code file is removed on every exit.
const scriptText = `#!/bin/sh
set -eu
umask 077
export HOME="${HOME:-/root}"
export NSELF_CI_AGENT_HOME=/var/lib/nself-ci-agent
mkdir -p "$NSELF_CI_AGENT_HOME"
cd /run/nself-ci
trap 'rm -f /run/nself-ci/code' EXIT
curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' --tlsv1.2 --output nself-ci-agent {{sh .AgentURL}}
printf '%s  %s\n' {{sh .AgentSHA256}} nself-ci-agent | sha256sum -c -
install -m 0755 nself-ci-agent /usr/local/bin/nself-ci-agent
/usr/local/bin/nself-ci-agent join --expect-fingerprint {{sh .RootFingerprint}} {{sh .EnrollURL}} < /run/nself-ci/code
rm -f /run/nself-ci/code
exec /usr/local/bin/nself-ci-agent pull --slots 1
`

// docText is the cloud-config document; the script and the code file are written by write_files.
const docText = `#cloud-config
ssh_pwauth: false
disable_root: true
write_files:
  - path: /run/nself-ci/code
    permissions: "0600"
    owner: root:root
    content: {{yaml .PairingCode}}
  - path: /run/nself-ci/join.sh
    permissions: "0700"
    owner: root:root
    content: {{yaml .Script}}
runcmd:
  - [systemctl, mask, --now, ssh.service, ssh.socket, sshd.service]
  - [/bin/sh, /run/nself-ci/join.sh]
`

var (
	scriptTmpl = template.Must(template.New("script").Funcs(template.FuncMap{"sh": shQ}).Option("missingkey=error").Parse(scriptText))
	docTmpl    = template.Must(template.New("doc").Funcs(template.FuncMap{"yaml": yamlQ}).Option("missingkey=error").Parse(docText))
)

// Render validates d and renders the user data.
func Render(d UserData) ([]byte, error) {
	if err := d.validate(); err != nil {
		return nil, err
	}
	var script bytes.Buffer
	if err := scriptTmpl.Execute(&script, d); err != nil {
		return nil, fail(ReasonUserData, err, "the join script cannot be rendered")
	}
	var doc bytes.Buffer
	if err := docTmpl.Execute(&doc, struct{ PairingCode, Script string }{d.PairingCode + "\n", script.String()}); err != nil {
		return nil, fail(ReasonUserData, err, "the cloud-config cannot be rendered")
	}
	if doc.Len() > maxUserDataBytes {
		return nil, fail(ReasonUserData, nil, "the user data is over the provider limit")
	}
	return doc.Bytes(), nil
}
