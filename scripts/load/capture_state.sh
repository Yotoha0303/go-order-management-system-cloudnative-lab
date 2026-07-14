#!/usr/bin/env bash
set -euo pipefail

OUTPUT_DIR="${1:?output directory is required}"
PHASE="${2:?capture phase is required}"
PROMETHEUS_URL="${PROMETHEUS_URL:-http://127.0.0.1:19090}"
BASE_URL="${BASE_URL:-http://127.0.0.1:8082}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:?MYSQL_PASSWORD is required}"

case "${PHASE}" in
  baseline|post)
    ;;
  *)
    echo "capture phase must be baseline or post" >&2
    exit 1
    ;;
esac

DESTINATION="${OUTPUT_DIR}/${PHASE}"
mkdir -p "${DESTINATION}/prometheus"

curl --fail --silent --show-error "${BASE_URL}/readyz" \
  > "${DESTINATION}/gateway-ready.txt"
docker compose ps -a > "${DESTINATION}/compose-ps.txt"
docker stats --no-stream > "${DESTINATION}/docker-stats.txt"

docker compose exec -T -e "MYSQL_PWD=${MYSQL_PASSWORD}" mysql mysql \
  --protocol=tcp -h 127.0.0.1 -uroot --batch --raw --skip-column-names \
  -e "SHOW GLOBAL STATUS WHERE Variable_name IN ('Threads_connected','Threads_running','Questions','Slow_queries','Innodb_row_lock_waits','Innodb_buffer_pool_reads','Innodb_buffer_pool_read_requests');" \
  > "${DESTINATION}/mysql-global-status.tsv"

docker compose exec -T -e "MYSQL_PWD=${MYSQL_PASSWORD}" mysql mysql \
  --protocol=tcp -h 127.0.0.1 -uroot --batch --raw --skip-column-names <<'SQL' \
  > "${DESTINATION}/mysql-business-state.tsv"
SELECT 'identity_users', COUNT(*) FROM go_order_identity.users;
SELECT 'catalog_products', COUNT(*) FROM go_order_catalog.catalog_products;
SELECT 'inventory_rows', COUNT(*) FROM go_order_inventory.inventory_items;
SELECT 'inventory_available_sum', COALESCE(SUM(available_quantity),0) FROM go_order_inventory.inventory_items;
SELECT 'inventory_reserved_sum', COALESCE(SUM(reserved_quantity),0) FROM go_order_inventory.inventory_items;
SELECT 'orders_total', COUNT(*) FROM go_order_ordering.orders_v2;
SELECT CONCAT('orders_', status), COUNT(*) FROM go_order_ordering.orders_v2 GROUP BY status ORDER BY status;
SELECT CONCAT('timeout_outbox_', status), COUNT(*) FROM go_order_ordering.order_timeout_outbox_v2 GROUP BY status ORDER BY status;
SELECT CONCAT('reconciliation_', status), COUNT(*) FROM go_order_ordering.order_reconciliation_tasks GROUP BY status ORDER BY status;
SQL

docker compose exec -T rabbitmq rabbitmqctl list_queues \
  name messages messages_ready messages_unacknowledged consumers \
  > "${DESTINATION}/rabbitmq-queues.tsv"

queries=(
  'sum by (service,method,route_group,status_class) (go_order_http_server_requests_total)'
  'histogram_quantile(0.95, sum by (le,service,route_group) (rate(go_order_http_server_request_duration_seconds_bucket[1m])))'
  'sum by (upstream,operation,outcome,status_class,retryable) (go_order_http_client_attempts_total)'
  'go_order_rabbitmq_session_up'
  'sum by (job,outcome) (go_order_rabbitmq_publish_total)'
  'sum by (job,outcome) (go_order_rabbitmq_delivery_total)'
  'max by (job,queue_role,state) (go_order_rabbitmq_queue_messages)'
  'max by (job,queue_role) (go_order_rabbitmq_queue_consumers)'
)

index=0
for query in "${queries[@]}"; do
  index=$((index + 1))
  curl --fail --silent --show-error --get \
    --data-urlencode "query=${query}" \
    "${PROMETHEUS_URL}/api/v1/query" \
    > "${DESTINATION}/prometheus/query-${index}.json"
  printf '%s\t%s\n' "${index}" "${query}" \
    >> "${DESTINATION}/prometheus/queries.tsv"
done

printf 'phase=%s\ncaptured_at=%s\n' \
  "${PHASE}" "$(date --utc +%Y-%m-%dT%H:%M:%SZ)" \
  > "${DESTINATION}/capture.env"
