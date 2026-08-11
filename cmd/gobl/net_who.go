package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/invopop/gobl.dev/internal/ops"
	goblnet "github.com/invopop/gobl/net"
)

type netWhoOpts struct {
	*rootOpts
	from string
}

func netWho(root *rootOpts) *netWhoOpts {
	return &netWhoOpts{rootOpts: root}
}

func (w *netWhoOpts) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "who <address>",
		Short: "Look up the party a GOBL Net domain belongs to (EXPERIMENTAL)",
		Long: "Fetch and verify the org.Party published at a GOBL Net address's\n" +
			"/.well-known/gobl/who endpoint. The request carries a bearer request\n" +
			"token minted from the --from domain identity.\n\n" +
			"EXPERIMENTAL: GOBL Net is under active development and may change without notice.",
		Args: cobra.ExactArgs(1),
		RunE: w.runE,
	}
	f := cmd.Flags()
	f.StringVar(&w.from, "from", "", "Local domain identity (~/.config/gobl/<from>/) used to mint the request token")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}

func (w *netWhoOpts) runE(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)

	if w.from == "" {
		return errors.New("--from is required to authenticate the request")
	}
	configDir := defaultConfigDir()
	key, err := loadPrivateKey(filepath.Join(configDir, w.from, "private.jwk"))
	if err != nil {
		return err
	}

	result, err := ops.NetWho(ctx, &ops.NetWhoOptions{
		Target:    goblnet.Address(args[0]),
		From:      goblnet.Address(w.from),
		FromKey:   key,
		ConfigDir: configDir,
	})
	if err != nil {
		if errors.Is(err, goblnet.ErrPending) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(),
				"request accepted (202): the owner may deliver their party to your inbox once approved")
			return nil
		}
		return err
	}

	enc := json.NewEncoder(cmd.OutOrStdout())
	if w.indent {
		enc.SetIndent("", "\t")
	}
	return enc.Encode(result)
}
