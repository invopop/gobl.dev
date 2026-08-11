package ops

import (
	"context"
	"io"

	jsonyaml "github.com/invopop/yaml"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/dsig"
	"github.com/invopop/gobl/head"
	"github.com/invopop/gobl/net"
	"github.com/invopop/gobl/org"
)

// Verify reads a GOBL document from in, and returns an error if there are any
// validation errors.
func Verify(ctx context.Context, in io.Reader, key *dsig.PublicKey) error {
	body, err := io.ReadAll(cancelableReader(ctx, in))
	if err != nil {
		return gobl.ErrInput.WithCause(err)
	}
	env := new(gobl.Envelope)
	if err := jsonyaml.Unmarshal(body, env); err != nil {
		return gobl.ErrInput.WithCause(err)
	}
	if err := env.Validate(); err != nil {
		return gobl.ErrValidation.WithCause(err)
	}
	if key == nil {
		return gobl.ErrInput.WithReason("public key required")
	}
	if !env.Signed() {
		return gobl.ErrSignature.WithReason("envelope is not signed")
	}
	if err := env.Signatures[0].VerifyPayload(key, env); err != nil {
		return gobl.ErrSignature.WithCause(err)
	}
	return nil
}

// VerifyRemote reads a GOBL envelope and verifies it using remote
// JWKS discovery via the GOBL Net client.
func VerifyRemote(ctx context.Context, in io.Reader, client *net.Client, addr net.Address) error {
	body, err := io.ReadAll(cancelableReader(ctx, in))
	if err != nil {
		return gobl.ErrInput.WithCause(err)
	}
	env := new(gobl.Envelope)
	if err := jsonyaml.Unmarshal(body, env); err != nil {
		return gobl.ErrInput.WithCause(err)
	}
	if err := env.Validate(); err != nil {
		return gobl.ErrValidation.WithCause(err)
	}
	if _, ok := env.Extract().(*org.Party); ok {
		subject, err := client.VerifyParty(ctx, env)
		if err != nil {
			return gobl.ErrValidation.WithCause(err)
		}
		if addr != "" && subject != addr {
			return gobl.ErrValidation.WithReason("party envelope belongs to %s, expected %s", subject, addr)
		}
		return nil
	}
	// Documents: every signature must verify against its issuer's
	// published key; with an expected address, at least one signature
	// must be its.
	if !env.Signed() {
		return gobl.ErrValidation.WithReason("envelope is not signed")
	}
	found := false
	for i, sig := range env.Signatures {
		p, err := head.SignedPayload(sig)
		if err != nil {
			return gobl.ErrValidation.WithReason("signature %d: unreadable payload", i)
		}
		iss, err := net.ParseAddress(p.Iss)
		if err != nil {
			return gobl.ErrValidation.WithReason("signature %d: invalid iss %q", i, p.Iss)
		}
		key, err := client.FetchKey(ctx, iss, sig.KeyID())
		if err != nil {
			return gobl.ErrValidation.WithCause(err)
		}
		if err := env.VerifySignature(sig, key); err != nil {
			return gobl.ErrValidation.WithCause(err)
		}
		if iss == addr {
			found = true
		}
	}
	if addr != "" && !found {
		return gobl.ErrValidation.WithReason("no signature by %s", addr)
	}
	return nil
}
