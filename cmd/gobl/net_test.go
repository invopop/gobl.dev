package main

import (
	"bytes"
	"context"
	"encoding/json"
	stdnet "net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl.dev/internal/ops"
	"github.com/invopop/gobl/dsig"
	"github.com/invopop/gobl/head"
	"github.com/invopop/gobl/net"
	"github.com/invopop/gobl/note"
	"github.com/invopop/gobl/uuid"
)

// initDomainForCLI scaffolds a domain at <configDir>/<domain>/ using
// the internal ops layer so cmd/gobl tests can run without invoking
// the full init command.
func initDomainForCLI(t *testing.T, configDir, domain string) {
	t.Helper()
	require.NoError(t, ops.InitDomain(&ops.InitOptions{
		ConfigDir: configDir,
		Domain:    domain,
		Name:      domain,
		Out:       new(bytes.Buffer),
	}))
}

func TestNetCmdSubcommands(t *testing.T) {
	n := netCmd(&rootOpts{})
	c := n.cmd()
	assert.Equal(t, "net", c.Use)
	have := map[string]bool{}
	for _, sub := range c.Commands() {
		have[sub.Name()] = true
	}
	assert.True(t, have["serve"])
	assert.True(t, have["send"])
	assert.True(t, have["who"])
	assert.True(t, have["requests"])
	assert.True(t, have["approve"])
}

// ---------- net send -----------

func signedNoteBody(t *testing.T) []byte {
	t.Helper()
	priv := dsig.NewES256Key()
	msg := &note.Message{Content: "hi"}
	msg.SetUUID(uuid.V7())
	env, err := gobl.Envelop(msg)
	require.NoError(t, err)
	require.NoError(t, env.Sign(priv,
		head.WithIssuer(net.Address("peer.example").String()),
		head.WithAudience(net.Address("acme.example").String())))
	out, err := json.Marshal(env)
	require.NoError(t, err)
	return out
}

func TestNetSendCmdMissingTo(t *testing.T) {
	o := netSend(&rootOpts{})
	c := o.cmd()
	c.SetArgs([]string{"-"})
	c.SetOut(new(bytes.Buffer))
	c.SetErr(new(bytes.Buffer))
	err := c.Execute()
	require.Error(t, err)
}

func TestNetSendCmdInvalidAddress(t *testing.T) {
	// The request token's aud must be a valid GOBL Net address, so raw
	// IP targets are rejected before any transport happens. (The full
	// send flow is covered in internal/ops with injected fetchers.)
	body := signedNoteBody(t)
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	initDomainForCLI(t, filepath.Join(tmp, ".config", "gobl"), "from.example")
	infile := filepath.Join(tmp, "env.json")
	require.NoError(t, os.WriteFile(infile, body, 0o644))

	o := netSend(&rootOpts{})
	c := o.cmd()
	c.SetOut(new(bytes.Buffer))
	c.SetErr(new(bytes.Buffer))
	c.SetArgs([]string{"--to", "127.0.0.1:8080", "--from", "from.example", infile})
	err := c.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid address")
}

func TestNetSendCmdBadInput(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	initDomainForCLI(t, filepath.Join(tmp, ".config", "gobl"), "from.example")
	o := netSend(&rootOpts{})
	c := o.cmd()
	c.SetOut(new(bytes.Buffer))
	c.SetErr(new(bytes.Buffer))
	c.SetArgs([]string{"--to", "acme.example", "--from", "from.example", "/no/such/file.json"})
	err := c.Execute()
	require.Error(t, err)
}

func TestNetSendCmdMissingKey(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	o := netSend(&rootOpts{})
	c := o.cmd()
	c.SetOut(new(bytes.Buffer))
	c.SetErr(new(bytes.Buffer))
	c.SetArgs([]string{"--to", "acme.example", "--from", "missing.example", "-"})
	err := c.Execute()
	require.Error(t, err)
}

// ---------- net who -----------

func TestNetWhoCmdMissingFromFlag(t *testing.T) {
	o := netWho(&rootOpts{})
	c := o.cmd()
	c.SetOut(new(bytes.Buffer))
	c.SetErr(new(bytes.Buffer))
	c.SetArgs([]string{"acme.example"})
	err := c.Execute()
	require.Error(t, err)
}

func TestNetWhoCmdMissingKey(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	o := netWho(&rootOpts{})
	c := o.cmd()
	c.SetOut(new(bytes.Buffer))
	c.SetErr(new(bytes.Buffer))
	c.SetArgs([]string{"--from", "missing.example", "acme.example"})
	err := c.Execute()
	require.Error(t, err)
}

// ---------- net requests / approve -----------

func TestNetRequestsCmdEmpty(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	initDomainForCLI(t, filepath.Join(tmp, ".config", "gobl"), "mine.example")

	o := netRequests(&rootOpts{})
	c := o.cmd()
	out := new(bytes.Buffer)
	c.SetOut(out)
	c.SetErr(new(bytes.Buffer))
	c.SetArgs([]string{"--domain", "mine.example"})
	require.NoError(t, c.Execute())
	assert.JSONEq(t, "[]", out.String())
}

func TestNetApproveCmdMissingIdentity(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	o := netApprove(&rootOpts{})
	c := o.cmd()
	c.SetOut(new(bytes.Buffer))
	c.SetErr(new(bytes.Buffer))
	c.SetArgs([]string{"--domain", "missing.example", "peer.example"})
	err := c.Execute()
	require.Error(t, err)
}

func TestNetWhoCmdRunENilCase(t *testing.T) {
	// Direct runE call with empty --from -> short-circuits with an
	// explicit error message.
	o := &netWhoOpts{rootOpts: &rootOpts{}}
	err := o.runE(&cobra.Command{}, []string{"acme.example"})
	require.Error(t, err)
}

// ---------- net serve -----------

func TestNetServeCmdValidateMutualACME(t *testing.T) {
	o := &netServeOpts{rootOpts: &rootOpts{}, acmeLive: true, acmeTest: true}
	require.Error(t, o.validate())
}

func TestNetServeCmdValidateMutualACMEAndTLS(t *testing.T) {
	o := &netServeOpts{rootOpts: &rootOpts{}, acmeLive: true, tlsCert: "x.pem"}
	require.Error(t, o.validate())
}

func TestNetServeCmdValidatePartialTLS(t *testing.T) {
	o := &netServeOpts{rootOpts: &rootOpts{}, tlsCert: "x.pem"}
	require.Error(t, o.validate())
}

func TestNetServeCmdValidateOK(t *testing.T) {
	o := &netServeOpts{rootOpts: &rootOpts{}}
	require.NoError(t, o.validate())
}

func TestNetServeCmdNoDomainsErrors(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	o := netServe(&rootOpts{})
	c := o.cmd()
	c.SetOut(new(bytes.Buffer))
	c.SetErr(new(bytes.Buffer))
	c.SetArgs([]string{"--config-dir", tmp, "--http-port", strconv.Itoa(freeCLIPort(t))})
	err := c.Execute()
	require.Error(t, err)
}

func TestNetServeCmdConfigDir(t *testing.T) {
	// A config-dir domain serves until the command context is
	// cancelled; --allow-unverified and --authority just plumb through.
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, ".config", "gobl")
	initDomainForCLI(t, configDir, "solo.example")

	o := netServe(&rootOpts{})
	c := o.cmd()
	c.SetOut(new(bytes.Buffer))
	c.SetErr(new(bytes.Buffer))
	port := freeCLIPort(t)
	c.SetArgs([]string{
		"--config-dir", configDir,
		"--authority", "sandbox.example",
		"--allow-unverified",
		"--http-port", strconv.Itoa(port),
	})

	ctx, cancel := context.WithCancel(context.Background())
	c.SetContext(ctx)
	done := make(chan error, 1)
	go func() { done <- c.Execute() }()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("netServe did not return")
	}
}

func freeCLIPort(t *testing.T) int {
	t.Helper()
	ln, err := stdnet.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close() //nolint:errcheck
	return ln.Addr().(*stdnet.TCPAddr).Port
}
