#!/bin/bash
# Reward 1 when /app/hello.txt holds exactly "hello isekai"; no network needed.
mkdir -p /logs/verifier
if [ "$(cat /app/hello.txt 2>/dev/null)" = "hello isekai" ]; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi
