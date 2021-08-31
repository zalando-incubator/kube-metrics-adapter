/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/kubernetes-sigs/custom-metrics-apiserver/pkg/apiserver"
	"github.com/kubernetes-sigs/custom-metrics-apiserver/pkg/cmd/server"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	rg "github.com/szuecs/routegroup-client/client/clientset/versioned"
	"github.com/zalando-incubator/cluster-lifecycle-manager/pkg/credentials-loader/platformiam"
	generatedopenapi "github.com/zalando-incubator/kube-metrics-adapter/pkg/api/generated/openapi"
	v1 "github.com/zalando-incubator/kube-metrics-adapter/pkg/apis/zalando.org/v1"
	"github.com/zalando-incubator/kube-metrics-adapter/pkg/client/clientset/versioned"
	"github.com/zalando-incubator/kube-metrics-adapter/pkg/collector"
	"github.com/zalando-incubator/kube-metrics-adapter/pkg/provider"
	"github.com/zalando-incubator/kube-metrics-adapter/pkg/zmon"
	"golang.org/x/oauth2"
	"k8s.io/apimachinery/pkg/fields"
	openapinamer "k8s.io/apiserver/pkg/endpoints/openapi"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
)

const (
	defaultClientGOTimeout = 30 * time.Second
)

// NewCommandStartAdapterServer provides a CLI handler for 'start adapter server' command
func NewCommandStartAdapterServer(stopCh <-chan struct{}) *cobra.Command {
	baseOpts := server.NewCustomMetricsAdapterServerOptions()
	o := AdapterServerOptions{
		CustomMetricsAdapterServerOptions: baseOpts,
		EnableCustomMetricsAPI:            true,
		EnableExternalMetricsAPI:          true,
		MetricsAddress:                    ":7979",
		ZMONTokenName:                     "zmon",
		CredentialsDir:                    "/meta/credentials",
	}

	cmd := &cobra.Command{
		Short: "Launch the custom metrics API adapter server",
		Long:  "Launch the custom metrics API adapter server",
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(); err != nil {
				return err
			}
			if err := o.Validate(args); err != nil {
				return err
			}
			if err := o.RunCustomMetricsAdapterServer(stopCh); err != nil {
				return err
			}
			return nil
		},
	}

	flags := cmd.Flags()
	o.SecureServing.AddFlags(flags)
	o.Authentication.AddFlags(flags)
	o.Authorization.AddFlags(flags)
	o.Features.AddFlags(flags)

	flags.StringVar(&o.RemoteKubeConfigFile, "lister-kubeconfig", o.RemoteKubeConfigFile, ""+
		"kubeconfig file pointing at the 'core' kubernetes server with enough rights to list "+
		"any described objects")
	flags.BoolVar(&o.EnableCustomMetricsAPI, "enable-custom-metrics-api", o.EnableCustomMetricsAPI, ""+
		"whether to enable Custom Metrics API")
	flags.BoolVar(&o.EnableExternalMetricsAPI, "enable-external-metrics-api", o.EnableExternalMetricsAPI, ""+
		"whether to enable External Metrics API")
	flags.BoolVar(&o.PrometheusEnabled, "prometheus-enabled", o.PrometheusEnabled, ""+
		"whether to enable the prometheus plugin (if enabled, other options must be provided or specified on a per-hpa basis to configure)")
	flags.StringVar(&o.PrometheusServer, "prometheus-server", o.PrometheusServer, ""+
		"url of prometheus server to query")
	flags.BoolVar(&o.InfluxDBEnabled, "influxdb-enabled", o.InfluxDBEnabled, ""+
		"whether to enable the influxdb plugin (if enabled, other options must be provided or specified on a per-hpa basis to configure)")
	flags.StringVar(&o.InfluxDBAddress, "influxdb-address", o.InfluxDBAddress, ""+
		"address of InfluxDB 2.x server to query (e.g. http://localhost:9999)")
	flags.StringVar(&o.InfluxDBToken, "influxdb-token", o.InfluxDBToken, ""+
		"token for InfluxDB 2.x server to query")
	flags.StringVar(&o.InfluxDBOrg, "influxdb-org", o.InfluxDBOrg, ""+
		"organization ID for InfluxDB 2.x server to query")
	flags.StringVar(&o.ZMONKariosDBEndpoint, "zmon-kariosdb-endpoint", o.ZMONKariosDBEndpoint, ""+
		"url of ZMON KariosDB endpoint to query for ZMON checks")
	flags.StringVar(&o.ZMONTokenName, "zmon-token-name", o.ZMONTokenName, ""+
		"name of the token used to query ZMON")
	flags.StringVar(&o.Token, "token", o.Token, ""+
		"static oauth2 token to use when calling external services like ZMON")
	flags.StringVar(&o.CredentialsDir, "credentials-dir", o.CredentialsDir, ""+
		"path to the credentials dir where tokens are stored")
	flags.BoolVar(&o.SkipperIngressMetrics, "skipper-ingress-metrics", o.SkipperIngressMetrics, ""+
		"whether to enable skipper ingress metrics")
	flags.BoolVar(&o.SkipperRouteGroupMetrics, "skipper-routegroup-metrics", o.SkipperRouteGroupMetrics, ""+
		"whether to enable skipper routegroup metrics")
	flags.StringArrayVar(&o.SkipperBackendWeightAnnotation, "skipper-backends-annotation", o.SkipperBackendWeightAnnotation, ""+
		"the annotation to get backend weights so that the returned metric can be weighted")
	flags.BoolVar(&o.AWSExternalMetrics, "aws-external-metrics", o.AWSExternalMetrics, ""+
		"whether to enable AWS external metrics")
	flags.StringSliceVar(&o.AWSRegions, "aws-region", o.AWSRegions, "the AWS regions which should be monitored. eg: eu-central, eu-west-1")
	flags.StringVar(&o.MetricsAddress, "metrics-address", o.MetricsAddress, "The address where to serve prometheus metrics")
	flags.BoolVar(&o.DisregardIncompatibleHPAs, "disregard-incompatible-hpas", o.DisregardIncompatibleHPAs, ""+
		"disregard failing to create collectors for incompatible HPAs")
	flags.DurationVar(&o.MetricsTTL, "metrics-ttl", 15*time.Minute, "TTL for metrics that are stored in in-memory cache.")
	flags.DurationVar(&o.GCInterval, "garbage-collector-interval", 10*time.Minute, "Interval to clean up metrics that are stored in in-memory cache.")
	flags.BoolVar(&o.ScalingScheduleMetrics, "scaling-schedule", o.ScalingScheduleMetrics, ""+
		"whether to enable time-based ScalingSchedule metrics")
	return cmd
}

func (o AdapterServerOptions) RunCustomMetricsAdapterServer(stopCh <-chan struct{}) error {
	go func() {
		http.Handle("/metrics", promhttp.Handler())
		klog.Fatal(http.ListenAndServe(o.MetricsAddress, nil))
	}()

	config, err := o.Config()
	if err != nil {
		return err
	}

	config.GenericConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(generatedopenapi.GetOpenAPIDefinitions, openapinamer.NewDefinitionNamer(apiserver.Scheme))
	config.GenericConfig.OpenAPIConfig.Info.Title = "kube-metrics-adapter"
	config.GenericConfig.OpenAPIConfig.Info.Version = "1.0.0"

	var clientConfig *rest.Config
	if len(o.RemoteKubeConfigFile) > 0 {
		loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: o.RemoteKubeConfigFile}
		loader := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &clientcmd.ConfigOverrides{})

		clientConfig, err = loader.ClientConfig()
	} else {
		clientConfig, err = rest.InClusterConfig()
	}
	if err != nil {
		return fmt.Errorf("unable to construct lister client config to initialize provider: %v", err)
	}

	// convert stop channel to a context
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-stopCh
		cancel()
	}()

	clientConfig.Timeout = defaultClientGOTimeout

	client, err := kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize new client: %v", err)
	}

	rgClient, err := rg.NewForConfig(clientConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize RouteGroup client: %v", err)
	}

	collectorFactory := collector.NewCollectorFactory()

	if o.PrometheusServer != "" && o.PrometheusEnabled {
		klog.Warningln("Prometheus server is configured, but not enabled. For backwards compatibility, the plugin will be enabled. Please update your configuration to pass --prometheus-enabled.")
		o.PrometheusEnabled = true
	}

	if o.PrometheusEnabled {
		promPlugin, err := collector.NewPrometheusCollectorPlugin(client, o.PrometheusServer)
		if err != nil {
			return fmt.Errorf("failed to initialize prometheus collector plugin: %v", err)
		}

		err = collectorFactory.RegisterObjectCollector("", "prometheus", promPlugin)
		if err != nil {
			return fmt.Errorf("failed to register prometheus object collector plugin: %v", err)
		}

		collectorFactory.RegisterExternalCollector([]string{collector.PrometheusMetricType, collector.PrometheusMetricNameLegacy}, promPlugin)

		// skipper collector can only be enabled if prometheus is.
		if o.SkipperIngressMetrics || o.SkipperRouteGroupMetrics {
			skipperPlugin, err := collector.NewSkipperCollectorPlugin(client, rgClient, promPlugin, o.SkipperBackendWeightAnnotation)
			if err != nil {
				return fmt.Errorf("failed to initialize skipper collector plugin: %v", err)
			}

			if o.SkipperIngressMetrics {
				err = collectorFactory.RegisterObjectCollector("Ingress", "", skipperPlugin)
				if err != nil {
					return fmt.Errorf("failed to register skipper Ingress collector plugin: %v", err)
				}
			}

			if o.SkipperRouteGroupMetrics {
				err = collectorFactory.RegisterObjectCollector("RouteGroup", "", skipperPlugin)
				if err != nil {
					return fmt.Errorf("failed to register skipper RouteGroup collector plugin: %v", err)
				}
			}
		}
	}

	if o.InfluxDBAddress != "" && o.InfluxDBEnabled {
		klog.Warningln("InfluxDB address is configured, but not enabled. For backwards compatibility, the plugin will be enabled. Please update your configuration to pass --influxdb-enabled.")
		o.InfluxDBEnabled = true
	}

	if o.InfluxDBEnabled {
		influxdbPlugin, err := collector.NewInfluxDBCollectorPlugin(client, o.InfluxDBAddress, o.InfluxDBToken, o.InfluxDBOrg)
		if err != nil {
			return fmt.Errorf("failed to initialize InfluxDB collector plugin: %v", err)
		}
		collectorFactory.RegisterExternalCollector([]string{collector.InfluxDBMetricType, collector.InfluxDBMetricNameLegacy}, influxdbPlugin)
	}

	plugin, _ := collector.NewHTTPCollectorPlugin()
	collectorFactory.RegisterExternalCollector([]string{collector.HTTPJSONPathType, collector.HTTPMetricNameLegacy}, plugin)
	// register generic pod collector
	err = collectorFactory.RegisterPodsCollector("", collector.NewPodCollectorPlugin(client))
	if err != nil {
		return fmt.Errorf("failed to register pod collector plugin: %v", err)
	}

	// enable ZMON based metrics
	if o.ZMONKariosDBEndpoint != "" {
		var tokenSource oauth2.TokenSource
		if o.Token != "" {
			tokenSource = oauth2.StaticTokenSource(&oauth2.Token{AccessToken: o.Token})
		} else {
			tokenSource = platformiam.NewTokenSource(o.ZMONTokenName, o.CredentialsDir)
		}

		httpClient := newOauth2HTTPClient(ctx, tokenSource)

		zmonClient := zmon.NewZMONClient(o.ZMONKariosDBEndpoint, httpClient)

		zmonPlugin, err := collector.NewZMONCollectorPlugin(zmonClient)
		if err != nil {
			return fmt.Errorf("failed to initialize ZMON collector plugin: %v", err)
		}

		collectorFactory.RegisterExternalCollector([]string{collector.ZMONMetricType, collector.ZMONCheckMetricLegacy}, zmonPlugin)
	}

	awsSessions := make(map[string]*session.Session, len(o.AWSRegions))
	for _, region := range o.AWSRegions {
		awsSessions[region], err = session.NewSessionWithOptions(session.Options{
			Config: aws.Config{
				Region: aws.String(region),
			},
		})
		if err != nil {
			return fmt.Errorf("unabled to create aws session for region: %s", region)
		}
	}

	if o.AWSExternalMetrics {
		collectorFactory.RegisterExternalCollector([]string{collector.AWSSQSQueueLengthMetric}, collector.NewAWSCollectorPlugin(awsSessions))
	}

	if o.ScalingScheduleMetrics {
		scalingScheduleClient, err := versioned.NewForConfig(clientConfig)
		if err != nil {
			return errors.New("unable to create [Cluster]ScalingSchedule.zalando.org/v1 client")
		}

		clusterScalingSchedulesStore := cache.NewStore(cache.MetaNamespaceKeyFunc)
		clusterReflector := cache.NewReflector(
			cache.NewListWatchFromClient(scalingScheduleClient.ZalandoV1().RESTClient(), "ClusterScalingSchedules", "", fields.Everything()),
			&v1.ClusterScalingSchedule{},
			clusterScalingSchedulesStore,
			0,
		)
		go clusterReflector.Run(ctx.Done())

		scalingSchedulesStore := cache.NewStore(cache.MetaNamespaceKeyFunc)
		reflector := cache.NewReflector(
			cache.NewListWatchFromClient(scalingScheduleClient.ZalandoV1().RESTClient(), "ScalingSchedules", "", fields.Everything()),
			&v1.ScalingSchedule{},
			scalingSchedulesStore,
			0,
		)
		go reflector.Run(ctx.Done())

		clusterPlugin, err := collector.NewClusterScalingScheduleCollectorPlugin(clusterScalingSchedulesStore, time.Now)
		if err != nil {
			return fmt.Errorf("unable to create ClusterScalingScheduleCollector plugin: %v", err)
		}
		err = collectorFactory.RegisterObjectCollector("ClusterScalingSchedule", "", clusterPlugin)
		if err != nil {
			return fmt.Errorf("failed to register ClusterScalingSchedule object collector plugin: %v", err)
		}

		plugin, err := collector.NewScalingScheduleCollectorPlugin(scalingSchedulesStore, time.Now)
		if err != nil {
			return fmt.Errorf("unable to create ScalingScheduleCollector plugin: %v", err)
		}
		err = collectorFactory.RegisterObjectCollector("ScalingSchedule", "", plugin)
		if err != nil {
			return fmt.Errorf("failed to register ScalingSchedule object collector plugin: %v", err)
		}
	}

	hpaProvider := provider.NewHPAProvider(client, 30*time.Second, 1*time.Minute, collectorFactory, o.DisregardIncompatibleHPAs, o.MetricsTTL, o.GCInterval)

	go hpaProvider.Run(ctx)

	customMetricsProvider := hpaProvider
	externalMetricsProvider := hpaProvider

	// var externalMetricsProvider := nil
	if !o.EnableCustomMetricsAPI {
		customMetricsProvider = nil
	}
	if !o.EnableExternalMetricsAPI {
		externalMetricsProvider = nil
	}

	informer := informers.NewSharedInformerFactory(client, 0)

	// In this example, the same provider implements both Custom Metrics API and External Metrics API
	server, err := config.Complete(informer).New("kube-metrics-adapter", customMetricsProvider, externalMetricsProvider)
	if err != nil {
		return err
	}
	return server.GenericAPIServer.PrepareRun().Run(ctx.Done())
}

// newInstrumentedOauth2HTTPClient creates an HTTP client with automatic oauth2
// token injection. Additionally it will spawn a go-routine for closing idle
// connections every 20 seconds on the http.Transport. This solves the problem
// of re-resolving DNS when the endpoint backend changes.
// https://github.com/golang/go/issues/23427
func newOauth2HTTPClient(ctx context.Context, tokenSource oauth2.TokenSource) *http.Client {
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
		IdleConnTimeout:       20 * time.Second,
		MaxIdleConns:          10,
		MaxIdleConnsPerHost:   2,
	}
	go func(transport *http.Transport, duration time.Duration) {
		for {
			select {
			case <-time.After(duration):
				transport.CloseIdleConnections()
			case <-ctx.Done():
				return
			}
		}
	}(transport, 20*time.Second)

	client := &http.Client{
		Transport: transport,
	}

	// add HTTP client to context (this is how the oauth2 lib gets it).
	ctx = context.WithValue(ctx, oauth2.HTTPClient, client)

	// instantiate an http.Client containg the token source.
	return oauth2.NewClient(ctx, tokenSource)
}

type AdapterServerOptions struct {
	*server.CustomMetricsAdapterServerOptions

	// RemoteKubeConfigFile is the config used to list pods from the master API server
	RemoteKubeConfigFile string
	// EnableCustomMetricsAPI switches on sample apiserver for Custom Metrics API
	EnableCustomMetricsAPI bool
	// EnableExternalMetricsAPI switches on sample apiserver for External Metrics API
	EnableExternalMetricsAPI bool
	// PrometheusEnabled enables the prometheus plugin
	PrometheusEnabled bool
	// PrometheusServer configures the default prometheus server
	PrometheusServer string
	// InfluxDBEnabled enables the influxdb plugin
	InfluxDBEnabled bool
	// InfluxDBAddress configures the default InfluxDB instance
	InfluxDBAddress string
	// InfluxDBToken is the token used for querying InfluxDB
	InfluxDBToken string
	// InfluxDBOrg is the organization ID used for querying InfluxDB
	InfluxDBOrg string
	// ZMONKariosDBEndpoint enables ZMON check queries to the specified
	// kariosDB endpoint
	ZMONKariosDBEndpoint string
	// ZMONTokenName is the name of the token used to query ZMON
	ZMONTokenName string
	// Token is an oauth2 token used to authenticate with services like
	// ZMON.
	Token string
	// CredentialsDir is the path to the dir where tokens are stored
	CredentialsDir string
	// SkipperIngressMetrics switches on support for skipper ingress based
	// metric collection.
	SkipperIngressMetrics bool
	// SkipperRouteGroupMetrics switches on support for skipper routegroup
	// based metric collection.
	SkipperRouteGroupMetrics bool
	// AWSExternalMetrics switches on support for getting external metrics
	// from AWS.
	AWSExternalMetrics bool
	// AWSRegions the AWS regions which are supported for monitoring.
	AWSRegions []string
	// MetricsAddress is the address where to serve prometheus metrics.
	MetricsAddress string
	// SkipperBackendWeightAnnotation is the annotation on the ingress indicating the backend weights
	SkipperBackendWeightAnnotation []string
	// Whether to disregard failing to create collectors for incompatible HPAs - such as when using
	// kube-metrics-adapter beside another Metrics Provider
	DisregardIncompatibleHPAs bool
	// TTL for metrics that are stored in in-memory cache
	MetricsTTL time.Duration
	// Interval to clean up metrics that are stored in in-memory cache
	GCInterval time.Duration
	// Time-based scaling based on the CRDs ScheduleScaling and ClusterScheduleScaling.
	ScalingScheduleMetrics bool
}
