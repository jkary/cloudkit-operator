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

// Important: Run "make" to regenerate code after modifying this file

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CloudkitVMSpec defines the desired state of CloudkitVM
type CloudkitVMSpec struct {
	// TemplateID is the unique identifier of the VM template to use when creating this VM
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern=^[a-zA-Z_][a-zA-Z0-9._]*$
	TemplateID string `json:"templateID,omitempty"`

	// TemplateParameters is a JSON-encoded map of the parameter values for the
	// selected VM template.
	// +kubebuilder:validation:Optional
	TemplateParameters string `json:"templateParameters,omitempty"`
}

// CloudkitVMPhaseType is a valid value for .status.phase
type CloudkitVMPhaseType string

const (
	// CloudkitVMPhasePending means the VM order has been received but not yet processed
	CloudkitVMPhasePending CloudkitVMPhaseType = "Pending"

	// CloudkitVMPhaseProvisioning means the VM is being provisioned
	CloudkitVMPhaseProvisioning CloudkitVMPhaseType = "Provisioning"

	// CloudkitVMPhaseReady means the VM and all associated resources are ready
	CloudkitVMPhaseReady CloudkitVMPhaseType = "Ready"

	// CloudkitVMPhaseFailed means the VM deployment has failed
	CloudkitVMPhaseFailed CloudkitVMPhaseType = "Failed"

	// CloudkitVMPhaseDeleting means there has been a request to delete the CloudkitVM
	CloudkitVMPhaseDeleting CloudkitVMPhaseType = "Deleting"
)

// CloudkitVMConditionType is a valid value for .status.conditions.type
type CloudkitVMConditionType string

const (
	// CloudkitVMConditionAccepted means the VM order has been accepted but work has not yet started
	CloudkitVMConditionAccepted CloudkitVMConditionType = "Accepted"

	// CloudkitVMConditionProgressing means that VM provisioning is in progress
	CloudkitVMConditionProgressing CloudkitVMConditionType = "Progressing"

	// CloudkitVMConditionAvailable means the VM is available and ready
	CloudkitVMConditionAvailable CloudkitVMConditionType = "Available"

	// CloudkitVMConditionFailed means the VM provisioning has failed
	CloudkitVMConditionFailed CloudkitVMConditionType = "Failed"
)

// CloudkitVMReferenceType contains a reference to the VM and resources created by this CloudkitVM
type CloudkitVMReferenceType struct {
	// Namespace that contains the VM resources
	Namespace string `json:"namespace"`

	// VMName is the name of the VirtualMachine resource
	VMName string `json:"vmName"`

	// HubID is the identifier of the hub where this VM is deployed
	HubID string `json:"hubID,omitempty"`

	// IPAddress is the IP address assigned to the VM
	IPAddress string `json:"ipAddress,omitempty"`
}

// CloudkitVMStatus defines the observed state of CloudkitVM
type CloudkitVMStatus struct {
	// Phase provides a single-value overview of the state of the CloudkitVM
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Enum=Pending;Provisioning;Ready;Failed;Deleting
	Phase CloudkitVMPhaseType `json:"phase,omitempty"`

	// Conditions holds an array of metav1.Condition that describe the state of the CloudkitVM
	// +kubebuilder:validation:Optional
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type" protobuf:"bytes,1,rep,name=conditions"`

	// Reference to the VM and namespace that contains the VM resources
	// +kubebuilder:validation:Optional
	VMReference *CloudkitVMReferenceType `json:"vmReference,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=cvm
// +kubebuilder:printcolumn:name="Template",type=string,JSONPath=`.spec.templateID`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="VM Name",type=string,JSONPath=`.status.vmReference.vmName`
// +kubebuilder:printcolumn:name="IP Address",type=string,JSONPath=`.status.vmReference.ipAddress`

// CloudkitVM is the Schema for the cloudkitvms API
type CloudkitVM struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CloudkitVMSpec   `json:"spec,omitempty"`
	Status CloudkitVMStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CloudkitVMList contains a list of CloudkitVM
type CloudkitVMList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CloudkitVM `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CloudkitVM{}, &CloudkitVMList{})
}
