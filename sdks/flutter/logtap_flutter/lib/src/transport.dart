import "transport_base.dart";

import "transport_stub.dart"
    if (dart.library.io) "transport_io.dart"
    if (dart.library.html) "transport_web.dart" as impl;

export "transport_base.dart";

LogtapTransport defaultTransport() => impl.defaultTransport();
