package rvtools_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRvtools(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Rvtools Suite")
}
