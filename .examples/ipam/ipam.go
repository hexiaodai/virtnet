package ipam

import (
	"context"
	"fmt"
	"sync"

	pb "github.com/hexiaodai/virtnet/api/v1alpha1/ipam"
	"github.com/hexiaodai/virtnet/pkg/barrel"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
)

var _ pb.IpamServer = (*ipam)(nil)

type ipam struct {
	ippoolClient   *ippool.IPPoolClient
	barrelClient   *barrel.BarrelClient
	endpointClient *endpoint.EndpointClient
	podClient      *pod.PodClient
}

func NewIPAM(
	ippoolClient *ippool.IPPoolClient,
	barrelClient *barrel.BarrelClient,
	podClient *pod.PodClient,
	endpointClient *endpoint.EndpointClient,
) pb.IpamServer {
	return &ipam{
		ippoolClient:   ippoolClient,
		barrelClient:   barrelClient,
		podClient:      podClient,
		endpointClient: endpointClient,
	}
}

func (i *ipam) Allocate(ctx context.Context, request *pb.AllocateRequest) (*pb.AllocateReply, error) {
	if err := request.ValidateAll(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	logger := logger.Named("Allocate").With(
		"CNICommand", "ADD",
		"ContainerID", request.ContainerID,
		"IfName", request.IfName,
		"NetNamespace", request.NetNamespace,
		"DefaultIPPools", request.DefaultIPPools,
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

	ippools, err := getPoolCandidates(request.DefaultIPPools, pod)
	if err != nil {
		logger.Errorw("getPoolCandidates()", "Error", err)
		return nil, status.Error(codes.Internal, err.Error())
	}
	logger.Debugw("getPoolCandidates()", "IPPools", ippools)

	ipsLock := sync.Mutex{}
	ipAllocationDetails := []v1alpha1.IPAllocationDetail{}
	eg, errgroupctx := errgroup.WithContext(ctx)
	eg.SetLimit(len(ippools))
	for _, ippool := range ippools {
		copyIPPool := ippool
		eg.Go(func() error {
			select {
			case <-errgroupctx.Done():
				logger.Warnw("context canceled", "IPPool", copyIPPool)
				return nil
			default:
			}

			ippoolName, ip, err := i.barrelClient.AllocateIP(errgroupctx, copyIPPool, pod)
			if err != nil {
				return err
			}
			logger.Debugw("ippoolClient.AllocateIP()", "IPPool", copyIPPool, "IP", ip)

			ippool, err := i.ippoolClient.GetIPPoolByNameFromCache(errgroupctx, ippoolName)
			if err != nil {
				return err
			}
			ipNet, _ := cniip.ParseIP(constant.IPv4, ippool.Spec.Subnet, true)
			ipNet.IP = ip
			address := ipNet.String()

			ipsLock.Lock()
			ipAllocationDetails = append(ipAllocationDetails, v1alpha1.IPAllocationDetail{
				NIC:         copyIPPool.NIC,
				IPv4:        &address,
				IPv4Pool:    &ippool.Name,
				IPv4Gateway: ippool.Spec.Gateway,
			})
			reply.IPs = append(reply.IPs, &pb.IPConfig{
				Address: address,
				Nic:     copyIPPool.NIC,
				Version: pb.EnumIPVersion_IPv4,
			})
			ipsLock.Unlock()
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		logger.Errorw("ippoolClient.AllocateIP()", "Error", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to allocate IP using 'ippoolClient.AllocateIP()': %v", err))
	}

	if err := i.endpointClient.CreateEndpoint(ctx, genEndpoint(request, reply, pod, ipAllocationDetails)); err != nil {
		logger.Errorw("endpointClient.CreateEndpoint()", "Error", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to create endpoint: %v", err))
	}

	return reply, nil
}

func (i *ipam) Release(ctx context.Context, request *pb.ReleaseRequest) (*pb.ReleaseReply, error) {
	if err := request.ValidateAll(); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

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

// func (i *ipam) Release(ctx context.Context, request *pb.ReleaseRequest) (*pb.ReleaseReply, error) {
// 	if err := request.ValidateAll(); err != nil {
// 		return nil, status.Error(codes.InvalidArgument, err.Error())
// 	}

// 	logger := logger.Named("Release").With(
// 		"CNICommand", "DELETE",
// 		"ContainerID", request.ContainerID,
// 		"IfName", request.IfName,
// 		"PodNamespace", request.PodNamespace,
// 		"PodName", request.PodName,
// 		"PodUID", request.PodUID,
// 	)

// 	logger.Debug("Start release")

// 	endpoint, err := i.endpointClient.GetEndpointFromCache(ctx, request.PodName, request.PodNamespace)
// 	if err != nil {
// 		return nil, status.Error(codes.Internal, "failed to get endpoint from cache: endpointClient.GetEndpointFromCache()")
// 	}

// 	selectedBarrels := []*v1alpha1.Barrel{}
// 	for _, ip := range endpoint.Status.Current.IPs {
// 		barrels, err := i.barrelClient.ListBarrelsWithOwnerFromCache(ctx, ptr.OrEmpty(ip.IPv4Pool))
// 		if err != nil {
// 			return nil, status.Error(codes.Internal, "failed to list Barrels 'barrelClient.ListBarrelsFromCache()'")
// 		}
// 		for _, barrel := range barrels.Items {
// 			allocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(barrel.Status.AllocatedIPs)
// 			if err != nil {
// 				logger.Errorf("failed to unmarshal AllocatedIPs with 'convert.UnmarshalIPPoolAllocatedIPs()': %v", err)
// 				continue
// 			}
// 			for _, allocatedIP := range allocatedIPs {
// 				nn := apitypes.NamespacedName{Namespace: request.PodNamespace, Name: request.PodName}
// 				if allocatedIP.PodUID == request.PodUID && allocatedIP.NamespacedName == nn.String() {
// 					selectedBarrelNamesLock.Lock()
// 					selectedBarrelNames = append(selectedBarrelNames, copyBarrel.Name)
// 					selectedBarrelNamesLock.Unlock()
// 					break
// 				}
// 			}
// 		}
// 	}

// 	return &pb.ReleaseReply{}, nil
// }

// func (i *ipam) Release(ctx context.Context, request *pb.ReleaseRequest) (*pb.ReleaseReply, error) {
// 	if err := request.ValidateAll(); err != nil {
// 		return nil, status.Error(codes.InvalidArgument, err.Error())
// 	}

// 	logger := logger.Named("Release")

// 	barrels, err := i.barrelClient.ListBarrelsFromCache(ctx)
// 	if err != nil {
// 		return nil, status.Error(codes.Internal, "failed to list Barrels 'barrelClient.ListBarrelsFromCache()'")
// 	}

// 	selectedBarrelNames := []string{}
// 	selectedBarrelNamesLock := sync.Mutex{}

// 	// eg, ctx := errgroup.WithContext(ctx)
// 	eg := errgroup.Group{}
// 	eg.SetLimit(32)
// 	for _, barrel := range barrels.Items {
// 		copyBarrel := barrel
// 		eg.Go(func() error {
// 			allocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(copyBarrel.Status.AllocatedIPs)
// 			if err != nil {
// 				logger.Errorf("failed to unmarshal AllocatedIPs with 'convert.UnmarshalIPPoolAllocatedIPs()': %v", err)
// 				return nil
// 			}
// 			for _, allallocatedIP := range allocatedIPs {
// 				nn := apitypes.NamespacedName{Namespace: request.PodNamespace, Name: request.PodName}
// 				if allallocatedIP.PodUID == request.PodUID && allallocatedIP.NamespacedName == nn.String() {
// 					selectedBarrelNamesLock.Lock()
// 					selectedBarrelNames = append(selectedBarrelNames, copyBarrel.Name)
// 					selectedBarrelNamesLock.Unlock()
// 					break
// 				}
// 			}
// 			return nil
// 		})
// 	}

// 	if err := eg.Wait(); err != nil {
// 		return nil, status.Error(codes.Internal, fmt.Sprintf("failed to wait for goroutines with 'eg.Wait()': %v", err))
// 	}

// 	// pod, err := i.podClient.GetPodByNameFromCache(ctx, apitypes.NamespacedName{Namespace: request.PodNamespace, Name: request.PodName})
// 	// if err != nil {

// 	// }

// 	// ippools, err := i.ippoolClient.ListIPPoolsFromCache(ctx)
// 	// if err != nil {
// 	// 	logger.Errorw("ippoolClient.ListIPPoolsFromCache()", "Error", err)
// 	// 	return nil, status.Error(codes.Internal, err.Error())
// 	// }

// 	// for _, ippool := range ippools.Items {
// 	// 	for _, iprange := range ippool.Spec.IPs {
// 	// 		cniip.IPRangeContainsIP(constant.IPv4, iprange, "")
// 	// 	}
// 	// }

// 	return nil, nil
// }

func getPoolCandidates(defaultPools []string, pod *corev1.Pod) (types.IPAMAnnoPodIPPoolsValue, error) {
	defaultIPPool := func() (types.IPAMAnnoPodIPPoolsValue, error) {
		if len(defaultPools) == 0 {
			return nil, status.Error(codes.Internal, "defaultPools is empty")
		}
		return types.IPAMAnnoPodIPPoolsValue{types.IPAMAnnoIPPoolItem{
			// default NIC
			NIC:       "eth0",
			IPv4Pools: defaultPools,
		}}, nil
	}

	if pod.Annotations == nil {
		return defaultIPPool()
	}

	if data, ok := pod.Annotations[constant.IPAMAnnoPodIPPools]; ok {
		return convert.UnmarshalIPAMAnnoPodIPPools(data)
	}

	if data, ok := pod.Annotations[constant.IPAMAnnoPodIPPool]; ok {
		value, err := convert.UnmarshalIPAMAnnoPodIPPool(data)
		if err != nil {
			return nil, err
		}
		return types.IPAMAnnoPodIPPoolsValue{value}, nil
	}

	return defaultIPPool()
}

func genEndpoint(request *pb.AllocateRequest, reply *pb.AllocateReply, pod *corev1.Pod, ipAllocationDetails []v1alpha1.IPAllocationDetail) *v1alpha1.Endpoint {
	endpoint := v1alpha1.Endpoint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
		Status: v1alpha1.EndpointStatus{
			Current: v1alpha1.PodIPAllocation{
				UID:  string(pod.UID),
				Node: pod.Spec.NodeName,
				IPs:  ipAllocationDetails,
			},
		},
	}

	var ownerRef metav1.OwnerReference
	// FIXME: topController
	if len(pod.ObjectMeta.OwnerReferences) > 0 {
		ownerRef = pod.ObjectMeta.OwnerReferences[0]
		endpoint.Status.OwnerControllerType = ownerRef.Kind
		endpoint.Status.OwnerControllerName = ownerRef.Name
	} else {
		endpoint.Status.OwnerControllerType = pod.Kind
		endpoint.Status.OwnerControllerName = pod.Name
	}

	if ownerRef.Kind != "VirtualMachineInstance" &&
		ownerRef.Kind != "StatefulSet" {
		endpoint.SetOwnerReferences([]metav1.OwnerReference{
			{
				APIVersion: pod.APIVersion,
				Kind:       pod.Kind,
				Name:       pod.Name,
				UID:        pod.UID,
			},
		})
	}

	controllerutil.AddFinalizer(&endpoint, v1alpha1.BarrelFinalizer)
	return &endpoint
}
