#!/bin/sh
# Shared command boundary for developer-only Homebrew probes.
run_brew() {
  label=$1
  shift
  status=0
  /usr/bin/sandbox-exec -f "${brew_sandbox:-$probe_root/sandbox.sb}" /usr/bin/env -i \
    HOME="$probe_root/home" PATH=/usr/bin:/bin:/usr/sbin:/sbin \
    TMPDIR="$probe_root/tmp" XDG_CONFIG_HOME="$probe_root/home/config" \
    HOMEBREW_CACHE="$probe_root/cache" HOMEBREW_LOGS="$probe_root/logs" \
    HOMEBREW_TEMP="$probe_root/tmp" HOMEBREW_NO_AUTO_UPDATE=1 \
    HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_INSTALL_FROM_API=1 \
    HOMEBREW_NO_ENV_HINTS=1 HOMEBREW_NO_COLOR=1 HOMEBREW_DEVELOPER=1 \
    HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_AUTOREMOVE=1 HOMEBREW_NO_BOOTSNAP=1 \
    "$probe_prefix/bin/brew" "$@" \
    > "$probe_root/$label.stdout" 2> "$probe_root/$label.stderr" || status=$?
  printf '%s\n' "$status" > "$probe_root/$label.status"
  printf '%s: exit %s\n' "$label" "$status"
  return "$status"
}
