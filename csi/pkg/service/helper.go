package service

import (
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
	"os"
)

func MountTemp(basePath, pvName string, mounter mount.Interface, createSubDir bool) error {
	tmpPath := "/tmp/"
	volCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
	}
	opts := volCap.GetMount().GetMountFlags()

	// 下面是检查目录
	nn, err := mounter.IsLikelyNotMountPoint(tmpPath)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(tmpPath, 0777)
			if err != nil {
				return status.Error(codes.Internal, "创建目录失败")
			}
			nn = true
		}
	}
	if !nn {
		return status.Error(codes.Internal, "无法处理tmp目录进行临时挂载")
	}

	// 挂载到临时目录
	err = mounter.Mount(basePath, tmpPath, "nfs", opts)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}
	defer func() {
		err := mount.CleanupMountPoint(tmpPath, mounter, true)
		if err != nil {
			klog.Warningf("cs 反挂出错", err)
		}
	}()

	if createSubDir {
		// 一旦挂载 /tmp 成功， 那我们就可以创建 /tmp/pvc-xxx-xx-x 子目录
		if err = os.Mkdir(tmpPath+pvName, 0777); err != nil && !os.IsExist(err) {
			return status.Errorf(codes.Internal, "无法创建子文件夹: %v", err.Error())
		}
	} else {
		if err = os.RemoveAll(tmpPath + pvName); err != nil {
			return status.Errorf(codes.Internal, "无法删除子文件夹: %v", err.Error())
		}
	}
	return nil
}
