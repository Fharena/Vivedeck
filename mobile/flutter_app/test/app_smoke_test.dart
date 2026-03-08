import 'package:flutter_test/flutter_test.dart';
import 'package:vibedeck_mobile/app.dart';
import 'package:vibedeck_mobile/services/agent_api.dart';
import 'package:vibedeck_mobile/state/app_controller.dart';

void main() {
  testWidgets('shows core shell labels', (tester) async {
    final controller = AppController(api: _FakeShellAgentApi());
    addTearDown(controller.dispose);

    await tester.pumpWidget(VibeDeckApp(controller: controller));
    await tester.pumpAndSettle();

    expect(find.text('VibeDeck Mobile'), findsOneWidget);
    expect(find.text('대화'), findsWidgets);
    expect(find.text('검토'), findsWidgets);
    expect(find.text('상태'), findsWidgets);
  });
}

class _FakeShellAgentApi extends AgentApi {
  @override
  Future<Map<String, dynamic>> p2pStatus(String baseUrl) async {
    return {
      'active': false,
      'sessionId': '',
      'pairingCode': '',
      'state': 'PAIRING',
    };
  }

  @override
  Future<Map<String, dynamic>> runtimeState(String baseUrl) async {
    return {
      'state': 'PAIRING',
      'history': const [],
    };
  }

  @override
  Future<Map<String, dynamic>> runtimeMetrics(String baseUrl) async {
    return {
      'state': 'PAIRING',
      'ack': {
        'pendingCount': 0,
        'maxPendingCount': 0,
        'pendingByTransport': {
          'http': 0,
          'p2p': 0,
          'unknown': 0,
        },
        'ackedCount': 0,
        'retryDispatchCount': 0,
        'expiredCount': 0,
        'exhaustedCount': 0,
        'lastAckRttMs': 0,
        'avgAckRttMs': 0,
        'maxAckRttMs': 0,
      },
    };
  }

  @override
  Future<Map<String, dynamic>> runtimeAdapter(String baseUrl) async {
    return {
      'name': 'mock-cursor',
      'mode': 'mock',
      'ready': true,
      'workspaceRoot': 'C:/demo/workspace',
      'binaryPath': 'node',
      'notes': const <String>[],
    };
  }

  @override
  Future<Map<String, dynamic>> runProfiles(String baseUrl) async {
    return {
      'profiles': const [
        {
          'id': 'test_all',
          'label': 'Demo Check',
          'command': 'git status --short',
          'scope': 'SMALL',
          'optional': false,
        },
      ],
    };
  }

  @override
  Future<Map<String, dynamic>> threads(String baseUrl) async {
    return {'threads': const []};
  }

  @override
  Future<Map<String, dynamic>> threadDetail(String baseUrl, String threadId) async {
    return {
      'thread': {
        'id': threadId,
        'title': '새 스레드',
        'sessionId': '',
        'state': 'draft',
        'currentJobId': '',
        'lastEventKind': '',
        'lastEventText': '',
        'updatedAt': 0,
      },
      'events': const [],
    };
  }

  @override
  void dispose() {}
}