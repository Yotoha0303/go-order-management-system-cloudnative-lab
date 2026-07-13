#!/usr/bin/env bash
set -euo pipefail

OUTPUT_DIR="${1:-fault-evidence/migration}"
CONTAINER="go-order-migration-drill-${GITHUB_RUN_ID:-local}"
PASSWORD="migration-drill-password"
DATABASE="fault_migration"
mkdir -p "${OUTPUT_DIR}" "${OUTPUT_DIR}/invalid"

cleanup() {
  docker rm -fv "${CONTAINER}" >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup

docker run -d --name "${CONTAINER}" \
  --tmpfs /var/lib/mysql:rw,noexec,nosuid,size=512m \
  -e "MYSQL_ROOT_PASSWORD=${PASSWORD}" \
  mysql:8.4 >/dev/null

for attempt in $(seq 1 60); do
  if docker exec -e "MYSQL_PWD=${PASSWORD}" "${CONTAINER}" \
    mysqladmin ping --protocol=tcp -h 127.0.0.1 -uroot --silent >/dev/null 2>&1; then
    break
  fi
  if [[ "${attempt}" -eq 60 ]]; then
    echo "isolated migration MySQL did not become ready" >&2
    exit 1
  fi
  sleep 2
done

docker exec -e "MYSQL_PWD=${PASSWORD}" "${CONTAINER}" \
  mysql --protocol=tcp -h 127.0.0.1 -uroot \
  -e "CREATE DATABASE ${DATABASE};"

cat > "${OUTPUT_DIR}/invalid/00001_invalid.sql" <<'SQL'
-- +goose Up
THIS IS DELIBERATELY INVALID SQL;
CREATE TABLE should_never_exist (id BIGINT PRIMARY KEY);

-- +goose Down
DROP TABLE IF EXISTS should_never_exist;
SQL

DSN="root:${PASSWORD}@tcp(127.0.0.1:3306)/${DATABASE}?parseTime=true&multiStatements=true"
started_ns="$(date +%s%N)"
if docker exec -i -e "MYSQL_PWD=${PASSWORD}" "${CONTAINER}" sh -c \
  'cat > /tmp/00001_invalid.sql && mkdir -p /tmp/invalid && mv /tmp/00001_invalid.sql /tmp/invalid/' \
  < "${OUTPUT_DIR}/invalid/00001_invalid.sql"; then
  :
else
  echo "failed to copy invalid migration fixture" >&2
  exit 1
fi

if docker exec -e "MYSQL_PWD=${PASSWORD}" "${CONTAINER}" sh -c \
  "goose -dir /tmp/invalid mysql '${DSN}' up" \
  > "${OUTPUT_DIR}/invalid-migration.log" 2>&1; then
  echo "deliberately invalid migration unexpectedly succeeded" >&2
  exit 1
fi
failed_ns="$(date +%s%N)"

table_count="$(docker exec -e "MYSQL_PWD=${PASSWORD}" "${CONTAINER}" \
  mysql --protocol=tcp -h 127.0.0.1 -uroot --batch --skip-column-names \
  -e "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='${DATABASE}' AND table_name='should_never_exist';")"
test "${table_count}" = "0"
test ! -e "${OUTPUT_DIR}/promotion-approved"

for directory in migrations migrations/identity migrations/catalog migrations/inventory migrations/ordering; do
  goose -dir "${directory}" validate >> "${OUTPUT_DIR}/normal-migrations.log"
done

failure_duration_ms="$(( (failed_ns - started_ns) / 1000000 ))"
python3 - "${OUTPUT_DIR}/migration-failure.json" "${failure_duration_ms}" <<'PY'
import json
import pathlib
import sys

path = pathlib.Path(sys.argv[1])
duration = int(sys.argv[2])
path.write_text(json.dumps({
    "schema_version": 1,
    "fault": "invalid_sql_migration",
    "failure_detected": True,
    "failure_duration_ms": duration,
    "promotion_continued": False,
    "invalid_table_created": False,
    "normal_migration_directories_valid": True,
    "isolated_database": "fault_migration",
}, indent=2, sort_keys=True) + "\n", encoding="utf-8")
PY

printf 'Invalid migration failed in isolation; promotion remained blocked and normal migrations still validate.\n'
