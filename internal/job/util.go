package job

import (
	"context"
	"fmt"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// This is used when sidecars are able to run. We should always use this method for checking
func IsJobContainerDone(
	ctx context.Context,
	c client.Client,
	job *batchv1.Job,
	containerName string,
) (bool, error) {

	if job == nil {
		return false, fmt.Errorf("job to check is nil")
	}

	for _, container := range job.Spec.Template.Spec.Containers {
		if container.Name == containerName {
			selector, err := labels.ValidatedSelectorFromSet(job.Labels)
			if err != nil {
				return false, fmt.Errorf("get selector: %w", err)
			}

			listOptions := client.ListOptions{
				LabelSelector: selector,
				Namespace:     job.Namespace,
			}

			var pods corev1.PodList
			err = c.List(ctx, &pods, &listOptions)
			if err != nil {
				return false, fmt.Errorf("get pods: %w", err)
			}

			var isOneFinished bool
			for _, pod := range pods.Items {
				for _, c := range pod.Status.ContainerStatuses {
					if c.Name == containerName {
						if c.State.Terminated == nil {
							log.FromContext(ctx).Info("Job not terminated still running")
							continue
						}
						if c.State.Terminated.ExitCode != 0 {
							log.FromContext(ctx).
								Info("Job has not 0 as exit code, check job")
							continue
						}
						if c.State.Terminated.Reason == "Completed" {
							log.FromContext(ctx).Info("Job completed")
							isOneFinished = true
						}
					}
				}
			}
			if isOneFinished {
				return true, nil
			} else {
				return false, nil
			}
		}
	}

	err := fmt.Errorf("job not found in container")
	log.FromContext(ctx).Error(err, "job not found in container")
	return false, err
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

	log.FromContext(ctx).WithValues("jobs", jobs.Items).Info("Delete jobs")

	for _, job := range jobs.Items {
		err = c.Delete(ctx, &job, client.PropagationPolicy("Foreground"))
		if err != nil {
			return err
		}
	}

	return nil
}
