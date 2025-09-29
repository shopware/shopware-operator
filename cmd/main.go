/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	zapz "go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/go-logr/zapr"
	shopv1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/config"
	"github.com/shopware/shopware-operator/internal/controller"
	"github.com/shopware/shopware-operator/internal/event"
	"github.com/shopware/shopware-operator/internal/event/nats"
	"github.com/shopware/shopware-operator/internal/logging"
	//+kubebuilder:scaffold:imports
)

var (
	scheme  = runtime.NewScheme()
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(shopv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	// Load configuration and set up flags
	cfg, err := config.LoadStoreConfig(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	logger := logging.NewLogger(cfg.LogLevel, cfg.LogFormat).
		With(zapz.String("service", "shopware-operator")).
		With(zapz.String("operator_version", version)).
		With(zapz.String("operator_date", date)).
		With(zapz.String("operator_commit", commit)).
		With(zapz.String("namespace", cfg.Namespace))
	setupLog := zapr.NewLogger(logger.Desugar()).WithName("setup")
	ctrl.SetLogger(setupLog)
	setupLog.Info("Config for operator", "config", cfg)

	if cfg.Namespace == "" {
		setupLog.Error(fmt.Errorf("namespace is not set correctly"), "missing env `NAMESPACE`")
		os.Exit(3)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		// Metrics:                 metricsserver.Options{BindAddress: cfg.MetricsAddr},
		HealthProbeBindAddress: cfg.ProbeAddr,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				cfg.Namespace: {},
			},
		},
		LeaderElection:          cfg.EnableLeaderElection,
		LeaderElectionID:        "d79142e5.shopware.com",
		LeaderElectionNamespace: cfg.Namespace,
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	nsClient := client.NewNamespacedClient(mgr.GetClient(), cfg.Namespace)

	// Event Registration
	var handlers []event.EventHandler

	if cfg.NatsHandler.Enable {
		n, err := nats.NewNatsEventServer(
			cfg.NatsHandler.Address,
			cfg.NatsHandler.NkeyFile,
			cfg.NatsHandler.CredentialsFile,
			cfg.NatsHandler.NatsTopic,
		)
		if err != nil {
			setupLog.Error(err, "unable to create NATS event server. Skip event publishing")
		} else {
			setupLog.Info("Nats connection established")
			handlers = append(handlers, n)
		}
	}

	// Cleanup all event handlers on exit
	defer func() {
		for _, handler := range handlers {
			handler.Close()
		}
	}()

	if err = (&controller.StoreReconciler{
		Logger:               logger,
		Client:               nsClient,
		EventHandlers:        handlers,
		Scheme:               mgr.GetScheme(),
		Recorder:             mgr.GetEventRecorderFor(fmt.Sprintf("shopware-controller-%s", cfg.Namespace)),
		DisableServiceChecks: cfg.DisableChecks,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create store controller", "controller", "Store")
		os.Exit(1)
	}
	if err = (&controller.StoreExecReconciler{
		Client:   nsClient,
		Logger:   logger,
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor(fmt.Sprintf("shopware-controller-%s", cfg.Namespace)),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create exec controller", "controller", "StoreExec")
		os.Exit(1)
	}
	if err = (&controller.StoreSnapshotCreateReconciler{
		Client:        nsClient,
		Logger:        logger,
		EventHandlers: handlers,
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor(fmt.Sprintf("shopware-controller-%s", cfg.Namespace)),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create snapshot create controller", "controller", "StoreSnapshot")
		os.Exit(1)
	}
	if err = (&controller.StoreSnapshotRestoreReconciler{
		Client:        nsClient,
		Logger:        logger,
		EventHandlers: handlers,
		Scheme:        mgr.GetScheme(),
		Recorder:      mgr.GetEventRecorderFor(fmt.Sprintf("shopware-controller-%s", cfg.Namespace)),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create snapshot restore controller", "controller", "StoreSnapshot")
		os.Exit(1)
	}
	if err = (&controller.StoreDebugInstanceReconciler{
		Client:   nsClient,
		Logger:   logger,
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor(fmt.Sprintf("shopware-controller-%s", cfg.Namespace)),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create instance controller", "controller", "StoreDebugInstance")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	defer func() {
		if err := recover(); err != nil {
			logger.Fatal("Panic occurred", zapz.Any("error", err))
		}
	}()

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
