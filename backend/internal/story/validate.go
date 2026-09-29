package story

import (
	"fmt"
	"sort"
)

// ValidateDefinition checks structural integrity and the condition rules:
//   - start node exists; every choice targets an existing node
//   - choice IDs are unique within a node
//   - gated conditions on a node's choices are pairwise provably mutually
//     exclusive (pairs that cannot be proven disjoint are rejected).
//     Unconditional choices are always-available player options and are not
//     subject to the rule.
func ValidateDefinition(d *Definition) error {
	if d == nil {
		return fmt.Errorf("nil definition")
	}
	if d.ID == "" {
		return fmt.Errorf("definition id is required")
	}
	if d.Version < 1 {
		return fmt.Errorf("definition version must be >= 1")
	}
	if _, ok := d.Nodes[d.Start]; !ok {
		return fmt.Errorf("start node %q does not exist", d.Start)
	}
	ids := make([]string, 0, len(d.Nodes))
	for id := range d.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		n := d.Nodes[id]
		if n.ID == "" {
			return fmt.Errorf("node with empty id")
		}
		if n.ID != id {
			return fmt.Errorf("node map key %q != node id %q", id, n.ID)
		}
		if err := validateNode(n, d); err != nil {
			return fmt.Errorf("node %q: %w", id, err)
		}
	}
	return nil
}

func validateNode(n Node, d *Definition) error {
	if n.Terminal && len(n.Choices) > 0 {
		return fmt.Errorf("terminal node cannot have choices")
	}
	seen := map[string]bool{}
	for _, ch := range n.Choices {
		if ch.ID == "" {
			return fmt.Errorf("choice with empty id")
		}
		if seen[ch.ID] {
			return fmt.Errorf("duplicate choice id %q", ch.ID)
		}
		seen[ch.ID] = true
		if _, ok := d.Nodes[ch.Next]; !ok {
			return fmt.Errorf("choice %q targets missing node %q", ch.ID, ch.Next)
		}
		for i, e := range ch.Effects {
			if err := validateEffect(e); err != nil {
				return fmt.Errorf("choice %q effect %d: %w", ch.ID, i, err)
			}
		}
	}
	for i, e := range n.Enter {
		if err := validateEffect(e); err != nil {
			return fmt.Errorf("enter effect %d: %w", i, err)
		}
	}
	for i := range n.Choices {
		for j := i + 1; j < len(n.Choices); j++ {
			if err := checkExclusion(n.Choices[i], n.Choices[j]); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateEffect(e Effect) error {
	if e.Key == "" {
		return fmt.Errorf("effect key is required")
	}
	modes := 0
	if e.Set != nil {
		modes++
	}
	if e.Add != nil {
		modes++
	}
	if e.Grant != "" {
		modes++
	}
	if modes != 1 {
		return fmt.Errorf("effect %q must set exactly one of set/add/grant", e.Key)
	}
	return nil
}

func checkExclusion(a, b Choice) error {
	// Unconditional choices are separate always-available options and do not
	// participate in condition exclusivity.
	if a.When == nil || b.When == nil {
		return nil
	}
	if provenDisjoint(*a.When, *b.When) {
		return nil
	}
	return fmt.Errorf("choices %q and %q: conditions are not provably mutually exclusive", a.ID, b.ID)
}

// provenDisjoint returns true only when the two single-key predicates are
// guaranteed never to hold at the same time. Anything uncertain is false.
func provenDisjoint(c1, c2 Condition) bool {
	if c1.Key != c2.Key {
		return false
	}
	if c1.Op == OpEq && c2.Op == OpEq {
		return !equalVal(c1.Val, c2.Val)
	}
	if (c1.Op == OpEq && c2.Op == OpNe) || (c1.Op == OpNe && c2.Op == OpEq) {
		eq := c1.Val
		ne := c2.Val
		if c1.Op == OpNe {
			eq, ne = c2.Val, c1.Val
		}
		return equalVal(eq, ne)
	}
	r1, ok1 := asRange(c1)
	r2, ok2 := asRange(c2)
	if ok1 && ok2 {
		return r1.before(r2) || r2.before(r1)
	}
	if c1.Op == OpEq {
		return pointOutsideRange(c1.Val, r2, ok2)
	}
	if c2.Op == OpEq {
		return pointOutsideRange(c2.Val, r1, ok1)
	}
	return false
}

// interval on the real line; -1 = open, 0 = closed boundary.
type interval struct {
	lo, hi         float64
	loOpen, hiOpen bool
	empty          bool
}

func (a interval) before(b interval) bool {
	if a.hi < b.lo {
		return true
	}
	if a.hi == b.lo && (a.hiOpen || b.loOpen) {
		return true
	}
	return false
}

func asRange(c Condition) (interval, bool) {
	v, ok := toFloat(c.Val)
	if !ok {
		return interval{}, false
	}
	switch c.Op {
	case OpGe:
		return interval{lo: v, hi: mathInf, loOpen: false, hiOpen: true}, true
	case OpGt:
		return interval{lo: v, hi: mathInf, loOpen: true, hiOpen: true}, true
	case OpLe:
		return interval{lo: negMathInf, hi: v, loOpen: true, hiOpen: false}, true
	case OpLt:
		return interval{lo: negMathInf, hi: v, loOpen: true, hiOpen: true}, true
	case OpEq:
		return interval{lo: v, hi: v}, true
	default:
		return interval{}, false
	}
}

const (
	mathInf    = 1e308
	negMathInf = -1e308
)

func pointOutsideRange(point any, r interval, ok bool) bool {
	if !ok {
		return false
	}
	v, ok := toFloat(point)
	if !ok {
		return false
	}
	if v < r.lo || v > r.hi {
		return true
	}
	if v == r.lo && r.loOpen {
		return true
	}
	if v == r.hi && r.hiOpen {
		return true
	}
	return false
}
