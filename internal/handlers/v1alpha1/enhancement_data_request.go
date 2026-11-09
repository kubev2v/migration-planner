package v1alpha1

import (
	"encoding/json"
	"strconv"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/handlers/validator"
)

type manualCountInput struct {
	field  string
	number *json.Number
}

func mapEnhancementDataRequest(request api.EnhancementDataRequest) (api.EnhancementData, error) {
	data := api.EnhancementData{
		DeployedEnvironment: request.DeployedEnvironment,
		ActiveEnvironments:  request.ActiveEnvironments,
		VmwareSubscription:  request.VmwareSubscription,
		VsphereCore:         request.VsphereCore,
		Nsx:                 request.Nsx,
		AriaOps:             request.AriaOps,
		AriaAutomation:      request.AriaAutomation,
		AriaSecure:          request.AriaSecure,
	}

	if request.VmwareVersionCounts != nil {
		counts, err := parseManualCounts(
			manualCountInput{
				field:  "PerpetualLicensesCount",
				number: request.VmwareVersionCounts.PerpetualLicensesCount,
			},
			manualCountInput{
				field:  "CoresLicensingCount",
				number: request.VmwareVersionCounts.CoresLicensingCount,
			},
			manualCountInput{
				field:  "EnvironmentCount",
				number: request.VmwareVersionCounts.EnvironmentCount,
			},
			manualCountInput{
				field:  "OtherNodesCount",
				number: request.VmwareVersionCounts.OtherNodesCount,
			},
		)
		if err != nil {
			return api.EnhancementData{}, err
		}

		data.VmwareVersionCounts = &api.VMwareVersionCountsInput{
			PerpetualLicensesCount: counts[0],
			CoresLicensingCount:    counts[1],
			EnvironmentCount:       counts[2],
			OtherNodesCount:        counts[3],
		}
	}

	if request.CustomerDetails != nil {
		counts, err := parseManualCounts(manualCountInput{
			field:  "PhysicalLocationsCount",
			number: request.CustomerDetails.PhysicalLocationsCount,
		})
		if err != nil {
			return api.EnhancementData{}, err
		}
		data.CustomerDetails = &api.CustomerDetailsInput{
			PhysicalLocationsCount: counts[0],
			TargetHardware:         request.CustomerDetails.TargetHardware,
		}
	}

	return data, nil
}

func parseManualCounts(inputs ...manualCountInput) ([]*int, error) {
	counts := make([]*int, len(inputs))
	for i, input := range inputs {
		if input.number == nil {
			continue
		}

		count, err := strconv.ParseInt(input.number.String(), 10, 32)
		if err != nil || count < 0 {
			if countErr := validator.ManualCountError(input.field); countErr != nil {
				return nil, countErr
			}
			return nil, err
		}

		value := int(count)
		counts[i] = &value
	}

	return counts, nil
}
