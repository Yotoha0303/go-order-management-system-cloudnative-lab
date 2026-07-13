#!/usr/bin/env bash
set -euo pipefail

OUTPUT_DIR="${1:-backup-evidence}"
SOURCE_CONTAINER="${SOURCE_MYSQL_CONTAINER:-go-order-management-system-mysql}"
RESTORE_CONTAINER="${RESTORE_MYSQL_CONTAINER:-go-order-backup-restore-${GITHUB_RUN_ID:-local}}"
SOURCE_PASSWORD="${MYSQL_PASSWORD:?MYSQL_PASSWORD is required}"
RESTORE_PASSWORD="${RESTORE_MYSQL_PASSWORD:-isolated-restore-password}"

DATABASES=(
  go_order_identity
  go_order_catalog
  go_order_inventory
  go_order_ordering
)

mkdir -p "${OUTPUT_DIR}/dumps" "${OUTPUT_DIR}/source-after" "${OUTPUT_DIR}/restored"

cleanup() {
  docker rm -fv "${RESTORE_CONTAINER}" >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup

require_container() {
  if ! docker inspect "${1}" >/dev/null 2>&1; then
    echo "required container is not available: ${1}" >&2
    exit 1
  fi
}
require_container "${SOURCE_CONTAINER}"

mysql_exec() {
  local container="$1"
  local password="$2"
  shift 2
  docker exec -e "MYSQL_PWD=${password}" "${container}" mysql \
    --protocol=tcp -h 127.0.0.1 -uroot --batch --skip-column-names "$@"
}

mysql_dump() {
  local container="$1"
  local password="$2"
  local database="$3"
  local destination="$4"
  docker exec -e "MYSQL_PWD=${password}" "${container}" mysqldump \
    --protocol=tcp -h 127.0.0.1 -uroot \
    --single-transaction --quick --routines --triggers --hex-blob \
    --set-gtid-purged=OFF --skip-dump-date --skip-comments --compact \
    --order-by-primary --databases "${database}" > "${destination}"
  test -s "${destination}"
}

backup_started_ns="$(date +%s%N)"
for database in "${DATABASES[@]}"; do
  mysql_dump "${SOURCE_CONTAINER}" "${SOURCE_PASSWORD}" "${database}" \
    "${OUTPUT_DIR}/dumps/${database}.sql"
done
backup_finished_ns="$(date +%s%N)"
backup_duration_ms="$(( (backup_finished_ns - backup_started_ns) / 1000000 ))"

sha256sum "${OUTPUT_DIR}"/dumps/*.sql | sort -k2 > "${OUTPUT_DIR}/source-before.sha256"

docker run -d --name "${RESTORE_CONTAINER}" \
  -e "MYSQL_ROOT_PASSWORD=${RESTORE_PASSWORD}" \
  mysql:8.4 >/dev/null

for attempt in $(seq 1 60); do
  if docker exec -e "MYSQL_PWD=${RESTORE_PASSWORD}" "${RESTORE_CONTAINER}" \
    mysqladmin ping --protocol=tcp -h 127.0.0.1 -uroot --silent >/dev/null 2>&1; then
    break
  fi
  if [[ "${attempt}" -eq 60 ]]; then
    echo "isolated restore MySQL did not become ready" >&2
    exit 1
  fi
  sleep 2
done

restore_started_ns="$(date +%s%N)"
for database in "${DATABASES[@]}"; do
  docker exec -i -e "MYSQL_PWD=${RESTORE_PASSWORD}" "${RESTORE_CONTAINER}" \
    mysql --protocol=tcp -h 127.0.0.1 -uroot < "${OUTPUT_DIR}/dumps/${database}.sql"
done
restore_finished_ns="$(date +%s%N)"
restore_duration_ms="$(( (restore_finished_ns - restore_started_ns) / 1000000 ))"

{
  echo "identity_users=$(mysql_exec "${RESTORE_CONTAINER}" "${RESTORE_PASSWORD}" -e 'SELECT COUNT(*) FROM go_order_identity.users;')"
  echo "identity_migrations=$(mysql_exec "${RESTORE_CONTAINER}" "${RESTORE_PASSWORD}" -e 'SELECT COUNT(*) FROM go_order_identity.goose_db_version WHERE is_applied = 1;')"
  echo "catalog_products=$(mysql_exec "${RESTORE_CONTAINER}" "${RESTORE_PASSWORD}" -e 'SELECT COUNT(*) FROM go_order_catalog.catalog_products;')"
  echo "catalog_migrations=$(mysql_exec "${RESTORE_CONTAINER}" "${RESTORE_PASSWORD}" -e 'SELECT COUNT(*) FROM go_order_catalog.goose_db_version WHERE is_applied = 1;')"
  echo "inventory_items=$(mysql_exec "${RESTORE_CONTAINER}" "${RESTORE_PASSWORD}" -e 'SELECT COUNT(*) FROM go_order_inventory.inventory_items;')"
  echo "inventory_stock_logs=$(mysql_exec "${RESTORE_CONTAINER}" "${RESTORE_PASSWORD}" -e 'SELECT COUNT(*) FROM go_order_inventory.inventory_stock_logs;')"
  echo "inventory_migrations=$(mysql_exec "${RESTORE_CONTAINER}" "${RESTORE_PASSWORD}" -e 'SELECT COUNT(*) FROM go_order_inventory.goose_db_version WHERE is_applied = 1;')"
  echo "orders=$(mysql_exec "${RESTORE_CONTAINER}" "${RESTORE_PASSWORD}" -e 'SELECT COUNT(*) FROM go_order_ordering.orders_v2;')"
  echo "order_items=$(mysql_exec "${RESTORE_CONTAINER}" "${RESTORE_PASSWORD}" -e 'SELECT COUNT(*) FROM go_order_ordering.order_items_v2;')"
  echo "ordering_migrations=$(mysql_exec "${RESTORE_CONTAINER}" "${RESTORE_PASSWORD}" -e 'SELECT COUNT(*) FROM go_order_ordering.goose_db_version WHERE is_applied = 1;')"
} | tee "${OUTPUT_DIR}/restore-verification.txt"

read_value() {
  awk -F= -v key="$1" '$1 == key {print $2}' "${OUTPUT_DIR}/restore-verification.txt"
}

test "$(read_value identity_users)" -ge 2
test "$(read_value identity_migrations)" -ge 1
test "$(read_value catalog_products)" -ge 1
test "$(read_value catalog_migrations)" -ge 1
test "$(read_value inventory_items)" -ge 1
test "$(read_value inventory_stock_logs)" -ge 1
test "$(read_value inventory_migrations)" -ge 1
test "$(read_value orders)" -ge 3
test "$(read_value order_items)" -ge 3
test "$(read_value ordering_migrations)" -ge 1

for database in "${DATABASES[@]}"; do
  mysql_dump "${RESTORE_CONTAINER}" "${RESTORE_PASSWORD}" "${database}" \
    "${OUTPUT_DIR}/restored/${database}.sql"
  cmp "${OUTPUT_DIR}/dumps/${database}.sql" "${OUTPUT_DIR}/restored/${database}.sql"
  mysql_dump "${SOURCE_CONTAINER}" "${SOURCE_PASSWORD}" "${database}" \
    "${OUTPUT_DIR}/source-after/${database}.sql"
  cmp "${OUTPUT_DIR}/dumps/${database}.sql" "${OUTPUT_DIR}/source-after/${database}.sql"
done

sha256sum "${OUTPUT_DIR}"/restored/*.sql | sort -k2 > "${OUTPUT_DIR}/restored.sha256"
sha256sum "${OUTPUT_DIR}"/source-after/*.sql | sort -k2 > "${OUTPUT_DIR}/source-after.sha256"

mysql_version="$(docker exec "${SOURCE_CONTAINER}" mysqldump --version | tr '\n' ' ')"
cat > "${OUTPUT_DIR}/timings.env" <<EOF
BACKUP_DURATION_MS=${backup_duration_ms}
RESTORE_DURATION_MS=${restore_duration_ms}
MYSQL_VERSION=${mysql_version}
EOF

printf 'backup_duration_ms=%s\nrestore_duration_ms=%s\n' \
  "${backup_duration_ms}" "${restore_duration_ms}"
printf 'All four logical dumps restored exactly and the source databases remained unchanged.\n'
