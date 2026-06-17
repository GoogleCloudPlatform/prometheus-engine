package migrate

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// LogLevel defines the severity of a migration log.
type LogLevel string

const (
	LevelInfo    LogLevel = "INFO"
	LevelWarning LogLevel = "WARNING"
	LevelError   LogLevel = "ERROR"
	LevelSuccess LogLevel = "SUCCESS"
)

// LogMessage represents a structured log entry associated with a specific resource.
type LogMessage struct {
	Level     LogLevel
	Kind      string
	Namespace string
	Name      string
	Message   string
}

// String formats the log message consistently: [LEVEL] [Kind: namespace/name] Message.
func (l LogMessage) String() string {
	if l.Kind == "" && l.Namespace == "" {
		return fmt.Sprintf("[%s] [%s] %s", l.Level, l.Name, l.Message)
	}
	return fmt.Sprintf("[%s] [%s:%s/%s] %s", l.Level, l.Kind, l.Namespace, l.Name, l.Message)
}

// ResourceConverter defines the interface for converting a specific Kubernetes resource kind.
type ResourceConverter interface {
	// ImportKey returns the Kind of the resource this converter handles (e.g., "PodMonitor").
	ImportKey() string
	// Convert translates the input unstructured resource to one or more output unstructured resources.
	// It returns the converted resources, structured log messages (warnings/infos), and any fatal error.
	Convert(unstruct *unstructured.Unstructured, cache *ResourceCache) ([]*unstructured.Unstructured, []LogMessage, error)
}

// ResourceCache stores parsed Kubernetes resources for cross-resource resolution.
type ResourceCache struct {
	// Map of Kind -> Namespace/Name -> Resource
	resources map[string]map[string]*unstructured.Unstructured
}

// NewResourceCache creates a new initialized ResourceCache.
func NewResourceCache() *ResourceCache {
	return &ResourceCache{
		resources: make(map[string]map[string]*unstructured.Unstructured),
	}
}

// Add adds a resource to the cache.
func (c *ResourceCache) Add(u *unstructured.Unstructured) {
	kind := u.GetKind()
	if _, ok := c.resources[kind]; !ok {
		c.resources[kind] = make(map[string]*unstructured.Unstructured)
	}

	ns := u.GetNamespace()
	if ns == "" {
		ns = "default" // All Prometheus Operator and K8s inputs are namespaced. Default to "default" if omitted.
	}

	key := fmt.Sprintf("%s/%s", ns, u.GetName())
	c.resources[kind][key] = u
}

// Get retrieves a resource from the cache by kind, namespace, and name.
func (c *ResourceCache) Get(kind, namespace, name string) (*unstructured.Unstructured, bool) {
	nsMap, ok := c.resources[kind]
	if !ok {
		return nil, false
	}
	key := fmt.Sprintf("%s/%s", namespace, name)
	r, ok := nsMap[key]
	return r, ok
}
