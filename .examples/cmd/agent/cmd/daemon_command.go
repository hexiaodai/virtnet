package cmd

import (
	"os"

	"github.com/hexiaodai/virtnet/pkg/constant"
	"github.com/hexiaodai/virtnet/pkg/grpc"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	ctrl "sigs.k8s.io/controller-runtime"
)

func getDaemonCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "VirtNet Agent Daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return setupDaemon()
		},
	}

	return cmd
}

func setupDaemon() error {
	ctx := ctrl.SetupSignalHandler()

	logger.Debug("Initialize CRD Manager")
	crdmgr, err := NewCRDManager()
	if err != nil {
		return err
	}

	logger.Debug("Start CRD Manager")
	go func() {
		if err := crdmgr.Mgr.Start(ctx); err != nil {
			panic(err)
		}
	}()

	logger.Debug("Register gRPC Handlers")
	grpchandler := RegisterGRPCHandler(crdmgr)

	logger.Debug("Initialize gRPC Server", zap.String("NetWork", "unix"), zap.String("Address", constant.DefaultUnixSocketPath))
	os.Remove(constant.DefaultUnixSocketPath)
	grpcserver := grpc.NewGRPCServer("unix", constant.DefaultUnixSocketPath, grpchandler)

	logger.Debug("Runing gRPC Server")
	go func() {
		if err := grpcserver.Run(); err != nil {
			panic(err)
		}
	}()
	logger.Debugf("gRPC Server listening at %v", constant.DefaultUnixSocketPath)

	<-ctx.Done()
	logger.Debug("Shutdown gRPC Server")
	if err := grpcserver.Stop(); err != nil {
		return err
	}

	return nil
}
