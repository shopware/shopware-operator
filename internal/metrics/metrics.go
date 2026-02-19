package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	v1 "github.com/shopware/shopware-operator/api/v1"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	storeState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shopware_store_state",
		Help: "Current state of a Shopware store (1 for active state, 0 otherwise)",
	}, []string{"store", "namespace", "state"})

	storeCurrentImage = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shopware_store_current_image",
		Help: "Current image of a Shopware store",
	}, []string{"store", "namespace", "image"})

	storeDeploymentReplicasAvailable = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shopware_store_deployment_replicas_available",
		Help: "Available replica count per deployment type",
	}, []string{"store", "namespace", "deployment_type"})

	storeDeploymentReplicasDesired = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shopware_store_deployment_replicas_desired",
		Help: "Desired replica count per deployment type",
	}, []string{"store", "namespace", "deployment_type"})

	storeDeploymentState = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shopware_store_deployment_state",
		Help: "Current state of a store deployment (1 for active state, 0 otherwise)",
	}, []string{"store", "namespace", "deployment_type", "state"})

	storeUsageDataConsent = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shopware_store_usage_data_consent",
		Help: "Usage data consent status (1 for allowed, 0 for revoked)",
	}, []string{"store", "namespace"})

	storeHPAEnabled = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shopware_store_hpa_enabled",
		Help: "Whether the HorizontalPodAutoscaler is enabled (1) or disabled (0)",
	}, []string{"store", "namespace"})

	storeHPAMinReplicas = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shopware_store_hpa_min_replicas",
		Help: "HPA minimum replicas",
	}, []string{"store", "namespace"})

	storeHPAMaxReplicas = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shopware_store_hpa_max_replicas",
		Help: "HPA maximum replicas",
	}, []string{"store", "namespace"})

	storeScheduledTaskSuspended = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shopware_store_scheduled_task_suspended",
		Help: "Whether the scheduled task CronJob is suspended (1) or active (0)",
	}, []string{"store", "namespace"})

	storeScheduledTaskLastRunStatus = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shopware_store_scheduled_task_last_run_status",
		Help: "Status of the latest scheduled task run (1 for success, -1 for failure, 0 for unknown/no runs)",
	}, []string{"store", "namespace"})

	storeScheduledTaskLastSuccessTime = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "shopware_store_scheduled_task_last_success_timestamp",
		Help: "Unix timestamp of the last successful scheduled task run",
	}, []string{"store", "namespace"})

	allStates = []v1.StatefulAppState{
		v1.StateWait,
		v1.StateSetup,
		v1.StateSetupError,
		v1.StateInitializing,
		v1.StateMigration,
		v1.StateMigrationError,
		v1.StateReady,
	}

	allDeploymentStates = []v1.DeploymentState{
		v1.DeploymentStateUnknown,
		v1.DeploymentStateError,
		v1.DeploymentStateNotFound,
		v1.DeploymentStateRunning,
		v1.DeploymentStateScaling,
	}
)

func init() {
	metrics.Registry.MustRegister(
		storeState,
		storeCurrentImage,
		storeDeploymentReplicasAvailable,
		storeDeploymentReplicasDesired,
		storeDeploymentState,
		storeUsageDataConsent,
		storeHPAEnabled,
		storeHPAMinReplicas,
		storeHPAMaxReplicas,
		storeScheduledTaskSuspended,
		storeScheduledTaskLastRunStatus,
		storeScheduledTaskLastSuccessTime,
	)
}

// UpdateStoreMetrics sets all gauge values from a Store's status.
func UpdateStoreMetrics(store *v1.Store) {
	name := store.Name
	ns := store.Namespace

	// Store state
	for _, s := range allStates {
		val := float64(0)
		if store.Status.State == s {
			val = 1
		}
		storeState.WithLabelValues(name, ns, string(s)).Set(val)
	}

	// Current image
	if store.Status.CurrentImageTag != "" {
		// Delete old image labels, then set new one
		storeCurrentImage.DeletePartialMatch(prometheus.Labels{
			"store":     name,
			"namespace": ns,
		})
		storeCurrentImage.WithLabelValues(name, ns, store.Status.CurrentImageTag).Set(1)
	}

	// Usage data consent
	val := float64(0)
	if store.Spec.ShopConfiguration.UsageDataConsent == "allowed" {
		val = 1
	}
	storeUsageDataConsent.WithLabelValues(name, ns).Set(val)

	// HPA
	hpa := store.Spec.HorizontalPodAutoscaler
	if hpa.Enabled {
		storeHPAEnabled.WithLabelValues(name, ns).Set(1)
		storeHPAMaxReplicas.WithLabelValues(name, ns).Set(float64(hpa.MaxReplicas))
		if hpa.MinReplicas != nil {
			storeHPAMinReplicas.WithLabelValues(name, ns).Set(float64(*hpa.MinReplicas))
		} else {
			storeHPAMinReplicas.WithLabelValues(name, ns).Set(0)
		}
	} else {
		storeHPAEnabled.WithLabelValues(name, ns).Set(0)
		storeHPAMinReplicas.WithLabelValues(name, ns).Set(0)
		storeHPAMaxReplicas.WithLabelValues(name, ns).Set(0)
	}

	// Deployment metrics
	setDeploymentMetrics(name, ns, "admin", store.Status.AdminState)
	setDeploymentMetrics(name, ns, "storefront", store.Status.StorefrontState)
	setDeploymentMetrics(name, ns, "worker", store.Status.WorkerState)
}

func setDeploymentMetrics(name, ns, deploymentType string, cond v1.DeploymentCondition) {
	// Parse available/desired from the Ready field (format: "available/desired")
	var available, desired int
	if cond.Ready != "" {
		fmtScan(cond.Ready, &available, &desired)
	}

	storeDeploymentReplicasAvailable.WithLabelValues(name, ns, deploymentType).Set(float64(available))
	storeDeploymentReplicasDesired.WithLabelValues(name, ns, deploymentType).Set(float64(cond.StoreReplicas))

	// Deployment state
	for _, s := range allDeploymentStates {
		val := float64(0)
		if cond.State == s {
			val = 1
		}
		storeDeploymentState.WithLabelValues(name, ns, deploymentType, string(s)).Set(val)
	}
}

func fmtScan(ready string, available, desired *int) {
	// Parse "X/Y" format
	for i, c := range ready {
		if c == '/' {
			*available = atoi(ready[:i])
			*desired = atoi(ready[i+1:])
			return
		}
	}
}

func atoi(s string) int {
	n := 0
	for _, c := range s {
		if c >= '0' && c <= '9' {
			n = n*10 + int(c-'0')
		}
	}
	return n
}

// UpdateScheduledTaskMetrics sets metrics for the scheduled task CronJob.
func UpdateScheduledTaskMetrics(store *v1.Store, cronJob *batchv1.CronJob) {
	name := store.Name
	ns := store.Namespace

	if cronJob == nil {
		storeScheduledTaskSuspended.WithLabelValues(name, ns).Set(0)
		storeScheduledTaskLastRunStatus.WithLabelValues(name, ns).Set(0)
		storeScheduledTaskLastSuccessTime.WithLabelValues(name, ns).Set(0)
		return
	}

	// Suspended
	if cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend {
		storeScheduledTaskSuspended.WithLabelValues(name, ns).Set(1)
	} else {
		storeScheduledTaskSuspended.WithLabelValues(name, ns).Set(0)
	}

	// Last run status: compare LastSuccessfulTime vs LastScheduleTime.
	// If last successful time >= last schedule time, the latest run succeeded.
	// If last schedule time is after last successful time, the job either
	// failed or is still running. Only report failure when no jobs are
	// currently active; otherwise report 0 (in progress / unknown).
	jobActive := len(cronJob.Status.Active) > 0
	switch {
	case cronJob.Status.LastSuccessfulTime != nil && cronJob.Status.LastScheduleTime != nil:
		if !cronJob.Status.LastSuccessfulTime.Before(cronJob.Status.LastScheduleTime) {
			storeScheduledTaskLastRunStatus.WithLabelValues(name, ns).Set(1)
		} else if jobActive {
			storeScheduledTaskLastRunStatus.WithLabelValues(name, ns).Set(0)
		} else {
			storeScheduledTaskLastRunStatus.WithLabelValues(name, ns).Set(-1)
		}
	case cronJob.Status.LastSuccessfulTime != nil:
		storeScheduledTaskLastRunStatus.WithLabelValues(name, ns).Set(1)
	case cronJob.Status.LastScheduleTime != nil:
		// Scheduled but never succeeded — could still be running
		if jobActive {
			storeScheduledTaskLastRunStatus.WithLabelValues(name, ns).Set(0)
		} else {
			storeScheduledTaskLastRunStatus.WithLabelValues(name, ns).Set(-1)
		}
	default:
		storeScheduledTaskLastRunStatus.WithLabelValues(name, ns).Set(0)
	}

	// Last success timestamp
	if cronJob.Status.LastSuccessfulTime != nil {
		storeScheduledTaskLastSuccessTime.WithLabelValues(name, ns).Set(float64(cronJob.Status.LastSuccessfulTime.Unix()))
	} else {
		storeScheduledTaskLastSuccessTime.WithLabelValues(name, ns).Set(0)
	}
}

// RemoveStoreMetrics removes all metrics for a deleted store.
func RemoveStoreMetrics(store *v1.Store) {
	name := store.Name
	ns := store.Namespace

	match := prometheus.Labels{
		"store":     name,
		"namespace": ns,
	}

	storeState.DeletePartialMatch(match)
	storeCurrentImage.DeletePartialMatch(match)
	storeDeploymentReplicasAvailable.DeletePartialMatch(match)
	storeDeploymentReplicasDesired.DeletePartialMatch(match)
	storeDeploymentState.DeletePartialMatch(match)
	storeUsageDataConsent.DeletePartialMatch(match)
	storeHPAEnabled.DeletePartialMatch(match)
	storeHPAMinReplicas.DeletePartialMatch(match)
	storeHPAMaxReplicas.DeletePartialMatch(match)
	storeScheduledTaskSuspended.DeletePartialMatch(match)
	storeScheduledTaskLastRunStatus.DeletePartialMatch(match)
	storeScheduledTaskLastSuccessTime.DeletePartialMatch(match)
}
