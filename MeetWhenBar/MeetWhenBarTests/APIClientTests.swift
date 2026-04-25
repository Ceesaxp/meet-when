import XCTest
@testable import MeetWhenBar

// MARK: - Mock URL Protocol

final class MockURLProtocol: URLProtocol {
    static var requestHandler: ((URLRequest) throws -> (HTTPURLResponse, Data))?

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        guard let handler = Self.requestHandler else {
            XCTFail("No request handler set")
            return
        }

        do {
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}

// MARK: - Tests

final class APIClientTests: XCTestCase {

    private var client: APIClient!
    private var session: URLSession!

    override func setUp() {
        super.setUp()
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [MockURLProtocol.self]
        session = URLSession(configuration: config)
        client = APIClient(
            baseURL: URL(string: "https://meet.example.com")!,
            token: "test-token",
            session: session
        )
    }

    override func tearDown() {
        MockURLProtocol.requestHandler = nil
        super.tearDown()
    }

    // MARK: - URL Validation

    func testValidateServerURL_https() {
        XCTAssertNotNil(APIClient.validateServerURL("https://meet.example.com"))
    }

    func testValidateServerURL_httpLocalhost() {
        XCTAssertNotNil(APIClient.validateServerURL("http://localhost:8080"))
        XCTAssertNotNil(APIClient.validateServerURL("http://127.0.0.1:8080"))
    }

    func testValidateServerURL_httpRemote_rejected() {
        XCTAssertNil(APIClient.validateServerURL("http://meet.example.com"))
    }

    func testValidateServerURL_invalid() {
        XCTAssertNil(APIClient.validateServerURL("not-a-url"))
        XCTAssertNil(APIClient.validateServerURL(""))
        XCTAssertNil(APIClient.validateServerURL("ftp://example.com"))
    }

    // MARK: - Login

    func testLogin_success() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.url?.path, "/api/v1/auth/login")
            XCTAssertEqual(request.httpMethod, "POST")
            // Should NOT have auth header (login is unauthenticated)
            XCTAssertNil(request.value(forHTTPHeaderField: "Authorization"))

            let json = """
            {
                "token": "new-token",
                "requires_org_selection": false,
                "host": {"id":"h1","name":"Jane","email":"j@a.com","slug":"jane","timezone":"UTC","smart_durations":false,"is_admin":false},
                "tenant": {"id":"t1","name":"Acme","slug":"acme"}
            }
            """.data(using: .utf8)!
            let response = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, json)
        }

        let resp = try await client.login(email: "j@a.com", password: "password")
        XCTAssertEqual(resp.token, "new-token")
        XCTAssertFalse(resp.requiresOrgSelection)
        XCTAssertEqual(resp.host?.name, "Jane")
    }

    func testLogin_invalidCredentials() async {
        MockURLProtocol.requestHandler = { request in
            let json = """
            {"error": "invalid email or password"}
            """.data(using: .utf8)!
            let response = HTTPURLResponse(url: request.url!, statusCode: 401, httpVersion: nil, headerFields: nil)!
            return (response, json)
        }

        do {
            _ = try await client.login(email: "bad@a.com", password: "wrong")
            XCTFail("Expected error")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("Unexpected error: \(error)")
        }
    }

    // MARK: - Me

    func testMe_bearerTokenSent() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer test-token")
            XCTAssertEqual(request.httpMethod, "GET")

            let json = """
            {
                "host": {"id":"h1","name":"Jane","email":"j@a.com","slug":"jane","timezone":"UTC","smart_durations":false,"is_admin":false},
                "tenant": {"id":"t1","name":"Acme","slug":"acme"}
            }
            """.data(using: .utf8)!
            let response = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, json)
        }

        let me = try await client.me()
        XCTAssertEqual(me.host.id, "h1")
        XCTAssertEqual(me.tenant.slug, "acme")
    }

    // MARK: - Today bookings

    func testTodayBookings() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.url?.path, "/api/v1/bookings/today")

            let json = """
            {
                "bookings": [
                    {
                        "id": "b1", "template_name": "Chat", "status": "confirmed",
                        "start_time": "2026-04-16T10:00:00Z", "end_time": "2026-04-16T10:30:00Z",
                        "duration": 30, "invitee_name": "Bob", "invitee_email": "bob@b.com",
                        "invitee_timezone": "UTC", "conference_link": "https://meet.google.com/abc",
                        "location_type": "google_meet", "created_at": "2026-04-15T10:00:00Z"
                    }
                ]
            }
            """.data(using: .utf8)!
            let response = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, json)
        }

        let bookings = try await client.todayBookings()
        XCTAssertEqual(bookings.count, 1)
        XCTAssertEqual(bookings[0].id, "b1")
        XCTAssertEqual(bookings[0].status, .confirmed)
    }

    // MARK: - Pending bookings

    func testPendingBookings_empty() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.url?.path, "/api/v1/bookings/pending")
            let json = "{\"bookings\": []}".data(using: .utf8)!
            let response = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, json)
        }

        let bookings = try await client.pendingBookings()
        XCTAssertTrue(bookings.isEmpty)
    }

    // MARK: - Approve

    func testApproveBooking() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.url?.path, "/api/v1/bookings/b1/approve")
            XCTAssertEqual(request.httpMethod, "POST")

            let json = """
            {
                "booking": {
                    "id": "b1", "template_name": "Chat", "status": "confirmed",
                    "start_time": "2026-04-16T10:00:00Z", "end_time": "2026-04-16T10:30:00Z",
                    "duration": 30, "invitee_name": "Bob", "invitee_email": "bob@b.com",
                    "invitee_timezone": "UTC", "conference_link": "",
                    "location_type": "google_meet", "created_at": "2026-04-15T10:00:00Z"
                }
            }
            """.data(using: .utf8)!
            let response = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, json)
        }

        let booking = try await client.approveBooking(id: "b1")
        XCTAssertEqual(booking.status, .confirmed)
    }

    // MARK: - Reject

    func testRejectBooking() async throws {
        MockURLProtocol.requestHandler = { request in
            XCTAssertEqual(request.url?.path, "/api/v1/bookings/b1/reject")

            // Verify reason was sent in body
            if let body = request.httpBody,
               let dict = try? JSONSerialization.jsonObject(with: body) as? [String: String] {
                XCTAssertEqual(dict["reason"], "schedule conflict")
            }

            let json = "{\"status\": \"rejected\"}".data(using: .utf8)!
            let response = HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!
            return (response, json)
        }

        try await client.rejectBooking(id: "b1", reason: "schedule conflict")
    }

    // MARK: - 401 handling

    func testUnauthorized_throwsAPIError() async {
        MockURLProtocol.requestHandler = { request in
            let json = "{\"error\": \"unauthorized\"}".data(using: .utf8)!
            let response = HTTPURLResponse(url: request.url!, statusCode: 401, httpVersion: nil, headerFields: nil)!
            return (response, json)
        }

        do {
            _ = try await client.todayBookings()
            XCTFail("Expected unauthorized error")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("Unexpected error: \(error)")
        }
    }

    // MARK: - Server error

    func testServerError_400() async {
        MockURLProtocol.requestHandler = { request in
            let json = "{\"error\": \"booking is not pending\"}".data(using: .utf8)!
            let response = HTTPURLResponse(url: request.url!, statusCode: 400, httpVersion: nil, headerFields: nil)!
            return (response, json)
        }

        do {
            _ = try await client.approveBooking(id: "b1")
            XCTFail("Expected error")
        } catch let error as APIError {
            XCTAssertEqual(error, .server("booking is not pending"))
        } catch {
            XCTFail("Unexpected error: \(error)")
        }
    }
}
