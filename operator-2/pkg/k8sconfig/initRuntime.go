package k8sconfig

import (
	v1 "github.com/shenyisyn/dbcore/pkg/apis/dbconfig/v1"
	"github.com/shenyisyn/dbcore/pkg/controllers"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

func InitManager() {
	logf.SetLogger(zap.New())
	mgr, err := manager.New(K8sRestConfig(), manager.Options{
		Logger: logf.Log.WithName("dbcore"),
	})
	if err != nil {
		mgr.GetLogger().Error(err, "unable to set up manager")
		os.Exit(1)
	}
	if err = v1.SchemeBuilder.AddToScheme(mgr.GetScheme()); err != nil {
		mgr.GetLogger().Error(err, "unable add scheme")
		os.Exit(1)
	}
	if err = builder.ControllerManagedBy(mgr).
		For(&v1.DbConfig{}).
		Complete(controllers.NewDbConfigController()); err != nil {
		mgr.GetLogger().Error(err, "unable to create manager")
		os.Exit(1)
	}
	if err = mgr.Start(signals.SetupSignalHandler()); err != nil {
		mgr.GetLogger().Error(err, "unable to start manager")
		os.Exit(1)
	}

}
