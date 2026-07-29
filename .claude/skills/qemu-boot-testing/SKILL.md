---
name: qemu-boot-testing
description: Use when a change to immucore or kairos-agent needs verification in a real boot — building a test ISO from a specific immucore/agent/sdk version or PR, testing kernel cmdline flags (kairos.ram.*, rd.immucore.*), troubleshooting boot failures or red failure screens, checking which yip/cloud-init stages fired, verifying userdata ingestion, or gathering immucore and journald logs from a booted system.
---

# QEMU Boot Testing for Immucore

## Overview

Unit tests can't prove a boot works. This skill boots the local immucore in QEMU
and captures evidence: serial console, `/run/immucore/immucore.log`,
`journalctl -b`, and screendumps of failure screens.

**REQUIRED BACKGROUND:** generic QEMU mechanics (keystroke driving via QMP,
screen recording, guest networking, gotchas) live in the **driving-qemu-vms**
skill in [kairos-io/skills](https://github.com/kairos-io/skills); this skill's
scripts already embed the serial-expect + screendump patterns from it.

## Workflow

**1. Build the test ISO** (once per code change, ~5 min, needs docker):

```bash
./tests/build_test_iso.sh                 # local immucore + kairos-agent@main
AGENT_REF=<branch|tag|sha> ./tests/build_test_iso.sh   # pin the agent
```

Picking the versions under test:

- **immucore** — always the local working tree, uncommitted changes included.
  Output: `build/immucore-test-<sha>.iso`; a `-dirty` suffix means the tree
  had uncommitted changes.
- **kairos-agent** — any git ref of kairos-io/kairos-agent via `AGENT_REF`
  (default `main`). For a PR, use its head sha or branch name. Forks need a
  Dockerfile.test edit (the repo URL is hardcoded).
- **kairos-sdk** — no knob. For immucore: `go mod edit -replace` or bump
  `go.mod` locally, then rebuild. For the agent: push a branch of
  kairos-agent with the sdk bump and point `AGENT_REF` at it.

Confirm the booted binaries are the ones you built — stale ISOs are the #1
source of confusing results:

```bash
CHECK_CMD="immucore version; kairos-agent version" ...   # plus login knobs
```

and compare against `git describe` / the agent ref you passed.

**2. Boot it with your scenario** (~2 min per boot):

```bash
CMDLINE="kairos.ram.create_partitions rd.immucore.debug" \
USERDATA=tests/qemu/userdata-login.yaml \
LOGIN_USER=kairos LOGIN_PASS=kairos \
CHECK_CMD="kairos-agent state" \
./tests/qemu/boot_and_capture.sh
```

`tests/qemu/userdata-login.yaml` is a ready-made cloud-config creating the
kairos/kairos user — use it verbatim or as the base for scenario userdata.

All knobs are documented in the script header. Key ones:

| Knob | Effect |
|------|--------|
| `CMDLINE` | tokens injected into the first grub entry (replaces `rd.cos.disable`, drops `install-mode`/`cdroot`) |
| `USERDATA` | cloud-config file, attached as cidata (NoCloud) cdrom — add a `users:` stage to get login credentials |
| `LOGIN_USER`/`LOGIN_PASS` | log in on serial getty and dump logs from inside |
| `CHECK_CMD` | root shell command, output lands in `check.log` |
| `DISK` | pre-made qcow2 (e.g. foreign GPT for wipe-guard scenarios); default fresh empty 2G |
| `ISO` | explicit ISO; default newest `build/immucore-test-*.iso` |
| `LOGDIR` | evidence output dir; default `build/logs` — set it per scenario to keep runs apart |

Exit codes: `0` boot (+login) ok, `2` red failure screen seen (screendump saved), `1` timeout/error.

**3. Read the evidence** in `LOGDIR` (default `build/logs/`). On exit 2 (failure
screen, no login) only `boot-serial.log` + `failure-screen.png` exist —
`immucore.log`/`journalctl.log`/`check.log` require a successful login:

- `immucore.log` — needs `rd.immucore.debug` in CMDLINE for DBG lines. Grep `sentinel`, `Detected in-RAM`, `Executing stage`.
- `journalctl.log` — full journal. Stage decisions: `grep "Executing stage"` (evaluated) vs `grep "Skip "` + `stage name:` (condition false → skipped). Service starts: `grep ": Started"`.
- `boot-serial.log` — everything, including kernel messages.
- `check.log` — your `CHECK_CMD` output.
- `failure-screen.png` — only on exit 2.

## Common Mistakes

- **Stale ISO**: error messages/flags in the ISO lag your code. Rebuild after every change; check the sha in the ISO name matches `git describe`.
- **No login user**: base image has no default credentials — always pass `USERDATA` with a `users:` stage if you need to get inside.
- **Agent behaves oddly**: published images ship kairos-agent with an older kairos-sdk; the test ISO builds it from `AGENT_REF` (default main) precisely to avoid this.
- **A stage shows in both "Executing" and "Skip" greps**: yip logs `Executing stage` before evaluating its `if:` — the `Skip` line is authoritative.
- **Boot "hangs"**: check `boot-serial.log` first; the failure banner text also appears on serial, and exit 2 + screendump means the boot stopped on purpose.
