package ippool

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/hexiaodai/virtnet/pkg/constant"
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	"github.com/hexiaodai/virtnet/pkg/logging"
	"github.com/hexiaodai/virtnet/pkg/ptr"
	"github.com/hexiaodai/virtnet/pkg/types"
	"github.com/hexiaodai/virtnet/pkg/utils/convert"
	"github.com/hexiaodai/virtnet/pkg/utils/retry"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	apitypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var clientLogger *zap.SugaredLogger

type IPPoolClient struct {
	ClientReader client.Reader
	Client       client.Client
	Cache        ctrlcache.Cache
}

func NewIPPoolClient(clientReader client.Reader, client client.Client, cache ctrlcache.Cache) *IPPoolClient {
	if clientLogger == nil {
		clientLogger = logger.Named("IPPoolClient")
	}
	return &IPPoolClient{
		ClientReader: clientReader,
		Client:       client,
		Cache:        cache,
	}
}

func (ic *IPPoolClient) AllocateIP(ctx context.Context, subnet string, pod *corev1.Pod, podTopOwnerRef *metav1.OwnerReference) (net.IP, error) {
	var ip net.IP
	var ippool v1alpha1.IPPool

	backoff := retry.DefaultRetry
	steps := backoff.Steps
	err := retry.RetryOnConflictWithContext(ctx, backoff, func(ctx context.Context) error {
		ippools := v1alpha1.IPPoolList{}
		ic.ClientReader.List(ctx, &ippools, &client.ListOptions{
			LabelSelector: labels.SelectorFromSet(labels.Set{
				constant.SubnetLabelName:      subnet,
				constant.IPPoolLabelExclusive: convert.PodTopCtrlToIPPoolLabelExclusiveValue(pod.Namespace, podTopOwnerRef),
			}),
			Limit: 1,
		})
		if len(ippools.Items) == 0 {
			return fmt.Errorf("no IPPool found for subnet '%s' and pod '%s/%s' and podTopCtrl '%s'", subnet, pod.Namespace, pod.Name, convert.PodTopCtrlToIPPoolLabelExclusiveValue(pod.Namespace, podTopOwnerRef))
		}
		ippool = ippools.Items[0]
		logger.Debugf("IPPool: %+v", ippool)

		appNamespacedName := types.AppNamespacedName{
			GroupVersionKind: v1alpha1.IPPoolGVK,
			Name:             ippool.Name,
		}
		logger := clientLogger.Named("AllocateIP").With(zap.Any("resource", appNamespacedName), zap.Any("Times", steps-backoff.Steps+1))
		logger.Debug("Re-get Barrel for IP allocation")
		newIP, newIPPool, err := ic.allocateIP(ctx, ippool.DeepCopy(), pod)
		if err != nil {
			return fmt.Errorf("failed to allocate IP from IPPool '%s': %w", ippool.Name, err)
		}
		if err := ic.Client.Status().Update(ctx, newIPPool); err != nil {
			logger.With(
				zap.String("Barrel-ResourceVersion", newIPPool.ResourceVersion),
				zap.String("Subnet", subnet),
				zap.String("IPPool", newIPPool.Name),
			).Warn("An conflict occurred when cleaning the IP allocation records of IPPool")
			return err
		}
		ip = newIP
		return nil
	})
	if err != nil {
		if wait.Interrupted(err) {
			err = fmt.Errorf("exhaust all retries (%d times), failed to allocate IP from IPPool %s", steps, ippool.Name)
		}
		return nil, err
	}
	return ip, nil
}

func (ic *IPPoolClient) allocateIP(ctx context.Context, ippool *v1alpha1.IPPool, pod *corev1.Pod) (net.IP, *v1alpha1.IPPool, error) {
	logging.FromContext(ctx)

	unallocatedIPs, err := convert.UnmarshalIPPoolUnallocatedIPs(ippool.Status.UnallocatedIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal IPPool '%s' unallocated IPs", ippool.Name)
	}
	if len(unallocatedIPs) == 0 {
		return nil, nil, fmt.Errorf("no unallocated IPs in IPPool '%s'", ippool.Name)
	}

	allocatedIPs, err := convert.UnmarshalIPPoolAllocatedIPs(ippool.Status.AllocatedIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to unmarshal IPPool '%s' allocatedIPs: %v", ippool.Name, err)
	}

	var ip net.IP

	selectIP := unallocatedIPs[0]
	ip = net.ParseIP(string(selectIP))
	unallocatedIPs = unallocatedIPs[1:]
	allocatedIPs[selectIP] = v1alpha1.PoolIPAllocation{
		NamespacedName: (apitypes.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}).String(),
		PodUID:         string(pod.UID),
	}

	rawUnallocatedIPs, err := convert.MarshalIPPoolUnallocatedIPs(unallocatedIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal IPPool '%s' unallocatedIPs: %w", ippool.Name, err)
	}
	rawAllocatedIPs, err := convert.MarshalIPPoolAllocatedIPs(allocatedIPs)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal IPPool '%s' allocatedIPs: %w", ippool.Name, err)
	}

	if rawUnallocatedIPs == nil || len(ptr.OrEmpty(rawUnallocatedIPs)) == 0 {
		ippool.Status.UnallocatedIPs = nil
	} else {
		ippool.Status.UnallocatedIPs = rawUnallocatedIPs
	}
	if rawAllocatedIPs == nil || len(ptr.OrEmpty(rawAllocatedIPs)) == 0 {
		ippool.Status.AllocatedIPs = nil
	} else {
		ippool.Status.AllocatedIPs = rawAllocatedIPs
	}

	ippool.Status.UnallocatedIPCount = ptr.Of(int64(len(unallocatedIPs)))
	ippool.Status.AllocatedIPCount = ptr.Of(int64(len(allocatedIPs)))

	return ip, ippool, nil
}

func (ic *IPPoolClient) createExclusiveIPPool(ctx context.Context, ns string, podTopOwnerRef *metav1.OwnerReference) (*v1alpha1.IPPool, error) {
	ippool := v1alpha1.IPPool{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-%s-%s-", strings.ToLower(podTopOwnerRef.Kind), strings.ToLower(ns), strings.ToLower(podTopOwnerRef.Name)),
		},
		Spec:   v1alpha1.IPPoolSpec{},
		Status: v1alpha1.IPPoolStatus{},
	}
	if err := ic.Client.Create(ctx, &ippool); err != nil {
		return nil, err
	}
	return &ippool, nil
}
