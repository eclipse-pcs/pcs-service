#!/usr/bin/env bash
#
# compare_all.sh — run all pcs-service shell integration tests.
#

set -euo pipefail

SCRIPT_NAME=$(basename "$0")
SCRIPT_DIR=$(cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=compare_common.sh
. "${SCRIPT_DIR}/compare_common.sh"

FAILED=()

run_one() {
  local name="$1"
  log_info "Running test ${name}"
  if ! "${SCRIPT_DIR}/compare.sh" test "${name}"; then
    FAILED+=("${name}")
    return 1
  fi
  if [[ "${COMPARE_ALL_SLEEP_BETWEEN_TESTS}" != "0" ]]; then
    sleep "${COMPARE_ALL_SLEEP_BETWEEN_TESTS}"
  fi
}

main() {
  cd "${SCRIPT_DIR}"
  ensure_workdir
  trap 'stop_pcs_service' EXIT

  for t in smoke object golden recovery stdin; do
    run_one "${t}" || true
  done

  log_info "Running recovery matrix"
  if ! "${SCRIPT_DIR}/compare_recovery.sh" test all; then
    FAILED+=("recovery_matrix")
  fi

  stop_pcs_service

  if [[ ${#FAILED[@]} -gt 0 ]]; then
    log_fail "Failed tests: ${FAILED[*]}"
    exit 1
  fi
  log_pass "All tests passed"
}

main "$@"
