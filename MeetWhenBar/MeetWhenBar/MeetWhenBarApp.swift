import SwiftUI

@main
struct MeetWhenBarApp: App {
    @State private var viewModel = AppViewModel()

    var body: some Scene {
        MenuBarExtra {
            menuContent
                .task {
                    await viewModel.restoreSession()
                }
                .onOpenURL { url in
                    Task {
                        await viewModel.handleOAuthCallback(url)
                    }
                }
        } label: {
            menuBarLabel
        }
        .menuBarExtraStyle(.window)
    }

    // MARK: - Menu bar icon

    private var menuBarLabel: some View {
        HStack(spacing: 2) {
            Image(systemName: viewModel.pendingCount > 0
                  ? "calendar.badge.exclamationmark"
                  : "calendar.badge.clock")

            if viewModel.pendingCount > 0 {
                Text("\(viewModel.pendingCount)")
                    .font(.caption2)
                    .monospacedDigit()
            }
        }
        .padding(.horizontal, 4)
    }

    // MARK: - Menu content

    @ViewBuilder
    private var menuContent: some View {
        switch viewModel.authState {
        case .loggedOut, .loggingIn:
            LoginView(viewModel: viewModel)
        case .orgSelection(let orgs):
            OrgSelectionView(viewModel: viewModel, orgs: orgs)
        case .authenticated:
            MainMenuView(viewModel: viewModel)
        }
    }
}
