package profile

type Layout struct {
	Overlay  Overlay
	OEMLabel string
	//https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-generator.sh#L71
	Mounts []string
}

type Overlay struct {
	// /run/overlay
	Base string
	// https://github.com/kairos-io/packages/blob/94aa3bef3d1330cb6c6905ae164f5004b6a58b8c/packages/system/dracut/immutable-rootfs/30cos-immutable-rootfs/cos-generator.sh#L22
	BackingBase string
}
