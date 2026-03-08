import 'dart:async';

import 'package:flutter/material.dart';

import 'screens/prompt_screen.dart';
import 'screens/review_screen.dart';
import 'screens/status_screen.dart';
import 'services/app_settings_store.dart';
import 'services/bootstrap_link_source.dart';
import 'state/app_controller.dart';

class VibeDeckApp extends StatelessWidget {
  const VibeDeckApp({
    super.key,
    this.controller,
    this.bootstrapLinkSource,
  });

  final AppController? controller;
  final BootstrapLinkSource? bootstrapLinkSource;

  @override
  Widget build(BuildContext context) {
    final baseTextTheme = ThemeData.light().textTheme;

    return MaterialApp(
      debugShowCheckedModeBanner: false,
      title: 'VibeDeck Mobile',
      theme: ThemeData(
        useMaterial3: true,
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF1F8C77),
          brightness: Brightness.light,
        ),
        fontFamily: 'monospace',
        textTheme: baseTextTheme.copyWith(
          headlineLarge: const TextStyle(
            fontSize: 36,
            fontWeight: FontWeight.w700,
            fontFamily: 'serif',
            color: Color(0xFF0F2D28),
          ),
          headlineMedium: const TextStyle(
            fontSize: 24,
            fontWeight: FontWeight.w700,
            fontFamily: 'serif',
            color: Color(0xFF0F2D28),
          ),
          titleLarge: const TextStyle(
            fontSize: 20,
            fontWeight: FontWeight.w700,
            fontFamily: 'serif',
            color: Color(0xFF0F2D28),
          ),
        ),
      ),
      home: MobileShell(
        controller: controller,
        bootstrapLinkSource: bootstrapLinkSource,
      ),
    );
  }
}

class MobileShell extends StatefulWidget {
  const MobileShell({
    super.key,
    this.controller,
    this.bootstrapLinkSource,
  });

  final AppController? controller;
  final BootstrapLinkSource? bootstrapLinkSource;

  @override
  State<MobileShell> createState() => _MobileShellState();
}

class _MobileShellState extends State<MobileShell> {
  int _index = 0;
  late final AppController _controller;
  late final bool _ownsController;
  late final BootstrapLinkSource _bootstrapLinkSource;
  StreamSubscription<Uri>? _bootstrapLinkSub;

  static const _titles = ['대화', '검토', '상태'];

  @override
  void initState() {
    super.initState();
    _controller =
        widget.controller ?? AppController(settingsStore: FileAppSettingsStore());
    _ownsController = widget.controller == null;
    _bootstrapLinkSource =
        widget.bootstrapLinkSource ??
        (_ownsController
            ? AppLinksBootstrapLinkSource()
            : const NoopBootstrapLinkSource());
    unawaited(_initializeApp());
  }

  Future<void> _initializeApp() async {
    await _controller.initialize();

    final initialUri = await _bootstrapLinkSource.getInitialUri();
    if (initialUri != null) {
      await _controller.applyBootstrapUri(initialUri);
    }

    _bootstrapLinkSub = _bootstrapLinkSource.uriStream.listen((uri) {
      unawaited(_controller.applyBootstrapUri(uri));
    });
  }

  @override
  void dispose() {
    unawaited(_bootstrapLinkSub?.cancel());
    if (_ownsController) {
      _controller.dispose();
    }
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _controller,
      builder: (context, _) {
        final colors = Theme.of(context).colorScheme;
        final screens = [
          PromptScreen(controller: _controller),
          ReviewScreen(controller: _controller),
          StatusScreen(controller: _controller),
        ];

        return Scaffold(
          body: Container(
            decoration: const BoxDecoration(
              gradient: LinearGradient(
                begin: Alignment.topCenter,
                end: Alignment.bottomCenter,
                colors: [Color(0xFFF6FBF9), Color(0xFFEFF4FF)],
              ),
            ),
            child: SafeArea(
              child: Column(
                children: [
                  Padding(
                    padding: const EdgeInsets.fromLTRB(20, 20, 20, 12),
                    child: Row(
                      children: [
                        Container(
                          width: 40,
                          height: 40,
                          decoration: BoxDecoration(
                            color: const Color(0xFF1F8C77),
                            borderRadius: BorderRadius.circular(12),
                          ),
                          child: _controller.isLoading
                              ? const Padding(
                                  padding: EdgeInsets.all(10),
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                    color: Colors.white,
                                  ),
                                )
                              : const Icon(
                                  Icons.auto_awesome,
                                  color: Colors.white,
                                ),
                        ),
                        const SizedBox(width: 12),
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                'VibeDeck Mobile',
                                style: Theme.of(context).textTheme.titleLarge,
                              ),
                              Text(
                                '${_titles[_index]} · ${_controller.connectionState}',
                                style: Theme.of(context)
                                    .textTheme
                                    .bodyMedium
                                    ?.copyWith(
                                      color: colors.primary,
                                      fontWeight: FontWeight.w600,
                                    ),
                              ),
                            ],
                          ),
                        ),
                      ],
                    ),
                  ),
                  Expanded(
                    child: AnimatedSwitcher(
                      duration: const Duration(milliseconds: 260),
                      child: screens[_index],
                    ),
                  ),
                ],
              ),
            ),
          ),
          bottomNavigationBar: NavigationBar(
            selectedIndex: _index,
            destinations: const [
              NavigationDestination(
                icon: Icon(Icons.forum_outlined),
                label: '대화',
              ),
              NavigationDestination(
                icon: Icon(Icons.rule_folder),
                label: '검토',
              ),
              NavigationDestination(
                icon: Icon(Icons.sync_alt),
                label: '상태',
              ),
            ],
            onDestinationSelected: (value) {
              setState(() {
                _index = value;
              });
            },
          ),
        );
      },
    );
  }
}
