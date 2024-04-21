package main

import (
	"database/sql"
	"flag"
	simulationv1alpha1 "github.com/930C/simulated-workload-operator/api/v1alpha1"
	"github.com/930C/workload-generator/internal/db"
	"github.com/930C/workload-generator/internal/experiment"
	"github.com/930C/workload-generator/internal/setup"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"io"
	"k8s.io/client-go/kubernetes/scheme"
	"os"
)

func init() {
	flag.StringVar(&setup.Kubeconfig, "kubeconfig", "", "path to Kubernetes config file")
	flag.StringVar(&setup.PrometheusURL, "prometheus-url", "http://127.0.0.1:9090", "URL to reach Prometheus")
	flag.IntVar(&setup.Experiment.StartResources, "startres", 1, "Starting number of resources")
	flag.IntVar(&setup.Experiment.EndResources, "endres", 5, "Ending number of resources")
	flag.StringVar(&setup.Experiment.TaskTypes, "tasks", "cpu,memory,io,sleep", "Types of tasks to run (comma separated)")
	flag.IntVar(&setup.Experiment.OpSpec.CPULimit, "cpulimit", 500, "CPU limit for tasks")
	flag.IntVar(&setup.Experiment.OpSpec.MemoryLimit, "memlimit", 128, "Memory limit for tasks")
	flag.IntVar(&setup.Experiment.OpSpec.MaxWorkerInstances, "maxworkers", 1, "Maximum number of worker instances")
	flag.BoolVar(&setup.Experiment.OpSpec.IsSharded, "sharded", false, "Whether to shard the database")
	flag.IntVar(&setup.Experiment.OpSpec.ShardCount, "shardcount", 0, "Number of shards")

	flag.Parse()
	simulationv1alpha1.AddToScheme(scheme.Scheme)

	logrus.SetLevel(logrus.DebugLevel)

	// open an output file, this will append to the today's file if server restarted.
	file, err := os.OpenFile("logrus.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err == nil {
		mw := io.MultiWriter(os.Stdout, file)
		logrus.SetOutput(mw)
	} else {
		logrus.Info("Failed to log to file, using default stderr")
	}
}

func main() {
	database, err := db.NewDatabase()
	if err != nil {
		logrus.Errorf("Error setting up database: %s\n", err)
		os.Exit(1)
	}
	defer func(database *sql.DB) {
		err := database.Close()
		if err != nil {
			logrus.Errorf("Error closing database: %s\n", err)
		}
	}(database)

	if err := setup.Setup(); err != nil {
		logrus.Errorf("Setup error: %s\n", err)
		os.Exit(1)
	}

	if err := experiment.RunExperiment(database); err != nil {
		logrus.Errorf("Error running experiment: %s\n", err)
		os.Exit(1)
	}
}
