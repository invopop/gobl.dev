package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/head"
	"github.com/invopop/gobl/net"
	"github.com/invopop/gobl/org"
)

// NetRequestsOptions configures NetRequests.
type NetRequestsOptions struct {
	ConfigDir string
	Domain    string
	Out       io.Writer
}

// NetRequests writes the domain's pending deferred who requests —
// requesters answered 202 and awaiting operator approval — as a JSON
// array to opts.Out.
func NetRequests(opts *NetRequestsOptions) error {
	if opts.Domain == "" {
		return gobl.ErrInput.WithReason("a --domain identity is required")
	}
	dc := domainConfigFor(opts.ConfigDir, opts.Domain)
	requests, err := listWhoRequests(dc.WhoRequestsDir)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(opts.Out)
	enc.SetIndent("", "  ")
	return enc.Encode(requests)
}

// listWhoRequests reads every <requester>.json record in dir. An
// absent directory means no pending requests.
func listWhoRequests(dir string) ([]whoRequest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []whoRequest{}, nil
		}
		return nil, fmt.Errorf("net requests: %w", err)
	}
	out := make([]whoRequest, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("net requests: %w", err)
		}
		req := new(whoRequest)
		if err := json.Unmarshal(data, req); err != nil {
			return nil, fmt.Errorf("net requests: %s: %w", e.Name(), err)
		}
		out = append(out, *req)
	}
	return out, nil
}

// NetApproveOptions configures NetApprove.
type NetApproveOptions struct {
	ConfigDir string
	Domain    string      // the identity whose who request is being approved
	Requester net.Address // the requester to disclose the party to
	Fetcher   net.Fetcher
	Log       *slog.Logger
}

// NetApprove approves a deferred who request: the domain's party
// envelope is signed with iss=self and aud=requester and delivered to
// the requester's inbox, then the pending request record is removed.
func NetApprove(ctx context.Context, opts *NetApproveOptions) error {
	if opts.Domain == "" {
		return gobl.ErrInput.WithReason("a --domain identity is required")
	}
	if opts.Requester == "" {
		return gobl.ErrInput.WithReason("a requester address is required")
	}
	log := logger(opts.Log)
	dc := domainConfigFor(opts.ConfigDir, opts.Domain)
	self := net.Address(dc.Domain)

	priv, err := loadPrivateKeyFile(dc.PrivateKeyFile)
	if err != nil {
		return err
	}
	env, err := readPartyEnvelope(dc)
	if err != nil {
		return err
	}
	// Receiving inboxes check the first signature's audience, so a
	// pre-signed envelope is rebuilt around its party document to make
	// the audience-bound signature the first one.
	if env.Signed() {
		party, ok := env.Extract().(*org.Party)
		if !ok {
			return fmt.Errorf("net approve: party file does not contain an org.Party")
		}
		if env, err = gobl.Envelop(party); err != nil {
			return fmt.Errorf("net approve: %w", err)
		}
	}
	if err := env.Sign(priv,
		head.WithIssuer(self.String()),
		head.WithAudience(opts.Requester.String())); err != nil {
		return fmt.Errorf("net approve: sign party: %w", err)
	}

	client := netClientFor(self, priv, opts.Fetcher)
	if err := client.Send(ctx, opts.Requester, env); err != nil {
		return err
	}

	record := filepath.Join(dc.WhoRequestsDir, string(opts.Requester)+".json")
	if err := os.Remove(record); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("net approve: clear request record: %w", err)
	}
	log.Info("who.approved", "requester", string(opts.Requester))
	return nil
}
