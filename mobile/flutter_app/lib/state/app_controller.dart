import 'dart:async';
import 'dart:collection';

import 'package:flutter/foundation.dart';

import '../services/agent_api.dart';
import '../services/mobile_direct_signaling_session.dart';
import '../services/signaling_api.dart';

class AppController extends ChangeNotifier {
  AppController({AgentApi? api}) : _api = api ?? AgentApi();

  final AgentApi _api;

  String agentBaseUrl = 'http://127.0.0.1:8080';
  String signalingBaseUrl = 'http://127.0.0.1:8081';

  bool isLoading = false;
  String activity = '';
  String? errorMessage;

  bool p2pActive = false;
  String sessionId = '';
  String pairingCode = '';
  String connectionState = 'PAIRING';

  int pendingAckCount = 0;
  UnmodifiableListView<RuntimeTransitionView> get runtimeHistory =>
      UnmodifiableListView(_runtimeHistory);
  final List<RuntimeTransitionView> _runtimeHistory = [];

  String promptDraft = '';
  String? currentJobId;
  String patchSummary = '';
  String patchResultStatus = '';
  String patchResultMessage = '';
  String runStatus = '';
  String runSummary = '';
  final List<String> topErrors = [];

  UnmodifiableListView<PatchFileView> get patchFiles =>
      UnmodifiableListView(_patchFiles);
  final List<PatchFileView> _patchFiles = [];

  String directPairingCode = '';
  String directSignalingState = 'IDLE';
  bool directSignalingConnected = false;
  String directSessionId = '';
  String directDeviceKey = '';
  UnmodifiableListView<String> get directSignalLogs =>
      UnmodifiableListView(_directSignalLogs);
  final List<String> _directSignalLogs = [];

  MobileDirectSignalingSession? _directSession;
  StreamSubscription<DirectSignalingState>? _directStateSub;
  StreamSubscription<DirectSignalEvent>? _directEventSub;
  StreamSubscription<Map<String, dynamic>>? _directEnvelopeSub;
  StreamSubscription<String>? _directErrorSub;

  int _seq = 1;

  void updateAgentBaseUrl(String value) {
    agentBaseUrl = value.trim();
    notifyListeners();
  }

  void updateSignalingBaseUrl(String value) {
    signalingBaseUrl = value.trim();
    notifyListeners();
  }

  void updateDirectPairingCode(String value) {
    directPairingCode = value.trim();
    notifyListeners();
  }

  Future<void> refreshStatus() {
    return _run('상태 갱신', _refreshStatusRaw);
  }

  Future<void> startP2P() {
    return _run('P2P 시작', () async {
      await _api.p2pStart(
        agentBaseUrl,
        signalingBaseUrl: signalingBaseUrl,
      );
      await _refreshStatusRaw();
    });
  }

  Future<void> stopP2P() {
    return _run('P2P 종료', () async {
      await _api.p2pStop(agentBaseUrl);
      await _refreshStatusRaw();
    });
  }

  Future<void> connectDirectSignaling() {
    final code = _resolveDirectPairingCode();
    if (code.isEmpty) {
      errorMessage = 'pairing code를 입력하거나 P2P 시작 후 발급 코드를 사용하세요.';
      notifyListeners();
      return Future.value();
    }

    return _run('Direct signaling 연결', () async {
      await _closeDirectSignalingSession();

      final session = MobileDirectSignalingSession();
      _directSession = session;
      _bindDirectSession(session);

      directPairingCode = code;
      _appendDirectLog('direct connect 시작 (code=$code)');

      await session.connect(
        signalingBaseUrl: signalingBaseUrl,
        pairingCode: code,
      );

      directSignalingConnected = session.isConnected;
      directSessionId = session.sessionId;
      directDeviceKey = session.mobileDeviceKey;
      notifyListeners();
    });
  }

  Future<void> disconnectDirectSignaling() {
    return _run('Direct signaling 종료', () async {
      await _closeDirectSignalingSession();
      directSignalingConnected = false;
      directSignalingState = 'CLOSED';
      notifyListeners();
    });
  }

  Future<void> submitPrompt({
    required String prompt,
    required String template,
    required Map<String, bool> context,
  }) {
    if (prompt.trim().isEmpty) {
      errorMessage = 'prompt를 입력해주세요.';
      notifyListeners();
      return Future.value();
    }

    return _run('Prompt 제출', () async {
      final envelope = _buildEnvelope(
        type: 'PROMPT_SUBMIT',
        payload: {
          'prompt': prompt.trim(),
          'template': template,
          'contextOptions': {
            'includeActiveFile': context['activeFile'] ?? false,
            'includeSelection': context['selection'] ?? false,
            'includeLatestError': context['latestError'] ?? false,
            'includeWorkspaceSummary': context['workspaceSummary'] ?? false,
          },
        },
      );

      promptDraft = prompt.trim();
      final responses = await _sendEnvelopeAndAck(envelope);
      _applyResponses(responses);
      await _refreshStatusRaw();
    });
  }

  Future<void> applyPatch({
    required bool applyAll,
    required Map<String, Set<String>> selectedByPath,
  }) {
    if ((currentJobId ?? '').isEmpty) {
      errorMessage = '적용할 job이 없습니다. Prompt를 먼저 제출해주세요.';
      notifyListeners();
      return Future.value();
    }

    final selected = <Map<String, dynamic>>[];
    if (!applyAll) {
      for (final entry in selectedByPath.entries) {
        if (entry.value.isEmpty) {
          continue;
        }
        selected.add(
          {
            'path': entry.key,
            'hunkIds': entry.value.toList(),
          },
        );
      }
    }

    return _run('Patch 적용', () async {
      final envelope = _buildEnvelope(
        type: 'PATCH_APPLY',
        payload: {
          'jobId': currentJobId,
          'mode': applyAll ? 'all' : 'selected',
          if (!applyAll) 'selected': selected,
        },
      );

      final responses = await _sendEnvelopeAndAck(envelope);
      _applyResponses(responses);
      await _refreshStatusRaw();
    });
  }

  Future<void> runProfile(String profileId) {
    if ((currentJobId ?? '').isEmpty) {
      errorMessage = '실행할 job이 없습니다. Prompt를 먼저 제출해주세요.';
      notifyListeners();
      return Future.value();
    }

    return _run('프로파일 실행', () async {
      final envelope = _buildEnvelope(
        type: 'RUN_PROFILE',
        payload: {
          'jobId': currentJobId,
          'profileId': profileId,
        },
      );

      final responses = await _sendEnvelopeAndAck(envelope);
      _applyResponses(responses);
      await _refreshStatusRaw();
    });
  }

  Future<void> _refreshStatusRaw() async {
    final p2p = await _api.p2pStatus(agentBaseUrl);
    p2pActive = p2p['active'] == true;
    sessionId = p2p['sessionId']?.toString() ?? '';
    pairingCode = p2p['pairingCode']?.toString() ?? '';

    if (directPairingCode.isEmpty && pairingCode.isNotEmpty) {
      directPairingCode = pairingCode;
    }

    final runtime = await _api.runtimeState(agentBaseUrl);
    connectionState = runtime['state']?.toString() ?? connectionState;
    _runtimeHistory
      ..clear()
      ..addAll(_parseHistory(runtime['history']));

    final pending = await _api.pendingAcks(agentBaseUrl);
    pendingAckCount = _toInt(pending['count']);
  }

  Future<List<Map<String, dynamic>>> _sendEnvelopeAndAck(
    Map<String, dynamic> envelope,
  ) async {
    final response = await _api.sendEnvelope(agentBaseUrl, envelope);
    final raw = response['responses'];
    if (raw is! List) {
      return const [];
    }

    final responses = raw
        .whereType<Map>()
        .map((item) => Map<String, dynamic>.from(item))
        .toList();

    for (final env in responses) {
      final typ = env['type']?.toString() ?? '';
      if (typ == 'CMD_ACK') {
        continue;
      }
      final rid = env['rid']?.toString() ?? '';
      final sid = env['sid']?.toString() ?? _sid();
      if (rid.isEmpty) {
        continue;
      }

      final ackEnvelope = _buildEnvelope(
        sid: sid,
        type: 'CMD_ACK',
        payload: {
          'requestRid': rid,
          'accepted': true,
          'message': 'received by mobile',
        },
      );

      try {
        await _api.sendEnvelope(agentBaseUrl, ackEnvelope);
      } catch (_) {
        // ACK 실패는 다음 refresh에서 pending으로 관측된다.
      }
    }

    return responses;
  }

  void _applyResponses(List<Map<String, dynamic>> responses) {
    for (final response in responses) {
      final type = response['type']?.toString() ?? '';
      final payload = response['payload'];
      if (payload is! Map) {
        continue;
      }
      final map = Map<String, dynamic>.from(payload);

      if (type == 'PROMPT_ACK') {
        currentJobId = map['jobId']?.toString();
      }

      if (type == 'PATCH_READY') {
        patchSummary = map['summary']?.toString() ?? '';
        _patchFiles
          ..clear()
          ..addAll(_parsePatchFiles(map['files']));
      }

      if (type == 'PATCH_RESULT') {
        patchResultStatus = map['status']?.toString() ?? '';
        patchResultMessage = map['message']?.toString() ?? '';
      }

      if (type == 'RUN_RESULT') {
        runStatus = map['status']?.toString() ?? '';
        runSummary = map['summary']?.toString() ?? '';
        topErrors
          ..clear()
          ..addAll(_parseTopErrors(map['topErrors']));
      }
    }
  }

  List<RuntimeTransitionView> _parseHistory(dynamic raw) {
    if (raw is! List) {
      return const [];
    }

    return raw
        .whereType<Map>()
        .map(
          (item) => RuntimeTransitionView(
            state: item['state']?.toString() ?? '',
            note: item['note']?.toString() ?? '',
            atMillis: _toInt(item['at']),
          ),
        )
        .toList();
  }

  List<PatchFileView> _parsePatchFiles(dynamic raw) {
    if (raw is! List) {
      return const [];
    }

    return raw.whereType<Map>().map((file) {
      final hunksRaw = file['hunks'];
      final hunks = <PatchHunkView>[];
      if (hunksRaw is List) {
        for (final hunk in hunksRaw.whereType<Map>()) {
          hunks.add(
            PatchHunkView(
              id: hunk['hunkId']?.toString() ?? '',
              header: hunk['header']?.toString() ?? '',
              diff: hunk['diff']?.toString() ?? '',
              risk: hunk['risk']?.toString() ?? '',
            ),
          );
        }
      }

      return PatchFileView(
        path: file['path']?.toString() ?? '',
        status: file['status']?.toString() ?? '',
        hunks: hunks,
      );
    }).toList();
  }

  List<String> _parseTopErrors(dynamic raw) {
    if (raw is! List) {
      return const [];
    }

    return raw.whereType<Map>().map((item) {
      final message = item['message']?.toString() ?? '';
      final path = item['path']?.toString() ?? '';
      final line = _toInt(item['line']);
      if (path.isEmpty) {
        return message;
      }
      return '$path:$line $message';
    }).toList();
  }

  Map<String, dynamic> _buildEnvelope({
    String? sid,
    required String type,
    required Map<String, dynamic> payload,
  }) {
    final now = DateTime.now().millisecondsSinceEpoch;
    final seq = _seq++;
    final rid = 'rid_${type.toLowerCase()}_$seq';

    return {
      'sid': sid ?? _sid(),
      'rid': rid,
      'seq': seq,
      'ts': now,
      'type': type,
      'payload': payload,
    };
  }

  String _sid() {
    if (sessionId.isNotEmpty) {
      return sessionId;
    }
    return 'sid-mobile-demo';
  }

  int _toInt(dynamic value) {
    if (value is int) {
      return value;
    }
    if (value is num) {
      return value.toInt();
    }
    return int.tryParse(value?.toString() ?? '') ?? 0;
  }

  Future<void> _run(String name, Future<void> Function() action) async {
    isLoading = true;
    activity = name;
    errorMessage = null;
    notifyListeners();

    try {
      await action();
    } on AgentApiException catch (e) {
      errorMessage = '[${e.statusCode}] ${e.message}';
    } on SignalingApiException catch (e) {
      errorMessage = '[${e.statusCode}] ${e.message}';
    } catch (e) {
      errorMessage = e.toString();
    } finally {
      isLoading = false;
      activity = '';
      notifyListeners();
    }
  }

  void _bindDirectSession(MobileDirectSignalingSession session) {
    _directStateSub = session.states.listen((state) {
      directSignalingState = state.name.toUpperCase();
      directSignalingConnected = session.isConnected;
      notifyListeners();
    });

    _directEventSub = session.events.listen((event) {
      _appendDirectLog(event.label);
      notifyListeners();
    });

    _directEnvelopeSub = session.envelopes.listen((envelope) {
      _handleDirectEnvelope(envelope);
      notifyListeners();
    });

    _directErrorSub = session.errors.listen((message) {
      _appendDirectLog('ERROR: $message');
      errorMessage = message;
      notifyListeners();
    });
  }

  Future<void> _closeDirectSignalingSession() async {
    await _directStateSub?.cancel();
    await _directEventSub?.cancel();
    await _directEnvelopeSub?.cancel();
    await _directErrorSub?.cancel();
    _directStateSub = null;
    _directEventSub = null;
    _directEnvelopeSub = null;
    _directErrorSub = null;

    final session = _directSession;
    _directSession = null;
    if (session != null) {
      await session.close();
      session.dispose();
    }

    directSignalingConnected = false;
    directSessionId = '';
    directDeviceKey = '';
  }

  void _handleDirectEnvelope(Map<String, dynamic> envelope) {
    final typ = envelope['type']?.toString() ?? 'UNKNOWN';
    final sid = envelope['sid']?.toString() ?? '';
    _appendDirectLog('ENVELOPE: $typ sid=$sid');

    if (typ == 'SIGNAL_OFFER') {
      _appendDirectLog('모바일 WebRTC peer 미연동: SIGNAL_OFFER 수신(스켈레톤 단계)');
    }
    if (typ == 'SIGNAL_ICE') {
      _appendDirectLog('모바일 WebRTC peer 미연동: SIGNAL_ICE 수신(스켈레톤 단계)');
    }
  }

  void _appendDirectLog(String log) {
    _directSignalLogs.insert(0, log);
    if (_directSignalLogs.length > 40) {
      _directSignalLogs.removeRange(40, _directSignalLogs.length);
    }
  }

  String _resolveDirectPairingCode() {
    if (directPairingCode.trim().isNotEmpty) {
      return directPairingCode.trim();
    }
    if (pairingCode.trim().isNotEmpty) {
      return pairingCode.trim();
    }
    return '';
  }

  @override
  void dispose() {
    unawaited(_closeDirectSignalingSession());
    _api.dispose();
    super.dispose();
  }
}

class PatchFileView {
  const PatchFileView({
    required this.path,
    required this.status,
    required this.hunks,
  });

  final String path;
  final String status;
  final List<PatchHunkView> hunks;
}

class PatchHunkView {
  const PatchHunkView({
    required this.id,
    required this.header,
    required this.diff,
    required this.risk,
  });

  final String id;
  final String header;
  final String diff;
  final String risk;
}

class RuntimeTransitionView {
  const RuntimeTransitionView({
    required this.state,
    required this.note,
    required this.atMillis,
  });

  final String state;
  final String note;
  final int atMillis;

  String get atLabel {
    if (atMillis <= 0) {
      return '-';
    }

    final dt = DateTime.fromMillisecondsSinceEpoch(atMillis);
    final hh = dt.hour.toString().padLeft(2, '0');
    final mm = dt.minute.toString().padLeft(2, '0');
    final ss = dt.second.toString().padLeft(2, '0');
    return '$hh:$mm:$ss';
  }
}
