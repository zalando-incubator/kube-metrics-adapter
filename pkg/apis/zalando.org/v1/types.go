package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// ScalingSchedule describes a namespaced time based metric to be used
// in autoscaling operations.
// +k8s:deepcopy-gen=true
type ScalingSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ScalingScheduleSpec `json:"spec"`
}

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// ClusterScalingSchedule describes a cluster scoped time based metric
// to be used in autoscaling operations.
// +k8s:deepcopy-gen=true
// +kubebuilder:resource:scope=Cluster
type ClusterScalingSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ScalingScheduleSpec `json:"spec"`
}

// ScalingScheduleSpec is the spec part of the ScalingSchedule.
// +k8s:deepcopy-gen=true
type ScalingScheduleSpec struct {
	// Schedules is the list of schedules for this ScalingSchedule
	// resource. All the schedules defined here will result on the value
	// to the same metric. New metrics require a new ScalingSchedule
	// resource.
	Schedules []Schedule `json:"schedules"`
}

// Schedule is the schedule details to be used inside a ScalingSchedule.
// +k8s:deepcopy-gen=true
type Schedule struct {
	Type ScheduleType `json:"type"`
	// Defines the details of a Repeating schedule.
	// +optional
	Period *SchedulePeriod `json:"period,omitempty"`
	// Defines the starting date of a OneTime schedule. It has to
	// be a RFC3339 formated date.
	// +optional
	Date *ScheduleDate `json:"date,omitempty"`
	// The duration in minutes that the configured value will be
	// returned for the defined schedule.
	DurationMinutes int `json:"durationMinutes"`
	// The metric value that will be returned for the defined schedule.
	Value int `json:"value"`
}

// Defines if the schedule is a OneTime schedule or
// Repeating one. If OneTime, date has to be defined. If Repeating,
// Period has to be defined.
// +kubebuilder:validation:Enum=OneTime;Repeating
type ScheduleType string

const (
	OneTimeSchedule   ScheduleType = "OneTime"
	RepeatingSchedule ScheduleType = "Repeating"
)

// SchedulePeriod is the details to be used for a Schedule of the
// Repeating type.
// +k8s:deepcopy-gen=true
type SchedulePeriod struct {
	// The startTime has the format HH:MM
	// +kubebuilder:validation:Pattern="(([0-1][0-9])|([2][0-3])):([0-5][0-9])"
	StartTime string `json:"startTime"`
	// The days that this schedule will be active.
	Days []ScheduleDay `json:"days"`
	// The location name corresponding to a file in the IANA
	// Time Zone database, like Europe/Berlin.
	Timezone string `json:"timezone"`
}

// ScheduleDay represents the valid inputs for days in a SchedulePeriod.
// +kubebuilder:validation:Enum=Sun;Mon;Tue;Wed;Thu;Fri;Sat
type ScheduleDay string

const (
	SundaySchedule    ScheduleDay = "Sun"
	MondaySchedule    ScheduleDay = "Mon"
	TuesdaySchedule   ScheduleDay = "Tue"
	WednesdaySchedule ScheduleDay = "Wed"
	ThursdaySchedule  ScheduleDay = "Thu"
	FridaySchedule    ScheduleDay = "Fri"
	SaturdaySchedule  ScheduleDay = "Sat"
)

// ScheduleDate is a RFC3339 representation of the date for a Schedule
// of the OneTime type.
// +kubebuilder:validation:Format="date-time"
type ScheduleDate string

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ScalingScheduleList is a list of namespaced scaling schedules.
// +k8s:deepcopy-gen=true
type ScalingScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ScalingSchedule `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ScalingScheduleList is a list of cluster scoped scaling schedules.
// +k8s:deepcopy-gen=true
type ClusterScalingScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []ScalingSchedule `json:"items"`
}
