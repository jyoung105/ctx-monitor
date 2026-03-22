package codex

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func benchmarkTestdataPath(name string) string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..", "..", "testdata", "codex", name)
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

func writeCodexSessionTree(b *testing.B, sessionCount int) string {
	b.Helper()

	src, err := os.ReadFile(benchmarkTestdataPath("rollout-simple.jsonl"))
	if err != nil {
		b.Fatalf("read source session: %v", err)
	}

	home := b.TempDir()
	sessionsDir := filepath.Join(home, "sessions")
	for i := 0; i < sessionCount; i++ {
		dir := filepath.Join(sessionsDir, "2026", "03", "22", string(rune('a'+(i%26))))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatalf("mkdir sessions dir: %v", err)
		}
		name := filepath.Join(dir, fmt.Sprintf("session-%04d-%s.jsonl", i, strings.Repeat("x", i%7)))
		if err := os.WriteFile(name, src, 0o644); err != nil {
			b.Fatalf("write session %d: %v", i, err)
		}
	}

	return home
}

func BenchmarkParseSession(b *testing.B) {
	cases := []struct {
		name        string
		targetBytes int
	}{
		{name: "256KB", targetBytes: 256 * 1024},
		{name: "1MB", targetBytes: 1 * 1024 * 1024},
		{name: "8MB", targetBytes: 8 * 1024 * 1024},
	}

	for _, tc := range cases {
		path := writeScaledFixtureFile(b, "rollout-simple.jsonl", tc.targetBytes)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(tc.targetBytes))
			for i := 0; i < b.N; i++ {
				sess, err := ParseSession(path)
				if err != nil {
					b.Fatalf("ParseSession: %v", err)
				}
				if sess.RawStats.LineCount == 0 {
					b.Fatal("expected parsed lines")
				}
			}
		})
	}
}

func BenchmarkParseSessionSummary(b *testing.B) {
	cases := []struct {
		name        string
		targetBytes int
	}{
		{name: "256KB", targetBytes: 256 * 1024},
		{name: "1MB", targetBytes: 1 * 1024 * 1024},
		{name: "8MB", targetBytes: 8 * 1024 * 1024},
	}

	for _, tc := range cases {
		path := writeScaledFixtureFile(b, "rollout-simple.jsonl", tc.targetBytes)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(tc.targetBytes))
			for i := 0; i < b.N; i++ {
				sess, err := ParseSessionSummary(path)
				if err != nil {
					b.Fatalf("ParseSessionSummary: %v", err)
				}
				if sess.RawStats.LineCount == 0 {
					b.Fatal("expected parsed lines")
				}
			}
		})
	}
}

func BenchmarkFindAllSessions(b *testing.B) {
	cases := []struct {
		name         string
		sessionCount int
	}{
		{name: "100", sessionCount: 100},
		{name: "1000", sessionCount: 1000},
	}

	for _, tc := range cases {
		home := writeCodexSessionTree(b, tc.sessionCount)
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			b.Setenv("CODEX_HOME", home)
			for i := 0; i < b.N; i++ {
				sessions, err := FindAllSessions()
				if err != nil {
					b.Fatalf("FindAllSessions: %v", err)
				}
				if len(sessions) != tc.sessionCount {
					b.Fatalf("session count = %d, want %d", len(sessions), tc.sessionCount)
				}
			}
		})
	}
}
