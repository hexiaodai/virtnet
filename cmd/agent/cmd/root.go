package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

func GetRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "[daemon]",
		Short:   "VirtNet Agent",
		Long:    "Interacting with the virtnet-ipam and virtnet-coo plugins through IPC and gRPC, it provides an API for IP allocation and release. This simplifies the installation process for the open-source community Underlay CNI plugins.",
		Example: "virtnet-agent daemon",
	}

	cmd.Run = func(cmd *cobra.Command, args []string) {
		if err := getDaemonCommand().Execute(); err != nil {
			logger.Error(err)
			os.Exit(1)
		}
	}

	return cmd
}
