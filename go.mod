module github.com/zalando-incubator/kube-metrics-adapter

require (
	github.com/NYTimes/gziphandler v1.0.1 // indirect
	github.com/aws/aws-sdk-go v1.30.0
	github.com/googleapis/gnostic v0.2.0 // indirect
	github.com/influxdata/influxdb-client-go v0.1.5
	github.com/kubernetes-incubator/custom-metrics-apiserver v0.0.0-20200323093244-5046ce1afe6b
	github.com/oliveagle/jsonpath v0.0.0-20180606110733-2e52cf6e6852
	github.com/prometheus/client_golang v1.5.1
	github.com/prometheus/common v0.4.1
	github.com/sirupsen/logrus v1.5.0
	github.com/spf13/cobra v0.0.7
	github.com/stretchr/testify v1.5.1
	github.com/zalando-incubator/cluster-lifecycle-manager v0.0.0-20180921141935-824b77fb1f84
	golang.org/x/crypto v0.0.0-20200220183623-bac4c82f6975 // indirect
	golang.org/x/oauth2 v0.0.0-20190604053449-0f29369cfe45
	k8s.io/api v0.17.3
	k8s.io/apimachinery v0.17.4
	k8s.io/client-go v0.17.3
	k8s.io/component-base v0.17.3
	k8s.io/klog v1.0.0
	k8s.io/metrics v0.17.3
)

go 1.13
