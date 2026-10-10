package cli

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirerun/serenity/internal/hosted/authority"
	"github.com/sirerun/serenity/internal/hosted/contracts"
	"github.com/sirerun/serenity/internal/hosted/privatefs"
	"github.com/sirerun/serenity/internal/hosted/runtimebroker"
	"github.com/sirerun/serenity/internal/writer"
	"github.com/spf13/cobra"
)

func newHostedAuthorityCmd() *cobra.Command {
	parent := &cobra.Command{Use: "authority", Short: "Explicit owner operations on hosted journal authority"}
	var config string
	parent.PersistentFlags().StringVar(&config, "operator-config", "", "private owner endpoint and explicit selected credential profile")
	parent.AddCommand(&cobra.Command{Use: "new-operation-id", Short: "Create a retry-stable operation ID without contacting a provider", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		id, err := authority.NewOperationID()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), id)
		return err
	}})
	open := func(cmd *cobra.Command) (*authority.Client, func(), error) {
		if config == "" {
			return nil, nil, errors.New("explicit owner configuration required")
		}
		return runtimebroker.OpenOwner(cmd.Context(), config)
	}
	parent.AddCommand(&cobra.Command{Use: "inspect", Short: "Inspect exact current reservation, lease and journal position", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		c, closeClient, err := open(cmd)
		if err != nil {
			return err
		}
		defer closeClient()
		state, err := c.Inspect(cmd.Context())
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(struct {
			Prepared *authority.Prepared `json:"prepared,omitempty"`
			Lease    *authority.Lease    `json:"lease,omitempty"`
			Current  *authority.Current  `json:"current,omitempty"`
			Adoption *authority.Adoption `json:"adoption,omitempty"`
		}{state.Prepared, state.Lease, state.Current, state.Adoption})
	}})
	for _, name := range []string{"enroll", "restore-authority"} {
		var operation, adoptionFile string
		action := name
		cmd := &cobra.Command{Use: action, Short: map[string]string{"enroll": "Enroll a first-ever authority with optional preserved legacy checkpoint provenance", "restore-authority": "Reconstruct a fresh authority table from the complete independent witness chain"}[action], Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			c, closeClient, err := open(cmd)
			if err != nil {
				return err
			}
			defer closeClient()
			if action == "enroll" {
				if adoptionFile != "" {
					data, err := privatefs.ReadFile(cmd.Context(), filepath.Dir(adoptionFile), filepath.Base(adoptionFile), 4096)
					if err != nil {
						return err
					}
					adoption, err := authority.ParseAdoption(data)
					if err != nil {
						return err
					}
					return c.EnrollWithAdoption(cmd.Context(), adoption, operation)
				}
				return c.Enroll(cmd.Context(), operation)
			}
			return c.RestoreAuthority(cmd.Context(), operation)
		}}
		cmd.Flags().StringVar(&operation, "operation-id", "", "exact retry-stable ID from new-operation-id")
		if action == "enroll" {
			cmd.Flags().StringVar(&adoptionFile, "adoption-file", "", "private immutable legacy checkpoint receipt; never asserts earlier journal history")
		}
		mustRequired(cmd, "operation-id")
		parent.AddCommand(cmd)
	}
	var leaseID, reservationID, closeOperation string
	closeCmd := &cobra.Command{Use: "close", Short: "Revoke one explicitly identified lease or reservation", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if (leaseID == "") == (reservationID == "") {
			return errors.New("exactly one lease-id or reservation-id is required")
		}
		c, closeClient, err := open(cmd)
		if err != nil {
			return err
		}
		defer closeClient()
		if leaseID != "" {
			return c.Close(cmd.Context(), authority.Lease{ID: leaseID}, closeOperation)
		}
		return c.ClosePrepared(cmd.Context(), authority.Prepared{ReservationID: reservationID}, closeOperation)
	}}
	closeCmd.Flags().StringVar(&leaseID, "lease-id", "", "exact lease ID obtained by inspect")
	closeCmd.Flags().StringVar(&reservationID, "reservation-id", "", "exact reservation ID obtained by inspect")
	closeCmd.Flags().StringVar(&closeOperation, "operation-id", "", "exact retry-stable operation ID")
	mustRequired(closeCmd, "operation-id")
	parent.AddCommand(closeCmd)
	var sealLease, sealOperation, expectedHead, recordedAt, sealWriter string
	var sealGeneration, sealSequence int64
	seal := &cobra.Command{Use: "seal", Short: "Seal the exact approved generation head; does not unfreeze accounts", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		recorded, err := time.Parse(time.RFC3339Nano, recordedAt)
		if err != nil {
			return errors.New("recorded-at must be an exact RFC3339 timestamp")
		}
		key, body, err := approvedSeal(sealGeneration, sealSequence, sealWriter, expectedHead, recorded)
		if err != nil {
			return err
		}
		c, closeClient, err := open(cmd)
		if err != nil {
			return err
		}
		defer closeClient()
		return c.Seal(cmd.Context(), authority.Lease{ID: sealLease}, key, body, sealOperation)
	}}
	seal.Flags().StringVar(&sealLease, "lease-id", "", "exact active lease ID from inspect")
	seal.Flags().StringVar(&sealOperation, "operation-id", "", "exact retry-stable operation ID")
	seal.Flags().StringVar(&expectedHead, "expected-head", "", "exact inspected head hash, or genesis for empty first generation")
	seal.Flags().StringVar(&recordedAt, "recorded-at", "", "stable RFC3339 timestamp reused on retries")
	seal.Flags().StringVar(&sealWriter, "writer-id", "", "exact writer ID from inspect")
	seal.Flags().Int64Var(&sealGeneration, "generation", 0, "exact active generation from inspect")
	seal.Flags().Int64Var(&sealSequence, "sequence", 0, "exact next sequence after inspected head")
	mustRequired(seal, "lease-id", "operation-id", "expected-head", "recorded-at", "writer-id", "generation", "sequence")
	parent.AddCommand(seal)
	var inventoryDir string
	inventory := &cobra.Command{Use: "inventory", Short: "Hash the complete stopped data tree for a legacy adoption receipt", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) (err error) {
		lock, err := writer.AcquireBrain(inventoryDir)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, lock.Close()) }()
		digest, err := runtimebroker.InventorySHA256(cmd.Context(), inventoryDir)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(cmd.OutOrStdout(), digest)
		return err
	}}
	inventory.Flags().StringVar(&inventoryDir, "data-dir", "", "private stopped data directory")
	mustRequired(inventory, "data-dir")
	parent.AddCommand(inventory)
	return parent
}
func approvedSeal(generation, sequence int64, writer, expected string, at time.Time) (string, []byte, error) {
	if generation < 1 || generation > 10000 || sequence < 1 || sequence > 9999999999 || writer == "" || strings.ContainsAny(writer, " \t\r\n") || at.IsZero() {
		return "", nil, errors.New("exact valid seal coordinates and writer required")
	}
	previous := expected
	if expected == "genesis" {
		if generation != 1 || sequence != 1 {
			return "", nil, errors.New("genesis is only the first generation position")
		}
		previous = ""
	} else {
		hash, err := hex.DecodeString(expected)
		if err != nil || len(hash) != 32 || strings.ToLower(expected) != expected {
			return "", nil, errors.New("exact previous hash required")
		}
	}
	o := contracts.JournalObject{Kind: contracts.JournalKindSeal, Generation: generation, SequenceID: sequence, PrevHash: previous, WriterID: writer, RecordedAt: at.UTC()}
	body, err := o.Encode()
	return contracts.JournalKey(generation, sequence), body, err
}
