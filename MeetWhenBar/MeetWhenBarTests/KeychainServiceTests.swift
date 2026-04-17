import XCTest
@testable import MeetWhenBar

final class KeychainServiceTests: XCTestCase {

    override func tearDown() {
        // Clean up after each test
        KeychainService.clearAll()
        super.tearDown()
    }

    func testSaveAndLoad() throws {
        try KeychainService.save("my-token-123", for: .sessionToken)
        let loaded = KeychainService.load(.sessionToken)
        XCTAssertEqual(loaded, "my-token-123")
    }

    func testLoadMissing_returnsNil() {
        KeychainService.delete(.sessionToken)
        XCTAssertNil(KeychainService.load(.sessionToken))
    }

    func testSaveOverwrites() throws {
        try KeychainService.save("first", for: .sessionToken)
        try KeychainService.save("second", for: .sessionToken)
        XCTAssertEqual(KeychainService.load(.sessionToken), "second")
    }

    func testDelete() throws {
        try KeychainService.save("token", for: .sessionToken)
        KeychainService.delete(.sessionToken)
        XCTAssertNil(KeychainService.load(.sessionToken))
    }

    func testClearAll() throws {
        try KeychainService.save("token", for: .sessionToken)
        try KeychainService.save("https://example.com", for: .serverURL)

        KeychainService.clearAll()

        XCTAssertNil(KeychainService.load(.sessionToken))
        XCTAssertNil(KeychainService.load(.serverURL))
    }

    func testDifferentKeysAreIndependent() throws {
        try KeychainService.save("my-token", for: .sessionToken)
        try KeychainService.save("https://meet.example.com", for: .serverURL)

        XCTAssertEqual(KeychainService.load(.sessionToken), "my-token")
        XCTAssertEqual(KeychainService.load(.serverURL), "https://meet.example.com")

        KeychainService.delete(.sessionToken)
        XCTAssertNil(KeychainService.load(.sessionToken))
        XCTAssertEqual(KeychainService.load(.serverURL), "https://meet.example.com")
    }
}
