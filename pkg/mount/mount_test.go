package mount_test

import (
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

			Expect(len(dag)).To(Equal(3))    // Expect 3 layers
			Expect(len(dag[0])).To(Equal(1)) // 1 Item for each layer, as are tight deps
			Expect(len(dag[1])).To(Equal(1))
			Expect(len(dag[2])).To(Equal(1))

			Expect(dag[0][0].Name).To(Equal("mount-base-overlay"))
			Expect(dag[1][0].Name).To(Equal("mount-oem"))
			Expect(dag[2][0].Name).To(Equal("write-fstab"))
		})

		It("mounts base overlay, attempt to mount oem, and updates the fstab", func() {
			s := &mount.State{Rootdir: "/", MountRoot: true}

			s.Register(g)

			dag := g.Analyze()

			Expect(len(dag)).To(Equal(5), s.WriteDAG(g))    // Expect 4 layers
			Expect(len(dag[0])).To(Equal(2), s.WriteDAG(g)) // 2 items in first layer
			Expect(len(dag[1])).To(Equal(1))                // 1 Item for each layer, as are tight deps
			Expect(len(dag[2])).To(Equal(1))
			Expect(len(dag[3])).To(Equal(1))
			Expect(len(dag[4])).To(Equal(1))

			Expect(dag[0][0].Name).To(Or(Equal("mount-base-overlay"), Equal("mount-state")))
			Expect(dag[0][1].Name).To(Or(Equal("mount-base-overlay"), Equal("mount-state")))

			Expect(dag[1][0].Name).To(Equal("discover-state"))
			Expect(dag[2][0].Name).To(Equal("mount-root"))
			Expect(dag[3][0].Name).To(Equal("mount-oem"))
			Expect(dag[4][0].Name).To(Equal("write-fstab"))
		})

		It("mounts all", func() {
			s := &mount.State{Rootdir: "/", MountRoot: true, OverlayDir: []string{"/etc"}, BindMounts: []string{"/etc/kubernetes"}, CustomMounts: map[string]string{"COS_PERSISTENT": "/usr/local"}}

			s.Register(g)

			dag := g.Analyze()

			Expect(len(dag)).To(Equal(6), s.WriteDAG(g))    // Expect 6 layers
			Expect(len(dag[0])).To(Equal(2), s.WriteDAG(g)) // 2 items in first layer
			Expect(len(dag[1])).To(Equal(1), s.WriteDAG(g)) // 1 Item for each layer, as are tight deps
			Expect(len(dag[2])).To(Equal(1), s.WriteDAG(g))
			Expect(len(dag[3])).To(Equal(3), s.WriteDAG(g))
			Expect(len(dag[4])).To(Equal(1), s.WriteDAG(g))
			Expect(len(dag[5])).To(Equal(1), s.WriteDAG(g))

			Expect(dag[0][0].Name).To(Or(Equal("mount-base-overlay"), Equal("mount-state")))
			Expect(dag[0][1].Name).To(Or(Equal("mount-base-overlay"), Equal("mount-state")))

			Expect(dag[1][0].Name).To(Equal("discover-state"))
			Expect(dag[2][0].Name).To(Equal("mount-root"))

			Expect(dag[3][0].Name).To(Or(Equal("mount-oem"), Equal("overlay-mount-/etc"), Equal("custom-mount-/usr/local")))
			Expect(dag[3][1].Name).To(Or(Equal("mount-oem"), Equal("overlay-mount-/etc"), Equal("custom-mount-/usr/local")))
			Expect(dag[3][2].Name).To(Or(Equal("mount-oem"), Equal("overlay-mount-/etc"), Equal("custom-mount-/usr/local")))

			Expect(dag[4][0].Name).To(Equal("mount-state-/etc/kubernetes"))
			Expect(dag[5][0].Name).To(Equal("write-fstab"))
		})
	})
})
