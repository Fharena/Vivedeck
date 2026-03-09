import 'dart:async';
import 'dart:convert';
import 'dart:io';

class LanDiscoveryException implements Exception {
  LanDiscoveryException(this.message);

  final String message;

  @override
  String toString() => 'LanDiscoveryException: $message';
}

abstract class LanDiscoveryService {
  Future<List<Map<String, dynamic>>> discover({Duration? timeout});
}

class UdpLanDiscoveryService implements LanDiscoveryService {
  UdpLanDiscoveryService({
    this.port = 42777,
    this.defaultTimeout = const Duration(milliseconds: 1400),
  });

  final int port;
  final Duration defaultTimeout;

  @override
  Future<List<Map<String, dynamic>>> discover({Duration? timeout}) async {
    RawDatagramSocket? socket;
    StreamSubscription<RawSocketEvent>? subscription;
    final discoveredByAgent = <String, Map<String, dynamic>>{};

    try {
      socket = await RawDatagramSocket.bind(
        InternetAddress.anyIPv4,
        0,
        reuseAddress: true,
      );
      socket.broadcastEnabled = true;

      subscription = socket.listen((event) {
        if (event != RawSocketEvent.read) {
          return;
        }
        Datagram? datagram;
        while ((datagram = socket?.receive()) != null) {
          final parsed = _parseResponse(datagram!);
          if (parsed == null) {
            continue;
          }
          final agentBaseUrl = parsed['agentBaseUrl']?.toString().trim() ?? '';
          if (agentBaseUrl.isEmpty) {
            continue;
          }
          discoveredByAgent[agentBaseUrl] = parsed;
        }
      });

      final probe = utf8.encode(
        jsonEncode({
          'type': 'vibedeck_discover',
          'version': 1,
        }),
      );
      socket.send(probe, InternetAddress('255.255.255.255'), port);
      await Future<void>.delayed(timeout ?? defaultTimeout);
    } on SocketException catch (e) {
      throw LanDiscoveryException('LAN broadcast를 시작하지 못했습니다. $e');
    } finally {
      await subscription?.cancel();
      socket?.close();
    }

    final discovered = discoveredByAgent.values.toList()
      ..sort((left, right) {
        final leftDisplay = left['displayName']?.toString() ?? '';
        final rightDisplay = right['displayName']?.toString() ?? '';
        final byDisplay = leftDisplay.compareTo(rightDisplay);
        if (byDisplay != 0) {
          return byDisplay;
        }
        return (left['agentBaseUrl']?.toString() ?? '').compareTo(
          right['agentBaseUrl']?.toString() ?? '',
        );
      });
    return discovered;
  }

  Map<String, dynamic>? _parseResponse(Datagram datagram) {
    try {
      final decoded = jsonDecode(utf8.decode(datagram.data));
      if (decoded is! Map<String, dynamic>) {
        return null;
      }
      final type = decoded['type']?.toString().trim().toLowerCase() ?? '';
      if (type != 'vibedeck_discover_result') {
        return null;
      }
      return {
        ...decoded,
        'sourceAddress': datagram.address.address,
      };
    } catch (_) {
      return null;
    }
  }
}
