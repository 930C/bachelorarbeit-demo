package utils

type OperatorSpec struct {
	MaxWorkerInstances int
	MemoryLimit        int
	CPULimit           int
	IsSharded          bool
	ShardCount         int
}

type ExperimentSpec struct {
	StartResources int
	EndResources   int
	TaskTypes      string
	OpSpec         OperatorSpec
}
