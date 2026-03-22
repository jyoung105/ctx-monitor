package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

func writeClaudeProjectsTree(b *testing.B, projectCount, sessionsPerProject int) (string, string) {
	b.Helper()

	src, err := os.ReadFile(benchmarkTestdataPath("session-simple.jsonl"))
	if err != nil {
		b.Fatalf("read source session: %v", err)
	}

	home := b.TempDir()
	base := filepath.Join(home, ".claude", "projects")
	targetCwd := ""

	for i := 0; i < projectCount; i++ {
		cwd := fmt.Sprintf("/Users/tester/project-%04d", i)
		if i == projectCount/2 {
			targetCwd = cwd
		}
		dir := filepath.Join(base, strings.ReplaceAll(cwd, "/", "-"))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			b.Fatalf("mkdir project dir: %v", err)
		}
		for j := 0; j < sessionsPerProject; j++ {
			name := filepath.Join(dir, fmt.Sprintf("session-%04d.jsonl", j))
			if err := os.WriteFile(name, src, 0o644); err != nil {
				b.Fatalf("write session file: %v", err)
			}
		}
	}

	return home, targetCwd
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

func BenchmarkParseSessionTimeline(b *testing.B) {
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
				sess, err := ParseSessionTimeline(path)
				if err != nil {
					b.Fatalf("ParseSessionTimeline: %v", err)
				}
				if len(sess.Messages) == 0 {
					b.Fatal("expected timeline messages")
				}
			}
		})
	}
}

func BenchmarkFindProjectDir(b *testing.B) {
	home, targetCwd := writeClaudeProjectsTree(b, 1000, 1)

	origTTL := projectDirCacheTTL
	origNow := claudeCacheNow
	projectDirCacheTTL = time.Hour
	claudeCacheNow = time.Now
	b.Cleanup(func() {
		projectDirCacheTTL = origTTL
		claudeCacheNow = origNow
		projectDirCacheMu.Lock()
		projectDirCache = map[string]projectDirCacheEntry{}
		projectDirCacheMu.Unlock()
	})

	b.Setenv("HOME", home)
	b.Run("cold", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			projectDirCacheMu.Lock()
			projectDirCache = map[string]projectDirCacheEntry{}
			projectDirCacheMu.Unlock()
			if got := FindProjectDir(targetCwd); got == "" {
				b.Fatal("expected project dir")
			}
		}
	})

	if got := FindProjectDir(targetCwd); got == "" {
		b.Fatal("expected warm project dir")
	}
	b.Run("warm", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if got := FindProjectDir(targetCwd); got == "" {
				b.Fatal("expected cached project dir")
			}
		}
	})
}

func BenchmarkFindAllSessions(b *testing.B) {
	home, targetCwd := writeClaudeProjectsTree(b, 10, 100)

	origTTL := sessionListCacheTTL
	origNow := claudeCacheNow
	sessionListCacheTTL = time.Hour
	claudeCacheNow = time.Now
	b.Cleanup(func() {
		sessionListCacheTTL = origTTL
		claudeCacheNow = origNow
		sessionListCacheMu.Lock()
		sessionListCache = map[string]sessionListCacheEntry{}
		sessionListCacheMu.Unlock()
	})

	b.Setenv("HOME", home)
	projectPath := filepath.Join(home, ".claude", "projects", strings.ReplaceAll(targetCwd, "/", "-"))

	b.Run("cold", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sessionListCacheMu.Lock()
			sessionListCache = map[string]sessionListCacheEntry{}
			sessionListCacheMu.Unlock()
			if sessions := FindAllSessions(projectPath); len(sessions) != 100 {
				b.Fatalf("session count = %d, want 100", len(sessions))
			}
		}
	})

	if sessions := FindAllSessions(projectPath); len(sessions) != 100 {
		b.Fatalf("session count = %d, want 100", len(sessions))
	}
	b.Run("warm", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if sessions := FindAllSessions(projectPath); len(sessions) != 100 {
				b.Fatalf("session count = %d, want 100", len(sessions))
			}
		}
	})
}
