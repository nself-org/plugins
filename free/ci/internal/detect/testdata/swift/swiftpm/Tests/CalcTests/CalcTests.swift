import XCTest
@testable import Calc

final class CalcTests: XCTestCase {
    func testTotal() {
        XCTAssertEqual(total(1, 2), 3)
    }
}
