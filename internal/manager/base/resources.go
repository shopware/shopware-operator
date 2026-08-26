package base

import (
	"context"
	"fmt"

	v1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/cronjob"
	"github.com/shopware/shopware-operator/internal/deployment"
	"github.com/shopware/shopware-operator/internal/hpa"
	"github.com/shopware/shopware-operator/internal/httproute"
	"github.com/shopware/shopware-operator/internal/ingress"
	"github.com/shopware/shopware-operator/internal/job"
	"github.com/shopware/shopware-operator/internal/k8s"
	"github.com/shopware/shopware-operator/internal/pdb"
	"github.com/shopware/shopware-operator/internal/secret"
	"github.com/shopware/shopware-operator/internal/service"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policy "k8s.io/api/policy/v1"
)

func (b *Base) EnsureAppSecrets(ctx context.Context, store *v1.Store) error {
	storeSecret, err := secret.EnsureStoreSecret(ctx, b.Client, b.Recorder, store)
	if err != nil {
		return fmt.Errorf("app secrets: %w", err)
	}

	if err := k8s.EnsureObjectWithHash(ctx, b.Client, nil, storeSecret, b.Scheme); err != nil {
		return fmt.Errorf("ensure store secret: %w", err)
	}

	return nil
}

func (b *Base) ReconcileServices(ctx context.Context, store *v1.Store) (err error) {
	objs := []*corev1.Service{
		service.StorefrontService(*store),
		service.AdminService(*store),
	}

	var changed bool
	for _, obj := range objs {
		if changed, err = k8s.HasObjectChanged(ctx, b.Client, obj); err != nil {
			return fmt.Errorf("reconcile unready svc: %w", err)
		}

		if changed {
			b.Eventf(store, "Diff service hash",
				"Update Store %s service in namespace %s for %s. Diff hash",
				store.Name,
				store.Namespace,
				obj.Labels["app"])
			if err := k8s.EnsureService(ctx, b.Client, store, obj, b.Scheme, true); err != nil {
				return fmt.Errorf("reconcile unready services: %w", err)
			}
		}
	}

	return nil
}

func (b *Base) ReconcileIngress(ctx context.Context, store *v1.Store) (err error) {
	if !store.Spec.Network.EnabledIngress {
		if err := ingress.DeleteStoreIngress(ctx, b.Client, *store); err != nil {
			return fmt.Errorf("delete ingress: %w", err)
		}
		return nil
	}

	var changed bool
	obj := ingress.StoreIngress(*store)

	if changed, err = k8s.HasObjectChanged(ctx, b.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready ingress: %w", err)
	}

	if changed {
		b.Eventf(store, "Diff ingress hash",
			"Update Store %s ingress in namespace %s. Diff hash",
			store.Name,
			store.Namespace)
		if err := k8s.EnsureIngress(ctx, b.Client, store, obj, b.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready ingress: %w", err)
		}
	}

	return nil
}

func (b *Base) ReconcileHTTPRoute(ctx context.Context, store *v1.Store) (err error) {
	if !store.Spec.Network.EnabledGateway {
		if err := httproute.DeleteStoreHTTPRoute(ctx, b.Client, *store); err != nil {
			return fmt.Errorf("delete httproute: %w", err)
		}
		return nil
	}

	var changed bool
	obj := httproute.StoreHTTPRoute(*store)

	if changed, err = k8s.HasObjectChanged(ctx, b.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready httproute: %w", err)
	}

	if changed {
		b.Eventf(store, "Diff httproute hash",
			"Update Store %s httproute in namespace %s. Diff hash",
			store.Name,
			store.Namespace)
		if err := k8s.EnsureHTTPRoute(ctx, b.Client, store, obj, b.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready httproute: %w", err)
		}
	}

	return nil
}

func (b *Base) ReconcilePDB(ctx context.Context, store *v1.Store) (err error) {
	var changed bool

	objs := []*policy.PodDisruptionBudget{
		pdb.AdminPDB(*store),
		pdb.StorefrontPDB(*store),
		pdb.WorkerPDB(*store),
	}

	for _, obj := range objs {
		if changed, err = k8s.HasObjectChanged(ctx, b.Client, obj); err != nil {
			return fmt.Errorf("reconcile unready pdb: %w", err)
		}

		if changed {
			b.Eventf(store, "Diff pdb hash",
				"Update Store %s pdb in namespace %s. Diff hash",
				store.Name,
				store.Namespace)
			if err := k8s.EnsurePDB(ctx, b.Client, store, obj, b.Scheme, true); err != nil {
				return fmt.Errorf("reconcile unready pdb: %w", err)
			}
		}
	}

	return nil
}

func (b *Base) ReconcileDeployment(ctx context.Context, store *v1.Store) (err error) {
	var changed bool

	workers, err := deployment.WorkerDeployments(*store)
	if err != nil {
		return fmt.Errorf("worker deployments: %w", err)
	}

	objs := []*appsv1.Deployment{
		deployment.StorefrontDeployment(*store),
		deployment.AdminDeployment(*store),
	}
	objs = append(objs, workers...)

	for _, obj := range objs {
		if changed, err = k8s.HasObjectChanged(ctx, b.Client, obj); err != nil {
			return fmt.Errorf("reconcile unready deployment: %w", err)
		}

		if changed {
			b.Eventf(store, "Diff deployment hash",
				"Update Store %s deployment in namespace %s for %s. Diff hash",
				store.Name,
				store.Namespace,
				obj.Labels["app"])
			if err := k8s.EnsureDeployment(ctx, b.Client, store, obj, b.Scheme, true); err != nil {
				return fmt.Errorf("reconcile unready deployment: %w", err)
			}
		}
	}

	if err := deployment.CleanupObsoleteWorkerDeployments(ctx, b.Client, *store); err != nil {
		return fmt.Errorf("cleanup worker deployments: %w", err)
	}

	return nil
}

func (b *Base) ReconcileHorizontalPodAutoscaler(ctx context.Context, store *v1.Store) (err error) {
	if !store.Spec.HorizontalPodAutoscaler.Enabled {
		return nil
	}

	var changed bool
	obj := hpa.StoreHPA(*store)

	if changed, err = k8s.HasObjectChanged(ctx, b.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready deployment: %w", err)
	}

	if changed {
		b.Eventf(store, "Diff hpa hash",
			"Update Store %s hpa in namespace %s. Diff hash",
			store.Name,
			store.Namespace)
		if err := k8s.EnsureHPA(ctx, b.Client, store, obj, b.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready hpa: %w", err)
		}
	}

	return nil
}

func (b *Base) ReconcileSetupJob(ctx context.Context, store *v1.Store) (err error) {
	var changed bool
	obj := job.SetupJob(*store)

	if changed, err = k8s.HasObjectChanged(ctx, b.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready setup job: %w", err)
	}

	if changed {
		b.Eventf(store, "Diff setup job hash",
			"Update Store %s setup job in namespace %s. Diff hash",
			store.Name,
			store.Namespace)
		if err := k8s.EnsureJob(ctx, b.Client, store, obj, b.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready setup job: %w", err)
		}
	}

	return nil
}

func (b *Base) ReconcileMigrationJob(ctx context.Context, store *v1.Store) (err error) {
	var changed bool
	obj := job.MigrationJob(*store)

	if changed, err = k8s.HasObjectChanged(ctx, b.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready migrate job: %w", err)
	}

	if changed {
		b.Eventf(store, "Diff migrate job hash",
			"Update Store %s migrate job in namespace %s. Diff hash",
			store.Name,
			store.Namespace)
		if err := k8s.EnsureJob(ctx, b.Client, store, obj, b.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready migrate job: %w", err)
		}
	}

	return nil
}

func (b *Base) ReconcileScheduledTask(ctx context.Context, store *v1.Store) (err error) {
	var changed bool
	obj := cronjob.ScheduledTaskJob(*store)

	if changed, err = k8s.HasObjectChanged(ctx, b.Client, obj); err != nil {
		return fmt.Errorf("reconcile unready setup job: %w", err)
	}

	if changed {
		b.Eventf(store, "Diff setup job hash",
			"Update Store %s scheduled task job in namespace %s. Diff hash",
			store.Name,
			store.Namespace)
		if err := k8s.EnsureCronJob(ctx, b.Client, store, obj, b.Scheme, true); err != nil {
			return fmt.Errorf("reconcile unready scheduled task job: %w", err)
		}
	}

	return nil
}
