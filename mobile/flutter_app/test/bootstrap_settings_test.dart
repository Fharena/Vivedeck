import 'package:flutter_test/flutter_test.dart';
import 'package:vibedeck_mobile/services/agent_api.dart';
import 'package:vibedeck_mobile/services/app_settings_store.dart';
import 'package:vibedeck_mobile/services/lan_discovery_service.dart';
import 'package:vibedeck_mobile/state/app_controller.dart';

void main() {
  test('loads bootstrap defaults and remembers recent hosts', () async {
    final settingsStore = InMemoryAppSettingsStore();
    final controller = AppController(
      api: FakeBootstrapAgentApi(),
      settingsStore: settingsStore,
    );

    addTearDown(controller.dispose);

    await controller.initialize();

    expect(controller.agentBaseUrl, 'http://192.168.0.24:8080');
    expect(controller.signalingBaseUrl, 'http://192.168.0.24:8081');
    expect(controller.bootstrap.workspaceRoot, 'C:/demo/workspace');
    expect(controller.bootstrap.adapter.provider, 'cursor');
    expect(controller.currentThreadId, 'thread-bootstrap-1');
    expect(controller.recentHosts, hasLength(1));
    expect(
        controller.recentHosts.single.agentBaseUrl, 'http://192.168.0.24:8080');
    expect(
      controller.recentHosts.single.signalingBaseUrl,
      'http://192.168.0.24:8081',
    );

    final restored = AppController(
      api: FakeBootstrapAgentApi(),
      settingsStore: settingsStore,
    );
    addTearDown(restored.dispose);

    await restored.initialize();

    expect(restored.agentBaseUrl, 'http://192.168.0.24:8080');
    expect(restored.signalingBaseUrl, 'http://192.168.0.24:8081');
    expect(restored.recentHosts, isNotEmpty);
  });

  test('discovers LAN host and auto-applies single result', () async {
    final settingsStore = InMemoryAppSettingsStore();
    final controller = AppController(
      api: FakeBootstrapAgentApi(),
      settingsStore: settingsStore,
      lanDiscoveryService: FakeLanDiscoveryService.single(),
    );
    addTearDown(controller.dispose);

    await controller.initialize();
    controller.updateAgentBaseUrl('http://127.0.0.1:8080');
    controller.updateSignalingBaseUrl('http://127.0.0.1:8081');

    await controller.discoverLanHosts();

    expect(controller.agentBaseUrl, 'http://192.168.0.77:8080');
    expect(controller.signalingBaseUrl, 'http://192.168.0.77:8081');
    expect(controller.currentThreadId, 'thread-lan-1');
    expect(controller.discoveredHosts, hasLength(1));
    expect(
        controller.recentHosts.first.agentBaseUrl, 'http://192.168.0.77:8080');
  });
}

class FakeLanDiscoveryService implements LanDiscoveryService {
  FakeLanDiscoveryService(this.results);

  final List<Map<String, dynamic>> results;

  factory FakeLanDiscoveryService.single() {
    return FakeLanDiscoveryService([
      {
        'type': 'vibedeck_discover_result',
        'version': 1,
        'displayName': 'Desk Host',
        'sourceAddress': '192.168.0.77',
        'agentBaseUrl': 'http://192.168.0.77:8080',
        'signalingBaseUrl': 'http://192.168.0.77:8081',
        'workspaceRoot': 'C:/demo/workspace',
        'currentThreadId': 'thread-lan-1',
        'adapter': {
          'name': 'cursor-agent-cli',
          'mode': 'cursor_agent_cli',
          'provider': 'cursor',
          'ready': true,
        },
        'recentThreads': [
          {
            'id': 'thread-lan-1',
            'title': 'LAN 연결 스레드',
            'updatedAt': DateTime(2026, 3, 9, 10, 15).millisecondsSinceEpoch,
            'current': true,
          },
        ],
      },
    ]);
  }

  @override
  Future<List<Map<String, dynamic>>> discover({Duration? timeout}) async =>
      results;
}

class FakeBootstrapAgentApi extends AgentApi {
  @override
  Future<Map<String, dynamic>> bootstrap(String baseUrl) async {
    final requested = baseUrl.trim();
    final agent = requested.isEmpty || requested.contains('127.0.0.1')
        ? 'http://192.168.0.24:8080'
        : requested;
    final signaling = agent.replaceFirst(':8080', ':8081');
    final threadId =
        agent.contains('192.168.0.77') ? 'thread-lan-1' : 'thread-bootstrap-1';
    final threadTitle = agent.contains('192.168.0.77') ? 'LAN 연결 스레드' : '최근 작업';
    return {
      'agentBaseUrl': agent,
      'signalingBaseUrl': signaling,
      'workspaceRoot': 'C:/demo/workspace',
      'currentThreadId': threadId,
      'adapter': {
        'name': 'cursor-agent-cli',
        'mode': 'cursor_agent_cli',
        'provider': 'cursor',
        'ready': true,
      },
      'recentThreads': [
        {
          'id': threadId,
          'title': threadTitle,
          'updatedAt': DateTime(2026, 3, 8, 20, 45).millisecondsSinceEpoch,
          'current': true,
        },
      ],
    };
  }

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
      'name': 'cursor-agent-cli',
      'mode': 'cursor_agent_cli',
      'ready': true,
      'workspaceRoot': 'C:/demo/workspace',
      'binaryPath': '/home/demo/.local/bin/cursor-agent',
      'notes': const ['bootstrap test adapter'],
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
    final threadId = baseUrl.contains('192.168.0.77')
        ? 'thread-lan-1'
        : 'thread-bootstrap-1';
    final title = baseUrl.contains('192.168.0.77') ? 'LAN 연결 스레드' : '최근 작업';
    return {
      'threads': [
        {
          'id': threadId,
          'title': title,
          'sessionId': 'sid-bootstrap',
          'state': 'draft',
          'currentJobId': '',
          'lastEventKind': '',
          'lastEventText': '',
          'updatedAt': DateTime(2026, 3, 8, 20, 45).millisecondsSinceEpoch,
        },
      ],
    };
  }

  @override
  Future<Map<String, dynamic>> threadDetail(
      String baseUrl, String threadId) async {
    return {
      'thread': {
        'id': threadId,
        'title': threadId == 'thread-lan-1' ? 'LAN 연결 스레드' : '최근 작업',
        'sessionId': 'sid-bootstrap',
        'state': 'draft',
        'currentJobId': '',
        'lastEventKind': '',
        'lastEventText': '',
        'updatedAt': DateTime(2026, 3, 8, 20, 45).millisecondsSinceEpoch,
      },
      'events': const [],
    };
  }

  @override
  void dispose() {}
}
