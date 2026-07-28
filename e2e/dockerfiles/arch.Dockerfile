FROM archlinux:base

# The base image ships with no pacman sync databases at all, so an
# install would fail with "target not found" before it ever reached the
# network. -Syu (not -Sy) because a refresh followed by an install is the
# partial-upgrade footgun; inside the image build, upgrading first is
# free. hm itself never syncs — see internal/packages/pacman.go.
RUN pacman -Syu --needed --noconfirm sudo curl ca-certificates \
 && pacman -Scc --noconfirm

# Trust the test CA so curl/git can validate the e2e webserver's cert.
COPY certs/ca.crt /etc/ca-certificates/trust-source/anchors/homie-e2e.crt
RUN update-ca-trust

RUN useradd -m -s /bin/bash scout \
 && echo 'scout ALL=(ALL) NOPASSWD: ALL' > /etc/sudoers.d/scout

USER scout
WORKDIR /home/scout
ENV USER=scout HOME=/home/scout
CMD ["sleep", "infinity"]
