package collector

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/api/apps/v1"
	autoscalingv2beta2 "k8s.io/api/autoscaling/v2beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestTargetRefReplicasDeployments(t *testing.T) {
	client := fake.NewSimpleClientset()
	name := "some-app"
	defaultNamespace := "default"
	deployment, err := newDeployment(client, defaultNamespace, name)
	require.NoError(t, err)

	// Create an HPA with the deployment as ref
	hpa, err := client.AutoscalingV2beta2().HorizontalPodAutoscalers(deployment.Namespace).
		Create(newHPA(defaultNamespace, name, "Deployment"))
	require.NoError(t, err)

	replicas, err := targetRefReplicas(client, hpa)
	require.NoError(t, err)
	require.Equal(t, deployment.Status.Replicas, replicas)
}

func TestTargetRefReplicasStatefulSets(t *testing.T) {
	client := fake.NewSimpleClientset()
	name := "some-app"
	defaultNamespace := "default"
	statefulSet, err := newStatefulSet(client, defaultNamespace, name)
	require.NoError(t, err)

	// Create an HPA with the statefulSet as ref
	hpa, err := client.AutoscalingV2beta2().HorizontalPodAutoscalers(statefulSet.Namespace).
		Create(newHPA(defaultNamespace, name, "StatefulSet"))
	require.NoError(t, err)

	replicas, err := targetRefReplicas(client, hpa)
	require.NoError(t, err)
	require.Equal(t, statefulSet.Status.Replicas, replicas)
}

func newHPA(namesapce string, refName string, refKind string) *autoscalingv2beta2.HorizontalPodAutoscaler {
	return &autoscalingv2beta2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name: namesapce,
		},
		Spec: autoscalingv2beta2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2beta2.CrossVersionObjectReference{
				Name: refName,
				Kind: refKind,
			},
		},
		Status: autoscalingv2beta2.HorizontalPodAutoscalerStatus{},
	}
}

func newDeployment(client *fake.Clientset, namespace string, name string) (*v1.Deployment, error) {
	return client.AppsV1().Deployments(namespace).Create(&v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v1.DeploymentSpec{},
		Status: v1.DeploymentStatus{
			ReadyReplicas: 1,
			Replicas:      2,
		},
	})
}

func newStatefulSet(client *fake.Clientset, namespace string, name string) (*v1.StatefulSet, error) {
	return client.AppsV1().StatefulSets(namespace).Create(&v1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Status: v1.StatefulSetStatus{
			ReadyReplicas: 1,
			Replicas:      2,
		},
	})
}
