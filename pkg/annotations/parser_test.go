package annotations

import (
	"testing"

	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2beta2"
)

func TestParser(t *testing.T) {
	for _, tc := range []struct {
		Name           string
		Annotations    map[string]string
		MetricName     string
		MetricType     autoscalingv2.MetricSourceType
		ExpectedConfig map[string]string
		PerReplica     bool
	}{
		{
			Name:           "no annotations",
			Annotations:    map[string]string{},
			ExpectedConfig: map[string]string{},
		},
		{
			Name: "pod metrics",
			Annotations: map[string]string{
				"metric-config.pods.requests-per-second.json-path/json-key":    "$.http_server.rps",
				"metric-config.pods.requests-per-second.json-path/path":        "/metrics",
				"metric-config.pods.requests-per-second.json-path/port":        "9090",
				"metric-config.pods.requests-per-second.json-path/scheme":      "https",
				"metric-config.pods.requests-per-second.json-path/min-pod-age": "30s",
			},
			MetricName: "requests-per-second",
			MetricType: autoscalingv2.PodsMetricSourceType,
			ExpectedConfig: map[string]string{
				"json-key": "$.http_server.rps",
				"path":     "/metrics",
				"port":     "9090",
				"scheme":   "https",
			},
		},
		{
			Name: "prometheus metrics",
			Annotations: map[string]string{
				"metric-config.object.processed-events-per-second.prometheus/query":       "scalar(sum(rate(event-service_events_count{application=\"event-service\",processed=\"true\"}[1m])))",
				"metric-config.object.processed-events-per-second.prometheus/per-replica": "true",
			},
			MetricName: "processed-events-per-second",
			MetricType: autoscalingv2.ObjectMetricSourceType,
			ExpectedConfig: map[string]string{
				"query": "scalar(sum(rate(event-service_events_count{application=\"event-service\",processed=\"true\"}[1m])))",
			},
			PerReplica: true,
		},
		{
			Name: "zmon collector",
			Annotations: map[string]string{
				"metric-config.external.zmon-check.zmon/key":             "custom.*",
				"metric-config.external.zmon-check.zmon/tag-application": "my-custom-app-*",
			},
			MetricName: "zmon-check",
			MetricType: autoscalingv2.ExternalMetricSourceType,
			ExpectedConfig: map[string]string{
				"key":             "custom.*",
				"tag-application": "my-custom-app-*",
			},
			PerReplica: false,
		},
		{
			Name: "influxdb metrics",
			Annotations: map[string]string{
				"metric-config.external.flux-query.influxdb/range1m": `from(bucket: "?") |> range(start: -1m)`,
				"metric-config.external.flux-query.influxdb/range2m": `from(bucket: "?") |> range(start: -2m)`,
				"metric-config.external.flux-query.influxdb/range3m": `from(bucket: "?") |> range(start: -3m)`,
				"metric-config.external.flux-query.influxdb/address": "http://localhost:9999",
				"metric-config.external.flux-query.influxdb/token":   "sEcr3TT0ken",
				"metric-config.external.flux-query.influxdb/org-id":  "deadbeef",
			},
			MetricName: "flux-query",
			MetricType: autoscalingv2.ExternalMetricSourceType,
			ExpectedConfig: map[string]string{
				"range1m": `from(bucket: "?") |> range(start: -1m)`,
				"range2m": `from(bucket: "?") |> range(start: -2m)`,
				"range3m": `from(bucket: "?") |> range(start: -3m)`,
				"address": "http://localhost:9999",
				"token":   "sEcr3TT0ken",
				"org-id":  "deadbeef",
			},
		},
		{
			Name: "http metrics",
			Annotations: map[string]string{
				"metric-config.external.http.json/json-key":   "$.metric.value",
				"metric-config.external.http.json/endpoint":   "http://metric-source.source-namespace.svc.cluster.local:8000/metrics",
				"metric-config.external.http.json/aggregator": "avg",
			},
			MetricName: "http",
			MetricType: autoscalingv2.ExternalMetricSourceType,
			ExpectedConfig: map[string]string{
				"json-key":   "$.metric.value",
				"endpoint":   "http://metric-source.source-namespace.svc.cluster.local:8000/metrics",
				"aggregator": "avg",
			},
		},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			hpaMap := make(AnnotationConfigMap)
			err := hpaMap.Parse(tc.Annotations)
			require.NoError(t, err)
			config, present := hpaMap.GetAnnotationConfig(tc.MetricName, tc.MetricType)
			if len(tc.ExpectedConfig) == 0 {
				require.False(t, present)
				return
			}
			require.True(t, present)
			for k, v := range tc.ExpectedConfig {
				require.Equal(t, v, config.Configs[k])
			}
			require.Equal(t, tc.PerReplica, config.PerReplica)
		})
	}
}
