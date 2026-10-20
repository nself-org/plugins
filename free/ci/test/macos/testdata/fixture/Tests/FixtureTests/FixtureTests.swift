import XCTest
@testable import Fixture

final class FixtureTests: XCTestCase {
    func testAdd() {
        XCTAssertEqual(fixtureSum(2, 3), 5)
    }
}
