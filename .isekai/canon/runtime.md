# Runtime, registries, proxy and dimensions — the v0.2 design

Specified ahead of building (v0.1 ships what is validated). Each part lands by the ladder.

## The container runtime

`runtime.kind: host | container`. In `container`, every tool of a session runs in one long-lived
container (engine `docker` or `podman`, image from config — typically the company's base image),
the world mounted read-write at `/work`, everything else disposable (`isekai runtime reset`).
Package caches (`go`, `pip`, `npm`, `cargo`, `maven`) are named volumes that survive a reset.

**Permissions follow what is at risk, not the command:**

| Act | host | container |
|---|---|---|
| install packages, edit system files | outward / destructive → asks | allowed — the container is disposable |
| `rm -rf` outside `/work` | destructive → asks | allowed |
| writes under `/work` (the real world) | territory + gate | unchanged |
| network to configured registries | outward → asks | allowed (egress allowlist) |
| any other network | outward → asks | asks |
| `git push`, deploys, anything leaving the world | outward → asks | asks |

Building images never mounts the host's Docker socket (root on the host): a rootless BuildKit (or
Docker-in-Docker) sidecar builds, and pushes only to an allowlisted registry.

## Registries

`registries: [{kind, url, tokenEnv | usernameEnv+passwordEnv, private}]`, `kind` ∈ `docker`,
`go`, `pypi`, `npm`, `cargo`, `maven`, `generic`, any host (Artifactory, GitLab, Nexus, Harbor).
The runtime writes each tool's native configuration in the container so plain commands work:
`docker/config.json`, `GOPROXY`/`GOPRIVATE`/`GONOSUMDB`, `pip.conf`, `.npmrc`,
`.cargo/config.toml`, `settings.xml`.

**Secrets, in two steps:**
- *v0.2* — tokens enter the container as env/files; every tool output is scrubbed of the token
  values before the model sees it. Honest limit: an encoded leak is not caught.
- *v0.3* — a credential-injecting egress proxy in the binary: the container holds no token; all
  traffic goes through the proxy, which enforces the allowlist and adds `Authorization` only for
  registry hosts (TLS interception with the proxy's CA trusted in the container). Every request is
  journaled — the egress instrument (`instruments/egress/`): what was fetched, from where, how much.

## Corporate proxy

`proxy: {http, https, noProxy: [], authEnv, caBundle}` applies everywhere: the binary's own calls
(providers, MCP over HTTP, webfetch); the container (`HTTP(S)_PROXY`, `NO_PROXY`, the CA trusted
system-wide, and each tool's own proxy setting — pip, npm, cargo, git `http.proxy`, Go, Maven);
image pulls through the engine; and the v0.3 egress proxy chains to it upstream (allowlist and
token injection first, then the corporate proxy). `status` shows the proxy and what bypasses it.

## Worlds and dimensions

The law is `isekai.md` §Worlds and dimensions; the build:
- **Discovery** — from the working directory: the innermost world, its parent world (if nested),
  the worlds nested in it (a bounded walk), and its dimension (the manifest naming this world).
- **Manifest** — `dimension.yaml`: `name`, `worlds: [{name, path | git}]`, `relations: [{from,
  to, kind, note}]`, `parent` (the dimension above). Machine registry of dimensions in
  `~/.isekai/dimensions/` (machine-shared, inward).
- **Sovereignty in code** — the territory policy stops at a nested world's border; a write there is
  refused with the hint to ask that world.
- **Relations in the ontology** — `is:World`, `is:Dimension`, typed world bonds; projection answers
  "what in `infra` does `app` depend on".
- **Learned relations** — detectors are open-ended: any cross-world reference seen in a tool result
  or a file (outputs, image names, secret paths, URLs, queue names, schemas…) is noted as a colony
  note; the second sighting produces a manifest diff for Veldora (config.Patch, human yes).
- **Routing** — a cross-world ask is a `dispatch` to the other world's Elf body (same dimension),
  or up to the dimension above; replies come back down the same path, one hop per level.
- **The board** opens on the map: dimensions as regions, worlds as cards, nested worlds within
  their parent, relations as typed edges; a world opens into its own pages.
