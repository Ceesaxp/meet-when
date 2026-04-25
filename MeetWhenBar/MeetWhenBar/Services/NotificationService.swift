import AppKit
import Foundation
import UserNotifications

/// Manages macOS notifications for booking events.
final class NotificationService: NSObject, UNUserNotificationCenterDelegate {

    static let shared = NotificationService()

    /// Category identifiers for actionable notifications.
    enum Category: String {
        case pendingBooking = "PENDING_BOOKING"
        case meetingReminder = "MEETING_REMINDER"
    }

    /// Action identifiers.
    enum Action: String {
        case approve = "APPROVE_ACTION"
        case joinMeeting = "JOIN_MEETING"
    }

    /// Callback for notification actions.
    var onApproveBooking: ((String) -> Void)?
    var onJoinMeeting: ((String) -> Void)?

    private override init() {
        super.init()
    }

    // MARK: - Setup

    func requestPermission() {
        let center = UNUserNotificationCenter.current()
        center.delegate = self

        center.requestAuthorization(options: [.alert, .sound, .badge]) { granted, error in
            if let error {
                print("Notification permission error: \(error)")
            }
        }

        // Register action categories
        let approveAction = UNNotificationAction(
            identifier: Action.approve.rawValue,
            title: "Approve",
            options: []
        )
        let pendingCategory = UNNotificationCategory(
            identifier: Category.pendingBooking.rawValue,
            actions: [approveAction],
            intentIdentifiers: []
        )

        let joinAction = UNNotificationAction(
            identifier: Action.joinMeeting.rawValue,
            title: "Join",
            options: [.foreground]
        )
        let reminderCategory = UNNotificationCategory(
            identifier: Category.meetingReminder.rawValue,
            actions: [joinAction],
            intentIdentifiers: []
        )

        center.setNotificationCategories([pendingCategory, reminderCategory])
    }

    // MARK: - Pending booking notifications

    /// Sends a notification for each newly appeared pending booking.
    func notifyNewPendingBookings(previous: [APIBooking], current: [APIBooking]) {
        let previousIDs = Set(previous.map(\.id))
        let newBookings = current.filter { !previousIDs.contains($0.id) }

        for booking in newBookings {
            let content = UNMutableNotificationContent()
            content.title = "New Booking Request"
            content.body = "\(booking.inviteeName) wants to book \(booking.templateName)"
            content.sound = .default
            content.categoryIdentifier = Category.pendingBooking.rawValue
            content.userInfo = ["bookingID": booking.id]

            let request = UNNotificationRequest(
                identifier: "pending-\(booking.id)",
                content: content,
                trigger: nil // fire immediately
            )

            UNUserNotificationCenter.current().add(request)
        }
    }

    // MARK: - Meeting reminder notifications

    /// Schedules a reminder notification 5 minutes before each upcoming booking.
    /// Deduplicates — won't reschedule if already pending.
    func scheduleMeetingReminders(bookings: [APIBooking]) {
        let center = UNUserNotificationCenter.current()

        center.getPendingNotificationRequests { existing in
            let existingIDs = Set(existing.map(\.identifier))

            for booking in bookings {
                let reminderID = "reminder-\(booking.id)"
                guard !existingIDs.contains(reminderID) else { continue }

                let fireDate = booking.startTime.addingTimeInterval(-5 * 60)
                guard fireDate > Date() else { continue }

                let content = UNMutableNotificationContent()
                content.title = "Meeting in 5 minutes"
                content.body = "\(booking.templateName) with \(booking.inviteeName)"
                content.sound = .default
                content.categoryIdentifier = Category.meetingReminder.rawValue
                content.userInfo = [
                    "bookingID": booking.id,
                    "conferenceLink": booking.conferenceLink,
                ]

                let interval = fireDate.timeIntervalSinceNow
                guard interval > 0 else { continue }
                let trigger = UNTimeIntervalNotificationTrigger(timeInterval: interval, repeats: false)

                let request = UNNotificationRequest(
                    identifier: reminderID,
                    content: content,
                    trigger: trigger
                )

                center.add(request)
            }
        }
    }

    /// Removes scheduled reminders for bookings that no longer exist (cancelled, etc.).
    func cancelStaleReminders(currentBookingIDs: Set<String>) {
        let center = UNUserNotificationCenter.current()
        center.getPendingNotificationRequests { existing in
            let stale = existing
                .filter { $0.identifier.hasPrefix("reminder-") }
                .filter { req in
                    let bookingID = String(req.identifier.dropFirst("reminder-".count))
                    return !currentBookingIDs.contains(bookingID)
                }
                .map(\.identifier)

            if !stale.isEmpty {
                center.removePendingNotificationRequests(withIdentifiers: stale)
            }
        }
    }

    // MARK: - UNUserNotificationCenterDelegate

    /// Show notifications even when app is in foreground.
    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        [.banner, .sound]
    }

    /// Handle notification actions.
    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse
    ) async {
        let userInfo = response.notification.request.content.userInfo

        switch response.actionIdentifier {
        case Action.approve.rawValue:
            if let bookingID = userInfo["bookingID"] as? String {
                onApproveBooking?(bookingID)
            }

        case Action.joinMeeting.rawValue:
            if let link = userInfo["conferenceLink"] as? String,
               let url = URL(string: link), !link.isEmpty {
                await MainActor.run {
                    NSWorkspace.shared.open(url)
                }
            }

        case UNNotificationDefaultActionIdentifier:
            // User tapped the notification body — no special action
            break

        default:
            break
        }
    }
}
