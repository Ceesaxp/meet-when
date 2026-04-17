import SwiftUI

struct SettingsView: View {
    @Bindable var viewModel: AppViewModel
    @Environment(\.dismiss) private var dismiss

    private let pollingOptions: [(label: String, seconds: TimeInterval)] = [
        ("30 seconds", 30),
        ("1 minute", 60),
        ("2 minutes", 120),
        ("5 minutes", 300),
    ]

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            // Header
            HStack {
                Text("Settings")
                    .font(.headline)
                Spacer()
                Button(action: { viewModel.showingSettings = false }) {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(.secondary)
                }
                .buttonStyle(.plain)
            }

            Divider()

            // Account
            VStack(alignment: .leading, spacing: 4) {
                Text("Account")
                    .font(.caption)
                    .fontWeight(.semibold)
                    .foregroundStyle(.secondary)

                if let host = viewModel.host {
                    LabeledContent("Name", value: host.name)
                        .font(.caption)
                    LabeledContent("Email", value: host.email)
                        .font(.caption)
                }
                if let tenant = viewModel.tenant {
                    LabeledContent("Organization", value: tenant.name)
                        .font(.caption)
                }
                LabeledContent("Server", value: viewModel.serverURL)
                    .font(.caption)
                    .lineLimit(1)
                    .truncationMode(.middle)
            }

            Divider()

            // Polling
            VStack(alignment: .leading, spacing: 4) {
                Text("Refresh Interval")
                    .font(.caption)
                    .fontWeight(.semibold)
                    .foregroundStyle(.secondary)

                Picker("", selection: Binding(
                    get: { viewModel.pollingInterval },
                    set: { newValue in
                        viewModel.pollingInterval = newValue
                        UserDefaults.standard.set(newValue, forKey: "pollingInterval")
                    }
                )) {
                    ForEach(pollingOptions, id: \.seconds) { option in
                        Text(option.label).tag(option.seconds)
                    }
                }
                .pickerStyle(.segmented)
                .labelsHidden()
            }

            Divider()

            // Notifications
            VStack(alignment: .leading, spacing: 4) {
                Text("Notifications")
                    .font(.caption)
                    .fontWeight(.semibold)
                    .foregroundStyle(.secondary)

                Toggle("Enable notifications", isOn: Binding(
                    get: { viewModel.notificationsEnabled },
                    set: { newValue in
                        viewModel.notificationsEnabled = newValue
                        UserDefaults.standard.set(newValue, forKey: "notificationsEnabled")
                    }
                ))
                .font(.caption)
                .toggleStyle(.switch)
                .controlSize(.small)
            }

            Spacer()
        }
        .padding(20)
        .frame(width: 280, height: 340)
    }
}
