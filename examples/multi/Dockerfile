# This example demonstrates a multi-container challenge.  The `# LAUNCH`
# directive lists the build stages cmgr should launch as separate, networked
# containers: each named stage becomes its own container reachable from the
# others (and from the solver) at a DNS hostname equal to the stage name.  A
# private `builder` stage — not launched — generates the per-build secrets that
# are copied into the runtime images.
# LAUNCH work randomDnsName

##############################################################################
## builder: generates this build's secrets (passwords + SSH key pair).       #
## It is never launched or shipped whole; only the files COPY-ed out of it    #
## reach the runtime images, and its metadata.json is what cmgr records.      #
##############################################################################
FROM ubuntu:24.04 AS builder
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    openssh-client \
    python3 && \
    rm -rf /var/lib/apt/lists/*

ARG FLAG
ARG SEED

WORKDIR /build
COPY finalize.py .

# The flag lives on Alice's home machine; her work account can SSH to it.
RUN echo "$FLAG" > flag.txt

# Alice's passwordless key pair links the work box to her home machine.
RUN ssh-keygen -t rsa -b 3072 -f id_rsa -N "" -C "asmith@work"

# Derive the per-build account passwords and metadata.json (seeded by SEED).
RUN mkdir /challenge && python3 finalize.py

##############################################################################
## work: the box the player logs into as Eve.  A misconfigured sudo rule     #
## grants passwordless apt, which is the privilege-escalation vector.        #
##############################################################################
FROM ubuntu:24.04 AS work
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    openssh-client \
    openssh-server \
    sudo && \
    rm -rf /var/lib/apt/lists/*
RUN mkdir -p /run/sshd && ssh-keygen -A

RUN useradd -m -s /bin/bash asmith
RUN useradd -m -s /bin/bash esmythe

# The vulnerable rule: the developer accounts may run apt/apt-get as root with
# no password, so apt's Pre-Invoke hook runs arbitrary code as root.  sudo
# ignores any drop-in that is group/world writable, so the mode must be 0440
# (COPY otherwise preserves the source file's permissions).
COPY apt-get.sudoers /etc/sudoers.d/badsudo
RUN chmod 0440 /etc/sudoers.d/badsudo

COPY --from=builder /build/set-passwords.sh .
RUN /bin/bash set-passwords.sh && rm set-passwords.sh

# Alice's (asmith's) key and SSH config let her pivot to her home machine.
RUN install -d -m 0700 -o asmith -g asmith /home/asmith/.ssh
COPY --from=builder /build/id_rsa /home/asmith/.ssh/id_rsa
COPY ssh_config /home/asmith/.ssh/config
RUN chown asmith:asmith /home/asmith/.ssh/id_rsa /home/asmith/.ssh/config && \
    chmod 0600 /home/asmith/.ssh/id_rsa /home/asmith/.ssh/config

CMD ["/usr/sbin/sshd", "-D", "-e"]

EXPOSE 22
# PUBLISH 22 AS ssh

##############################################################################
## randomDnsName: Alice's home machine, holding the flag.  It is reachable    #
## only from `work` using her key, and its stage name doubles as an           #
## unguessable DNS hostname on the challenge network.                         #
##############################################################################
FROM ubuntu:24.04 AS randomDnsName
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    openssh-server && \
    rm -rf /var/lib/apt/lists/*
RUN mkdir -p /run/sshd && ssh-keygen -A

RUN useradd -m -s /bin/bash alice

COPY --from=builder /build/flag.txt /home/alice/flag.txt
RUN install -d -m 0700 -o alice -g alice /home/alice/.ssh
COPY --from=builder /build/id_rsa.pub /home/alice/.ssh/authorized_keys
RUN chown alice:alice /home/alice/flag.txt /home/alice/.ssh/authorized_keys && \
    chmod 0644 /home/alice/.ssh/authorized_keys

CMD ["/usr/sbin/sshd", "-D", "-e"]
