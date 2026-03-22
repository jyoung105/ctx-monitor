package claude

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func benchmarkTestdataPath(name string) string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "testdata", "claude", name)
}

func writeScaledFixtureFile(b *testing.B, name string, targetBytes int) string {
	b.Helper()

	src, err := os.ReadFile(benchmarkTestdataPath(name))
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}
	if len(src) == 0 {
		b.Fatal("fixture is empty")
	}

	repeat := targetBytes / len(src)
	if targetBytes%len(src) != 0 {
		repeat++
	}
	if repeat < 1 {
		repeat = 1
	}

	data := strings.Repeat(string(src), repeat)
	path := filepath.Join(b.TempDir(), name)
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		b.Fatalf("write scaled fixture: %v", err)
	}
	return path
}

func BenchmarkParseSession(b *testing.B) {
	cases := []struct {
		name        string
		fixture     string
		targetBytes int
	}{
		{name: "Simple256KB", fixture: "session-simple.jsonl", targetBytes: 256 * 1024},
		{name: "Simple1MB", fixture: "session-simple.jsonl", targetBytes: 1 * 1024 * 1024},
		{name: "Compaction1MB", fixture: "session-compaction.jsonl", targetBytes: 1 * 1024 * 1024},
		{name: "Compaction8MB", fixture: "session-compaction.jsonl", targetBytes: 8 * 1024 * 1024},
	}

	for _, tc := range cases {
		path := writeScaledFixtureFile(b, tc.fixture, tc.targetBytes)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(tc.targetBytes))
			for i := 0; i < b.N; i++ {
				sess, err := ParseSession(path)
				if err != nil {
					b.Fatalf("ParseSession: %v", err)
				}
				if len(sess.Messages) == 0 {
					b.Fatal("expected parsed messages")
				}
			}
		})
	}
}

func BenchmarkParseSessionSummary(b *testing.B) {
	cases := []struct {
		name        string
		fixture     string
		targetBytes int
	}{
		{name: "Simple256KB", fixture: "session-simple.jsonl", targetBytes: 256 * 1024},
		{name: "Simple1MB", fixture: "session-simple.jsonl", targetBytes: 1 * 1024 * 1024},
		{name: "Compaction1MB", fixture: "session-compaction.jsonl", targetBytes: 1 * 1024 * 1024},
		{name: "Compaction8MB", fixture: "session-compaction.jsonl", targetBytes: 8 * 1024 * 1024},
	}

	for _, tc := range cases {
		path := writeScaledFixtureFile(b, tc.fixture, tc.targetBytes)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(tc.targetBytes))
			for i := 0; i < b.N; i++ {
				sess, err := ParseSessionSummary(path)
				if err != nil {
					b.Fatalf("ParseSessionSummary: %v", err)
				}
				if len(sess.Turns) == 0 {
					b.Fatal("expected summary turns")
				}
			}
		})
	}
}
