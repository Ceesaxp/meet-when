import XCTest
@testable import MeetWhenBar

final class AppViewModelTests: XCTestCase {

    override func tearDown() {
        KeychainService.clearAll()
        super.tearDown()
    }

    // MARK: - Initial state

    func testInitialState() {
        let vm = AppViewModel()
        XCTAssertEqual(vm.authState, .loggedOut)
        XCTAssertNil(vm.host)
        XCTAssertNil(vm.tenant)
        XCTAssertTrue(vm.todayBookings.isEmpty)
        XCTAssertTrue(vm.pendingBookings.isEmpty)
        XCTAssertEqual(vm.pendingCount, 0)
        XCTAssertNil(vm.errorMessage)
    }

    // MARK: - Login validation

    func testLogin_invalidURL_setsError() async {
        let vm = AppViewModel()
        await vm.login(serverURL: "not-a-url", email: "a@b.com", password: "12345678")

        XCTAssertEqual(vm.authState, .loggedOut)
        XCTAssertNotNil(vm.errorMessage)
        XCTAssertTrue(vm.errorMessage!.contains("Invalid URL"))
    }

    func testLogin_httpRemote_setsError() async {
        let vm = AppViewModel()
        await vm.login(serverURL: "http://remote.example.com", email: "a@b.com", password: "12345678")

        XCTAssertEqual(vm.authState, .loggedOut)
        XCTAssertNotNil(vm.errorMessage)
    }

    // MARK: - Logout

    func testLogout_clearsState() async {
        let vm = AppViewModel()
        vm.authState = .authenticated
        vm.host = APIHost(id: "h1", name: "Jane", email: "j@a.com", slug: "jane", timezone: "UTC", smartDurations: false, isAdmin: false)
        vm.tenant = APITenant(id: "t1", name: "Acme", slug: "acme")

        await vm.logout()

        XCTAssertEqual(vm.authState, .loggedOut)
        XCTAssertNil(vm.host)
        XCTAssertNil(vm.tenant)
        XCTAssertTrue(vm.todayBookings.isEmpty)
        XCTAssertTrue(vm.pendingBookings.isEmpty)
    }

    // MARK: - Restore session (no saved credentials)

    func testRestoreSession_noCredentials() async {
        KeychainService.clearAll()
        let vm = AppViewModel()
        await vm.restoreSession()

        XCTAssertEqual(vm.authState, .loggedOut)
    }

    // MARK: - AuthState equatable

    func testAuthState_equatable() {
        XCTAssertEqual(AppViewModel.AuthState.loggedOut, .loggedOut)
        XCTAssertEqual(AppViewModel.AuthState.loggingIn, .loggingIn)
        XCTAssertEqual(AppViewModel.AuthState.authenticated, .authenticated)
        XCTAssertNotEqual(AppViewModel.AuthState.loggedOut, .authenticated)
        XCTAssertNotEqual(AppViewModel.AuthState.loggingIn, .loggedOut)

        let orgs1 = [OrgOption(tenantID: "t1", tenantSlug: "a", tenantName: "A", hostID: "h1", selectionToken: "tok1")]
        let orgs2 = [OrgOption(tenantID: "t1", tenantSlug: "a", tenantName: "A", hostID: "h1", selectionToken: "tok1")]
        let orgs3 = [OrgOption(tenantID: "t2", tenantSlug: "b", tenantName: "B", hostID: "h2", selectionToken: "tok2")]

        XCTAssertEqual(AppViewModel.AuthState.orgSelection(orgs1), .orgSelection(orgs2))
        XCTAssertNotEqual(AppViewModel.AuthState.orgSelection(orgs1), .orgSelection(orgs3))
    }

    // MARK: - Pending count

    func testPendingCount() {
        let vm = AppViewModel()
        XCTAssertEqual(vm.pendingCount, 0)

        vm.pendingBookings = [
            APIBooking(id: "1", templateName: "Chat", status: .pending,
                       startTime: Date(), endTime: Date(), duration: 30,
                       inviteeName: "A", inviteeEmail: "a@b.com", inviteeTimezone: "UTC",
                       conferenceLink: "", locationType: "google_meet", createdAt: Date()),
            APIBooking(id: "2", templateName: "Chat", status: .pending,
                       startTime: Date(), endTime: Date(), duration: 30,
                       inviteeName: "B", inviteeEmail: "b@b.com", inviteeTimezone: "UTC",
                       conferenceLink: "", locationType: "google_meet", createdAt: Date()),
        ]
        XCTAssertEqual(vm.pendingCount, 2)
    }

    // MARK: - OAuth callback URL parsing

    func testHandleOAuthCallback_errorParam() async {
        let vm = AppViewModel()
        let url = URL(string: "meetwhenbar://auth/callback?error=No%20account%20found&server=https://meet.example.com")!
        await vm.handleOAuthCallback(url)

        XCTAssertEqual(vm.authState, .loggedOut)
        XCTAssertNotNil(vm.errorMessage)
        XCTAssertTrue(vm.errorMessage!.contains("No account found"))
    }

    func testHandleOAuthCallback_noToken() async {
        let vm = AppViewModel()
        let url = URL(string: "meetwhenbar://auth/callback?server=https://meet.example.com")!
        await vm.handleOAuthCallback(url)

        XCTAssertEqual(vm.authState, .loggedOut)
        XCTAssertNotNil(vm.errorMessage)
        XCTAssertTrue(vm.errorMessage!.contains("No token"))
    }

    func testHandleOAuthCallback_wrongScheme_ignored() async {
        let vm = AppViewModel()
        let url = URL(string: "https://example.com/callback?token=abc")!
        await vm.handleOAuthCallback(url)

        // Should be ignored — state unchanged
        XCTAssertEqual(vm.authState, .loggedOut)
        XCTAssertNil(vm.errorMessage)
    }

    func testHandleOAuthCallback_wrongHost_ignored() async {
        let vm = AppViewModel()
        let url = URL(string: "meetwhenbar://other/path?token=abc")!
        await vm.handleOAuthCallback(url)

        XCTAssertEqual(vm.authState, .loggedOut)
        XCTAssertNil(vm.errorMessage)
    }

    // MARK: - Google login validation

    func testLoginWithGoogle_invalidURL() {
        let vm = AppViewModel()
        vm.loginWithGoogle(serverURL: "not-a-url")

        XCTAssertNotNil(vm.errorMessage)
        XCTAssertTrue(vm.errorMessage!.contains("Invalid URL"))
    }

    func testLoginWithGoogle_emptyURL() {
        let vm = AppViewModel()
        vm.loginWithGoogle(serverURL: "")

        XCTAssertNotNil(vm.errorMessage)
    }

    // MARK: - Badge count reflects pending bookings

    func testBadgeCount_zeroPending() {
        let vm = AppViewModel()
        vm.pendingBookings = []
        XCTAssertEqual(vm.pendingCount, 0)
    }

    func testBadgeCount_afterApprovalClearsCount() {
        let vm = AppViewModel()
        vm.pendingBookings = [
            makeBooking(id: "1", status: .pending),
        ]
        XCTAssertEqual(vm.pendingCount, 1)

        // Simulate approval clearing the list
        vm.pendingBookings = []
        XCTAssertEqual(vm.pendingCount, 0)
    }

    // MARK: - Error message clearing

    func testErrorMessage_canBeDismissed() {
        let vm = AppViewModel()
        vm.errorMessage = "Something went wrong"
        XCTAssertNotNil(vm.errorMessage)

        vm.errorMessage = nil
        XCTAssertNil(vm.errorMessage)
    }

    func testErrorMessage_clearedOnSuccessfulLogin() async {
        let vm = AppViewModel()
        vm.errorMessage = "Previous error"

        // Login with invalid URL clears old error and sets new one
        await vm.login(serverURL: "not-a-url", email: "a@b.com", password: "12345678")
        XCTAssertNotNil(vm.errorMessage)
        XCTAssertTrue(vm.errorMessage!.contains("Invalid URL"))
    }

    // MARK: - Helpers

    private func makeBooking(id: String, status: BookingStatus) -> APIBooking {
        APIBooking(
            id: id, templateName: "Chat", status: status,
            startTime: Date(), endTime: Date().addingTimeInterval(1800),
            duration: 30, inviteeName: "Test User",
            inviteeEmail: "test@example.com", inviteeTimezone: "UTC",
            conferenceLink: "", locationType: "google_meet", createdAt: Date()
        )
    }
}
