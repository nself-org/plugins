package enroll

// Purpose: the joining node's side of enrollment (Epic D5, review F3): learn the root from the served
//          chain, get it confirmed by the operator, check the clock, send CSR + node key + code over a
//          connection verified against that root, and validate what comes back.
// Inputs:  JoinOptions: the enroll URL, how the operator confirms, how the code is read, the node key.
// Outputs: JoinResult (certificate, chain, root, the new TLS client key, agent URL, fingerprints).
// Constraints: nothing secret leaves the node before the root fingerprint is confirmed (an interactive
//          yes or an exact --expect-fingerprint; no terminal and no flag is E660 fingerprint_unconfirmed);
//          the code is requested only after confirmation and is never printed or put on argv; the node
//          prints its CSR key fingerprint and node-key id BEFORE the enroll request so the admin can
//          compare; a clock more than 120 s off is refused before any CSR is sent; the issued
//          certificate must carry our CSR key, chain to the confirmed root and name us as a node.

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/pki"
)

// JoinOptions configure Join.
type JoinOptions struct {
	URL string
	// ExpectFingerprint, when set, must equal the served root fingerprint.
	ExpectFingerprint string
	// Confirm asks the operator to confirm the fingerprint; nil means no terminal (a refusal without ExpectFingerprint).
	Confirm func(rootFingerprint string) (bool, error)
	// Code returns the pairing code (called only after the fingerprint is confirmed).
	Code                                    func() (string, error)
	Name, OS, Arch, MachineID, AgentVersion string
	// NodeKey is the raw Ed25519 public key; NodeKeyID its id; NodeSign signs with the private half.
	NodeKey   []byte
	NodeKeyID string
	NodeSign  func(msg []byte) ([]byte, error)
	// Out receives the fingerprints the admin compares.
	Out io.Writer
	Now func() time.Time
	// Sleep waits between the two requests (the listener allows one per second); default time.Sleep.
	Sleep func(time.Duration)
}

// JoinResult is a completed enrollment.
type JoinResult struct {
	NodeID                          string
	CertDER                         []byte
	Chain                           [][]byte
	RootDER                         []byte
	KeyDER                          []byte // PKCS8 of the new TLS client key: seal it before it touches disk
	AgentURL                        string
	NotAfter                        time.Time
	RootFingerprint, CSRFingerprint string
}

// RemoteError is an error reply of the listener.
type RemoteError struct{ Code, Reason, Msg string }

func (e *RemoteError) Error() string     { return fmt.Sprintf("%s: %s (%s)", e.Code, e.Msg, e.Reason) }
func (e *RemoteError) ErrCode() string   { return e.Code }
func (e *RemoteError) ErrReason() string { return e.Reason }

func refuse(reason, format string, a ...any) error {
	return &pki.Error{Reason: reason, Msg: fmt.Sprintf(format, a...)}
}

// servedRoot connects without verification, reads the chain the server presents and returns the
// self-signed root at its end. Nothing is sent over this connection.
func servedRoot(ctx context.Context, host string) (*x509.Certificate, error) {
	var peers []*x509.Certificate
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true, VerifyConnection: func(cs tls.ConnectionState) error {
		peers = cs.PeerCertificates
		return nil
	}}
	d := tls.Dialer{Config: cfg, NetDialer: &net.Dialer{Timeout: 10 * time.Second}}
	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return nil, err
	}
	_ = conn.Close()
	if len(peers) == 0 {
		return nil, refuse(pki.ReasonBadChain, "the server presented no certificate")
	}
	root := peers[len(peers)-1]
	if !bytes.Equal(root.RawIssuer, root.RawSubject) || root.CheckSignatureFrom(root) != nil {
		return nil, refuse(pki.ReasonBadChain, "the server did not present a root certificate")
	}
	return root, nil
}

func post(ctx context.Context, c *http.Client, u string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		var eb ErrorBody
		if json.Unmarshal(raw, &eb) != nil || eb.Code == "" {
			return fmt.Errorf("enrollment listener answered HTTP %d", resp.StatusCode)
		}
		return &RemoteError{Code: eb.Code, Reason: eb.Reason, Msg: eb.Message}
	}
	return json.Unmarshal(raw, out)
}

// Join performs the enrollment.
func Join(ctx context.Context, o JoinOptions) (JoinResult, error) {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Sleep == nil {
		o.Sleep = time.Sleep
	}
	u, err := url.Parse(o.URL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" {
		return JoinResult{}, fmt.Errorf("enroll URL must be https://host:port/v1/enroll")
	}
	if u.Port() == "" {
		u.Host = net.JoinHostPort(u.Hostname(), "3866")
	}
	if u.Path == "" || u.Path == "/" {
		u.Path = Route
	}
	root, err := servedRoot(ctx, u.Host)
	if err != nil {
		return JoinResult{}, err
	}
	fp := pki.CertFingerprint(root.Raw)
	if err = confirm(o, fp); err != nil {
		return JoinResult{}, err
	}
	cfg, err := pki.TLSClientConfig(pki.ClientOptions{RootDER: root.Raw, ServerName: u.Hostname(), Now: o.Now})
	if err != nil {
		return JoinResult{}, err
	}
	hc := &http.Client{Timeout: 12 * time.Minute, Transport: &http.Transport{Proxy: nil, TLSClientConfig: cfg, DisableKeepAlives: true},
		// A redirect would re-send the code and CSR to wherever it points (even http://): refuse to follow.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var hello Hello
	if err = post(ctx, hc, u.String(), Request{Phase: PhaseHello, Version: Version, ClientTime: o.Now().Unix()}, &hello); err != nil {
		var invalid x509.CertificateInvalidError
		if errors.As(err, &invalid) && invalid.Reason == x509.Expired { // a fresh server leaf is never expired: this clock is far off
			return JoinResult{}, refuse(pki.ReasonClockSkew, "the coordinator's certificate is not valid at this node's time; the clocks differ by more than %s", pki.ClockSkew)
		}
		return JoinResult{}, err
	}
	if d := o.Now().Sub(time.Unix(hello.ServerTime, 0)); d > pki.ClockSkew || -d > pki.ClockSkew {
		return JoinResult{}, refuse(pki.ReasonClockSkew, "this clock differs from the coordinator's by more than %s", pki.ClockSkew)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return JoinResult{}, err
	}
	csr, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{Subject: pkix.Name{CommonName: o.Name}}, key)
	if err != nil {
		return JoinResult{}, err
	}
	csrFP, err := pki.CSRKeyFingerprint(csr)
	if err != nil {
		return JoinResult{}, err
	}
	fmt.Fprintf(o.Out, "CSR key fingerprint: %s\nnode key id:         %s\nnode key sha256:     %s\nThe admin compares the CSR key fingerprint before approving; `nodes rotate --node-key --key-id` takes the sha256.\n", csrFP, o.NodeKeyID, pki.NodeKeyDigest(o.NodeKey))
	code, err := o.Code()
	if err != nil {
		return JoinResult{}, err
	}
	o.Sleep(Interval + 100*time.Millisecond)
	now := o.Now().Unix()
	sig, err := o.NodeSign(Transcript(hello.CoordinatorID, fp, o.Name, now, csrFP, o.NodeKeyID))
	if err != nil {
		return JoinResult{}, err
	}
	var resp Response
	req := Request{Phase: PhaseEnroll, Version: Version, ClientTime: now, Code: code, CSR: csr, NodeKey: o.NodeKey, NodeKeySig: sig, Name: o.Name, OS: o.OS, Arch: o.Arch, MachineID: o.MachineID, AgentVersion: o.AgentVersion}
	if err = post(ctx, hc, u.String(), req, &resp); err != nil {
		return JoinResult{}, err
	}
	return finish(o, resp, root, key, fp, csrFP)
}

func confirm(o JoinOptions, fp string) error {
	switch {
	case o.ExpectFingerprint != "":
		if !pki.Equal(o.ExpectFingerprint, fp) {
			return refuse(pki.ReasonFingerprintMismatch, "the served root fingerprint %s is not the expected one; nothing was sent", fp)
		}
	case o.Confirm == nil:
		return refuse(pki.ReasonFingerprintUnconfirmed, "no terminal and no --expect-fingerprint: root fingerprint %s was not confirmed", fp)
	default:
		if ok, err := o.Confirm(fp); err != nil || !ok {
			return refuse(pki.ReasonFingerprintUnconfirmed, "root fingerprint %s was not confirmed", fp)
		}
	}
	return nil
}

func finish(o JoinOptions, resp Response, root *x509.Certificate, key *ecdsa.PrivateKey, fp, csrFP string) (JoinResult, error) {
	leaf, err := x509.ParseCertificate(resp.Cert)
	if err != nil || !bytes.Equal(resp.Root, root.Raw) {
		return JoinResult{}, refuse(pki.ReasonBadChain, "the coordinator's answer does not match the confirmed root")
	}
	pool, inter := x509.NewCertPool(), x509.NewCertPool()
	pool.AddCert(root)
	for _, der := range resp.Chain[min(1, len(resp.Chain)):] {
		if c, perr := x509.ParseCertificate(der); perr == nil {
			inter.AddCert(c)
		}
	}
	_, verr := leaf.Verify(x509.VerifyOptions{Roots: pool, Intermediates: inter, CurrentTime: o.Now(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}})
	id, kind := pki.IdentityOf(leaf)
	pub, _ := leaf.PublicKey.(*ecdsa.PublicKey)
	if verr != nil || kind != pki.KindNode || id != strings.ToLower(o.Name) || pub == nil || !pub.Equal(&key.PublicKey) {
		return JoinResult{}, refuse(pki.ReasonBadChain, "the issued certificate is not ours or does not chain to the confirmed root")
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return JoinResult{}, err
	}
	return JoinResult{NodeID: id, CertDER: resp.Cert, Chain: resp.Chain, RootDER: resp.Root, KeyDER: keyDER, AgentURL: resp.AgentURL, NotAfter: leaf.NotAfter, RootFingerprint: fp, CSRFingerprint: csrFP}, nil
}
