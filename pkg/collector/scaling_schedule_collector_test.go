package collector

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	v1 "github.com/zalando-incubator/kube-metrics-adapter/pkg/apis/zalando.org/v1"
	scheduledscaling "github.com/zalando-incubator/kube-metrics-adapter/pkg/controller/scheduledscaling"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	hHMMFormat                   = "15:04"
	defaultScalingWindowDuration = 1 * time.Minute
	defaultRampSteps             = 10
	defaultTimeZone              = "Europe/Berlin"
)

type schedule struct {
	kind      string
	date      string
	endDate   string
	startTime string
	endTime   string
	days      []v1.ScheduleDay
	timezone  string
	duration  int
	value     int64
}

func TestScalingScheduleCollector(t *testing.T) {
	nowRFC3339 := "2009-11-10T23:00:00+01:00" // +01:00 is Berlin timezone (default)
	nowTime, _ := time.Parse(time.RFC3339, nowRFC3339)
	nowWeekday := v1.TuesdaySchedule

	// now func always return in UTC for test purposes
	utc, _ := time.LoadLocation("UTC")
	uTCNow := nowTime.In(utc)
	uTCNowRFC3339 := uTCNow.Format(time.RFC3339)
	now := func() time.Time {
		return uTCNow
	}

	tenMinutes := int64(10)

	for _, tc := range []struct {
		msg                          string
		schedules                    []schedule
		scalingWindowDurationMinutes *int64
		expectedValue                int64
		err                          error
		rampSteps                    int
	}{
		{
			msg: "Return the right value for one time config",
			schedules: []schedule{
				{
					date:     nowRFC3339,
					kind:     "OneTime",
					duration: 15,
					value:    100,
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return the right value for one time config - ten minutes after starting a 15 minutes long schedule",
			schedules: []schedule{
				{
					date:     nowTime.Add(-time.Minute * 10).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 15,
					value:    100,
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return the right value - utilise end date instead of start date + duration for one time config",
			schedules: []schedule{
				{
					date:     nowTime.Add(-2 * time.Hour).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 60,
					endDate:  nowTime.Add(1 * time.Hour).Format(time.RFC3339),
					value:    100,
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return the right value - utilise start date + duration instead of end date for one time config",
			schedules: []schedule{
				{
					date:     nowTime.Add(-2 * time.Hour).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 150,
					endDate:  nowTime.Add(-1 * time.Hour).Format(time.RFC3339),
					value:    100,
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return the right value - use end date with no duration set for one time config",
			schedules: []schedule{
				{
					date:    nowTime.Add(-2 * time.Hour).Format(time.RFC3339),
					kind:    "OneTime",
					endDate: nowTime.Add(1 * time.Hour).Format(time.RFC3339),
					value:   100,
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return the right value (0) for one time config no duration or end date set",
			schedules: []schedule{
				{
					date:  nowTime.Add(time.Minute * 1).Format(time.RFC3339),
					kind:  "OneTime",
					value: 100,
				},
			},
			expectedValue: 0,
		},
		{
			msg: "Return the right value for one time config - 30 seconds before ending",
			schedules: []schedule{
				{
					date:     nowTime.Add(-time.Minute * 15).Add(time.Second * 30).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 15,
					value:    100,
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return the scaled value (60) for one time config - 20 seconds before starting",
			schedules: []schedule{
				{
					date:     nowTime.Add(time.Second * 20).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 45,
					value:    100,
				},
			},
			expectedValue: 60,
		},
		{
			msg: "Return the scaled value (60) for one time config - 20 seconds after",
			schedules: []schedule{
				{
					date:     nowTime.Add(-time.Minute * 45).Add(-time.Second * 20).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 45,
					value:    100,
				},
			},
			expectedValue: 60,
		},
		{
			msg: "10 steps (default) return 90% of the metric, even 1 second before",
			schedules: []schedule{
				{
					date:     nowTime.Add(time.Second * 1).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 45,
					value:    100,
				},
			},
			expectedValue: 90,
		},
		{
			msg: "5 steps return 80% of the metric, even 1 second before",
			schedules: []schedule{
				{
					date:     nowTime.Add(time.Second * 1).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 45,
					value:    100,
				},
			},
			expectedValue: 80,
			rampSteps:     5,
		},
		{
			msg:                          "Return the scaled value (90) for one time config with a custom scaling window - 30 seconds before starting",
			scalingWindowDurationMinutes: &tenMinutes,
			schedules: []schedule{
				{
					date:     nowTime.Add(time.Second * 30).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 45,
					value:    100,
				},
			},
			expectedValue: 90,
		},
		{
			msg:                          "Return the scaled value (90) for one time config with a custom scaling window - 30 seconds after",
			scalingWindowDurationMinutes: &tenMinutes,
			schedules: []schedule{
				{
					date:     nowTime.Add(-time.Minute * 45).Add(-time.Second * 30).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 45,
					value:    100,
				},
			},
			expectedValue: 90,
		},
		{
			msg: "Return the default value (0) for one time config not started yet (20 minutes before)",
			schedules: []schedule{
				{
					date:     nowTime.Add(time.Minute * 20).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 65,
					value:    100,
				},
			},
			expectedValue: 0,
		},
		{
			msg: "Return the default value (0) for one time config that ended (20 minutes after now for a 15 minutes long schedule)",
			schedules: []schedule{
				{
					date:     nowTime.Add(-time.Minute * 20).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 15,
					value:    100,
				},
			},
			expectedValue: 0,
		},
		{
			msg: "Return error for one time config not in RFC3339 format",
			schedules: []schedule{
				{
					date:     nowTime.Add(-time.Minute * 20).Format(time.RFC822),
					kind:     "OneTime",
					duration: 15,
					value:    100,
				},
			},
			err: scheduledscaling.ErrInvalidScheduleDate,
		},
		{
			msg: "Return error for one time config end date not in RFC3339 format",
			schedules: []schedule{
				{
					date:     nowTime.Add(-time.Minute * 20).Format(time.RFC3339),
					endDate:  nowTime.Add(1 * time.Hour).Format(time.RFC822),
					kind:     "OneTime",
					duration: 15,
					value:    100,
				},
			},
			err: scheduledscaling.ErrInvalidScheduleDate,
		},
		{
			msg: "Return the right value for two one time config",
			schedules: []schedule{
				{
					// 20 minutes after now for a 15 minutes long schedule
					date:     nowTime.Add(-time.Minute * 20).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 15,
					value:    100,
				},
				{
					// starting now
					date:     nowRFC3339,
					kind:     "OneTime",
					duration: 15,
					value:    100,
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return the biggest value for multiples one time configs (all starting now)",
			schedules: []schedule{
				{
					date:     nowRFC3339,
					kind:     "OneTime",
					duration: 15,
					value:    100,
				},
				{
					date:     nowRFC3339,
					kind:     "OneTime",
					duration: 15,
					value:    120,
				},
				{
					date:     nowRFC3339,
					kind:     "OneTime",
					duration: 15,
					value:    110,
				},
			},
			expectedValue: 120,
		},
		{
			msg: "Return the right value for a repeating schedule",
			schedules: []schedule{
				{
					kind:      "Repeating",
					duration:  15,
					value:     100,
					startTime: nowTime.Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return the right value - utilise end date instead of start time + duration for repeating schedule",
			schedules: []schedule{
				{
					kind:      "Repeating",
					duration:  60,
					value:     100,
					startTime: nowTime.Add(-2 * time.Hour).Format(hHMMFormat),
					// nowTime + 59m = 23:59.
					endTime: nowTime.Add(59 * time.Minute).Format(hHMMFormat),
					days:    []v1.ScheduleDay{nowWeekday},
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return the right value - utilise start time + duration instead of end time for repeating schedule",
			schedules: []schedule{
				{
					kind:      "Repeating",
					duration:  150,
					value:     100,
					startTime: nowTime.Add(-2 * time.Hour).Format(hHMMFormat),
					endTime:   nowTime.Add(-1 * time.Hour).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return the right value for a repeating schedule - 5 minutes after started",
			schedules: []schedule{
				{
					kind:      "Repeating",
					duration:  15,
					value:     100,
					startTime: nowTime.Add(-5 * time.Minute).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return the right value for a repeating schedule - 5 minutes before ending",
			schedules: []schedule{
				{
					kind:      "Repeating",
					duration:  15,
					value:     100,
					startTime: nowTime.Add(-10 * time.Minute).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return the default value (0) for a repeating schedule in the wrong day of the week",
			schedules: []schedule{
				{
					kind:      "Repeating",
					duration:  15,
					value:     100,
					startTime: nowTime.Format(hHMMFormat),
					days:      []v1.ScheduleDay{v1.MondaySchedule}, // Not nowWeekday
				},
			},
			expectedValue: 0,
		},
		{
			msg: "Return the right value for a repeating schedule in the right timezone",
			schedules: []schedule{
				{
					kind:     "Repeating",
					duration: 15,
					value:    100,
					// Sao Paulo is -3 hours from Berlin
					startTime: nowTime.Add(-time.Hour * 3).Format(hHMMFormat),
					timezone:  "America/Sao_Paulo",
					days:      []v1.ScheduleDay{nowWeekday},
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return an error if the start time is not in the format HH:MM",
			schedules: []schedule{
				{
					kind:      "Repeating",
					duration:  15,
					value:     100,
					startTime: "15h25min",
					days:      []v1.ScheduleDay{nowWeekday},
				},
			},
			err: scheduledscaling.ErrInvalidScheduleStartTime,
		},
		{
			msg: "Return the right value for a repeating schedule in the right timezone even in the day after it",
			schedules: []schedule{
				{
					kind:     "Repeating",
					duration: 15,
					value:    100,
					// Tokyo is +8 hours from Berlin
					startTime: nowTime.Add(time.Hour * 8).Format(hHMMFormat),
					timezone:  "Asia/Tokyo",
					// It's Wednesday in Japan
					days: []v1.ScheduleDay{v1.WednesdaySchedule},
				},
			},
			expectedValue: 100,
		},
		{
			msg: "Return default (0) for a repeating schedule in the right timezone in the day after it",
			schedules: []schedule{
				{
					kind:     "Repeating",
					duration: 15,
					value:    100,
					// Tokyo is +8 hours from Berlin
					startTime: nowTime.Add(time.Hour * 8).Format(hHMMFormat),
					timezone:  "Asia/Tokyo",
					// It's Wednesday in Japan
					days: []v1.ScheduleDay{nowWeekday},
				},
			},
			expectedValue: 0,
		},
		{
			msg: "Return the default value (0) for a repeating schedule in the right day of the week but five minutes too early",
			schedules: []schedule{
				{
					kind:      "Repeating",
					duration:  15,
					value:     100,
					startTime: nowTime.Add(5 * time.Minute).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
			},
			expectedValue: 0,
		},
		{
			msg: "Return the default value (0) for a repeating schedule in the right day of the week but five minutes too late (schedule started 20 minutes ago and lasted 15.)",
			schedules: []schedule{
				{
					kind:      "Repeating",
					duration:  15,
					value:     100,
					startTime: nowTime.Add(-20 * time.Minute).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
			},
			expectedValue: 0,
		},
		{
			msg: "Return the biggest value for multiple repeating schedules",
			schedules: []schedule{
				{
					// in time schedule - 30 minutes before
					kind:      "Repeating",
					duration:  60,
					value:     110,
					startTime: nowTime.Add(-30 * time.Minute).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
				{
					// in time schedule - 1 minute before ending
					// biggest value
					kind:      "Repeating",
					duration:  6,
					value:     120,
					startTime: nowTime.Add(-5 * time.Minute).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
				{
					// in time schedule - 10 minute after starting
					kind:      "Repeating",
					duration:  35,
					value:     100,
					startTime: nowTime.Add(-10 * time.Minute).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
			},
			expectedValue: 120,
		},
		{
			msg: "Return the biggest value for multiple repeating schedules - 1 minute too late for 120",
			schedules: []schedule{
				{
					// in time schedule - 30 minutes before
					// biggest value
					kind:      "Repeating",
					duration:  60,
					value:     110,
					startTime: nowTime.Add(-30 * time.Minute).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
				{
					// not in time schedule - 1 minute too late
					kind:      "Repeating",
					duration:  4,
					value:     120,
					startTime: nowTime.Add(-5 * time.Minute).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
				{
					// in time schedule - 10 minute after starting
					kind:      "Repeating",
					duration:  35,
					value:     100,
					startTime: nowTime.Add(-10 * time.Minute).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
			},
			expectedValue: 110,
		},
		{
			msg: "Return the biggest value for multiple repeating and oneTime configs",
			schedules: []schedule{
				{
					// in time schedule - 30 minutes before
					// biggest value
					kind:      "Repeating",
					duration:  60,
					value:     110,
					startTime: nowTime.Add(-30 * time.Minute).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
				{
					// not in time schedule - 1 minute too late
					kind:      "Repeating",
					duration:  4,
					value:     130,
					startTime: nowTime.Add(-5 * time.Minute).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
				{
					// in time schedule - 10 minutes after starting
					kind:      "Repeating",
					duration:  35,
					value:     100,
					startTime: nowTime.Add(-10 * time.Minute).Format(hHMMFormat),
					days:      []v1.ScheduleDay{nowWeekday},
				},
				{
					// in time schedule
					date:     nowRFC3339,
					kind:     "OneTime",
					duration: 15,
					value:    90,
				},
				{
					// invalid schedule - start tomorrow
					date:     nowTime.Add(24 * time.Hour).Format(time.RFC3339),
					kind:     "OneTime",
					duration: 15,
					value:    140,
				},
				{
					date:     nowRFC3339,
					kind:     "OneTime",
					duration: 15,
					value:    110,
				},
			},
			expectedValue: 110,
		},
	} {
		t.Run(tc.msg, func(t *testing.T) {
			scalingScheduleName := "my_scaling_schedule"
			namespace := "default"

			rampSteps := tc.rampSteps
			if rampSteps == 0 {
				rampSteps = defaultRampSteps
			}

			schedules := getSchedules(tc.schedules)
			store := newMockStore(scalingScheduleName, namespace, tc.scalingWindowDurationMinutes, schedules)
			plugin, err := NewScalingScheduleCollectorPlugin(store, now, defaultScalingWindowDuration, defaultTimeZone, rampSteps)
			require.NoError(t, err)

			clusterStore := newClusterMockStore(scalingScheduleName, tc.scalingWindowDurationMinutes, schedules)
			clusterPlugin, err := NewClusterScalingScheduleCollectorPlugin(clusterStore, now, defaultScalingWindowDuration, defaultTimeZone, rampSteps)
			require.NoError(t, err)

			clusterStoreFirstRun := newClusterMockStoreFirstRun(scalingScheduleName, tc.scalingWindowDurationMinutes, schedules)
			clusterPluginFirstRun, err := NewClusterScalingScheduleCollectorPlugin(clusterStoreFirstRun, now, defaultScalingWindowDuration, defaultTimeZone, rampSteps)
			require.NoError(t, err)

			hpa := makeScalingScheduleHPA(namespace, scalingScheduleName)

			configs, err := ParseHPAMetrics(hpa)
			require.NoError(t, err)
			require.Len(t, configs, 2)

			collectorFactory := NewCollectorFactory()
			err = collectorFactory.RegisterObjectCollector("ScalingSchedule", "", plugin)
			require.NoError(t, err)
			err = collectorFactory.RegisterObjectCollector("ClusterScalingSchedule", "", clusterPlugin)
			require.NoError(t, err)
			collectorFactoryFirstRun := NewCollectorFactory()
			err = collectorFactoryFirstRun.RegisterObjectCollector("ClusterScalingSchedule", "", clusterPluginFirstRun)
			require.NoError(t, err)

			collector, err := collectorFactory.NewCollector(hpa, configs[0], 0)
			require.NoError(t, err)
			collector, ok := collector.(*ScalingScheduleCollector)
			require.True(t, ok)

			clusterCollector, err := collectorFactory.NewCollector(hpa, configs[1], 0)
			require.NoError(t, err)
			clusterCollector, ok = clusterCollector.(*ClusterScalingScheduleCollector)
			require.True(t, ok)

			clusterCollectorFirstRun, err := collectorFactoryFirstRun.NewCollector(hpa, configs[1], 0)
			require.NoError(t, err)
			clusterCollectorFirstRun, ok = clusterCollectorFirstRun.(*ClusterScalingScheduleCollector)
			require.True(t, ok)

			checkCollectedMetrics := func(t *testing.T, collected []CollectedMetric, resourceType string) {
				if tc.err != nil {
					require.Equal(t, tc.err, err)
				} else {
					require.NoError(t, err, "failed to collect %s metrics: %v", resourceType, err)
					require.Len(t, collected, 1, "the number of metrics returned is not 1")
					require.EqualValues(t, tc.expectedValue, collected[0].Custom.Value.Value(), "the returned metric is not expected value")
					require.EqualValues(t, autoscalingv2.ObjectMetricSourceType, collected[0].Type)
					require.EqualValues(t, scalingScheduleName, collected[0].Custom.DescribedObject.Name)
					require.EqualValues(t, namespace, collected[0].Custom.DescribedObject.Namespace)
					require.EqualValues(t, "zalando.org/v1", collected[0].Custom.DescribedObject.APIVersion)
					require.EqualValues(t, resourceType, collected[0].Custom.DescribedObject.Kind)
					require.EqualValues(t, uTCNowRFC3339, collected[0].Custom.Timestamp.Time.Format(time.RFC3339))
					require.EqualValues(t, collected[0].Custom.Metric.Name, scalingScheduleName)
					require.EqualValues(t, namespace, collected[0].Namespace)
				}
			}

			collected, err := collector.GetMetrics()
			checkCollectedMetrics(t, collected, "ScalingSchedule")

			clusterCollected, err := clusterCollector.GetMetrics()
			checkCollectedMetrics(t, clusterCollected, "ClusterScalingSchedule")

			clusterCollectedFirstRun, err := clusterCollectorFirstRun.GetMetrics()
			checkCollectedMetrics(t, clusterCollectedFirstRun, "ClusterScalingSchedule")
		})
	}
}

func TestScalingScheduleObjectNotPresentReturnsError(t *testing.T) {
	store := mockStore{
		make(map[string]interface{}),
		getByKeyFn,
	}
	plugin, err := NewScalingScheduleCollectorPlugin(store, time.Now, defaultScalingWindowDuration, defaultTimeZone, defaultRampSteps)
	require.NoError(t, err)

	clusterStore := mockStore{
		make(map[string]interface{}),
		getByKeyFn,
	}
	clusterPlugin, err := NewClusterScalingScheduleCollectorPlugin(clusterStore, time.Now, defaultScalingWindowDuration, defaultTimeZone, defaultRampSteps)
	require.NoError(t, err)

	hpa := makeScalingScheduleHPA("namespace", "scalingScheduleName")

	configs, err := ParseHPAMetrics(hpa)
	require.NoError(t, err)
	require.Len(t, configs, 2)

	collectorFactory := NewCollectorFactory()
	err = collectorFactory.RegisterObjectCollector("ScalingSchedule", "", plugin)
	require.NoError(t, err)
	err = collectorFactory.RegisterObjectCollector("ClusterScalingSchedule", "", clusterPlugin)
	require.NoError(t, err)

	collector, err := collectorFactory.NewCollector(hpa, configs[0], 0)
	require.NoError(t, err)
	collector, ok := collector.(*ScalingScheduleCollector)
	require.True(t, ok)

	clusterCollector, err := collectorFactory.NewCollector(hpa, configs[1], 0)
	require.NoError(t, err)
	clusterCollector, ok = clusterCollector.(*ClusterScalingScheduleCollector)
	require.True(t, ok)

	_, err = collector.GetMetrics()
	require.Error(t, err)
	require.Equal(t, ErrScalingScheduleNotFound, err)

	_, err = clusterCollector.GetMetrics()
	require.Error(t, err)
	require.Equal(t, ErrClusterScalingScheduleNotFound, err)

	var invalidObject struct{}

	store.d["namespace/scalingScheduleName"] = invalidObject
	clusterStore.d["scalingScheduleName"] = invalidObject

	_, err = collector.GetMetrics()
	require.Error(t, err)
	require.Equal(t, ErrNotScalingScheduleFound, err)

	_, err = clusterCollector.GetMetrics()
	require.Error(t, err)
	require.Equal(t, ErrNotClusterScalingScheduleFound, err)
}

func TestReturnsErrorWhenStoreDoes(t *testing.T) {
	store := mockStore{
		make(map[string]interface{}),
		func(d map[string]interface{}, key string) (item interface{}, exists bool, err error) {
			return nil, true, errors.New("Unexpected store error")
		},
	}

	plugin, err := NewScalingScheduleCollectorPlugin(store, time.Now, defaultScalingWindowDuration, defaultTimeZone, defaultRampSteps)
	require.NoError(t, err)

	clusterPlugin, err := NewClusterScalingScheduleCollectorPlugin(store, time.Now, defaultScalingWindowDuration, defaultTimeZone, defaultRampSteps)
	require.NoError(t, err)

	hpa := makeScalingScheduleHPA("namespace", "scalingScheduleName")
	configs, err := ParseHPAMetrics(hpa)
	require.NoError(t, err)
	require.Len(t, configs, 2)

	collectorFactory := NewCollectorFactory()
	err = collectorFactory.RegisterObjectCollector("ScalingSchedule", "", plugin)
	require.NoError(t, err)
	err = collectorFactory.RegisterObjectCollector("ClusterScalingSchedule", "", clusterPlugin)
	require.NoError(t, err)

	collector, err := collectorFactory.NewCollector(hpa, configs[0], 0)
	require.NoError(t, err)
	collector, ok := collector.(*ScalingScheduleCollector)
	require.True(t, ok)

	clusterCollector, err := collectorFactory.NewCollector(hpa, configs[1], 0)
	require.NoError(t, err)
	clusterCollector, ok = clusterCollector.(*ClusterScalingScheduleCollector)
	require.True(t, ok)

	_, err = collector.GetMetrics()
	require.Error(t, err)

	_, err = clusterCollector.GetMetrics()
	require.Error(t, err)
}

type mockStore struct {
	d          map[string]interface{}
	getByKeyFn func(d map[string]interface{}, key string) (item interface{}, exists bool, err error)
}

func (m mockStore) GetByKey(key string) (item interface{}, exists bool, err error) {
	return m.getByKeyFn(m.d, key)
}

func getByKeyFn(d map[string]interface{}, key string) (item interface{}, exists bool, err error) {
	item, exists = d[key]
	return item, exists, nil
}

func newMockStore(name, namespace string, scalingWindowDurationMinutes *int64, schedules []v1.Schedule) mockStore {
	return mockStore{
		map[string]interface{}{
			fmt.Sprintf("%s/%s", namespace, name): &v1.ScalingSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: v1.ScalingScheduleSpec{
					ScalingWindowDurationMinutes: scalingWindowDurationMinutes,
					Schedules:                    schedules,
				},
			},
		},
		getByKeyFn,
	}
}

func newClusterMockStore(name string, scalingWindowDurationMinutes *int64, schedules []v1.Schedule) mockStore {
	return mockStore{
		map[string]interface{}{
			name: &v1.ClusterScalingSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: v1.ScalingScheduleSpec{
					ScalingWindowDurationMinutes: scalingWindowDurationMinutes,
					Schedules:                    schedules,
				},
			},
		},
		getByKeyFn,
	}
}

// The cache.Store returns the v1.ClusterScalingSchedule items as
// v1.ScalingSchedule when it first lists it. When it's update it
// asserts it correctly to the v1.ClusterScalingSchedule type.
func newClusterMockStoreFirstRun(name string, scalingWindowDurationMinutes *int64, schedules []v1.Schedule) mockStore {
	return mockStore{
		map[string]interface{}{
			name: &v1.ScalingSchedule{
				ObjectMeta: metav1.ObjectMeta{
					Name: name,
				},
				Spec: v1.ScalingScheduleSpec{
					ScalingWindowDurationMinutes: scalingWindowDurationMinutes,
					Schedules:                    schedules,
				},
			},
		},
		getByKeyFn,
	}
}

func getSchedules(schedules []schedule) (result []v1.Schedule) {
	for _, schedule := range schedules {
		switch schedule.kind {
		case string(v1.OneTimeSchedule):
			date := v1.ScheduleDate(schedule.date)
			endDate := v1.ScheduleDate(schedule.endDate)
			result = append(result,
				v1.Schedule{
					Type:            v1.OneTimeSchedule,
					Date:            &date,
					EndDate:         &endDate,
					DurationMinutes: schedule.duration,
					Value:           schedule.value,
				},
			)
		case string(v1.RepeatingSchedule):
			period := v1.SchedulePeriod{
				StartTime: schedule.startTime,
				EndTime:   schedule.endTime,
				Days:      schedule.days,
				Timezone:  schedule.timezone,
			}
			result = append(result,
				v1.Schedule{
					Type:            v1.RepeatingSchedule,
					Period:          &period,
					DurationMinutes: schedule.duration,
					Value:           schedule.value,
				},
			)
		}
	}
	return
}

func makeScalingScheduleHPA(namespace, scalingScheduleName string) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				Kind: "Deployment",
				Name: "Application",
			},
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ObjectMetricSourceType,
					Object: &autoscalingv2.ObjectMetricSource{
						DescribedObject: autoscalingv2.CrossVersionObjectReference{
							Name:       scalingScheduleName,
							APIVersion: "zalando.org/v1",
							Kind:       "ScalingSchedule",
						},
						Metric: autoscalingv2.MetricIdentifier{
							Name: scalingScheduleName,
						},
					},
				}, {
					Type: autoscalingv2.ObjectMetricSourceType,
					Object: &autoscalingv2.ObjectMetricSource{
						DescribedObject: autoscalingv2.CrossVersionObjectReference{
							Name:       scalingScheduleName,
							APIVersion: "zalando.org/v1",
							Kind:       "ClusterScalingSchedule",
						},
						Metric: autoscalingv2.MetricIdentifier{
							Name: scalingScheduleName,
						},
					},
				},
			},
		},
	}
}
