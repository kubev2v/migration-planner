package eventwrap_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestEventwrap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Eventwrap Suite")
}
