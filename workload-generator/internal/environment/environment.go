package environment

import (
	"fmt"
	promapi "github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Environment struct {
	K8sClient client.Client
	PromAPI   v1.API
}

func NewEnvironment(kubeconfig, prometheusUrl string) (*Environment, error) {
	Config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("building kubeconfig: %s", err)
	}

	k8sClient, err := client.New(Config, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, fmt.Errorf("creating Kubernetes client: %s", err)
	}

	promClient, err := promapi.NewClient(promapi.Config{Address: prometheusUrl})
	if err != nil {
		return nil, fmt.Errorf("creating Prometheus client: %s", err)
	}
	promAPI := v1.NewAPI(promClient)

	return &Environment{
		K8sClient: k8sClient,
		PromAPI:   promAPI,
	}, nil
}
