package mount_test

import (
	"context"
	"time"

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
	})

	Context("simple invocation", func() {
		It("generates normal dag", func() {
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
	Expect(len(dag)).To(Equal(2), actualDag)
	Expect(len(dag[0])).To(Equal(1), actualDag)
	Expect(len(dag[1])).To(Equal(1), actualDag)

	Expect(dag[0][0].Name).To(Equal("init"))
	Expect(dag[1][0].Name).To(Equal("create-sentinel"))

}
func checkDag(dag [][]herd.GraphEntry, actualDag string) {
	Expect(len(dag)).To(Equal(10), actualDag)
	Expect(len(dag[0])).To(Equal(1), actualDag)
	Expect(len(dag[1])).To(Equal(3), actualDag)
	Expect(len(dag[2])).To(Equal(1), actualDag)
	Expect(len(dag[3])).To(Equal(1), actualDag)
	Expect(len(dag[4])).To(Equal(1), actualDag)
	Expect(len(dag[5])).To(Equal(1), actualDag)
	Expect(len(dag[6])).To(Equal(1), actualDag)
	Expect(len(dag[7])).To(Equal(2), actualDag)
	Expect(len(dag[8])).To(Equal(2), actualDag)
	Expect(len(dag[9])).To(Equal(1), actualDag)

	Expect(dag[0][0].Name).To(Equal("init"))
	Expect(dag[1][0].Name).To(Or(
		Equal("mount-tmpfs"),
		Equal("create-sentinel"),
		Equal("mount-state"),
	), actualDag)
	Expect(dag[1][1].Name).To(Or(
		Equal("mount-tmpfs"),
		Equal("create-sentinel"),
		Equal("mount-state"),
	), actualDag)
	Expect(dag[1][2].Name).To(Or(
		Equal("mount-tmpfs"),
		Equal("create-sentinel"),
		Equal("mount-state"),
	), actualDag)
	Expect(dag[2][0].Name).To(Equal("discover-state"), actualDag)
	Expect(dag[3][0].Name).To(Equal("mount-root"), actualDag)
	Expect(dag[4][0].Name).To(Equal("mount-oem"), actualDag)
	Expect(dag[5][0].Name).To(Equal("rootfs-hook"), actualDag)
	Expect(dag[6][0].Name).To(Equal("load-config"), actualDag)
	Expect(dag[7][0].Name).To(Or(Equal("mount-base-overlay"), Equal("custom-mount")), actualDag)
	Expect(dag[7][1].Name).To(Or(Equal("mount-base-overlay"), Equal("custom-mount")), actualDag)
	Expect(dag[8][0].Name).To(Or(Equal("mount-bind"), Equal("overlay-mount")), actualDag)
	Expect(dag[8][1].Name).To(Or(Equal("mount-bind"), Equal("overlay-mount")), actualDag)
	Expect(dag[9][0].Name).To(Equal("write-fstab"), actualDag)
}
