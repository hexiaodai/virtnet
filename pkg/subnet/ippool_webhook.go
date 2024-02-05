package subnet

import (
	"context"
	"errors"

	"github.com/hexiaodai/virtnet/pkg/constant"
	"github.com/hexiaodai/virtnet/pkg/k8s/apis/virtnet/v1alpha1"
	"github.com/hexiaodai/virtnet/pkg/logging"
	"github.com/hexiaodai/virtnet/pkg/types"

	"go.uber.org/zap"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var webhookLogger *zap.SugaredLogger

type SubnetWebhook struct {
	Client client.Client
}

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (webhook *SubnetWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	if webhookLogger == nil {
		webhookLogger = logger.Named("Webhook")
	}
	webhookLogger.Debug("Setting up webhook with manager")

	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.Subnet{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

var _ webhook.CustomDefaulter = (*SubnetWebhook)(nil)

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (webhook *SubnetWebhook) Default(ctx context.Context, obj runtime.Object) error {
	subnet := obj.(*v1alpha1.Subnet)
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.SubnetGVK,
		Namespace:        subnet.Namespace,
		Name:             subnet.Name,
	}

	logger := webhookLogger.Named("Mutating").
		With("Application", appNamespacedName).
		With("Operation", "DEFAULT")

	logger.Debugf("Request Subnet: %+v", *subnet)

	errs := field.ErrorList{}
	if err := webhook.mutateSubnet(logging.IntoContext(ctx, logger), subnet); err != nil {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		return nil
	}

	logger.Errorf("Failed to mutate Subnet: %v", errs.ToAggregate().Error())
	return apierrors.NewInvalid(
		v1alpha1.SubnetGVK.GroupKind(),
		subnet.Name,
		errs,
	)
}

var _ webhook.CustomValidator = (*SubnetWebhook)(nil)

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (webhook *SubnetWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	subnet := obj.(*v1alpha1.Subnet)
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.SubnetGVK,
		Namespace:        subnet.Namespace,
		Name:             subnet.Name,
	}

	logger := webhookLogger.Named("Validating").
		With("Application", appNamespacedName).
		With("Operation", "CREATE")

	logger.Debugf("Request Subnet: %+v", *subnet)

	// if ownerRef := haveSubnetOwner(ipPool); ownerRef != nil {
	// 	logger.Debugf("Subnet already have owner: %+v, noting to validate", ownerRef)
	// 	return nil, nil
	// }

	var errs field.ErrorList
	if err := webhook.validateSubnetGateway(subnet); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateSubnetExcludeIPs(constant.IPv4, subnet.Spec.Subnet, subnet.Spec.ExcludeIPs); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateSubnetCIDR(logging.IntoContext(ctx, logger), subnet); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateSubnetIPs(logging.IntoContext(ctx, logger), subnet); err != nil {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		return nil, nil
	}

	logger.Errorf("Failed to create Subnet: %v", errs.ToAggregate().Error())
	return nil, apierrors.NewInvalid(
		v1alpha1.SubnetGVK.GroupKind(),
		subnet.Name,
		errs,
	)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (webhook *SubnetWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldSubnet := oldObj.(*v1alpha1.Subnet)
	newSubnet := newObj.(*v1alpha1.Subnet)
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.SubnetGVK,
		Namespace:        newSubnet.Namespace,
		Name:             newSubnet.Name,
	}

	logger := webhookLogger.Named("Validating").
		With("Application", appNamespacedName).
		With("Operation", "UPDATE")

	logger.Debugf("Request old Subnet: %v", oldSubnet)
	logger.Debugf("Request new Subnet: %v", newSubnet)

	// if ownerRef := haveSubnetOwner(newSubnet); ownerRef != nil {
	// 	logger.Debugf("Subnet already have owner: %+v, noting to validate", ownerRef)
	// 	// logger.Debugf("Subnet cannot be updated if it has an owner '%v'", v1alpha1.SubnetGVK.String())
	// 	// return nil, apierrors.NewForbidden(
	// 	// 	v1alpha1.SubnetGVR.GroupResource(),
	// 	// 	newSubnet.Name,
	// 	// 	fmt.Errorf("Subnet cannot be updated if it has an owner '%v'", v1alpha1.SubnetGVK.String()),
	// 	// )
	// 	return nil, nil
	// }

	if newSubnet.DeletionTimestamp != nil {
		if !controllerutil.ContainsFinalizer(newSubnet, v1alpha1.SubnetFinalizer) {
			return nil, nil
		}

		return nil, apierrors.NewForbidden(
			v1alpha1.SubnetGVR.GroupResource(),
			newSubnet.Name,
			errors.New("cannot update a terminating Subnet"),
		)
	}

	var errs field.ErrorList
	if err := webhook.validateSubnetShouldNotBeChanged(oldSubnet, newSubnet); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateSubnetGateway(newSubnet); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateSubnetExcludeIPs(constant.IPv4, newSubnet.Spec.Subnet, newSubnet.Spec.ExcludeIPs); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateSubnetIPs(logging.IntoContext(ctx, logger), newSubnet); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateSubnetIPInUse(newSubnet); err != nil {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		return nil, nil
	}

	logger.Errorf("Failed to update Subnet: %v", errs.ToAggregate().Error())
	return nil, apierrors.NewInvalid(
		v1alpha1.SubnetGVK.GroupKind(),
		newSubnet.Name,
		errs,
	)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (webhook *SubnetWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
