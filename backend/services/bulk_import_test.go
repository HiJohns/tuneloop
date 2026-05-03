package services

import (
	"strings"
	"testing"
)

func TestParseCSV(t *testing.T) {
	input := "\xef\xbb\xbfname,age\nAlice,30\nBob,25\n\n"
	headers, records, err := ParseCSV(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}
	if len(headers) != 2 || headers[0] != "name" || headers[1] != "age" {
		t.Errorf("headers = %v, want [name age]", headers)
	}
	if len(records) != 2 {
		t.Fatalf("records len = %d, want 2", len(records))
	}
	if records[0].Fields["name"] != "Alice" || records[0].Fields["age"] != "30" {
		t.Errorf("first record = %v, want Alice/30", records[0].Fields)
	}
}

func TestDeduplicateRecords(t *testing.T) {
	records := []CSVRecord{
		{RowNum: 1, Fields: map[string]string{"email": "a@example.com", "name": "A1"}},
		{RowNum: 2, Fields: map[string]string{"email": "b@example.com", "name": "B"}},
		{RowNum: 3, Fields: map[string]string{"email": "a@example.com", "name": "A2"}},
	}
	result := DeduplicateRecords(records, "email")
	if len(result) != 2 {
		t.Fatalf("deduped len = %d, want 2", len(result))
	}
	// Last occurrence should win
	if result[1].Fields["name"] != "A2" {
		t.Errorf("last occurrence name = %q, want A2", result[1].Fields["name"])
	}
}

func TestValidateRequired(t *testing.T) {
	record := CSVRecord{Fields: map[string]string{"email": "test@example.com", "name": ""}}
	errs := ValidateRequired(record, []string{"email", "name"})
	if len(errs) != 1 {
		t.Fatalf("errors len = %d, want 1", len(errs))
	}
	if !strings.Contains(errs[0], "name") {
		t.Errorf("error = %q, want to contain 'name'", errs[0])
	}
}

func TestValidateEmail(t *testing.T) {
	if err := ValidateEmail("test@example.com"); err != nil {
		t.Errorf("ValidateEmail('test@example.com') = %v, want nil", err)
	}
	if err := ValidateEmail("invalid"); err == nil {
		t.Error("ValidateEmail('invalid') = nil, want error")
	}
	if err := ValidateEmail(""); err == nil {
		t.Error("ValidateEmail('') = nil, want error")
	}
}

func TestSplitTags(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"a|b|c", []string{"a", "b", "c"}},
		{"", []string{}},
		{"single", []string{"single"}},
		{"a | b", []string{"a", "b"}},
	}
	for _, tt := range tests {
		got := SplitTags(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("SplitTags(%q) = %v, want %v", tt.input, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("SplitTags(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
			}
		}
	}
}

func TestIsEmptyRow(t *testing.T) {
	if !isEmptyRow([]string{"", "", ""}) {
		t.Error("isEmptyRow(['', '', '']) = false, want true")
	}
	if isEmptyRow([]string{"", "a", ""}) {
		t.Error("isEmptyRow(['', 'a', '']) = true, want false")
	}
}
