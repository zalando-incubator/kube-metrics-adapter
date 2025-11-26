package scheduledscaling

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	log "github.com/sirupsen/logrus"
	v1 "github.com/zalando-incubator/kube-metrics-adapter/pkg/apis/zalando.org/v1"
	zalandov1 "github.com/zalando-incubator/kube-metrics-adapter/pkg/client/clientset/versioned/typed/zalando.org/v1"
	"github.com/zalando-incubator/kube-metrics-adapter/pkg/recorder"
	"golang.org/x/sync/errgroup"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	kube_record "k8s.io/client-go/tools/record"
)

const (
	// The format used by v1.SchedulePeriod.StartTime. 15:04 are
	// the defined reference time in time.Format.
	hourColonMinuteLayout = "15:04"
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
	// ErrNotScalingScheduleFound is returned when a item returned from
	// the ScalingScheduleCollectorPlugin.store was expected to
	// be an ScalingSchedule but the type assertion failed.
	ErrNotScalingScheduleFound = errors.New("error converting returned object to ScalingSchedule")
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
type now func() time.Time

type scalingScheduleStore interface {
	List() []interface{}
}

type Controller struct {
	client                      zalandov1.ZalandoV1Interface
	kubeClient                  kubernetes.Interface
	scaler                      TargetScaler
	recorder                    kube_record.EventRecorder
	scalingScheduleStore        scalingScheduleStore
	clusterScalingScheduleStore scalingScheduleStore
	now                         now
	defaultScalingWindow        time.Duration
	defaultTimeZone             string
	hpaTolerance                float64
}

func NewController(zclient zalandov1.ZalandoV1Interface, kubeClient kubernetes.Interface, scaler TargetScaler, scalingScheduleStore, clusterScalingScheduleStore scalingScheduleStore, now now, defaultScalingWindow time.Duration, defaultTimeZone string, hpaThreshold float64) *Controller {
	return &Controller{
		client:                      zclient,
		kubeClient:                  kubeClient,
		scaler:                      scaler,
		recorder:                    recorder.CreateEventRecorder(kubeClient),
		scalingScheduleStore:        scalingScheduleStore,
		clusterScalingScheduleStore: clusterScalingScheduleStore,
		now:                         now,
		defaultScalingWindow:        defaultScalingWindow,
		defaultTimeZone:             defaultTimeZone,
		hpaTolerance:                hpaThreshold,
	}
}

func (c *Controller) Run(ctx context.Context) {
	log.Info("Running Scaling Schedule Controller")

	ticker := time.NewTicker(time.Second * 10)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			err := c.runOnce(ctx)
			if err != nil {
				log.Errorf("failed to run scheduled scaling controller loop: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (c *Controller) updateStatus(ctx context.Context, schedules []*v1.ScalingSchedule, clusterschedules []*v1.ClusterScalingSchedule) error {
	// ScalingSchedules
	var scalingGroup errgroup.Group
	scalingGroup.SetLimit(10)

	for _, schedule := range schedules {
		schedule = schedule.DeepCopy()

		scalingGroup.Go(func() error {
			activeSchedules, err := c.activeSchedules(schedule.Spec)
			if err != nil {
				log.Errorf("Failed to check for active schedules in ScalingSchedule %s/%s: %v", schedule.Namespace, schedule.Name, err)
				return nil
			}

			active := len(activeSchedules) > 0

			if active != schedule.Status.Active {
				schedule.Status.Active = active
				_, err := c.client.ScalingSchedules(schedule.Namespace).UpdateStatus(ctx, schedule, metav1.UpdateOptions{})
				if err != nil {
					log.Errorf("Failed to update status for ScalingSchedule %s/%s: %v", schedule.Namespace, schedule.Name, err)
					return nil
				}

				status := "inactive"
				if active {
					status = "active"
				}

				log.Infof("Marked Scaling Schedule %s/%s as %s", schedule.Namespace, schedule.Name, status)
			}
			return nil
		})
	}

	err := scalingGroup.Wait()
	if err != nil {
		return fmt.Errorf("failed waiting for cluster scaling schedules: %w", err)
	}

	// ClusterScalingSchedules
	var clusterScalingGroup errgroup.Group
	clusterScalingGroup.SetLimit(10)

	for _, schedule := range clusterschedules {
		schedule = schedule.DeepCopy()

		clusterScalingGroup.Go(func() error {
			activeSchedules, err := c.activeSchedules(schedule.Spec)
			if err != nil {
				log.Errorf("Failed to check for active schedules in ClusterScalingSchedule %s: %v", schedule.Name, err)
				return nil
			}

			active := len(activeSchedules) > 0

			if active != schedule.Status.Active {
				schedule.Status.Active = active
				_, err := c.client.ClusterScalingSchedules().UpdateStatus(ctx, schedule, metav1.UpdateOptions{})
				if err != nil {
					log.Errorf("Failed to update status for ClusterScalingSchedule %s: %v", schedule.Name, err)
					return nil
				}

				status := "inactive"
				if active {
					status = "active"
				}

				log.Infof("Marked Cluster Scaling Schedule %s as %s", schedule.Name, status)
			}
			return nil
		})
	}

	err = clusterScalingGroup.Wait()
	if err != nil {
		return fmt.Errorf("failed waiting for cluster scaling schedules: %w", err)
	}

	return nil
}

func (c *Controller) runOnce(ctx context.Context) error {
	schedulesInterface := c.scalingScheduleStore.List()
	namespacedSchedules := make([]*v1.ScalingSchedule, 0, len(schedulesInterface))
	schedules := make([]v1.ScalingScheduler, 0)
	for _, scheduleInterface := range schedulesInterface {
		schedule, ok := scheduleInterface.(*v1.ScalingSchedule)
		if !ok {
			return ErrNotScalingScheduleFound
		}
		namespacedSchedules = append(namespacedSchedules, schedule)
		schedules = append(schedules, schedule)
	}

	clusterschedulesInterface := c.clusterScalingScheduleStore.List()
	clusterschedules := make([]*v1.ClusterScalingSchedule, 0, len(clusterschedulesInterface))
	for _, scheduleInterface := range clusterschedulesInterface {
		schedule, ok := scheduleInterface.(*v1.ClusterScalingSchedule)
		if !ok {
			return ErrNotScalingScheduleFound
		}
		clusterschedules = append(clusterschedules, schedule)
		schedules = append(schedules, schedule)
	}

	err := c.updateStatus(ctx, namespacedSchedules, clusterschedules)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	log.Info("Adjusting scaling")
	err = c.adjustScaling(ctx, schedules)
	if err != nil {
		return fmt.Errorf("failed to adjust scaling: %w", err)
	}

	return nil
}

// activeScheduledScaling returns a map of the current active schedules and the
// max value per schedule.
func (c *Controller) activeScheduledScaling(schedules []v1.ScalingScheduler) map[string]int64 {
	currentActiveSchedules := make(map[string]int64)

	for _, schedule := range schedules {
		activeSchedules, err := c.activeSchedules(schedule.ResourceSpec())
		if err != nil {
			log.Errorf("Failed to check for active schedules in ScalingSchedule %s: %v", schedule.Identifier(), err)
			continue
		}

		if len(activeSchedules) == 0 {
			continue
		}

		maxValue := int64(0)
		for _, activeSchedule := range activeSchedules {
			if activeSchedule.Value > maxValue {
				maxValue = activeSchedule.Value
			}
		}
		currentActiveSchedules[schedule.Identifier()] = maxValue
	}

	return currentActiveSchedules
}

// adjustHPAScaling adjusts the scaling for a single HPA based on the active
// scaling schedules. An adjustment is made if the current HPA scale is below
// the desired and the change is within the HPA tolerance.
func (c *Controller) adjustHPAScaling(ctx context.Context, hpa *autoscalingv2.HorizontalPodAutoscaler, activeSchedules map[string]int64) error {
	current := int64(hpa.Status.CurrentReplicas)
	if current == 0 {
		return nil
	}

	highestExpected, usageRatio, highestObject := highestActiveSchedule(hpa, activeSchedules, current)

	highestExpected = int64(math.Min(float64(highestExpected), float64(hpa.Spec.MaxReplicas)))

	var change float64
	if highestExpected > current {
		change = math.Abs(1.0 - usageRatio)
	}

	if change > 0 && change <= c.hpaTolerance {
		err := c.scaler.Scale(ctx, hpa, int32(highestExpected))
		if err != nil {
			reference := fmt.Sprintf("%s/%s/%s", hpa.Spec.ScaleTargetRef.Kind, hpa.Namespace, hpa.Spec.ScaleTargetRef.Name)
			log.Errorf("Failed to scale target %s for HPA %s/%s: %v", reference, hpa.Namespace, hpa.Name, err)
			return nil
		}

		scheduleRef := highestObject.Name
		if highestObject.Kind == "ScalingSchedule" {
			scheduleRef = hpa.Namespace + "/" + scheduleRef
		}

		c.recorder.Eventf(
			hpa,
			corev1.EventTypeNormal,
			"ScalingAdjusted",
			"Scaling schedule '%s' adjusted replicas %d -> %d based on metric: %s",
			highestObject.Kind,
			current,
			highestExpected,
			scheduleRef,
		)
	}
	return nil
}

// highestActiveSchedule returns the highest active schedule value and
// corresponding object.
func highestActiveSchedule(hpa *autoscalingv2.HorizontalPodAutoscaler, activeSchedules map[string]int64, currentReplicas int64) (int64, float64, autoscalingv2.CrossVersionObjectReference) {
	var highestExpected int64
	var usageRatio float64
	var highestObject autoscalingv2.CrossVersionObjectReference
	for _, metric := range hpa.Spec.Metrics {
		if metric.Type != autoscalingv2.ObjectMetricSourceType {
			continue
		}

		switch metric.Object.DescribedObject.Kind {
		case "ClusterScalingSchedule", "ScalingSchedule":
		default:
			continue
		}

		scheduleName := metric.Object.DescribedObject.Name

		if metric.Object.Target.AverageValue == nil {
			continue
		}

		target := int64(metric.Object.Target.AverageValue.MilliValue() / 1000)
		if target == 0 {
			continue
		}

		var value int64
		switch metric.Object.DescribedObject.Kind {
		case "ScalingSchedule":
			value = activeSchedules[hpa.Namespace+"/"+scheduleName]
		case "ClusterScalingSchedule":
			value = activeSchedules[scheduleName]
		}

		expected := int64(math.Ceil(float64(value) / float64(target)))
		if expected > highestExpected {
			highestExpected = expected
			usageRatio = float64(value) / (float64(target) * float64(currentReplicas))
			highestObject = metric.Object.DescribedObject
		}
	}

	return highestExpected, usageRatio, highestObject
}

func (c *Controller) adjustScaling(ctx context.Context, schedules []v1.ScalingScheduler) error {
	currentActiveSchedules := c.activeScheduledScaling(schedules)

	hpas, err := c.kubeClient.AutoscalingV2().HorizontalPodAutoscalers(metav1.NamespaceAll).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list HPAs: %w", err)
	}

	var hpaGroup errgroup.Group
	hpaGroup.SetLimit(10)

	for _, hpa := range hpas.Items {
		hpa := hpa.DeepCopy()

		hpaGroup.Go(func() error {
			return c.adjustHPAScaling(ctx, hpa, currentActiveSchedules)
		})
	}

	err = hpaGroup.Wait()
	if err != nil {
		return fmt.Errorf("failed to wait for handling of HPAs: %w", err)
	}

	return nil
}

func (c *Controller) activeSchedules(spec v1.ScalingScheduleSpec) ([]v1.Schedule, error) {
	scalingWindowDuration := c.defaultScalingWindow
	if spec.ScalingWindowDurationMinutes != nil {
		scalingWindowDuration = time.Duration(*spec.ScalingWindowDurationMinutes) * time.Minute
	}
	if scalingWindowDuration < 0 {
		return nil, fmt.Errorf("scaling window duration cannot be negative: %d", scalingWindowDuration)
	}

	activeSchedules := make([]v1.Schedule, 0, len(spec.Schedules))
	for _, schedule := range spec.Schedules {
		startTime, endTime, err := ScheduleStartEnd(c.now(), schedule, c.defaultTimeZone)
		if err != nil {
			return nil, err
		}

		scalingStart := startTime.Add(-scalingWindowDuration)
		scalingEnd := endTime.Add(scalingWindowDuration)

		if Between(c.now(), scalingStart, scalingEnd) {
			activeSchedules = append(activeSchedules, schedule)
		}
	}

	return activeSchedules, nil
}

func ScheduleStartEnd(now time.Time, schedule v1.Schedule, defaultTimeZone string) (time.Time, time.Time, error) {
	var startTime, endTime time.Time
	switch schedule.Type {
	case v1.RepeatingSchedule:
		location, err := time.LoadLocation(schedule.Period.Timezone)
		if schedule.Period.Timezone == "" || err != nil {
			location, err = time.LoadLocation(defaultTimeZone)
			if err != nil {
				return time.Time{}, time.Time{}, fmt.Errorf("unexpected error loading default location: %s", err.Error())
			}
		}
		nowInLocation := now.In(location)
		weekday := nowInLocation.Weekday()
		for _, day := range schedule.Period.Days {
			if days[day] == weekday {
				parsedStartTime, err := time.Parse(hourColonMinuteLayout, schedule.Period.StartTime)
				if err != nil {
					return time.Time{}, time.Time{}, ErrInvalidScheduleStartTime
				}
				startTime = time.Date(
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

				// If no end time was provided, set it to equal the start time
				if schedule.Period.EndTime == "" {
					endTime = startTime
				} else {
					parsedEndTime, err := time.Parse(hourColonMinuteLayout, schedule.Period.EndTime)
					if err != nil {
						return time.Time{}, time.Time{}, ErrInvalidScheduleDate
					}
					endTime = time.Date(
						// v1.SchedulePeriod.StartTime can't define the
						// year, month or day, so we compute it as the
						// current date in the configured location.
						nowInLocation.Year(),
						nowInLocation.Month(),
						nowInLocation.Day(),
						// Hours and minute are configured in the
						// v1.SchedulePeriod.StartTime.
						parsedEndTime.Hour(),
						parsedEndTime.Minute(),
						parsedEndTime.Second(),
						parsedEndTime.Nanosecond(),
						location,
					)

				}
			}
		}
	case v1.OneTimeSchedule:
		var err error
		startTime, err = time.Parse(time.RFC3339, string(*schedule.Date))
		if err != nil {
			return time.Time{}, time.Time{}, ErrInvalidScheduleDate
		}

		// If no end time was provided, set it to equal the start time
		if schedule.EndDate == nil || string(*schedule.EndDate) == "" {
			endTime = startTime
		} else {
			endTime, err = time.Parse(time.RFC3339, string(*schedule.EndDate))
			if err != nil {
				return time.Time{}, time.Time{}, ErrInvalidScheduleDate
			}
		}
	}

	// Use either the defined end time/date or the start time/date + the
	// duration, whichever is longer.
	if startTime.Add(schedule.Duration()).After(endTime) {
		endTime = startTime.Add(schedule.Duration())
	}

	return startTime, endTime, nil
}

func Between(timestamp, start, end time.Time) bool {
	if timestamp.Before(start) {
		return false
	}
	return timestamp.Before(end)
}
