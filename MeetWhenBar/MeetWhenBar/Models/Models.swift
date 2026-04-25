import Foundation

// MARK: - Auth

struct LoginRequest: Encodable {
    let email: String
    let password: String
}

struct LoginResponse: Codable {
    let token: String?
    let requiresOrgSelection: Bool
    let orgs: [OrgOption]?
    let host: APIHost?
    let tenant: APITenant?

    enum CodingKeys: String, CodingKey {
        case token
        case requiresOrgSelection = "requires_org_selection"
        case orgs
        case host
        case tenant
    }
}

struct OrgOption: Codable, Identifiable {
    let tenantID: String
    let tenantSlug: String
    let tenantName: String
    let hostID: String
    let selectionToken: String

    var id: String { hostID }

    enum CodingKeys: String, CodingKey {
        case tenantID = "tenant_id"
        case tenantSlug = "tenant_slug"
        case tenantName = "tenant_name"
        case hostID = "host_id"
        case selectionToken = "selection_token"
    }
}

struct SelectOrgRequest: Encodable {
    let hostID: String
    let selectionToken: String

    enum CodingKeys: String, CodingKey {
        case hostID = "host_id"
        case selectionToken = "selection_token"
    }
}

// MARK: - Host & Tenant

struct APIHost: Codable {
    let id: String
    let name: String
    let email: String
    let slug: String
    let timezone: String
    let smartDurations: Bool
    let isAdmin: Bool

    enum CodingKeys: String, CodingKey {
        case id, name, email, slug, timezone
        case smartDurations = "smart_durations"
        case isAdmin = "is_admin"
    }
}

struct APITenant: Codable {
    let id: String
    let name: String
    let slug: String
}

struct MeResponse: Codable {
    let host: APIHost
    let tenant: APITenant
}

// MARK: - Bookings

struct APIBooking: Codable, Identifiable {
    let id: String
    let templateName: String
    let status: BookingStatus
    let startTime: Date
    let endTime: Date
    let duration: Int
    let inviteeName: String
    let inviteeEmail: String
    let inviteeTimezone: String
    let conferenceLink: String
    let locationType: String
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case templateName = "template_name"
        case status
        case startTime = "start_time"
        case endTime = "end_time"
        case duration
        case inviteeName = "invitee_name"
        case inviteeEmail = "invitee_email"
        case inviteeTimezone = "invitee_timezone"
        case conferenceLink = "conference_link"
        case locationType = "location_type"
        case createdAt = "created_at"
    }

    /// Whether this booking has a joinable conference link.
    var hasConferenceLink: Bool {
        !conferenceLink.isEmpty && conferenceLink.hasPrefix("http")
    }

    /// Human-readable label for the conference provider.
    var conferenceLabel: String {
        switch locationType {
        case "google_meet": return "Join Meet"
        case "zoom": return "Join Zoom"
        case "phone": return "Call"
        default: return "Join"
        }
    }
}

enum BookingStatus: String, Codable {
    case pending
    case confirmed
    case cancelled
    case rejected
}

struct BookingsResponse: Codable {
    let bookings: [APIBooking]
}

// MARK: - Generic

struct StatusResponse: Codable {
    let status: String
}

struct ErrorResponse: Codable {
    let error: String
}
