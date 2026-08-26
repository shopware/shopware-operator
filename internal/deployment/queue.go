package deployment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/k8s"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	adminContainerName    = "shopware-admin"
	queueStatsExecTimeout = 15 * time.Second
)

var messengerStatsCommand = []string{"php", "bin/console", "messenger:stats", "--format=json", "--no-debug"}

func GetAdminQueueStats(
	ctx context.Context,
	c client.Client,
	clientset *kubernetes.Clientset,
	restConfig *rest.Config,
	store v1.Store,
) ([]v1.QueueTransportStats, []string, error) {
	pod, err := getRunningAdminPod(ctx, c, store)
	if err != nil {
		return nil, nil, err
	}

	execCtx, cancel := context.WithTimeout(ctx, queueStatsExecTimeout)
	defer cancel()

	stdout, stderr, err := k8s.ExecInPod(
		execCtx,
		clientset,
		restConfig,
		pod.Namespace,
		pod.Name,
		adminContainerName,
		messengerStatsCommand,
	)
	if err != nil {
		if trimmed := strings.TrimSpace(stderr); trimmed != "" {
			return nil, nil, fmt.Errorf("%w (stderr: %s)", err, trimmed)
		}
		return nil, nil, err
	}

	return parseMessengerStats([]byte(stdout))
}

func getRunningAdminPod(
	ctx context.Context,
	c client.Client,
	store v1.Store,
) (*corev1.Pod, error) {
	d := AdminDeployment(store)
	search := &appsv1.Deployment{}
	if err := c.Get(ctx, types.NamespacedName{
		Namespace: d.Namespace,
		Name:      d.Name,
	}, search); err != nil {
		return nil, err
	}

	selector, err := metav1.LabelSelectorAsSelector(search.Spec.Selector)
	if err != nil {
		return nil, err
	}

	pods := &corev1.PodList{}
	if err := c.List(ctx, pods,
		client.InNamespace(search.Namespace),
		client.MatchingLabelsSelector{Selector: selector},
	); err != nil {
		return nil, err
	}

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase != corev1.PodRunning || pod.DeletionTimestamp != nil {
			continue
		}
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				return pod, nil
			}
		}
	}

	return nil, fmt.Errorf("no ready admin pod found for store %s/%s", store.Namespace, store.Name)
}

func parseMessengerStats(raw []byte) ([]v1.QueueTransportStats, []string, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, nil, fmt.Errorf("empty messenger:stats output")
	}

	var uncountable []string
	var wrapper struct {
		Transports            json.RawMessage `json:"transports"`
		UncountableTransports []string        `json:"uncountable_transports"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && len(wrapper.Transports) > 0 {
		raw = wrapper.Transports
		uncountable = wrapper.UncountableTransports
	}

	var list []struct {
		Name      string `json:"name"`
		Transport string `json:"transport"`
		Count     *int64 `json:"count"`
		Size      *int64 `json:"size"`
	}
	if err := json.Unmarshal(raw, &list); err == nil {
		stats := make([]v1.QueueTransportStats, 0, len(list))
		for _, item := range list {
			name := item.Name
			if name == "" {
				name = item.Transport
			}
			var count int64
			if item.Count != nil {
				count = *item.Count
			} else if item.Size != nil {
				count = *item.Size
			}
			stats = append(stats, v1.QueueTransportStats{Name: name, Count: count})
		}
		return stats, uncountable, nil
	}

	var objects map[string]struct {
		Count *int64 `json:"count"`
		Size  *int64 `json:"size"`
	}
	if err := json.Unmarshal(raw, &objects); err == nil {
		stats := make([]v1.QueueTransportStats, 0, len(objects))
		for _, name := range slices.Sorted(maps.Keys(objects)) {
			var count int64
			if objects[name].Count != nil {
				count = *objects[name].Count
			} else if objects[name].Size != nil {
				count = *objects[name].Size
			}
			stats = append(stats, v1.QueueTransportStats{Name: name, Count: count})
		}
		return stats, uncountable, nil
	}

	var counts map[string]int64
	if err := json.Unmarshal(raw, &counts); err == nil {
		stats := make([]v1.QueueTransportStats, 0, len(counts))
		for _, name := range slices.Sorted(maps.Keys(counts)) {
			stats = append(stats, v1.QueueTransportStats{Name: name, Count: counts[name]})
		}
		return stats, uncountable, nil
	}

	preview := string(raw)
	if len(preview) > 200 {
		preview = preview[:200]
	}
	return nil, nil, fmt.Errorf("unexpected messenger:stats output: %s", preview)
}
