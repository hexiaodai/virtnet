package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"time"

	pbcoor "github.com/hexiaodai/virtnet/api/v1alpha1/coordinator"
	"github.com/vishvananda/netlink"

	"github.com/containernetworking/cni/pkg/skel"
	"github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	cnispecversion "github.com/containernetworking/cni/pkg/version"
	"github.com/containernetworking/plugins/pkg/ns"
	coordinatorypes "github.com/hexiaodai/virtnet/cmd/coordinator/types"
	ipamtypes "github.com/hexiaodai/virtnet/cmd/ipam/types"
	"github.com/hexiaodai/virtnet/pkg/errgroup"
	"github.com/hexiaodai/virtnet/pkg/logging"
	"github.com/hexiaodai/virtnet/pkg/networking/ipchecking"
	"github.com/hexiaodai/virtnet/pkg/networking/networking"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func init() {
	// this ensures that main runs only on main thread (thread group leader).
	// since namespace ops (unshare, setns) are done for a single thread, we
	// must ensure that the goroutine does not jump from OS thread to thread
	runtime.LockOSThread()
}

func main() {
	logger := logging.LoggerFromOutputFile(logging.LogLevelDebug, logging.DefaultCoordinatorPluginLogFilePath)
	logger.Named(coordinatorypes.BinNamePlugin).With(zap.Any("os.Args", os.Args)).Debug("Starting VirtNet Coordinator plugin")

	skel.PluginMain(cmdAdd, cmdCheck, cmdDel, cnispecversion.All, "VirtNet Coordinator")
}

func cmdAdd(args *skel.CmdArgs) error {
	startTime := time.Now()

	k8sArgs := ipamtypes.K8sArgs{}
	err := types.LoadArgs(args.Args, &k8sArgs)
	if err != nil {
		return fmt.Errorf("failed to load CNI ENV args: %w", err)
	}

	conf, err := coordinatorypes.ParseConfig(args.StdinData)
	if err != nil {
		return err
	}

	logger := logging.LoggerFromOutputFile(conf.LogLevel, conf.LogOutputFilePath).Named(coordinatorypes.BinNamePlugin).With(
		zap.String("Action", "ADD"),
		zap.String("ContainerID", args.ContainerID),
		zap.String("Netns", args.Netns),
		zap.String("IfName", args.IfName),
		zap.String("PodName", string(k8sArgs.K8S_POD_NAME)),
		zap.String("PodNamespace", string(k8sArgs.K8S_POD_NAMESPACE)),
	)
	logger.Debug("Processing CNI ADD request")

	// parse prevResult
	prevResult, err := current.GetResult(conf.PrevResult)
	if err != nil {
		logger.Error("failed to convert prevResult", zap.Error(err))
		return err
	}

	ipFamily, err := networking.GetIPFamilyByResult(prevResult)
	if err != nil {
		logger.Error("failed to GetIPFamilyByResult", zap.Error(err))
		return err
	}

	logger.Debugf("Connecting to coordinator server at %s", conf.CoordinatorUnixSocketPath)
	conn, err := grpc.Dial(fmt.Sprintf("unix:%s", conf.CoordinatorUnixSocketPath), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to coordinator server: %v", err)
	}
	defer conn.Close()
	coorClient := pbcoor.NewCoordinatorClient(conn)

	logger.Debug("Requesting getCoordinatorConfig from Coordinator server")
	reply, err := coorClient.GetCoordinatorConfig(context.TODO(), &pbcoor.GetCoordinatorConfigRequest{
		PodName:      string(k8sArgs.K8S_POD_NAME),
		PodNamespace: string(k8sArgs.K8S_POD_NAMESPACE),
	})
	if err != nil {
		return fmt.Errorf("'coorClient.GetCoordinatorConfig()': %w", err)
	}
	logger.Debugw("getCoordinatorConfig response", "reply", reply)

	coordinator := coordinatorypes.Coordinator{
		FirstInvoke:      coordinatorypes.K8sFirstNICName == args.IfName,
		IPFamily:         ipFamily,
		CurrentRuleTable: 0,
		HostRuleTable:    0,
		HostVethName:     coordinatorypes.GenerateHostVethName(args.ContainerID),
		PodVethName:      coordinatorypes.DefaultUnderlayVethName,
		CurrentInterface: args.IfName,
		HijackCIDR:       reply.ServiceCIDR,
	}
	logger.Debugw("coordinator initialized", "coordinator", coordinator)

	coordinator.Netns, err = ns.GetNS(args.Netns)
	if err != nil {
		logger.Error("failed to GetNS,", zap.Error(err))
		return fmt.Errorf("failed to GetNS %q: %v", args.Netns, err)
	}
	defer coordinator.Netns.Close()

	coordinator.HostNs, err = ns.GetCurrentNS()
	if err != nil {
		return fmt.Errorf("failed to get current netns: %v", err)
	}
	// defer coordinator.HostNs.Close()

	if coordinator.FirstInvoke {
		if err := coordinator.SetupVeth(logging.IntoContext(context.TODO(), logger), args.ContainerID); err != nil {
			return fmt.Errorf("failed to create veth-pair device: %w", err)
		}
		logger.Debug("Setup veth-pair device successfully", zap.String("hostVethPairName", coordinator.HostVethName))
	}

	errg := errgroup.Group{}
	if conf.IPConflict {
		logger.Debug("Try to detect ip conflict")
		ipc, err := ipchecking.NewIPChecker(coordinatorypes.IPCheckerRetry, coordinatorypes.IPCheckerInterval, coordinatorypes.IPCheckerTimeOut, coordinator.HostNs, coordinator.Netns, logger.Desugar())
		if err != nil {
			return fmt.Errorf("failed to run NewIPChecker: %w", err)
		}
		ipc.DoIPConflictChecking(prevResult.IPs, coordinator.CurrentInterface, &errg)
	} else {
		logger.Debug("disable detect ip conflict")
	}

	logger.Debug("Waiting for ip conflict detection")
	if err = errg.Wait(); err != nil {
		logger.Error("failed to detect gateway and ip checking", zap.Error(err))
		return err
	}

	// 包括了 Pod 路由表的 IP 地址
	// get ip addresses of the node
	coordinator.HostIPRouteForPod, err = coordinator.GetAllHostIPRouteForPod(logging.IntoContext(context.TODO(), logger), ipFamily)
	if err != nil {
		logger.Error("failed to get IPAddressOnNode", zap.Error(err))
		return fmt.Errorf("failed to get IPAddressOnNode: %v", err)
	}
	logger.Debug("host IP for route to Pod", zap.Any("hostIPRouteForPod", coordinator.HostIPRouteForPod))

	// get ips of this interface(preInterfaceName) from, including ipv4 and ipv6
	coordinator.CurrentPodAddress, err = networking.IPAddressByName(coordinator.Netns, args.IfName, ipFamily)
	if err != nil {
		logger.Error(err.Error())
		return fmt.Errorf("failed to IPAddressByName for pod %s : %v", args.IfName, err)
	}
	logger.Debug("Get CurrentPodAddress", zap.Any("CurrentPodAddress", coordinator.CurrentPodAddress))

	if err = coordinator.SetupNeighborhood(logging.IntoContext(context.TODO(), logger)); err != nil {
		logger.Error("failed to SetupNeighborhood", zap.Error(err))
		return err
	}

	if err = coordinator.SetupHostRoutes(logging.IntoContext(context.TODO(), logger)); err != nil {
		logger.Error("failed to SetupHostRoutes", zap.Error(err))
		return err
	}

	if err = coordinator.SetupHijackRoutes(logging.IntoContext(context.TODO(), logger), coordinator.CurrentRuleTable); err != nil {
		logger.Error("failed to SetupHijackRoutes", zap.Error(err))
		return err
	}

	logger.Infof("coordinator end, time cost: %v", time.Since(startTime))
	return types.PrintResult(conf.PrevResult, conf.CNIVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	k8sArgs := ipamtypes.K8sArgs{}
	err := types.LoadArgs(args.Args, &k8sArgs)
	if err != nil {
		return fmt.Errorf("failed to load CNI ENV args: %w", err)
	}

	conf, err := coordinatorypes.ParseConfig(args.StdinData)
	if err != nil {
		return err
	}

	logger := logging.LoggerFromOutputFile(conf.LogLevel, conf.LogOutputFilePath).Named(coordinatorypes.BinNamePlugin).With(
		zap.String("Action", "DELETE"),
		zap.String("ContainerID", args.ContainerID),
		zap.String("Netns", args.Netns),
		zap.String("IfName", args.IfName),
		zap.String("PodName", string(k8sArgs.K8S_POD_NAME)),
		zap.String("PodNamespace", string(k8sArgs.K8S_POD_NAMESPACE)),
	)
	logger.Debug("Processing CNI DELETE request")

	logger.Debugf("Connecting to coordinator server at %s", conf.CoordinatorUnixSocketPath)
	conn, err := grpc.Dial(fmt.Sprintf("unix:%s", conf.CoordinatorUnixSocketPath), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to coordinator server: %v", err)
	}
	defer conn.Close()
	coorClient := pbcoor.NewCoordinatorClient(conn)

	logger.Debug("Requesting getCoordinatorConfig from Coordinator server")
	reply, err := coorClient.GetCoordinatorConfig(context.TODO(), &pbcoor.GetCoordinatorConfigRequest{
		PodName:      string(k8sArgs.K8S_POD_NAME),
		PodNamespace: string(k8sArgs.K8S_POD_NAMESPACE),
	})
	if err != nil {
		return fmt.Errorf("'coorClient.GetCoordinatorConfig()': %w", err)
	}
	logger.Debugw("getCoordinatorConfig response", "reply", reply)

	coordinator := coordinatorypes.Coordinator{
		CurrentRuleTable: 0,
		HostRuleTable:    0,
		HostVethName:     coordinatorypes.GenerateHostVethName(args.ContainerID),
		PodVethName:      coordinatorypes.DefaultUnderlayVethName,
		CurrentInterface: args.IfName,
		HijackCIDR:       reply.ServiceCIDR,
	}
	logger.Debugw("coordinator initialized", "coordinator", coordinator)

	coordinator.Netns, err = ns.GetNS(args.Netns)
	if err != nil {
		if _, ok := err.(ns.NSPathNotExistErr); ok {
			logger.Debug("Pod's netns already gone. Nothing to do.")
			return nil
		}
		logger.Error("failed to GetNS,", zap.Error(err))
		return fmt.Errorf("failed to GetNS %q: %v", args.Netns, err)
	}
	defer coordinator.Netns.Close()

	vethLink, err := netlink.LinkByName(coordinator.HostVethName)
	if err != nil {
		if _, ok := err.(netlink.LinkNotFoundError); ok {
			logger.Debug("Host veth has gone, nothing to do", zap.String("HostVeth", coordinator.HostVethName))
		} else {
			logger.Warn(fmt.Sprintf("failed to get host veth device %s: %v", coordinator.HostVethName, err))
			return fmt.Errorf("failed to get host veth device %s: %v", coordinator.HostVethName, err)
		}
	} else {
		if err = netlink.LinkDel(vethLink); err != nil {
			logger.Warn("failed to del hostVeth", zap.Error(err))
			return fmt.Errorf("failed to del hostVeth %s: %w", coordinator.HostVethName, err)
		}
		logger.Debug("success to del hostVeth", zap.String("HostVeth", coordinator.HostVethName))
	}

	err = coordinator.Netns.Do(func(netNS ns.NetNS) error {
		coordinator.CurrentPodAddress, err = networking.GetAddersByName(args.IfName, netlink.FAMILY_ALL)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		// ignore err
		logger.Warn("failed to GetAddersByName, ignore error", zap.Error(err))
	}

	// Cannot delete main routing table
	// for idx := range coordinator.CurrentPodAddress {
	// 	ipNet := networking.ConvertMaxMaskIPNet(coordinator.CurrentPodAddress[idx].IP)
	// 	err = networking.DelToRuleTable(ipNet, coordinator.HostRuleTable)
	// 	if err != nil && !os.IsNotExist(err) {
	// 		logger.Error("failed to DelToRuleTable", zap.Int("HostRuleTable", coordinator.HostRuleTable), zap.String("Dst", ipNet.String()), zap.Error(err))
	// 		return fmt.Errorf("failed to DelToRuleTable: %v", err)
	// 	}
	// }

	logger.Info("cmdDel end")
	return nil
}

func cmdCheck(args *skel.CmdArgs) error {
	return nil
}
