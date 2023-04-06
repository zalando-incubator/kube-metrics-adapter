package scheduledscaling

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "github.com/zalando-incubator/kube-metrics-adapter/pkg/apis/zalando.org/v1"
	scalingschedulefake "github.com/zalando-incubator/kube-metrics-adapter/pkg/client/clientset/versioned/fake"
	zalandov1 "github.com/zalando-incubator/kube-metrics-adapter/pkg/client/clientset/versioned/typed/zalando.org/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

			controller := NewController(client.ZalandoV1(), scalingSchedulesStore, clusterScalingSchedulesStore, now, 0, "Europe/Berlin")

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
