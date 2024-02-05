package pod

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/hexiaodai/virtnet/pkg/constant"
	cniip "github.com/hexiaodai/virtnet/pkg/ip"
	"github.com/hexiaodai/virtnet/pkg/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/hexiaodai/virtnet/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"go.uber.org/zap"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apitypes "k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var clientLogger *zap.SugaredLogger

type PodClient struct {
	client       ctrlclient.Client
	cache        cache.Cache
	clientReader client.Reader
}

func NewPodClient(client ctrlclient.Client, cache cache.Cache, clientReader client.Reader) *PodClient {
	if clientLogger == nil {
		clientLogger = logger.Named("IPPoolClient")
	}
	clientLogger.Debug("Setting up podClient")
	return &PodClient{
		client:       client,
		cache:        cache,
		clientReader: clientReader,
	}
}

func (client *PodClient) GetPodByNameFromCache(ctx context.Context, namespacedName apitypes.NamespacedName) (*corev1.Pod, error) {
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: corev1.SchemeGroupVersion.WithKind("Pod"),
		Namespace:        namespacedName.Namespace,
		Name:             namespacedName.Name,
	}

	var pod corev1.Pod
	if err := client.cache.Get(ctx, namespacedName, &pod); err != nil {
		return nil, fmt.Errorf("failed to get pod using 'client.cache.Get()', resource is '%v': %w", appNamespacedName.String(), err)
	}

	return &pod, nil
}

func (client *PodClient) GetServiceClusterIPRange(ctx context.Context) (*net.IPNet, error) {
	var list corev1.PodList
	if err := client.cache.List(ctx, &list, &ctrlclient.ListOptions{
		Namespace: metav1.NamespaceSystem,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			"component": "kube-apiserver",
		}),
		Limit: 1,
	}); err != nil {
		return nil, fmt.Errorf("failed to get 'kube-apiserver' pod: %w", err)
	}
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("failed to get 'kube-apiserver' pod")
	}
	kubeAPIServerPod := list.Items[0]
	var kubeAPIServerContainer *corev1.Container
	for _, container := range kubeAPIServerPod.Spec.Containers {
		if container.Name == "kube-apiserver" {
			kubeAPIServerContainer = &container
			break
		}
	}
	if kubeAPIServerContainer == nil {
		return nil, fmt.Errorf("kube-apiserver not found in %v/%v", kubeAPIServerPod.Namespace, kubeAPIServerPod.Name)
	}

	for _, cmd := range kubeAPIServerContainer.Command {
		if !strings.HasPrefix(cmd, "--service-cluster-ip-range=") {
			continue
		}
		splitString := strings.Split(cmd, "=")
		if len(splitString) != 2 {
			return nil, fmt.Errorf("failed to get '--service-cluster-ip-range' from 'kube-apiserver' container")
		}
		ipRange := splitString[1]
		ipnet, err := cniip.ParseIP(constant.IPv4, ipRange, true)
		if err != nil {
			return nil, fmt.Errorf("failed to parse '--service-cluster-ip-range' from 'kube-apiserver' container: %w", err)
		}
		return ipnet, nil
	}
	return nil, fmt.Errorf("'--service-cluster-ip-range' not found in pod %v/%v, container kube-apiserver", kubeAPIServerPod.Namespace, kubeAPIServerPod.Name)
}

func (client *PodClient) GetTopOwnerRef(ctx context.Context, pod *corev1.Pod) (*metav1.OwnerReference, error) {
	defaultOwnerRef := func(typeMeta metav1.TypeMeta, objectMeta metav1.ObjectMeta) *metav1.OwnerReference {
		return &metav1.OwnerReference{
			APIVersion:         typeMeta.APIVersion,
			Kind:               typeMeta.Kind,
			Name:               objectMeta.Name,
			UID:                objectMeta.UID,
			BlockOwnerDeletion: ptr.Of(true),
		}
	}

	if len(pod.ObjectMeta.OwnerReferences) == 0 {
		logger.Debugf("pod %v/%v has no owner", pod.Namespace, pod.Name)
		return defaultOwnerRef(pod.TypeMeta, pod.ObjectMeta), nil
	}

	podOwnerRef := pod.ObjectMeta.OwnerReferences[0]
	switch podOwnerRef.Kind {
	case "ReplicaSet":
		rs := appsv1.ReplicaSet{}
		if err := client.clientReader.Get(ctx, apitypes.NamespacedName{Namespace: pod.Namespace, Name: podOwnerRef.Name}, &rs); err != nil {
			return nil, err
		}
		if len(rs.OwnerReferences) == 0 {
			logger.Debugf("replicaset %v/%v has no owner", rs.Namespace, rs.Name)
			return defaultOwnerRef(rs.TypeMeta, rs.ObjectMeta), nil
		}
		return &rs.ObjectMeta.OwnerReferences[0], nil
	case "DaemonSet", "StatefulSet":
		return &pod.ObjectMeta.OwnerReferences[0], nil
	case "VirtualMachineInstance":
		vmi := virtv1.VirtualMachineInstance{}
		if err := client.clientReader.Get(ctx, apitypes.NamespacedName{Namespace: pod.Namespace, Name: podOwnerRef.Name}, &vmi); err != nil {
			return nil, err
		}
		if len(vmi.OwnerReferences) == 0 {
			logger.Debug("virtualmachineinstance %v/%v has no owner", vmi.Namespace, vmi.Name)
			return defaultOwnerRef(vmi.TypeMeta, vmi.ObjectMeta), nil
		}
		return &vmi.ObjectMeta.OwnerReferences[0], nil
	}

	logger.Debugf("unable to determine owner of pod %v/%v", pod.Namespace, pod.Name)
	return defaultOwnerRef(pod.TypeMeta, pod.ObjectMeta), nil
}
