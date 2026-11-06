from pathlib import Path
import sys

path = Path(sys.argv[1])
src = path.read_text(encoding='utf-8')
old_columns = '''    private var columns: [GridItem] {
        [GridItem(.adaptive(minimum: CGFloat(DirectReadabilityPolicy.minimumTileWidth(
            accessibilityText: dynamicTypeSize.isAccessibilitySize)), spacing: 12))]
    }
'''
new_columns = '''    private var columns: [GridItem] {
        let minimumWidth = CGFloat(DirectReadabilityPolicy.minimumTileWidth(
            accessibilityText: dynamicTypeSize.isAccessibilitySize))
        return [GridItem(.adaptive(minimum: minimumWidth), spacing: 12)]
    }
'''
assert src.count(old_columns) == 1, 'P52 GridItem anchor must be unique'
src = src.replace(old_columns, new_columns, 1)
reader = src.index('private struct DirectMessageReaderView: View {')
body = src.index('    var body: some View {', reader)
content_start = src.index('                    if !hasAccess {', body)
content_end = src.index('                }\n                .padding(16)\n                .frame(maxWidth:', content_start)
contents = src[content_start:content_end]
attachments_start = contents.index('                        if !detail.attachments.isEmpty {')
attachments_end = contents.index('                        Text("Pliki pozostają', attachments_start)
attachments = contents[attachments_start:attachments_end]
contents = contents[:attachments_start] + '                        attachmentContents(detail)\n' + contents[attachments_end:]
detail_start = contents.index('                    } else if let detail = displayedDetail {')
detail_body_start = detail_start + len('                    } else if let detail = displayedDetail {\n')
detail_end = contents.index('                    } else if portal.directMessageDetailBusy {', detail_body_start)
detail_body = contents[detail_body_start:detail_end]
contents = contents[:detail_body_start] + '                        messageContent(detail)\n' + contents[detail_end:]
helpers = '''    /// Separate result-builder expressions to reduce Xcode type-checking work.
    /// Content, permission gates and side effects remain unchanged.
    @ViewBuilder
    private var readerContents: some View {
''' + contents + '''    }

    @ViewBuilder
    private func messageContent(_ detail: DirectMessageDetail) -> some View {
''' + detail_body + '''    }

    @ViewBuilder
    private func attachmentContents(_ detail: DirectMessageDetail) -> some View {
''' + attachments + '''    }

'''
src = src[:body] + helpers + src[body:]
body = src.index('    var body: some View {', src.index('private struct DirectMessageReaderView: View {'))
content_start = src.index('                    if !hasAccess {', body)
content_end = src.index('                }\n                .padding(16)\n                .frame(maxWidth:', content_start)
src = src[:content_start] + '                    readerContents\n' + src[content_end:]
for part in ('private func attachmentContents(', 'private func messageContent(', 'private var readerContents:'):
    assert src.count(part) == 1, part
text_size_decl = '    @State private var textSize: DirectMessageReaderTextSize = .normal\n'
assert src.count(text_size_decl) == 1
src = src.replace(text_size_decl,
    text_size_decl + '    @ScaledMetric(relativeTo: .body) private var bodyTextScale: CGFloat = 1\n', 1)
old_font = '.font(.system(size: textSize.pointSize, relativeTo: .body))'
assert src.count(old_font) == 1
src = src.replace(old_font, '.font(.system(size: textSize.pointSize * bodyTextScale))', 1)
path.write_text(src, encoding='utf-8')
print('P53 SwiftUI compile repairs applied to original P52 source')
