package k8sconfig

import (
	v1 "github.com/shenyisyn/dbcore/pkg/apis/dbconfig/v1"
	"github.com/shenyisyn/dbcore/pkg/controllers"
	appv1 "k8s.io/api/apps/v1"
	"log"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

// 初始化 控制器管理器
func InitManager() {
	logf.SetLogger(zap.New())
	mgr, err := manager.New(K8sRestConfig(),
		manager.Options{
			Logger: logf.Log.WithName("dbcore"),
		})
	if err != nil {
		log.Fatal("创建管理器失败:", err.Error())
	}
	err = v1.SchemeBuilder.AddToScheme(mgr.GetScheme())
	if err != nil {
		mgr.GetLogger().Error(err, "unable add schema")
		os.Exit(1)
	}
	//初始化控制器对象
	dbconfigController := controllers.NewDbConfigController()
	if err = builder.ControllerManagedBy(mgr).
		For(&v1.DbConfig{}).
		//监听 deployment
		Watches(&source.Kind{Type: &appv1.Deployment{}},
			handler.Funcs{
				DeleteFunc: dbconfigController.OnDelete,
			},
		).
		Complete(dbconfigController); err != nil {
		mgr.GetLogger().Error(err, "unable to create manager")
		os.Exit(1)
	}
	if err = mgr.Start(signals.SetupSignalHandler()); err != nil {
		mgr.GetLogger().Error(err, "unable to start manager")
	}
}
