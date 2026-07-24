package health

import "testing"

func TestCompleteThreadSampleDistinguishesExactLimit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		sourceComplete bool
		total          int
		want           bool
	}{
		{name: "below limit", sourceComplete: true, total: threadListLimit - 1, want: true},
		{name: "exact limit", sourceComplete: true, total: threadListLimit, want: true},
		{name: "above limit", sourceComplete: true, total: threadListLimit + 1, want: false},
		{name: "incomplete source", sourceComplete: false, total: 1, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := completeThreadSample(tt.sourceComplete, tt.total, threadListLimit); got != tt.want {
				t.Fatalf("completeThreadSample() = %v, want %v", got, tt.want)
			}
		})
	}
}
