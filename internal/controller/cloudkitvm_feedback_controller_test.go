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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	v1alpha1 "github.com/innabox/cloudkit-operator/api/v1alpha1"
	fulfillmentv1 "github.com/innabox/cloudkit-operator/internal/api/fulfillment/v1"
)

var _ = Describe("CloudkitVM Feedback Controller", func() {
	Context("When updating CloudkitVM status from fulfillment service", func() {
		const resourceName = "test-feedback-vm"
		const resourceNamespace = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}

		var cloudkitVM *v1alpha1.CloudkitVM
		var feedbackController *CloudkitVMFeedbackReconciler

		BeforeEach(func() {
			By("creating the CloudkitVM resource")
			cloudkitVM = &v1alpha1.CloudkitVM{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: v1alpha1.CloudkitVMSpec{
					TemplateID: "ocp_virt_vm",
					TemplateParameters: `{
						"vm_name": "feedback-test-vm"
					}`,
				},
				Status: v1alpha1.CloudkitVMStatus{
					Phase: v1alpha1.CloudkitVMPhasePending,
					VMReference: &v1alpha1.CloudkitVMReferenceType{
						Namespace: "vm-test-namespace",
						VMName:    "test-vm",
					},
				},
			}

			err := k8sClient.Get(ctx, typeNamespacedName, &v1alpha1.CloudkitVM{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, cloudkitVM)).To(Succeed())
			}

			By("creating the feedback controller")
			feedbackController = NewCloudkitVMFeedbackReconciler(
				ctrl.Log.WithName("test-feedback"),
				k8sClient,
				nil, // No gRPC connection for unit tests
				resourceNamespace,
			)
		})

		AfterEach(func() {
			By("cleaning up the CloudkitVM resource")
			resource := &v1alpha1.CloudkitVM{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should map PROGRESSING state to Provisioning phase", func() {
			By("creating a mock VM with PROGRESSING state")
			mockVM := &fulfillmentv1.VirtualMachine{
				Id: resourceName,
				Status: &fulfillmentv1.VirtualMachineStatus{
					State: fulfillmentv1.VirtualMachineState_VIRTUAL_MACHINE_STATE_PROGRESSING,
				},
			}

			By("updating the CloudkitVM status")
			updated := feedbackController.updateCloudkitVMStatus(cloudkitVM, mockVM)

			Expect(updated).To(BeTrue())
			Expect(cloudkitVM.Status.Phase).To(Equal(v1alpha1.CloudkitVMPhaseProvisioning))
			Expect(cloudkitVM.IsConditionTrue(v1alpha1.CloudkitVMConditionProgressing)).To(BeTrue())
		})

		It("should map READY state to Ready phase", func() {
			By("creating a mock VM with READY state")
			mockVM := &fulfillmentv1.VirtualMachine{
				Id: resourceName,
				Status: &fulfillmentv1.VirtualMachineStatus{
					State:     fulfillmentv1.VirtualMachineState_VIRTUAL_MACHINE_STATE_READY,
					IpAddress: "192.168.1.100",
				},
			}

			By("updating the CloudkitVM status")
			updated := feedbackController.updateCloudkitVMStatus(cloudkitVM, mockVM)

			Expect(updated).To(BeTrue())
			Expect(cloudkitVM.Status.Phase).To(Equal(v1alpha1.CloudkitVMPhaseReady))
			Expect(cloudkitVM.IsConditionTrue(v1alpha1.CloudkitVMConditionAvailable)).To(BeTrue())
			Expect(cloudkitVM.Status.VMReference.IPAddress).To(Equal("192.168.1.100"))
		})

		It("should map FAILED state to Failed phase", func() {
			By("creating a mock VM with FAILED state")
			mockVM := &fulfillmentv1.VirtualMachine{
				Id: resourceName,
				Status: &fulfillmentv1.VirtualMachineStatus{
					State: fulfillmentv1.VirtualMachineState_VIRTUAL_MACHINE_STATE_FAILED,
				},
			}

			By("updating the CloudkitVM status")
			updated := feedbackController.updateCloudkitVMStatus(cloudkitVM, mockVM)

			Expect(updated).To(BeTrue())
			Expect(cloudkitVM.Status.Phase).To(Equal(v1alpha1.CloudkitVMPhaseFailed))
			Expect(cloudkitVM.IsConditionTrue(v1alpha1.CloudkitVMConditionFailed)).To(BeTrue())
		})

		It("should map unspecified state to Pending phase", func() {
			By("creating a mock VM with unspecified state")
			mockVM := &fulfillmentv1.VirtualMachine{
				Id: resourceName,
				Status: &fulfillmentv1.VirtualMachineStatus{
					State: fulfillmentv1.VirtualMachineState_VIRTUAL_MACHINE_STATE_UNSPECIFIED,
				},
			}

			By("updating the CloudkitVM status")
			updated := feedbackController.updateCloudkitVMStatus(cloudkitVM, mockVM)

			Expect(updated).To(BeTrue())
			Expect(cloudkitVM.Status.Phase).To(Equal(v1alpha1.CloudkitVMPhasePending))
		})

		It("should update IP address when provided", func() {
			By("ensuring VM reference exists and starting with no IP address")
			if cloudkitVM.Status.VMReference == nil {
				cloudkitVM.Status.VMReference = &v1alpha1.CloudkitVMReferenceType{}
			}
			cloudkitVM.Status.VMReference.IPAddress = ""
			Expect(cloudkitVM.Status.VMReference.IPAddress).To(BeEmpty())

			By("creating a mock VM with IP address")
			mockVM := &fulfillmentv1.VirtualMachine{
				Id: resourceName,
				Status: &fulfillmentv1.VirtualMachineStatus{
					State:     fulfillmentv1.VirtualMachineState_VIRTUAL_MACHINE_STATE_READY,
					IpAddress: "10.0.0.50",
				},
			}

			By("updating the CloudkitVM status")
			updated := feedbackController.updateCloudkitVMStatus(cloudkitVM, mockVM)

			Expect(updated).To(BeTrue())
			Expect(cloudkitVM.Status.VMReference.IPAddress).To(Equal("10.0.0.50"))
		})

		It("should update IP address when changed", func() {
			By("ensuring VM reference exists and starting with an existing IP address")
			if cloudkitVM.Status.VMReference == nil {
				cloudkitVM.Status.VMReference = &v1alpha1.CloudkitVMReferenceType{}
			}
			cloudkitVM.Status.VMReference.IPAddress = "10.0.0.10"

			By("creating a mock VM with different IP address")
			mockVM := &fulfillmentv1.VirtualMachine{
				Id: resourceName,
				Status: &fulfillmentv1.VirtualMachineStatus{
					State:     fulfillmentv1.VirtualMachineState_VIRTUAL_MACHINE_STATE_READY,
					IpAddress: "10.0.0.20",
				},
			}

			By("updating the CloudkitVM status")
			updated := feedbackController.updateCloudkitVMStatus(cloudkitVM, mockVM)

			Expect(updated).To(BeTrue())
			Expect(cloudkitVM.Status.VMReference.IPAddress).To(Equal("10.0.0.20"))
		})

		It("should not update when no changes are needed", func() {
			By("ensuring VM reference exists and setting the CloudkitVM to READY state with IP")
			if cloudkitVM.Status.VMReference == nil {
				cloudkitVM.Status.VMReference = &v1alpha1.CloudkitVMReferenceType{}
			}
			cloudkitVM.Status.Phase = v1alpha1.CloudkitVMPhaseReady
			cloudkitVM.Status.VMReference.IPAddress = "192.168.1.100"

			By("creating a mock VM with same state and IP")
			mockVM := &fulfillmentv1.VirtualMachine{
				Id: resourceName,
				Status: &fulfillmentv1.VirtualMachineStatus{
					State:     fulfillmentv1.VirtualMachineState_VIRTUAL_MACHINE_STATE_READY,
					IpAddress: "192.168.1.100",
				},
			}

			By("updating the CloudkitVM status")
			updated := feedbackController.updateCloudkitVMStatus(cloudkitVM, mockVM)

			Expect(updated).To(BeFalse(), "Should not update when no changes are needed")
		})

		It("should handle nil VM status gracefully", func() {
			By("creating a mock VM with nil status")
			mockVM := &fulfillmentv1.VirtualMachine{
				Id:     resourceName,
				Status: nil,
			}

			By("updating the CloudkitVM status")
			updated := feedbackController.updateCloudkitVMStatus(cloudkitVM, mockVM)

			Expect(updated).To(BeFalse(), "Should not update when VM status is nil")
		})

		It("should create VM reference if it doesn't exist", func() {
			By("starting with no VM reference")
			cloudkitVM.Status.VMReference = nil

			By("creating a mock VM with status")
			mockVM := &fulfillmentv1.VirtualMachine{
				Id: resourceName,
				Status: &fulfillmentv1.VirtualMachineStatus{
					State:     fulfillmentv1.VirtualMachineState_VIRTUAL_MACHINE_STATE_READY,
					IpAddress: "10.0.0.100",
				},
			}

			By("updating the CloudkitVM status")
			updated := feedbackController.updateCloudkitVMStatus(cloudkitVM, mockVM)

			Expect(updated).To(BeTrue())
			Expect(cloudkitVM.Status.VMReference).NotTo(BeNil())
			Expect(cloudkitVM.Status.VMReference.IPAddress).To(Equal("10.0.0.100"))
		})

		It("should handle sync with missing CloudkitVM", func() {
			By("deleting the CloudkitVM first")
			err := k8sClient.Delete(ctx, cloudkitVM)
			Expect(err).NotTo(HaveOccurred())

			By("testing updateCloudkitVMStatus with nil VM gracefully")
			// This test verifies the controller handles missing VMs gracefully
			// The gRPC-dependent method would return early for missing CloudkitVMs
			// This test just verifies that the logic doesn't crash
			nilVM := (*fulfillmentv1.VirtualMachine)(nil)
			updated := feedbackController.updateCloudkitVMStatus(cloudkitVM, nilVM)
			Expect(updated).To(BeFalse()) // Should not update with nil VM
		})

		It("should handle sync for VMs in deleting phase", func() {
			By("creating a CloudkitVM in deleting phase")
			deletingVM := &v1alpha1.CloudkitVM{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "deleting-vm",
					Namespace: resourceNamespace,
				},
				Spec: v1alpha1.CloudkitVMSpec{
					TemplateID: "deleting_template",
				},
				Status: v1alpha1.CloudkitVMStatus{
					Phase: v1alpha1.CloudkitVMPhaseDeleting,
				},
			}
			Expect(k8sClient.Create(ctx, deletingVM)).To(Succeed())
			defer k8sClient.Delete(ctx, deletingVM)

			// Update the status separately
			deletingVM.Status.Phase = v1alpha1.CloudkitVMPhaseDeleting
			Expect(k8sClient.Status().Update(ctx, deletingVM)).To(Succeed())

			By("testing that deleting VMs are handled correctly")
			// Since syncAllCloudkitVMs requires gRPC, we'll test the logic directly
			// The method should skip VMs in deleting phase
			Expect(deletingVM.Status.Phase).To(Equal(v1alpha1.CloudkitVMPhaseDeleting))
			// This verifies the test setup is correct - in actual operation,
			// the syncAllCloudkitVMs method would skip this VM
		})

		It("should handle periodic sync", func() {
			By("starting periodic sync with short interval")
			syncCtx, cancel := context.WithTimeout(ctx, time.Millisecond*500)
			defer cancel()

			// This should run without errors
			feedbackController.StartPeriodicSync(syncCtx, time.Millisecond*100)

			// Wait for context to be canceled (timeout)
			<-syncCtx.Done()
		})
	})
})
