import 'package:flutter/material.dart';

class ReviewScreen extends StatefulWidget {
  const ReviewScreen({super.key});

  @override
  State<ReviewScreen> createState() => _ReviewScreenState();
}

class _ReviewScreenState extends State<ReviewScreen> {
  final List<FilePatchView> _files = [
    FilePatchView(
      path: 'src/auth/middleware.ts',
      status: 'modified',
      hunks: [
        HunkView(
          id: 'h1',
          header: '@@ -12,7 +12,9 @@',
          diff: '- if (!token) throw new Error()\n+ if (!token) return res.status(401).send()',
          risk: 'low',
        ),
      ],
    ),
    FilePatchView(
      path: 'tests/auth/middleware.test.ts',
      status: 'modified',
      hunks: [
        HunkView(
          id: 'h2',
          header: '@@ -41,6 +41,6 @@',
          diff: '- expect(status).toBe(500)\n+ expect(status).toBe(401)',
          risk: 'low',
        ),
      ],
    ),
  ];

  final Set<String> _selectedHunks = {'h1'};

  @override
  Widget build(BuildContext context) {
    final totalHunks = _files.fold<int>(0, (sum, file) => sum + file.hunks.length);

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
              Text('파일 ${_files.length}개 / 헝크 $totalHunks개', style: Theme.of(context).textTheme.bodyMedium),
              const SizedBox(height: 4),
              Text(
                '선택 적용 모드에서는 체크된 헝크만 적용됩니다.',
                style: Theme.of(context).textTheme.bodySmall,
              ),
            ],
          ),
        ),
        const SizedBox(height: 12),
        ..._files.map((file) => _PatchFileCard(
              file: file,
              selectedHunks: _selectedHunks,
              onToggleHunk: (hunkId, selected) {
                setState(() {
                  if (selected) {
                    _selectedHunks.add(hunkId);
                  } else {
                    _selectedHunks.remove(hunkId);
                  }
                });
              },
            )),
        const SizedBox(height: 12),
        Row(
          children: [
            Expanded(
              child: ElevatedButton(
                onPressed: () {
                  ScaffoldMessenger.of(context).showSnackBar(
                    const SnackBar(content: Text('PATCH_APPLY(all) 요청 전송')), 
                  );
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
                onPressed: _selectedHunks.isEmpty
                    ? null
                    : () {
                        ScaffoldMessenger.of(context).showSnackBar(
                          SnackBar(content: Text('PATCH_APPLY(selected) ${_selectedHunks.length}개 헝크')), 
                        );
                      },
                style: OutlinedButton.styleFrom(
                  padding: const EdgeInsets.symmetric(vertical: 14),
                ),
                child: const Text('선택 적용'),
              ),
            ),
          ],
        ),
      ],
    );
  }
}

class _PatchFileCard extends StatelessWidget {
  const _PatchFileCard({
    required this.file,
    required this.selectedHunks,
    required this.onToggleHunk,
  });

  final FilePatchView file;
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

class FilePatchView {
  const FilePatchView({
    required this.path,
    required this.status,
    required this.hunks,
  });

  final String path;
  final String status;
  final List<HunkView> hunks;
}

class HunkView {
  const HunkView({
    required this.id,
    required this.header,
    required this.diff,
    required this.risk,
  });

  final String id;
  final String header;
  final String diff;
  final String risk;
}
