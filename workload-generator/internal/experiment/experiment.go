package experiment

import (
	"context"
	"database/sql"
	"fmt"
	simulationv1alpha1 "github.com/930C/simulated-workload-operator/api/v1alpha1"
	"github.com/930C/workload-generator/internal/db"
	"github.com/930C/workload-generator/internal/metrics"
	"github.com/930C/workload-generator/internal/setup"
	"github.com/930C/workload-generator/internal/utils"
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
		fmt.Println("\nReceived Ctrl+C. Starting cleanup...")
		if err := dbConn.Close(); err != nil {
			fmt.Printf("Database close error: %v\n", err)
		}

		if err := cleanupResources(ctx); err != nil {
			fmt.Printf("Cleanup error: %v\n", err)
		}

		fmt.Println("Cleanup finished successfully. Exiting...")
		cancel()
	}()

	metricsDone := make(chan bool)
	go metrics.FetchAndStoreMetrics(dbConn, metricsDone)

	if err := experimentCycle(ctx, dbConn); err != nil {
		fmt.Printf("Experiment error: %v\n", err)
		return err
	}

	<-metricsDone

	// Clear the interrupt handle as we're about to exit
	signal.Stop(c)

	fmt.Println("Experiment and cleanup finished successfully.")
	return nil
}

func experimentCycle(ctx context.Context, dbConn *sql.DB) error {
	for NumResources := setup.Experiment.StartResources; NumResources <= setup.Experiment.EndResources; NumResources++ {
		for _, st := range strings.Split(setup.Experiment.TaskTypes, ",") {
			WorkloadSpec.SimulationType = st
			WorkloadSpec.Intensity = defaultIntensityMap[st]

			fmt.Printf("Running %s simulation with %d intensity...\n", st, WorkloadSpec.Intensity)
			if err := runSingleExperiment(ctx, dbConn, NumResources); err != nil {
				return err
			}

			// Wait for 10 seconds before starting the next experiment
			fmt.Printf("Waiting for 10 seconds before starting the next run...\n")
			time.Sleep(10 * time.Second)
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
	if err := ensureEnoughMemory(ctx); err != nil {
		return err
	}

	var resourcesCreated []*simulationv1alpha1.Workload

	fmt.Println("STARTING THE EXPERIMENT...")

	// Start time
	startTime := time.Now()

	// Iterate over the number of resources to create
	for i := 0; i < NumResources; i++ {
		workload := simulationv1alpha1.Workload{
			Spec:       WorkloadSpec,
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("workload-%d", i), Namespace: "default"},
		}

		if err := setup.K8sClient.Create(ctx, &workload); err != nil {
			fmt.Printf("Failed to create workload: %s\n", err)
			continue
		}
		resourcesCreated = append(resourcesCreated, &workload)
	}

	defer func(rsc []*simulationv1alpha1.Workload) {
		deleteCreatedResources(ctx, rsc)
	}(resourcesCreated)

	// Wait for resources to be reconciled
	if err := waitForReconciliation(ctx, resourcesCreated); err != nil {
		return fmt.Errorf("waiting for reconciliation: %s", err)
	}

	// Measure the time taken
	endTime := time.Now()
	duration := endTime.Sub(startTime).Seconds()

	// Store the result
	if err := db.InsertResult(dbConn, NumResources, WorkloadSpec, OpSpec, duration); err != nil {
		return fmt.Errorf("failed to record experiment result: %s", err)
	}

	fmt.Printf("RUN COMPLETED for %d resources with %s simulation type in %f seconds\n", NumResources, WorkloadSpec.SimulationType, duration)
	return nil
}

func ensureNamespaceClear(ctx context.Context) error {
	// Poll the state every 5 seconds until the namespace is clear
	for {
		workloads := &simulationv1alpha1.WorkloadList{}
		err := setup.K8sClient.List(ctx, workloads)
		if err != nil {
			return fmt.Errorf("Failed to list workloads: %v", err)
		}

		if len(workloads.Items) == 0 {
			break
		}

		fmt.Println("Namespace is not clear of workloads. Retrying in 5 seconds...")
		time.Sleep(5 * time.Second)
	}
	return nil
}

func ensureEnoughMemory(ctx context.Context) error {
	for {
		ok, err := metrics.CheckMemory(60) // Set memory limit here
		if err != nil {
			fmt.Printf("Memory check error: %v\n", err)
			time.Sleep(5 * time.Second)
			continue
		}
		if ok {
			break
		}
		fmt.Println("(Memory usage is too high, waiting...)")
		time.Sleep(5 * time.Second) // Wait 5 seconds before checking again
	}
	return nil
}

func waitForReconciliation(ctx context.Context, workloads []*simulationv1alpha1.Workload) error {
	return wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		for _, workload := range workloads {
			updatedWorkload := &simulationv1alpha1.Workload{}
			if err := setup.K8sClient.Get(ctx, client.ObjectKey{
				Namespace: workload.Namespace, Name: workload.Name}, updatedWorkload); err != nil {
				_ = fmt.Errorf("getting workload: %s", err)
				return false, err
			}
			// Check if the 'Available' condition is True in the Status Conditions
			if !isConditionTrue(updatedWorkload.Status.Conditions, "Available") {
				return false, nil
			}
		}
		return true, nil
	})
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
			fmt.Printf("Failed to delete workload: %s\n", err)
		}
	}

	fmt.Printf("Waiting for resources to be deleted...\n")

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
			if updatedWorkload.DeletionTimestamp != nil {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		fmt.Printf("Error waiting for deletion: %s\n", err)
		return
	}

	fmt.Println("Resources deleted successfully")
}

func cleanupResources(ctx context.Context) error {
	fmt.Println("Starting to cleanup Workload resources...")

	workloads := &simulationv1alpha1.WorkloadList{}
	err := setup.K8sClient.List(ctx, workloads)
	if err != nil {
		return fmt.Errorf("Failed to list workloads: %v", err)
	}

	for _, workload := range workloads.Items {
		err = setup.K8sClient.Delete(ctx, &workload)
		if err != nil {
			fmt.Printf("Failed to delete workload: %v\n", err)
			continue
		}
	}
	return nil
}
