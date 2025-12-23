import "dart:io";

import "payload.dart";

class _FileDeviceIdStore implements LogtapDeviceIdStore {
  final File _file;

  _FileDeviceIdStore(this._file);

  @override
  Future<String?> load() async {
    try {
      if (!await _file.exists()) return null;
      final s = (await _file.readAsString()).trim();
      return s.isEmpty ? null : s;
    } catch (_) {
      return null;
    }
  }

  @override
  Future<void> save(String deviceId) async {
    try {
      await _file.writeAsString(deviceId, flush: true);
    } catch (_) {}
  }
}

String _defaultDeviceIdFilePath() {
  final sep = Platform.pathSeparator;
  if (Platform.isWindows) {
    final base = Platform.environment["APPDATA"] ??
        Platform.environment["USERPROFILE"] ??
        Directory.current.path;
    return "$base${sep}logtap_device_id";
  }
  final base = Platform.environment["HOME"] ?? Directory.current.path;
  return "$base${sep}.logtap_device_id";
}

LogtapDeviceIdStore defaultDeviceIdStore() {
  return _FileDeviceIdStore(File(_defaultDeviceIdFilePath()));
}

