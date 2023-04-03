module github.com/kairos-io/immucore

go 1.20

// replace any upstream elemental dep with our own
replace github.com/rancher/elemental-cli v0.2.1 => github.com/kairos-io/elemental-cli v0.1.0

replace github.com/rancher/elemental-cli v0.2.0 => github.com/kairos-io/elemental-cli v0.1.0

// Until yip is fixed, replace with an older known working version
replace github.com/mudler/yip v1.0.0 => github.com/mudler/yip v0.11.5-0.20230124143654-91e88dfb6648

replace github.com/mudler/yip v1.0.1 => github.com/mudler/yip v0.11.5-0.20230124143654-91e88dfb6648

require (
	github.com/containerd/containerd v1.6.19
	github.com/deniswernert/go-fstab v0.0.0-20141204152952-eb4090f26517
	github.com/hashicorp/go-multierror v1.1.1
	github.com/jaypipes/ghw v0.10.0
	github.com/joho/godotenv v1.5.1
	github.com/kairos-io/kairos v1.24.3-56.0.20230309161837-a50b11904989
	github.com/moby/sys/mountinfo v0.6.2
	github.com/mudler/go-kdetect v0.0.0-20210802130128-dd92e121bed8
	github.com/mudler/yip v1.0.0
	github.com/onsi/ginkgo/v2 v2.9.2
	github.com/onsi/gomega v1.27.6
	github.com/rancher/elemental-cli v0.2.0
	github.com/rs/zerolog v1.29.0
	github.com/sirupsen/logrus v1.9.0
	github.com/spectrocloud-labs/herd v0.4.2
	github.com/twpayne/go-vfs v1.7.2
	github.com/urfave/cli/v2 v2.25.1
	golang.org/x/sys v0.6.0
	gopkg.in/yaml.v3 v3.0.1
)

require (
	atomicgo.dev/cursor v0.1.1 // indirect
	atomicgo.dev/keyboard v0.2.9 // indirect
	github.com/Masterminds/goutils v1.1.1 // indirect
	github.com/Masterminds/semver/v3 v3.2.0 // indirect
	github.com/Masterminds/sprig/v3 v3.2.3 // indirect
	github.com/Microsoft/go-winio v0.6.0 // indirect
	github.com/Microsoft/hcsshim v0.9.7 // indirect
	github.com/ProtonMail/go-crypto v0.0.0-20230117203413-a47887b8f098 // indirect
	github.com/Sabayon/pkgs-checker v0.8.4 // indirect
	github.com/StackExchange/wmi v1.2.1 // indirect
	github.com/acomagu/bufpipe v1.0.3 // indirect
	github.com/aybabtme/rgbterm v0.0.0-20170906152045-cc83f3b3ce59 // indirect
	github.com/cavaliergopher/grab v2.0.0+incompatible // indirect
	github.com/cloudflare/circl v1.3.1 // indirect
	github.com/containerd/cgroups v1.0.4 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/crillab/gophersat v1.3.2-0.20210701121804-72b19f5b6b38 // indirect
	github.com/davidcassany/linuxkit/pkg/metadata v0.0.0-20230124104020-93ac9dd5b8e1 // indirect
	github.com/denisbrodbeck/machineid v1.0.1 // indirect
	github.com/diskfs/go-diskfs v1.2.1-0.20230123115902-fce1828bbbfa // indirect
	github.com/distribution/distribution v2.8.1+incompatible // indirect
	github.com/docker/distribution v2.8.0+incompatible // indirect
	github.com/docker/docker v20.10.23+incompatible // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/docker/libnetwork v0.8.0-dev.2.0.20200917202933-d0951081b35f // indirect
	github.com/emirpasic/gods v1.18.1 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-git/gcfg v1.5.0 // indirect
	github.com/go-git/go-billy/v5 v5.3.1 // indirect
	github.com/go-git/go-git/v5 v5.4.2 // indirect
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/pprof v0.0.0-20230228050547-1710fef4ab10 // indirect
	github.com/google/renameio v1.0.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/gookit/color v1.5.2 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-version v1.3.0 // indirect
	github.com/huandu/xstrings v1.4.0 // indirect
	github.com/imdario/mergo v0.3.13 // indirect
	github.com/ishidawataru/sctp v0.0.0-20210707070123-9a39160e9062 // indirect
	github.com/itchyny/gojq v0.12.11 // indirect
	github.com/itchyny/timefmt-go v0.1.5 // indirect
	github.com/jaypipes/pcidb v1.0.0 // indirect
	github.com/jbenet/go-context v0.0.0-20150711004518-d14ea06fba99 // indirect
	github.com/jinzhu/copier v0.0.0-20180308034124-7e38e58719c3 // indirect
	github.com/kendru/darwin/go/depgraph v0.0.0-20221105232959-877d6a81060c // indirect
	github.com/kevinburke/ssh_config v1.2.0 // indirect
	github.com/knqyf263/go-deb-version v0.0.0-20190517075300-09fca494f03d // indirect
	github.com/lithammer/fuzzysearch v1.1.5 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.17 // indirect
	github.com/mattn/go-runewidth v0.0.14 // indirect
	github.com/mitchellh/copystructure v1.2.0 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/hashstructure/v2 v2.0.2 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/libnetwork v0.8.0-dev.2.0.20200612180813-9e99af28df21 // indirect
	github.com/moby/moby v20.10.9+incompatible // indirect
	github.com/moby/sys/mount v0.3.0 // indirect
	github.com/mudler/entities v0.0.0-20220905203055-68348bae0f49 // indirect
	github.com/mudler/luet v0.0.0-20230117111542-5d3751888844 // indirect
	github.com/mudler/topsort v0.0.0-20201103161459-db5c7901c290 // indirect
	github.com/nfnt/resize v0.0.0-20180221191011-83c6a9932646 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.3-0.20211202183452-c5a74bcca799 // indirect
	github.com/opencontainers/runc v1.1.2 // indirect
	github.com/otiai10/copy v1.2.1-0.20200916181228-26f84a0b1578 // indirect
	github.com/packethost/packngo v0.29.0 // indirect
	github.com/phayes/permbits v0.0.0-20190612203442-39d7c581d2ee // indirect
	github.com/pierrec/lz4 v2.6.1+incompatible // indirect
	github.com/pilebones/go-udev v0.0.0-20210126000448-a3c2a7a4afb7 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pkg/xattr v0.4.9 // indirect
	github.com/pterm/pterm v0.12.54 // indirect
	github.com/qeesung/image2ascii v1.0.1 // indirect
	github.com/rivo/uniseg v0.4.3 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/samber/lo v1.37.0 // indirect
	github.com/satori/go.uuid v1.2.1-0.20181028125025-b2ce2384e17b // indirect
	github.com/sergi/go-diff v1.3.1 // indirect
	github.com/shopspring/decimal v1.3.1 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/tredoe/osutil/v2 v2.0.0-rc.16 // indirect
	github.com/ulikunitz/xz v0.5.11 // indirect
	github.com/vishvananda/netlink v1.2.1-beta.2 // indirect
	github.com/vishvananda/netns v0.0.4 // indirect
	github.com/wayneashleyberry/terminal-dimensions v1.1.0 // indirect
	github.com/willdonnelly/passwd v0.0.0-20141013001024-7935dab3074c // indirect
	github.com/xanzy/ssh-agent v0.3.1 // indirect
	github.com/xo/terminfo v0.0.0-20220910002029-abceb7e1c41e // indirect
	github.com/xrash/smetrics v0.0.0-20201216005158-039620a65673 // indirect
	github.com/zcalusic/sysinfo v0.9.5 // indirect
	github.com/zloylos/grsync v1.6.1 // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/crypto v0.6.0 // indirect
	golang.org/x/exp v0.0.0-20220909182711-5c715a9e8561 // indirect
	golang.org/x/mod v0.9.0 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/term v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	golang.org/x/tools v0.7.0 // indirect
	gopkg.in/djherbis/times.v1 v1.3.0 // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/warnings.v0 v0.1.2 // indirect
	gopkg.in/yaml.v1 v1.0.0-20140924161607-9f9df34309c0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	howett.net/plist v1.0.0 // indirect
	k8s.io/klog/v2 v2.30.0 // indirect
	k8s.io/mount-utils v0.23.0 // indirect
	k8s.io/utils v0.0.0-20211116205334-6203023598ed // indirect
	pault.ag/go/modprobe v0.1.2 // indirect
	pault.ag/go/topsort v0.1.1 // indirect
)
