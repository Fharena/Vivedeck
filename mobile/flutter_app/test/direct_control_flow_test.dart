import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:vibedeck_mobile/services/agent_api.dart';
import 'package:vibedeck_mobile/services/mobile_direct_signaling_session.dart';
import 'package:vibedeck_mobile/services/signaling_api.dart';
import 'package:vibedeck_mobile/state/app_controller.dart';

void main() {
  test('drives prompt patch run loop over direct control path', () async {
    final threadStore = FakeThreadStore();
    final agentApi = FakeAgentApi(threadStore);
    final directSession = FakeDirectSession(threadStore: threadStore);
    final controller = AppController(
      api: agentApi,
      directSessionFactory: () => directSession,
    );

    addTearDown(controller.dispose);

    controller.updateDirectPairingCode('PAIR123');
    await controller.connectDirectSignaling();

    expect(controller.directControlReady, isTrue);
    expect(controller.controlPath, 'DIRECT');

    await controller.submitPrompt(
      prompt: 'auth middleware 버그 수정',
      context: const {
        'activeFile': true,
        'selection': true,
        'latestError': true,
        'workspaceSummary': false,
      },
    );

    expect(controller.currentThreadId, 'thread-direct-1');
    expect(controller.currentJobId, 'job-direct-1');
    expect(controller.patchSummary, 'Mock patch for direct flow');
    expect(controller.patchFiles, hasLength(1));

    expect(controller.patchFiles.single.path, 'src/auth/middleware.ts');

    await controller.applyPatch(
      applyAll: true,
      selectedByPath: const <String, Set<String>>{},
    );

    expect(controller.patchResultStatus, 'success');
    expect(controller.patchResultMessage, 'patch applied');

    await controller.runProfile('test_all');

    expect(controller.runStatus, 'failed');
    expect(controller.runSummary, '1 failing test in auth middleware');
    expect(controller.runOutput, contains('AssertionError: expected 401 got 500'));
    expect(controller.topErrors.single, contains('tests/auth/middleware.test.ts:44'));

    expect(agentApi.httpEnvelopeTypes, isEmpty);
    expect(
      directSession.requestEnvelopeTypes,
      ['PROMPT_SUBMIT', 'PATCH_APPLY', 'RUN_PROFILE'],
    );
    expect(
      directSession.ackRequestRids,
      [
        'rid-prompt-ack-direct-1',
        'rid-patch-ready-direct-1',
        'rid-patch-result-direct-1',
        'rid-run-result-direct-1',
      ],
    );
  });

  test('falls back to HTTP when direct control request fails', () async {
    final threadStore = FakeThreadStore();
    final agentApi = FakeAgentApi(threadStore);
    final directSession = FakeDirectSession(
      threadStore: threadStore,
      failRequests: true,
    );
    final controller = AppController(
      api: agentApi,
      directSessionFactory: () => directSession,
    );

    addTearDown(controller.dispose);

    controller.updateDirectPairingCode('PAIR123');
    await controller.connectDirectSignaling();
    await controller.submitPrompt(
      prompt: 'auth middleware 버그 수정',
      context: const {
        'activeFile': true,
        'selection': true,
        'latestError': true,
        'workspaceSummary': false,
      },
    );

    expect(controller.currentThreadId, 'thread-http-1');
    expect(controller.currentJobId, 'job-http-1');
    expect(controller.patchSummary, 'Mock patch through HTTP fallback');
    expect(agentApi.httpEnvelopeTypes, ['PROMPT_SUBMIT', 'CMD_ACK', 'CMD_ACK']);
    expect(directSession.requestEnvelopeTypes, ['PROMPT_SUBMIT']);
    expect(
      controller.directSignalLogs.any(
        (log) => log.contains('DIRECT 제어 실패 -> HTTP 폴백'),
      ),
      isTrue,
    );
  });
}

class FakeAgentApi extends AgentApi {
  FakeAgentApi(this.threadStore);

  @override
  Future<Map<String, dynamic>> bootstrap(String baseUrl) async {
    final threads = threadStore.listThreads();
    return {
      'agentBaseUrl': 'http://127.0.0.1:8080',
      'signalingBaseUrl': 'http://127.0.0.1:8081',
      'workspaceRoot': 'C:/vibedeck-demo/workspace',
      'currentThreadId': threads.isEmpty ? '' : threads.first['id']?.toString() ?? '',
      'adapter': {
        'name': 'cursor-agent-cli',
        'mode': 'cursor_agent_cli',
        'provider': 'cursor',
        'ready': true,
      },
      'recentThreads': threads.map((thread) => {...Map<String, dynamic>.from(thread), 'current': true}).toList(),
    };
  }

  final FakeThreadStore threadStore;
  final List<Map<String, dynamic>> sentEnvelopes = [];

  List<String> get httpEnvelopeTypes => sentEnvelopes
      .map((envelope) => envelope['type']?.toString() ?? '')
      .where((type) => type.isNotEmpty)
      .toList();

  @override
  Future<Map<String, dynamic>> p2pStart(
    String baseUrl, {
    String? signalingBaseUrl,
  }) async {
    return _p2pStatus();
  }

  @override
  Future<Map<String, dynamic>> p2pStatus(String baseUrl) async {
    return _p2pStatus();
  }

  @override
  Future<Map<String, dynamic>> p2pStop(String baseUrl) async {
    return {
      'active': false,
      'sessionId': '',
      'pairingCode': '',
      'state': 'CLOSED',
    };
  }

  @override
  Future<Map<String, dynamic>> runtimeState(String baseUrl) async {
    return {
      'state': 'P2P_CONNECTED',
      'history': [
        {
          'state': 'pairing',
          'note': 'fake status',
          'at': DateTime(2026, 3, 7, 10, 0, 0).millisecondsSinceEpoch,
        },
        {
          'state': 'p2p_connected',
          'note': 'fake direct connected',
          'at': DateTime(2026, 3, 7, 10, 0, 1).millisecondsSinceEpoch,
        },
      ],
    };
  }

  @override
  Future<Map<String, dynamic>> pendingAcks(String baseUrl) async {
    return {
      'pending': const [],
      'count': 0,
    };
  }

  @override
  Future<Map<String, dynamic>> runtimeMetrics(String baseUrl) async {
    return {
      'state': 'P2P_CONNECTED',
      'ack': {
        'pendingCount': 0,
        'maxPendingCount': 2,
        'pendingByTransport': {
          'http': 0,
          'p2p': 0,
          'unknown': 0,
        },
        'ackedCount': 4,
        'retryDispatchCount': 0,
        'expiredCount': 0,
        'exhaustedCount': 0,
        'lastAckRttMs': 18,
        'avgAckRttMs': 12,
        'maxAckRttMs': 18,
      },
    };
  }

  @override
  Future<Map<String, dynamic>> runtimeAdapter(String baseUrl) async {
    return {
      'name': 'cursor-agent-cli',
      'mode': 'cursor_agent_cli',
      'ready': true,
      'workspaceRoot': 'C:/vibedeck-demo/workspace',
      'binaryPath': '/home/fharena/.local/bin/cursor-agent',
      'notes': const ['fake test adapter'],
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
    return {'threads': threadStore.listThreads()};
  }

  @override
  Future<Map<String, dynamic>> threadDetail(String baseUrl, String threadId) async {
    return threadStore.threadDetail(threadId);
  }

  @override
  Future<Map<String, dynamic>> sendEnvelope(
    String baseUrl,
    Map<String, dynamic> envelope,
  ) async {
    sentEnvelopes.add(Map<String, dynamic>.from(envelope));

    final type = envelope['type']?.toString() ?? '';
    if (type == 'PROMPT_SUBMIT') {
      final sid = envelope['sid']?.toString() ?? 'sid-http-1';
      final requestRid = envelope['rid']?.toString() ?? 'rid-http-submit';
      threadStore.recordPrompt(
        threadId: 'thread-http-1',
        sessionId: sid,
        jobId: 'job-http-1',
        prompt: 'auth middleware 버그 수정',
        summary: 'Mock patch through HTTP fallback',
      );
      return {
        'responses': [
          cmdAckEnvelope(sid: sid, requestRid: requestRid, seq: 10),
          promptAckEnvelope(
            sid: sid,
            rid: 'rid-prompt-ack-http-1',
            seq: 11,
            threadId: 'thread-http-1',
            jobId: 'job-http-1',
          ),
          patchReadyEnvelope(
            sid: sid,
            rid: 'rid-patch-ready-http-1',
            seq: 12,
            summary: 'Mock patch through HTTP fallback',
            jobId: 'job-http-1',
          ),
        ],
      };
    }

    if (type == 'PATCH_APPLY') {
      threadStore.recordPatch(
        threadId: 'thread-http-1',
        jobId: 'job-http-1',
        status: 'success',
        message: 'patch applied',
      );
    }

    if (type == 'RUN_PROFILE') {
      threadStore.recordRun(
        threadId: 'thread-http-1',
        jobId: 'job-http-1',
        profileId: 'test_all',
        status: 'failed',
        summary: '1 failing test in auth middleware',
        excerpt: 'AssertionError: expected 401 got 500',
        output: 'FAIL tests/auth/middleware.test.ts\nAssertionError: expected 401 got 500',
      );
    }

    return {
      'handled': true,
      'requestRid': envelope['payload'] is Map
          ? (envelope['payload'] as Map)['requestRid']
          : null,
    };
  }

  @override
  void dispose() {}

  Map<String, dynamic> _p2pStatus() {
    return {
      'active': true,
      'sessionId': 'sid-direct-1',
      'pairingCode': 'PAIR123',
      'state': 'P2P_CONNECTED',
    };
  }
}

class FakeDirectSession extends MobileDirectSignalingSession {
  FakeDirectSession({
    required this.threadStore,
    this.failRequests = false,
  });

  final FakeThreadStore threadStore;
  final bool failRequests;

  final StreamController<DirectSignalingState> _states =
      StreamController<DirectSignalingState>.broadcast();
  final StreamController<DirectSignalEvent> _events =
      StreamController<DirectSignalEvent>.broadcast();
  final StreamController<Map<String, dynamic>> _envelopes =
      StreamController<Map<String, dynamic>>.broadcast();
  final StreamController<String> _errors = StreamController<String>.broadcast();

  final List<Map<String, dynamic>> requestEnvelopes = [];
  final List<Map<String, dynamic>> sentAckEnvelopes = [];

  List<String> get requestEnvelopeTypes => requestEnvelopes
      .map((envelope) => envelope['type']?.toString() ?? '')
      .where((type) => type.isNotEmpty)
      .toList();

  List<String> get ackRequestRids => sentAckEnvelopes
      .map((envelope) =>
          ((envelope['payload'] as Map?)?['requestRid']?.toString() ?? ''))
      .where((rid) => rid.isNotEmpty)
      .toList();

  @override
  Stream<DirectSignalingState> get states => _states.stream;

  @override
  Stream<DirectSignalEvent> get events => _events.stream;

  @override
  Stream<Map<String, dynamic>> get envelopes => _envelopes.stream;

  @override
  Stream<String> get errors => _errors.stream;

  @override
  bool get isControlReady => isConnected && isDataChannelOpen;

  @override
  Future<void> connect({
    required String signalingBaseUrl,
    required String pairingCode,
  }) async {
    sessionId = 'sid-direct-1';
    mobileDeviceKey = 'mobile-device-1';
    isConnected = true;
    isPeerConnected = true;
    isDataChannelOpen = true;

    _states.add(DirectSignalingState.claiming);
    _events.add(_event('pairing claim 시작: $pairingCode'));
    _states.add(DirectSignalingState.claimed);
    _events.add(_event('pairing claim 성공: sid=$sessionId'));
    _states.add(DirectSignalingState.wsConnected);
    _states.add(DirectSignalingState.peerConnected);
    _states.add(DirectSignalingState.dataChannelOpen);
    _events.add(_event('data channel state: open'));
  }

  @override
  Future<List<Map<String, dynamic>>> sendControlEnvelopeAndAwaitResponses(
    Map<String, dynamic> envelope, {
    Duration timeout = const Duration(seconds: 6),
    Duration quietPeriod = const Duration(milliseconds: 280),
  }) async {
    requestEnvelopes.add(Map<String, dynamic>.from(envelope));

    if (failRequests) {
      throw SignalingApiException(0, 'forced direct failure');
    }

    final sid = envelope['sid']?.toString() ?? sessionId;
    final requestRid = envelope['rid']?.toString() ?? 'rid-request';
    final type = envelope['type']?.toString() ?? '';

    switch (type) {
      case 'PROMPT_SUBMIT':
        threadStore.recordPrompt(
          threadId: 'thread-direct-1',
          sessionId: sid,
          jobId: 'job-direct-1',
          prompt: 'auth middleware 버그 수정',
          summary: 'Mock patch for direct flow',
        );
        return [
          cmdAckEnvelope(sid: sid, requestRid: requestRid, seq: 20),
          promptAckEnvelope(
            sid: sid,
            rid: 'rid-prompt-ack-direct-1',
            seq: 21,
            threadId: 'thread-direct-1',
            jobId: 'job-direct-1',
          ),
          patchReadyEnvelope(
            sid: sid,
            rid: 'rid-patch-ready-direct-1',
            seq: 22,
            summary: 'Mock patch for direct flow',
            jobId: 'job-direct-1',
          ),
        ];
      case 'PATCH_APPLY':
        threadStore.recordPatch(
          threadId: 'thread-direct-1',
          jobId: 'job-direct-1',
          status: 'success',
          message: 'patch applied',
        );
        return [
          cmdAckEnvelope(sid: sid, requestRid: requestRid, seq: 30),
          patchResultEnvelope(
            sid: sid,
            rid: 'rid-patch-result-direct-1',
            seq: 31,
          ),
        ];
      case 'RUN_PROFILE':
        threadStore.recordRun(
          threadId: 'thread-direct-1',
          jobId: 'job-direct-1',
          profileId: 'test_all',
          status: 'failed',
          summary: '1 failing test in auth middleware',
          excerpt: 'AssertionError: expected 401 got 500',
          output: 'FAIL tests/auth/middleware.test.ts\nAssertionError: expected 401 got 500',
        );
        return [
          cmdAckEnvelope(sid: sid, requestRid: requestRid, seq: 40),
          runResultEnvelope(
            sid: sid,
            rid: 'rid-run-result-direct-1',
            seq: 41,
            jobId: 'job-direct-1',
          ),
        ];
      default:
        return [cmdAckEnvelope(sid: sid, requestRid: requestRid, seq: 50)];
    }
  }

  @override
  Future<void> sendControlEnvelope(Map<String, dynamic> envelope) async {
    sentAckEnvelopes.add(Map<String, dynamic>.from(envelope));
  }

  @override
  Future<void> close() async {
    isConnected = false;
    isPeerConnected = false;
    isDataChannelOpen = false;
    _states.add(DirectSignalingState.closed);
  }

  @override
  void dispose() {
    unawaited(close());
    unawaited(_states.close());
    unawaited(_events.close());
    unawaited(_envelopes.close());
    unawaited(_errors.close());
  }

  DirectSignalEvent _event(String message) {
    return DirectSignalEvent(
      message: message,
      at: DateTime(2026, 3, 7, 10, 0, 0),
    );
  }
}

class FakeThreadStore {
  String _threadId = '';
  String _sessionId = '';
  String _title = '새 스레드';
  String _state = 'draft';
  String _currentJobId = '';
  String _lastEventKind = '';
  String _lastEventText = '';
  int _updatedAt = 0;
  final List<Map<String, dynamic>> _events = [];

  List<Map<String, dynamic>> listThreads() {
    if (_threadId.isEmpty) {
      return const [];
    }
    return [threadSummary()];
  }

  Map<String, dynamic> threadDetail(String threadId) {
    if (_threadId.isEmpty || threadId != _threadId) {
      return {
        'thread': {
          'id': threadId,
          'title': threadId,
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
    return {
      'thread': threadSummary(),
      'events': _events,
    };
  }

  void recordPrompt({
    required String threadId,
    required String sessionId,
    required String jobId,
    required String prompt,
    required String summary,
  }) {
    _threadId = threadId;
    _sessionId = sessionId;
    _title = prompt;
    _currentJobId = jobId;
    _events
      ..clear()
      ..add(_event(
        kind: 'prompt_submitted',
        role: 'user',
        title: '프롬프트 제출',
        body: prompt,
        jobId: jobId,
      ))
      ..add(_event(
        kind: 'prompt_accepted',
        role: 'system',
        title: '작업 시작',
        body: 'prompt accepted',
        jobId: jobId,
      ))
      ..add(_event(
        kind: 'patch_ready',
        role: 'assistant',
        title: '패치 준비 완료',
        body: summary,
        jobId: jobId,
        data: {
          'summary': summary,
          'fileCount': 1,
          'files': [
            {
              'path': 'src/auth/middleware.ts',
              'status': 'modified',
              'hunks': [
                {
                  'hunkId': 'h1',
                  'header': '@@ -12,7 +12,9 @@',
                  'diff': '- if (!token) throw new Error()\n+ if (!token) return res.status(401).send()',
                  'risk': 'low',
                },
              ],
            },
          ],
        },
      ));
    _state = 'patch_ready';
    _lastEventKind = 'patch_ready';
    _lastEventText = summary;
    _updatedAt = _events.last['at'] as int;
  }

  void recordPatch({
    required String threadId,
    required String jobId,
    required String status,
    required String message,
  }) {
    if (_threadId != threadId) {
      return;
    }
    _events.add(_event(
      kind: 'patch_applied',
      role: 'system',
      title: '패치 적용 결과',
      body: message,
      jobId: jobId,
      data: {
        'status': status,
        'message': message,
      },
    ));
    _state = status;
    _lastEventKind = 'patch_applied';
    _lastEventText = message;
    _updatedAt = _events.last['at'] as int;
  }

  void recordRun({
    required String threadId,
    required String jobId,
    required String profileId,
    required String status,
    required String summary,
    required String excerpt,
    required String output,
  }) {
    if (_threadId != threadId) {
      return;
    }
    _events.add(_event(
      kind: 'run_finished',
      role: 'system',
      title: '실행 결과',
      body: summary,
      jobId: jobId,
      data: {
        'status': status,
        'summary': summary,
        'excerpt': excerpt,
        'output': output,
        'profileId': profileId,
        'topErrors': [
          {
            'message': 'expected 401 got 500',
            'path': 'tests/auth/middleware.test.ts',
            'line': 44,
            'column': 13,
          },
        ],
      },
    ));
    _state = status;
    _lastEventKind = 'run_finished';
    _lastEventText = summary;
    _updatedAt = _events.last['at'] as int;
  }

  Map<String, dynamic> threadSummary() {
    return {
      'id': _threadId,
      'title': _title,
      'sessionId': _sessionId,
      'state': _state,
      'currentJobId': _currentJobId,
      'lastEventKind': _lastEventKind,
      'lastEventText': _lastEventText,
      'updatedAt': _updatedAt,
    };
  }

  Map<String, dynamic> _event({
    required String kind,
    required String role,
    required String title,
    required String body,
    required String jobId,
    Map<String, dynamic>? data,
  }) {
    final at = DateTime(2026, 3, 7, 10, 0, _events.length).millisecondsSinceEpoch;
    return {
      'id': 'evt-${_events.length + 1}',
      'threadId': _threadId,
      'jobId': jobId,
      'kind': kind,
      'role': role,
      'title': title,
      'body': body,
      'data': data ?? const <String, dynamic>{},
      'at': at,
    };
  }
}

Map<String, dynamic> cmdAckEnvelope({
  required String sid,
  required String requestRid,
  required int seq,
}) {
  return {
    'sid': sid,
    'rid': 'rid-cmd-ack-$seq',
    'seq': seq,
    'ts': 1700000000000 + seq,
    'type': 'CMD_ACK',
    'payload': {
      'requestRid': requestRid,
      'accepted': true,
      'message': 'ack',
    },
  };
}

Map<String, dynamic> promptAckEnvelope({
  required String sid,
  required String rid,
  required int seq,
  required String threadId,
  required String jobId,
}) {
  return {
    'sid': sid,
    'rid': rid,
    'seq': seq,
    'ts': 1700000001000 + seq,
    'type': 'PROMPT_ACK',
    'payload': {
      'threadId': threadId,
      'jobId': jobId,
      'state': 'patch_ready',
      'message': 'prompt accepted',
    },
  };
}

Map<String, dynamic> patchReadyEnvelope({
  required String sid,
  required String rid,
  required int seq,
  required String summary,
  required String jobId,
}) {
  return {
    'sid': sid,
    'rid': rid,
    'seq': seq,
    'ts': 1700000002000 + seq,
    'type': 'PATCH_READY',
    'payload': {
      'jobId': jobId,
      'summary': summary,
      'files': [
        {
          'path': 'src/auth/middleware.ts',
          'status': 'modified',
          'hunks': [
            {
              'hunkId': 'h1',
              'header': '@@ -12,7 +12,9 @@',
              'diff':
                  '- if (!token) throw new Error()\n+ if (!token) return res.status(401).send()',
              'risk': 'low',
            },
          ],
        },
      ],
    },
  };
}

Map<String, dynamic> patchResultEnvelope({
  required String sid,
  required String rid,
  required int seq,
}) {
  return {
    'sid': sid,
    'rid': rid,
    'seq': seq,
    'ts': 1700000003000 + seq,
    'type': 'PATCH_RESULT',
    'payload': {
      'jobId': 'job-direct-1',
      'status': 'success',
      'message': 'patch applied',
    },
  };
}

Map<String, dynamic> runResultEnvelope({
  required String sid,
  required String rid,
  required int seq,
  required String jobId,
}) {
  return {
    'sid': sid,
    'rid': rid,
    'seq': seq,
    'ts': 1700000004000 + seq,
    'type': 'RUN_RESULT',
    'payload': {
      'jobId': jobId,
      'runId': 'run-test-all-1',
      'profileId': 'test_all',
      'status': 'failed',
      'summary': '1 failing test in auth middleware',
      'topErrors': [
        {
          'message': 'expected 401 got 500',
          'path': 'tests/auth/middleware.test.ts',
          'line': 44,
          'column': 13,
        },
      ],
      'excerpt': 'AssertionError: expected 401 got 500',
      'output': 'FAIL tests/auth/middleware.test.ts\nAssertionError: expected 401 got 500',
    },
  };
}
