package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
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
	assert.Equal(t, "ubl+en16931-v2017", resp.Header.Get(api.FormatHeader), "detected format")
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

// multipartExport builds a multipart export body with the document as a
// field, or as a file when asFile is set, and the attachments as files.
func multipartExport(t *testing.T, doc []byte, asFile bool, files map[string][]byte) (*bytes.Buffer, string) {
	t.Helper()
	body := new(bytes.Buffer)
	mw := multipart.NewWriter(body)
	if asFile {
		fw, err := mw.CreateFormFile("document", "invoice.json")
		require.NoError(t, err)
		_, err = fw.Write(doc)
		require.NoError(t, err)
	} else if doc != nil {
		require.NoError(t, mw.WriteField("document", string(doc)))
	}
	for name, data := range files {
		fw, err := mw.CreateFormFile("attachment", name)
		require.NoError(t, err)
		_, err = fw.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, mw.Close())
	return body, mw.FormDataContentType()
}

func TestExportImportAttachments(t *testing.T) {
	srv := httptest.NewServer(api.NewHandler())
	defer srv.Close()

	doc, err := os.ReadFile("../internal/ops/testdata/convert-invoice.json")
	require.NoError(t, err)

	for _, asFile := range []bool{false, true} {
		body, ct := multipartExport(t, doc, asFile, map[string][]byte{"invoice.pdf": []byte("%PDF-1.7")})
		resp, err := http.Post(srv.URL+prefix+"/export?format=ubl+en16931-v2017", ct, body)
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck
		require.Equal(t, http.StatusOK, resp.StatusCode, "document as file: %v", asFile)
		xml, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(xml), `filename="invoice.pdf"`)
		assert.Contains(t, string(xml), `mimeCode="application/pdf"`)

		// The envelope alone by default.
		resp, err = http.Post(srv.URL+prefix+"/import", "application/xml", bytes.NewReader(xml))
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck
		require.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "1", resp.Header.Get(api.AttachmentsHeader))
		env := new(gobl.Envelope)
		require.NoError(t, json.NewDecoder(resp.Body).Decode(env))
		assert.NotNil(t, env.Extract())

		// With the attachments when requested.
		resp, err = http.Post(srv.URL+prefix+"/import?attachments=true", "application/xml", bytes.NewReader(xml))
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var res struct {
			Format      string            `json:"format"`
			Envelope    *gobl.Envelope    `json:"envelope"`
			Attachments []json.RawMessage `json:"attachments"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&res))
		assert.Equal(t, "ubl+en16931-v2017", res.Format)
		require.NotNil(t, res.Envelope)
		_, ok := res.Envelope.Extract().(*bill.Invoice)
		assert.True(t, ok)
		require.Len(t, res.Attachments, 1)
		var att struct {
			Name string `json:"name"`
			MIME string `json:"mime"`
			Data []byte `json:"data"`
		}
		require.NoError(t, json.Unmarshal(res.Attachments[0], &att))
		assert.Equal(t, "invoice.pdf", att.Name)
		assert.Equal(t, "application/pdf", att.MIME)
		assert.Equal(t, []byte("%PDF-1.7"), att.Data)
	}
}

func TestImportAttachmentsEmpty(t *testing.T) {
	srv := httptest.NewServer(api.NewHandler())
	defer srv.Close()

	doc, err := os.ReadFile("../internal/ops/testdata/convert-invoice.json")
	require.NoError(t, err)
	resp, err := http.Post(srv.URL+prefix+"/export?format=ubl+en16931-v2017", "application/json", bytes.NewReader(doc))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	xml, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	resp, err = http.Post(srv.URL+prefix+"/import?attachments=1", "application/xml", bytes.NewReader(xml))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "0", resp.Header.Get(api.AttachmentsHeader))
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"attachments":[]`)

	resp, err = http.Post(srv.URL+prefix+"/import?attachments=maybe", "application/xml", bytes.NewReader(xml))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestExportMultipartErrors(t *testing.T) {
	srv := httptest.NewServer(api.NewHandler())
	defer srv.Close()

	t.Run("no document", func(t *testing.T) {
		body, ct := multipartExport(t, nil, false, map[string][]byte{"invoice.pdf": []byte("%PDF")})
		resp, err := http.Post(srv.URL+prefix+"/export?format=ubl+en16931-v2017", ct, body)
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(data), "no document part")
	})
	t.Run("malformed", func(t *testing.T) {
		resp, err := http.Post(srv.URL+prefix+"/export?format=ubl+en16931-v2017", "multipart/form-data; boundary=x", strings.NewReader("garbage"))
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestConvertBodyLimits(t *testing.T) {
	srv := httptest.NewServer(api.NewHandler())
	defer srv.Close()
	defer api.SetBodyLimits(1<<20, 1<<20)()

	doc, err := os.ReadFile("../internal/ops/testdata/convert-invoice.json")
	require.NoError(t, err)
	large := bytes.Repeat([]byte("x"), 2<<20)

	assertTooLarge := func(t *testing.T, resp *http.Response) {
		t.Helper()
		defer resp.Body.Close() //nolint:errcheck
		assert.Equal(t, http.StatusRequestEntityTooLarge, resp.StatusCode)
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		assert.Contains(t, string(data), "larger than the 1 MB limit")
	}

	t.Run("import", func(t *testing.T) {
		resp, err := http.Post(srv.URL+prefix+"/import", "application/xml", bytes.NewReader(large))
		require.NoError(t, err)
		assertTooLarge(t, resp)
	})
	t.Run("export", func(t *testing.T) {
		resp, err := http.Post(srv.URL+prefix+"/export?format=ubl+en16931-v2017", "application/json", bytes.NewReader(large))
		require.NoError(t, err)
		assertTooLarge(t, resp)
	})
	t.Run("export with attachments", func(t *testing.T) {
		body, ct := multipartExport(t, doc, false, map[string][]byte{"large.pdf": large})
		resp, err := http.Post(srv.URL+prefix+"/export?format=ubl+en16931-v2017", ct, body)
		require.NoError(t, err)
		assertTooLarge(t, resp)
	})
	t.Run("within the limit", func(t *testing.T) {
		body, ct := multipartExport(t, doc, false, map[string][]byte{"small.pdf": []byte("%PDF")})
		resp, err := http.Post(srv.URL+prefix+"/export?format=ubl+en16931-v2017", ct, body)
		require.NoError(t, err)
		defer resp.Body.Close() //nolint:errcheck
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
