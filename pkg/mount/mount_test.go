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
		g = herd.DAG()
	})

	Context("simple invocation", func() {
		It("mounts base overlay, attempt to mount oem, and updates the fstab", func() {
			s := &mount.State{Rootdir: "/"}

			s.Register(g)

			dag := g.Analyze()

			Expect(len(dag)).To(Equal(7), s.WriteDAG(g))    // Expect 3 layers
			Expect(len(dag[0])).To(Equal(1), s.WriteDAG(g)) // 1 Item for each layer, as are tight deps
			Expect(len(dag[1])).To(Equal(1), s.WriteDAG(g))
			Expect(len(dag[2])).To(Equal(1), s.WriteDAG(g))
			Expect(len(dag[4])).To(Equal(2), s.WriteDAG(g))

			Expect(dag[0][0].Name).To(Equal("mount-base-overlay"), s.WriteDAG(g))
			Expect(dag[1][0].Name).To(Equal("mount-oem"), s.WriteDAG(g))
			Expect(dag[2][0].Name).To(Equal("rootfs-hook"), s.WriteDAG(g))
			Expect(dag[3][0].Name).To(Equal("load-config"), s.WriteDAG(g))
			Expect(dag[4][0].Name).To(Or(Equal("overlay-mount"), Equal("custom-mount")), s.WriteDAG(g))
			Expect(dag[4][1].Name).To(Or(Equal("overlay-mount"), Equal("custom-mount")), s.WriteDAG(g))
			Expect(dag[5][0].Name).To(Equal("mount-bind"), s.WriteDAG(g))
			Expect(dag[6][0].Name).To(Equal("write-fstab"), s.WriteDAG(g))
		})

		It("mounts base overlay, attempt to mount oem, and updates the fstab", func() {
			s := &mount.State{Rootdir: "/", MountRoot: true}

			s.Register(g)

			dag := g.Analyze()

			Expect(len(dag)).To(Equal(9), s.WriteDAG(g))    // Expect 4 layers
			Expect(len(dag[0])).To(Equal(2), s.WriteDAG(g)) // 2 items in first layer
			Expect(len(dag[1])).To(Equal(1))                // 1 Item for each layer, as are tight deps
			Expect(len(dag[2])).To(Equal(1))
			Expect(len(dag[3])).To(Equal(1))
			Expect(len(dag[6])).To(Equal(2))

			Expect(dag[0][0].Name).To(Or(Equal("mount-base-overlay"), Equal("mount-state")), s.WriteDAG(g))
			Expect(dag[0][1].Name).To(Or(Equal("mount-base-overlay"), Equal("mount-state")), s.WriteDAG(g))
			Expect(dag[1][0].Name).To(Equal("discover-state"))
			Expect(dag[2][0].Name).To(Equal("mount-root"))
			Expect(dag[3][0].Name).To(Equal("mount-oem"))
			Expect(dag[4][0].Name).To(Equal("rootfs-hook"))
			Expect(dag[5][0].Name).To(Equal("load-config"))
			Expect(dag[6][0].Name).To(Or(Equal("overlay-mount"), Equal("custom-mount")), s.WriteDAG(g))
			Expect(dag[6][1].Name).To(Or(Equal("overlay-mount"), Equal("custom-mount")), s.WriteDAG(g))

			Expect(dag[7][0].Name).To(Equal("mount-bind"))
			Expect(dag[8][0].Name).To(Equal("write-fstab"))

		})

		It("mounts all", func() {
			s := &mount.State{Rootdir: "/", MountRoot: true,
				OverlayDir:   []string{"/etc"},
				BindMounts:   []string{"/etc/kubernetes"},
				CustomMounts: map[string]string{"COS_PERSISTENT": "/usr/local"}}

			s.Register(g)

			dag := g.Analyze()

			Expect(len(dag)).To(Equal(9), s.WriteDAG(g))    // Expect 6 layers
			Expect(len(dag[0])).To(Equal(2), s.WriteDAG(g)) // 2 items in first layer
			Expect(len(dag[1])).To(Equal(1))                // 1 Item for each layer, as are tight deps
			Expect(len(dag[2])).To(Equal(1))
			Expect(len(dag[3])).To(Equal(1))
			Expect(len(dag[6])).To(Equal(2))

			Expect(dag[0][0].Name).To(Or(Equal("mount-base-overlay"), Equal("mount-state")), s.WriteDAG(g))
			Expect(dag[0][1].Name).To(Or(Equal("mount-base-overlay"), Equal("mount-state")), s.WriteDAG(g))
			Expect(dag[1][0].Name).To(Equal("discover-state"))
			Expect(dag[2][0].Name).To(Equal("mount-root"))
			Expect(dag[3][0].Name).To(Equal("mount-oem"))
			Expect(dag[4][0].Name).To(Equal("rootfs-hook"))
			Expect(dag[5][0].Name).To(Equal("load-config"))
			Expect(dag[6][0].Name).To(Or(Equal("overlay-mount"), Equal("custom-mount")), s.WriteDAG(g))
			Expect(dag[6][1].Name).To(Or(Equal("overlay-mount"), Equal("custom-mount")), s.WriteDAG(g))

			Expect(dag[7][0].Name).To(Equal("mount-bind"))
			Expect(dag[8][0].Name).To(Equal("write-fstab"))
		})

		It("Mountop timeouts", func() {
			s := &mount.State{}
			f := s.MountOP("", "", "", []string{}, 1*time.Second)
			err := f(context.Background())
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("exhausted"))
		})
	})
})
