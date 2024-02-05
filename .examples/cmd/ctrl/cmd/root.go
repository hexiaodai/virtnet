package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

func GetRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "[ctrl]",
		Short:   "VirtNet Controller",
		Long:    "Interacting with kube-apiserver, the VirtNet Agent manages IPPool, Subnet, and Endpoint CRD resources. It enforces validation, creation, and status updates for these CRDs. Additionally, it dynamically scales IP pools, assigns fixed IP addresses to StatefulSets and VirtualMachines, and handles IP address allocation and release, among other functionalities.",
		Example: "virtnet-ctrl ctrl",
	}

	cmd.Run = func(cmd *cobra.Command, args []string) {
		if err := getManagerCommand().Execute(); err != nil {
			logger.Error(err)
			os.Exit(1)
		}
	}

	return cmd
}
