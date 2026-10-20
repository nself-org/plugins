import XCTest
@testable import Fixture

final class FixtureTests: XCTestCase {
    func testAdd() {
        XCTAssertEqual(add(2, 3), 5)
    }
}
