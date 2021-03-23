package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	monitoring "github.com/google/gpe-collector/pkg/operator/apis/monitoring"
)

const (
	Version = "v1alpha1"
)

var (
	// SchemeBuilder initializes a scheme builder.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	// AddToScheme is a global function that registers this API group & version to a scheme.
	AddToScheme = SchemeBuilder.AddToScheme
	// SchemeGroupVersion is group version used to register these objects.
	SchemeGroupVersion = schema.GroupVersion{Group: monitoring.GroupName, Version: Version}
)

// Kind takes an unqualified kind and returns back a Group qualified GroupKind.
func Kind(kind string) schema.GroupKind {
	return SchemeGroupVersion.WithKind(kind).GroupKind()
}

// Resource takes an unqualified resource and returns a Group qualified GroupResource.
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

// PodMonitoringResource returns a PodMonitoring GroupVersionResource.
// This can be used to enforce API types.
func PodMonitoringResource() metav1.GroupVersionResource {
	return metav1.GroupVersionResource{
		Group:    monitoring.GroupName,
		Version:  Version,
		Resource: "PodMonitoring",
	}
}

// ServiceMonitoringResource returns a ServiceMonitoring GroupVersionResource.
// This can be used to enforce API types.
func ServiceMonitoringResource() metav1.GroupVersionResource {
	return metav1.GroupVersionResource{
		Group:    monitoring.GroupName,
		Version:  Version,
		Resource: "ServiceMonitoring",
	}
}

// Adds the list of known types to Scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&ServiceMonitoring{},
		&ServiceMonitoringList{},
		&PodMonitoring{},
		&PodMonitoringList{},
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
