package v1alpha1

import (
	"errors"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubev2v/migration-planner/internal/handlers/validator"
)

var _ = Describe("RVTools signature validation", func() {
	DescribeTable("distinguishes invalid input from file-system failures", func(problem string, invalidInput bool) {
		path := filepath.Join(GinkgoT().TempDir(), "upload.xlsx")
		switch problem {
		case "directory":
			Expect(os.Mkdir(path, 0700)).To(Succeed())
		case "short file":
			Expect(os.WriteFile(path, []byte("PK"), 0600)).To(Succeed())
		case "invalid signature":
			Expect(os.WriteFile(path, []byte("JSON"), 0600)).To(Succeed())
		}
		err := validateXLSXFile(path)
		Expect(err).To(HaveOccurred())
		var invalid *validator.ErrInvalidFile
		Expect(errors.As(err, &invalid)).To(Equal(invalidInput))
		if invalidInput {
			Expect(invalid.Error()).To(Equal("file is not a valid Excel file"))
		} else {
			var filesystem *os.PathError
			Expect(errors.As(err, &filesystem)).To(BeTrue())
		}
	},
		Entry("missing file is internal", "missing", false),
		Entry("failed signature read is internal", "directory", false),
		Entry("short file is invalid input", "short file", true),
		Entry("invalid signature is invalid input", "invalid signature", true),
	)
})
