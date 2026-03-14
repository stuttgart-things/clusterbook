package main

import (
	"os"

	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/feature"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/ratelimiter"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/stuttgart-things/clusterbook/provider/apis/v1alpha1"
	ipassignmentctrl "github.com/stuttgart-things/clusterbook/provider/internal/controller/ipassignment"
	networkctrl "github.com/stuttgart-things/clusterbook/provider/internal/controller/network"
)

func main() {
	ctrl.SetLogger(zap.New())
	log := ctrl.Log.WithName("provider-clusterbook")

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		log.Error(err, "cannot add client-go scheme")
		os.Exit(1)
	}
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		log.Error(err, "cannot add provider scheme")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
	})
	if err != nil {
		log.Error(err, "cannot create controller manager")
		os.Exit(1)
	}

	o := controller.Options{
		Logger:                  logging.NewLogrLogger(log),
		MaxConcurrentReconciles: 1,
		Features:                &feature.Flags{},
		GlobalRateLimiter:       ratelimiter.NewGlobal(10),
	}

	if err := ipassignmentctrl.Setup(mgr, o); err != nil {
		log.Error(errors.Wrap(err, "cannot setup IPAssignment controller"), "")
		os.Exit(1)
	}

	if err := networkctrl.Setup(mgr, o); err != nil {
		log.Error(errors.Wrap(err, "cannot setup Network controller"), "")
		os.Exit(1)
	}

	log.Info("starting provider-clusterbook")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		log.Error(err, "cannot start controller manager")
		os.Exit(1)
	}
}
