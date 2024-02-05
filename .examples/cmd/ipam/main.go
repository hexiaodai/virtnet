package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/containernetworking/cni/pkg/skel"
	cnitypes "github.com/containernetworking/cni/pkg/types"
	current "github.com/containernetworking/cni/pkg/types/100"
	cnispecversion "github.com/containernetworking/cni/pkg/version"
	pbipam "github.com/hexiaodai/virtnet/api/v1alpha1/ipam"
	"github.com/hexiaodai/virtnet/cmd/ipam/types"
	"github.com/hexiaodai/virtnet/pkg/constant"
	cniip "github.com/hexiaodai/virtnet/pkg/ip"
	"github.com/hexiaodai/virtnet/pkg/logging"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var version string

func main() {
	logger := logging.LoggerFromOutputFile(logging.LogLevelDebug, logging.DefaultIPAMPluginLogFilePath)
	logger.Named(types.BinNamePlugin).With(zap.Any("os.Args", os.Args)).Debug("Starting VirtNest IPAM plugin")

	skel.PluginMain(cmdAdd, cmdCheck, cmdDel,
		cnispecversion.All,
		"VirtNest IPAM "+version)
}

func cmdAdd(args *skel.CmdArgs) error {
	startTime := time.Now()
	var logger *zap.SugaredLogger

	conf, err := types.LoadNetConf(args.StdinData)
	if nil != err {
		return fmt.Errorf("failed to load CNI network configuration: %v", err)
	}

	logger = logging.LoggerFromOutputFile(conf.IPAM.LogLevel, conf.IPAM.LogOutputFilePath).Named(types.BinNamePlugin).With(
		zap.String("Action", "Add"),
		zap.String("ContainerID", args.ContainerID),
		zap.String("Netns", args.Netns),
		zap.String("IfName", args.IfName),
	)
	logger.Debug("Processing CNI ADD request")
	logger.Debugf("CNI network configuration: %+v", *conf)

	k8sArgs := types.K8sArgs{}
	if err = cnitypes.LoadArgs(args.Args, &k8sArgs); nil != err {
		return fmt.Errorf("failed to load CNI ENV args: %w", err)
	}

	logger = logger.With(
		zap.String("PodName", string(k8sArgs.K8S_POD_NAME)),
		zap.String("PodNamespace", string(k8sArgs.K8S_POD_NAMESPACE)),
		zap.String("PodUID", string(k8sArgs.K8S_POD_UID)),
	)
	logger.Debugw("Loaded CNI ENV args", "types.K8sArgs", k8sArgs)

	logger.Debugf("Connecting to IPAM server at %s", conf.IPAM.IPAMUnixSocketPath)
	conn, err := grpc.Dial(fmt.Sprintf("unix:%s", conf.IPAM.IPAMUnixSocketPath), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to ipam server: %v", err)
	}
	defer conn.Close()
	ipamClient := pbipam.NewIpamClient(conn)

	logger.Debug("Requesting IP address allocation from IPAM server")
	reply, err := ipamClient.Allocate(context.TODO(), &pbipam.AllocateRequest{
		ContainerID:    args.ContainerID,
		NetNamespace:   args.Netns,
		IfName:         args.IfName,
		PodName:        string(k8sArgs.K8S_POD_NAME),
		PodNamespace:   string(k8sArgs.K8S_POD_NAMESPACE),
		PodUID:         string(k8sArgs.K8S_POD_UID),
		DefaultIPPools: conf.IPAM.DefaultIPv4IPPool,
	})
	if err != nil {
		return fmt.Errorf("'ipamClient.Allocate()': %w", err)
	}
	logger.Debugw("IP address allocation response", "reply", reply)

	result := &current.Result{
		CNIVersion: conf.CNIVersion,
	}

	for _, ip := range reply.IPs {
		if ip.Nic != args.IfName {
			continue
		}
		ipVersion := constant.IPv4
		if ip.Version == pbipam.EnumIPVersion_IPv6 {
			ipVersion = constant.IPv6
		}
		address, err := cniip.ParseIP(ipVersion, ip.Address, true)
		if err != nil {
			return fmt.Errorf("failed to parse IP address: %v", err)
		}
		result.IPs = append(result.IPs, &current.IPConfig{
			Address: *address,
		})
	}

	logger.Debugf("IPAM allocation result: %+v", result)
	logger.Infof("IPAM end, time cost: %v", time.Since(startTime))
	return cnitypes.PrintResult(result, conf.CNIVersion)
}

func cmdDel(args *skel.CmdArgs) error {
	var logger *zap.SugaredLogger

	conf, err := types.LoadNetConf(args.StdinData)
	if nil != err {
		return fmt.Errorf("failed to load CNI network configuration: %v", err)
	}

	logger = logging.LoggerFromOutputFile(conf.IPAM.LogLevel, conf.IPAM.LogOutputFilePath).Named(types.BinNamePlugin).With(
		zap.String("Action", "Delete"),
		zap.String("ContainerID", args.ContainerID),
		zap.String("Netns", args.Netns),
		zap.String("IfName", args.IfName),
	)
	logger.Debug("Processing CNI DELETE request")
	logger.Debugf("CNI network configuration: %+v", *conf)

	k8sArgs := types.K8sArgs{}
	if err = cnitypes.LoadArgs(args.Args, &k8sArgs); nil != err {
		return fmt.Errorf("failed to load CNI ENV args: %w", err)
	}

	logger = logger.With(
		zap.String("PodName", string(k8sArgs.K8S_POD_NAME)),
		zap.String("PodNamespace", string(k8sArgs.K8S_POD_NAMESPACE)),
		zap.String("PodUID", string(k8sArgs.K8S_POD_UID)),
	)
	logger.Debugw("Loaded CNI ENV args", "types.K8sArgs", k8sArgs)

	logger.Debugf("Connecting to IPAM server at %s", conf.IPAM.IPAMUnixSocketPath)
	conn, err := grpc.Dial(fmt.Sprintf("unix:%s", conf.IPAM.IPAMUnixSocketPath), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to ipam server: %v", err)
	}
	defer conn.Close()
	ipamClient := pbipam.NewIpamClient(conn)

	logger.Debug("Requesting IP address allocation from IPAM server")
	reply, err := ipamClient.Release(context.TODO(), &pbipam.ReleaseRequest{
		ContainerID:  args.ContainerID,
		IfName:       args.IfName,
		NetNamespace: args.Netns,
		PodName:      string(k8sArgs.K8S_POD_NAME),
		PodNamespace: string(k8sArgs.K8S_POD_NAMESPACE),
		PodUID:       string(k8sArgs.K8S_POD_UID),
	})
	if err != nil {
		return fmt.Errorf("'ipamClient.Allocate()': %v", err)
	}
	logger.Debug("IPAM server response:", reply)
	logger.Debug("IPAM release successfully")
	return nil
}

func cmdCheck(args *skel.CmdArgs) error {
	return nil
}
