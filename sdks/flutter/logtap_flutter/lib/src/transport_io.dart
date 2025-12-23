import "dart:async";
import "dart:io";
import "dart:typed_data";

import "transport_base.dart";

class _IoTransport implements LogtapTransport {
  final HttpClient _client = HttpClient();

  @override
  Future<int> post(
    Uri uri, {
    required Map<String, String> headers,
    required Uint8List body,
    required Duration timeout,
  }) async {
    final req = await _client.postUrl(uri).timeout(timeout);
    headers.forEach((k, v) => req.headers.set(k, v));
    req.add(body);
    final res = await req.close().timeout(timeout);
    await res.drain<void>();
    return res.statusCode;
  }

  @override
  void close() {
    _client.close(force: true);
  }
}

LogtapTransport defaultTransport() => _IoTransport();
