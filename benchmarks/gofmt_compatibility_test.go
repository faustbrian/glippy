package benchmarks_test

import (
	"bytes"
	"go/format"
	"runtime"
	"testing"
)

func TestMotivatingLayoutsAreGofmtFixedPoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		source string
	}{
		{
			name: "expanded initializer and block",
			source: `package fixedpoint

func check(client Client, t T) {
	if _, err := client.Discover(nil); !errors.Is(err, ErrContextRequired) {
		t.Fatal(err)
	}
}
`,
		},
		{
			name: "split ordinary statements",
			source: `package fixedpoint

func run() {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result := work(ctx)
}
`,
		},
		{
			name: "broken boolean chain",
			source: `package fixedpoint

func condition() bool {
	return foo &&
		bar &&
		baz &&
		somethingReallyLong
}
`,
		},
		{
			name: "broken call with trailing comma",
			source: `package fixedpoint

func call() {
	result, err := client.executeContent(
		ctx,
		OperationInfo,
		http.MethodGet,
		"/",
		nil,
		"application/json",
		200,
	)
}
`,
		},
		{
			name: "expanded function literal",
			source: `package fixedpoint

func register() {
	handler := func(
		ctx context.Context,
		request Request,
	) (
		Result,
		error,
	) {
		return execute(
			ctx,
			request,
		)
	}
}
`,
		},
		{
			name: "broken selector chain",
			source: `package fixedpoint

func execute() {
	result := client.
		Service().
		Execute()
}
`,
		},
		{
			name: "broken type union",
			source: `package fixedpoint

type Scalar interface {
	~int |
		~int64 |
		~string
}
`,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				t.Parallel()
				formatted, err := format.Source([]byte(test.source))
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(formatted, []byte(test.source)) {
					t.Fatalf(
						"layout is not a gofmt fixed point under %s:\n%s",
						runtime.Version(),
						formatted,
					)
				}
			},
		)
	}
}
