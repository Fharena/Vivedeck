import 'dart:convert';

import 'package:http/http.dart' as http;

class SignalingApiException implements Exception {
  SignalingApiException(this.statusCode, this.message);

  final int statusCode;
  final String message;

  @override
  String toString() => 'SignalingApiException($statusCode): $message';
}

class SignalingClaimResult {
  const SignalingClaimResult({
    required this.sessionId,
    required this.mobileDeviceKey,
  });

  final String sessionId;
  final String mobileDeviceKey;
}

class SignalingApi {
  SignalingApi({http.Client? client}) : _client = client ?? http.Client();

  final http.Client _client;

  Future<SignalingClaimResult> claimPairing(
    String signalingBaseUrl,
    String pairingCode,
  ) async {
    final code = pairingCode.trim();
    if (code.isEmpty) {
      throw SignalingApiException(0, 'pairing code is empty');
    }

    final url = _joinPath(signalingBaseUrl, '/v1/pairings/$code/claim');
    final response = await _client.post(url);
    final payload = _decode(response);
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw SignalingApiException(
        response.statusCode,
        payload['error']?.toString() ?? payload.toString(),
      );
    }

    final sessionId = payload['sessionId']?.toString() ?? '';
    final mobileDeviceKey = payload['mobileDeviceKey']?.toString() ?? '';
    if (sessionId.isEmpty || mobileDeviceKey.isEmpty) {
      throw SignalingApiException(
          response.statusCode, 'invalid claim response');
    }

    return SignalingClaimResult(
      sessionId: sessionId,
      mobileDeviceKey: mobileDeviceKey,
    );
  }

  Uri buildSessionWebSocketUri({
    required String signalingBaseUrl,
    required String sessionId,
    required String deviceKey,
    required String role,
  }) {
    final base = Uri.parse(signalingBaseUrl.trim());

    String scheme = base.scheme;
    if (scheme == 'http') {
      scheme = 'ws';
    } else if (scheme == 'https') {
      scheme = 'wss';
    } else if (scheme != 'ws' && scheme != 'wss') {
      throw SignalingApiException(
          0, 'unsupported signaling scheme: ${base.scheme}');
    }

    final normalizedPath = base.path.replaceFirst(RegExp(r'/+$'), '');
    final wsPath = '$normalizedPath/v1/sessions/$sessionId/ws';

    return base.replace(
      scheme: scheme,
      path: wsPath,
      queryParameters: <String, String>{
        'deviceKey': deviceKey,
        'role': role,
      },
    );
  }

  Uri _joinPath(String baseUrl, String path) {
    final base = Uri.parse(baseUrl.trim());
    final normalizedPath = base.path.replaceFirst(RegExp(r'/+$'), '');
    return base.replace(path: '$normalizedPath$path');
  }

  Map<String, dynamic> _decode(http.Response response) {
    final text = utf8.decode(response.bodyBytes);
    if (text.isEmpty) {
      return <String, dynamic>{};
    }

    final decoded = jsonDecode(text);
    if (decoded is Map<String, dynamic>) {
      return decoded;
    }
    return <String, dynamic>{'data': decoded};
  }
}
