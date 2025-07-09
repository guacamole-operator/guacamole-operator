/*
Copyright 2023.

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
	"log/slog"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/go-logr/logr"

	v1alpha1 "github.com/guacamole-operator/guacamole-operator/api/v1alpha1"
	"github.com/guacamole-operator/guacamole-operator/controllers"
	"github.com/guacamole-operator/guacamole-operator/internal/config"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var connectionConcurrentReconciles int
	var guacConcurrency int
	var usePriorityQueue bool

	flag.StringVar(&metricsAddr, "metrics-bind-address",
		config.EnvOrDefault("METRICS_BIND_ADDRESS", ":8080"),
		"The address the metric endpoint binds to.")

	flag.StringVar(&probeAddr, "health-probe-bind-address",
		config.EnvOrDefault("HEALTH_PROBE_BIND_ADDRESS", ":8081"),
		"The address the probe endpoint binds to.")

	flag.BoolVar(&enableLeaderElection, "leader-elect",
		config.EnvBoolOrDefault("LEADER_ELECT", false),
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	//nolint:mnd
	flag.IntVar(&connectionConcurrentReconciles, "connection-concurrent-reconciles",
		config.EnvIntOrDefault("CONNECTION_CONCURRENT_RECONCILES", 10),
		"Number of concurrent reconciles for connection resources.")

	//nolint:mnd
	flag.IntVar(&guacConcurrency, "guac-concurrency",
		config.EnvIntOrDefault("GUAC_CONCURRENCY", 100),
		"Number of concurrent requests to the Guacamole API.")

	flag.BoolVar(&usePriorityQueue, "priority-queue",
		config.EnvBoolOrDefault("PRIORITY_QUEUE", false),
		"Use controller-runtime's priority queue implementation.")

	flag.Parse()

	opts := slog.HandlerOptions{
		AddSource: true,
		Level:     slog.Level(-1),
	}
	handler := slog.NewJSONHandler(os.Stderr, &opts)

	logger := logr.FromSlogHandler(handler)
	ctrl.SetLogger(logger)

	var err error
	options := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		WebhookServer:          webhook.NewServer(webhook.Options{Port: 9443}), //nolint:mnd
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "643bc562.guacamole-operator.github.io",
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
	}

	// Set watch namespace. Defaults to cluster scope.
	options.Cache.DefaultNamespaces = map[string]cache.Config{
		getWatchNamespace(): {},
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), options)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.GuacamoleReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Guacamole")
		os.Exit(1)
	}
	if err = (&controllers.ConnectionReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		ConcurrentReconciles: connectionConcurrentReconciles,
		GuacConcurrency:      guacConcurrency,
		UsePriorityQueue:     usePriorityQueue,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Connection")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

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

// getWatchNamespace returns the namespace the operator should be watching for changes.
// Mainly for local testing purposes.
func getWatchNamespace() string {
	// WatchNamespaceEnvVar is the constant for env variable WATCH_NAMESPACE
	// which specifies the Namespace to watch.
	// An empty value means the operator is running with cluster scope.
	watchNamespaceEnvVar := "WATCH_NAMESPACE"

	ns, _ := os.LookupEnv(watchNamespaceEnvVar)
	return ns
}
