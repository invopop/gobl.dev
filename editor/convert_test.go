package editor

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/invopop/gobl.dev/editor/examples"
	"github.com/invopop/gobl"
)

// buildEnvelope runs the curated example through the GOBL build flow to
// produce a calculated envelope ready for conversion.
func buildEnvelope(t *testing.T, exampleID string) *gobl.Envelope {
	t.Helper()
	data, ok := examples.Get(exampleID)
	if !ok {
		t.Fatalf("example %q not found", exampleID)
	}
	doc, err := gobl.Parse(data)
	if err != nil {
		t.Fatalf("parse example %s: %v", exampleID, err)
	}
	env, err := gobl.Envelop(doc)
	if err != nil {
		t.Fatalf("envelop %s: %v", exampleID, err)
	}
	if err := env.Calculate(); err != nil {
		t.Fatalf("calculate envelope for %s: %v", exampleID, err)
	}
	return env
}

func TestFormatConversions(t *testing.T) {
	cases := []struct {
		format    string
		example   string
		wantStart string
	}{
		{"ubl", "de-xrechnung", "<?xml"},
		{"ubl-peppol", "de-xrechnung", "<?xml"},
		{"ubl-xrechnung", "de-xrechnung", "<?xml"},
		{"cii", "de-xrechnung", "<?xml"},
		{"cii-facturx", "fr-facturx", "<?xml"},
		{"cii-xrechnung", "de-xrechnung", "<?xml"},
		// TODO: re-enable once FatturaPA is re-enabled in convert.go.
		// {"fatturapa", "it-fatturapa", "<?xml"},
		{"html", "us", "<html"},
	}
	for _, c := range cases {
		t.Run(c.format+"/"+c.example, func(t *testing.T) {
			f, ok := findFormat(c.format)
			if !ok {
				t.Fatalf("format %q not registered", c.format)
			}
			env := buildEnvelope(t, c.example)
			out, err := f.convert(context.Background(), env)
			if err != nil {
				t.Fatalf("convert: %v", err)
			}
			if len(out) == 0 {
				t.Fatalf("empty output")
			}
			if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(string(out))), c.wantStart) {
				t.Fatalf("unexpected prefix; got %q", head(out, 80))
			}
		})
	}
}

func TestHandleConvertSurfacesValidationFaults(t *testing.T) {
	// us.json is a minimal US sales-tax invoice — it doesn't satisfy EN16931,
	// so asking for UBL forces gobl.ubl to auto-inject en16931 and fail
	// validation. The resulting *gobl.Error must round-trip through the HTTP
	// layer as {key, message, faults:[{code, paths, message}]} so the editor
	// can highlight the offending paths exactly like it does for /v0/build.
	env := buildEnvelope(t, "us")
	raw, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/_editor/convert?format=ubl", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	handleConvert(w, r)

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status: got %d, want 422", w.Code)
	}
	var body struct {
		Key     string `json:"key"`
		Message string `json:"message"`
		Faults  []struct {
			Code    string   `json:"code"`
			Paths   []string `json:"paths"`
			Message string   `json:"message"`
		} `json:"faults"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Key == "" {
		t.Fatalf("missing gobl error key; body: %+v", body)
	}
	if len(body.Faults) == 0 {
		t.Fatalf("expected faults list; got message=%q", body.Message)
	}
	for _, f := range body.Faults {
		if f.Code == "" {
			t.Errorf("fault missing code: %+v", f)
		}
		if len(f.Paths) == 0 {
			t.Errorf("fault missing paths: %+v", f)
		}
		if f.Message == "" {
			t.Errorf("fault missing message: %+v", f)
		}
	}
}

func TestHandleConvertAcceptsBareDocument(t *testing.T) {
	// When the "Envelop" checkbox is unchecked the editor sends a bare
	// bill.Invoice (no envelope wrapper). handleConvert must wrap it on the
	// fly rather than bouncing the request.
	raw, ok := examples.Get("de-xrechnung")
	if !ok {
		t.Fatal("example not found")
	}
	r := httptest.NewRequest(http.MethodPost, "/_editor/convert?format=ubl-xrechnung", bytes.NewReader(raw))
	w := httptest.NewRecorder()
	handleConvert(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 — body: %s", w.Code, w.Body.String())
	}
	if !strings.HasPrefix(w.Body.String(), "<?xml") {
		t.Fatalf("unexpected prefix: %s", head(w.Body.Bytes(), 80))
	}
}

func TestHandleConvertUnknownFormat(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/_editor/convert?format=nope", bytes.NewReader([]byte("{}")))
	w := httptest.NewRecorder()
	handleConvert(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want 400", w.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if msg, _ := body["message"].(string); !strings.Contains(msg, "unknown format") {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func head(b []byte, n int) string {
	if len(b) < n {
		return string(b)
	}
	return string(b[:n])
}
