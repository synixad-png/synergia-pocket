package main

// v0.10.1 — regression suite for the original "downloaded without extension"
// report. These tests add no requirements to previously frozen modules.

import (
	"archive/zip"
	"bytes"
	"mime"
	"net/http"
	"strings"
	"testing"
)

func schoolTestResponse(contentType, disposition string) *http.Response {
	h := make(http.Header)
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	if disposition != "" {
		h.Set("Content-Disposition", disposition)
	}
	return &http.Response{StatusCode: 200, Header: h}
}

func TestV101SchoolNameWithoutExtensionUsesRealPDFSignature(t *testing.T) {
	r := schoolTestResponse("application/octet-stream", "")
	f, err := schoolBinary(r, []byte("%PDF-1.7\n%%EOF\n"), "Uchwała zarządu")
	if err != nil || f.Name != "Uchwała zarządu.pdf" {
		t.Fatalf("%+v %v", f, err)
	}
}

func TestV101SchoolFilenameFromContentDispositionIsPreferred(t *testing.T) {
	cases := []struct{ header, fallback, body, want string }{
		{`attachment; filename="Zaswiadczenie.pdf"`, "Dokument", "%PDF-1.4 sample", "Zaswiadczenie.pdf"},
		{`attachment; filename*=utf-8''Za%C5%82%C4%85cznik%20szkolny.pdf`, "Dokument", "%PDF-1.4 sample", "Załącznik szkolny.pdf"},
		{`attachment; filename="Materialy"`, "Dokument", "%PDF-1.4 sample", "Materialy.pdf"},
		{`attachment; filename="Bezpieczny"`, "Dokument", "opaque bytes", "Bezpieczny.bin"},
		{`attachment; filename="../../tajne.pdf"`, "Dokument", "%PDF-1.4 sample", "tajne.pdf"},
	}
	for _, tc := range cases {
		r := schoolTestResponse("application/octet-stream", tc.header)
		file, err := schoolBinary(r, []byte(tc.body), tc.fallback)
		if err != nil || file.Name != tc.want {
			t.Fatalf("header=%q name=%q want=%q err=%v", tc.header, file.Name, tc.want, err)
		}
	}
}

func TestV101KnownMimeTypeWithoutSignature(t *testing.T) {
	cases := []struct{ mime, name, want string }{
		{"application/pdf", "Plan na 2026", "Plan na 2026.pdf"},
		{"application/msword", "Wniosek", "Wniosek.doc"},
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", "Raport", "Raport.docx"},
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "Zestawienie", "Zestawienie.xlsx"},
		{"application/vnd.openxmlformats-officedocument.presentationml.presentation", "Slajdy", "Slajdy.pptx"},
		{"text/plain; charset=utf-8", "Informacja", "Informacja.txt"},
	}
	for _, tc := range cases {
		file, err := schoolBinary(schoolTestResponse(tc.mime, ""), []byte("opaque"), tc.name)
		if err != nil || file.Name != tc.want {
			t.Fatalf("type=%s result=%s want=%s err=%v", tc.mime, file.Name, tc.want, err)
		}
	}
}

func TestV101SignaturesRecognizeImageFormats(t *testing.T) {
	cases := []struct {
		bytes []byte
		want  string
	}{
		{[]byte{0xff, 0xd8, 0xff, 0xe0, 0, 0}, ".jpg"},
		{append([]byte("\x89PNG\r\n\x1a\n"), 0), ".png"},
		{[]byte("GIF89a000"), ".gif"},
		{[]byte("RIFF" + "0000" + "WEBP" + "VP8 "), ".webp"},
	}
	for _, tc := range cases {
		file, err := schoolBinary(schoolTestResponse("application/octet-stream", ""), tc.bytes, "Obraz")
		if err != nil || file.Name != "Obraz"+tc.want {
			t.Fatalf("name=%q want=%s err=%v", file.Name, tc.want, err)
		}
	}
}

func schoolTestZip(t *testing.T, names ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, name := range names {
		f, e := w.Create(name)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write([]byte("mock")); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func TestV101OOXMLRecognizedFromZipStructure(t *testing.T) {
	for _, tc := range []struct{ path, ext string }{
		{"word/document.xml", ".docx"}, {"xl/workbook.xml", ".xlsx"}, {"ppt/presentation.xml", ".pptx"},
	} {
		b := schoolTestZip(t, "[Content_Types].xml", tc.path)
		got, err := schoolBinary(schoolTestResponse("application/octet-stream", ""), b, "Szkolne materiały")
		if err != nil || got.Name != "Szkolne materiały"+tc.ext {
			t.Fatal(got.Name, err)
		}
	}
	b := schoolTestZip(t, "document.txt")
	got, e := schoolBinary(schoolTestResponse("application/octet-stream", ""), b, "Archiwum")
	if e != nil || got.Name != "Archiwum.zip" {
		t.Fatal(got.Name, e)
	}
}

func TestV101KeepKnownExtensionAndDoNotInventPDF(t *testing.T) {
	for _, tc := range []struct{ name, ext, want string }{
		{"Dokument.docx", ".pdf", "Dokument.docx"},
		{"Sprawozdanie.2026", ".pdf", "Sprawozdanie.2026.pdf"},
		{"Dokument.bin", ".pdf", "Dokument.pdf"},
		{"Dokument.dat", ".docx", "Dokument.docx"},
		{"Dokument", ".bin", "Dokument.bin"},
		{"Raport.PDF", ".pdf", "Raport.PDF"},
	} {
		if got := schoolNameWithExtension(tc.name, tc.ext); got != tc.want {
			t.Fatalf("%q + %s -> %q expected %q", tc.name, tc.ext, got, tc.want)
		}
	}
	unknown, err := schoolBinary(schoolTestResponse("application/octet-stream", ""), []byte("opaque data"), "Unknown")
	if err != nil || !strings.HasSuffix(unknown.Name, ".bin") {
		t.Fatal(unknown.Name, err)
	}
}

func TestV101CorrectContentDispositionHeaderOnLocalRoute(t *testing.T) {
	f, err := schoolBinary(schoolTestResponse("application/octet-stream", ""), []byte("%PDF-1.7 sample"), "Informacja dla rodziców")
	if err != nil {
		t.Fatal(err)
	}
	header := mime.FormatMediaType("attachment", map[string]string{"filename": f.Name})
	_, params, err := mime.ParseMediaType(header)
	if err != nil || params["filename"] != "Informacja dla rodziców.pdf" {
		t.Fatalf("%q %+v %v", header, params, err)
	}
}

func TestV101FrontendUsesRealResponseFileName(t *testing.T) {
	for _, fragment := range []string{"function schoolFilenameFromResponse(response,fallback)", "Content-Disposition", "schoolFilenameFromResponse(response,doc.name)"} {
		if !strings.Contains(htmlPage, fragment) {
			t.Fatal(fragment)
		}
	}
	if strings.Contains(htmlPage, "a.download=String(doc.name||'dokument.bin')") {
		t.Fatal("old bug still present")
	}
}
