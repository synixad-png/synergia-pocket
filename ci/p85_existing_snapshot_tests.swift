import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

// Contract tests with synthetic cookies only; never use real account tokens.
private func cookie(_ name: String = "oauth_token", value: String = "synthetic-token",
                    domain: String = "synergia.librus.pl", path: String = "/",
                    secure: Bool = true, expiry: Date? = nil,
                    httpOnly: Bool = true) -> HTTPCookie {
    var props: [HTTPCookiePropertyKey: Any] = [
        .name: name, .value: value, .domain: domain, .path: path
    ]
    if secure { props[.secure] = "TRUE" }
    if httpOnly { props[HTTPCookiePropertyKey("HttpOnly")] = "TRUE" }
    if let expiry { props[.expires] = expiry }
    guard let result = HTTPCookie(properties: props) else { fatalError("cookie construction") }
    return result
}

@main
struct CookieSnapshotRegressionTests {
    static func main() throws {
        var checked = 0
        func check(_ condition: @autoclosure () -> Bool, _ label: String) {
            guard condition() else { fatalError("FAILED: \(label)") }
            checked += 1
        }
        let now = Date(timeIntervalSince1970: 1_800_000_000)
        let valid = cookie()
        let mailbox = cookie("sessionid", value: "synthetic-mail", domain: "wiadomosci.librus.pl")
        let device = cookie("DeviceCookie", value: "synthetic-device", domain: "api.librus.pl")
        let safe = [valid, mailbox, device]
        let snapshot = DirectSessionCookieSnapshot.capture(safe, verifiedOwner: "account-123", now: now)
        check(snapshot != nil, "verified snapshot")
        check(snapshot?.owner == "account-123", "owner bound")
        check(snapshot?.cookies.count == 3, "multi-domain session retained")
        check(DirectSessionCookieSnapshot.capture(safe, verifiedOwner: "", now: now) == nil,
              "empty verified owner denied")
        let saved = try PropertyListEncoder().encode(snapshot!)
        let decoded = try PropertyListDecoder().decode(DirectSessionCookieSnapshot.self, from: saved)
        let restored = decoded.restoredCookies(now: now.addingTimeInterval(60))
        check(restored?.count == 3, "persisted roundtrip")
        check(restored?.first(where: { $0.name == "oauth_token" })?.value == "synthetic-token",
              "token preserved")
        check(restored?.contains(where: { $0.domain == "wiadomosci.librus.pl" }) == true,
              "mail host preserved")
        check(restored?.contains(where: { $0.name == "oauth_token" && $0.isSecure }) == true,
              "secure flag retained")
        check(restored?.contains(where: { $0.name == "oauth_token" && $0.isHTTPOnly }) == true,
              "HttpOnly flag retained")
        check(decoded.restoredCookies(now: now.addingTimeInterval(
            DirectSessionCookieSnapshot.lifetime + 1)) == nil, "stale snapshot refused")
        check(DirectSessionCookieSnapshot.capture([mailbox], verifiedOwner: "a", now: now) == nil,
              "unverified message-only session denied")
        check(DirectSessionCookieSnapshot.capture([cookie(secure: false)], verifiedOwner: "a", now: now) == nil,
              "insecure cookie excluded")
        check(DirectSessionCookieSnapshot.capture([cookie(domain: "evil.example")],
              verifiedOwner: "a", now: now) == nil, "unknown domain excluded")
        check(DirectSessionCookieSnapshot.capture([cookie(domain: "librus.pl.evil.example")],
              verifiedOwner: "a", now: now) == nil, "suffix attack excluded")
        check(DirectSessionCookieSnapshot.capture([cookie(expiry: now.addingTimeInterval(-10))],
              verifiedOwner: "a", now: now) == nil, "expired gateway cookie excluded")
        check(DirectSessionCookieSnapshot.capture([cookie(path: "/loguj")],
              verifiedOwner: "a", now: now) == nil, "wrong path gateway token refused")
        let withExpiry = cookie(expiry: now.addingTimeInterval(7200))
        let twoHours = DirectSessionCookieSnapshot.capture([withExpiry], verifiedOwner: "owner", now: now)
        check(twoHours?.restoredCookies(now: now.addingTimeInterval(7100))?.count == 1,
              "unexpired token restored")
        check(twoHours?.restoredCookies(now: now.addingTimeInterval(7201)) == nil,
              "provider-expired token refused")
        let many = [valid] + (0..<97).map { cookie("x\($0)", value: "a") }
        check(DirectSessionCookieSnapshot.capture(many, verifiedOwner: "owner", now: now) == nil,
              "unbounded cookie jar rejected")
        print("PASS DirectSessionCookieSnapshot: \(checked) synthetic assertions")
    }
}
