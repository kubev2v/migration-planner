package duckdb_parser

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("VM network filtering", func() {
	It("preserves network names with deprecated lambda syntax disabled", func() {
		parser, db, cleanup := setupTestParser(GinkgoTB(), nil)
		DeferCleanup(cleanup)
		db.SetMaxOpenConns(1)

		_, err := db.Exec(`SET lambda_syntax = 'DISABLE_SINGLE_ARROW'`)
		Expect(err).To(Succeed())

		_, err = db.Exec(`
			INSERT INTO vinfo ("VM ID", "VM", "Network #1", "Network #2", "Network #3", "Network #4", "Network #5", "Network #6")
			VALUES ('vm-1', 'first', 'management', NULL, '', 'VM', 'storage', 'management')
		`)
		Expect(err).To(Succeed())

		vms, err := parser.VMs(context.Background(), Filters{}, Options{})
		Expect(err).To(Succeed())
		Expect(vms).To(HaveLen(1))
		Expect([]string(vms[0].Networks)).To(Equal([]string{"management", "storage", "management"}))
	})
})
