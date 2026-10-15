package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/invopop/gobl.dev/internal/ops"
	"github.com/invopop/gobl/cbc"
	gconvert "github.com/invopop/gobl/convert"
	"github.com/spf13/cobra"
)

type convertOpts struct {
	*rootOpts
	to          []string // formats to export into, in order of preference
	from        []string // formats to limit import detection to
	list        bool
	attach      []string // files to embed on export
	attachments string   // directory to save embedded files to on import
}

func convert(root *rootOpts) *convertOpts {
	return &convertOpts{
		rootOpts: root,
	}
}

func (o *convertOpts) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Args:  cobra.MaximumNArgs(2),
		RunE:  o.runE,
		Use:   "convert [infile] [outfile]",
		Short: "Convert documents between GOBL and external formats",
		Long: `Convert documents between GOBL and the external formats registered by the
bundled converters, such as UBL and CII.

With --to, the input GOBL document or envelope is exported into the first of the
formats given that accepts it. Without --to, the format of the input is detected
and it is imported into a GOBL envelope. Use --list to see the available formats.

On export, --attach embeds files, such as a PDF version of the invoice, in
formats that support attachments. On import, --attachments saves the files
embedded in the document to a directory.`,
		Example: `  gobl convert invoice.json invoice.xml --to ubl+en16931-v2017
  gobl convert invoice.json invoice.xml --to ubl+peppol+bis-v3 --attach invoice.pdf
  gobl convert invoice.xml -i
  gobl convert invoice.xml invoice.json --attachments ./files
  gobl convert --list -i`,
	}

	f := cmd.Flags()
	f.StringSliceVarP(&o.to, "to", "t", nil, "Format keys to export into, in order of preference.")
	f.StringSliceVar(&o.from, "from", nil, "Format keys to limit import detection to.")
	f.BoolVarP(&o.list, "list", "l", false, "List the available formats.")
	f.StringArrayVar(&o.attach, "attach", nil, "File to embed in the exported document. May be repeated.")
	f.StringVar(&o.attachments, "attachments", "", "Directory to save the files embedded in an imported document to.")

	return cmd
}

func (o *convertOpts) runE(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)

	if len(o.to) > 0 && len(o.from) > 0 {
		return errors.New("--to and --from cannot be used together")
	}
	if len(o.attach) > 0 && len(o.to) == 0 {
		return errors.New("--attach requires --to")
	}
	if o.attachments != "" && len(o.to) > 0 {
		return errors.New("--attachments cannot be used with --to")
	}

	if o.list {
		return o.encode(cmd, args, ops.Formats())
	}

	input, err := openInput(cmd, args)
	if err != nil {
		return err
	}
	defer input.Close() // nolint:errcheck

	if len(o.to) == 0 {
		res, err := ops.Import(ctx, &ops.ImportOptions{
			Input:   input,
			Formats: formatKeys(o.from),
		})
		if err != nil {
			return err
		}
		if o.attachments != "" {
			if err := saveAttachments(o.attachments, res.Attachments); err != nil {
				return err
			}
		}
		return o.encode(cmd, args, res.Envelope)
	}

	atts, err := loadAttachments(o.attach)
	if err != nil {
		return err
	}
	res, err := ops.Export(ctx, &ops.ExportOptions{
		ParseOptions: &ops.ParseOptions{
			Input: input,
		},
		Formats:     formatKeys(o.to),
		Attachments: atts,
	})
	if err != nil {
		return err
	}

	out, err := o.openOutput(cmd, args)
	if err != nil {
		return err
	}
	defer out.Close() // nolint:errcheck

	_, err = out.Write(res.Data)
	return err
}

func (o *convertOpts) encode(cmd *cobra.Command, args []string, obj any) error {
	out, err := o.openOutput(cmd, args)
	if err != nil {
		return err
	}
	defer out.Close() // nolint:errcheck

	enc := json.NewEncoder(out)
	if o.indent {
		enc.SetIndent("", "\t")
	}
	return enc.Encode(obj)
}

func formatKeys(list []string) []cbc.Key {
	keys := make([]cbc.Key, len(list))
	for i, k := range list {
		keys[i] = cbc.Key(k)
	}
	return keys
}

// loadAttachments reads the files to embed in an exported document.
func loadAttachments(paths []string) ([]*gconvert.Attachment, error) {
	atts := make([]*gconvert.Attachment, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		atts = append(atts, ops.NewAttachment(p, "", data))
	}
	return atts, nil
}

// saveAttachments writes the files embedded in an imported document to the
// directory. Only the base of each name is used, as names come from the
// document, and repeated names are numbered.
func saveAttachments(dir string, atts []*gconvert.Attachment) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	used := make(map[string]bool)
	for i, a := range atts {
		name := attachmentFileName(a, i)
		base, ext := strings.TrimSuffix(name, filepath.Ext(name)), filepath.Ext(name)
		for n := 2; used[name]; n++ {
			name = fmt.Sprintf("%s-%d%s", base, n, ext)
		}
		used[name] = true
		if err := os.WriteFile(filepath.Join(dir, name), a.Data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// attachmentFileName provides a file name for an attachment from its name or
// ID, without any directories.
func attachmentFileName(a *gconvert.Attachment, i int) string {
	for _, n := range []string{a.Name, a.ID} {
		n = path.Base(strings.ReplaceAll(n, `\`, "/"))
		if n != "" && n != "." && n != ".." && n != "/" {
			return n
		}
	}
	return fmt.Sprintf("attachment-%d", i+1)
}
