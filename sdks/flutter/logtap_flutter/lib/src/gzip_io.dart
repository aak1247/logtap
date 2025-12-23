import "dart:io";
import "dart:typed_data";

Uint8List gzipEncode(Uint8List bytes) {
  return Uint8List.fromList(gzip.encode(bytes));
}

