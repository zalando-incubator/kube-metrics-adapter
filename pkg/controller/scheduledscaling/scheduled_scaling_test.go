package scheduledscaling

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "github.com/zalando-incubator/kube-metrics-adapter/pkg/apis/zalando.org/v1"
	scalingschedulefake "github.com/zalando-incubator/kube-metrics-adapter/pkg/client/clientset/versioned/fake"
	zfake "github.com/zalando-incubator/kube-metrics-adapter/pkg/client/clientset/versioned/fake"
	zalandov1 "github.com/zalando-incubator/kube-metrics-adapter/pkg/client/clientset/versioned/typed/zalando.org/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
)

const (
	hHMMFormat = "15:04"
)

type fakeClusterScalingScheduleStore struct {
	client zalandov1.ZalandoV1Interface
}

func (s fakeClusterScalingScheduleStore) List() []interface{} {
	schedules, err := s.client.ClusterScalingSchedules().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil
	}

	objects := make([]interface{}, 0, len(schedules.Items))
	for _, schedule := range schedules.Items {
		schedule := schedule
		objects = append(objects, &schedule)
	}

	return objects
}

type fakeScalingScheduleStore struct {
	client zalandov1.ZalandoV1Interface
}

func (s fakeScalingScheduleStore) List() []interface{} {
	schedules, err := s.client.ScalingSchedules(corev1.NamespaceAll).List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil
	}

	objects := make([]interface{}, 0, len(schedules.Items))
	for _, schedule := range schedules.Items {
		schedule := schedule
		objects = append(objects, &schedule)
	}

	return objects
}

type schedule struct {
	schedules       []v1.Schedule
	expectedActive  bool
	preActiveStatus bool
}

func scheduleDate(date string) *v1.ScheduleDate {
	d := v1.ScheduleDate(date)
	return &d
}

func TestRunOnce(t *testing.T) {
	nowRFC3339 := "2009-11-10T23:00:00+01:00" // +01:00 is Berlin timezone (default)
	nowTime, _ := time.Parse(time.RFC3339, nowRFC3339)
	nowWeekday := v1.TuesdaySchedule

	// now func always return in UTC for test purposes
	utc, _ := time.LoadLocation("UTC")
	uTCNow := nowTime.In(utc)
	// uTCNowRFC3339 := uTCNow.Format(time.RFC3339)
	now := func() time.Time {
		return uTCNow
	}
	// date := v1.ScheduleDate(schedule.date)
	// 			endDate := v1.ScheduleDate(schedule.endDate)
	for _, tc := range []struct {
		msg             string
		schedules       map[string]schedule
		preActiveStatus bool
		// scalingWindowDurationMinutes *int64
		// expectedValue                int64
		// err                          error
		// rampSteps                    int
	}{
		{
			msg: "OneTime Schedules",
			schedules: map[string]schedule{
				"active": {
					schedules: []v1.Schedule{
						{
							Type:            v1.OneTimeSchedule,
							Date:            scheduleDate(nowRFC3339),
							DurationMinutes: 15,
						},
					},
					expectedActive: true,
				},
				"inactive": {
					schedules: []v1.Schedule{
						{
							Type:            v1.OneTimeSchedule,
							Date:            scheduleDate(nowTime.Add(1 * time.Hour).Format(time.RFC3339)),
							DurationMinutes: 15,
						},
					},
					expectedActive: false,
				},
			},
		},
		{
			msg: "OneTime Schedules change active",
			schedules: map[string]schedule{
				"active": {
					schedules: []v1.Schedule{
						{
							Type:            v1.OneTimeSchedule,
							Date:            scheduleDate(nowRFC3339),
							DurationMinutes: 15,
						},
					},
					preActiveStatus: false,
					expectedActive:  true,
				},
				"inactive": {
					schedules: []v1.Schedule{
						{
							Type:            v1.OneTimeSchedule,
							Date:            scheduleDate(nowTime.Add(1 * time.Hour).Format(time.RFC3339)),
							DurationMinutes: 15,
						},
					},
					preActiveStatus: true,
					expectedActive:  false,
				},
			},
		},
		{
			msg: "Repeating Schedules",
			schedules: map[string]schedule{
				"active": {
					schedules: []v1.Schedule{
						{
							Type:            v1.RepeatingSchedule,
							DurationMinutes: 15,
							Period: &v1.SchedulePeriod{
								Days:      []v1.ScheduleDay{nowWeekday},
								StartTime: nowTime.Format(hHMMFormat),
							},
						},
					},
					expectedActive: true,
				},
				"inactive": {
					schedules: []v1.Schedule{
						{
							Type:            v1.RepeatingSchedule,
							DurationMinutes: 15,
							Period: &v1.SchedulePeriod{
								Days:      []v1.ScheduleDay{nowWeekday},
								StartTime: nowTime.Add(1 * time.Hour).Format(hHMMFormat),
							},
						},
					},
					expectedActive: false,
				},
			},
		},
		{
			msg: "Repeating Schedules change active",
			schedules: map[string]schedule{
				"active": {
					schedules: []v1.Schedule{
						{
							Type:            v1.RepeatingSchedule,
							DurationMinutes: 15,
							Period: &v1.SchedulePeriod{
								Days:      []v1.ScheduleDay{nowWeekday},
								StartTime: nowTime.Format(hHMMFormat),
							},
						},
					},
					preActiveStatus: false,
					expectedActive:  true,
				},
				"inactive": {
					schedules: []v1.Schedule{
						{
							Type:            v1.RepeatingSchedule,
							DurationMinutes: 15,
							Period: &v1.SchedulePeriod{
								Days:      []v1.ScheduleDay{nowWeekday},
								StartTime: nowTime.Add(1 * time.Hour).Format(hHMMFormat),
							},
						},
					},
					preActiveStatus: true,
					expectedActive:  false,
				},
			},
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			// setup fake client and cache
			client := scalingschedulefake.NewSimpleClientset()

			clusterScalingSchedulesStore := fakeClusterScalingScheduleStore{
				client: client.ZalandoV1(),
			}

			scalingSchedulesStore := fakeScalingScheduleStore{
				client: client.ZalandoV1(),
			}

			// add schedules
			err := applySchedules(client.ZalandoV1(), tc.schedules)
			require.NoError(t, err)

			controller := NewController(client.ZalandoV1(), fake.NewSimpleClientset(), nil, scalingSchedulesStore, clusterScalingSchedulesStore, now, 0, "Europe/Berlin", 0.10)

			err = controller.runOnce(context.Background())
			require.NoError(t, err)

			// check schedule status
			err = checkSchedules(t, client.ZalandoV1(), tc.schedules)
			require.NoError(t, err)

		})
	}
}

func applySchedules(client zalandov1.ZalandoV1Interface, schedules map[string]schedule) error {
	for name, schedule := range schedules {
		spec := v1.ScalingScheduleSpec{
			// ScalingWindowDurationMinutes *int64 `json:"scalingWindowDurationMinutes,omitempty"`
			Schedules: schedule.schedules,
		}

		status := v1.ScalingScheduleStatus{
			Active: schedule.preActiveStatus,
		}

		scalingSchedule := &v1.ScalingSchedule{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "default",
			},

			Spec:   spec,
			Status: status,
		}

		_, err := client.ScalingSchedules(scalingSchedule.Namespace).Create(context.Background(), scalingSchedule, metav1.CreateOptions{})
		if err != nil {
			return err
		}

		clusterScalingSchedule := &v1.ClusterScalingSchedule{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},

			Spec:   spec,
			Status: status,
		}

		_, err = client.ClusterScalingSchedules().Create(context.Background(), clusterScalingSchedule, metav1.CreateOptions{})
		if err != nil {
			return err
		}
	}

	return nil
}

func checkSchedules(t *testing.T, client zalandov1.ZalandoV1Interface, schedules map[string]schedule) error {
	for name, expectedSchedule := range schedules {
		scalingSchedule, err := client.ScalingSchedules("default").Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		require.Equal(t, expectedSchedule.expectedActive, scalingSchedule.Status.Active)

		clusterScalingSchedule, err := client.ClusterScalingSchedules().Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		require.Equal(t, expectedSchedule.expectedActive, clusterScalingSchedule.Status.Active)
	}
	return nil
}

type mockScaler struct {
	client kubernetes.Interface
}

func (s *mockScaler) Scale(ctx context.Context, hpa *autoscalingv2.HorizontalPodAutoscaler, replicas int32) error {
	switch hpa.Spec.ScaleTargetRef.Kind {
	case "Deployment":
		deployment, err := s.client.AppsV1().Deployments(hpa.Namespace).Get(ctx, hpa.Spec.ScaleTargetRef.Name, metav1.GetOptions{})
		if err != nil {
			return err
		}

		deployment.Spec.Replicas = &replicas
		_, err = s.client.AppsV1().Deployments(hpa.Namespace).Update(ctx, deployment, metav1.UpdateOptions{})
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported kind %s", hpa.Spec.ScaleTargetRef.Kind)
	}

	return nil
}

func TestAdjustScaling(t *testing.T) {
	for _, tc := range []struct {
		msg             string
		currentReplicas int32
		desiredReplicas int32
		targetValue     int64
	}{
		{
			msg:             "current less than 10%% below desired",
			currentReplicas: 95, // 5.3% increase to desired
			desiredReplicas: 100,
			targetValue:     10, // 1000/10 = 100
		},
		{
			msg:             "current more than 10%% below desired, no adjustment",
			currentReplicas: 90, // 11% increase to desired
			desiredReplicas: 90,
			targetValue:     10, // 1000/10 = 100
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			kubeClient := fake.NewSimpleClientset()
			scalingScheduleClient := zfake.NewSimpleClientset()
			controller := NewController(
				scalingScheduleClient.ZalandoV1(),
				kubeClient,
				&mockScaler{client: kubeClient},
				nil,
				nil,
				time.Now,
				time.Hour,
				"Europe/Berlin",
				0.10,
			)

			scheduleDate := v1.ScheduleDate(time.Now().Add(-10 * time.Minute).Format(time.RFC3339))
			clusterScalingSchedules := []v1.ScalingScheduler{
				&v1.ClusterScalingSchedule{
					ObjectMeta: metav1.ObjectMeta{
						Name: "schedule-1",
					},
					Spec: v1.ScalingScheduleSpec{
						Schedules: []v1.Schedule{
							{
								Type:            v1.OneTimeSchedule,
								Date:            &scheduleDate,
								DurationMinutes: 15,
								Value:           1000,
							},
						},
					},
				},
			}

			deployment := &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "deployment-1",
				},
				Spec: appsv1.DeploymentSpec{
					Replicas: ptr.To(tc.currentReplicas),
				},
			}

			_, err := kubeClient.AppsV1().Deployments("default").Create(context.Background(), deployment, metav1.CreateOptions{})
			require.NoError(t, err)

			hpa := &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name: "hpa-1",
				},
				Spec: v2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: v2.CrossVersionObjectReference{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
						Name:       "deployment-1",
					},
					MinReplicas: ptr.To(int32(1)),
					MaxReplicas: 1000,
					Metrics: []v2.MetricSpec{
						{
							Type: v2.ObjectMetricSourceType,
							Object: &v2.ObjectMetricSource{
								DescribedObject: v2.CrossVersionObjectReference{
									APIVersion: "zalando.org/v1",
									Kind:       "ClusterScalingSchedule",
									Name:       "schedule-1",
								},
								Target: v2.MetricTarget{
									Type:         v2.AverageValueMetricType,
									AverageValue: resource.NewQuantity(tc.targetValue, resource.DecimalSI),
								},
							},
						},
					},
				},
			}

			hpa, err = kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").Create(context.Background(), hpa, metav1.CreateOptions{})
			require.NoError(t, err)

			hpa.Status.CurrentReplicas = tc.currentReplicas
			_, err = kubeClient.AutoscalingV2().HorizontalPodAutoscalers("default").UpdateStatus(context.Background(), hpa, metav1.UpdateOptions{})
			require.NoError(t, err)

			err = controller.adjustScaling(context.Background(), clusterScalingSchedules)
			require.NoError(t, err)

			deployment, err = kubeClient.AppsV1().Deployments("default").Get(context.Background(), "deployment-1", metav1.GetOptions{})
			require.NoError(t, err)

			require.Equal(t, tc.desiredReplicas, ptr.Deref(deployment.Spec.Replicas, 0))
		})
	}
}
