/*
Copyright (c) 2025 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License"); you may not use this file except in compliance with the
License. You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software distributed under the License is distributed on an
"AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the License for the specific
language governing permissions and limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"google.golang.org/grpc"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	clnt "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/innabox/cloudkit-operator/api/v1alpha1"
	fulfillmentv1 "github.com/innabox/cloudkit-operator/internal/api/fulfillment/v1"
)

// CloudkitVMFeedbackReconciler watches for changes to VirtualMachines in the fulfillment service
// and updates the corresponding CloudkitVM status
type CloudkitVMFeedbackReconciler struct {
	logger              logr.Logger
	client              clnt.Client
	grpcConn            *grpc.ClientConn
	cloudkitVMNamespace string
}

func NewCloudkitVMFeedbackReconciler(
	logger logr.Logger,
	client clnt.Client,
	grpcConn *grpc.ClientConn,
	cloudkitVMNamespace string,
) *CloudkitVMFeedbackReconciler {
	if cloudkitVMNamespace == "" {
		cloudkitVMNamespace = defaultCloudkitVMNamespace
	}

	return &CloudkitVMFeedbackReconciler{
		logger:              logger,
		client:              client,
		grpcConn:            grpcConn,
		cloudkitVMNamespace: cloudkitVMNamespace,
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *CloudkitVMFeedbackReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Since we're watching external VirtualMachine resources via gRPC,
	// we don't have direct Kubernetes resources to watch.
	// Instead, we'll set up a periodic sync or event-based updates.

	// For now, we'll use a simple approach where the main CloudkitVM controller
	// can trigger updates, and this feedback controller provides the logic
	// to sync status from fulfillment service.

	return nil
}

// UpdateCloudkitVMFromFulfillmentService fetches the latest VM status from fulfillment service
// and updates the corresponding CloudkitVM resource
func (r *CloudkitVMFeedbackReconciler) UpdateCloudkitVMFromFulfillmentService(ctx context.Context, cloudkitVMName string) error {
	if r.grpcConn == nil {
		return fmt.Errorf("gRPC connection is not available")
	}

	// Fetch VM from fulfillment service
	client := fulfillmentv1.NewVirtualMachinesClient(r.grpcConn)
	req := &fulfillmentv1.VirtualMachinesGetRequest{
		Id: cloudkitVMName,
	}

	resp, err := client.Get(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to get VM from fulfillment service: %w", err)
	}

	vm := resp.GetObject()
	if vm == nil {
		return fmt.Errorf("VM not found in fulfillment service")
	}

	// Fetch corresponding CloudkitVM resource
	var cloudkitVM v1alpha1.CloudkitVM
	err = r.client.Get(ctx, clnt.ObjectKey{
		Namespace: r.cloudkitVMNamespace,
		Name:      cloudkitVMName,
	}, &cloudkitVM)
	if err != nil {
		if errors.IsNotFound(err) {
			r.logger.Info("CloudkitVM not found, may have been deleted", "name", cloudkitVMName)
			return nil
		}
		return fmt.Errorf("failed to fetch CloudkitVM: %w", err)
	}

	// Update CloudkitVM status based on fulfillment service VM status
	updated := r.updateCloudkitVMStatus(&cloudkitVM, vm)
	if !updated {
		r.logger.Info("No updates needed for CloudkitVM status", "name", cloudkitVMName)
		return nil
	}

	// Update the CloudkitVM status
	if err := r.client.Status().Update(ctx, &cloudkitVM); err != nil {
		return fmt.Errorf("failed to update CloudkitVM status: %w", err)
	}

	r.logger.Info("Successfully updated CloudkitVM status",
		"name", cloudkitVMName,
		"phase", cloudkitVM.Status.Phase)

	return nil
}

func (r *CloudkitVMFeedbackReconciler) updateCloudkitVMStatus(cloudkitVM *v1alpha1.CloudkitVM, vm *fulfillmentv1.VirtualMachine) bool {
	updated := false
	vmStatus := vm.GetStatus()

	if vmStatus == nil {
		return false
	}

	// Map fulfillment service VM state to CloudkitVM phase
	var newPhase v1alpha1.CloudkitVMPhaseType
	switch vmStatus.GetState() {
	case fulfillmentv1.VirtualMachineState_VIRTUAL_MACHINE_STATE_PROGRESSING:
		newPhase = v1alpha1.CloudkitVMPhaseProvisioning
	case fulfillmentv1.VirtualMachineState_VIRTUAL_MACHINE_STATE_READY:
		newPhase = v1alpha1.CloudkitVMPhaseReady
	case fulfillmentv1.VirtualMachineState_VIRTUAL_MACHINE_STATE_FAILED:
		newPhase = v1alpha1.CloudkitVMPhaseFailed
	default:
		newPhase = v1alpha1.CloudkitVMPhasePending
	}

	if cloudkitVM.Status.Phase != newPhase {
		cloudkitVM.Status.Phase = newPhase
		updated = true

		// Update conditions based on new phase
		switch newPhase {
		case v1alpha1.CloudkitVMPhaseProvisioning:
			cloudkitVM.SetProgressingCondition(metav1.ConditionTrue, "VMProvisioning", "VM is being provisioned")
		case v1alpha1.CloudkitVMPhaseReady:
			cloudkitVM.SetAvailableCondition(metav1.ConditionTrue, "VMReady", "VM is ready and available")
		case v1alpha1.CloudkitVMPhaseFailed:
			cloudkitVM.SetFailedCondition(metav1.ConditionTrue, "VMFailed", "VM provisioning failed")
		}
	}

	// Update VM reference with fulfillment service data
	if cloudkitVM.Status.VMReference == nil {
		cloudkitVM.Status.VMReference = &v1alpha1.CloudkitVMReferenceType{}
		updated = true
	}

	// Update IP address if available
	if vmStatus.GetIpAddress() != "" && cloudkitVM.Status.VMReference.IPAddress != vmStatus.GetIpAddress() {
		cloudkitVM.Status.VMReference.IPAddress = vmStatus.GetIpAddress()
		updated = true
	}

	// TODO: Update hub ID if available when the field is added to the protobuf
	// The GetHub method is not available in current fulfillment API version

	return updated
}

// StartPeriodicSync starts a background goroutine that periodically syncs VM status
// This is a simple approach - in a production system, you might want to use
// event-driven updates based on fulfillment service events
func (r *CloudkitVMFeedbackReconciler) StartPeriodicSync(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.syncAllCloudkitVMs(ctx)
			}
		}
	}()
}

func (r *CloudkitVMFeedbackReconciler) syncAllCloudkitVMs(ctx context.Context) {
	var cloudkitVMList v1alpha1.CloudkitVMList
	err := r.client.List(ctx, &cloudkitVMList, clnt.InNamespace(r.cloudkitVMNamespace))
	if err != nil {
		r.logger.Error(err, "Failed to list CloudkitVMs for periodic sync")
		return
	}

	for _, cloudkitVM := range cloudkitVMList.Items {
		if cloudkitVM.Status.Phase == v1alpha1.CloudkitVMPhaseDeleting {
			continue // Skip VMs that are being deleted
		}

		err := r.UpdateCloudkitVMFromFulfillmentService(ctx, cloudkitVM.Name)
		if err != nil {
			r.logger.Error(err, "Failed to update CloudkitVM during periodic sync",
				"name", cloudkitVM.Name)
		}
	}
}
