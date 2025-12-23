import "dart:async";
import "dart:convert";
import "dart:math";
import "dart:ui" as ui;

import "package:flutter/foundation.dart";

import "device_id_store.dart";
import "gzip.dart";
import "payload.dart";
import "transport.dart";

class LogtapClient {
  static const String sdkName = "logtap-flutter";
  static const String sdkVersion = "0.1.0";

  final String _baseUrl;
  final String _projectId;
  final String? _projectKey;

  final Duration _flushInterval;
  final int _maxBatchSize;
  final int _maxQueueSize;
  final Duration _timeout;

  final bool _gzip;
  final bool _persistDeviceId;

  final LogtapBeforeSend? _beforeSend;

  final LogtapTransport _transport;
  final bool _ownsTransport;

  LogtapDeviceIdStore? _deviceIdStore;

  String _deviceId;
  JsonMap? _user;

  final JsonMap? _globalFields;
  final JsonMap? _globalProperties;
  final Map<String, String>? _globalTags;
  final JsonMap? _globalContexts;

  final List<JsonMap> _logQueue = [];
  final List<JsonMap> _trackQueue = [];

  int _backoffMs = 0;
  Future<void>? _flushing;
  Timer? _timer;

  bool _capturingFlutterErrors = false;
  FlutterExceptionHandler? _prevFlutterOnError;
  bool Function(Object, StackTrace)? _prevPlatformOnError;

  LogtapClient._({
    required String baseUrl,
    required String projectId,
    required String deviceId,
    required String? projectKey,
    required Duration flushInterval,
    required int maxBatchSize,
    required int maxQueueSize,
    required Duration timeout,
    required bool gzip,
    required bool persistDeviceId,
    required LogtapBeforeSend? beforeSend,
    required LogtapTransport transport,
    required bool ownsTransport,
    required LogtapDeviceIdStore? deviceIdStore,
    required JsonMap? user,
    required JsonMap? globalFields,
    required JsonMap? globalProperties,
    required Map<String, String>? globalTags,
    required JsonMap? globalContexts,
  })  : _baseUrl = baseUrl,
        _projectId = projectId,
        _deviceId = deviceId,
        _projectKey = projectKey,
        _flushInterval = flushInterval,
        _maxBatchSize = maxBatchSize,
        _maxQueueSize = maxQueueSize,
        _timeout = timeout,
        _gzip = gzip,
        _persistDeviceId = persistDeviceId,
        _beforeSend = beforeSend,
        _transport = transport,
        _ownsTransport = ownsTransport,
        _deviceIdStore = deviceIdStore,
        _user = user,
        _globalFields = globalFields,
        _globalProperties = globalProperties,
        _globalTags = globalTags,
        _globalContexts = globalContexts {
    if (_flushInterval > Duration.zero) {
      _timer = Timer.periodic(_flushInterval, (_) => flush());
    }
  }

  static Future<LogtapClient> create(
    LogtapClientOptions options, {
    LogtapTransport? transport,
  }) async {
    final baseUrl = _normalizeBaseUrl(options.baseUrl);
    final projectId = _normalizeProjectId(options.projectId);
    if (projectId.isEmpty) {
      throw ArgumentError.value(options.projectId, "projectId", "projectId required");
    }

    final ownsTransport = transport == null;
    final t = transport ?? defaultTransport();

    LogtapDeviceIdStore? store = options.deviceIdStore;
    if (store == null && options.persistDeviceId) {
      store = defaultDeviceIdStore();
    }

    String deviceId = (options.deviceId ?? "").trim();
    if (deviceId.isEmpty) {
      if (options.persistDeviceId) {
        final existing = (await store?.load())?.trim() ?? "";
        if (existing.isNotEmpty) {
          deviceId = existing.trim();
        } else {
          deviceId = _newDeviceId();
          await store?.save(deviceId);
        }
      } else {
        deviceId = _newDeviceId();
      }
    } else if (options.persistDeviceId) {
      await store?.save(deviceId);
    }

    final gzipSupported = gzipEncode(Uint8List(0)) != null;
    final gzipEnabled = options.gzip && gzipSupported;

    return LogtapClient._(
      baseUrl: baseUrl,
      projectId: projectId,
      deviceId: deviceId,
      projectKey: options.projectKey?.trim().isEmpty == true ? null : options.projectKey?.trim(),
      flushInterval: options.flushInterval,
      maxBatchSize: options.maxBatchSize,
      maxQueueSize: options.maxQueueSize,
      timeout: options.timeout,
      gzip: gzipEnabled,
      persistDeviceId: options.persistDeviceId,
      beforeSend: options.beforeSend,
      transport: t,
      ownsTransport: ownsTransport,
      deviceIdStore: store,
      user: options.user?.toJson(),
      globalFields: _jsonSafeMap(options.globalFields),
      globalProperties: _jsonSafeMap(options.globalProperties),
      globalTags: options.globalTags == null ? null : Map<String, String>.from(options.globalTags!),
      globalContexts: _jsonSafeMap(options.globalContexts),
    );
  }

  void setUser(LogtapUser? user) {
    _user = user?.toJson();
  }

  void identify(String userId, [JsonMap? traits]) {
    final id = userId.trim();
    if (id.isEmpty) return;
    setUser(LogtapUser(id: id, traits: traits));
  }

  void clearUser() {
    _user = null;
  }

  Future<void> setDeviceId(String deviceId, {bool? persist}) async {
    final id = deviceId.trim();
    if (id.isEmpty) return;
    _deviceId = id;

    final shouldPersist = persist ?? _persistDeviceId;
    if (!shouldPersist) return;

    _deviceIdStore ??= defaultDeviceIdStore();
    await _deviceIdStore?.save(id);
  }

  void log(
    LogtapLevel level,
    String message, [
    JsonMap? fields,
    LogtapLogOptions? options,
  ]) {
    final msg = message.trim();
    if (msg.isEmpty) return;

    final ts = (options?.timestamp ?? DateTime.now()).toUtc().toIso8601String();
    final payload = <String, dynamic>{
      "level": level.wireValue,
      "message": msg,
      "timestamp": ts,
      "device_id": (options?.deviceId ?? _deviceId).trim(),
      "trace_id": options?.traceId?.trim().isEmpty == true ? null : options?.traceId?.trim(),
      "span_id": options?.spanId?.trim().isEmpty == true ? null : options?.spanId?.trim(),
      "fields": _mergeJsonMap(_globalFields, _jsonSafeMap(fields)),
      "tags": _mergeStringMap(_globalTags, options?.tags),
      "user": (options?.user?.toJson() ?? _user),
      "contexts": _mergeJsonMap(_globalContexts, _jsonSafeMap(options?.contexts)),
      "extra": _jsonSafeMap(options?.extra),
      "sdk": _sdkInfo(),
    }..removeWhere((_, v) => v == null);

    _enqueue(_logQueue, payload);
  }

  void debug(String message, [JsonMap? fields, LogtapLogOptions? options]) {
    log(LogtapLevel.debug, message, fields, options);
  }

  void info(String message, [JsonMap? fields, LogtapLogOptions? options]) {
    log(LogtapLevel.info, message, fields, options);
  }

  void warn(String message, [JsonMap? fields, LogtapLogOptions? options]) {
    log(LogtapLevel.warn, message, fields, options);
  }

  void error(String message, [JsonMap? fields, LogtapLogOptions? options]) {
    log(LogtapLevel.error, message, fields, options);
  }

  void fatal(String message, [JsonMap? fields, LogtapLogOptions? options]) {
    log(LogtapLevel.fatal, message, fields, options);
  }

  void track(
    String name, [
    JsonMap? properties,
    LogtapTrackOptions? options,
  ]) {
    final n = name.trim();
    if (n.isEmpty) return;

    final ts = (options?.timestamp ?? DateTime.now()).toUtc().toIso8601String();
    final payload = <String, dynamic>{
      "name": n,
      "timestamp": ts,
      "device_id": (options?.deviceId ?? _deviceId).trim(),
      "trace_id": options?.traceId?.trim().isEmpty == true ? null : options?.traceId?.trim(),
      "span_id": options?.spanId?.trim().isEmpty == true ? null : options?.spanId?.trim(),
      "properties": _mergeJsonMap(_globalProperties, _jsonSafeMap(properties)),
      "tags": _mergeStringMap(_globalTags, options?.tags),
      "user": (options?.user?.toJson() ?? _user),
      "contexts": _mergeJsonMap(_globalContexts, _jsonSafeMap(options?.contexts)),
      "extra": _jsonSafeMap(options?.extra),
      "sdk": _sdkInfo(),
    }..removeWhere((_, v) => v == null);

    _enqueue(_trackQueue, payload);
  }

  Future<void> flush() {
    if (_flushing != null) return _flushing!;
    _flushing = _flushInner().whenComplete(() => _flushing = null);
    return _flushing!;
  }

  Future<void> close() async {
    _timer?.cancel();
    _timer = null;
    await flush();
    if (_ownsTransport) {
      _transport.close();
    }
  }

  void captureFlutterErrors({bool includeStackTrace = true}) {
    if (_capturingFlutterErrors) return;
    _capturingFlutterErrors = true;

    _prevFlutterOnError = FlutterError.onError;
    FlutterError.onError = (FlutterErrorDetails details) {
      try {
        final fields = <String, dynamic>{
          "kind": "FlutterError.onError",
          "exception": details.exceptionAsString(),
          "library": details.library,
          "context": details.context?.toDescription(),
          "stack": includeStackTrace ? details.stack?.toString() : null,
        }..removeWhere((_, v) => v == null);
        error(details.exceptionAsString(), fields);
      } catch (_) {}

      _prevFlutterOnError?.call(details);
    };

    _prevPlatformOnError = ui.PlatformDispatcher.instance.onError;
    ui.PlatformDispatcher.instance.onError = (Object e, StackTrace st) {
      try {
        final fields = <String, dynamic>{
          "kind": "PlatformDispatcher.onError",
          "error": e.toString(),
          "stack": includeStackTrace ? st.toString() : null,
        }..removeWhere((_, v) => v == null);
        fatal(e.toString(), fields);
      } catch (_) {}

      final prev = _prevPlatformOnError;
      if (prev != null) return prev(e, st);
      return false;
    };
  }

  JsonMap _sdkInfo() {
    return <String, dynamic>{
      "name": sdkName,
      "version": sdkVersion,
      "runtime": kIsWeb ? "flutter_web" : "flutter",
      "platform": defaultTargetPlatform.name,
    };
  }

  Future<void> _flushInner() async {
    while (true) {
      if (_backoffMs > 0) {
        await Future.delayed(Duration(milliseconds: _backoffMs));
      }

      final sentLogs = await _flushQueue("/logs/", _logQueue);
      final sentTrack = await _flushQueue("/track/", _trackQueue);

      if (sentLogs || sentTrack) {
        _backoffMs = 0;
        continue;
      }
      return;
    }
  }

  Future<bool> _flushQueue(String path, List<JsonMap> queue) async {
    if (queue.isEmpty) return false;

    final batchSize = min(_maxBatchSize, queue.length);
    final batch = queue.sublist(0, batchSize);
    queue.removeRange(0, batchSize);

    final ok = await _postJson(path, batch);
    if (!ok) {
      queue.insertAll(0, batch);
      _trimQueue(queue);
      _bumpBackoff();
      return false;
    }
    return true;
  }

  Future<bool> _postJson(String path, List<JsonMap> payload) async {
    final uri = Uri.parse("$_baseUrl/api/${Uri.encodeComponent(_projectId)}$path");

    final jsonString = jsonEncode(payload);
    Uint8List body = Uint8List.fromList(utf8.encode(jsonString));

    final headers = <String, String>{"Content-Type": "application/json"};
    final pk = _projectKey;
    if (pk != null && pk.isNotEmpty) {
      headers["X-Project-Key"] = pk;
    }

    if (_gzip) {
      final gz = gzipEncode(body);
      if (gz != null) {
        body = gz;
        headers["Content-Encoding"] = "gzip";
      }
    }

    try {
      final status = await _transport.post(
        uri,
        headers: headers,
        body: body,
        timeout: _timeout,
      );
      return status >= 200 && status < 300;
    } catch (_) {
      return false;
    }
  }

  void _enqueue(List<JsonMap> queue, JsonMap payload) {
    final p = _applyBeforeSend(payload);
    if (p == null) return;
    queue.add(p);
    _trimQueue(queue);
  }

  JsonMap? _applyBeforeSend(JsonMap payload) {
    final fn = _beforeSend;
    if (fn == null) return payload;
    try {
      return fn(payload);
    } catch (_) {
      return payload;
    }
  }

  void _trimQueue(List<JsonMap> queue) {
    if (queue.length <= _maxQueueSize) return;
    queue.removeRange(0, queue.length - _maxQueueSize);
  }

  void _bumpBackoff() {
    if (_backoffMs <= 0) {
      _backoffMs = 500;
      return;
    }
    _backoffMs = min(_backoffMs * 2, 30000);
  }

  static String _normalizeBaseUrl(String baseUrl) {
    final s = baseUrl.trim();
    if (s.isEmpty) {
      throw ArgumentError.value(baseUrl, "baseUrl", "baseUrl required");
    }
    return s.endsWith("/") ? s.substring(0, s.length - 1) : s;
  }

  static String _normalizeProjectId(Object projectId) {
    if (projectId is int) return projectId.toString();
    if (projectId is String) return projectId.trim();
    return projectId.toString();
  }

  static String _newDeviceId() => "d_${_randomHex(16)}";

  static String _randomHex(int bytes) {
    final rnd = _secureRandom();
    final sb = StringBuffer();
    for (var i = 0; i < bytes; i++) {
      final v = rnd.nextInt(256);
      sb.write(v.toRadixString(16).padLeft(2, "0"));
    }
    return sb.toString();
  }

  static Random _secureRandom() {
    try {
      return Random.secure();
    } catch (_) {
      return Random();
    }
  }

  static JsonMap? _mergeJsonMap(JsonMap? a, JsonMap? b) {
    if ((a == null || a.isEmpty) && (b == null || b.isEmpty)) return null;
    return <String, dynamic>{...?a, ...?b};
  }

  static Map<String, String>? _mergeStringMap(
    Map<String, String>? a,
    Map<String, String>? b,
  ) {
    if ((a == null || a.isEmpty) && (b == null || b.isEmpty)) return null;
    return <String, String>{...?a, ...?b};
  }

  static JsonMap? _jsonSafeMap(JsonMap? input) {
    if (input == null || input.isEmpty) return null;
    final out = <String, dynamic>{};
    for (final entry in input.entries) {
      out[entry.key] = _jsonSafe(entry.value);
    }
    return out;
  }

  static Object? _jsonSafe(Object? value) {
    if (value == null) return null;
    if (value is String || value is num || value is bool) return value;
    if (value is DateTime) return value.toUtc().toIso8601String();
    if (value is Map) {
      final out = <String, dynamic>{};
      value.forEach((k, v) {
        out[k.toString()] = _jsonSafe(v);
      });
      return out;
    }
    if (value is Iterable) {
      return value.map((v) => _jsonSafe(v)).toList();
    }
    return value.toString();
  }
}
