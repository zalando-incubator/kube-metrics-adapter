## Usage

[Helm](https://helm.sh) must be installed to use the charts.  Please refer to
Helm's [documentation](https://helm.sh/docs) to get started.

Once Helm has been set up correctly, add the repo as follows:

  helm repo add kube-metrics-adapter https://zalando-incubator.github.io/kube-metrics-adapter

If you had already added this repo earlier, run `helm repo update` to retrieve
the latest versions of the packages.  You can then run `helm search repo
kube-metrics-adapter` to see the charts.

To install the kube-metrics-adapter chart:

    helm install my-kube-metrics-adapter kube-metrics-adapter/kube-metrics-adapter

To uninstall the chart:

    helm delete my-kube-metrics-adapter
    