import SwiftUI

struct LogViewerWindow: View {
    @ObservedObject var viewModel: LogViewModel

    var body: some View {
        VStack(spacing: 0) {
            toolbar
            Divider()
            logList
        }
        .frame(minWidth: 600, minHeight: 400)
        .task {
            await viewModel.loadFromIPC()
        }
    }

    private var toolbar: some View {
        HStack(spacing: 8) {
            HStack(spacing: 4) {
                Image(systemName: "magnifyingglass")
                    .foregroundColor(.secondary)
                TextField("Search logs...", text: $viewModel.searchText)
                    .textFieldStyle(.plain)
                if !viewModel.searchText.isEmpty {
                    Button {
                        viewModel.searchText = ""
                    } label: {
                        Image(systemName: "xmark.circle.fill")
                            .foregroundColor(.secondary)
                    }
                    .buttonStyle(.plain)
                }
            }
            .padding(6)
            .background(Color(nsColor: .controlBackgroundColor))
            .cornerRadius(6)

            levelFilters
            autoScrollToggle
        }
        .padding(8)
    }

    private var levelFilters: some View {
        HStack(spacing: 2) {
            ForEach(LogLevel.allCases, id: \.rawValue) { level in
                Button {
                    viewModel.toggleLevel(level)
                } label: {
                    Text(level.shortLabel)
                        .font(.caption.weight(.medium))
                        .padding(.horizontal, 6)
                        .padding(.vertical, 2)
                        .background(viewModel.isLevelActive(level) ? level.color.opacity(0.2) : Color.clear)
                        .foregroundColor(viewModel.isLevelActive(level) ? level.color : .secondary)
                        .cornerRadius(4)
                }
                .buttonStyle(.plain)
            }
        }
    }

    private var autoScrollToggle: some View {
        Button {
            viewModel.autoScroll.toggle()
        } label: {
            Image(systemName: viewModel.autoScroll ? "arrow.down.to.line.circle.fill" : "arrow.down.to.line")
                .foregroundColor(viewModel.autoScroll ? .accentColor : .secondary)
        }
        .buttonStyle(.plain)
        .help(viewModel.autoScroll ? "Auto-scroll enabled" : "Auto-scroll disabled")
    }

    private var logList: some View {
        ScrollViewReader { proxy in
            List(viewModel.filteredEntries) { entry in
                LogEntryRow(entry: entry)
                    .id(entry.id)
                    .listRowSeparator(.hidden)
                    .listRowInsets(EdgeInsets(top: 1, leading: 8, bottom: 1, trailing: 8))
            }
            .listStyle(.plain)
            .onChange(of: viewModel.filteredEntries.count) { _ in
                if viewModel.autoScroll, let lastID = viewModel.filteredEntries.last?.id {
                    proxy.scrollTo(lastID, anchor: .bottom)
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

    private static let timeFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "HH:mm:ss.SSS"
        return f
    }()

    var body: some View {
        HStack(alignment: .top, spacing: 8) {
            Text(Self.timeFormatter.string(from: entry.time))
                .font(.system(.caption, design: .monospaced))
                .foregroundColor(.secondary)
                .frame(width: 85, alignment: .leading)

            Text(entry.level.shortLabel)
                .font(.system(.caption, design: .monospaced).weight(.bold))
                .foregroundColor(entry.level.color)
                .frame(width: 36, alignment: .leading)

            Text(entry.message)
                .font(.system(.caption, design: .monospaced))
                .lineLimit(nil)
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        .padding(.vertical, 1)
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
        case .debug: return .secondary
        case .info: return .blue
        case .warn: return .orange
        case .error: return .red
        }
    }
}
