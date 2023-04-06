package scheduledscaling

import (
	"errors"
	"fmt"
	"time"

	v1 "github.com/zalando-incubator/kube-metrics-adapter/pkg/apis/zalando.org/v1"
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
