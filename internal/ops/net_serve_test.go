package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-jose/go-jose/v4"
	"github.com/invopop/gobl"
	"github.com/invopop/gobl/dsig"
	"github.com/invopop/gobl/head"
	"github.com/invopop/gobl/net"
	"github.com/invopop/gobl/note"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mapFetcher struct {
	data map[string][]byte
	errs map[string]error
}

func (m *mapFetcher) Fetch(_ context.Context, url string, _ http.Header) ([]byte, error) {
	if err, ok := m.errs[url]; ok {
		return nil, err
	}
	body, ok := m.data[url]
	if !ok {
		return nil, net.ErrFetchFailed
	}
	return body, nil
}

func (m *mapFetcher) Post(_ context.Context, _ string, _ []byte, _ http.Header) error {
	return net.ErrFetchFailed
}

// jwkBytes returns the single-JWK bytes served at the per-key endpoint
// for this key.
func jwkBytes(t *testing.T, key *dsig.PrivateKey) []byte {
	t.Helper()
	b, err := json.Marshal(key.Public())
	require.NoError(t, err)
	return b
}

const (
	testServeDomain = "me.example"
	testPeerDomain  = "peer.example"
)

var testPeerKey = dsig.NewES256Key()

// testAuthority endorses the shared test peer; naming itself as
// verifier makes the peer verified with a single countersignature.
const testAuthority = "authority.example"

var testAuthorityKey = dsig.NewES256Key()

// serveOpts builds NetServeOptions with the given fetcher, the shared
// test authority, and a discarded log — the standard fixture for
// handler tests.
func serveOpts(fetcher net.Fetcher) *NetServeOptions {
	return &NetServeOptions{
		Fetcher:     fetcher,
		Authorities: []net.Address{testAuthority},
		Log:         discardLog(),
	}
}

// peerFetcher resolves the peer's, the authority's, and the served
// domain's published keys plus the peer's endorsed-and-verified who —
// everything the served domain's client needs to verify request
// tokens, envelope signatures, and the sender endorsement.
func peerFetcher(t *testing.T) *mapFetcher {
	t.Helper()
	return &mapFetcher{data: map[string][]byte{
		net.Address(testPeerDomain).KeyURL(testPeerKey.ID()):     jwkBytes(t, testPeerKey),
		net.Address(testServeDomain).KeyURL(privateKey.ID()):     jwkBytes(t, privateKey),
		net.Address(testAuthority).KeyURL(testAuthorityKey.ID()): jwkBytes(t, testAuthorityKey),
		net.Address(testPeerDomain).WhoURL():                     peerWhoBytes(t, testAuthorityKey, testAuthority, testAuthority),
	}}
}

// bearer mints a request token for the peer targeting the given
// address and returns it as an Authorization header value.
func bearer(t *testing.T, to net.Address) string {
	t.Helper()
	token, err := net.NewToken(testPeerKey, testPeerDomain, to, 0)
	require.NoError(t, err)
	return "Bearer " + token
}

// doReq performs an HTTP request with an optional Authorization value.
func doReq(t *testing.T, method, url string, body []byte, auth string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	require.NoError(t, err)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// writeServeDomain lays down the on-disk state for testServeDomain.
func writeServeDomain(t *testing.T, cfg string) domainConfig {
	t.Helper()
	dc := domainConfigFor(cfg, testServeDomain)
	require.NoError(t, os.MkdirAll(filepath.Join(cfg, testServeDomain), 0o700))
	writeKey(t, dc.KeysDir, privateKey)
	writePrivate(t, dc.PrivateKeyFile, privateKey)
	writeRawParty(t, dc.PartyFile, &org.Party{Name: "Me"})
	return dc
}

// setupNetServer stands up a single-domain handler for testServeDomain
// (signed by the package privateKey) whose client can resolve both the
// served domain's and the peer's /keys. Returns the server and config.
func setupNetServer(t *testing.T) (*httptest.Server, domainConfig) {
	t.Helper()
	dc := writeServeDomain(t, t.TempDir())
	h, err := buildDomainHandler(dc, serveOpts(peerFetcher(t)))
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, dc
}

// signedNoteTo wraps a note.Message in an envelope signed by the peer
// and bound to the given audience.
func signedNoteTo(t *testing.T, content string, aud net.Address) *gobl.Envelope {
	t.Helper()
	msg := &note.Message{Content: content}
	msg.SetUUID(uuid.V7())
	env, err := gobl.Envelop(msg)
	require.NoError(t, err)
	require.NoError(t, env.Sign(testPeerKey,
		head.WithIssuer(net.Address(testPeerDomain).String()),
		head.WithAudience(aud.String())))
	return env
}

func marshalEnv(t *testing.T, env *gobl.Envelope) []byte {
	t.Helper()
	body, err := json.Marshal(env)
	require.NoError(t, err)
	return body
}

func TestNetServeKeys(t *testing.T) {
	srv, _ := setupNetServer(t)

	// Per-key endpoint: known kid returns the single JWK, no auth needed.
	resp, err := http.Get(srv.URL + net.KeyPath(privateKey.ID()))
	require.NoError(t, err)
	defer resp.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	jwk := new(jose.JSONWebKey)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(jwk))
	assert.Equal(t, privateKey.ID(), jwk.KeyID)

	// Unknown kid returns 404 — no enumeration is exposed.
	resp404, err := http.Get(srv.URL + net.KeyPath("unknown-kid"))
	require.NoError(t, err)
	defer resp404.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNotFound, resp404.StatusCode)

	// The bulk /keys endpoint no longer exists.
	respBulk, err := http.Get(srv.URL + net.KeysPath)
	require.NoError(t, err)
	defer respBulk.Body.Close() //nolint:errcheck
	assert.Equal(t, http.StatusNotFound, respBulk.StatusCode)
}

func TestNetServeWho(t *testing.T) {
	srv, _ := setupNetServer(t)

	resp := doReq(t, http.MethodGet, srv.URL+net.WhoPath, nil, bearer(t, testServeDomain))
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "private", resp.Header.Get("Cache-Control"))

	env := new(gobl.Envelope)
	require.NoError(t, json.NewDecoder(resp.Body).Decode(env))
	require.True(t, env.Signed())

	p, err := headSignedPayload(env)
	require.NoError(t, err)
	assert.Equal(t, net.Address(testServeDomain).String(), p.Iss, "response is the domain's self-signature")
	assert.Empty(t, p.Aud, "the static who response is not audience-bound")

	party, ok := env.Extract().(*org.Party)
	require.True(t, ok)
	assert.Equal(t, "Me", party.Name)
}

func TestNetServeWhoRequiresToken(t *testing.T) {
	srv, _ := setupNetServer(t)

	t.Run("missing token", func(t *testing.T) {
		resp := doReq(t, http.MethodGet, srv.URL+net.WhoPath, nil, "")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("token bound to another audience", func(t *testing.T) {
		resp := doReq(t, http.MethodGet, srv.URL+net.WhoPath, nil, bearer(t, "other.example"))
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("token from an unresolvable issuer", func(t *testing.T) {
		other := dsig.NewES256Key()
		token, err := net.NewToken(other, "unknown.example", testServeDomain, 0)
		require.NoError(t, err)
		resp := doReq(t, http.MethodGet, srv.URL+net.WhoPath, nil, "Bearer "+token)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("not a bearer header", func(t *testing.T) {
		resp := doReq(t, http.MethodGet, srv.URL+net.WhoPath, nil, "Basic dXNlcjpwdw==")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestNetServeWhoNoParty(t *testing.T) {
	// A domain without a party file is receive-only: /who answers 204.
	cfg := t.TempDir()
	dc := domainConfigFor(cfg, testServeDomain)
	require.NoError(t, os.MkdirAll(filepath.Join(cfg, testServeDomain), 0o700))
	writeKey(t, dc.KeysDir, privateKey)
	writePrivate(t, dc.PrivateKeyFile, privateKey)

	h, err := buildDomainHandler(dc, serveOpts(peerFetcher(t)))
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp := doReq(t, http.MethodGet, srv.URL+net.WhoPath, nil, bearer(t, testServeDomain))
	assert.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestNetServeWhoDeferred(t *testing.T) {
	cfg := t.TempDir()
	dc := writeServeDomain(t, cfg)
	require.NoError(t, os.WriteFile(dc.WhoDeferredFile, nil, 0o644))

	h, err := buildDomainHandler(dc, serveOpts(peerFetcher(t)))
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp := doReq(t, http.MethodGet, srv.URL+net.WhoPath, nil, bearer(t, testServeDomain))
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// The request was recorded for the operator to approve.
	data, err := os.ReadFile(filepath.Join(dc.WhoRequestsDir, testPeerDomain+".json"))
	require.NoError(t, err)
	req := new(whoRequest)
	require.NoError(t, json.Unmarshal(data, req))
	assert.Equal(t, net.Address(testPeerDomain), req.Requester)
	assert.NotEmpty(t, req.Time)

	// Still 401 without a token: deferral does not open the endpoint.
	resp401 := doReq(t, http.MethodGet, srv.URL+net.WhoPath, nil, "")
	assert.Equal(t, http.StatusUnauthorized, resp401.StatusCode)
}

func TestNetServeInboxAccepts(t *testing.T) {
	srv, dc := setupNetServer(t)

	env := signedNoteTo(t, "hello inbox", testServeDomain)
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	files, err := os.ReadDir(dc.InboxDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, env.Head.UUID.String()+".json", files[0].Name())
}

func TestNetServeInboxRequiresToken(t *testing.T) {
	srv, dc := setupNetServer(t)

	env := signedNoteTo(t, "no token", testServeDomain)
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), "")
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	files, err := os.ReadDir(dc.InboxDir)
	require.NoError(t, err)
	assert.Empty(t, files, "nothing is persisted before authentication")
}

func TestNetServeInboxIntermediaryToken(t *testing.T) {
	// The request token may name a different party than the envelope's
	// signer: a trusted intermediary transmitting on the signer's
	// behalf. The server resolves the intermediary's key to verify the
	// token and the signer's key to verify the envelope.
	intermediaryKey := dsig.NewES256Key()
	const intermediary = "carrier.example"

	dc := writeServeDomain(t, t.TempDir())
	fetcher := peerFetcher(t)
	fetcher.data[net.Address(intermediary).KeyURL(intermediaryKey.ID())] = jwkBytes(t, intermediaryKey)
	h, err := buildDomainHandler(dc, serveOpts(fetcher))
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	defer srv.Close()

	token, err := net.NewToken(intermediaryKey, intermediary, testServeDomain, 0)
	require.NoError(t, err)
	env := signedNoteTo(t, "via intermediary", testServeDomain)
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), "Bearer "+token)
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
}

// peerWhoBytes builds the peer's self-signed who envelope, optionally
// countersigned by an authority key naming a verifier.
func peerWhoBytes(t *testing.T, authKey *dsig.PrivateKey, authority, verifier net.Address) []byte {
	t.Helper()
	party := &org.Party{
		Name:      "Peer",
		Endpoints: []*org.Endpoint{{URI: net.Address(testPeerDomain).URI()}},
	}
	party.SetUUID(uuid.V7())
	env, err := gobl.Envelop(party)
	require.NoError(t, err)
	require.NoError(t, env.Sign(testPeerKey, head.WithIssuer(net.Address(testPeerDomain).String())))
	if authKey != nil {
		opts := []head.SignOption{
			head.WithIssuer(authority.String()),
			head.WithAudience(net.Address(testPeerDomain).String()),
		}
		if verifier != "" {
			opts = append(opts, head.WithVerifier(verifier.String()))
		}
		require.NoError(t, env.Sign(authKey, opts...))
	}
	return marshalEnv(t, env)
}

func TestNetServeInboxEndorsementPolicy(t *testing.T) {
	authorityKey := dsig.NewES256Key()
	const authority = "kyc.example"

	setup := func(t *testing.T, whoBytes []byte, allowUnverified bool) (*httptest.Server, domainConfig) {
		dc := writeServeDomain(t, t.TempDir())
		fetcher := peerFetcher(t)
		fetcher.data[net.Address(authority).KeyURL(authorityKey.ID())] = jwkBytes(t, authorityKey)
		fetcher.data[net.Address(testPeerDomain).WhoURL()] = whoBytes
		opts := serveOpts(fetcher)
		opts.Authorities = []net.Address{authority}
		opts.AllowUnverified = allowUnverified
		h, err := buildDomainHandler(dc, opts)
		require.NoError(t, err)
		srv := httptest.NewServer(h)
		t.Cleanup(srv.Close)
		return srv, dc
	}

	t.Run("unendorsed sender is rejected even when unverified is allowed", func(t *testing.T) {
		srv, dc := setup(t, peerWhoBytes(t, nil, "", ""), true)
		env := signedNoteTo(t, "unendorsed", testServeDomain)
		resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		files, err := os.ReadDir(dc.InboxDir)
		require.NoError(t, err)
		assert.Empty(t, files)
	})

	t.Run("registered-only sender is rejected by default", func(t *testing.T) {
		srv, dc := setup(t, peerWhoBytes(t, authorityKey, authority, ""), false)
		env := signedNoteTo(t, "registered only", testServeDomain)
		resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		files, err := os.ReadDir(dc.InboxDir)
		require.NoError(t, err)
		assert.Empty(t, files)
	})

	t.Run("registered-only sender is accepted with AllowUnverified", func(t *testing.T) {
		srv, dc := setup(t, peerWhoBytes(t, authorityKey, authority, ""), true)
		env := signedNoteTo(t, "sandbox", testServeDomain)
		resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
		assert.Equal(t, http.StatusAccepted, resp.StatusCode)
		files, err := os.ReadDir(dc.InboxDir)
		require.NoError(t, err)
		assert.Len(t, files, 1)
	})

	t.Run("verified sender is accepted by default", func(t *testing.T) {
		// The authority names itself as verifier, so its single
		// countersignature carries both attestations.
		srv, dc := setup(t, peerWhoBytes(t, authorityKey, authority, authority), false)
		env := signedNoteTo(t, "verified", testServeDomain)
		resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
		assert.Equal(t, http.StatusAccepted, resp.StatusCode)
		files, err := os.ReadDir(dc.InboxDir)
		require.NoError(t, err)
		assert.Len(t, files, 1)
	})

	t.Run("party envelope fulfilling a pending who request skips endorsement", func(t *testing.T) {
		srv, dc := setup(t, peerWhoBytes(t, nil, "", ""), false)
		// Mark an outstanding who request to the peer.
		require.NoError(t, os.MkdirAll(dc.WhoPendingDir, 0o755))
		pending := filepath.Join(dc.WhoPendingDir, testPeerDomain)
		require.NoError(t, os.WriteFile(pending, nil, 0o644))

		party := &org.Party{
			Name:      "Peer",
			Endpoints: []*org.Endpoint{{URI: net.Address(testPeerDomain).URI()}},
		}
		party.SetUUID(uuid.V7())
		env, err := gobl.Envelop(party)
		require.NoError(t, err)
		require.NoError(t, env.Sign(testPeerKey,
			head.WithIssuer(net.Address(testPeerDomain).String()),
			head.WithAudience(net.Address(testServeDomain).String())))

		resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
		assert.Equal(t, http.StatusAccepted, resp.StatusCode)
		assert.NoFileExists(t, pending, "the pending marker is consumed")

		files, err := os.ReadDir(dc.InboxDir)
		require.NoError(t, err)
		assert.Len(t, files, 1)
	})
}

func TestNetServeInboxValidationFails(t *testing.T) {
	srv, _ := setupNetServer(t)
	// Envelope JSON that parses but lacks required fields (digest, etc.)
	// so env.Validate fails with 422.
	body := []byte(`{"$schema":"https://gobl.org/draft-0/envelope","head":{"uuid":"01906c00-0000-7000-0000-000000000000","dig":{"alg":"sha256","val":"x"}},"doc":null}`)
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, body, bearer(t, testServeDomain))
	// 422 (validation), or other 4xx — anything that isn't 202.
	assert.NotEqual(t, http.StatusAccepted, resp.StatusCode)
}

// TestNetServeInboxRejectsTraversalUUID confirms that a payload trying
// to escape the inbox directory via the head.uuid field is rejected
// (env.Validate enforces UUID format; handleInbox re-parses as
// defence-in-depth) and that no file is written outside the inbox dir.
func TestNetServeInboxRejectsTraversalUUID(t *testing.T) {
	srv, dc := setupNetServer(t)

	body := []byte(`{"$schema":"https://gobl.org/draft-0/envelope","head":{"uuid":"../../etc/passwd","dig":{"alg":"sha256","val":"x"}},"doc":{}}`)
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, body, bearer(t, testServeDomain))
	assert.NotEqual(t, http.StatusAccepted, resp.StatusCode)

	// Nothing was written inside the inbox dir...
	files, err := os.ReadDir(dc.InboxDir)
	require.NoError(t, err)
	assert.Empty(t, files)

	// ...nor anywhere up the path. Walk a few levels above and assert
	// no "passwd"-like artefacts appeared.
	parent := filepath.Dir(filepath.Dir(dc.InboxDir))
	for _, suspect := range []string{"passwd", "passwd.json", "etc"} {
		_, statErr := os.Stat(filepath.Join(parent, suspect))
		assert.True(t, os.IsNotExist(statErr), "traversal artefact at %s/%s should not exist", parent, suspect)
	}
}

func TestNetServeInboxWriteFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("write-permission tests do not apply when running as root")
	}
	srv, dc := setupNetServer(t)

	// Make the inbox directory read-only so os.Create fails.
	require.NoError(t, os.Chmod(dc.InboxDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(dc.InboxDir, 0o755) })

	env := signedNoteTo(t, "fail to write", testServeDomain)
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
}

func TestNetServeInboxRejectsBadJSON(t *testing.T) {
	srv, _ := setupNetServer(t)
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, []byte("not json"), bearer(t, testServeDomain))
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestNetServeInboxAudMismatch(t *testing.T) {
	srv, _ := setupNetServer(t)
	// An envelope bound to a different recipient is rejected — prevents
	// replay against an inbox the signer didn't intend.
	env := signedNoteTo(t, "wrong aud", "other.example")
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestNetServeInboxAudMissing(t *testing.T) {
	srv, _ := setupNetServer(t)
	// An envelope signed without an aud is rejected — inboxes require
	// the signature to be bound to their address so the same envelope
	// cannot be replayed against multiple inboxes.
	msg := &note.Message{Content: "no aud"}
	msg.SetUUID(uuid.V7())
	env, err := gobl.Envelop(msg)
	require.NoError(t, err)
	require.NoError(t, env.Sign(testPeerKey, head.WithIssuer(net.Address(testPeerDomain).String())))
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestNetServeInboxRejectsBadSignature(t *testing.T) {
	srv, dc := setupNetServer(t)

	// Signed by a key whose /keys the server cannot resolve for the iss.
	other := dsig.NewES256Key()
	msg := &note.Message{Content: "bad sig"}
	msg.SetUUID(uuid.V7())
	env, err := gobl.Envelop(msg)
	require.NoError(t, err)
	require.NoError(t, env.Sign(other, head.WithIssuer(net.Address("unknown.example").String()), head.WithAudience(net.Address(testServeDomain).String())))

	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	files, err := os.ReadDir(dc.InboxDir)
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestSignedPartyBytesPreSigned(t *testing.T) {
	// A party file already carrying the domain's self-signature (no
	// aud) is served verbatim — countersignatures survive.
	party := &org.Party{Name: "Me"}
	party.SetUUID(uuid.V7())
	env, err := gobl.Envelop(party)
	require.NoError(t, err)
	require.NoError(t, env.Sign(privateKey, head.WithIssuer(net.Address(testServeDomain).String())))
	want := marshalEnv(t, env)

	got, err := signedPartyBytes(env, privateKey, testServeDomain)
	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(got))
	assert.Len(t, env.Signatures, 1, "no second signature is added")
}

func TestSignedPartyBytesSignFails(t *testing.T) {
	party := &org.Party{Name: "Me"}
	party.SetUUID(uuid.V7())
	env, err := gobl.Envelop(party)
	require.NoError(t, err)
	_, err = signedPartyBytes(env, &dsig.PrivateKey{}, testServeDomain)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sign party")
}

func headSignedPayload(env *gobl.Envelope) (*head.SigningPayload, error) {
	return head.SignedPayload(env.Signatures[0])
}

func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names, nil
}

func TestNetServeTokenVerificationUnavailable(t *testing.T) {
	// The requester's key endpoint is unreachable (503): the server
	// must answer 503 so the client retries, not 401.
	dc := writeServeDomain(t, t.TempDir())
	fetcher := peerFetcher(t)
	fetcher.errs = map[string]error{
		net.Address(testPeerDomain).KeyURL(testPeerKey.ID()): fmt.Errorf("%w: HTTP 503", net.ErrUnavailable),
	}
	h, err := buildDomainHandler(dc, serveOpts(fetcher))
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	resp := doReq(t, http.MethodGet, srv.URL+net.WhoPath, nil, bearer(t, testServeDomain))
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestNetServeAuthoritySupplementsDefault(t *testing.T) {
	// --authority supplements the default trust list: a sender
	// endorsed by the DEFAULT authority (lookup.gobl.org) must still
	// be accepted when extras are configured.
	defaultAuthority := net.Address("lookup.gobl.org")
	defaultKey := dsig.NewES256Key()

	dc := writeServeDomain(t, t.TempDir())
	fetcher := peerFetcher(t)
	fetcher.data[defaultAuthority.KeyURL(defaultKey.ID())] = jwkBytes(t, defaultKey)
	fetcher.data[net.Address(testPeerDomain).WhoURL()] = peerWhoBytes(t, defaultKey, defaultAuthority, defaultAuthority)

	opts := serveOpts(fetcher) // configures the testAuthority extra
	h, err := buildDomainHandler(dc, opts)
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	env := signedNoteTo(t, "endorsed by the default authority", testServeDomain)
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
	assert.Equal(t, http.StatusAccepted, resp.StatusCode)
}

func TestNetServeEndorsementUnavailable(t *testing.T) {
	// The sender's who endpoint cannot be reached while checking the
	// endorsement: 503, not a permanent 403.
	dc := writeServeDomain(t, t.TempDir())
	fetcher := peerFetcher(t)
	fetcher.errs = map[string]error{
		net.Address(testPeerDomain).WhoURL(): fmt.Errorf("%w: HTTP 503", net.ErrUnavailable),
	}
	h, err := buildDomainHandler(dc, serveOpts(fetcher))
	require.NoError(t, err)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	env := signedNoteTo(t, "transient who outage", testServeDomain)
	resp := doReq(t, http.MethodPost, srv.URL+net.InboxPath, marshalEnv(t, env), bearer(t, testServeDomain))
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}
