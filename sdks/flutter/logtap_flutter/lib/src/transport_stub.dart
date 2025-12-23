import "dart:typed_data";

import "transport_base.dart";

class _UnsupportedTransport implements LogtapTransport {
  @override
  Future<int> post(
    Uri uri, {
    required Map<String, String> headers,
    required Uint8List body,
    required Duration timeout,
  }) async {
    throw UnsupportedError("No HTTP transport available on this platform");
  }

  @override
  void close() {}
}

LogtapTransport defaultTransport() => _UnsupportedTransport();
