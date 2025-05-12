package controller

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	cloudkitv1alpha1 "github.com/innabox/cloudkit-operator/api/v1alpha1"
)

var instanceTracking map[string]time.Time

const DefaultRequestInterval string = "5m"

func triggerWebHook(ctx context.Context, url string, instance *cloudkitv1alpha1.ClusterOrder) error {
	log := ctrllog.FromContext(ctx)

	if lastUpdate, found := instanceTracking[url]; found {
		duration, err := time.ParseDuration(instance.MinimumRequestInterval)
		if err != nil {
			instance.MinimumRequestInterval = ""
		}

		if instance.MinimumRequestInterval != "" {
			log.Info("Invalid minimum request interval.  Setting it to the default.", "MinimumRequestInterval", DefaultRequestInterval, "")
			duration, _ = time.ParseDuration(DefaultRequestInterval)
		}

		minInterval := int64(duration.Seconds())
		delta := int64(time.Since(lastUpdate).Seconds())
		if delta < minInterval {
			log.Info("Tring to call webhook too soon.", "url", url, "minInterval", minInterval, "delta", int(delta), "lastUpdate", lastUpdate.String())
			return fmt.Errorf("request inside the minimum request interval of %s", instance.MinimumRequestInterval)
		}
	} else {
		instanceTracking[url] = time.Now()
	}

	log.Info("Triggering webhook " + url)

	jsonData, err := json.Marshal(instance)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("received non-success status code: %d", resp.StatusCode)
	}

	return nil
}
