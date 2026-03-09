import 'package:flutter_test/flutter_test.dart';
import 'package:vibedeck_mobile/services/agent_api.dart';
import 'package:vibedeck_mobile/state/app_controller.dart';

void main() {
  test('shows no-patch reason when agent returns markdown-only summary', () async {
    final controller = AppController(api: _FakeReviewAgentApi.noPatch());
    addTearDown(controller.dispose);

    await controller.submitPrompt(
      prompt: '삼전 주식동향 요약 md 만들어줘',
      context: const {
        'activeFile': false,
        'selection': false,
        'latestError': false,
        'workspaceSummary': false,
      },
    );

    expect(controller.patchFiles, isEmpty);
    expect(controller.patchAvailabilityReason, contains('코드 변경 없이 완료'));
  });

  test('tracks current job files separately from raw run output', () async {
    final controller = AppController(api: _FakeReviewAgentApi.withPatch());
    addTearDown(controller.dispose);

    await controller.submitPrompt(
      prompt: '계산기 파일 만들어줘',
      context: const {
        'activeFile': false,
        'selection': false,
        'latestError': false,
        'workspaceSummary': false,
      },
    );
    await controller.runProfile('test_all');

    expect(controller.currentJobFiles, ['src/calculator.py']);
    expect(controller.runOutput, contains('README.md'));
  });
}

class _FakeReviewAgentApi extends AgentApi {
  _FakeReviewAgentApi._({required this.withPatch});

  factory _FakeReviewAgentApi.noPatch() => _FakeReviewAgentApi._(withPatch: false);
  factory _FakeReviewAgentApi.withPatch() => _FakeReviewAgentApi._(withPatch: true);

  final bool withPatch;

  String _threadId = '';
  String _jobId = '';
  String _patchSummary = '';
  List<Map<String, dynamic>> _patchFiles = const [];
  String _runStatus = '';
  String _runSummary = '';
  String _runOutput = '';
  List<String> _runChangedFiles = const [];

  @override
  Future<Map<String, dynamic>> bootstrap(String baseUrl) async {
    return {
      'agentBaseUrl': 'http://127.0.0.1:8080',
      'signalingBaseUrl': 'http://127.0.0.1:8081',
      'workspaceRoot': 'C:/demo/workspace',
      'currentThreadId': _threadId,
      'adapter': {
        'name': 'cursor-agent-cli',
        'mode': 'cursor_agent_cli',
        'provider': 'cursor',
        'ready': true,
      },
      'recentThreads': _threadId.isEmpty
          ? const []
          : [
              {
                'id': _threadId,
                'title': '리뷰 테스트',
                'updatedAt': DateTime(2026, 3, 9, 11, 20).millisecondsSinceEpoch,
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
        'pendingByTransport': {'http': 0, 'p2p': 0, 'unknown': 0},
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
      'notes': const ['review feedback test'],
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
    if (_threadId.isEmpty) {
      return {'threads': const []};
    }
    return {
      'threads': [
        {
          'id': _threadId,
          'title': '리뷰 테스트',
          'sessionId': 'sid-review',
          'state': _runStatus.isEmpty ? 'patch_ready' : _runStatus,
          'currentJobId': _jobId,
          'lastEventKind': _runStatus.isEmpty ? 'patch_ready' : 'run_finished',
          'lastEventText': _runStatus.isEmpty ? _patchSummary : _runSummary,
          'updatedAt': DateTime(2026, 3, 9, 11, 20).millisecondsSinceEpoch,
        },
      ],
    };
  }

  @override
  Future<Map<String, dynamic>> threadDetail(String baseUrl, String threadId) async {
    final events = <Map<String, dynamic>>[];
    if (_threadId.isNotEmpty) {
      events.add({
        'id': 'evt-1',
        'threadId': _threadId,
        'jobId': _jobId,
        'kind': 'patch_ready',
        'role': 'assistant',
        'title': '패치 준비 완료',
        'body': _patchSummary,
        'data': {
          'summary': _patchSummary,
          'files': _patchFiles,
        },
        'at': DateTime(2026, 3, 9, 11, 20).millisecondsSinceEpoch,
      });
    }
    if (_runStatus.isNotEmpty) {
      events.add({
        'id': 'evt-2',
        'threadId': _threadId,
        'jobId': _jobId,
        'kind': 'run_finished',
        'role': 'system',
        'title': '실행 결과',
        'body': _runSummary,
        'data': {
          'status': _runStatus,
          'summary': _runSummary,
          'excerpt': _runOutput,
          'output': _runOutput,
          'changedFiles': _runChangedFiles,
          'topErrors': const [],
        },
        'at': DateTime(2026, 3, 9, 11, 21).millisecondsSinceEpoch,
      });
    }

    return {
      'thread': {
        'id': _threadId,
        'title': '리뷰 테스트',
        'sessionId': 'sid-review',
        'state': _runStatus.isEmpty ? 'patch_ready' : _runStatus,
        'currentJobId': _jobId,
        'lastEventKind': _runStatus.isEmpty ? 'patch_ready' : 'run_finished',
        'lastEventText': _runStatus.isEmpty ? _patchSummary : _runSummary,
        'updatedAt': DateTime(2026, 3, 9, 11, 21).millisecondsSinceEpoch,
      },
      'events': events,
    };
  }

  @override
  Future<Map<String, dynamic>> sendEnvelope(
    String baseUrl,
    Map<String, dynamic> envelope,
  ) async {
    final type = envelope['type']?.toString() ?? '';
    final sid = envelope['sid']?.toString() ?? 'sid-review';
    final requestRid = envelope['rid']?.toString() ?? 'rid-review';

    if (type == 'PROMPT_SUBMIT') {
      _threadId = 'thread-review-1';
      _jobId = 'job-review-1';
      _patchSummary = withPatch
          ? 'Calculator patch ready'
          : 'Cursor Agent completed without code changes';
      _patchFiles = withPatch
          ? const [
              {
                'path': 'src/calculator.py',
                'status': 'added',
                'hunks': [
                  {
                    'hunkId': 'hunk-1',
                    'header': '@@ -0,0 +1,3 @@',
                    'diff': '+print("hello")',
                    'risk': 'low',
                  },
                ],
              },
            ]
          : const [];

      return {
        'responses': [
          {
            'sid': sid,
            'rid': 'rid-cmd-ack-review',
            'seq': 1,
            'ts': 1700000000001,
            'type': 'CMD_ACK',
            'payload': {
              'requestRid': requestRid,
              'accepted': true,
              'message': 'accepted',
            },
          },
          {
            'sid': sid,
            'rid': 'rid-prompt-ack-review',
            'seq': 2,
            'ts': 1700000000002,
            'type': 'PROMPT_ACK',
            'payload': {
              'threadId': _threadId,
              'jobId': _jobId,
              'accepted': true,
              'message': 'task started',
            },
          },
          {
            'sid': sid,
            'rid': 'rid-patch-ready-review',
            'seq': 3,
            'ts': 1700000000003,
            'type': 'PATCH_READY',
            'payload': {
              'jobId': _jobId,
              'summary': _patchSummary,
              'files': _patchFiles,
            },
          },
        ],
      };
    }

    if (type == 'RUN_PROFILE') {
      _runStatus = 'passed';
      _runSummary = 'git status captured';
      _runOutput = 'M notes.txt\n?? README.md\n?? calculator.py';
      _runChangedFiles = const ['src/calculator.py'];
      return {
        'responses': [
          {
            'sid': sid,
            'rid': 'rid-cmd-ack-run',
            'seq': 4,
            'ts': 1700000000004,
            'type': 'CMD_ACK',
            'payload': {
              'requestRid': requestRid,
              'accepted': true,
              'message': 'accepted',
            },
          },
          {
            'sid': sid,
            'rid': 'rid-run-result-review',
            'seq': 5,
            'ts': 1700000000005,
            'type': 'RUN_RESULT',
            'payload': {
              'jobId': _jobId,
              'profileId': 'test_all',
              'status': _runStatus,
              'summary': _runSummary,
              'excerpt': _runOutput,
              'output': _runOutput,
              'changedFiles': _runChangedFiles,
              'topErrors': const [],
            },
          },
        ],
      };
    }

    return {
      'responses': [
        {
          'sid': sid,
          'rid': 'rid-cmd-ack-generic',
          'seq': 10,
          'ts': 1700000000010,
          'type': 'CMD_ACK',
          'payload': {
            'requestRid': requestRid,
            'accepted': true,
            'message': 'accepted',
          },
        },
      ],
    };
  }

  @override
  void dispose() {}
}
