package iso_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/kubev2v/migration-planner/pkg/iso"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("iso initializer", func() {
	var (
		tempDir         string
		targetIsoFile   string
		targetIsoSha256 string
		testData        []byte
		initializer     *iso.IsoInitializer
		testDownloader  *testIsoDownloader
	)

	BeforeEach(func() {
		var err error
		tempDir, err = os.MkdirTemp("", "iso-initializer-test")
		Expect(err).To(BeNil())

		targetIsoFile = filepath.Join(tempDir, "test.iso")
		testData = []byte("test iso content for validation")

		hasher := sha256.New()
		hasher.Write(testData)
		targetIsoSha256 = fmt.Sprintf("%x", hasher.Sum(nil))

		testDownloader = &testIsoDownloader{
			dataToWrite: testData,
		}
		initializer = iso.NewIsoInitializer(testDownloader)
	})

	AfterEach(func() {
		os.RemoveAll(tempDir)
	})

	Context("when target file does not exist", func() {
		It("should download the file", func() {
			err := initializer.Initialize(context.TODO(), targetIsoFile, targetIsoSha256)
			Expect(err).To(BeNil())
			Expect(testDownloader.hasBeenCalled).To(BeTrue())

			// Verify file was created with correct content
			data, err := os.ReadFile(targetIsoFile)
			Expect(err).To(BeNil())
			Expect(data).To(Equal(testData))
		})
	})

	Context("when target file exists with correct SHA256", func() {
		BeforeEach(func() {
			err := os.WriteFile(targetIsoFile, testData, 0644)
			Expect(err).To(BeNil())
		})

		It("should not download anything and preserve the original file", func() {
			err := initializer.Initialize(context.TODO(), targetIsoFile, targetIsoSha256)
			Expect(err).To(BeNil())
			Expect(testDownloader.hasBeenCalled).To(BeFalse())

			// Verify the original file still exists and contains the correct data
			data, err := os.ReadFile(targetIsoFile)
			Expect(err).To(BeNil())
			Expect(data).To(Equal(testData))

			// Verify file hasn't been modified
			fileInfo, err := os.Stat(targetIsoFile)
			Expect(err).To(BeNil())
			Expect(fileInfo.Size()).To(Equal(int64(len(testData))))
		})
	})

	Context("when target file exists with incorrect SHA256", func() {
		BeforeEach(func() {
			wrongData := []byte("wrong content")
			err := os.WriteFile(targetIsoFile, wrongData, 0644)
			Expect(err).To(BeNil())
		})

		It("should download and replace the file", func() {
			err := initializer.Initialize(context.TODO(), targetIsoFile, targetIsoSha256)
			Expect(err).To(BeNil())
			Expect(testDownloader.hasBeenCalled).To(BeTrue())

			// Verify file was replaced with correct content
			data, err := os.ReadFile(targetIsoFile)
			Expect(err).To(BeNil())
			Expect(data).To(Equal(testData))

			// Verify temporary file is cleaned up - directory should contain only the target file
			entries, err := os.ReadDir(tempDir)
			Expect(err).To(BeNil())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Name()).To(Equal("test.iso"))
			Expect(entries[0].IsDir()).To(BeFalse())
		})
	})

	Context("when download fails", func() {
		var originalData []byte

		BeforeEach(func() {
			// Create an existing file with wrong content to trigger download
			originalData = []byte("original wrong content")
			err := os.WriteFile(targetIsoFile, originalData, 0644)
			Expect(err).To(BeNil())

			testDownloader.shouldReturnError = true
		})

		It("should return the download error and preserve the original file", func() {
			err := initializer.Initialize(context.TODO(), targetIsoFile, targetIsoSha256)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to write the image to the temporary iso file"))
			Expect(testDownloader.hasBeenCalled).To(BeTrue())

			// Verify the original file still exists and contains the original data
			data, err := os.ReadFile(targetIsoFile)
			Expect(err).To(BeNil())
			Expect(data).To(Equal(originalData))

			// Verify file hasn't been corrupted
			fileInfo, err := os.Stat(targetIsoFile)
			Expect(err).To(BeNil())
			Expect(fileInfo.Size()).To(Equal(int64(len(originalData))))
		})
	})

	Context("when context is cancelled", func() {
		var originalData []byte

		BeforeEach(func() {
			// Create an existing file with wrong content to trigger download
			originalData = []byte("original content before cancellation")
			err := os.WriteFile(targetIsoFile, originalData, 0644)
			Expect(err).To(BeNil())
		})

		It("should handle cancellation gracefully and preserve the original file", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel() // Cancel immediately

			testDownloader.shouldRespectContext = true
			err := initializer.Initialize(ctx, targetIsoFile, targetIsoSha256)
			Expect(err).To(HaveOccurred())

			// Verify the original file still exists and contains the original data
			data, err := os.ReadFile(targetIsoFile)
			Expect(err).To(BeNil())
			Expect(data).To(Equal(originalData))

			// Verify file hasn't been corrupted
			fileInfo, err := os.Stat(targetIsoFile)
			Expect(err).To(BeNil())
			Expect(fileInfo.Size()).To(Equal(int64(len(originalData))))

			// Verify no temporary files are left behind
			entries, err := os.ReadDir(tempDir)
			Expect(err).To(BeNil())
			Expect(entries).To(HaveLen(1))
			Expect(entries[0].Name()).To(Equal("test.iso"))
		})
	})

})

// testIsoDownloader is a simple implementation of IsoDownloader for testing
type testIsoDownloader struct {
	dataToWrite          []byte
	shouldReturnError    bool
	shouldRespectContext bool
	hasBeenCalled        bool
	calledTime           time.Time
}

func (t *testIsoDownloader) Download(ctx context.Context, dst io.WriteSeeker) error {
	t.hasBeenCalled = true
	t.calledTime = time.Now()

	if t.shouldRespectContext {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	if t.shouldReturnError {
		return fmt.Errorf("test download error")
	}

	_, err := dst.Write(t.dataToWrite)
	return err
}
