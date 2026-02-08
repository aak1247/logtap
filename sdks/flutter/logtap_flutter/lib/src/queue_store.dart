import "queue_store_stub.dart"
    if (dart.library.io) "queue_store_io.dart"
    if (dart.library.html) "queue_store_web.dart" as impl;

import "payload.dart";

LogtapQueueStore defaultQueueStore(String projectId) => impl.defaultQueueStore(projectId);

