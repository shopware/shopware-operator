package k8s

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/shopware/shopware-operator/internal/tracing"
	"github.com/shopware/shopware-operator/internal/util"
	"go.opentelemetry.io/otel/attribute"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

func ExecInPod(
	ctx context.Context,
	clientset *kubernetes.Clientset,
	restConfig *rest.Config,
	namespace string,
	podName string,
	containerName string,
	command []string,
) (stdoutResult string, stderrResult string, err error) {
	ctx, span := tracing.Start(ctx, "k8s.ExecInPod",
		tracing.AttrNamespace.String(namespace),
		attribute.String("k8s.pod.name", podName),
		attribute.String("k8s.container.name", containerName),
		attribute.String("process.command_line", strings.Join(command, " ")),
	)
	defer func() {
		span.SetAttributes(
			attribute.Int("process.stdout.size", len(stdoutResult)),
			attribute.Int("process.stderr.size", len(stderrResult)),
		)
		if err != nil && stderrResult != "" {
			span.SetAttributes(attribute.String("process.stderr", util.Truncate(stderrResult, 1000)))
		}
		tracing.End(span, &err)
	}()

	req := clientset.CoreV1().RESTClient().
		Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName,
			Command:   command,
			Stdout:    true,
			Stderr:    true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(restConfig, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("create executor: %w", err)
	}

	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return stdout.String(), stderr.String(), fmt.Errorf("exec in pod %s: %w", podName, err)
	}

	return stdout.String(), stderr.String(), nil
}
