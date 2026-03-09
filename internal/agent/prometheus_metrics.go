package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Fharena/VibeDeck/internal/runtime"
)

func renderPrometheusMetrics(state runtime.ConnectionState, p2pActive bool, ack runtime.AckMetrics, control ControlMetricsSnapshot) string {
	var builder strings.Builder

	builder.WriteString("# HELP vibedeck_runtime_state Current runtime state of the agent.\n")
	builder.WriteString("# TYPE vibedeck_runtime_state gauge\n")
	for _, candidate := range allConnectionStates() {
		value := 0
		if state == candidate {
			value = 1
		}
		fmt.Fprintf(&builder, "vibedeck_runtime_state{state=%q} %d\n", candidate, value)
	}

	builder.WriteString("# HELP vibedeck_p2p_active Whether the direct P2P session is active.\n")
	builder.WriteString("# TYPE vibedeck_p2p_active gauge\n")
	if p2pActive {
		builder.WriteString("vibedeck_p2p_active 1\n")
	} else {
		builder.WriteString("vibedeck_p2p_active 0\n")
	}

	builder.WriteString("# HELP vibedeck_ack_pending Current number of pending ACKs.\n")
	builder.WriteString("# TYPE vibedeck_ack_pending gauge\n")
	fmt.Fprintf(&builder, "vibedeck_ack_pending %d\n", ack.PendingCount)

	builder.WriteString("# HELP vibedeck_ack_pending_max Maximum observed pending ACK queue depth.\n")
	builder.WriteString("# TYPE vibedeck_ack_pending_max gauge\n")
	fmt.Fprintf(&builder, "vibedeck_ack_pending_max %d\n", ack.MaxPendingCount)

	builder.WriteString("# HELP vibedeck_ack_pending_by_transport Current pending ACKs split by transport.\n")
	builder.WriteString("# TYPE vibedeck_ack_pending_by_transport gauge\n")
	for _, transport := range sortedStringKeys(ack.PendingByTransport) {
		fmt.Fprintf(
			&builder,
			"vibedeck_ack_pending_by_transport{transport=%q} %d\n",
			transport,
			ack.PendingByTransport[transport],
		)
	}

	builder.WriteString("# HELP vibedeck_ack_acked_total Total acknowledged control responses.\n")
	builder.WriteString("# TYPE vibedeck_ack_acked_total counter\n")
	fmt.Fprintf(&builder, "vibedeck_ack_acked_total %d\n", ack.AckedCount)

	builder.WriteString("# HELP vibedeck_ack_retry_dispatch_total Total retried control responses dispatched over P2P.\n")
	builder.WriteString("# TYPE vibedeck_ack_retry_dispatch_total counter\n")
	fmt.Fprintf(&builder, "vibedeck_ack_retry_dispatch_total %d\n", ack.RetryDispatchCount)

	builder.WriteString("# HELP vibedeck_ack_expired_total Total ACKs that expired.\n")
	builder.WriteString("# TYPE vibedeck_ack_expired_total counter\n")
	fmt.Fprintf(&builder, "vibedeck_ack_expired_total %d\n", ack.ExpiredCount)

	builder.WriteString("# HELP vibedeck_ack_exhausted_total Total ACKs that exhausted retry attempts.\n")
	builder.WriteString("# TYPE vibedeck_ack_exhausted_total counter\n")
	fmt.Fprintf(&builder, "vibedeck_ack_exhausted_total %d\n", ack.ExhaustedCount)

	builder.WriteString("# HELP vibedeck_ack_rtt_ms ACK round-trip time in milliseconds.\n")
	builder.WriteString("# TYPE vibedeck_ack_rtt_ms gauge\n")
	fmt.Fprintf(&builder, "vibedeck_ack_rtt_ms{stat=%q} %d\n", "last", ack.LastAckRTTMs)
	fmt.Fprintf(&builder, "vibedeck_ack_rtt_ms{stat=%q} %d\n", "avg", ack.AvgAckRTTMs)
	fmt.Fprintf(&builder, "vibedeck_ack_rtt_ms{stat=%q} %d\n", "max", ack.MaxAckRTTMs)

	builder.WriteString("# HELP vibedeck_control_requests_total Total control request outcomes.\n")
	builder.WriteString("# TYPE vibedeck_control_requests_total counter\n")
	appendControlPrometheusStats(&builder, "all", "all", control.Totals)
	for _, path := range sortedStringKeys(control.ByPath) {
		appendControlPrometheusStats(&builder, "all", path, control.ByPath[path])
	}
	for _, messageType := range sortedStringKeys(control.ByType) {
		appendControlPrometheusStats(&builder, messageType, "all", control.ByType[messageType])
	}
	for _, messageType := range sortedStringKeys(control.ByTypePath) {
		for _, path := range sortedStringKeys(control.ByTypePath[messageType]) {
			appendControlPrometheusStats(&builder, messageType, path, control.ByTypePath[messageType][path])
		}
	}

	builder.WriteString("# HELP vibedeck_control_latency_ms Control handler latency in milliseconds.\n")
	builder.WriteString("# TYPE vibedeck_control_latency_ms gauge\n")
	appendControlPrometheusLatency(&builder, "all", "all", control.Totals)
	for _, path := range sortedStringKeys(control.ByPath) {
		appendControlPrometheusLatency(&builder, "all", path, control.ByPath[path])
	}
	for _, messageType := range sortedStringKeys(control.ByType) {
		appendControlPrometheusLatency(&builder, messageType, "all", control.ByType[messageType])
	}
	for _, messageType := range sortedStringKeys(control.ByTypePath) {
		for _, path := range sortedStringKeys(control.ByTypePath[messageType]) {
			appendControlPrometheusLatency(&builder, messageType, path, control.ByTypePath[messageType][path])
		}
	}

	return builder.String()
}

func appendControlPrometheusStats(builder *strings.Builder, messageType, path string, stats ControlMetricStats) {
	fmt.Fprintf(builder, "vibedeck_control_requests_total{type=%q,path=%q,result=%q} %d\n", messageType, path, "success", stats.Successes)
	fmt.Fprintf(builder, "vibedeck_control_requests_total{type=%q,path=%q,result=%q} %d\n", messageType, path, "error", stats.Failures)
	fmt.Fprintf(builder, "vibedeck_control_requests_total{type=%q,path=%q,result=%q} %d\n", messageType, path, "timeout", stats.Timeouts)
}

func appendControlPrometheusLatency(builder *strings.Builder, messageType, path string, stats ControlMetricStats) {
	fmt.Fprintf(builder, "vibedeck_control_latency_ms{type=%q,path=%q,stat=%q} %d\n", messageType, path, "last", stats.LastLatencyMs)
	fmt.Fprintf(builder, "vibedeck_control_latency_ms{type=%q,path=%q,stat=%q} %d\n", messageType, path, "avg", stats.AvgLatencyMs)
	fmt.Fprintf(builder, "vibedeck_control_latency_ms{type=%q,path=%q,stat=%q} %d\n", messageType, path, "max", stats.MaxLatencyMs)
}

func allConnectionStates() []runtime.ConnectionState {
	return []runtime.ConnectionState{
		runtime.StatePairing,
		runtime.StateSignaling,
		runtime.StateP2PConnecting,
		runtime.StateP2PConnected,
		runtime.StateRelayConnected,
		runtime.StateReconnecting,
		runtime.StateClosed,
	}
}

func sortedStringKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
