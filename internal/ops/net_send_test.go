package ops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/head"
	"github.com/invopop/gobl/net"
	"github.com/invopop/gobl/note"
	"github.com/invopop/gobl/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func signedNoteEnvelope(t *testing.T, content string) []byte {
	t.Helper()
	msg := &note.Message{Content: content}
	msg.SetUUID(uuid.V7())
	env, err := gobl.Envelop(msg)
	require.NoError(t, err)
	require.NoError(t, env.Sign(testPeerKey, head.WithIssuer(net.Address(testPeerDomain).URI()), head.WithAudience(net.Address(testServeDomain).URI())))
	body, err := json.Marshal(env)
	require.NoError(t, err)
	return body
}

func sendOpts(t *testing.T, body []byte, srvURL string) *NetSendOptions {
	t.Helper()
	return &NetSendOptions{
		Input:   bytes.NewReader(body),
		To:      net.Address(testServeDomain),
		From:    net.Address(testPeerDomain),
		FromKey: testPeerKey,
		Fetcher: routeTo(srvURL),
	}
}

func TestNetSendSuccess(t *testing.T) {
	body := signedNoteEnvelope(t, "round trip")

	var received []byte
	var receivedContentType, receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, net.InboxPath, r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		receivedContentType = r.Header.Get("Content-Type")
		receivedAuth = r.Header.Get("Authorization")
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	err := NetSend(context.Background(), sendOpts(t, body, srv.URL))
	require.NoError(t, err)
	assert.Equal(t, "application/json", receivedContentType)
	assert.True(t, strings.HasPrefix(receivedAuth, "Bearer "), "the request carries a bearer token")
	assert.JSONEq(t, string(body), string(received))
}

func TestNetSendRejectsBadEnvelope(t *testing.T) {
	err := NetSend(context.Background(), &NetSendOptions{
		Input:   bytes.NewReader([]byte("not json")),
		To:      net.Address(testServeDomain),
		From:    net.Address(testPeerDomain),
		FromKey: testPeerKey,
	})
	require.Error(t, err)
}

func TestNetSendNon202(t *testing.T) {
	body := signedNoteEnvelope(t, "bad sig path")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "no thanks", http.StatusUnauthorized)
	}))
	defer srv.Close()

	err := NetSend(context.Background(), sendOpts(t, body, srv.URL))
	require.Error(t, err)
	assert.True(t, errors.Is(err, net.ErrInboxRejected))
}

func TestNetSendMissingFrom(t *testing.T) {
	err := NetSend(context.Background(), &NetSendOptions{
		Input: bytes.NewReader(signedNoteEnvelope(t, "x")),
		To:    net.Address(testServeDomain),
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--from identity")
}

func TestNetSendMissingTo(t *testing.T) {
	err := NetSend(context.Background(), &NetSendOptions{
		Input:   bytes.NewReader(signedNoteEnvelope(t, "x")),
		From:    net.Address(testPeerDomain),
		FromKey: testPeerKey,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "destination address is required")
}

func TestNetSendInvalidEnvelopeJSON(t *testing.T) {
	// Looks like JSON but fails to unmarshal into Envelope.
	err := NetSend(context.Background(), &NetSendOptions{
		Input:   bytes.NewReader([]byte("[1,2,3]")),
		To:      net.Address("example.com"),
		From:    net.Address(testPeerDomain),
		FromKey: testPeerKey,
	})
	require.Error(t, err)
}

func TestNetSendInputReadError(t *testing.T) {
	err := NetSend(context.Background(), &NetSendOptions{
		Input:   errReader{},
		To:      net.Address("example.com"),
		From:    net.Address(testPeerDomain),
		FromKey: testPeerKey,
	})
	require.Error(t, err)
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

func TestNetSendInvalidAddress(t *testing.T) {
	body := signedNoteEnvelope(t, "x")
	err := NetSend(context.Background(), &NetSendOptions{
		Input:   bytes.NewReader(body),
		To:      net.Address("localhost"), // single label fails FQDN validation
		From:    net.Address(testPeerDomain),
		FromKey: testPeerKey,
	})
	require.Error(t, err)
}

func TestNetSendTransportError(t *testing.T) {
	body := signedNoteEnvelope(t, "x")
	// Route to a closed loopback port so the POST fails.
	err := NetSend(context.Background(), &NetSendOptions{
		Input:   bytes.NewReader(body),
		To:      net.Address(testServeDomain),
		From:    net.Address(testPeerDomain),
		FromKey: testPeerKey,
		Fetcher: routeTo("http://127.0.0.1:1"),
	})
	require.Error(t, err)
}

func TestNetSendRoundTrip(t *testing.T) {
	srv, dc := setupNetServer(t)

	body := signedNoteEnvelope(t, "round trip via serve")
	err := NetSend(context.Background(), sendOpts(t, body, srv.URL))
	require.NoError(t, err)

	// Confirm the envelope landed in the inbox directory.
	files, err := readDirNames(dc.InboxDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.True(t, strings.HasSuffix(files[0], ".json"))
}

func TestNetRequestsAndApprove(t *testing.T) {
	// Two identities: owner defers /who disclosure; requester asks,
	// gets 202, and later receives the owner's party in its inbox once
	// the owner approves.
	ctx := context.Background()
	ownerCfg := t.TempDir()
	requesterCfg := t.TempDir()
	const owner = "owner.example"
	const requester = "requester.example"
	initTestDomain(t, ownerCfg, owner)
	initTestDomain(t, requesterCfg, requester)
	ownerKey := domainPrivateKey(t, ownerCfg, owner)
	requesterKey := domainPrivateKey(t, requesterCfg, requester)

	ownerDC := domainConfigFor(ownerCfg, owner)
	requesterDC := domainConfigFor(requesterCfg, requester)
	require.NoError(t, os.WriteFile(ownerDC.WhoDeferredFile, nil, 0o644))

	// The owner's server verifies the requester's tokens; the
	// requester's server verifies the owner's token and party
	// signature.
	ownerSrvH, err := buildDomainHandler(ownerDC, serveOpts(&mapFetcher{data: map[string][]byte{
		net.Address(requester).KeyURL(requesterKey.ID()): jwkBytes(t, requesterKey),
	}}))
	require.NoError(t, err)
	ownerSrv := httptest.NewServer(ownerSrvH)
	defer ownerSrv.Close()

	requesterSrvH, err := buildDomainHandler(requesterDC, serveOpts(&mapFetcher{data: map[string][]byte{
		net.Address(owner).KeyURL(ownerKey.ID()): jwkBytes(t, ownerKey),
	}}))
	require.NoError(t, err)
	requesterSrv := httptest.NewServer(requesterSrvH)
	defer requesterSrv.Close()

	// 1. The requester asks who the owner is and is deferred.
	_, err = NetWho(ctx, &NetWhoOptions{
		Target:    owner,
		From:      requester,
		FromKey:   requesterKey,
		ConfigDir: requesterCfg,
		Fetcher:   routeTo(ownerSrv.URL),
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, net.ErrPending))

	// 2. The owner sees the pending request.
	out := new(bytes.Buffer)
	require.NoError(t, NetRequests(&NetRequestsOptions{
		ConfigDir: ownerCfg,
		Domain:    owner,
		Out:       out,
	}))
	var requests []whoRequest
	require.NoError(t, json.Unmarshal(out.Bytes(), &requests))
	require.Len(t, requests, 1)
	assert.Equal(t, net.Address(requester), requests[0].Requester)

	// 3. The owner approves: its party envelope is delivered to the
	// requester's inbox, bound to the requester.
	require.NoError(t, NetApprove(ctx, &NetApproveOptions{
		ConfigDir: ownerCfg,
		Domain:    owner,
		Requester: requester,
		Fetcher:   routeTo(requesterSrv.URL),
		Log:       discardLog(),
	}))

	// The request record is cleared...
	out.Reset()
	require.NoError(t, NetRequests(&NetRequestsOptions{
		ConfigDir: ownerCfg,
		Domain:    owner,
		Out:       out,
	}))
	requests = nil
	require.NoError(t, json.Unmarshal(out.Bytes(), &requests))
	assert.Empty(t, requests)

	// ...the requester's pending marker was consumed by the delivery...
	assert.NoFileExists(t, filepath.Join(requesterDC.WhoPendingDir, owner))

	// ...and the owner's party envelope landed in the requester's
	// inbox, audience-bound to the requester.
	files, err := os.ReadDir(requesterDC.InboxDir)
	require.NoError(t, err)
	require.Len(t, files, 1)
	data, err := os.ReadFile(filepath.Join(requesterDC.InboxDir, files[0].Name()))
	require.NoError(t, err)
	env := new(gobl.Envelope)
	require.NoError(t, json.Unmarshal(data, env))
	p, err := headSignedPayload(env)
	require.NoError(t, err)
	assert.Equal(t, net.Address(owner).URI(), p.Iss)
	assert.Equal(t, net.Address(requester).URI(), p.Aud)
}

func TestNetRequestsEmpty(t *testing.T) {
	cfg := t.TempDir()
	initTestDomain(t, cfg, "quiet.example")
	out := new(bytes.Buffer)
	require.NoError(t, NetRequests(&NetRequestsOptions{
		ConfigDir: cfg,
		Domain:    "quiet.example",
		Out:       out,
	}))
	assert.JSONEq(t, "[]", out.String())
}

func TestNetRequestsMissingDomain(t *testing.T) {
	err := NetRequests(&NetRequestsOptions{ConfigDir: t.TempDir(), Out: new(bytes.Buffer)})
	require.Error(t, err)
}

func TestNetApproveMissingArgs(t *testing.T) {
	require.Error(t, NetApprove(context.Background(), &NetApproveOptions{ConfigDir: t.TempDir()}))
	require.Error(t, NetApprove(context.Background(), &NetApproveOptions{ConfigDir: t.TempDir(), Domain: "a.example"}))
}
