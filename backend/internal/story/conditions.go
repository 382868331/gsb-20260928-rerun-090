package story

import (
	"fmt"
	"sort"
	"strings"
)

var validOps = map[string]bool{
	"==": true, "!=": true,
	">": true, ">=": true, "<": true, "<=": true,
	"exists": true, "missing": true,
}

// ValidateCondition checks the structural validity of a condition tree.
func ValidateCondition(c *Condition) error {
	if c == nil {
		return nil
	}
	n := countConditionKinds(c)
	if n != 1 {
		return fmt.Errorf("condition must have exactly one of fact/all/any")
	}
	if c.Fact != "" {
		if !validOps[c.Op] {
			return fmt.Errorf("fact %q: unknown op %q", c.Fact, c.Op)
		}
		if c.Op == "exists" || c.Op == "missing" {
			if c.Value != nil {
				return fmt.Errorf("fact %q: op %s takes no value", c.Fact, c.Op)
			}
			return nil
		}
		if c.Value == nil {
			return fmt.Errorf("fact %q: op %s requires a value", c.Fact, c.Op)
		}
		switch c.Value.(type) {
		case bool:
			if c.Op != "==" && c.Op != "!=" {
				return fmt.Errorf("fact %q: bool supports only == and !=", c.Fact)
			}
		case string, float64, int, int64:
		default:
			return fmt.Errorf("fact %q: unsupported value type", c.Fact)
		}
		return nil
	}
	for i := range c.All {
		if err := ValidateCondition(&c.All[i]); err != nil {
			return err
		}
	}
	for i := range c.Any {
		if err := ValidateCondition(&c.Any[i]); err != nil {
			return err
		}
	}
	return nil
}

func countConditionKinds(c *Condition) int {
	n := 0
	if c.Fact != "" {
		n++
	}
	if len(c.All) > 0 {
		n++
	}
	if len(c.Any) > 0 {
		n++
	}
	return n
}

// Evaluate reports whether the condition holds for the given facts.
func Evaluate(c *Condition, facts map[string]interface{}) (bool, error) {
	if c == nil {
		return true, nil
	}
	if err := ValidateCondition(c); err != nil {
		return false, err
	}
	if c.Fact != "" {
		return evalAtom(c, facts[c.Fact]), nil
	}
	if c.All != nil {
		for i := range c.All {
			ok, err := Evaluate(&c.All[i], facts)
			if err != nil || !ok {
				return false, err
			}
		}
		return true, nil
	}
	for i := range c.Any {
		ok, err := Evaluate(&c.Any[i], facts)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func evalAtom(c *Condition, actual interface{}) bool {
	switch c.Op {
	case "exists":
		return actual != nil
	case "missing":
		return actual == nil
	case "==":
		return looseEqual(actual, c.Value)
	case "!=":
		return !looseEqual(actual, c.Value)
	}
	a, aok := toFloat(actual)
	b, bok := toFloat(c.Value)
	if !aok || !bok {
		return false
	}
	switch c.Op {
	case ">":
		return a > b
	case ">=":
		return a >= b
	case "<":
		return a < b
	case "<=":
		return a <= b
	}
	return false
}

func looseEqual(a, b interface{}) bool {
	if af, ok := toFloat(a); ok {
		if bf, ok := toFloat(b); ok {
			return af == bf
		}
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func toFloat(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	}
	return 0, false
}

// literal is a single ground constraint on a fact.
type literal struct {
	fact  string
	op    string
	value interface{}
}

// dnf converts a condition into disjunctive normal form: any-outer, all-inner.
func toDNF(c *Condition) ([][]literal, error) {
	if c == nil {
		return [][]literal{{}}, nil
	}
	if c.Fact != "" {
		return [][]literal{{{c.Fact, c.Op, c.Value}}}, nil
	}
	if c.All != nil {
		dnf := [][]literal{{}}
		for i := range c.All {
			sub, err := toDNF(&c.All[i])
			if err != nil {
				return nil, err
			}
			dnf = crossProduct(dnf, sub)
		}
		return dnf, nil
	}
	var out [][]literal
	for i := range c.Any {
		sub, err := toDNF(&c.Any[i])
		if err != nil {
			return nil, err
		}
		out = append(out, sub...)
	}
	return out, nil
}

func crossProduct(a, b [][]literal) [][]literal {
	var out [][]literal
	for _, x := range a {
		for _, y := range b {
			merged := make([]literal, 0, len(x)+len(y))
			merged = append(merged, x...)
			merged = append(merged, y...)
			out = append(out, merged)
		}
	}
	return out
}

func simplifyConjunction(lits []literal) []literal {
	seen := map[string]bool{}
	var out []literal
	for _, l := range lits {
		key := fmt.Sprintf("%s|%s|%v", l.fact, l.op, l.value)
		if !seen[key] {
			seen[key] = true
			out = append(out, l)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].fact < out[j].fact
	})
	return out
}

// conjunctionSatisfiable performs a conservative pairwise ground-literal check.
// exists/missing constraints are handled as unary; comparisons pairwise.
func conjunctionSatisfiable(lits []literal) bool {
	lits = simplifyConjunction(lits)
	present := map[string]bool{}
	missing := map[string]bool{}
	var ground []literal
	for _, l := range lits {
		switch l.op {
		case "exists":
			present[l.fact] = true
		case "missing":
			missing[l.fact] = true
		default:
			ground = append(ground, l)
		}
	}
	for k := range present {
		if missing[k] {
			return false
		}
	}
	for _, l := range ground {
		if missing[l.fact] && l.op != "missing" {
			return false
		}
	}
	for i := 0; i < len(ground); i++ {
		for j := i + 1; j < len(ground); j++ {
			if !literalsCompatible(ground[i], ground[j]) {
				return false
			}
		}
	}
	return true
}

func literalsCompatible(a, b literal) bool {
	if a.fact != b.fact {
		return true
	}
	// Equality / inequality against constants.
	if a.op == "==" {
		return literalAllowsValue(b, a.value)
	}
	if b.op == "==" {
		return literalAllowsValue(a, b.value)
	}
	if a.op == "!=" && b.op == "!=" {
		return true
	}
	af, aok := toFloat(a.value)
	bf, bok := toFloat(b.value)
	if !aok || !bok {
		// Non-numeric inequalities on the same fact could still conflict
		// (e.g. != "x" and != "x"), but duplicates were removed above.
		return true
	}
	switch {
	case a.op == ">=" && b.op == "<":
		return af < bf
	case a.op == "<" && b.op == ">=":
		return bf < af
	case a.op == "<=" && b.op == ">":
		return bf > af
	case a.op == ">" && b.op == "<=":
		return af > bf
	case a.op == ">" && b.op == "<":
		return af < bf
	case a.op == "<" && b.op == ">":
		return bf < af
	case a.op == ">" && b.op == "!=" || a.op == "!=" && b.op == ">":
		return true
	}
	return true
}

func literalAllowsValue(l literal, v interface{}) bool {
	switch l.op {
	case "==":
		return looseEqual(l.value, v)
	case "!=":
		return !looseEqual(l.value, v)
	}
	lf, lok := toFloat(l.value)
	vf, vok := toFloat(v)
	if !lok || !vok {
		return true
	}
	switch l.op {
	case ">":
		return vf > lf
	case ">=":
		return vf >= lf
	case "<":
		return vf < lf
	case "<=":
		return vf <= lf
	}
	return true
}

// ConditionsOverlap reports whether the two condition trees can ever be true
// for the same fact assignment. It is conservative: unknown cases report true.
func ConditionsOverlap(a, b *Condition) (bool, error) {
	da, err := toDNF(a)
	if err != nil {
		return true, err
	}
	db, err := toDNF(b)
	if err != nil {
		return true, err
	}
	for _, ca := range da {
		for _, cb := range db {
			merged := make([]literal, 0, len(ca)+len(cb))
			merged = append(merged, ca...)
			merged = append(merged, cb...)
			if conjunctionSatisfiable(merged) {
				return true, nil
			}
		}
	}
	return false, nil
}

// ConditionSatisfiable reports whether the condition can hold for some facts.
func ConditionSatisfiable(c *Condition) (bool, error) {
	if c == nil {
		return true, nil
	}
	dnf, err := toDNF(c)
	if err != nil {
		return false, err
	}
	for _, conj := range dnf {
		if conjunctionSatisfiable(conj) {
			return true, nil
		}
	}
	return false, nil
}

func condKey(c *Condition) string {
	b, _ := jsonMarshal(c)
	return string(b)
}

// ChoiceMutexReport lists pairs of choices on the same node whose conditions
// can be simultaneously satisfied.
type ChoiceMutexReport struct {
	Node       string   `json:"node"`
	ChoiceA    string   `json:"choiceA"`
	ChoiceB    string   `json:"choiceB"`
	Message    string   `json:"message"`
}

// CheckChoiceMutex returns overlapping conditional choice pairs.
func CheckChoiceMutex(def *Definition) []ChoiceMutexReport {
	var reports []ChoiceMutexReport
	nodeIDs := make([]string, 0, len(def.Nodes))
	for id := range def.Nodes {
		nodeIDs = append(nodeIDs, id)
	}
	sort.Strings(nodeIDs)
	for _, nid := range nodeIDs {
		n := def.Nodes[nid]
		for i := 0; i < len(n.Choices); i++ {
			for j := i + 1; j < len(n.Choices); j++ {
				a := n.Choices[i]
				b := n.Choices[j]
				if a.Condition == nil || b.Condition == nil {
					continue
				}
				overlap, err := ConditionsOverlap(a.Condition, b.Condition)
				if err != nil {
					continue
				}
				if overlap {
					reports = append(reports, ChoiceMutexReport{
						Node:    nid,
						ChoiceA: a.ID,
						ChoiceB: b.ID,
						Message: fmt.Sprintf("choices %s and %s can be available at the same time", a.ID, b.ID),
					})
				}
			}
		}
	}
	_ = strings.TrimSpace
	_ = condKey
	return reports
}
