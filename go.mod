module github.com/zalando-incubator/kube-metrics-adapter

require (
	github.com/NYTimes/gziphandler v1.0.1 // indirect
	github.com/aws/aws-sdk-go v1.40.12
	github.com/go-openapi/spec v0.20.3
	github.com/influxdata/influxdb-client-go v0.2.0
	github.com/influxdata/line-protocol v0.0.0-20201012155213-5f565037cbc9 // indirect
	github.com/kubernetes-sigs/custom-metrics-apiserver v0.0.0-20201216091021-1b9fa998bbaa
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/prometheus/client_golang v1.10.0
	github.com/prometheus/common v0.25.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.1.3
	github.com/spyzhov/ajson v0.4.2
	github.com/stretchr/testify v1.7.0
	github.com/zalando-incubator/cluster-lifecycle-manager v0.0.0-20180921141935-824b77fb1f84
	golang.org/x/crypto v0.0.0-20201124201722-c8d3bf9c5392 // indirect
	golang.org/x/oauth2 v0.0.0-20200107190931-bf48bf16ab8d
	k8s.io/api v0.20.9
	k8s.io/apimachinery v0.20.9
	k8s.io/apiserver v0.20.9
	k8s.io/client-go v0.20.9
	k8s.io/code-generator v0.22.0
	k8s.io/component-base v0.20.9
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20210421082810-95288971da7e
	k8s.io/metrics v0.20.9
	sigs.k8s.io/controller-tools v0.5.0
)

go 1.16
