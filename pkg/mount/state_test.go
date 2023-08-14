package mount_test

import (
	"context"
	"github.com/kairos-io/immucore/internal/utils"
	"time"

	cnst "github.com/kairos-io/immucore/internal/constants"
	"github.com/kairos-io/immucore/pkg/mount"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spectrocloud-labs/herd"
)

var _ = Describe("mounting immutable setup", func() {
	var g *herd.Graph

	BeforeEach(func() {
		g = herd.DAG(herd.EnableInit)
		Expect(g).ToNot(BeNil())
		cnst.LogDir = "/tmp/immucore"
		Expect(utils.SetLogger()).ToNot(HaveOccurred())
	})

	Context("SortedBindMounts()", func() {
		It("returns the nodes with less depth first and in alfabetical order", func() {
			s := &mount.State{
				BindMounts: []string{
					"/etc/nginx/config.d/",
					"/etc/nginx",
					"/etc/kubernetes/child",
					"/etc/kubernetes",
					"/etc/kubernetes/child/grand-child",
				},
			}
			Expect(s.SortedBindMounts()).To(Equal([]string{
				"/etc/kubernetes",
				"/etc/nginx",
				"/etc/kubernetes/child",
				"/etc/nginx/config.d/",
				"/etc/kubernetes/child/grand-child",
			}))
		})
	})

	Context("simple invocation", func() {
		It("generates normal dag", func() {
			Skip("Cant override bootstate yet")
			s := &mount.State{
				Rootdir:      "/",
				TargetImage:  "/cOS/myimage.img",
				TargetDevice: "/dev/disk/by-label/COS_LABEL",
			}

			err := s.RegisterNormalBoot(g)
			Expect(err).ToNot(HaveOccurred())

			dag := g.Analyze()

			checkDag(dag, s.WriteDAG(g))

		})
		It("generates normal dag with extra dirs", func() {
			Skip("Cant override bootstate yet")
			s := &mount.State{Rootdir: "/",
				OverlayDirs:  []string{"/etc"},
				BindMounts:   []string{"/etc/kubernetes"},
				CustomMounts: map[string]string{"COS_PERSISTENT": "/usr/local"}}

			err := s.RegisterNormalBoot(g)
			Expect(err).ToNot(HaveOccurred())

			dag := g.Analyze()

			checkDag(dag, s.WriteDAG(g))
		})
		It("generates livecd dag", func() {
			s := &mount.State{}
			err := s.RegisterLiveMedia(g)
			Expect(err).ToNot(HaveOccurred())
			dag := g.Analyze()
			checkLiveCDDag(dag, s.WriteDAG(g))

		})

		It("Mountop timeouts", func() {
			s := &mount.State{}
			f := s.MountOP("/dev/doesntexist", "/tmp/jojobizarreadventure", "", []string{}, 500*time.Millisecond)
			err := f(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exhausted"))
		})
	})
})

func checkLiveCDDag(dag [][]herd.GraphEntry, actualDag string) {
	Expect(len(dag)).To(Equal(5), actualDag)
	Expect(len(dag[0])).To(Equal(1), actualDag)
	Expect(len(dag[1])).To(Equal(2), actualDag)
	Expect(len(dag[2])).To(Equal(1), actualDag)
	Expect(len(dag[3])).To(Equal(1), actualDag)
	Expect(len(dag[4])).To(Equal(1), actualDag)

	Expect(dag[0][0].Name).To(Equal("init"))
	Expect(dag[1][0].Name).To(Or(Equal(cnst.OpSentinel), Equal(cnst.OpWaitForSysroot)), actualDag)
	Expect(dag[1][1].Name).To(Or(Equal(cnst.OpSentinel), Equal(cnst.OpWaitForSysroot)), actualDag)
	Expect(dag[2][0].Name).To(Equal(cnst.OpMountOEM), actualDag)
	Expect(dag[3][0].Name).To(Equal(cnst.OpRootfsHook), actualDag)
	Expect(dag[4][0].Name).To(Equal(cnst.OpInitramfsHook), actualDag)

}
func checkDag(dag [][]herd.GraphEntry, actualDag string) {
	Expect(len(dag)).To(Equal(12), actualDag)
	Expect(len(dag[0])).To(Equal(1), actualDag)
	Expect(len(dag[1])).To(Equal(4), actualDag)
	Expect(len(dag[2])).To(Equal(1), actualDag)
	Expect(len(dag[3])).To(Equal(1), actualDag)
	Expect(len(dag[4])).To(Equal(1), actualDag)
	Expect(len(dag[5])).To(Equal(1), actualDag)
	Expect(len(dag[6])).To(Equal(1), actualDag)
	Expect(len(dag[7])).To(Equal(2), actualDag)
	Expect(len(dag[8])).To(Equal(1), actualDag)
	Expect(len(dag[9])).To(Equal(1), actualDag)
	Expect(len(dag[10])).To(Equal(1), actualDag)
	Expect(len(dag[11])).To(Equal(1), actualDag)

	Expect(dag[0][0].Name).To(Equal("init"))
	Expect(dag[1][0].Name).To(Or(
		Equal(cnst.OpMountTmpfs),
		Equal(cnst.OpSentinel),
		Equal(cnst.OpMountState),
		Equal(cnst.OpLvmActivate),
	), actualDag)
	Expect(dag[1][1].Name).To(Or(
		Equal(cnst.OpMountTmpfs),
		Equal(cnst.OpSentinel),
		Equal(cnst.OpMountState),
		Equal(cnst.OpLvmActivate),
	), actualDag)
	Expect(dag[1][2].Name).To(Or(
		Equal(cnst.OpMountTmpfs),
		Equal(cnst.OpSentinel),
		Equal(cnst.OpMountState),
		Equal(cnst.OpLvmActivate),
	), actualDag)
	Expect(dag[1][3].Name).To(Or(
		Equal(cnst.OpMountTmpfs),
		Equal(cnst.OpSentinel),
		Equal(cnst.OpMountState),
		Equal(cnst.OpLvmActivate),
	), actualDag)
	Expect(dag[2][0].Name).To(Equal(cnst.OpDiscoverState), actualDag)
	Expect(dag[3][0].Name).To(Equal(cnst.OpMountRoot), actualDag)
	Expect(dag[4][0].Name).To(Equal(cnst.OpMountOEM), actualDag)
	Expect(dag[5][0].Name).To(Equal(cnst.OpRootfsHook), actualDag)
	Expect(dag[6][0].Name).To(Equal(cnst.OpLoadConfig), actualDag)
	Expect(dag[7][0].Name).To(Or(Equal(cnst.OpMountBaseOverlay), Equal(cnst.OpCustomMounts)), actualDag)
	Expect(dag[7][1].Name).To(Or(Equal(cnst.OpMountBaseOverlay), Equal(cnst.OpCustomMounts)), actualDag)
	Expect(dag[8][0].Name).To(Equal(cnst.OpOverlayMount), actualDag)
	Expect(dag[9][0].Name).To(Equal(cnst.OpMountBind), actualDag)
	Expect(dag[10][0].Name).To(Equal(cnst.OpWriteFstab), actualDag)
	Expect(dag[11][0].Name).To(Equal(cnst.OpInitramfsHook), actualDag)
}
