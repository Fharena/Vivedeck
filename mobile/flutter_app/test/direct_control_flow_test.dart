import 'dart:async';

import 'package:flutter_test/flutter_test.dart';
import 'package:vibedeck_mobile/app.dart';
import 'package:vibedeck_mobile/services/agent_api.dart';
import 'package:vibedeck_mobile/services/mobile_direct_signaling_session.dart';
import 'package:vibedeck_mobile/services/signaling_api.dart';
import 'package:vibedeck_mobile/state/app_controller.dart';

void main() {
  testWidgets('drives prompt patch run loop over direct control path', (
    tester,
  ) async {
    final agentApi = FakeAgentApi();
    final directSession = FakeDirectSession();
    final controller = AppController(
      api: agentApi,
      directSessionFactory: () => directSession,
    );

    addTearDown(controller.dispose);

    await tester.pumpWidget(VibeDeckApp(controller: controller));
    await tester.pumpAndSettle();

    controller.updateDirectPairingCode('PAIR123');
    await controller.connectDirectSignaling();
    await tester.pumpAndSettle();

    expect(controller.directControlReady, isTrue);
    expect(controller.controlPath, 'DIRECT');

    await controller.submitPrompt(
      prompt: 'auth middleware 버그 수정',
      template: 'fix_bug',
      context: const {
        'activeFile': true,
        'selection': true,
        'latestError': true,
        'workspaceSummary': false,
      },
    );
    await tester.pumpAndSettle();

    expect(controller.currentJobId, 'job-direct-1');
    expect(controller.patchSummary, 'Mock patch for direct flow');
    expect(controller.patchFiles, hasLength(1));

    await tester.tap(find.text('Review').last);
    await tester.pumpAndSettle();

    expect(find.text('src/auth/middleware.ts'), findsOneWidget);

    await tester.tap(find.text('전체 적용'));
    await tester.pumpAndSettle();

    expect(controller.patchResultStatus, 'success');
    expect(find.textContaining('PATCH_RESULT: success'), findsWidgets);

    await tester.tap(find.text('test_all 실행'));
    await tester.pumpAndSettle();

    expect(controller.runStatus, 'failed');
    expect(find.textContaining('RUN_RESULT: failed'), findsWidgets);
    expect(find.textContaining('1 failing test in auth middleware'),
        findsOneWidget);

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
    final agentApi = FakeAgentApi();
    final directSession = FakeDirectSession(failRequests: true);
    final controller = AppController(
      api: agentApi,
      directSessionFactory: () => directSession,
    );

    addTearDown(controller.dispose);

    controller.updateDirectPairingCode('PAIR123');
    await controller.connectDirectSignaling();
    await controller.submitPrompt(
      prompt: 'auth middleware 버그 수정',
      template: 'fix_bug',
      context: const {
        'activeFile': true,
        'selection': true,
        'latestError': true,
        'workspaceSummary': false,
      },
    );

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
  FakeAgentApi();

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
  Future<Map<String, dynamic>> sendEnvelope(
    String baseUrl,
    Map<String, dynamic> envelope,
  ) async {
    sentEnvelopes.add(Map<String, dynamic>.from(envelope));

    final type = envelope['type']?.toString() ?? '';
    if (type == 'PROMPT_SUBMIT') {
      final sid = envelope['sid']?.toString() ?? 'sid-http-1';
      final requestRid = envelope['rid']?.toString() ?? 'rid-http-submit';
      return {
        'responses': [
          cmdAckEnvelope(sid: sid, requestRid: requestRid, seq: 10),
          promptAckEnvelope(
            sid: sid,
            rid: 'rid-prompt-ack-http-1',
            seq: 11,
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
  FakeDirectSession({this.failRequests = false});

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
        return [
          cmdAckEnvelope(sid: sid, requestRid: requestRid, seq: 20),
          promptAckEnvelope(
            sid: sid,
            rid: 'rid-prompt-ack-direct-1',
            seq: 21,
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
        return [
          cmdAckEnvelope(sid: sid, requestRid: requestRid, seq: 30),
          patchResultEnvelope(
            sid: sid,
            rid: 'rid-patch-result-direct-1',
            seq: 31,
          ),
        ];
      case 'RUN_PROFILE':
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
  required String jobId,
}) {
  return {
    'sid': sid,
    'rid': rid,
    'seq': seq,
    'ts': 1700000001000 + seq,
    'type': 'PROMPT_ACK',
    'payload': {
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
    },
  };
}
