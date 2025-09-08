package controller

import (
	"context"
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/innabox/cloudkit-operator/api/v1alpha1"
)

func (r *CloudkitVMReconciler) newVMNamespace(ctx context.Context, instance *v1alpha1.CloudkitVM) (*appResource, error) {
	log := ctrllog.FromContext(ctx)

	var namespaceList corev1.NamespaceList
	var namespaceName string

	if err := r.List(ctx, &namespaceList, vmLabelSelectorFromInstance(instance)); err != nil {
		log.Error(err, "failed to list VM namespaces")
		return nil, err
	}

	if len(namespaceList.Items) > 1 {
		return nil, fmt.Errorf("found multiple matching namespaces for CloudkitVM %s", instance.GetName())
	}

	if len(namespaceList.Items) == 0 {
		namespaceName = generateVMNamespaceName(instance)
		if namespaceName == "" {
			return nil, fmt.Errorf("failed to generate VM namespace name")
		}
	} else {
		namespaceName = namespaceList.Items[0].GetName()
	}

	namespace := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name:   namespaceName,
			Labels: commonVMLabelsFromInstance(instance),
		},
	}

	mutateFn := func() error {
		ensureCommonVMLabels(instance, namespace)
		// Note: Cannot set controller reference for cluster-scoped resources (Namespace)
		// from namespace-scoped resources (CloudkitVM). We rely on labels for cleanup.
		return nil
	}

	return &appResource{object: namespace, mutateFn: mutateFn}, nil
}

func vmLabelSelectorFromInstance(instance *v1alpha1.CloudkitVM) client.ListOption {
	labels := commonVMLabelsFromInstance(instance)
	return client.MatchingLabels(labels)
}

func commonVMLabelsFromInstance(instance *v1alpha1.CloudkitVM) map[string]string {
	labels := make(map[string]string)
	labels["app.kubernetes.io/name"] = cloudkitAppName
	labels["app.kubernetes.io/component"] = "vm"
	labels[cloudkitVMNameLabel] = instance.GetName()
	labels[cloudkitVMIDLabel] = string(instance.GetUID())
	return labels
}

func ensureCommonVMLabels(instance *v1alpha1.CloudkitVM, obj client.Object) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	maps.Copy(labels, commonVMLabelsFromInstance(instance))
	obj.SetLabels(labels)
}
