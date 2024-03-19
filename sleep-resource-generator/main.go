package main

import (
	"context"
	"strconv"
	"time"

	"github.com/930c/sleep-operator/api/v1alpha1"
	demo "github.com/930c/sleep-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	// Define a new scheme that includes your custom resource
	myScheme = runtime.NewScheme()
)

func init() {
	// Add your custom resource to the scheme
	_ = v1alpha1.AddToScheme(myScheme)
}

func main() {
	// Get a config to talk to the apiserver
	cfg, err := clientcmd.BuildConfigFromFlags("", "/home/lac/.kube/config")
	if err != nil {
		panic(err.Error())
	}

	// Create a new client
	c, err := client.New(cfg, client.Options{Scheme: myScheme})
	if err != nil {
		panic(err.Error())
	}

	// Define the interval between each batch of creations
	interval := time.Minute * 5

	for {
		// Create 100 Sleep resources
		for i := 0; i < 100; i++ {
			sleep := &demo.Sleep{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "sleep-" + strconv.Itoa(i),
					Namespace: "default",
				},
				Spec: demo.SleepSpec{
					Duration: 60, // Sleep duration in seconds
				},
			}

			// Create the Sleep resource in the Kubernetes cluster
			if err := c.Create(context.Background(), sleep); err != nil {
				panic(err.Error())
			}
		}

		// Wait for the defined interval before the next batch
		time.Sleep(interval)
	}
}
