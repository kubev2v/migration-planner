package metrics

import (
	"context"
	"fmt"

	"github.com/kubev2v/migration-planner/internal/store"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

type inventoryStatsCollector struct {
	store             store.Store
	totalVm           *prometheus.Desc
	totalCustomers    *prometheus.Desc
	totalInventories  *prometheus.Desc
	totalVmByOs       *prometheus.Desc
	totalVmByCustomer *prometheus.Desc // WARN: possible high cadinality
}

func newInventoryStatsCollector(s store.Store) prometheus.Collector {
	fqName := func(name string) string {
		return fmt.Sprintf("%s_inventory_%s", assistedMigration, name)
	}

	return &inventoryStatsCollector{
		store: s,
		totalVm: prometheus.NewDesc(
			fqName("vms_total"),
			"Total number of vms.",
			nil,
			prometheus.Labels{},
		),
		totalCustomers: prometheus.NewDesc(
			fqName("customers_total"),
			"Total number of customers. Organization is counted.",
			nil,
			prometheus.Labels{},
		),
		totalInventories: prometheus.NewDesc(
			fqName("inventories_total"),
			"Total number of inventories",
			nil,
			prometheus.Labels{},
		),
		totalVmByOs: prometheus.NewDesc(
			fqName("vms_by_os_total"),
			"Total VMs by OS type",
			[]string{"os"},
			prometheus.Labels{},
		),
		totalVmByCustomer: prometheus.NewDesc(
			fqName("vms_by_customer_total"),
			"Total VM by customers",
			[]string{"org_id"},
			prometheus.Labels{},
		),
	}
}

func (c *inventoryStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.totalVm
	ch <- c.totalCustomers
	ch <- c.totalInventories
	ch <- c.totalVmByOs
	ch <- c.totalVmByCustomer
}

// Collect implements Collector.
func (c *inventoryStatsCollector) Collect(ch chan<- prometheus.Metric) {
	stats, err := c.store.Statistics(context.Background())
	if err != nil {
		zap.S().Named("inventory_collector").Errorf("failed to collect inventory statistics: %s", err)
		return
	}
	ch <- prometheus.MustNewConstMetric(c.totalVm, prometheus.GaugeValue, float64(stats.Vms.Total))
	ch <- prometheus.MustNewConstMetric(c.totalInventories, prometheus.GaugeValue, float64(stats.TotalInventories))
	ch <- prometheus.MustNewConstMetric(c.totalCustomers, prometheus.GaugeValue, float64(stats.TotalCustomers))

	for osType, total := range stats.Vms.TotalByOS {
		ch <- prometheus.MustNewConstMetric(c.totalVmByOs, prometheus.GaugeValue, float64(total), osType)
	}

	for orgID, total := range stats.Vms.TotalByCustomer {
		ch <- prometheus.MustNewConstMetric(c.totalVmByCustomer, prometheus.GaugeValue, float64(total), orgID)
	}
}
