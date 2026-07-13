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

mkdir -p \
  "${OUTPUT_DIR}/dumps" \
  "${OUTPUT_DIR}/logical/source-before" \
  "${OUTPUT_DIR}/logical/restored" \
  "${OUTPUT_DIR}/logical/source-after"

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
    --protocol=tcp -h 127.0.0.1 -uroot --batch --raw --skip-column-names "$@"
}

mysql_dump() {
  local container="$1"
  local password="$2"
  local database="$3"
  local destination="$4"
  docker exec -e "MYSQL_PWD=${password}" "${container}" mysqldump \
    --protocol=tcp -h 127.0.0.1 -uroot \
    --single-transaction --quick --routines --triggers --hex-blob \
    --set-gtid-purged=OFF --skip-dump-date --skip-comments \
    --order-by-primary --databases "${database}" > "${destination}"
  test -s "${destination}"
  grep -q 'FOREIGN_KEY_CHECKS=0' "${destination}"
}

query_fingerprint() {
  local container="$1"
  local password="$2"
  local query="$3"
  local destination="$4"
  mysql_exec "${container}" "${password}" -e "${query}" > "${destination}"
}

logical_fingerprint() {
  local container="$1"
  local password="$2"
  local database="$3"
  local destination_dir="$4"
  mkdir -p "${destination_dir}"

  query_fingerprint "${container}" "${password}" \
    "SELECT SCHEMA_NAME, DEFAULT_CHARACTER_SET_NAME, DEFAULT_COLLATION_NAME, DEFAULT_ENCRYPTION FROM information_schema.SCHEMATA WHERE SCHEMA_NAME='${database}';" \
    "${destination_dir}/${database}.schema.tsv"

  query_fingerprint "${container}" "${password}" \
    "SELECT TABLE_NAME, ENGINE, COALESCE(TABLE_COLLATION,''), COALESCE(CREATE_OPTIONS,''), COALESCE(AUTO_INCREMENT,0) FROM information_schema.TABLES WHERE TABLE_SCHEMA='${database}' AND TABLE_TYPE='BASE TABLE' ORDER BY TABLE_NAME;" \
    "${destination_dir}/${database}.tables.tsv"

  query_fingerprint "${container}" "${password}" \
    "SELECT TABLE_NAME, ORDINAL_POSITION, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COALESCE(COLUMN_DEFAULT,'<NULL>'), EXTRA, COALESCE(CHARACTER_SET_NAME,''), COALESCE(COLLATION_NAME,''), COALESCE(GENERATION_EXPRESSION,'') FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='${database}' ORDER BY TABLE_NAME, ORDINAL_POSITION;" \
    "${destination_dir}/${database}.columns.tsv"

  query_fingerprint "${container}" "${password}" \
    "SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, SEQ_IN_INDEX, COALESCE(COLUMN_NAME,''), COALESCE(SUB_PART,0), INDEX_TYPE, COALESCE(COLLATION,''), NULLABLE FROM information_schema.STATISTICS WHERE TABLE_SCHEMA='${database}' ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX;" \
    "${destination_dir}/${database}.indexes.tsv"

  query_fingerprint "${container}" "${password}" \
    "SELECT tc.TABLE_NAME, tc.CONSTRAINT_NAME, tc.CONSTRAINT_TYPE, tc.ENFORCED, COALESCE(kcu.COLUMN_NAME,''), COALESCE(kcu.ORDINAL_POSITION,0), COALESCE(kcu.REFERENCED_TABLE_NAME,''), COALESCE(kcu.REFERENCED_COLUMN_NAME,'') FROM information_schema.TABLE_CONSTRAINTS tc LEFT JOIN information_schema.KEY_COLUMN_USAGE kcu ON tc.CONSTRAINT_SCHEMA=kcu.CONSTRAINT_SCHEMA AND tc.TABLE_NAME=kcu.TABLE_NAME AND tc.CONSTRAINT_NAME=kcu.CONSTRAINT_NAME WHERE tc.CONSTRAINT_SCHEMA='${database}' ORDER BY tc.TABLE_NAME, tc.CONSTRAINT_NAME, kcu.ORDINAL_POSITION;" \
    "${destination_dir}/${database}.constraints.tsv"

  query_fingerprint "${container}" "${password}" \
    "SELECT tc.TABLE_NAME, cc.CONSTRAINT_NAME, HEX(cc.CHECK_CLAUSE), tc.ENFORCED FROM information_schema.CHECK_CONSTRAINTS cc JOIN information_schema.TABLE_CONSTRAINTS tc ON tc.CONSTRAINT_SCHEMA=cc.CONSTRAINT_SCHEMA AND tc.CONSTRAINT_NAME=cc.CONSTRAINT_NAME WHERE cc.CONSTRAINT_SCHEMA='${database}' ORDER BY tc.TABLE_NAME, cc.CONSTRAINT_NAME;" \
    "${destination_dir}/${database}.checks.tsv"

  query_fingerprint "${container}" "${password}" \
    "SELECT CONSTRAINT_NAME, TABLE_NAME, REFERENCED_TABLE_NAME, UPDATE_RULE, DELETE_RULE FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA='${database}' ORDER BY TABLE_NAME, CONSTRAINT_NAME;" \
    "${destination_dir}/${database}.references.tsv"

  query_fingerprint "${container}" "${password}" \
    "SELECT TRIGGER_NAME, EVENT_MANIPULATION, EVENT_OBJECT_TABLE, ACTION_TIMING, ACTION_ORIENTATION, HEX(ACTION_STATEMENT), SQL_MODE, DEFINER, CHARACTER_SET_CLIENT, COLLATION_CONNECTION, DATABASE_COLLATION FROM information_schema.TRIGGERS WHERE TRIGGER_SCHEMA='${database}' ORDER BY TRIGGER_NAME;" \
    "${destination_dir}/${database}.triggers.tsv"

  query_fingerprint "${container}" "${password}" \
    "SELECT ROUTINE_NAME, ROUTINE_TYPE, COALESCE(DTD_IDENTIFIER,''), HEX(COALESCE(ROUTINE_DEFINITION,'')), IS_DETERMINISTIC, SQL_DATA_ACCESS, SECURITY_TYPE, SQL_MODE, HEX(COALESCE(ROUTINE_COMMENT,'')), DEFINER, CHARACTER_SET_CLIENT, COLLATION_CONNECTION, DATABASE_COLLATION FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA='${database}' ORDER BY ROUTINE_TYPE, ROUTINE_NAME;" \
    "${destination_dir}/${database}.routines.tsv"

  query_fingerprint "${container}" "${password}" \
    "SELECT SPECIFIC_NAME, ORDINAL_POSITION, COALESCE(PARAMETER_MODE,''), COALESCE(PARAMETER_NAME,''), COALESCE(DTD_IDENTIFIER,''), COALESCE(CHARACTER_SET_NAME,''), COALESCE(COLLATION_NAME,'') FROM information_schema.PARAMETERS WHERE SPECIFIC_SCHEMA='${database}' ORDER BY SPECIFIC_NAME, ORDINAL_POSITION;" \
    "${destination_dir}/${database}.parameters.tsv"

  docker exec -e "MYSQL_PWD=${password}" "${container}" mysqldump \
    --protocol=tcp -h 127.0.0.1 -uroot \
    --single-transaction --quick --hex-blob --set-gtid-purged=OFF \
    --skip-dump-date --skip-comments --no-create-info --skip-triggers \
    --order-by-primary "${database}" > "${destination_dir}/${database}.data.sql"

  (
    cd "${destination_dir}"
    sha256sum \
      "${database}.schema.tsv" \
      "${database}.tables.tsv" \
      "${database}.columns.tsv" \
      "${database}.indexes.tsv" \
      "${database}.constraints.tsv" \
      "${database}.checks.tsv" \
      "${database}.references.tsv" \
      "${database}.triggers.tsv" \
      "${database}.routines.tsv" \
      "${database}.parameters.tsv" \
      "${database}.data.sql" \
      | sort -k2 > "${database}.fingerprint.sha256"
  )
}

compare_fingerprints() {
  local expected_dir="$1"
  local actual_dir="$2"
  local database="$3"
  local component
  for component in \
    schema.tsv tables.tsv columns.tsv indexes.tsv constraints.tsv checks.tsv references.tsv \
    triggers.tsv routines.tsv parameters.tsv data.sql fingerprint.sha256; do
    cmp \
      "${expected_dir}/${database}.${component}" \
      "${actual_dir}/${database}.${component}"
  done
}

backup_started_ns="$(date +%s%N)"
for database in "${DATABASES[@]}"; do
  mysql_dump "${SOURCE_CONTAINER}" "${SOURCE_PASSWORD}" "${database}" \
    "${OUTPUT_DIR}/dumps/${database}.sql"
  logical_fingerprint "${SOURCE_CONTAINER}" "${SOURCE_PASSWORD}" "${database}" \
    "${OUTPUT_DIR}/logical/source-before"
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
  logical_fingerprint "${RESTORE_CONTAINER}" "${RESTORE_PASSWORD}" "${database}" \
    "${OUTPUT_DIR}/logical/restored"
  logical_fingerprint "${SOURCE_CONTAINER}" "${SOURCE_PASSWORD}" "${database}" \
    "${OUTPUT_DIR}/logical/source-after"
  compare_fingerprints \
    "${OUTPUT_DIR}/logical/source-before" \
    "${OUTPUT_DIR}/logical/restored" \
    "${database}"
  compare_fingerprints \
    "${OUTPUT_DIR}/logical/source-before" \
    "${OUTPUT_DIR}/logical/source-after" \
    "${database}"
done

(
  cd "${OUTPUT_DIR}/logical/restored"
  sha256sum *.fingerprint.sha256 | sort -k2 > "../../restored-logical.sha256"
)
(
  cd "${OUTPUT_DIR}/logical/source-after"
  sha256sum *.fingerprint.sha256 | sort -k2 > "../../source-after-logical.sha256"
)

mysql_version="$(docker exec "${SOURCE_CONTAINER}" mysqldump --version | tr '\n' ' ')"
cat > "${OUTPUT_DIR}/timings.env" <<EOF
BACKUP_DURATION_MS=${backup_duration_ms}
RESTORE_DURATION_MS=${restore_duration_ms}
MYSQL_VERSION=${mysql_version}
EOF

printf 'backup_duration_ms=%s\nrestore_duration_ms=%s\n' \
  "${backup_duration_ms}" "${restore_duration_ms}"
printf 'All four databases matched by complete logical schema and ordered-data fingerprints; source fingerprints remained unchanged.\n'
