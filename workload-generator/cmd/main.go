package main

import (
	"database/sql"
	"flag"
	"fmt"
	simulationv1alpha1 "github.com/930C/simulated-workload-operator/api/v1alpha1"
	"github.com/930C/workload-generator/internal/db"
	"github.com/930C/workload-generator/internal/experiment"
	"github.com/930C/workload-generator/internal/setup"
	_ "github.com/mattn/go-sqlite3"
	"k8s.io/client-go/kubernetes/scheme"
	"os"
)

func init() {
	flag.StringVar(&setup.Kubeconfig, "kubeconfig", "", "path to Kubernetes config file")
	flag.StringVar(&setup.PrometheusURL, "prometheus-url", "http://127.0.0.1:9090", "URL to reach Prometheus")
	flag.Parse()
	simulationv1alpha1.AddToScheme(scheme.Scheme)
}

func main() {
	db, err := db.NewDatabase()
	if err != nil {
		fmt.Printf("Error setting up database: %s\n", err)
		os.Exit(1)
	}
	defer func(db *sql.DB) {
		err := db.Close()
		if err != nil {
			fmt.Printf("Error closing database: %s\n", err)
		}
	}(db)

	if err := setup.Setup(); err != nil {
		fmt.Printf("Setup error: %s\n", err)
		os.Exit(1)
	}

	if err := experiment.RunExperiment(db); err != nil {
		fmt.Printf("Error running experiment: %s\n", err)
		os.Exit(1)
	}
}
