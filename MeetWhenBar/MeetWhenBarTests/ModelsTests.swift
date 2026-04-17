import XCTest
@testable import MeetWhenBar

final class ModelsTests: XCTestCase {

    private let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .iso8601
        return d
    }()

    // MARK: - LoginResponse

    func testDecodeLoginResponse_singleOrg() throws {
        let json = """
        {
            "token": "abc123",
            "requires_org_selection": false,
            "host": {
                "id": "h1",
                "name": "Jane",
                "email": "jane@example.com",
                "slug": "jane",
                "timezone": "America/New_York",
                "smart_durations": true,
                "is_admin": false
            },
            "tenant": {
                "id": "t1",
                "name": "Acme",
                "slug": "acme"
            }
        }
        """.data(using: .utf8)!

        let resp = try decoder.decode(LoginResponse.self, from: json)
        XCTAssertEqual(resp.token, "abc123")
        XCTAssertFalse(resp.requiresOrgSelection)
        XCTAssertEqual(resp.host?.name, "Jane")
        XCTAssertEqual(resp.tenant?.slug, "acme")
        XCTAssertNil(resp.orgs)
    }

    func testDecodeLoginResponse_multiOrg() throws {
        let json = """
        {
            "token": null,
            "requires_org_selection": true,
            "orgs": [
                {
                    "tenant_id": "t1",
                    "tenant_slug": "acme",
                    "tenant_name": "Acme Corp",
                    "host_id": "h1",
                    "selection_token": "tok1"
                },
                {
                    "tenant_id": "t2",
                    "tenant_slug": "beta",
                    "tenant_name": "Beta Inc",
                    "host_id": "h2",
                    "selection_token": "tok2"
                }
            ]
        }
        """.data(using: .utf8)!

        let resp = try decoder.decode(LoginResponse.self, from: json)
        XCTAssertTrue(resp.requiresOrgSelection)
        XCTAssertNil(resp.token)
        XCTAssertEqual(resp.orgs?.count, 2)
        XCTAssertEqual(resp.orgs?[0].tenantName, "Acme Corp")
        XCTAssertEqual(resp.orgs?[1].hostID, "h2")
    }

    // MARK: - APIBooking

    func testDecodeBooking() throws {
        let json = """
        {
            "id": "b1",
            "template_name": "Quick Chat",
            "status": "confirmed",
            "start_time": "2026-03-30T14:00:00Z",
            "end_time": "2026-03-30T14:30:00Z",
            "duration": 30,
            "invitee_name": "Bob",
            "invitee_email": "bob@example.com",
            "invitee_timezone": "Europe/London",
            "conference_link": "https://meet.google.com/abc",
            "location_type": "google_meet",
            "created_at": "2026-03-29T10:00:00Z"
        }
        """.data(using: .utf8)!

        let booking = try decoder.decode(APIBooking.self, from: json)
        XCTAssertEqual(booking.id, "b1")
        XCTAssertEqual(booking.status, .confirmed)
        XCTAssertEqual(booking.duration, 30)
        XCTAssertEqual(booking.inviteeName, "Bob")
        XCTAssertTrue(booking.hasConferenceLink)
        XCTAssertEqual(booking.conferenceLabel, "Join Meet")
    }

    func testBookingConferenceHelpers() {
        let noLink = APIBooking(
            id: "1", templateName: "Test", status: .confirmed,
            startTime: Date(), endTime: Date(), duration: 30,
            inviteeName: "A", inviteeEmail: "a@b.com", inviteeTimezone: "UTC",
            conferenceLink: "", locationType: "google_meet", createdAt: Date()
        )
        XCTAssertFalse(noLink.hasConferenceLink)

        let zoom = APIBooking(
            id: "2", templateName: "Test", status: .confirmed,
            startTime: Date(), endTime: Date(), duration: 30,
            inviteeName: "A", inviteeEmail: "a@b.com", inviteeTimezone: "UTC",
            conferenceLink: "https://zoom.us/j/123", locationType: "zoom", createdAt: Date()
        )
        XCTAssertTrue(zoom.hasConferenceLink)
        XCTAssertEqual(zoom.conferenceLabel, "Join Zoom")
    }

    // MARK: - BookingStatus

    func testBookingStatusDecoding() throws {
        let statuses = ["\"pending\"", "\"confirmed\"", "\"cancelled\"", "\"rejected\""]
        let expected: [BookingStatus] = [.pending, .confirmed, .cancelled, .rejected]

        for (json, expected) in zip(statuses, expected) {
            let decoded = try decoder.decode(BookingStatus.self, from: json.data(using: .utf8)!)
            XCTAssertEqual(decoded, expected)
        }
    }

    // MARK: - BookingsResponse

    func testDecodeBookingsResponse_empty() throws {
        let json = """
        {"bookings": []}
        """.data(using: .utf8)!

        let resp = try decoder.decode(BookingsResponse.self, from: json)
        XCTAssertTrue(resp.bookings.isEmpty)
    }

    // MARK: - ErrorResponse

    func testDecodeErrorResponse() throws {
        let json = """
        {"error": "invalid email or password"}
        """.data(using: .utf8)!

        let resp = try decoder.decode(ErrorResponse.self, from: json)
        XCTAssertEqual(resp.error, "invalid email or password")
    }

    // MARK: - MeResponse

    func testDecodeMeResponse() throws {
        let json = """
        {
            "host": {
                "id": "h1", "name": "Jane", "email": "j@a.com",
                "slug": "jane", "timezone": "UTC",
                "smart_durations": false, "is_admin": true
            },
            "tenant": {"id": "t1", "name": "Acme", "slug": "acme"}
        }
        """.data(using: .utf8)!

        let resp = try decoder.decode(MeResponse.self, from: json)
        XCTAssertEqual(resp.host.id, "h1")
        XCTAssertTrue(resp.host.isAdmin)
        XCTAssertEqual(resp.tenant.name, "Acme")
    }
}
