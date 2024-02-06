package ipam

import (
	"context"
	"fmt"

	pb "github.com/hexiaodai/virtnet/api/v1alpha1/ipam"
	"github.com/hexiaodai/virtnet/pkg/constant"
	"github.com/hexiaodai/virtnet/pkg/endpoint"
	cniip "github.com/hexiaodai/virtnet/pkg/ip"
	"github.com/hexiaodai/virtnet/pkg/ippool"
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	"github.com/hexiaodai/virtnet/pkg/pod"
	"github.com/hexiaodai/virtnet/pkg/ptr"
	"github.com/hexiaodai/virtnet/pkg/types"
	"github.com/hexiaodai/virtnet/pkg/utils/convert"
	"k8s.io/apimachinery/pkg/api/errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
)

var _ pb.IpamServer = (*ipam)(nil)

type ipam struct {
	ippoolClient   *ippool.IPPoolClient
	endpointClient *endpoint.EndpointClient
	podClient      *pod.PodClient
}

func NewIPAM(ippoolClient *ippool.IPPoolClient, podClient *pod.PodClient, endpointClient *endpoint.EndpointClient) pb.IpamServer {
	return &ipam{
		ippoolClient:   ippoolClient,
		podClient:      podClient,
		endpointClient: endpointClient,
	}
}

func (i *ipam) Allocate(ctx context.Context, request *pb.AllocateRequest) (*pb.AllocateReply, error) {
	logger := logger.Named("Allocate").With(
		"CNICommand", "ADD",
		"ContainerID", request.ContainerID,
		"IfName", request.IfName,
		"NetNamespace", request.NetNamespace,
		"DefaultSubnet", request.DefaultSubnet,
		"PodNamespace", request.PodNamespace,
		"PodName", request.PodName,
		"PodUID", request.PodUID,
	)

	logger.Debug("Start allocate")

	reply := &pb.AllocateReply{}

	endpoint, err := i.endpointClient.GetEndpointFromCache(ctx, request.PodName, request.PodNamespace)
	if err != nil && !errors.IsNotFound(err) {
		logger.Errorw("endpointClient.GetEndpointFromCache()", "Error", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	logger.Debugw("endpointClient.GetEndpointFromCache()", "Endpoint", endpoint)

	if endpoint != nil {
		for _, ip := range endpoint.Status.Current.IPs {
			reply.IPs = append(reply.IPs, &pb.IPConfig{
				Address: ptr.OrEmpty(ip.IPv4),
				Nic:     ip.NIC,
				Version: pb.EnumIPVersion_IPv4,
			})
		}
		logger.Debugw("reply", reply)
		return reply, nil
	}

	pod, err := i.podClient.GetPodByNameFromCache(ctx, apitypes.NamespacedName{Namespace: request.PodNamespace, Name: request.PodName})
	if err != nil {
		logger.Errorw("podClient.GetPodByNameFromCache()", "Error", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	logger.Debugw("podClient.GetPodByNameFromCache()", "Pod", pod)

	subnet, err := getSubnetCandidates(request.DefaultSubnet, pod)
	if err != nil {
		logger.Errorw("getSubnetCandidates()", "Error", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	logger.Debugw("getSubnetCandidates()", "subnet", subnet)

	ipAllocationDetails := []v1alpha1.IPAllocationDetail{}

	ippool, ip, err := i.ippoolClient.AllocateIP(ctx, subnet, pod)
	if err != nil {
		logger.Errorw("ippoolClient.AllocateIP", "Error", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to allocate IP in subnet '%v': %v", subnet.IPv4Subnet, err))
	}
	logger.Debugw("ippoolClient.AllocateIP()", "IPPool", ippool.Name, "IP", ip)

	ipNet, err := cniip.ParseIP(constant.IPv4, ippool.Spec.Subnet, true)
	if err != nil {
		logger.Errorf("failed to parse subnet '%v': %v", ippool.Spec.Subnet, err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to parse subnet '%v': %v", ippool.Spec.Subnet, err))
	}
	ipNet.IP = ip
	address := ipNet.String()

	ipAllocationDetails = append(ipAllocationDetails, v1alpha1.IPAllocationDetail{
		NIC:         subnet.NIC,
		IPv4:        &address,
		IPv4Pool:    &ippool.Name,
		IPv4Gateway: ippool.Spec.Gateway,
	})
	reply.IPs = append(reply.IPs, &pb.IPConfig{
		Address: address,
		Nic:     subnet.NIC,
		Version: pb.EnumIPVersion_IPv4,
	})

	endpointOwnerRef, endpointStatusOwnerRef, podTopOwnerRef, err := i.podClient.GetEndpointOwnerAndEndpointStatusOwnerAndTopOwnerRef(ctx, pod)
	if err != nil {
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to get pod top owner ref: %v", err))
	}

	if err := i.endpointClient.CreateEndpoint(ctx, request, reply, pod, ipAllocationDetails, endpointOwnerRef, endpointStatusOwnerRef, podTopOwnerRef); err != nil {
		logger.Errorw("endpointClient.CreateEndpoint()", "Error", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create endpoint: %v", err))
	}

	return reply, nil
}

func (i *ipam) Release(ctx context.Context, request *pb.ReleaseRequest) (*pb.ReleaseReply, error) {
	logger := logger.Named("Release").With(
		"CNICommand", "DELETE",
		"ContainerID", request.ContainerID,
		"IfName", request.IfName,
		"PodNamespace", request.PodNamespace,
		"PodName", request.PodName,
		"PodUID", request.PodUID,
	)

	logger.Debug("Start release")

	return &pb.ReleaseReply{}, nil
}

func getSubnetCandidates(defaultSubnet string, pod *corev1.Pod) (types.IPAMAnnoSubnet, error) {
	defaultIPPool := func() (types.IPAMAnnoSubnet, error) {
		if len(defaultSubnet) == 0 {
			return types.IPAMAnnoSubnet{}, status.Error(codes.Internal, "defaultSubnet is empty")
		}
		return types.IPAMAnnoSubnet{
			// default NIC
			NIC:        "eth0",
			IPv4Subnet: defaultSubnet,
		}, nil
	}

	if pod.Annotations == nil {
		return defaultIPPool()
	}

	if data, ok := pod.Annotations[constant.IPAMAnnoPodSubnet]; ok {
		return convert.UnmarshalIPAMAnnoSubnet(data)
	}

	return defaultIPPool()
}
