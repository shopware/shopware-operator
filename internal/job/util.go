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
) (bool, error) {

	if job == nil {
		return false, fmt.Errorf("job to check is nil")
	}

	for _, container := range job.Spec.Template.Spec.Containers {
		if container.Name == job.Name {
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

			for _, pod := range pods.Items {
				for _, c := range pod.Status.ContainerStatuses {
					if c.Name == job.Name {
						if c.State.Terminated == nil {
							log.FromContext(ctx).Info("Setup not terminated still running")
							return false, nil
						}
						if c.State.Terminated.ExitCode != 0 {
							log.FromContext(ctx).
								Info("Setup job has not 0 as exit code, check setup")
							return false, fmt.Errorf(
								"Errors in setup: %s",
								c.State.Terminated.Reason,
							)
						}

						if c.State.Terminated.Reason == "Completed" {
							log.FromContext(ctx).Info("Setup job completed")
							return true, nil
						}
					}
				}
			}
		}
	}

	return false, nil
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
