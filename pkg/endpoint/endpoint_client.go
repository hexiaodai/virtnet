package endpoint

import (
	"context"

	pb "github.com/hexiaodai/virtnet/api/v1alpha1/ipam"
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var clientLogger *zap.SugaredLogger

type EndpointClient struct {
	client client.Client
	cache  cache.Cache
}

func NewEndpointClient(client client.Client, cache cache.Cache) *EndpointClient {
	if clientLogger == nil {
		clientLogger = logger.Named("EndpointClient")
	}
	clientLogger.Debug("Setting up endpointClient")
	return &EndpointClient{
		client: client,
		cache:  cache,
	}
}

func (ec *EndpointClient) CreateEndpoint(ctx context.Context, request *pb.AllocateRequest, reply *pb.AllocateReply, pod *corev1.Pod, ipAllocationDetails []v1alpha1.IPAllocationDetail, endpointOwnerRef, endpointStatusOwnerRef, podTopCtrlRef *metav1.OwnerReference) error {
	return ec.client.Create(ctx, generateEndpoint(request, reply, pod, ipAllocationDetails, endpointOwnerRef, endpointStatusOwnerRef, podTopCtrlRef))
}

func (ec *EndpointClient) GetEndpointFromCache(ctx context.Context, podName, podNamespace string) (*v1alpha1.Endpoint, error) {
	endpoint := v1alpha1.Endpoint{}
	if err := ec.cache.Get(ctx, client.ObjectKey{Namespace: podNamespace, Name: podName}, &endpoint); err != nil {
		return nil, err
	}
	return &endpoint, nil
}

func generateEndpoint(request *pb.AllocateRequest, reply *pb.AllocateReply, pod *corev1.Pod, ipAllocationDetails []v1alpha1.IPAllocationDetail, endpointOwnerRef, endpointStatusOwnerRef, podTopCtrlRef *metav1.OwnerReference) *v1alpha1.Endpoint {
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

	endpoint.Status.OwnerControllerType = endpointStatusOwnerRef.Kind
	endpoint.Status.OwnerControllerName = endpointStatusOwnerRef.Name

	endpoint.SetOwnerReferences([]metav1.OwnerReference{*endpointOwnerRef})
	controllerutil.AddFinalizer(&endpoint, v1alpha1.IPPoolFinalizer)
	return &endpoint
}
