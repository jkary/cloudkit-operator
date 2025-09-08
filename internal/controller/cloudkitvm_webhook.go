package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	cloudkitv1alpha1 "github.com/innabox/cloudkit-operator/api/v1alpha1"
)

func postCloudkitVMWebhook(ctx context.Context, webhookURL string, cloudkitVM *cloudkitv1alpha1.CloudkitVM) error {
	log := ctrllog.FromContext(ctx)

	if webhookURL == "" {
		log.Info("VM webhook URL is not configured, skipping webhook call")
		return nil
	}

	// Create the webhook payload
	payload := map[string]interface{}{
		"apiVersion": cloudkitVM.APIVersion,
		"kind":       cloudkitVM.Kind,
		"metadata": map[string]interface{}{
			"name":              cloudkitVM.Name,
			"namespace":         cloudkitVM.Namespace,
			"uid":               cloudkitVM.UID,
			"resourceVersion":   cloudkitVM.ResourceVersion,
			"creationTimestamp": cloudkitVM.CreationTimestamp,
		},
		"spec": map[string]interface{}{
			"templateID":         cloudkitVM.Spec.TemplateID,
			"templateParameters": cloudkitVM.Spec.TemplateParameters,
		},
		"status": map[string]interface{}{
			"phase":       cloudkitVM.Status.Phase,
			"conditions":  cloudkitVM.Status.Conditions,
			"vmReference": cloudkitVM.Status.VMReference,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal CloudkitVM webhook payload: %w", err)
	}

	log.Info("Posting CloudkitVM to webhook",
		"webhookURL", webhookURL,
		"cloudkitVMName", cloudkitVM.Name,
		"templateID", cloudkitVM.Spec.TemplateID)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "cloudkit-operator/1.0")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request to webhook: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned non-success status code: %d", resp.StatusCode)
	}

	log.Info("Successfully posted CloudkitVM to webhook",
		"webhookURL", webhookURL,
		"cloudkitVMName", cloudkitVM.Name,
		"responseStatus", resp.StatusCode)

	return nil
}
