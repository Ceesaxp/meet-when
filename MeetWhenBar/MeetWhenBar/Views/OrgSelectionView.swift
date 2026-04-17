import SwiftUI

struct OrgSelectionView: View {
    @Bindable var viewModel: AppViewModel
    let orgs: [OrgOption]

    var body: some View {
        VStack(spacing: 12) {
            Text("Select Organization")
                .font(.headline)

            Text("Your account belongs to multiple organizations.")
                .font(.caption)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)

            Divider()

            ForEach(orgs) { org in
                Button(action: {
                    Task { await viewModel.selectOrg(org) }
                }) {
                    HStack {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(org.tenantName)
                                .font(.body)
                            Text(org.tenantSlug)
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                        Spacer()
                        Image(systemName: "chevron.right")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .padding(.vertical, 4)
            }

            if let error = viewModel.errorMessage {
                Text(error)
                    .font(.caption)
                    .foregroundStyle(.red)
            }

            Divider()

            Button("Back to Login") {
                viewModel.authState = .loggedOut
            }
            .buttonStyle(.plain)
            .font(.caption)
            .foregroundStyle(.secondary)
        }
        .padding(20)
        .frame(width: 280)
    }
}
