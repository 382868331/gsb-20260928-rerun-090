package story

// ptrFloat returns a pointer to v, used by Effect.Add in literals.
func ptrFloat(v float64) *float64 { return &v }

// SeedStory is the short verification story: it exercises mutually exclusive
// numeric conditions, branch convergence, mid-story saves and one-time
// rewards when revisiting a node.
func SeedStory() *Definition {
	return &Definition{
		ID:      "crossroads",
		Version: 1,
		Title:   "岔路口的一夜",
		Start:   "campfire",
		Nodes: map[string]Node{
			"campfire": {
				ID:    "campfire",
				Title: "篝火旁",
				Text:  "你在岔路口的篝火边醒来。行囊里只有 0 枚金币。左边是密林，右边是市集。",
				Enter: []Effect{{Key: "gold", Set: float64(0)}},
				Choices: []Choice{
					{ID: "go_market", Text: "去市集", Next: "market"},
					{ID: "go_forest", Text: "进密林", Next: "forest"},
				},
			},
			"market": {
				ID:    "market",
				Title: "清晨市集",
				Text:  "商队领队塞给你一笔跑腿钱（仅首次到访发放）。",
				Enter: []Effect{
					{Key: "gold", Add: ptrFloat(3), Once: true},
					{Key: "items", Grant: "coin_purse"},
				},
				Choices: []Choice{
					{ID: "to_gate", Text: "前往城门", Next: "gate"},
				},
			},
			"forest": {
				ID:    "forest",
				Title: "雾气密林",
				Text:  "你在树根下摸到一个钱袋（仅首次到访发放）。",
				Enter: []Effect{
					{Key: "gold", Add: ptrFloat(5), Once: true},
					{Key: "items", Grant: "forest_bag"},
				},
				Choices: []Choice{
					{ID: "to_gate", Text: "穿过林子到城门", Next: "gate"},
				},
			},
			"gate": {
				ID:    "gate",
				Title: "城门前",
				Text:  "两条路在城门前汇合。卫兵打量你的钱袋。",
				Choices: []Choice{
					{ID: "bribe_rich", Text: "掏钱买路（金币 >= 5）", Next: "city",
						When: &Condition{Key: "gold", Op: OpGe, Val: float64(5)}},
					{ID: "bribe_poor", Text: "恳求放行（金币 < 5）", Next: "beggar",
						When: &Condition{Key: "gold", Op: OpLt, Val: float64(5)}},
				},
			},
			"beggar": {
				ID:    "beggar",
				Title: "城门洞",
				Text:  "卫兵收下你仅有的一枚金币，让你蹲在门洞过夜。天快亮了。",
				Enter: []Effect{{Key: "gold", Add: ptrFloat(-1)}},
				Choices: []Choice{
					{ID: "back_to_camp", Text: "返回篝火再想办法", Next: "campfire"},
					{ID: "to_market", Text: "再去市集碰碰运气", Next: "market"},
					{ID: "sneak_in", Text: "天亮后混入运菜车入城", Next: "city"},
				},
			},
			"city": {
				ID:       "city",
				Title:    "入城",
				Text:     "你走进晨光中的城市。（完）",
				Terminal: true,
			},
		},
	}
}
