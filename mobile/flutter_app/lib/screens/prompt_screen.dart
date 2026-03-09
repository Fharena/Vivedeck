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
          key: const ValueKey('thread-screen'),
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
          children: [
            _SectionCard(
              title: '공유 스레드',
              subtitle: '모바일과 IDE가 같은 작업 히스토리를 공유합니다.',
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    children: [
                      _MetricChip(
                        label: '현재 스레드',
                        value: widget.controller.currentThreadTitle,
                      ),
                      _MetricChip(
                        label: '현재 Job',
                        value: widget.controller.currentJobId ?? '-',
                      ),
                      _MetricChip(
                        label: '작업 디렉토리',
                        value: widget.controller.adapterRuntime.workspaceRoot.isEmpty
                            ? '-'
                            : widget.controller.adapterRuntime.workspaceRoot,
                      ),
                      _MetricChip(
                        label: '스레드 상태',
                        value: widget.controller.currentThreadState,
                      ),
                      _MetricChip(
                        label: '실행 프로파일',
                        value: '${widget.controller.runProfiles.length}',
                      ),
                    ],
                  ),
                  const SizedBox(height: 10),
                  Row(
                    children: [
                      Expanded(
                        child: FilledButton.tonalIcon(
                          onPressed: () {
                            widget.controller.beginNewThread();
                            _promptController.clear();
                            widget.controller.updatePromptDraft('');
                          },
                          icon: const Icon(Icons.add_comment_outlined),
                          label: const Text('새 스레드'),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
            const SizedBox(height: 12),
            _SectionCard(
              title: '최근 스레드',
              subtitle: '기존 스레드를 선택하면 같은 대화를 이어서 진행합니다.',
              child: widget.controller.threads.isEmpty
                  ? const Text('아직 생성된 스레드가 없습니다.')
                  : Column(
                      children: widget.controller.threads
                          .take(6)
                          .map(
                            (thread) => _ThreadTile(
                              thread: thread,
                              selected: thread.id == widget.controller.currentThreadId,
                              onTap: () async {
                                await widget.controller.selectThread(thread.id);
                              },
                            ),
                          )
                          .toList(),
                    ),
            ),
            const SizedBox(height: 12),
            _SectionCard(
              title: '실시간 세션',
              subtitle: 'Cursor와 모바일의 참여자, 초안, 포커스를 같은 세션에서 공유합니다.',
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Wrap(
                    spacing: 8,
                    runSpacing: 8,
                    children: [
                      _MetricChip(
                        label: '세션 단계',
                        value: widget.controller.currentSessionPhase,
                      ),
                      _MetricChip(
                        label: '참여자',
                        value: widget.controller.liveParticipantSummary,
                      ),
                      _MetricChip(
                        label: '활동',
                        value: widget.controller.liveComposerTyping
                            ? '작성 중'
                            : '대기',
                      ),
                    ],
                  ),
                  const SizedBox(height: 10),
                  Text(
                    widget.controller.liveActivitySummary,
                    style: Theme.of(context).textTheme.bodyMedium,
                  ),
                  const SizedBox(height: 8),
                  Text(
                    '포커스: ${widget.controller.liveFocusSummary}',
                    style: Theme.of(context).textTheme.bodySmall,
                  ),
                  if (widget.controller.liveDraftPreview.isNotEmpty) ...[
                    const SizedBox(height: 10),
                    Container(
                      width: double.infinity,
                      padding: const EdgeInsets.all(12),
                      decoration: BoxDecoration(
                        color: const Color(0xFFF5FBF8),
                        borderRadius: BorderRadius.circular(12),
                        border: Border.all(color: colors.outlineVariant),
                      ),
                      child: Text(
                        widget.controller.liveDraftPreview,
                        style: Theme.of(context).textTheme.bodyMedium,
                      ),
                    ),
                  ],
                ],
              ),
            ),
            const SizedBox(height: 12),
            _SectionCard(
              title: '대화 타임라인',
              subtitle: '프롬프트, 패치, 실행 결과가 하나의 흐름으로 누적됩니다.',
              child: widget.controller.threadEvents.isEmpty
                  ? const Text('스레드 이벤트가 없습니다. 프롬프트를 먼저 보내보세요.')
                  : Column(
                      children: widget.controller.threadEvents
                          .map((event) => _ThreadEventTile(event: event))
                          .toList(),
                    ),
            ),
            const SizedBox(height: 12),
            _SectionCard(
              title: '프롬프트 작성',
              subtitle: '템플릿 대신 자연어로 목적과 제약사항을 구체적으로 적습니다.',
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  TextField(
                    controller: _promptController,
                    minLines: 4,
                    maxLines: 7,
                    onChanged: widget.controller.updatePromptDraft,
                    decoration: InputDecoration(
                      hintText: '예: src/hello.py 파일을 만들고 hello world만 출력하게 해줘.',
                      filled: true,
                      fillColor: Colors.white,
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(12),
                        borderSide: BorderSide.none,
                      ),
                    ),
                  ),
                  const SizedBox(height: 12),
                  Text('컨텍스트 옵션',
                      style: Theme.of(context).textTheme.bodyMedium),
                  const SizedBox(height: 8),
                  _ContextTile(
                    label: '활성 파일',
                    value: _context['activeFile']!,
                    onChanged: (value) =>
                        setState(() => _context['activeFile'] = value),
                  ),
                  _ContextTile(
                    label: '선택 영역',
                    value: _context['selection']!,
                    onChanged: (value) =>
                        setState(() => _context['selection'] = value),
                  ),
                  _ContextTile(
                    label: '최근 오류',
                    value: _context['latestError']!,
                    onChanged: (value) =>
                        setState(() => _context['latestError'] = value),
                  ),
                  _ContextTile(
                    label: '워크스페이스 요약',
                    value: _context['workspaceSummary']!,
                    onChanged: (value) => setState(
                      () => _context['workspaceSummary'] = value,
                    ),
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
                            final messenger = ScaffoldMessenger.of(context);
                            await widget.controller.submitPrompt(
                              prompt: _promptController.text,
                              context: _context,
                            );

                            if (!mounted) {
                              return;
                            }

                            final error = widget.controller.errorMessage;
                            if (error == null) {
                              _promptController.clear();
                              widget.controller.updatePromptDraft('');
                            }
                            messenger.showSnackBar(
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
                    label: const Text('입력 지우기'),
                    style: OutlinedButton.styleFrom(
                      padding: const EdgeInsets.symmetric(vertical: 14),
                    ),
                    onPressed: () {
                      _promptController.clear();
                      widget.controller.updatePromptDraft('');
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
                  Text('공유 초안', style: Theme.of(context).textTheme.titleMedium),
                  const SizedBox(height: 8),
                  Text(
                    widget.controller.liveDraftPreview.isNotEmpty
                        ? widget.controller.liveDraftPreview
                        : (widget.controller.promptDraft.isEmpty
                            ? '아직 공유된 draft가 없습니다.'
                            : widget.controller.promptDraft),
                    style: Theme.of(context).textTheme.bodyMedium,
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
            style: Theme.of(context)
                .textTheme
                .bodySmall
                ?.copyWith(color: colors.primary),
          ),
          const SizedBox(height: 12),
          child,
        ],
      ),
    );
  }
}

class _ThreadTile extends StatelessWidget {
  const _ThreadTile({
    required this.thread,
    required this.selected,
    required this.onTap,
  });

  final ThreadSummaryView thread;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final colors = Theme.of(context).colorScheme;
    return Card(
      margin: const EdgeInsets.only(bottom: 8),
      color: selected ? const Color(0xFFE9F7F2) : null,
      child: ListTile(
        onTap: onTap,
        leading: Icon(
          selected ? Icons.forum : Icons.chat_bubble_outline,
          color: colors.primary,
        ),
        title: Text(thread.title),
        subtitle: Text(
          thread.lastEventText.isEmpty ? thread.state : thread.lastEventText,
          maxLines: 2,
          overflow: TextOverflow.ellipsis,
        ),
        trailing: Text(thread.updatedAtLabel),
      ),
    );
  }
}

class _ThreadEventTile extends StatelessWidget {
  const _ThreadEventTile({required this.event});

  final ThreadEventView event;

  @override
  Widget build(BuildContext context) {
    final isUser = event.role == 'user';
    final bgColor = isUser ? const Color(0xFFE9F7F2) : const Color(0xFFF5F7FF);
    return Container(
      margin: const EdgeInsets.only(bottom: 10),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: bgColor,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(color: const Color(0xFFDCE3ED)),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(
                isUser ? Icons.person_outline : Icons.smart_toy_outlined,
                size: 18,
              ),
              const SizedBox(width: 6),
              Expanded(
                child: Text(
                  event.title,
                  style: Theme.of(context).textTheme.titleSmall,
                ),
              ),
              Text(event.atLabel,
                  style: Theme.of(context).textTheme.bodySmall),
            ],
          ),
          if (event.body.isNotEmpty) ...[
            const SizedBox(height: 8),
            Text(event.body),
          ],
          if (event.data['status'] != null || event.data['profileId'] != null) ...[
            const SizedBox(height: 8),
            Wrap(
              spacing: 8,
              runSpacing: 8,
              children: [
                if (event.data['status'] != null)
                  _MetricChip(
                    label: 'status',
                    value: event.data['status'].toString(),
                  ),
                if (event.data['profileId'] != null)
                  _MetricChip(
                    label: 'profile',
                    value: event.data['profileId'].toString(),
                  ),
                if (event.data['fileCount'] != null)
                  _MetricChip(
                    label: 'files',
                    value: event.data['fileCount'].toString(),
                  ),
              ],
            ),
          ],
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
        border: Border.all(color: const Color(0xFFD6E9E3)),
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
