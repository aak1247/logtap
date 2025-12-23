import "dart:typed_data";

import "gzip_stub.dart" if (dart.library.io) "gzip_io.dart" as impl;

Uint8List? gzipEncode(Uint8List bytes) => impl.gzipEncode(bytes);

