import "dart:async";
import "dart:html" as html;
import "dart:typed_data";

import "transport_base.dart";

class _WebTransport implements LogtapTransport {
  @override
  Future<int> post(
    Uri uri, {
    required Map<String, String> headers,
    required Uint8List body,
    required Duration timeout,
  }) async {
    final req = await html.HttpRequest.request(
      uri.toString(),
      method: "POST",
      requestHeaders: headers,
      sendData: body,
    ).timeout(timeout);
    return req.status ?? 0;
  }

  @override
  void close() {}
}

LogtapTransport defaultTransport() => _WebTransport();
