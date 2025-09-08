/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package controller implements the controller logic
package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/innabox/cloudkit-operator/api/v1alpha1"
	fulfillmentv1 "github.com/innabox/cloudkit-operator/internal/api/fulfillment/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

// NewVMComponentFn is the type of a function that creates a required VM component
type NewVMComponentFn func(context.Context, *v1alpha1.CloudkitVM) (*appResource, error)

type vmComponent struct {
	name string
	fn   NewVMComponentFn
}

func (r *CloudkitVMReconciler) vmComponents() []vmComponent {
	return []vmComponent{
		{"VM Namespace", r.newVMNamespace},
	}
}

// CloudkitVMReconciler reconciles a CloudkitVM object
type CloudkitVMReconciler struct {
	client.Client
	Scheme                 *runtime.Scheme
	GRPCConn               *grpc.ClientConn
	CreateVMWebhook        string
	DeleteVMWebhook        string
	CloudkitVMNamespace    string
	MinimumRequestInterval time.Duration
}

func NewCloudkitVMReconciler(
	client client.Client,
	scheme *runtime.Scheme,
	grpcConn *grpc.ClientConn,
	createVMWebhook string,
	deleteVMWebhook string,
	cloudkitVMNamespace string,
	minimumRequestInterval time.Duration,
) *CloudkitVMReconciler {

	if cloudkitVMNamespace == "" {
		cloudkitVMNamespace = defaultCloudkitVMNamespace
	}

	return &CloudkitVMReconciler{
		Client:                 client,
		Scheme:                 scheme,
		GRPCConn:               grpcConn,
		CreateVMWebhook:        createVMWebhook,
		DeleteVMWebhook:        deleteVMWebhook,
		CloudkitVMNamespace:    cloudkitVMNamespace,
		MinimumRequestInterval: minimumRequestInterval,
	}
}

// +kubebuilder:rbac:groups=cloudkit.openshift.io,resources=cloudkitvms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cloudkit.openshift.io,resources=cloudkitvms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cloudkit.openshift.io,resources=cloudkitvms/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop
func (r *CloudkitVMReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	// Fetch the CloudkitVM instance
	var cloudkitVM v1alpha1.CloudkitVM
	if err := r.Get(ctx, req.NamespacedName, &cloudkitVM); err != nil {
		log.Error(err, "unable to fetch CloudkitVM")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if the CloudkitVM instance is being deleted
	if cloudkitVM.GetDeletionTimestamp() != nil {
		return r.reconcileDelete(ctx, &cloudkitVM)
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(&cloudkitVM, cloudkitVMFinalizer) {
		controllerutil.AddFinalizer(&cloudkitVM, cloudkitVMFinalizer)
		if err := r.Update(ctx, &cloudkitVM); err != nil {
			log.Error(err, "failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	return r.reconcileNormal(ctx, &cloudkitVM)
}

func (r *CloudkitVMReconciler) reconcileNormal(ctx context.Context, cloudkitVM *v1alpha1.CloudkitVM) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	// Set initial phase if not set
	if cloudkitVM.Status.Phase == "" {
		cloudkitVM.Status.Phase = v1alpha1.CloudkitVMPhasePending
		cloudkitVM.SetAcceptedCondition(metav1.ConditionTrue, "CloudkitVMAccepted", "CloudkitVM has been accepted for processing")
	}

	// Create required components (namespace, service account, role binding)
	for _, component := range r.vmComponents() {
		log.Info("Reconciling component", "component", component.name)

		resource, err := component.fn(ctx, cloudkitVM)
		if err != nil {
			log.Error(err, "failed to create component", "component", component.name)
			cloudkitVM.Status.Phase = v1alpha1.CloudkitVMPhaseFailed
			cloudkitVM.SetFailedCondition(metav1.ConditionTrue, "ComponentCreationFailed", fmt.Sprintf("Failed to create %s: %v", component.name, err))
			r.Status().Update(ctx, cloudkitVM)
			return ctrl.Result{}, err
		}

		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, resource.object, resource.mutateFn); err != nil {
			log.Error(err, "failed to create or update component", "component", component.name)
			cloudkitVM.Status.Phase = v1alpha1.CloudkitVMPhaseFailed
			cloudkitVM.SetFailedCondition(metav1.ConditionTrue, "ComponentUpdateFailed", fmt.Sprintf("Failed to update %s: %v", component.name, err))
			r.Status().Update(ctx, cloudkitVM)
			return ctrl.Result{}, err
		}
	}

	// Update VM reference if not set
	if cloudkitVM.Status.VMReference == nil {
		namespaceName := generateVMNamespaceName(cloudkitVM)
		cloudkitVM.Status.VMReference = &v1alpha1.CloudkitVMReferenceType{
			Namespace: namespaceName,
			VMName:    fmt.Sprintf("%s-vm", cloudkitVM.Name),
		}
	}

	// Create VM in fulfillment service if gRPC connection is available
	if r.GRPCConn != nil {
		if err := r.createVMInFulfillmentService(ctx, cloudkitVM); err != nil {
			log.Error(err, "failed to create VM in fulfillment service")
			cloudkitVM.Status.Phase = v1alpha1.CloudkitVMPhaseFailed
			cloudkitVM.SetFailedCondition(metav1.ConditionTrue, "FulfillmentServiceError", fmt.Sprintf("Failed to create VM in fulfillment service: %v", err))
		} else {
			cloudkitVM.Status.Phase = v1alpha1.CloudkitVMPhaseProvisioning
			cloudkitVM.SetProgressingCondition(metav1.ConditionTrue, "VMCreated", "VM created in fulfillment service")
		}
	}

	// Call webhook for VM provisioning
	if r.CreateVMWebhook != "" {
		if err := r.callCreateVMWebhook(ctx, cloudkitVM); err != nil {
			log.Error(err, "failed to call create VM webhook")
			cloudkitVM.Status.Phase = v1alpha1.CloudkitVMPhaseFailed
			cloudkitVM.SetFailedCondition(metav1.ConditionTrue, "WebhookCallFailed", fmt.Sprintf("Failed to call create VM webhook: %v", err))
		}
	}

	// Update status
	if err := r.Status().Update(ctx, cloudkitVM); err != nil {
		log.Error(err, "failed to update CloudkitVM status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CloudkitVMReconciler) reconcileDelete(ctx context.Context, cloudkitVM *v1alpha1.CloudkitVM) (ctrl.Result, error) {
	log := ctrllog.FromContext(ctx)

	cloudkitVM.Status.Phase = v1alpha1.CloudkitVMPhaseDeleting

	// Call delete webhook
	if r.DeleteVMWebhook != "" {
		if err := r.callDeleteVMWebhook(ctx, cloudkitVM); err != nil {
			log.Error(err, "failed to call delete VM webhook")
		}
	}

	// Delete VM from fulfillment service if gRPC connection is available
	if r.GRPCConn != nil {
		if err := r.deleteVMFromFulfillmentService(ctx, cloudkitVM); err != nil {
			log.Error(err, "failed to delete VM from fulfillment service")
		}
	}

	// Remove finalizer to allow deletion
	controllerutil.RemoveFinalizer(cloudkitVM, cloudkitVMFinalizer)
	if err := r.Update(ctx, cloudkitVM); err != nil {
		log.Error(err, "failed to remove finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *CloudkitVMReconciler) createVMInFulfillmentService(ctx context.Context, cloudkitVM *v1alpha1.CloudkitVM) error {
	if r.GRPCConn == nil {
		return fmt.Errorf("gRPC connection is not available")
	}

	client := fulfillmentv1.NewVirtualMachinesClient(r.GRPCConn)

	// Parse template parameters
	templateParams := make(map[string]*anypb.Any)
	// TODO: Parse the JSON templateParameters and convert to protobuf Any

	vm := &fulfillmentv1.VirtualMachine{
		Id: cloudkitVM.Name,
		Spec: &fulfillmentv1.VirtualMachineSpec{
			Template:           cloudkitVM.Spec.TemplateID,
			TemplateParameters: templateParams,
		},
	}

	req := &fulfillmentv1.VirtualMachinesCreateRequest{
		Object: vm,
	}

	_, err := client.Create(ctx, req)
	return err
}

func (r *CloudkitVMReconciler) deleteVMFromFulfillmentService(ctx context.Context, cloudkitVM *v1alpha1.CloudkitVM) error {
	if r.GRPCConn == nil {
		return fmt.Errorf("gRPC connection is not available")
	}

	client := fulfillmentv1.NewVirtualMachinesClient(r.GRPCConn)

	req := &fulfillmentv1.VirtualMachinesDeleteRequest{
		Id: cloudkitVM.Name,
	}

	_, err := client.Delete(ctx, req)
	return err
}

func (r *CloudkitVMReconciler) callCreateVMWebhook(ctx context.Context, cloudkitVM *v1alpha1.CloudkitVM) error {
	// Check for existing requests to prevent spam
	delta := checkForExistingRequest(ctx, fmt.Sprintf("vm-%s", cloudkitVM.Name), r.MinimumRequestInterval)
	if delta > 0 {
		return fmt.Errorf("request too frequent, wait %v", delta)
	}

	if err := postCloudkitVMWebhook(ctx, r.CreateVMWebhook, cloudkitVM); err != nil {
		return err
	}

	// Add to inflight requests to prevent spam
	addInflightRequest(ctx, fmt.Sprintf("vm-%s", cloudkitVM.Name), r.MinimumRequestInterval)
	return nil
}

func (r *CloudkitVMReconciler) callDeleteVMWebhook(ctx context.Context, cloudkitVM *v1alpha1.CloudkitVM) error {
	return postCloudkitVMWebhook(ctx, r.DeleteVMWebhook, cloudkitVM)
}

// SetupWithManager sets up the controller with the Manager.
func (r *CloudkitVMReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.CloudkitVM{}).
		Owns(&corev1.Namespace{}).
		WithEventFilter(predicate.Or(predicate.GenerationChangedPredicate{}, predicate.LabelChangedPredicate{})).
		Watches(
			&corev1.Namespace{},
			handler.EnqueueRequestsFromMapFunc(r.findCloudkitVMsForNamespace),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}

func (r *CloudkitVMReconciler) findCloudkitVMsForNamespace(ctx context.Context, obj client.Object) []reconcile.Request {
	attachedCloudkitVMs := &v1alpha1.CloudkitVMList{}
	listOps := &client.ListOptions{
		Namespace: r.CloudkitVMNamespace,
	}
	err := r.List(ctx, attachedCloudkitVMs, listOps)
	if err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, len(attachedCloudkitVMs.Items))
	for i, item := range attachedCloudkitVMs.Items {
		requests[i] = reconcile.Request{
			NamespacedName: client.ObjectKeyFromObject(&item),
		}
	}
	return requests
}
