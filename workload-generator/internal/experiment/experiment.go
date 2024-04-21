package experiment

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/930C/bachelorarbeit-demo/workload-generator/internal/db"
	"github.com/930C/bachelorarbeit-demo/workload-generator/internal/metrics"
	"github.com/930C/bachelorarbeit-demo/workload-generator/internal/setup"
	"github.com/930C/bachelorarbeit-demo/workload-generator/internal/utils"
	simulationv1alpha1 "github.com/930C/simulated-workload-operator/api/v1alpha1"
	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"os"
	"os/signal"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

var (
	WorkloadSpec = simulationv1alpha1.WorkloadSpec{
		//SimulationType: "io",
		Duration: 10,
		//Intensity: 15,
	}
	OpSpec = utils.OperatorSpec{
		MaxWorkerInstances: 1,
		MemoryLimit:        500,
		CPULimit:           128,
		IsSharded:          false,
		ShardCount:         1,
	}

	defaultIntensityMap = map[string]int{
		"io":     15, // MiB to write and read
		"cpu":    0,
		"memory": 100, // MiB to allocate and deallocate
		"sleep":  0,
	}
)

func RunExperiment(dbConn *sql.DB) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Listen for SIGINT (Ctrl+C)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	// Launch a goroutine that cancels the context when SIGINT is detected
	go func() {
		<-c
		logrus.Info("Received Ctrl+C. Starting cleanup...")
		if err := dbConn.Close(); err != nil {
			logrus.Errorf("Database close error: %v", err)
		}

		if err := cleanupResources(ctx); err != nil {
			logrus.Errorf("Cleanup error: %v", err)
		}

		logrus.Info("Cleanup finished successfully. Exiting...")
		cancel()
	}()

	metricsDone := make(chan bool)
	go metrics.FetchAndStoreMetrics(dbConn, ctx, metricsDone)

	if err := experimentCycle(ctx, dbConn); err != nil {
		logrus.Errorf("Experiment error: %v", err)
		return err
	}

	<-metricsDone

	// Clear the interrupt handle as we're about to exit
	signal.Stop(c)

	logrus.Info("Experiment and cleanup finished successfully.")
	return nil
}

func experimentCycle(ctx context.Context, dbConn *sql.DB) error {
	for NumResources := setup.Experiment.StartResources; NumResources <= setup.Experiment.EndResources; NumResources++ {
		for _, st := range strings.Split(setup.Experiment.TaskTypes, ",") {
			WorkloadSpec.SimulationType = st
			WorkloadSpec.Intensity = defaultIntensityMap[st]

			logrus.Info("----------------------------------------")
			logrus.Infof("Running %s simulation with %d intensity...", st, WorkloadSpec.Intensity)
			if err := runSingleExperiment(ctx, dbConn, NumResources); err != nil {
				return err
			}
		}
	}
	return nil
}

func runSingleExperiment(ctx context.Context, dbConn *sql.DB, NumResources int) error {

	// Validate that the namespace is clear
	if err := ensureNamespaceClear(ctx); err != nil {
		return err
	}

	// Validate that the memory usage is below the threshold
	if err := ensureEnoughMemory(); err != nil {
		return err
	}

	var resourcesCreated []*simulationv1alpha1.Workload

	// Sleep for 5 seconds before starting the experiment
	time.Sleep(5 * time.Second)

	startTime := time.Now()

	logrus.Info("STARTING THE RUN...")

	// Iterate over the number of resources to create
	for i := 0; i < NumResources; i++ {
		logrus.WithFields(
			logrus.Fields{
				"Type":     WorkloadSpec.SimulationType,
				"Resource": i,
			})

		workload := simulationv1alpha1.Workload{
			Spec:       WorkloadSpec,
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("workload-%d-%s", i, strings.ToLower(WorkloadSpec.SimulationType)), Namespace: "default"},
		}

		if err := setup.K8sClient.Create(ctx, &workload); err != nil {
			logrus.Errorf("Failed to create workload: %s", err)
			continue
		}
		logrus.Debugf("Workload %s created", workload.Name)
		resourcesCreated = append(resourcesCreated, &workload)
	}

	defer func(rsc []*simulationv1alpha1.Workload) {
		deleteCreatedResources(ctx, rsc)
	}(resourcesCreated)

	// Wait for resources to be reconciled
	endTime, err := waitForReconciliation(ctx, resourcesCreated)
	if err != nil {
		return fmt.Errorf("waiting for reconciliation: %s", err)
	}

	duration := endTime.Sub(startTime).Seconds()

	// Store the result
	if err := db.InsertResult(dbConn, NumResources, WorkloadSpec, OpSpec, duration); err != nil {
		return fmt.Errorf("failed to record experiment result: %s", err)
	}

	logrus.Infof("RUN COMPLETED with %d resources as %s simulation type in %f seconds", NumResources, WorkloadSpec.SimulationType, duration)
	return nil
}

func ensureNamespaceClear(ctx context.Context) error {
	// Poll the state every 5 seconds until the namespace is clear
	for {
		workloads := &simulationv1alpha1.WorkloadList{}
		err := setup.K8sClient.List(ctx, workloads)
		if err != nil {
			return fmt.Errorf("failed to list workloads: %v", err)
		}

		if len(workloads.Items) == 0 {
			break
		}

		logrus.Warn("Namespace is not clear of workloads. Retrying in 5 seconds...")
		time.Sleep(5 * time.Second)
	}
	return nil
}

func ensureEnoughMemory() error {
	for {
		ok, err := metrics.CheckMemory(60) // Set memory limit here
		if err != nil {
			logrus.Errorf("Memory check error: %v", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if ok {
			break
		}
		logrus.Warn("Memory usage is too high, waiting...")
		time.Sleep(5 * time.Second) // Wait 5 seconds before checking again
	}
	return nil
}

func waitForReconciliation(ctx context.Context, workloads []*simulationv1alpha1.Workload) (time.Time, error) {
	const nanoSecondsLayout = "2006-01-02T15:04:05.999999999Z07:00"
	var endTime time.Time
	logrus.Debug("Waiting for workloads to be reconciled...")

	// make copy of workloads
	workloadsCopy := append([]*simulationv1alpha1.Workload(nil), workloads...)

	if err := wait.PollUntilContextCancel(ctx, time.Second, true, func(ctx context.Context) (bool, error) {
		for i, workload := range workloadsCopy {
			updatedWorkload := &simulationv1alpha1.Workload{}
			if err := setup.K8sClient.Get(ctx, client.ObjectKey{
				Namespace: workload.Namespace, Name: workload.Name}, updatedWorkload); err != nil {
				_ = fmt.Errorf("getting workload: %s", err)
				return false, err
			}
			// Check if the 'Available' condition is True in the Status Conditions
			if !isConditionTrue(updatedWorkload.Status.Conditions, "Available") {
				continue
			}

			//get the first startTime from workload status
			//if i == 0 {
			//	startTime, _ = time.Parse(nanoSecondsLayout, updatedWorkload.Status.StartTime)
			//}

			// Since it's established the workload is reconciled and available, get the lastTransitionTime
			endTime, _ = time.Parse(nanoSecondsLayout, updatedWorkload.Status.EndTime)
			logrus.Debugf("Workload %s is reconciled at %s", workload.Name, endTime)

			// Remove the workload from the list
			workloadsCopy = append(workloadsCopy[:i], workloadsCopy[i+1:]...)
			break
		}
		return len(workloadsCopy) == 0, nil
	}); err != nil {
		return time.Time{}, fmt.Errorf("waiting for reconciliation: %s", err)
	}

	return endTime, nil
}

// getConditionTransitionTime checks the list of workload conditions and returns the lastTransitionTime
// of the given condition type.
func getConditionTransitionTime(conditions []metav1.Condition, conditionType string) string {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return condition.LastTransitionTime.String()
		}
	}
	return ""
}

// isConditionTrue checks if the specific condition in the list of conditions is True.
func isConditionTrue(conditions []metav1.Condition, conditionType string) bool {
	for _, condition := range conditions {
		if condition.Type == conditionType && condition.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}

func deleteCreatedResources(ctx context.Context, workloads []*simulationv1alpha1.Workload) {
	for _, workload := range workloads {
		if err := setup.K8sClient.Delete(ctx, workload); err != nil {
			logrus.Errorf("Failed to delete workload: %s", err)
		}
	}

	logrus.Info("Waiting for resources to be deleted...")

	// Wait for resources to be deleted
	err := wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		for _, workload := range workloads {
			updatedWorkload := &simulationv1alpha1.Workload{}
			if err := setup.K8sClient.Get(ctx, client.ObjectKey{
				Namespace: workload.Namespace, Name: workload.Name}, updatedWorkload); err != nil {
				if errors.IsNotFound(err) {
					return true, nil
				}
				return false, nil
			}
			logrus.Debugf("Workload %s still exists...", workload.Name)
		}
		return false, nil
	})
	if err != nil {
		logrus.Errorf("Error waiting for deletion: %s", err)
		return
	}

	logrus.Info("Resources deleted successfully")
}

func cleanupResources(ctx context.Context) error {
	logrus.Info("Starting to cleanup Workload resources...")

	workloads := &simulationv1alpha1.WorkloadList{}
	err := setup.K8sClient.List(ctx, workloads)
	if err != nil {
		return fmt.Errorf("failed to list workloads: %v", err)
	}

	for _, workload := range workloads.Items {
		err = setup.K8sClient.Delete(ctx, &workload)
		if err != nil {
			logrus.Errorf("failed to delete workload: %v", err)
			continue
		}
	}
	return nil
}
