package ops

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"path"
	"path/filepath"
	"slices"
	"strings"

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
	// Attachments are files to embed in the exported document. Formats that
	// cannot embed them are skipped.
	Attachments []*convert.Attachment
}

// NewAttachment prepares a file to embed in an exported document. The MIME
// type is taken from the name's extension when not given, and directories
// are removed from the name.
func NewAttachment(name, mimeType string, data []byte) *convert.Attachment {
	name = path.Base(strings.ReplaceAll(name, `\`, "/"))
	if name == "." || name == "/" {
		name = ""
	}
	if mimeType == "" || mimeType == "application/octet-stream" {
		if t := mime.TypeByExtension(filepath.Ext(name)); t != "" {
			mimeType = t
		}
	}
	if t, _, err := mime.ParseMediaType(mimeType); err == nil {
		mimeType = t
	}
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	return &convert.Attachment{Name: name, MIME: mimeType, Data: data}
}

// Formats lists every format registered with GOBL's convert package by the
// converters in the bundle.
func Formats() []*convert.Format {
	return convert.Formats()
}

// Import detects the format of the input and converts it into a GOBL
// envelope using the registered converters, providing the format detected
// and any files embedded in the input.
func Import(ctx context.Context, opts *ImportOptions) (*convert.ImportResult, error) {
	data, err := io.ReadAll(cancelableReader(ctx, opts.Input))
	if err != nil {
		return nil, gobl.ErrInput.WithCause(err)
	}
	if len(data) == 0 {
		return nil, gobl.ErrInput.WithReason("no input data")
	}
	res, err := convert.Import(data, convert.WithFormats(opts.Formats...))
	if err != nil {
		return nil, gobl.ErrInternal.WithCause(err)
	}
	return res, nil
}

// FormatCheck reports whether a format accepts a document for export. When
// it does not, Error gives the reason, usually as rules faults.
type FormatCheck struct {
	*convert.Format
	Accepted bool        `json:"accepted"`
	Error    *gobl.Error `json:"error,omitempty"`
}

// Export converts the GOBL document or envelope in the input into the first
// of the requested formats that accepts it. Documents are wrapped in an
// envelope, and unsigned envelopes are calculated, before conversion so that
// drafts can be exported directly.
func Export(ctx context.Context, opts *ExportOptions) (*convert.ExportResult, error) {
	if len(opts.Formats) == 0 {
		return nil, gobl.ErrInput.WithReason("no output format provided")
	}
	env, err := exportEnvelope(ctx, opts.ParseOptions)
	if err != nil {
		return nil, err
	}
	out, err := convert.Export(env, opts.Formats[0],
		convert.WithAlternatives(opts.Formats[1:]...),
		convert.WithAttachments(opts.Attachments...),
	)
	if err != nil {
		return nil, gobl.ErrInternal.WithCause(err)
	}
	return out, nil
}

// CheckFormats reports, for every registered format that exports the
// document's schema, whether it accepts the GOBL document or envelope in the
// input, prepared as for Export.
//
// The convert register only asks a format's converter whether it accepts a
// document during an export, so each format is checked by exporting a copy
// of the envelope, as converters may adjust the document they convert.
func CheckFormats(ctx context.Context, opts *ParseOptions) ([]*FormatCheck, error) {
	env, err := exportEnvelope(ctx, opts)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(env)
	if err != nil {
		return nil, gobl.ErrInternal.WithCause(err)
	}

	list := make([]*FormatCheck, 0)
	for _, f := range convert.Formats() {
		if !slices.Contains(f.Export, env.Document.Schema) {
			continue
		}
		cp := new(gobl.Envelope)
		if err := json.Unmarshal(data, cp); err != nil {
			return nil, gobl.ErrInternal.WithCause(err)
		}
		c := &FormatCheck{Format: f, Accepted: true}
		if _, err := convert.Export(cp, f.Key); err != nil {
			// Formats that accept the document but fail to convert it are
			// still offered, so the failure is shown when they are used.
			c.Accepted = !errors.Is(err, convert.ErrNotSupported)
			c.Error = gobl.ErrInternal.WithCause(err)
		}
		list = append(list, c)
	}
	return list, nil
}

// exportEnvelope parses the input into an envelope ready for export.
func exportEnvelope(ctx context.Context, opts *ParseOptions) (*gobl.Envelope, error) {
	popts := *opts
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
	return env, nil
}
