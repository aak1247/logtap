import "device_id_store_stub.dart"
    if (dart.library.io) "device_id_store_io.dart"
    if (dart.library.html) "device_id_store_web.dart" as impl;

import "payload.dart";

LogtapDeviceIdStore defaultDeviceIdStore() => impl.defaultDeviceIdStore();

