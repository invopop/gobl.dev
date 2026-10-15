package api

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl.dev/internal/ops"
	"github.com/invopop/gobl/cbc"
	"github.com/invopop/gobl/convert"
)

// FormatHeader is the response header that names the key of the format an
// export was converted into, or an import was detected as.
const FormatHeader = "GOBL-Format"

// AttachmentsHeader is the response header that gives the number of files
// embedded in an imported document.
const AttachmentsHeader = "GOBL-Attachments"

// multipartMemory is the size of a multipart export request kept in memory.
// Larger files are stored temporarily on disk while the request is handled.
const multipartMemory = 32 << 20

// importResponse is the import response when attachments are requested.
type importResponse struct {
	Format      cbc.Key               `json:"format"`
	Envelope    *gobl.Envelope        `json:"envelope"`
	Attachments []*convert.Attachment `json:"attachments"`
}

func handleFormatList(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, map[string]any{"formats": ops.Formats()})
}

// handleFormatCheck reports, for each format that exports the schema of the
// GOBL document in the body, whether it accepts the document.
func handleFormatCheck(w http.ResponseWriter, r *http.Request) {
	checks, err := ops.CheckFormats(r.Context(), &ops.ParseOptions{
		Input: r.Body,
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, map[string]any{"formats": checks})
}

// handleImport converts the document in the body into a GOBL envelope. With
// the "attachments" query parameter, the response also includes the files
// embedded in the document.
func handleImport(w http.ResponseWriter, r *http.Request) {
	withAttachments := false
	if v := r.URL.Query().Get("attachments"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			WriteError(w, gobl.ErrInput.WithReason("invalid attachments parameter: %s", v))
			return
		}
		withAttachments = b
	}
	res, err := ops.Import(r.Context(), &ops.ImportOptions{
		Input:   r.Body,
		Formats: queryFormats(r),
	})
	if err != nil {
		WriteError(w, err)
		return
	}
	w.Header().Set(FormatHeader, res.Format.Key.String())
	w.Header().Set(AttachmentsHeader, strconv.Itoa(len(res.Attachments)))
	if !withAttachments {
		WriteJSON(w, res.Envelope)
		return
	}
	atts := res.Attachments
	if atts == nil {
		atts = []*convert.Attachment{}
	}
	WriteJSON(w, &importResponse{
		Format:      res.Format.Key,
		Envelope:    res.Envelope,
		Attachments: atts,
	})
}

// handleExport converts the GOBL document in the body into the first of the
// requested formats that accepts it. A multipart body provides the document
// in its "document" part and files to embed in "attachment" parts.
func handleExport(w http.ResponseWriter, r *http.Request) {
	opts := &ops.ExportOptions{
		ParseOptions: &ops.ParseOptions{
			Input: r.Body,
		},
		Formats: queryFormats(r),
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt == "multipart/form-data" {
		if err := r.ParseMultipartForm(multipartMemory); err != nil {
			WriteError(w, gobl.ErrInput.WithReason("invalid multipart body: %s", err))
			return
		}
		defer r.MultipartForm.RemoveAll() //nolint:errcheck
		if err := readExportForm(r.MultipartForm, opts); err != nil {
			WriteError(w, err)
			return
		}
	}
	out, err := ops.Export(r.Context(), opts)
	if err != nil {
		WriteError(w, err)
		return
	}
	mime := out.Format.MIME
	if mime == "" {
		mime = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set(FormatHeader, out.Format.Key.String())
	_, _ = w.Write(out.Data) //nolint:errcheck
}

// readExportForm reads the document and the attachments of a multipart
// export request. The document may be a field or a file.
func readExportForm(form *multipart.Form, opts *ops.ExportOptions) error {
	if v := form.Value["document"]; len(v) > 0 {
		opts.Input = strings.NewReader(v[0])
	} else if f := form.File["document"]; len(f) > 0 {
		data, err := readFormFile(f[0])
		if err != nil {
			return err
		}
		opts.Input = bytes.NewReader(data)
	} else {
		return gobl.ErrInput.WithReason("multipart body has no document part")
	}
	for _, fh := range form.File["attachment"] {
		data, err := readFormFile(fh)
		if err != nil {
			return err
		}
		opts.Attachments = append(opts.Attachments,
			ops.NewAttachment(fh.Filename, fh.Header.Get("Content-Type"), data),
		)
	}
	return nil
}

func readFormFile(fh *multipart.FileHeader) ([]byte, error) {
	f, err := fh.Open()
	if err != nil {
		return nil, gobl.ErrInput.WithCause(err)
	}
	defer f.Close() //nolint:errcheck
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, gobl.ErrInput.WithCause(err)
	}
	return data, nil
}

// queryFormats reads the format keys from the "format" query parameter, which
// may be repeated or hold a comma-separated list, preserving their order.
// Keys contain "+", which query strings decode as a space, so unescaped keys
// such as "ubl+en16931-v2017" are restored.
func queryFormats(r *http.Request) []cbc.Key {
	var keys []cbc.Key
	for _, v := range r.URL.Query()["format"] {
		for _, k := range strings.Split(v, ",") {
			if k = strings.ReplaceAll(strings.TrimSpace(k), " ", "+"); k != "" {
				keys = append(keys, cbc.Key(k))
			}
		}
	}
	return keys
}
