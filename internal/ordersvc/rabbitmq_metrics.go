package ordersvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	platformmetrics "go-order-management-system/internal/platform/metrics"
)

const (
	rabbitMQSessionMetric = "go_order_rabbitmq_session_up"
	rabbitMQDeliveryMetric = "go_order_rabbitmq_delivery_total"
)

var rabbitMQQueues = []struct {
	role string
	name string
}{
	{role: "delay", name: timeoutDelayQueue},
	{role: "cancel", name: timeoutCancelQueue},
}

func setRabbitMQSessionUp(up bool) {
	value := 0.0
	if up {
		value = 1
	}
	platformmetrics.Default.SetGauge(
		rabbitMQSessionMetric,
		"Whether the Timeout Worker has an active RabbitMQ consumer and publisher session.",
		platformmetrics.Labels{"role": "timeout_worker"},
		value,
	)
}

func recordRabbitMQDelivery(outcome string) {
	platformmetrics.Default.IncCounter(
		rabbitMQDeliveryMetric,
		"Total RabbitMQ timeout-delivery outcomes owned by the application.",
		platformmetrics.Labels{"outcome": outcome},
	)
}

type rabbitMQQueueSnapshot struct {
	Messages               int64 `json:"messages"`
	MessagesReady          int64 `json:"messages_ready"`
	MessagesUnacknowledged int64 `json:"messages_unacknowledged"`
	Consumers              int64 `json:"consumers"`
}

type rabbitMQManagementCollector struct {
	baseURL  string
	username string
	password string
	vhost    string
	client   *http.Client
}

func RabbitMQManagementPrometheusCollector(baseURL, amqpURL string, timeout time.Duration) (platformmetrics.Collector, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return platformmetrics.Collector{}, errors.New("RabbitMQ management URL is required")
	}
	parsedManagementURL, err := url.Parse(baseURL)
	if err != nil || parsedManagementURL.Scheme == "" || parsedManagementURL.Host == "" {
		return platformmetrics.Collector{}, fmt.Errorf("invalid RabbitMQ management URL %q", baseURL)
	}

	username, password, vhost, err := rabbitMQManagementTarget(amqpURL)
	if err != nil {
		return platformmetrics.Collector{}, err
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	collector := &rabbitMQManagementCollector{
		baseURL:  baseURL,
		username: username,
		password: password,
		vhost:    vhost,
		client:   &http.Client{Timeout: timeout},
	}
	return platformmetrics.Collector{Name: "rabbitmq_management", Collect: collector.collect}, nil
}

func rabbitMQManagementTarget(amqpURL string) (string, string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(amqpURL))
	if err != nil || (parsed.Scheme != "amqp" && parsed.Scheme != "amqps") || parsed.Host == "" {
		return "", "", "", errors.New("invalid RabbitMQ AMQP URL")
	}
	if parsed.User == nil || parsed.User.Username() == "" {
		return "", "", "", errors.New("RabbitMQ AMQP URL must include a username")
	}
	password, ok := parsed.User.Password()
	if !ok || password == "" {
		return "", "", "", errors.New("RabbitMQ AMQP URL must include a password")
	}

	escapedVhost := strings.TrimPrefix(parsed.EscapedPath(), "/")
	vhost := "/"
	if escapedVhost != "" {
		vhost, err = url.PathUnescape(escapedVhost)
		if err != nil || vhost == "" {
			return "", "", "", errors.New("RabbitMQ AMQP URL contains an invalid virtual host")
		}
	}
	return parsed.User.Username(), password, vhost, nil
}

func (collector *rabbitMQManagementCollector) collect(ctx context.Context, registry *platformmetrics.Registry) error {
	registry.SetGauge(
		"go_order_rabbitmq_management_up",
		"Whether the RabbitMQ management API collector completed successfully.",
		nil,
		0,
	)
	for _, queue := range rabbitMQQueues {
		snapshot, err := collector.fetchQueue(ctx, queue.name)
		if err != nil {
			return err
		}
		for state, value := range map[string]int64{
			"total":          snapshot.Messages,
			"ready":          snapshot.MessagesReady,
			"unacknowledged": snapshot.MessagesUnacknowledged,
		} {
			registry.SetGauge(
				"go_order_rabbitmq_queue_messages",
				"Current bounded RabbitMQ queue message count by queue role and state.",
				platformmetrics.Labels{"queue_role": queue.role, "state": state},
				float64(value),
			)
		}
		registry.SetGauge(
			"go_order_rabbitmq_queue_consumers",
			"Current RabbitMQ consumer count by bounded queue role.",
			platformmetrics.Labels{"queue_role": queue.role},
			float64(snapshot.Consumers),
		)
	}
	registry.SetGauge(
		"go_order_rabbitmq_management_up",
		"Whether the RabbitMQ management API collector completed successfully.",
		nil,
		1,
	)
	return nil
}

func (collector *rabbitMQManagementCollector) fetchQueue(ctx context.Context, queueName string) (rabbitMQQueueSnapshot, error) {
	endpoint := collector.baseURL + "/api/queues/" + url.PathEscape(collector.vhost) + "/" + url.PathEscape(queueName)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return rabbitMQQueueSnapshot{}, fmt.Errorf("create RabbitMQ management request: %w", err)
	}
	request.SetBasicAuth(collector.username, collector.password)
	response, err := collector.client.Do(request)
	if err != nil {
		return rabbitMQQueueSnapshot{}, fmt.Errorf("query RabbitMQ management API: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return rabbitMQQueueSnapshot{}, fmt.Errorf("RabbitMQ management API returned HTTP %d", response.StatusCode)
	}
	var snapshot rabbitMQQueueSnapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		return rabbitMQQueueSnapshot{}, fmt.Errorf("decode RabbitMQ queue snapshot: %w", err)
	}
	return snapshot, nil
}
