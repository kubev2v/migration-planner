package mappers_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api "github.com/kubev2v/migration-planner/api/v1alpha1"
	"github.com/kubev2v/migration-planner/internal/handlers/v1alpha1/mappers"
	model "github.com/kubev2v/migration-planner/internal/model"
)

func TestInventoryMTVStatusRoundTrip(t *testing.T) {
	for _, status := range []*api.OsInfoMtvStatus{nil, new(api.Green), new(api.Blue)} {
		name := "unreported"
		if status != nil {
			name = string(*status)
		}
		t.Run(name, func(t *testing.T) {
			input := api.Inventory{
				Vcenter: &api.InventoryData{Vms: api.VMs{
					OsInfo: &map[string]api.OsInfo{
						"guest": {Count: 1, Supported: true, MtvStatus: status},
					},
				}},
			}
			data, err := json.Marshal(mappers.InventoryFromAPI(input))
			require.NoError(t, err)
			var stored model.Inventory
			require.NoError(t, json.Unmarshal(data, &stored))
			output := mappers.InventoryToAPI(stored)
			require.NotNil(t, output.Vcenter)
			require.NotNil(t, output.Vcenter.Vms.OsInfo)
			assert.Equal(t, status, (*output.Vcenter.Vms.OsInfo)["guest"].MtvStatus)
		})
	}
}
