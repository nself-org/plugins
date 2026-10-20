package usage

// Purpose: GitHub, the providers.UsageSource that reads the enhanced-billing usage report (Epic D12).
// Inputs:  Config (the guarded REST client, the billing account, a clock).
// Outputs: providers.UsageRow lines for the Actions minutes of the requested period.
// Constraints: only GET /users/{u}/settings/billing/usage or /organizations/{o}/settings/billing/usage
//          is ever requested, with year, month and (for a one-day period) day. The preview
//          /usage/summary and the retired /settings/billing/actions are never requested. Every request
//          goes through rest.Client (SSRF guard, size caps). A 401/403/404/410, a body without
//          `usageItems`, or a line that does not parse is E709 and nothing is returned: a partly
//          understood report is never guessed at. Quota, transient and missing-credential errors pass
//          through unchanged, because they say "try again", not "this API is gone".

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nself-org/plugins/free/ci/internal/providers"
	"github.com/nself-org/plugins/free/ci/internal/providers/rest"
)

const (
	apiVersion = "2022-11-28"
	// MaxMonths bounds one Usage call, so a bad period cannot fan out into endless requests.
	MaxMonths = 12

	userPath = "/users/{account}/settings/billing/usage"
	orgPath  = "/organizations/{account}/settings/billing/usage"
)

// Config builds a GitHub source.
type Config struct {
	Client  *rest.Client
	Account Account
}

// GitHub is the importer's providers.UsageSource.
type GitHub struct {
	client  *rest.Client
	account Account
}

var _ providers.UsageSource = (*GitHub)(nil)

// New returns a source for c.Account.
func New(c Config) (*GitHub, error) {
	if c.Client == nil {
		return nil, errors.New("usage: no REST client")
	}
	if _, err := ParseAccount(c.Account.String()); err != nil {
		return nil, err
	}
	return &GitHub{client: c.Client, account: c.Account}, nil
}

// unavailable is the one error every "this report cannot be read" outcome becomes.
func unavailable(reason string, status int) *providers.Error {
	return providers.NewError(providers.Config, "E709", reason, status, time.Time{})
}

// wrap turns a funnel error into E709 unless it only asks for a retry or a credential.
func wrap(err error) error {
	var pe *providers.Error
	if !errors.As(err, &pe) {
		return unavailable("request", 0)
	}
	switch {
	case pe.Class == providers.Quota, pe.Class == providers.Transient, pe.Class == providers.Outage, pe.Code == "E700":
		return err
	}
	return unavailable(fmt.Sprintf("http_%d", pe.Status), pe.Status)
}

type item struct {
	Date           string   `json:"date"`
	Product        string   `json:"product"`
	SKU            string   `json:"sku"`
	Quantity       *float64 `json:"quantity"`
	UnitType       string   `json:"unitType"`
	NetAmount      *float64 `json:"netAmount"`
	RepositoryName string   `json:"repositoryName"`
}

type report struct {
	UsageItems *[]item `json:"usageItems"`
}

// months lists the (year, month) pairs the period touches.
func months(p providers.Period) ([]time.Time, error) {
	from, to := p.From.UTC(), p.To.UTC()
	if from.IsZero() || to.Before(from) {
		return nil, unavailable("period", 0)
	}
	var out []time.Time
	for m := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, time.UTC); !m.After(to); m = m.AddDate(0, 1, 0) {
		if len(out) == MaxMonths {
			return nil, unavailable("period.too_long", 0)
		}
		out = append(out, m)
	}
	return out, nil
}

// Usage implements providers.UsageSource. The rows are the Actions minute lines whose UTC day is in
// [From, To] (by day), in the order GitHub listed them.
func (g *GitHub) Usage(ctx context.Context, p providers.Period) ([]providers.UsageRow, error) {
	ms, err := months(p)
	if err != nil {
		return nil, err
	}
	path := userPath
	if g.account.Kind == KindOrg {
		path = orgPath
	}
	from, to := dayOf(p.From), dayOf(p.To)
	var rows []providers.UsageRow
	for _, m := range ms {
		q := url.Values{"year": {strconv.Itoa(m.Year())}, "month": {strconv.Itoa(int(m.Month()))}}
		if from.Equal(to) {
			q.Set("day", strconv.Itoa(from.Day()))
		}
		var rep report
		if _, err := g.client.DoQuery(ctx, "GET", path, map[string]string{"account": g.account.Login}, q, nil, &rep); err != nil {
			return nil, wrap(err)
		}
		if rep.UsageItems == nil {
			return nil, unavailable("shape.usageItems", 200)
		}
		for _, it := range *rep.UsageItems {
			row, keep, err := g.row(it)
			if err != nil {
				return nil, err
			}
			if keep && !row.Date.Before(from) && !row.Date.After(to) {
				rows = append(rows, row)
			}
		}
	}
	return rows, nil
}

func dayOf(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// row converts one report line; keep is false for a line that is not Actions minutes.
func (g *GitHub) row(it item) (providers.UsageRow, bool, error) {
	if !strings.EqualFold(it.Product, "actions") || !strings.EqualFold(it.UnitType, "minutes") {
		return providers.UsageRow{}, false, nil
	}
	day, err := time.Parse(time.RFC3339, it.Date)
	if err != nil || it.SKU == "" || it.Quantity == nil || it.NetAmount == nil {
		return providers.UsageRow{}, false, unavailable("shape.item", 200)
	}
	q, n := *it.Quantity, *it.NetAmount
	if bad(q) || bad(n) {
		return providers.UsageRow{}, false, unavailable("shape.amount", 200)
	}
	return providers.UsageRow{Account: g.account.String(), Date: dayOf(day), Product: "Actions", SKU: it.SKU, Quantity: q, Unit: "minutes",
		NetMicro: int64(math.Round(n * 1e6)), Repo: it.RepositoryName}, true, nil
}

// bad reports an amount that cannot be a consumption: negative, NaN, infinite or past int64 micro-units.
func bad(v float64) bool { return math.IsNaN(v) || math.IsInf(v, 0) || v < 0 || v > 1e12 }
