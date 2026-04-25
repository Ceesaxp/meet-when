import XCTest
@testable import MeetWhenBar

final class NotificationServiceTests: XCTestCase {

    // MARK: - New pending detection

    func testNewPendingDetection_noNewBookings() {
        let previous = [makeBooking(id: "1"), makeBooking(id: "2")]
        let current = [makeBooking(id: "1"), makeBooking(id: "2")]

        let newOnes = detectNew(previous: previous, current: current)
        XCTAssertTrue(newOnes.isEmpty)
    }

    func testNewPendingDetection_oneNew() {
        let previous = [makeBooking(id: "1")]
        let current = [makeBooking(id: "1"), makeBooking(id: "2")]

        let newOnes = detectNew(previous: previous, current: current)
        XCTAssertEqual(newOnes.count, 1)
        XCTAssertEqual(newOnes[0].id, "2")
    }

    func testNewPendingDetection_allNew() {
        let previous: [APIBooking] = []
        let current = [makeBooking(id: "1"), makeBooking(id: "2")]

        let newOnes = detectNew(previous: previous, current: current)
        XCTAssertEqual(newOnes.count, 2)
    }

    func testNewPendingDetection_bookingRemoved() {
        let previous = [makeBooking(id: "1"), makeBooking(id: "2")]
        let current = [makeBooking(id: "1")]

        let newOnes = detectNew(previous: previous, current: current)
        XCTAssertTrue(newOnes.isEmpty)
    }

    // MARK: - Stale reminder detection

    func testStaleReminderIDs() {
        let reminderIDs = ["reminder-b1", "reminder-b2", "reminder-b3"]
        let currentBookingIDs: Set<String> = ["b1", "b3"]

        let stale = reminderIDs.filter { id in
            let bookingID = String(id.dropFirst("reminder-".count))
            return !currentBookingIDs.contains(bookingID)
        }

        XCTAssertEqual(stale, ["reminder-b2"])
    }

    // MARK: - Settings persistence

    func testPollingIntervalDefault() {
        // Clear any stored value
        UserDefaults.standard.removeObject(forKey: "pollingInterval")
        let vm = AppViewModel()
        XCTAssertEqual(vm.pollingInterval, 60)
    }

    func testPollingIntervalPersistence() {
        UserDefaults.standard.set(120.0, forKey: "pollingInterval")
        let vm = AppViewModel()
        XCTAssertEqual(vm.pollingInterval, 120)

        // Cleanup
        UserDefaults.standard.removeObject(forKey: "pollingInterval")
    }

    func testNotificationsEnabledDefault() {
        UserDefaults.standard.removeObject(forKey: "notificationsEnabled")
        let vm = AppViewModel()
        XCTAssertTrue(vm.notificationsEnabled)
    }

    func testNotificationsEnabledPersistence() {
        UserDefaults.standard.set(false, forKey: "notificationsEnabled")
        let vm = AppViewModel()
        XCTAssertFalse(vm.notificationsEnabled)

        // Cleanup
        UserDefaults.standard.removeObject(forKey: "notificationsEnabled")
    }

    func testShowingSettingsDefault() {
        let vm = AppViewModel()
        XCTAssertFalse(vm.showingSettings)
    }

    // MARK: - Helpers

    /// Replicates the detection logic from NotificationService.notifyNewPendingBookings
    private func detectNew(previous: [APIBooking], current: [APIBooking]) -> [APIBooking] {
        let previousIDs = Set(previous.map(\.id))
        return current.filter { !previousIDs.contains($0.id) }
    }

    private func makeBooking(id: String) -> APIBooking {
        APIBooking(
            id: id, templateName: "Chat", status: .pending,
            startTime: Date().addingTimeInterval(3600),
            endTime: Date().addingTimeInterval(5400),
            duration: 30, inviteeName: "Test",
            inviteeEmail: "test@example.com", inviteeTimezone: "UTC",
            conferenceLink: "", locationType: "google_meet", createdAt: Date()
        )
    }
}
