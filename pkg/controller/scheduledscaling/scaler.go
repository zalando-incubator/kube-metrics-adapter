package scheduledscaling

import (
	"context"
	"fmt"
	"time"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	cacheddiscovery "k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/scale"
	scaleclient "k8s.io/client-go/scale"
)

// TargetScaler is an interface for scaling a target referenced resource in an
// HPA to the desired replicas.
type TargetScaler interface {
	Scale(ctx context.Context, hpa *autoscalingv2.HorizontalPodAutoscaler, replicas int32) error
}

type hpaTargetScaler struct {
	mapper      apimeta.RESTMapper
	scaleClient scaleclient.ScalesGetter
}

// NewHPATargetScaler creates a new TargetScaler that can scale resources
// targeted by HPAs. It takes a Kubernetes client and a REST config and uses a
// restmapper to resolve the target reference API.
func NewHPATargetScaler(ctx context.Context, kubeClient kubernetes.Interface, cfg *rest.Config) (TargetScaler, error) {
	cachedClient := cacheddiscovery.NewMemCacheClient(kubeClient.Discovery())
	restMapper := restmapper.NewDeferredDiscoveryRESTMapper(cachedClient)
	go wait.Until(func() {
		restMapper.Reset()
	}, 30*time.Second, ctx.Done())

	scaleKindResolver := scale.NewDiscoveryScaleKindResolver(kubeClient.Discovery())
	scaleClient, err := scale.NewForConfig(cfg, restMapper, dynamic.LegacyAPIPathResolverFunc, scaleKindResolver)
	if err != nil {
		return nil, fmt.Errorf("failed to create scale client: %w", err)
	}

	return &hpaTargetScaler{
		mapper:      restMapper,
		scaleClient: scaleClient,
	}, nil
}

// Scale scales the target resource of the given HPA to the desired number of
// replicas.
func (s *hpaTargetScaler) Scale(ctx context.Context, hpa *autoscalingv2.HorizontalPodAutoscaler, replicas int32) error {
	reference := fmt.Sprintf("%s/%s/%s", hpa.Spec.ScaleTargetRef.Kind, hpa.Namespace, hpa.Spec.ScaleTargetRef.Name)

	targetGV, err := schema.ParseGroupVersion(hpa.Spec.ScaleTargetRef.APIVersion)
	if err != nil {
		return fmt.Errorf("invalid API version '%s' in scale target reference: %w", hpa.Spec.ScaleTargetRef.APIVersion, err)
	}

	targetGK := schema.GroupKind{
		Group: targetGV.Group,
		Kind:  hpa.Spec.ScaleTargetRef.Kind,
	}

	mappings, err := s.mapper.RESTMappings(targetGK)
	if err != nil {
		return fmt.Errorf("unable to determine resource for scale target reference: %w", err)
	}

	scale, targetGR, err := s.scaleForResourceMappings(ctx, hpa.Namespace, hpa.Spec.ScaleTargetRef.Name, mappings)
	if err != nil {
		return fmt.Errorf("failed to get scale subresource for %s: %w", reference, err)
	}

	scale.Spec.Replicas = replicas
	_, err = s.scaleClient.Scales(hpa.Namespace).Update(ctx, targetGR, scale, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to rescale %s: %w", reference, err)
	}

	return nil
}

// scaleForResourceMappings attempts to fetch the scale for the
// resource with the given name and namespace, trying each RESTMapping
// in turn until a working one is found.  If none work, the first error
// is returned.  It returns both the scale, as well as the group-resource from
// the working mapping.
// from: https://github.com/kubernetes/kubernetes/blob/c9092f69fc0c099062dd23cd6ee226bcd52ec790/pkg/controller/podautoscaler/horizontal.go#L1326-L1353
func (s *hpaTargetScaler) scaleForResourceMappings(ctx context.Context, namespace, name string, mappings []*apimeta.RESTMapping) (*autoscalingv1.Scale, schema.GroupResource, error) {
	var firstErr error
	for i, mapping := range mappings {
		targetGR := mapping.Resource.GroupResource()
		scale, err := s.scaleClient.Scales(namespace).Get(ctx, targetGR, name, metav1.GetOptions{})
		if err == nil {
			return scale, targetGR, nil
		}

		// if this is the first error, remember it,
		// then go on and try other mappings until we find a good one
		if i == 0 {
			firstErr = err
		}
	}

	// make sure we handle an empty set of mappings
	if firstErr == nil {
		firstErr = fmt.Errorf("unrecognized resource")
	}

	return nil, schema.GroupResource{}, firstErr
}
