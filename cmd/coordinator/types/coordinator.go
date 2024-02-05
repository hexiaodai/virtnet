package types

import (
	"context"
	"fmt"
	"net"
	"os"

	"github.com/cilium/cilium/pkg/mac"
	"github.com/containernetworking/plugins/pkg/ip"
	"github.com/containernetworking/plugins/pkg/ns"
	"github.com/hexiaodai/virtnet/pkg/logging"
	"github.com/hexiaodai/virtnet/pkg/networking/networking"
	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
)

type Coordinator struct {
	FirstInvoke bool

	IPFamily         int
	CurrentRuleTable int
	HostRuleTable    int

	HostVethName     string
	PodVethName      string
	CurrentInterface string

	HijackCIDR []string

	Netns  ns.NetNS
	HostNs ns.NetNS

	HostVethHwAddress net.HardwareAddr
	PodVethHwAddress  net.HardwareAddr

	CurrentPodAddress []netlink.Addr

	HostIPRouteForPod []net.IP
}

// SetupVeth sets up a pair of virtual ethernet devices. move one to the host and other
// one to container.
func (c *Coordinator) SetupVeth(ctx context.Context, containerID string) error {
	logger := logging.FromContext(ctx)

	// systemd 242+ tries to set a "persistent" MAC addr for any virtual device
	// by default (controlled by MACAddressPolicy). As setting happens
	// asynchronously after a device has been created, ep.Mac and ep.HostMac
	// can become stale which has a serious consequence - the kernel will drop
	// any packet sent to/from the endpoint. However, we can trick systemd by
	// explicitly setting MAC addrs for both veth ends. This sets
	// addr_assign_type for NET_ADDR_SET which prevents systemd from changing
	// the addrs.
	podVethMAC, err := mac.GenerateRandMAC()
	if err != nil {
		return fmt.Errorf("unable to generate podVeth mac addr: %s", err)
	}

	hostVethMAC, err := mac.GenerateRandMAC()
	if err != nil {
		return fmt.Errorf("unable to generate hostVeth mac addr: %s", err)
	}

	var containerInterface net.Interface
	hostVethName := GenerateHostVethName(containerID)
	err = c.Netns.Do(func(hostNS ns.NetNS) error {
		_, containerInterface, err = ip.SetupVethWithName(c.PodVethName, hostVethName, 1500, podVethMAC.String(), hostNS)
		if err != nil {
			return err
		}

		link, err := netlink.LinkByName(containerInterface.Name)
		if err != nil {
			return err
		}

		if err := netlink.LinkSetUp(link); err != nil {
			return fmt.Errorf("failed to set %q UP: %v", containerInterface.Name, err)
		}
		return nil
	})

	hostVethLink, err := netlink.LinkByName(hostVethName)
	if err != nil {
		return err
	}

	if err = netlink.LinkSetHardwareAddr(hostVethLink, net.HardwareAddr(hostVethMAC)); err != nil {
		return fmt.Errorf("failed to set host veth mac: %v", err)
	}

	logger.Debugw("Successfully to set veth mac", "podVethMac", podVethMAC.String(), "hostVethMac", hostVethMAC.String())
	return nil
}

// SetupNeighborhood setup neighborhood tables for pod and host.
// equivalent to: `ip neigh add ....`
func (c *Coordinator) SetupNeighborhood(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	var err error
	c.HostVethHwAddress, c.PodVethHwAddress, err = networking.GetHwAddressByName(c.Netns, c.PodVethName, c.HostVethName)
	if err != nil {
		logger.Error("failed to GetHwAddressByName", err)
		return fmt.Errorf("failed to GetHwAddressByName: %v", err)
	}

	logger = logger.Named("SetupNeighborhood").With(
		zap.String("HostVethName", c.HostVethName),
		zap.String("HostVethMac", c.HostVethHwAddress.String()),
		zap.String("PodVethName", c.PodVethName),
		zap.String("PodVethMac", c.PodVethHwAddress.String()),
	)

	// do any cleans?
	nList, err := netlink.NeighList(0, c.IPFamily)
	if err != nil {
		logger.Warn("failed to get NeighList, ignore clean dirty neigh table")
	}

	hostVethlink, err := netlink.LinkByName(c.HostVethName)
	if err != nil {
		logger.Error("failed to setup neigh table, couldn't find host veth link", err)
		return fmt.Errorf("failed to setup neigh table, couldn't find host veth link: %v", err)
	}

	for idx := range nList {
		for _, ipAddr := range c.CurrentPodAddress {
			if nList[idx].IP.Equal(ipAddr.IP) {
				if err = netlink.NeighDel(&nList[idx]); err != nil && !os.IsNotExist(err) {
					logger.Warn("failed to clean dirty neigh table, it may cause the pod can't communicate with the node, please clean it up manually",
						"dirty neigh table", nList[idx].String())
				} else {
					logger.Debug("successfully cleaned up the dirty neigh table", "dirty neigh table", nList[idx].String())
				}
				break
			}
		}
	}

	for _, ipAddr := range c.CurrentPodAddress {
		if err = networking.AddStaticNeighborTable(hostVethlink.Attrs().Index, ipAddr.IP, c.PodVethHwAddress); err != nil {
			logger.Errorw("failed to setup neigh table, it may cause the pod can't communicate with the node, please clean it up manually", "error", err)
			return err
		}
	}

	if !c.FirstInvoke {
		return nil
	}

	err = c.Netns.Do(func(_ ns.NetNS) error {
		podVethlink, err := netlink.LinkByName(c.PodVethName)
		if err != nil {
			logger.Error("failed to setup neigh table, couldn't find pod veth link", err)
			return fmt.Errorf("failed to setup neigh table, couldn't find pod veth link: %v", err)
		}

		for _, ipAddr := range c.HostIPRouteForPod {
			if err := networking.AddStaticNeighborTable(podVethlink.Attrs().Index, ipAddr, c.HostVethHwAddress); err != nil {
				logger.Errorw("failed to setup neigh table, it may cause the pod can't communicate with the node, please clean it up manually", "error", err)
				return err
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	logger.Debug("Setup Neighborhood Table Successfully")
	return nil
}

// SetupHostRoutes create routes for all host IPs, make sure that traffic to
// pod's host is forward to veth pair device.
func (c *Coordinator) SetupHostRoutes(ctx context.Context) error {
	logger := logging.FromContext(ctx)

	var err error
	err = c.Netns.Do(func(_ ns.NetNS) error {
		// traffic sent to the pod its node is forwarded via veth0/eth0
		// eq: "ip r add <ipAddressOnNode> dev veth0/eth0 table <ruleTable>"
		for _, hostAddress := range c.HostIPRouteForPod {
			ipNet := networking.ConvertMaxMaskIPNet(hostAddress)
			var src *net.IPNet
			if err = networking.AddRoute(logging.IntoContext(ctx, logger), c.CurrentRuleTable, c.IPFamily, netlink.SCOPE_LINK, c.PodVethName, src, ipNet, nil, nil); err != nil {
				logger.Error("failed to AddRoute for ipAddressOnNode", zap.Error(err))
				return err
			}
		}
		return nil
	})

	if err != nil {
		return err
	}

	var ipFamilies []int
	if c.IPFamily == netlink.FAMILY_ALL {
		ipFamilies = append(ipFamilies, netlink.FAMILY_V4, netlink.FAMILY_V6)
	} else {
		ipFamilies = append(ipFamilies, c.IPFamily)
	}

	// make sure `ip rule from all lookup 500 pref 32765` exist
	rule := netlink.NewRule()
	rule.Table = c.HostRuleTable
	rule.Priority = DefaultHostRulePriority
	for _, ipfamily := range ipFamilies {
		rule.Family = ipfamily
		if err = netlink.RuleAdd(rule); err != nil && !os.IsExist(err) {
			logger.Error("failed to Add ToRuleTable for host", "rule", rule.String(), err)
			return fmt.Errorf("failed to Add ToRuleTable for host(%+v): %v", rule.String(), err)
		}
	}

	for idx := range c.CurrentPodAddress {
		ipNet := networking.ConvertMaxMaskIPNet(c.CurrentPodAddress[idx].IP)

		// do any cleans dirty route tables
		filterRoute := &netlink.Route{
			Dst:   ipNet,
			Table: c.HostRuleTable,
		}

		filterRoutes, err := netlink.RouteListFiltered(c.IPFamily, filterRoute, netlink.RT_FILTER_TABLE)
		if err != nil {
			logger.Warn("failed to fetch route list filter by RT_FILTER_DST, ignore clean dirty route table")
		}

		for idx := range filterRoutes {
			if networking.IPNetEqual(filterRoutes[idx].Dst, ipNet) {
				if err = netlink.RouteDel(&filterRoutes[idx]); err != nil && !os.IsNotExist(err) {
					logger.Warn("failed to clean dirty route table, it may cause the pod can't communicate with the node, please clean it up manually",
						zap.String("dirty route table", filterRoutes[idx].String()))
				} else {
					logger.Debug("successfully cleaned up the dirty route table", zap.String("dirty route table", filterRoutes[idx].String()))
				}
			}
		}

		// set routes for host
		// equivalent: ip add  <chainedIPs> dev <hostVethName> table  on host
		if err = networking.AddRoute(logging.IntoContext(ctx, logger), c.HostRuleTable, c.IPFamily, netlink.SCOPE_LINK, c.HostVethName, nil, ipNet, nil, nil); err != nil {
			logger.Error("failed to AddRouteTable for preInterfaceIPAddress", zap.Error(err))
			return fmt.Errorf("failed to AddRouteTable for preInterfaceIPAddress: %v", err)
		}
		logger.Info("add route for to pod in host", zap.String("Dst", ipNet.String()))
	}

	return nil
}

// setupRoutes setup hijack subnet routes for pod and host
// equivalent to: `ip route add $route table $ruleTable`
func (c *Coordinator) SetupHijackRoutes(ctx context.Context, ruleTable int) error {
	logger := logging.FromContext(ctx)

	v4Gw, v6Gw, err := networking.GetGatewayIP(c.CurrentPodAddress)
	if err != nil {
		logger.Error("failed to GetGatewayIP", zap.Error(err))
		return err
	}

	logger.Debug("SetupHijackRoutes", zap.String("v4Gw", v4Gw.String()), zap.String("v6Gw", v6Gw.String()))

	err = c.Netns.Do(func(_ ns.NetNS) error {
		// make sure that veth0/eth0 forwards traffic within the cluster
		// eq: ip route add <cluster/service cidr> dev veth0/eth0
		for _, hijack := range c.HijackCIDR {
			_, ipNet, err := net.ParseCIDR(hijack)
			// nip, ipNet, err := net.ParseCIDR(hijack)
			if err != nil {
				logger.Error("Invalid Hijack Cidr", zap.String("Cidr", hijack), zap.Error(err))
				return err
			}

			// var src *net.IPNet
			// if nip.To4() != nil {
			// 	if v4Gw == nil {
			// 		logger.Warn("ignore adding hijack routing table(ipv4), due to ipv4 gateway is nil", zap.String("IPv4 Hijack cidr", hijack))
			// 		continue
			// 	}
			// 	src = c.v4PodOverlayNicAddr
			// }

			// if nip.To4() == nil {
			// 	if v6Gw == nil {
			// 		logger.Warn("ignore adding hijack routing table(ipv6), due to ipv6 gateway is nil", zap.String("IPv6 Hijack cidr", hijack))
			// 		continue
			// 	}
			// 	src = c.v6PodOverlayNicAddr
			// }

			if err := networking.AddRoute(logging.IntoContext(ctx, logger), ruleTable, c.IPFamily, netlink.SCOPE_UNIVERSE, c.PodVethName, nil, ipNet, v4Gw, v6Gw); err != nil {
				logger.Error("failed to AddRoute for hijackCIDR", zap.String("Dst", ipNet.String()), zap.Error(err))
				return fmt.Errorf("failed to AddRoute for hijackCIDR: %v", err)
			}
		}
		logger.Debug("AddRouteTable for localCIDRs successfully", zap.Strings("localCIDRs", c.HijackCIDR))

		return nil
	})
	return err
}

// Pod 路由网关 IP
// 主机 IP
func (c *Coordinator) GetAllHostIPRouteForPod(ctx context.Context, ipFamily int) (finalNodeIpList []net.IP, err error) {
	logger := logging.FromContext(ctx)

	// allPodIP 主要是获取网卡 IP 路由的网关地址，这个 Pod 访问网关地址需要走 veth0 网卡
	// get all ip of pod
	var allPodIP []netlink.Addr
	if err := c.Netns.Do(func(netNS ns.NetNS) error {
		allPodIP, err = networking.GetAllIPAddress(ipFamily, []string{`^lo$`})
		if err != nil {
			logger.Error("failed to GetAllIPAddress in pod", zap.Error(err))
			return fmt.Errorf("failed to GetAllIPAddress in pod: %v", err)
		}
		return nil
	}); err != nil {
		logger.Error("failed to all ip of pod", zap.Error(err))
		return nil, err
	}
	logger.Debug("all pod IP", zap.Any("allPodIP", allPodIP))

	finalNodeIpList = []net.IP{}

OUTER1:
	// get node ip by `ip r get podIP`
	for _, item := range allPodIP {
		var t net.IP
		v4Gw, v6Gw, err := networking.GetGatewayIP([]netlink.Addr{item})
		if err != nil {
			return nil, fmt.Errorf("failed to GetGatewayIP for pod ip %+v : %+v ", item, zap.Error(err))
		}
		if len(v4Gw) > 0 && (ipFamily == netlink.FAMILY_V4 || ipFamily == netlink.FAMILY_ALL) {
			t = v4Gw
		} else if len(v6Gw) > 0 && (ipFamily == netlink.FAMILY_V6 || ipFamily == netlink.FAMILY_ALL) {
			t = v6Gw
		}
		for _, k := range finalNodeIpList {
			if k.Equal(t) {
				continue OUTER1
			}
		}
		finalNodeIpList = append(finalNodeIpList, t)
	}

	var defaultNodeInterfacesToExclude = []string{
		"docker.*", "cbr.*", "dummy.*",
		"virbr.*", "lxcbr.*", "veth.*", `^lo$`,
		`^cali.*`, "flannel.*", "kube-ipvs.*",
		"cni.*", "vx-submariner", "cilium*",
	}

	// get additional host ip
	additionalIp, err := networking.GetAllIPAddress(ipFamily, defaultNodeInterfacesToExclude)
	if err != nil {
		return nil, fmt.Errorf("failed to get IPAddressOnNode: %v", err)
	}

OUTER2:
	for _, t := range additionalIp {
		if len(t.IP) == 0 {
			continue OUTER2
		}

		for _, k := range finalNodeIpList {
			if k.Equal(t.IP) {
				continue OUTER2
			}
		}
		if t.IP.To4() != nil {
			if ipFamily == netlink.FAMILY_V4 || ipFamily == netlink.FAMILY_ALL {
				finalNodeIpList = append(finalNodeIpList, t.IP)
			}
		} else {
			if ipFamily == netlink.FAMILY_V6 || ipFamily == netlink.FAMILY_ALL {
				finalNodeIpList = append(finalNodeIpList, t.IP)
			}
		}
	}

	return finalNodeIpList, nil
}
