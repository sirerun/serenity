package cli

import (
	"fmt"

	"github.com/sirerun/serenity/internal/hosted/plans"
	"github.com/sirerun/serenity/internal/hosted/service"
	"github.com/spf13/cobra"
)

func newHostedCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "hosted", Short: "Operate the managed Serenity service"}
	cmd.AddCommand(&cobra.Command{Use: "plans", Short: "Print the version 1 hosted pricing and limits", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		data, err := plans.JSON()
		if err != nil {
			return err
		}
		_, err = cmd.OutOrStdout().Write(data)
		return err
	}})
	var path string
	serve := &cobra.Command{Use: "serve", Short: "Serve hosted signup, dashboard and authenticated memory", Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error {
		return fmt.Errorf("%w: hosted serve requires an admitted journal dependency factory", service.ErrStartupUnavailable)
	}}
	serve.Flags().StringVar(&path, "config", "", "hosted JSON configuration file")
	if err := serve.MarkFlagRequired("config"); err != nil {
		panic(err)
	}
	cmd.AddCommand(serve)
	for _, action := range []string{"backup", "restore"} {
		var dataDir, snapshot string
		child := &cobra.Command{Use: action, Short: action + " a hosted control snapshot and brain bundles", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("%w: hosted %s requires an admitted journal operator coordinator", service.ErrStartupUnavailable, action)
		}}
		child.Flags().StringVar(&dataDir, "data-dir", "", "hosted data directory")
		child.Flags().StringVar(&snapshot, "snapshot", "", "local snapshot directory")
		if err := child.MarkFlagRequired("data-dir"); err != nil {
			panic(err)
		}
		if err := child.MarkFlagRequired("snapshot"); err != nil {
			panic(err)
		}
		cmd.AddCommand(child)
	}
	return cmd
}
