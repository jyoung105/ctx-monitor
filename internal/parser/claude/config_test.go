package claude

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseMemoryConfig_UsesTTLCache(t *testing.T) {
	origTTL := memoryConfigCacheTTL
	origNow := claudeConfigParseNow
	origCache := memoryConfigCache
	memoryConfigCacheTTL = time.Hour
	currentTime := time.Date(2026, time.March, 22, 16, 0, 0, 0, time.UTC)
	claudeConfigParseNow = func() time.Time { return currentTime }
	memoryConfigCache = map[string]memoryConfigCacheEntry{}
	t.Cleanup(func() {
		memoryConfigCacheTTL = origTTL
		claudeConfigParseNow = origNow
		memoryConfigCache = origCache
	})

	home := t.TempDir()
	projectDir := filepath.Join(home, "project")
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir home claude: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(projectDir, "nested", ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir nested claude: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "CLAUDE.md"), []byte("global memory"), 0o644); err != nil {
		t.Fatalf("write global memory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "nested", ".claude", "CLAUDE.md"), []byte("nested memory"), 0o644); err != nil {
		t.Fatalf("write nested memory: %v", err)
	}

	first := parseMemoryConfig(projectDir, home)
	if len(first.Files) != 2 {
		t.Fatalf("first file count = %d, want 2", len(first.Files))
	}

	if err := os.Remove(filepath.Join(projectDir, "nested", ".claude", "CLAUDE.md")); err != nil {
		t.Fatalf("remove nested memory: %v", err)
	}
	second := parseMemoryConfig(projectDir, home)
	if len(second.Files) != 2 {
		t.Fatalf("cached file count = %d, want 2 within TTL", len(second.Files))
	}

	currentTime = currentTime.Add(2 * time.Hour)
	third := parseMemoryConfig(projectDir, home)
	if len(third.Files) != 1 {
		t.Fatalf("post-TTL file count = %d, want 1", len(third.Files))
	}
}

func TestFindSkillFiles_UsesTTLCache(t *testing.T) {
	origTTL := skillFileCacheTTL
	origNow := claudeConfigParseNow
	origCache := skillFileCache
	skillFileCacheTTL = time.Hour
	currentTime := time.Date(2026, time.March, 22, 16, 30, 0, 0, time.UTC)
	claudeConfigParseNow = func() time.Time { return currentTime }
	skillFileCache = map[string]skillFileCacheEntry{}
	t.Cleanup(func() {
		skillFileCacheTTL = origTTL
		claudeConfigParseNow = origNow
		skillFileCache = origCache
	})

	skillsDir := filepath.Join(t.TempDir(), ".claude", "skills")
	if err := os.MkdirAll(filepath.Join(skillsDir, "alpha"), 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "alpha", "SKILL.md"), []byte("---\nname: alpha\n---\nbody"), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}

	first := findSkillFiles(skillsDir)
	if len(first) != 1 {
		t.Fatalf("first skill count = %d, want 1", len(first))
	}

	if err := os.MkdirAll(filepath.Join(skillsDir, "beta"), 0o755); err != nil {
		t.Fatalf("mkdir second skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "beta", "SKILL.md"), []byte("---\nname: beta\n---\nbody"), 0o644); err != nil {
		t.Fatalf("write second skill file: %v", err)
	}

	second := findSkillFiles(skillsDir)
	if len(second) != 1 {
		t.Fatalf("cached skill count = %d, want 1 within TTL", len(second))
	}

	currentTime = currentTime.Add(2 * time.Hour)
	third := findSkillFiles(skillsDir)
	if len(third) != 2 {
		t.Fatalf("post-TTL skill count = %d, want 2", len(third))
	}
}
