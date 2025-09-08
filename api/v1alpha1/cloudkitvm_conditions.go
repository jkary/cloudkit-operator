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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SetAcceptedCondition sets the Accepted condition on the CloudkitVM
func (c *CloudkitVM) SetAcceptedCondition(status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               string(CloudkitVMConditionAccepted),
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&c.Status.Conditions, condition)
}

// SetProgressingCondition sets the Progressing condition on the CloudkitVM
func (c *CloudkitVM) SetProgressingCondition(status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               string(CloudkitVMConditionProgressing),
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&c.Status.Conditions, condition)
}

// SetAvailableCondition sets the Available condition on the CloudkitVM
func (c *CloudkitVM) SetAvailableCondition(status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               string(CloudkitVMConditionAvailable),
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&c.Status.Conditions, condition)
}

// SetFailedCondition sets the Failed condition on the CloudkitVM
func (c *CloudkitVM) SetFailedCondition(status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               string(CloudkitVMConditionFailed),
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&c.Status.Conditions, condition)
}

// GetCondition returns the condition with the given type, if it exists
func (c *CloudkitVM) GetCondition(conditionType CloudkitVMConditionType) *metav1.Condition {
	return meta.FindStatusCondition(c.Status.Conditions, string(conditionType))
}

// IsConditionTrue returns true if the condition with the given type is present and has status True
func (c *CloudkitVM) IsConditionTrue(conditionType CloudkitVMConditionType) bool {
	condition := c.GetCondition(conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}
