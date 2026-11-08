import Foundation
@main struct P81SchoolFilesPolicyTests {
    static func main() {
        var count = 0
        func test(_ valid: Bool, _ message: String) {
            precondition(valid, "P81: " + message)
            count += 1
        }
        func item(_ path: Any) -> [String: Any] {
            ["id":31,"displayName":"Szkolny regulamin.pdf","downloadUrl":path]
        }
        let relative = DirectSchoolFilePolicy.parse(item("/pliki_szkoly/pobierz/31"))
        test(relative != nil, "relative path")
        let absolute = DirectSchoolFilePolicy.parse(item("https://synergia.librus.pl/pliki_szkoly/pobierz/31"))
        test(absolute == relative, "first-party absolute path")
        test((try? DirectSchoolFilePolicy.documentURL(absolute!))?.absoluteString ==
             "https://synergia.librus.pl/pliki_szkoly/pobierz/31", "canonical URL")
        for value in [
            "https://other.test/pliki_szkoly/pobierz/31",
            "http://synergia.librus.pl/pliki_szkoly/pobierz/31",
            "https://synergia.librus.pl:443/pliki_szkoly/pobierz/31",
            "https://synergia.librus.pl@evil.test/pliki_szkoly/pobierz/31",
            "https://synergia.librus.pl/pliki_szkoly/pobierz/32",
            "https://synergia.librus.pl/pliki_szkoly/pobierz/31?token=x",
            "https://synergia.librus.pl/pliki_szkoly/pobierz/31#frag",
            "https://synergia.librus.pl/pliki_szkoly/pobierz/31/../../private",
            "/pliki_szkoly/pobierz/31?token=x",
            "https://synergia.librus.pl/pliki_szkoly/pobierz/31/"
        ] {
            test(DirectSchoolFilePolicy.parse(item(value)) == nil, "untrusted URL")
        }
        test(DirectSchoolFilePolicy.parse(["id":31,"displayName":"Example.pdf"]) == nil,
             "never synthesize path")
        test(DirectSchoolFilePolicy.parse(item(NSNull())) == nil, "null path stays unavailable")
        print("P81 SchoolFiles policy: \(count)/\(count) PASS")
    }
}
