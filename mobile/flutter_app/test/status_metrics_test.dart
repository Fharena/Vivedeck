import 'package:flutter_test/flutter_test.dart';
import 'package:vibedeck_mobile/app.dart';
import 'package:vibedeck_mobile/services/agent_api.dart';
import 'package:vibedeck_mobile/state/app_controller.dart';

void main() {
  testWidgets('shows ack observability metrics on status screen',
      (tester) async {
    final controller = AppController(api: FakeMetricsAgentApi());

    addTearDown(controller.dispose);

    await tester.pumpWidget(VibeDeckApp(controller: controller));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Status').last);
    await tester.pumpAndSettle();

    expect(find.text('ACK Observability'), findsOneWidget);
    expect(find.text('24ms'), findsOneWidget);
    expect(find.text('33ms'), findsOneWidget);
    expect(find.text('47ms'), findsOneWidget);
    expect(find.textContaining('transport split: http=1'), findsOneWidget);
    expect(find.textContaining('/ p2p=2 / unknown=0'), findsOneWidget);
  });
}

class FakeMetricsAgentApi extends AgentApi {
  FakeMetricsAgentApi();

  @override
  Future<Map<String, dynamic>> p2pStatus(String baseUrl) async {
    return {
      'active': true,
      'sessionId': 'sid-metrics-1',
      'pairingCode': 'PAIR999',
      'state': 'P2P_CONNECTED',
    };
  }

  @override
  Future<Map<String, dynamic>> runtimeState(String baseUrl) async {
    return {
      'state': 'P2P_CONNECTED',
      'history': [
        {
          'state': 'p2p_connected',
          'note': 'metrics ready',
          'at': DateTime(2026, 3, 7, 10, 5, 0).millisecondsSinceEpoch,
        },
      ],
    };
  }

  @override
  Future<Map<String, dynamic>> pendingAcks(String baseUrl) async {
    return {
      'pending': const [],
      'count': 3,
    };
  }

  @override
  Future<Map<String, dynamic>> runtimeMetrics(String baseUrl) async {
    return {
      'state': 'P2P_CONNECTED',
      'ack': {
        'pendingCount': 3,
        'maxPendingCount': 5,
        'pendingByTransport': {
          'http': 1,
          'p2p': 2,
          'unknown': 0,
        },
        'ackedCount': 11,
        'retryDispatchCount': 4,
        'expiredCount': 1,
        'exhaustedCount': 1,
        'lastAckRttMs': 33,
        'avgAckRttMs': 24,
        'maxAckRttMs': 47,
      },
    };
  }

  @override
  void dispose() {}
}
