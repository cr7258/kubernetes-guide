/*
Copyright 2017 The Kubernetes Authors.

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

package framework

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"google.golang.org/grpc/grpclog"
	"k8s.io/klog/v2"

	"k8s.io/kubernetes/pkg/util/env"
)

var etcdURL = ""

const installEtcd = `
Cannot find etcd, cannot run integration tests
Please see https://git.k8s.io/community/contributors/devel/sig-testing/integration-tests.md#install-etcd-dependency for instructions.

You can use 'hack/install-etcd.sh' to install a copy in third_party/.

`

// getEtcdPath returns a path to an etcd executable.
func getEtcdPath() (string, error) {
	return exec.LookPath("etcd")
}

// getAvailablePort returns a TCP port that is available for binding.
func getAvailablePort() (int, error) {
	l, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("could not bind to a port: %v", err)
	}
	// It is possible but unlikely that someone else will bind this port before we
	// get a chance to use it.
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// startEtcd executes an etcd instance. The returned function will signal the
// etcd process and wait for it to exit.
func startEtcd() (func(), error) {
	if runtime.GOARCH == "arm64" {
		os.Setenv("ETCD_UNSUPPORTED_ARCH", "arm64")
	}

	etcdURL = env.GetEnvAsStringOrFallback("KUBE_INTEGRATION_ETCD_URL", "http://127.0.0.1:2379")
	conn, err := net.Dial("tcp", strings.TrimPrefix(etcdURL, "http://"))
	if err == nil {
		klog.Infof("etcd already running at %s", etcdURL)
		conn.Close()
		return func() {}, nil
	}
	klog.V(1).Infof("could not connect to etcd: %v", err)

	currentURL, stop, err := RunCustomEtcd("integration_test_etcd_data", nil)
	if err != nil {
		return nil, err
	}

	etcdURL = currentURL
	os.Setenv("KUBE_INTEGRATION_ETCD_URL", etcdURL)

	return stop, nil
}

// RunCustomEtcd starts a custom etcd instance for test purposes.
func RunCustomEtcd(dataDir string, customFlags []string) (url string, stopFn func(), err error) {
	// TODO: Check for valid etcd version.
	etcdPath, err := getEtcdPath()
	if err != nil {
		fmt.Fprint(os.Stderr, installEtcd)
		return "", nil, fmt.Errorf("could not find etcd in PATH: %v", err)
	}
	etcdPort, err := getAvailablePort()
	if err != nil {
		return "", nil, fmt.Errorf("could not get a port: %v", err)
	}
	customURL := fmt.Sprintf("http://127.0.0.1:%d", etcdPort)

	klog.Infof("starting etcd on %s", customURL)

	etcdDataDir, err := ioutil.TempDir(os.TempDir(), dataDir)
	if err != nil {
		return "", nil, fmt.Errorf("unable to make temp etcd data dir %s: %v", dataDir, err)
	}
	klog.Infof("storing etcd data in: %v", etcdDataDir)

	ctx, cancel := context.WithCancel(context.Background())
	args := []string{
		"--data-dir",
		etcdDataDir,
		"--listen-client-urls",
		customURL,
		"--advertise-client-urls",
		customURL,
		"--listen-peer-urls",
		"http://127.0.0.1:0",
		"-log-level",
		"warn", // set to info or debug for more logs
	}
	args = append(args, customFlags...)
	cmd := exec.CommandContext(ctx, etcdPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	stop := func() {
		cancel()
		err := cmd.Wait()
		klog.Infof("etcd exit status: %v", err)
		err = os.RemoveAll(etcdDataDir)
		if err != nil {
			klog.Warningf("error during etcd cleanup: %v", err)
		}
	}

	// Quiet etcd logs for integration tests
	// Comment out to get verbose logs if desired
	grpclog.SetLoggerV2(grpclog.NewLoggerV2(ioutil.Discard, ioutil.Discard, os.Stderr))

	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("failed to run etcd: %v", err)
	}

	var i int32 = 1
	const pollCount = int32(300)

	for i <= pollCount {
		conn, err := net.DialTimeout("tcp", strings.TrimPrefix(customURL, "http://"), 1*time.Second)
		if err == nil {
			conn.Close()
			break
		}

		if i == pollCount {
			stop()
			return "", nil, fmt.Errorf("could not start etcd")
		}

		time.Sleep(100 * time.Millisecond)
		i = i + 1
	}

	return customURL, stop, nil
}

// EtcdMain starts an etcd instance before running tests.
func EtcdMain(tests func() int) {
	stop, err := startEtcd()
	if err != nil {
		klog.Fatalf("cannot run integration tests: unable to start etcd: %v", err)
	}
	result := tests()
	stop() // Don't defer this. See os.Exit documentation.
	os.Exit(result)
}

// GetEtcdURL returns the URL of the etcd instance started by EtcdMain.
func GetEtcdURL() string {
	return etcdURL
}
