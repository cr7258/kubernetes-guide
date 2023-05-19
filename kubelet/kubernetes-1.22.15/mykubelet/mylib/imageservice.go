package mylib

import (
	cri "k8s.io/cri-api/pkg/apis"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

type MyImageService struct {
}

func (m MyImageService) ListImages(filter *runtimeapi.ImageFilter) ([]*runtimeapi.Image, error) {
	return []*runtimeapi.Image{}, nil
}

func (m MyImageService) ImageStatus(image *runtimeapi.ImageSpec) (*runtimeapi.Image, error) {
	return &runtimeapi.Image{}, nil
}

func (m MyImageService) PullImage(image *runtimeapi.ImageSpec, auth *runtimeapi.AuthConfig, podSandboxConfig *runtimeapi.PodSandboxConfig) (string, error) {
	//TODO implement me
	panic("implement me")
}

func (m MyImageService) RemoveImage(image *runtimeapi.ImageSpec) error {
	//TODO implement me
	panic("implement me")
}

func (m MyImageService) ImageFsInfo() ([]*runtimeapi.FilesystemUsage, error) {
	return []*runtimeapi.FilesystemUsage{}, nil
}

var _ cri.ImageManagerService = &MyImageService{}
