import SwiftUI

struct LogViewerWindow: View {
    @EnvironmentObject var viewModel: LogViewModel

    var body: some View {
        VStack(spacing: 0) {
            logContent
        }
        .frame(minWidth: 700, minHeight: 450)
        .toolbar {
            ToolbarItemGroup(placement: .automatic) {
                levelFilterButtons
                Spacer()
                autoScrollToggle
                clearButton
            }
        }
        .searchable(text: $viewModel.searchText, prompt: "Search logs...")
        .task {
            await viewModel.loadFromIPC()
        }
    }

    private var logContent: some View {
        ScrollViewReader { proxy in
            ScrollView {
                LazyVStack(alignment: .leading, spacing: 0) {
                    ForEach(viewModel.filteredEntries) { entry in
                        LogEntryRow(entry: entry)
                            .id(entry.id)
                    }
                }
                .padding(.horizontal, 8)
                .padding(.vertical, 4)
            }
            .onChange(of: viewModel.filteredEntries.count) { _ in
                if viewModel.autoScroll, let lastID = viewModel.filteredEntries.last?.id {
                    withAnimation(.easeOut(duration: 0.2)) {
                        proxy.scrollTo(lastID, anchor: .bottom)
                    }
                }
            }
            .overlay {
                if viewModel.isLoading {
                    ProgressView("Loading logs...")
                } else if viewModel.filteredEntries.isEmpty {
                    emptyState
                }
            }
        }
        .background(Color(nsColor: .textBackgroundColor))
    }

    private var levelFilterButtons: some View {
        HStack(spacing: 2) {
            ForEach(LogLevel.allCases, id: \.rawValue) { level in
                Button {
                    viewModel.toggleLevel(level)
                } label: {
                    HStack(spacing: 4) {
                        Circle()
                            .fill(level.color)
                            .frame(width: 6, height: 6)
                        Text(level.shortLabel)
                            .font(.caption.weight(.medium))
                    }
                    .padding(.horizontal, 8)
                    .padding(.vertical, 4)
                    .background(
                        RoundedRectangle(cornerRadius: 5)
                            .fill(viewModel.isLevelActive(level) ? level.color.opacity(0.15) : Color.clear)
                    )
                    .foregroundColor(viewModel.isLevelActive(level) ? level.color : .secondary.opacity(0.5))
                }
                .buttonStyle(.plain)
            }
        }
    }

    private var autoScrollToggle: some View {
        Button {
            viewModel.autoScroll.toggle()
        } label: {
            Image(systemName: viewModel.autoScroll ? "arrow.down.to.line.circle.fill" : "arrow.down.to.line.circle")
                .foregroundColor(viewModel.autoScroll ? .accentColor : .secondary)
        }
        .buttonStyle(.plain)
        .help(viewModel.autoScroll ? "Auto-scroll enabled" : "Auto-scroll disabled")
    }

    private var clearButton: some View {
        Button {
            viewModel.clearEntries()
        } label: {
            Image(systemName: "trash")
                .foregroundColor(.secondary)
        }
        .buttonStyle(.plain)
        .help("Clear logs")
    }

    private var emptyState: some View {
        VStack(spacing: 8) {
            Image(systemName: "doc.text")
                .font(.largeTitle)
                .foregroundColor(.secondary)
            if !viewModel.searchText.isEmpty || viewModel.activeLevels.count < LogLevel.allCases.count {
                Text("No matching log entries")
                    .foregroundColor(.secondary)
            } else {
                Text("No log entries yet")
                    .foregroundColor(.secondary)
            }
        }
    }
}

// MARK: - Log Entry Row

struct LogEntryRow: View {
    let entry: LogEntry
    @State private var isExpanded: Bool = false
    @State private var isHovered: Bool = false

    private static let timeFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "HH:mm:ss.SSS"
        return f
    }()

    private static let messageTruncationLimit = 200

    var body: some View {
        HStack(alignment: .top, spacing: 8) {
            Text(Self.timeFormatter.string(from: entry.time))
                .font(.system(.caption, design: .monospaced))
                .foregroundColor(.secondary)
                .frame(width: 85, alignment: .leading)

            Text(entry.level.shortLabel)
                .font(.system(.caption, design: .monospaced).weight(.bold))
                .foregroundColor(.white)
                .padding(.horizontal, 4)
                .padding(.vertical, 1)
                .background(
                    RoundedRectangle(cornerRadius: 3)
                        .fill(entry.level.color.opacity(0.8))
                )
                .frame(width: 40, alignment: .leading)

            messageView
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.vertical, 3)
        .padding(.horizontal, 4)
        .background(
            RoundedRectangle(cornerRadius: 3)
                .fill(isHovered ? Color(nsColor: .selectedContentBackgroundColor).opacity(0.1) : Color.clear)
        )
        .onHover { hovering in
            isHovered = hovering
        }
        .contextMenu {
            Button("Copy Message") {
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(entry.message, forType: .string)
            }
            Button("Copy Full Entry") {
                let text = "\(Self.timeFormatter.string(from: entry.time)) [\(entry.level.shortLabel)] \(entry.message)"
                NSPasteboard.general.clearContents()
                NSPasteboard.general.setString(text, forType: .string)
            }
        }
    }

    @ViewBuilder
    private var messageView: some View {
        let isTruncated = entry.message.count > Self.messageTruncationLimit

        if isTruncated && !isExpanded {
            VStack(alignment: .leading, spacing: 2) {
                Text(String(entry.message.prefix(Self.messageTruncationLimit)) + "...")
                    .font(.system(.caption, design: .monospaced))
                    .lineLimit(nil)
                Button {
                    isExpanded = true
                } label: {
                    Text("Show more")
                        .font(.caption2)
                        .foregroundColor(.accentColor)
                }
                .buttonStyle(.plain)
            }
        } else {
            VStack(alignment: .leading, spacing: 2) {
                Text(entry.message)
                    .font(.system(.caption, design: .monospaced))
                    .lineLimit(nil)
                    .textSelection(.enabled)
                if isTruncated {
                    Button {
                        isExpanded = false
                    } label: {
                        Text("Show less")
                            .font(.caption2)
                            .foregroundColor(.accentColor)
                    }
                    .buttonStyle(.plain)
                }
            }
        }
    }
}

// MARK: - LogLevel UI extensions

extension LogLevel {
    var shortLabel: String {
        switch self {
        case .debug: return "DBG"
        case .info: return "INF"
        case .warn: return "WRN"
        case .error: return "ERR"
        }
    }

    var color: Color {
        switch self {
        case .debug: return .gray
        case .info: return .blue
        case .warn: return .orange
        case .error: return .red
        }
    }
}
