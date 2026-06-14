#!/usr/bin/env bash
# Бутстрап автономного Grok: первый прогон по полному промту.
# Держит общий lock, чтобы крон-тики не запускали второй grok параллельно.
export HOME=/root
export PATH=/usr/local/bin:/usr/bin:/bin
LOCK=/root/olcrtc/build-state/run.lock
LOG=/root/olcrtc/build-state/run.log
guard_disk() {
  local pct; pct=$(df --output=pcent / | tail -1 | tr -dc '0-9')
  if [ "${pct:-0}" -ge 90 ]; then
    rm -rf /tmp/go-build* 2>/dev/null
    /usr/local/go/bin/go clean -cache 2>/dev/null
    journalctl --vacuum-size=200M >/dev/null 2>&1
    echo "$(date -u +%FT%TZ) disk-guard: cleaned (was ${pct}%)" >> "$LOG"
  fi
}
exec 9>"$LOCK"
if ! flock -n 9; then
  echo "$(date -u +%FT%TZ) bootstrap: already running, skip" >> "$LOG"
  exit 0
fi
guard_disk
echo "$(date -u +%FT%TZ) bootstrap: START" >> "$LOG"
cd /root/olcrtc || exit 1
grok --prompt-file /root/olcrtc/docs/AUTONOMOUS_BUILD_PROMPT.md >> "$LOG" 2>&1
echo "$(date -u +%FT%TZ) bootstrap: END rc=$?" >> "$LOG"
