package metrics

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/930C/workload-generator/internal/db"
	"github.com/930C/workload-generator/internal/setup"
	"github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"
	"time"
)

const (
	// Prometheus query to get CPU usage of the operator
	cpuUsageQueryStr = "sum(rate(process_cpu_seconds_total{namespace=\"simulated-workload-operator-system\", job=\"simulated-workload-operator-controller-manager-metrics-service\"}[5s]))"
	// Prometheus query to get memory usage of the operator
	memoryUsageQueryStr = "sum(process_resident_memory_bytes{namespace=\"simulated-workload-operator-system\", job=\"simulated-workload-operator-controller-manager-metrics-service\"})"
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

func FetchAndStoreMetrics(conn *sql.DB, ctx context.Context, done <-chan bool) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			cpuUsage, err := collectResourceUsage(ctx, cpuUsageQueryStr)
			if err != nil {
				logrus.Error("error collecting CPU usage: ", err)
				continue
			}

			memoryUsage, err := collectResourceUsage(ctx, memoryUsageQueryStr)
			if err != nil {
				logrus.Error("error collecting memory usage: ", err)
				continue
			}

			err = db.InsertMetric(conn, cpuUsage, memoryUsage)
			if err != nil {
				logrus.Error("error inserting metric: ", err)
			}
		case <-done:
			// We got signal to stop, exiting the goroutine
			logrus.Debug("Metrics goroutine received signal to stop")
			return
		}
	}
}

func collectResourceUsage(ctx context.Context, queryString string) (float64, error) {
	result, _, err := setup.PromAPI.Query(ctx, queryString, time.Now())
	if err != nil {
		return 0, fmt.Errorf("error querying Prometheus: %s", err)
	}

	switch v := result.(type) {
	case model.Vector:
		for _, sample := range v {
			f64val := float64(sample.Value)
			return f64val, nil
		}
	case *model.Scalar:
		f64val := float64(v.Value)
		return f64val, nil
	default:
		return 0, fmt.Errorf("unexpected type %T", v)
	}

	return 0, nil
}
