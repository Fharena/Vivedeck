import 'package:flutter/material.dart';

import '../state/app_controller.dart';

class StatusScreen extends StatefulWidget {
  const StatusScreen({
    super.key,
    required this.controller,
  });

  final AppController controller;

  @override
  State<StatusScreen> createState() => _StatusScreenState();
}

class _StatusScreenState extends State<StatusScreen> {
  late final TextEditingController _agentUrlController;
  late final TextEditingController _signalingUrlController;
  late final TextEditingController _directPairingCodeController;

  @override
  void initState() {
    super.initState();
    _agentUrlController =
        TextEditingController(text: widget.controller.agentBaseUrl);
    _signalingUrlController =
        TextEditingController(text: widget.controller.signalingBaseUrl);
    _directPairingCodeController = TextEditingController(
      text: widget.controller.directPairingCode.isNotEmpty
          ? widget.controller.directPairingCode
          : widget.controller.pairingCode,
    );
  }

  @override
  void dispose() {
    _agentUrlController.dispose();
    _signalingUrlController.dispose();
    _directPairingCodeController.dispose();
    super.dispose();
  }

  String _msLabel(int value) => '${value}ms';

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        final state = widget.controller.connectionState;
        final resolvedPairingCode =
            widget.controller.directPairingCode.isNotEmpty
                ? widget.controller.directPairingCode
                : widget.controller.pairingCode;

        if (_directPairingCodeController.text.isEmpty &&
            resolvedPairingCode.isNotEmpty) {
          _directPairingCodeController.text = resolvedPairingCode;
        }

        return ListView(
          key: const ValueKey('status-screen'),
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
          children: [
            Container(
              decoration: BoxDecoration(
                color: const Color(0xFFEDF9F6),
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: const Color(0xFFB9E6DA)),
              ),
              padding: const EdgeInsets.all(14),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('연결 설정', style: Theme.of(context).textTheme.titleMedium),
                  const SizedBox(height: 10),
                  TextField(
                    controller: _agentUrlController,
                    decoration: const InputDecoration(
                      labelText: 'Agent Base URL',
                      hintText: 'http://127.0.0.1:8080',
                    ),
                  ),
                  const SizedBox(height: 8),
                  TextField(
                    controller: _signalingUrlController,
                    decoration: const InputDecoration(
                      labelText: 'Signaling Base URL',
                      hintText: 'http://127.0.0.1:8081',
                    ),
                  ),
                  const SizedBox(height: 8),
                  Row(
                    children: [
                      Expanded(
                        child: FilledButton.tonalIcon(
                          onPressed: widget.controller.isLoading
                              ? null
                              : () async {
                                  widget.controller.updateAgentBaseUrl(
                                      _agentUrlController.text);
                                  widget.controller.updateSignalingBaseUrl(
                                    _signalingUrlController.text,
                                  );
                                  await widget.controller.refreshStatus();
                                },
                          icon: const Icon(Icons.save_outlined),
                          label: const Text('설정 저장 + 갱신'),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),
            Container(
              decoration: BoxDecoration(
                color: Colors.white,
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: const Color(0xFFD6E9E3)),
              ),
              padding: const EdgeInsets.all(14),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('세션 상태', style: Theme.of(context).textTheme.titleMedium),
                  const SizedBox(height: 8),
                  Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    children: [
                      _MetricPill(label: 'Current', value: state),
                      _MetricPill(
                        label: 'P2P Active',
                        value: widget.controller.p2pActive ? 'true' : 'false',
                      ),
                      _MetricPill(
                        label: 'Pending ACK',
                        value: '${widget.controller.pendingAckCount}',
                      ),
                      _MetricPill(
                        label: 'Control Path',
                        value: widget.controller.controlPath,
                      ),
                    ],
                  ),
                  const SizedBox(height: 10),
                  Text(
                    'sessionId: ${widget.controller.sessionId.isEmpty ? '-' : widget.controller.sessionId}',
                  ),
                  const SizedBox(height: 4),
                  Text(
                    'pairingCode: ${widget.controller.pairingCode.isEmpty ? '-' : widget.controller.pairingCode}',
                  ),
                  if (widget.controller.activity.isNotEmpty) ...[
                    const SizedBox(height: 8),
                    Text('작업중: ${widget.controller.activity}'),
                  ],
                  if (widget.controller.errorMessage != null) ...[
                    const SizedBox(height: 8),
                    Text(
                      widget.controller.errorMessage!,
                      style: TextStyle(
                        color: Theme.of(context).colorScheme.error,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                  ],
                ],
              ),
            ),
            const SizedBox(height: 12),
            Container(
              decoration: BoxDecoration(
                color: const Color(0xFFFFFBF2),
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: const Color(0xFFFFE2B8)),
              ),
              padding: const EdgeInsets.all(14),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'ACK Observability',
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                  const SizedBox(height: 8),
                  Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    children: [
                      _MetricPill(
                        label: 'Avg RTT',
                        value:
                            _msLabel(widget.controller.ackMetrics.avgAckRttMs),
                      ),
                      _MetricPill(
                        label: 'Last RTT',
                        value:
                            _msLabel(widget.controller.ackMetrics.lastAckRttMs),
                      ),
                      _MetricPill(
                        label: 'Max RTT',
                        value:
                            _msLabel(widget.controller.ackMetrics.maxAckRttMs),
                      ),
                      _MetricPill(
                        label: 'Peak Queue',
                        value:
                            '${widget.controller.ackMetrics.maxPendingCount}',
                      ),
                      _MetricPill(
                        label: 'Acked',
                        value: '${widget.controller.ackMetrics.ackedCount}',
                      ),
                      _MetricPill(
                        label: 'Retries',
                        value:
                            '${widget.controller.ackMetrics.retryDispatchCount}',
                      ),
                      _MetricPill(
                        label: 'Expired',
                        value: '${widget.controller.ackMetrics.expiredCount}',
                      ),
                      _MetricPill(
                        label: 'Exhausted',
                        value: '${widget.controller.ackMetrics.exhaustedCount}',
                      ),
                    ],
                  ),
                  const SizedBox(height: 10),
                  Text(
                    'transport split: ${widget.controller.ackMetrics.pendingSplitLabel}',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: ElevatedButton.icon(
                    onPressed: widget.controller.isLoading
                        ? null
                        : widget.controller.startP2P,
                    icon: const Icon(Icons.link),
                    label: const Text('P2P 시작'),
                    style: ElevatedButton.styleFrom(
                      backgroundColor: const Color(0xFF1F8C77),
                      foregroundColor: Colors.white,
                      padding: const EdgeInsets.symmetric(vertical: 14),
                    ),
                  ),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: OutlinedButton.icon(
                    onPressed: widget.controller.isLoading
                        ? null
                        : widget.controller.stopP2P,
                    icon: const Icon(Icons.link_off),
                    label: const Text('P2P 종료'),
                    style: OutlinedButton.styleFrom(
                      padding: const EdgeInsets.symmetric(vertical: 14),
                    ),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 8),
            OutlinedButton.icon(
              onPressed: widget.controller.isLoading
                  ? null
                  : widget.controller.refreshStatus,
              icon: const Icon(Icons.refresh),
              label: const Text('상태 갱신'),
            ),
            const SizedBox(height: 12),
            Container(
              decoration: BoxDecoration(
                color: const Color(0xFFF7F8FF),
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: const Color(0xFFDBDFF7)),
              ),
              padding: const EdgeInsets.all(14),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Direct Signaling + WebRTC',
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                  const SizedBox(height: 8),
                  TextField(
                    controller: _directPairingCodeController,
                    decoration: const InputDecoration(
                      labelText: 'Pairing Code',
                      hintText: '예: 9K3P7Q',
                    ),
                  ),
                  const SizedBox(height: 10),
                  Row(
                    children: [
                      Expanded(
                        child: FilledButton.icon(
                          onPressed: widget.controller.isLoading
                              ? null
                              : () async {
                                  widget.controller.updateDirectPairingCode(
                                    _directPairingCodeController.text,
                                  );
                                  await widget.controller
                                      .connectDirectSignaling();
                                },
                          icon: const Icon(Icons.usb),
                          label: const Text('Direct 연결'),
                        ),
                      ),
                      const SizedBox(width: 10),
                      Expanded(
                        child: OutlinedButton.icon(
                          onPressed: widget.controller.isLoading
                              ? null
                              : () async {
                                  await widget.controller
                                      .disconnectDirectSignaling();
                                },
                          icon: const Icon(Icons.usb_off),
                          label: const Text('Direct 종료'),
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 10),
                  Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    children: [
                      _MetricPill(
                        label: 'Direct State',
                        value: widget.controller.directSignalingState,
                      ),
                      _MetricPill(
                        label: 'WS Connected',
                        value: widget.controller.directSignalingConnected
                            ? 'true'
                            : 'false',
                      ),
                      _MetricPill(
                        label: 'Peer Connected',
                        value: widget.controller.directPeerConnected
                            ? 'true'
                            : 'false',
                      ),
                      _MetricPill(
                        label: 'Control Ready',
                        value: widget.controller.directControlReady
                            ? 'true'
                            : 'false',
                      ),
                    ],
                  ),
                  const SizedBox(height: 8),
                  Text(
                    'directSessionId: ${widget.controller.directSessionId.isEmpty ? '-' : widget.controller.directSessionId}',
                  ),
                  const SizedBox(height: 4),
                  Text(
                    'directDeviceKey: ${widget.controller.directDeviceKey.isEmpty ? '-' : widget.controller.directDeviceKey}',
                  ),
                  const SizedBox(height: 8),
                  Text(
                    'Control Ready=true 이면 Prompt/Review 액션이 HTTP 대신 DataChannel(DIRECT) 경로를 우선 사용합니다.',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                  const SizedBox(height: 8),
                  Text('최근 direct 로그',
                      style: Theme.of(context).textTheme.titleSmall),
                  const SizedBox(height: 6),
                  if (widget.controller.directSignalLogs.isEmpty)
                    const Text('로그 없음')
                  else
                    ...widget.controller.directSignalLogs.take(10).map((log) =>
                        Text(log,
                            style: Theme.of(context).textTheme.bodySmall)),
                ],
              ),
            ),
            const SizedBox(height: 12),
            Card(
              child: Padding(
                padding: const EdgeInsets.all(14),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'Runtime Timeline',
                      style: Theme.of(context).textTheme.titleMedium,
                    ),
                    const SizedBox(height: 8),
                    if (widget.controller.runtimeHistory.isEmpty)
                      const Text('히스토리가 없습니다.')
                    else
                      ...widget.controller.runtimeHistory.map(
                        (event) => ListTile(
                          contentPadding: EdgeInsets.zero,
                          leading: const Icon(Icons.timeline),
                          title: Text(event.state),
                          subtitle: Text(event.note.isEmpty ? '-' : event.note),
                          trailing: Text(event.atLabel),
                        ),
                      ),
                  ],
                ),
              ),
            ),
          ],
        );
      },
    );
  }
}

class _MetricPill extends StatelessWidget {
  const _MetricPill({
    required this.label,
    required this.value,
  });

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(22),
        border: Border.all(color: const Color(0xFFD6E9E3)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: Theme.of(context).textTheme.bodySmall),
          Text(
            value,
            style: Theme.of(
              context,
            ).textTheme.bodyMedium?.copyWith(fontWeight: FontWeight.w700),
          ),
        ],
      ),
    );
  }
}
