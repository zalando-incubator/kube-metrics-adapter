module github.com/zalando-incubator/kube-metrics-adapter

require (
	github.com/aws/aws-sdk-go v1.40.22
	github.com/go-openapi/spec v0.20.3
	github.com/influxdata/influxdb-client-go v0.2.0
	github.com/influxdata/line-protocol v0.0.0-20201012155213-5f565037cbc9 // indirect
	github.com/kubernetes-sigs/custom-metrics-apiserver v0.0.0-20201216091021-1b9fa998bbaa
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/prometheus/client_golang v1.11.0
	github.com/prometheus/common v0.28.0
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/cobra v1.2.1
	github.com/spyzhov/ajson v0.4.2
	github.com/stretchr/testify v1.7.0
	github.com/szuecs/routegroup-client v0.18.3
	github.com/zalando-incubator/cluster-lifecycle-manager v0.0.0-20180921141935-824b77fb1f84
	golang.org/x/oauth2 v0.0.0-20210819190943-2bc19b11175f
	k8s.io/api v0.23.0
	k8s.io/apimachinery v0.23.0
	k8s.io/apiserver v0.23.0
	k8s.io/client-go v0.23.0
	k8s.io/code-generator v0.23.0
	k8s.io/component-base v0.23.0
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20211115234752-e816edb12b65
	k8s.io/metrics v0.21.5
	sigs.k8s.io/controller-tools v0.8.0
)

go 1.16
