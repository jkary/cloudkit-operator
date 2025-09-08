package controller

import (
	"fmt"

	v1alpha1 "github.com/innabox/cloudkit-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/util/rand"
)

const (
	defaultCloudkitVMNamespace string = "cloudkit-vms"
)

var (
	cloudkitVMNameLabel string = fmt.Sprintf("%s/cloudkitvm", cloudkitNamePrefix)
	cloudkitVMIDLabel   string = fmt.Sprintf("%s/cloudkitvm-uuid", cloudkitNamePrefix)
	cloudkitVMFinalizer string = fmt.Sprintf("%s/vm-finalizer", cloudkitNamePrefix)
)

func generateVMNamespaceName(instance *v1alpha1.CloudkitVM) string {
	return fmt.Sprintf("vm-%s-%s", instance.GetName(), rand.String(6))
}
