#!/usr/bin/env bash
# The €0 benchmark smoke: Harbor runs tasks/hello-file in Docker with the agent inside, against the
# fake OpenAI-compatible server. FAKE_MODE=right must score 1.0 and FAKE_MODE=wrong 0.0; each launch
# is appended to bench/runs.jsonl.
#   bench/smoke.sh            (expects bin/<dist> and bin/fakevllm built, and the <dist>-harbor image)
set -euo pipefail
R=$(cd "$(dirname "$0")/.." && pwd)
DIST=${BENCH_DIST:-$(ls "$R/cmd")}
BIN="$R/bin/$DIST"
IMAGE=${HARBOR_IMAGE:-$DIST-harbor}
GW=$(docker network inspect bridge -f '{{(index .IPAM.Config 0).Gateway}}')
PORT=18000
[ -x "$BIN" ] || { echo "missing $BIN"; exit 2; }
[ -x "$R/bin/fakevllm" ] || { echo "missing $R/bin/fakevllm"; exit 2; }
sed "s#http://172.17.0.1:18000/v1#http://$GW:$PORT/v1#" "$R/bench/configs/fake-vllm.yaml" > "$R/bench/.smoke-config.yaml"

run_mode() {
  local mode=$1 want=$2
  FAKE_MODE=$mode "$R/bin/fakevllm" -addr "$GW:$PORT" > "$R/bench/.fakevllm-$mode.log" 2>&1 &
  local fpid=$!
  trap 'kill $fpid 2>/dev/null || true' RETURN
  sleep 1
  docker run --rm -v /var/run/docker.sock:/var/run/docker.sock -v "$R:$R" -w "$R/bench" \
    --group-add "$(stat -c %g /var/run/docker.sock)" \
    -e PYTHONPATH="$R/bench/harbor" -e BENCH_DIST="$DIST" -e BENCH_BIN="$BIN" \
    -e BENCH_CONFIG="$R/bench/.smoke-config.yaml" -e VLLM_API_KEY=fake -e BENCH_FORWARD_ENV=VLLM_API_KEY \
    "$IMAGE" run -p tasks/hello-file -a harbor_agent:HarborAgent -m vllm/fake/vllm-coder \
    --allow-agent-host "$GW" -o jobs -q -y
  local job; job=$(ls -1t "$R/bench/jobs" | head -1)
  python3 "$R/bench/record.py" "$R/bench/jobs/$job" --purpose "smoke: fake vLLM ($mode)" --expect "$want"
}

run_mode right 1.0
run_mode wrong 0.0
echo "smoke: right→1.0 and wrong→0.0 as expected"
