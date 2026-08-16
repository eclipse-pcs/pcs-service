#!/usr/bin/env bash
#
# compare.sh — black-box shell tests for pcs-service (TCP split/merge).
#
# Usage:
#   ./compare.sh list
#   ./compare.sh test smoke
#

set -euo pipefail

SCRIPT_NAME=$(basename "$0")
SCRIPT_DIR=$(cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=compare_common.sh
. "${SCRIPT_DIR}/compare_common.sh"

COMMAND=""
COMMAND_ARG=""
VERBOSE=0

usage() {
  cat <<EOF
Usage: ${SCRIPT_NAME} [options] <command>

Commands:
  list                 List available tests.
  test <name>          Run smoke | object | golden | recovery | stdin

Options:
  -v, --verbose        Show subprocess output.
  -h, --help

Run from: ${SCRIPT_DIR}
EOF
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      -v | --verbose) VERBOSE=1 ;;
      -h | --help) usage; exit 0 ;;
      list)
        COMMAND=list
        ;;
      test)
        COMMAND=test
        ;;
      smoke | object | golden | recovery | stdin)
        if [[ "${COMMAND}" == "test" && -z "${COMMAND_ARG}" ]]; then
          COMMAND_ARG="$1"
        else
          die "Unknown argument: $1"
        fi
        ;;
      *)
        die "Unknown argument: $1"
        ;;
    esac
    shift
  done
}

run_verbose() {
  if [[ "${VERBOSE}" -eq 1 ]]; then
    "$@"
  else
    "$@" >/dev/null 2>&1
  fi
}

test_smoke() {
  log_info "test smoke (streaming)"
  local workdir="${DATA_DIR}/smoke"
  prepare_workdir "${workdir}"
  write_secret_file "${workdir}/${PCS_TEST_FILE}" "${PCS_TEST_SECRET}"
  split_and_merge "${workdir}" "${PCS_TEST_FILE}"
  log_pass "smoke"
}

test_object() {
  log_info "test object (odd-length secret)"
  local workdir="${DATA_DIR}/object"
  local base="odd.txt"
  prepare_workdir "${workdir}"
  # 17 bytes — odd length exercises seam + parity tail
  write_secret_file "${workdir}/${base}" "odd-len-secret!!"
  split_and_merge "${workdir}" "${base}"
  log_pass "object"
}

test_golden() {
  log_info "test golden (particle file layout + round-trip)"
  local workdir="${DATA_DIR}/golden"
  prepare_workdir "${workdir}"
  write_secret_file "${workdir}/${PCS_TEST_FILE}" "Golden shell decode check"
  "${BIN_DIR}/pcs-split" \
    -host="${PCS_SERVICE_HOST}" \
    -port="${PCS_SERVICE_PORT}" \
    -token="${PCS_SERVICE_TOKEN}" \
    -f "${workdir}/${PCS_TEST_FILE}" \
    -o "${workdir}" || die "pcs-split failed"
  verify_six_particles "${workdir}" "${PCS_TEST_FILE}"
  local min_size=64
  local f
  for f in \
    "${workdir}/storageA/${PCS_TEST_FILE}.ec" \
    "${workdir}/storageA/${PCS_TEST_FILE}.on" \
    "${workdir}/storageB/${PCS_TEST_FILE}.oc" \
    "${workdir}/storageB/${PCS_TEST_FILE}.en" \
    "${workdir}/storageC/${PCS_TEST_FILE}.cp" \
    "${workdir}/storageC/${PCS_TEST_FILE}.np"; do
    [[ -f "${f}" ]] || die "missing ${f}"
    local sz
    sz=$(wc -c <"${f}" | tr -d ' ')
    [[ "${sz}" -ge "${min_size}" ]] || die "particle file too small: ${f} (${sz} bytes)"
  done
  local out
  out=$(reconstructed_path "${workdir}" "${PCS_TEST_FILE}")
  "${BIN_DIR}/pcs-merge" \
    -host="${PCS_SERVICE_HOST}" \
    -port="${PCS_SERVICE_PORT}" \
    -token="${PCS_SERVICE_TOKEN}" \
    -f "${PCS_TEST_FILE}" \
    -dir="${workdir}" \
    -o "${out}" || die "pcs-merge failed"
  cmp -s "${workdir}/${PCS_TEST_FILE}" "${out}" || die "golden round-trip mismatch"
  log_pass "golden"
}

test_recovery() {
  log_info "test recovery (missing even cypher particle)"
  local workdir="${DATA_DIR}/recovery"
  local base="${PCS_TEST_FILE}"
  prepare_workdir "${workdir}"
  write_secret_file "${workdir}/${base}" "parity recovery shell test"
  "${BIN_DIR}/pcs-split" \
    -host="${PCS_SERVICE_HOST}" \
    -port="${PCS_SERVICE_PORT}" \
    -token="${PCS_SERVICE_TOKEN}" \
    -f "${workdir}/${base}" \
    -o "${workdir}" || die "pcs-split failed"
  rm -f "${workdir}/storageA/${base}.ec"
  local out
  out=$(reconstructed_path "${workdir}" "${base}")
  "${BIN_DIR}/pcs-merge" \
    -host="${PCS_SERVICE_HOST}" \
    -port="${PCS_SERVICE_PORT}" \
    -token="${PCS_SERVICE_TOKEN}" \
    -f "${base}" \
    -dir="${workdir}" \
    -o "${out}" || die "pcs-merge recovery failed"
  cmp -s "${workdir}/${base}" "${out}" || die "Recovery round-trip mismatch"
  log_pass "recovery"
}

test_stdin() {
  log_info "test stdin (cat pipe streaming)"
  local workdir="${DATA_DIR}/stdin"
  local base="input.bin"
  prepare_workdir "${workdir}"
  # 512 KiB — exercises multi-chunk streaming without slow CI
  dd if=/dev/urandom of="${workdir}/${base}" bs=1024 count=512 status=none
  cat "${workdir}/${base}" | "${BIN_DIR}/pcs-split" \
    -host="${PCS_SERVICE_HOST}" \
    -port="${PCS_SERVICE_PORT}" \
    -token="${PCS_SERVICE_TOKEN}" \
    -f - \
    -name="${base}" \
    -o "${workdir}" || die "pcs-split stdin failed"
  verify_six_particles "${workdir}" "${base}"
  local out
  out=$(reconstructed_path "${workdir}" "${base}")
  "${BIN_DIR}/pcs-merge" \
    -host="${PCS_SERVICE_HOST}" \
    -port="${PCS_SERVICE_PORT}" \
    -token="${PCS_SERVICE_TOKEN}" \
    -f "${base}" \
    -dir="${workdir}" \
    -o "${out}" || die "pcs-merge failed"
  cmp -s "${workdir}/${base}" "${out}" || die "stdin round-trip mismatch"
  log_pass "stdin"
}

run_test() {
  case "${COMMAND_ARG}" in
    smoke) test_smoke ;;
    object) test_object ;;
    golden) test_golden ;;
    recovery) test_recovery ;;
    stdin) test_stdin ;;
    *) die "Unknown test: ${COMMAND_ARG} (use smoke, object, golden, recovery, or stdin)" ;;
  esac
}

main() {
  parse_args "$@"
  cd "${SCRIPT_DIR}"
  ensure_workdir

  case "${COMMAND}" in
    list)
      echo "smoke"
      echo "object"
      echo "golden"
      echo "recovery"
      echo "stdin"
      ;;
    test)
      trap 'stop_pcs_service' EXIT
      start_pcs_service
      run_test
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
