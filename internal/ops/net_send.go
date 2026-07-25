package ops

import (
	"context"
	"encoding/json"
	"io"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/dsig"
	"github.com/invopop/gobl/net"
)

// NetSendOptions configures the gobl net send command.
type NetSendOptions struct {
	Input   io.Reader
	To      net.Address
	From    net.Address      // sender's GOBL Net address (mints the request token)
	FromKey *dsig.PrivateKey // sender's signing key
	Fetcher net.Fetcher      // optional; defaults to net.NewHTTPFetcher()
}

// NetSend reads a GOBL envelope from opts.Input and POSTs it to the
// destination address's inbox endpoint with a request token minted
// from the --from identity. Returns net.ErrInboxRejected if the inbox
// does not respond with 202.
func NetSend(ctx context.Context, opts *NetSendOptions) error {
	if opts.To == "" {
		return gobl.ErrInput.WithReason("destination address is required")
	}
	if opts.From == "" || opts.FromKey == nil {
		return gobl.ErrInput.WithReason("a --from identity (with its private key) is required to authenticate the request")
	}

	body, err := io.ReadAll(cancelableReader(ctx, opts.Input))
	if err != nil {
		return gobl.ErrInput.WithCause(err)
	}

	env := new(gobl.Envelope)
	if err := json.Unmarshal(body, env); err != nil {
		return gobl.ErrInput.WithCause(err)
	}
	if err := env.Validate(); err != nil {
		return gobl.ErrValidation.WithCause(err)
	}

	client := netClientFor(opts.From, opts.FromKey, opts.Fetcher)
	return client.Send(ctx, opts.To, env)
}
