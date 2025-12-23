import "package:flutter/material.dart";
import "package:logtap_flutter/logtap_flutter.dart";

void main() {
  runApp(const DemoApp());
}

class DemoApp extends StatefulWidget {
  const DemoApp({super.key});

  @override
  State<DemoApp> createState() => _DemoAppState();
}

class _DemoAppState extends State<DemoApp> {
  final _baseUrl = TextEditingController(text: "http://localhost:8080");
  final _projectId = TextEditingController(text: "1");
  final _projectKey = TextEditingController();

  bool _gzip = true;
  LogtapClient? _client;
  String _status = "未初始化";

  @override
  void dispose() {
    _baseUrl.dispose();
    _projectId.dispose();
    _projectKey.dispose();
    _client?.close();
    super.dispose();
  }

  Future<void> _init() async {
    final prevClient = _client;
    setState(() {
      _status = "初始化中...";
      _client = null;
    });

    try {
      await prevClient?.close();

      // Create a client once with user-provided config.
      final client = await LogtapClient.create(
        LogtapClientOptions(
          baseUrl: _baseUrl.text.trim(),
          projectId: _projectId.text.trim(),
          projectKey: _projectKey.text.trim().isEmpty ? null : _projectKey.text.trim(),
          gzip: _gzip,
          globalTags: const {"env": "demo", "runtime": "flutter"},
          globalContexts: const {"demo": {"kind": "flutter"}},
        ),
      );

      // Optional: capture Flutter/Platform errors.
      client.captureFlutterErrors();
      client.identify("u_demo_flutter", {"plan": "free"});

      client.info("flutter demo init", {"k": "v"});
      client.track("demo_init", {"kind": "flutter"});
      await client.flush();

      setState(() {
        _client = client;
        _status = "已初始化并 flush";
      });
    } catch (err) {
      setState(() => _status = "初始化失败: $err");
    }
  }

  @override
  Widget build(BuildContext context) {
    final client = _client;

    return MaterialApp(
      debugShowCheckedModeBanner: false,
      home: Scaffold(
        appBar: AppBar(title: const Text("logtap Flutter Demo")),
        body: Padding(
          padding: const EdgeInsets.all(16),
          child: ListView(
            children: [
              Text(_status),
              const SizedBox(height: 12),
              TextField(
                controller: _baseUrl,
                decoration: const InputDecoration(
                  labelText: "Base URL",
                  hintText: "http://localhost:8080",
                ),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: _projectId,
                decoration: const InputDecoration(
                  labelText: "Project ID",
                  hintText: "1",
                ),
              ),
              const SizedBox(height: 12),
              TextField(
                controller: _projectKey,
                decoration: const InputDecoration(
                  labelText: "Project Key（可选）",
                  hintText: "pk_xxx",
                ),
              ),
              const SizedBox(height: 12),
              SwitchListTile(
                value: _gzip,
                onChanged: (v) => setState(() => _gzip = v),
                title: const Text("gzip（Web 会自动降级）"),
              ),
              const SizedBox(height: 12),
              FilledButton(
                onPressed: _init,
                child: const Text("初始化"),
              ),
              const Divider(height: 32),
              FilledButton.tonal(
                onPressed: client == null
                    ? null
                    : () {
                        client.info("hello from flutter", {
                          "ts": DateTime.now().toUtc().toIso8601String(),
                        });
                        setState(() => _status = "已入队：info");
                      },
                child: const Text("发送 info 日志"),
              ),
              const SizedBox(height: 12),
              FilledButton.tonal(
                onPressed: client == null
                    ? null
                    : () {
                        client.track("signup", {
                          "from": "flutter_demo",
                          "ts": DateTime.now().toUtc().toIso8601String(),
                        });
                        setState(() => _status = "已入队：track");
                      },
                child: const Text("发送 event（track）"),
              ),
              const SizedBox(height: 12),
              FilledButton.tonal(
                onPressed: client == null
                    ? null
                    : () async {
                        await client.flush();
                        setState(() => _status = "flush 完成");
                      },
                child: const Text("flush"),
              ),
              const SizedBox(height: 12),
              FilledButton.tonal(
                onPressed: client == null
                    ? null
                    : () {
                        FlutterError.reportError(
                          FlutterErrorDetails(
                            exception: Exception("demo flutter error"),
                            stack: StackTrace.current,
                          ),
                        );
                        setState(() => _status = "已上报 FlutterError");
                      },
                child: const Text("制造 FlutterError（自动捕获）"),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
