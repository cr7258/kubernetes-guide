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

package devicemanager

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	cadvisorapi "github.com/google/cadvisor/info/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/record"
	pluginapi "k8s.io/kubelet/pkg/apis/deviceplugin/v1beta1"
	watcherapi "k8s.io/kubelet/pkg/apis/pluginregistration/v1"
	"k8s.io/kubernetes/pkg/kubelet/checkpointmanager"
	"k8s.io/kubernetes/pkg/kubelet/cm/devicemanager/checkpoint"
	plugin "k8s.io/kubernetes/pkg/kubelet/cm/devicemanager/plugin/v1beta1"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager"
	"k8s.io/kubernetes/pkg/kubelet/cm/topologymanager/bitmask"
	"k8s.io/kubernetes/pkg/kubelet/config"
	"k8s.io/kubernetes/pkg/kubelet/lifecycle"
	"k8s.io/kubernetes/pkg/kubelet/pluginmanager"
	schedulerframework "k8s.io/kubernetes/pkg/scheduler/framework"
)

const (
	testResourceName = "fake-domain/resource"
)

func newWrappedManagerImpl(socketPath string, manager *ManagerImpl) *wrappedManagerImpl {
	w := &wrappedManagerImpl{
		ManagerImpl: manager,
		callback:    manager.genericDeviceUpdateCallback,
	}
	w.socketdir, _ = filepath.Split(socketPath)
	w.server, _ = plugin.NewServer(socketPath, w, w)
	return w
}

type wrappedManagerImpl struct {
	*ManagerImpl
	socketdir string
	callback  func(string, []pluginapi.Device)
}

func (m *wrappedManagerImpl) PluginListAndWatchReceiver(r string, resp *pluginapi.ListAndWatchResponse) {
	var devices []pluginapi.Device
	for _, d := range resp.Devices {
		devices = append(devices, *d)
	}
	m.callback(r, devices)
}

func tmpSocketDir() (socketDir, socketName, pluginSocketName string, err error) {
	socketDir, err = ioutil.TempDir("", "device_plugin")
	if err != nil {
		return
	}
	socketName = socketDir + "/server.sock"
	pluginSocketName = socketDir + "/device-plugin.sock"
	os.MkdirAll(socketDir, 0755)
	return
}

func TestNewManagerImpl(t *testing.T) {
	socketDir, socketName, _, err := tmpSocketDir()
	topologyStore := topologymanager.NewFakeManager()
	require.NoError(t, err)
	defer os.RemoveAll(socketDir)
	_, err = newManagerImpl(socketName, nil, topologyStore)
	require.NoError(t, err)
	os.RemoveAll(socketDir)
}

func TestNewManagerImplStart(t *testing.T) {
	socketDir, socketName, pluginSocketName, err := tmpSocketDir()
	require.NoError(t, err)
	defer os.RemoveAll(socketDir)
	m, _, p := setup(t, []*pluginapi.Device{}, func(n string, d []pluginapi.Device) {}, socketName, pluginSocketName)
	cleanup(t, m, p)
	// Stop should tolerate being called more than once.
	cleanup(t, m, p)
}

func TestNewManagerImplStartProbeMode(t *testing.T) {
	socketDir, socketName, pluginSocketName, err := tmpSocketDir()
	require.NoError(t, err)
	defer os.RemoveAll(socketDir)
	m, _, p, _ := setupInProbeMode(t, []*pluginapi.Device{}, func(n string, d []pluginapi.Device) {}, socketName, pluginSocketName)
	cleanup(t, m, p)
}

// Tests that the device plugin manager correctly handles registration and re-registration by
// making sure that after registration, devices are correctly updated and if a re-registration
// happens, we will NOT delete devices; and no orphaned devices left.
func TestDevicePluginReRegistration(t *testing.T) {
	socketDir, socketName, pluginSocketName, err := tmpSocketDir()
	require.NoError(t, err)
	defer os.RemoveAll(socketDir)
	devs := []*pluginapi.Device{
		{ID: "Dev1", Health: pluginapi.Healthy},
		{ID: "Dev2", Health: pluginapi.Healthy},
	}
	devsForRegistration := []*pluginapi.Device{
		{ID: "Dev3", Health: pluginapi.Healthy},
	}
	for _, preStartContainerFlag := range []bool{false, true} {
		for _, getPreferredAllocationFlag := range []bool{false, true} {
			m, ch, p1 := setup(t, devs, nil, socketName, pluginSocketName)
			p1.Register(socketName, testResourceName, "")

			select {
			case <-ch:
			case <-time.After(5 * time.Second):
				t.Fatalf("timeout while waiting for manager update")
			}
			capacity, allocatable, _ := m.GetCapacity()
			resourceCapacity := capacity[v1.ResourceName(testResourceName)]
			resourceAllocatable := allocatable[v1.ResourceName(testResourceName)]
			require.Equal(t, resourceCapacity.Value(), resourceAllocatable.Value(), "capacity should equal to allocatable")
			require.Equal(t, int64(2), resourceAllocatable.Value(), "Devices are not updated.")

			p2 := plugin.NewDevicePluginStub(devs, pluginSocketName+".new", testResourceName, preStartContainerFlag, getPreferredAllocationFlag)
			err = p2.Start()
			require.NoError(t, err)
			p2.Register(socketName, testResourceName, "")

			select {
			case <-ch:
			case <-time.After(5 * time.Second):
				t.Fatalf("timeout while waiting for manager update")
			}
			capacity, allocatable, _ = m.GetCapacity()
			resourceCapacity = capacity[v1.ResourceName(testResourceName)]
			resourceAllocatable = allocatable[v1.ResourceName(testResourceName)]
			require.Equal(t, resourceCapacity.Value(), resourceAllocatable.Value(), "capacity should equal to allocatable")
			require.Equal(t, int64(2), resourceAllocatable.Value(), "Devices shouldn't change.")

			// Test the scenario that a plugin re-registers with different devices.
			p3 := plugin.NewDevicePluginStub(devsForRegistration, pluginSocketName+".third", testResourceName, preStartContainerFlag, getPreferredAllocationFlag)
			err = p3.Start()
			require.NoError(t, err)
			p3.Register(socketName, testResourceName, "")

			select {
			case <-ch:
			case <-time.After(5 * time.Second):
				t.Fatalf("timeout while waiting for manager update")
			}
			capacity, allocatable, _ = m.GetCapacity()
			resourceCapacity = capacity[v1.ResourceName(testResourceName)]
			resourceAllocatable = allocatable[v1.ResourceName(testResourceName)]
			require.Equal(t, resourceCapacity.Value(), resourceAllocatable.Value(), "capacity should equal to allocatable")
			require.Equal(t, int64(1), resourceAllocatable.Value(), "Devices of plugin previously registered should be removed.")
			p2.Stop()
			p3.Stop()
			cleanup(t, m, p1)
		}
	}
}

// Tests that the device plugin manager correctly handles registration and re-registration by
// making sure that after registration, devices are correctly updated and if a re-registration
// happens, we will NOT delete devices; and no orphaned devices left.
// While testing above scenario, plugin discovery and registration will be done using
// Kubelet probe based mechanism
func TestDevicePluginReRegistrationProbeMode(t *testing.T) {
	socketDir, socketName, pluginSocketName, err := tmpSocketDir()
	require.NoError(t, err)
	defer os.RemoveAll(socketDir)
	devs := []*pluginapi.Device{
		{ID: "Dev1", Health: pluginapi.Healthy},
		{ID: "Dev2", Health: pluginapi.Healthy},
	}
	devsForRegistration := []*pluginapi.Device{
		{ID: "Dev3", Health: pluginapi.Healthy},
	}

	m, ch, p1, _ := setupInProbeMode(t, devs, nil, socketName, pluginSocketName)

	// Wait for the first callback to be issued.
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.FailNow()
	}
	capacity, allocatable, _ := m.GetCapacity()
	resourceCapacity := capacity[v1.ResourceName(testResourceName)]
	resourceAllocatable := allocatable[v1.ResourceName(testResourceName)]
	require.Equal(t, resourceCapacity.Value(), resourceAllocatable.Value(), "capacity should equal to allocatable")
	require.Equal(t, int64(2), resourceAllocatable.Value(), "Devices are not updated.")

	p2 := plugin.NewDevicePluginStub(devs, pluginSocketName+".new", testResourceName, false, false)
	err = p2.Start()
	require.NoError(t, err)
	// Wait for the second callback to be issued.
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.FailNow()
	}

	capacity, allocatable, _ = m.GetCapacity()
	resourceCapacity = capacity[v1.ResourceName(testResourceName)]
	resourceAllocatable = allocatable[v1.ResourceName(testResourceName)]
	require.Equal(t, resourceCapacity.Value(), resourceAllocatable.Value(), "capacity should equal to allocatable")
	require.Equal(t, int64(2), resourceAllocatable.Value(), "Devices are not updated.")

	// Test the scenario that a plugin re-registers with different devices.
	p3 := plugin.NewDevicePluginStub(devsForRegistration, pluginSocketName+".third", testResourceName, false, false)
	err = p3.Start()
	require.NoError(t, err)
	// Wait for the third callback to be issued.
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.FailNow()
	}

	capacity, allocatable, _ = m.GetCapacity()
	resourceCapacity = capacity[v1.ResourceName(testResourceName)]
	resourceAllocatable = allocatable[v1.ResourceName(testResourceName)]
	require.Equal(t, resourceCapacity.Value(), resourceAllocatable.Value(), "capacity should equal to allocatable")
	require.Equal(t, int64(1), resourceAllocatable.Value(), "Devices of previous registered should be removed")
	p2.Stop()
	p3.Stop()
	cleanup(t, m, p1)
}

func setupDeviceManager(t *testing.T, devs []*pluginapi.Device, callback monitorCallback, socketName string) (Manager, <-chan interface{}) {
	topologyStore := topologymanager.NewFakeManager()
	m, err := newManagerImpl(socketName, nil, topologyStore)
	require.NoError(t, err)
	updateChan := make(chan interface{})

	w := newWrappedManagerImpl(socketName, m)
	if callback != nil {
		w.callback = callback
	}

	originalCallback := w.callback
	w.callback = func(resourceName string, devices []pluginapi.Device) {
		originalCallback(resourceName, devices)
		updateChan <- new(interface{})
	}
	activePods := func() []*v1.Pod {
		return []*v1.Pod{}
	}

	err = w.Start(activePods, &sourcesReadyStub{})
	require.NoError(t, err)

	return w, updateChan
}

func setupDevicePlugin(t *testing.T, devs []*pluginapi.Device, pluginSocketName string) *plugin.Stub {
	p := plugin.NewDevicePluginStub(devs, pluginSocketName, testResourceName, false, false)
	err := p.Start()
	require.NoError(t, err)
	return p
}

func setupPluginManager(t *testing.T, pluginSocketName string, m Manager) pluginmanager.PluginManager {
	pluginManager := pluginmanager.NewPluginManager(
		filepath.Dir(pluginSocketName), /* sockDir */
		&record.FakeRecorder{},
	)

	runPluginManager(pluginManager)
	pluginManager.AddHandler(watcherapi.DevicePlugin, m.GetWatcherHandler())
	return pluginManager
}

func runPluginManager(pluginManager pluginmanager.PluginManager) {
	sourcesReady := config.NewSourcesReady(func(_ sets.String) bool { return true })
	go pluginManager.Run(sourcesReady, wait.NeverStop)
}

func setup(t *testing.T, devs []*pluginapi.Device, callback monitorCallback, socketName string, pluginSocketName string) (Manager, <-chan interface{}, *plugin.Stub) {
	m, updateChan := setupDeviceManager(t, devs, callback, socketName)
	p := setupDevicePlugin(t, devs, pluginSocketName)
	return m, updateChan, p
}

func setupInProbeMode(t *testing.T, devs []*pluginapi.Device, callback monitorCallback, socketName string, pluginSocketName string) (Manager, <-chan interface{}, *plugin.Stub, pluginmanager.PluginManager) {
	m, updateChan := setupDeviceManager(t, devs, callback, socketName)
	p := setupDevicePlugin(t, devs, pluginSocketName)
	pm := setupPluginManager(t, pluginSocketName, m)
	return m, updateChan, p, pm
}

func cleanup(t *testing.T, m Manager, p *plugin.Stub) {
	p.Stop()
	m.Stop()
}

func TestUpdateCapacityAllocatable(t *testing.T) {
	socketDir, socketName, _, err := tmpSocketDir()
	topologyStore := topologymanager.NewFakeManager()
	require.NoError(t, err)
	defer os.RemoveAll(socketDir)
	testManager, err := newManagerImpl(socketName, nil, topologyStore)
	as := assert.New(t)
	as.NotNil(testManager)
	as.Nil(err)

	devs := []pluginapi.Device{
		{ID: "Device1", Health: pluginapi.Healthy},
		{ID: "Device2", Health: pluginapi.Healthy},
		{ID: "Device3", Health: pluginapi.Unhealthy},
	}
	callback := testManager.genericDeviceUpdateCallback

	// Adds three devices for resource1, two healthy and one unhealthy.
	// Expects capacity for resource1 to be 2.
	resourceName1 := "domain1.com/resource1"
	e1 := &endpointImpl{}
	testManager.endpoints[resourceName1] = endpointInfo{e: e1, opts: nil}
	callback(resourceName1, devs)
	capacity, allocatable, removedResources := testManager.GetCapacity()
	resource1Capacity, ok := capacity[v1.ResourceName(resourceName1)]
	as.True(ok)
	resource1Allocatable, ok := allocatable[v1.ResourceName(resourceName1)]
	as.True(ok)
	as.Equal(int64(3), resource1Capacity.Value())
	as.Equal(int64(2), resource1Allocatable.Value())
	as.Equal(0, len(removedResources))

	// Deletes an unhealthy device should NOT change allocatable but change capacity.
	devs1 := devs[:len(devs)-1]
	callback(resourceName1, devs1)
	capacity, allocatable, removedResources = testManager.GetCapacity()
	resource1Capacity, ok = capacity[v1.ResourceName(resourceName1)]
	as.True(ok)
	resource1Allocatable, ok = allocatable[v1.ResourceName(resourceName1)]
	as.True(ok)
	as.Equal(int64(2), resource1Capacity.Value())
	as.Equal(int64(2), resource1Allocatable.Value())
	as.Equal(0, len(removedResources))

	// Updates a healthy device to unhealthy should reduce allocatable by 1.
	devs[1].Health = pluginapi.Unhealthy
	callback(resourceName1, devs)
	capacity, allocatable, removedResources = testManager.GetCapacity()
	resource1Capacity, ok = capacity[v1.ResourceName(resourceName1)]
	as.True(ok)
	resource1Allocatable, ok = allocatable[v1.ResourceName(resourceName1)]
	as.True(ok)
	as.Equal(int64(3), resource1Capacity.Value())
	as.Equal(int64(1), resource1Allocatable.Value())
	as.Equal(0, len(removedResources))

	// Deletes a healthy device should reduce capacity and allocatable by 1.
	devs2 := devs[1:]
	callback(resourceName1, devs2)
	capacity, allocatable, removedResources = testManager.GetCapacity()
	resource1Capacity, ok = capacity[v1.ResourceName(resourceName1)]
	as.True(ok)
	resource1Allocatable, ok = allocatable[v1.ResourceName(resourceName1)]
	as.True(ok)
	as.Equal(int64(0), resource1Allocatable.Value())
	as.Equal(int64(2), resource1Capacity.Value())
	as.Equal(0, len(removedResources))

	// Tests adding another resource.
	resourceName2 := "resource2"
	e2 := &endpointImpl{}
	e2.client = plugin.NewPluginClient(resourceName2, socketName, testManager)
	testManager.endpoints[resourceName2] = endpointInfo{e: e2, opts: nil}
	callback(resourceName2, devs)
	capacity, allocatable, removedResources = testManager.GetCapacity()
	as.Equal(2, len(capacity))
	resource2Capacity, ok := capacity[v1.ResourceName(resourceName2)]
	as.True(ok)
	resource2Allocatable, ok := allocatable[v1.ResourceName(resourceName2)]
	as.True(ok)
	as.Equal(int64(3), resource2Capacity.Value())
	as.Equal(int64(1), resource2Allocatable.Value())
	as.Equal(0, len(removedResources))

	// Expires resourceName1 endpoint. Verifies testManager.GetCapacity() reports that resourceName1
	// is removed from capacity and it no longer exists in healthyDevices after the call.
	e1.setStopTime(time.Now().Add(-1*endpointStopGracePeriod - time.Duration(10)*time.Second))
	capacity, allocatable, removed := testManager.GetCapacity()
	as.Equal([]string{resourceName1}, removed)
	as.NotContains(capacity, v1.ResourceName(resourceName1))
	as.NotContains(allocatable, v1.ResourceName(resourceName1))
	val, ok := capacity[v1.ResourceName(resourceName2)]
	as.True(ok)
	as.Equal(int64(3), val.Value())
	as.NotContains(testManager.healthyDevices, resourceName1)
	as.NotContains(testManager.unhealthyDevices, resourceName1)
	as.NotContains(testManager.endpoints, resourceName1)
	as.Equal(1, len(testManager.endpoints))

	// Stops resourceName2 endpoint. Verifies its stopTime is set, allocate and
	// preStartContainer calls return errors.
	e2.client.Disconnect()
	as.False(e2.stopTime.IsZero())
	_, err = e2.allocate([]string{"Device1"})
	reflect.DeepEqual(err, fmt.Errorf(errEndpointStopped, e2))
	_, err = e2.preStartContainer([]string{"Device1"})
	reflect.DeepEqual(err, fmt.Errorf(errEndpointStopped, e2))
	// Marks resourceName2 unhealthy and verifies its capacity/allocatable are
	// correctly updated.
	testManager.markResourceUnhealthy(resourceName2)
	capacity, allocatable, removed = testManager.GetCapacity()
	val, ok = capacity[v1.ResourceName(resourceName2)]
	as.True(ok)
	as.Equal(int64(3), val.Value())
	val, ok = allocatable[v1.ResourceName(resourceName2)]
	as.True(ok)
	as.Equal(int64(0), val.Value())
	as.Empty(removed)
	// Writes and re-reads checkpoints. Verifies we create a stopped endpoint
	// for resourceName2, its capacity is set to zero, and we still consider
	// it as a DevicePlugin resource. This makes sure any pod that was scheduled
	// during the time of propagating capacity change to the scheduler will be
	// properly rejected instead of being incorrectly started.
	err = testManager.writeCheckpoint()
	as.Nil(err)
	testManager.healthyDevices = make(map[string]sets.String)
	testManager.unhealthyDevices = make(map[string]sets.String)
	err = testManager.readCheckpoint()
	as.Nil(err)
	as.Equal(1, len(testManager.endpoints))
	as.Contains(testManager.endpoints, resourceName2)
	capacity, allocatable, removed = testManager.GetCapacity()
	val, ok = capacity[v1.ResourceName(resourceName2)]
	as.True(ok)
	as.Equal(int64(0), val.Value())
	val, ok = allocatable[v1.ResourceName(resourceName2)]
	as.True(ok)
	as.Equal(int64(0), val.Value())
	as.Empty(removed)
	as.True(testManager.isDevicePluginResource(resourceName2))
}

func TestGetAllocatableDevicesMultipleResources(t *testing.T) {
	socketDir, socketName, _, err := tmpSocketDir()
	topologyStore := topologymanager.NewFakeManager()
	require.NoError(t, err)
	defer os.RemoveAll(socketDir)
	testManager, err := newManagerImpl(socketName, nil, topologyStore)
	as := assert.New(t)
	as.NotNil(testManager)
	as.Nil(err)

	resource1Devs := []pluginapi.Device{
		{ID: "R1Device1", Health: pluginapi.Healthy},
		{ID: "R1Device2", Health: pluginapi.Healthy},
		{ID: "R1Device3", Health: pluginapi.Unhealthy},
	}
	resourceName1 := "domain1.com/resource1"
	e1 := &endpointImpl{}
	testManager.endpoints[resourceName1] = endpointInfo{e: e1, opts: nil}
	testManager.genericDeviceUpdateCallback(resourceName1, resource1Devs)

	resource2Devs := []pluginapi.Device{
		{ID: "R2Device1", Health: pluginapi.Healthy},
	}
	resourceName2 := "other.domain2.org/resource2"
	e2 := &endpointImpl{}
	testManager.endpoints[resourceName2] = endpointInfo{e: e2, opts: nil}
	testManager.genericDeviceUpdateCallback(resourceName2, resource2Devs)

	allocatableDevs := testManager.GetAllocatableDevices()
	as.Equal(2, len(allocatableDevs))

	devInstances1, ok := allocatableDevs[resourceName1]
	as.True(ok)
	checkAllocatableDevicesConsistsOf(as, devInstances1, []string{"R1Device1", "R1Device2"})

	devInstances2, ok := allocatableDevs[resourceName2]
	as.True(ok)
	checkAllocatableDevicesConsistsOf(as, devInstances2, []string{"R2Device1"})

}

func TestGetAllocatableDevicesHealthTransition(t *testing.T) {
	socketDir, socketName, _, err := tmpSocketDir()
	topologyStore := topologymanager.NewFakeManager()
	require.NoError(t, err)
	defer os.RemoveAll(socketDir)
	testManager, err := newManagerImpl(socketName, nil, topologyStore)
	as := assert.New(t)
	as.NotNil(testManager)
	as.Nil(err)

	resource1Devs := []pluginapi.Device{
		{ID: "R1Device1", Health: pluginapi.Healthy},
		{ID: "R1Device2", Health: pluginapi.Healthy},
		{ID: "R1Device3", Health: pluginapi.Unhealthy},
	}

	// Adds three devices for resource1, two healthy and one unhealthy.
	// Expects allocatable devices for resource1 to be 2.
	resourceName1 := "domain1.com/resource1"
	e1 := &endpointImpl{}
	testManager.endpoints[resourceName1] = endpointInfo{e: e1, opts: nil}

	testManager.genericDeviceUpdateCallback(resourceName1, resource1Devs)

	allocatableDevs := testManager.GetAllocatableDevices()
	as.Equal(1, len(allocatableDevs))
	devInstances, ok := allocatableDevs[resourceName1]
	as.True(ok)
	checkAllocatableDevicesConsistsOf(as, devInstances, []string{"R1Device1", "R1Device2"})

	// Unhealthy device becomes healthy
	resource1Devs = []pluginapi.Device{
		{ID: "R1Device1", Health: pluginapi.Healthy},
		{ID: "R1Device2", Health: pluginapi.Healthy},
		{ID: "R1Device3", Health: pluginapi.Healthy},
	}
	testManager.genericDeviceUpdateCallback(resourceName1, resource1Devs)

	allocatableDevs = testManager.GetAllocatableDevices()
	as.Equal(1, len(allocatableDevs))
	devInstances, ok = allocatableDevs[resourceName1]
	as.True(ok)
	checkAllocatableDevicesConsistsOf(as, devInstances, []string{"R1Device1", "R1Device2", "R1Device3"})
}

func checkAllocatableDevicesConsistsOf(as *assert.Assertions, devInstances DeviceInstances, expectedDevs []string) {
	as.Equal(len(expectedDevs), len(devInstances))
	for _, deviceID := range expectedDevs {
		_, ok := devInstances[deviceID]
		as.True(ok)
	}
}

func constructDevices(devices []string) checkpoint.DevicesPerNUMA {
	ret := checkpoint.DevicesPerNUMA{}
	for _, dev := range devices {
		ret[0] = append(ret[0], dev)
	}
	return ret
}

func constructAllocResp(devices, mounts, envs map[string]string) *pluginapi.ContainerAllocateResponse {
	resp := &pluginapi.ContainerAllocateResponse{}
	for k, v := range devices {
		resp.Devices = append(resp.Devices, &pluginapi.DeviceSpec{
			HostPath:      k,
			ContainerPath: v,
			Permissions:   "mrw",
		})
	}
	for k, v := range mounts {
		resp.Mounts = append(resp.Mounts, &pluginapi.Mount{
			ContainerPath: k,
			HostPath:      v,
			ReadOnly:      true,
		})
	}
	resp.Envs = make(map[string]string)
	for k, v := range envs {
		resp.Envs[k] = v
	}
	return resp
}

func TestCheckpoint(t *testing.T) {
	resourceName1 := "domain1.com/resource1"
	resourceName2 := "domain2.com/resource2"
	resourceName3 := "domain2.com/resource3"
	as := assert.New(t)
	tmpDir, err := ioutil.TempDir("", "checkpoint")
	as.Nil(err)
	defer os.RemoveAll(tmpDir)
	ckm, err := checkpointmanager.NewCheckpointManager(tmpDir)
	as.Nil(err)
	testManager := &ManagerImpl{
		endpoints:         make(map[string]endpointInfo),
		healthyDevices:    make(map[string]sets.String),
		unhealthyDevices:  make(map[string]sets.String),
		allocatedDevices:  make(map[string]sets.String),
		podDevices:        newPodDevices(),
		checkpointManager: ckm,
	}

	testManager.podDevices.insert("pod1", "con1", resourceName1,
		constructDevices([]string{"dev1", "dev2"}),
		constructAllocResp(map[string]string{"/dev/r1dev1": "/dev/r1dev1", "/dev/r1dev2": "/dev/r1dev2"},
			map[string]string{"/home/r1lib1": "/usr/r1lib1"}, map[string]string{}))
	testManager.podDevices.insert("pod1", "con1", resourceName2,
		constructDevices([]string{"dev1", "dev2"}),
		constructAllocResp(map[string]string{"/dev/r2dev1": "/dev/r2dev1", "/dev/r2dev2": "/dev/r2dev2"},
			map[string]string{"/home/r2lib1": "/usr/r2lib1"},
			map[string]string{"r2devices": "dev1 dev2"}))
	testManager.podDevices.insert("pod1", "con2", resourceName1,
		constructDevices([]string{"dev3"}),
		constructAllocResp(map[string]string{"/dev/r1dev3": "/dev/r1dev3"},
			map[string]string{"/home/r1lib1": "/usr/r1lib1"}, map[string]string{}))
	testManager.podDevices.insert("pod2", "con1", resourceName1,
		constructDevices([]string{"dev4"}),
		constructAllocResp(map[string]string{"/dev/r1dev4": "/dev/r1dev4"},
			map[string]string{"/home/r1lib1": "/usr/r1lib1"}, map[string]string{}))
	testManager.podDevices.insert("pod3", "con3", resourceName3,
		checkpoint.DevicesPerNUMA{nodeWithoutTopology: []string{"dev5"}},
		constructAllocResp(map[string]string{"/dev/r1dev5": "/dev/r1dev5"},
			map[string]string{"/home/r1lib1": "/usr/r1lib1"}, map[string]string{}))

	testManager.healthyDevices[resourceName1] = sets.NewString()
	testManager.healthyDevices[resourceName1].Insert("dev1")
	testManager.healthyDevices[resourceName1].Insert("dev2")
	testManager.healthyDevices[resourceName1].Insert("dev3")
	testManager.healthyDevices[resourceName1].Insert("dev4")
	testManager.healthyDevices[resourceName1].Insert("dev5")
	testManager.healthyDevices[resourceName2] = sets.NewString()
	testManager.healthyDevices[resourceName2].Insert("dev1")
	testManager.healthyDevices[resourceName2].Insert("dev2")
	testManager.healthyDevices[resourceName3] = sets.NewString()
	testManager.healthyDevices[resourceName3].Insert("dev5")

	expectedPodDevices := testManager.podDevices
	expectedAllocatedDevices := testManager.podDevices.devices()
	expectedAllDevices := testManager.healthyDevices

	err = testManager.writeCheckpoint()

	as.Nil(err)
	testManager.podDevices = newPodDevices()
	err = testManager.readCheckpoint()
	as.Nil(err)

	as.Equal(expectedPodDevices.size(), testManager.podDevices.size())
	for podUID, containerDevices := range expectedPodDevices.devs {
		for conName, resources := range containerDevices {
			for resource := range resources {
				expDevices := expectedPodDevices.containerDevices(podUID, conName, resource)
				testDevices := testManager.podDevices.containerDevices(podUID, conName, resource)
				as.True(reflect.DeepEqual(expDevices, testDevices))
				opts1 := expectedPodDevices.deviceRunContainerOptions(podUID, conName)
				opts2 := testManager.podDevices.deviceRunContainerOptions(podUID, conName)
				as.Equal(len(opts1.Envs), len(opts2.Envs))
				as.Equal(len(opts1.Mounts), len(opts2.Mounts))
				as.Equal(len(opts1.Devices), len(opts2.Devices))
			}
		}
	}
	as.True(reflect.DeepEqual(expectedAllocatedDevices, testManager.allocatedDevices))
	as.True(reflect.DeepEqual(expectedAllDevices, testManager.healthyDevices))
}

type activePodsStub struct {
	activePods []*v1.Pod
}

func (a *activePodsStub) getActivePods() []*v1.Pod {
	return a.activePods
}

func (a *activePodsStub) updateActivePods(newPods []*v1.Pod) {
	a.activePods = newPods
}

type MockEndpoint struct {
	getPreferredAllocationFunc func(available, mustInclude []string, size int) (*pluginapi.PreferredAllocationResponse, error)
	allocateFunc               func(devs []string) (*pluginapi.AllocateResponse, error)
	initChan                   chan []string
}

func (m *MockEndpoint) preStartContainer(devs []string) (*pluginapi.PreStartContainerResponse, error) {
	m.initChan <- devs
	return &pluginapi.PreStartContainerResponse{}, nil
}

func (m *MockEndpoint) getPreferredAllocation(available, mustInclude []string, size int) (*pluginapi.PreferredAllocationResponse, error) {
	if m.getPreferredAllocationFunc != nil {
		return m.getPreferredAllocationFunc(available, mustInclude, size)
	}
	return nil, nil
}

func (m *MockEndpoint) allocate(devs []string) (*pluginapi.AllocateResponse, error) {
	if m.allocateFunc != nil {
		return m.allocateFunc(devs)
	}
	return nil, nil
}

func (m *MockEndpoint) setStopTime(t time.Time) {}

func (m *MockEndpoint) isStopped() bool { return false }

func (m *MockEndpoint) stopGracePeriodExpired() bool { return false }

func makePod(limits v1.ResourceList) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: uuid.NewUUID(),
		},
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Resources: v1.ResourceRequirements{
						Limits: limits,
					},
				},
			},
		},
	}
}

func getTestManager(tmpDir string, activePods ActivePodsFunc, testRes []TestResource) (*wrappedManagerImpl, error) {
	monitorCallback := func(resourceName string, devices []pluginapi.Device) {}
	ckm, err := checkpointmanager.NewCheckpointManager(tmpDir)
	if err != nil {
		return nil, err
	}
	m := &ManagerImpl{
		healthyDevices:        make(map[string]sets.String),
		unhealthyDevices:      make(map[string]sets.String),
		allocatedDevices:      make(map[string]sets.String),
		endpoints:             make(map[string]endpointInfo),
		podDevices:            newPodDevices(),
		devicesToReuse:        make(PodReusableDevices),
		topologyAffinityStore: topologymanager.NewFakeManager(),
		activePods:            activePods,
		sourcesReady:          &sourcesReadyStub{},
		checkpointManager:     ckm,
		allDevices:            NewResourceDeviceInstances(),
	}
	testManager := &wrappedManagerImpl{
		ManagerImpl: m,
		socketdir:   tmpDir,
		callback:    monitorCallback,
	}

	for _, res := range testRes {
		testManager.healthyDevices[res.resourceName] = sets.NewString(res.devs.Devices().UnsortedList()...)
		if res.resourceName == "domain1.com/resource1" {
			testManager.endpoints[res.resourceName] = endpointInfo{
				e:    &MockEndpoint{allocateFunc: allocateStubFunc()},
				opts: nil,
			}
		}
		if res.resourceName == "domain2.com/resource2" {
			testManager.endpoints[res.resourceName] = endpointInfo{
				e: &MockEndpoint{
					allocateFunc: func(devs []string) (*pluginapi.AllocateResponse, error) {
						resp := new(pluginapi.ContainerAllocateResponse)
						resp.Envs = make(map[string]string)
						for _, dev := range devs {
							switch dev {
							case "dev3":
								resp.Envs["key2"] = "val2"

							case "dev4":
								resp.Envs["key2"] = "val3"
							}
						}
						resps := new(pluginapi.AllocateResponse)
						resps.ContainerResponses = append(resps.ContainerResponses, resp)
						return resps, nil
					},
				},
				opts: nil,
			}
		}
		testManager.allDevices[res.resourceName] = makeDevice(res.devs, res.topology)

	}
	return testManager, nil
}

type TestResource struct {
	resourceName     string
	resourceQuantity resource.Quantity
	devs             checkpoint.DevicesPerNUMA
	topology         bool
}

func TestFilterByAffinity(t *testing.T) {
	as := require.New(t)
	allDevices := ResourceDeviceInstances{
		"res1": map[string]pluginapi.Device{
			"dev1": {
				ID: "dev1",
				Topology: &pluginapi.TopologyInfo{
					Nodes: []*pluginapi.NUMANode{
						{
							ID: 1,
						},
					},
				},
			},
			"dev2": {
				ID: "dev2",
				Topology: &pluginapi.TopologyInfo{
					Nodes: []*pluginapi.NUMANode{
						{
							ID: 1,
						},
						{
							ID: 2,
						},
					},
				},
			},
			"dev3": {
				ID: "dev3",
				Topology: &pluginapi.TopologyInfo{
					Nodes: []*pluginapi.NUMANode{
						{
							ID: 2,
						},
					},
				},
			},
			"dev4": {
				ID: "dev4",
				Topology: &pluginapi.TopologyInfo{
					Nodes: []*pluginapi.NUMANode{
						{
							ID: 2,
						},
					},
				},
			},
			"devwithouttopology": {
				ID: "dev5",
			},
		},
	}

	fakeAffinity, _ := bitmask.NewBitMask(2)
	fakeHint := topologymanager.TopologyHint{
		NUMANodeAffinity: fakeAffinity,
		Preferred:        true,
	}
	testManager := ManagerImpl{
		topologyAffinityStore: topologymanager.NewFakeManagerWithHint(&fakeHint),
		allDevices:            allDevices,
	}

	testCases := []struct {
		available               sets.String
		fromAffinityExpected    sets.String
		notFromAffinityExpected sets.String
		withoutTopologyExpected sets.String
	}{
		{
			available:               sets.NewString("dev1", "dev2"),
			fromAffinityExpected:    sets.NewString("dev2"),
			notFromAffinityExpected: sets.NewString("dev1"),
			withoutTopologyExpected: sets.NewString(),
		},
		{
			available:               sets.NewString("dev1", "dev2", "dev3", "dev4"),
			fromAffinityExpected:    sets.NewString("dev2", "dev3", "dev4"),
			notFromAffinityExpected: sets.NewString("dev1"),
			withoutTopologyExpected: sets.NewString(),
		},
	}

	for _, testCase := range testCases {
		fromAffinity, notFromAffinity, withoutTopology := testManager.filterByAffinity("", "", "res1", testCase.available)
		as.Truef(fromAffinity.Equal(testCase.fromAffinityExpected), "expect devices from affinity to be %v but got %v", testCase.fromAffinityExpected, fromAffinity)
		as.Truef(notFromAffinity.Equal(testCase.notFromAffinityExpected), "expect devices not from affinity to be %v but got %v", testCase.notFromAffinityExpected, notFromAffinity)
		as.Truef(withoutTopology.Equal(testCase.withoutTopologyExpected), "expect devices without topology to be %v but got %v", testCase.notFromAffinityExpected, notFromAffinity)
	}
}

func TestPodContainerDeviceAllocation(t *testing.T) {
	res1 := TestResource{
		resourceName:     "domain1.com/resource1",
		resourceQuantity: *resource.NewQuantity(int64(2), resource.DecimalSI),
		devs:             checkpoint.DevicesPerNUMA{0: []string{"dev1", "dev2"}},
		topology:         true,
	}
	res2 := TestResource{
		resourceName:     "domain2.com/resource2",
		resourceQuantity: *resource.NewQuantity(int64(1), resource.DecimalSI),
		devs:             checkpoint.DevicesPerNUMA{0: []string{"dev3", "dev4"}},
		topology:         false,
	}
	testResources := make([]TestResource, 2)
	testResources = append(testResources, res1)
	testResources = append(testResources, res2)
	as := require.New(t)
	podsStub := activePodsStub{
		activePods: []*v1.Pod{},
	}
	tmpDir, err := ioutil.TempDir("", "checkpoint")
	as.Nil(err)
	defer os.RemoveAll(tmpDir)
	testManager, err := getTestManager(tmpDir, podsStub.getActivePods, testResources)
	as.Nil(err)

	testPods := []*v1.Pod{
		makePod(v1.ResourceList{
			v1.ResourceName(res1.resourceName): res1.resourceQuantity,
			v1.ResourceName("cpu"):             res1.resourceQuantity,
			v1.ResourceName(res2.resourceName): res2.resourceQuantity}),
		makePod(v1.ResourceList{
			v1.ResourceName(res1.resourceName): res2.resourceQuantity}),
		makePod(v1.ResourceList{
			v1.ResourceName(res2.resourceName): res2.resourceQuantity}),
	}
	testCases := []struct {
		description               string
		testPod                   *v1.Pod
		expectedContainerOptsLen  []int
		expectedAllocatedResName1 int
		expectedAllocatedResName2 int
		expErr                    error
	}{
		{
			description:               "Successful allocation of two Res1 resources and one Res2 resource",
			testPod:                   testPods[0],
			expectedContainerOptsLen:  []int{3, 2, 2},
			expectedAllocatedResName1: 2,
			expectedAllocatedResName2: 1,
			expErr:                    nil,
		},
		{
			description:               "Requesting to create a pod without enough resources should fail",
			testPod:                   testPods[1],
			expectedContainerOptsLen:  nil,
			expectedAllocatedResName1: 2,
			expectedAllocatedResName2: 1,
			expErr:                    fmt.Errorf("requested number of devices unavailable for domain1.com/resource1. Requested: 1, Available: 0"),
		},
		{
			description:               "Successful allocation of all available Res1 resources and Res2 resources",
			testPod:                   testPods[2],
			expectedContainerOptsLen:  []int{0, 0, 1},
			expectedAllocatedResName1: 2,
			expectedAllocatedResName2: 2,
			expErr:                    nil,
		},
	}
	activePods := []*v1.Pod{}
	for _, testCase := range testCases {
		pod := testCase.testPod
		activePods = append(activePods, pod)
		podsStub.updateActivePods(activePods)
		err := testManager.Allocate(pod, &pod.Spec.Containers[0])
		if !reflect.DeepEqual(err, testCase.expErr) {
			t.Errorf("DevicePluginManager error (%v). expected error: %v but got: %v",
				testCase.description, testCase.expErr, err)
		}
		runContainerOpts, err := testManager.GetDeviceRunContainerOptions(pod, &pod.Spec.Containers[0])
		if testCase.expErr == nil {
			as.Nil(err)
		}
		if testCase.expectedContainerOptsLen == nil {
			as.Nil(runContainerOpts)
		} else {
			as.Equal(len(runContainerOpts.Devices), testCase.expectedContainerOptsLen[0])
			as.Equal(len(runContainerOpts.Mounts), testCase.expectedContainerOptsLen[1])
			as.Equal(len(runContainerOpts.Envs), testCase.expectedContainerOptsLen[2])
		}
		as.Equal(testCase.expectedAllocatedResName1, testManager.allocatedDevices[res1.resourceName].Len())
		as.Equal(testCase.expectedAllocatedResName2, testManager.allocatedDevices[res2.resourceName].Len())
	}

}

func TestGetDeviceRunContainerOptions(t *testing.T) {
	res1 := TestResource{
		resourceName:     "domain1.com/resource1",
		resourceQuantity: *resource.NewQuantity(int64(2), resource.DecimalSI),
		devs:             checkpoint.DevicesPerNUMA{0: []string{"dev1", "dev2"}},
		topology:         true,
	}
	res2 := TestResource{
		resourceName:     "domain2.com/resource2",
		resourceQuantity: *resource.NewQuantity(int64(1), resource.DecimalSI),
		devs:             checkpoint.DevicesPerNUMA{0: []string{"dev3", "dev4"}},
		topology:         false,
	}

	testResources := make([]TestResource, 2)
	testResources = append(testResources, res1)
	testResources = append(testResources, res2)

	podsStub := activePodsStub{
		activePods: []*v1.Pod{},
	}
	as := require.New(t)

	tmpDir, err := ioutil.TempDir("", "checkpoint")
	as.Nil(err)
	defer os.RemoveAll(tmpDir)

	testManager, err := getTestManager(tmpDir, podsStub.getActivePods, testResources)
	as.Nil(err)

	pod1 := makePod(v1.ResourceList{
		v1.ResourceName(res1.resourceName): res1.resourceQuantity,
		v1.ResourceName(res2.resourceName): res2.resourceQuantity,
	})
	pod2 := makePod(v1.ResourceList{
		v1.ResourceName(res2.resourceName): res2.resourceQuantity,
	})

	activePods := []*v1.Pod{pod1, pod2}
	podsStub.updateActivePods(activePods)

	err = testManager.Allocate(pod1, &pod1.Spec.Containers[0])
	as.Nil(err)
	err = testManager.Allocate(pod2, &pod2.Spec.Containers[0])
	as.Nil(err)

	// when pod is in activePods, GetDeviceRunContainerOptions should return
	runContainerOpts, err := testManager.GetDeviceRunContainerOptions(pod1, &pod1.Spec.Containers[0])
	as.Nil(err)
	as.Equal(len(runContainerOpts.Devices), 3)
	as.Equal(len(runContainerOpts.Mounts), 2)
	as.Equal(len(runContainerOpts.Envs), 2)

	activePods = []*v1.Pod{pod2}
	podsStub.updateActivePods(activePods)
	testManager.UpdateAllocatedDevices()

	// when pod is removed from activePods,G etDeviceRunContainerOptions should return error
	runContainerOpts, err = testManager.GetDeviceRunContainerOptions(pod1, &pod1.Spec.Containers[0])
	as.Nil(err)
	as.Nil(runContainerOpts)
}

func TestInitContainerDeviceAllocation(t *testing.T) {
	// Requesting to create a pod that requests resourceName1 in init containers and normal containers
	// should succeed with devices allocated to init containers reallocated to normal containers.
	res1 := TestResource{
		resourceName:     "domain1.com/resource1",
		resourceQuantity: *resource.NewQuantity(int64(2), resource.DecimalSI),
		devs:             checkpoint.DevicesPerNUMA{0: []string{"dev1", "dev2"}},
		topology:         false,
	}
	res2 := TestResource{
		resourceName:     "domain2.com/resource2",
		resourceQuantity: *resource.NewQuantity(int64(1), resource.DecimalSI),
		devs:             checkpoint.DevicesPerNUMA{0: []string{"dev3", "dev4"}},
		topology:         true,
	}
	testResources := make([]TestResource, 2)
	testResources = append(testResources, res1)
	testResources = append(testResources, res2)
	as := require.New(t)
	podsStub := activePodsStub{
		activePods: []*v1.Pod{},
	}
	tmpDir, err := ioutil.TempDir("", "checkpoint")
	as.Nil(err)
	defer os.RemoveAll(tmpDir)

	testManager, err := getTestManager(tmpDir, podsStub.getActivePods, testResources)
	as.Nil(err)

	podWithPluginResourcesInInitContainers := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			UID: uuid.NewUUID(),
		},
		Spec: v1.PodSpec{
			InitContainers: []v1.Container{
				{
					Name: string(uuid.NewUUID()),
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceName(res1.resourceName): res2.resourceQuantity,
						},
					},
				},
				{
					Name: string(uuid.NewUUID()),
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceName(res1.resourceName): res1.resourceQuantity,
						},
					},
				},
			},
			Containers: []v1.Container{
				{
					Name: string(uuid.NewUUID()),
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceName(res1.resourceName): res2.resourceQuantity,
							v1.ResourceName(res2.resourceName): res2.resourceQuantity,
						},
					},
				},
				{
					Name: string(uuid.NewUUID()),
					Resources: v1.ResourceRequirements{
						Limits: v1.ResourceList{
							v1.ResourceName(res1.resourceName): res2.resourceQuantity,
							v1.ResourceName(res2.resourceName): res2.resourceQuantity,
						},
					},
				},
			},
		},
	}
	podsStub.updateActivePods([]*v1.Pod{podWithPluginResourcesInInitContainers})
	for _, container := range podWithPluginResourcesInInitContainers.Spec.InitContainers {
		err = testManager.Allocate(podWithPluginResourcesInInitContainers, &container)
	}
	for _, container := range podWithPluginResourcesInInitContainers.Spec.Containers {
		err = testManager.Allocate(podWithPluginResourcesInInitContainers, &container)
	}
	as.Nil(err)
	podUID := string(podWithPluginResourcesInInitContainers.UID)
	initCont1 := podWithPluginResourcesInInitContainers.Spec.InitContainers[0].Name
	initCont2 := podWithPluginResourcesInInitContainers.Spec.InitContainers[1].Name
	normalCont1 := podWithPluginResourcesInInitContainers.Spec.Containers[0].Name
	normalCont2 := podWithPluginResourcesInInitContainers.Spec.Containers[1].Name
	initCont1Devices := testManager.podDevices.containerDevices(podUID, initCont1, res1.resourceName)
	initCont2Devices := testManager.podDevices.containerDevices(podUID, initCont2, res1.resourceName)
	normalCont1Devices := testManager.podDevices.containerDevices(podUID, normalCont1, res1.resourceName)
	normalCont2Devices := testManager.podDevices.containerDevices(podUID, normalCont2, res1.resourceName)
	as.Equal(1, initCont1Devices.Len())
	as.Equal(2, initCont2Devices.Len())
	as.Equal(1, normalCont1Devices.Len())
	as.Equal(1, normalCont2Devices.Len())
	as.True(initCont2Devices.IsSuperset(initCont1Devices))
	as.True(initCont2Devices.IsSuperset(normalCont1Devices))
	as.True(initCont2Devices.IsSuperset(normalCont2Devices))
	as.Equal(0, normalCont1Devices.Intersection(normalCont2Devices).Len())
}

func TestUpdatePluginResources(t *testing.T) {
	pod := &v1.Pod{}
	pod.UID = types.UID("testPod")

	resourceName1 := "domain1.com/resource1"
	devID1 := "dev1"

	resourceName2 := "domain2.com/resource2"
	devID2 := "dev2"

	as := assert.New(t)
	monitorCallback := func(resourceName string, devices []pluginapi.Device) {}
	tmpDir, err := ioutil.TempDir("", "checkpoint")
	as.Nil(err)
	defer os.RemoveAll(tmpDir)

	ckm, err := checkpointmanager.NewCheckpointManager(tmpDir)
	as.Nil(err)
	m := &ManagerImpl{
		allocatedDevices:  make(map[string]sets.String),
		healthyDevices:    make(map[string]sets.String),
		podDevices:        newPodDevices(),
		checkpointManager: ckm,
	}
	testManager := wrappedManagerImpl{
		ManagerImpl: m,
		callback:    monitorCallback,
	}
	testManager.podDevices.devs[string(pod.UID)] = make(containerDevices)

	// require one of resource1 and one of resource2
	testManager.allocatedDevices[resourceName1] = sets.NewString()
	testManager.allocatedDevices[resourceName1].Insert(devID1)
	testManager.allocatedDevices[resourceName2] = sets.NewString()
	testManager.allocatedDevices[resourceName2].Insert(devID2)

	cachedNode := &v1.Node{
		Status: v1.NodeStatus{
			Allocatable: v1.ResourceList{
				// has no resource1 and two of resource2
				v1.ResourceName(resourceName2): *resource.NewQuantity(int64(2), resource.DecimalSI),
			},
		},
	}
	nodeInfo := &schedulerframework.NodeInfo{}
	nodeInfo.SetNode(cachedNode)

	testManager.UpdatePluginResources(nodeInfo, &lifecycle.PodAdmitAttributes{Pod: pod})

	allocatableScalarResources := nodeInfo.Allocatable.ScalarResources
	// allocatable in nodeInfo is less than needed, should update
	as.Equal(1, int(allocatableScalarResources[v1.ResourceName(resourceName1)]))
	// allocatable in nodeInfo is more than needed, should skip updating
	as.Equal(2, int(allocatableScalarResources[v1.ResourceName(resourceName2)]))
}

func TestDevicePreStartContainer(t *testing.T) {
	// Ensures that if device manager is indicated to invoke `PreStartContainer` RPC
	// by device plugin, then device manager invokes PreStartContainer at endpoint interface.
	// Also verifies that final allocation of mounts, envs etc is same as expected.
	res1 := TestResource{
		resourceName:     "domain1.com/resource1",
		resourceQuantity: *resource.NewQuantity(int64(2), resource.DecimalSI),
		devs:             checkpoint.DevicesPerNUMA{0: []string{"dev1", "dev2"}},
		topology:         false,
	}
	as := require.New(t)
	podsStub := activePodsStub{
		activePods: []*v1.Pod{},
	}
	tmpDir, err := ioutil.TempDir("", "checkpoint")
	as.Nil(err)
	defer os.RemoveAll(tmpDir)

	testManager, err := getTestManager(tmpDir, podsStub.getActivePods, []TestResource{res1})
	as.Nil(err)

	ch := make(chan []string, 1)
	testManager.endpoints[res1.resourceName] = endpointInfo{
		e: &MockEndpoint{
			initChan:     ch,
			allocateFunc: allocateStubFunc(),
		},
		opts: &pluginapi.DevicePluginOptions{PreStartRequired: true},
	}
	pod := makePod(v1.ResourceList{
		v1.ResourceName(res1.resourceName): res1.resourceQuantity})
	activePods := []*v1.Pod{}
	activePods = append(activePods, pod)
	podsStub.updateActivePods(activePods)
	err = testManager.Allocate(pod, &pod.Spec.Containers[0])
	as.Nil(err)
	runContainerOpts, err := testManager.GetDeviceRunContainerOptions(pod, &pod.Spec.Containers[0])
	as.Nil(err)
	var initializedDevs []string
	select {
	case <-time.After(time.Second):
		t.Fatalf("Timed out while waiting on channel for response from PreStartContainer RPC stub")
	case initializedDevs = <-ch:
		break
	}

	as.Contains(initializedDevs, "dev1")
	as.Contains(initializedDevs, "dev2")
	as.Equal(len(initializedDevs), res1.devs.Devices().Len())

	expectedResps, err := allocateStubFunc()([]string{"dev1", "dev2"})
	as.Nil(err)
	as.Equal(1, len(expectedResps.ContainerResponses))
	expectedResp := expectedResps.ContainerResponses[0]
	as.Equal(len(runContainerOpts.Devices), len(expectedResp.Devices))
	as.Equal(len(runContainerOpts.Mounts), len(expectedResp.Mounts))
	as.Equal(len(runContainerOpts.Envs), len(expectedResp.Envs))

	pod2 := makePod(v1.ResourceList{
		v1.ResourceName(res1.resourceName): *resource.NewQuantity(int64(0), resource.DecimalSI)})
	activePods = append(activePods, pod2)
	podsStub.updateActivePods(activePods)
	err = testManager.Allocate(pod2, &pod2.Spec.Containers[0])
	as.Nil(err)
	_, err = testManager.GetDeviceRunContainerOptions(pod2, &pod2.Spec.Containers[0])
	as.Nil(err)
	select {
	case <-time.After(time.Millisecond):
		t.Log("When pod resourceQuantity is 0,  PreStartContainer RPC stub will be skipped")
	case <-ch:
		break
	}
}

func TestResetExtendedResource(t *testing.T) {
	as := assert.New(t)
	tmpDir, err := ioutil.TempDir("", "checkpoint")
	as.Nil(err)
	defer os.RemoveAll(tmpDir)
	ckm, err := checkpointmanager.NewCheckpointManager(tmpDir)
	as.Nil(err)
	testManager := &ManagerImpl{
		endpoints:         make(map[string]endpointInfo),
		healthyDevices:    make(map[string]sets.String),
		unhealthyDevices:  make(map[string]sets.String),
		allocatedDevices:  make(map[string]sets.String),
		podDevices:        newPodDevices(),
		checkpointManager: ckm,
	}

	extendedResourceName := "domain.com/resource"
	testManager.podDevices.insert("pod", "con", extendedResourceName,
		constructDevices([]string{"dev1"}),
		constructAllocResp(map[string]string{"/dev/dev1": "/dev/dev1"},
			map[string]string{"/home/lib1": "/usr/lib1"}, map[string]string{}))

	testManager.healthyDevices[extendedResourceName] = sets.NewString()
	testManager.healthyDevices[extendedResourceName].Insert("dev1")
	// checkpoint is present, indicating node hasn't been recreated
	err = testManager.writeCheckpoint()
	as.Nil(err)

	as.False(testManager.ShouldResetExtendedResourceCapacity())

	// checkpoint is absent, representing node recreation
	ckpts, err := ckm.ListCheckpoints()
	as.Nil(err)
	for _, ckpt := range ckpts {
		err = ckm.RemoveCheckpoint(ckpt)
		as.Nil(err)
	}
	as.True(testManager.ShouldResetExtendedResourceCapacity())
}

func allocateStubFunc() func(devs []string) (*pluginapi.AllocateResponse, error) {
	return func(devs []string) (*pluginapi.AllocateResponse, error) {
		resp := new(pluginapi.ContainerAllocateResponse)
		resp.Envs = make(map[string]string)
		for _, dev := range devs {
			switch dev {
			case "dev1":
				resp.Devices = append(resp.Devices, &pluginapi.DeviceSpec{
					ContainerPath: "/dev/aaa",
					HostPath:      "/dev/aaa",
					Permissions:   "mrw",
				})

				resp.Devices = append(resp.Devices, &pluginapi.DeviceSpec{
					ContainerPath: "/dev/bbb",
					HostPath:      "/dev/bbb",
					Permissions:   "mrw",
				})

				resp.Mounts = append(resp.Mounts, &pluginapi.Mount{
					ContainerPath: "/container_dir1/file1",
					HostPath:      "host_dir1/file1",
					ReadOnly:      true,
				})

			case "dev2":
				resp.Devices = append(resp.Devices, &pluginapi.DeviceSpec{
					ContainerPath: "/dev/ccc",
					HostPath:      "/dev/ccc",
					Permissions:   "mrw",
				})

				resp.Mounts = append(resp.Mounts, &pluginapi.Mount{
					ContainerPath: "/container_dir1/file2",
					HostPath:      "host_dir1/file2",
					ReadOnly:      true,
				})

				resp.Envs["key1"] = "val1"
			}
		}
		resps := new(pluginapi.AllocateResponse)
		resps.ContainerResponses = append(resps.ContainerResponses, resp)
		return resps, nil
	}
}

func makeDevice(devOnNUMA checkpoint.DevicesPerNUMA, topology bool) map[string]pluginapi.Device {
	res := make(map[string]pluginapi.Device)
	var topologyInfo *pluginapi.TopologyInfo
	for node, devs := range devOnNUMA {
		if topology {
			topologyInfo = &pluginapi.TopologyInfo{Nodes: []*pluginapi.NUMANode{{ID: node}}}
		} else {
			topologyInfo = nil
		}
		for idx := range devs {
			res[devs[idx]] = pluginapi.Device{ID: devs[idx], Topology: topologyInfo}
		}
	}
	return res
}

const deviceManagerCheckpointFilename = "kubelet_internal_checkpoint"

var oldCheckpoint string = `{"Data":{"PodDeviceEntries":[{"PodUID":"13ac2284-0d19-44b7-b94f-055b032dba9b","ContainerName":"centos","ResourceName":"example.com/deviceA","DeviceIDs":["DevA3"],"AllocResp":"CiIKHUVYQU1QTEVDT01ERVZJQ0VBX0RFVkEzX1RUWTEwEgEwGhwKCi9kZXYvdHR5MTASCi9kZXYvdHR5MTAaAnJ3"},{"PodUID":"86b9a017-c9ca-4069-815f-46ca3e53c1e4","ContainerName":"centos","ResourceName":"example.com/deviceA","DeviceIDs":["DevA4"],"AllocResp":"CiIKHUVYQU1QTEVDT01ERVZJQ0VBX0RFVkE0X1RUWTExEgEwGhwKCi9kZXYvdHR5MTESCi9kZXYvdHR5MTEaAnJ3"}],"RegisteredDevices":{"example.com/deviceA":["DevA1","DevA2","DevA3","DevA4"]}},"Checksum":405612085}`

func TestReadPreNUMACheckpoint(t *testing.T) {
	socketDir, socketName, _, err := tmpSocketDir()
	require.NoError(t, err)
	defer os.RemoveAll(socketDir)

	err = ioutil.WriteFile(filepath.Join(socketDir, deviceManagerCheckpointFilename), []byte(oldCheckpoint), 0644)
	require.NoError(t, err)

	topologyStore := topologymanager.NewFakeManager()
	nodes := []cadvisorapi.Node{{Id: 0}}
	m, err := newManagerImpl(socketName, nodes, topologyStore)
	require.NoError(t, err)

	// TODO: we should not calling private methods, but among the existing tests we do anyway
	err = m.readCheckpoint()
	require.NoError(t, err)
}
