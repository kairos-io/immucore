#!/usr/bin/env python3
"""Drive a QEMU serial console over telnet for boot_and_capture.sh.

Waits for either a login prompt or the red "KAIROS BOOT FAILED" banner.
On the banner: grabs a screendump via the QEMU monitor and exits 2.
On login prompt: if LOGIN_USER/LOGIN_PASS are set, logs in and dumps
/run/immucore/immucore.log and journalctl -b to the serial stream fenced
with BEGIN_/END_ markers (split into files by the calling script), plus an
optional CHECK_CMD. Exits 0 on success, 1 on timeout/auth failure.

argv: serial_port monitor_port serial_log_path
env:  TIMEOUT LOGIN_USER LOGIN_PASS CHECK_CMD SCREENDUMP
"""
import os
import shlex
import socket
import sys
import time

serial_port, monitor_port, logpath = int(sys.argv[1]), int(sys.argv[2]), sys.argv[3]
timeout = int(os.environ.get("TIMEOUT", "300"))
user = os.environ.get("LOGIN_USER", "")
password = os.environ.get("LOGIN_PASS", "")
check_cmd = os.environ.get("CHECK_CMD", "")
screendump = os.environ.get("SCREENDUMP", "/tmp/failure-screen")

sock = socket.create_connection(("127.0.0.1", serial_port), timeout=10)
sock.settimeout(1)
log = open(logpath, "wb")
buf = b""


def pump():
    global buf
    try:
        data = sock.recv(4096)
        if data:
            log.write(data)
            log.flush()
            buf += data
    except socket.timeout:
        pass


def wait_for(needles, deadline_s, desc):
    """Wait until any needle appears; returns the needle or None."""
    global buf
    deadline = time.time() + deadline_s
    while time.time() < deadline:
        pump()
        for needle in needles:
            if needle in buf:
                print(f">>> saw {desc}: {needle.decode(errors='replace')!r}")
                buf = b""
                return needle
    print(f"!!! timeout ({deadline_s}s) waiting for {desc}")
    return None


def send(line):
    sock.sendall(line.encode() + b"\n")


def settle():
    """Wait for the shell prompt before typing the next command — a line sent
    while the previous command is still tearing down gets eaten as its stdin."""
    wait_for([b"~]$", b"~ #", b":~$"], 15, "shell prompt")


def grab_screendump():
    try:
        mon = socket.create_connection(("127.0.0.1", monitor_port), timeout=5)
        time.sleep(0.5)
        mon.recv(4096)
        mon.sendall(f"screendump {screendump}.ppm\n".encode())
        time.sleep(2)
        mon.close()
        print(f">>> screendump captured: {screendump}.ppm (harness converts it to .png)")
    except OSError as e:
        print(f"!!! screendump failed: {e}")


hit = wait_for([b"login:", b"KAIROS BOOT FAILED"], timeout, "login prompt or failure banner")
if hit is None:
    sys.exit(1)
if hit == b"KAIROS BOOT FAILED":
    # keep draining serial while the screen settles so the full failure text
    # (not just the banner title) lands in the serial log too
    end = time.time() + 8
    while time.time() < end:
        pump()
    grab_screendump()
    sys.exit(2)

if not user:
    print(">>> boot reached login prompt (no LOGIN_USER set, not logging in)")
    sys.exit(0)

time.sleep(1)
send(user)
if not wait_for([b"Password:"], 30, "password prompt"):
    sys.exit(1)
send(password)
time.sleep(2)
send("echo LOGIN_OK_$(id -un)")
if not wait_for([("LOGIN_OK_" + user).encode()], 30, "shell as " + user):
    sys.exit(1)
settle()

# markers are split in the command text (BEGIN_"X") so the echoed command
# line never matches the awk fences in boot_and_capture.sh, only real output
if check_cmd:
    # one single line: extra lines sent while a command runs get eaten as its
    # stdin. shlex.quote wraps the whole fenced pipeline (including any quotes
    # inside CHECK_CMD); the BEGIN_"X" marker split keeps the echoed command
    # line from matching the awk fences.
    fenced = 'echo BEGIN_"CHECK"; ' + check_cmd + '; echo END_"CHECK"'
    send("sudo /bin/sh -c " + shlex.quote(fenced))
    if not wait_for([b"END_CHECK"], 60, "check command"):
        sys.exit(1)
    settle()
send("sudo /bin/sh -c 'echo BEGIN_\"IMMUCORE_LOG\"; cat /run/immucore/immucore.log; echo END_\"IMMUCORE_LOG\"'")
if not wait_for([b"END_IMMUCORE_LOG"], 60, "immucore log dump"):
    sys.exit(1)
settle()
send("sudo /bin/sh -c 'echo BEGIN_\"JOURNAL\"; journalctl -b --no-pager -o short-precise; echo END_\"JOURNAL\"'")
if not wait_for([b"END_JOURNAL"], 180, "journal dump"):
    sys.exit(1)
print(">>> login + log capture complete")
sys.exit(0)
