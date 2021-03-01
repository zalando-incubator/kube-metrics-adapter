module github.com/zalando-incubator/kube-metrics-adapter

require (
	github.com/NYTimes/gziphandler v1.0.1 // indirect
	github.com/aws/aws-sdk-go v1.37.1
	github.com/go-openapi/spec v0.20.2
	github.com/influxdata/influxdb-client-go v0.2.0
	github.com/influxdata/line-protocol v0.0.0-20201012155213-5f565037cbc9 // indirect
	github.com/kubernetes-sigs/custom-metrics-apiserver v0.0.0-20201216091021-1b9fa998bbaa
	github.com/mattn/go-colorable v0.1.4 // indirect
	github.com/mattn/go-isatty v0.0.10 // indirect
	github.com/onsi/gomega v1.8.1 // indirect
	github.com/prometheus/client_golang v1.9.0
	github.com/prometheus/common v0.15.0
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/cobra v0.0.7
	github.com/spyzhov/ajson v0.4.2
	github.com/stretchr/testify v1.7.0
	github.com/zalando-incubator/cluster-lifecycle-manager v0.0.0-20180921141935-824b77fb1f84
	golang.org/x/crypto v0.0.0-20201124201722-c8d3bf9c5392 // indirect
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	k8s.io/api v0.20.0
	k8s.io/apimachinery v0.20.4
	k8s.io/apiserver v0.20.0
	k8s.io/client-go v0.20.0
	k8s.io/component-base v0.20.0
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20201113171705-d219536bb9fd
	k8s.io/metrics v0.20.0
)

go 1.15
