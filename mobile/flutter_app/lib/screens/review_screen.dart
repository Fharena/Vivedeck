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
        _syncSelection(files);

        final totalHunks = files.fold<int>(0, (sum, file) => sum + file.hunks.length);

        return ListView(
          key: const ValueKey('review-screen'),
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
          children: [
            Container(
              decoration: BoxDecoration(
                borderRadius: BorderRadius.circular(16),
                gradient: const LinearGradient(
                  begin: Alignment.topLeft,
                  end: Alignment.bottomRight,
                  colors: [Color(0xFFFFF7EC), Color(0xFFFFFDF8)],
                ),
                border: Border.all(color: const Color(0xFFFFD7AA)),
              ),
              padding: const EdgeInsets.all(14),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('PATCH_READY', style: Theme.of(context).textTheme.titleMedium),
                  const SizedBox(height: 6),
                  Text('파일 ${files.length}개 / 헝크 $totalHunks개', style: Theme.of(context).textTheme.bodyMedium),
                  const SizedBox(height: 4),
                  Text(
                    widget.controller.patchSummary.isEmpty
                        ? '아직 patch가 없습니다. Prompt 제출 후 응답을 기다려주세요.'
                        : widget.controller.patchSummary,
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),
            if (files.isEmpty)
              Card(
                child: Padding(
                  padding: const EdgeInsets.all(16),
                  child: Text(
                    'Prompt 제출 후 PATCH_READY 응답이 도착하면 파일/헝크가 표시됩니다.',
                    style: Theme.of(context).textTheme.bodyMedium,
                  ),
                ),
              )
            else
              ...files.map(
                (file) => _PatchFileCard(
                  file: file,
                  selectedHunks: _selectedByPath[file.path] ?? <String>{},
                  onToggleHunk: (hunkId, selected) {
                    setState(() {
                      final selectedSet = _selectedByPath.putIfAbsent(file.path, () => <String>{});
                      if (selected) {
                        selectedSet.add(hunkId);
                      } else {
                        selectedSet.remove(hunkId);
                      }
                    });
                  },
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
                            _showResultSnack();
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
                            _showResultSnack();
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
              Text(
                'PATCH_RESULT: ${widget.controller.patchResultStatus} / ${widget.controller.patchResultMessage}',
                style: Theme.of(context).textTheme.bodyMedium,
              ),
            ],
            const SizedBox(height: 12),
            OutlinedButton.icon(
              onPressed: widget.controller.isLoading || (widget.controller.currentJobId ?? '').isEmpty
                  ? null
                  : () async {
                      await widget.controller.runProfile('test_all');
                      if (!mounted) {
                        return;
                      }
                      final message = widget.controller.errorMessage ??
                          'RUN_RESULT: ${widget.controller.runStatus} / ${widget.controller.runSummary}';
                      ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(message)));
                    },
              icon: const Icon(Icons.play_arrow_rounded),
              label: const Text('test_all 실행'),
            ),
            if (widget.controller.runSummary.isNotEmpty) ...[
              const SizedBox(height: 8),
              Text('RUN_RESULT: ${widget.controller.runStatus}', style: Theme.of(context).textTheme.titleSmall),
              const SizedBox(height: 4),
              Text(widget.controller.runSummary),
              if (widget.controller.topErrors.isNotEmpty) ...[
                const SizedBox(height: 6),
                ...widget.controller.topErrors.map((line) => Text('• $line')),
              ],
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

  void _showResultSnack() {
    final message = widget.controller.errorMessage ??
        'PATCH_RESULT: ${widget.controller.patchResultStatus} / ${widget.controller.patchResultMessage}';
    ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(message)));
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
