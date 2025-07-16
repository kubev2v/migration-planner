package model

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
)

type VmStats struct {
	// Total is the total number of vms
	Total int
	// Total number of vm by customer (key is organization id)
	TotalByCustomer map[string]int
	// Total number of vm by OS.
	TotalByOS map[string]int
}

type OsStats struct {
	// TotalTypes is total of os types. (e.g. how many different os we found)
	Total int
}

type StorageCustomerStats struct {
	TotalByProvider map[string]int
	Domain          string
}

type InventoryStats struct {
	Vms              VmStats
	Os               OsStats
	TotalCustomers   int
	TotalAssessments int
	TotalInventories int
	Storage          []StorageCustomerStats
}

func NewInventoryStats(sources []Source) InventoryStats {
	return InventoryStats{
		Vms:              computeVmStats(sources),
		Os:               computeOsStats(sources),
		TotalInventories: computeInventories(sources),
		TotalAssessments: len(sources),
		TotalCustomers:   computeTotalCustomers(sources),
		Storage:          computeStorateStats(sources),
	}
}

func computeVmStats(sources []Source) VmStats {
	total := 0
	os := make(map[string]int)
	orgTotal := make(map[string]int)
	for _, s := range sources {
		if s.Inventory == nil {
			continue
		}
		total += s.Inventory.Data.Vms.Total
		for k, v := range s.Inventory.Data.Vms.Os {
			if oldValue, found := os[k]; found {
				oldValue += v
				os[k] = oldValue
			} else {
				os[k] = v
			}
		}

		domain, err := getDomainName(s)
		if err != nil {
			zap.S().Debugw("failed to get domain from username", "error", err, "username", s.Username)
			domain = s.OrgID
		}
		orgTotal[domain] = s.Inventory.Data.Vms.Total
	}

	return VmStats{
		Total:           total,
		TotalByOS:       os,
		TotalByCustomer: orgTotal,
	}
}

func computeOsStats(sources []Source) OsStats {
	os := make(map[string]any)
	for _, s := range sources {
		if s.Inventory == nil {
			continue
		}
		for k := range s.Inventory.Data.Vms.Os {
			os[k] = struct{}{}
		}
	}

	total := 0
	for range os {
		total += 1
	}

	return OsStats{Total: total}
}

func computeTotalCustomers(sources []Source) int {
	orgIDs := make(map[string]any)
	for _, s := range sources {
		orgIDs[s.OrgID] = struct{}{}
	}

	total := 0
	for range orgIDs {
		total += 1
	}

	return total
}

func computeStorateStats(sources []Source) []StorageCustomerStats {
	stats := make([]StorageCustomerStats, 0, len(sources))
	statsPerCustomer := make(map[string]StorageCustomerStats)
	for _, s := range sources {
		if s.Inventory == nil {
			continue
		}

		domain, err := getDomainName(s)
		if err != nil {
			zap.S().Debugw("failed to get domain from username", "error", err, "username", s.Username)
			domain = s.OrgID
		}

		storageSourceStats := computeSourceStorageStats(s)
		val, found := statsPerCustomer[domain]
		if found {
			statsPerCustomer[domain] = StorageCustomerStats{
				Domain:          domain,
				TotalByProvider: sum(val.TotalByProvider, storageSourceStats),
			}
		} else {
			statsPerCustomer[domain] = StorageCustomerStats{Domain: domain, TotalByProvider: storageSourceStats}
		}
	}

	for _, v := range statsPerCustomer {
		stats = append(stats, v)
	}

	return stats
}

func computeSourceStorageStats(source Source) map[string]int {
	totalByProvider := make(map[string]int)

	for _, storage := range source.Inventory.Data.Infra.Datastores {
		if val, found := totalByProvider[storage.Type]; found {
			val += storage.TotalCapacityGB
			totalByProvider[storage.Type] = val
			continue
		}
		totalByProvider[storage.Type] = storage.TotalCapacityGB
	}

	return totalByProvider
}

func computeInventories(sources []Source) int {
	total := 0
	for _, s := range sources {
		if s.Inventory == nil {
			continue
		}
		total += 1
	}
	return total
}

func sum(m1, m2 map[string]int) map[string]int {
	result := make(map[string]int)

	for k1, v1 := range m1 {
		v2, found := m2[k1]
		if found {
			v1 += v2
			result[k1] = v1
			continue
		}
		result[k1] = v1
	}

	// sum values from m2 not found in m1
	for k2, v2 := range m2 {
		if _, found := m1[k2]; !found {
			result[k2] = v2
		}
	}

	return result
}

func getDomainName(s Source) (string, error) {
	const (
		dotChar = "."
		atChar  = "@"
	)

	if s.EmailDomain != nil && *s.EmailDomain != "" {
		return *s.EmailDomain, nil
	}

	// if email domain not set, try to get the domain from username
	if !strings.Contains(s.Username, atChar) {
		return "", fmt.Errorf("username %q is not an email", s.Username)
	}

	domain := strings.Split(s.Username, atChar)[1]
	if strings.TrimSpace(domain) == "" {
		return "", fmt.Errorf("username %q is malformatted", s.Username)
	}

	// split the domain name by subdomain and return only the top domain
	// a.b.c.redhat.com => redhat.com
	parts := strings.Split(domain, dotChar)
	if len(parts) < 2 {
		return domain, nil
	}

	return strings.Join(parts[len(parts)-2:], dotChar), nil
}
