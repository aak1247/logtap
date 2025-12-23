typedef JsonMap = Map<String, dynamic>;

enum LogtapLevel { debug, info, warn, error, fatal, event }

extension LogtapLevelWire on LogtapLevel {
  String get wireValue {
    return switch (this) {
      LogtapLevel.debug => "debug",
      LogtapLevel.info => "info",
      LogtapLevel.warn => "warn",
      LogtapLevel.error => "error",
      LogtapLevel.fatal => "fatal",
      LogtapLevel.event => "event",
    };
  }
}

class LogtapUser {
  final String? id;
  final String? email;
  final String? username;
  final JsonMap? traits;

  const LogtapUser({this.id, this.email, this.username, this.traits});

  JsonMap toJson() {
    final out = <String, dynamic>{};
    if (id != null && id!.trim().isNotEmpty) out["id"] = id!.trim();
    if (email != null && email!.trim().isNotEmpty) out["email"] = email!.trim();
    if (username != null && username!.trim().isNotEmpty) out["username"] = username!.trim();
    if (traits != null && traits!.isNotEmpty) out["traits"] = traits;
    return out;
  }
}

class LogtapLogOptions {
  final DateTime? timestamp;
  final String? traceId;
  final String? spanId;
  final Map<String, String>? tags;
  final String? deviceId;
  final LogtapUser? user;
  final JsonMap? contexts;
  final JsonMap? extra;

  const LogtapLogOptions({
    this.timestamp,
    this.traceId,
    this.spanId,
    this.tags,
    this.deviceId,
    this.user,
    this.contexts,
    this.extra,
  });
}

class LogtapTrackOptions {
  final DateTime? timestamp;
  final String? traceId;
  final String? spanId;
  final Map<String, String>? tags;
  final String? deviceId;
  final LogtapUser? user;
  final JsonMap? contexts;
  final JsonMap? extra;

  const LogtapTrackOptions({
    this.timestamp,
    this.traceId,
    this.spanId,
    this.tags,
    this.deviceId,
    this.user,
    this.contexts,
    this.extra,
  });
}

typedef LogtapBeforeSend = JsonMap? Function(JsonMap payload);

abstract class LogtapDeviceIdStore {
  Future<String?> load();
  Future<void> save(String deviceId);
}

class LogtapClientOptions {
  final String baseUrl;
  final Object projectId;
  final String? projectKey;

  final Duration flushInterval;
  final int maxBatchSize;
  final int maxQueueSize;
  final Duration timeout;

  final bool gzip;
  final bool persistDeviceId;
  final String? deviceId;
  final LogtapDeviceIdStore? deviceIdStore;

  final LogtapUser? user;
  final JsonMap? globalFields;
  final JsonMap? globalProperties;
  final Map<String, String>? globalTags;
  final JsonMap? globalContexts;

  final LogtapBeforeSend? beforeSend;

  const LogtapClientOptions({
    required this.baseUrl,
    required this.projectId,
    this.projectKey,
    this.flushInterval = const Duration(seconds: 2),
    this.maxBatchSize = 50,
    this.maxQueueSize = 1000,
    this.timeout = const Duration(seconds: 5),
    this.gzip = false,
    this.persistDeviceId = true,
    this.deviceId,
    this.deviceIdStore,
    this.user,
    this.globalFields,
    this.globalProperties,
    this.globalTags,
    this.globalContexts,
    this.beforeSend,
  });
}
