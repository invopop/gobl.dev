package ops

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/invopop/gobl/dsig"
	"github.com/invopop/gobl/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupServerWithLog stands up the test domain handler chain with a
// captured logger so individual test cases can assert on log lines.
func setupServerWithLog(t *testing.T) (*httptest.Server, *bytes.Buffer, domainConfig) {
	t.Helper()
	dc := writeServeDomain(t, t.TempDir())
	buf := new(bytes.Buffer)
	opts := serveOpts(peerFetcher(t))
	opts.Log = slog.New(slog.NewTextHandler(buf, nil))
	h, err := buildDomainHandler(dc, opts)
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, buf, dc
}

func TestAccessLogKeysLookup(t *testing.T) {
	srv, buf, _ := setupServerWithLog(t)

	// Known kid -> 200 + keys.lookup found=true.
	resp, err := http.Get(srv.URL + net.KeyPath(privateKey.ID()))
	require.NoError(t, err)
	_ = resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	out := buf.String()
	assert.Contains(t, out, "keys.lookup")
	assert.Contains(t, out, "kid="+privateKey.ID())
	assert.Contains(t, out, "found=true")
	assert.Contains(t, out, "http_request")
	assert.Contains(t, out, "status=200")

	buf.Reset()
	// Unknown kid -> 404 + keys.lookup found=false.
	resp404, err := http.Get(srv.URL + net.KeyPath("ghost"))
	require.NoError(t, err)
	_ = resp404.Body.Close()
	assert.Equal(t, http.StatusNotFound, resp404.StatusCode)
	out = buf.String()
	assert.Contains(t, out, "keys.lookup")
	assert.Contains(t, out, "found=false")
	assert.Contains(t, out, "status=404")
}

func TestAccessLogAuthTokenMissing(t *testing.T) {
	srv, buf, _ := setupServerWithLog(t)
	resp := doReq(t, http.MethodGet, srv.URL+net.WhoPath, nil, "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	out := buf.String()
	assert.Contains(t, out, "auth.rejected")
	assert.Contains(t, out, "reason=token_missing")
	assert.Contains(t, out, "status=401")
}

func TestAccessLogAuthTokenInvalid(t *testing.T) {
	srv, buf, _ := setupServerWithLog(t)
	// Token minted by an issuer whose keys the server cannot resolve.
	other := dsig.NewES256Key()
	token, err := net.NewToken(other, "unknown.example", testServeDomain, 0)
	require.NoError(t, err)
	resp := doReq(t, http.MethodGet, srv.URL+net.WhoPath, nil, "Bearer "+token)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	out := buf.String()
	assert.Contains(t, out, "auth.rejected")
	assert.Contains(t, out, "reason=token_invalid")
	assert.Contains(t, out, "status=401")
}

func TestAccessLogWhoServed(t *testing.T) {
	srv, buf, _ := setupServerWithLog(t)
	resp := doReq(t, http.MethodGet, srv.URL+net.WhoPath, nil, bearer(t, testServeDomain))
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	out := buf.String()
	assert.Contains(t, out, "who.served")
	assert.Contains(t, out, "requester="+testPeerDomain)
	assert.Contains(t, out, "status=200")
}

func TestAccessLogWhoDeferred(t *testing.T) {
	dc := writeServeDomain(t, t.TempDir())
	require.NoError(t, os.WriteFile(dc.WhoDeferredFile, nil, 0o644))
	buf := new(bytes.Buffer)
	opts := serveOpts(peerFetcher(t))
	opts.Log = slog.New(slog.NewTextHandler(buf, nil))
	h, err := buildDomainHandler(dc, opts)
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp := doReq(t, http.MethodGet, srv.URL+net.WhoPath, nil, bearer(t, testServeDomain))
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	out := buf.String()
	assert.Contains(t, out, "who.deferred")
	assert.Contains(t, out, "requester="+testPeerDomain)
	assert.Contains(t, out, "status=202")
}

func TestAccessLogInboxNotEndorsed(t *testing.T) {
	authorityKey := dsig.NewES256Key()
	const authority = "kyc.example"
	dc := writeServeDomain(t, t.TempDir())
	fetcher := peerFetcher(t)
	fetcher.data[net.Address(authority).KeyURL(authorityKey.ID())] = jwkBytes(t, authorityKey)
	fetcher.data[net.Address(testPeerDomain).WhoURL()] = peerWhoBytes(t, nil, "", "")
	buf := new(bytes.Buffer)
	opts := serveOpts(fetcher)
	opts.Log = slog.New(slog.NewTextHandler(buf, nil))
	opts.Authorities = []net.Address{authority}
	h, err := buildDomainHandler(dc, opts)
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	defer srv.Close()

	env := signedNoteTo(t, "unendorsed", testServeDomain)
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
	assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	out := buf.String()
	assert.Contains(t, out, "inbox.rejected")
	assert.Contains(t, out, "reason=not_endorsed")
	assert.Contains(t, out, "status=403")
}

func TestAccessLogInboxAccepted(t *testing.T) {
	srv, buf, dc := setupServerWithLog(t)

	env := signedNoteTo(t, "logged", testServeDomain)
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
	out := buf.String()
	assert.Contains(t, out, "inbox.accepted")
	assert.Contains(t, out, "envelope="+env.Head.UUID.String())
	assert.Contains(t, out, "status=202")

	// Sanity: the envelope was persisted.
	files, _ := os.ReadDir(dc.InboxDir)
	require.Len(t, files, 1)
}

func TestAccessLogInboxAudMismatch(t *testing.T) {
	srv, buf, _ := setupServerWithLog(t)
	env := signedNoteTo(t, "wrong aud", "other.example")
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	out := buf.String()
	assert.Contains(t, out, "inbox.rejected")
	assert.Contains(t, out, "reason=verify_failed")
	assert.Contains(t, out, "status=401")
}

func TestStatusRecorderImplicit200(t *testing.T) {
	// When the inner handler writes a body without an explicit
	// WriteHeader, the recorder treats it as 200.
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	_, _ = rec.Write([]byte("hello"))
	assert.Equal(t, http.StatusOK, rec.status)
}

func TestStatusRecorderRespectsFirst(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	rec.WriteHeader(http.StatusCreated)
	rec.WriteHeader(http.StatusBadRequest) // second call must not overwrite
	assert.Equal(t, http.StatusCreated, rec.status)
}

func TestCORSAllowAll(t *testing.T) {
	srv, _, _ := setupServerWithLog(t)

	t.Run("GET response carries ACAO=*", func(t *testing.T) {
		resp, err := http.Get(srv.URL + net.JWKSPath)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("OPTIONS preflight returns 204 with full CORS headers", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodOptions, srv.URL+net.JWKSPath, nil)
		require.NoError(t, err)
		req.Header.Set("Origin", "https://jwt.io")
		req.Header.Set("Access-Control-Request-Method", "GET")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
		assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
		assert.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "GET")
		assert.Contains(t, resp.Header.Get("Access-Control-Allow-Headers"), "Content-Type")
		assert.Contains(t, resp.Header.Get("Access-Control-Allow-Headers"), "Authorization")
		assert.NotEmpty(t, resp.Header.Get("Access-Control-Max-Age"))
	})

	t.Run("per-kid endpoint also carries ACAO", func(t *testing.T) {
		resp, err := http.Get(srv.URL + net.KeyPath(privateKey.ID()))
		require.NoError(t, err)
		_ = resp.Body.Close()
		assert.Equal(t, "*", resp.Header.Get("Access-Control-Allow-Origin"))
	})
}

func TestAccessLogMiddlewareDirect(t *testing.T) {
	// Drive the middleware directly to confirm field shape.
	buf := new(bytes.Buffer)
	log := slog.New(slog.NewTextHandler(buf, nil))
	h := accessLog(log, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Host = "acme.example:8080"
	h.ServeHTTP(rec, req)

	out := buf.String()
	assert.Contains(t, out, "http_request")
	assert.Contains(t, out, "method=GET")
	assert.Contains(t, out, "path=/x")
	assert.Contains(t, out, "host=acme.example")
	assert.Contains(t, out, "remote=10.0.0.1:1234")
	assert.Contains(t, out, "status=200")
	assert.Contains(t, out, "duration_ms=")
}
