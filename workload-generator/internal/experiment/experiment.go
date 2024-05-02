package experiment

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/930C/bachelorarbeit-demo/workload-generator/internal/db"
	"github.com/930C/bachelorarbeit-demo/workload-generator/internal/environment"
	"github.com/930C/bachelorarbeit-demo/workload-generator/internal/metrics"
	simulationv1alpha1 "github.com/930C/simulated-workload-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"strings"
	"time"
)

type Runner struct {
	Env          *environment.Environment
	DbConn       *sql.DB
	Spec         Spec
	OperatorSpec OperatorSpec
	WorkloadSpec simulationv1alpha1.WorkloadSpec
}

type Spec struct {
	StartResources int
	EndResources   int
	Steps          int
}

type OperatorSpec struct {
	MaxWorkerInstances int
	MemoryLimit        int
	CPULimit           int
	ShardCount         int
}

func (r Runner) Run(ctx context.Context) error {
	for NumResources := r.Spec.StartResources; NumResources <= r.Spec.EndResources; NumResources += r.Spec.Steps {
		logrus.Info("----------------------------------------")
		logrus.Infof("Running %s simulation with %d intensity...", r.WorkloadSpec.SimulationType, r.WorkloadSpec.Intensity)
		if err := r.runSingleExperiment(ctx, NumResources); err != nil {
			logrus.Errorf("Experiment error: %v", err)
			return err
		}
	}

	return nil
}

func (r Runner) runSingleExperiment(ctx context.Context, resources int) error {
	if err := r.ensureNamespaceClear(ctx); err != nil {
		return err
	}

	if err := r.ensureEnoughMemory(ctx); err != nil {
		return err
	}

	time.Sleep(5 * time.Second)

	startTime := time.Now()
	logrus.Info("STARTING THE RUN...")

	// Iterate over the number of resources to create
	for i := 0; i < resources; i++ {
		entry := logrus.WithFields(logrus.Fields{"Type": r.WorkloadSpec.SimulationType, "Resource": i})

		workload := simulationv1alpha1.Workload{
			Spec:       r.WorkloadSpec,
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("workload-%d-%s", i, strings.ToLower(r.WorkloadSpec.SimulationType)), Namespace: "default"},
		}

		if err := r.Env.K8sClient.Create(ctx, &workload); err != nil {
			entry.Errorf("Failed to create workload: %s", err)
			continue
		}
	}

	logrus.Info("Resources created successfully")

	// Wait for resources to be reconciled
	endTime, err := r.waitForReconciliation(ctx)
	if err != nil {
		return fmt.Errorf("failed waiting for reconciliation: %s", err)
	}

	duration := endTime.Sub(startTime).Seconds()

	if _, err := r.DbConn.Exec(db.InsertResultQuery, resources, duration, r.WorkloadSpec.SimulationType, r.WorkloadSpec.Duration, r.WorkloadSpec.Intensity, r.OperatorSpec.MaxWorkerInstances, r.OperatorSpec.MemoryLimit, r.OperatorSpec.CPULimit, r.OperatorSpec.ShardCount); err != nil {
		logrus.Errorf("failed to insert result: %v", err)
	}

	r.deleteAllWorkloads(ctx)

	logrus.Infof("RUN COMPLETED with %d resources as %s simulation type in %f seconds", resources, r.WorkloadSpec.SimulationType, duration)
	return nil
}

func (r Runner) ensureNamespaceClear(ctx context.Context) error {
	logrus.Info("Ensuring namespace is clear of workloads...")
	// Poll the state every 5 seconds until the namespace is clear
	for {
		workloads := &simulationv1alpha1.WorkloadList{}
		err := r.Env.K8sClient.List(ctx, workloads)
		if err != nil {
			return fmt.Errorf("failed to list workloads: %v", err)
		}

		if len(workloads.Items) == 0 {
			break
		}

		logrus.Warn("Namespace is not clear of workloads. Cleaning up...")

		r.deleteAllWorkloads(ctx)
	}
	logrus.Info("Namespace is clear of workloads")
	return nil
}

func (r Runner) waitForReconciliation(ctx context.Context) (time.Time, error) {
	const nanoSecondsLayout = "2006-01-02T15:04:05.999999999Z07:00"
	var endTime time.Time
	var workload simulationv1alpha1.Workload
	logrus.Infof("Waiting for workloads to be reconciled...")

	if err := wait.PollUntilContextCancel(ctx, time.Second*2, true, func(ctx context.Context) (bool, error) {
		workloadList := &simulationv1alpha1.WorkloadList{}
		err := r.Env.K8sClient.List(ctx, workloadList)
		if err != nil {
			return false, err
		}

		for _, workload = range workloadList.Items {
			if !isConditionTrue(workload.Status.Conditions, "Available") {
				logrus.Tracef("Workload %s is not yet reconciled", workload.Name)
				return false, nil
			}

			tempEndTime, _ := time.Parse(nanoSecondsLayout, workload.Status.EndTime)

			if tempEndTime.After(endTime) {
				logrus.Debugf("New end time: %s from workload %s", tempEndTime, workload.Name)
				endTime = tempEndTime
			}
		}

		return true, nil
	}); err != nil {
		return time.Time{}, fmt.Errorf("error while waiting for reconciliation: %s", err)
	}

	logrus.Infof("All workloads reconciled at %s", endTime)

	return endTime, nil
}

func isConditionTrue(conditions []metav1.Condition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r Runner) deleteAllWorkloads(ctx context.Context) {
	workloadList := &simulationv1alpha1.WorkloadList{}
	if err := r.Env.K8sClient.List(ctx, workloadList); err != nil {
		logrus.Errorf("Failed to list workloads: %s", err)
		return
	}

	for _, workload := range workloadList.Items {
		if err := r.Env.K8sClient.Delete(ctx, &workload); err != nil {
			logrus.Errorf("Failed to delete workload: %s", err)
		}
	}

	logrus.Info("Waiting for all resources to be deleted...")

	err := wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		if err := r.Env.K8sClient.List(ctx, workloadList); err != nil {
			logrus.Errorf("Error getting workload list: %s", err)
			return false, nil
		}
		if len(workloadList.Items) == 0 {
			return true, nil
		}

		logrus.Debugf("%d workloads still exist...", len(workloadList.Items))

		return false, nil
	})

	if err != nil {
		logrus.Errorf("Error waiting for deletion: %s", err)
		return
	}

	logrus.Info("All resources deleted successfully")
}

func (r Runner) ensureEnoughMemory(ctx context.Context) error {
	logrus.Info("Ensuring enough memory is available...")
	for {
		memoryMetrics, err := metrics.CollectMemoryUsage(ctx, r.Env.PromAPI)
		if err != nil {
			return err
		}

		aboveThreshold := false
		for _, metric := range memoryMetrics {
			if metric.Value > 60*1024*1024 { // 60MiB
				logrus.Warnf("Memory usage is above 60 MiB on %s", metric.Pod)
				aboveThreshold = true
				break
			}
		}

		if !aboveThreshold {
			break
		}

		time.Sleep(5 * time.Second)
	}
	logrus.Info("Enough memory is available")
	return nil
}
