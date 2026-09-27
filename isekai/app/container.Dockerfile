# The runtime a containered session runs in (--containered; canon/containered.md). The binary is
# not in the image: it is mounted read-only from the host, so the image never goes stale with it.
# The agent's bash sees these tools; name a richer image with --image <ref>.
FROM debian:bookworm-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      bash ca-certificates coreutils curl diffutils file findutils gawk git grep jq less make \
      patch procps python3 ripgrep sed tar unzip xz-utils \
 && rm -rf /var/lib/apt/lists/*
ENV LANG=C.UTF-8
