package db

import (
	"database/sql"
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
	shard_count INTEGER,
	experiment_time DATETIME DEFAULT CURRENT_TIMESTAMP)`

	InsertResultQuery = `INSERT INTO results
	(num_resources, duration_seconds, workload_type, workload_duration, workload_intensity, max_worker_instances, memory_limit, cpu_limit, shard_count)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

	createMetricsTableQuery = `CREATE TABLE IF NOT EXISTS metrics (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        cpu_usage FLOAT,
        memory_usage FLOAT,
        metric_time DATETIME DEFAULT CURRENT_TIMESTAMP
    )`

	createMetricsTable = `
	CREATE TABLE IF NOT EXISTS metrics (
	    id INTEGER PRIMARY KEY AUTOINCREMENT,
	    time DATETIME,
	    pod TEXT,
	    type TEXT,
	    value FLOAT
	                                   )`

	insertMetricQuery = `INSERT INTO metrics
    (cpu_usage, memory_usage)
    VALUES (?, ?)`
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

	if _, err = db.Exec(createMetricsTable); err != nil {
		return nil, err
	}

	return db, nil
}
