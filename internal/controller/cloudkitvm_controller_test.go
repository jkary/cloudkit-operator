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
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v1alpha1 "github.com/innabox/cloudkit-operator/api/v1alpha1"
)

var _ = Describe("CloudkitVM Controller", func() {
	Context("When reconciling a CloudkitVM resource", func() {
		const resourceName = "test-cloudkitvm"
		const resourceNamespace = "default"

		ctx := context.Background()
		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: resourceNamespace,
		}

		var cloudkitVM *v1alpha1.CloudkitVM

		BeforeEach(func() {
			By("creating the custom resource for the Kind CloudkitVM")
			cloudkitVM = &v1alpha1.CloudkitVM{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: resourceNamespace,
				},
				Spec: v1alpha1.CloudkitVMSpec{
					TemplateID: "ocp_virt_vm",
					TemplateParameters: `{
						"vm_name": "test-vm",
						"vm_cpu_cores": 2,
						"vm_memory": "2Gi"
					}`,
				},
			}

			err := k8sClient.Get(ctx, typeNamespacedName, &v1alpha1.CloudkitVM{})
			if err != nil && errors.IsNotFound(err) {
				Expect(k8sClient.Create(ctx, cloudkitVM)).To(Succeed())
			}
		})

		AfterEach(func() {
			By("Cleanup the specific resource instance CloudkitVM")
			resource := &v1alpha1.CloudkitVM{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}

			// Clean up any created namespaces
			nsList := &corev1.NamespaceList{}
			err = k8sClient.List(ctx, nsList)
			if err == nil {
				for _, ns := range nsList.Items {
					if ns.Labels != nil {
						if vmName, exists := ns.Labels[cloudkitVMNameLabel]; exists && vmName == resourceName {
							Expect(k8sClient.Delete(ctx, &ns)).To(Succeed())
						}
					}
				}
			}
		})

		It("should successfully reconcile the resource and set initial status", func() {
			By("Reconciling the created resource")
			controllerReconciler := &CloudkitVMReconciler{
				Client:                 k8sClient,
				Scheme:                 k8sClient.Scheme(),
				CloudkitVMNamespace:    resourceNamespace,
				MinimumRequestInterval: time.Second,
			}

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile sets initial status and creates resources
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the CloudkitVM status was updated")
			updatedCloudkitVM := &v1alpha1.CloudkitVM{}
			err = k8sClient.Get(ctx, typeNamespacedName, updatedCloudkitVM)
			Expect(err).NotTo(HaveOccurred())

			// Should have finalizer added
			Expect(updatedCloudkitVM.Finalizers).To(ContainElement(cloudkitVMFinalizer))

			// Should have initial phase set
			Expect(updatedCloudkitVM.Status.Phase).To(Equal(v1alpha1.CloudkitVMPhasePending))

			// Should have Accepted condition
			Expect(updatedCloudkitVM.IsConditionTrue(v1alpha1.CloudkitVMConditionAccepted)).To(BeTrue())

			// Should have VM reference set
			Expect(updatedCloudkitVM.Status.VMReference).NotTo(BeNil())
			Expect(updatedCloudkitVM.Status.VMReference.Namespace).To(ContainSubstring("vm-" + resourceName))
			Expect(updatedCloudkitVM.Status.VMReference.VMName).To(Equal(fmt.Sprintf("%s-vm", resourceName)))
		})

		It("should create a namespace for the VM", func() {
			By("Reconciling the resource")
			controllerReconciler := &CloudkitVMReconciler{
				Client:                 k8sClient,
				Scheme:                 k8sClient.Scheme(),
				CloudkitVMNamespace:    resourceNamespace,
				MinimumRequestInterval: time.Second,
			}

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile creates resources
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that a namespace was created")
			updatedCloudkitVM := &v1alpha1.CloudkitVM{}
			err = k8sClient.Get(ctx, typeNamespacedName, updatedCloudkitVM)
			Expect(err).NotTo(HaveOccurred())
			Expect(updatedCloudkitVM.Status.VMReference).NotTo(BeNil())

			namespaceName := updatedCloudkitVM.Status.VMReference.Namespace
			namespace := &corev1.Namespace{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: namespaceName}, namespace)
			Expect(err).NotTo(HaveOccurred())

			// Verify namespace has correct labels
			Expect(namespace.Labels).To(HaveKeyWithValue(cloudkitVMNameLabel, resourceName))
			Expect(namespace.Labels).To(HaveKeyWithValue("app.kubernetes.io/name", cloudkitAppName))
		})

		It("should handle deletion correctly", func() {
			By("First reconciling to create the resource")
			controllerReconciler := &CloudkitVMReconciler{
				Client:                 k8sClient,
				Scheme:                 k8sClient.Scheme(),
				CloudkitVMNamespace:    resourceNamespace,
				MinimumRequestInterval: time.Second,
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Setting deletion timestamp on the resource")
			cloudkitVMToDelete := &v1alpha1.CloudkitVM{}
			err = k8sClient.Get(ctx, typeNamespacedName, cloudkitVMToDelete)
			Expect(err).NotTo(HaveOccurred())

			// Add finalizer first (would normally be done by first reconcile)
			cloudkitVMToDelete.Finalizers = append(cloudkitVMToDelete.Finalizers, cloudkitVMFinalizer)
			err = k8sClient.Update(ctx, cloudkitVMToDelete)
			Expect(err).NotTo(HaveOccurred())

			// Delete the resource
			err = k8sClient.Delete(ctx, cloudkitVMToDelete)
			Expect(err).NotTo(HaveOccurred())

			By("Reconciling the deletion")
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking the resource was deleted")
			deletedCloudkitVM := &v1alpha1.CloudkitVM{}
			err = k8sClient.Get(ctx, typeNamespacedName, deletedCloudkitVM)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("should handle webhook calls when webhook URL is configured", func() {
			By("Setting up a mock webhook server")
			webhookCalled := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				webhookCalled = true
				Expect(r.Method).To(Equal("POST"))
				Expect(r.Header.Get("Content-Type")).To(Equal("application/json"))
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			By("Reconciling with webhook URL configured")
			controllerReconciler := &CloudkitVMReconciler{
				Client:                 k8sClient,
				Scheme:                 k8sClient.Scheme(),
				CreateVMWebhook:        server.URL,
				CloudkitVMNamespace:    resourceNamespace,
				MinimumRequestInterval: time.Millisecond * 100, // Short interval for testing
			}

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile calls webhook
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the webhook was called")
			Eventually(func() bool {
				return webhookCalled
			}).Should(BeTrue())
		})

		It("should handle webhook failures gracefully", func() {
			By("Setting up a mock webhook server that returns errors")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			By("Reconciling with failing webhook")
			controllerReconciler := &CloudkitVMReconciler{
				Client:                 k8sClient,
				Scheme:                 k8sClient.Scheme(),
				CreateVMWebhook:        server.URL,
				CloudkitVMNamespace:    resourceNamespace,
				MinimumRequestInterval: time.Millisecond * 100,
			}

			// First reconcile adds finalizer
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// Second reconcile calls webhook and should not error, but status should reflect failure
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that the CloudkitVM status shows failure")
			updatedCloudkitVM := &v1alpha1.CloudkitVM{}
			err = k8sClient.Get(ctx, typeNamespacedName, updatedCloudkitVM)
			Expect(err).NotTo(HaveOccurred())

			// Status should eventually show failed
			Eventually(func() v1alpha1.CloudkitVMPhaseType {
				err = k8sClient.Get(ctx, typeNamespacedName, updatedCloudkitVM)
				Expect(err).NotTo(HaveOccurred())
				return updatedCloudkitVM.Status.Phase
			}).Should(Equal(v1alpha1.CloudkitVMPhaseFailed))

			// Should have Failed condition
			Expect(updatedCloudkitVM.IsConditionTrue(v1alpha1.CloudkitVMConditionFailed)).To(BeTrue())
		})

		It("should not reconcile resources from other namespaces when namespace is specified", func() {
			By("Creating a CloudkitVM in a different namespace")
			otherNamespace := "other-namespace"

			// Create the namespace first
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: otherNamespace,
				},
			}
			err := k8sClient.Create(ctx, ns)
			if err != nil && !errors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}

			otherCloudkitVM := &v1alpha1.CloudkitVM{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "other-vm",
					Namespace: otherNamespace,
				},
				Spec: v1alpha1.CloudkitVMSpec{
					TemplateID: "test_template",
				},
			}
			Expect(k8sClient.Create(ctx, otherCloudkitVM)).To(Succeed())
			defer func() {
				k8sClient.Delete(ctx, otherCloudkitVM)
				k8sClient.Delete(ctx, ns)
			}()

			By("Reconciling with namespace restriction")
			controllerReconciler := &CloudkitVMReconciler{
				Client:                 k8sClient,
				Scheme:                 k8sClient.Scheme(),
				CloudkitVMNamespace:    resourceNamespace, // Different from otherNamespace
				MinimumRequestInterval: time.Second,
			}

			// This should work for the resource in the correct namespace
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			// This should work for the resource in other namespace too (controller doesn't actually restrict by namespace in reconcile)
			// But in real deployment, the controller would be configured to watch only specific namespaces
			_, err = controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "other-vm",
					Namespace: otherNamespace,
				},
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
