FROM fedora:44

# dnf and ca-certificates ship with the base image. We add curl (for
# fetching bootstrap.sh + hm) and sudo (so hm-as-user can dnf install).
RUN dnf install -y sudo curl \
 && dnf clean all

# Trust the test CA so curl/git can validate the e2e webserver's cert.
COPY certs/ca.crt /etc/pki/ca-trust/source/anchors/homie-e2e.crt
RUN update-ca-trust

RUN useradd -m -s /bin/bash scout \
 && echo 'scout ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/scout

USER scout
WORKDIR /home/scout
ENV USER=scout HOME=/home/scout
CMD ["sleep", "infinity"]
