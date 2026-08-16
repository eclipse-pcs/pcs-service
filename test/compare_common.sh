#!/usr/bin/env bash
# Shared helpers for pcs-service integration test scripts.

SCRIPT_DIR="${SCRIPT_DIR:-$(cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"

# shellcheck source=compare_env.sh
. "${SCRIPT_DIR}/compare_env.sh"
if [[ -f "${SCRIPT_DIR}/compare_env.local.sh" ]]; then
  # shellcheck source=/dev/null
  . "${SCRIPT_DIR}/compare_env.local.sh"
fi

if [[ -z "${SCRIPT_NAME:-}" ]]; then
  SCRIPT_NAME=$(basename "${BASH_SOURCE[0]}")
fi

log_info() { printf '[%s] INFO %s\n' "${SCRIPT_NAME}" "$*"; }
log_pass() { printf '[%s] PASS %s\n' "${SCRIPT_NAME}" "$*"; }
log_fail() { printf '[%s] FAIL %s\n' "${SCRIPT_NAME}" "$*"; }

die() {
  printf '[%s] ERROR: %s\n' "${SCRIPT_NAME}" "$*" >&2
  exit 1
}

ensure_workdir() {
  [[ "${PWD}" == "${SCRIPT_DIR}" ]] || die "Run from ${SCRIPT_DIR} (current: ${PWD})"
}

ensure_directory() {
  [[ -d "$1" ]] || mkdir -p "$1"
}

wait_for_port() {
  local port="$1"
  local retries="${2:-60}"
  while (( retries > 0 )); do
    if nc -z "${PCS_SERVICE_HOST}" "${port}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.2
    ((retries--)) || true
  done
  return 1
}

build_bins() {
  ensure_directory "${BIN_DIR}"
  log_info "Building pcs-service binaries -> ${BIN_DIR}"
  (cd "${REPO_ROOT}" && go build -o "${BIN_DIR}/pcs-service" ./cmd/pcs-service) || die "go build pcs-service failed"
  (cd "${REPO_ROOT}" && go build -o "${BIN_DIR}/pcs-split" ./cmd/pcs-split) || die "go build pcs-split failed"
  (cd "${REPO_ROOT}" && go build -o "${BIN_DIR}/pcs-merge" ./cmd/pcs-merge) || die "go build pcs-merge failed"
}

PCS_SERVICE_PID_FILE="${SCRIPT_DIR}/pcs-service.pid"

stop_pcs_service() {
  if [[ -f "${PCS_SERVICE_PID_FILE}" ]]; then
    local pid
    pid=$(cat "${PCS_SERVICE_PID_FILE}")
    if kill -0 "${pid}" 2>/dev/null; then
      log_info "Stopping pcs-service (pid ${pid})"
      kill "${pid}" 2>/dev/null || true
      wait "${pid}" 2>/dev/null || true
    fi
    rm -f "${PCS_SERVICE_PID_FILE}"
  fi
  if command -v lsof >/dev/null 2>&1; then
    local port_pid
    port_pid=$(lsof -ti "tcp:${PCS_SERVICE_PORT}" -sTCP:LISTEN 2>/dev/null || true)
    if [[ -n "${port_pid}" ]]; then
      log_info "Stopping stale listener on port ${PCS_SERVICE_PORT} (pid ${port_pid})"
      kill "${port_pid}" 2>/dev/null || true
      wait "${port_pid}" 2>/dev/null || true
    fi
  fi
}

start_pcs_service() {
  build_bins
  stop_pcs_service
  ensure_directory "${LOG_DIR}"
  log_info "Starting pcs-service on ${PCS_SERVICE_LISTEN}"
  (
    "${BIN_DIR}/pcs-service" \
      --listen="${PCS_SERVICE_LISTEN}" \
      --token="${PCS_SERVICE_TOKEN}" \
      --chunk-size=7 \
      >>"${LOG_DIR}/pcs-service.log" 2>&1 &
    echo $! >"${PCS_SERVICE_PID_FILE}"
  )
  wait_for_port "${PCS_SERVICE_PORT}" 90 || die "pcs-service port ${PCS_SERVICE_PORT} not ready"
}

prepare_workdir() {
  local workdir="$1"
  rm -rf "${workdir}"
  ensure_directory "${workdir}"
  ensure_directory "${workdir}/storageA"
  ensure_directory "${workdir}/storageB"
  ensure_directory "${workdir}/storageC"
}

write_secret_file() {
  local path="$1"
  local content="$2"
  printf '%s' "${content}" >"${path}"
}

verify_six_particles() {
  local dir="$1"
  local base="$2"
  local missing=0
  [[ -f "${dir}/storageA/${base}.ec" ]] || missing=1
  [[ -f "${dir}/storageA/${base}.on" ]] || missing=1
  [[ -f "${dir}/storageB/${base}.oc" ]] || missing=1
  [[ -f "${dir}/storageB/${base}.en" ]] || missing=1
  [[ -f "${dir}/storageC/${base}.cp" ]] || missing=1
  [[ -f "${dir}/storageC/${base}.np" ]] || missing=1
  [[ "${missing}" -eq 0 ]] || die "Missing particle files for ${base} under ${dir}"
}

reconstructed_path() {
  local workdir="$1"
  local base="$2"
  local stem="${base%.*}"
  local ext=""
  if [[ "${stem}" != "${base}" ]]; then
    ext=".${base##*.}"
  fi
  if [[ -z "${ext}" ]]; then
    printf '%s/%s_reconstructed' "${workdir}" "${stem}"
  else
    printf '%s/%s_reconstructed%s' "${workdir}" "${stem}" "${ext}"
  fi
}

split_and_merge() {
  local workdir="$1"
  local base="$2"
  local secret_file="${workdir}/${base}"
  local out_file
  out_file=$(reconstructed_path "${workdir}" "${base}")

  "${BIN_DIR}/pcs-split" \
    -host="${PCS_SERVICE_HOST}" \
    -port="${PCS_SERVICE_PORT}" \
    -token="${PCS_SERVICE_TOKEN}" \
    -f "${secret_file}" \
    -o "${workdir}" || die "pcs-split failed"

  verify_six_particles "${workdir}" "${base}"

  "${BIN_DIR}/pcs-merge" \
    -host="${PCS_SERVICE_HOST}" \
    -port="${PCS_SERVICE_PORT}" \
    -token="${PCS_SERVICE_TOKEN}" \
    -f "${base}" \
    -dir="${workdir}" \
    -o "${out_file}" || die "pcs-merge failed"

  cmp -s "${secret_file}" "${out_file}" || die "Round-trip content mismatch for ${base}"
}

make_fixed_size_file() {
  local path="$1"
  local size="$2"
  dd if=/dev/urandom of="${path}" bs="${size}" count=1 status=none
}

recovery_roundtrip() {
  local workdir="$1"
  local base="$2"
  local secret_file="$3"
  local delete_rel="$4"
  local out_file
  out_file=$(reconstructed_path "${workdir}" "${base}")

  "${BIN_DIR}/pcs-split" \
    -host="${PCS_SERVICE_HOST}" \
    -port="${PCS_SERVICE_PORT}" \
    -token="${PCS_SERVICE_TOKEN}" \
    -f "${secret_file}" \
    -o "${workdir}" || die "pcs-split failed"

  verify_six_particles "${workdir}" "${base}"
  rm -f "${workdir}/${delete_rel}"

  "${BIN_DIR}/pcs-merge" \
    -host="${PCS_SERVICE_HOST}" \
    -port="${PCS_SERVICE_PORT}" \
    -token="${PCS_SERVICE_TOKEN}" \
    -f "${base}" \
    -dir="${workdir}" \
    -o "${out_file}" || die "pcs-merge recovery failed"

  cmp -s "${secret_file}" "${out_file}" || die "Recovery round-trip mismatch for ${base}"

  if [[ -f "${LOG_DIR}/pcs-service.log" ]]; then
    if ! grep -q "recovery=true" "${LOG_DIR}/pcs-service.log"; then
      die "expected recovery=true in server log"
    fi
  fi
}
