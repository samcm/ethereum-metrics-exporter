package disk

import (
	"context"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

// UsageMetrics reports disk usage metrics
type UsageMetrics interface {
	// StartAsync starts the disk usage metrics collection.
	StartAsync(ctx context.Context)
	// GetUsage returns the usage of the directories.
	GetUsage(ctx context.Context, directories []string) ([]Usage, error)
}

type diskUsage struct {
	log         logrus.FieldLogger
	metrics     Metrics
	directories []string
}

// NewUsage returns a new DiskUsage instance.
func NewUsage(ctx context.Context, log logrus.FieldLogger, namespace string, directories []string) (UsageMetrics, error) {
	return &diskUsage{
		log:         log,
		metrics:     NewMetrics(log, namespace),
		directories: directories,
	}, nil
}

func (d *diskUsage) StartAsync(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second * 60):
				if _, err := d.GetUsage(ctx, d.directories); err != nil {
					d.log.WithError(err).Error("Failed to get disk usage")
				}
			}
		}
	}()
}

func (d *diskUsage) GetUsage(ctx context.Context, directories []string) ([]Usage, error) {
	//nolint:prealloc // we dont know how much success we'll have
	var diskUsed []Usage

	for _, directory := range directories {
		info, err := os.Lstat(directory)
		if err != nil {
			d.log.WithField("directory", directory).Warn("Directory does not exist")
			continue
		}

		used, err := getDiskUsed(directory, info)
		if err != nil {
			d.log.WithField("directory", directory).WithError(err).Error("Failed to get usage")
			continue
		}

		diskUsed = append(diskUsed, Usage{
			Directory:  directory,
			UsageBytes: used,
		})
	}

	for _, disk := range diskUsed {
		d.metrics.ObserveDiskUsage(disk)
	}

	return diskUsed, nil
}

func getDiskUsed(currentPath string, info os.FileInfo) (int64, error) {
	size := info.Size()
	if !info.IsDir() {
		return size, nil
	}

	dir, err := os.Open(currentPath)

	if err != nil {
		return size, err
	}
	defer dir.Close()

	files, err := dir.Readdir(-1)
	if err != nil {
		return size, err
	}

	for _, file := range files {
		if file.Name() == "." || file.Name() == ".." {
			continue
		}

		s, _ := getDiskUsed(currentPath+"/"+file.Name(), file)

		size += s
	}

	return size, nil
}
