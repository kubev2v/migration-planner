package collector

import (
	vspheremodel "github.com/kubev2v/forklift/pkg/controller/provider/model/vsphere"
	apiplanner "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/opa"
)

func updateOsInfoForVM(vm *vspheremodel.VM, inv *apiplanner.InventoryData) {
	guestName := vmGuestName(*vm)
	osInfoMap := *inv.Vms.OsInfo
	osInfo, found := osInfoMap[guestName]

	if !found || osInfo.Supported {
		osInfo.Supported = isOsSupported(vm.Concerns)
	}
	osInfo.Count++

	opa.AddOSUpgradeConcernToVM(vm, guestName) // TODO: The long-term solution should be implemented as part of ECOPROJECT-3571.
	if !osInfo.Supported && osInfo.UpgradeRecommendation == nil {
		osInfo.UpgradeRecommendation = addUpgradeRecommendationIfExist(vm.Concerns)
	}

	osInfoMap[guestName] = osInfo
}

func addUpgradeRecommendationIfExist(concerns []vspheremodel.Concern) *string {
	for _, c := range concerns {
		if c.Id == "vmware.os.upgrade.recommendation" {
			return &c.Assessment
		}
	}
	return nil
}

func isOsSupported(concerns []vspheremodel.Concern) bool {
	for _, concern := range concerns {
		if concern.Id == "vmware.os.unsupported" {
			return false
		}
	}
	return true
}

func vmGuestName(vm vspheremodel.VM) string {
	if vm.GuestNameFromVmwareTools != "" {
		return vm.GuestNameFromVmwareTools
	}
	return vm.GuestName
}
