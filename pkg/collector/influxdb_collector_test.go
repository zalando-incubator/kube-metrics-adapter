package collector

import (
	"context"
	"strings"
	"testing"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestInfluxDBCollector_New(t *testing.T) {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
		},
	}
	t.Run("simple", func(t *testing.T) {
		m := &MetricConfig{
			MetricTypeName: MetricTypeName{
				Type: autoscalingv2.ExternalMetricSourceType,
				Metric: autoscalingv2.MetricIdentifier{
					Name: "flux-query",
					// This is actually useless, because the selector should be flattened in Config when parsing.
					Selector: &v1.LabelSelector{
						MatchLabels: map[string]string{
							"query-name": "range2m",
						},
					},
				},
			},
			CollectorType: "influxdb",
			Config: map[string]string{
				"range1m":    `from(bucket: "?") |> range(start: -1m)`,
				"range2m":    `from(bucket: "?") |> range(start: -2m)`,
				"range3m":    `from(bucket: "?") |> range(start: -3m)`,
				"query-name": "range2m",
			},
		}
		c, err := NewInfluxDBCollector(context.Background(), hpa, "http://localhost:9999", "secret", "deadbeef", m, time.Second)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := c.org, "deadbeef"; want != got {
			t.Errorf("unexpected value -want/+got:\n\t-%s\n\t+%s", want, got)
		}
		if got, want := c.address, "http://localhost:9999"; want != got {
			t.Errorf("unexpected value -want/+got:\n\t-%s\n\t+%s", want, got)
		}
		if got, want := c.token, "secret"; want != got {
			t.Errorf("unexpected value -want/+got:\n\t-%s\n\t+%s", want, got)
		}
		if got, want := c.query, `from(bucket: "?") |> range(start: -2m)`; want != got {
			t.Errorf("unexpected value -want/+got:\n\t-%s\n\t+%s", want, got)
		}
	})
	t.Run("override params", func(t *testing.T) {
		m := &MetricConfig{
			MetricTypeName: MetricTypeName{
				Type: autoscalingv2.ExternalMetricSourceType,
				Metric: autoscalingv2.MetricIdentifier{
					Name: "flux-query",
					Selector: &v1.LabelSelector{
						MatchLabels: map[string]string{
							"query-name": "range2m",
						},
					},
				},
			},
			CollectorType: "influxdb",
			Config: map[string]string{
				"range1m":    `from(bucket: "?") |> range(start: -1m)`,
				"range2m":    `from(bucket: "?") |> range(start: -2m)`,
				"range3m":    `from(bucket: "?") |> range(start: -3m)`,
				"address":    "http://localhost:9999",
				"token":      "sEcr3TT0ken",
				"org":        "deadbeef1234",
				"query-name": "range3m",
			},
		}
		c, err := NewInfluxDBCollector(context.Background(), hpa, "http://localhost:8888", "secret", "deadbeef", m, time.Second)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got, want := c.org, "deadbeef1234"; want != got {
			t.Errorf("unexpected value -want/+got:\n\t-%s\n\t+%s", want, got)
		}
		if got, want := c.address, "http://localhost:9999"; want != got {
			t.Errorf("unexpected value -want/+got:\n\t-%s\n\t+%s", want, got)
		}
		if got, want := c.token, "sEcr3TT0ken"; want != got {
			t.Errorf("unexpected value -want/+got:\n\t-%s\n\t+%s", want, got)
		}
		if got, want := c.query, `from(bucket: "?") |> range(start: -3m)`; want != got {
			t.Errorf("unexpected value -want/+got:\n\t-%s\n\t+%s", want, got)
		}
	})
	// Errors.
	for _, tc := range []struct {
		name            string
		mTypeName       MetricTypeName
		config          map[string]string
		errorStartsWith string
	}{
		{
			name: "object metric",
			mTypeName: MetricTypeName{
				Type: autoscalingv2.ObjectMetricSourceType,
			},
			errorStartsWith: "InfluxDB does not support object",
		},
		{
			name: "no selector",
			mTypeName: MetricTypeName{
				Type: autoscalingv2.ExternalMetricSourceType,
				Metric: autoscalingv2.MetricIdentifier{
					Name: "flux-query",
				},
			},
			//  The selector should be flattened into the config by the parsing step, but it isn't.
			config: map[string]string{
				"range1m": `from(bucket: "?") |> range(start: -1m)`,
				"range2m": `from(bucket: "?") |> range(start: -2m)`,
				"range3m": `from(bucket: "?") |> range(start: -3m)`,
			},
			errorStartsWith: "selector for Flux query is not specified",
		},
		{
			name: "referencing non-existing query",
			mTypeName: MetricTypeName{
				Type: autoscalingv2.ExternalMetricSourceType,
				Metric: autoscalingv2.MetricIdentifier{
					Name: "flux-query",
				},
			},
			config: map[string]string{
				"range1m":    `from(bucket: "?") |> range(start: -1m)`,
				"range2m":    `from(bucket: "?") |> range(start: -2m)`,
				"range3m":    `from(bucket: "?") |> range(start: -3m)`,
				"query-name": "rangeXm",
			},
			errorStartsWith: "no Flux query defined for metric",
		},
	} {
		t.Run("error - "+tc.name, func(t *testing.T) {
			m := &MetricConfig{
				MetricTypeName: tc.mTypeName,
				CollectorType:  "influxdb",
				Config:         tc.config,
			}
			_, err := NewInfluxDBCollector(context.Background(), hpa, "http://localhost:9999", "secret", "deadbeef", m, time.Second)
			if err == nil {
				t.Fatal("expected error got none")
			}
			if want, got := tc.errorStartsWith, err.Error(); !strings.HasPrefix(got, want) {
				t.Fatalf("%s should start with %s", got, want)
			}
		})
	}
}
