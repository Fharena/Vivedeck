import 'package:flutter_test/flutter_test.dart';
import 'package:vibedeck_mobile/app.dart';

void main() {
  testWidgets('shows core shell labels', (tester) async {
    await tester.pumpWidget(const VibeDeckApp());

    expect(find.text('VibeDeck Mobile'), findsOneWidget);
    expect(find.text('Prompt'), findsWidgets);
    expect(find.text('Review'), findsWidgets);
    expect(find.text('Status'), findsWidgets);
  });
}
