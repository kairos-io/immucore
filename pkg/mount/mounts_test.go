package mount

import (
	"github.com/kairos-io/immucore/internal/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("mount utils", func() {
	BeforeEach(func() {
	})

	Context("ReadCMDLineArg", func() {
		It("splits arguments from cmdline", func() {
			Skip("No way of overriding the cmdline yet")
			Expect(len(utils.ReadCMDLineArg("testvalue/key="))).To(Equal(1))
		})
		It("returns properly for stanzas without value", func() {
			Skip("No way of overriding the cmdline yet")
			Expect(len(utils.ReadCMDLineArg("singlevalue"))).To(Equal(1))
		})
	})
})
