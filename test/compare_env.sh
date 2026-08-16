#!/usr/bin/env bash
#
# compare_env.sh — default environment for pcs-service bash integration tests.
# Override in compare_env.local.sh (untracked) if needed.
#

SCRIPT_DIR="${SCRIPT_DIR:-$(cd -- "$(dirname "${BASH_SOURCE[0]}")" && pwd)}"
REPO_ROOT="${REPO_ROOT:-$(cd "${SCRIPT_DIR}/.." && pwd)}"
DATA_DIR="${DATA_DIR:-${SCRIPT_DIR}/_data}"
BIN_DIR="${BIN_DIR:-${REPO_ROOT}/bin}"
LOG_DIR="${LOG_DIR:-${SCRIPT_DIR}/_logs}"

PCS_SERVICE_HOST="${PCS_SERVICE_HOST:-127.0.0.1}"
PCS_SERVICE_PORT="${PCS_SERVICE_PORT:-14567}"
PCS_SERVICE_TOKEN="${PCS_SERVICE_TOKEN:-TEST_TOKEN}"
PCS_SERVICE_LISTEN="${PCS_SERVICE_HOST}:${PCS_SERVICE_PORT}"

PCS_TEST_FILE="${PCS_TEST_FILE:-hello.txt}"
PCS_TEST_SECRET="${PCS_TEST_SECRET:-pcs-service shell smoke $(date -u +%Y-%m-%dT%H:%M:%SZ)}"

COMPARE_ALL_SLEEP_BETWEEN_TESTS="${COMPARE_ALL_SLEEP_BETWEEN_TESTS:-0}"
