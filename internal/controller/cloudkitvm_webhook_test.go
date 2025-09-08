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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2" //nolint:revive,staticcheck
	. "github.com/onsi/gomega"    //nolint:revive,staticcheck
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/innabox/cloudkit-operator/api/v1alpha1"
)

var _ = Describe("CloudkitVM Webhook", func() {
	Context("When calling webhook endpoints", func() {
		var cloudkitVM *v1alpha1.CloudkitVM
		var ctx context.Context

		BeforeEach(func() {
			ctx = context.Background()
			cloudkitVM = &v1alpha1.CloudkitVM{
				ObjectMeta: metav1.ObjectMeta{
					Name:            "test-webhook-vm",
					Namespace:       "default",
					UID:             "test-uid-123",
					ResourceVersion: "12345",
					CreationTimestamp: metav1.Time{
						Time: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
					},
				},
				Spec: v1alpha1.CloudkitVMSpec{
					TemplateID: "ocp_virt_vm",
					TemplateParameters: `{
						"vm_name": "webhook-test-vm",
						"vm_cpu_cores": 2,
						"vm_memory": "4Gi"
					}`,
				},
				Status: v1alpha1.CloudkitVMStatus{
					Phase: v1alpha1.CloudkitVMPhaseProvisioning,
					Conditions: []metav1.Condition{
						{
							Type:   string(v1alpha1.CloudkitVMConditionProgressing),
							Status: metav1.ConditionTrue,
							Reason: "VMCreated",
						},
					},
					VMReference: &v1alpha1.CloudkitVMReferenceType{
						Namespace: "vm-test-namespace",
						VMName:    "test-vm-instance",
						IPAddress: "192.168.1.50",
					},
				},
			}
		})

		It("should successfully post CloudkitVM to webhook", func() {
			By("setting up a mock webhook server")
			var receivedPayload map[string]interface{}
			var receivedHeaders http.Header

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeaders = r.Header
				body, err := io.ReadAll(r.Body)
				Expect(err).NotTo(HaveOccurred())

				err = json.Unmarshal(body, &receivedPayload)
				Expect(err).NotTo(HaveOccurred())

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			By("calling the webhook")
			err := postCloudkitVMWebhook(ctx, server.URL, cloudkitVM)
			Expect(err).NotTo(HaveOccurred())

			By("verifying the webhook received correct headers")
			Expect(receivedHeaders.Get("Content-Type")).To(Equal("application/json"))
			Expect(receivedHeaders.Get("User-Agent")).To(Equal("cloudkit-operator/1.0"))

			By("verifying the webhook received correct payload structure")
			Expect(receivedPayload).To(HaveKey("apiVersion"))
			Expect(receivedPayload).To(HaveKey("kind"))
			Expect(receivedPayload).To(HaveKey("metadata"))
			Expect(receivedPayload).To(HaveKey("spec"))
			Expect(receivedPayload).To(HaveKey("status"))

			By("verifying metadata fields")
			metadata := receivedPayload["metadata"].(map[string]interface{})
			Expect(metadata["name"]).To(Equal("test-webhook-vm"))
			Expect(metadata["namespace"]).To(Equal("default"))
			Expect(metadata["uid"]).To(Equal("test-uid-123"))
			Expect(metadata["resourceVersion"]).To(Equal("12345"))
			Expect(metadata).To(HaveKey("creationTimestamp"))

			By("verifying spec fields")
			spec := receivedPayload["spec"].(map[string]interface{})
			Expect(spec["templateID"]).To(Equal("ocp_virt_vm"))
			Expect(spec["templateParameters"]).To(ContainSubstring("webhook-test-vm"))

			By("verifying status fields")
			status := receivedPayload["status"].(map[string]interface{})
			Expect(status["phase"]).To(Equal(string(v1alpha1.CloudkitVMPhaseProvisioning)))
			Expect(status).To(HaveKey("conditions"))
			Expect(status).To(HaveKey("vmReference"))

			vmRef := status["vmReference"].(map[string]interface{})
			Expect(vmRef["namespace"]).To(Equal("vm-test-namespace"))
			Expect(vmRef["vmName"]).To(Equal("test-vm-instance"))
			Expect(vmRef["ipAddress"]).To(Equal("192.168.1.50"))
		})

		It("should handle empty webhook URL gracefully", func() {
			By("calling webhook with empty URL")
			err := postCloudkitVMWebhook(ctx, "", cloudkitVM)
			Expect(err).NotTo(HaveOccurred()) // Should not error, just log and return
		})

		It("should handle HTTP errors from webhook", func() {
			By("setting up a mock webhook server that returns errors")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()

			By("calling the webhook")
			err := postCloudkitVMWebhook(ctx, server.URL, cloudkitVM)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("webhook returned non-success status code: 500"))
		})

		It("should handle network errors", func() {
			By("calling webhook with invalid URL")
			err := postCloudkitVMWebhook(ctx, "http://invalid-url-that-does-not-exist:9999", cloudkitVM)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to send HTTP request"))
		})

		It("should handle malformed URLs", func() {
			By("calling webhook with malformed URL")
			err := postCloudkitVMWebhook(ctx, "not-a-valid-url", cloudkitVM)
			Expect(err).To(HaveOccurred())
		})

		It("should include all required fields in payload", func() {
			By("setting up a webhook server to capture payload")
			var receivedPayload map[string]interface{}

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				Expect(err).NotTo(HaveOccurred())

				err = json.Unmarshal(body, &receivedPayload)
				Expect(err).NotTo(HaveOccurred())

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			By("calling webhook")
			err := postCloudkitVMWebhook(ctx, server.URL, cloudkitVM)
			Expect(err).NotTo(HaveOccurred())

			By("verifying all required fields are present")
			// Check top-level fields
			requiredTopLevel := []string{"apiVersion", "kind", "metadata", "spec", "status"}
			for _, field := range requiredTopLevel {
				Expect(receivedPayload).To(HaveKey(field), "Missing required field: %s", field)
			}

			// Check metadata fields
			metadata := receivedPayload["metadata"].(map[string]interface{})
			requiredMetadata := []string{"name", "namespace", "uid", "resourceVersion", "creationTimestamp"}
			for _, field := range requiredMetadata {
				Expect(metadata).To(HaveKey(field), "Missing required metadata field: %s", field)
			}

			// Check spec fields
			spec := receivedPayload["spec"].(map[string]interface{})
			requiredSpec := []string{"templateID", "templateParameters"}
			for _, field := range requiredSpec {
				Expect(spec).To(HaveKey(field), "Missing required spec field: %s", field)
			}

			// Check status fields
			status := receivedPayload["status"].(map[string]interface{})
			requiredStatus := []string{"phase", "conditions", "vmReference"}
			for _, field := range requiredStatus {
				Expect(status).To(HaveKey(field), "Missing required status field: %s", field)
			}
		})

		It("should handle CloudkitVM with minimal data", func() {
			By("creating a minimal CloudkitVM")
			minimalVM := &v1alpha1.CloudkitVM{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "minimal-vm",
					Namespace: "default",
				},
				Spec: v1alpha1.CloudkitVMSpec{
					TemplateID: "basic_template",
				},
			}

			By("setting up webhook server")
			var receivedPayload map[string]interface{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				Expect(err).NotTo(HaveOccurred())

				err = json.Unmarshal(body, &receivedPayload)
				Expect(err).NotTo(HaveOccurred())

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			By("calling webhook with minimal VM")
			err := postCloudkitVMWebhook(ctx, server.URL, minimalVM)
			Expect(err).NotTo(HaveOccurred())

			By("verifying basic structure is maintained")
			Expect(receivedPayload).To(HaveKey("metadata"))
			Expect(receivedPayload).To(HaveKey("spec"))

			spec := receivedPayload["spec"].(map[string]interface{})
			Expect(spec["templateID"]).To(Equal("basic_template"))
		})

		It("should handle different HTTP success codes", func() {
			successCodes := []int{http.StatusOK, http.StatusCreated, http.StatusAccepted}

			for _, code := range successCodes {
				By(fmt.Sprintf("testing with HTTP status code %d", code))
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(code)
				}))

				err := postCloudkitVMWebhook(ctx, server.URL, cloudkitVM)
				Expect(err).NotTo(HaveOccurred(), "Should succeed with status code %d", code)

				server.Close()
			}
		})

		It("should handle different HTTP error codes", func() {
			errorCodes := []int{
				http.StatusBadRequest,
				http.StatusUnauthorized,
				http.StatusForbidden,
				http.StatusNotFound,
				http.StatusInternalServerError,
				http.StatusBadGateway,
			}

			for _, code := range errorCodes {
				By(fmt.Sprintf("testing with HTTP status code %d", code))
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(code)
				}))

				err := postCloudkitVMWebhook(ctx, server.URL, cloudkitVM)
				Expect(err).To(HaveOccurred(), "Should error with status code %d", code)
				Expect(err.Error()).To(ContainSubstring(fmt.Sprintf("status code: %d", code)))

				server.Close()
			}
		})

		It("should use POST method", func() {
			By("setting up webhook server to verify HTTP method")
			var receivedMethod string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedMethod = r.Method
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			By("calling webhook")
			err := postCloudkitVMWebhook(ctx, server.URL, cloudkitVM)
			Expect(err).NotTo(HaveOccurred())

			By("verifying POST method was used")
			Expect(receivedMethod).To(Equal("POST"))
		})

		It("should handle context cancellation", func() {
			By("creating a context that will be canceled")
			cancelCtx, cancel := context.WithCancel(ctx)

			By("setting up a slow webhook server")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				time.Sleep(time.Second) // Simulate slow response
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			By("canceling the context immediately")
			cancel()

			By("calling webhook with canceled context")
			err := postCloudkitVMWebhook(cancelCtx, server.URL, cloudkitVM)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("context canceled"))
		})
	})
})
