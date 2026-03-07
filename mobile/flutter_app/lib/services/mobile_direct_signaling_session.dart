import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'signaling_api.dart';

enum DirectSignalingState {
  idle,
  claiming,
  claimed,
  wsConnecting,
  wsConnected,
  closed,
  error,
}

class DirectSignalEvent {
  const DirectSignalEvent({
    required this.message,
    required this.at,
  });

  final String message;
  final DateTime at;

  String get label {
    final hh = at.hour.toString().padLeft(2, '0');
    final mm = at.minute.toString().padLeft(2, '0');
    final ss = at.second.toString().padLeft(2, '0');
    return '[$hh:$mm:$ss] $message';
  }
}

class MobileDirectSignalingSession {
  MobileDirectSignalingSession({SignalingApi? signalingApi})
      : _signalingApi = signalingApi ?? SignalingApi();

  final SignalingApi _signalingApi;

  final StreamController<DirectSignalingState> _stateController =
      StreamController<DirectSignalingState>.broadcast();
  final StreamController<DirectSignalEvent> _eventController =
      StreamController<DirectSignalEvent>.broadcast();
  final StreamController<Map<String, dynamic>> _envelopeController =
      StreamController<Map<String, dynamic>>.broadcast();
  final StreamController<String> _errorController =
      StreamController<String>.broadcast();

  WebSocket? _webSocket;
  StreamSubscription? _webSocketSubscription;

  String sessionId = '';
  String mobileDeviceKey = '';
  bool isConnected = false;

  Stream<DirectSignalingState> get states => _stateController.stream;
  Stream<DirectSignalEvent> get events => _eventController.stream;
  Stream<Map<String, dynamic>> get envelopes => _envelopeController.stream;
  Stream<String> get errors => _errorController.stream;

  Future<void> connect({
    required String signalingBaseUrl,
    required String pairingCode,
  }) async {
    await close();

    _emitState(DirectSignalingState.claiming);
    _emitEvent('pairing claim 시작: ${pairingCode.trim()}');

    final claim =
        await _signalingApi.claimPairing(signalingBaseUrl, pairingCode);
    sessionId = claim.sessionId;
    mobileDeviceKey = claim.mobileDeviceKey;

    _emitState(DirectSignalingState.claimed);
    _emitEvent('pairing claim 성공: sid=$sessionId');

    final wsUri = _signalingApi.buildSessionWebSocketUri(
      signalingBaseUrl: signalingBaseUrl,
      sessionId: sessionId,
      deviceKey: mobileDeviceKey,
      role: 'mobile',
    );

    _emitState(DirectSignalingState.wsConnecting);
    _emitEvent('ws 연결 시도: $wsUri');

    try {
      _webSocket = await WebSocket.connect(wsUri.toString());
    } catch (e) {
      _emitState(DirectSignalingState.error);
      _emitError('ws 연결 실패: $e');
      rethrow;
    }

    isConnected = true;
    _emitState(DirectSignalingState.wsConnected);
    _emitEvent('ws 연결 완료');

    _webSocketSubscription = _webSocket!.listen(
      _onWebSocketData,
      onError: (Object error) {
        isConnected = false;
        _emitState(DirectSignalingState.error);
        _emitError('ws 에러: $error');
      },
      onDone: () {
        isConnected = false;
        _emitState(DirectSignalingState.closed);
        _emitEvent('ws 종료: code=${_webSocket?.closeCode}');
      },
      cancelOnError: false,
    );
  }

  Future<void> sendEnvelope(Map<String, dynamic> envelope) async {
    final ws = _webSocket;
    if (ws == null || !isConnected) {
      throw SignalingApiException(0, 'direct signaling ws is not connected');
    }

    final data = jsonEncode(envelope);
    ws.add(data);
  }

  Future<void> close() async {
    await _webSocketSubscription?.cancel();
    _webSocketSubscription = null;

    if (_webSocket != null) {
      await _webSocket!.close();
      _webSocket = null;
    }

    if (isConnected) {
      isConnected = false;
      _emitState(DirectSignalingState.closed);
      _emitEvent('direct signaling 세션 종료');
    }
  }

  void dispose() {
    unawaited(close());
    _stateController.close();
    _eventController.close();
    _envelopeController.close();
    _errorController.close();
  }

  void _onWebSocketData(dynamic data) {
    if (data is! String) {
      _emitEvent('ws 메시지 무시(non-string)');
      return;
    }

    Map<String, dynamic> envelope;
    try {
      final decoded = jsonDecode(data);
      if (decoded is! Map<String, dynamic>) {
        _emitEvent('ws 메시지 무시(non-map json)');
        return;
      }
      envelope = decoded;
    } catch (_) {
      _emitEvent('ws 메시지 파싱 실패');
      return;
    }

    final typ = envelope['type']?.toString() ?? 'UNKNOWN';
    _emitEvent('수신: $typ');
    _envelopeController.add(envelope);
  }

  void _emitState(DirectSignalingState state) {
    if (!_stateController.isClosed) {
      _stateController.add(state);
    }
  }

  void _emitEvent(String message) {
    if (!_eventController.isClosed) {
      _eventController.add(
        DirectSignalEvent(
          message: message,
          at: DateTime.now(),
        ),
      );
    }
  }

  void _emitError(String message) {
    if (!_errorController.isClosed) {
      _errorController.add(message);
    }
  }
}
