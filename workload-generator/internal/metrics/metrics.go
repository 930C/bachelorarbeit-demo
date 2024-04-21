package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/930C/workload-generator/internal/db"
	"github.com/930C/workload-generator/internal/setup"
	"github.com/prometheus/common/model"
	"time"
)

func CheckMemory(maxK8sMemoryUsage int) (bool, error) {
	promQuery := "process_resident_memory_bytes{job='simulated-workload-operator-controller-manager-metrics-service', namespace='simulated-workload-operator-system'}"

	result, _, err := setup.PromAPI.Query(context.Background(), promQuery, time.Now())
	if err != nil {
		return false, fmt.Errorf("querying Prometheus for memory usage: %s", err)
	}

	vectorResult, ok := result.(model.Vector)
	if !ok {
		return false, fmt.Errorf("invalid query result format")
	}

	for _, sample := range vectorResult {
		// sample.Value contains the memory usage of the operator
		// if any of the instances has memory usage more than threshold return false
		if float64(sample.Value) > float64(maxK8sMemoryUsage)*1024*1024 { // Convert MiB to bytes
			return false, nil
		}
	}

	return true, nil
}

func CollectMetrics() error {
	query := "sum(rate(controller_runtime_reconcile_total{namespace=\"simulated-workload-operator-system\", job=\"simulated-workload-operator-controller-manager-experiment-service\"}[5s]))"
	result, _, err := setup.PromAPI.Query(context.Background(), query, time.Now())
	if err != nil {
		return fmt.Errorf("querying Prometheus: %s", err)
	}

	fmt.Printf("Number of reconciliations completed: %v\n", result)
	return nil
}

func FetchAndStoreMetrics(conn *sql.DB, done <-chan bool) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cpuUsage, err := collectCPUUsage()
			if err != nil {
				fmt.Println(err)
				continue
			}

			memoryUsage, err := collectMemoryUsage()
			if err != nil {
				fmt.Println(err)
				continue
			}

			err = db.InsertMetric(conn, cpuUsage, memoryUsage)
			if err != nil {
				fmt.Println(err)
			}
		case <-done:
			// We got signal to stop, exiting the goroutine
			return
		}
	}
}

func collectMemoryUsage() (float64, error) {
	result, _, err := setup.PromAPI.Query(context.Background(), "sum(process_resident_memory_bytes{namespace=\"simulated-workload-operator-system\", job=\"simulated-workload-operator-controller-manager-metrics-service\"})", time.Now())
	if err != nil {
		fmt.Printf("Error querying Prometheus: %s\n", err)
		return 0, err
	}

	switch v := result.(type) {
	case model.Vector:
		// v is of type model.Vector.
		for _, sample := range v {
			f64val := float64(sample.Value)
			// Now f64val is a float64 version of each sample from your vector.
			// You may choose to append it to some slice or use it right here.
			return f64val, nil
		}
	case *model.Scalar:
		// v is of type *model.Scalar.
		f64val := float64(v.Value)
		// f64val is a float64 of the scalar value.
		return f64val, nil
	default:
		// v is of a different type.
		fmt.Printf("Unexpected type %T\n", v)
	}

	return 0, nil
}

func collectCPUUsage() (float64, error) {
	// Collect CPU usage from Prometheus
	result, _, err := setup.PromAPI.Query(context.Background(), "sum(rate(process_cpu_seconds_total{namespace=\"simulated-workload-operator-system\", job=\"simulated-workload-operator-controller-manager-metrics-service\"}[5s]))", time.Now())
	if err != nil {
		fmt.Printf("Error querying Prometheus: %s\n", err)
		return 0, err
	}

	switch v := result.(type) {
	case model.Vector:
		// v is of type model.Vector.
		for _, sample := range v {
			f64val := float64(sample.Value)
			// Now f64val is a float64 version of each sample from your vector.
			// You may choose to append it to some slice or use it right here.
			return f64val, nil
		}
	case *model.Scalar:
		// v is of type *model.Scalar.
		f64val := float64(v.Value)
		// f64val is a float64 of the scalar value.
		return f64val, nil
	default:
		// v is of a different type.
		fmt.Printf("Unexpected type %T\n", v)
	}

	fmt.Printf("CPU Usage: %v\n", result)
	return 0, nil
}
