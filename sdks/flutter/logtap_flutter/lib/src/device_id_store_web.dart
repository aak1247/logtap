import "dart:html" as html;

import "payload.dart";

class _LocalStorageDeviceIdStore implements LogtapDeviceIdStore {
  static const _k = "logtap_device_id";

  @override
  Future<String?> load() async {
    try {
      final v = (html.window.localStorage[_k] ?? "").trim();
      return v.isEmpty ? null : v;
    } catch (_) {
      return null;
    }
  }

  @override
  Future<void> save(String deviceId) async {
    try {
      html.window.localStorage[_k] = deviceId;
    } catch (_) {}
  }
}

LogtapDeviceIdStore defaultDeviceIdStore() => _LocalStorageDeviceIdStore();

