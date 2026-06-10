package ops

import (
	"context"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/dsig"
	"github.com/invopop/gobl/head"
)

// SignOptions are the options used for signing a GOBL document.
type SignOptions struct {
	*ParseOptions
	PrivateKey *dsig.PrivateKey

	// Issuer is the signer's verifiable GOBL Net address (a gobl: URI) and
	// Audience is the optional GOBL Net audience the signature is bound to;
	// either may be empty.
	Issuer   cbc.URI
	Audience cbc.URI
}

// Sign parses a GOBL document into an envelope, performs calculations,
// validates it, and finally signs its headers. The parsed envelope *must* be a
// draft, or else an error is returned.
func Sign(ctx context.Context, opts *SignOptions) (*gobl.Envelope, error) {
	// Always envelop incoming data.
	opts.Envelop = true

	obj, err := parseGOBLData(ctx, opts.ParseOptions)
	if err != nil {
		return nil, gobl.ErrInternal.WithCause(err)
	}

	env, ok := obj.(*gobl.Envelope)
	if !ok {
		panic("parsed sign data must be an envelope")
	}

	if err := env.Calculate(); err != nil {
		return nil, gobl.ErrInternal.WithCause(err)
	}

	// Sign envelope headers. Validation is done transparently in `Sign`.
	var signOpts []head.SignOption
	if opts.Issuer != "" {
		signOpts = append(signOpts, head.WithIssuer(opts.Issuer))
	}
	if opts.Audience != "" {
		signOpts = append(signOpts, head.WithAudience(opts.Audience))
	}
	if err := env.Sign(opts.PrivateKey, signOpts...); err != nil {
		return nil, gobl.ErrInternal.WithCause(err)
	}

	return env, nil
}
