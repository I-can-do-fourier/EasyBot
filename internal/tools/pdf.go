package tools

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"unicode/utf16"
	"unicode/utf8"

	"easybot/internal/llm"
	"easybot/internal/security"
	pdfapi "github.com/pdfcpu/pdfcpu/pkg/api"
	pdfcpu "github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	pdfmodel "github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

const (
	defaultPDFMaxBytes = 32000
	maxPDFOutputBytes  = 200000
	maxPDFFileBytes    = 20 << 20
)

var disablePDFCPUConfigDir sync.Once

type readPdfTool struct {
	policy security.PathPolicy
	audit  *security.AuditLog
}

func NewReadPdfTool(policy security.PathPolicy, audit *security.AuditLog) Tool {
	return readPdfTool{policy: policy, audit: audit}
}

func (t readPdfTool) Spec() llm.ToolSpec {
	return llm.ToolSpec{
		Type: "function",
		Function: llm.ToolFunction{
			Name:        "read_pdf",
			Description: "Extract plain text from a text-based PDF file up to a bounded number of bytes. Does not OCR scanned PDFs.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":      map[string]any{"type": "string"},
					"max_bytes": map[string]any{"type": "integer", "minimum": 1, "maximum": maxPDFOutputBytes},
				},
				"required": []string{"path"},
			},
		},
	}
}

func (t readPdfTool) SafetySummary(raw string) (bool, string) {
	return true, "read-only PDF extraction"
}

func (t readPdfTool) Run(ctx context.Context, raw string) Result {
	var args struct {
		Path     string `json:"path"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return Result{Error: err.Error()}
	}
	if args.MaxBytes <= 0 || args.MaxBytes > maxPDFOutputBytes {
		args.MaxBytes = defaultPDFMaxBytes
	}

	path, err := t.policy.Resolve(args.Path)
	if err != nil {
		return Result{Error: err.Error()}
	}
	info, err := os.Stat(path)
	if err != nil {
		return Result{Error: err.Error()}
	}
	if info.IsDir() {
		return Result{Error: "path is a directory"}
	}
	if info.Size() > maxPDFFileBytes {
		return Result{Error: fmt.Sprintf("PDF is too large: %d bytes (max %d)", info.Size(), maxPDFFileBytes)}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return Result{Error: err.Error()}
	}

	text, truncated, err := extractPDFText(ctx, data, args.MaxBytes)
	if err != nil {
		return Result{Error: err.Error()}
	}
	t.audit.Write("read_pdf", "ok", path)
	return Result{
		OK: true,
		Data: map[string]any{
			"path":      path,
			"content":   text,
			"truncated": truncated,
		},
	}
}

func extractPDFText(ctx context.Context, data []byte, maxBytes int) (string, bool, error) {
	if maxBytes <= 0 || maxBytes > maxPDFOutputBytes {
		maxBytes = defaultPDFMaxBytes
	}
	if !looksLikePDF(data) {
		return "", false, errors.New("file is not a PDF")
	}

	disablePDFCPUConfigDir.Do(pdfapi.DisableConfigDir)
	conf := pdfmodel.NewDefaultConfiguration()
	conf.Cmd = pdfmodel.EXTRACTCONTENT
	conf.Optimize = false

	pdfCtx, err := pdfapi.ReadValidateAndOptimize(bytes.NewReader(data), conf)
	if err != nil {
		return "", false, err
	}

	var out pdfTextBuffer
	out.buf.maxBytes = maxBytes
	foundText := false

	for page := 1; page <= pdfCtx.PageCount; page++ {
		if err := ctx.Err(); err != nil {
			return "", false, err
		}

		r, err := pdfcpu.ExtractPageContent(pdfCtx, page)
		if err != nil {
			return "", false, err
		}
		if r == nil {
			continue
		}

		content, err := io.ReadAll(r)
		if err != nil {
			return "", false, err
		}
		if out.buf.buf.Len() > 0 {
			out.newline()
		}
		if appendPDFText(content, &out) {
			foundText = true
		}
		if out.buf.truncated {
			break
		}
	}

	text := strings.TrimSpace(out.buf.String())
	if !foundText || text == "" {
		return "", false, errors.New("no extractable text found in PDF")
	}
	return text, out.buf.truncated, nil
}

func looksLikePDF(data []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(data), []byte("%PDF-"))
}

type pdfTokenKind int

const (
	pdfTokenWord pdfTokenKind = iota
	pdfTokenString
	pdfTokenArray
)

type pdfToken struct {
	kind pdfTokenKind
	text string
}

func appendPDFText(data []byte, out *pdfTextBuffer) bool {
	tokens := tokenizePDFContent(data)
	inTextObject := false
	found := false
	lastString := ""
	lastArray := ""

	for _, tok := range tokens {
		switch tok.kind {
		case pdfTokenString:
			lastString = tok.text
		case pdfTokenArray:
			lastArray = tok.text
		case pdfTokenWord:
			switch tok.text {
			case "BT":
				inTextObject = true
			case "ET":
				if inTextObject {
					out.newline()
				}
				inTextObject = false
				lastString = ""
				lastArray = ""
			case "Tj":
				if inTextObject && out.appendText(lastString) {
					found = true
				}
				lastString = ""
			case "'", "\"":
				if inTextObject && out.appendText(lastString) {
					found = true
				}
				if inTextObject {
					out.newline()
				}
				lastString = ""
			case "TJ":
				if inTextObject && out.appendText(lastArray) {
					found = true
				}
				lastArray = ""
			case "Td", "TD", "T*":
				if inTextObject {
					out.newline()
				}
			}
		}
		if out.buf.truncated {
			return found
		}
	}
	return found
}

func tokenizePDFContent(data []byte) []pdfToken {
	tokens := make([]pdfToken, 0, 64)
	for i := 0; i < len(data); {
		switch {
		case isPDFWhiteSpace(data[i]):
			i++
		case data[i] == '%':
			for i < len(data) && data[i] != '\n' && data[i] != '\r' {
				i++
			}
		case data[i] == '(':
			text, next, err := parsePDFLiteralString(data, i)
			if err != nil {
				i++
				continue
			}
			tokens = append(tokens, pdfToken{kind: pdfTokenString, text: text})
			i = next
		case data[i] == '<' && i+1 < len(data) && data[i+1] != '<':
			text, next, err := parsePDFHexString(data, i)
			if err != nil {
				i++
				continue
			}
			tokens = append(tokens, pdfToken{kind: pdfTokenString, text: text})
			i = next
		case data[i] == '[':
			text, next := parsePDFArrayText(data, i)
			tokens = append(tokens, pdfToken{kind: pdfTokenArray, text: text})
			i = next
		default:
			start := i
			for i < len(data) && !isPDFDelimiter(data[i]) && !isPDFWhiteSpace(data[i]) {
				i++
			}
			if start == i {
				i++
				continue
			}
			tokens = append(tokens, pdfToken{kind: pdfTokenWord, text: string(data[start:i])})
		}
	}
	return tokens
}

func parsePDFArrayText(data []byte, start int) (string, int) {
	var builder strings.Builder
	for i := start + 1; i < len(data); {
		switch {
		case isPDFWhiteSpace(data[i]):
			i++
		case data[i] == ']':
			return builder.String(), i + 1
		case data[i] == '(':
			text, next, err := parsePDFLiteralString(data, i)
			if err != nil {
				return builder.String(), i + 1
			}
			builder.WriteString(text)
			i = next
		case data[i] == '<' && i+1 < len(data) && data[i+1] != '<':
			text, next, err := parsePDFHexString(data, i)
			if err != nil {
				return builder.String(), i + 1
			}
			builder.WriteString(text)
			i = next
		default:
			i++
		}
	}
	return builder.String(), len(data)
}

func parsePDFLiteralString(data []byte, start int) (string, int, error) {
	var out []byte
	depth := 1
	for i := start + 1; i < len(data); i++ {
		switch data[i] {
		case '\\':
			if i+1 >= len(data) {
				return decodePDFTextBytes(out), len(data), nil
			}
			next := data[i+1]
			switch next {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '(', ')', '\\':
				out = append(out, next)
			case '\n':
			case '\r':
				if i+2 < len(data) && data[i+2] == '\n' {
					i++
				}
			default:
				if next >= '0' && next <= '7' {
					value := int(next - '0')
					consumed := 1
					for consumed < 3 && i+1+consumed < len(data) {
						ch := data[i+1+consumed]
						if ch < '0' || ch > '7' {
							break
						}
						value = value*8 + int(ch-'0')
						consumed++
					}
					out = append(out, byte(value))
					i += consumed
					continue
				}
				out = append(out, next)
			}
			i++
		case '(':
			depth++
			out = append(out, '(')
		case ')':
			depth--
			if depth == 0 {
				return decodePDFTextBytes(out), i + 1, nil
			}
			out = append(out, ')')
		default:
			out = append(out, data[i])
		}
	}
	return decodePDFTextBytes(out), len(data), errors.New("unterminated PDF string")
}

func parsePDFHexString(data []byte, start int) (string, int, error) {
	end := start + 1
	hexBytes := make([]byte, 0, 32)
	for end < len(data) {
		if data[end] == '>' {
			break
		}
		if !isPDFWhiteSpace(data[end]) {
			hexBytes = append(hexBytes, data[end])
		}
		end++
	}
	if end >= len(data) {
		return "", len(data), errors.New("unterminated PDF hex string")
	}
	if len(hexBytes)%2 == 1 {
		hexBytes = append(hexBytes, '0')
	}
	decoded := make([]byte, hex.DecodedLen(len(hexBytes)))
	if _, err := hex.Decode(decoded, hexBytes); err != nil {
		return "", end + 1, err
	}
	return decodePDFTextBytes(decoded), end + 1, nil
}

func decodePDFTextBytes(data []byte) string {
	if len(data) >= 2 {
		switch {
		case data[0] == 0xFE && data[1] == 0xFF:
			return decodeUTF16(data[2:], true)
		case data[0] == 0xFF && data[1] == 0xFE:
			return decodeUTF16(data[2:], false)
		}
	}
	if utf8.Valid(data) {
		return string(data)
	}
	runes := make([]rune, 0, len(data))
	for _, b := range data {
		runes = append(runes, rune(b))
	}
	return string(runes)
}

func decodeUTF16(data []byte, bigEndian bool) string {
	if len(data)%2 == 1 {
		data = data[:len(data)-1]
	}
	words := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
		if bigEndian {
			words = append(words, uint16(data[i])<<8|uint16(data[i+1]))
		} else {
			words = append(words, uint16(data[i+1])<<8|uint16(data[i]))
		}
	}
	return string(utf16.Decode(words))
}

func isPDFWhiteSpace(b byte) bool {
	switch b {
	case 0x00, '\t', '\n', '\f', '\r', ' ':
		return true
	default:
		return false
	}
}

func isPDFDelimiter(b byte) bool {
	switch b {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	default:
		return false
	}
}

type pdfTextBuffer struct {
	buf limitedBuffer
}

func (b *pdfTextBuffer) appendText(text string) bool {
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		return false
	}
	if b.buf.buf.Len() > 0 {
		last := b.lastByte()
		if last != '\n' && last != ' ' {
			_, _ = b.buf.Write([]byte(" "))
		}
	}
	_, _ = b.buf.Write([]byte(text))
	return true
}

func (b *pdfTextBuffer) newline() {
	if b.buf.buf.Len() == 0 {
		return
	}
	if b.lastByte() == '\n' {
		return
	}
	_, _ = b.buf.Write([]byte("\n"))
}

func (b *pdfTextBuffer) lastByte() byte {
	raw := b.buf.buf.Bytes()
	if len(raw) == 0 {
		return 0
	}
	return raw[len(raw)-1]
}
