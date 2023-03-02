<h1 align="center">
  <br>
     <img width="184" alt="kairos-white-column 5bc2fe34" src="https://user-images.githubusercontent.com/2420543/193010398-72d4ba6e-7efe-4c2e-b7ba-d3a826a55b7d.png"><br>
    Immucore
<br>
</h1>

<h3 align="center">The Kairos immutability management interface </h3>
<p align="center">
  <a href="https://opensource.org/licenses/">
    <img src="https://img.shields.io/badge/licence-APL2-brightgreen"
         alt="license">
  </a>
  <a href="https://github.com/kairos-io/immucore/issues"><img src="https://img.shields.io/github/issues/kairos-io/immucore"></a>
  <a href="https://kairos.io/docs/" target=_blank> <img src="https://img.shields.io/badge/Documentation-blue"
         alt="docs"></a>
  <img src="https://img.shields.io/badge/made%20with-Go-blue">
  <img src="https://goreportcard.com/badge/github.com/kairos-io/immucore" alt="go report card" />
</p>


## What is immucore?

---

Immucore is the management interface to mount Kairos disks and filesystems.
Is a dracut module responsible for mounting the root tree during boot time with the immutable specific setup.
The immutability concept refers to read only root (/) system.
To ensure the linux OS is still functional certain paths or areas are required to be writable,
in those cases an ephemeral overlay tmpfs is set in place.

Additionally, the immutable rootfs module can also mount a custom list of device blocks with read write permissions, those are mostly devoted to store persistent data.


Immucore is mostly configured via kernel command line parameters or via the `/run/cos/cos-layout.env` environment file.

These are the read write paths the module mounts as part of the overlay
ephemeral tmpfs: `/etc`, `/root`, `/home`, `/opt`, `/srv`, `/usr/local`
and `/var`.


## Kernel configuration parameters

---

The immutable rootfs can be configured with the following kernel parameters:

* `cos-img/filename=<imgfile>`: This is one of the main parameters, it defines
  the location of the image file to boot from. This defines the booting mode for
  Immucore, setting in motion the full DAG to set up the system.

* `rd.cos.overlay=tmpfs:<size>`: This defines the size of the tmpfs used for
  the ephemeral overlayfs. It can be expressed in MiB or as a % of the available
  memory. Defaults to `rd.cos.overlay=tmpfs:20%` if not present.

* `rd.cos.overlay=LABEL=<vol_label>`: Optionally and mostly for debugging
  purposes the overlayfs can be mounted on top of a persistent block device.
  Block devices can be expressed by LABEL (`LABEL=<blk_label>`) or by UUID
  (`UUID=<blk_uuid>`)

* `rd.cos.mount=LABEL:<blk_label>:<mountpoint>`: This option defines a
  persistent block device and its mountpoint. Block devices can also be
  defined by UUID (`UUID=<blk_uuid>:<mountpoint>`). This option can be passed
  multiple times.

* `rd.cos.oemlabel=<label>`: This option sets the label to search for in order
  to mount the OEM partition. Defaults to COS_OEM

* `rd.cos.oemtimeout=<seconds>`: By default we assume the existence of a
  persistent block device labelled `COS_OEM` which is used to keep some
  configuration data (mostly cloud-init files). The immutable rootfs tries
  to mount this device at very early stages of the boot even before applying
  the immutable rootfs configs. It's done this way to enable to configure the
  immutable rootfs module within the cloud-init files. As the `COS_OEM` device
  might not be always present the boot process just continues without failing
  after a certain timeout. This option configures such a timeout. Defaults to
  10s.

* `rd.cos.debugrw`/`rd.immucore.debugrw`: This is a boolean option, true if present, false if not.
  This option sets the root image to be mounted as a writable device. Note this
  completely breaks the concept of an immutable root. This is helpful for
  debugging or testing purposes, so changes persist across reboots.

* `rd.cos.disable`/`rd.immucore.disable`: This is a boolean option, true if present, false if not.
  It disables the execution of any immutable rootfs module logic at boot.

* `rd.immucore.debug`: Enables debug logging

* `rd.immucore.uki`: Enables UKI booting (Experimental)


### Configuration with an environment file

---

The immutable rootfs can be configured with the `/run/cos/cos-layout.env`
environment file. It is important to note that all the immutable root
configuration is applied in initrd before switching root and after
`rootfs` cloud-init stage but before `initramfs` stage. So immutable rootfs
configuration via cloud-init using the `/run/cos/cos-layout.env` file is
only effective if called in any of the `rootfs.before`, `rootfs` or
`rootfs.after` cloud-init stages.


In the environment file few options are available:


* `VOLUMES=LABEL=<blk_label>:<mountpoint>`: This variable expects a block device
  and it mountpoint pair space separated list. The default cOS configuration is:

  `VOLUMES="LABEL=COS_OEM:/oem LABEL=COS_PERSISTENT:/usr/local"`

* `OVERLAY`: It defines the underlying device for the overlayfs as in
  `rd.cos.overlay=` kernel parameter.

* `MERGE=true`: Sets makes the `VOLUMES` values to be merged with any other
  volume that might have been defined in the kernel command line. The merging
  criteria is simple: any overlapping volume is overwritten all others are
  appended to whatever was already defined as a kernel parameter. If not
  defined defaults to `true`.

* `RW_PATHS`: This is a space separated list of paths. These are the paths
  that will be used for the ephemeral overlayfs. These are the paths that
  will be mounted as overlay on top of the `OVERLAY` (or `rd.cos.overlay`)
  device. Default value is:

  `RW_PATHS="/etc /root /home /opt /srv /usr/local /var"`
  **Note**: as those paths are overlay with an ephemeral mount (`tmpfs`),
  additional data wrote on those location won't be available on subsequent boots.

* `PERSISTENT_STATE_TARGET`: This is the folder where the persistent state data
  will be stored, if any. Default value is `/usr/local/.state`.

* `PERSISTENT_STATE_PATHS`: This is a space separated list of paths. These are
  the paths that will become writable and store its data inside
  `PERSISTENT_STATE_TARGET`. By default, this variable is empty, which means
  no persistent state area is created or used.

  **Note**: The specified paths needs either to exist or be located in an area
  which is writeable ( for example, inside locations specified with `RW_PATHS`).
  The dracut module will attempt to create non-existant directories,
  but might fail if the mountpoint where are located is read-only.

* `PERSISTENT_STATE_BIND="true|false"`: When this variable is set to true
  the persistent state paths are bind mounted (instead of using overlayfs)
  after being mirrored with the original content. By default, this variable is
  set to `false`.

Note that persistent state are is set up once the ephemeral paths and persistent
volumes are mounted. Persistent state paths can't be an already existing mount
point. If the persistent state requires any of the paths that are part of the
ephemeral area by default, then `RW_PATHS` needs to be defined to avoid
overlapping paths.

For example a common cOS configuration can be expressed as part of the
cloud-init configuration as follows:

```yaml
name: example
stage:
  rootfs:
    - name: "Layout configuration"
      environment_file: /run/cos/cos-layout.env
      environment:
        VOLUMES: "LABEL=COS_OEM:/oem LABEL=COS_PERSISTENT:/usr/local"
        OVERLAY: "tmpfs:25%"
```

You can also see the default config that we provide in https://github.com/kairos-io/kairos/blob/master/overlay/files/system/oem/11_persistency.yaml

## What is the default workflow of Immucore

----

It starts pretty early in the boot process, just after `systemd-udev-settle.service` and before `dracut-initqueue.service`
To see the full bootup process from dracut you can check [here](https://man7.org/linux/man-pages/man7/dracut.bootup.7.html)

Just after starting, Immucore mounts `/proc` if it's not mounted in order to read the `/proc/cmdline` and obtains the different stanzas in order to configure itself.
After checking the cmdline, it knows in which path is being booted, either active/passive/recovery or Netboot/LiveCD/Do nothing.

Based on that it builds a DAG with the steps needed to complete and process through the DAG until its completed. It also builds a `State` object which has all the configs needed to mount and configure the system properly.
Once the DAG has been completed (and with no errors), Immucore its finished, and it's ready for the initramfs init process to do a switch_root and pivot into the final root to boot the system.

When booting from Netboot/LiveCD/Do nothing (`rd.cos.disable` or `rd.immucore.disable` on the cmdline) the DAG It's pretty simple. 
It proceeds to create a sentinel file under `/run/cos/` with the boot mode (`live_mode`) so cloud configs can identify that they are booting from live media and ends.


When booting from active/passive/recovery the DAG gets a bit more complicated. You can see the default DAG for an active/passive/recovery system by running immucore with `--dry-run`

```bash
1.
 <init> (background: false) (weak: false)
2.
 <mount-state> (background: false) (weak: false)
 <mount-base-overlay> (background: false) (weak: false)
 <mount-tmpfs> (background: false) (weak: false)
 <create-sentinel> (background: false) (weak: false)
3.
 <discover-state> (background: false) (weak: false)
4.
 <mount-root> (background: false) (weak: false)
5.
 <mount-oem> (background: false) (weak: false)
6.
 <rootfs-hook> (background: false) (weak: false)
7.
 <load-config> (background: false) (weak: false)
8.
 <custom-mount> (background: false) (weak: false)
 <overlay-mount> (background: false) (weak: false)
9.
 <mount-bind> (background: false) (weak: false)
10.
 <write-fstab> (background: false) (weak: true)
```

As shown in the DAG, the steps are in order and that shows their dependencies, i.e. `mount-root` depends on `discover-state`and that is why it's just below it.
It won't run until the previous step has completed **without errors**.
There is also the `weak` value which indicates that this step has weak dependencies. It will run even if its dependencies failed, instead of refusing to run.



### Steps explained

 - `mount-state`: Will mount the `COS_STATE` partition under `/run/initramfs/cos-state`
 - `mount-tmpfs`: Will mount `/tmp` 
 - `create-sentinel`: Will create the sentinel file identifying the boot mode (`active_mode`, `passive_mode`, `recovery_mode` or `live_mode`) under `/run/cos/`
 - `mount-base-overlay`: Will mount the base overlay under `/run/overlay`
 - `discover-state`: Will find the correct image under `/run/initramfs/cos-state` and mount it as a loop device
 - `mount-root`: Will mount the `/dev/disk/by-label/$LABEL` device under the sysroot (Usually `/sysroot`). This label is set in grub depending on the selected entry, as part of the cmdline (i.e. `root=LABEL=COS_ACTIVE`) 
 - `mount-oem`: Will **try** to mount the oem label device under `/sysroot/oem`. This label is set in grub by default (`rd.cos.oemlabel=COS_OEM`) but also on the default `cos-layout.env` file with Kairos. This partition is not mandatory so It's allowed to fail.
 - `rootfs-hook`: Runs the cloud config stage `rootfs`. Notice that this runs very early in the process so things like binds or RW paths are not yet mounted.
 - `load-config`: This parses the `/run/cos/cos-layout.env` file (usually generated by the `rootfs` stage) and loads all the configurations.
 - `overlay-mount`: This mounts the paths set in the config (`RW_PATHS`) under the `/run/overlay` dir, so they are RW.
 - `custom-mount`: This mounts the paths set in the config (`VOLUMES`) or in cmdline `rd.cos.mount=` in the given path (`LABEL=COS_PERSISTENT:/usr/local`)
 - `mount-bind`: This mounts the paths set in the config (`PERSISTENT_STATE_PATHS` and `CUSTOM_BIND_MOUNTS`) as bind mounts under the `PERSISTENT_STATE_TARGET` which defaults to `/usr/local/.state`
 - `write-fstab`: Writes the final fstab with all the mounts into `/sysroot/fstab`

### UKI mode (Experimental)

---

There is currently support to boot in UKI mode without doing a final switch_root into `/sysroot`
This means that the initramfs is not really an initramfs but the final system and contains all the needed stuff to boot.
This mixed with a UKI binary in which we dump everything into the final binary means that you can have a single EFI file with your full system.

This is currently activated by setting the `rd.immucore.uki` on the cmdline.


------

<table>
<tr>
<th align="center">
<img width="640" height="1px">
<p> 
<small>
Documentation
</small>
</p>
</th>
<th align="center">
<img width="640" height="1">
<p> 
<small>
Contribute
</small>
</p>
</th>
</tr>
<tr>
<td>

 📚 [Getting started with Kairos](https://kairos.io/docs/getting-started) <br> :bulb: [Examples](https://kairos.io/docs/examples) <br> :movie_camera: [Video](https://kairos.io/docs/media/) <br> :open_hands:[Engage with the Community](https://kairos.io/community/)
  
</td>
<td>
  
🙌[ CONTRIBUTING.md ]( https://github.com/kairos-io/kairos/blob/master/CONTRIBUTING.md ) <br> :raising_hand: [ GOVERNANCE ]( https://github.com/kairos-io/kairos/blob/master/GOVERNANCE.md ) <br>:construction_worker:[Code of conduct](https://github.com/kairos-io/kairos/blob/master/CODE_OF_CONDUCT.md) 
  
</td>
</tr>
</table>

