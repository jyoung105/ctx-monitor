package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	claudeparser "github.com/tonylee/ctx-monitor/internal/parser/claude"
)

func benchmarkRepoPath(parts ...string) string {
	_, filename, _, _ := runtime.Caller(0)
	all := append([]string{filepath.Dir(filename), "..", ".."}, parts...)
	return filepath.Join(all...)
}

func writeScaledBenchmarkFixture(b *testing.B, srcPath, outName string, targetBytes int) string {
	b.Helper()

	src, err := os.ReadFile(srcPath)
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

	path := filepath.Join(b.TempDir(), outName)
	if err := os.WriteFile(path, []byte(strings.Repeat(string(src), repeat)), 0o644); err != nil {
		b.Fatalf("write fixture: %v", err)
	}
	return path
}

func setupCodexBenchmarkEnv(b *testing.B, sessionPath string) cliArgs {
	b.Helper()

	home := b.TempDir()
	projectDir := filepath.Join(home, "project")
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		b.Fatalf("mkdir codex home: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".agents", "skills", "demo-skill"), 0o755); err != nil {
		b.Fatalf("mkdir skills dir: %v", err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		b.Fatalf("mkdir project dir: %v", err)
	}

	copyFiles := map[string]string{
		benchmarkRepoPath("testdata", "codex", "config.toml"): filepath.Join(home, ".codex", "config.toml"),
		benchmarkRepoPath("testdata", "codex", "AGENTS.md"):   filepath.Join(home, ".codex", "AGENTS.md"),
	}
	for src, dst := range copyFiles {
		data, err := os.ReadFile(src)
		if err != nil {
			b.Fatalf("read %s: %v", src, err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			b.Fatalf("write %s: %v", dst, err)
		}
	}

	skillBody := "---\nname: demo-skill\ndescription: benchmark skill\n---\n\n# Demo\nThis benchmark skill simulates realistic skill loading.\n"
	if err := os.WriteFile(filepath.Join(home, ".agents", "skills", "demo-skill", "SKILL.md"), []byte(skillBody), 0o644); err != nil {
		b.Fatalf("write skill: %v", err)
	}

	b.Setenv("HOME", home)
	b.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))

	return cliArgs{
		session: sessionPath,
		project: projectDir,
	}
}

func BenchmarkBuildCompositionCodex(b *testing.B) {
	cases := []struct {
		name        string
		targetBytes int
	}{
		{name: "256KB", targetBytes: 256 * 1024},
		{name: "1MB", targetBytes: 1 * 1024 * 1024},
	}

	srcPath := benchmarkRepoPath("testdata", "codex", "rollout-simple.jsonl")
	for _, tc := range cases {
		sessionPath := writeScaledBenchmarkFixture(b, srcPath, "rollout-simple.jsonl", tc.targetBytes)
		b.Run(tc.name, func(b *testing.B) {
			args := setupCodexBenchmarkEnv(b, sessionPath)
			b.ReportAllocs()
			b.SetBytes(int64(tc.targetBytes))
			for i := 0; i < b.N; i++ {
				comp := buildComposition("codex", args)
				if comp == nil || comp.TotalUsedTokens == 0 {
					b.Fatal("expected composition with usage")
				}
			}
		})
	}
}

func BenchmarkBuildTimelineData(b *testing.B) {
	srcPath := benchmarkRepoPath("testdata", "claude", "session-simple.jsonl")
	sessionPath := writeScaledBenchmarkFixture(b, srcPath, "session-simple.jsonl", 1*1024*1024)
	sess, err := claudeparser.ParseSession(sessionPath)
	if err != nil {
		b.Fatalf("ParseSession: %v", err)
	}
	if len(sess.Messages) == 0 {
		b.Fatal("expected parsed messages")
	}

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		data := buildTimelineData("bench-session", sess.Messages)
		if data == nil {
			b.Fatal("expected timeline data")
		}
	}
}
