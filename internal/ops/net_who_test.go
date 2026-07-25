package ops

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/dsig"
	"github.com/invopop/gobl/head"
	"github.com/invopop/gobl/net"
	"github.com/invopop/gobl/org"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testRewriteFetcher rewrites every well-known https URL to a fixed
// test-server base so ops-layer tests can exercise real HTTP without
// TLS. Test-only: production clients always dial the address itself.
type testRewriteFetcher struct {
	base  string
	inner *net.HTTPFetcher
}

func (f *testRewriteFetcher) rewrite(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	bu, err := url.Parse(f.base)
	if err != nil {
		return raw
	}
	u.Scheme = bu.Scheme
	u.Host = bu.Host
	return u.String()
}

func (f *testRewriteFetcher) Fetch(ctx context.Context, raw string, header http.Header) ([]byte, error) {
	return f.inner.Fetch(ctx, f.rewrite(raw), header)
}

func (f *testRewriteFetcher) Post(ctx context.Context, raw string, body []byte, header http.Header) error {
	return f.inner.Post(ctx, f.rewrite(raw), body, header)
}

// routeTo returns a fetcher that rewrites every well-known URL to the
// given httptest server, permitting loopback dials.
func routeTo(srvURL string) net.Fetcher {
	return &testRewriteFetcher{
		base:  srvURL,
		inner: &net.HTTPFetcher{Client: &http.Client{Timeout: 5 * time.Second}},
	}
}

// domainPrivateKey reads the private key InitDomain generated for a
// domain under configDir.
func domainPrivateKey(t *testing.T, configDir, domain string) *dsig.PrivateKey {
	t.Helper()
	dc := domainConfigFor(configDir, domain)
	privBytes, err := os.ReadFile(dc.PrivateKeyFile)
	require.NoError(t, err)
	key := new(dsig.PrivateKey)
	require.NoError(t, json.Unmarshal(privBytes, key))
	return key
}

// serveDomainFrom stands up the handler for an InitDomain-scaffolded
// domain whose client resolves the peer's published key.
func serveDomainFrom(t *testing.T, configDir, domain string) *httptest.Server {
	t.Helper()
	dc := domainConfigFor(configDir, domain)
	h, err := buildDomainHandler(dc, serveOpts(&mapFetcher{data: map[string][]byte{
		net.Address(testPeerDomain).KeyURL(testPeerKey.ID()): jwkBytes(t, testPeerKey),
	}}))
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestNetWho(t *testing.T) {
	configDir := t.TempDir()
	initTestDomain(t, configDir, "acme.example")
	srv := serveDomainFrom(t, configDir, "acme.example")

	env, err := NetWho(context.Background(), &NetWhoOptions{
		Target:  "acme.example",
		From:    net.Address(testPeerDomain),
		FromKey: testPeerKey,
		// Routes every well-known URL — the who lookup and the
		// target's per-key endpoint — to the test server.
		Fetcher: routeTo(srv.URL),
	})
	require.NoError(t, err)
	require.NotNil(t, env)
	require.True(t, env.Signed(), "returned envelope retains the target's signature")

	// The static who response is the target's self-signature, not
	// bound to any caller.
	p, err := head.SignedPayload(env.Signatures[0])
	require.NoError(t, err)
	assert.Equal(t, net.Address("acme.example").String(), p.Iss)
	assert.Empty(t, p.Aud)

	party, ok := env.Extract().(*org.Party)
	require.True(t, ok)
	assert.Equal(t, "acme.example", party.Name)
	require.Len(t, party.Endpoints, 1)
	assert.Equal(t, "gobl:acme.example", party.Endpoints[0].URI.String())
}

func TestNetWhoPending(t *testing.T) {
	configDir := t.TempDir()
	initTestDomain(t, configDir, "acme.example")
	// Mark the served domain for deferred disclosure.
	dc := domainConfigFor(configDir, "acme.example")
	require.NoError(t, os.WriteFile(dc.WhoDeferredFile, nil, 0o644))
	srv := serveDomainFrom(t, configDir, "acme.example")

	callerDir := t.TempDir()
	_, err := NetWho(context.Background(), &NetWhoOptions{
		Target:    "acme.example",
		From:      net.Address(testPeerDomain),
		FromKey:   testPeerKey,
		ConfigDir: callerDir,
		Fetcher:   routeTo(srv.URL),
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, net.ErrPending))

	// The pending state was recorded so the caller's inbox will accept
	// the party envelope the target may deliver later.
	assert.FileExists(t, filepath.Join(callerDir, testPeerDomain, "who-pending", "acme.example"))
}

func TestNetWhoMissingFrom(t *testing.T) {
	_, err := NetWho(context.Background(), &NetWhoOptions{Target: "acme.example"})
	require.Error(t, err)
}

func TestNetWhoMissingTarget(t *testing.T) {
	_, err := NetWho(context.Background(), &NetWhoOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target address is required")
}

// staticHandler returns a fixed status+body — useful for error-path tests.
func staticHandler(status int, body string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
}

func newWhoOpts(t *testing.T, target string, srvURL string) *NetWhoOptions {
	t.Helper()
	return &NetWhoOptions{
		Target:  net.Address(target),
		From:    net.Address(testPeerDomain),
		FromKey: testPeerKey,
		Fetcher: routeTo(srvURL),
	}
}

func TestNetWhoNon200(t *testing.T) {
	srv := httptest.NewServer(staticHandler(http.StatusForbidden, "no"))
	defer srv.Close()
	_, err := NetWho(context.Background(), newWhoOpts(t, "acme.example", srv.URL))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "HTTP 403")
}

func TestNetWhoInvalidResponseJSON(t *testing.T) {
	srv := httptest.NewServer(staticHandler(http.StatusOK, "not json"))
	defer srv.Close()
	_, err := NetWho(context.Background(), newWhoOpts(t, "acme.example", srv.URL))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid who envelope")
}

func TestNetWhoUnsignedResponse(t *testing.T) {
	srv := httptest.NewServer(staticHandler(http.StatusOK, `{"doc":{}}`))
	defer srv.Close()
	_, err := NetWho(context.Background(), newWhoOpts(t, "acme.example", srv.URL))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not signed")
}

// TestNetWhoResponseWrongIssuer: response is signed but by the peer
// (i.e., not by the target) — the verified issuer does not match the
// fetched address.
func TestNetWhoResponseWrongIssuer(t *testing.T) {
	env, err := gobl.Envelop(&org.Party{Name: "Wrong"})
	require.NoError(t, err)
	require.NoError(t, env.Sign(testPeerKey, head.WithIssuer(net.Address(testPeerDomain).String())))
	body, err := json.Marshal(env)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == net.KeyPath(testPeerKey.ID()) {
			_, _ = w.Write(jwkBytes(t, testPeerKey))
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	_, err = NetWho(context.Background(), newWhoOpts(t, testServeDomain, srv.URL))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match address")
}

// TestNetWhoTransportError exercises the transport error path. Uses
// port 1 which is closed on a non-root host.
func TestNetWhoTransportError(t *testing.T) {
	_, err := NetWho(context.Background(), &NetWhoOptions{
		Target:  net.Address(testServeDomain),
		From:    net.Address(testPeerDomain),
		FromKey: testPeerKey,
		Fetcher: routeTo("http://127.0.0.1:1"),
	})
	require.Error(t, err)
}

// TestNetWhoResponseAudBound: a who response bound to a caller (aud
// set) is not a conforming public identity and is rejected.
func TestNetWhoResponseAudBound(t *testing.T) {
	configDir := t.TempDir()
	initTestDomain(t, configDir, testServeDomain)
	targetKey := domainPrivateKey(t, configDir, testServeDomain)

	env, err := gobl.Envelop(&org.Party{Name: "X"})
	require.NoError(t, err)
	require.NoError(t, env.Sign(targetKey, head.WithIssuer(net.Address(testServeDomain).String()), head.WithAudience(net.Address(testPeerDomain).String())))
	body, err := json.Marshal(env)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == net.KeyPath(targetKey.ID()) {
			_, _ = w.Write(jwkBytes(t, targetKey))
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	_, err = NetWho(context.Background(), newWhoOpts(t, testServeDomain, srv.URL))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "audience-bound")
}

// TestNetWhoResponseDocNotParty: response is correctly signed by the
// target, but the document is not an org.Party.
func TestNetWhoResponseDocNotParty(t *testing.T) {
	configDir := t.TempDir()
	initTestDomain(t, configDir, testServeDomain)
	targetKey := domainPrivateKey(t, configDir, testServeDomain)

	// Wrap a non-party document.
	wrap, err := gobl.Envelop(&org.Endpoint{URI: "gobl:x.example"})
	require.NoError(t, err)
	require.NoError(t, wrap.Sign(targetKey, head.WithIssuer(net.Address(testServeDomain).String())))
	body, err := json.Marshal(wrap)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == net.KeyPath(targetKey.ID()) {
			_, _ = w.Write(jwkBytes(t, targetKey))
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	_, err = NetWho(context.Background(), newWhoOpts(t, testServeDomain, srv.URL))
	require.Error(t, err)
	assert.True(t, errors.Is(err, net.ErrPartyMissing))
}
