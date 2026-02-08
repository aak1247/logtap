import "payload.dart";

class _InMemoryQueueStore implements LogtapQueueStore {
  JsonMap? _state;

  @override
  Future<JsonMap?> load() async => _state;

  @override
  Future<void> save(JsonMap state) async {
    _state = state;
  }

  @override
  Future<void> clear() async {
    _state = null;
  }
}

LogtapQueueStore defaultQueueStore(String projectId) => _InMemoryQueueStore();

