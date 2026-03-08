import 'package:flutter_test/flutter_test.dart';
import 'package:vibedeck_mobile/services/agent_api.dart';
import 'package:vibedeck_mobile/state/app_controller.dart';

void main() {
  test('loads ack observability metrics and adapter runtime', () async {
    final controller = AppController(api: FakeMetricsAgentApi());

    addTearDown(controller.dispose);

    await controller.refreshStatus();

    expect(controller.ackMetrics.avgAckRttMs, 24);
    expect(controller.ackMetrics.lastAckRttMs, 33);
    expect(controller.ackMetrics.maxAckRttMs, 47);
    expect(controller.ackMetrics.pendingSplitLabel, 'http=1 / p2p=2 / unknown=0');
    expect(controller.adapterRuntime.workspaceRoot, 'C:/demo/workspace');
    expect(controller.adapterRuntime.name, 'cursor-agent-cli');
    expect(controller.currentThreadId, 'thread-metrics-1');
    expect(controller.currentThreadTitle, 'metrics thread');
  });
}

class FakeMetricsAgentApi extends AgentApi {
  FakeMetricsAgentApi();

  @override
  Future<Map<String, dynamic>> bootstrap(String baseUrl) async {
    return {
      'agentBaseUrl': 'http://192.168.0.24:8080',
      'signalingBaseUrl': 'http://192.168.0.24:8081',
      'workspaceRoot': 'C:/demo/workspace',
      'currentThreadId': 'thread-metrics-1',
      'adapter': {
        'name': 'cursor-agent-cli',
        'mode': 'cursor_agent_cli',
        'provider': 'cursor',
        'ready': true,
      },
      'recentThreads': [
        {
          'id': 'thread-metrics-1',
          'title': 'metrics thread',
          'updatedAt': DateTime(2026, 3, 7, 10, 5, 1).millisecondsSinceEpoch,
          'current': true,
        },
      ],
    };
  }

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
  Future<Map<String, dynamic>> runtimeAdapter(String baseUrl) async {
    return {
      'name': 'cursor-agent-cli',
      'mode': 'cursor_agent_cli',
      'ready': true,
      'workspaceRoot': 'C:/demo/workspace',
      'binaryPath': '/home/demo/.local/bin/cursor-agent',
      'notes': const ['demo metrics adapter'],
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
    return {
      'threads': [
        {
          'id': 'thread-metrics-1',
          'title': 'metrics thread',
          'sessionId': 'sid-metrics-1',
          'state': 'patch_ready',
          'currentJobId': 'job-metrics-1',
          'lastEventKind': 'patch_ready',
          'lastEventText': 'patch ready',
          'updatedAt': DateTime(2026, 3, 7, 10, 5, 1).millisecondsSinceEpoch,
        },
      ],
    };
  }

  @override
  Future<Map<String, dynamic>> threadDetail(String baseUrl, String threadId) async {
    return {
      'thread': {
        'id': threadId,
        'title': 'metrics thread',
        'sessionId': 'sid-metrics-1',
        'state': 'patch_ready',
        'currentJobId': 'job-metrics-1',
        'lastEventKind': 'patch_ready',
        'lastEventText': 'patch ready',
        'updatedAt': DateTime(2026, 3, 7, 10, 5, 1).millisecondsSinceEpoch,
      },
      'events': [
        {
          'id': 'evt-metrics-1',
          'threadId': threadId,
          'jobId': 'job-metrics-1',
          'kind': 'patch_ready',
          'role': 'assistant',
          'title': '패치 준비 완료',
          'body': 'metrics ready',
          'data': {
            'summary': 'metrics ready',
          },
          'at': DateTime(2026, 3, 7, 10, 5, 1).millisecondsSinceEpoch,
        },
      ],
    };
  }

  @override
  void dispose() {}
}
