package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.uber.org/zap"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
)

const (
	SourceTypeAgent   string = "agent"
	SourceTypeRvtools string = "rvtools"
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

type CustomerAssessments struct {
	AgentCount  int
	RvToolCount int
}

type StorageCustomerStats struct {
	TotalByProvider map[string]int
	Domain          string
}

type InventoryStats struct {
	Vms                                VmStats
	Os                                 OsStats
	TotalCustomers                     int
	TotalAssessmentsByCustomerBySource map[string]CustomerAssessments
	TotalInventories                   int
	Storage                            []StorageCustomerStats
}

type domainInventory struct {
	Inventory api.InventoryData
	OrgID     string
}

func NewInventoryStats(assessments []Assessment) InventoryStats {
	domainInventories := make([]domainInventory, 0, len(assessments))
	orgIDs := make(map[string]struct{})

	for _, a := range assessments {
		latesSnapshot := a.Snapshots[0] // we called List which orders snapshots by created_at. So we get the latest here

		domain, err := getDomainNameFromAssessment(a)
		if err != nil {
			zap.S().Debugw("failed to get domain from username", "error", err, "username", a.Username)
			domain = a.OrgID
		}

		inventory := &api.Inventory{}

		if err := json.Unmarshal(latesSnapshot.Inventory, &inventory); err != nil {
			continue
		}

		domainInventories = append(domainInventories, domainInventory{
			Inventory: *inventory.Vcenter,
			OrgID:     domain,
		})
		orgIDs[a.OrgID] = struct{}{}
	}

	return InventoryStats{
		Vms:                                computeVmStats(domainInventories),
		Os:                                 computeOsStats(domainInventories),
		TotalInventories:                   len(domainInventories),
		TotalAssessmentsByCustomerBySource: computeAssessmentsByCustomerBySource(assessments),
		TotalCustomers:                     len(orgIDs),
		Storage:                            computeStorageStats(domainInventories),
	}
}

func computeVmStats(domainInventories []domainInventory) VmStats {
	total := 0
	os := make(map[string]int)
	orgTotal := make(map[string]int)

	for _, ds := range domainInventories {
		total += ds.Inventory.Vms.Total
		if ds.Inventory.Vms.OsInfo != nil {
			for k, v := range *ds.Inventory.Vms.OsInfo {
				os[k] += v.Count
			}
		}
		orgTotal[ds.OrgID] += ds.Inventory.Vms.Total
	}

	return VmStats{
		Total:           total,
		TotalByOS:       os,
		TotalByCustomer: orgTotal,
	}
}

func computeOsStats(domainSnapshots []domainInventory) OsStats {
	os := make(map[string]struct{})

	for _, ds := range domainSnapshots {
		if ds.Inventory.Vms.OsInfo != nil {
			for k := range *ds.Inventory.Vms.OsInfo {
				os[k] = struct{}{}
			}
		}
	}

	return OsStats{Total: len(os)}
}

func computeAssessmentsByCustomerBySource(assessments []Assessment) map[string]CustomerAssessments {
	assessmentsByCustomer := make(map[string]CustomerAssessments)

	for _, a := range assessments {
		domain, err := getDomainNameFromAssessment(a)
		if err != nil {
			zap.S().Debugw("failed to get domain from username", "error", err, "username", a.Username)
			domain = a.OrgID
		}

		customer := assessmentsByCustomer[domain]

		switch a.SourceType {
		case SourceTypeAgent:
			customer.AgentCount++
		case SourceTypeRvtools:
			customer.RvToolCount++
		}

		assessmentsByCustomer[domain] = customer
	}

	return assessmentsByCustomer
}

func computeStorageStats(domainInventories []domainInventory) []StorageCustomerStats {
	stats := make([]StorageCustomerStats, 0, len(domainInventories))
	statsPerCustomer := make(map[string]StorageCustomerStats)

	for _, ds := range domainInventories {
		storageSnapshotStats := computeInventoryStorageStats(ds.Inventory)
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

func computeInventoryStorageStats(inventory api.InventoryData) map[string]int {
	totalByProvider := make(map[string]int)

	for _, storage := range inventory.Infra.Datastores {
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
