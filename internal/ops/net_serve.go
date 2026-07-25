package ops

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	stdnet "net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/cal"
	"github.com/invopop/gobl/dsig"
	"github.com/invopop/gobl/head"
	"github.com/invopop/gobl/net"
	"github.com/invopop/gobl/org"
	"github.com/invopop/gobl/uuid"
)

const (
	netServeShutdownTimeout = 10 * time.Second
	netInboxMaxBody         = 1 << 20 // 1 MiB

	defaultHTTPPort  = 80
	defaultHTTPSPort = 443

	acmeStagingDirectoryURL = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// NetServeOptions configures the GOBL Net HTTP server.
type NetServeOptions struct {
	// ConfigDir is the base directory whose <domain>/ subdirectories are
	// auto-discovered and served.
	ConfigDir string

	// Authorities supplements the default trusted authority list
	// (net.Authorities, i.e. lookup.gobl.org) for the inbox's
	// sender-endorsement policy: the server trusts the default plus
	// these extras. Incoming envelopes are accepted only from senders
	// whose who identity carries a countersignature from a trusted
	// authority with a confirmed verifier. AllowUnverified relaxes
	// the verifier requirement (sandbox environments and testing);
	// endorsement itself is always required.
	Authorities     []net.Address
	AllowUnverified bool

	Fetcher net.Fetcher  // optional; defaults to net.NewHTTPFetcher()
	Out     io.Writer    // optional; defaults to os.Stdout (reserved for results, currently unused)
	Log     *slog.Logger // optional; defaults to slog.Default()

	// Port overrides (zero means use the default — 80 / 443).
	HTTPPort  int
	HTTPSPort int

	// ACME options. ACMELive and ACMETest are mutually exclusive.
	ACMELive  bool
	ACMETest  bool
	Domain    string // restricts multi-domain discovery to one domain
	ACMEEmail string
	CertDir   string

	// File-based TLS. CertFile and KeyFile must be supplied together.
	CertFile string
	KeyFile  string
}

// domainConfig groups the on-disk paths that make up one GOBL Net
// identity. The directory name is the domain.
type domainConfig struct {
	Domain          string
	KeysDir         string // directory of <kid>.json public JWK files
	PrivateKeyFile  string
	PartyFile       string
	InboxDir        string
	WhoRequestsDir  string // inbound who requests answered 202, awaiting approval
	WhoPendingDir   string // outbound who requests answered 202 by the peer
	WhoDeferredFile string // marker file: defer /who disclosure to operator approval
}

// logger returns the configured slog.Logger, falling back to slog.Default()
// so library callers (and tests) get sensible behaviour without explicit
// wiring.
func (o *NetServeOptions) logger() *slog.Logger {
	if o != nil && o.Log != nil {
		return o.Log
	}
	return slog.Default()
}

// domainConfigFor builds the standard paths for a domain inside configDir.
func domainConfigFor(configDir, domain string) domainConfig {
	dir := filepath.Join(configDir, domain)
	return domainConfig{
		Domain:          domain,
		KeysDir:         filepath.Join(dir, "keys"),
		PrivateKeyFile:  filepath.Join(dir, "private.jwk"),
		PartyFile:       filepath.Join(dir, "party.json"),
		InboxDir:        filepath.Join(dir, "inbox"),
		WhoRequestsDir:  filepath.Join(dir, "who-requests"),
		WhoPendingDir:   filepath.Join(dir, "who-pending"),
		WhoDeferredFile: filepath.Join(dir, "who-deferred"),
	}
}

// discoverDomains lists the immediate subdirectories of configDir (skipping
// "certs") that look like a domain identity (containing a keys/ dir
// and/or a party.json), returning a domainConfig for each.
func discoverDomains(configDir string) ([]domainConfig, error) {
	entries, err := os.ReadDir(configDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("net serve: read config dir: %w", err)
	}
	var out []domainConfig
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "certs" {
			continue
		}
		dc := domainConfigFor(configDir, e.Name())
		if dirExists(dc.KeysDir) || fileExists(dc.PartyFile) {
			out = append(out, dc)
		}
	}
	return out, nil
}

// buildDomainHandler prepares one domain's on-disk state (keys, party,
// inbox) and returns its mux.
//
//   - GET  /keys   — open, serves the published keys.
//   - GET  /who    — authenticated identity lookup (see handleWho).
//   - POST /inbox  — authenticated envelope delivery (see handleInbox).
//
// The who and inbox routes require a request token; keys stay open so
// peers can verify this domain's signatures and tokens.
func buildDomainHandler(dc domainConfig, opts *NetServeOptions) (http.Handler, error) {
	l := opts.logger()
	keysByKID, err := ensureKeys(dc, l)
	if err != nil {
		return nil, err
	}
	priv, err := loadPrivateKeyFile(dc.PrivateKeyFile)
	if err != nil {
		return nil, err
	}
	self := net.Address(dc.Domain)
	if self == "" {
		return nil, errors.New("net serve: domain is required")
	}

	fetcher := opts.Fetcher
	if fetcher == nil {
		fetcher = net.NewHTTPFetcher()
	}
	// WithAuthorities replaces the client's trust list, and this
	// server's contract is to supplement the default, so the list is
	// built as default-plus-extras.
	authorities := append(append([]net.Address{}, net.Authorities...), opts.Authorities...)
	client := net.NewClient(
		net.WithFetcher(fetcher),
		net.WithIdentity(self, priv),
		net.WithAuthorities(authorities...),
	)

	// A domain without a party file is a receive-only account: /who
	// answers 204 and deliveries are unaffected.
	var partyEnvBytes []byte
	if fileExists(dc.PartyFile) {
		partyEnv, err := readPartyEnvelope(dc)
		if err != nil {
			return nil, err
		}
		partyEnvBytes, err = signedPartyBytes(partyEnv, priv, self)
		if err != nil {
			return nil, err
		}
	} else {
		l.Info("who.no_party", "domain", dc.Domain, "party_file", dc.PartyFile)
	}

	if err := os.MkdirAll(dc.InboxDir, 0o755); err != nil {
		return nil, fmt.Errorf("net serve: create inbox dir: %w", err)
	}

	deferred := fileExists(dc.WhoDeferredFile)

	jwksBytes, keyCount, err := buildJWKS(keysByKID)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+net.KeysPath+"/{kid}", handleKey(l, keysByKID))
	mux.HandleFunc("GET "+net.JWKSPath, handleJWKS(l, jwksBytes, keyCount))
	mux.Handle("GET "+net.WhoPath, requireAuth(l, client, self,
		handleWho(l, partyEnvBytes, deferred, dc.WhoRequestsDir)))
	mux.Handle("POST "+net.InboxPath, requireAuth(l, client, self,
		handleInbox(l, client, dc, self, opts.AllowUnverified)))
	return accessLog(l, corsAllowAll(mux)), nil
}

// signedPartyBytes returns the static /who response body: the party
// envelope self-signed by this domain. An envelope already carrying the
// domain's self-signature as its first signature (e.g. countersigned by
// an Authority out-of-band) is served verbatim; anything else is signed
// once at startup with iss=self and no audience.
func signedPartyBytes(env *gobl.Envelope, priv *dsig.PrivateKey, self net.Address) ([]byte, error) {
	signed := false
	if env.Signed() && self != "" {
		if p, err := head.SignedPayload(env.Signatures[0]); err == nil && p.Aud == "" {
			if got, gerr := net.ParseAddress(p.Iss); gerr == nil && got == self {
				signed = true
			}
		}
	}
	if !signed {
		opts := []head.SignOption{}
		if self != "" {
			opts = append(opts, head.WithIssuer(self.String()))
		}
		if err := env.Sign(priv, opts...); err != nil {
			return nil, fmt.Errorf("net serve: sign party: %w", err)
		}
	}
	out, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("net serve: marshal party: %w", err)
	}
	return out, nil
}

// requesterCtxKey carries the verified requester Address through the
// request context.
type requesterCtxKey struct{}

// requesterFrom returns the requester Address stashed by requireAuth,
// or "" when authentication is disabled.
func requesterFrom(r *http.Request) net.Address {
	a, _ := r.Context().Value(requesterCtxKey{}).(net.Address)
	return a
}

// requireAuth enforces the request token (spec §5.5) on who and inbox
// requests: the Authorization bearer token must verify against the
// issuer's published key, be bound to this domain, and be fresh. The
// verified requester is stashed in the request context.
func requireAuth(log *slog.Logger, client *net.Client, self net.Address, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if header == "" {
			log.Warn("auth.rejected", "path", r.URL.Path, "reason", "token_missing", "remote", r.RemoteAddr)
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}
		requester, err := client.VerifyAuthorization(r.Context(), header, self)
		if err != nil {
			// A token that cannot be *checked* (the issuer's key
			// endpoint is unreachable) is not an invalid token: answer
			// 503 so the client retries.
			if errors.Is(err, net.ErrUnavailable) {
				log.Warn("auth.rejected", "path", r.URL.Path, "reason", "token_unavailable", "remote", r.RemoteAddr, "error", err.Error())
				http.Error(w, "could not verify request token: "+err.Error(), http.StatusServiceUnavailable)
				return
			}
			reason := "token_invalid"
			if errors.Is(err, net.ErrTokenExpired) {
				reason = "token_expired"
			}
			log.Warn("auth.rejected", "path", r.URL.Path, "reason", reason, "remote", r.RemoteAddr, "error", err.Error())
			http.Error(w, "invalid request token: "+err.Error(), http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), requesterCtxKey{}, requester)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// buildJWKS materialises the bulk JWK Set response by sorting the
// published keys newest-first (by valid_from descending, with
// UUIDv7 kid descending as a tie-breaker) and wrapping them in the
// standard `{"keys":[...]}` envelope. Returned bytes are ready to be
// served as application/json verbatim.
func buildJWKS(keysByKID map[string][]byte) ([]byte, int, error) {
	type entry struct {
		kid       string
		validFrom *cal.Timestamp
		raw       json.RawMessage
	}
	entries := make([]entry, 0, len(keysByKID))
	for kid, body := range keysByKID {
		pk := new(dsig.PublicKey)
		if err := json.Unmarshal(body, pk); err != nil {
			return nil, 0, fmt.Errorf("net serve: build jwks: parse %s: %w", kid, err)
		}
		entries = append(entries, entry{
			kid:       kid,
			validFrom: pk.ValidFrom,
			raw:       append(json.RawMessage(nil), body...),
		})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		ai, aj := entries[i].validFrom, entries[j].validFrom
		switch {
		case ai != nil && aj != nil:
			if !ai.Equal(aj.Time) {
				return ai.After(aj.Time)
			}
		case ai != nil && aj == nil:
			return true // keys with valid_from sort before keys without
		case ai == nil && aj != nil:
			return false
		}
		// Fall back to kid descending — UUIDv7 kids are time-ordered.
		return entries[i].kid > entries[j].kid
	})
	out := struct {
		Keys []json.RawMessage `json:"keys"`
	}{Keys: make([]json.RawMessage, len(entries))}
	for i, e := range entries {
		out.Keys[i] = e.raw
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, 0, fmt.Errorf("net serve: build jwks: %w", err)
	}
	return b, len(entries), nil
}

// handleJWKS serves the pre-built JWK Set bytes verbatim.
func handleJWKS(log *slog.Logger, body []byte, count int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		log.Info("jwks.served", "count", count)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}
}

// handleKey serves a single published JWK by its kid path value, or 404
// if the kid is not in the domain's published set.
func handleKey(log *slog.Logger, keysByKID map[string][]byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kid := r.PathValue("kid")
		body, ok := keysByKID[kid]
		log.Info("keys.lookup", "kid", kid, "found", ok)
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}
}

// buildRouter returns an HTTP handler dispatching by the request Host
// header to the matching domain's handler.
func buildRouter(domains []domainConfig, opts *NetServeOptions) (http.Handler, error) {
	handlers := make(map[string]http.Handler, len(domains))
	for _, dc := range domains {
		h, err := buildDomainHandler(dc, opts)
		if err != nil {
			return nil, err
		}
		handlers[dc.Domain] = h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := stripPort(r.Host)
		if h, ok := handlers[host]; ok {
			h.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	}), nil
}

func stripPort(host string) string {
	if h, _, err := stdnet.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// ensureKeys returns a map of kid → single-JWK JSON bytes for every
// public key published by this domain. Each key lives in its own file
// at <keysDir>/<kid>.json. If neither the keys directory nor the
// private key file exist, a fresh ECDSA P-256 keypair is generated and
// persisted (single-key bootstrap). If only one of the two exists the
// setup is inconsistent.
func ensureKeys(dc domainConfig, log *slog.Logger) (map[string][]byte, error) {
	keysExists := dirExists(dc.KeysDir)
	privExists := fileExists(dc.PrivateKeyFile)

	switch {
	case keysExists && privExists:
		keysByKID, err := readKeysDir(dc.KeysDir)
		if err != nil {
			return nil, err
		}
		if len(keysByKID) == 0 {
			return nil, fmt.Errorf("net serve: keys directory %s contains no JWKs", dc.KeysDir)
		}
		priv, err := loadPrivateKeyFile(dc.PrivateKeyFile)
		if err != nil {
			return nil, err
		}
		if _, ok := keysByKID[priv.ID()]; !ok {
			return nil, fmt.Errorf("net serve: private key kid %q is not published under %s", priv.ID(), dc.KeysDir)
		}
		return keysByKID, nil

	case !keysExists && !privExists:
		return generateKeypair(dc.KeysDir, dc.PrivateKeyFile, log)

	default:
		present, missing := dc.KeysDir, dc.PrivateKeyFile
		if !keysExists {
			present, missing = dc.PrivateKeyFile, dc.KeysDir
		}
		return nil, fmt.Errorf(
			"net serve: inconsistent key setup — %s exists but %s does not "+
				"(remove both to auto-generate, or supply both)",
			present, missing,
		)
	}
}

// readKeysDir reads each <kid>.json file in dir, validates that the
// JWK's kid matches the filename stem, and returns the raw file bytes
// keyed by kid. Non-JSON entries and subdirectories are ignored so the
// operator can drop sidecar files alongside their keys.
func readKeysDir(dir string) (map[string][]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("net serve: read keys dir: %w", err)
	}
	keysByKID := make(map[string][]byte, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		kid := strings.TrimSuffix(name, ".json")
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("net serve: read %s: %w", name, err)
		}
		pk := new(dsig.PublicKey)
		if err := json.Unmarshal(data, pk); err != nil {
			return nil, fmt.Errorf("net serve: %s: invalid JWK: %w", name, err)
		}
		if pk.ID() != kid {
			return nil, fmt.Errorf("net serve: %s: filename kid %q does not match JWK kid %q", name, kid, pk.ID())
		}
		keysByKID[kid] = data
	}
	return keysByKID, nil
}

// generateKeypair creates an ECDSA P-256 keypair, writes the private
// key to privFile (0600) and the public key to keysDir/<kid>.json
// (stamping valid_from = now), logs the action, and returns the
// per-kid JWK map.
func generateKeypair(keysDir, privFile string, log *slog.Logger) (map[string][]byte, error) {
	if err := os.MkdirAll(filepath.Dir(privFile), 0o700); err != nil {
		return nil, fmt.Errorf("net serve: create config dir: %w", err)
	}
	priv := dsig.NewES256Key()
	privBytes, err := json.MarshalIndent(priv, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("net serve: marshal private key: %w", err)
	}
	if err := os.WriteFile(privFile, privBytes, 0o600); err != nil {
		return nil, fmt.Errorf("net serve: write private key: %w", err)
	}
	pubBytes, err := publishedKeyBytes(priv)
	if err != nil {
		return nil, fmt.Errorf("net serve: marshal public key: %w", err)
	}
	if err := os.MkdirAll(keysDir, 0o755); err != nil {
		return nil, fmt.Errorf("net serve: create keys dir: %w", err)
	}
	keyFile := filepath.Join(keysDir, priv.ID()+".json")
	if err := os.WriteFile(keyFile, pubBytes, 0o644); err != nil {
		return nil, fmt.Errorf("net serve: write key file: %w", err)
	}
	logger(log).Info("generated keypair", "kid", priv.ID(), "private", privFile, "key_file", keyFile)
	return map[string][]byte{priv.ID(): pubBytes}, nil
}

// logger normalises a possibly-nil *slog.Logger to slog.Default().
func logger(l *slog.Logger) *slog.Logger {
	if l != nil {
		return l
	}
	return slog.Default()
}

// dirExists reports whether path exists and is a directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// publishedKeyBytes marshals the public counterpart of priv as a
// dsig.PublicKey with valid_from stamped to the current UTC time,
// floored to the second: signature `iat` claims carry whole seconds,
// so a sub-second valid_from would reject signatures made immediately
// after key generation.
func publishedKeyBytes(priv *dsig.PrivateKey) ([]byte, error) {
	pubJSON, err := json.Marshal(priv.Public())
	if err != nil {
		return nil, err
	}
	pk := new(dsig.PublicKey)
	if err := json.Unmarshal(pubJSON, pk); err != nil {
		return nil, err
	}
	now := cal.TimestampOf(time.Now().UTC().Truncate(time.Second))
	pk.ValidFrom = &now
	return json.Marshal(pk)
}

func loadPrivateKeyFile(path string) (*dsig.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("net serve: read private key: %w", err)
	}
	k := new(dsig.PrivateKey)
	if err := json.Unmarshal(b, k); err != nil {
		return nil, fmt.Errorf("net serve: invalid private key: %w", err)
	}
	return k, nil
}

// readPartyEnvelope reads the domain's party.json (a raw org.Party or an
// envelope, possibly already signed by an external authority) and returns
// it as an unsigned *gobl.Envelope. The /who handler signs a fresh copy
// per request with iss=self, aud=requester.
func readPartyEnvelope(dc domainConfig) (*gobl.Envelope, error) {
	data, err := os.ReadFile(dc.PartyFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf(
				"net serve: party file not found at %s — create one with `gobl init %s` "+
					"or supply a raw org.Party / signed envelope",
				dc.PartyFile, dc.Domain,
			)
		}
		return nil, fmt.Errorf("net serve: read party file: %w", err)
	}

	env := new(gobl.Envelope)
	if err := json.Unmarshal(data, env); err == nil && env.Document != nil && !env.Document.IsEmpty() {
		return env, nil
	}
	// Not an envelope — parse as a raw org.Party and wrap it.
	party := new(org.Party)
	if err := json.Unmarshal(data, party); err != nil {
		return nil, fmt.Errorf("net serve: party file: invalid JSON: %w", err)
	}
	env, err = gobl.Envelop(party)
	if err != nil {
		return nil, fmt.Errorf("net serve: party file: %w", err)
	}
	return env, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// resolveDomains determines which identities to serve: an explicit single
// identity (manual mode) when PartyFile/KeysDir are set, otherwise the
// domains discovered under ConfigDir (optionally filtered by Domain).
func resolveDomains(opts *NetServeOptions) ([]domainConfig, error) {
	if opts.ConfigDir == "" {
		return nil, errors.New("net serve: no config dir configured")
	}
	all, err := discoverDomains(opts.ConfigDir)
	if err != nil {
		return nil, err
	}
	if opts.Domain != "" {
		for _, dc := range all {
			if dc.Domain == opts.Domain {
				return []domainConfig{dc}, nil
			}
		}
		// Not yet on disk — construct it (keys auto-generate; party required).
		return []domainConfig{domainConfigFor(opts.ConfigDir, opts.Domain)}, nil
	}
	return all, nil
}

func domainNames(domains []domainConfig) []string {
	var names []string
	for _, dc := range domains {
		if dc.Domain != "" {
			names = append(names, dc.Domain)
		}
	}
	return names
}

// NetServe runs the GOBL Net HTTP server. It always serves over plain
// HTTP and, when a TLS source is configured, additionally over HTTPS with
// identical content (no HTTP→HTTPS redirect). In the default mode it
// discovers every <domain>/ directory under ConfigDir and routes requests
// by the HTTP Host header. The server shuts down gracefully on ctx cancel.
func NetServe(ctx context.Context, opts *NetServeOptions) error {
	if opts.Out == nil {
		opts.Out = os.Stdout
	}

	domains, err := resolveDomains(opts)
	if err != nil {
		return err
	}
	if len(domains) == 0 {
		return gobl.ErrInput.WithReason("net serve: no domains configured — run `gobl init <domain>` first")
	}

	log := opts.logger()
	router, err := buildRouter(domains, opts)
	if err != nil {
		return err
	}

	httpHandler := router
	var tlsConfig *tls.Config

	switch {
	case opts.ACMELive || opts.ACMETest:
		names := domainNames(domains)
		m := newAutocertManager(opts, names)
		httpHandler = m.HTTPHandler(router)
		tlsConfig = m.TLSConfig()
		log.Info("ACME enabled", "domains", names)
	case opts.CertFile != "" && opts.KeyFile != "":
		cert, err := tls.LoadX509KeyPair(opts.CertFile, opts.KeyFile)
		if err != nil {
			return fmt.Errorf("net serve: load TLS keypair: %w", err)
		}
		tlsConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
	}

	httpPort := opts.HTTPPort
	if httpPort == 0 {
		httpPort = defaultHTTPPort
	}
	httpsPort := opts.HTTPSPort
	if httpsPort == 0 {
		httpsPort = defaultHTTPSPort
	}

	httpLn, err := listenTCP(httpPort)
	if err != nil {
		return err
	}

	var httpsLn stdnet.Listener
	if tlsConfig != nil {
		httpsLn, err = listenTCP(httpsPort)
		if err != nil {
			_ = httpLn.Close()
			return err
		}
	}

	return serveOnListeners(ctx, opts, httpHandler, router, tlsConfig, httpLn, httpsLn)
}

// serveOnListeners runs the HTTP (and optionally HTTPS) servers on the
// provided listeners. Both listeners are closed by the http.Server lifecycle.
func serveOnListeners(
	ctx context.Context,
	opts *NetServeOptions,
	httpHandler http.Handler,
	httpsHandler http.Handler,
	tlsConfig *tls.Config,
	httpLn stdnet.Listener,
	httpsLn stdnet.Listener,
) error {
	httpSrv := &http.Server{
		Handler:           httpHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	var httpsSrv *http.Server
	if httpsLn != nil {
		httpsSrv = &http.Server{
			Handler:           httpsHandler,
			TLSConfig:         tlsConfig,
			ReadHeaderTimeout: 10 * time.Second,
		}
	}

	srvCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	log := opts.logger()
	log.Info("GOBL Net listening", "scheme", "http", "addr", httpLn.Addr().String())
	if httpsLn != nil {
		log.Info("GOBL Net listening", "scheme", "https", "addr", httpsLn.Addr().String())
	}

	errCh := make(chan error, 2)
	go func() {
		err := httpSrv.Serve(httpLn)
		if !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http: %w", err)
			cancel()
			return
		}
		errCh <- nil
	}()
	if httpsSrv != nil {
		go func() {
			err := httpsSrv.ServeTLS(httpsLn, "", "")
			if !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("https: %w", err)
				cancel()
				return
			}
			errCh <- nil
		}()
	}

	<-srvCtx.Done()
	log.Info("Shutting down")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), netServeShutdownTimeout)
	defer shutdownCancel()
	_ = httpSrv.Shutdown(shutdownCtx)
	if httpsSrv != nil {
		_ = httpsSrv.Shutdown(shutdownCtx)
	}

	expected := 1
	if httpsSrv != nil {
		expected = 2
	}
	var firstErr error
	for i := 0; i < expected; i++ {
		if err := <-errCh; err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func newAutocertManager(opts *NetServeOptions, domains []string) *autocert.Manager {
	certDir := opts.CertDir
	if certDir == "" {
		certDir = "certs"
	}
	m := &autocert.Manager{
		Cache:      autocert.DirCache(certDir),
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(domains...),
		Email:      opts.ACMEEmail,
	}
	if opts.ACMETest {
		m.Client = &acme.Client{DirectoryURL: acmeStagingDirectoryURL}
	}
	return m
}

// listenTCP binds to the requested port on all interfaces. On EACCES it
// returns a wrapped error that guides the operator to a fix.
func listenTCP(port int) (stdnet.Listener, error) {
	addr := ":" + strconv.Itoa(port)
	ln, err := stdnet.Listen("tcp", addr)
	if err == nil {
		return ln, nil
	}
	if errors.Is(err, syscall.EACCES) {
		return nil, fmt.Errorf(
			"net serve: cannot bind %s — permission denied. "+
				"Use --http-port / --https-port to pick an unprivileged port, "+
				"grant the binary CAP_NET_BIND_SERVICE "+
				"(setcap 'cap_net_bind_service=+ep' <binary>), or run with sudo / "+
				"inside a container that maps the host port externally",
			addr,
		)
	}
	return nil, fmt.Errorf("net serve: listen %s: %w", addr, err)
}

func serveBytes(body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write(body)
	}
}

// whoRequest is the record written for each deferred /who request so
// the operator can review and approve it later (`gobl net requests`,
// `gobl net approve`).
type whoRequest struct {
	Requester net.Address `json:"requester"`
	Time      string      `json:"time"`
}

// recordWhoRequest persists a deferred who request as
// <dir>/<requester>.json. The requester is a canonicalized FQDN, so it
// is safe as a filename component.
func recordWhoRequest(dir string, requester net.Address) error {
	if dir == "" {
		return fmt.Errorf("net serve: no who-requests directory configured")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(whoRequest{
		Requester: requester,
		Time:      time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, string(requester)+".json"), data, 0o644)
}

// handleWho answers an authenticated identity lookup (GET). The
// response is the domain's static self-signed party envelope; the
// request token identifies the requester for the audit log. A domain
// without a party file answers 204 (receive-only); a domain with
// deferred disclosure records the request and answers 202 — the owner
// may later deliver its party to the requester's inbox (`gobl net
// approve`).
func handleWho(log *slog.Logger, partyEnvBytes []byte, deferred bool, requestsDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requester := requesterFrom(r)
		if len(partyEnvBytes) == 0 {
			log.Info("who.served", "requester", string(requester), "status", http.StatusNoContent)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if deferred {
			if err := recordWhoRequest(requestsDir, requester); err != nil {
				log.Error("who.request_write_failed", "requester", string(requester), "error", err.Error())
				http.Error(w, "could not record request", http.StatusInternalServerError)
				return
			}
			log.Info("who.deferred", "requester", string(requester))
			w.WriteHeader(http.StatusAccepted)
			return
		}
		log.Info("who.served", "requester", string(requester), "status", http.StatusOK)
		w.Header().Set("Cache-Control", "private")
		serveBytes(partyEnvBytes)(w, r)
	})
}

// handleInbox accepts a signed envelope delivery. The request token
// (checked by requireAuth) authenticates the transmitting peer — which
// may differ from the envelope's signer when a trusted intermediary
// delivers on the signer's behalf. The envelope's own signature and
// audience are verified independently, and the sender-endorsement
// policy applies to the envelope's signer: an authority
// countersignature is always required, with a confirmed verifier
// unless allowUnverified relaxes it. A party envelope answering one of
// our own deferred who requests (who-pending) is accepted without
// endorsement.
func handleInbox(log *slog.Logger, client *net.Client, dc domainConfig, selfAddr net.Address, allowUnverified bool) http.Handler {
	dir := dc.InboxDir
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, netInboxMaxBody))
		if err != nil {
			log.Warn("inbox.rejected", "reason", "read_body", "remote", r.RemoteAddr, "error", err.Error())
			http.Error(w, "could not read body", http.StatusBadRequest)
			return
		}

		env := new(gobl.Envelope)
		if err := json.Unmarshal(body, env); err != nil {
			log.Warn("inbox.rejected", "reason", "bad_body", "remote", r.RemoteAddr)
			http.Error(w, "invalid envelope JSON", http.StatusBadRequest)
			return
		}

		if err := env.Validate(); err != nil {
			log.Warn("inbox.rejected", "reason", "validation", "remote", r.RemoteAddr, "error", err.Error())
			http.Error(w, "envelope failed validation: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}

		sender, err := client.VerifyEnvelope(r.Context(), env, "")
		if err != nil {
			if errors.Is(err, net.ErrUnavailable) {
				log.Warn("inbox.rejected", "reason", "verify_unavailable", "remote", r.RemoteAddr, "error", err.Error())
				http.Error(w, "could not verify envelope: "+err.Error(), http.StatusServiceUnavailable)
				return
			}
			log.Warn("inbox.rejected", "reason", "verify_failed", "remote", r.RemoteAddr, "error", err.Error())
			http.Error(w, "signature verification failed: "+err.Error(), http.StatusUnauthorized)
			return
		}
		// Inboxes require the envelope to be bound to this address. A
		// missing or mismatched aud is rejected so the same valid
		// envelope cannot be replayed against a different inbox.
		p, perr := head.SignedPayload(env.Signatures[0])
		if perr != nil {
			log.Warn("inbox.rejected", "reason", "verify_failed", "caller", string(sender), "error", perr.Error())
			http.Error(w, "could not read signed payload", http.StatusUnauthorized)
			return
		}
		if p.Aud == "" {
			log.Warn("inbox.rejected", "reason", "aud_missing", "caller", string(sender))
			http.Error(w, "envelope must be signed with an audience matching this inbox", http.StatusUnauthorized)
			return
		}
		// Canonicalize both sides so U-Label or trailing-dot forms
		// compare equal, mirroring gobl's VerifyEnvelope.
		if aud, aerr := net.ParseAddress(p.Aud); aerr != nil || aud != selfAddr {
			log.Warn("inbox.rejected", "reason", "aud_mismatch", "caller", string(sender), "aud", p.Aud)
			http.Error(w, "envelope audience does not match this inbox", http.StatusUnauthorized)
			return
		}
		// A self-signed party envelope from an address we have an
		// outstanding who request to fulfils that request (spec §8.3)
		// and needs no endorsement — it carries exactly what a 200 who
		// response would.
		pendingFile := filepath.Join(dc.WhoPendingDir, string(sender))
		if _, isParty := env.Extract().(*org.Party); isParty && dc.WhoPendingDir != "" && fileExists(pendingFile) {
			_ = os.Remove(pendingFile)
			log.Info("who.fulfilled", "caller", string(sender))
		} else if _, err := client.VerifySender(r.Context(), sender, !allowUnverified); err != nil {
			// A transient failure to resolve the sender's who or a
			// verifier key must not read as a permanent rejection.
			if errors.Is(err, net.ErrUnavailable) {
				log.Warn("inbox.rejected", "reason", "verify_unavailable", "caller", string(sender), "error", err.Error())
				http.Error(w, "could not verify sender endorsement: "+err.Error(), http.StatusServiceUnavailable)
				return
			}
			log.Warn("inbox.rejected", "reason", "not_endorsed", "caller", string(sender), "error", err.Error())
			http.Error(w, "sender is not endorsed: "+err.Error(), http.StatusForbidden)
			return
		}

		// Re-parse the UUID before using it as a filename component.
		// env.Validate() above has already rejected non-UUID values,
		// but re-parsing here is defence-in-depth: any future change
		// that weakens upstream validation cannot let a path-traversal
		// payload reach filepath.Join. The parsed canonical form is
		// guaranteed to match [0-9a-f-]{36}.
		parsedUUID, err := uuid.Parse(env.Head.UUID.String())
		if err != nil {
			log.Warn("inbox.rejected", "reason", "malformed_uuid", "caller", string(sender), "error", err.Error())
			http.Error(w, "envelope UUID is malformed", http.StatusUnprocessableEntity)
			return
		}
		filename := filepath.Join(dir, parsedUUID.String()+".json")
		f, err := os.Create(filename)
		if err != nil {
			log.Error("inbox.write_failed", "caller", string(sender), "envelope", parsedUUID.String(), "error", err.Error())
			http.Error(w, "could not write inbox file", http.StatusInternalServerError)
			return
		}
		defer f.Close() //nolint:errcheck
		if _, err := f.Write(body); err != nil {
			log.Error("inbox.write_failed", "caller", string(sender), "envelope", parsedUUID.String(), "error", err.Error())
			http.Error(w, "could not write inbox file", http.StatusInternalServerError)
			return
		}

		log.Info("inbox.accepted", "caller", string(sender), "envelope", parsedUUID.String())
		w.WriteHeader(http.StatusAccepted)
	})
}
