import 'package:flutter/material.dart';

import '../state/app_controller.dart';

class PromptScreen extends StatefulWidget {
  const PromptScreen({
    super.key,
    required this.controller,
  });

  final AppController controller;

  @override
  State<PromptScreen> createState() => _PromptScreenState();
}

class _PromptScreenState extends State<PromptScreen> {
  final _promptController = TextEditingController(
    text: '테스트 실패 원인을 분석하고 auth middleware 패치를 제안해줘.',
  );

  final List<String> _templates = [
    'fix_bug',
    'refactor',
    'test_add',
    'perf_tune',
  ];

  String _selectedTemplate = 'fix_bug';

  final Map<String, bool> _context = {
    'activeFile': true,
    'selection': true,
    'latestError': true,
    'workspaceSummary': false,
  };

  @override
  void dispose() {
    _promptController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: widget.controller,
      builder: (context, _) {
        final colors = Theme.of(context).colorScheme;

        return ListView(
          key: const ValueKey('prompt-screen'),
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
          children: [
            _SectionCard(
              title: 'Prompt 작성',
              subtitle: '모바일에서 에이전트 작업을 시작합니다.',
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  TextField(
                    controller: _promptController,
                    minLines: 4,
                    maxLines: 7,
                    decoration: InputDecoration(
                      hintText: '수정 목표와 제약사항을 구체적으로 적어주세요.',
                      filled: true,
                      fillColor: Colors.white,
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(12),
                        borderSide: BorderSide.none,
                      ),
                    ),
                  ),
                  const SizedBox(height: 12),
                  Text('Template', style: Theme.of(context).textTheme.bodyMedium),
                  const SizedBox(height: 8),
                  Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    children: _templates
                        .map(
                          (template) => ChoiceChip(
                            label: Text(template),
                            selected: template == _selectedTemplate,
                            onSelected: (_) {
                              setState(() {
                                _selectedTemplate = template;
                              });
                            },
                          ),
                        )
                        .toList(),
                  ),
                  const SizedBox(height: 8),
                  Text(
                    'SID: ${widget.controller.sessionId.isEmpty ? 'sid-mobile-demo' : widget.controller.sessionId}',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),
            _SectionCard(
              title: 'Context 옵션',
              subtitle: '필요한 컨텍스트만 선택해 토큰/응답 품질을 조절합니다.',
              child: Column(
                children: [
                  _ContextTile(
                    label: 'Active File',
                    value: _context['activeFile']!,
                    onChanged: (value) => setState(() => _context['activeFile'] = value),
                  ),
                  _ContextTile(
                    label: 'Selection',
                    value: _context['selection']!,
                    onChanged: (value) => setState(() => _context['selection'] = value),
                  ),
                  _ContextTile(
                    label: 'Latest Error',
                    value: _context['latestError']!,
                    onChanged: (value) => setState(() => _context['latestError'] = value),
                  ),
                  _ContextTile(
                    label: 'Workspace Summary',
                    value: _context['workspaceSummary']!,
                    onChanged: (value) => setState(() => _context['workspaceSummary'] = value),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),
            Row(
              children: [
                Expanded(
                  child: ElevatedButton.icon(
                    icon: const Icon(Icons.send_rounded),
                    label: const Text('Prompt 제출'),
                    style: ElevatedButton.styleFrom(
                      padding: const EdgeInsets.symmetric(vertical: 14),
                      backgroundColor: const Color(0xFF1F8C77),
                      foregroundColor: Colors.white,
                    ),
                    onPressed: widget.controller.isLoading
                        ? null
                        : () async {
                            await widget.controller.submitPrompt(
                              prompt: _promptController.text,
                              template: _selectedTemplate,
                              context: _context,
                            );

                            if (!mounted) {
                              return;
                            }

                            final error = widget.controller.errorMessage;
                            ScaffoldMessenger.of(context).showSnackBar(
                              SnackBar(
                                content: Text(
                                  error ?? 'PROMPT_SUBMIT 완료',
                                ),
                              ),
                            );
                          },
                  ),
                ),
                const SizedBox(width: 10),
                Expanded(
                  child: OutlinedButton.icon(
                    icon: const Icon(Icons.restart_alt),
                    label: const Text('초안 초기화'),
                    style: OutlinedButton.styleFrom(
                      padding: const EdgeInsets.symmetric(vertical: 14),
                    ),
                    onPressed: () {
                      setState(() {
                        _promptController.text = '';
                        _selectedTemplate = _templates.first;
                      });
                    },
                  ),
                ),
              ],
            ),
            const SizedBox(height: 12),
            Container(
              decoration: BoxDecoration(
                color: colors.surface,
                borderRadius: BorderRadius.circular(14),
                border: Border.all(color: colors.outlineVariant),
              ),
              padding: const EdgeInsets.all(14),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('최근 제출', style: Theme.of(context).textTheme.titleMedium),
                  const SizedBox(height: 8),
                  Text(
                    widget.controller.promptDraft.isEmpty
                        ? '아직 제출된 prompt가 없습니다.'
                        : widget.controller.promptDraft,
                    style: Theme.of(context).textTheme.bodyMedium,
                  ),
                  const SizedBox(height: 8),
                  Text(
                    'jobId: ${widget.controller.currentJobId ?? '-'}',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                ],
              ),
            ),
            if (widget.controller.errorMessage != null) ...[
              const SizedBox(height: 12),
              Text(
                widget.controller.errorMessage!,
                style: TextStyle(color: colors.error, fontWeight: FontWeight.w700),
              ),
            ],
          ],
        );
      },
    );
  }
}

class _SectionCard extends StatelessWidget {
  const _SectionCard({
    required this.title,
    required this.subtitle,
    required this.child,
  });

  final String title;
  final String subtitle;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;

    return Container(
      decoration: BoxDecoration(
        borderRadius: BorderRadius.circular(16),
        gradient: const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [Color(0xFFFFFFFF), Color(0xFFF5FFFA)],
        ),
        border: Border.all(color: colors.outlineVariant),
      ),
      padding: const EdgeInsets.all(14),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(title, style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 4),
          Text(
            subtitle,
            style: Theme.of(context).textTheme.bodySmall?.copyWith(color: colors.primary),
          ),
          const SizedBox(height: 12),
          child,
        ],
      ),
    );
  }
}

class _ContextTile extends StatelessWidget {
  const _ContextTile({
    required this.label,
    required this.value,
    required this.onChanged,
  });

  final String label;
  final bool value;
  final ValueChanged<bool> onChanged;

  @override
  Widget build(BuildContext context) {
    return SwitchListTile.adaptive(
      contentPadding: EdgeInsets.zero,
      value: value,
      title: Text(label),
      onChanged: onChanged,
    );
  }
}
