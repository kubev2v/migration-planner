package model

import (
	"testing"
)

func TestStringArrayValue(t *testing.T) {
	tests := []struct {
		name    string
		input   StringArray
		want    string
		wantNil bool
	}{
		{
			name:    "nil array",
			input:   nil,
			wantNil: true,
		},
		{
			name:  "empty array",
			input: StringArray{},
			want:  "{}",
		},
		{
			name:  "single element",
			input: StringArray{"hello"},
			want:  `{"hello"}`,
		},
		{
			name:  "multiple elements",
			input: StringArray{"a", "b", "c"},
			want:  `{"a","b","c"}`,
		},
		{
			name:  "element with comma",
			input: StringArray{"a,b", "c"},
			want:  `{"a,b","c"}`,
		},
		{
			name:  "element with double quotes",
			input: StringArray{`say "hello"`, "world"},
			want:  `{"say \"hello\"","world"}`,
		},
		{
			name:  "element with backslash",
			input: StringArray{`path\to\file`},
			want:  `{"path\\to\\file"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.input.Value()
			if err != nil {
				t.Fatalf("Value() error = %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("Value() = %v, want nil", got)
				}
				return
			}
			if got != tt.want {
				t.Errorf("Value() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStringArrayScan(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    StringArray
		wantNil bool
	}{
		{
			name:    "nil input",
			input:   nil,
			wantNil: true,
		},
		{
			name:  "empty braces string",
			input: "{}",
			want:  StringArray{},
		},
		{
			name:  "empty string",
			input: "",
			want:  StringArray{},
		},
		{
			name:  "single element",
			input: `{"hello"}`,
			want:  StringArray{"hello"},
		},
		{
			name:  "multiple elements",
			input: `{"a","b","c"}`,
			want:  StringArray{"a", "b", "c"},
		},
		{
			name:  "element with comma",
			input: `{"a,b","c"}`,
			want:  StringArray{"a,b", "c"},
		},
		{
			name:  "element with escaped quotes",
			input: `{"say \"hello\"","world"}`,
			want:  StringArray{`say "hello"`, "world"},
		},
		{
			name:  "element with escaped backslash",
			input: `{"path\\to\\file"}`,
			want:  StringArray{`path\to\file`},
		},
		{
			name:  "byte slice input",
			input: []byte(`{"x","y"}`),
			want:  StringArray{"x", "y"},
		},
		{
			name:  "trailing empty element",
			input: `{"a",""}`,
			want:  StringArray{"a", ""},
		},
		{
			name:  "single empty element",
			input: `{""}`,
			want:  StringArray{""},
		},
		{
			name:  "leading empty element",
			input: `{"","b"}`,
			want:  StringArray{"", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got StringArray
			err := got.Scan(tt.input)
			if err != nil {
				t.Fatalf("Scan() error = %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Errorf("Scan() = %v, want nil", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("Scan() len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("Scan()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestStringArrayScanUnsupportedType(t *testing.T) {
	var got StringArray
	err := got.Scan(123)
	if err == nil {
		t.Fatal("Scan(int) should return error")
	}
}

func TestStringArrayRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input StringArray
	}{
		{
			name:  "simple values",
			input: StringArray{"microsegmentation", "multi_cloud"},
		},
		{
			name:  "values with special characters",
			input: StringArray{`say "hello"`, `path\to`, "a,b"},
		},
		{
			name:  "empty array",
			input: StringArray{},
		},
		{
			name:  "trailing empty element",
			input: StringArray{"a", ""},
		},
		{
			name:  "single empty element",
			input: StringArray{""},
		},
		{
			name:  "leading empty element",
			input: StringArray{"", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := tt.input.Value()
			if err != nil {
				t.Fatalf("Value() error = %v", err)
			}
			var got StringArray
			err = got.Scan(v)
			if err != nil {
				t.Fatalf("Scan() error = %v", err)
			}
			if len(got) != len(tt.input) {
				t.Fatalf("round-trip len = %d, want %d", len(got), len(tt.input))
			}
			for i := range got {
				if got[i] != tt.input[i] {
					t.Errorf("round-trip[%d] = %q, want %q", i, got[i], tt.input[i])
				}
			}
		})
	}
}
