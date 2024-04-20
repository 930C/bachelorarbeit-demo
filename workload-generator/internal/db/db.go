package db

import (
	"database/sql"
	"fmt"
	simulationv1alpha1 "github.com/930C/simulated-workload-operator/api/v1alpha1"
	"github.com/930C/workload-generator/internal/utils"
)

const (
	createResultTableQuery = `CREATE TABLE IF NOT EXISTS results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
	num_resources INTEGER,
	duration_seconds FLOAT,
	workload_type TEXT,
	workload_duration INTEGER,
	workload_intensity INTEGER,
	max_worker_instances INTEGER,
	memory_limit INTEGER,
	cpu_limit INTEGER,
	is_sharded INTEGER,
	shard_count INTEGER,
	experiment_time DATETIME DEFAULT CURRENT_TIMESTAMP)`

	insertResultQuery = `INSERT INTO results
	(num_resources, duration_seconds, workload_type, workload_duration, workload_intensity, max_worker_instances, memory_limit, cpu_limit, is_sharded, shard_count)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
)

// NewDatabase creates a new database connection and returns it
func NewDatabase() (*sql.DB, error) {
	db, err := sql.Open("sqlite3", "./experiments.db")
	if err != nil {
		return nil, err
	}

	if _, err = db.Exec(createResultTableQuery); err != nil {
		return nil, err
	}

	return db, nil
}

func InsertResult(db *sql.DB, numResources int, workload simulationv1alpha1.WorkloadSpec, opSpec utils.OperatorSpec, duration float64) error {
	_, err := db.Exec(insertResultQuery, numResources, duration, workload.SimulationType, workload.Duration, workload.Intensity, opSpec.MaxWorkerInstances, opSpec.MemoryLimit, opSpec.CPULimit, opSpec.IsSharded, opSpec.ShardCount)
	if err != nil {
		fmt.Println(fmt.Errorf("error inserting result: %v", err))
	}
	return err
}
