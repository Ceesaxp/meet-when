import SwiftUI

struct LoginView: View {
    @Bindable var viewModel: AppViewModel

    @State private var serverURL = ""
    @State private var email = ""
    @State private var password = ""

    var body: some View {
        VStack(spacing: 16) {
            Text("Meet When")
                .font(.headline)

            VStack(alignment: .leading, spacing: 8) {
                Text("Server URL")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                TextField("https://meet.example.com", text: $serverURL)
                    .textFieldStyle(.roundedBorder)

                Text("Email")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                TextField("jane@acme.com", text: $email)
                    .textFieldStyle(.roundedBorder)
                    .textContentType(.emailAddress)

                Text("Password")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                SecureField("Password", text: $password)
                    .textFieldStyle(.roundedBorder)
            }

            if let error = viewModel.errorMessage {
                Text(error)
                    .font(.caption)
                    .foregroundStyle(.red)
                    .multilineTextAlignment(.center)
            }

            Button(action: {
                Task {
                    await viewModel.login(serverURL: serverURL, email: email, password: password)
                }
            }) {
                if viewModel.authState == .loggingIn {
                    ProgressView()
                        .controlSize(.small)
                        .frame(maxWidth: .infinity)
                } else {
                    Text("Sign In")
                        .frame(maxWidth: .infinity)
                }
            }
            .buttonStyle(.borderedProminent)
            .disabled(serverURL.isEmpty || email.isEmpty || password.isEmpty || viewModel.authState == .loggingIn)

            dividerWithLabel("or")

            Button(action: {
                viewModel.loginWithGoogle(serverURL: serverURL)
            }) {
                HStack(spacing: 6) {
                    Image(systemName: "globe")
                        .font(.caption)
                    Text("Sign in with Google")
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
            .disabled(serverURL.isEmpty)

            Divider()

            Button("Quit Meet When") {
                NSApplication.shared.terminate(nil)
            }
            .buttonStyle(.plain)
            .font(.caption)
            .foregroundStyle(.secondary)
        }
        .padding(20)
        .frame(width: 280)
        .onAppear {
            // Restore saved server URL if available
            if let saved = KeychainService.load(.serverURL) {
                serverURL = saved
            }
        }
    }

    private func dividerWithLabel(_ label: String) -> some View {
        HStack {
            Rectangle()
                .fill(.quaternary)
                .frame(height: 1)
            Text(label)
                .font(.caption2)
                .foregroundStyle(.tertiary)
            Rectangle()
                .fill(.quaternary)
                .frame(height: 1)
        }
    }
}
