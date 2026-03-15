package tools

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/I-can-do-fourier/EasyBot/internal/security"
)

func TestReadPdfToolExtractsPlainText(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.pdf")
	if err := os.WriteFile(path, validPDF(t, []byte("BT\n/F1 12 Tf\n72 720 Td\n(Hello PDF) Tj\nT*\n[(Line ) 120 (Two)] TJ\nET\n"), false), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewReadPdfTool(security.NewPathPolicy([]string{root}), security.NewAuditLog(nil))
	result := tool.Run(context.Background(), mustJSON(t, map[string]any{"path": path}))
	if !result.OK {
		t.Fatalf("unexpected error: %s", result.Error)
	}

	data := result.Data.(map[string]any)
	content := data["content"].(string)
	if !strings.Contains(content, "Hello PDF") {
		t.Fatalf("expected extracted text to include first line, got %q", content)
	}
	if !strings.Contains(content, "Line Two") {
		t.Fatalf("expected extracted text to include second line, got %q", content)
	}
	if data["truncated"].(bool) {
		t.Fatal("did not expect output to be truncated")
	}
}

func TestReadPdfToolExtractsFlateEncodedText(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "compressed.pdf")
	if err := os.WriteFile(path, validPDF(t, []byte("BT\n/F1 12 Tf\n72 720 Td\n<FEFF00480069> Tj\nET\n"), true), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewReadPdfTool(security.NewPathPolicy([]string{root}), security.NewAuditLog(nil))
	result := tool.Run(context.Background(), mustJSON(t, map[string]any{"path": path}))
	if !result.OK {
		t.Fatalf("unexpected error: %s", result.Error)
	}

	content := result.Data.(map[string]any)["content"].(string)
	if !strings.Contains(content, "Hi") {
		t.Fatalf("expected extracted text to include decoded UTF-16 text, got %q", content)
	}
}

func TestReadPdfToolTruncatesOutput(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "long.pdf")
	if err := os.WriteFile(path, validPDF(t, []byte("BT\n/F1 12 Tf\n72 720 Td\n("+strings.Repeat("A", 128)+") Tj\nET\n"), false), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewReadPdfTool(security.NewPathPolicy([]string{root}), security.NewAuditLog(nil))
	result := tool.Run(context.Background(), mustJSON(t, map[string]any{"path": path, "max_bytes": 16}))
	if !result.OK {
		t.Fatalf("unexpected error: %s", result.Error)
	}

	data := result.Data.(map[string]any)
	content := data["content"].(string)
	if len(content) != 16 {
		t.Fatalf("expected 16 bytes of text, got %d", len(content))
	}
	if !data["truncated"].(bool) {
		t.Fatal("expected truncated output")
	}
}

func TestReadPdfToolRejectsNonPDF(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := NewReadPdfTool(security.NewPathPolicy([]string{root}), security.NewAuditLog(nil))
	result := tool.Run(context.Background(), mustJSON(t, map[string]any{"path": path}))
	if result.OK {
		t.Fatal("expected non-PDF file to fail")
	}
	if !strings.Contains(result.Error, "not a PDF") {
		t.Fatalf("unexpected error: %s", result.Error)
	}
}

func TestRegistryIncludesReadPdfTool(t *testing.T) {
	root := t.TempDir()
	reg := NewRegistry(
		security.NewPathPolicy([]string{root}),
		security.DefaultCommandPolicy(),
		security.NewAuditLog(nil),
		32000,
	)

	found := false
	for _, spec := range reg.Specs() {
		if spec.Function.Name == "read_pdf" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected read_pdf tool to be registered")
	}
}

func validPDF(t *testing.T, content []byte, flateEncoded bool) []byte {
	t.Helper()

	stream := content
	if flateEncoded {
		var compressed bytes.Buffer
		writer := zlib.NewWriter(&compressed)
		if _, err := writer.Write(content); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		stream = compressed.Bytes()
	}

	objects := [][]byte{
		[]byte("<< /Type /Catalog /Pages 2 0 R >>"),
		[]byte("<< /Type /Pages /Count 1 /Kids [3 0 R] >>"),
		[]byte("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>"),
		[]byte("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"),
	}

	var contentObject bytes.Buffer
	fmt.Fprintf(&contentObject, "<< /Length %d", len(stream))
	if flateEncoded {
		contentObject.WriteString(" /Filter /FlateDecode")
	}
	contentObject.WriteString(" >>\nstream\n")
	contentObject.Write(stream)
	contentObject.WriteString("\nendstream")
	objects = append(objects, contentObject.Bytes())

	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.4\n")

	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		offsets[i+1] = pdf.Len()
		fmt.Fprintf(&pdf, "%d 0 obj\n", i+1)
		pdf.Write(object)
		pdf.WriteString("\nendobj\n")
	}

	xrefOffset := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n", len(objects)+1)
	pdf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&pdf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xrefOffset)

	return pdf.Bytes()
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
