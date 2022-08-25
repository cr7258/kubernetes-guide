package main

import (
	"fmt"
	"github.com/valyala/fasthttp"
	"jtproxy/pkg/filters"
	"jtproxy/pkg/k8sconfig"
	"jtproxy/pkg/sysinit"
	v1 "k8s.io/api/networking/v1"
	"log"
	"os"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

func ProxyHandler(ctx *fasthttp.RequestCtx) {
	//代表匹配到了 path
	if getProxy := sysinit.GetRoute(ctx.Request); getProxy != nil {
		filters.ProxyFilters(getProxy.RequestFilters).Do(ctx)  //过滤
		getProxy.Proxy.ServeHTTP(ctx)                          //反代
		filters.ProxyFilters(getProxy.ResponseFilters).Do(ctx) //过滤
	} else {
		ctx.Response.SetStatusCode(404)
		ctx.Response.SetBodyString("404...")
	}

	// jtthink.ServeHTTP(ctx)

}

func main() {
	logf.SetLogger(zap.New())
	var mylog = logf.Log.WithName("jtproxy")
	mgr, err := manager.New(k8sconfig.K8sRestConfig(), manager.Options{})
	if err != nil {
		mylog.Error(err, "unable to set up manager")
		os.Exit(1)
	}

	proxyCtl := k8sconfig.NewJtProxyController()
	err = builder.ControllerManagedBy(mgr).
		For(&v1.Ingress{}).
		Watches(&source.Kind{
			Type: &v1.Ingress{},
		}, handler.Funcs{
			DeleteFunc: proxyCtl.OnDelete}).
		Complete(proxyCtl)

	if err != nil {
		mylog.Error(err, "unable to create manager")
		os.Exit(1)
	}
	if err = sysinit.InitConfig(); err != nil { //初始化  业务系统配置
		mylog.Error(err, "unable to load sysconfig")
		os.Exit(1)
	}
	errCh := make(chan error)
	// 启动控制器管理器
	go func() {
		if err = mgr.Start(signals.SetupSignalHandler()); err != nil {
			errCh <- err
		}
	}()
	// 启动网关
	go func() {
		if err = fasthttp.ListenAndServe(fmt.Sprintf(":%d", sysinit.SysConfig.Server.Port), ProxyHandler); err != nil {
			errCh <- err
		}
	}()
	getError := <-errCh
	log.Println(getError.Error())
}
