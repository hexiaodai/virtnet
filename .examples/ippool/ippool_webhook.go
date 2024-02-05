package ippool

import (
	"cni/pkg/constant"
	"cni/pkg/k8s/apis/cni.virtnest.io/v1alpha1"
	"cni/pkg/logging"
	"cni/pkg/types"
	"context"
	"errors"

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

type IPPoolWebhook struct {
	Client client.Client
}

// SetupWebhookWithManager will setup the manager to manage the webhooks
func (webhook *IPPoolWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	if webhookLogger == nil {
		webhookLogger = logger.Named("Webhook")
	}
	webhookLogger.Debug("Setting up webhook with manager")

	return ctrl.NewWebhookManagedBy(mgr).
		For(&v1alpha1.IPPool{}).
		WithDefaulter(webhook).
		WithValidator(webhook).
		Complete()
}

var _ webhook.CustomDefaulter = (*IPPoolWebhook)(nil)

// Default implements webhook.CustomDefaulter so a webhook will be registered for the type.
func (webhook *IPPoolWebhook) Default(ctx context.Context, obj runtime.Object) error {
	ipPool := obj.(*v1alpha1.IPPool)
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.IPPoolGVK,
		Namespace:        ipPool.Namespace,
		Name:             ipPool.Name,
	}

	logger := webhookLogger.Named("Mutating").
		With("Application", appNamespacedName).
		With("Operation", "DEFAULT")

	logger.Debugf("Request IPPool: %+v", *ipPool)

	errs := field.ErrorList{}
	if err := webhook.mutateIPPool(logging.IntoContext(ctx, logger), ipPool); err != nil {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		return nil
	}

	logger.Errorf("Failed to mutate IPPool: %v", errs.ToAggregate().Error())
	return apierrors.NewInvalid(
		v1alpha1.IPPoolGVK.GroupKind(),
		ipPool.Name,
		errs,
	)
}

var _ webhook.CustomValidator = (*IPPoolWebhook)(nil)

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (webhook *IPPoolWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	ipPool := obj.(*v1alpha1.IPPool)
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.IPPoolGVK,
		Namespace:        ipPool.Namespace,
		Name:             ipPool.Name,
	}

	logger := webhookLogger.Named("Validating").
		With("Application", appNamespacedName).
		With("Operation", "CREATE")

	logger.Debugf("Request IPPool: %+v", *ipPool)

	if ownerRef := haveIPPoolOwner(ipPool); ownerRef != nil {
		logger.Debugf("IPPool already have owner: %+v, noting to validate", ownerRef)
		return nil, nil
	}

	var errs field.ErrorList
	if err := webhook.validateIPPoolGateway(ipPool); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateIPPoolExcludeIPs(constant.IPv4, ipPool.Spec.Subnet, ipPool.Spec.ExcludeIPs); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateIPPoolCIDR(logging.IntoContext(ctx, logger), ipPool); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateIPPoolIPs(logging.IntoContext(ctx, logger), ipPool); err != nil {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		return nil, nil
	}

	logger.Errorf("Failed to create IPPool: %v", errs.ToAggregate().Error())
	return nil, apierrors.NewInvalid(
		v1alpha1.IPPoolGVK.GroupKind(),
		ipPool.Name,
		errs,
	)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (webhook *IPPoolWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldIPPool := oldObj.(*v1alpha1.IPPool)
	newIPPool := newObj.(*v1alpha1.IPPool)
	appNamespacedName := types.AppNamespacedName{
		GroupVersionKind: v1alpha1.IPPoolGVK,
		Namespace:        newIPPool.Namespace,
		Name:             newIPPool.Name,
	}

	logger := webhookLogger.Named("Validating").
		With("Application", appNamespacedName).
		With("Operation", "UPDATE")

	logger.Debugf("Request old IPPool: %v", oldIPPool)
	logger.Debugf("Request new IPPool: %v", newIPPool)

	if ownerRef := haveIPPoolOwner(newIPPool); ownerRef != nil {
		logger.Debugf("IPPool already have owner: %+v, noting to validate", ownerRef)
		// logger.Debugf("IPPool cannot be updated if it has an owner '%v'", v1alpha1.IPPoolGVK.String())
		// return nil, apierrors.NewForbidden(
		// 	v1alpha1.IPPoolGVR.GroupResource(),
		// 	newIPPool.Name,
		// 	fmt.Errorf("IPPool cannot be updated if it has an owner '%v'", v1alpha1.IPPoolGVK.String()),
		// )
		return nil, nil
	}

	if newIPPool.DeletionTimestamp != nil {
		if !controllerutil.ContainsFinalizer(newIPPool, v1alpha1.IPPoolFinalizer) {
			return nil, nil
		}

		return nil, apierrors.NewForbidden(
			v1alpha1.IPPoolGVR.GroupResource(),
			newIPPool.Name,
			errors.New("cannot update a terminating IPPool"),
		)
	}

	var errs field.ErrorList
	if err := webhook.validateIPPoolShouldNotBeChanged(oldIPPool, newIPPool); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateIPPoolGateway(newIPPool); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateIPPoolExcludeIPs(constant.IPv4, newIPPool.Spec.Subnet, newIPPool.Spec.ExcludeIPs); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateIPPoolIPs(logging.IntoContext(ctx, logger), newIPPool); err != nil {
		errs = append(errs, err)
	}

	if err := webhook.validateIPPoolIPInUse(newIPPool); err != nil {
		errs = append(errs, err)
	}

	if len(errs) == 0 {
		return nil, nil
	}

	logger.Errorf("Failed to update IPPool: %v", errs.ToAggregate().Error())
	return nil, apierrors.NewInvalid(
		v1alpha1.IPPoolGVK.GroupKind(),
		newIPPool.Name,
		errs,
	)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (webhook *IPPoolWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}
