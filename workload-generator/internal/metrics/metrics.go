package metrics

import (
	"context"
	"database/sql"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"github.com/sirupsen/logrus"
	"time"
)

type Metric struct {
	ID    int
	Time  time.Time
	Pod   string
	Type  string
	Value float64
}

//const (
//	// Prometheus query to get CPU usage of the operator
//	cpuUsageQueryStr = "sum(rate(process_cpu_seconds_total{namespace=\"simulated-workload-operator-system\", job=\"simulated-workload-operator-controller-manager-metrics-service\"}[5s]))"
//	// Prometheus query to get memory usage of the operator
//	memoryUsageQueryStr = "sum(process_resident_memory_bytes{namespace=\"simulated-workload-operator-system\", job=\"simulated-workload-operator-controller-manager-metrics-service\"})"
//)

//func CheckMemory(maxK8sMemoryUsage int) (bool, error) {
//	promQuery := "process_resident_memory_bytes{job='simulated-workload-operator-controller-manager-metrics-service', namespace='simulated-workload-operator-system'}"
//
//	result, _, err := r.Env.PromAPI.Query(context.Background(), promQuery, time.Now())
//	if err != nil {
//		return false, fmt.Errorf("querying Prometheus for memory usage: %s", err)
//	}
//
//	vectorResult, ok := result.(model.Vector)
//	if !ok {
//		return false, fmt.Errorf("invalid query result format")
//	}
//
//	for _, sample := range vectorResult {
//		// sample.Value contains the memory usage of the operator
//		// if any of the instances has memory usage more than threshold return false
//		if float64(sample.Value) > float64(maxK8sMemoryUsage)*1024*1024 { // Convert MiB to bytes
//			return false, nil
//		}
//	}
//
//	return true, nil
//}

func FetchAndStoreMetrics(ctx context.Context, dbConn *sql.DB, promAPI v1.API) {
	metrics := make([]Metric, 0)

	for {
		select {
		case <-ctx.Done():
			if err := InsertMetrics(dbConn, metrics); err != nil {
				logrus.Errorf("Error inserting metrics into database: %s\n", err)
			}
			return
		default:
			cpuMetrics, err := collectCPUUsage(ctx, promAPI)
			if err != nil {
				logrus.Error("error collecting CPU usage: ", err)
				continue
			}

			memoryMetrics, err := CollectMemoryUsage(ctx, promAPI)
			if err != nil {
				logrus.Error("error collecting memory usage: ", err)
				continue
			}

			metrics = append(metrics, cpuMetrics...)
			metrics = append(metrics, memoryMetrics...)

			err = InsertMetrics(dbConn, metrics)
			if err != nil {
				logrus.Error("error inserting metric: ", err)
			}
			metrics = make([]Metric, 0)
			time.Sleep(time.Second * 1) // adjust interval as needed
		}
	}
}

func InsertMetrics(dbConn *sql.DB, metrics []Metric) error {
	if len(metrics) == 0 {
		return nil
	}

	stmt := `INSERT INTO metrics (time, pod, type, value) VALUES `

	// Placeholder slice for sql arguments
	args := make([]interface{}, 0, len(metrics)*4)

	// Create placehoders for multiple insert and fill the arguments slice
	for _, metric := range metrics {
		stmt += "(?,?,?,?),"
		args = append(args, metric.Time, metric.Pod, metric.Type, metric.Value)
	}

	// Remove the last comma
	stmt = stmt[:len(stmt)-1]

	// Prepare and execute the statement
	_, err := dbConn.Exec(stmt, args...)
	if err != nil {
		logrus.Errorf("Could not insert metrics: %v\n", err)
		return err
	}

	return nil
}

func collectCPUUsage(ctx context.Context, promAPI v1.API) ([]Metric, error) {
	query := `rate(process_cpu_seconds_total{job=~"simulated-workload-operator-controller-manager-metrics-service|sharder"}[5s])`

	result, warnings, err := promAPI.Query(ctx, query, time.Now())
	if err != nil {
		// handle error
		logrus.Errorf("Error querying Prometheus: %v\n", err)
		return nil, err
	}
	if len(warnings) > 0 {
		logrus.Warnf("Warnings: %v\n", warnings)
	}

	var metrics []Metric
	vec := result.(model.Vector)
	for _, item := range vec {
		metrics = append(metrics, Metric{
			Time:  item.Timestamp.Time(),
			Pod:   string(item.Metric["pod"]),
			Type:  "cpu",
			Value: float64(item.Value),
		})
	}
	return metrics, nil
}

func CollectMemoryUsage(ctx context.Context, promAPI v1.API) ([]Metric, error) {
	query := `process_resident_memory_bytes{job=~"simulated-workload-operator-controller-manager-metrics-service|sharder"}`

	result, warnings, err := promAPI.Query(ctx, query, time.Now())
	if err != nil {
		// handle error
		logrus.Errorf("Error querying Prometheus: %v\n", err)
		return nil, err
	}
	if len(warnings) > 0 {
		logrus.Warnf("Warnings: %v\n", warnings)
	}

	var metrics []Metric
	vec := result.(model.Vector)
	for _, item := range vec {
		metrics = append(metrics, Metric{
			Time:  item.Timestamp.Time(),
			Pod:   string(item.Metric["pod"]),
			Type:  "memory",
			Value: float64(item.Value),
		})
	}
	return metrics, nil
}
