package mount_test

import (
	"context"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
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
				Logger:      log.Output(zerolog.ConsoleWriter{Out: os.Stderr}).With().Logger(),
				Rootdir:     "/",
				TargetImage: "/cOS/myimage.img",
				TargetLabel: "COS_LABEL",
				MountRoot:   true,
			}

			err := s.RegisterNormalBoot(g)
			Expect(err).ToNot(HaveOccurred())

			dag := g.Analyze()

			checkDag(dag, s.WriteDAG(g))

		})
		It("generates normal dag with extra dirs", func() {
			s := &mount.State{Rootdir: "/", MountRoot: true,
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
	Expect(len(dag[1])).To(Equal(2), actualDag)

	Expect(dag[0][0].Name).To(Equal("init"))
	Expect(dag[1][0].Name).To(Or(
		Equal("mount-tmpfs"),
		Equal("create-sentinel"),
	), actualDag)
	Expect(dag[1][1].Name).To(Or(
		Equal("mount-tmpfs"),
		Equal("create-sentinel"),
	), actualDag)

}
func checkDag(dag [][]herd.GraphEntry, actualDag string) {
	Expect(len(dag)).To(Equal(10), actualDag)
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

	Expect(dag[0][0].Name).To(Equal("init"))
	Expect(dag[1][0].Name).To(Or(
		Equal("mount-tmpfs"),
		Equal("create-sentinel"),
		Equal("mount-base-overlay"),
		Equal("mount-state"),
	), actualDag)
	Expect(dag[1][1].Name).To(Or(
		Equal("mount-tmpfs"),
		Equal("create-sentinel"),
		Equal("mount-base-overlay"),
		Equal("mount-state"),
	), actualDag)
	Expect(dag[1][2].Name).To(Or(
		Equal("mount-tmpfs"),
		Equal("create-sentinel"),
		Equal("mount-base-overlay"),
		Equal("mount-state"),
	), actualDag)
	Expect(dag[1][3].Name).To(Or(
		Equal("mount-tmpfs"),
		Equal("create-sentinel"),
		Equal("mount-base-overlay"),
		Equal("mount-state"),
	), actualDag)
	Expect(dag[2][0].Name).To(Equal("discover-state"), actualDag)
	Expect(dag[3][0].Name).To(Equal("mount-root"), actualDag)
	Expect(dag[4][0].Name).To(Equal("mount-oem"), actualDag)
	Expect(dag[5][0].Name).To(Equal("rootfs-hook"), actualDag)
	Expect(dag[6][0].Name).To(Equal("load-config"), actualDag)
	Expect(dag[7][0].Name).To(Or(Equal("overlay-mount"), Equal("custom-mount")), actualDag)
	Expect(dag[7][1].Name).To(Or(Equal("overlay-mount"), Equal("custom-mount")), actualDag)
	Expect(dag[8][0].Name).To(Equal("mount-bind"), actualDag)
	Expect(dag[9][0].Name).To(Equal("write-fstab"), actualDag)
}
