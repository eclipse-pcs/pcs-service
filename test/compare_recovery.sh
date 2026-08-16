#!/usr/bin/env bash
#
# compare_recovery.sh — parity recovery matrix (streaming merge path).
#
# Usage:
#   ./compare_recovery.sh list
#   ./compare_recovery.sh test small_odd_ec
#   ./compare_recovery.sh test all
#

set -euo pipefail

SCRIPT_NAME=$(basename "$0")
SCRIPT_DIR=$(cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=compare_common.sh
. "${SCRIPT_DIR}/compare_common.sh"

COMMAND=""
COMMAND_ARG=""

RECOVERY_CASES=(
  small_even_ec small_even_oc small_even_en small_even_on
  small_odd_ec small_odd_oc small_odd_en small_odd_on
  large_even_ec large_even_oc large_even_en large_even_on
  large_odd_ec large_odd_oc large_odd_en large_odd_on
)

usage() {
  cat <<EOF
Usage: ${SCRIPT_NAME} <command>

Commands:
  list                 List matrix case names.
  test <name|all>      Run one case or the full matrix.

Run from: ${SCRIPT_DIR}
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      list) COMMAND=list ;;
      test) COMMAND=test; shift; COMMAND_ARG="${1:-}"; break ;;
      -h | --help) usage; exit 0 ;;
      *) die "Unknown argument: $1" ;;
    esac
    shift
  done
}

case_delete_rel() {
  local case_name="$1"
  case "${case_name}" in
    *_ec) echo "storageA/${2}.ec" ;;
    *_oc) echo "storageB/${2}.oc" ;;
    *_en) echo "storageB/${2}.en" ;;
    *_on) echo "storageA/${2}.on" ;;
    *) die "unknown case ${case_name}" ;;
  esac
}

case_secret_size() {
  local case_name="$1"
  case "${case_name}" in
    small_even_*) echo 32 ;;
    small_odd_*) echo 33 ;;
    large_even_*) echo 524288 ;;
    large_odd_*) echo 524289 ;;
    *) die "unknown case ${case_name}" ;;
  esac
}

run_case() {
  local case_name="$1"
  local workdir="${DATA_DIR}/recovery_matrix/${case_name}"
  local base="secret.bin"
  local size
  size=$(case_secret_size "${case_name}")
  local delete_rel
  delete_rel=$(case_delete_rel "${case_name}" "${base}")

  prepare_workdir "${workdir}"
  make_fixed_size_file "${workdir}/${base}" "${size}"

  log_info "recovery matrix ${case_name} (size=${size}, delete=${delete_rel})"
  : >"${LOG_DIR}/pcs-service.log"
  recovery_roundtrip "${workdir}" "${base}" "${workdir}/${base}" "${delete_rel}"
  log_pass "${case_name}"
}

main() {
  parse_args "$@"
  cd "${SCRIPT_DIR}"
  ensure_workdir

  case "${COMMAND}" in
    list)
      printf '%s\n' "${RECOVERY_CASES[@]}"
      ;;
    test)
      [[ -n "${COMMAND_ARG}" ]] || die "test requires a case name or all"
      trap 'stop_pcs_service' EXIT
      start_pcs_service
      if [[ "${COMMAND_ARG}" == "all" ]]; then
        for c in "${RECOVERY_CASES[@]}"; do
          run_case "${c}"
        done
      else
        local found=0
        for c in "${RECOVERY_CASES[@]}"; do
          if [[ "${c}" == "${COMMAND_ARG}" ]]; then
            found=1
            break
          fi
        done
        [[ "${found}" -eq 1 ]] || die "Unknown case: ${COMMAND_ARG}"
        run_case "${COMMAND_ARG}"
      fi
      ;;
    "")
      usage
      exit 0
      ;;
    *)
      die "Unknown command: ${COMMAND}"
      ;;
  esac
}

main "$@"
