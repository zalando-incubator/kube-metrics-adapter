package collector

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/metrics/pkg/apis/external_metrics"
)

func TestHostnameCollectorPluginConstructor(tt *testing.T) {
	for _, testcase := range []struct {
		msg     string
		name    string
		isValid bool
	}{
		{"No metric name", "", false},
		{"Valid metric name", "a_valid_metric_name", true},
	} {
		tt.Run(testcase.msg, func(t *testing.T) {

			fakePlugin := &FakeCollectorPlugin{}
			plugin, err := NewHostnameCollectorPlugin(fakePlugin, testcase.name)

			if testcase.isValid {
				require.NoError(t, err)
				require.NotNil(t, plugin)
				require.Equal(t, testcase.name, plugin.metricName)
				require.Equal(t, fakePlugin, plugin.promPlugin)
			} else {
				require.NotNil(t, err)
				require.Nil(t, plugin)
			}
		})
	}
}

func TestHostnamePluginNewCollector(tt *testing.T) {
	fakePlugin := &FakeCollectorPlugin{}

	pattern, err := regexp.Compile("^[a-zA-Z0-9.-]+$")
	require.Nil(tt, err, "Something is up, regex compiling failed.")

	plugin := &HostnameCollectorPlugin{
		metricName: "a_valid_one",
		promPlugin: fakePlugin,
		pattern:    pattern,
	}
	interval := time.Duration(42)

	for _, testcase := range []struct {
		msg           string
		config        *MetricConfig
		expectedQuery string
		shouldWork    bool
	}{
		{
			"No hostname config",
			&MetricConfig{Config: make(map[string]string)},
			"",
			false,
		},
		{
			"Nil metric config",
			nil,
			"",
			false,
		},
		{
			"Valid hostname no prom query config",
			&MetricConfig{Config: map[string]string{"hostnames": "foo.bar.baz"}},
			`scalar(sum(rate(a_valid_one{host=~"foo_bar_baz"}[1m])) * 1.0000)`,
			true,
		},
		{
			"Valid hostname no prom query config",
			&MetricConfig{Config: map[string]string{"hostnames": "foo.bar.baz", "weight": "42"}},
			`scalar(sum(rate(a_valid_one{host=~"foo_bar_baz"}[1m])) * 0.4200)`,
			true,
		},
		{
			"Multiple valid hostnames no prom query config",
			&MetricConfig{Config: map[string]string{"hostnames": "foo.bar.baz,foz.bax.bas"}},
			`scalar(sum(rate(a_valid_one{host=~"foo_bar_baz|foz_bax_bas"}[1m])) * 1.0000)`,
			true,
		},
		{
			"Valid hostname with prom query config",
			&MetricConfig{
				Config: map[string]string{"hostnames": "foo.bar.baz", "query": "some_other_query"},
			},
			`scalar(sum(rate(a_valid_one{host=~"foo_bar_baz"}[1m])) * 1.0000)`,
			true,
		},
	} {
		tt.Run(testcase.msg, func(t *testing.T) {
			c, err := plugin.NewCollector(
				&autoscalingv2.HorizontalPodAutoscaler{},
				testcase.config,
				interval,
			)

			if testcase.shouldWork {
				require.NotNil(t, c)
				require.Nil(t, err)
				require.Equal(t, testcase.expectedQuery, fakePlugin.config["query"])
			} else {
				require.Nil(t, c)
				require.NotNil(t, err)
			}
		})
	}
}

func TestHostnameCollectorGetMetrics(tt *testing.T) {
	genericErr := fmt.Errorf("This is an error")
	expectedMetric := *resource.NewQuantity(int64(42), resource.DecimalSI)

	for _, testcase := range []struct {
		msg        string
		stub       func() ([]CollectedMetric, error)
		shouldWork bool
	}{
		{
			"Internal collector error",
			func() ([]CollectedMetric, error) {
				return nil, genericErr
			},
			false,
		},
		{
			"Invalid metric collection from internal collector",
			func() ([]CollectedMetric, error) {
				return []CollectedMetric{
					{External: external_metrics.ExternalMetricValue{Value: *resource.NewQuantity(int64(24), resource.DecimalSI)}},
					{External: external_metrics.ExternalMetricValue{Value: *resource.NewQuantity(int64(42), resource.DecimalSI)}},
				}, nil
			},
			false,
		},
		{
			"Internal collector return single metric",
			func() ([]CollectedMetric, error) {
				return []CollectedMetric{
					{External: external_metrics.ExternalMetricValue{Value: *resource.NewQuantity(int64(42), resource.DecimalSI)}},
				}, nil
			},
			true,
		},
	} {
		tt.Run(testcase.msg, func(t *testing.T) {
			fake := makeCollectorWithStub(testcase.stub)
			c := &HostnameCollector{promCollector: fake}
			m, err := c.GetMetrics()

			if testcase.shouldWork {
				require.Nil(t, err)
				require.NotNil(t, m)
				require.Len(t, m, 1)
				require.Equal(t, expectedMetric, m[0].External.Value)
			} else {
				require.NotNil(t, err)
				require.Nil(t, m)
			}
		})
	}
}

func TestHostnameCollectorInterval(t *testing.T) {
	interval := time.Duration(42)
	fakePlugin := &FakeCollectorPlugin{}
	pattern, err := regexp.Compile("^[a-zA-Z0-9.-]+$")
	require.Nil(t, err, "Something is up, regex compiling failed.")
	plugin := &HostnameCollectorPlugin{
		metricName: "a_valid_one",
		promPlugin: fakePlugin,
		pattern:    pattern,
	}
	c, err := plugin.NewCollector(
		&autoscalingv2.HorizontalPodAutoscaler{},
		&MetricConfig{Config: map[string]string{"hostnames": "foo.bar.baz"}},
		interval,
	)

	require.Nil(t, err)
	require.NotNil(t, c)
	require.Equal(t, interval, c.Interval())
}

func TestHostnameCollectorAndCollectorFabricInteraction(t *testing.T) {
	expectedQuery := `scalar(sum(rate(a_metric{host=~"just_testing_com"}[1m])) * 0.4200)`
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"metric-config.external.foo.requests-per-second/hostnames": "just.testing.com",
				"metric-config.external.foo.requests-per-second/weight":    "42",
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ExternalMetricSourceType,
					External: &autoscalingv2.ExternalMetricSource{
						Metric: autoscalingv2.MetricIdentifier{
							Name: "foo",
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"type": "requests-per-second"},
							},
						},
					},
				},
			},
		},
	}

	factory := NewCollectorFactory()
	fakePlugin := makePlugin(42)
	hostnamePlugin, err := NewHostnameCollectorPlugin(fakePlugin, "a_metric")
	require.NoError(t, err)
	factory.RegisterExternalCollector([]string{HostnameMetricType}, hostnamePlugin)
	conf, err := ParseHPAMetrics(hpa)
	require.NoError(t, err)
	require.Len(t, conf, 1)

	c, err := factory.NewCollector(hpa, conf[0], 0)

	require.NoError(t, err)
	_, ok := c.(*HostnameCollector)
	require.True(t, ok)
	require.Equal(t, expectedQuery, fakePlugin.config["query"])

}

func TestHostnamePrometheusCollectorInteraction(t *testing.T) {
	hostnameQuery := `scalar(sum(rate(a_metric{host=~"just_testing_com"}[1m])) * 0.4200)`
	promQuery := "sum(rate(rps[1m]))"
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"metric-config.external.foo.requests-per-second/hostnames": "just.testing.com",
				"metric-config.external.foo.requests-per-second/weight":    "42",
				"metric-config.external.bar.prometheus/query":              promQuery,
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ExternalMetricSourceType,
					External: &autoscalingv2.ExternalMetricSource{
						Metric: autoscalingv2.MetricIdentifier{
							Name: "foo",
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"type": "requests-per-second"},
							},
						},
					},
				},
				{
					Type: autoscalingv2.ExternalMetricSourceType,
					External: &autoscalingv2.ExternalMetricSource{
						Metric: autoscalingv2.MetricIdentifier{
							Name: "bar",
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{"type": "prometheus"},
							},
						},
					},
				},
			},
		},
	}

	factory := NewCollectorFactory()
	promPlugin, err := NewPrometheusCollectorPlugin(nil, "http://prometheus")
	require.NoError(t, err)
	factory.RegisterExternalCollector([]string{PrometheusMetricType, PrometheusMetricNameLegacy}, promPlugin)
	hostnamePlugin, err := NewHostnameCollectorPlugin(promPlugin, "a_metric")
	require.NoError(t, err)
	factory.RegisterExternalCollector([]string{HostnameMetricType}, hostnamePlugin)

	conf, err := ParseHPAMetrics(hpa)
	require.NoError(t, err)
	require.Len(t, conf, 2)

	collectors := make(map[string]Collector)
	collectors["hostname"], err = factory.NewCollector(hpa, conf[0], 0)
	require.NoError(t, err)
	collectors["prom"], err = factory.NewCollector(hpa, conf[1], 0)
	require.NoError(t, err)

	prom, ok := collectors["prom"].(*PrometheusCollector)
	require.True(t, ok)
	hostname, ok := collectors["hostname"].(*HostnameCollector)
	require.True(t, ok)
	hostnameProm, ok := hostname.promCollector.(*PrometheusCollector)
	require.True(t, ok)

	require.Equal(t, promQuery, prom.query)
	require.Equal(t, hostnameQuery, hostnameProm.query)
}
