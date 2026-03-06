import 'package:flutter/material.dart';

class StatusScreen extends StatefulWidget {
  const StatusScreen({super.key});

  @override
  State<StatusScreen> createState() => _StatusScreenState();
}

class _StatusScreenState extends State<StatusScreen> {
  final List<String> _states = [
    'SIGNALING',
    'P2P_CONNECTING',
    'P2P_CONNECTED',
    'RECONNECTING',
  ];

  int _index = 2;
  int _pendingAcks = 1;

  List<StatusEvent> _history = [
    StatusEvent(state: 'SIGNALING', note: 'pairing 생성 완료', minutesAgo: 3),
    StatusEvent(state: 'P2P_CONNECTING', note: 'offer 송신', minutesAgo: 2),
    StatusEvent(state: 'P2P_CONNECTED', note: 'datachannel open', minutesAgo: 1),
  ];

  @override
  Widget build(BuildContext context) {
    final state = _states[_index];

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
              Text('세션 상태', style: Theme.of(context).textTheme.titleMedium),
              const SizedBox(height: 8),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: List.generate(_states.length, (i) {
                  return ChoiceChip(
                    selected: _index == i,
                    label: Text(_states[i]),
                    onSelected: (_) {
                      setState(() {
                        _index = i;
                        _history = [
                          StatusEvent(state: _states[i], note: '수동 상태 확인', minutesAgo: 0),
                          ..._history,
                        ];
                      });
                    },
                  );
                }),
              ),
              const SizedBox(height: 12),
              Row(
                children: [
                  _MetricPill(label: 'Current', value: state),
                  const SizedBox(width: 8),
                  _MetricPill(label: 'Pending ACK', value: '$_pendingAcks'),
                ],
              ),
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
                Text('Runtime Timeline', style: Theme.of(context).textTheme.titleMedium),
                const SizedBox(height: 8),
                ..._history.map(
                  (event) => ListTile(
                    contentPadding: EdgeInsets.zero,
                    leading: const Icon(Icons.timeline),
                    title: Text(event.state),
                    subtitle: Text(event.note),
                    trailing: Text('${event.minutesAgo}m'),
                  ),
                ),
              ],
            ),
          ),
        ),
        const SizedBox(height: 12),
        ElevatedButton.icon(
          onPressed: () {
            setState(() {
              _pendingAcks = _pendingAcks == 0 ? 2 : _pendingAcks - 1;
              _history = [
                StatusEvent(state: _states[_index], note: 'runtime 상태 조회 갱신', minutesAgo: 0),
                ..._history,
              ];
            });
          },
          icon: const Icon(Icons.refresh),
          label: const Text('상태 갱신 시뮬레이션'),
          style: ElevatedButton.styleFrom(
            backgroundColor: const Color(0xFF1F8C77),
            foregroundColor: Colors.white,
            padding: const EdgeInsets.symmetric(vertical: 14),
          ),
        ),
      ],
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
          Text(value, style: Theme.of(context).textTheme.bodyMedium?.copyWith(fontWeight: FontWeight.w700)),
        ],
      ),
    );
  }
}

class StatusEvent {
  const StatusEvent({
    required this.state,
    required this.note,
    required this.minutesAgo,
  });

  final String state;
  final String note;
  final int minutesAgo;
}
