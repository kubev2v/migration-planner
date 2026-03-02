package complexity_test

import (
	"testing"

	"github.com/kubev2v/migration-planner/pkg/estimations/complexity"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestComplexity(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Complexity Suite")
}

var _ = Describe("ClassifyOS", func() {
	DescribeTable("keyword substring matching",
		func(osName string, expectedScore int) {
			Expect(complexity.ClassifyOS(osName)).To(Equal(expectedScore))
		},
		// Score 1 keywords
		Entry("Red Hat exact", "Red Hat", 1),
		Entry("Red Hat full name", "Red Hat Enterprise Linux 8 (64-bit)", 1),
		Entry("Rocky Linux exact", "Rocky Linux", 1),
		Entry("Rocky Linux full name", "Rocky Linux 9 (64-bit)", 1),
		// Score 2 keywords
		Entry("CentOS exact", "CentOS", 2),
		Entry("CentOS full name", "CentOS 7 (64-bit)", 2),
		Entry("Windows exact", "Windows", 2),
		Entry("Windows full name", "Microsoft Windows Server 2019 (64-bit)", 2),
		// Score 3 keywords
		Entry("Ubuntu exact", "Ubuntu", 3),
		Entry("Ubuntu full name", "Ubuntu Linux (64-bit)", 3),
		Entry("SUSE exact", "SUSE Linux Enterprise", 3),
		Entry("SUSE full name", "SUSE Linux Enterprise 15 (64-bit)", 3),
		// Score 4 keywords
		Entry("Oracle exact", "Oracle", 4),
		Entry("Oracle full name", "Oracle Linux 8 (64-bit)", 4),
		Entry("Microsoft SQL exact", "Microsoft SQL", 4),
		Entry("Microsoft SQL full name", "Microsoft SQL Server 2019", 4),
		// Score 0 fallback (unclassified)
		Entry("unknown OS", "FreeBSD (64-bit)", 0),
		Entry("empty string", "", 0),
		Entry("generic other", "Other (64-bit)", 0),
		Entry("Amazon Linux", "Amazon Linux 2 (64-bit)", 0),
		// Case-insensitive matching
		Entry("all lowercase Red Hat", "red hat enterprise linux 8 (64-bit)", 1),
		Entry("all uppercase CENTOS", "CENTOS 7 (64-BIT)", 2),
		Entry("mixed case Windows", "microsoft WINDOWS server 2019", 2),
		Entry("all lowercase ubuntu", "ubuntu linux (64-bit)", 3),
		Entry("mixed case SUSE", "SUSE Linux enterprise 15 (64-bit)", 3),
		Entry("all lowercase oracle", "oracle linux 8 (64-bit)", 4),
	)
})

var _ = Describe("ScoreDiskTierLabel", func() {
	DescribeTable("inventory label to score mapping",
		func(label string, expectedScore int) {
			Expect(complexity.ScoreDiskTierLabel(label)).To(Equal(expectedScore))
		},
		Entry("score 1 label", "Easy (0-10TB)", 1),
		Entry("score 2 label", "Medium (10-20TB)", 2),
		Entry("score 3 label", "Hard (20-50TB)", 3),
		Entry("score 4 label", "White Glove (>50TB)", 4),
		Entry("unknown label returns 0", "Unknown Tier", 0),
		Entry("empty label returns 0", "", 0),
	)
})

var _ = Describe("OSBreakdown", func() {
	It("always returns exactly 5 entries in score order 0–4", func() {
		result := complexity.OSBreakdown([]complexity.VMOsEntry{})
		Expect(result).To(HaveLen(5))
		for i, entry := range result {
			Expect(entry.Score).To(Equal(i))
		}
	})

	It("returns all zero counts for empty input", func() {
		result := complexity.OSBreakdown([]complexity.VMOsEntry{})
		for _, entry := range result {
			Expect(entry.VMCount).To(Equal(0))
		}
	})

	It("accumulates VM counts per score correctly", func() {
		entries := []complexity.VMOsEntry{
			{Name: "Red Hat Enterprise Linux 8 (64-bit)", Count: 100},
			{Name: "Red Hat Enterprise Linux 9 (64-bit)", Count: 50},
			{Name: "CentOS 7 (64-bit)", Count: 30},
			{Name: "FreeBSD (64-bit)", Count: 10},
		}
		result := complexity.OSBreakdown(entries)

		Expect(result[0].Score).To(Equal(0))
		Expect(result[0].VMCount).To(Equal(10)) // FreeBSD (unclassified)
		Expect(result[1].Score).To(Equal(1))
		Expect(result[1].VMCount).To(Equal(150)) // 100 + 50 Red Hat
		Expect(result[2].Score).To(Equal(2))
		Expect(result[2].VMCount).To(Equal(30)) // CentOS
		Expect(result[3].Score).To(Equal(3))
		Expect(result[3].VMCount).To(Equal(0))
		Expect(result[4].Score).To(Equal(4))
		Expect(result[4].VMCount).To(Equal(0))
	})

	It("places unrecognised OS names into score 0", func() {
		entries := []complexity.VMOsEntry{
			{Name: "VMware Photon OS (64-bit)", Count: 5},
			{Name: "Debian GNU/Linux 12 (64-bit)", Count: 3},
		}
		result := complexity.OSBreakdown(entries)
		Expect(result[0].Score).To(Equal(0))
		Expect(result[0].VMCount).To(Equal(8))
	})

	It("scores covering all five tiers", func() {
		entries := []complexity.VMOsEntry{
			{Name: "Red Hat Enterprise Linux 9 (64-bit)", Count: 10},
			{Name: "Microsoft Windows Server 2022 (64-bit)", Count: 5},
			{Name: "Ubuntu Linux (64-bit)", Count: 3},
			{Name: "Oracle Linux 8 (64-bit)", Count: 2},
			{Name: "Other (64-bit)", Count: 1},
		}
		result := complexity.OSBreakdown(entries)
		Expect(result[0].VMCount).To(Equal(1))  // score 0: unclassified (Other)
		Expect(result[1].VMCount).To(Equal(10)) // score 1: Red Hat
		Expect(result[2].VMCount).To(Equal(5))  // score 2: Windows
		Expect(result[3].VMCount).To(Equal(3))  // score 3: Ubuntu
		Expect(result[4].VMCount).To(Equal(2))  // score 4: Oracle
	})
})

var _ = Describe("DiskSizeRangeRatings", func() {
	It("returns exactly 4 entries", func() {
		Expect(complexity.DiskSizeRangeRatings()).To(HaveLen(4))
	})

	It("keys contain only the numeric range, not the word prefix", func() {
		ratings := complexity.DiskSizeRangeRatings()
		Expect(ratings).To(HaveKey("0-10TB"))
		Expect(ratings).To(HaveKey("10-20TB"))
		Expect(ratings).To(HaveKey("20-50TB"))
		Expect(ratings).To(HaveKey(">50TB"))
	})

	It("no original word-prefixed keys are present", func() {
		ratings := complexity.DiskSizeRangeRatings()
		Expect(ratings).NotTo(HaveKey("Easy (0-10TB)"))
		Expect(ratings).NotTo(HaveKey("Medium (10-20TB)"))
		Expect(ratings).NotTo(HaveKey("Hard (20-50TB)"))
		Expect(ratings).NotTo(HaveKey("White Glove (>50TB)"))
	})

	It("scores are preserved correctly after key reformatting", func() {
		ratings := complexity.DiskSizeRangeRatings()
		Expect(ratings["0-10TB"]).To(Equal(1))
		Expect(ratings["10-20TB"]).To(Equal(2))
		Expect(ratings["20-50TB"]).To(Equal(3))
		Expect(ratings[">50TB"]).To(Equal(4))
	})

	It("returns a new map each call (does not mutate DiskSizeScores)", func() {
		r1 := complexity.DiskSizeRangeRatings()
		r2 := complexity.DiskSizeRangeRatings()
		Expect(r1).To(Equal(r2))
		// Mutating the returned map must not affect DiskSizeScores
		r1["0-10TB"] = 99
		Expect(complexity.DiskSizeScores["Easy (0-10TB)"]).To(Equal(1))
	})
})

var _ = Describe("OSRatings", func() {
	It("returns an empty map for empty input", func() {
		result := complexity.OSRatings([]complexity.VMOsEntry{})
		Expect(result).NotTo(BeNil())
		Expect(result).To(BeEmpty())
	})

	It("returns score 1 for a known easy OS", func() {
		result := complexity.OSRatings([]complexity.VMOsEntry{
			{Name: "Red Hat Enterprise Linux 8 (64-bit)", Count: 10},
		})
		Expect(result).To(HaveLen(1))
		Expect(result["Red Hat Enterprise Linux 8 (64-bit)"]).To(Equal(1))
	})

	It("returns score 0 for an unclassified OS", func() {
		result := complexity.OSRatings([]complexity.VMOsEntry{
			{Name: "FreeBSD (64-bit)", Count: 3},
		})
		Expect(result).To(HaveLen(1))
		Expect(result["FreeBSD (64-bit)"]).To(Equal(0))
	})

	It("returns correct scores for multiple distinct OS names", func() {
		entries := []complexity.VMOsEntry{
			{Name: "Red Hat Enterprise Linux 9 (64-bit)", Count: 100},
			{Name: "CentOS 7 (64-bit)", Count: 20},
			{Name: "Ubuntu Linux (64-bit)", Count: 5},
			{Name: "Oracle Linux 8 (64-bit)", Count: 2},
			{Name: "FreeBSD (64-bit)", Count: 1},
		}
		result := complexity.OSRatings(entries)
		Expect(result).To(HaveLen(5))
		Expect(result["Red Hat Enterprise Linux 9 (64-bit)"]).To(Equal(1))
		Expect(result["CentOS 7 (64-bit)"]).To(Equal(2))
		Expect(result["Ubuntu Linux (64-bit)"]).To(Equal(3))
		Expect(result["Oracle Linux 8 (64-bit)"]).To(Equal(4))
		Expect(result["FreeBSD (64-bit)"]).To(Equal(0))
	})

	It("produces one map entry per distinct OS name regardless of VM count", func() {
		entries := []complexity.VMOsEntry{
			{Name: "Red Hat Enterprise Linux 8 (64-bit)", Count: 500},
			{Name: "Red Hat Enterprise Linux 9 (64-bit)", Count: 300},
		}
		result := complexity.OSRatings(entries)
		Expect(result).To(HaveLen(2))
		Expect(result["Red Hat Enterprise Linux 8 (64-bit)"]).To(Equal(1))
		Expect(result["Red Hat Enterprise Linux 9 (64-bit)"]).To(Equal(1))
	})
})

// Real-world VMware OS strings collected from an actual rvtools output
var _ = Describe("ClassifyOS real-world inventory strings", func() {
	DescribeTable("VMware OS name → score",
		func(osName string, expectedScore int) {
			Expect(complexity.ClassifyOS(osName)).To(Equal(expectedScore))
		},
		// CentOS → keyword "CentOS" → score 2
		Entry("CentOS 4/5 32-bit", "CentOS 4/5 (32-bit)", 2),
		Entry("CentOS 4/5/6 64-bit", "CentOS 4/5/6 (64-bit)", 2),
		Entry("CentOS 6 64-bit", "CentOS 6 (64-bit)", 2),
		Entry("CentOS 7 64-bit", "CentOS 7 (64-bit)", 2),
		// Debian — no matching keyword → score 0
		Entry("Debian GNU/Linux 11 64-bit", "Debian GNU/Linux 11 (64-bit)", 0),
		// FreeBSD — no matching keyword → score 0
		Entry("FreeBSD 64-bit", "FreeBSD (64-bit)", 0),
		// Appgate / custom Linux — no matching keyword → score 0
		Entry("Appgate SDP Linux", "Linux 6.5.0-45-generic Appgate SDP 1.0 Appgate SDP 1.0", 0),
		// Windows — keyword "Windows" → score 2
		Entry("Windows 10 64-bit", "Microsoft Windows 10 (64-bit)", 2),
		Entry("Windows Server 2008 64-bit", "Microsoft Windows Server 2008 (64-bit)", 2),
		Entry("Windows Server 2008 R2 64-bit", "Microsoft Windows Server 2008 R2 (64-bit)", 2),
		Entry("Windows Server 2012 64-bit", "Microsoft Windows Server 2012 (64-bit)", 2),
		Entry("Windows Server 2016 64-bit", "Microsoft Windows Server 2016 (64-bit)", 2),
		Entry("Windows Server 2016 or later 64-bit", "Microsoft Windows Server 2016 or later (64-bit)", 2),
		Entry("Windows Server 2019 64-bit", "Microsoft Windows Server 2019 (64-bit)", 2),
		Entry("Windows Server 2022 64-bit", "Microsoft Windows Server 2022 (64-bit)", 2),
		// Other / generic — no matching keyword → score 0
		Entry("Other 32-bit", "Other (32-bit)", 0),
		Entry("Other 64-bit", "Other (64-bit)", 0),
		Entry("Other 2.6.x Linux 64-bit", "Other 2.6.x Linux (64-bit)", 0),
		Entry("Other 3.x Linux 64-bit", "Other 3.x Linux (64-bit)", 0),
		Entry("Other 3.x or later Linux 64-bit", "Other 3.x or later Linux (64-bit)", 0),
		Entry("Other 4.x or later Linux 64-bit", "Other 4.x or later Linux (64-bit)", 0),
		Entry("Other 5.x or later Linux 64-bit", "Other 5.x or later Linux (64-bit)", 0),
		Entry("Other Linux 64-bit", "Other Linux (64-bit)", 0),
		// Red Hat — keyword "Red Hat" → score 1
		Entry("RHEL 5 64-bit", "Red Hat Enterprise Linux 5 (64-bit)", 1),
		Entry("RHEL 6 64-bit", "Red Hat Enterprise Linux 6 (64-bit)", 1),
		Entry("RHEL 7 64-bit", "Red Hat Enterprise Linux 7 (64-bit)", 1),
		Entry("RHEL 8 64-bit", "Red Hat Enterprise Linux 8 (64-bit)", 1),
		Entry("RHEL 9 64-bit", "Red Hat Enterprise Linux 9 (64-bit)", 1),
		Entry("Red Hat Fedora 64-bit", "Red Hat Fedora (64-bit)", 1),
		// SUSE Linux Enterprise — keyword "SUSE Linux Enterprise" → score 3
		Entry("SUSE Linux Enterprise 11 64-bit", "SUSE Linux Enterprise 11 (64-bit)", 3),
		Entry("SUSE Linux Enterprise 12 64-bit", "SUSE Linux Enterprise 12 (64-bit)", 3),
		// Ubuntu — keyword "Ubuntu" → score 3
		Entry("Ubuntu Linux 64-bit", "Ubuntu Linux (64-bit)", 3),
		// VMware Photon OS — no matching keyword → score 0
		Entry("VMware Photon OS 64-bit", "VMware Photon OS (64-bit)", 0),
	)
})

var _ = Describe("DiskBreakdown", func() {
	It("always returns exactly 4 entries in score order 1–4", func() {
		result := complexity.DiskBreakdown([]complexity.DiskTierInput{})
		Expect(result).To(HaveLen(4))
		for i, entry := range result {
			Expect(entry.Score).To(Equal(i + 1))
		}
	})

	It("returns all zero values for empty input", func() {
		result := complexity.DiskBreakdown([]complexity.DiskTierInput{})
		for _, entry := range result {
			Expect(entry.VMCount).To(Equal(0))
			Expect(entry.TotalSizeTB).To(Equal(0.0))
		}
	})

	It("passes through VMCount and TotalSizeTB for known tier labels", func() {
		tiers := []complexity.DiskTierInput{
			{Label: "Easy (0-10TB)", VMCount: 360, TotalSizeTB: 42.48},
			{Label: "Hard (20-50TB)", VMCount: 5, TotalSizeTB: 25.1},
		}
		result := complexity.DiskBreakdown(tiers)

		Expect(result[0].Score).To(Equal(1))
		Expect(result[0].VMCount).To(Equal(360))
		Expect(result[0].TotalSizeTB).To(Equal(42.48))

		Expect(result[1].Score).To(Equal(2))
		Expect(result[1].VMCount).To(Equal(0))

		Expect(result[2].Score).To(Equal(3))
		Expect(result[2].VMCount).To(Equal(5))
		Expect(result[2].TotalSizeTB).To(Equal(25.1))

		Expect(result[3].Score).To(Equal(4))
		Expect(result[3].VMCount).To(Equal(0))
	})

	It("ignores unknown tier labels", func() {
		tiers := []complexity.DiskTierInput{
			{Label: "Easy (0-10TB)", VMCount: 10, TotalSizeTB: 5.0},
			{Label: "UnknownTier", VMCount: 99, TotalSizeTB: 999.0},
		}
		result := complexity.DiskBreakdown(tiers)

		// Only score-1 entry should be populated; unknown label must not appear
		Expect(result[0].VMCount).To(Equal(10))
		total := 0
		for _, e := range result {
			total += e.VMCount
		}
		Expect(total).To(Equal(10)) // the 99 from UnknownTier is discarded
	})

	It("handles all four tiers populated", func() {
		tiers := []complexity.DiskTierInput{
			{Label: "Easy (0-10TB)", VMCount: 100, TotalSizeTB: 5.0},
			{Label: "Medium (10-20TB)", VMCount: 20, TotalSizeTB: 15.0},
			{Label: "Hard (20-50TB)", VMCount: 5, TotalSizeTB: 30.0},
			{Label: "White Glove (>50TB)", VMCount: 1, TotalSizeTB: 75.0},
		}
		result := complexity.DiskBreakdown(tiers)
		Expect(result[0].VMCount).To(Equal(100))
		Expect(result[1].VMCount).To(Equal(20))
		Expect(result[2].VMCount).To(Equal(5))
		Expect(result[3].VMCount).To(Equal(1))
	})
})
