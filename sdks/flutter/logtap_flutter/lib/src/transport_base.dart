import "dart:typed_data";

abstract class LogtapTransport {
  Future<int> post(
    Uri uri, {
    required Map<String, String> headers,
    required Uint8List body,
    required Duration timeout,
  });

  void close();
}

