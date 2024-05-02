package main

import (
	"context"
	"database/sql"
	"flag"
	"github.com/930C/bachelorarbeit-demo/workload-generator/internal/db"
	"github.com/930C/bachelorarbeit-demo/workload-generator/internal/environment"
	"github.com/930C/bachelorarbeit-demo/workload-generator/internal/experiment"
	"github.com/930C/bachelorarbeit-demo/workload-generator/internal/metrics"
	simulationv1alpha1 "github.com/930C/simulated-workload-operator/api/v1alpha1"
	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
	"io"
	"k8s.io/client-go/kubernetes/scheme"
	"os"
)

var (
	envKubeconfig    string
	envPrometheusURL string
	ExperimentRunner experiment.Runner
)

func init() {
	flag.StringVar(&envKubeconfig, "kubeconfig", "", "path to Kubernetes config file")
	flag.StringVar(&envPrometheusURL, "prometheus-url", "http://127.0.0.1:9090", "URL to reach Prometheus")

	flag.IntVar(&ExperimentRunner.Spec.StartResources, "startres", 11, "Starting number of resources")
	flag.IntVar(&ExperimentRunner.Spec.EndResources, "endres", 50, "Ending number of resources")
	flag.IntVar(&ExperimentRunner.Spec.Steps, "steps", 1, "Number of steps each resource")

	flag.StringVar(&ExperimentRunner.WorkloadSpec.SimulationType, "task", "io", "Type of tasks to run (io, cpu, memory, sleep)")
	flag.IntVar(&ExperimentRunner.WorkloadSpec.Intensity, "intensity", 15, "Intensity of the Experiment")
	flag.IntVar(&ExperimentRunner.WorkloadSpec.Duration, "duration", 10, "Duration of each Workloads")

	flag.IntVar(&ExperimentRunner.OperatorSpec.CPULimit, "cpulimit", 500, "CPU limit for tasks")
	flag.IntVar(&ExperimentRunner.OperatorSpec.MemoryLimit, "memlimit", 128, "Memory limit for tasks")
	flag.IntVar(&ExperimentRunner.OperatorSpec.MaxWorkerInstances, "maxworkers", 1, "Maximum number of worker instances")
	flag.IntVar(&ExperimentRunner.OperatorSpec.ShardCount, "shardcount", 3, "Number of shards (0 means no sharding)")

	flag.Parse()
	err := simulationv1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		return
	}

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
	var err error

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ExperimentRunner.DbConn, err = db.NewDatabase()
	if err != nil {
		logrus.Errorf("Error setting up database: %s\n", err)
		os.Exit(1)
	}
	defer func(database *sql.DB) {
		err := database.Close()
		if err != nil {
			logrus.Errorf("Error closing database: %s\n", err)
		}
	}(ExperimentRunner.DbConn)

	ExperimentRunner.Env, err = environment.NewEnvironment(envKubeconfig, envPrometheusURL)
	if err != nil {
		logrus.Errorf("Error creating environment: %s\n", err)
		os.Exit(1)
	}

	go metrics.FetchAndStoreMetrics(ctx, ExperimentRunner.DbConn, ExperimentRunner.Env.PromAPI)

	if err := ExperimentRunner.Run(ctx); err != nil {
		logrus.Errorf("Error running experiment: %s\n", err)
		os.Exit(1)
	}

	cancel() // trigger cancellation of context when experiment is done
}
