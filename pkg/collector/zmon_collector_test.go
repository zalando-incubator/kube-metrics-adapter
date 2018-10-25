package collector

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zalando-incubator/kube-metrics-adapter/pkg/zmon"
	autoscalingv2beta1 "k8s.io/api/autoscaling/v2beta1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

type zmonMock struct {
	dataPoints []zmon.DataPoint
	entities   []zmon.Entity
}

func (m zmonMock) Query(checkID int, key string, tags map[string]string, aggregators []string, duration time.Duration) ([]zmon.DataPoint, error) {
	return m.dataPoints, nil
}

func TestZMONCollectorNewCollector(t *testing.T) {
	collectPlugin, _ := NewZMONCollectorPlugin(zmonMock{})

	config := &MetricConfig{
		MetricTypeName: MetricTypeName{
			Name: ZMONCheckMetric,
		},
		Labels: map[string]string{
			zmonCheckIDLabelKey:             "1234",
			zmonAggregatorsLabelKey:         "max",
			zmonTagPrefixLabelKey + "alias": "cluster_alias",
			zmonDurationLabelKey:            "5m",
			zmonKeyLabelKey:                 "key",
		},
	}

	hpa := &autoscalingv2beta1.HorizontalPodAutoscaler{}

	collector, err := collectPlugin.NewCollector(hpa, config, 1*time.Second)
	require.NoError(t, err)
	require.NotNil(t, collector)
	zmonCollector := collector.(*ZMONCollector)
	require.Equal(t, "key", zmonCollector.key)
	require.Equal(t, 1234, zmonCollector.checkID)
	require.Equal(t, 1*time.Second, zmonCollector.interval)
	require.Equal(t, 5*time.Minute, zmonCollector.duration)
	require.Equal(t, []string{"max"}, zmonCollector.aggregators)
	require.Equal(t, map[string]string{"alias": "cluster_alias"}, zmonCollector.tags)

	// check that annotations overwrites labels
	hpa.ObjectMeta = metav1.ObjectMeta{
		Annotations: map[string]string{
			zmonKeyAnnotationKey:                 "annotation_key",
			zmonTagPrefixAnnotationKey + "alias": "cluster_alias_annotation",
		},
	}
	collector, err = collectPlugin.NewCollector(hpa, config, 1*time.Second)
	require.NoError(t, err)
	require.NotNil(t, collector)
	zmonCollector = collector.(*ZMONCollector)
	require.Equal(t, "annotation_key", zmonCollector.key)
	require.Equal(t, map[string]string{"alias": "cluster_alias_annotation"}, zmonCollector.tags)

	// should fail if the metric name isn't ZMON
	config.Name = "non-zmon-check"
	_, err = collectPlugin.NewCollector(nil, config, 1*time.Second)
	require.Error(t, err)

	// should fail if the check id is not specified.
	delete(config.Labels, zmonCheckIDLabelKey)
	config.Name = ZMONCheckMetric
	_, err = collectPlugin.NewCollector(nil, config, 1*time.Second)
	require.Error(t, err)
}

func TestZMONCollectorGetMetrics(tt *testing.T) {
	config := &MetricConfig{
		MetricTypeName: MetricTypeName{
			Name: ZMONCheckMetric,
			Type: "foo",
		},
		Labels: map[string]string{
			zmonCheckIDLabelKey:             "1234",
			zmonAggregatorsLabelKey:         "max",
			zmonTagPrefixLabelKey + "alias": "cluster_alias",
			zmonDurationLabelKey:            "5m",
			zmonKeyLabelKey:                 "key",
		},
	}

	for _, ti := range []struct {
		msg              string
		dataPoints       []zmon.DataPoint
		collectedMetrics []CollectedMetric
	}{
		{
			msg: "test successfully getting metrics",
			dataPoints: []zmon.DataPoint{
				{
					Time:  time.Time{},
					Value: 1.0,
				},
			},
			collectedMetrics: []CollectedMetric{
				{
					Type: config.Type,
					External: external_metrics.ExternalMetricValue{
						MetricName:   config.Name,
						MetricLabels: config.Labels,
						Timestamp:    metav1.Time{Time: time.Time{}},
						Value:        *resource.NewMilliQuantity(int64(1.0)*1000, resource.DecimalSI),
					},
				},
			},
		},
		{
			msg: "test not getting any metrics",
		},
	} {
		tt.Run(ti.msg, func(t *testing.T) {
			z := zmonMock{
				dataPoints: ti.dataPoints,
			}

			zmonCollector, err := NewZMONCollector(z, config, nil, 1*time.Second)
			require.NoError(t, err)

			metrics, _ := zmonCollector.GetMetrics()
			require.Equal(t, ti.collectedMetrics, metrics)
		})
	}
}

func TestZMONCollectorInterval(t *testing.T) {
	collector := ZMONCollector{interval: 1 * time.Second}
	require.Equal(t, 1*time.Second, collector.Interval())
}
