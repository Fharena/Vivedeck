import 'dart:async';
import 'dart:convert';
import 'dart:io';

import 'package:flutter_webrtc/flutter_webrtc.dart';

import 'signaling_api.dart';

enum DirectSignalingState {
  idle,
  claiming,
  claimed,
  wsConnecting,
  wsConnected,
  peerInitializing,
  waitingOffer,
  answering,
  peerConnected,
  dataChannelOpen,
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
  final StreamController<Map<String, dynamic>> _controlEnvelopeController =
      StreamController<Map<String, dynamic>>.broadcast();
  final StreamController<String> _errorController =
      StreamController<String>.broadcast();

  WebSocket? _webSocket;
  StreamSubscription? _webSocketSubscription;
  RTCPeerConnection? _peerConnection;
  RTCDataChannel? _dataChannel;

  final List<Map<String, dynamic>> _pendingRemoteIce = [];

  String sessionId = '';
  String mobileDeviceKey = '';

  bool isConnected = false;
  bool isPeerConnected = false;
  bool isDataChannelOpen = false;

  bool _disposed = false;
  bool _remoteOfferApplied = false;
  bool _awaitingControlResponse = false;

  int _signalSeq = 1;

  Stream<DirectSignalingState> get states => _stateController.stream;
  Stream<DirectSignalEvent> get events => _eventController.stream;
  Stream<Map<String, dynamic>> get envelopes => _envelopeController.stream;
  Stream<Map<String, dynamic>> get controlEnvelopes =>
      _controlEnvelopeController.stream;
  Stream<String> get errors => _errorController.stream;

  bool get isControlReady => isConnected && isDataChannelOpen;

  Future<void> connect({
    required String signalingBaseUrl,
    required String pairingCode,
  }) async {
    await close();

    _remoteOfferApplied = false;
    _pendingRemoteIce.clear();

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

    await _initializePeer();

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

  Future<List<Map<String, dynamic>>> sendControlEnvelopeAndAwaitResponses(
    Map<String, dynamic> envelope, {
    Duration timeout = const Duration(seconds: 6),
    Duration quietPeriod = const Duration(milliseconds: 280),
  }) async {
    if (_awaitingControlResponse) {
      throw SignalingApiException(
          0, 'direct control request is already in-flight');
    }

    final requestRid = envelope['rid']?.toString() ?? '';
    if (requestRid.isEmpty) {
      throw SignalingApiException(
          0, 'rid is required for direct control request');
    }

    _awaitingControlResponse = true;

    final responses = <Map<String, dynamic>>[];
    final completer = Completer<List<Map<String, dynamic>>>();

    bool gotAck = false;
    Timer? idleTimer;
    StreamSubscription<Map<String, dynamic>>? sub;

    void cleanup() {
      idleTimer?.cancel();
      sub?.cancel();
      _awaitingControlResponse = false;
    }

    void completeWith(List<Map<String, dynamic>> value) {
      if (!completer.isCompleted) {
        cleanup();
        completer.complete(value);
      }
    }

    void completeError(Object error) {
      if (!completer.isCompleted) {
        cleanup();
        completer.completeError(error);
      }
    }

    void scheduleIdleComplete() {
      idleTimer?.cancel();
      idleTimer = Timer(quietPeriod, () {
        completeWith(List<Map<String, dynamic>>.from(responses));
      });
    }

    sub = _controlEnvelopeController.stream.listen(
      (env) {
        final typ = env['type']?.toString() ?? '';
        if (typ == 'CMD_ACK') {
          final payload = _asMap(env['payload']);
          final ackRID = payload['requestRid']?.toString() ?? '';
          if (ackRID != requestRid) {
            return;
          }
          gotAck = true;
          responses.add(env);
          scheduleIdleComplete();
          return;
        }

        if (!gotAck) {
          return;
        }

        responses.add(env);
        scheduleIdleComplete();
      },
      onError: completeError,
    );

    try {
      await sendControlEnvelope(envelope);
    } catch (e) {
      completeError(e);
    }

    try {
      return await completer.future.timeout(
        timeout,
        onTimeout: () {
          throw SignalingApiException(
            0,
            'direct control response timeout: rid=$requestRid',
          );
        },
      );
    } finally {
      cleanup();
    }
  }

  Future<void> sendControlEnvelope(Map<String, dynamic> envelope) async {
    final channel = _dataChannel;
    if (channel == null || !isDataChannelOpen) {
      throw SignalingApiException(0, 'direct control data channel is not open');
    }

    final data = jsonEncode(envelope);
    channel.send(RTCDataChannelMessage(data));

    final typ = envelope['type']?.toString() ?? 'UNKNOWN';
    _emitEvent('dc 송신: $typ');
  }

  Future<void> close() async {
    await _webSocketSubscription?.cancel();
    _webSocketSubscription = null;

    if (_webSocket != null) {
      await _webSocket!.close();
      _webSocket = null;
    }

    try {
      await _dataChannel?.close();
    } catch (_) {
      // close best-effort
    }
    _dataChannel = null;

    try {
      await _peerConnection?.close();
    } catch (_) {
      // close best-effort
    }
    _peerConnection = null;

    isConnected = false;
    isPeerConnected = false;
    isDataChannelOpen = false;
    _remoteOfferApplied = false;
    _pendingRemoteIce.clear();

    _emitState(DirectSignalingState.closed);
    _emitEvent('direct signaling 세션 종료');
  }

  void dispose() {
    if (_disposed) {
      return;
    }
    _disposed = true;

    unawaited(close());

    _stateController.close();
    _eventController.close();
    _envelopeController.close();
    _controlEnvelopeController.close();
    _errorController.close();
  }

  Future<void> _initializePeer() async {
    _emitState(DirectSignalingState.peerInitializing);
    _emitEvent('peer 초기화 시작');

    try {
      final peer = await createPeerConnection({
        'iceServers': [
          {
            'urls': ['stun:stun.l.google.com:19302'],
          },
        ],
      });

      _peerConnection = peer;

      peer.onIceCandidate = (candidate) {
        if (candidate.candidate == null || candidate.candidate!.isEmpty) {
          return;
        }

        final env = _buildSignalEnvelope(
          type: 'SIGNAL_ICE',
          payload: {
            'candidate': candidate.candidate,
            if (candidate.sdpMid != null) 'sdpMid': candidate.sdpMid,
            if (candidate.sdpMLineIndex != null)
              'sdpMLineIndex': candidate.sdpMLineIndex,
          },
        );

        unawaited(_sendSignalEnvelope(env));
      };

      peer.onConnectionState = (state) {
        final label = state.toString();
        _emitEvent('peer state: $label');

        if (_containsAny(label, const ['connected'])) {
          isPeerConnected = true;
          _emitState(DirectSignalingState.peerConnected);
          return;
        }

        if (_containsAny(label, const ['failed', 'disconnected', 'closed'])) {
          isPeerConnected = false;
          if (!_containsAny(label, const ['closed'])) {
            _emitState(DirectSignalingState.error);
          }
        }
      };

      peer.onDataChannel = (channel) {
        _bindDataChannel(channel);
      };
    } catch (e) {
      _emitState(DirectSignalingState.error);
      _emitError('peer 초기화 실패: $e');
      rethrow;
    }

    _emitState(DirectSignalingState.waitingOffer);
    _emitEvent('peer 초기화 완료, SIGNAL_OFFER 대기');
  }

  Future<void> _onWebSocketData(dynamic data) async {
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
    _emitEvent('ws 수신: $typ');
    _envelopeController.add(envelope);

    try {
      await _handleSignalEnvelope(envelope);
    } catch (e) {
      _emitState(DirectSignalingState.error);
      _emitError('signal 처리 실패($typ): $e');
    }
  }

  Future<void> _handleSignalEnvelope(Map<String, dynamic> envelope) async {
    final typ = envelope['type']?.toString() ?? '';
    if (typ.isEmpty || typ == 'CMD_ACK') {
      return;
    }

    if (typ == 'SIGNAL_READY') {
      _emitEvent('SIGNAL_READY 수신');
      return;
    }

    if (typ == 'SIGNAL_OFFER') {
      await _onSignalOffer(envelope);
      return;
    }

    if (typ == 'SIGNAL_ICE') {
      await _onSignalIce(envelope);
      return;
    }
  }

  Future<void> _onSignalOffer(Map<String, dynamic> envelope) async {
    final pc = _peerConnection;
    if (pc == null) {
      throw SignalingApiException(0, 'peer is not initialized');
    }

    final payload = _asMap(envelope['payload']);
    final sdp = payload['sdp']?.toString() ?? '';
    if (sdp.isEmpty) {
      throw SignalingApiException(0, 'SIGNAL_OFFER payload.sdp is required');
    }

    _emitState(DirectSignalingState.answering);
    _emitEvent('SIGNAL_OFFER 처리 시작');

    await pc.setRemoteDescription(RTCSessionDescription(sdp, 'offer'));
    _remoteOfferApplied = true;

    await _flushPendingRemoteIce();

    final answer = await pc.createAnswer();
    await pc.setLocalDescription(answer);

    final answerSdp = answer.sdp?.toString() ?? '';
    if (answerSdp.isEmpty) {
      throw SignalingApiException(0, 'local answer sdp is empty');
    }

    final answerEnvelope = _buildSignalEnvelope(
      type: 'SIGNAL_ANSWER',
      payload: {'sdp': answerSdp},
    );

    await _sendSignalEnvelope(answerEnvelope);
    _emitEvent('SIGNAL_ANSWER 송신 완료');
    _emitState(DirectSignalingState.peerConnected);
  }

  Future<void> _onSignalIce(Map<String, dynamic> envelope) async {
    final payload = _asMap(envelope['payload']);

    if (!_remoteOfferApplied) {
      _pendingRemoteIce.add(payload);
      _emitEvent('원격 ICE 큐잉(offer 이전)');
      return;
    }

    await _applyRemoteIce(payload);
  }

  Future<void> _flushPendingRemoteIce() async {
    if (_pendingRemoteIce.isEmpty) {
      return;
    }

    final queue = List<Map<String, dynamic>>.from(_pendingRemoteIce);
    _pendingRemoteIce.clear();

    for (final payload in queue) {
      await _applyRemoteIce(payload);
    }
  }

  Future<void> _applyRemoteIce(Map<String, dynamic> payload) async {
    final pc = _peerConnection;
    if (pc == null) {
      throw SignalingApiException(0, 'peer is not initialized');
    }

    final candidate = payload['candidate']?.toString() ?? '';
    if (candidate.isEmpty) {
      return;
    }

    final sdpMid = payload['sdpMid']?.toString();
    final rawLine = payload['sdpMLineIndex'];
    int? sdpMLineIndex;
    if (rawLine is int) {
      sdpMLineIndex = rawLine;
    } else {
      sdpMLineIndex = int.tryParse(rawLine?.toString() ?? '');
    }

    await pc.addCandidate(RTCIceCandidate(candidate, sdpMid, sdpMLineIndex));
    _emitEvent('원격 ICE 적용 완료');
  }

  void _bindDataChannel(RTCDataChannel channel) {
    _dataChannel = channel;
    _emitEvent('data channel 수신: ${channel.label}');

    channel.onDataChannelState = (state) {
      final label = state.toString();
      _emitEvent('data channel state: $label');

      if (_containsAny(label, const ['open'])) {
        isDataChannelOpen = true;
        _emitState(DirectSignalingState.dataChannelOpen);
        return;
      }

      if (_containsAny(label, const ['closing', 'closed'])) {
        isDataChannelOpen = false;
      }
    };

    channel.onMessage = (message) {
      if (message.isBinary) {
        _emitEvent('dc 메시지 무시(binary)');
        return;
      }

      final text = message.text;
      Map<String, dynamic> envelope;
      try {
        final decoded = jsonDecode(text);
        if (decoded is! Map<String, dynamic>) {
          _emitEvent('dc 메시지 무시(non-map json)');
          return;
        }
        envelope = decoded;
      } catch (_) {
        _emitEvent('dc 메시지 파싱 실패');
        return;
      }

      final typ = envelope['type']?.toString() ?? 'UNKNOWN';
      _emitEvent('dc 수신: $typ');
      _envelopeController.add(envelope);
      _controlEnvelopeController.add(envelope);
    };
  }

  Map<String, dynamic> _buildSignalEnvelope({
    required String type,
    required Map<String, dynamic> payload,
  }) {
    final now = DateTime.now().millisecondsSinceEpoch;
    final seq = _signalSeq++;

    return {
      'sid': sessionId,
      'rid': 'mobile_signal_${type.toLowerCase()}_$seq',
      'seq': seq,
      'ts': now,
      'type': type,
      'payload': payload,
    };
  }

  Future<void> _sendSignalEnvelope(Map<String, dynamic> envelope) async {
    final ws = _webSocket;
    if (ws == null || !isConnected) {
      throw SignalingApiException(0, 'signaling ws is not connected');
    }

    final typ = envelope['type']?.toString() ?? 'UNKNOWN';
    ws.add(jsonEncode(envelope));
    _emitEvent('ws 송신: $typ');
  }

  Map<String, dynamic> _asMap(dynamic value) {
    if (value is Map<String, dynamic>) {
      return value;
    }
    if (value is Map) {
      return Map<String, dynamic>.from(value);
    }
    return <String, dynamic>{};
  }

  bool _containsAny(String source, List<String> tokens) {
    final lower = source.toLowerCase();
    for (final token in tokens) {
      if (lower.contains(token.toLowerCase())) {
        return true;
      }
    }
    return false;
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
