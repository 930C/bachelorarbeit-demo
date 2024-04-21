package setup

import (
	"fmt"
	"github.com/930C/bachelorarbeit-demo/workload-generator/internal/utils"
	promapi "github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	Kubeconfig    string
	PrometheusURL string
	K8sClient     client.Client
	PromAPI       v1.API
	Experiment    utils.ExperimentSpec
)

func Setup() error {
	Config, err := clientcmd.BuildConfigFromFlags("", Kubeconfig)
	if err != nil {
		return fmt.Errorf("building kubeconfig: %s", err)
	}

	K8sClient, err = client.New(Config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return fmt.Errorf("creating Kubernetes client: %s", err)
	}

	PromClient, err := promapi.NewClient(promapi.Config{Address: PrometheusURL})
	if err != nil {
		return fmt.Errorf("creating Prometheus client: %s", err)
	}
	PromAPI = v1.NewAPI(PromClient)
	return nil
}
