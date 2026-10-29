package usage

// Purpose: Importer, the off-by-default step that turns a UsageSource's lines into SCHED-07 ledger
// rows (Epic D12).
// Inputs:  Settings (billing_import, billing_account), a providers.UsageSource, a replacer (default
//          economics.ReplaceUsage) and a clock.
// Outputs: Result; one economics.Usage per (account, date, sku, repo) with the key
//          github:<account>:<date>:<sku>:<repo>; the period is written as a whole, in one transaction
//          (ReplaceUsage), so a second import replaces the same rows and a row GitHub no longer reports
//          (renamed repo or sku, changed billing account) is deleted instead of counted twice.
// Constraints: with billing_import false Run returns Disabled before the source is touched (no API
//          call, no credential read). The whole period is read and converted before the first write,
//          so a failing or malformed report writes nothing, and the ledger write is atomic: a failure
//          leaves the earlier rows and a rerun converges. An empty report writes and deletes nothing.
//          No remaining value is computed or reported here.

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/economics"
	"github.com/nself-org/plugins/free/ci/internal/policy"
	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
	"github.com/nself-org/plugins/free/ci/internal/providers/runtime"
)

const (
	// Provider is the ledger provider id the rows are written under (the hosted GitHub class).
	Provider = "github"
	// LookbackDays is how many UTC days back (today included) one import reads at least. GitHub
	// finalizes a day late, so re-reading a week corrects the partial lines of the last days. Run
	// reads from the first of the month when that is earlier, so no day of the month is ever skipped.
	LookbackDays = 7
)

// Replacer writes the usage of provider for the UTC days from..to (both included) as exactly lines,
// dropping what else the ledger holds for those days; economics.ReplaceUsage is the production one.
type Replacer func(ctx context.Context, provider, from, to string, lines []economics.Usage) error

// Importer imports one account's usage.
type Importer struct {
	Settings Settings
	Source   providers.UsageSource // may be nil only while Settings.Import is false
	Replace  Replacer              // default economics.ReplaceUsage
	Now      func() time.Time      // default time.Now
}

// Result says what a run did.
type Result struct {
	Disabled bool   `json:"disabled"`
	Account  string `json:"account,omitempty"`
	From     string `json:"from,omitempty"`
	To       string `json:"to,omitempty"`
	Rows     int    `json:"rows"`
}

// FromPolicy builds an Importer from the operator's effective policy: the API URL, token source and
// private-base switch come from the providers.github.* keys, the import switch and account from ours.
// With the import off it builds no client.
func FromPolicy(ctx context.Context, eff *policy.Effective) (*Importer, error) {
	set, err := SettingsFrom(eff)
	if err != nil || !set.Import {
		return &Importer{Settings: set}, err
	}
	acct, err := ParseAccount(set.Account)
	if err != nil {
		return nil, unconfigured()
	}
	rs, err := runtime.SettingsFrom(eff)
	if err != nil {
		return nil, err
	}
	client := &rest.Client{BaseURL: rs.GitHubAPIURL, Auth: AuthFrom(rs), UserAgent: "usage",
		APIVersion: apiVersion, Provider: "github", AllowPrivateBase: rs.GitHubAllowPrivateBase}
	src, err := New(Config{Client: client, Account: acct})
	if err != nil {
		return nil, err
	}
	return &Importer{Settings: set, Source: src}, nil
}

// AuthFrom is the GitHub token source of the policy in hand: the variable named by
// providers.github.token_env, else gh's own login for the API host (the rule runtime.Credential
// applies to the process-wide policy). It is built from rs so the token and the API URL always come
// from the same effective policy, whether or not the coordinator installed a policy source.
func AuthFrom(rs runtime.Settings) rest.AuthFunc {
	if rs.GitHubTokenEnv != "" {
		return rest.EnvToken(rs.GitHubTokenEnv)
	}
	return rest.GHToken(ghHost(rs.GitHubAPIURL))
}

// ghHost is the host gh knows the login by: api.github.com is github.com, a GHES API URL is its own host.
func ghHost(apiURL string) string {
	u, err := url.Parse(apiURL)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	if h := strings.ToLower(u.Hostname()); h != "api.github.com" {
		return h
	}
	return "github.com"
}

func unconfigured() error {
	return providers.NewError(providers.Config, "E700", "billing_account", 0, time.Time{})
}

// Run imports the last LookbackDays days.
func (i *Importer) Run(ctx context.Context) (Result, error) {
	if !i.Settings.Import {
		return Result{Disabled: true}, nil
	}
	acct, err := ParseAccount(i.Settings.Account)
	if err != nil || i.Source == nil {
		return Result{}, unconfigured()
	}
	now := time.Now
	if i.Now != nil {
		now = i.Now
	}
	to := dayOf(now())
	from := to.AddDate(0, 0, 1-LookbackDays)
	if first := time.Date(to.Year(), to.Month(), 1, 0, 0, 0, 0, time.UTC); first.Before(from) {
		from = first // the whole month: the monthly allowance counts every day, and the month request returns them anyway
	}
	res := Result{Account: acct.String(), From: from.Format("2006-01-02"), To: to.Format("2006-01-02")}
	rows, err := i.Source.Usage(ctx, providers.Period{From: from, To: to})
	if err != nil {
		return res, err
	}
	lines, err := lines(acct, rows, now())
	if err != nil {
		return res, err
	}
	// An empty report replaces nothing: a malformed or truncated answer must not erase known usage (the
	// allowance would read larger), so a billing account that really went quiet keeps its old rows
	// until they leave the month.
	if len(lines) == 0 {
		return res, nil
	}
	replace := i.Replace
	if replace == nil {
		replace = economics.ReplaceUsage
	}
	if err := replace(ctx, Provider, res.From, res.To, lines); err != nil {
		return res, fmt.Errorf("replace usage %s..%s: %w", res.From, res.To, err)
	}
	res.Rows = len(lines)
	return res, nil
}

// Key is the idempotent ledger key of one line.
func Key(account, date, sku, repo string) string {
	return "github:" + account + ":" + date + ":" + sku + ":" + repo
}

// lines folds source rows that share a key (GitHub can list one sku and repo twice a day) and
// returns them in key order.
func lines(acct Account, rows []providers.UsageRow, at time.Time) ([]economics.Usage, error) {
	type sum struct {
		minutes float64
		micro   int64
		date    string
		sku     string
	}
	by := map[string]*sum{}
	for _, r := range rows {
		date := r.Date.UTC().Format("2006-01-02")
		k := Key(acct.String(), date, r.SKU, r.Repo)
		s := by[k]
		if s == nil {
			s = &sum{date: date, sku: r.SKU}
			by[k] = s
		}
		s.minutes += r.Quantity
		if r.NetMicro < 0 || s.micro > math.MaxInt64-r.NetMicro {
			return nil, unavailable("shape.amount", 200)
		}
		s.micro += r.NetMicro
		if s.minutes < 0 || s.minutes > math.MaxInt32*1e3 {
			return nil, unavailable("shape.amount", 200)
		}
	}
	keys := make([]string, 0, len(by))
	for k := range by {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]economics.Usage, 0, len(keys))
	for _, k := range keys {
		s := by[k]
		out = append(out, economics.Usage{Key: k, Provider: Provider, Date: s.date, Series: s.sku,
			Minutes: int64(math.Round(s.minutes)), CostMicro: s.micro, ReportedAt: at})
	}
	return out, nil
}
