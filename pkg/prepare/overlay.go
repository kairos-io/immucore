package prepare

/*
func hasMountpoint(path string, mounts []string) bool {
	for _, mount := range mounts {
		if strings.HasSuffix(mount, path) {
			return true
		}
	}
	return false
}

func getStateMountpoints(statePaths []string, mountpoints []string) string {
	var stateMounts string
	for _, path := range statePaths {
		if !hasMountpoint(path, mountpoints) {
			stateMounts += path + " "
		}
	}
	return stateMounts
}
func getOverlayMountpoints(rwPaths []string, mounts []string) string {
	var mountpoints string

	for _, path := range rwPaths {
		if !hasMountpoint(path, mounts) {
			mountpoints += path + ":overlay "
		}
	}
	return mountpoints
}
*/
