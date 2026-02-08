import "dart:convert";
import "dart:html" as html;

import "payload.dart";

class _LocalStorageQueueStore implements LogtapQueueStore {
  final String _k;

  _LocalStorageQueueStore(String projectId) : _k = "logtap_queue_$projectId";

  @override
  Future<JsonMap?> load() async {
    try {
      final raw = (html.window.localStorage[_k] ?? "").trim();
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
      html.window.localStorage[_k] = jsonEncode(state);
    } catch (_) {}
  }

  @override
  Future<void> clear() async {
    try {
      html.window.localStorage.remove(_k);
    } catch (_) {}
  }
}

LogtapQueueStore defaultQueueStore(String projectId) => _LocalStorageQueueStore(projectId);

