#!/usr/bin/env sh
set -eu

BASE_URL="${BASE_URL:-http://127.0.0.1:8082}"
MYSQL_PASSWORD="${MYSQL_PASSWORD:?MYSQL_PASSWORD is required}"
IDENTITY_DB_NAME="${IDENTITY_DB_NAME:-go_order_identity}"
SMOKE_RUNTIME="${SMOKE_RUNTIME:-compose}"
KUBERNETES_NAMESPACE="${KUBERNETES_NAMESPACE:-go-order-system}"
SMOKE_RUN_SUFFIX="${SMOKE_RUN_SUFFIX:-}"

suffix=""
if [ -n "$SMOKE_RUN_SUFFIX" ]; then
  suffix="-$SMOKE_RUN_SUFFIX"
fi
admin_username="ci-admin${suffix}"
buyer_username="ci-buyer${suffix}"
paid_key="ci-order-pay${suffix}"
cancel_key="ci-order-cancel${suffix}"
timeout_key="ci-order-timeout${suffix}"

json_get() {
  python3 -c '
import json
import sys

value = json.load(sys.stdin)
for part in sys.argv[1].split("."):
    value = value[int(part)] if isinstance(value, list) else value[part]
if isinstance(value, bool):
    print(str(value).lower())
elif value is None:
    print("")
else:
    print(value)
' "$1"
}

assert_eq() {
  expected="$1"
  actual="$2"
  message="$3"
  if [ "$expected" != "$actual" ]; then
    printf 'assertion failed: %s; expected=%s actual=%s\n' "$message" "$expected" "$actual" >&2
    exit 1
  fi
}

request() {
  method="$1"
  path="$2"
  token="${3:-}"
  body="${4:-}"

  set -- --fail-with-body --silent --show-error --request "$method" "${BASE_URL}${path}"
  if [ -n "$token" ]; then
    set -- "$@" --header "Authorization: Bearer ${token}"
  fi
  if [ -n "$body" ]; then
    set -- "$@" --header "Content-Type: application/json" --data "$body"
  fi
  curl "$@"
}

register_user() {
  username="$1"
  password="$2"
  request POST /api/v1/auth/register "" "{\"username\":\"${username}\",\"password\":\"${password}\"}" >/dev/null
}

login_user() {
  username="$1"
  password="$2"
  response="$(request POST /api/v1/auth/login "" "{\"username\":\"${username}\",\"password\":\"${password}\"}")"
  printf '%s' "$response" | json_get data.access_token
}

inventory_available() {
  response="$(request GET "/api/v1/inventory/products/$1" "$2")"
  printf '%s' "$response" | json_get data.available_quantity
}

inventory_reserved() {
  response="$(request GET "/api/v1/inventory/products/$1" "$2")"
  printf '%s' "$response" | json_get data.reserved_quantity
}

order_status() {
  response="$(request GET "/api/v1/orders/$1" "$2")"
  printf '%s' "$response" | json_get data.status
}

promote_admin() {
  username="$1"
  sql="
    UPDATE user_roles ur
    JOIN users u ON u.id = ur.user_id
    JOIN roles r ON r.role_name = 'admin'
    SET ur.role_id = r.id
    WHERE u.username = '${username}';
  "

  case "$SMOKE_RUNTIME" in
    compose)
      docker compose exec -T mysql mysql \
        -uroot \
        -p"${MYSQL_PASSWORD}" \
        "${IDENTITY_DB_NAME}" \
        --batch --skip-column-names \
        --execute="$sql"
      ;;
    kubernetes)
      mysql_pod="$(kubectl -n "$KUBERNETES_NAMESPACE" get pods \
        -l app.kubernetes.io/name=mysql \
        -o jsonpath='{.items[0].metadata.name}')"
      [ -n "$mysql_pod" ] || {
        printf '%s\n' 'MySQL Pod was not found' >&2
        exit 1
      }
      kubectl -n "$KUBERNETES_NAMESPACE" exec "$mysql_pod" -- mysql \
        -uroot \
        -p"${MYSQL_PASSWORD}" \
        "${IDENTITY_DB_NAME}" \
        --batch --skip-column-names \
        --execute="$sql"
      ;;
    *)
      printf 'unsupported SMOKE_RUNTIME: %s\n' "$SMOKE_RUNTIME" >&2
      exit 1
      ;;
  esac
}

printf '%s\n' '1. Register and promote the smoke-test administrator'
register_user "$admin_username" 'CiAdmin123!'
promote_admin "$admin_username"

admin_token="$(login_user "$admin_username" 'CiAdmin123!')"
[ -n "$admin_token" ] || { printf '%s\n' 'administrator login returned an empty token' >&2; exit 1; }

printf '%s\n' '2. Create a Catalog product and initialize Inventory'
product_response="$(request POST /api/v1/products "$admin_token" '{"name":"Saga Smoke Product","description":"cross-service consistency smoke test","price_fen":1250}')"
product_id="$(printf '%s' "$product_response" | json_get data.id)"
request PATCH "/api/v1/products/${product_id}/on-sale" "$admin_token" >/dev/null
request POST /api/v1/inventory/init "$admin_token" "{\"product_id\":${product_id},\"quantity\":10}" >/dev/null
assert_eq 10 "$(inventory_available "$product_id" "$admin_token")" 'initial available stock'
assert_eq 0 "$(inventory_reserved "$product_id" "$admin_token")" 'initial reserved stock'

printf '%s\n' '3. Register a buyer and verify idempotent reservation'
register_user "$buyer_username" 'CiBuyer123!'
buyer_token="$(login_user "$buyer_username" 'CiBuyer123!')"
[ -n "$buyer_token" ] || { printf '%s\n' 'buyer login returned an empty token' >&2; exit 1; }

paid_order_body="{\"idempotency_key\":\"${paid_key}\",\"items\":[{\"product_id\":${product_id},\"quantity\":2}]}"
paid_order_response="$(request POST /api/v1/orders "$buyer_token" "$paid_order_body")"
paid_order_id="$(printf '%s' "$paid_order_response" | json_get data.id)"
assert_eq pending "$(printf '%s' "$paid_order_response" | json_get data.status)" 'new order status'

retry_response="$(request POST /api/v1/orders "$buyer_token" "$paid_order_body")"
assert_eq "$paid_order_id" "$(printf '%s' "$retry_response" | json_get data.id)" 'idempotent order id'
assert_eq 8 "$(inventory_available "$product_id" "$buyer_token")" 'available stock after one reservation'
assert_eq 2 "$(inventory_reserved "$product_id" "$buyer_token")" 'reserved stock after one reservation'

printf '%s\n' '4. Pay the order and confirm the reservation'
pay_response="$(request PATCH "/api/v1/orders/${paid_order_id}/pay" "$buyer_token")"
assert_eq paid "$(printf '%s' "$pay_response" | json_get data.status)" 'paid order status'
assert_eq 8 "$(inventory_available "$product_id" "$buyer_token")" 'available stock after payment'
assert_eq 0 "$(inventory_reserved "$product_id" "$buyer_token")" 'reserved stock after payment confirmation'

printf '%s\n' '5. Cancel another order and release its reservation'
cancel_body="{\"idempotency_key\":\"${cancel_key}\",\"items\":[{\"product_id\":${product_id},\"quantity\":3}]}"
cancel_response="$(request POST /api/v1/orders "$buyer_token" "$cancel_body")"
cancel_order_id="$(printf '%s' "$cancel_response" | json_get data.id)"
assert_eq 5 "$(inventory_available "$product_id" "$buyer_token")" 'available stock while cancel order is reserved'
assert_eq 3 "$(inventory_reserved "$product_id" "$buyer_token")" 'reserved stock before cancellation'

cancel_response="$(request PATCH "/api/v1/orders/${cancel_order_id}/cancel" "$buyer_token")"
assert_eq cancelled "$(printf '%s' "$cancel_response" | json_get data.status)" 'cancelled order status'
assert_eq 8 "$(inventory_available "$product_id" "$buyer_token")" 'available stock after cancellation'
assert_eq 0 "$(inventory_reserved "$product_id" "$buyer_token")" 'reserved stock after cancellation'

printf '%s\n' '6. Verify RabbitMQ timeout cancellation and compensation'
timeout_body="{\"idempotency_key\":\"${timeout_key}\",\"items\":[{\"product_id\":${product_id},\"quantity\":1}]}"
timeout_response="$(request POST /api/v1/orders "$buyer_token" "$timeout_body")"
timeout_order_id="$(printf '%s' "$timeout_response" | json_get data.id)"
assert_eq 7 "$(inventory_available "$product_id" "$buyer_token")" 'available stock while timeout order is reserved'
assert_eq 1 "$(inventory_reserved "$product_id" "$buyer_token")" 'reserved stock before timeout'

timeout_status="pending"
attempt=0
while [ "$attempt" -lt 45 ]; do
  timeout_status="$(order_status "$timeout_order_id" "$buyer_token")"
  if [ "$timeout_status" = "cancelled" ]; then
    break
  fi
  attempt=$((attempt + 1))
  sleep 1
done
assert_eq cancelled "$timeout_status" 'timeout order status'
assert_eq 8 "$(inventory_available "$product_id" "$buyer_token")" 'available stock after timeout release'
assert_eq 0 "$(inventory_reserved "$product_id" "$buyer_token")" 'reserved stock after timeout release'

printf '%s\n' 'microservices Saga smoke test passed'
