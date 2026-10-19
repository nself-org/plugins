import XCTest
@testable import App

final class AppTests: XCTestCase {
    func testAdd() {
        XCTAssertEqual(total(1, 2), 3)
    }
}
