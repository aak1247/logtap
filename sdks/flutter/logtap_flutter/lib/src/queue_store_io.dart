import "dart:convert";
import "dart:io";

import "payload.dart";

class _FileQueueStore implements LogtapQueueStore {
  final File _file;

  _FileQueueStore(this._file);

  @override
  Future<JsonMap?> load() async {
    try {
      if (!await _file.exists()) return null;
      final raw = (await _file.readAsString()).trim();
      if (raw.isEmpty) return null;
      final v = jsonDecode(raw);
      if (v is Map) {
        return Map<String, dynamic>.from(v);
      }
      return null;
    } catch (_) {
      return null;
    }
  }

  @override
  Future<void> save(JsonMap state) async {
    try {
      await _file.writeAsString(jsonEncode(state), flush: true);
    } catch (_) {}
  }

  @override
  Future<void> clear() async {
    try {
      if (await _file.exists()) {
        await _file.delete();
      }
    } catch (_) {}
  }
}

String _defaultQueueFilePath(String projectId) {
  final sep = Platform.pathSeparator;
  final safeId = projectId.replaceAll(RegExp(r"[^a-zA-Z0-9_.-]"), "_");
  if (Platform.isWindows) {
    final base = Platform.environment["APPDATA"] ??
        Platform.environment["USERPROFILE"] ??
        Directory.current.path;
    return "$base${sep}logtap_queue_$safeId.json";
  }
  final base = Platform.environment["HOME"] ?? Directory.current.path;
  return "$base${sep}.logtap_queue_$safeId.json";
}

LogtapQueueStore defaultQueueStore(String projectId) {
  return _FileQueueStore(File(_defaultQueueFilePath(projectId)));
}

