package story

import (
	"encoding/json"
	"sort"
)

// CompareRules reports only changes reachable from the instance boundary. The
// completed log is intentionally excluded: migration never rewrites history or
// replays rewards.
func CompareRules(oldDef, newDef *Definition, inst *Instance) []RuleChange {
	changes := []RuleChange{}
	reachable := reachableNodes(oldDef, inst.Current)
	nodes := make([]string, 0, len(reachable))
	for id := range reachable {
		nodes = append(nodes, id)
	}
	sort.Strings(nodes)
	for _, id := range nodes {
		oNode := oldDef.Nodes[id]
		nNode, ok := newDef.Nodes[id]
		if !ok {
			changes = append(changes, RuleChange{
				NodeID: id, Kind: "node", Impact: "该未来节点在新版本中不存在，必须明确选择接续节点，不能静默跳到默认位置。",
			})
			continue
		}
		for _, oldChoice := range oNode.Choices {
			var newChoice *Choice
			for i := range nNode.Choices {
				if nNode.Choices[i].ID == oldChoice.ID {
					newChoice = &nNode.Choices[i]
					break
				}
			}
			if newChoice == nil {
				changes = append(changes, RuleChange{
					NodeID: id, ChoiceID: oldChoice.ID, Kind: "target",
					Before: oldChoice.Target, Impact: "该未来选择已被移除；旧版本存档仍可保留原选择，采用新版后不能再选。",
				})
				continue
			}
			if jsonString(oldChoice.Condition) != jsonString(newChoice.Condition) {
				changes = append(changes, RuleChange{
					NodeID: id, ChoiceID: oldChoice.ID, Kind: "condition",
					Before: jsonString(oldChoice.Condition), After: jsonString(newChoice.Condition),
					Impact: "采用新版后，该未来选择会按当前状态重新判断可见和可提交条件。",
				})
			}
			if jsonString(oldChoice.Effects) != jsonString(newChoice.Effects) {
				changes = append(changes, RuleChange{
					NodeID: id, ChoiceID: oldChoice.ID, Kind: "rewards",
					Before: jsonString(oldChoice.Effects), After: jsonString(newChoice.Effects),
					Impact: "只有采用新版后的新选择使用新奖励；已经领取的奖励不会补发、扣回或重复发放。",
				})
			}
			if oldChoice.Target != newChoice.Target {
				changes = append(changes, RuleChange{
					NodeID: id, ChoiceID: oldChoice.ID, Kind: "target",
					Before: oldChoice.Target, After: newChoice.Target,
					Impact: "采用新版后，新做出的该选择进入新后续节点；已发生的跳转保持原样。",
				})
			}
		}
		for _, newChoice := range nNode.Choices {
			found := false
			for _, oldChoice := range oNode.Choices {
				if oldChoice.ID == newChoice.ID {
					found = true
				}
			}
			if !found {
				changes = append(changes, RuleChange{
					NodeID: id, ChoiceID: newChoice.ID, Kind: "target", After: newChoice.Target,
					Impact: "新版本增加了一个未来选择；只有采用新版的分支会看到并可能选择它。",
				})
			}
		}
	}
	return changes
}

func reachableNodes(d *Definition, start string) map[string]bool {
	seen := map[string]bool{}
	var walk func(id string)
	walk = func(id string) {
		if _, done := seen[id]; done {
			return
		}
		n, ok := d.Nodes[id]
		if !ok {
			return
		}
		seen[id] = true
		for _, ch := range n.Choices {
			walk(ch.Target)
		}
	}
	walk(start)
	return seen
}

func ReachableStarts(d *Definition) []string {
	var out []string
	for id, n := range d.Nodes {
		if n.Type != "end" {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func jsonString(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
