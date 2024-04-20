package metrics

import (
	"context"
	"fmt"
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
