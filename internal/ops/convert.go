package ops

import (
	"context"
	"io"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/convert"
)

// ImportOptions define the options used to import a document in an external
// format into GOBL.
type ImportOptions struct {
	Input io.Reader
	// Formats, when set, limits format detection to the given keys.
	Formats []cbc.Key
}

// ExportOptions define the options used to export a GOBL document into an
// external format.
type ExportOptions struct {
	*ParseOptions
	// Formats lists the keys of the formats to export into, in order of
	// preference. The first one that accepts the document is used.
	Formats []cbc.Key
}

// Formats lists every format registered with GOBL's convert package by the
// converters in the bundle.
func Formats() []*convert.Format {
	return convert.Formats()
}

// Import detects the format of the input and converts it into a GOBL
// envelope using the registered converters.
func Import(ctx context.Context, opts *ImportOptions) (*gobl.Envelope, error) {
	data, err := io.ReadAll(cancelableReader(ctx, opts.Input))
	if err != nil {
		return nil, gobl.ErrInput.WithCause(err)
	}
	if len(data) == 0 {
		return nil, gobl.ErrInput.WithReason("no input data")
	}
	env, err := convert.Import(data, opts.Formats...)
	if err != nil {
		return nil, gobl.ErrInternal.WithCause(err)
	}
	return env, nil
}

// Export converts the GOBL document or envelope in the input into the first
// of the requested formats that accepts it. Documents are wrapped in an
// envelope, and unsigned envelopes are calculated, before conversion so that
// drafts can be exported directly.
func Export(ctx context.Context, opts *ExportOptions) (*convert.Output, error) {
	if len(opts.Formats) == 0 {
		return nil, gobl.ErrInput.WithReason("no output format provided")
	}

	popts := *opts.ParseOptions
	popts.Envelop = true
	obj, err := parseGOBLData(ctx, &popts)
	if err != nil {
		return nil, gobl.ErrInternal.WithCause(err)
	}
	env, ok := obj.(*gobl.Envelope)
	if !ok {
		return nil, gobl.ErrInput.WithReason("input must be a GOBL document or envelope")
	}

	// Signed envelopes are sealed, so their contents are converted as they are.
	if len(env.Signatures) == 0 {
		if err := env.Calculate(); err != nil {
			return nil, gobl.ErrInternal.WithCause(err)
		}
	}
	if err := env.Validate(); err != nil {
		return nil, gobl.ErrInternal.WithCause(err)
	}

	out, err := convert.Export(env, opts.Formats...)
	if err != nil {
		return nil, gobl.ErrInternal.WithCause(err)
	}
	return out, nil
}
