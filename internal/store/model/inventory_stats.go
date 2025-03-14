package model

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

type InventoryStats struct {
	Vms              VmStats
	Os               OsStats
	TotalCustomers   int
	TotalInventories int
}

func NewInventoryStats(sources []Source) InventoryStats {
	return InventoryStats{
		Vms:              computeVmStats(sources),
		Os:               computeOsStats(sources),
		TotalInventories: len(sources),
		TotalCustomers:   computeTotalCustomers(sources),
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
		orgTotal[s.OrgID] = s.Inventory.Data.Vms.Total
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
