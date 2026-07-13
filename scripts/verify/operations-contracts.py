#!/usr/bin/env python3

import pathlib

ROOT = pathlib.Path(__file__).resolve().parents[2]
RUNTIME = ROOT / ".github/workflows/load-test.yml"
CONTRACTS = ROOT / ".github/workflows/operations-contracts.yml"
DRIVER = ROOT / "scripts/load/order_create_load.py"
DRIVER_TEST = ROOT / "scripts/load/test_order_create_load.py"
SAMPLER = ROOT / "scripts/load/resource_sampler.py"
SAMPLER_TEST = ROOT / "scripts/load/test_resource_sampler.py"
ANALYZER = ROOT / "scripts/load/analyze_load.py"
ANALYZER_TEST = ROOT / "scripts/load/test_analyze_load.py"
CAPTURE = ROOT / "scripts/load/capture_state.sh"
RUNBOOK = ROOT / "docs/runbooks/operations.md"
LOAD_DOC = ROOT / "docs/verification/load-test.md"

EXPECTED_PROMETHEUS_TARGETS = {
    "api-gateway": 1,
    "identity-service": 1,
    "catalog-service": 1,
    "inventory-service": 1,
    "order-service": 1,
    "order-timeout-worker": 2,
    "order-reconciliation-worker": 2,
}


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)


def read(path: pathlib.Path) -> str:
    require(path.is_file(), f"required operations file is missing: {path.relative_to(ROOT)}")
    return path.read_text(encoding="utf-8")


def verify_runtime_workflow() -> None:
    workflow = read(RUNTIME)
    require("workflow_dispatch:" in workflow, "manual bounded load entry is missing")
    require("pull_request:" not in workflow, "pull requests must not execute runtime load")
    require("permissions:\n  contents: read" in workflow, "top-level load permissions must remain read-only")
    require(workflow.count("issues: write") == 1, "issue write permission must be isolated to evidence")
    require("timeout-minutes: 30" in workflow, "load workflow must have a bounded timeout")
    require('GATEWAY_RATE_LIMIT_PER_CLIENT_RPS: "5000"' in workflow, "disposable rate-limit override is missing")
    require("ORDER_TIMEOUT_DELAY: 10m" in workflow, "load orders must not time out during measurement")
    require("Create synthetic load identities" in workflow, "synthetic identity setup is missing")
    require("--prepare-fixture" in workflow, "in-memory fixture preparation is missing")
    require('--concurrency-levels "1,4,8,16,32"' in workflow, "five bounded concurrency levels are missing")
    require("--warmup-seconds 5" in workflow, "warm-up duration is missing")
    require("--stage-seconds 8" in workflow, "measured stage duration is missing")
    require("--max-requests-per-stage 3000" in workflow, "independent per-stage request ceiling is missing")
    require("max_total_measured_requests" in workflow and '"15000"' in workflow, "total measured-request ceiling is not asserted")
    require("Wait for nine healthy Prometheus targets across seven jobs" in workflow, "nine-target readiness gate is missing")
    require("from collections import Counter" in workflow, "Prometheus readiness must count individual targets")
    require("activeTargets" in workflow and 'target.get("health") == "up"' in workflow, "Prometheus readiness must inspect active healthy targets")
    require("active_counts" in workflow and "healthy_counts" in workflow and "mismatches" in workflow, "Prometheus target-count comparison is incomplete")
    for job, expected_count in EXPECTED_PROMETHEUS_TARGETS.items():
        require(f'"{job}": {expected_count}' in workflow, f"Prometheus readiness count is wrong or missing for {job}")
    require("resource_sampler.py" in workflow, "resource sampler is not executed")
    require("--start-file" in workflow and "load-evidence/measurement-start" in workflow, "measured-stage start marker is missing")
    require("--measurement-start-file" in workflow, "load driver does not publish the measurement-start marker")
    require("--stop-file" in workflow and "load-evidence/load-complete" in workflow, "resource sampling completion marker is missing")
    require("--start-timeout-seconds 60" in workflow, "resource sampler start wait is not bounded")
    require("--max-duration-seconds 180" in workflow, "resource sampler hard safety timeout is missing")
    require('touch "${stop_file}"' in workflow, "load driver completion must stop the sampler")
    require("start_reason=measurement_start_file" in workflow, "sampler start outcome is not enforced")
    require("stop_reason=completion_file" in workflow, "sampler completion outcome is not enforced")
    require("order_create_load.py" in workflow, "order load driver is not executed")
    require("analyze_load.py" in workflow, "capacity analyzer is not executed")
    require(workflow.count("capture_state.sh load-evidence") == 2, "baseline and post-load state capture are required")
    require('wait "${sampler_pid}"' in workflow, "resource sampler must be awaited in the same shell lifecycle")
    require("trap cleanup_sampler EXIT" in workflow, "resource sampler cleanup trap is missing")
    require("best_success_rps" in workflow and "best_healthy_successful_throughput_rps" in workflow, "healthy successful throughput output is missing")
    require("highest_healthy_p95_ms" in workflow and "healthy_stage_count" in workflow, "healthy pre-boundary summary fields are missing")
    require("best_rps" not in workflow, "ambiguous attempted-throughput output must not remain")
    require("load-summary.json" in workflow and "samples.jsonl" in read(DRIVER), "raw and summary load evidence is incomplete")
    require("capacity-report.json" in workflow and "capacity-report.md" in workflow, "capacity analysis outputs are missing")
    require("actions/upload-artifact@v4" in workflow, "load evidence artifact is missing")
    require("retention-days: 30" in workflow, "load artifact retention must be explicit")
    require("if: always()" in workflow and "down -v --remove-orphans" in workflow, "disposable load resources must always be removed")
    require('ACCEPTANCE_ISSUE_NUMBER: "52"' in workflow, "Phase 8.5 evidence target is missing")
    require("All nine expected Prometheus targets across seven scrape jobs were healthy" in workflow, "target-health evidence claim is missing")
    require("independent 3000-request safety ceiling" in workflow, "per-stage request-boundary evidence claim is missing")
    require("snapshots overlapping completion are discarded" in workflow, "overlapping snapshot evidence boundary is missing")
    require("no production SLO or capacity guarantee is claimed" in workflow, "non-production evidence boundary is missing")


def verify_contract_workflow() -> None:
    workflow = read(CONTRACTS)
    require("pull_request:" in workflow, "operations contracts must run on pull requests")
    require("permissions:\n  contents: read" in workflow, "operations contracts must be read-only")
    require("issues: write" not in workflow, "operations contracts must not write issues")
    require("packages: read" not in workflow and "packages: write" not in workflow, "operations contracts must not access packages")
    require("python3 -m unittest" in workflow, "load tooling unit tests are not wired")
    require("bash -n scripts/load/capture_state.sh" in workflow, "state-capture shell validation is missing")
    require("scripts/verify/operations-contracts.py" in workflow, "static operations verification is missing")


def verify_load_tooling() -> None:
    driver = read(DRIVER)
    tests = read(DRIVER_TEST)
    sampler = read(SAMPLER)
    sampler_tests = read(SAMPLER_TEST)
    analyzer = read(ANALYZER)
    analyzer_tests = read(ANALYZER_TEST)
    capture = read(CAPTURE)

    require("MAX_CONCURRENCY = 32" in driver, "load concurrency cap is missing")
    require("MAX_STAGE_SECONDS = 15.0" in driver, "stage duration cap is missing")
    require("MAX_REQUESTS_PER_STAGE = 3000" in driver, "per-stage request cap is missing")
    require("max_requests_per_stage" in driver and "max_total_measured_requests" in driver, "per-stage and total request bounds are not recorded")
    require("request_limit=args.max_requests_per_stage" in driver, "each measured stage must receive the independent request ceiling")
    require("stop_reason" in driver and "request_limit" in driver and "measurement_eligible" in driver, "request-cap truncation is not explicitly recorded")
    require("configured_duration_seconds" in driver and "issuance_elapsed_seconds" in driver, "stage-duration evidence is incomplete")
    require("Each measured stage has its own request safety ceiling" in driver, "per-stage ceiling interpretation boundary is missing")
    require("idempotency_key" in driver and "uuid.uuid4" in driver, "unique idempotency keys are missing")
    require("access_token" in driver, "fixture login must follow the Gateway token contract")
    require("price_fen" in driver and "/on-sale" in driver, "Catalog fixture contract is incomplete")
    require('"tokens_persisted": False' in driver, "fixture must state that tokens are not persisted")
    require('"credentials_persisted": False' in driver, "load report must state that credentials are not persisted")
    require("successful_throughput_rps" in driver, "successful throughput is not separated from attempts")
    require("write_measurement_start" in driver and "measurement_started_at" in driver and "measurement_finished_at" in driver, "measured interval markers are incomplete")
    require("latency_ms" in driver and '"p50"' in driver and '"p95"' in driver and '"p99"' in driver, "latency percentiles are incomplete")
    require("throughput_rps" in driver and "error_rate" in driver, "throughput/error metrics are incomplete")
    require("test_validate_args_enforces_per_stage_request_cap" in tests, "per-stage request-cap test is missing")
    require("test_measurement_start_marker_creates_parent_and_timestamp" in tests, "measurement-start regression test is missing")
    require("test_real_local_server_stage_produces_success_samples_without_timing_race" in tests, "non-flaky local HTTP load test is missing")
    require("duration_seconds=2.0" in tests, "local load test duration is too short for busy runners")
    require("MAX_DURATION_SECONDS = 240.0" in sampler, "resource sampling hard cap is missing")
    require("MAX_START_TIMEOUT_SECONDS = 120.0" in sampler, "resource sampling start wait is not bounded")
    require("--start-file" in sampler and "measurement_start_file" in sampler, "sampler must wait for measured stages")
    require("--stop-file" in sampler and "completion_file" in sampler, "sampler must follow the load-driver completion marker")
    require("records = snapshot_func()" in sampler, "sampler must stage a snapshot before deciding whether to persist it")
    require(sampler.count("if stop_file.is_file():") >= 2, "sampler must check completion before collection and before persisting every snapshot")
    require("maximum duration before the load driver completed" in sampler, "premature sampler completion must fail")
    require('["docker", "stats"' in sampler, "Docker resource sampling is missing")
    require("test_sampler_discards_snapshot_overlapping_completion" in sampler_tests, "overlapping completion snapshot regression test is missing")
    require("test_wait_for_measurement_start_rejects_early_completion" in sampler_tests, "early completion/start ordering test is missing")
    require("gateway_rate_limit" in analyzer, "Gateway rate-limit classification is missing")
    require("request_error_boundary" in analyzer, "request error classification is missing")
    require("throughput_plateau_with_tail_growth" in analyzer, "plateau/tail classification is missing")
    require("request_ceiling_before_stage_duration" in analyzer, "request-ceiling safety classification is missing")
    require("not_reached_within_bounded_range" in analyzer, "no-saturation classification is missing")
    require("healthy_stages_before_boundary" in analyzer, "healthy pre-boundary stage selection is missing")
    require("best_healthy_successful_throughput" in analyzer, "healthy successful throughput calculation is missing")
    require("diagnostic lead, not proof" in analyzer, "inference caution is missing")
    require("previous = stage" in analyzer, "boundary candidates must be evaluated chronologically")
    require("test_429_is_classified_as_gateway_rate_limit" in analyzer_tests, "429 analysis test is missing")
    require("test_earlier_plateau_precedes_later_request_errors" in analyzer_tests, "chronological boundary regression test is missing")
    require("test_request_ceiling_is_not_misreported_as_capacity" in analyzer_tests, "request-ceiling interpretation test is missing")
    require("test_best_throughput_uses_successes_and_stops_before_error_boundary" in analyzer_tests, "failed-fast throughput exclusion test is missing")
    require("test_boundary_stage_is_excluded_from_best_healthy_throughput" in analyzer_tests, "boundary-stage exclusion test is missing")
    require("test_no_saturation_is_reported_without_inventing_a_bottleneck" in analyzer_tests, "no-saturation test is missing")
    require("mysql-global-status.tsv" in capture and "mysql-business-state.tsv" in capture, "MySQL evidence capture is incomplete")
    require("rabbitmq-queues.tsv" in capture, "RabbitMQ queue capture is missing")
    require("service,method,route_group,status_class" in capture, "server request evidence must preserve emitted route and status labels")
    require("route,status" not in capture, "obsolete server metric labels must not be queried")
    require("upstream,operation,outcome,status_class,retryable" in capture, "HTTP client evidence must preserve all emitted labels")
    require("max by (job,role) (go_order_rabbitmq_session_up)" in capture, "RabbitMQ session evidence must preserve role and scrape job")
    require("sum by (job,outcome) (go_order_rabbitmq_publish_total)" in capture, "RabbitMQ publish evidence must use actual labels plus scrape job")
    require("sum by (job,outcome) (go_order_rabbitmq_delivery_total)" in capture, "RabbitMQ delivery evidence must use actual labels plus scrape job")
    require("service,operation,outcome" not in capture, "nonexistent RabbitMQ publish labels must not be queried")
    require("go_order_http_server_request_duration_seconds_bucket" in capture, "server latency metric capture is missing")
    require("go_order_rabbitmq_consume_total" not in capture, "nonexistent RabbitMQ consume metric must not be queried")


def verify_documentation() -> None:
    runbook = read(RUNBOOK)
    load_doc = read(LOAD_DOC)
    for phrase in (
        "Architecture quick reference",
        "Evidence-first rule",
        "Immutable release and automatic test delivery",
        "MySQL backup and isolated restore",
        "Incident: RabbitMQ outage",
        "Incident: HTTP timeout or circuit open",
        "Incident: Worker crash while holding a lease",
        "Incident: migration failure",
        "Incident: high latency, errors or saturation",
        "Post-incident review template",
        "Production enhancements intentionally left optional",
    ):
        require(phrase in runbook, f"Runbook is missing section: {phrase}")
    for phrase in (
        "P50/P95/P99",
        "Concurrency levels",
        "Measured request ceiling per stage",
        "Maximum total measured requests",
        "Tokens remain in the Python process memory",
        "nine expected Prometheus targets",
        "two healthy targets",
        "measured stages only",
        "successful throughput",
        "emitted metric labels",
        "RabbitMQ session",
        "Operational evidence",
        "Capacity-boundary rules",
        "measured evidence from inference",
        "Not reached within bounded range",
        "not a production SLO",
    ):
        require(phrase in load_doc, f"load-test documentation is missing: {phrase}")


def main() -> int:
    verify_runtime_workflow()
    verify_contract_workflow()
    verify_load_tooling()
    verify_documentation()
    print("Operator Runbook and bounded load-test contracts verified")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except Exception as exc:
        print(f"Operations contract verification failed: {exc}")
        raise
