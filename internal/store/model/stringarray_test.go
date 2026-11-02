package model_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/kubev2v/migration-planner/internal/store/model"
)

var _ = Describe("String Array Value", func() {
	tests := []struct {
		name    string
		input   model.StringArray
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
			input: model.StringArray{},
			want:  "{}",
		},
		{
			name:  "single element",
			input: model.StringArray{"hello"},
			want:  `{"hello"}`,
		},
		{
			name:  "multiple elements",
			input: model.StringArray{"a", "b", "c"},
			want:  `{"a","b","c"}`,
		},
		{
			name:  "element with comma",
			input: model.StringArray{"a,b", "c"},
			want:  `{"a,b","c"}`,
		},
		{
			name:  "element with double quotes",
			input: model.StringArray{`say "hello"`, "world"},
			want:  `{"say \"hello\"","world"}`,
		},
		{
			name:  "element with backslash",
			input: model.StringArray{`path\to\file`},
			want:  `{"path\\to\\file"}`,
		},
	}

	for _, tt := range tests {
		It(tt.name, func() {
			got, err := tt.input.Value()
			Expect(err).To(Succeed(), "Value() error = %v", err)
			if tt.wantNil {
				Expect(got).To(BeNil(), "Value() = %v, want nil", got)
				return
			}
			Expect(got).To(Equal(tt.want), "Value() = %v, want %v", got, tt.want)
		})
	}
})

var _ = Describe("String Array Scan", func() {
	tests := []struct {
		name    string
		input   any
		want    model.StringArray
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
			want:  model.StringArray{},
		},
		{
			name:  "empty string",
			input: "",
			want:  model.StringArray{},
		},
		{
			name:  "single element",
			input: `{"hello"}`,
			want:  model.StringArray{"hello"},
		},
		{
			name:  "multiple elements",
			input: `{"a","b","c"}`,
			want:  model.StringArray{"a", "b", "c"},
		},
		{
			name:  "element with comma",
			input: `{"a,b","c"}`,
			want:  model.StringArray{"a,b", "c"},
		},
		{
			name:  "element with escaped quotes",
			input: `{"say \"hello\"","world"}`,
			want:  model.StringArray{`say "hello"`, "world"},
		},
		{
			name:  "element with escaped backslash",
			input: `{"path\\to\\file"}`,
			want:  model.StringArray{`path\to\file`},
		},
		{
			name:  "byte slice input",
			input: []byte(`{"x","y"}`),
			want:  model.StringArray{"x", "y"},
		},
		{
			name:  "trailing empty element",
			input: `{"a",""}`,
			want:  model.StringArray{"a", ""},
		},
		{
			name:  "single empty element",
			input: `{""}`,
			want:  model.StringArray{""},
		},
		{
			name:  "leading empty element",
			input: `{"","b"}`,
			want:  model.StringArray{"", "b"},
		},
	}

	for _, tt := range tests {
		It(tt.name, func() {
			var got model.StringArray
			err := got.Scan(tt.input)
			Expect(err).To(Succeed(), "Scan() error = %v", err)
			if tt.wantNil {
				Expect(got).To(BeNil(), "Scan() = %v, want nil", got)
				return
			}
			Expect(len(got)).To(Equal(len(tt.want)), "Scan() len = %d, want %d", len(got), len(tt.want))
			for i := range got {
				Expect(got[i]).To(Equal(tt.want[i]), "Scan()[%d] = %q, want %q", i, got[i], tt.want[i])
			}
		})
	}
})

var _ = Describe("String Array Scan Unsupported Type", func() {
	It("rejects unsupported input types", func() {
		var got model.StringArray
		err := got.Scan(123)
		Expect(err).To(HaveOccurred())

	})
})

var _ = Describe("String Array Round Trip", func() {
	tests := []struct {
		name  string
		input model.StringArray
	}{
		{
			name:  "simple values",
			input: model.StringArray{"microsegmentation", "multi_cloud"},
		},
		{
			name:  "values with special characters",
			input: model.StringArray{`say "hello"`, `path\to`, "a,b"},
		},
		{
			name:  "empty array",
			input: model.StringArray{},
		},
		{
			name:  "trailing empty element",
			input: model.StringArray{"a", ""},
		},
		{
			name:  "single empty element",
			input: model.StringArray{""},
		},
		{
			name:  "leading empty element",
			input: model.StringArray{"", "b"},
		},
	}

	for _, tt := range tests {
		It(tt.name, func() {
			v, err := tt.input.Value()
			Expect(err).To(Succeed(), "Value() error = %v", err)
			var got model.StringArray
			err = got.Scan(v)
			Expect(err).To(Succeed(), "Scan() error = %v", err)
			Expect(len(got)).To(Equal(len(tt.input)), "round-trip len = %d, want %d", len(got), len(tt.input))
			for i := range got {
				Expect(got[i]).To(Equal(tt.input[i]), "round-trip[%d] = %q, want %q", i, got[i], tt.input[i])
			}
		})
	}
})
