#!/usr/bin/env bash
#
# setup.sh — create data dirs for pcs-service integration tests.
#

set -euo pipefail

SCRIPT_NAME=$(basename "$0")
SCRIPT_DIR=$(cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=compare_common.sh
. "${SCRIPT_DIR}/compare_common.sh"

main() {
  cd "${SCRIPT_DIR}"
  ensure_workdir
  ensure_directory "${DATA_DIR}"
  ensure_directory "${BIN_DIR}"
  ensure_directory "${LOG_DIR}"
  log_pass "Directories ready under ${SCRIPT_DIR}"
  log_info "Run: ./compare.sh test smoke"
  log_info "Or:  ./compare_all.sh"
}

main "$@"
