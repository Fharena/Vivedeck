import 'package:flutter/material.dart';

import '../state/app_controller.dart';

class ReviewScreen extends StatefulWidget {
  const ReviewScreen({
    super.key,
    required this.controller,
  });

  final AppController controller;

  @override
  State<ReviewScreen> createState() => _ReviewScreenState();
}

class _ReviewScreenState extends State<ReviewScreen> {
  final Map<String, Set<String>> _selectedByPath = {};

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        final files = widget.controller.patchFiles;
        final profiles = widget.controller.runProfiles;
        _syncSelection(files);

        final totalHunks = files.fold<int>(0, (sum, file) => sum + file.hunks.length);

        return ListView(
          key: const ValueKey('review-screen'),
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
          children: [
            _SectionCard(
              title: '검토 컨텍스트',
              subtitle: '현재 스레드와 워크스페이스 기준으로 패치와 실행을 검토합니다.',
              accent: const Color(0xFFFFE2B8),
              background: const Color(0xFFFFFBF2),
              child: Wrap(
                spacing: 8,
                runSpacing: 8,
                children: [
                  _MetricChip(label: '스레드', value: widget.controller.currentThreadTitle),
                  _MetricChip(label: '상태', value: widget.controller.currentThreadState),
                  _MetricChip(label: 'Job', value: widget.controller.currentJobId ?? '-'),
                  _MetricChip(
                    label: '워크스페이스',
                    value: widget.controller.adapterRuntime.workspaceRoot.isEmpty
                        ? '-'
                        : widget.controller.adapterRuntime.workspaceRoot,
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),
            _SectionCard(
              title: '패치 검토',
              subtitle: '파일 ${files.length}개 / 헝크 $totalHunks개',
              accent: const Color(0xFFFFD7AA),
              background: const Color(0xFFFFF7EC),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    widget.controller.patchSummary.isEmpty
                        ? '아직 patch가 없습니다. 대화 화면에서 프롬프트를 먼저 보내세요.'
                        : widget.controller.patchSummary,
                    style: Theme.of(context).textTheme.bodyMedium,
                  ),
                  const SizedBox(height: 12),
                  if (files.isEmpty)
                    const Text('PATCH_READY 응답이 도착하면 파일/헝크가 여기에 표시됩니다.')
                  else
                    ...files.map(
                      (file) => _PatchFileCard(
                        file: file,
                        selectedHunks: _selectedByPath[file.path] ?? <String>{},
                        onToggleHunk: (hunkId, selected) {
                          setState(() {
                            final selectedSet = _selectedByPath.putIfAbsent(
                              file.path,
                              () => <String>{},
                            );
                            if (selected) {
                              selectedSet.add(hunkId);
                            } else {
                              selectedSet.remove(hunkId);
                            }
                          });
                        },
                      ),
                    ),
                ],
              ),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: ElevatedButton(
                    onPressed: widget.controller.isLoading || files.isEmpty
                        ? null
                        : () async {
                            await widget.controller.applyPatch(
                              applyAll: true,
                              selectedByPath: _selectedByPath,
                            );

                            if (!mounted) {
                              return;
                            }
                            _showPatchSnack();
                          },
                    style: ElevatedButton.styleFrom(
                      backgroundColor: const Color(0xFF1F8C77),
                      foregroundColor: Colors.white,
                      padding: const EdgeInsets.symmetric(vertical: 14),
                    ),
                    child: const Text('전체 적용'),
                  ),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: OutlinedButton(
                    onPressed: widget.controller.isLoading || !_hasSelection()
                        ? null
                        : () async {
                            await widget.controller.applyPatch(
                              applyAll: false,
                              selectedByPath: _selectedByPath,
                            );

                            if (!mounted) {
                              return;
                            }
                            _showPatchSnack();
                          },
                    style: OutlinedButton.styleFrom(
                      padding: const EdgeInsets.symmetric(vertical: 14),
                    ),
                    child: const Text('선택 적용'),
                  ),
                ),
              ],
            ),
            if (widget.controller.patchResultStatus.isNotEmpty) ...[
              const SizedBox(height: 12),
              _InlineResultBanner(
                title: '패치 적용 결과',
                status: widget.controller.patchResultStatus,
                message: widget.controller.patchResultMessage,
              ),
            ],
            const SizedBox(height: 12),
            _SectionCard(
              title: '실행 프로파일',
              subtitle: '고정 test_all 대신 agent가 노출한 실행 목록을 사용합니다.',
              accent: const Color(0xFFC7D2FE),
              background: const Color(0xFFF6F7FF),
              child: profiles.isEmpty
                  ? const Text('노출된 실행 프로파일이 없습니다.')
                  : Column(
                      children: profiles
                          .map(
                            (profile) => _RunProfileCard(
                              profile: profile,
                              isLoading: widget.controller.isLoading,
                              enabled: (widget.controller.currentJobId ?? '').isNotEmpty,
                              onRun: () async {
                                final messenger = ScaffoldMessenger.of(context);
                                await widget.controller.runProfile(profile.id);
                                if (!mounted) {
                                  return;
                                }
                                final message = widget.controller.errorMessage ??
                                    'RUN_RESULT: ${widget.controller.runStatus} / ${widget.controller.runSummary}';
                                messenger.showSnackBar(SnackBar(content: Text(message)));
                              },
                            ),
                          )
                          .toList(),
                    ),
            ),
            if (widget.controller.runStatus.isNotEmpty ||
                widget.controller.runOutput.isNotEmpty) ...[
              const SizedBox(height: 12),
              _SectionCard(
                title: '실행 결과',
                subtitle: '요약과 함께 출력 발췌를 바로 확인합니다.',
                accent: const Color(0xFFB9E6DA),
                background: const Color(0xFFEDF9F6),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Wrap(
                      spacing: 8,
                      runSpacing: 8,
                      children: [
                        _MetricChip(
                          label: '상태',
                          value: widget.controller.runStatus,
                        ),
                        if (widget.controller.runProfiles.isNotEmpty)
                          _MetricChip(
                            label: '프로파일 수',
                            value: '${widget.controller.runProfiles.length}',
                          ),
                      ],
                    ),
                    const SizedBox(height: 10),
                    Text(
                      widget.controller.runSummary.isEmpty
                          ? '아직 실행 결과 요약이 없습니다.'
                          : widget.controller.runSummary,
                    ),
                    if (widget.controller.topErrors.isNotEmpty) ...[
                      const SizedBox(height: 10),
                      Text(
                        '상위 에러',
                        style: Theme.of(context).textTheme.titleSmall,
                      ),
                      const SizedBox(height: 6),
                      ...widget.controller.topErrors
                          .map((line) => Text('• $line')),
                    ],
                    if (widget.controller.runOutput.isNotEmpty) ...[
                      const SizedBox(height: 12),
                      Text(
                        '실행 출력',
                        style: Theme.of(context).textTheme.titleSmall,
                      ),
                      const SizedBox(height: 6),
                      Container(
                        width: double.infinity,
                        decoration: BoxDecoration(
                          color: const Color(0xFF0F172A),
                          borderRadius: BorderRadius.circular(14),
                        ),
                        padding: const EdgeInsets.all(12),
                        child: SelectableText(
                          widget.controller.runOutput,
                          style: const TextStyle(
                            color: Color(0xFFE2E8F0),
                            fontFamily: 'monospace',
                            height: 1.35,
                          ),
                        ),
                      ),
                    ],
                  ],
                ),
              ),
            ],
          ],
        );
      },
    );
  }

  void _syncSelection(List<PatchFileView> files) {
    final paths = files.map((file) => file.path).toSet();
    _selectedByPath.removeWhere((path, _) => !paths.contains(path));

    for (final file in files) {
      final selected = _selectedByPath.putIfAbsent(file.path, () => <String>{});
      final validIds = file.hunks.map((hunk) => hunk.id).toSet();
      selected.removeWhere((id) => !validIds.contains(id));
    }
  }

  bool _hasSelection() {
    for (final value in _selectedByPath.values) {
      if (value.isNotEmpty) {
        return true;
      }
    }
    return false;
  }

  void _showPatchSnack() {
    final message = widget.controller.errorMessage ??
        'PATCH_RESULT: ${widget.controller.patchResultStatus} / ${widget.controller.patchResultMessage}';
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(message)));
  }
}

class _SectionCard extends StatelessWidget {
  const _SectionCard({
    required this.title,
    required this.subtitle,
    required this.child,
    required this.accent,
    required this.background,
  });

  final String title;
  final String subtitle;
  final Widget child;
  final Color accent;
  final Color background;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: background,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: accent),
      ),
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(title, style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 4),
          Text(subtitle, style: Theme.of(context).textTheme.bodySmall),
          const SizedBox(height: 12),
          child,
        ],
      ),
    );
  }
}

class _MetricChip extends StatelessWidget {
  const _MetricChip({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
      decoration: BoxDecoration(
        color: Colors.white,
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: const Color(0xFFDCE3ED)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(label, style: Theme.of(context).textTheme.bodySmall),
          Text(
            value,
            style: Theme.of(context)
                .textTheme
                .bodyMedium
                ?.copyWith(fontWeight: FontWeight.w700),
          ),
        ],
      ),
    );
  }
}

class _InlineResultBanner extends StatelessWidget {
  const _InlineResultBanner({
    required this.title,
    required this.status,
    required this.message,
  });

  final String title;
  final String status;
  final String message;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: const Color(0xFFF6FFFA),
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: const Color(0xFFB9E6DA)),
      ),
      padding: const EdgeInsets.all(12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(title, style: Theme.of(context).textTheme.titleSmall),
          const SizedBox(height: 4),
          Text('상태: $status'),
          if (message.isNotEmpty) Text(message),
        ],
      ),
    );
  }
}

class _RunProfileCard extends StatelessWidget {
  const _RunProfileCard({
    required this.profile,
    required this.isLoading,
    required this.enabled,
    required this.onRun,
  });

  final RunProfileView profile;
  final bool isLoading;
  final bool enabled;
  final Future<void> Function() onRun;

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.only(bottom: 10),
      child: Padding(
        padding: const EdgeInsets.all(14),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text(profile.displayLabel,
                style: Theme.of(context).textTheme.titleSmall),
            const SizedBox(height: 4),
            Text(profile.command),
            const SizedBox(height: 8),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                _MetricChip(label: 'scope', value: profile.scope),
                _MetricChip(
                  label: 'optional',
                  value: profile.optional ? 'true' : 'false',
                ),
              ],
            ),
            const SizedBox(height: 10),
            Align(
              alignment: Alignment.centerLeft,
              child: OutlinedButton.icon(
                onPressed: isLoading || !enabled ? null : () => onRun(),
                icon: const Icon(Icons.play_arrow_rounded),
                label: Text('${profile.id} 실행'),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _PatchFileCard extends StatelessWidget {
  const _PatchFileCard({
    required this.file,
    required this.selectedHunks,
    required this.onToggleHunk,
  });

  final PatchFileView file;
  final Set<String> selectedHunks;
  final void Function(String hunkId, bool selected) onToggleHunk;

  @override
  Widget build(BuildContext context) {
    return Card(
      margin: const EdgeInsets.only(bottom: 10),
      child: ExpansionTile(
        tilePadding: const EdgeInsets.symmetric(horizontal: 14),
        title: Text(file.path),
        subtitle: Text('status: ${file.status}'),
        childrenPadding: const EdgeInsets.fromLTRB(10, 0, 10, 10),
        children: file.hunks
            .map(
              (hunk) => Container(
                margin: const EdgeInsets.only(bottom: 10),
                decoration: BoxDecoration(
                  color: const Color(0xFFF8FAFC),
                  borderRadius: BorderRadius.circular(12),
                  border: Border.all(color: const Color(0xFFDCE3ED)),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    CheckboxListTile(
                      dense: true,
                      controlAffinity: ListTileControlAffinity.leading,
                      value: selectedHunks.contains(hunk.id),
                      onChanged: (value) => onToggleHunk(hunk.id, value ?? false),
                      title: Text('${hunk.id} ${hunk.header}'),
                      subtitle: Text('risk: ${hunk.risk}'),
                    ),
                    Padding(
                      padding: const EdgeInsets.fromLTRB(14, 0, 14, 12),
                      child: Text(
                        hunk.diff,
                        style: Theme.of(context).textTheme.bodySmall?.copyWith(
                              fontFamily: 'monospace',
                            ),
                      ),
                    ),
                  ],
                ),
              ),
            )
            .toList(),
      ),
    );
  }
}
