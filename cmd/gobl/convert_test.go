package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/invopop/gobl"
	"github.com/invopop/gobl/bill"
	gconvert "github.com/invopop/gobl/convert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runConvert(t *testing.T, stdin []byte, args ...string) ([]byte, error) {
	t.Helper()
	cmd := root().cmd()
	stdout := new(bytes.Buffer)
	cmd.SetIn(bytes.NewReader(stdin))
	cmd.SetOut(stdout)
	cmd.SetArgs(append([]string{"convert"}, args...))
	err := cmd.Execute()
	return stdout.Bytes(), err
}

func Test_convert(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		out, err := runConvert(t, nil, "--list")
		require.NoError(t, err)
		var formats []map[string]any
		require.NoError(t, json.Unmarshal(out, &formats))
		keys := make([]string, 0, len(formats))
		for _, f := range formats {
			keys = append(keys, f["key"].(string))
		}
		assert.Contains(t, keys, "ubl+en16931-v2017")
		assert.Contains(t, keys, "cii+en16931-v2017")
	})

	t.Run("export and import", func(t *testing.T) {
		xml, err := runConvert(t, nil, "testdata/convert-invoice.json", "--to", "ubl+en16931-v2017")
		require.NoError(t, err)
		assert.Contains(t, string(xml), "<Invoice")

		out, err := runConvert(t, xml)
		require.NoError(t, err)
		env := new(gobl.Envelope)
		require.NoError(t, json.Unmarshal(out, env))
		inv, ok := env.Extract().(*bill.Invoice)
		require.True(t, ok)
		assert.Equal(t, "Provide One GmbH", inv.Supplier.Name)
	})

	t.Run("export preference order", func(t *testing.T) {
		xml, err := runConvert(t, nil, "testdata/convert-invoice.json", "--to", "cii+en16931-v2017,ubl+en16931-v2017")
		require.NoError(t, err)
		assert.Contains(t, string(xml), "CrossIndustryInvoice")
	})

	t.Run("import restricted to other format", func(t *testing.T) {
		xml, err := runConvert(t, nil, "testdata/convert-invoice.json", "--to", "ubl+en16931-v2017")
		require.NoError(t, err)
		_, err = runConvert(t, xml, "--from", "cii+en16931-v2017")
		assert.ErrorContains(t, err, "unknown-format")
	})

	t.Run("unknown format", func(t *testing.T) {
		_, err := runConvert(t, nil, "testdata/convert-invoice.json", "--to", "foo")
		assert.ErrorContains(t, err, "unknown-format")
	})

	t.Run("to and from", func(t *testing.T) {
		_, err := runConvert(t, nil, "--to", "a", "--from", "b")
		assert.EqualError(t, err, "--to and --from cannot be used together")
	})

	t.Run("attachments", func(t *testing.T) {
		dir := t.TempDir()
		pdf := filepath.Join(dir, "invoice.pdf")
		require.NoError(t, os.WriteFile(pdf, []byte("%PDF-1.7"), 0o644))

		xml, err := runConvert(t, nil, "testdata/convert-invoice.json", "--to", "ubl+en16931-v2017", "--attach", pdf)
		require.NoError(t, err)
		assert.Contains(t, string(xml), `mimeCode="application/pdf"`)
		assert.Contains(t, string(xml), `filename="invoice.pdf"`)

		out := filepath.Join(dir, "out")
		_, err = runConvert(t, xml, "--attachments", out)
		require.NoError(t, err)
		data, err := os.ReadFile(filepath.Join(out, "invoice.pdf"))
		require.NoError(t, err)
		assert.Equal(t, []byte("%PDF-1.7"), data)
	})

	t.Run("attachment not found", func(t *testing.T) {
		_, err := runConvert(t, nil, "testdata/convert-invoice.json", "--to", "ubl+en16931-v2017", "--attach", "missing.pdf")
		assert.ErrorContains(t, err, "missing.pdf")
	})

	t.Run("attach without to", func(t *testing.T) {
		_, err := runConvert(t, nil, "--attach", "invoice.pdf")
		assert.EqualError(t, err, "--attach requires --to")
	})

	t.Run("attachments with to", func(t *testing.T) {
		_, err := runConvert(t, nil, "--to", "a", "--attachments", "files")
		assert.EqualError(t, err, "--attachments cannot be used with --to")
	})
}

func Test_saveAttachments(t *testing.T) {
	dir := t.TempDir()
	atts := []*gconvert.Attachment{
		{Name: "../../escape.pdf", Data: []byte("1")},
		{Name: `C:\files\escape.pdf`, Data: []byte("2")},
		{ID: "doc-3", Data: []byte("3")},
		{Name: "..", Data: []byte("4")},
	}
	require.NoError(t, saveAttachments(dir, atts))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name()
	}
	assert.ElementsMatch(t, []string{"escape.pdf", "escape-2.pdf", "doc-3", "attachment-4"}, names)
	data, err := os.ReadFile(filepath.Join(dir, "escape-2.pdf"))
	require.NoError(t, err)
	assert.Equal(t, []byte("2"), data)
}
