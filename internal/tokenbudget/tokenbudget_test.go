package tokenbudget

import "testing"

func TestEstimateTextIsCJKSafe(t *testing.T) {
	chinese := EstimateText("这是一个需要长期执行的软件工程任务")
	latin := EstimateText("this is a long running software engineering task")
	if chinese < 17 {
		t.Fatalf("CJK estimate is unsafe: %d", chinese)
	}
	if latin <= 0 || latin >= len("this is a long running software engineering task") {
		t.Fatalf("unexpected latin estimate: %d", latin)
	}
}
