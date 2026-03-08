import 'dart:convert';
import 'dart:io';

import 'package:path_provider/path_provider.dart';

class SavedHostEntry {
  const SavedHostEntry({
    required this.agentBaseUrl,
    required this.signalingBaseUrl,
    required this.lastUsedAtMillis,
  });

  final String agentBaseUrl;
  final String signalingBaseUrl;
  final int lastUsedAtMillis;

  factory SavedHostEntry.fromMap(Map<String, dynamic> map) {
    return SavedHostEntry(
      agentBaseUrl: map['agentBaseUrl']?.toString() ?? '',
      signalingBaseUrl: map['signalingBaseUrl']?.toString() ?? '',
      lastUsedAtMillis: map['lastUsedAt'] is num
          ? (map['lastUsedAt'] as num).toInt()
          : int.tryParse(map['lastUsedAt']?.toString() ?? '') ?? 0,
    );
  }

  Map<String, dynamic> toMap() {
    return {
      'agentBaseUrl': agentBaseUrl,
      'signalingBaseUrl': signalingBaseUrl,
      'lastUsedAt': lastUsedAtMillis,
    };
  }
}

class AppSettingsSnapshot {
  const AppSettingsSnapshot({
    this.agentBaseUrl = '',
    this.signalingBaseUrl = '',
    this.recentHosts = const [],
  });

  final String agentBaseUrl;
  final String signalingBaseUrl;
  final List<SavedHostEntry> recentHosts;

  factory AppSettingsSnapshot.fromMap(Map<String, dynamic> map) {
    final recentHostsRaw = map['recentHosts'];
    return AppSettingsSnapshot(
      agentBaseUrl: map['agentBaseUrl']?.toString() ?? '',
      signalingBaseUrl: map['signalingBaseUrl']?.toString() ?? '',
      recentHosts: recentHostsRaw is List
          ? recentHostsRaw
                .whereType<Map>()
                .map((item) => SavedHostEntry.fromMap(Map<String, dynamic>.from(item)))
                .toList()
          : const [],
    );
  }

  Map<String, dynamic> toMap() {
    return {
      'agentBaseUrl': agentBaseUrl,
      'signalingBaseUrl': signalingBaseUrl,
      'recentHosts': recentHosts.map((item) => item.toMap()).toList(),
    };
  }
}

abstract class AppSettingsStore {
  Future<AppSettingsSnapshot> load();

  Future<void> save(AppSettingsSnapshot snapshot);
}

class InMemoryAppSettingsStore implements AppSettingsStore {
  AppSettingsSnapshot _snapshot = const AppSettingsSnapshot();

  @override
  Future<AppSettingsSnapshot> load() async => _snapshot;

  @override
  Future<void> save(AppSettingsSnapshot snapshot) async {
    _snapshot = snapshot;
  }
}

class FileAppSettingsStore implements AppSettingsStore {
  FileAppSettingsStore({
    this.fileName = 'vibedeck_mobile_settings.json',
  });

  final String fileName;

  @override
  Future<AppSettingsSnapshot> load() async {
    try {
      final file = await _settingsFile();
      if (!await file.exists()) {
        return const AppSettingsSnapshot();
      }
      final text = await file.readAsString();
      if (text.trim().isEmpty) {
        return const AppSettingsSnapshot();
      }
      final decoded = jsonDecode(text);
      if (decoded is Map<String, dynamic>) {
        return AppSettingsSnapshot.fromMap(decoded);
      }
    } catch (_) {
      return const AppSettingsSnapshot();
    }
    return const AppSettingsSnapshot();
  }

  @override
  Future<void> save(AppSettingsSnapshot snapshot) async {
    try {
      final file = await _settingsFile();
      await file.parent.create(recursive: true);
      await file.writeAsString(jsonEncode(snapshot.toMap()), flush: true);
    } catch (_) {
      // 설정 저장 실패는 런타임 동작을 막지 않는다.
    }
  }

  Future<File> _settingsFile() async {
    final directory = await getApplicationSupportDirectory();
    return File('${directory.path}${Platform.pathSeparator}$fileName');
  }
}
