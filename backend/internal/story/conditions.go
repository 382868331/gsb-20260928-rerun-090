package story

import (
	"encoding/json"
	"fmt"
	"math"
)

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

// Eval reports whether the condition holds against state. A missing key
// evaluates to nil (numeric comparisons then fail rather than panic).
func (c *Condition) Eval(state map[string]any) (bool, error) {
	got := state[c.Key]
	want := c.Val
	switch c.Op {
	case OpEq:
		return equalVal(got, want), nil
	case OpNe:
		return !equalVal(got, want), nil
	case OpGt, OpGe, OpLt, OpLe:
		a, ok1 := toFloat(got)
		b, ok2 := toFloat(want)
		if !ok1 || !ok2 {
			return false, fmt.Errorf("condition %s %s: non-numeric operands", c.Key, c.Op)
		}
		switch c.Op {
		case OpGt:
			return a > b, nil
		case OpGe:
			return a >= b, nil
		case OpLt:
			return a < b, nil
		default:
			return a <= b, nil
		}
	default:
		return false, fmt.Errorf("unknown op %q", c.Op)
	}
}

func equalVal(a, b any) bool {
	if af, ok1 := toFloat(a); ok1 {
		if bf, ok2 := toFloat(b); ok2 {
			return math.Abs(af-bf) < 1e-12
		}
	}
	return a == b
}
