package collector

import (
	"errors"
	"fmt"
	"time"

	v1 "github.com/zalando-incubator/kube-metrics-adapter/pkg/apis/zalando.org/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/custom_metrics"
)

const (
	// The format used by v1.SchedulePeriod.StartTime. 15:04 are
	// the defined reference time in time.Format.
	hourColonMinuteLayout = "15:04"
	// The default timezone used in v1.SchedulePeriod if none is
	// defined.
	// TODO(jonathanbeber): it should be configurable.
	defaultTimeZone = "Europe/Berlin"
)

var days = map[v1.ScheduleDay]time.Weekday{
	v1.SundaySchedule:    time.Sunday,
	v1.MondaySchedule:    time.Monday,
	v1.TuesdaySchedule:   time.Tuesday,
	v1.WednesdaySchedule: time.Wednesday,
	v1.ThursdaySchedule:  time.Thursday,
	v1.FridaySchedule:    time.Friday,
	v1.SaturdaySchedule:  time.Saturday,
}

var (
	// ErrScalingScheduleNotFound is returned when a item referenced in
	// the HPA config is not in the ScalingScheduleCollectorPlugin.store.
	ErrScalingScheduleNotFound = errors.New("referenced ScalingSchedule not found")
	// ErrNotScalingScheduleFound is returned when a item returned from
	// the ScalingScheduleCollectorPlugin.store was expected to
	// be an ScalingSchedule but the type assertion failed.
	ErrNotScalingScheduleFound = errors.New("error converting returned object to ScalingSchedule")
	// ErrClusterScalingScheduleNotFound is returned when a item referenced in
	// the HPA config is not in the ClusterScalingScheduleCollectorPlugin.store.
	ErrClusterScalingScheduleNotFound = errors.New("referenced ClusterScalingSchedule not found")
	// ErrNotClusterScalingScheduleFound is returned when a item returned from
	// the ClusterScalingScheduleCollectorPlugin.store was expected to
	// be an ClusterScalingSchedule but the type assertion failed. When
	// returned the type assertion to ScalingSchedule failed too.
	ErrNotClusterScalingScheduleFound = errors.New("error converting returned object to ClusterScalingSchedule")
	// ErrInvalidScheduleDate is returned when the v1.ScheduleDate is
	// not a valid RFC3339 date. It shouldn't happen since the
	// validation is done by the CRD.
	ErrInvalidScheduleDate = errors.New("could not parse the specified schedule date, format is not RFC3339")
	// ErrInvalidScheduleStartTime is returned when the
	// v1.SchedulePeriod.StartTime is not in the format specified by
	// hourColonMinuteLayout. It shouldn't happen since the validation
	// is done by the CRD.
	ErrInvalidScheduleStartTime = errors.New("could not parse the specified schedule period start time, format is not HH:MM")
)

// Now is the function that returns a time.Time object representing the
// current moment. Its main implementation is the time.Now func in the
// std lib. It's used mainly for test/mock purposes.
type Now func() time.Time

// Store represent an in memory Store for the [Cluster]ScalingSchedule
// objects. Its main implementation is the [cache.cache][0] struct
// returned by the [cache.NewStore][1] function. Here it's used mainly
// for tests/mock purposes.
//
// [1]: https://pkg.go.dev/k8s.io/client-go/tools/cache#NewStore
// [0]: https://github.com/kubernetes/client-go/blob/v0.21.1/tools/cache/Store.go#L132-L140
type Store interface {
	GetByKey(key string) (item interface{}, exists bool, err error)
}

// ScalingScheduleCollectorPlugin is a collector plugin for initializing metrics
// collectors for getting ScalingSchedule configured metrics.
type ScalingScheduleCollectorPlugin struct {
	store Store
	now   Now
}

// ClusterScalingScheduleCollectorPlugin is a collector plugin for initializing metrics
// collectors for getting ClusterScalingSchedule configured metrics.
type ClusterScalingScheduleCollectorPlugin struct {
	store Store
	now   Now
}

// NewScalingScheduleCollectorPlugin initializes a new ScalingScheduleCollectorPlugin.
func NewScalingScheduleCollectorPlugin(store Store, now Now) (*ScalingScheduleCollectorPlugin, error) {
	return &ScalingScheduleCollectorPlugin{
		store: store,
		now:   now,
	}, nil
}

// NewClusterScalingScheduleCollectorPlugin initializes a new ClusterScalingScheduleCollectorPlugin.
func NewClusterScalingScheduleCollectorPlugin(store Store, now Now) (*ClusterScalingScheduleCollectorPlugin, error) {
	return &ClusterScalingScheduleCollectorPlugin{
		store: store,
		now:   now,
	}, nil
}

// NewCollector initializes a new scaling schedule collector from the
// specified HPA. It's the only required method to implement the
// collector.CollectorPlugin interface.
func (c *ScalingScheduleCollectorPlugin) NewCollector(hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return NewScalingScheduleCollector(c.store, c.now, hpa, config, interval)
}

// NewCollector initializes a new cluster wide scaling schedule
// collector from the specified HPA. It's the only required method to
// implement the collector.CollectorPlugin interface.
func (c *ClusterScalingScheduleCollectorPlugin) NewCollector(hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (Collector, error) {
	return NewClusterScalingScheduleCollector(c.store, c.now, hpa, config, interval)
}

// ScalingScheduleCollector is a metrics collector for time based
// scaling metrics.
type ScalingScheduleCollector struct {
	scalingScheduleCollector
}

// ClusterScalingScheduleCollector is a metrics collector for time based
// scaling metrics.
type ClusterScalingScheduleCollector struct {
	scalingScheduleCollector
}

// scalingScheduleCollector is a representation of the internal data
// struct used by both ClusterScalingScheduleCollector and the
// ScalingScheduleCollector.
type scalingScheduleCollector struct {
	store           Store
	now             Now
	metric          autoscalingv2.MetricIdentifier
	objectReference custom_metrics.ObjectReference
	hpa             *autoscalingv2.HorizontalPodAutoscaler
	interval        time.Duration
	config          MetricConfig
}

// NewScalingScheduleCollector initializes a new ScalingScheduleCollector.
func NewScalingScheduleCollector(store Store, now Now, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (*ScalingScheduleCollector, error) {
	return &ScalingScheduleCollector{
		scalingScheduleCollector{
			store:           store,
			now:             now,
			objectReference: config.ObjectReference,
			hpa:             hpa,
			metric:          config.Metric,
			interval:        interval,
			config:          *config,
		},
	}, nil
}

// NewClusterScalingScheduleCollector initializes a new ScalingScheduleCollector.
func NewClusterScalingScheduleCollector(store Store, now Now, hpa *autoscalingv2.HorizontalPodAutoscaler, config *MetricConfig, interval time.Duration) (*ClusterScalingScheduleCollector, error) {
	return &ClusterScalingScheduleCollector{
		scalingScheduleCollector{
			store:           store,
			now:             now,
			objectReference: config.ObjectReference,
			hpa:             hpa,
			metric:          config.Metric,
			interval:        interval,
			config:          *config,
		},
	}, nil
}

// GetMetrics is the main implementation for collector.Collector interface
func (c *ScalingScheduleCollector) GetMetrics() ([]CollectedMetric, error) {
	scalingScheduleInterface, exists, err := c.store.GetByKey(fmt.Sprintf("%s/%s", c.objectReference.Namespace, c.objectReference.Name))
	if !exists {
		return nil, ErrScalingScheduleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("unexpected error retrieving the ScalingSchedule: %s", err.Error())
	}

	scalingSchedule, ok := scalingScheduleInterface.(*v1.ScalingSchedule)
	if !ok {
		return nil, ErrNotScalingScheduleFound
	}
	return calculateMetrics(scalingSchedule.Spec.Schedules, c.now(), c.objectReference, c.metric)
}

// GetMetrics is the main implementation for collector.Collector interface
func (c *ClusterScalingScheduleCollector) GetMetrics() ([]CollectedMetric, error) {
	clusterScalingScheduleInterface, exists, err := c.store.GetByKey(c.objectReference.Name)
	if !exists {
		return nil, ErrClusterScalingScheduleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("unexpected error retrieving the ClusterScalingSchedule: %s", err.Error())
	}

	// The [cache.Store][0] returns the v1.ClusterScalingSchedule items as
	// a v1.ScalingSchedule when it first lists it. Once the objects are
	// updated/patched it asserts it correctly to the
	// v1.ClusterScalingSchedule type. It means we have to handle both
	// cases.
	// TODO(jonathanbeber): Identify why it happens and fix in the upstream.
	//
	// [0]: https://github.com/kubernetes/client-go/blob/v0.21.1/tools/cache/Store.go#L132-L140
	var clusterScalingSchedule v1.ClusterScalingSchedule
	scalingSchedule, ok := clusterScalingScheduleInterface.(*v1.ScalingSchedule)
	if !ok {
		css, ok := clusterScalingScheduleInterface.(*v1.ClusterScalingSchedule)
		if !ok {
			return nil, ErrNotClusterScalingScheduleFound
		}
		clusterScalingSchedule = *css
	} else {
		clusterScalingSchedule = v1.ClusterScalingSchedule(*scalingSchedule)
	}

	return calculateMetrics(clusterScalingSchedule.Spec.Schedules, c.now(), c.objectReference, c.metric)
}

// Interval returns the interval at which the collector should run.
func (c *ScalingScheduleCollector) Interval() time.Duration {
	return c.interval
}

// Interval returns the interval at which the collector should run.
func (c *ClusterScalingScheduleCollector) Interval() time.Duration {
	return c.interval
}

func calculateMetrics(schedules []v1.Schedule, now time.Time, objectReference custom_metrics.ObjectReference, metric autoscalingv2.MetricIdentifier) ([]CollectedMetric, error) {
	value := 0
	for _, schedule := range schedules {
		switch schedule.Type {
		case v1.RepeatingSchedule:
			location, err := time.LoadLocation(schedule.Period.Timezone)
			if schedule.Period.Timezone == "" || err != nil {
				location, err = time.LoadLocation(defaultTimeZone)
				if err != nil {
					return nil, fmt.Errorf("unexpected error loading default location: %s", err.Error())
				}
			}
			nowInLocation := now.In(location)
			weekday := nowInLocation.Weekday()
			for _, day := range schedule.Period.Days {
				if days[day] == weekday {
					parsedStartTime, err := time.Parse(hourColonMinuteLayout, schedule.Period.StartTime)
					if err != nil {
						return nil, ErrInvalidScheduleStartTime
					}
					scheduledTime := time.Date(
						// v1.SchedulePeriod.StartTime can't define the
						// year, month or day, so we compute it as the
						// current date in the configured location.
						nowInLocation.Year(),
						nowInLocation.Month(),
						nowInLocation.Day(),
						// Hours and minute are configured in the
						// v1.SchedulePeriod.StartTime.
						parsedStartTime.Hour(),
						parsedStartTime.Minute(),
						parsedStartTime.Second(),
						parsedStartTime.Nanosecond(),
						location,
					)
					if within(now, scheduledTime, schedule.DurationMinutes) && schedule.Value > value {
						value = schedule.Value
					}
					break
				}
			}
		case v1.OneTimeSchedule:
			scheduledTime, err := time.Parse(time.RFC3339, string(*schedule.Date))
			if err != nil {
				return nil, ErrInvalidScheduleDate
			}
			if within(now, scheduledTime, schedule.DurationMinutes) && schedule.Value > value {
				value = schedule.Value
			}
		}
	}

	return []CollectedMetric{
		{
			Type:      autoscalingv2.ObjectMetricSourceType,
			Namespace: objectReference.Namespace,
			Custom: custom_metrics.MetricValue{
				DescribedObject: objectReference,
				Timestamp:       metav1.Time{Time: now},
				Value:           *resource.NewMilliQuantity(int64(value*1000), resource.DecimalSI),
				Metric:          custom_metrics.MetricIdentifier(metric),
			},
		},
	}, nil
}

// within receive two time.Time and a number of minutes. It returns true
// if the first given time, instant, is within the period of the second
// given time (start) plus the given number of minutes.
func within(instant, start time.Time, minutes int) bool {
	return (instant.After(start) || instant.Equal(start)) &&
		instant.Before(start.Add(time.Duration(minutes)*time.Minute))
}
