package main

import (
	"errors"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/invopop/gobl.dev/internal/ops"
	goblnet "github.com/invopop/gobl/net"
)

type netSendOpts struct {
	*rootOpts
	to   string
	from string
}

func netSend(root *rootOpts) *netSendOpts {
	return &netSendOpts{rootOpts: root}
}

func (s *netSendOpts) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send [infile]",
		Short: "Send a signed GOBL envelope to a GOBL Net inbox (EXPERIMENTAL)",
		Long: "Send a signed GOBL envelope to a GOBL Net inbox. The request carries\n" +
			"a bearer request token minted from the --from domain identity, which\n" +
			"may differ from the envelope's signer when transmitting on another\n" +
			"party's behalf.\n\n" +
			"EXPERIMENTAL: GOBL Net is under active development and may change without notice.",
		Args: cobra.MaximumNArgs(1),
		RunE: s.runE,
	}
	f := cmd.Flags()
	f.StringVarP(&s.to, "to", "t", "", "Destination GOBL Net address (FQDN)")
	f.StringVar(&s.from, "from", "", "Local domain identity (~/.config/gobl/<from>/) used to mint the request token")
	_ = cmd.MarkFlagRequired("to")
	_ = cmd.MarkFlagRequired("from")
	return cmd
}

func (s *netSendOpts) runE(cmd *cobra.Command, args []string) error {
	ctx := commandContext(cmd)

	if s.from == "" {
		return errors.New("--from is required to authenticate the request")
	}
	key, err := loadPrivateKey(filepath.Join(defaultConfigDir(), s.from, "private.jwk"))
	if err != nil {
		return err
	}

	input, err := openInput(cmd, args)
	if err != nil {
		return err
	}
	defer input.Close() // nolint:errcheck

	return ops.NetSend(ctx, &ops.NetSendOptions{
		Input:   input,
		To:      goblnet.Address(s.to),
		From:    goblnet.Address(s.from),
		FromKey: key,
	})
}
