package src

import (
	"context"
	"fmt"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
	"os"
	"path/filepath"
	"sobey-runtime/common"
	util "sobey-runtime/utils"
	"strings"
	"time"
)

func (ss *sobeyService) ListImages(ctx context.Context, req *runtimeapi.ListImagesRequest) (*runtimeapi.ListImagesResponse, error) {
	var result []*runtimeapi.Image
	return &runtimeapi.ListImagesResponse{Images: result}, nil
}

func (ss *sobeyService) ImageStatus(ctx context.Context, req *runtimeapi.ImageStatusRequest) (*runtimeapi.ImageStatusResponse, error) {
	return &runtimeapi.ImageStatusResponse{}, nil
}

func (ss *sobeyService) PullImage(ctx context.Context, req *runtimeapi.PullImageRequest) (*runtimeapi.PullImageResponse, error) {
	image := req.Image.Image
	if strings.HasSuffix(image, ":latest") {
		imageArr := strings.Split(image, ":")
		image = strings.Join(imageArr[:len(imageArr)-1], ":")
	}
	srcPath := fmt.Sprintf("%s%s", ss.repo, image)
	desPath := filepath.Join(common.ServerImageDirPath, image)
	err := util.DownLoadFile(srcPath, desPath)
	if err != nil {
		fmt.Println(err)
	}
	return &runtimeapi.PullImageResponse{ImageRef: req.Image.Image}, nil
}

func (ss *sobeyService) RemoveImage(ctx context.Context, req *runtimeapi.RemoveImageRequest) (*runtimeapi.RemoveImageResponse, error) {
	return &runtimeapi.RemoveImageResponse{}, nil
}

func (ss *sobeyService) ImageFsInfo(ctx context.Context, req *runtimeapi.ImageFsInfoRequest) (*runtimeapi.ImageFsInfoResponse, error) {
	bytes, inodes, err := dirSize(common.ServerImageDirPath)
	if err != nil {
		return nil, err
	}
	return &runtimeapi.ImageFsInfoResponse{
		ImageFilesystems: []*runtimeapi.FilesystemUsage{
			{
				Timestamp: time.Now().Unix(),
				FsId: &runtimeapi.FilesystemIdentifier{
					Mountpoint: common.ServerImageDirPath,
				},
				UsedBytes: &runtimeapi.UInt64Value{
					Value: uint64(bytes),
				},
				InodesUsed: &runtimeapi.UInt64Value{
					Value: uint64(inodes),
				},
			},
		},
	}, nil
}

func dirSize(path string) (int64, int64, error) {
	bytes := int64(0)
	inodes := int64(0)
	err := filepath.Walk(path, func(dir string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		inodes++
		if !info.IsDir() {
			bytes += info.Size()
		}
		return nil
	})
	return bytes, inodes, err
}
