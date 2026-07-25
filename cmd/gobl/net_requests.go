package main

import (
	"github.com/spf13/cobra"

	"github.com/invopop/gobl.dev/internal/ops"
	goblnet "github.com/invopop/gobl/net"
)

type netRequestsOpts struct {
	*rootOpts
	domain string
}

func netRequests(root *rootOpts) *netRequestsOpts {
	return &netRequestsOpts{rootOpts: root}
}

func (o *netRequestsOpts) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "requests",
		Short: "List deferred /who requests awaiting approval (EXPERIMENTAL)",
		Long: "List the authenticated /who requests a deferred-disclosure domain has\n" +
			"answered 202 and recorded for approval. Approve one with\n" +
			"`gobl net approve <requester> --domain <domain>`.\n\n" +
			"EXPERIMENTAL: GOBL Net is under active development and may change without notice.",
		Args: cobra.NoArgs,
		RunE: o.runE,
	}
	f := cmd.Flags()
	f.StringVar(&o.domain, "domain", "", "Local domain identity (~/.config/gobl/<domain>/) whose requests to list")
	_ = cmd.MarkFlagRequired("domain")
	return cmd
}

func (o *netRequestsOpts) runE(cmd *cobra.Command, _ []string) error {
	return ops.NetRequests(&ops.NetRequestsOptions{
		ConfigDir: defaultConfigDir(),
		Domain:    o.domain,
		Out:       cmd.OutOrStdout(),
	})
}

type netApproveOpts struct {
	*rootOpts
	domain string
}

func netApprove(root *rootOpts) *netApproveOpts {
	return &netApproveOpts{rootOpts: root}
}

func (o *netApproveOpts) cmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "approve <requester>",
		Short: "Approve a deferred /who request (EXPERIMENTAL)",
		Long: "Approve a deferred /who request: sign the domain's party envelope for\n" +
			"the requester (aud=requester) and deliver it to the requester's inbox,\n" +
			"then clear the recorded request.\n\n" +
			"EXPERIMENTAL: GOBL Net is under active development and may change without notice.",
		Args: cobra.ExactArgs(1),
		RunE: o.runE,
	}
	f := cmd.Flags()
	f.StringVar(&o.domain, "domain", "", "Local domain identity (~/.config/gobl/<domain>/) approving the request")
	_ = cmd.MarkFlagRequired("domain")
	return cmd
}

func (o *netApproveOpts) runE(cmd *cobra.Command, args []string) error {
	return ops.NetApprove(commandContext(cmd), &ops.NetApproveOptions{
		ConfigDir: defaultConfigDir(),
		Domain:    o.domain,
		Requester: goblnet.Address(args[0]),
	})
}
