from pathlib import Path
import hashlib
root = Path('synergia-direct-ios')
source = root / 'Sources'
page = source / 'DirectPocketModuleScreen.swift'
s = page.read_text()
assert s.count('case .grades: DirectPocketGradesScreen()') == 1
s = s.replace('case .grades: DirectPocketGradesScreen()', 'case .grades: DirectPocketGradesScreen(gradeStore: portal.directGradeStore)', 1)
start = s.index('private struct DirectPocketGradesScreen: View {')
end = s.index('private struct DirectPocketTimetableScreen: View {', start)
grade = s[start:end]
assert grade.count('portal.directGradeStore') >= 9
assert grade.count('@EnvironmentObject private var portal: SynergiaPortal') == 1
grade = grade.replace('@EnvironmentObject private var portal: SynergiaPortal', '@EnvironmentObject private var portal: SynergiaPortal\n    // Observe independent unread state published by the grade store.\n    @ObservedObject var gradeStore: DirectGradeStore', 1)
grade = grade.replace('portal.directGradeStore', 'gradeStore')
s = s[:start] + grade + s[end:]
page.write_text(s)
files = source / 'DirectSchoolFiles.swift'
s = files.read_text()
marker = '    static func parse(_ item: [String: Any]) -> DirectSchoolFileReference? {'
assert s.count(marker) == 1
s = s.replace(marker, '''    /// The SchoolFiles record id and download URL token may differ.
    /// Only a server-provided canonical path on the official Synergia host is allowed.
    private static func verifiedDownloadPath(_ supplied: String) -> String? {
        let path: String
        if supplied.hasPrefix("https://") {
            guard let url = URL(string: supplied), url.scheme == "https",
                  url.host?.lowercased() == "synergia.librus.pl",
                  url.port == nil, url.user == nil, url.password == nil,
                  url.query == nil, url.fragment == nil,
                  URLComponents(url: url, resolvingAgainstBaseURL: false)?.percentEncodedPath == url.path else { return nil }
            path = url.path
        } else {
            path = supplied
        }
        guard path.hasPrefix(prefix),
              let token = identifier(String(path.dropFirst(prefix.count))),
              path == prefix + token else { return nil }
        return path
    }

''' + marker, 1)
assert 'link == prefix + id else { return nil }' in s
s = s.replace('link == prefix + id else { return nil }\n        return DirectSchoolFileReference(id: id, displayName: name, path: link)', 'let path = verifiedDownloadPath(link) else { return nil }\n        return DirectSchoolFileReference(id: id, displayName: name, path: path)', 1)
s = s.replace('guard file.path == prefix + file.id,\n              identifier(file.id) == file.id,', 'guard identifier(file.id) == file.id,\n              verifiedDownloadPath(file.path) == file.path,', 1)
s = s.replace('url.scheme == "https", url.host == "synergia.librus.pl", url.query == nil,', 'url.scheme == "https", url.host == "synergia.librus.pl", url.port == nil,\n              url.query == nil,', 1)
assert 'verifiedDownloadPath(file.path) == file.path' in s
files.write_text(s)
t = root / 'Tests/school_files_standalone.swift'
if not t.exists():
    t.write_text('''import Foundation
@main
enum P81SchoolFileSmoke {
    static func main() {
        let item: [String: Any] = ["id": "17613", "displayName": "Regulamin", "downloadUrl": "/pliki_szkoly/pobierz/1"]
        let file = DirectSchoolFilePolicy.parse(item)
        assert(file?.id == "17613" && file?.path == "/pliki_szkoly/pobierz/1")
        if let file = file {
            let url = try? DirectSchoolFilePolicy.documentURL(file)
            assert(url?.absoluteString == "https://synergia.librus.pl/pliki_szkoly/pobierz/1")
        }
        assert(DirectSchoolFilePolicy.parse(["id": "17613", "displayName": "Regulamin"]) == nil)
        assert(DirectSchoolFilePolicy.parse(["id": "17613", "displayName": "Regulamin", "downloadUrl": "https://evil.example/pliki_szkoly/pobierz/1"]) == nil)
        print("P81 SchoolFiles smoke: 4/4 PASS")
    }
}
''')
else:
    s = t.read_text()
    old = 'check("mismatched id rejected") {DirectSchoolFilePolicy.parse(fixture(31,"/pliki_szkoly/pobierz/32")) == nil}'
    assert s.count(old) == 1
    new = '''check("independent download token accepted") {
                guard let ref = DirectSchoolFilePolicy.parse(fixture(31,"/pliki_szkoly/pobierz/32")) else { return false }
                return (try DirectSchoolFilePolicy.documentURL(ref)).absoluteString ==
                    "https://synergia.librus.pl/pliki_szkoly/pobierz/32"
            }
            check("real SchoolFiles fixture accepted") {
                DirectSchoolFilePolicy.parse(fixture("17613", "/pliki_szkoly/pobierz/1", "Regulamin")) != nil
            }
            check("hostile external link refused") {
                DirectSchoolFilePolicy.parse(fixture(31,"https://evil.example/pliki_szkoly/pobierz/32")) == nil
            }
            check("missing link is not invented") {
                DirectSchoolFilePolicy.parse(["id": "17613", "displayName": "Regulamin"]) == nil
            }'''
    t.write_text(s.replace(old, new, 1))
for rel, expected in {
    'Sources/DirectPocketModuleScreen.swift': '072e903182638697671a6ea91b02f53cb50b624c58d7141cfa2b6c0287da9db6'
}.items():
    actual = hashlib.sha256((root / rel).read_bytes()).hexdigest()
    assert actual == expected, (rel, actual, expected)
print('P81 unread grade refresh and SchoolFiles safe independent link: PASS')
