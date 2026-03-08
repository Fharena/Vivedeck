import 'dart:async';

import 'package:app_links/app_links.dart';

abstract class BootstrapLinkSource {
  Future<Uri?> getInitialUri();

  Stream<Uri> get uriStream;
}

class AppLinksBootstrapLinkSource implements BootstrapLinkSource {
  AppLinksBootstrapLinkSource({AppLinks? appLinks}) : _appLinks = appLinks ?? AppLinks();

  final AppLinks _appLinks;

  @override
  Future<Uri?> getInitialUri() {
    return _appLinks.getInitialLink();
  }

  @override
  Stream<Uri> get uriStream {
    return _appLinks.uriLinkStream;
  }
}

class NoopBootstrapLinkSource implements BootstrapLinkSource {
  const NoopBootstrapLinkSource();

  @override
  Future<Uri?> getInitialUri() async => null;

  @override
  Stream<Uri> get uriStream => const Stream<Uri>.empty();
}
