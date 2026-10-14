package main

import (
	"encoding/json"
	"errors"

	"github.com/invopop/gobl.dev/internal/ops"
	"github.com/invopop/gobl/cbc"
	"github.com/spf13/cobra"
)

type convertOpts struct {
	*rootOpts
	to   []string // formats to export into, in order of preference
	from []string // formats to limit import detection to
	list bool
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
and it is imported into a GOBL envelope. Use --list to see the available formats.`,
		Example: `  gobl convert invoice.json invoice.xml --to ubl+en16931-v2017
  gobl convert invoice.xml -i
  gobl convert --list -i`,
	}

	f := cmd.Flags()
	f.StringSliceVarP(&o.to, "to", "t", nil, "Format keys to export into, in order of preference.")
	f.StringSliceVar(&o.from, "from", nil, "Format keys to limit import detection to.")
	f.BoolVarP(&o.list, "list", "l", false, "List the available formats.")

	return cmd
}

func (o *convertOpts) runE(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)

	if len(o.to) > 0 && len(o.from) > 0 {
		return errors.New("--to and --from cannot be used together")
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
		env, err := ops.Import(ctx, &ops.ImportOptions{
			Input:   input,
			Formats: formatKeys(o.from),
		})
		if err != nil {
			return err
		}
		return o.encode(cmd, args, env)
	}

	res, err := ops.Export(ctx, &ops.ExportOptions{
		ParseOptions: &ops.ParseOptions{
			Input: input,
		},
		Formats: formatKeys(o.to),
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
