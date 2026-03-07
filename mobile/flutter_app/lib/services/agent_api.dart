import 'dart:convert';

import 'package:http/http.dart' as http;

class AgentApiException implements Exception {
  AgentApiException(this.statusCode, this.message);

  final int statusCode;
  final String message;

  @override
  String toString() => 'AgentApiException($statusCode): $message';
}

class AgentApi {
  AgentApi({http.Client? client}) : _client = client ?? http.Client();

  final http.Client _client;

  Future<Map<String, dynamic>> p2pStart(
    String baseUrl, {
    String? signalingBaseUrl,
  }) {
    final body = <String, dynamic>{};
    if (signalingBaseUrl != null && signalingBaseUrl.trim().isNotEmpty) {
      body['signalingBaseUrl'] = signalingBaseUrl.trim();
    }

    return _request(
      method: 'POST',
      baseUrl: baseUrl,
      path: '/v1/agent/p2p/start',
      body: body,
    );
  }

  Future<Map<String, dynamic>> p2pStatus(String baseUrl) {
    return _request(
      method: 'GET',
      baseUrl: baseUrl,
      path: '/v1/agent/p2p/status',
    );
  }

  Future<Map<String, dynamic>> p2pStop(String baseUrl) {
    return _request(
      method: 'POST',
      baseUrl: baseUrl,
      path: '/v1/agent/p2p/stop',
    );
  }

  Future<Map<String, dynamic>> runtimeState(String baseUrl) {
    return _request(
      method: 'GET',
      baseUrl: baseUrl,
      path: '/v1/agent/runtime/state',
    );
  }

  Future<Map<String, dynamic>> pendingAcks(String baseUrl) {
    return _request(
      method: 'GET',
      baseUrl: baseUrl,
      path: '/v1/agent/runtime/acks/pending',
    );
  }

  Future<Map<String, dynamic>> sendEnvelope(
    String baseUrl,
    Map<String, dynamic> envelope,
  ) {
    return _request(
      method: 'POST',
      baseUrl: baseUrl,
      path: '/v1/agent/envelope',
      body: envelope,
    );
  }

  void dispose() {
    _client.close();
  }

  Future<Map<String, dynamic>> _request({
    required String method,
    required String baseUrl,
    required String path,
    Map<String, dynamic>? body,
  }) async {
    final normalized = baseUrl.trim();
    if (normalized.isEmpty) {
      throw AgentApiException(0, 'agent base url is empty');
    }

    final uri = Uri.parse('${normalized.replaceAll(RegExp(r'/+$'), '')}$path');

    late final http.Response response;
    if (method == 'GET') {
      response = await _client.get(uri);
    } else if (method == 'POST') {
      response = await _client.post(
        uri,
        headers: const {'Content-Type': 'application/json'},
        body: body == null ? '{}' : jsonEncode(body),
      );
    } else {
      throw AgentApiException(0, 'unsupported method: $method');
    }

    final text = utf8.decode(response.bodyBytes);
    dynamic decoded;
    if (text.isEmpty) {
      decoded = <String, dynamic>{};
    } else {
      try {
        decoded = jsonDecode(text);
      } catch (_) {
        decoded = <String, dynamic>{'raw': text};
      }
    }

    if (response.statusCode < 200 || response.statusCode >= 300) {
      final message = decoded is Map<String, dynamic>
          ? (decoded['error']?.toString() ?? decoded.toString())
          : decoded.toString();
      throw AgentApiException(response.statusCode, message);
    }

    if (decoded is Map<String, dynamic>) {
      return decoded;
    }

    return <String, dynamic>{'data': decoded};
  }
}
