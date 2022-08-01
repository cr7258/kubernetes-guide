package store

import (
	"context"
	"encoding/json"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"log"
	"time"
)

const Etcd_Resource_Prefix = "/jtthink"

// 生成 key
// key的格式是
// /前缀/group/resources/namespace/提交的资源名称
func genKey(gvr schema.GroupVersionResource, form string) (string, error) {
	var m map[string]interface{}
	err := json.Unmarshal([]byte(form), &m)
	if err != nil {
		return "", err
	}
	var name, ns string
	if _name, ok := m["name"]; !ok {
		return "", fmt.Errorf("Name is requured")
	} else {
		name = _name.(string)
	}
	if _ns, ok := m["namespace"]; !ok { //这块还没想好怎么处理 ，写强制写上  namespace
		return "", fmt.Errorf("Namespace is requured")
	} else {
		ns = _ns.(string)
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s", Etcd_Resource_Prefix,
		gvr.Group,
		gvr.Resource,
		ns,
		name), nil
}

type EtcdStore struct {
	//cli     *clientv3.Client//  这一个属性是不需要的
	newFunc func() (*clientv3.Client, error) //创建 client 的函数
	timeout time.Duration                    //操作etcd时的超时时间
}

func NewEtcdStore() *EtcdStore {
	newFunc := func() (*clientv3.Client, error) {
		cli, err := clientv3.New(clientv3.Config{
			Endpoints:   []string{"localhost:2379"}, //直接写死的 ,配置什么的请大家自行搞定
			DialTimeout: 5 * time.Second,
		})
		if err != nil {
			return nil, err
		}
		return cli, nil
	}
	cli, err := newFunc()
	if err != nil {
		log.Fatalln(err)
	}
	defer cli.Close()
	//上面 就是做个连接测试 ,并不实际使用cli .下面每个操作函数单独去 初始化  -----这点和视频说的不同

	return &EtcdStore{timeout: time.Second * 3, newFunc: newFunc}
}
func (e *EtcdStore) Add(gvr schema.GroupVersionResource, form string) error {
	cli, err := e.newFunc()
	if err != nil {
		return err
	}
	defer cli.Close() //别忘了关闭
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()
	key, err := genKey(gvr, form)
	if err != nil {
		return err
	}
	_, err = cli.Put(ctx, key, form)
	return err
}

//暴露出 cli 创建函数
func (e *EtcdStore) NewClient() (*clientv3.Client, error) {
	cli, err := e.newFunc()
	if err != nil {
		return nil, err
	}
	return cli, nil
}
func (e *EtcdStore) Watch(cli *clientv3.Client, key string) clientv3.WatchChan {
	watcher := clientv3.NewWatcher(cli)
	return watcher.Watch(context.Background(), key)
}
