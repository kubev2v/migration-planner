package mappers_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kubev2v/migration-planner/internal/model"
	"github.com/kubev2v/migration-planner/internal/store/mappers"
	storemodel "github.com/kubev2v/migration-planner/internal/store/model"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gorm.io/gorm"
)

func TestMappers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Store Mappers Suite")
}

var _ = Describe("Source inventory version round trip", func() {
	DescribeTable("preserves inventory and version", func(version uint, raw string) {
		source, err := mappers.SourceFromStore(storemodel.Source{Inventory: []byte(raw)})
		Expect(err).NotTo(HaveOccurred())
		Expect(source.Version).To(Equal(version))
		Expect(source.Inventory).NotTo(BeNil())
		Expect(source.Inventory.VcenterId).To(Equal("vc-1"))
		Expect(source.Inventory.Vcenter).NotTo(BeNil())
		Expect(source.Inventory.Vcenter.Vms.Total).To(BeNumerically("==", 7))

		ai := model.NewAssessmentInventory(uuid.New(), uuid.New(), "main", source.Version, false, *source.Inventory)
		Expect(ai.VMsCount).To(BeNumerically("==", 7))
		Expect(ai.HostsCount).To(BeNumerically("==", 2))

		stored, err := mappers.SourceToStore(source)
		Expect(err).NotTo(HaveOccurred())
		var shape map[string]json.RawMessage
		Expect(json.Unmarshal(stored.Inventory, &shape)).To(Succeed())
		_, wrapped := shape["vcenter_id"]
		Expect(wrapped).To(Equal(version == model.InventoryVersionV2))

		restored, err := mappers.SourceFromStore(stored)
		Expect(err).NotTo(HaveOccurred())
		Expect(restored.Inventory).To(Equal(source.Inventory))
		Expect(restored.Version).To(Equal(source.Version))
		if version == model.InventoryVersionV2 {
			want := time.Date(2026, 9, 20, 10, 30, 0, 0, time.UTC)
			Expect(source.Inventory.CollectionTime).NotTo(BeNil())
			Expect(source.Inventory.CollectionTime.Equal(want)).To(BeTrue())
			Expect(source.Inventory.Clusters["cluster-1"].Vms.Total).To(BeNumerically("==", 3))
		}

		subset, err := mappers.SourceSubsetInventoryFromStore(storemodel.SourceSubsetInventory{Inventory: []byte(raw)})
		Expect(err).NotTo(HaveOccurred())
		storedSubset, err := mappers.SourceSubsetInventoryToStore(subset)
		Expect(err).NotTo(HaveOccurred())
		restoredSubset, err := mappers.SourceSubsetInventoryFromStore(storedSubset)
		Expect(err).NotTo(HaveOccurred())
		Expect(restoredSubset).To(Equal(subset))
		Expect(subset.Version).To(Equal(version))
	},
		Entry("legacy v1", uint(model.InventoryVersionV1), `{"vcenter":{"id":"vc-1"},"vms":{"total":7},"infra":{"totalHosts":2}}`),
		Entry("v2", uint(model.InventoryVersionV2), `{"vcenter_id":"vc-1","vcenter":{"vms":{"total":7},"infra":{"totalHosts":2}},"clusters":{"cluster-1":{"vms":{"total":3}}},"collection_time":"2026-09-20T10:30:00Z"}`),
	)
})

var _ = Describe("Source metadata round trip", func() {
	It("preserves source and subset metadata", func() {
		now := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
		sourceID := uuid.New()
		domain, agentVersion, token := "example.com", "1.2.3", "token"
		stored := storemodel.Source{
			Model: gorm.Model{CreatedAt: now, UpdatedAt: now},
			ID:    sourceID, Name: "source", Username: "user", OrgID: "org", VCenterID: "vc-1", OnPremises: true, UpdateType: "manual", EmailDomain: &domain,
			Agents:     []storemodel.Agent{{Model: gorm.Model{CreatedAt: now, UpdatedAt: now}, ID: uuid.New(), SourceID: sourceID, Status: "connected", StatusInfo: "ready", CredUrl: "url", Version: "v1"}},
			Labels:     []storemodel.Label{{Key: "env", Value: "test", SourceID: sourceID.String()}},
			ImageInfra: storemodel.ImageInfra{Model: gorm.Model{ID: 9, CreatedAt: now, UpdatedAt: now}, SourceID: sourceID, ImageTokenKey: "key", HttpProxyUrl: "http", HttpsProxyUrl: "https", NoProxyDomains: "localhost", CertificateChain: "cert", SshPublicKey: "ssh", IpAddress: "192.0.2.1", SubnetMask: "255.255.255.0", DefaultGateway: "192.0.2.254", Dns: "192.0.2.2", AgentVersion: &agentVersion, AgentToken: &token},
		}
		source, err := mappers.SourceFromStore(stored)
		Expect(err).NotTo(HaveOccurred())
		restored, err := mappers.SourceToStore(source)
		Expect(err).NotTo(HaveOccurred())
		Expect(restored).To(Equal(stored))
		subset := storemodel.SourceSubsetInventory{Model: gorm.Model{CreatedAt: now, UpdatedAt: now}, ID: uuid.New(), SourceID: sourceID, Name: "subset", VCenterID: "", VMsCount: 0, UpdateType: "manual", Inventory: []byte(`{"vcenter_id":"vc-1","clusters":{}}`)}
		mapped, err := mappers.SourceSubsetInventoryFromStore(subset)
		Expect(err).NotTo(HaveOccurred())
		restoredSubset, err := mappers.SourceSubsetInventoryToStore(mapped)
		Expect(err).NotTo(HaveOccurred())
		restoredSubset.Inventory = subset.Inventory
		Expect(restoredSubset).To(Equal(subset))
	})
})

var _ = Describe("Malformed source inventories", func() {
	DescribeTable("rejects malformed source and subset inventories", func(raw string) {
		_, err := mappers.SourceFromStore(storemodel.Source{Inventory: []byte(raw)})
		Expect(err).To(HaveOccurred())

		_, err = mappers.SourceSubsetInventoryFromStore(storemodel.SourceSubsetInventory{Inventory: []byte(raw)})
		Expect(err).To(HaveOccurred())
	},
		Entry("invalid JSON", `{"vms":`),
		Entry("invalid clusters shape", `{"clusters":[],"vcenter_id":"vc-1"}`),
	)
})
