package cmd

import (
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"
)

func getManagerCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "controller",
		Aliases: []string{"ctrl"},
		Short:   "VirtNet Controller Manager",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setupManager()
		},
	}

	return cmd
}

func setupManager() error {
	ctx := ctrl.SetupSignalHandler()

	logger.Debug("Initialize CRD Manager")
	crdmgr, err := NewCRDManager()
	if err != nil {
		return err
	}

	logger.Debug("Register Ctrl")
	if err := RegisterCtrl(crdmgr); err != nil {
		return err
	}

	logger.Debug("Start Ctrl Manager")
	go func() {
		if err := crdmgr.Start(ctx); err != nil {
			panic(err)
		}
	}()

	<-ctx.Done()

	return nil
}
