package service

import (
	"context"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"k8s.io/mount-utils"
	"os"
	"strings"
)

var VolumeSet FakeVolumes

func init() {
	VolumeSet = make(FakeVolumes, 0)
}

var (
	basePath string // 172.18.0.1:/home/nfsdata
)

type ControllerService struct {
	mounter mount.Interface //依然要初始化这个
}

func NewControllerService() *ControllerService {
	return &ControllerService{mounter: mount.New("")}
}

func (cs *ControllerService) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.Info("删除volume")
	vid := req.GetVolumeId()
	klog.Info("删除的volume是：", vid)
	pvName := strings.Replace(vid, "jtthink-volume-", "", -1)
	klog.Info("删除的pv是：", pvName)

	if err := MountTemp(basePath, pvName, cs.mounter, false); err != nil {
		return nil, err
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (s *ControllerService) ControllerPublishVolume(ctx context.Context, request *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	klog.Info("发布PublishVolume")
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (s *ControllerService) ControllerUnpublishVolume(ctx context.Context, request *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	klog.Info("执行UnpublishVolume")
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (s *ControllerService) ValidateVolumeCapabilities(ctx context.Context, request *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ValidateVolumeCapabilities not implemented")

}

func (s *ControllerService) ListVolumes(ctx context.Context, request *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	klog.Info("列出volume")
	return &csi.ListVolumesResponse{
		Entries: VolumeSet.List(),
	}, nil
}

func (s *ControllerService) GetCapacity(ctx context.Context, request *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return &csi.GetCapacityResponse{
		AvailableCapacity: 100 * 1024 * 1024,
	}, nil
}

func (s *ControllerService) ControllerGetCapabilities(ctx context.Context, request *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	capList := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME, //删除和创建volume
		//csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME, // 包含attach过程，把块存储挂载到宿主机（好比一个磁盘）
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
	return nil, status.Errorf(codes.Unimplemented, "method CreateSnapshot not implemented")

}

func (s *ControllerService) DeleteSnapshot(ctx context.Context, request *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteSnapshot not implemented")
}

func (s *ControllerService) ListSnapshots(ctx context.Context, request *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListSnapshots not implemented")

}

func (s *ControllerService) ControllerExpandVolume(ctx context.Context, request *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ControllerExpandVolume not implemented")

}

func (s *ControllerService) ControllerGetVolume(ctx context.Context, request *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	klog.Info("获取volume,id是:", request.VolumeId)
	v, err := VolumeSet.Get(request.VolumeId)
	if err != nil {
		return nil, err
	}
	return &csi.ControllerGetVolumeResponse{
		Volume: v,
	}, nil
}

func (cs *ControllerService) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	//klog.Info("调用 CreateVolume, 创建 volume")
	//pvName := req.GetName()
	//klog.Info("PV 名称是", pvName)
	//klog.Info("参数是:", req.GetParameters())
	//
	//basePath = req.GetParameters()["server"] + ":" + req.GetParameters()["path"]
	//
	//if err := MountTemp(basePath, pvName, cs.mounter, true); err != nil {
	//	return nil, err
	//}
	//
	//return &csi.CreateVolumeResponse{
	//	Volume: &csi.Volume{
	//		VolumeId:      "jtthink-volume-" + req.GetName(),
	//		CapacityBytes: 0,
	//	},
	//}, nil

	klog.Info("调用 CreateVolume, 创建 volume")
	klog.Info("PV 名称是", req.GetName())
	klog.Info("参数是:", req.GetParameters())
	basePath = req.GetParameters()["server"] + ":" + req.GetParameters()["path"]
	tmpPath := "/tmp/"
	volCap := &csi.VolumeCapability{
		AccessType: &csi.VolumeCapability_Mount{
			Mount: &csi.VolumeCapability_MountVolume{},
		},
	}
	opts := volCap.GetMount().GetMountFlags()

	// 下面是检查目录
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

	// 挂载到临时目录
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
	// 一旦挂载 /tmp 成功， 那我们就可以创建 /tmp/pvc-xxx-xx-x 子目录
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
