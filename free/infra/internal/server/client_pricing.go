package server

// Purpose: pricing and regional availability reads for infrastructure quotes.
// Inputs: provider JSON fetched through the shared client transport.
// Outputs: gross EUR prices and datacenter server-type availability.
// Constraints: no price estimates or fallback prices; malformed prices fail.

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
)

// LocationPrice contains gross EUR prices at one location.
type LocationPrice struct {
	Location     string
	HourlyGross  float64
	MonthlyGross float64
}

// ServerTypePricing contains prices for one server type across locations.
type ServerTypePricing struct {
	ID     int64
	Name   string
	Prices []LocationPrice
}

// PrimaryIPPricing contains prices for an IPv4 or IPv6 primary IP.
type PrimaryIPPricing struct {
	Type   string
	Prices []LocationPrice
}

// Pricing holds gross EUR prices and the server backup percentage.
type Pricing struct {
	Currency         string
	ServerTypes      []ServerTypePricing
	PrimaryIPs       []PrimaryIPPricing
	BackupPercentage float64
}

// Location describes a Hetzner deployment region.
type Location struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Country     string `json:"country"`
	City        string `json:"city"`
	NetworkZone string `json:"network_zone"`
}

// Datacenter includes supported and currently available server-type IDs.
type Datacenter struct {
	ID          int64    `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Location    Location `json:"location"`
	ServerTypes struct {
		Supported []int64 `json:"supported"`
		Available []int64 `json:"available"`
	} `json:"server_types"`
}

type wireLocationPrice struct {
	Location    string `json:"location"`
	PriceHourly struct {
		Gross json.Number `json:"gross"`
	} `json:"price_hourly"`
	PriceMonthly struct {
		Gross json.Number `json:"gross"`
	} `json:"price_monthly"`
}

func grossPrices(prices []wireLocationPrice) ([]LocationPrice, error) {
	result := make([]LocationPrice, 0, len(prices))
	for _, price := range prices {
		if price.Location == "" {
			return nil, fmt.Errorf("pricing contains an entry without a location")
		}
		hourly, err := nonnegativeNumber(price.PriceHourly.Gross)
		if err != nil {
			return nil, err
		}
		monthly, err := nonnegativeNumber(price.PriceMonthly.Gross)
		if err != nil {
			return nil, err
		}
		result = append(result, LocationPrice{price.Location, hourly, monthly})
	}
	return result, nil
}

func nonnegativeNumber(value json.Number) (float64, error) {
	n, err := value.Float64()
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return 0, fmt.Errorf("pricing contains an invalid or missing number")
	}
	return n, nil
}

// GetPricing reads current gross EUR pricing, without estimating missing values.
func (c *hetznerClient) GetPricing(ctx context.Context) (*Pricing, error) {
	var out struct {
		Pricing struct {
			Currency    string `json:"currency"`
			ServerTypes []struct {
				ID     int64               `json:"id"`
				Name   string              `json:"name"`
				Prices []wireLocationPrice `json:"prices"`
			} `json:"server_types"`
			PrimaryIPs []struct {
				Type   string              `json:"type"`
				Prices []wireLocationPrice `json:"prices"`
			} `json:"primary_ips"`
			ServerBackup struct {
				Percentage json.Number `json:"percentage"`
			} `json:"server_backup"`
		} `json:"pricing"`
	}
	if err := c.do(ctx, "GET", "/pricing", nil, &out); err != nil {
		return nil, fmt.Errorf("get pricing: %w", err)
	}
	w := out.Pricing
	if w.Currency != "EUR" {
		return nil, fmt.Errorf("pricing currency must be EUR")
	}
	percentage, err := nonnegativeNumber(w.ServerBackup.Percentage)
	if err != nil {
		return nil, err
	}
	p := &Pricing{Currency: w.Currency, BackupPercentage: percentage}
	for _, st := range w.ServerTypes {
		prices, err := grossPrices(st.Prices)
		if err != nil {
			return nil, err
		}
		p.ServerTypes = append(p.ServerTypes, ServerTypePricing{st.ID, st.Name, prices})
	}
	for _, ip := range w.PrimaryIPs {
		prices, err := grossPrices(ip.Prices)
		if err != nil {
			return nil, err
		}
		p.PrimaryIPs = append(p.PrimaryIPs, PrimaryIPPricing{ip.Type, prices})
	}
	return p, nil
}

// ListLocations returns all deployment locations.
func (c *hetznerClient) ListLocations(ctx context.Context) ([]Location, error) {
	return listPages[Location](ctx, c, "/locations", "locations", nil)
}

// ListDatacenters returns server-type availability for each datacenter.
func (c *hetznerClient) ListDatacenters(ctx context.Context) ([]Datacenter, error) {
	return listPages[Datacenter](ctx, c, "/datacenters", "datacenters", nil)
}

func listPages[T any](ctx context.Context, c *hetznerClient, path, key string, query url.Values) ([]T, error) {
	if query == nil {
		query = url.Values{}
	}
	result := make([]T, 0)
	for page := 1; ; {
		query.Set("page", fmt.Sprint(page))
		query.Set("per_page", "50")
		var out map[string]json.RawMessage
		if err := c.do(ctx, "GET", path+"?"+query.Encode(), nil, &out); err != nil {
			return nil, err
		}
		var items []T
		if err := json.Unmarshal(out[key], &items); err != nil {
			return nil, fmt.Errorf("decode list response: %w", err)
		}
		result = append(result, items...)
		var meta struct {
			Pagination struct {
				NextPage int `json:"next_page"`
			} `json:"pagination"`
		}
		if raw := out["meta"]; len(raw) != 0 {
			if err := json.Unmarshal(raw, &meta); err != nil {
				return nil, fmt.Errorf("decode pagination: %w", err)
			}
		}
		next := meta.Pagination.NextPage
		if next == 0 {
			return result, nil
		}
		if next <= page {
			return nil, fmt.Errorf("invalid pagination sequence")
		}
		page = next
	}
}
