package utils

type OperatorSpec struct {
	MaxWorkerInstances int
	MemoryLimit        int
	CPULimit           int
	IsSharded          bool
	ShardCount         int
}
