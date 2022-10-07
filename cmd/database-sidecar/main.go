package main

import (
	"context"
	"flag"
	"os"

	databasev1alpha1 "github.com/pluralsh/database-interface-api/apis/database/v1alpha1"
	databasespec "github.com/pluralsh/database-interface-api/spec"
	"github.com/pluralsh/database-interface-controller/pkg/database"
	databaseaccess "github.com/pluralsh/database-interface-controller/pkg/database-access"
	"github.com/pluralsh/database-interface-controller/pkg/provisioner"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(databasev1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
}

func main() {
	var enableLeaderElection bool
	var debug bool
	var driverAddress string

	flag.BoolVar(&debug, "debug", true,
		"Enable debug")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&driverAddress, "driver-addr", "unix:///var/lib/database/database.sock", "path to unix domain socket where driver is listening")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	ctxInfo := context.Background()
	provisionerClient, err := provisioner.NewDefaultProvisionerClient(ctxInfo, driverAddress, debug)
	if err != nil {
		setupLog.Error(err, "unable to create provisioner client")
		os.Exit(1)
	}

	info, err := provisionerClient.DriverGetInfo(ctxInfo, &databasespec.DriverGetInfoRequest{})
	if err != nil {
		setupLog.Error(err, "unable to get driver info")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "1237ec41.plural.sh",
		MetricsBindAddress: "0",
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		os.Exit(1)
	}

	if err = (&database.Reconciler{
		Client:            mgr.GetClient(),
		Log:               ctrl.Log.WithName("controllers").WithName("Database"),
		DriverName:        info.Name,
		ProvisionerClient: provisionerClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Database")
		os.Exit(1)
	}
	if err = (&databaseaccess.Reconciler{
		Client:            mgr.GetClient(),
		Log:               ctrl.Log.WithName("controllers").WithName("DatabaseAccess"),
		DriverName:        info.Name,
		ProvisionerClient: provisionerClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DatabaseAccess")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()
	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}

}
