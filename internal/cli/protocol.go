package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/sirerun/serenity/docs/protocol/schemas"
)

// newProtocolCmd wires `serenity protocol`, RFC 0001 section 8's own
// promised machine-readable schema delivery mechanism ("Each ships with a
// machine-readable schema (`serenity protocol --json`), conformance
// fixtures in-repo, and a conformance command"). It enumerates every wire
// object across the three protocols this server implements
// (MEMORY_VERBS v1, DISPOSITION v1, DIRECTION v1) from
// docs/protocol/schemas -- the same embedded schema set
// docs/protocol/schemas/schemas_test.go compiles as draft-2020-12 and
// reflects against the live Go wire types, so what this command prints is
// never a second, hand-maintained copy of the contract.
func newProtocolCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "protocol",
		Short: "List Serenity's protocol objects and their JSON Schemas",
		Long: "protocol enumerates every wire object across the three protocols\n" +
			"(MEMORY_VERBS v1, DISPOSITION v1, DIRECTION v1 -- RFC 0001 section 8)\n" +
			"this server implements.\n\n" +
			"--json prints each object's full draft-2020-12 schema (docs/protocol/\n" +
			"schemas/*.json, embedded verbatim) -- the machine-readable contract a\n" +
			"client can validate requests and responses against without vendoring\n" +
			"documentation prose.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProtocol(jsonOut, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "machine-readable JSON output (every object's full schema)")
	cmd.AddCommand(newProtocolConformanceCmd())
	return cmd
}

// protocolObjectManifest is one object's entry in --json's manifest.
// Schema carries the object's schema file exactly as embedded ($id and
// protocol_version live inside it, not duplicated as separate top-level
// fields here).
type protocolObjectManifest struct {
	Protocol string          `json:"protocol"`
	Object   string          `json:"object"`
	Schema   json.RawMessage `json:"schema"`
}

type protocolManifest struct {
	ProtocolVersion int                      `json:"protocol_version"`
	Objects         []protocolObjectManifest `json:"objects"`
}

func runProtocol(jsonOut bool, out io.Writer) error {
	entries := schemas.All()
	if jsonOut {
		manifest := protocolManifest{ProtocolVersion: 1, Objects: make([]protocolObjectManifest, 0, len(entries))}
		for _, e := range entries {
			raw, err := schemas.Raw(e)
			if err != nil {
				return fmt.Errorf("protocol: reading schema for %s/%s: %w", e.Protocol, e.Object, err)
			}
			manifest.Objects = append(manifest.Objects, protocolObjectManifest{
				Protocol: e.Protocol,
				Object:   e.Object,
				Schema:   json.RawMessage(raw),
			})
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(manifest)
	}

	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROTOCOL\tOBJECT\tSCHEMA $ID")
	for _, e := range entries {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", e.Protocol, e.Object, e.ID())
	}
	return tw.Flush()
}
