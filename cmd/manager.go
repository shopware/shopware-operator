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
	"flag"
	"fmt"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"

	"github.com/go-logr/zapr"
	"go.uber.org/zap"

	shopv1 "github.com/shopware/shopware-operator/api/v1"
	"github.com/shopware/shopware-operator/internal/controller"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(shopv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var debug bool
	var logStructured bool
	var disableChecks bool
	var probeAddr string
	var namespace string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.StringVar(&namespace, "namespace", "default", "The namespace in which the operator is running in")
	flag.BoolVar(&debug, "debug", false, "Set's the logger to debug with more logging output")
	flag.BoolVar(&logStructured, "log-structured", false, "Set's the logger to output with human logs")
	flag.BoolVar(&disableChecks, "disable-checks", false,
		"Disable the s3 connection check and the database connection check")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	flag.Parse()

	var cfg zap.Config

	if logStructured {
		cfg = zap.NewProductionConfig()
	} else {
		cfg = zap.NewDevelopmentConfig()
	}

	if debug {
		cfg.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}

	zlogger, err := cfg.Build()
	if err != nil {
		setupLog.Error(err, "setup zap logger")
		return
	}
	logger := zapr.NewLogger(zlogger)
	ctrl.SetLogger(logger)

	// Overwrite namespace when env is set, which is always set running in a cluster
	ns := os.Getenv("NAMESPACE")
	if ns != "" {
		namespace = ns
	}

	if namespace == "" {
		setupLog.Error(fmt.Errorf("namespace is not set correctly"), "missing env `NAMESPACE` or flag `--namespace`")
		os.Exit(3)
	}

	if disableChecks {
		setupLog.Info("S3 and database checks are disabled")
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		// Metrics:                 metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		Cache: cache.Options{
			DefaultNamespaces: map[string]cache.Config{
				namespace: {},
			},
		},
		LeaderElection:          enableLeaderElection,
		LeaderElectionID:        "d79142e5.shopware.com",
		LeaderElectionNamespace: namespace,
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

	nsClient := client.NewNamespacedClient(mgr.GetClient(), namespace)

	if err = (&controller.StoreReconciler{
		Client:               nsClient,
		Scheme:               mgr.GetScheme(),
		Recorder:             mgr.GetEventRecorderFor(fmt.Sprintf("shopware-controller-%s", namespace)),
		DisableServiceChecks: disableChecks,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create store controller", "controller", "Store")
		os.Exit(1)
	}
	if err = (&controller.StoreExecReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor(fmt.Sprintf("shopware-controller-%s", namespace)),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create exec controller", "controller", "StoreExec")
		os.Exit(1)
	}
	if err = (&controller.StoreSnapshotReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor(fmt.Sprintf("shopware-controller-%s", namespace)),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create snapshot controller", "controller", "StoreSnapshot")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	defer func() {
		if err := recover(); err != nil {
			zlogger.Fatal("Panic occurred", zap.Any("error", err))
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
