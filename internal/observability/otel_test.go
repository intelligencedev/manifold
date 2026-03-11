package observability

import "testing"

func TestHasScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		want     bool
	}{
		{name: "bare host port", endpoint: "otel-collector:4318", want: false},
		{name: "http url", endpoint: "http://otel-collector:4318", want: true},
		{name: "https url", endpoint: "https://collector.example.com:4318", want: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := hasScheme(test.endpoint); got != test.want {
				t.Fatalf("hasScheme(%q) = %v, want %v", test.endpoint, got, test.want)
			}
		})
	}
}
