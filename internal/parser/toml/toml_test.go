package toml

import (
	"testing"
)

func TestParseSimpleString(t *testing.T) {
	result := Parse(`key = "value"`)
	v, ok := result["key"]
	if !ok {
		t.Fatal("key not found")
	}
	if s, ok := v.(string); !ok || s != "value" {
		t.Errorf("key = %v, want \"value\"", v)
	}
}

func TestParseInteger(t *testing.T) {
	result := Parse(`port = 8080`)
	v, ok := result["port"]
	if !ok {
		t.Fatal("port not found")
	}
	if n, ok := v.(int); !ok || n != 8080 {
		t.Errorf("port = %v (%T), want int 8080", v, v)
	}
}

func TestParseFloat(t *testing.T) {
	result := Parse(`ratio = 3.14`)
	v, ok := result["ratio"]
	if !ok {
		t.Fatal("ratio not found")
	}
	f, ok := v.(float64)
	if !ok {
		t.Fatalf("ratio type = %T, want float64", v)
	}
	if f < 3.13 || f > 3.15 {
		t.Errorf("ratio = %v, want ~3.14", f)
	}
}

func TestParseBoolean(t *testing.T) {
	result := Parse(`enabled = true`)
	v, ok := result["enabled"]
	if !ok {
		t.Fatal("enabled not found")
	}
	b, ok := v.(bool)
	if !ok || !b {
		t.Errorf("enabled = %v (%T), want bool true", v, v)
	}
}

func TestParseTable(t *testing.T) {
	result := Parse("[server]\nhost = \"localhost\"")
	srv, ok := result["server"]
	if !ok {
		t.Fatal("server table not found")
	}
	m, ok := srv.(map[string]interface{})
	if !ok {
		t.Fatalf("server = %T, want map", srv)
	}
	if h, ok := m["host"]; !ok || h != "localhost" {
		t.Errorf("server.host = %v, want \"localhost\"", h)
	}
}

func TestParseNestedTable(t *testing.T) {
	result := Parse("[db.primary]\nhost = \"db1\"")
	db, ok := result["db"]
	if !ok {
		t.Fatal("db not found")
	}
	dbMap, ok := db.(map[string]interface{})
	if !ok {
		t.Fatalf("db = %T, want map", db)
	}
	primary, ok := dbMap["primary"]
	if !ok {
		t.Fatal("db.primary not found")
	}
	primaryMap, ok := primary.(map[string]interface{})
	if !ok {
		t.Fatalf("db.primary = %T, want map", primary)
	}
	if h, ok := primaryMap["host"]; !ok || h != "db1" {
		t.Errorf("db.primary.host = %v, want \"db1\"", h)
	}
}

func TestParseArray(t *testing.T) {
	result := Parse(`ports = [80, 443, 8080]`)
	v, ok := result["ports"]
	if !ok {
		t.Fatal("ports not found")
	}
	arr, ok := v.([]interface{})
	if !ok {
		t.Fatalf("ports = %T, want []interface{}", v)
	}
	if len(arr) != 3 {
		t.Fatalf("ports len = %d, want 3", len(arr))
	}
	wantInts := []int{80, 443, 8080}
	for i, want := range wantInts {
		if n, ok := arr[i].(int); !ok || n != want {
			t.Errorf("ports[%d] = %v (%T), want int %d", i, arr[i], arr[i], want)
		}
	}
}

func TestParseQuotedStringWithEscapes(t *testing.T) {
	result := Parse(`msg = "hello\nworld"`)
	v, ok := result["msg"]
	if !ok {
		t.Fatal("msg not found")
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("msg = %T, want string", v)
	}
	if s != "hello\nworld" {
		t.Errorf("msg = %q, want %q", s, "hello\nworld")
	}
}

func TestParseComments(t *testing.T) {
	result := Parse("# this is a comment\nkey = \"val\"")
	v, ok := result["key"]
	if !ok {
		t.Fatal("key not found after comment")
	}
	if s, ok := v.(string); !ok || s != "val" {
		t.Errorf("key = %v, want \"val\"", v)
	}
	// comment line should not appear as a key
	if _, ok := result["# this is a comment"]; ok {
		t.Error("comment line incorrectly parsed as key")
	}
}

func TestParseEmpty(t *testing.T) {
	result := Parse("")
	if len(result) != 0 {
		t.Errorf("Parse(\"\") returned %d entries, want 0", len(result))
	}
}

func TestParseMultiLineArray(t *testing.T) {
	input := "ports = [\n  80,\n  443,\n  8080\n]"
	result := Parse(input)
	v, ok := result["ports"]
	if !ok {
		t.Fatal("ports not found")
	}
	arr, ok := v.([]interface{})
	if !ok {
		t.Fatalf("ports = %T, want []interface{}", v)
	}
	if len(arr) != 3 {
		t.Fatalf("ports len = %d, want 3", len(arr))
	}
}
