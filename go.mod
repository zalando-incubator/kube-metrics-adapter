module github.com/zalando-incubator/kube-metrics-adapter

require (
	github.com/NYTimes/gziphandler v1.0.1 // indirect
	github.com/aws/aws-sdk-go v1.36.19
	github.com/go-openapi/spec v0.20.0
	github.com/influxdata/influxdb-client-go v0.2.0
	github.com/influxdata/line-protocol v0.0.0-20201012155213-5f565037cbc9 // indirect
	github.com/kubernetes-incubator/custom-metrics-apiserver v0.0.0-20201023074945-51cc7b53320e
	github.com/mattn/go-colorable v0.1.4 // indirect
	github.com/mattn/go-isatty v0.0.10 // indirect
	github.com/onsi/gomega v1.8.1 // indirect
	github.com/prometheus/client_golang v1.9.0
	github.com/prometheus/common v0.15.0
	github.com/sirupsen/logrus v1.7.0
	github.com/spf13/cobra v0.0.7
	github.com/spyzhov/ajson v0.4.2
	github.com/stretchr/testify v1.6.1
	github.com/zalando-incubator/cluster-lifecycle-manager v0.0.0-20180921141935-824b77fb1f84
	golang.org/x/crypto v0.0.0-20201124201722-c8d3bf9c5392 // indirect
	golang.org/x/net v0.0.0-20201224014010-6772e930b67b // indirect
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	golang.org/x/tools v0.0.0-20200204192400-7124308813f3 // indirect
	honnef.co/go/tools v0.0.1-2020.1.3 // indirect
	k8s.io/api v0.18.14
	k8s.io/apimachinery v0.20.1
	k8s.io/apiserver v0.18.14
	k8s.io/client-go v0.18.14
	k8s.io/component-base v0.18.14
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20201113171705-d219536bb9fd
	k8s.io/metrics v0.18.8
)

go 1.15
