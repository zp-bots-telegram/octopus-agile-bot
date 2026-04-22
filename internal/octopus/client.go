// Package octopus is a thin HTTP client for Octopus Energy's public REST API.
// See https://docs.octopus.energy/rest/guides/endpoints/.
package octopus

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zp-bots-telegram/octopus-agile-bot/internal/agile"
)

const DefaultBaseURL = "https://api.octopus.energy"

// Client is a minimal Octopus API client. It uses HTTP Basic Auth with the API key
// as the username and an empty password, as per Octopus's documentation.
type Client struct {
	base   string
	apiKey string
	http   *http.Client
}

type Option func(*Client)

func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }
func WithBaseURL(u string) Option          { return func(c *Client) { c.base = strings.TrimRight(u, "/") } }

func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		base:   DefaultBaseURL,
		apiKey: apiKey,
		http:   &http.Client{Timeout: 10 * time.Second},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Product is a subset of the /v1/products/ response fields we actually use.
type Product struct {
	Code       string `json:"code"`
	FullName   string `json:"full_name"`
	IsVariable bool   `json:"is_variable"`
	IsTracker  bool   `json:"is_tracker"`
	IsPrepay   bool   `json:"is_prepay"`
	Brand      string `json:"brand"`
	Available  string `json:"available_from"`
}

type productsResponse struct {
	Count   int       `json:"count"`
	Next    string    `json:"next"`
	Results []Product `json:"results"`
}

// Products walks `/v1/products/` and returns every page.
func (c *Client) Products(ctx context.Context) ([]Product, error) {
	u := c.base + "/v1/products/"
	var all []Product
	for u != "" {
		var resp productsResponse
		if err := c.getJSON(ctx, u, &resp); err != nil {
			return nil, err
		}
		all = append(all, resp.Results...)
		u = resp.Next
	}
	return all, nil
}

// LatestAgileProduct returns the most recent Octopus-branded Agile product.
func (c *Client) LatestAgileProduct(ctx context.Context) (Product, error) {
	ps, err := c.Products(ctx)
	if err != nil {
		return Product{}, err
	}
	var best Product
	var bestTime time.Time
	for _, p := range ps {
		if !strings.EqualFold(p.Brand, "OCTOPUS_ENERGY") {
			continue
		}
		if !strings.HasPrefix(p.Code, "AGILE-") {
			continue
		}
		t, err := time.Parse(time.RFC3339, p.Available)
		if err != nil {
			continue
		}
		if t.After(bestTime) {
			bestTime = t
			best = p
		}
	}
	if best.Code == "" {
		return Product{}, fmt.Errorf("no Agile product found")
	}
	return best, nil
}

// GridSupplyPoint is one entry of /v1/industry/grid-supply-points/.
type GridSupplyPoint struct {
	GroupID string `json:"group_id"`
}

type gspResponse struct {
	Count   int               `json:"count"`
	Results []GridSupplyPoint `json:"results"`
}

// RegionForPostcode resolves a UK postcode to its DNO region letter (A-P) via
// /v1/industry/grid-supply-points/. The endpoint is public, so an API key is not
// strictly required, but we pass one when we have one.
func (c *Client) RegionForPostcode(ctx context.Context, postcode string) (string, error) {
	u, err := url.Parse(c.base + "/v1/industry/grid-supply-points/")
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("postcode", strings.ToUpper(strings.ReplaceAll(postcode, " ", "")))
	u.RawQuery = q.Encode()

	var resp gspResponse
	if err := c.getJSON(ctx, u.String(), &resp); err != nil {
		return "", err
	}
	if len(resp.Results) == 0 {
		return "", fmt.Errorf("no grid supply point for postcode %q", postcode)
	}
	region := strings.TrimPrefix(resp.Results[0].GroupID, "_")
	if len(region) != 1 || region[0] < 'A' || region[0] > 'P' {
		return "", fmt.Errorf("unexpected group_id %q", resp.Results[0].GroupID)
	}
	return region, nil
}

// standardUnitRate mirrors a single entry from standard-unit-rates.
type standardUnitRate struct {
	ValueExcVAT float64   `json:"value_exc_vat"`
	ValueIncVAT float64   `json:"value_inc_vat"`
	ValidFrom   time.Time `json:"valid_from"`
	ValidTo     time.Time `json:"valid_to"`
}

type ratesResponse struct {
	Count   int                `json:"count"`
	Next    string             `json:"next"`
	Results []standardUnitRate `json:"results"`
}

// StandardUnitRates fetches half-hourly rates for a tariff, following the `next`
// pagination cursor until exhausted. Passing zero-valued times omits the filter.
func (c *Client) StandardUnitRates(
	ctx context.Context, productCode, tariffCode string,
	periodFrom, periodTo time.Time,
) ([]agile.HalfHour, error) {
	u, err := url.Parse(fmt.Sprintf(
		"%s/v1/products/%s/electricity-tariffs/%s/standard-unit-rates/",
		c.base, productCode, tariffCode,
	))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	if !periodFrom.IsZero() {
		q.Set("period_from", periodFrom.UTC().Format(time.RFC3339))
	}
	if !periodTo.IsZero() {
		q.Set("period_to", periodTo.UTC().Format(time.RFC3339))
	}
	q.Set("page_size", "1500")
	u.RawQuery = q.Encode()

	var all []agile.HalfHour
	next := u.String()
	for next != "" {
		var resp ratesResponse
		if err := c.getJSON(ctx, next, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Results {
			all = append(all, agile.HalfHour{
				ValidFrom:      r.ValidFrom,
				ValidTo:        r.ValidTo,
				UnitRateExcVAT: r.ValueExcVAT,
				UnitRateIncVAT: r.ValueIncVAT,
			})
		}
		next = resp.Next
	}
	return all, nil
}

func (c *Client) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.SetBasicAuth(c.apiKey, "")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("octopus: %s %s: status %d: %s", req.Method, u, resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
