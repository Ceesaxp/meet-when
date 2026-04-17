import Foundation
import SwiftUI

/// Root application state managing auth, polling, and data.
@Observable
final class AppViewModel {

    // MARK: - Auth state

    enum AuthState: Equatable {
        case loggedOut
        case loggingIn
        case orgSelection([OrgOption])
        case authenticated

        static func == (lhs: AuthState, rhs: AuthState) -> Bool {
            switch (lhs, rhs) {
            case (.loggedOut, .loggedOut),
                 (.loggingIn, .loggingIn),
                 (.authenticated, .authenticated):
                return true
            case (.orgSelection(let a), .orgSelection(let b)):
                return a.map(\.hostID) == b.map(\.hostID)
            default:
                return false
            }
        }
    }

    var authState: AuthState = .loggedOut
    var host: APIHost?
    var tenant: APITenant?
    var errorMessage: String?

    // MARK: - Booking data

    var todayBookings: [APIBooking] = []
    var pendingBookings: [APIBooking] = []
    var isLoading = false

    /// Badge count shown on menu bar icon.
    var pendingCount: Int { pendingBookings.count }

    // MARK: - Settings

    var showingSettings = false
    var notificationsEnabled: Bool = UserDefaults.standard.object(forKey: "notificationsEnabled") as? Bool ?? true

    // MARK: - Polling

    var pollingInterval: TimeInterval = {
        let saved = UserDefaults.standard.double(forKey: "pollingInterval")
        return saved > 0 ? saved : 60
    }()
    private var pollingTask: Task<Void, Never>?

    // MARK: - API client

    private(set) var apiClient: APIClient?
    var serverURL: String = ""

    // MARK: - Lifecycle

    /// Attempts to restore a previous session from Keychain on launch.
    func restoreSession() async {
        guard let savedURL = KeychainService.load(.serverURL),
              let savedToken = KeychainService.load(.sessionToken),
              let url = APIClient.validateServerURL(savedURL) else {
            authState = .loggedOut
            return
        }

        serverURL = savedURL
        let client = APIClient(baseURL: url, token: savedToken)
        apiClient = client

        do {
            let me = try await client.me()
            host = me.host
            tenant = me.tenant
            authState = .authenticated
            startPolling()
        } catch {
            // Token expired or server unreachable — require re-login
            KeychainService.delete(.sessionToken)
            authState = .loggedOut
        }
    }

    // MARK: - Auth actions

    func login(serverURL: String, email: String, password: String) async {
        errorMessage = nil
        authState = .loggingIn

        guard let url = APIClient.validateServerURL(serverURL) else {
            errorMessage = "Invalid URL. Use https:// (or http://localhost for dev)."
            authState = .loggedOut
            return
        }

        self.serverURL = serverURL
        let client = APIClient(baseURL: url)
        apiClient = client

        do {
            let response = try await client.login(email: email, password: password)

            if response.requiresOrgSelection, let orgs = response.orgs {
                authState = .orgSelection(orgs)
            } else if let token = response.token {
                try KeychainService.save(serverURL, for: .serverURL)
                try KeychainService.save(token, for: .sessionToken)
                await client.setToken(token)
                host = response.host
                tenant = response.tenant
                authState = .authenticated
                startPolling()
            } else {
                errorMessage = "Unexpected server response."
                authState = .loggedOut
            }
        } catch let error as APIError {
            errorMessage = error.errorDescription
            authState = .loggedOut
        } catch {
            errorMessage = "Connection failed. Check the server URL."
            authState = .loggedOut
        }
    }

    func selectOrg(_ org: OrgOption) async {
        errorMessage = nil
        guard let client = apiClient else { return }

        do {
            let response = try await client.selectOrg(hostID: org.hostID, selectionToken: org.selectionToken)

            guard let token = response.token else {
                errorMessage = "Failed to complete org selection."
                authState = .loggedOut
                return
            }

            try KeychainService.save(serverURL, for: .serverURL)
            try KeychainService.save(token, for: .sessionToken)
            await client.setToken(token)
            host = response.host
            tenant = response.tenant
            authState = .authenticated
            startPolling()
        } catch let error as APIError {
            errorMessage = error.errorDescription
            authState = .loggedOut
        } catch {
            errorMessage = "Connection failed."
            authState = .loggedOut
        }
    }

    func logout() async {
        pollingTask?.cancel()
        pollingTask = nil

        if let client = apiClient {
            try? await client.logout()
        }

        KeychainService.clearAll()
        apiClient = nil
        host = nil
        tenant = nil
        todayBookings = []
        pendingBookings = []
        authState = .loggedOut
    }

    // MARK: - Booking actions

    func refreshBookings() async {
        guard let client = apiClient else { return }
        isLoading = true
        defer { isLoading = false }

        do {
            async let today = client.todayBookings()
            async let pending = client.pendingBookings()
            let (t, p) = try await (today, pending)

            let previousPending = pendingBookings

            todayBookings = t.sorted { $0.startTime < $1.startTime }
            pendingBookings = p.sorted { $0.startTime < $1.startTime }

            // Notify about new pending bookings
            if notificationsEnabled {
                NotificationService.shared.notifyNewPendingBookings(previous: previousPending, current: pendingBookings)
                NotificationService.shared.scheduleMeetingReminders(bookings: todayBookings)
                NotificationService.shared.cancelStaleReminders(
                    currentBookingIDs: Set(todayBookings.map(\.id))
                )
            }
        } catch is CancellationError {
            // Polling was cancelled — ignore
        } catch let error as APIError where error == .unauthorized {
            await handleUnauthorized()
        } catch {
            // Silently fail on polling errors — will retry next cycle
        }
    }

    func approveBooking(_ id: String) async {
        guard let client = apiClient else { return }
        do {
            _ = try await client.approveBooking(id: id)
            await refreshBookings()
        } catch let error as APIError where error == .unauthorized {
            await handleUnauthorized()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func rejectBooking(_ id: String, reason: String = "") async {
        guard let client = apiClient else { return }
        do {
            try await client.rejectBooking(id: id, reason: reason)
            await refreshBookings()
        } catch let error as APIError where error == .unauthorized {
            await handleUnauthorized()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    func cancelBooking(_ id: String, reason: String = "") async {
        guard let client = apiClient else { return }
        do {
            try await client.cancelBooking(id: id, reason: reason)
            await refreshBookings()
        } catch let error as APIError where error == .unauthorized {
            await handleUnauthorized()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    // MARK: - Google OAuth

    /// Opens the system browser to start Google OAuth via the server.
    func loginWithGoogle(serverURL: String) {
        errorMessage = nil

        guard let baseURL = APIClient.validateServerURL(serverURL) else {
            errorMessage = "Invalid URL. Use https:// (or http://localhost for dev)."
            return
        }

        self.serverURL = serverURL
        let googleURL = baseURL.appendingPathComponent("/api/v1/auth/google")
        NSWorkspace.shared.open(googleURL)
    }

    /// Handles the meetwhenbar://auth/callback URL scheme callback.
    func handleOAuthCallback(_ url: URL) async {
        guard let components = URLComponents(url: url, resolvingAgainstBaseURL: false),
              components.scheme == "meetwhenbar",
              components.host == "auth" else {
            return
        }

        let params = Dictionary(
            uniqueKeysWithValues: (components.queryItems ?? []).compactMap { item in
                item.value.map { (item.name, $0) }
            }
        )

        // Check for error
        if let error = params["error"] {
            errorMessage = error
            authState = .loggedOut
            return
        }

        // Extract token and server URL
        guard let token = params["token"], !token.isEmpty else {
            errorMessage = "Authentication failed. No token received."
            authState = .loggedOut
            return
        }

        let callbackServerURL = params["server"] ?? serverURL

        guard let baseURL = APIClient.validateServerURL(callbackServerURL) else {
            errorMessage = "Invalid server URL in callback."
            authState = .loggedOut
            return
        }

        // Save credentials and establish session
        do {
            self.serverURL = callbackServerURL
            try KeychainService.save(callbackServerURL, for: .serverURL)
            try KeychainService.save(token, for: .sessionToken)

            let client = APIClient(baseURL: baseURL, token: token)
            apiClient = client

            let me = try await client.me()
            host = me.host
            tenant = me.tenant
            authState = .authenticated
            startPolling()
        } catch {
            errorMessage = "Failed to verify session: \(error.localizedDescription)"
            authState = .loggedOut
        }
    }

    // MARK: - Dashboard

    func openDashboard() {
        guard let url = URL(string: serverURL)?.appendingPathComponent("/dashboard") else { return }
        NSWorkspace.shared.open(url)
    }

    func openConferenceLink(_ urlString: String) {
        guard let url = URL(string: urlString) else { return }
        NSWorkspace.shared.open(url)
    }

    // MARK: - Private

    private func startPolling() {
        // Request notification permissions on first authenticated session
        if notificationsEnabled {
            NotificationService.shared.requestPermission()
            NotificationService.shared.onApproveBooking = { [weak self] bookingID in
                Task { await self?.approveBooking(bookingID) }
            }
        }

        pollingTask?.cancel()
        pollingTask = Task { [weak self] in
            guard let self else { return }
            // Initial fetch
            await self.refreshBookings()
            // Periodic refresh
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(self.pollingInterval))
                guard !Task.isCancelled else { break }
                await self.refreshBookings()
            }
        }
    }

    private func handleUnauthorized() async {
        KeychainService.delete(.sessionToken)
        authState = .loggedOut
        host = nil
        tenant = nil
        todayBookings = []
        pendingBookings = []
        pollingTask?.cancel()
        pollingTask = nil
    }
}

// MARK: - APIError Equatable (for pattern matching)

extension APIError: Equatable {
    static func == (lhs: APIError, rhs: APIError) -> Bool {
        switch (lhs, rhs) {
        case (.unauthorized, .unauthorized): return true
        case (.invalidURL, .invalidURL): return true
        case (.invalidResponse, .invalidResponse): return true
        case (.server(let a), .server(let b)): return a == b
        default: return false
        }
    }
}
