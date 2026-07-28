package utils_test

import (
	"os"
	"strings"

	"github.com/kairos-io/immucore/internal/constants"
	"github.com/kairos-io/immucore/internal/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/twpayne/go-vfs/v4"
	"github.com/twpayne/go-vfs/v4/vfst"
)

var _ = Describe("ensure-partitions helpers", func() {
	var fs vfs.FS
	var cleanup func()

	BeforeEach(func() {
		fs, cleanup, _ = vfst.NewTestFS(map[string]interface{}{
			"/proc/cmdline": "",
		})
		fakeCmdline, _ := fs.RawPath("/proc/cmdline")
		Expect(os.Setenv("HOST_PROC_CMDLINE", fakeCmdline)).To(Succeed())
	})
	AfterEach(func() {
		cleanup()
		_ = os.Unsetenv("HOST_PROC_CMDLINE")
	})

	Context("ParseAutoCreateDisk", func() {
		It("Reports not-set when flag absent", func() {
			explicit, set := utils.ParseAutoCreateDisk()
			Expect(set).To(BeFalse())
			Expect(explicit).To(BeEmpty())
		})
		It("Detects bare flag as set with no explicit disk", func() {
			Expect(fs.WriteFile("/proc/cmdline", []byte("kairos.ram.auto_create_partitions\n"), 0o600)).To(Succeed())
			explicit, set := utils.ParseAutoCreateDisk()
			Expect(set).To(BeTrue())
			Expect(explicit).To(BeEmpty())
		})
		It("Detects explicit disk path", func() {
			Expect(fs.WriteFile("/proc/cmdline", []byte("kairos.ram.auto_create_partitions=/dev/vda\n"), 0o600)).To(Succeed())
			explicit, set := utils.ParseAutoCreateDisk()
			Expect(set).To(BeTrue())
			Expect(explicit).To(Equal("/dev/vda"))
		})
		It("Treats empty value as bare (set, no explicit)", func() {
			Expect(fs.WriteFile("/proc/cmdline", []byte("kairos.ram.auto_create_partitions=\n"), 0o600)).To(Succeed())
			explicit, set := utils.ParseAutoCreateDisk()
			Expect(set).To(BeTrue())
			Expect(explicit).To(BeEmpty())
		})
	})

	Context("AutoCreateWipeEnabled", func() {
		It("Defaults to false", func() {
			Expect(utils.AutoCreateWipeEnabled()).To(BeFalse())
		})
		It("Trips when the wipe flag is present", func() {
			Expect(fs.WriteFile("/proc/cmdline", []byte("kairos.ram.auto_create_partitions kairos.ram.wipe\n"), 0o600)).To(Succeed())
			Expect(utils.AutoCreateWipeEnabled()).To(BeTrue())
		})
	})

	Context("ParseAutoCreateSize", func() {
		It("Returns fallback when flag absent", func() {
			Expect(utils.ParseAutoCreateSize(constants.CmdlineAutoCreateOemSize, 64)).To(Equal(uint64(64)))
		})
		It("Parses a plain MiB integer", func() {
			Expect(fs.WriteFile("/proc/cmdline", []byte("kairos.ram.oem=128\n"), 0o600)).To(Succeed())
			Expect(utils.ParseAutoCreateSize(constants.CmdlineAutoCreateOemSize, 64)).To(Equal(uint64(128)))
		})
		It("Falls back on garbage input", func() {
			Expect(fs.WriteFile("/proc/cmdline", []byte("kairos.ram.oem=notanumber\n"), 0o600)).To(Succeed())
			Expect(utils.ParseAutoCreateSize(constants.CmdlineAutoCreateOemSize, 64)).To(Equal(uint64(64)))
		})
		It("Accepts 0 (means expand-to-end for persistent)", func() {
			Expect(fs.WriteFile("/proc/cmdline", []byte("kairos.ram.persistent=0\n"), 0o600)).To(Succeed())
			Expect(utils.ParseAutoCreateSize(constants.CmdlineAutoCreatePersistentSize, 42)).To(Equal(uint64(0)))
		})
	})

	Context("BuildEnsurePartitionsStage", func() {
		It("Emits both partitions with init_disk true when the disk is empty", func() {
			y := utils.BuildEnsurePartitionsStage("/dev/vda", false, false, 64, 0, true)
			Expect(y).To(ContainSubstring("path: /dev/vda"))
			Expect(y).To(ContainSubstring("init_disk: true"))
			Expect(y).To(ContainSubstring("fsLabel: COS_OEM"))
			Expect(y).To(ContainSubstring("fsLabel: COS_PERSISTENT"))
			// persistent gets size 0 => yip's "fill to end" semantics
			Expect(y).To(ContainSubstring("size: 0"))
			Expect(y).To(ContainSubstring("size: 64"))
		})
		It("Only emits persistent when OEM is already present", func() {
			y := utils.BuildEnsurePartitionsStage("/dev/vda", true, false, 64, 0, false)
			Expect(y).To(ContainSubstring("init_disk: false"))
			Expect(y).ToNot(ContainSubstring("fsLabel: COS_OEM"))
			Expect(y).To(ContainSubstring("fsLabel: COS_PERSISTENT"))
		})
		It("Only emits OEM when persistent is already present", func() {
			y := utils.BuildEnsurePartitionsStage("/dev/vdb", false, true, 128, 0, false)
			Expect(y).To(ContainSubstring("init_disk: false"))
			Expect(y).To(ContainSubstring("fsLabel: COS_OEM"))
			Expect(y).To(ContainSubstring("size: 128"))
			Expect(y).ToNot(ContainSubstring("fsLabel: COS_PERSISTENT"))
		})
	})

	Context("Rendered operator messages", func() {
		It("Missing-partitions message lists exactly what is missing and the flag syntax", func() {
			out := utils.RenderMissingPartitionsMessage(false, false, []string{"/dev/vda", "/dev/vdb"})
			Expect(out).To(ContainSubstring("COS_OEM (missing)"))
			Expect(out).To(ContainSubstring("COS_PERSISTENT (missing)"))
			Expect(out).To(ContainSubstring("kairos.ram.auto_create_partitions"))
			Expect(out).To(ContainSubstring("/dev/vda"))
			Expect(out).To(ContainSubstring("/dev/vdb"))
			Expect(out).To(ContainSubstring("kairos.ram.wipe"))
		})
		It("Missing-partitions message reflects partial state", func() {
			out := utils.RenderMissingPartitionsMessage(true, false, nil)
			Expect(out).To(ContainSubstring("COS_PERSISTENT (missing)"))
			Expect(out).ToNot(ContainSubstring("COS_OEM (missing)"))
			Expect(out).To(ContainSubstring("(none"))
		})
		It("Ambiguous-disk message names every candidate", func() {
			out := utils.RenderAmbiguousDiskMessage([]string{"/dev/vda", "/dev/vdb", "/dev/nvme0n1"})
			Expect(strings.Count(out, "kairos.ram.auto_create_partitions=")).To(Equal(3))
			Expect(out).To(ContainSubstring("/dev/nvme0n1"))
		})
		It("Wipe-required message names the offending disk and the flag", func() {
			out := utils.RenderWipeRequiredMessage("/dev/vda")
			Expect(out).To(ContainSubstring("/dev/vda"))
			Expect(out).To(ContainSubstring("kairos.ram.wipe"))
			Expect(out).To(ContainSubstring("DESTROYS"))
		})
	})
})
