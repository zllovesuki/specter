package config

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.miragespace.co/specter/acme"
	"go.miragespace.co/specter/config"
	"go.miragespace.co/specter/util"

	"github.com/urfave/cli/v3"
)

func Generate() *cli.Command {
	return &cli.Command{
		Name:  "config",
		Usage: "manage shared server configuration",
		Commands: []*cli.Command{{
			Name:      "init",
			Usage:     "initialize shared configuration on the first server",
			ArgsUsage: " ",
			Description: `Create the first server's shared configuration from a JSON array of
{name, schema, value} documents. Run once as the server's service user.
Other servers obtain the documents from peers. The server may be running;
existing shared configuration is never replaced.
See docs/shared-configuration.md for the input format and rollout sequence.`,
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "data-dir",
					Aliases:  []string{"data"},
					Usage:    "the first server's existing data directory",
					Required: true,
				},
				&cli.StringFlag{
					Name:     "file",
					Usage:    "JSON file containing the initial named documents",
					Required: true,
				},
			},
			Action: cmdInit,
		}},
	}
}

type initialDocument struct {
	Name   string          `json:"name"`
	Schema uint64          `json:"schema"`
	Value  json.RawMessage `json:"value"`
}

func cmdInit(_ context.Context, cmd *cli.Command) error {
	if cmd.Args().Len() != 0 {
		return fmt.Errorf("config init takes no positional arguments")
	}
	if cmd.String("data-dir") == "" {
		return fmt.Errorf("data-dir must not be empty")
	}
	file, err := os.Open(cmd.String("file"))
	if err != nil {
		return fmt.Errorf("open initial configuration: %w", err)
	}
	defer file.Close()

	records, err := readInitialDocuments(file)
	if err != nil {
		return err
	}
	dataDir, err := filepath.Abs(cmd.String("data-dir"))
	if err != nil {
		return fmt.Errorf("resolve data directory: %w", err)
	}
	dir := filepath.Join(dataDir, "config")
	if err := config.Init(dir, records); err != nil {
		return fmt.Errorf("initialize shared configuration: %w", err)
	}
	_, err = fmt.Fprintf(cmd.Root().Writer, "Initialized %d shared configuration document(s) in %s\n", len(records), filepath.Join(dir, config.FileName))
	return err
}

func readInitialDocuments(input io.Reader) ([]config.Record, error) {
	data, err := io.ReadAll(io.LimitReader(input, config.MaxSnapshotBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read initial configuration: %w", err)
	}
	if len(data) > config.MaxSnapshotBytes {
		return nil, fmt.Errorf("initial configuration exceeds %d bytes", config.MaxSnapshotBytes)
	}
	var documents []initialDocument
	if err := util.DecodeJSON(bytes.NewReader(data), &documents); err != nil {
		return nil, fmt.Errorf("decode initial configuration: %w", err)
	}
	if len(documents) == 0 || len(documents) > config.MaxDocuments {
		return nil, fmt.Errorf("initial configuration must contain between 1 and %d documents", config.MaxDocuments)
	}

	records := make([]config.Record, 0, len(documents))
	names := make(map[string]bool, len(documents))
	for i, document := range documents {
		if !json.Valid(document.Value) {
			return nil, fmt.Errorf("document %d must include a JSON value", i+1)
		}
		if names[document.Name] {
			return nil, fmt.Errorf("document %d repeats an earlier document name", i+1)
		}
		names[document.Name] = true
		record, err := config.NewRecord(document.Name, document.Schema, 1, document.Value)
		if err != nil {
			return nil, fmt.Errorf("document %d: %w", i+1, err)
		}
		if document.Name == acme.DNSConfigName {
			if _, err := acme.DecodeDNSConfig(record, ""); err != nil {
				return nil, fmt.Errorf("document %d: %w", i+1, err)
			}
		}
		records = append(records, record)
	}
	return records, nil
}
