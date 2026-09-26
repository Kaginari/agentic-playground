# The Harbor runner: Harbor's CLI in a container. Task containers are siblings started through
# the host's Docker socket (mounted at run time), so nothing lands in the host's Python.
FROM docker:27-cli AS dockercli
FROM python:3.13-slim
COPY --from=dockercli /usr/local/bin/docker /usr/local/bin/docker
COPY --from=dockercli /usr/local/libexec/docker/cli-plugins /usr/local/libexec/docker/cli-plugins
RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates \
 && rm -rf /var/lib/apt/lists/* && pip install --no-cache-dir harbor
WORKDIR /bench
ENTRYPOINT ["harbor"]
