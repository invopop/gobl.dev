package ops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/dsig"
	"github.com/invopop/gobl/net"
)

// netClientFor builds a net.Client authenticated as from.
func netClientFor(from net.Address, key *dsig.PrivateKey, fetcher net.Fetcher) *net.Client {
	if fetcher == nil {
		fetcher = net.NewHTTPFetcher()
	}
	return net.NewClient(
		net.WithFetcher(fetcher),
		net.WithIdentity(from, key),
	)
}

// NetWhoOptions configures NetWho.
type NetWhoOptions struct {
	Target    net.Address      // domain being queried
	From      net.Address      // caller's GOBL Net address (mints the request token)
	FromKey   *dsig.PrivateKey // caller's signing key
	ConfigDir string           // optional; records deferred (202) requests under <ConfigDir>/<From>/who-pending/
	Fetcher   net.Fetcher      // optional; defaults to net.NewHTTPFetcher()
}

// NetWho performs an authenticated GOBL Net identity lookup: it GETs
// the target's /who endpoint with a request token minted from the
// --from identity and verifies the returned party envelope (signature,
// issuer, document type) via net.Client.Who. On a 202 the request was
// recorded by the owner for deferred disclosure: the pending state is
// noted under the caller's config directory — so the inbox accepts the
// party envelope the owner may deliver later — and net.ErrPending is
// returned for the caller to handle.
func NetWho(ctx context.Context, opts *NetWhoOptions) (*gobl.Envelope, error) {
	if opts.Target == "" {
		return nil, gobl.ErrInput.WithReason("target address is required")
	}
	if opts.From == "" || opts.FromKey == nil {
		return nil, gobl.ErrInput.WithReason("a --from identity (with its private key) is required to authenticate the request")
	}

	client := netClientFor(opts.From, opts.FromKey, opts.Fetcher)
	env, err := client.Who(ctx, opts.Target)
	if err != nil {
		if errors.Is(err, net.ErrPending) && opts.ConfigDir != "" {
			if werr := recordWhoPending(opts.ConfigDir, opts.From, opts.Target); werr != nil {
				return nil, fmt.Errorf("net who: record pending request: %w", werr)
			}
		}
		return nil, err
	}
	return env, nil
}

// recordWhoPending marks an outbound who request as deferred (202) so
// the domain's inbox will accept the party envelope the target may
// deliver later without requiring endorsement.
func recordWhoPending(configDir string, from, target net.Address) error {
	dir := filepath.Join(configDir, string(from), "who-pending")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, string(target)), nil, 0o644)
}
