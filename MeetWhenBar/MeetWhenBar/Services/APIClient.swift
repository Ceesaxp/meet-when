import Foundation

/// HTTP client for the Meet-When API v1 (JSON over Bearer token auth).
actor APIClient {
    private let session: URLSession
    private var baseURL: URL
    private var token: String?

    private static let decoder: JSONDecoder = {
        let d = JSONDecoder()
        d.dateDecodingStrategy = .iso8601
        return d
    }()

    private static let encoder: JSONEncoder = {
        let e = JSONEncoder()
        e.dateEncodingStrategy = .iso8601
        return e
    }()

    // MARK: - Init

    init(baseURL: URL, token: String? = nil, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.token = token
        self.session = session
    }

    // MARK: - Configuration

    func setToken(_ token: String?) {
        self.token = token
    }

    func setBaseURL(_ url: URL) {
        self.baseURL = url
    }

    // MARK: - Auth endpoints

    /// Login with email and password. Does not require a token.
    func login(email: String, password: String) async throws -> LoginResponse {
        let body = LoginRequest(email: email, password: password)
        return try await post("/api/v1/auth/login", body: body, authenticated: false)
    }

    /// Complete org selection after multi-org login.
    func selectOrg(hostID: String, selectionToken: String) async throws -> LoginResponse {
        let body = SelectOrgRequest(hostID: hostID, selectionToken: selectionToken)
        return try await post("/api/v1/auth/login/select-org", body: body, authenticated: false)
    }

    /// Logout and invalidate the current session.
    func logout() async throws {
        let _: StatusResponse = try await post("/api/v1/auth/logout", body: Empty?.none)
        self.token = nil
    }

    /// Get the current authenticated user's profile.
    func me() async throws -> MeResponse {
        try await get("/api/v1/me")
    }

    // MARK: - Booking endpoints

    func todayBookings() async throws -> [APIBooking] {
        let resp: BookingsResponse = try await get("/api/v1/bookings/today")
        return resp.bookings
    }

    func pendingBookings() async throws -> [APIBooking] {
        let resp: BookingsResponse = try await get("/api/v1/bookings/pending")
        return resp.bookings
    }

    func listBookings(status: String? = nil, includeArchived: Bool = false) async throws -> [APIBooking] {
        var query = [URLQueryItem]()
        if let status { query.append(.init(name: "status", value: status)) }
        if includeArchived { query.append(.init(name: "include_archived", value: "true")) }

        let resp: BookingsResponse = try await get("/api/v1/bookings", query: query)
        return resp.bookings
    }

    func getBooking(id: String) async throws -> APIBooking {
        let resp: SingleBookingResponse = try await get("/api/v1/bookings/\(id)")
        return resp.booking
    }

    func approveBooking(id: String) async throws -> APIBooking {
        let resp: SingleBookingResponse = try await post("/api/v1/bookings/\(id)/approve", body: Empty?.none)
        return resp.booking
    }

    func rejectBooking(id: String, reason: String = "") async throws {
        let body = ["reason": reason]
        let _: StatusResponse = try await post("/api/v1/bookings/\(id)/reject", body: body)
    }

    func cancelBooking(id: String, reason: String = "") async throws {
        let body = ["reason": reason]
        let _: StatusResponse = try await post("/api/v1/bookings/\(id)/cancel", body: body)
    }

    // MARK: - HTTP primitives

    private func get<T: Decodable>(_ path: String, query: [URLQueryItem] = []) async throws -> T {
        var components = URLComponents(url: baseURL.appendingPathComponent(path), resolvingAgainstBaseURL: false)!
        if !query.isEmpty { components.queryItems = query }
        guard let url = components.url else { throw APIError.invalidURL }

        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        applyAuth(&request)

        return try await execute(request)
    }

    private func post<T: Decodable, B: Encodable>(_ path: String, body: B?, authenticated: Bool = true) async throws -> T {
        let url = baseURL.appendingPathComponent(path)
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")

        if let body {
            request.httpBody = try Self.encoder.encode(body)
        }

        if authenticated { applyAuth(&request) }

        return try await execute(request)
    }

    private func applyAuth(_ request: inout URLRequest) {
        if let token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
    }

    private func execute<T: Decodable>(_ request: URLRequest) async throws -> T {
        let (data, response) = try await session.data(for: request)

        guard let http = response as? HTTPURLResponse else {
            throw APIError.invalidResponse
        }

        switch http.statusCode {
        case 200..<300:
            return try Self.decoder.decode(T.self, from: data)
        case 401:
            throw APIError.unauthorized
        case 400..<500:
            if let err = try? Self.decoder.decode(ErrorResponse.self, from: data) {
                throw APIError.server(err.error)
            }
            throw APIError.server("Request failed (\(http.statusCode))")
        default:
            throw APIError.server("Server error (\(http.statusCode))")
        }
    }

    // MARK: - URL validation

    /// Validates that the URL is HTTPS (or localhost for development).
    static func validateServerURL(_ urlString: String) -> URL? {
        guard let url = URL(string: urlString),
              let scheme = url.scheme?.lowercased(),
              let host = url.host?.lowercased() else {
            return nil
        }

        // Allow http only for localhost / 127.0.0.1
        if scheme == "http" {
            guard host == "localhost" || host == "127.0.0.1" else {
                return nil
            }
        } else if scheme != "https" {
            return nil
        }

        return url
    }
}

// MARK: - Supporting types

enum APIError: LocalizedError {
    case invalidURL
    case invalidResponse
    case unauthorized
    case server(String)

    var errorDescription: String? {
        switch self {
        case .invalidURL: return "Invalid URL"
        case .invalidResponse: return "Invalid server response"
        case .unauthorized: return "Session expired"
        case .server(let msg): return msg
        }
    }
}

/// Empty body placeholder for POST requests with no body.
private struct Empty: Encodable {}

private struct SingleBookingResponse: Codable {
    let booking: APIBooking
}
