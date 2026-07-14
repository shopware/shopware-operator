package job

import (
	"context"
	"fmt"

	"github.com/shopware/shopware-operator/internal/logging"
	"go.uber.org/zap"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var ttlSecondsAfterFinished int32 = 86400 // Hardcoded to 1 day for now

type JobState struct {
	ExitCode int
	Running  bool
}

func (s JobState) HasErrors() bool {
	return s.ExitCode != 0
}

func (s JobState) IsDone() bool {
	return !s.Running
}

// This is used when sidecars are able to run. We should always use this method for checking
func IsJobContainerDone(
	ctx context.Context,
	c client.Client,
	job *batchv1.Job,
	containerName string,
) (JobState, error) {
	if job == nil {
		return JobState{}, fmt.Errorf("job to check is nil")
	}

	logger := logging.FromContext(ctx).With(zap.String("job", job.Name))

	// TODO: if job is created this returns an error JobNotFoundInContainer

	var errorStates []JobState
	for _, container := range job.Spec.Template.Spec.Containers {
		if container.Name == containerName {
			// Use the job's selector which matches the actual pods
			selector, err := metav1.LabelSelectorAsSelector(job.Spec.Selector)
			if err != nil {
				return JobState{}, fmt.Errorf("convert selector: %w", err)
			}

			listOptions := client.ListOptions{
				LabelSelector: selector,
				Namespace:     job.Namespace,
			}

			var pods corev1.PodList
			err = c.List(ctx, &pods, &listOptions)
			if err != nil {
				return JobState{}, fmt.Errorf("get pods: %w", err)
			}

			for _, pod := range pods.Items {
				if pod.Status.Phase == corev1.PodPending {
					logger.Infow("The job pod is still pending. Could be stuck, check the conditions", zap.Any("conditions", pod.Status.Conditions))
					return JobState{
						ExitCode: -1,
						Running:  true,
					}, nil
				}

				for _, c := range pod.Status.ContainerStatuses {
					if c.Name == containerName {
						logger.Infof("Found container for job `%s`", c.Name)
						if c.State.Terminated == nil {
							logger.Info("Job not terminated still running")
							return JobState{
								ExitCode: -1,
								Running:  true,
							}, nil
						}
						if c.State.Terminated.ExitCode != 0 {
							logger.With(zap.Int32("exitcode", c.State.Terminated.ExitCode)).
								Info("Job has not 0 as exit code, check job")
							errorStates = append(errorStates, JobState{
								ExitCode: int(c.State.Terminated.ExitCode),
								Running:  false,
							})
						}
						if c.State.Terminated.Reason == "Completed" {
							logger.Info("Job completed")
							return JobState{
								ExitCode: 0,
								Running:  false,
							}, nil
						}
					}
				}
			}
		}
	}

	if job.Status.Succeeded > 0 {
		logger.Info(fmt.Sprintf("job not found in container: %s. But job has succeeded continue with job done.", containerName))
		return JobState{
			ExitCode: 0,
			Running:  false,
		}, nil
	}

	if len(errorStates) > 0 {
		// Return the latest error state
		return errorStates[len(errorStates)-1], nil
	}

	if job.Status.Failed > 0 {
		logger.Info(fmt.Sprintf("job not found in container: %s. But job has failed.", containerName))
		return JobState{
			ExitCode: -404,
			Running:  false,
		}, nil
	}

	// We tried to detect if this is a timing issue, but unfortunately we could no verify this. So it's better to run into
	// an endless loop until the job is done. If the container name is not found it should be catched by an e2e test.
	logger.Debugw(
		"No result yet for job completed, this can be a timing problem but if the job never finishes this is a operator problem",
		zap.Any("job_status", job.Status), zap.Any("job_spec", job.Spec),
	)
	return JobState{
		Running: true,
	}, nil
}

func deleteJobsByLabel(
	ctx context.Context,
	c client.Client,
	namespace string,
	la map[string]string,
) error {
	selector, err := labels.ValidatedSelectorFromSet(la)
	if err != nil {
		return fmt.Errorf("get selector: %w", err)
	}

	listOptions := client.ListOptions{
		LabelSelector: selector,
		Namespace:     namespace,
	}

	var jobs batchv1.JobList
	err = c.List(ctx, &jobs, &listOptions)
	if err != nil {
		return fmt.Errorf("get jobs: %w", err)
	}

	logging.FromContext(ctx).With(zap.Any("jobs", jobs.Items)).Info("Delete jobs")

	for _, job := range jobs.Items {
		err = c.Delete(ctx, &job, client.PropagationPolicy("Foreground"))
		if err != nil {
			return err
		}
	}

	return nil
}
