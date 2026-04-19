package metrics

import (
	"errors"
	"io/fs"
	"math"
	"os"
	"path/filepath"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

const gb = 1024 * 1024 * 1024

type DirectoryCollector struct {
	folderPath string
	desc       *prometheus.Desc
}

func newDirectoryCollector(name, folderPath string) prometheus.Collector {
	return &DirectoryCollector{
		folderPath: folderPath,
		desc: prometheus.NewDesc(
			prometheus.BuildFQName(assistedMigration, "directory_size", name),
			"Total size of the folder.",
			nil,
			nil,
		),
	}
}

func dirSizeBytes(root string) (int64, error) {
	fi, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	if !fi.IsDir() {
		return fi.Size(), nil
	}

	var size int64
	err = filepath.WalkDir(root, func(_ string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		size += info.Size()
		return nil
	})
	return size, err
}

func (c *DirectoryCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.desc
}

func (c *DirectoryCollector) Collect(ch chan<- prometheus.Metric) {
	n, err := dirSizeBytes(c.folderPath)
	if err != nil {
		zap.S().Named("directory_collector").Errorw(
			"failed to measure folder size",
			"path", c.folderPath,
			"error", err,
		)
		return
	}

	sizeGB := math.Round((float64(n)/gb)*100) / 100
	ch <- prometheus.MustNewConstMetric(c.desc, prometheus.GaugeValue, sizeGB)
}
