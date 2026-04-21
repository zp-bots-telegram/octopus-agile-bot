package octopus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProducts_Paginated(t *testing.T) {
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/products/", func(w http.ResponseWriter, r *http.Request) {
		assertBasicAuth(t, r, "sk_test")
		page := r.URL.Query().Get("page")
		switch page {
		case "", "1":
			fmt.Fprintf(w, `{"count":2,"next":%q,"results":[{"code":"AGILE-24-10-01","full_name":"Agile Oct 2024","is_variable":true,"brand":"OCTOPUS_ENERGY","available_from":"2024-10-01T00:00:00Z"}]}`, srv.URL+"/v1/products/?page=2")
		case "2":
			fmt.Fprintln(w, `{"count":2,"next":"","results":[{"code":"AGILE-22-08-31","full_name":"Agile Aug 2022","is_variable":true,"brand":"OCTOPUS_ENERGY","available_from":"2022-08-31T00:00:00Z"}]}`)
		}
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient("sk_test", WithBaseURL(srv.URL))
	prods, err := c.Products(context.Background())
	require.NoError(t, err)
	assert.Len(t, prods, 2)
}

func TestLatestAgileProduct(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/products/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, `{"count":3,"next":"","results":[
			{"code":"AGILE-22-08-31","brand":"OCTOPUS_ENERGY","is_variable":true,"available_from":"2022-08-31T00:00:00Z"},
			{"code":"AGILE-24-10-01","brand":"OCTOPUS_ENERGY","is_variable":true,"available_from":"2024-10-01T00:00:00Z"},
			{"code":"TRACKER-24-01-01","brand":"OCTOPUS_ENERGY","is_tracker":true,"available_from":"2024-01-01T00:00:00Z"}
		]}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient("sk_test", WithBaseURL(srv.URL))
	p, err := c.LatestAgileProduct(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "AGILE-24-10-01", p.Code)
}

func TestStandardUnitRates_Paginated(t *testing.T) {
	var srv *httptest.Server
	base := "/v1/products/AGILE-24-10-01/electricity-tariffs/E-1R-AGILE-24-10-01-C/standard-unit-rates/"
	mux := http.NewServeMux()
	mux.HandleFunc(base, func(w http.ResponseWriter, r *http.Request) {
		assertBasicAuth(t, r, "sk_test")
		if r.URL.Query().Get("page") == "2" {
			fmt.Fprintln(w, `{"count":2,"next":"","results":[
				{"value_exc_vat":20.0,"value_inc_vat":21.0,"valid_from":"2026-04-20T01:00:00Z","valid_to":"2026-04-20T01:30:00Z"}
			]}`)
			return
		}
		fmt.Fprintf(w, `{"count":2,"next":%q,"results":[
			{"value_exc_vat":10.0,"value_inc_vat":10.5,"valid_from":"2026-04-20T00:00:00Z","valid_to":"2026-04-20T00:30:00Z"}
		]}`, srv.URL+base+"?page=2")
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient("sk_test", WithBaseURL(srv.URL))
	rates, err := c.StandardUnitRates(
		context.Background(),
		"AGILE-24-10-01", "E-1R-AGILE-24-10-01-C",
		time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)
	assert.Len(t, rates, 2)
	assert.InDelta(t, 10.5, rates[0].UnitRateIncVAT, 1e-9)
	assert.Equal(t, time.Date(2026, 4, 20, 1, 0, 0, 0, time.UTC), rates[1].ValidFrom)
}

func TestGet_NonOKReturnsError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/products/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewClient("sk_test", WithBaseURL(srv.URL))
	_, err := c.Products(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}

// assertBasicAuth uses asserts rather than requires because it runs inside the
// httptest.Server handler goroutine — a require.FailNow would panic and tear down the
// connection before the client sees the response.
func assertBasicAuth(t *testing.T, r *http.Request, wantUser string) {
	t.Helper()
	user, pass, ok := r.BasicAuth()
	assert.True(t, ok, "missing basic auth")
	assert.Equal(t, wantUser, user)
	assert.Equal(t, "", pass)
}
