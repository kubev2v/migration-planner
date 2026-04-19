package image_test

import (
	"os"
	"time"

	"github.com/kubev2v/migration-planner/internal/image"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ImageCleaner", func() {

	Describe("Register and cleanup", func() {
		It("removes expired files and deletes them from disk", func() {
			f, err := os.CreateTemp("", "image-cleaner-*")
			Expect(err).NotTo(HaveOccurred())
			path := f.Name()
			Expect(f.Close()).To(Succeed())
			defer func() { _ = os.Remove(path) }()

			c := image.NewImageCleaner(3 * time.Second)
			c.Register(path, -time.Second)

			_, err = os.Stat(path)
			Expect(err).NotTo(HaveOccurred())

			c.Start()
			defer c.Stop()

			Eventually(func() bool {
				_, err := os.Stat(path)
				return os.IsNotExist(err)
			}, "1m", "1s").Should(BeTrue())
		})

		It("keeps files that are not yet expired", func() {
			f, err := os.CreateTemp("", "image-cleaner-*")
			Expect(err).NotTo(HaveOccurred())
			path := f.Name()
			Expect(f.Close()).To(Succeed())
			defer func() { _ = os.Remove(path) }()

			c := image.NewImageCleaner(time.Millisecond)
			c.Register(path, time.Hour)

			c.Start()
			defer c.Stop()

			Consistently(func() bool {
				_, err := os.Stat(path)
				return err == nil
			}, "200ms", "20ms").Should(BeTrue())
		})
	})

	Describe("Start and Stop", func() {
		It("allows starting and stopping the background loop", func() {
			c := image.NewImageCleaner(time.Hour)
			c.Start()
			c.Stop()
		})
	})
})
