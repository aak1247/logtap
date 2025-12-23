import "payload.dart";

class _InMemoryDeviceIdStore implements LogtapDeviceIdStore {
  String? _value;

  @override
  Future<String?> load() async => _value;

  @override
  Future<void> save(String deviceId) async {
    _value = deviceId;
  }
}

LogtapDeviceIdStore defaultDeviceIdStore() => _InMemoryDeviceIdStore();

