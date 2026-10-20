package checks

// Purpose: the optional GitHub App adapter (Ticket P7-AUTO-22, Epic AUTO D7). After the commit
// status was delivered, it creates (or updates) ONE check run named `nself-ci/gates` per
// (revision, run) with the gate table as output text, failing-check annotations (50 per
// request, paged) and the conclusion the status carries.
// Inputs:  the run id, the posted status and the published evidence (Request); a Config and the
// App's PEM; a ClientFactory that builds REST-funnel clients.
// Outputs: a check run on the commit. Nothing else: no status, no ledger read or write.
// Constraints: the check name is never `nself-ci` (that is the Actions app's required check, CI
// D18); statuses remain the ledger and the result of record, so an error here is a note and never
// changes a verdict; the PEM, the JWT and the installation token are never logged or stored (they
// are registered with the redactor, which masks every text and error that leaves); permissions
// are limited to checks:write, statuses:write, contents:read, metadata:read. An existing check
// run of the same (revision, run) is updated, so a repeated publication never duplicates it.

import (
	"context"
	"crypto/rsa"
	"net/url"
	"sync"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/model"
	"github.com/nself-org/plugins/free/ci/internal/redact"
	"github.com/nself-org/plugins/free/ci/internal/report"
)

// CheckName is the name of the check run. It must never equal report.Context.
const CheckName = "nself-ci/gates"

// Request is what the run hands over after a status was delivered (mirrors run.PublishDetailRequest;
// the run package converts, so this package never imports it).
type Request struct {
	Owner, Repo, RunID, Revision string
	Status                       report.Status
	Evidence                     model.Evidence
	Projection                   model.ProjectionResult
}

// Publisher publishes check runs for one App installation.
type Publisher struct {
	Client ClientFactory
	Now    func() time.Time
	Red    *redact.Redactor

	app, installation string
	key               *rsa.PrivateKey
	mu                sync.Mutex
	sources           map[string]*TokenSource // one installation token per repository
}

// tokens returns the token source of repo, which caches its token until 60 s before expiry.
func (p *Publisher) tokens(repo string) *TokenSource {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.sources[repo] == nil {
		p.sources[repo] = &TokenSource{AppID: p.app, InstallationID: p.installation, Repo: repo, Key: p.key, Client: p.Client, Now: p.Now, Red: p.Red}
	}
	return p.sources[repo]
}

// New builds a Publisher from the configuration and the PEM. The PEM is parsed once and then
// held only as the parsed key; its text is registered with the redactor.
func New(cfg Config, pem []byte, client ClientFactory, now func() time.Time) (*Publisher, error) {
	red := redact.New()
	registerValue(red, pem)
	key, err := ParseKey(pem)
	if err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	return &Publisher{Client: client, Now: now, Red: red, app: cfg.AppID, installation: cfg.InstallationID, key: key, sources: map[string]*TokenSource{}}, nil
}

type output struct {
	Title       string       `json:"title"`
	Summary     string       `json:"summary"`
	Text        string       `json:"text,omitempty"`
	Annotations []Annotation `json:"annotations,omitempty"`
}

type checkBody struct {
	Name        string  `json:"name,omitempty"`
	HeadSHA     string  `json:"head_sha,omitempty"`
	ExternalID  string  `json:"external_id,omitempty"`
	Status      string  `json:"status,omitempty"`
	Conclusion  string  `json:"conclusion,omitempty"`
	CompletedAt string  `json:"completed_at,omitempty"`
	Output      *output `json:"output,omitempty"`
}

type checkRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	ExternalID string `json:"external_id"`
	Output     struct {
		AnnotationsCount int `json:"annotations_count"`
	} `json:"output"`
}

type checkList struct {
	CheckRuns []checkRun `json:"check_runs"`
}

const (
	pathList   = "/repos/{owner}/{repo}/commits/{ref}/check-runs"
	pathCreate = "/repos/{owner}/{repo}/check-runs"
	pathUpdate = "/repos/{owner}/{repo}/check-runs/{id}"
)

// Detail publishes the check run for r. A pending status has no conclusion yet and publishes nothing.
func (p *Publisher) Detail(ctx context.Context, r Request) error {
	if err := r.Status.Validate(); err != nil || r.RunID == "" || r.Revision != r.Status.Revision {
		return coded("the check run needs a valid status, a run id and the status revision")
	}
	conclusion := ConclusionFor(r.Status.State, r.Evidence)
	if conclusion == "" {
		return nil
	}
	// the token first: once it is known to the redactor, every text rendered below is masked against it
	tokens := p.tokens(r.Repo)
	if _, err := tokens.Token(ctx); err != nil {
		return err
	}
	client, err := p.Client(tokens.Auth())
	if err != nil {
		return coded("the App client could not be built")
	}
	if err := p.publish(ctx, client, r, conclusion); err != nil {
		if e, ok := err.(*Error); ok {
			return e
		}
		return coded("the check run was not published: " + safeText(p.Red, err.Error()))
	}
	return nil
}

func (p *Publisher) publish(ctx context.Context, c API, r Request, conclusion Conclusion) error {
	params := map[string]string{"owner": r.Owner, "repo": r.Repo}
	out := output{Title: truncRunes(maskString(p.Red, Title(r.Projection)), 255), Summary: maskString(p.Red, cell(r.Projection.Summary)), Text: capText(maskString(p.Red, Table(r.Evidence, r.Projection)))}
	all := annotationsMasked(p.Red, r.Evidence)

	existing, err := p.find(ctx, c, r)
	if err != nil {
		return err
	}
	body := checkBody{Status: "completed", Conclusion: string(conclusion), CompletedAt: p.Now().UTC().Format(time.RFC3339), Output: &out}
	if existing != nil { // a repeat sends only what the check run does not hold yet, so a failed page is completed on retry and nothing is doubled
		all = all[min(existing.Output.AnnotationsCount, len(all)):]
	}
	pages := Pages(all)
	if len(pages) > 0 {
		out.Annotations, pages = pages[0], pages[1:]
	}
	var id int64
	if existing == nil {
		body.Name, body.HeadSHA, body.ExternalID = CheckName, r.Revision, r.RunID
		var made checkRun
		if _, err := c.DoQuery(ctx, "POST", pathCreate, params, nil, body, &made); err != nil {
			return coded("the check run was not created: " + safeText(p.Red, err.Error()))
		}
		id = made.ID
	} else {
		id = existing.ID
		params["id"] = itoa(id)
		if _, err := c.DoQuery(ctx, "PATCH", pathUpdate, params, nil, body, nil); err != nil {
			return coded("the check run was not updated: " + safeText(p.Red, err.Error()))
		}
	}
	if len(pages) > 0 && id == 0 {
		return coded("the check run id is missing; annotations were not sent")
	}
	for i, page := range pages { // later pages: GitHub appends annotations on update
		params["id"] = itoa(id)
		more := checkBody{Output: &output{Title: out.Title, Summary: out.Summary, Text: out.Text, Annotations: page}} // text again: an update may replace it
		if _, err := c.DoQuery(ctx, "PATCH", pathUpdate, params, nil, more, nil); err != nil {
			return coded("annotation page " + itoa(int64(i+2)) + " was not sent: " + safeText(p.Red, err.Error()))
		}
	}
	return nil
}

// find returns the check run of this (revision, run), or nil.
func (p *Publisher) find(ctx context.Context, c API, r Request) (*checkRun, error) {
	var list checkList
	q := url.Values{"check_name": {CheckName}, "filter": {"all"}, "per_page": {"100"}}
	params := map[string]string{"owner": r.Owner, "repo": r.Repo, "ref": r.Revision}
	if _, err := c.DoQuery(ctx, "GET", pathList, params, q, nil, &list); err != nil {
		return nil, coded("existing check runs were not read: " + safeText(p.Red, err.Error()))
	}
	for i := range list.CheckRuns {
		if x := list.CheckRuns[i]; x.Name == CheckName && x.ExternalID == r.RunID {
			return &x, nil
		}
	}
	return nil, nil
}

func annotationsMasked(red *redact.Redactor, ev model.Evidence) []Annotation {
	a := Annotations(ev)
	for i := range a {
		a[i].Message = maskString(red, a[i].Message)
		a[i].Title = truncRunes(maskString(red, a[i].Title), 255)
		a[i].Path = maskString(red, a[i].Path)
	}
	return a
}
