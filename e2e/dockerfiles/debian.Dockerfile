FROM debian:13.5

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
ENV USER=scout HOME=/home/scout
CMD ["sleep", "infinity"]
