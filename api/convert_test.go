package api_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl.dev/api"
	"github.com/invopop/gobl/bill"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatListEndpoint(t *testing.T) {
	srv := httptest.NewServer(api.NewHandler())
	defer srv.Close()

	resp, err := http.Get(srv.URL + prefix + "/formats")
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Formats []struct {
			Key string `json:"key"`
		} `json:"formats"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	keys := make([]string, len(body.Formats))
	for i, f := range body.Formats {
		keys[i] = f.Key
	}
	assert.Contains(t, keys, "ubl+en16931-v2017")
	assert.Contains(t, keys, "cii+en16931-v2017")
}

func TestExportImportEndpoints(t *testing.T) {
	srv := httptest.NewServer(api.NewHandler())
	defer srv.Close()

	doc, err := os.ReadFile("../internal/ops/testdata/convert-invoice.json")
	require.NoError(t, err)

	resp, err := http.Post(srv.URL+prefix+"/export?format=foo+bar,cii%2Ben16931-v2017", "application/json", strings.NewReader(string(doc)))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "unknown keys are rejected")

	// The "+" in keys may be left unescaped.
	resp, err = http.Post(srv.URL+prefix+"/export?format=ubl+en16931-v2017", "application/json", strings.NewReader(string(doc)))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "ubl+en16931-v2017", resp.Header.Get(api.FormatHeader))
	assert.Contains(t, resp.Header.Get("Content-Type"), "xml")
	xml, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(xml), "<Invoice")

	resp, err = http.Post(srv.URL+prefix+"/import", "application/xml", strings.NewReader(string(xml)))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusOK, resp.StatusCode)
	env := new(gobl.Envelope)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(env))
	inv, ok := env.Extract().(*bill.Invoice)
	require.True(t, ok)
	assert.Equal(t, "Provide One GmbH", inv.Supplier.Name)
}

func TestExportEndpointErrors(t *testing.T) {
	srv := httptest.NewServer(api.NewHandler())
	defer srv.Close()

	t.Run("no format", func(t *testing.T) {
		resp, err := http.Post(srv.URL+prefix+"/export", "application/json", strings.NewReader(string(testInvoice)))
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("not supported", func(t *testing.T) {
		resp, err := http.Post(srv.URL+prefix+"/export?format=ubl%2Ben16931-v2017", "application/json", strings.NewReader(string(testInvoice)))
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck
		assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, "not-supported", body["key"])
	})
}

func TestImportEndpointUnknown(t *testing.T) {
	srv := httptest.NewServer(api.NewHandler())
	defer srv.Close()

	resp, err := http.Post(srv.URL+prefix+"/import", "application/xml", strings.NewReader("<foo/>"))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode)
}

func TestFormatCheckEndpoint(t *testing.T) {
	srv := httptest.NewServer(api.NewHandler())
	defer srv.Close()

	doc, err := os.ReadFile("../internal/ops/testdata/convert-invoice.json")
	require.NoError(t, err)

	resp, err := http.Post(srv.URL+prefix+"/formats", "application/json", strings.NewReader(string(doc)))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var body struct {
		Formats []struct {
			Key      string `json:"key"`
			Accepted bool   `json:"accepted"`
			Error    *struct {
				Key    string           `json:"key"`
				Faults []map[string]any `json:"faults"`
			} `json:"error"`
		} `json:"formats"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	got := make(map[string]bool)
	for _, f := range body.Formats {
		got[f.Key] = f.Accepted
		if f.Key == "ubl+de-xrechnung-v3" {
			require.NotNil(t, f.Error)
			assert.Equal(t, "not-supported", f.Error.Key)
			assert.NotEmpty(t, f.Error.Faults, "refusals are given as faults")
		}
	}
	assert.True(t, got["ubl+en16931-v2017"])
	assert.False(t, got["ubl+de-xrechnung-v3"])
	assert.NotContains(t, got, "cdar+fr-ctc-cdv-v1", "formats that do not export invoices are left out")

	resp, err = http.Post(srv.URL+prefix+"/formats", "application/json", strings.NewReader(`{"$schema": "https://gobl.org/draft-0/bill/invoice"}`))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode, "documents that do not build are rejected")
}
