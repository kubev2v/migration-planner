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

type domainSnapshot struct {
	Snapshot Snapshot
	OrgID    string
}

func NewInventoryStats(assessments []Assessment) InventoryStats {
	domainSnapshots := make([]domainSnapshot, 0, len(assessments))
	orgIDs := make(map[string]struct{})

	for _, a := range assessments {
		latesSnapshot := a.Snapshots[0] // we called List which orders snapshots by created_at. So we get the latest here

		domain, err := getDomainNameFromAssessment(a)
		if err != nil {
			zap.S().Debugw("failed to get domain from username", "error", err, "username", a.Username)
			domain = a.OrgID
		}

		domainSnapshots = append(domainSnapshots, domainSnapshot{
			Snapshot: latesSnapshot,
			OrgID:    domain,
		})
		orgIDs[a.OrgID] = struct{}{}
	}

	return InventoryStats{
		Vms:              computeVmStats(domainSnapshots),
		Os:               computeOsStats(domainSnapshots),
		TotalInventories: len(domainSnapshots),
		TotalAssessments: len(assessments),
		TotalCustomers:   len(orgIDs),
		Storage:          computeStorageStats(domainSnapshots),
	}
}

func computeVmStats(domainSnapshots []domainSnapshot) VmStats {
	total := 0
	os := make(map[string]int)
	orgTotal := make(map[string]int)

	for _, ds := range domainSnapshots {
		total += ds.Snapshot.Inventory.Data.Vms.Total
		for k, v := range ds.Snapshot.Inventory.Data.Vms.Os {
			if oldValue, found := os[k]; found {
				oldValue += v
				os[k] = oldValue
			} else {
				os[k] = v
			}
		}
		orgTotal[ds.OrgID] = ds.Snapshot.Inventory.Data.Vms.Total
	}

	return VmStats{
		Total:           total,
		TotalByOS:       os,
		TotalByCustomer: orgTotal,
	}
}

func computeOsStats(domainSnapshots []domainSnapshot) OsStats {
	os := make(map[string]struct{})

	for _, ds := range domainSnapshots {
		for k := range ds.Snapshot.Inventory.Data.Vms.Os {
			os[k] = struct{}{}
		}
	}

	return OsStats{Total: len(os)}
}

func computeStorageStats(domainSnapshots []domainSnapshot) []StorageCustomerStats {
	stats := make([]StorageCustomerStats, 0, len(domainSnapshots))
	statsPerCustomer := make(map[string]StorageCustomerStats)

	for _, ds := range domainSnapshots {
		storageSnapshotStats := computeSnapshotStorageStats(&ds.Snapshot)
		val, found := statsPerCustomer[ds.OrgID]
		if found {
			statsPerCustomer[ds.OrgID] = StorageCustomerStats{
				Domain:          ds.OrgID,
				TotalByProvider: sum(val.TotalByProvider, storageSnapshotStats),
			}
		} else {
			statsPerCustomer[ds.OrgID] = StorageCustomerStats{Domain: ds.OrgID, TotalByProvider: storageSnapshotStats}
		}
	}

	for _, v := range statsPerCustomer {
		stats = append(stats, v)
	}

	return stats
}

func computeSnapshotStorageStats(snapshot *Snapshot) map[string]int {
	totalByProvider := make(map[string]int)

	for _, storage := range snapshot.Inventory.Data.Infra.Datastores {
		if val, found := totalByProvider[storage.Type]; found {
			val += storage.TotalCapacityGB
			totalByProvider[storage.Type] = val
			continue
		}
		totalByProvider[storage.Type] = storage.TotalCapacityGB
	}

	return totalByProvider
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

func getDomainNameFromAssessment(a Assessment) (string, error) {
	const (
		dotChar = "."
		atChar  = "@"
	)

	// if email domain not set, try to get the domain from username
	if !strings.Contains(a.Username, atChar) {
		return "", fmt.Errorf("username %q is not an email", a.Username)
	}

	domain := strings.Split(a.Username, atChar)[1]
	if strings.TrimSpace(domain) == "" {
		return "", fmt.Errorf("username %q is malformatted", a.Username)
	}

	// split the domain name by subdomain and return only the top domain
	// a.b.c.redhat.com => redhat.com
	parts := strings.Split(domain, dotChar)
	if len(parts) < 2 {
		return domain, nil
	}

	return strings.Join(parts[len(parts)-2:], dotChar), nil
}
