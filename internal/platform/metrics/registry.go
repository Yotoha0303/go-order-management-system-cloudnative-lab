package metrics

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const contentType = "text/plain; version=0.0.4; charset=utf-8"

var (
	metricNamePattern = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)
	labelNamePattern  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
)

type Labels map[string]string

type Collector struct {
	Name    string
	Collect func(context.Context, *Registry) error
}

type metricKind string

const (
	counterKind   metricKind = "counter"
	gaugeKind     metricKind = "gauge"
	histogramKind metricKind = "histogram"
)

type labelPair struct {
	name  string
	value string
}

type sample struct {
	labels       []labelPair
	value        float64
	histogramSum float64
	histogramN   uint64
	buckets      []uint64
}

type family struct {
	name    string
	help    string
	kind    metricKind
	bounds  []float64
	samples map[string]*sample
}

type Registry struct {
	mu       sync.RWMutex
	families map[string]*family
}

func NewRegistry() *Registry {
	return &Registry{families: make(map[string]*family)}
}

var Default = NewRegistry()

func (registry *Registry) AddCounter(name, help string, labels Labels, delta float64) {
	if delta < 0 || math.IsNaN(delta) || math.IsInf(delta, 0) {
		panic("counter delta must be finite and non-negative")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	metric := registry.familyLocked(name, help, counterKind, nil)
	entry := metric.sampleLocked(labels)
	entry.value += delta
}

func (registry *Registry) IncCounter(name, help string, labels Labels) {
	registry.AddCounter(name, help, labels, 1)
}

func (registry *Registry) SetGauge(name, help string, labels Labels, value float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		panic("gauge value must be finite")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	metric := registry.familyLocked(name, help, gaugeKind, nil)
	metric.sampleLocked(labels).value = value
}

func (registry *Registry) ObserveHistogram(name, help string, labels Labels, value float64, bounds []float64) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		panic("histogram observation must be finite")
	}
	normalized := normalizeBounds(bounds)
	registry.mu.Lock()
	defer registry.mu.Unlock()
	metric := registry.familyLocked(name, help, histogramKind, normalized)
	entry := metric.sampleLocked(labels)
	entry.histogramN++
	entry.histogramSum += value
	for index, bound := range metric.bounds {
		if value <= bound {
			entry.buckets[index]++
		}
	}
}

func (registry *Registry) Handler(collectors ...Collector) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		for _, collector := range collectors {
			if collector.Collect == nil {
				continue
			}
			if err := collector.Collect(request.Context(), registry); err != nil {
				name := strings.TrimSpace(collector.Name)
				if name == "" {
					name = "unknown"
				}
				registry.IncCounter(
					"go_order_metrics_collection_errors_total",
					"Total metric collection errors by collector.",
					Labels{"collector": name},
				)
			}
		}
		writer.Header().Set("Content-Type", contentType)
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write(registry.Gather())
	})
}

func (registry *Registry) Gather() []byte {
	registry.mu.RLock()
	defer registry.mu.RUnlock()

	names := make([]string, 0, len(registry.families))
	for name := range registry.families {
		names = append(names, name)
	}
	sort.Strings(names)

	var buffer bytes.Buffer
	for _, name := range names {
		metric := registry.families[name]
		fmt.Fprintf(&buffer, "# HELP %s %s\n", metric.name, escapeHelp(metric.help))
		fmt.Fprintf(&buffer, "# TYPE %s %s\n", metric.name, metric.kind)

		keys := make([]string, 0, len(metric.samples))
		for key := range metric.samples {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			entry := metric.samples[key]
			switch metric.kind {
			case histogramKind:
				for index, bound := range metric.bounds {
					writeSample(&buffer, metric.name+"_bucket", appendLabel(entry.labels, "le", formatFloat(bound)), float64(entry.buckets[index]))
				}
				writeSample(&buffer, metric.name+"_bucket", appendLabel(entry.labels, "le", "+Inf"), float64(entry.histogramN))
				writeSample(&buffer, metric.name+"_sum", entry.labels, entry.histogramSum)
				writeSample(&buffer, metric.name+"_count", entry.labels, float64(entry.histogramN))
			default:
				writeSample(&buffer, metric.name, entry.labels, entry.value)
			}
		}
	}
	return buffer.Bytes()
}

func (registry *Registry) familyLocked(name, help string, kind metricKind, bounds []float64) *family {
	validateMetricName(name)
	if strings.TrimSpace(help) == "" {
		panic("metric help is required")
	}
	if registry.families == nil {
		registry.families = make(map[string]*family)
	}
	metric, ok := registry.families[name]
	if !ok {
		metric = &family{name: name, help: help, kind: kind, bounds: append([]float64(nil), bounds...), samples: make(map[string]*sample)}
		registry.families[name] = metric
		return metric
	}
	if metric.kind != kind || metric.help != help || !equalBounds(metric.bounds, bounds) {
		panic("metric family definition conflicts with existing registration: " + name)
	}
	return metric
}

func (metric *family) sampleLocked(labels Labels) *sample {
	pairs, key := normalizeLabels(labels)
	entry, ok := metric.samples[key]
	if !ok {
		entry = &sample{labels: pairs}
		if metric.kind == histogramKind {
			entry.buckets = make([]uint64, len(metric.bounds))
		}
		metric.samples[key] = entry
	}
	return entry
}

func normalizeLabels(labels Labels) ([]labelPair, string) {
	keys := make([]string, 0, len(labels))
	for name := range labels {
		validateLabelName(name)
		keys = append(keys, name)
	}
	sort.Strings(keys)
	pairs := make([]labelPair, 0, len(keys))
	var key strings.Builder
	for _, name := range keys {
		value := labels[name]
		pairs = append(pairs, labelPair{name: name, value: value})
		fmt.Fprintf(&key, "%d:%s=%d:%s;", len(name), name, len(value), value)
	}
	return pairs, key.String()
}

func normalizeBounds(bounds []float64) []float64 {
	if len(bounds) == 0 {
		panic("histogram bounds are required")
	}
	normalized := append([]float64(nil), bounds...)
	sort.Float64s(normalized)
	result := normalized[:0]
	for _, value := range normalized {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			panic("histogram bounds must be finite")
		}
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func equalBounds(left, right []float64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func appendLabel(labels []labelPair, name, value string) []labelPair {
	result := make([]labelPair, 0, len(labels)+1)
	result = append(result, labels...)
	result = append(result, labelPair{name: name, value: value})
	sort.Slice(result, func(left, right int) bool { return result[left].name < result[right].name })
	return result
}

func writeSample(buffer *bytes.Buffer, name string, labels []labelPair, value float64) {
	buffer.WriteString(name)
	if len(labels) > 0 {
		buffer.WriteByte('{')
		for index, pair := range labels {
			if index > 0 {
				buffer.WriteByte(',')
			}
			fmt.Fprintf(buffer, `%s="%s"`, pair.name, escapeLabelValue(pair.value))
		}
		buffer.WriteByte('}')
	}
	buffer.WriteByte(' ')
	buffer.WriteString(formatFloat(value))
	buffer.WriteByte('\n')
}

func validateMetricName(name string) {
	if !metricNamePattern.MatchString(name) {
		panic("invalid metric name: " + name)
	}
}

func validateLabelName(name string) {
	if !labelNamePattern.MatchString(name) || strings.HasPrefix(name, "__") {
		panic("invalid metric label name: " + name)
	}
}

func escapeHelp(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n").Replace(value)
}

func escapeLabelValue(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\"", "\\\"").Replace(value)
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'g', -1, 64)
}
