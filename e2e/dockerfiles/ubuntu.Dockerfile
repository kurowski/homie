FROM ubuntu:26.04

# Minimal base. The point of `hm apply` is to install everything else,
# so we ship only what's needed to reach the first line of bootstrap.sh:
# bash, sudo (so hm-as-user can apt-get install), curl (to fetch the
# script and the hm binary), and ca-certificates (so curl can validate
# our nginx webserver's TLS cert once we install the test CA on top).
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      bash sudo curl ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# Trust the test CA so curl/git can validate the e2e webserver's cert.
COPY certs/ca.crt /usr/local/share/ca-certificates/homie-e2e.crt
RUN update-ca-certificates

RUN useradd -m -s /bin/bash scout \
 && echo 'scout ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/scout

USER scout
WORKDIR /home/scout
# USER is normally set by login shells; docker exec doesn't set it.
# Real curl|bash users have it for free — set it here to match.
ENV USER=scout HOME=/home/scout
CMD ["sleep", "infinity"]
