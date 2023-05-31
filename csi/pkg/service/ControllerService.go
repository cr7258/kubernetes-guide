package service

import (
	"context"
	"fmt"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/utils/mount"
	"os"
	"strings"
)

var VolumeSet FakeVolumes

func init() {
	VolumeSet = make(FakeVolumes, 0)
}

type ControllerService struct {
	mounter mount.Interface //依然要初始化这个
}

func NewControllerService() *ControllerService {
	return &ControllerService{mounter: mount.New("")}
}

func (s *ControllerService) DeleteVolume(ctx context.Context, request *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.Info("删除volume")
	//TODO implement me
	VolumeSet.Delete(request.VolumeId)
	return &csi.DeleteVolumeResponse{}, nil
}

func (s *ControllerService) ControllerPublishVolume(ctx context.Context, request *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	klog.Info("发布PublishVolume")
	//TODO implement me
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (s *ControllerService) ControllerUnpublishVolume(ctx context.Context, request *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	//TODO implement me
	klog.Info("执行UnpublishVolume")
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (s *ControllerService) ValidateVolumeCapabilities(ctx context.Context, request *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	//TODO implement me
	return nil, status.Errorf(codes.Unimplemented, "method ValidateVolumeCapabilities not implemented")

}

func (s *ControllerService) ListVolumes(ctx context.Context, request *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	klog.Info("列出volume")
	//TODO implement me
	return &csi.ListVolumesResponse{
		Entries: VolumeSet.List(),
	}, nil
}

func (s *ControllerService) GetCapacity(ctx context.Context, request *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	//TODO implement me

	return &csi.GetCapacityResponse{
		AvailableCapacity: 100 * 1024 * 1024,
	}, nil
}

func (s *ControllerService) ControllerGetCapabilities(ctx context.Context, request *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	capList := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME, //删除和创建volume
		//csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME, // 包含attach过程
	}
	var caps []*csi.ControllerServiceCapability
	for _, capobj := range capList {
		c := &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{
					Type: capobj,
				},
			},
		}
		caps = append(caps, c)
	}
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: caps,
	}, nil
}

func (s *ControllerService) CreateSnapshot(ctx context.Context, request *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	//TODO implement me
	return nil, status.Errorf(codes.Unimplemented, "method CreateSnapshot not implemented")

}

func (s *ControllerService) DeleteSnapshot(ctx context.Context, request *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	//TODO implement me
	return nil, status.Errorf(codes.Unimplemented, "method DeleteSnapshot not implemented")
}

func (s *ControllerService) ListSnapshots(ctx context.Context, request *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	//TODO implement me
	return nil, status.Errorf(codes.Unimplemented, "method ListSnapshots not implemented")

}

func (s *ControllerService) ControllerExpandVolume(ctx context.Context, request *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	//TODO implement me
	return nil, status.Errorf(codes.Unimplemented, "method ControllerExpandVolume not implemented")

}

func (s *ControllerService) ControllerGetVolume(ctx context.Context, request *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	klog.Info("获取volume,id是:", request.VolumeId)
	//TODO implement me
	v, err := VolumeSet.Get(request.VolumeId)
	if err != nil {
		return nil, err
	}
	return &csi.ControllerGetVolumeResponse{
		Volume: v,
	}, nil
}

func (cs *ControllerService) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.Info("创建volume")
	klog.Info("名称是", req.GetName())
	klog.Info("参数是:", req.GetParameters())
	basePath := "172.18.0.1:/home/nfsdata" //  根目录
	tmpPath := "/tmp/"
	volCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
	}
	opts := volCap.GetMount().GetMountFlags()

	//下面是检查目录
	nn, err := cs.mounter.IsLikelyNotMountPoint(tmpPath)
	if err != nil {
		if os.IsNotExist(err) {
			err = os.MkdirAll(tmpPath, 0777)
			if err != nil {
				return nil, status.Error(codes.Internal, err.Error())
			}
			nn = true
		}
	}
	if !nn {
		return nil, status.Error(codes.Internal, "无法处理tmp目录进行临时挂载")
	}

	//挂载到临时目录
	err = cs.mounter.Mount(basePath, tmpPath, "nfs", opts)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer func() {
		err := mount.CleanupMountPoint(tmpPath, cs.mounter, true)
		if err != nil {
			klog.Warningf("cs 反挂出错", err)
		}
	}()
	//一旦挂载成功， 那我们就可以再/tmp/pvc-xxx-xx-x-
	if err = os.Mkdir(tmpPath+req.GetName(), 0777); err != nil && !os.IsExist(err) {
		return nil, status.Errorf(codes.Internal, "failed to make subdirectory: %v", err.Error())
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      "jtthink-volume-" + req.GetName(),
			CapacityBytes: 0,
		},
	}, nil

}

var _ csi.ControllerServer = &ControllerService{}

// ////////////////////////////////以下是自定义函数

const (
	paramServer           = "server"
	paramShare            = "share"
	paramSubDir           = "subdir"
	mountOptionsField     = "mountoptions"
	mountPermissionsField = "mountpermissions"
	pvcNameKey            = "csi.storage.k8s.io/pvc/name"
	pvcNamespaceKey       = "csi.storage.k8s.io/pvc/namespace"
	pvNameKey             = "csi.storage.k8s.io/pv/name"
	pvcNameMetadata       = "${pvc.metadata.name}"
	pvcNamespaceMetadata  = "${pvc.metadata.namespace}"
	pvNameMetadata        = "${pv.metadata.name}"
	separator             = "#"
)

type nfsVolume struct {
	// Volume id
	id string
	// Address of the NFS server.
	// Matches paramServer.
	server string
	// Base directory of the NFS server to create volumes under
	// Matches paramShare.
	baseDir string
	// Subdirectory of the NFS server to create volumes under
	subDir string
	// size of volume
	size int64
	// pv name when subDir is not empty
	uuid string
}

const (
	idServer = iota
	idBaseDir
	idSubDir
	idUUID
	totalIDElements // Always last
)

func replaceWithMap(str string, m map[string]string) string {
	for k, v := range m {
		if k != "" {
			str = strings.ReplaceAll(str, k, v)
		}
	}
	return str
}

// 官方的一个 拼凑ID的方式
func getVolumeIDFromNfsVol(vol *nfsVolume) string {
	idElements := make([]string, totalIDElements)
	idElements[idServer] = strings.Trim(vol.server, "/")
	idElements[idBaseDir] = strings.Trim(vol.baseDir, "/")
	idElements[idSubDir] = strings.Trim(vol.subDir, "/")
	idElements[idUUID] = vol.uuid
	return strings.Join(idElements, separator)
}
func newNFSVolume(name string, size int64, params map[string]string) (*nfsVolume, error) {
	var server, baseDir, subDir string
	subDirReplaceMap := map[string]string{}

	for k, v := range params {
		switch strings.ToLower(k) {
		case paramServer:
			server = v
		case paramShare:
			baseDir = v
		case paramSubDir:
			subDir = v
		case pvcNamespaceKey:
			subDirReplaceMap[pvcNamespaceMetadata] = v
		case pvcNameKey:
			subDirReplaceMap[pvcNameMetadata] = v
		case pvNameKey:
			subDirReplaceMap[pvNameMetadata] = v
		}
	}

	if server == "" {
		return nil, fmt.Errorf("%v is a required parameter", paramServer)
	}

	vol := &nfsVolume{
		server:  server,
		baseDir: baseDir,
		size:    size,
	}
	if subDir == "" {
		// use pv name by default if not specified
		vol.subDir = name
	} else {
		// replace pv/pvc name namespace metadata in subDir
		vol.subDir = replaceWithMap(subDir, subDirReplaceMap)
		// make volume id unique if subDir is provided
		vol.uuid = name
	}
	vol.id = getVolumeIDFromNfsVol(vol)
	return vol, nil
}
